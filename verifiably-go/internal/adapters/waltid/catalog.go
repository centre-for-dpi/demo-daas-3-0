package waltid

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/verifiably/verifiably-go/vctypes"
)

// catalogMu serialises edits to credential-issuer-metadata.conf. Two custom
// schemas saved in quick succession would otherwise race on the read-modify-
// write of the HOCON file. Using package-level state is fine — the file is
// pinned to a single host path and only one verifiably-go process touches it.
var catalogMu sync.Mutex

// appendCredentialType registers a custom schema in walt.id's HOCON catalog
// (credential-issuer-metadata.conf) so its configurationId becomes a real
// member of credential_configurations_supported. Without this, walt.id rejects
// any configurationId it didn't see at boot — which is why earlier versions
// of this adapter "borrowed" a known configurationId and signed the user's
// custom data through it. Once the schema lives in the catalog, the borrow
// trick is no-op'd in IssueToWallet.
//
// Phase 1 supports jwt_vc_json only (the catalog format the user explicitly
// asked us to ship first). Other formats (vc+sd-jwt, mso_mdoc, ldp_vc,
// jwt_vc_json-ld) will be added in Phase 2 by extending the format switch
// below — the file-edit path is identical, only the entry block differs.
//
// Idempotent: a re-save of the same schema (matching configID + type) is a
// no-op so multiple deploys don't accumulate duplicate entries. Concurrent
// callers serialise via catalogMu.
func appendCredentialType(catalogPath string, schema vctypes.Schema) (configID string, changed bool, err error) {
	catalogMu.Lock()
	defer catalogMu.Unlock()

	typeName := customSchemaTypeName(schema)
	configID = customSchemaConfigID(schema)
	formatSuffix := waltidFormatSuffix(schema.Std)
	if formatSuffix == "" {
		return "", false, fmt.Errorf("walt.id catalog: format for Std=%q not yet supported in Phase 1 (jwt_vc_json only)", schema.Std)
	}

	data, err := os.ReadFile(catalogPath)
	if err != nil {
		return "", false, fmt.Errorf("read catalog: %w", err)
	}
	content := string(data)

	if strings.Contains(content, `"`+configID+`"`) {
		return configID, false, nil
	}

	simpleEntry := fmt.Sprintf("    %s = [VerifiableCredential, %s],", typeName, typeName)
	formatEntry := buildJWTVCJsonEntry(configID, typeName, schema)

	lastBrace := strings.LastIndex(content, "}")
	if lastBrace == -1 {
		return "", false, fmt.Errorf("invalid HOCON: no closing brace in %s", catalogPath)
	}

	newContent := content[:lastBrace] + "\n" + simpleEntry + "\n\n" + formatEntry + "\n" + content[lastBrace:]
	if err := os.WriteFile(catalogPath, []byte(newContent), 0o644); err != nil {
		return "", false, fmt.Errorf("write catalog: %w", err)
	}
	return configID, true, nil
}

// customSchemaTypeName returns the catalog/VC-type identifier for a custom
// schema. Prefers an explicitly-declared AdditionalType (via the builder's
// "Extra Type" field) so an operator who knows the canonical name can pin it;
// falls back to a CamelCased version of the schema name. Mirrors the type
// chosen in buildCredentialData so the catalog entry, the VC's `type` array
// and the configurationId all align.
func customSchemaTypeName(schema vctypes.Schema) string {
	if len(schema.AdditionalTypes) > 0 && strings.TrimSpace(schema.AdditionalTypes[0]) != "" {
		return strings.TrimSpace(schema.AdditionalTypes[0])
	}
	return sanitizeTypeName(schema.Name)
}

// customSchemaConfigID returns the catalog key the appended entry uses.
// Pattern matches walt.id's existing stock entries: "<TypeName>_<formatSuffix>"
// (e.g. "FarmerCred_jwt_vc_json"). BaseType() in vctypes strips the same
// suffixes so `Schema.BaseType()` of an issued credential collapses back to
// the type name.
func customSchemaConfigID(schema vctypes.Schema) string {
	return customSchemaTypeName(schema) + "_" + waltidFormatSuffix(schema.Std)
}

