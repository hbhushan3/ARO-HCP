package database

// Copyright (c) Microsoft Corporation.
// Licensed under the Apache License 2.0.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	azcorearm "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"

	"github.com/Azure/ARO-HCP/internal/api/arm"
)

const (
	resourcesContainer  = "Resources"
	subsContainer       = "Subscriptions"
	billingContainer    = "Billing"
	operationsContainer = "Operations"
	locksContainer      = "Locks"
)

var ErrNotFound = errors.New("DocumentNotFound")

func isResponseError(err error, statusCode int) bool {
	var responseError *azcore.ResponseError
	return errors.As(err, &responseError) && responseError.StatusCode == statusCode
}

// DBClient is a document store for frontend to perform required CRUD operations against
type DBClient interface {
	// DBConnectionTest is used to health check the database. If the database is not reachable or otherwise not ready
	// to be used, an error should be returned.
	DBConnectionTest(ctx context.Context) error

	// GetLockClient returns a LockClient, or nil if the DBClient does not support a LockClient.
	GetLockClient() *LockClient

	// GetResourceDoc retrieves a ResourceDocument from the database given its resourceID.
	// ErrNotFound is returned if an associated ResourceDocument cannot be found.
	GetResourceDoc(ctx context.Context, resourceID *arm.ResourceID) (*ResourceDocument, error)
	CreateResourceDoc(ctx context.Context, doc *ResourceDocument) error
	UpdateResourceDoc(ctx context.Context, resourceID *arm.ResourceID, callback func(*ResourceDocument) bool) (bool, error)
	// DeleteResourceDoc deletes a ResourceDocument from the database given the resourceID
	// of a Microsoft.RedHatOpenShift/HcpOpenShiftClusters resource or NodePools child resource.
	DeleteResourceDoc(ctx context.Context, resourceID *arm.ResourceID) error
	ListResourceDocs(ctx context.Context, prefix *arm.ResourceID, resourceType *azcorearm.ResourceType, pageSizeHint int32, continuationToken *string) ([]*ResourceDocument, *string, error)

	GetOperationDoc(ctx context.Context, operationID string) (*OperationDocument, error)
	CreateOperationDoc(ctx context.Context, doc *OperationDocument) error
	DeleteOperationDoc(ctx context.Context, operationID string) error

	// GetSubscriptionDoc retrieves a SubscriptionDocument from the database given the subscriptionID.
	// ErrNotFound is returned if an associated SubscriptionDocument cannot be found.
	GetSubscriptionDoc(ctx context.Context, subscriptionID string) (*SubscriptionDocument, error)
	CreateSubscriptionDoc(ctx context.Context, doc *SubscriptionDocument) error
	UpdateSubscriptionDoc(ctx context.Context, subscriptionID string, callback func(*SubscriptionDocument) bool) (bool, error)
}

var _ DBClient = &CosmosDBClient{}

// CosmosDBClient defines the needed values to perform CRUD operations against the async DB
type CosmosDBClient struct {
	client     *azcosmos.DatabaseClient
	lockClient *LockClient
}

// NewCosmosDBClient instantiates a Cosmos DatabaseClient targeting Frontends async DB
func NewCosmosDBClient(ctx context.Context, databaseClient *azcosmos.DatabaseClient) (DBClient, error) {
	// DatabaseClient.NewContainer only fails if the container ID is empty.
	lockContainerClient, _ := databaseClient.NewContainer(locksContainer)
	lockClient, err := NewLockClient(ctx, lockContainerClient)
	if err != nil {
		return nil, err
	}

	return &CosmosDBClient{
		client:     databaseClient,
		lockClient: lockClient,
	}, nil
}

// DBConnectionTest checks the async database is accessible on startup
func (d *CosmosDBClient) DBConnectionTest(ctx context.Context) error {
	if _, err := d.client.Read(ctx, nil); err != nil {
		return fmt.Errorf("failed to read Cosmos database information during healthcheck: %v", err)
	}

	return nil
}

