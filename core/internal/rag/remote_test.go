package rag

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func makeManifest() Manifest {
	return Manifest{
		Version:   1,
		CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Chunks:    1000,
		BlobURL:   "https://acme.blob.core.windows.net/foundry/rag.gob?sv=...",
	}
}

func TestVerifyManifest_ValidSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := SignManifest(makeManifest(), priv)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyManifest(signed, pub); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestVerifyManifest_TamperedManifest(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	signed, _ := SignManifest(makeManifest(), priv)
	signed.Chunks = 1_000_000 // tamper post-signature
	err := VerifyManifest(signed, pub)
	if !errors.Is(err, ErrManifestSignature) {
		t.Fatalf("want ErrManifestSignature, got %v", err)
	}
}

func TestVerifyManifest_BadSignature(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	m := makeManifest()
	m.Signature = base64.StdEncoding.EncodeToString(make([]byte, 64)) // 64 zero bytes
	err := VerifyManifest(m, pub)
	if !errors.Is(err, ErrManifestSignature) {
		t.Fatalf("want ErrManifestSignature, got %v", err)
	}
}

func TestVerifyManifest_EmptySignature(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	m := makeManifest()
	err := VerifyManifest(m, pub)
	if !errors.Is(err, ErrManifestSignature) {
		t.Fatalf("want ErrManifestSignature, got %v", err)
	}
}

func TestVerifyManifest_WrongKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	pub2, _, _ := ed25519.GenerateKey(rand.Reader)
	signed, _ := SignManifest(makeManifest(), priv)
	err := VerifyManifest(signed, pub2)
	if !errors.Is(err, ErrManifestSignature) {
		t.Fatalf("want ErrManifestSignature, got %v", err)
	}
}

func TestLoadPublicKey_MissingFile(t *testing.T) {
	_, err := LoadPublicKey(t.TempDir())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadPublicKey_ValidFile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".foundry", "keys")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	b64 := base64.StdEncoding.EncodeToString(pub)
	if err := os.WriteFile(filepath.Join(dir, "rag.pub"), []byte(b64+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPublicKey(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != ed25519.PublicKeySize {
		t.Fatalf("size mismatch: %d", len(got))
	}
}

func TestLoadPublicKey_WrongSize(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".foundry", "keys")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "rag.pub"), []byte(base64.StdEncoding.EncodeToString([]byte("short"))), 0o644)
	_, err := LoadPublicKey(root)
	if err == nil {
		t.Fatal("expected size error")
	}
}
