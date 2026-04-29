package waltid

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verifiably/verifiably-go/vctypes"
)

// seedCatalog mirrors the production baseline at
// deploy/compose/stack/issuer-api/config/credential-issuer-metadata.conf
// in miniature. Tests don't need every walt.id stock entry — what matters
// is that the file ends with a closing brace so appendCredentialType has
// somewhere to splice into.
const seedCatalog = `supportedCredentialTypes = {
    BankId = [VerifiableCredential, BankId],
    UniversityDegree = [VerifiableCredential, UniversityDegree],
}
`

func writeSeed(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "credential-issuer-metadata.conf")
	if err := os.WriteFile(path, []byte(seedCatalog), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return path
}

func TestAppendCredentialType_jwtVcJson(t *testing.T) {
	path := writeSeed(t)
	schema := vctypes.Schema{
		ID:     "custom-abc123",
		Name:   "Farmer Cred",
		Desc:   "Identity for verified farmers",
		Std:    "w3c_vcdm_2",
		Custom: true,
	}
	cfgID, changed, err := appendCredentialType(path, schema)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true on first save")
	}
	want := "FarmerCred_jwt_vc_json"
	if cfgID != want {
		t.Fatalf("configID = %q, want %q", cfgID, want)
	}
	got, _ := os.ReadFile(path)
	gotStr := string(got)
	for _, frag := range []string{
		`FarmerCred = [VerifiableCredential, FarmerCred],`,
		`"FarmerCred_jwt_vc_json" = {`,
		`format = "jwt_vc_json"`,
		`type = ["VerifiableCredential", "FarmerCred"]`,
		`name = "Farmer Cred"`,
		`description = "Identity for verified farmers"`,
	} {
		if !strings.Contains(gotStr, frag) {
			t.Errorf("expected file to contain %q\n--full file--\n%s", frag, gotStr)
		}
	}
	// Outer object must still close cleanly — last non-whitespace char is }.
	trimmed := strings.TrimRight(gotStr, " \t\r\n")
	if !strings.HasSuffix(trimmed, "}") {
		t.Errorf("file does not end with closing brace; got tail %q", trimmed[len(trimmed)-min(50, len(trimmed)):])
	}
}

func TestAppendCredentialType_idempotent(t *testing.T) {
	path := writeSeed(t)
	schema := vctypes.Schema{
		ID: "custom-x", Name: "Foo", Std: "w3c_vcdm_2", Custom: true,
	}
	if _, changed, err := appendCredentialType(path, schema); err != nil || !changed {
		t.Fatalf("first append: changed=%v err=%v", changed, err)
	}
	if _, changed, err := appendCredentialType(path, schema); err != nil || changed {
		t.Fatalf("second append: changed=%v (want false), err=%v", changed, err)
	}
	got, _ := os.ReadFile(path)
	if c := strings.Count(string(got), `"Foo_jwt_vc_json"`); c != 1 {
		t.Errorf("expected 1 entry, got %d duplicates", c)
	}
}

func TestAppendCredentialType_unsupportedFormat(t *testing.T) {
	path := writeSeed(t)
	schema := vctypes.Schema{
		ID: "custom-y", Name: "Bar", Std: "mso_mdoc", Custom: true,
	}
	if _, _, err := appendCredentialType(path, schema); err == nil {
		t.Fatalf("expected error for Phase-2 format mso_mdoc")
	}
}

func TestRemoveCredentialType_roundTrip(t *testing.T) {
	path := writeSeed(t)
	schema := vctypes.Schema{
		ID: "custom-z", Name: "Baz Bat", Desc: "test", Std: "w3c_vcdm_2", Custom: true,
	}
	if _, _, err := appendCredentialType(path, schema); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := removeCredentialType(path, schema); err != nil {
		t.Fatalf("remove: %v", err)
	}
	got, _ := os.ReadFile(path)
	gotStr := string(got)
	if strings.Contains(gotStr, "BazBat") {
		t.Errorf("expected BazBat removed, but it survived:\n%s", gotStr)
	}
	// Seed entries must remain.
	for _, want := range []string{"BankId", "UniversityDegree"} {
		if !strings.Contains(gotStr, want) {
			t.Errorf("expected seed entry %q to survive removal\n%s", want, gotStr)
		}
	}
	trimmed := strings.TrimRight(gotStr, " \t\r\n")
	if !strings.HasSuffix(trimmed, "}") {
		t.Errorf("file lost its closing brace: %q", trimmed)
	}
}

func TestAppendCredentialType_extraTypeOverride(t *testing.T) {
	// AdditionalTypes (the builder's "Extra Type" field) should pin the
	// type name verbatim — operators who know the canonical name (e.g.
	// matching an external schema) can override sanitizeTypeName.
	path := writeSeed(t)
	schema := vctypes.Schema{
		ID:              "custom-q",
		Name:            "doesnt matter",
		AdditionalTypes: []string{"FarmCertificate"},
		Std:             "w3c_vcdm_2",
		Custom:          true,
	}
	cfgID, _, err := appendCredentialType(path, schema)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if cfgID != "FarmCertificate_jwt_vc_json" {
		t.Errorf("configID = %q, want FarmCertificate_jwt_vc_json", cfgID)
	}
}

