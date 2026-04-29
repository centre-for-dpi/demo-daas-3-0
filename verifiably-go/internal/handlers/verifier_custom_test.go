package handlers

import (
	"strings"
	"testing"

	"github.com/verifiably/verifiably-go/vctypes"
)

// TestCustomSchemaCredTypeMatchesIssuedVC locks in the alignment between
// the catalog (what walt.id advertises), the issued credential (what the
// wallet stores) and the verifier's PD filter (what the wallet matches
// against).
//
// Regression target: "your wallet has no credential matching this request
// (verifier asked for custom-di5upc0ecas3 in vc+sd-jwt format)" reported
// on 2026-04-29. The verifier handler was using picked.BaseType() which
// returned the schema's random "custom-..." ID — the issued credential
// carried CustomTypeName(), so the wallet matcher never saw a hit.
func TestCustomSchemaCredTypeMatchesIssuedVC(t *testing.T) {
	cases := []struct {
		name   string
		schema vctypes.Schema
		want   string
	}{
		{
			name: "AdditionalTypes wins",
			schema: vctypes.Schema{
				ID: "custom-abc", Name: "Farmer Credential", Custom: true,
				AdditionalTypes: []string{"FarmerCredential"},
			},
			want: "FarmerCredential",
		},
		{
			name: "blank AdditionalTypes falls back to sanitised Name",
			schema: vctypes.Schema{
				ID: "custom-xyz", Name: "Farmer Credential", Custom: true,
				AdditionalTypes: []string{""},
			},
			want: "FarmerCredential",
		},
		{
			name: "no AdditionalTypes at all → sanitised Name",
			schema: vctypes.Schema{
				ID: "custom-xyz", Name: "My Cool Cred", Custom: true,
			},
			want: "MyCoolCred",
		},
		{
			name: "empty Name falls back to placeholder",
			schema: vctypes.Schema{
				ID: "custom-empty", Custom: true,
			},
			want: "CustomCredential",
		},
		{
			name: "non-alphanumeric Name still produces a valid identifier",
			schema: vctypes.Schema{
				ID: "custom-emoji", Name: "🎓 ✨ !!!", Custom: true,
			},
			want: "CustomCredential",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.schema.CustomTypeName()
			if got != tc.want {
				t.Errorf("CustomTypeName() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCustomSchemaSDJWTUsesTypeNameNotID is a closer-to-the-bug regression:
// when the verifier handler picks a custom SD-JWT schema, the assembled
// template's CredentialType + Vct must NOT be the random Schema.ID. They
// must be CustomTypeName so the verifier's PD filter aligns with the
// catalog vct (which is also CustomTypeName) and the issued vct (also
// CustomTypeName).
//
// We can't drive assembleCustomTemplate end-to-end from a unit test
// (requires r.Form etc.), but we can check the building block: for a
// custom schema, BaseType() returns the random ID — that's the trap the
// new verifier branch sidesteps with picked.CustomTypeName().
func TestCustomSchemaBaseTypeIsTheTrap(t *testing.T) {
	s := vctypes.Schema{
		ID:              "custom-di5upc0ecas3",
		Name:            "Farmer Credential",
		AdditionalTypes: []string{"FarmerCredential"},
		Std:             "sd_jwt_vc (IETF)",
		Custom:          true,
	}
	// BaseType() returning ID verbatim is the documented behaviour for
	// non-suffix-matching IDs (vctypes/vctypes.go:180). The fix lives in
	// the verifier handler, not here — guard against accidentally changing
	// BaseType behaviour and breaking other callers.
	if bt := s.BaseType(); bt != "custom-di5upc0ecas3" {
		t.Errorf("BaseType() = %q, want the schema ID verbatim", bt)
	}
	if !strings.HasPrefix(s.Std, "sd_jwt_vc") {
		t.Fatalf("Std prefix check broke")
	}
	if name := s.CustomTypeName(); name != "FarmerCredential" {
		t.Errorf("CustomTypeName() = %q, want FarmerCredential", name)
	}
}
