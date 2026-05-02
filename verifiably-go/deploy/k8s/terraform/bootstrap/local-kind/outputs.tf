output "kubeconfig_path" {
  description = "Absolute path to the cluster kubeconfig — feed into platform/."
  value       = local.kubeconfig_path
  depends_on  = [null_resource.metallb]
}

output "lb_mode" {
  description = "Load-balancer mode — drives platform/ MetalLB toggling."
  value       = "metallb"
}

output "cluster_name" {
  value = var.cluster_name
}

output "host_http_url" {
  description = "Host URL that reaches ingress-nginx :80 inside the cluster."
  value       = "http://localhost:${var.host_http_port}"
}

output "host_https_url" {
  value = "https://localhost:${var.host_https_port}"
}
