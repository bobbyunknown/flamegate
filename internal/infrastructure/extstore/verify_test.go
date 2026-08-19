package extstore

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"testing"
)

// buildTestZip writes an archive with the given files and returns its path.
func buildTestZip(t *testing.T, files map[string]string) string {
	t.Helper()
	path := t.TempDir() + "/ext.zip"
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for name, content := range files {
		hw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := hw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func sha256sum(t *testing.T, data string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}

// signedZip builds a zip whose SHA256SUMS is signed by the private key.
func signedZip(t *testing.T, priv ed25519.PrivateKey, includeSig bool) string {
	t.Helper()
	files := map[string]string{"schema.wasm": "wasm-bytes"}
	sumsLine := sha256sum(t, files["schema.wasm"]) + "  schema.wasm\n"
	if includeSig {
		if len(priv) != ed25519.PrivateKeySize {
			t.Fatal("signedZip requires a valid private key")
		}
		sig := ed25519.Sign(priv, []byte(sumsLine))
		files["SHA256SUMS"] = sumsLine
		files["SHA256SUMS.sig"] = string(sig) // raw 64-byte binary in zip
		return buildTestZip(t, files)
	}
	files["SHA256SUMS"] = sumsLine
	return buildTestZip(t, files)
}

func pubHex(t *testing.T, pub ed25519.PublicKey) string {
	t.Helper()
	return hex.EncodeToString(pub)
}

func TestVerifyOfficialSigned(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	zipPath := signedZip(t, priv, true)
	v, err := VerifyArchive(zipPath, []string{pubHex(t, pub)}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if v.Trust != TrustOfficial {
		t.Fatalf("trust = %v, want official", v.Trust)
	}
	if !v.ChecksumOK || !v.SignatureOK {
		t.Fatalf("v = %+v", v)
	}
}

func TestVerifyOfficialNoSig(t *testing.T) {
	zipPath := signedZip(t, nil, false) // no sig regardless
	_, err := VerifyArchive(zipPath, []string{}, true, true)
	if err == nil {
		t.Fatal("official unsigned should fail")
	}
	if !errors.Is(err, ErrSignatureMissing) && !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("err = %v", err)
	}
}

func TestVerifyCommunityUnsignedAllowed(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	zipPath := signedZip(t, nil, false)
	v, err := VerifyArchive(zipPath, []string{pubHex(t, pub)}, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if v.Trust != TrustCommunity {
		t.Fatalf("trust = %v, want community", v.Trust)
	}
	if v.SignatureGiven {
		t.Fatal("no signature expected")
	}
}

func TestVerifyCommunityUnsignedStrict(t *testing.T) {
	zipPath := signedZip(t, nil, false)
	_, err := VerifyArchive(zipPath, nil, false, false)
	if !errors.Is(err, ErrUntrustedSource) {
		t.Fatalf("err = %v, want ErrUntrustedSource", err)
	}
}

func TestVerifyCommunitySignedValid(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	zipPath := signedZip(t, priv, true)
	v, err := VerifyArchive(zipPath, []string{pubHex(t, pub)}, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if v.Trust != TrustCommunity {
		t.Fatalf("trust = %v, want community", v.Trust)
	}
	if !v.SignatureOK {
		t.Fatalf("expected valid signature")
	}
}

func TestVerifyInvalidSignature(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	zipPath := signedZip(t, priv, true)
	_, err := VerifyArchive(zipPath, []string{pubHex(t, otherPub)}, true, false)
	if !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("err = %v, want ErrSignatureInvalid", err)
	}
}

func TestVerifyChecksumMismatch(t *testing.T) {
	zipPath := buildTestZip(t, map[string]string{
		"schema.json": "{}",
		"codex.wasm":  "not-really-wasm",
		"SHA256SUMS":  "0000000000000000000000000000000000000000000000000000000000000000  codex.wasm\n",
	})
	_, err := VerifyArchive(zipPath, nil, true, false)
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("err = %v, want ErrChecksumMismatch", err)
	}
}

func TestVerifyMissingManifest(t *testing.T) {
	zipPath := buildTestZip(t, map[string]string{"schema.json": "{}", "codex.wasm": "x"})
	_, err := VerifyArchive(zipPath, nil, true, false)
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("err = %v, want ErrChecksumMismatch", err)
	}
}