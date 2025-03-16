//go:build go1.18
// +build go1.18

// Code generated by Microsoft (R) AutoRest Code Generator (autorest: 3.10.4, generator: @autorest/go@4.0.0-preview.63)
// Changes may cause incorrect behavior and will be lost if the code is regenerated.
// Code generated by @autorest/go. DO NOT EDIT.

package fake

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/fake/server"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"

	"github.com/Azure/ARO-HCP/internal/api/v20240610preview/generated"
)

// HcpClusterVersionsServer is a fake server for instances of the generated.HcpClusterVersionsClient type.
type HcpClusterVersionsServer struct {
	// NewListPager is the fake for method HcpClusterVersionsClient.NewListPager
	// HTTP status codes to indicate success: http.StatusOK
	NewListPager func(location string, options *generated.HcpClusterVersionsClientListOptions) (resp azfake.PagerResponder[generated.HcpClusterVersionsClientListResponse])
}

// NewHcpClusterVersionsServerTransport creates a new instance of HcpClusterVersionsServerTransport with the provided implementation.
// The returned HcpClusterVersionsServerTransport instance is connected to an instance of generated.HcpClusterVersionsClient via the
// azcore.ClientOptions.Transporter field in the client's constructor parameters.
func NewHcpClusterVersionsServerTransport(srv *HcpClusterVersionsServer) *HcpClusterVersionsServerTransport {
	return &HcpClusterVersionsServerTransport{
		srv:          srv,
		newListPager: newTracker[azfake.PagerResponder[generated.HcpClusterVersionsClientListResponse]](),
	}
}

// HcpClusterVersionsServerTransport connects instances of generated.HcpClusterVersionsClient to instances of HcpClusterVersionsServer.
// Don't use this type directly, use NewHcpClusterVersionsServerTransport instead.
type HcpClusterVersionsServerTransport struct {
	srv          *HcpClusterVersionsServer
	newListPager *tracker[azfake.PagerResponder[generated.HcpClusterVersionsClientListResponse]]
}

// Do implements the policy.Transporter interface for HcpClusterVersionsServerTransport.
func (h *HcpClusterVersionsServerTransport) Do(req *http.Request) (*http.Response, error) {
	rawMethod := req.Context().Value(runtime.CtxAPINameKey{})
	method, ok := rawMethod.(string)
	if !ok {
		return nil, nonRetriableError{errors.New("unable to dispatch request, missing value for CtxAPINameKey")}
	}

	var resp *http.Response
	var err error

	switch method {
	case "HcpClusterVersionsClient.NewListPager":
		resp, err = h.dispatchNewListPager(req)
	default:
		err = fmt.Errorf("unhandled API %s", method)
	}

	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (h *HcpClusterVersionsServerTransport) dispatchNewListPager(req *http.Request) (*http.Response, error) {
	if h.srv.NewListPager == nil {
		return nil, &nonRetriableError{errors.New("fake for method NewListPager not implemented")}
	}
	newListPager := h.newListPager.get(req)
	if newListPager == nil {
		const regexStr = `/subscriptions/(?P<subscriptionId>[!#&$-;=?-\[\]_a-zA-Z0-9~%@]+)/locations/(?P<location>[!#&$-;=?-\[\]_a-zA-Z0-9~%@]+)/providers/Microsoft\.RedHatOpenShift/hcpOpenShiftVersions`
		regex := regexp.MustCompile(regexStr)
		matches := regex.FindStringSubmatch(req.URL.EscapedPath())
		if matches == nil || len(matches) < 2 {
			return nil, fmt.Errorf("failed to parse path %s", req.URL.Path)
		}
		locationParam, err := url.PathUnescape(matches[regex.SubexpIndex("location")])
		if err != nil {
			return nil, err
		}
		resp := h.srv.NewListPager(locationParam, nil)
		newListPager = &resp
		h.newListPager.add(req, newListPager)
		server.PagerResponderInjectNextLinks(newListPager, req, func(page *generated.HcpClusterVersionsClientListResponse, createLink func() string) {
			page.NextLink = to.Ptr(createLink())
		})
	}
	resp, err := server.PagerResponderNext(newListPager, req)
	if err != nil {
		return nil, err
	}
	if !contains([]int{http.StatusOK}, resp.StatusCode) {
		h.newListPager.remove(req)
		return nil, &nonRetriableError{fmt.Errorf("unexpected status code %d. acceptable values are http.StatusOK", resp.StatusCode)}
	}
	if !server.PagerResponderMore(newListPager) {
		h.newListPager.remove(req)
	}
	return resp, nil
}
