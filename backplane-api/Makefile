-include ../setup-env.mk
-include ../helm-cmd.mk

ARO_HCP_IMAGE_REGISTRY ?= ${ARO_HCP_IMAGE_ACR}.azurecr.io

deploy-backplane:

	kubectl create namespace backplane-api --dry-run=client -o yaml | kubectl apply -f -
	helm upgrade --install backplane-api ./deploy/helm/backplane-api \
		--namespace aro-hcp \
		--set statefulset.replicaCount=2 \
		--set statefulset.imageName="quay.io/hbhushan/backplane:728a447" \
		--set serviceAccount.name="frontend" \
		--set istio.enabled=true \
		--set istio.ingressNamespace="aks-istio-ingress" \
		--set istio.ingressPort="8001"\
		--set service.tlsSecretName="backplane-api-tls" \
		
		


# --set statefulset.imageName=${ARO_HCP_IMAGE_ACR}.azurecr.io/backplane-api@$${DIGEST} 
# --set pullBinding.registry=${ARO_HCP_IMAGE_ACR}.azurecr.io \
# --set pullBinding.scope="repo		sitory:app-sre/backplane-api:pull"
undeploy-backplane:
	helm uninstall backplane-api --namespace aro-hcp
	kubectl delete namespace backplane-api

.PHONY: deploy undeploy
