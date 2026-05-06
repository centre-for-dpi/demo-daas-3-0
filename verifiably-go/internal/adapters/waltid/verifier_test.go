package waltid

import (
	"reflect"
	"testing"
)

// TestBuildVCPolicies_StatusListShape pins the wire shape buildVCPolicies
// emits to walt.id's verifier-api for the credential-status policy. The
// values were lifted from the decompiled
// waltid-verification-policies-jvm-1.0.0-SNAPSHOT.jar bytecode:
//
//   - StatusPolicyArgument is a sealed class with a JsonClassDiscriminator
//     keyed off "discriminator". Subclasses: IETFStatusPolicyAttribute
//     (SerialName "ietf", fields: value) and W3CStatusPolicyAttribute
//     (SerialName "w3c", fields: value, purpose, type).
//
//   - W3CStatusValidator.customValidations compares the parsed
//     args.type against the published list's vc.credentialSubject.type
//     (which has to be "BitstringStatusList" for the
//     W3cStatusListExpansionAlgorithmFactory to dispatch to the BSL 2023
//     path). args.type=BitstringStatusListEntry fails with "Type
//     validation failed: expected BitstringStatusListEntry, but got
//     BitstringStatusList".
//
// If anyone changes the strings without verifying against the jar, this
// test fails before walt.id sees the bad request.
func TestBuildVCPolicies_StatusListShape(t *testing.T) {
	cases := []struct {
		name   string
		format string
		want   any
	}{
		{
			name:   "W3C VCDM 2.0 (ldp_vc)",
			format: "ldp_vc",
			want: map[string]any{
				"policy": "credential-status",
				"args": map[string]any{
					"discriminator": "w3c",
					"value":         0,
					"purpose":       "revocation",
					"type":          "BitstringStatusList",
				},
			},
		},
		{
			name:   "W3C JWT-VC (jwt_vc_json)",
			format: "jwt_vc_json",
			want: map[string]any{
				"policy": "credential-status",
				"args": map[string]any{
					"discriminator": "w3c",
					"value":         0,
					"purpose":       "revocation",
					"type":          "BitstringStatusList",
				},
			},
		},
		{
			name:   "SD-JWT VC (vc+sd-jwt)",
			format: "vc+sd-jwt",
			want: map[string]any{
				"policy": "credential-status",
				"args": map[string]any{
					"discriminator": "ietf",
					"value":         0,
				},
			},
		},
		{
			name:   "SD-JWT VC (dc+sd-jwt)",
			format: "dc+sd-jwt",
			want: map[string]any{
				"policy": "credential-status",
				"args": map[string]any{
					"discriminator": "ietf",
					"value":         0,
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildVCPolicies([]string{"status-list"}, "", tc.format)
			if len(got) != 1 {
				t.Fatalf("expected 1 policy entry, got %d: %+v", len(got), got)
			}
			if !reflect.DeepEqual(got[0], tc.want) {
				t.Fatalf("status-list policy shape mismatch:\n  got:  %#v\n  want: %#v", got[0], tc.want)
			}
		})
	}
}

// TestBuildVCPolicies_StringPolicies confirms the simple string policies
// stay strings (walt.id treats them as named policies with no args).
func TestBuildVCPolicies_StringPolicies(t *testing.T) {
	got := buildVCPolicies([]string{"signature", "expired", "not-before"}, "", "ldp_vc")
	want := []any{"signature", "expired", "not-before"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("string policies:\n  got:  %#v\n  want: %#v", got, want)
	}
}

// TestBuildVCPolicies_WebhookOmittedWhenURLEmpty matches the existing
// behavior — a checked Webhook policy with an empty URL is dropped
// rather than sent to walt.id, which would reject it.
func TestBuildVCPolicies_WebhookOmittedWhenURLEmpty(t *testing.T) {
	if got := buildVCPolicies([]string{"webhook"}, "", "ldp_vc"); len(got) != 0 {
		t.Fatalf("expected webhook with empty URL to be dropped: %#v", got)
	}
	got := buildVCPolicies([]string{"webhook"}, "https://hook.example/x", "ldp_vc")
	want := []any{
		map[string]any{
			"policy": "webhook",
			"args":   map[string]any{"url": "https://hook.example/x"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("webhook policy:\n  got:  %#v\n  want: %#v", got, want)
	}
}
