# Kyverno + cosign image-verification policy. Phase 7.2.
#
# Kyverno is installed as a separate helm_release (kept in this file for
# locality with the policy resource it enables).

resource "helm_release" "kyverno" {
  name       = "kyverno"
  repository = "https://kyverno.github.io/kyverno"
  chart      = "kyverno"
  version    = "3.2.7"
  namespace  = "kyverno"

  create_namespace = true
  wait             = true
  timeout          = 600

  depends_on = [kubernetes_namespace.platform]
}

resource "kubectl_manifest" "verify_signed_images" {
  yaml_body = yamlencode({
    apiVersion = "kyverno.io/v1"
    kind       = "ClusterPolicy"
    metadata = {
      name = "require-signed-images-waltid"
    }
    spec = {
      validationFailureAction = "Enforce"
      background              = false
      webhookTimeoutSeconds   = 30
      failurePolicy           = "Fail"
      rules = [
        {
          name = "verify-cosign-signature"
          match = {
            any = [{
              resources = {
                kinds      = ["Pod"]
                namespaces = ["waltid"]
              }
            }]
          }
          # Only enforce on images we publish; upstream walt.id images
          # aren't cosign-signed by walt.id, so we skip them via image-list.
          verifyImages = [{
            imageReferences = ["ghcr.io/*/verifiably-go*"]
            attestors = [{
              entries = [{
                keyless = {
                  url     = "https://fulcio.sigstore.dev"
                  subject = "https://github.com/*/verifiably-go*"
                  issuer  = "https://token.actions.githubusercontent.com"
                  rekor   = { url = "https://rekor.sigstore.dev" }
                }
              }]
            }]
          }]
        }
      ]
    }
  })
  depends_on = [helm_release.kyverno]
}