func (d *CosmosDBClient) GetLockClient() *LockClient {
	return d.lockClient
}

// GetResourceDoc retrieves a resource document from the "resources" DB using resource ID
func (d *CosmosDBClient) GetResourceDoc(ctx context.Context, resourceID *arm.ResourceID) (*ResourceDocument, error) {
	// Make sure partition key is lowercase.
	pk := azcosmos.NewPartitionKeyString(strings.ToLower(resourceID.SubscriptionID))

	container, err := d.client.NewContainer(resourcesContainer)
	if err != nil {
		return nil, err
	}

	query := "SELECT * FROM c WHERE STRINGEQUALS(c.key, @key, true)"
	opt := azcosmos.QueryOptions{
		PageSizeHint:    1,
		QueryParameters: []azcosmos.QueryParameter{{Name: "@key", Value: resourceID.String()}},
	}

	queryPager := container.NewQueryItemsPager(query, pk, &opt)

	var doc *ResourceDocument
	for queryPager.More() {
		queryResponse, err := queryPager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, item := range queryResponse.Items {
			err = json.Unmarshal(item, &doc)
			if err != nil {
				return nil, err
			}
		}
	}
	if doc != nil {
		// Replace the key field from Cosmos with the given resourceID,
		// which typically comes from the URL. This helps preserve the
		// casing of the resource group and resource name from the URL
		// to meet RPC requirements:
		//
		// Put Resource | Arguments
		//
		// The resource group names and resource names should be matched
		// case insensitively. ... Additionally, the Resource Provier must
		// preserve the casing provided by the user. The service must return
		// the most recently specified casing to the client and must not
		// normalize or return a toupper or tolower form of the resource
		// group or resource name. The resource group name and resource
		// name must come from the URL and not the request body.
		doc.Key = resourceID
		return doc, nil
	}
	return nil, ErrNotFound
}

// CreateResourceDoc creates a resource document in the "resources" DB during resource creation
func (d *CosmosDBClient) CreateResourceDoc(ctx context.Context, doc *ResourceDocument) error {
	// Make sure partition key is lowercase.
	doc.PartitionKey = strings.ToLower(doc.PartitionKey)

	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	container, err := d.client.NewContainer(resourcesContainer)
	if err != nil {
		return err
	}

	_, err = container.CreateItem(ctx, azcosmos.NewPartitionKeyString(doc.PartitionKey), data, nil)
	if err != nil {
		return err
	}

	return nil
}

// UpdateResourceDoc updates a resource document by first fetching the document and passing it to
// the provided callback for modifications to be applied. It then attempts to replace the existing
// document with the modified document and an "etag" precondition. Upon a precondition failure the
// function repeats for a limited number of times before giving up.
//
// The callback function should return true if modifications were applied, signaling to proceed
// with the document replacement. The boolean return value reflects this: returning true if the
// document was sucessfully replaced, or false with or without an error to indicate no change.
func (d *CosmosDBClient) UpdateResourceDoc(ctx context.Context, resourceID *arm.ResourceID, callback func(*ResourceDocument) bool) (bool, error) {
	// Make sure partition key is lowercase.
	pk := azcosmos.NewPartitionKeyString(strings.ToLower(resourceID.SubscriptionID))

	container, err := d.client.NewContainer(resourcesContainer)
	if err != nil {
		return false, err
	}

	options := &azcosmos.ItemOptions{}

	for try := 0; try < 5; try++ {
		var doc *ResourceDocument
		var data []byte

		doc, err = d.GetResourceDoc(ctx, resourceID)
		if err != nil {
			return false, err
		}

		if !callback(doc) {
			return false, nil
		}

		data, err = json.Marshal(doc)
		if err != nil {
			return false, err
		}

		options.IfMatchEtag = &doc.ETag
		_, err = container.ReplaceItem(ctx, pk, doc.ID, data, options)

		var responseError *azcore.ResponseError
		if !errors.As(err, &responseError) || responseError.StatusCode != http.StatusPreconditionFailed {
			return (err == nil), err
		}
	}

	return false, err
}

