package extstore

import (
	"archive/zip"
	"bufio"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

// Verification is the outcome of archive verification.
type Verification struct {
	ChecksumOK      bool
	SignatureOK     bool
	SignatureGiven  bool
	Trust           TrustLevel
}

// VerifyArchive verifies a downloaded extension archive:
//   - Every file listed in SHA256SUMS must hash to the listed value.
//   - If SHA256SUMS.sig is present it is verified against the SHA256SUMS bytes
//     with one of the supplied public keys. A present-but-invalid signature is
//     a hard failure even in unsigned-allowed mode.
//   - isOfficial (store source) requires a valid signature; otherwise the result
//     depends on allowUnsigned.
//
// pubKeys are hex-encoded 32-byte Ed25519 public keys.
func VerifyArchive(zipPath string, pubKeys []string, allowUnsigned, isOfficial bool) (*Verification, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("verify: open zip: %w", err)
	}
	defer zr.Close()

	sumsBytes, sums, err := readSHA256SUMS(zr)
	if err != nil {
		return nil, err
	}
	if sums == nil {
		return nil, ErrChecksumMismatch // manifest missing
	}

	if err := verifyAllChecksums(zr, sums); err != nil {
		return nil, err
	}

	sigBytes := readSignature(zr)
	hasSig := len(sigBytes) > 0
	sigOK := false
	if hasSig {
		ok, err := verifySignature(sumsBytes, sigBytes, pubKeys)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrSignatureInvalid
		}
		sigOK = true
	}

	trust := TrustCommunity
	if hasSig && sigOK {
		trust = TrustCommunity // verified signature, non-official pubkey still community
	}
	if isOfficial {
		if !hasSig || !sigOK {
			return nil, ErrSignatureMissing
		}
		trust = TrustOfficial
	}
	if !hasSig {
		if !allowUnsigned {
			return nil, ErrUntrustedSource
		}
		trust = TrustCommunity
	}

	v := &Verification{ChecksumOK: true, SignatureGiven: hasSig, SignatureOK: sigOK, Trust: trust}
	if isOfficial && !sigOK {
		v.Trust = TrustOfficial // official chain already errored above; defensive
	}
	return v, nil
}

// readSHA256SUMS returns the raw SHA256SUMS bytes and the parsed map, or (nil,
// nil, nil) if the manifest is absent. If present but malformed it returns an
// error distinct from "missing".
func readSHA256SUMS(zr *zip.ReadCloser) ([]byte, map[string]string, error) {
	f, err := zr.Open("SHA256SUMS")
	if err != nil {
		return nil, nil, nil // absent
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, nil, fmt.Errorf("verify: read SHA256SUMS: %w", err)
	}
	sums := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, nil, fmt.Errorf("verify: malformed SHA256SUMS line: %q", line)
		}
		sums[fields[1]] = strings.ToLower(fields[0])
	}
	if err := sc.Err(); err != nil {
		return nil, nil, err
	}
	return data, sums, nil
}

func verifyAllChecksums(zr *zip.ReadCloser, sums map[string]string) error {
	// Every entry in the manifest must exist in the archive with a matching hash.
	for name, wantHex := range sums {
		name = strings.TrimPrefix(name, "./")
		rc, err := zr.Open(name)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrChecksumMismatch, name)
		}
		h := sha256.New()
		if _, err := io.Copy(h, rc); err != nil {
			rc.Close() //nolint:errcheck
			return fmt.Errorf("verify: hash %s: %w", name, err)
		}
		_ = rc.Close()
		got := hex.EncodeToString(h.Sum(nil))
		if got != wantHex {
			return fmt.Errorf("%w: %s (got %s want %s)", ErrChecksumMismatch, name, got, wantHex)
		}
	}
	return nil
}

func readSignature(zr *zip.ReadCloser) []byte {
	for _, f := range zr.File {
		if strings.EqualFold(f.Name, "SHA256SUMS.sig") {
			rc, err := f.Open()
			if err != nil {
				return nil
			}
			defer rc.Close() //nolint:errcheck
			data, _ := io.ReadAll(rc)
			return data
		}
	}
	return nil
}

// verifySignature checks a signature over the SHA256SUMS bytes. Supports raw
// 64-byte binary signatures and 128-hex-char representations.
func verifySignature(message, sigBytes []byte, pubKeys []string) (bool, error) {
	if len(pubKeys) == 0 {
		return false, nil
	}
	var sig []byte
	if len(sigBytes) == ed25519.SignatureSize {
		sig = sigBytes
	} else if len(sigBytes) == ed25519.SignatureSize*2 {
		dec, err := hex.DecodeString(strings.TrimSpace(string(sigBytes)))
		if err != nil {
			return false, fmt.Errorf("verify: invalid hex signature: %w", err)
		}
		sig = dec
	} else {
		return false, nil
	}

	for _, k := range pubKeys {
		pub, err := decodePubKey(k)
		if err != nil {
			continue // skip malformed configured key
		}
		if ed25519.Verify(pub, message, sig) {
			return true, nil
		}
	}
	return false, nil
}

func decodePubKey(k string) (ed25519.PublicKey, error) {
	k = strings.TrimSpace(k)
	raw, err := hex.DecodeString(k)
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key must be %d bytes", ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}