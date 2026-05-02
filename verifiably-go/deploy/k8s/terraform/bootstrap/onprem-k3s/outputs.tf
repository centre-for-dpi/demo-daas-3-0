output "kubeconfig_path" {
  value      = local.kubeconfig_path
  depends_on = [null_resource.k3s_join]
}

output "lb_mode" {
  value = "metallb"
}

output "cluster_name" {
  value = var.cluster_name
}
