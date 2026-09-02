package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sirupsen/logrus"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence"
	"github.com/bobbyunknown/flamegate/internal/shared/crypto"
	"github.com/bobbyunknown/flamegate/internal/shared/vault"
)

// newBulkTestServer builds a minimal Server wired with just the store and vault
// needed by adminBulkCreateAccounts. Validation is exercised with validate=false
// so no upstream connector/refresher is required.
func newBulkTestServer(t *testing.T) (*Handler, *persistence.DB) {
	t.Helper()
	db, err := persistence.OpenDB("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Migrate())
	require.NoError(t, db.EnsureDefault())
	t.Cleanup(func() { _ = db.Close() })

	mk, err := crypto.GenerateMasterKey()
	require.NoError(t, err)
	sealer, err := crypto.NewSealer(mk)
	require.NoError(t, err)

	s := &Handler{
		accounts: db.Accounts(),
		vault:    vault.New(sealer),
		log:      logrus.StandardLogger(),
	}
	return s, db
}

func bulkResponse(t *testing.T, s *Handler, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/accounts/bulk", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.adminBulkCreateAccounts(rec, req)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	return rec.Code, out
}

func TestBulkCreateAccounts_PartialOutcomes(t *testing.T) {
	s, db := newBulkTestServer(t)

	// sk-1 appears twice (the second is a duplicate), one item has an empty key.
	body := `{
		"provider": "custom-openai",
		"base_url": "https://api.openai.com/v1",
		"validate": false,
		"items": [
			{"label": "alpha", "api_key": "sk-1"},
			{"api_key": "sk-2"},
			{"label": "dup", "api_key": "sk-1"},
			{"label": "empty", "api_key": ""}
		]
	}`
	code, out := bulkResponse(t, s, body)
	require.Equal(t, http.StatusOK, code)
	require.EqualValues(t, 4, out["total"])
	require.EqualValues(t, 2, out["created"])
	require.EqualValues(t, 1, out["skipped"])
	require.EqualValues(t, 1, out["failed"])

	// Two accounts persisted.
	accs, err := db.Accounts().ListByTenant(context.Background(), adminTenant)
	require.NoError(t, err)
	require.Len(t, accs, 2)

	// Results preserve input order.
	results, ok := out["results"].([]any)
	require.True(t, ok)
	require.Len(t, results, 4)
	statuses := make([]string, len(results))
	for i, r := range results {
		statuses[i] = r.(map[string]any)["status"].(string)
	}
	require.Equal(t, []string{"created", "created", "skipped", "error"}, statuses)
}

func TestBulkCreateAccounts_Validation(t *testing.T) {
	s, _ := newBulkTestServer(t)

	// Unknown provider.
	code, _ := bulkResponse(t, s, `{"provider":"does-not-exist","items":[{"api_key":"x"}]}`)
	require.Equal(t, http.StatusBadRequest, code)

	// Missing provider.
	code, _ = bulkResponse(t, s, `{"items":[{"api_key":"x"}]}`)
	require.Equal(t, http.StatusBadRequest, code)

	// Empty items.
	code, _ = bulkResponse(t, s, `{"provider":"openai","items":[]}`)
	require.Equal(t, http.StatusBadRequest, code)
}
