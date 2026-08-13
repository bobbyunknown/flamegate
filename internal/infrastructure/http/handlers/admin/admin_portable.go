package admin

import (
	"fmt"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/shared/crypto"
)

// portableAccountSecret carries a single account's secrets in passphrase-encrypted
// form inside a portable backup. Each field is a base64 salt+ciphertext pair.
type portableAccountSecret struct {
	Secret  *crypto.PortableSecret `json:"secret,omitempty"`
	Token   *crypto.PortableSecret `json:"token,omitempty"`
	Refresh *crypto.PortableSecret `json:"refresh,omitempty"`
}

// exportPortableSecrets opens an account's master-key-sealed blobs and re-encrypts
// each present secret under the passphrase, writing the result into out under the
// "portable_secret" key. The master-key blobs are NOT included in a portable
// backup. Returns an error if any sealed blob cannot be opened (master key
// mismatch), so the caller can fail the export loudly rather than ship a broken
// backup.
func (s *Handler) exportPortableSecrets(out map[string]any, a schema.Account, passphrase string) error {
	if s.vault == nil {
		return fmt.Errorf("vault not configured")
	}
	sealer := s.vault.Sealer()
	ps := portableAccountSecret{}

	if a.SecretCiphertext != "" {
		plain, err := sealer.OpenString(crypto.Sealed{WrappedDEK: a.SecretWrappedDEK, Ciphertext: a.SecretCiphertext})
		if err != nil {
			return fmt.Errorf("open secret: %w", err)
		}
		sec, err := crypto.SealPortableString(passphrase, plain)
		if err != nil {
			return fmt.Errorf("reseal secret: %w", err)
		}
		ps.Secret = &sec
	}
	if a.TokenCiphertext != "" {
		plain, err := sealer.OpenString(crypto.Sealed{WrappedDEK: a.TokenWrappedDEK, Ciphertext: a.TokenCiphertext})
		if err != nil {
			return fmt.Errorf("open token: %w", err)
		}
		sec, err := crypto.SealPortableString(passphrase, plain)
		if err != nil {
			return fmt.Errorf("reseal token: %w", err)
		}
		ps.Token = &sec
	}
	if a.RefreshCiphertext != "" {
		plain, err := sealer.OpenString(crypto.Sealed{WrappedDEK: a.RefreshWrappedDEK, Ciphertext: a.RefreshCiphertext})
		if err != nil {
			return fmt.Errorf("open refresh: %w", err)
		}
		sec, err := crypto.SealPortableString(passphrase, plain)
		if err != nil {
			return fmt.Errorf("reseal refresh: %w", err)
		}
		ps.Refresh = &sec
	}

	out["portable_secret"] = ps
	return nil
}

// importPortableSecrets decrypts an account's passphrase-encrypted secrets and
// re-seals them under the local master key, writing the sealed blobs onto acc.
// A wrong passphrase fails AEAD authentication and returns an error.
func (s *Handler) importPortableSecrets(acc *schema.Account, ps portableAccountSecret, passphrase string) error {
	if s.vault == nil {
		return fmt.Errorf("vault not configured")
	}
	sealer := s.vault.Sealer()

	if ps.Secret != nil {
		plain, err := crypto.OpenPortableString(passphrase, *ps.Secret)
		if err != nil {
			return fmt.Errorf("open portable secret (wrong passphrase?): %w", err)
		}
		sealed, err := sealer.SealString(plain)
		if err != nil {
			return fmt.Errorf("seal secret: %w", err)
		}
		acc.SecretWrappedDEK = sealed.WrappedDEK
		acc.SecretCiphertext = sealed.Ciphertext
	}
	if ps.Token != nil {
		plain, err := crypto.OpenPortableString(passphrase, *ps.Token)
		if err != nil {
			return fmt.Errorf("open portable token (wrong passphrase?): %w", err)
		}
		sealed, err := sealer.SealString(plain)
		if err != nil {
			return fmt.Errorf("seal token: %w", err)
		}
		acc.TokenWrappedDEK = sealed.WrappedDEK
		acc.TokenCiphertext = sealed.Ciphertext
	}
	if ps.Refresh != nil {
		plain, err := crypto.OpenPortableString(passphrase, *ps.Refresh)
		if err != nil {
			return fmt.Errorf("open portable refresh (wrong passphrase?): %w", err)
		}
		sealed, err := sealer.SealString(plain)
		if err != nil {
			return fmt.Errorf("seal refresh: %w", err)
		}
		acc.RefreshWrappedDEK = sealed.WrappedDEK
		acc.RefreshCiphertext = sealed.Ciphertext
	}
	return nil
}
