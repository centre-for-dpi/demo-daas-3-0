package main

import (
	"reflect"
	"testing"

	"github.com/verifiably/verifiably-go/internal/auth"
)

// TestApplyEnvOverrides_PerField checks the per-provider scalar override
// path: an operator only needs to set what changed (typically issuer URL +
// client secret) without re-typing the whole provider config.
func TestApplyEnvOverrides_PerField(t *testing.T) {
	t.Setenv("VERIFIABLY_OIDC_KEYCLOAK_ISSUER_URL", "https://idp.example.com/realms/foo")
	t.Setenv("VERIFIABLY_OIDC_KEYCLOAK_CLIENT_SECRET", "hunter2")
	t.Setenv("VERIFIABLY_OIDC_KEYCLOAK_SCOPES", "openid, profile, email, custom_scope")

	in := []auth.ProviderConfig{
		{
			ID:        "keycloak",
			Type:      "oidc",
			IssuerURL: "http://localhost:8180/realms/old",
			ClientID:  "vcplatform",
			Scopes:    []string{"openid"},
		},
	}
	out := applyEnvOverrides(in)
	if out[0].IssuerURL != "https://idp.example.com/realms/foo" {
		t.Errorf("IssuerURL not overridden: %s", out[0].IssuerURL)
	}
	if out[0].ClientSecret != "hunter2" {
		t.Errorf("ClientSecret not overridden: %s", out[0].ClientSecret)
	}
	if !reflect.DeepEqual(out[0].Scopes, []string{"openid", "profile", "email", "custom_scope"}) {
		t.Errorf("Scopes CSV not parsed: %v", out[0].Scopes)
	}
	if out[0].ClientID != "vcplatform" {
		t.Errorf("ClientID should be untouched, got %s", out[0].ClientID)
	}
}

// TestApplyEnvOverrides_HyphenatedID covers the env-name normalisation:
// dashes/dots in the provider ID become underscores in the env var so
// "my-idp" reads VERIFIABLY_OIDC_MY_IDP_ISSUER_URL. Without this, an
// operator would have to know the exact transform up-front.
func TestApplyEnvOverrides_HyphenatedID(t *testing.T) {
	t.Setenv("VERIFIABLY_OIDC_MY_IDP_ISSUER_URL", "https://my-idp.test")
	in := []auth.ProviderConfig{{ID: "my-idp", Type: "oidc"}}
	out := applyEnvOverrides(in)
	if out[0].IssuerURL != "https://my-idp.test" {
		t.Errorf("hyphen→underscore env name not honoured: %s", out[0].IssuerURL)
	}
}

// TestApplyEnvOverrides_NoEnvLeavesConfigsUnchanged guards against an
// empty env clobbering values.
func TestApplyEnvOverrides_NoEnvLeavesConfigsUnchanged(t *testing.T) {
	in := []auth.ProviderConfig{
		{ID: "x", IssuerURL: "https://kept", ClientID: "kept", ClientSecret: "kept"},
	}
	out := applyEnvOverrides(in)
	if !reflect.DeepEqual(out, in) {
		t.Errorf("empty env mutated config: %+v vs %+v", out, in)
	}
}

// TestLoadProviderConfigs_EnvJSONOverridesFile verifies precedence: when
// VERIFIABLY_OIDC_PROVIDERS is set, it wins regardless of what the JSON
// file would have contained. This is the "single-line update to swap the
// whole IdP stack" knob.
func TestLoadProviderConfigs_EnvJSONOverridesFile(t *testing.T) {
	t.Setenv("VERIFIABLY_OIDC_PROVIDERS", `[{"id":"my_custom","type":"oidc","displayName":"My Custom","issuerUrl":"https://custom.example.com","clientId":"foo","clientSecret":"bar"}]`)
	// Point the file lookup at /dev/null so we'd notice if it leaked through.
	t.Setenv("VERIFIABLY_AUTH_PROVIDERS_FILE", "/dev/null")
	cfgs, source := loadProviderConfigs()
	if len(cfgs) != 1 || cfgs[0].ID != "my_custom" {
		t.Fatalf("env JSON did not win, got %+v (source=%s)", cfgs, source)
	}
	if source != "VERIFIABLY_OIDC_PROVIDERS env" {
		t.Errorf("source label should mention env, got %q", source)
	}
}

// TestLoadProviderConfigs_EnvJSONLayersWithFieldOverrides confirms the
// two env-var paths compose: VERIFIABLY_OIDC_PROVIDERS sets the base,
// then per-field overrides patch specific fields without re-declaring
// the whole entry.
func TestLoadProviderConfigs_EnvJSONLayersWithFieldOverrides(t *testing.T) {
	t.Setenv("VERIFIABLY_OIDC_PROVIDERS", `[{"id":"my_idp","type":"oidc","issuerUrl":"https://base","clientId":"base","clientSecret":"base"}]`)
	t.Setenv("VERIFIABLY_OIDC_MY_IDP_CLIENT_SECRET", "rotated")
	cfgs, _ := loadProviderConfigs()
	if cfgs[0].ClientSecret != "rotated" {
		t.Errorf("per-field override should layer on env JSON: %+v", cfgs[0])
	}
	if cfgs[0].IssuerURL != "https://base" {
		t.Errorf("untouched fields should keep env-JSON value: %+v", cfgs[0])
	}
}
