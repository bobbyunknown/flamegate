package crypto

// HashPassword produces an argon2id verifier for a dashboard password, using
// the same parameters as API key hashing. Passwords are never stored in
// plaintext; only this hash is persisted.
func HashPassword(plaintext string) (string, error) {
	return HashAPIKey(plaintext)
}

// VerifyPassword reports whether plaintext matches the stored argon2id hash via
// a constant-time comparison.
func VerifyPassword(plaintext, encodedHash string) (bool, error) {
	return VerifyAPIKey(plaintext, encodedHash)
}
