package crypto

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func newTestSealer(t *testing.T) *Sealer {
	t.Helper()
	key, err := GenerateMasterKey()
	require.NoError(t, err)
	s, err := NewSealer(key)
	require.NoError(t, err)
	return s
}

func TestSealOpen_RoundTrip(t *testing.T) {
	s := newTestSealer(t)
	secrets := [][]byte{
		[]byte("sk-proj-abc123"),
		[]byte(""),
		bytes.Repeat([]byte("x"), 4096),
		[]byte("unicode: 日本語 🔐"),
	}
	for _, secret := range secrets {
		sealed, err := s.Seal(secret)
		require.NoError(t, err)
		require.NotEmpty(t, sealed.WrappedDEK)
		require.NotEmpty(t, sealed.Ciphertext)

		got, err := s.Open(sealed)
		require.NoError(t, err)
		// Compare as strings so a round-tripped empty input (nil vs []byte{})
		// is treated as equal; only content fidelity matters.
		require.Equal(t, string(secret), string(got))
	}
}

func TestSeal_UniqueCiphertextPerCall(t *testing.T) {
	s := newTestSealer(t)
	a, err := s.SealString("same-secret")
	require.NoError(t, err)
	b, err := s.SealString("same-secret")
	require.NoError(t, err)
	// Fresh DEK + nonce each time => ciphertext and wrapped DEK must differ.
	require.NotEqual(t, a.Ciphertext, b.Ciphertext)
	require.NotEqual(t, a.WrappedDEK, b.WrappedDEK)
}

func TestOpen_WrongMasterKeyFails(t *testing.T) {
	s1 := newTestSealer(t)
	s2 := newTestSealer(t)
	sealed, err := s1.SealString("top-secret")
	require.NoError(t, err)

	_, err = s2.Open(sealed)
	require.Error(t, err, "opening with a different master key must fail")
}

func TestOpen_TamperedCiphertextFails(t *testing.T) {
	s := newTestSealer(t)
	sealed, err := s.SealString("integrity")
	require.NoError(t, err)
	sealed.Ciphertext = "not-base64-$$$"
	_, err = s.Open(sealed)
	require.ErrorIs(t, err, ErrMalformedCiphertext)
}

func TestNewSealer_RejectsBadKeySize(t *testing.T) {
	_, err := NewSealer([]byte("too-short"))
	require.ErrorIs(t, err, ErrInvalidKeySize)
}

func TestMasterKeyEncodeDecode(t *testing.T) {
	key, err := GenerateMasterKey()
	require.NoError(t, err)
	encoded := EncodeMasterKey(key)
	decoded, err := DecodeMasterKey(encoded)
	require.NoError(t, err)
	require.Equal(t, key, decoded)

	_, err = DecodeMasterKey("c2hvcnQ=") // "short" -> wrong length
	require.ErrorIs(t, err, ErrInvalidKeySize)
}
