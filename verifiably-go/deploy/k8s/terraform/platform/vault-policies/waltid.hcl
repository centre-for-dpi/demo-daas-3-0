# Vault policy for the waltid namespace.
#
# Granted to ServiceAccounts in the `waltid` namespace via the Vault
# Kubernetes auth role 'waltid' (created by the bootstrap Job).

# Read-only access to walt.id KV secrets.
path "secret/data/waltid/*" {
  capabilities = ["read"]
}

# Transit operations on the issuer + wallet signing keys. Use via
# the agent injector sidecar pattern — pods send sign/verify requests
# to Vault and never see the private key bytes.
path "transit/sign/waltid-issuer-key" {
  capabilities = ["update"]
}
path "transit/sign/waltid-wallet-key" {
  capabilities = ["update"]
}
path "transit/verify/waltid-issuer-key" {
  capabilities = ["update"]
}
path "transit/verify/waltid-wallet-key" {
  capabilities = ["update"]
}
