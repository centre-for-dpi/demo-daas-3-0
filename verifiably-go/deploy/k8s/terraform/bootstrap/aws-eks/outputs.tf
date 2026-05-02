output "kubeconfig_path" {
  value      = local.kubeconfig_path
  depends_on = [null_resource.kubeconfig]
}

output "lb_mode" {
  value = "cloud"
}

output "cluster_name" {
  value = module.eks.cluster_name
}

output "cluster_endpoint" {
  value = module.eks.cluster_endpoint
}
