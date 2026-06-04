// remote.go (added in v0.2) verifies signed remote RAG index manifests.
//
// Trust model: each workspace pins exactly one ed25519 public key at
// .foundry/keys/rag.pub (base64-encoded, raw 32-byte key). Every manifest
// the team publishes is signed with the matching private key; the sidecar
// refuses to download or merge any manifest that fails verification.
//
// The blob URL inside the manifest may be a SAS-signed pointer to customer
// Azure Blob Storage; the SAS lifetime is the only auth knob — Foundry never
// brokers the storage account credential.
package rag

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Manifest is the JSON object served from the team's signed index endpoint.
type Manifest struct {
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	Chunks    int       `json:"chunks"`
	BlobURL   string    `json:"blob_url"`
	Signature string    `json:"signature"`
}

// ErrManifestSignature is returned for any tampering or bad signature.
var ErrManifestSignature = errors.New("rag.remote: manifest signature verification failed")

// canonicalBytes returns a deterministic JSON encoding of m with Signature
// blanked. Both signer and verifier must use this exact encoding.
func canonicalBytes(m Manifest) ([]byte, error) {
	m.Signature = ""
	// json.Marshal in Go is deterministic for typed structs (field order is
	// declaration order, no map iteration), so this is safe.
	return json.Marshal(m)
}

// VerifyManifest returns nil on a valid signature, ErrManifestSignature otherwise.
func VerifyManifest(m Manifest, pubKey ed25519.PublicKey) error {
	if len(pubKey) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: public key size = %d, want %d", ErrManifestSignature, len(pubKey), ed25519.PublicKeySize)
	}
	if strings.TrimSpace(m.Signature) == "" {
		return fmt.Errorf("%w: empty signature", ErrManifestSignature)
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil {
		return fmt.Errorf("%w: signature decode: %v", ErrManifestSignature, err)
	}
	body, err := canonicalBytes(m)
	if err != nil {
		return fmt.Errorf("%w: canonical encode: %v", ErrManifestSignature, err)
	}
	if !ed25519.Verify(pubKey, body, sig) {
		return ErrManifestSignature
	}
	return nil
}

// SignManifest is the signer-side helper (used by tooling, not the sidecar).
// Returns a copy of m with Signature populated.
func SignManifest(m Manifest, priv ed25519.PrivateKey) (Manifest, error) {
	body, err := canonicalBytes(m)
	if err != nil {
		return Manifest{}, err
	}
	sig := ed25519.Sign(priv, body)
	m.Signature = base64.StdEncoding.EncodeToString(sig)
	return m, nil
}

// LoadPublicKey reads .foundry/keys/rag.pub. The file is base64-encoded raw
// 32-byte ed25519 public key, optionally with a trailing newline.
func LoadPublicKey(workspaceRoot string) (ed25519.PublicKey, error) {
	path := filepath.Join(workspaceRoot, ".foundry", "keys", "rag.pub")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("rag.remote: read pubkey: %w", err)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil {
		return nil, fmt.Errorf("rag.remote: decode pubkey: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("rag.remote: pubkey is %d bytes, want %d", len(raw), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}