// waltidFormatSuffix maps verifiably-go's Std taxonomy to the walt.id format
// key used as a configurationId suffix. Empty return means "not yet
// supported"; appendCredentialType errors so the operator sees a clear
// message rather than a silent fall-through to the borrow trick.
//
// Phase 1 ships jwt_vc_json. Phase 2 will add: "ldp_vc" → "ldp_vc",
// "jwt_vc_json-ld" → "jwt_vc_json-ld", "sd_jwt_vc (IETF)" → "vc+sd-jwt",
// "mso_mdoc" → "mso_mdoc".
func waltidFormatSuffix(std string) string {
	switch std {
	case "w3c_vcdm_2", "jwt_vc", "":
		return "jwt_vc_json"
	default:
		return ""
	}
}

// buildJWTVCJsonEntry renders a HOCON block describing one jwt_vc_json
// credential configuration. The shape mirrors walt.id's stock entries
// (KiwiAccessCredential_jwt_vc_json, BirthCertificate_jwt_vc_json) verbatim
// so walt.id's CredentialTypeConfig deserialiser accepts it without complaint.
//
// EdDSA + ES256 are the two algorithms walt.id signs with by default; listing
// both makes the configuration usable from issuer DIDs of either curve.
func buildJWTVCJsonEntry(configID, typeName string, schema vctypes.Schema) string {
	display := strings.TrimSpace(schema.Name)
	if display == "" {
		display = typeName
	}
	desc := strings.TrimSpace(schema.Desc)
	if desc == "" || desc == "—" {
		desc = display
	}
	return fmt.Sprintf(`    "%s" = {
        format = "jwt_vc_json"
        cryptographic_binding_methods_supported = ["did"]
        credential_signing_alg_values_supported = ["EdDSA", "ES256"]
        credential_definition = {
            type = ["VerifiableCredential", "%s"]
        }
        display = [
            {
                name = "%s"
                description = "%s"
                locale = "en-US"
                background_color = "#FFFFFF"
                text_color = "#000000"
            }
        ]
    }`, configID, typeName, hoconEscape(display), hoconEscape(desc))
}

// hoconEscape prepares a free-text string for inclusion in a HOCON quoted
// string literal: backslashes first (so we don't double-escape the ones we
// add), then double quotes. HOCON otherwise treats `"` inside a quoted
// string as the terminator and silently truncates.
func hoconEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// removeCredentialType deletes the catalog entries for a custom schema so a
// DeleteCustomSchema cleans up the HOCON file as well as the in-memory
// registry. Called from the Adapter's DeleteCustomSchema hook.
//
// Removes both the simple-array form (one line ending in a comma) and the
// configID block (multi-line, balanced braces). Idempotent.
func removeCredentialType(catalogPath string, schema vctypes.Schema) error {
	catalogMu.Lock()
	defer catalogMu.Unlock()

	typeName := customSchemaTypeName(schema)
	configID := customSchemaConfigID(schema)

	data, err := os.ReadFile(catalogPath)
	if err != nil {
		return fmt.Errorf("read catalog: %w", err)
	}
	content := string(data)
	updated := stripSimpleEntry(content, typeName)
	updated = stripBlockEntry(updated, configID)
	if updated == content {
		return nil
	}
	return os.WriteFile(catalogPath, []byte(updated), 0o644)
}

// stripSimpleEntry removes a `    TypeName = [VerifiableCredential, TypeName],`
// line from the HOCON content. Tolerates leading whitespace variation.
func stripSimpleEntry(content, typeName string) string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	prefix := typeName + " ="
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) && strings.Contains(trimmed, "[VerifiableCredential") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// stripBlockEntry removes a `"<configID>" = { ... }` block by counting braces
// from the opening `{` until they balance. Walt.id's HOCON entries don't
// nest unbalanced braces so byte-counting is sufficient.
func stripBlockEntry(content, configID string) string {
	needle := `"` + configID + `" =`
	start := strings.Index(content, needle)
	if start == -1 {
		return content
	}
	open := strings.Index(content[start:], "{")
	if open == -1 {
		return content
	}
	open += start
	depth := 0
	end := -1
	for i := open; i < len(content); i++ {
		switch content[i] {
		case '{':
			depth++
		case '}':
			depth--
		}
		if depth == 0 {
			end = i + 1
			break
		}
	}
	if end == -1 {
		return content
	}
	for end < len(content) && (content[end] == '\n' || content[end] == '\r' || content[end] == ' ' || content[end] == '\t') {
		end++
	}
	return content[:start] + content[end:]
}
