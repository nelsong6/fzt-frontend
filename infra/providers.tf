provider "azurerm" {
  features {}
  use_oidc = true
  # subscription_id / tenant_id come from the ARM_* env vars the shared
  # tofu workflow exports for OIDC auth — no need to plumb them through
  # tofu variables.
  resource_provider_registrations = "none"
}

# Cluster-subscription provider for the federated identity credential —
# FICs are Entra resources (not subscription-bound), but the AKS OIDC
# issuer URL is read from the cluster's remote state and the cluster
# lives in var.cluster_subscription_id (separate from this stack's
# workload sub by default).
provider "azurerm" {
  alias = "cluster"

  features {}
  use_oidc                        = true
  subscription_id                 = var.cluster_subscription_id
  resource_provider_registrations = "none"
}
