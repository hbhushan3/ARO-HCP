SHELL = /bin/bash
DEPLOY_ENV ?= personal-dev
PRINCIPAL_ID ?= $(shell az ad signed-in-user show -o json | jq -r '.id')
$(shell ./create-config.sh $(DEPLOY_ENV))
include config.mk

# Set SKIP_CONFIRM to a non-empty value to skip "what-if" confirmation prompts.
ifndef SKIP_CONFIRM
PROMPT_TO_CONFIRM = "--confirm-with-what-if"
endif

PERSIST ?= false

MGMT_KUBECONFIG_FILE ?= ${HOME}/.kube/${MGMT_RESOURCEGROUP}.kubeconfig
SVC_KUBECONFIG_FILE ?= ${HOME}/.kube/${SVC_RESOURCEGROUP}.kubeconfig

DEPLOYMENT_NAME_SUFFIX ?=
GLOBAL_RG_DEPLOYMENT_NAME ?= global${DEPLOYMENT_NAME_SUFFIX}
MGMG_RG_DEPLOYMENT_NAME ?= mgmt${DEPLOYMENT_NAME_SUFFIX}
SVC_RG_DEPLOYMENT_NAME ?= svc${DEPLOYMENT_NAME_SUFFIX}
REGIONAL_RG_DEPLOYMENT_NAME ?= region${DEPLOYMENT_NAME_SUFFIX}
METRICS_INFRA_RG_DEPLOYMENT_NAME ?= metrics-infra${DEPLOYMENT_NAME_SUFFIX}
ROLE_ASSIGNMENTS_DEPLOYMENT_NAME ?= role-assignments${DEPLOYMENT_NAME_SUFFIX}

list:
	@grep '^[^#[:space:]].*:' Makefile
.PHONY: list