// DeleteResourceDoc removes a resource document from the "resources" DB using resource ID
func (d *CosmosDBClient) DeleteResourceDoc(ctx context.Context, resourceID *arm.ResourceID) error {
	// Make sure partition key is lowercase.
	pk := azcosmos.NewPartitionKeyString(strings.ToLower(resourceID.SubscriptionID))

	doc, err := d.GetResourceDoc(ctx, resourceID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return fmt.Errorf("while attempting to delete the resource, failed to get resource document: %w", err)
	}

	container, err := d.client.NewContainer(resourcesContainer)
	if err != nil {
		return err
	}

	_, err = container.DeleteItem(ctx, pk, doc.ID, nil)
	if err != nil {
		return err
	}
	return nil
}

func (d *CosmosDBClient) ListResourceDocs(ctx context.Context, prefix *arm.ResourceID, resourceType *azcorearm.ResourceType, pageSizeHint int32, continuationToken *string) ([]*ResourceDocument, *string, error) {
	// Make sure partition key is lowercase.
	pk := azcosmos.NewPartitionKeyString(strings.ToLower(prefix.SubscriptionID))

	container, err := d.client.NewContainer(resourcesContainer)
	if err != nil {
		return nil, nil, err
	}

	query := "SELECT * FROM c WHERE STARTSWITH(c.key, @prefix, true)"
	opt := azcosmos.QueryOptions{
		PageSizeHint:      pageSizeHint,
		ContinuationToken: continuationToken,
		QueryParameters: []azcosmos.QueryParameter{
			{
				Name:  "@prefix",
				Value: prefix.String() + "/",
			},
		},
	}

	var response azcosmos.QueryItemsResponse
	resourceDocs := make([]*ResourceDocument, 0, pageSizeHint)

	// Loop until we fill the pre-allocated resourceDocs slice,
	// or until we run out of items from the resources container.
	for opt.PageSizeHint > 0 {
		response, err = container.NewQueryItemsPager(query, pk, &opt).NextPage(ctx)
		if err != nil {
			return nil, nil, err
		}

		for _, item := range response.Items {
			var doc ResourceDocument
			err = json.Unmarshal(item, &doc)
			if err != nil {
				return nil, nil, err
			}
			if resourceType == nil || strings.EqualFold(resourceType.String(), doc.Key.ResourceType.String()) {
				resourceDocs = append(resourceDocs, &doc)
			}
		}

		if response.ContinuationToken == nil {
			break
		}

		opt.PageSizeHint = int32(cap(resourceDocs) - len(resourceDocs))
		opt.ContinuationToken = response.ContinuationToken
	}

	return resourceDocs, response.ContinuationToken, nil
}

// GetOperationDoc retrieves the asynchronous operation document for the given
// operation ID from the "operations" container
func (d *CosmosDBClient) GetOperationDoc(ctx context.Context, operationID string) (*OperationDocument, error) {
	// Make sure lookup keys are lowercase.
	operationID = strings.ToLower(operationID)

	container, err := d.client.NewContainer(operationsContainer)
	if err != nil {
		return nil, err
	}

	pk := azcosmos.NewPartitionKeyString(operationID)

	response, err := container.ReadItem(ctx, pk, operationID, nil)
	if isResponseError(err, http.StatusNotFound) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}

	var doc *OperationDocument
	err = json.Unmarshal(response.Value, &doc)
	if err != nil {
		return nil, err
	}

	return doc, nil
}

