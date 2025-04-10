// This is used to grant backplane-api the ability to fetch kubeconfigs from
// dev AKS Management clusters.
param aksManagementClusterName string
param location string = resourceGroup().location
param principalID string

// https://learn.microsoft.com/en-us/azure/aks/manage-azure-rbac#create-role-assignments-for-users-to-access-the-cluster
// Azure Kubernetes Service RBAC Admin
// Allows access to all resources under cluster/namespace, except update or delete resource quotas and namespaces.
var aksClusterRbacClusterAdminRoleId = subscriptionResourceId(
  'Microsoft.Authorization/roleDefinitions/',
  '3498e952-d568-435e-9b2c-8d77e338d7f7'
)
resource aksCluster 'Microsoft.ContainerService/managedClusters@2024-02-01' existing = {
  name: aksManagementClusterName
}

// az aks command invoke --resource-group hcp-standalone-hb --name aro-hcp-cluster-001 --command "kubectl get ns"
resource currentUserAksClusterAdmin 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  scope: aksCluster
  name: guid(location, aksManagementClusterName, aksClusterRbacClusterAdminRoleId, principalID)
  properties: {
    roleDefinitionId: aksClusterRbacClusterAdminRoleId
    principalId: principalID
  }
}
