# MetalLB — only when lb_mode = metallb AND bootstrap didn't already install
# it. local-kind bootstrap does install MetalLB itself so this is typically
# a no-op for the dev path; on a bare onprem-k3s cluster you'd flip
# metallb_already_installed = false to let this module install it.
resource "helm_release" "metallb" {
  count = (var.lb_mode == "metallb" && !var.metallb_already_installed) ? 1 : 0

  name       = "metallb"
  repository = "https://metallb.github.io/metallb"
  chart      = "metallb"
  version    = var.chart_versions.metallb
  namespace  = "metallb-system"

  create_namespace = true
  wait             = true
  timeout          = 600
}
