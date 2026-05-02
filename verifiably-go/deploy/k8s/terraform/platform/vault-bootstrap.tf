# Vault bootstrap — runs once after Vault is initialized + unsealed (an
# operator-driven step we do not automate here; see runbooks/vault-init.md).
#
# Phase 7.1 uses the Vault provider to:
#   - mount transit + KV-v2
#   - create the waltid policy (loaded from vault-policies/waltid.hcl)
#   - bind the policy to a Kubernetes auth role for SA waltid/* in namespace waltid
#   - create the issuer + wallet Transit signing keys (Ed25519 by default)

# This file is gated by var.bootstrap_vault — set to true only AFTER you've
# run the manual init+unseal step. Keep false on first apply so terraform
# doesn't try to talk to a sealed Vault.
variable "bootstrap_vault" {
  description = "Run Vault bootstrap (set true after manual init/unseal)."
  type        = bool
  default     = false
}

variable "vault_root_token" {
  description = "Initial root token from `vault operator init` — used only by this bootstrap."
  type        = string
  default     = ""
  sensitive   = true
}

# We don't add the Vault provider to required_providers unconditionally
# because it would force users to set VAULT_ADDR / token env vars even
# when bootstrap_vault=false. When you flip the flag, uncomment the block
# in versions.tf:
#   vault = { source = "hashicorp/vault", version = "~> 4.4" }
#
# Then run:
#   terraform init -upgrade
#   terraform apply -var bootstrap_vault=true -var vault_root_token=$ROOT_TOKEN
#
# The resources below are deliberately commented out until that wiring lands.

# resource "vault_mount" "transit" {
#   count = var.bootstrap_vault ? 1 : 0
#   path  = "transit"
#   type  = "transit"
# }
#
# resource "vault_mount" "kv_secret" {
#   count = var.bootstrap_vault ? 1 : 0
#   path  = "secret"
#   type  = "kv-v2"
# }
#
# resource "vault_policy" "waltid" {
#   count  = var.bootstrap_vault ? 1 : 0
#   name   = "waltid"
#   policy = file("${path.module}/vault-policies/waltid.hcl")
# }
#
# resource "vault_auth_backend" "kubernetes" {
#   count = var.bootstrap_vault ? 1 : 0
#   type  = "kubernetes"
#   path  = "kubernetes"
# }
#
# resource "vault_kubernetes_auth_backend_role" "waltid" {
#   count                            = var.bootstrap_vault ? 1 : 0
#   backend                          = vault_auth_backend.kubernetes[0].path
#   role_name                        = "waltid"
#   bound_service_account_names      = ["external-secrets", "*"]
#   bound_service_account_namespaces = ["external-secrets", "waltid"]
#   token_policies                   = [vault_policy.waltid[0].name]
#   token_ttl                        = 86400
# }
#
# resource "vault_transit_secret_backend_key" "issuer" {
#   count   = var.bootstrap_vault ? 1 : 0
#   backend = vault_mount.transit[0].path
#   name    = "waltid-issuer-key"
#   type    = "ed25519"
# }
#
# resource "vault_transit_secret_backend_key" "wallet" {
#   count   = var.bootstrap_vault ? 1 : 0
#   backend = vault_mount.transit[0].path
#   name    = "waltid-wallet-key"
#   type    = "ed25519"
# }
