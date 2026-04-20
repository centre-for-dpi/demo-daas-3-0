package oidc

import "testing"

func TestSwapAuthority(t *testing.T) {
	tests := []struct {
		name, endpoint, authority, want string
	}{
		{
			name:      "localhost→container",
			endpoint:  "https://localhost:9443/oauth2/token",
			authority: "https://wso2is:9443",
			want:      "https://wso2is:9443/oauth2/token",
		},
		{
			name:      "container→public",
			endpoint:  "https://wso2is:9443/oauth2/authorize?foo=1",
			authority: "https://172.24.0.1:9443",
			want:      "https://172.24.0.1:9443/oauth2/authorize?foo=1",
		},
		{
			name:      "preserves path, query, scheme change",
			endpoint:  "http://keycloak:8180/realms/x/protocol/openid-connect/token",
			authority: "https://auth.example.com",
			want:      "https://auth.example.com/realms/x/protocol/openid-connect/token",
		},
		{
			name:      "empty endpoint is passthrough",
			endpoint:  "",
			authority: "https://x:1",
			want:      "",
		},
		{
			name:      "empty authority is passthrough",
			endpoint:  "https://localhost:9443/oauth2/token",
			authority: "",
			want:      "https://localhost:9443/oauth2/token",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := swapAuthority(tt.endpoint, tt.authority)
			if got != tt.want {
				t.Errorf("swapAuthority(%q, %q) = %q, want %q", tt.endpoint, tt.authority, got, tt.want)
			}
		})
	}
}

func TestURLAuthority(t *testing.T) {
	tests := map[string]string{
		"https://wso2is:9443/oauth2/token": "https://wso2is:9443",
		"https://172.24.0.1:9443":          "https://172.24.0.1:9443",
		"http://keycloak:8180/realms/x":    "http://keycloak:8180",
		"":                                  "",
	}
	for in, want := range tests {
		if got := urlAuthority(in); got != want {
			t.Errorf("urlAuthority(%q) = %q, want %q", in, got, want)
		}
	}
}
