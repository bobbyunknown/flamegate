package crypto

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	const pw = "s3cr3t-dashboard-password!"
	hash, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == pw {
		t.Fatal("hash equals plaintext; password not hashed")
	}

	ok, err := VerifyPassword(pw, hash)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatal("VerifyPassword returned false for correct password")
	}
}

func TestPasswordWrongDoesNotVerify(t *testing.T) {
	hash, err := HashPassword("correct-horse")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	ok, err := VerifyPassword("wrong-horse", hash)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if ok {
		t.Fatal("VerifyPassword returned true for wrong password")
	}
}

func TestPasswordHashIsSalted(t *testing.T) {
	// Two hashes of the same password must differ due to random salts.
	h1, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	h2, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if h1 == h2 {
		t.Fatal("two hashes of the same password are identical; salt missing")
	}
}

func TestVerifyPasswordInvalidHash(t *testing.T) {
	if _, err := VerifyPassword("whatever", "not-a-valid-hash"); err == nil {
		t.Fatal("expected error for malformed hash, got nil")
	}
}

func TestPasswordEmpty(t *testing.T) {
	hash, err := HashPassword("")
	if err != nil {
		t.Fatalf("HashPassword(empty): %v", err)
	}
	ok, err := VerifyPassword("", hash)
	if err != nil {
		t.Fatalf("VerifyPassword(empty): %v", err)
	}
	if !ok {
		t.Fatal("empty password did not verify against its own hash")
	}
}
