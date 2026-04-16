// Package ldpsigner implements an in-process W3C Verifiable Credential signer
// using JSON-LD (LDP_VC) format with Ed25519Signature2020 proofs.
//
// Why this exists: Walt.id's issuer-api HTTP service only exposes
// /openid4vc/jwt/issue, /openid4vc/sdjwt/issue, /openid4vc/mdoc/issue — it
// does NOT expose an LDP_VC endpoint. But LDP_VC is the W3C canonical format
// that Inji Verify, CREDEBL Agent, and the standalone verification-adapter
// all verify cryptographically. To make "Walt.id issues, Inji Verify verifies"
// work, we sign LDP_VC credentials in-process using the same URDNA2015
// two-hash pattern the adapter's issue-and-verify tool uses.
//
// The signing pattern (W3C Data Integrity):
//
//  1. Separate the credential into `document` (credential minus proof) and
//     `proofOptions` (the proof block without proofValue, plus @context).
//  2. Canonicalize each via URDNA2015 → N-Quads.
//  3. hashData = SHA256(canonicalize(proofOptions)) || SHA256(canonicalize(document))
//  4. signature = Ed25519.Sign(privKey, hashData)
//  5. proofValue = "z" + base58btc(signature)
//
// This matches what's in verification-adapter/test/issue-and-verify/main.go.
package ldpsigner

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/mr-tron/base58"
	"github.com/piprate/json-gold/ld"
)

// Signer is a reusable in-process LDP_VC signer. It holds a long-lived
// Ed25519 issuer keypair (regenerated each process startup — a real
// deployment would persist the key). The canonicalizer is goroutine-safe
// via an internal mutex.
type Signer struct {
	mu      sync.Mutex
	pubKey  ed25519.PublicKey
	privKey ed25519.PrivateKey
	did     string
	proc    *ld.JsonLdProcessor
	opts    *ld.JsonLdOptions
}

// New creates a new Signer with a fresh Ed25519 keypair and derives a did:key
// identifier. The keypair is process-lifetime; restart the server to rotate.
func New() (*Signer, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	opts := ld.NewJsonLdOptions("")
	opts.Algorithm = "URDNA2015"
	opts.Format = "application/n-quads"
	return &Signer{
		pubKey:  pub,
		privKey: priv,
		did:     deriveDidKey(pub),
		proc:    ld.NewJsonLdProcessor(),
		opts:    opts,
	}, nil
}

// DID returns the signer's issuer DID (did:key:z...).
func (s *Signer) DID() string { return s.did }

// Sign produces a signed W3C Verifiable Credential in LDP_VC format.
// The `credentialType` list supplements VerifiableCredential (e.g. ["UniversityDegree"]).
// `credentialSubject` holds the claim fields.
func (s *Signer) Sign(credentialType []string, credentialSubject map[string]any, subjectDID string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Build the credential. VCDM 1.1 with Ed25519Signature2020 context.
	now := time.Now().UTC().Format(time.RFC3339)
	// Build type array: always start with VerifiableCredential, then add
	// any caller-provided types that aren't already VerifiableCredential.
	types := []any{"VerifiableCredential"}
	for _, t := range credentialType {
		if t != "VerifiableCredential" {
			types = append(types, t)
		}
	}

	// Ensure credentialSubject has an id.
	if subjectDID != "" {
		credentialSubject["id"] = subjectDID
	} else if _, ok := credentialSubject["id"]; !ok {
		credentialSubject["id"] = "did:key:z6MkSubjectPlaceholder"
	}

	credential := map[string]any{
		"@context": []any{
			"https://www.w3.org/2018/credentials/v1",
			// @vocab fallback: maps all custom claim fields (fullName, district,
			// etc.) to schema.org IRIs during URDNA2015 canonicalization. Without
			// this, different JSON-LD implementations (Go json-gold vs Java
			// titanium-json-ld) handle undefined terms inconsistently, causing
			// signature verification to fail across implementations (e.g., our
			// Go-signed credentials fail Inji Verify's Java LdpVerifier).
			map[string]any{"@vocab": "https://schema.org/"},
			"https://w3id.org/security/suites/ed25519-2020/v1",
		},
		"id":                fmt.Sprintf("urn:uuid:%s", randomUUID()),
		"type":              types,
		"issuer":            s.did,
		"issuanceDate":      now,
		"credentialSubject": credentialSubject,
	}

	verificationMethod := s.did + "#" + s.did[8:] // did:key:z6Mk...#z6Mk...
	proofOptions := map[string]any{
		"@context":           credential["@context"],
		"type":               "Ed25519Signature2020",
		"created":            now,
		"verificationMethod": verificationMethod,
		"proofPurpose":       "assertionMethod",
	}

	canonDoc, err := s.canonicalize(credential)
	if err != nil {
		return nil, fmt.Errorf("canonicalize document: %w", err)
	}
	canonProof, err := s.canonicalize(proofOptions)
	if err != nil {
		return nil, fmt.Errorf("canonicalize proof options: %w", err)
	}

	// Two-hash pattern: SHA256(canonProof) || SHA256(canonDoc).
	proofHash := sha256.Sum256([]byte(canonProof))
	docHash := sha256.Sum256([]byte(canonDoc))
	hashData := append(proofHash[:], docHash[:]...)

	sig := ed25519.Sign(s.privKey, hashData)
	proofValue := "z" + base58.Encode(sig)

	credential["proof"] = map[string]any{
		"type":               "Ed25519Signature2020",
		"created":            now,
		"verificationMethod": verificationMethod,
		"proofPurpose":       "assertionMethod",
		"proofValue":         proofValue,
	}
	return credential, nil
}

// SignJSON is a convenience wrapper that returns the signed credential as JSON bytes.
func (s *Signer) SignJSON(credentialType []string, credentialSubject map[string]any, subjectDID string) ([]byte, error) {
	cred, err := s.Sign(credentialType, credentialSubject, subjectDID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(cred)
}

// PublicKey returns the Ed25519 public key. Used by the /sync endpoint of an
// external verification adapter so it can cache the issuer key for later
// offline verification.
func (s *Signer) PublicKey() ed25519.PublicKey { return s.pubKey }

// canonicalize runs URDNA2015 normalization on a JSON-LD document.
func (s *Signer) canonicalize(doc map[string]any) (string, error) {
	raw, _ := json.Marshal(doc)
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return "", err
	}
	result, err := s.proc.Normalize(normalized, s.opts)
	if err != nil {
		return "", err
	}
	s2, ok := result.(string)
	if !ok {
		return "", fmt.Errorf("unexpected normalize result type %T", result)
	}
	return s2, nil
}

// deriveDidKey produces a did:key from an Ed25519 public key.
// Format: did:key:z + base58btc(0xed01 + pubkey).
func deriveDidKey(pub ed25519.PublicKey) string {
	multicodec := append([]byte{0xed, 0x01}, pub...)
	return "did:key:z" + base58.Encode(multicodec)
}

func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, v := range ss {
		out[i] = v
	}
	return out
}

func randomUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	// RFC 4122 variant + version 4
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
