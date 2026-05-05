# Single-EC2 quickstart for the K8s waltid scenario

End-state after this guide: a single Ubuntu 24.04 EC2 instance running
k3s + the full waltid umbrella chart, reachable from your laptop's
browser at `https://app.verifiably.local` (and four sibling
hostnames).

Time budget: ~5 min to launch the instance, ~15 min for the user-data
script to install everything and pre-pull images, ~10–15 min for the
single `k8s-deploy.sh up` command. ~30–35 min total.

## 1. Launch the instance

| Field | Value |
|---|---|
| AMI | **Ubuntu Server 24.04 LTS (HVM, SSD)**, amd64 — pick the latest. In `us-east-1` the canonical search string is `ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*`. |
| Instance type | **m6i.2xlarge** (8 vCPU / 32 GiB) |
| Key pair | Use one you already have, or create a new one — you'll SSH with it. |
| VPC + subnet | Default VPC + a public subnet is fine for testing. |
| Public IP | **Auto-assign** (or attach an Elastic IP after launch if you want a stable address). |
| Security group | Inbound: TCP **22** (SSH) from your IP, TCP **80** + **443** from `0.0.0.0/0`. Outbound: all (default). |
| Storage | **1 × 100 GiB gp3** root volume (the default 8 GiB will not fit the platform PVCs). |
| User data | Paste the contents of `verifiably-go/deploy/cloud/ec2-bootstrap.sh` (use the *Advanced details → User data* field). |
| IAM role | None needed for the demo. |

Launch. The instance will boot, then run user-data for ~10–15 min while
it installs Docker / kubectl / helm / terraform / k6 / k3s and pre-pulls
the heavier umbrella images into k3s containerd.

## 2. Wait for the bootstrap to finish

SSH in once the instance reaches "Running" in the console:

```sh
ssh -i ~/.ssh/your-key.pem ubuntu@<public-ip>
```

Tail the cloud-init log until you see the success marker:

```sh
sudo tail -F /var/log/cloud-init-output.log
# wait until you see: [ec2-bootstrap] bootstrap complete
ls /var/lib/cloud/instance/k8s-bootstrap-complete   # exists → ready
```

If you see `k8s-bootstrap-failed` instead, check the same log; the
`fail` line tells you which step failed. Re-running the script is safe
(every step is idempotent).

## 3. One command brings up the cluster

```sh
git clone https://github.com/centre-for-dpi/demo-daas-3-0.git
cd demo-daas-3-0/verifiably-go
./deploy/k8s/scripts/k8s-deploy.sh up waltid --target=ec2
```

What happens:

1. **[1/3]** k3s already running → skipped (this is the difference from
   `--target=local|onprem|aws`).
2. **[2/3]** `terraform apply platform/` — installs ingress-nginx,
   cert-manager, MetalLB, CNPG, MinIO operator, ESO, Vault HA,
   kube-prometheus-stack, Loki + Promtail, Argo CD, Kyverno. ~5–8 min.
3. **[3/3]** `terraform apply workloads/` — installs the umbrella chart
   (walt-issuer + walt-verifier + walt-wallet + verifiably-go +
   keycloak + wso2is + libretranslate). ~5–7 min.

`kubectl wait` then blocks until every pod in the `waltid` namespace
reports Ready. When it returns, the demo is up.

## 4. Reach the demo from your laptop

The umbrella creates 5 ingress hostnames, all on the same EC2 IP:

| Hostname | Service |
|---|---|
| `app.verifiably.local` | verifiably-go (the demo UI you point your browser at) |
| `wallet.verifiably.local` | walt.id wallet API |
| `issuer.verifiably.local` | walt.id issuer API |
| `verifier.verifiably.local` | walt.id verifier API |
| `keycloak.verifiably.local` | Keycloak admin + OIDC |

Add this block to `/etc/hosts` on your **laptop**, replacing
`<ec2-public-ip>` with the instance's public IP (or elastic IP):

```
<ec2-public-ip>  app.verifiably.local
<ec2-public-ip>  wallet.verifiably.local
<ec2-public-ip>  issuer.verifiably.local
<ec2-public-ip>  verifier.verifiably.local
<ec2-public-ip>  keycloak.verifiably.local
```

Then open `https://app.verifiably.local` in your browser. The first
visit shows a TLS warning because cert-manager issued self-signed
certificates from the platform's `selfsigned` ClusterIssuer — click
through ("Advanced → Proceed") on each hostname once. From there the
demo flows work as in the compose stack.

## 5. Useful follow-ups

```sh
# pod state on the EC2 instance
sudo k3s kubectl -n waltid get pods
# … or via the path the deploy script exports:
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
kubectl get ingress -A
kubectl -n waltid logs deployment/waltid-verifiably-go --tail=200

# the deploy script's own status helper:
./deploy/k8s/scripts/k8s-deploy.sh status

# tear down workloads but keep the platform layer (faster re-deploy):
./deploy/k8s/scripts/k8s-deploy.sh down waltid

# nuke the cluster entirely (also wipes k3s + all data):
./deploy/k8s/scripts/k8s-deploy.sh reset
sudo /usr/local/bin/k3s-uninstall.sh
```

## 6. Costs

m6i.2xlarge in us-east-1 is **~$0.384/hr** on-demand, plus
**~$0.10/GB/month** for the 100 GiB gp3 volume (~$10/month if left
running). Stop the instance when not in use — k3s state lives on the
EBS volume and survives stop/start, so a restart is fast (no need to
re-run user-data or the deploy script unless you reset).

## 7. If something goes wrong

| Symptom | Likely cause | Fix |
|---|---|---|
| `k8s-deploy.sh up` says `k3s kubeconfig missing` | User-data didn't finish (or failed) | `sudo tail -200 /var/log/cloud-init-output.log`; re-run by `sudo bash deploy/cloud/ec2-bootstrap.sh` |
| Some pods stuck in `ContainerCreating` for > 10 min | Image pull throttled by Docker Hub anonymous limit | Wait 6 hours, or auth: `sudo k3s ctr config plugins."io.containerd.grpc.v1.cri".registry.configs."docker.io".auth.username=…` |
| `kubectl wait` times out with pods still `Pending` | Node out of CPU / RAM | `kubectl describe node` to confirm; bump instance to m6i.4xlarge |
| Browser says `ERR_CONNECTION_REFUSED` on `https://app.verifiably.local` | `/etc/hosts` entry missing or wrong IP | Re-check entry; `curl -k --resolve app.verifiably.local:443:<ec2-ip> https://app.verifiably.local/` |
| Self-signed cert warning won't go away after 5 mins | cert-manager Order still pending | `kubectl get clusterissuer,certificate,order,challenge -A` to see what's stuck |
