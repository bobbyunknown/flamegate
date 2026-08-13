package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

// Portable secrets let a backup move between machines that do NOT share a
// master key. Instead of the master key (KEK), the data key is derived from a
// user-supplied passphrase via argon2id. On export each secret is re-keyed from
// the local master key to the passphrase; on import it is re-keyed back to the
// destination machine's master key. The passphrase never leaves the user.

// PortableSecret is the passphrase-encrypted form of a single secret. Salt is
// per-secret so each derivation is unique; Ciphertext is nonce||ct, base64.
type PortableSecret struct {
	Salt       string `json:"salt"`
	Ciphertext string `json:"ciphertext"`
}

// derivePortableKey stretches a passphrase into a 32-byte AES key using
// argon2id with the same cost parameters as API-key hashing.
func derivePortableKey(passphrase string, salt []byte) []byte {
	return argon2.IDKey([]byte(passphrase), salt, argonTime, argonMemory, argonThreads, KeySize)
}

// SealPortable encrypts plaintext under a key derived from the passphrase.
func SealPortable(passphrase string, plaintext []byte) (PortableSecret, error) {
	if passphrase == "" {
		return PortableSecret{}, fmt.Errorf("crypto: empty passphrase")
	}
	salt := make([]byte, argonSaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return PortableSecret{}, fmt.Errorf("generate salt: %w", err)
	}
	gcm, err := newGCM(derivePortableKey(passphrase, salt))
	if err != nil {
		return PortableSecret{}, err
	}
	ct, err := encrypt(gcm, plaintext)
	if err != nil {
		return PortableSecret{}, fmt.Errorf("encrypt portable: %w", err)
	}
	return PortableSecret{
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Ciphertext: base64.StdEncoding.EncodeToString(ct),
	}, nil
}

// OpenPortable decrypts a PortableSecret using the passphrase. A wrong
// passphrase fails AEAD authentication with ErrMalformedCiphertext.
func OpenPortable(passphrase string, sec PortableSecret) ([]byte, error) {
	if passphrase == "" {
		return nil, fmt.Errorf("crypto: empty passphrase")
	}
	salt, err := base64.StdEncoding.DecodeString(sec.Salt)
	if err != nil {
		return nil, fmt.Errorf("%w: salt: %v", ErrMalformedCiphertext, err)
	}
	ct, err := base64.StdEncoding.DecodeString(sec.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("%w: ciphertext: %v", ErrMalformedCiphertext, err)
	}
	gcm, err := newGCM(derivePortableKey(passphrase, salt))
	if err != nil {
		return nil, err
	}
	return decrypt(gcm, ct)
}

// SealPortableString is a convenience wrapper for string secrets.
func SealPortableString(passphrase, plaintext string) (PortableSecret, error) {
	return SealPortable(passphrase, []byte(plaintext))
}

// OpenPortableString is a convenience wrapper returning a string.
func OpenPortableString(passphrase string, sec PortableSecret) (string, error) {
	b, err := OpenPortable(passphrase, sec)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