// CreateOperationDoc writes an asynchronous operation document to the "operations"
// container
func (d *CosmosDBClient) CreateOperationDoc(ctx context.Context, doc *OperationDocument) error {
	container, err := d.client.NewContainer(operationsContainer)
	if err != nil {
		return err
	}

	pk := azcosmos.NewPartitionKeyString(doc.ID)

	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	_, err = container.CreateItem(ctx, pk, data, nil)
	if err != nil {
		return err
	}

	return nil
}

// DeleteOperationDoc deletes the asynchronous operation document for the given
// operation ID from the "operations" container
func (d *CosmosDBClient) DeleteOperationDoc(ctx context.Context, operationID string) error {
	// Make sure lookup keys are lowercase.
	operationID = strings.ToLower(operationID)

	container, err := d.client.NewContainer(operationsContainer)
	if err != nil {
		return err
	}

	pk := azcosmos.NewPartitionKeyString(operationID)

	_, err = container.DeleteItem(ctx, pk, operationID, nil)
	if isResponseError(err, http.StatusNotFound) {
		return ErrNotFound
	}

	return err
}

// GetSubscriptionDoc retreives a subscription document from async DB using the subscription ID
func (d *CosmosDBClient) GetSubscriptionDoc(ctx context.Context, subscriptionID string) (*SubscriptionDocument, error) {
	// Make sure lookup keys are lowercase.
	subscriptionID = strings.ToLower(subscriptionID)

	container, err := d.client.NewContainer(subsContainer)
	if err != nil {
		return nil, err
	}

	pk := azcosmos.NewPartitionKeyString(subscriptionID)

	response, err := container.ReadItem(ctx, pk, subscriptionID, nil)
	if isResponseError(err, http.StatusNotFound) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}

	var doc *SubscriptionDocument
	err = json.Unmarshal(response.Value, &doc)
	if err != nil {
		return nil, err
	}

	return doc, nil
}

// CreateSubscriptionDoc creates/updates a subscription document in the async DB during cluster creation/patching
func (d *CosmosDBClient) CreateSubscriptionDoc(ctx context.Context, doc *SubscriptionDocument) error {
	// Make sure lookup keys are lowercase.
	doc.ID = strings.ToLower(doc.ID)

	container, err := d.client.NewContainer(subsContainer)
	if err != nil {
		return err
	}

	pk := azcosmos.NewPartitionKeyString(doc.ID)

	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	_, err = container.CreateItem(ctx, pk, data, nil)

	return err
}

// UpdateSubscriptionDoc updates a subscription document by first fetching the document and
// passing it to the provided callback for modifications to be applied. It then attempts to
// replace the existing document with the modified document and an "etag" precondition. Upon
// a precondition failure the function repeats for a limited number of times before giving up.
//
// The callback function should return true if modifications were applied, signaling to proceed
// with the document replacement. The boolean return value reflects this: returning true if the
// document was successfully replaced, or false with or without an error to indicate no change.
func (d *CosmosDBClient) UpdateSubscriptionDoc(ctx context.Context, subscriptionID string, callback func(*SubscriptionDocument) bool) (bool, error) {
	// Make sure partition key is lowercase.
	pk := azcosmos.NewPartitionKeyString(strings.ToLower(subscriptionID))

	container, err := d.client.NewContainer(subsContainer)
	if err != nil {
		return false, err
	}

	options := &azcosmos.ItemOptions{}

	for try := 0; try < 5; try++ {
		var doc *SubscriptionDocument
		var data []byte

		doc, err = d.GetSubscriptionDoc(ctx, subscriptionID)
		if err != nil {
			return false, err
		}

		if !callback(doc) {
			return false, nil
		}

		data, err = json.Marshal(doc)
		if err != nil {
			return false, err
		}

		options.IfMatchEtag = &doc.ETag
		_, err = container.ReplaceItem(ctx, pk, doc.ID, data, options)

		var responseError *azcore.ResponseError
		if !errors.As(err, &responseError) || responseError.StatusCode != http.StatusPreconditionFailed {
			return (err == nil), err
		}
	}

	return false, err
}
