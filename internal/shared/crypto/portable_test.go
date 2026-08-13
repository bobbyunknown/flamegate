package crypto

import (
	"testing"
)

func TestPortableRoundTrip(t *testing.T) {
	const pass = "correct horse battery staple"
	const secret = "sk-super-secret-value-123"

	sealed, err := SealPortableString(pass, secret)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if sealed.Salt == "" || sealed.Ciphertext == "" {
		t.Fatal("sealed fields empty")
	}

	got, err := OpenPortableString(pass, sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if got != secret {
		t.Fatalf("round trip mismatch: got %q want %q", got, secret)
	}
}

func TestPortableWrongPassphraseFails(t *testing.T) {
	sealed, err := SealPortableString("right-pass", "secret")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := OpenPortableString("wrong-pass", sealed); err == nil {
		t.Fatal("expected error opening with wrong passphrase, got nil")
	}
}

func TestPortableEmptyPassphraseRejected(t *testing.T) {
	if _, err := SealPortableString("", "secret"); err == nil {
		t.Fatal("expected error sealing with empty passphrase")
	}
	if _, err := OpenPortableString("", PortableSecret{Salt: "x", Ciphertext: "y"}); err == nil {
		t.Fatal("expected error opening with empty passphrase")
	}
}

func TestPortableUniqueSaltPerSeal(t *testing.T) {
	a, err := SealPortableString("pass", "secret")
	if err != nil {
		t.Fatalf("seal a: %v", err)
	}
	b, err := SealPortableString("pass", "secret")
	if err != nil {
		t.Fatalf("seal b: %v", err)
	}
	if a.Salt == b.Salt {
		t.Fatal("expected unique salt per seal")
	}
	if a.Ciphertext == b.Ciphertext {
		t.Fatal("expected unique ciphertext per seal (random salt+nonce)")
	}
}
