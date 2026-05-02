variable "cluster_name" {
  description = "Name of the kind cluster."
  type        = string
  default     = "verifiably-dev"
}

variable "control_plane_replicas" {
  description = "Number of kind control-plane nodes (HA when > 1)."
  type        = number
  default     = 1
}

variable "worker_replicas" {
  description = "Number of kind worker nodes."
  type        = number
  default     = 2
}

variable "node_image" {
  description = "kind node image (pinned digest in prod-ish runs)."
  type        = string
  default     = "kindest/node:v1.30.4"
}

variable "kubeconfig_dir" {
  description = "Where to write the cluster kubeconfig."
  type        = string
  default     = "../../.tfstate"
}

variable "host_http_port" {
  description = "Host port mapped to ingress-nginx :80 inside the cluster."
  type        = number
  default     = 8080
}

variable "host_https_port" {
  description = "Host port mapped to ingress-nginx :443 inside the cluster."
  type        = number
  default     = 8443
}

variable "metallb_pool_cidr" {
  description = "IP range MetalLB advertises for LoadBalancer services. Must lie inside the docker network kind uses (default kind subnet is 172.18.0.0/16)."
  type        = string
  default     = "172.18.255.200-172.18.255.250"
}
