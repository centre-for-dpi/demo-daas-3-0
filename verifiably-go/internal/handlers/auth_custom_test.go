package handlers

import "testing"

// TestSlugify checks the display-name → registry-ID transform used by
// AddCustomProvider. Important because the slug becomes the URL key
// /auth/start sees, so deterministic + URL-safe output matters.
func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"My Keycloak":              "my-keycloak",
		"My  Keycloak  ":           "my-keycloak",
		"  --leading + trailing--": "leading-trailing",
		"Auth0 (prod)":             "auth0-prod",
		"WSO2 / IS":                "wso2-is",
		"":                         "",
		"!!!":                      "",
		"Internal IdP — staging":   "internal-idp-staging",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
