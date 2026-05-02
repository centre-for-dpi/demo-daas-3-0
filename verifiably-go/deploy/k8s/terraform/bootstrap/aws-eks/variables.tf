variable "region" {
  description = "AWS region."
  type        = string
}

variable "cluster_name" {
  type    = string
  default = "verifiably-eks"
}

variable "kubernetes_version" {
  type    = string
  default = "1.30"
}

variable "vpc_id" {
  description = "Existing VPC the cluster joins. Provision separately."
  type        = string
}

variable "subnet_ids" {
  description = "Private subnets for nodes; should span >= 2 AZs."
  type        = list(string)
}

variable "control_plane_subnet_ids" {
  description = "Optional override for control-plane ENIs (defaults to subnet_ids)."
  type        = list(string)
  default     = []
}

variable "instance_type" {
  type    = string
  default = "t3.large"
}

variable "node_group_min" {
  type    = number
  default = 2
}

variable "node_group_max" {
  type    = number
  default = 6
}

variable "node_group_desired" {
  type    = number
  default = 3
}

variable "kubeconfig_dir" {
  type    = string
  default = "../../.tfstate"
}
