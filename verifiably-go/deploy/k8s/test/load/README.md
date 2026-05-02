# Load tests

## wallet-scale.js — Phase 6.2

Goal: confirm whether wallet-api can run with `replicas > 1` without
breaking sessions or wallet state under round-robin load balancing.

### How to run

1. Bring up the cluster: `./deploy/k8s/scripts/k8s-deploy.sh up waltid`
2. Apply baseline (1 replica):
   ```sh
   kubectl -n waltid scale deploy waltid-walt-wallet --replicas=1
   k6 run --env BASE_URL=https://wallet.verifiably.local --env VUS=50 \
          deploy/k8s/test/load/wallet-scale.js | tee report-1replica.txt
   ```
3. Apply scale-out (3 replicas):
   ```sh
   kubectl -n waltid scale deploy waltid-walt-wallet --replicas=3
   k6 run --env BASE_URL=https://wallet.verifiably.local --env VUS=50 \
          deploy/k8s/test/load/wallet-scale.js | tee report-3replicas.txt
   ```
4. Compare `cross_pod_drift` rate between the two.

### Interpretation

- **`cross_pod_drift` = 0 in both runs** → wallet-api is stateless w.r.t.
  sessions; safe to enable HPA. Update `walt-wallet/values.yaml`
  `wallet.hpa.enabled: true`.
- **`cross_pod_drift` jumps in the 3-replica run** → wallet-api keeps
  in-process session state. Options:
    - sticky sessions via ingress-nginx `nginx.ingress.kubernetes.io/affinity: cookie`
    - move session state to Redis (walt.id supports this — see ktor-server-sessions-redis)
  Document the chosen path in `docs/k8s/wallet-scaling.md`.

### Status

Not yet run — pending Phase 8.1 cluster-up + first `up waltid` end-to-end.
