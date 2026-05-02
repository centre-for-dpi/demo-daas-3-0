# Runbook — Vault initialization, unseal, and Transit-key rotation

This runbook covers the operator steps that surround the Terraform
bootstrap. The platform module deploys Vault HA but does NOT initialize
or unseal it — that's a deliberate, security-relevant manual step you
own.

## 1. Initialize Vault (first time only)

After `./k8s-deploy.sh up waltid` completes, Vault pods will be in a
"Sealed" state.

```sh
kubectl -n vault exec -it vault-0 -- vault operator init \
    -key-shares=5 -key-threshold=3 -format=json > vault-init.json
```

**Store `vault-init.json` securely** — it contains the 5 unseal keys and
the initial root token. Lose them and the cluster is unrecoverable.

## 2. Unseal each replica

```sh
for i in 0 1 2; do
  for k in $(jq -r '.unseal_keys_b64[0:3][]' vault-init.json); do
    kubectl -n vault exec vault-$i -- vault operator unseal "$k"
  done
done
```

Verify:
```sh
kubectl -n vault exec -it vault-0 -- vault status
# expect: Initialized=true, Sealed=false
```

## 3. Bootstrap policies + Transit keys via Terraform

Edit `deploy/k8s/terraform/platform/versions.tf` to add the Vault
provider, then re-run apply with the bootstrap flag:

```sh
ROOT_TOKEN=$(jq -r .root_token vault-init.json)
export VAULT_ADDR="$(terraform -chdir=deploy/k8s/terraform/platform output -raw vault_address)"

cd deploy/k8s/terraform/platform
terraform init -upgrade
terraform apply -auto-approve \
    -var bootstrap_vault=true \
    -var "vault_root_token=$ROOT_TOKEN"
```

This creates:
- `transit/` mount
- `secret/` (KV-v2) mount
- `waltid` policy
- `kubernetes/` auth method bound to SAs in `waltid` and `external-secrets`
- Transit keys `waltid-issuer-key`, `waltid-wallet-key` (Ed25519)

## 4. Seed the wallet auth secrets

The walt-wallet chart reads `WALLET_ENCRYPTION_KEY` and `WALLET_SIGN_KEY`
from a Secret materialized by ESO from `secret/data/waltid/wallet/auth`.

```sh
ENC=$(openssl rand -hex 8)        # 16 ASCII chars = 128 bits
SIG=$(openssl rand -hex 8)
kubectl -n vault exec -it vault-0 -- vault kv put secret/waltid/wallet/auth \
    encryption-key="$ENC" sign-key="$SIG"
```

ESO refreshes every hour; force immediate sync:
```sh
kubectl -n waltid annotate externalsecret wallet-auth force-sync="$(date +%s)" --overwrite
```

## 5. Switch wallet-api to Vault Transit

Edit `deploy/k8s/config/wallet/registration-defaults.conf` to replace
the in-process key backend with Vault TSE:

```hocon
defaultKeyConfig: {
    backend: tse
    config: {
        server: "http://vault.vault.svc.cluster.local:8200/v1/transit"
        accessKey: "${VAULT_TOKEN}"
    }
    keyType: Ed25519
}
```

Then sync charts: `./k8s-deploy.sh sync-config`. The Vault Agent injector
sidecar (added in walt-wallet's deployment.yaml — TODO: Phase 7.1
follow-up) populates `VAULT_TOKEN` automatically.

## 6. Rotation

```sh
# Rotate Transit signing keys (creates a new key version, old version
# remains valid for verify until manually deleted).
kubectl -n vault exec -it vault-0 -- vault write -f transit/keys/waltid-issuer-key/rotate
kubectl -n vault exec -it vault-0 -- vault write -f transit/keys/waltid-wallet-key/rotate

# Rotate symmetric encryption + sign keys (overwrites the KV value, ESO
# picks it up at next refresh):
ENC=$(openssl rand -hex 8); SIG=$(openssl rand -hex 8)
kubectl -n vault exec -it vault-0 -- vault kv put secret/waltid/wallet/auth \
    encryption-key="$ENC" sign-key="$SIG"

# Wait for ESO to refresh, then bounce the wallet pod so it picks up the
# new env values from the regenerated Secret.
kubectl -n waltid rollout restart deploy waltid-walt-wallet
```

Expected rotation cadence: signing keys every 90 days, symmetric keys
every 30 days. Track via Grafana panel "Secret age" (TBD — Phase 6.1
follow-up).