modules := $(wildcard ./templates/*.bicep)
parameters := $(filter-out $(wildcard ./templates/*.tmpl.bicepparam),$(wildcard ./templates/*.bicepparam))

fmt: $(modules:.bicep=.bicep.fmt) $(parameters:.bicepparam=.biceparam.fmt)

lint: $(modules:.bicep=.bicep.lint) $(parameters:.bicepparam=.biceparam.lint)

%.bicep.fmt %.bicepparam.fmt:
	az bicep format --file $(basename $@)

%.bicep.lint %.bicepparam.lint:
	az bicep lint --file $(basename $@)

feature-registration: # hardcoded to eastus as this is a subscription deployment, not a resource group
	@az deployment sub create \
		--name "feature-registration" \
		--location eastus \
		--template-file templates/feature-registration.bicep \
		$(PROMPT_TO_CONFIRM)
.PHONY: feature-registration

cleanup-orphaned-rolebindings:
	@scripts/cleanup-orphaned-rolebindings.sh $(SVC_RESOURCEGROUP)
	@scripts/cleanup-orphaned-rolebindings.sh $(MGMT_RESOURCEGROUP)
	@scripts/cleanup-orphaned-rolebindings.sh $(REGIONAL_RESOURCEGROUP)
	@scripts/cleanup-orphaned-rolebindings.sh ${GLOBAL_RESOURCEGROUP}
.PHONY: cleanup-orphaned-rolebindings

create-mock-identities:
	az deployment group wait --created --name "aro-hcp-dev-mock-certificates" --resource-group $(GLOBAL_RESOURCEGROUP) --interval 10
	az deployment group create \
		--name "aro-hcp-dev-mock-certificates" \
		--resource-group $(GLOBAL_RESOURCEGROUP) \
		--template-file templates/mock-identities.bicep $(PROMPT_TO_CONFIRM) \
		--parameters configurations/mock-identities.bicepparam
	APPLICATION_NAME=aro-dev-first-party2 \
	KEY_VAULT_NAME=aro-hcp-dev-svc-kv \
	CERTIFICATE_NAME=firstPartyCert2 \
	ROLE_DEFINITION_NAME=dev-first-party-mock \
	SUBSCRIPTION_ID=$(shell az account show --query id --output tsv) \
	./scripts/create-sp-for-rbac.sh

	APPLICATION_NAME=aro-dev-arm-helper2 \
	KEY_VAULT_NAME=aro-hcp-dev-svc-kv \
	CERTIFICATE_NAME=armHelperCert2 \
	ROLE_DEFINITION_NAME='Role Based Access Control Administrator' \
	SUBSCRIPTION_ID=$(shell az account show --query id --output tsv) \
	./scripts/create-sp-for-rbac.sh

	APPLICATION_NAME=aro-dev-msi-mock2 \
	KEY_VAULT_NAME=aro-hcp-dev-svc-kv \
	CERTIFICATE_NAME=msiMockCert2 \
	ROLE_DEFINITION_NAME=dev-msi-mock \
	SUBSCRIPTION_ID=$(shell az account show --query id --output tsv) \
	./scripts/create-sp-for-rbac.sh

	# Assign additional roles to aro-dev-msi-mock2
	@for ROLE in "Managed Identity Contributor" "Network Contributor"; do \
	az role assignment create \
	--assignee "$(shell az ad sp list --display-name aro-dev-msi-mock2 --query [0].appId -o tsv)" \
	--scope "/subscriptions/$(shell az account show --query id --output tsv)" \
	--role "$$ROLE"; \
	done
.PHONY: create-mock-identities

#
# Global
#

global:
	../templatize.sh $(DEPLOY_ENV) -p global-pipeline.yaml -P run -c public
.PHONY: global

global.what-if:
	../templatize.sh $(DEPLOY_ENV) -p global-pipeline.yaml -P run -c public -d
.PHONY: global.what-if

#
# Region
#

region:
	@scripts/cleanup-orphaned-rolebindings.sh $(REGIONAL_RESOURCEGROUP)
	../templatize.sh $(DEPLOY_ENV) -p region-pipeline.yaml -P run -c public
.PHONY: region

region.clean:
	@if [ "$$(az group exists --name $(REGIONAL_RESOURCEGROUP) --output json)" = "true" ]; then \
		echo "Will delete Azure resource group $(REGIONAL_RESOURCEGROUP)"; \
		az group delete -g $(REGIONAL_RESOURCEGROUP); \
	fi
.PHONY: region.clean

region.what-if:
	../templatize.sh $(DEPLOY_ENV) -p region-pipeline.yaml -P run -c public -d
.PHONY: region.what-if

#
# Service Cluster
#

svc:
	@scripts/cleanup-orphaned-rolebindings.sh $(SVC_RESOURCEGROUP)
	../templatize.sh $(DEPLOY_ENV) -p svc-pipeline.yaml -P run -c public
.PHONY: svc

svc.cs-pr-check-msi:
	@./ensure-no-running-deployment.sh $(SVC_RESOURCEGROUP) cs-integ-msi
	az deployment group create \
		--name cs-integ-msi \
		--resource-group $(SVC_RESOURCEGROUP) \
		--template-file templates/cs-integration-msi.bicep \
		$(PROMPT_TO_CONFIRM) \
		--parameters \
			configurations/cs-integ-msi.bicepparam
.PHONY: svc.cs-pr-check-msi

svc.aks.admin-access:
	@scripts/aks-admin-access.sh $(SVC_RESOURCEGROUP) $(PRINCIPAL_ID)
.PHONY: svc.aks.admin-access

svc.aks.kubeconfig:
	@az aks get-credentials --overwrite-existing --only-show-errors -n ${AKS_NAME} -g $(SVC_RESOURCEGROUP) -f "${SVC_KUBECONFIG_FILE}"
	@kubelogin convert-kubeconfig -l azurecli --kubeconfig "${SVC_KUBECONFIG_FILE}"
.PHONY: svc.aks.kubeconfig

svc.aks.kubeconfigfile:
	@echo ${SVC_KUBECONFIG_FILE}
.PHONY: svc.aks.kubeconfigfile

svc.oidc.storage.permissions:
	@USER_TYPE=$(shell az account show -o json | jq -r '.user.type') && \
	if [ "$${USER_TYPE}" = "user" ]; then \
		STORAGEACCOUNTID=$(shell az storage account show -n ${OIDC_STORAGE_ACCOUNT} -g ${SVC_RESOURCEGROUP} --query id -o tsv) && \
		az role assignment create \
		--role "Storage Blob Data Contributor" \
		--assignee ${PRINCIPAL_ID} \
		--scope "$${STORAGEACCOUNTID}" \
		--only-show-errors; \
	fi
.PHONY: svc.oidc.storage.permissions

svc.kv.permission:
	@scripts/kv-permissions.sh $(PRINCIPAL_ID) $(SVC_KV_RESOURCEGROUP) $(SVC_KV_NAME)
.PHONY: svc.kv.permission

svc.dev.permissions: svc.oidc.storage.permissions svc.kv.permission
.PHONY: svc.dev.permissions

svc.init: region svc svc.aks.admin-access svc.aks.kubeconfig svc.dev.permissions
.PHONY: svc.init

svc.what-if:
	../templatize.sh $(DEPLOY_ENV) -p svc-pipeline.yaml -P run -c public -d
.PHONY: svc.what-if

svc.dev-role-assignments:
	@./ensure-no-running-deployment.sh $(SVC_RESOURCEGROUP) ${ROLE_ASSIGNMENTS_DEPLOYMENT_NAME}
	az deployment group create \
		--name ${ROLE_ASSIGNMENTS_DEPLOYMENT_NAME} \
		--resource-group "${SVC_RESOURCEGROUP}" \
		--template-file templates/dev-roleassignments.bicep \
		--parameters configurations/dev-role-assignments.bicepparam \
		--parameters principalID=${PRINCIPAL_ID}
.PHONY: svc.dev-role-assignments

svc.clean:
	@if [ "$$(az group exists --name $(SVC_RESOURCEGROUP) --output json)" = "true" ]; then \
		echo "Will delete Azure resource group $(SVC_RESOURCEGROUP)"; \
		az group delete -g $(SVC_RESOURCEGROUP); \
	fi
.PHONY: svc.clean

#
# Management Cluster
#

mgmt:
	../templatize.sh $(DEPLOY_ENV) -p mgmt-pipeline.yaml -P run -c public
.PHONY: mgmt

mgmt.aks.admin-access:
	@scripts/aks-admin-access.sh $(MGMT_RESOURCEGROUP) $(PRINCIPAL_ID)
.PHONY: mgmt.aks.admin-access

mgmt.aks.kubeconfig:
	@az aks get-credentials --overwrite-existing --only-show-errors -n ${AKS_NAME} -g $(MGMT_RESOURCEGROUP) -f "${MGMT_KUBECONFIG_FILE}"
	@kubelogin convert-kubeconfig -l azurecli --kubeconfig "${MGMT_KUBECONFIG_FILE}"
.PHONY: mgmt.aks.kubeconfig

mgmt.aks.kubeconfigfile:
	@echo ${MGMT_KUBECONFIG_FILE}
.PHONY: mgmt.aks.kubeconfigfile

mgmt.init: region mgmt mgmt.aks.admin-access mgmt.aks.kubeconfig mgmt.dev.permissions
.PHONY: mgmt.init

mgmt.what-if:
	../templatize.sh $(DEPLOY_ENV) -p mgmt-pipeline.yaml -P run -c public -d
.PHONY: mgmt.what-if

mgmt.clean:
	@if [ "$$(az group exists --name $(MGMT_RESOURCEGROUP) --output json)" = "true" ]; then \
		echo "Will delete Azure resource group $(MGMT_RESOURCEGROUP)"; \
		az group delete -g $(MGMT_RESOURCEGROUP); \
	fi
.PHONY: mgmt.clean

mgmt.kv.permission:
	@scripts/kv-permissions.sh $(PRINCIPAL_ID) $(MGMT_RESOURCEGROUP) $(CX_KV_NAME)
	@scripts/kv-permissions.sh $(PRINCIPAL_ID) $(MGMT_RESOURCEGROUP) $(MSI_KV_NAME)
	@scripts/kv-permissions.sh $(PRINCIPAL_ID) $(MGMT_RESOURCEGROUP) $(MGMT_KV_NAME)
.PHONY: mgmt.kv.permission

mgmt.dev.permissions: mgmt.kv.permission
.PHONY: mgmt.dev.permissions

# ACR DEV customizations

acr: acr-svc-cfg acr-ocp-cfg
.PHONY: acr

acr.what-if: acr-svc-cfg.what-if acr-ocp-cfg.what-if
.PHONY: acr.what-if

acr-svc-cfg: # DEV only setup of caching rules in OCP ACR
	@./ensure-no-running-deployment.sh $(GLOBAL_RESOURCEGROUP) ${GLOBAL_RG_DEPLOYMENT_NAME}-acr-svc
	az deployment group create \
		--name ${GLOBAL_RG_DEPLOYMENT_NAME}-acr-svc \
		--resource-group $(GLOBAL_RESOURCEGROUP) \
		--template-file templates/dev-acr.bicep \
		$(PROMPT_TO_CONFIRM) \
		--parameters \
			configurations/acr-svc.bicepparam
.PHONY: acr-svc-cfg

acr-svc-cfg.what-if:
	az deployment group what-if \
		--name ${GLOBAL_RG_DEPLOYMENT_NAME}-acr-svc \
		--resource-group $(GLOBAL_RESOURCEGROUP) \
		--template-file templates/dev-acr.bicep \
		--parameters \
			configurations/acr-svc.bicepparam
.PHONY: acr-svc-cfg.what-if

acr-ocp-cfg: # DEV only setup of caching rules in OCP ACR
	@./ensure-no-running-deployment.sh $(GLOBAL_RESOURCEGROUP) ${GLOBAL_RG_DEPLOYMENT_NAME}-acr-ocp
	az deployment group create \
		--name ${GLOBAL_RG_DEPLOYMENT_NAME}-acr-ocp \
		--resource-group $(GLOBAL_RESOURCEGROUP) \
		--template-file templates/dev-acr.bicep \
		$(PROMPT_TO_CONFIRM) \
		--parameters \
			configurations/acr-ocp.bicepparam
.PHONY: acr-ocp-cfg

acr-ocp-cfg.what-if:
	az deployment group what-if \
		--name ${GLOBAL_RG_DEPLOYMENT_NAME}-acr-ocp \
		--resource-group $(GLOBAL_RESOURCEGROUP) \
		--template-file templates/dev-acr.bicep \
		--parameters \
			configurations/acr-ocp.bicepparam
.PHONY: acr-ocp-cfg.what-if

#
# Postgres Authentication Helpers
#

cs-current-user-pg-connect:
	@scripts/cs-current-user-pg-connect.sh $(SVC_RESOURCEGROUP) $(CS_PG_NAME)
.PHONY: cs-current-user-pg-connect

cs-miwi-pg-connect:
	@scripts/cs-miwi-pg-connect.sh $(SVC_RESOURCEGROUP) $(CS_PG_NAME) $(CS_MI_NAME) $(CS_NS_NAME) $(CS_SA_NAME)
.PHONY: cs-miwi-pg-connect

maestro-current-user-pg-connect:
	@scripts/cs-current-user-pg-connect.sh $(SVC_RESOURCEGROUP) $(MAESTRO_PG_NAME)
.PHONY: maestro-current-user-pg-connect

maestro-miwi-pg-connect:
	@scripts/cs-miwi-pg-connect.sh $(SVC_RESOURCEGROUP) $(MAESTRO_PG_NAME) "maestro-server" "maestro" "maestro"
.PHONY: maestro-miwi-pg-connect

#
# Secrets
#

secrets-download:
	@[ "${SECRET_SA_ACCOUNT_NAME}" ] || ( echo ">> SECRET_SA_ACCOUNT_NAME is not set"; exit 1 )
	rm -rf secrets
	az storage blob download -n secrets.tar.gz -c secrets -f secrets.tar.gz --account-name ${SECRET_SA_ACCOUNT_NAME} --auth-mode login >/dev/null
	tar -xzf secrets.tar.gz
	rm secrets.tar.gz
.PHONY: secrets-download

secrets-yes-i-want-to-update-the-shared-secrets:
	@[ "${SECRET_SA_ACCOUNT_NAME}" ] || ( echo ">> SECRET_SA_ACCOUNT_NAME is not set"; exit 1 )
	tar -czf secrets.tar.gz secrets
	az storage blob upload -n secrets.tar.gz -c secrets -f secrets.tar.gz --overwrite --auth-mode login --account-name ${SECRET_SA_ACCOUNT_NAME} >/dev/null
	rm secrets.tar.gz
.PHONY: secrets-yes-i-want-to-update-the-shared-secrets

#
# Operator roles for platform workload identity
#

operator-roles:
	az deployment sub create \
	    --location westus3 \
		--name operator-roles \
		--template-file templates/dev-operator-roles.bicep \
		$(PROMPT_TO_CONFIRM) \
		--parameters configurations/dev-operator-roles.bicepparam
.PHONY: operator-roles

operator-roles.what-if:
	az deployment sub what-if \
	    --location westus3 \
		--name operator-roles \
		--template-file templates/dev-operator-roles.bicep \
		--parameters configurations/dev-operator-roles.bicepparam
.PHONY: operator-roles.what-if

#
# Common
#

what-if: global.what-if acr.what-if region.what-if svc.what-if mgmt.what-if operator-roles.what-if
.PHONY: what-if

infra: region svc.init mgmt.init
.PHONY: infra

clean: svc.clean mgmt.clean region.clean
.PHONY: clean

#
# Local CS Development
#

local-cs-permissions: svc.dev.permissions mgmt.dev.permissions
.PHONY: local-cs-permissions
