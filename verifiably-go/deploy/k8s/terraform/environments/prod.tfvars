# Prod environment — fill in for your deployment target.
target              = "onprem"          # or "aws"
cluster_name        = "verifiably-prod"
domain              = "verifiably.example.com"
cluster_issuer_email = "ops@example.com"
lb_mode             = "metallb"         # "cloud" on aws
