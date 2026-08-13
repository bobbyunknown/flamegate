package middleware

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCollapseDoubleV1(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"/v1/v1/messages", "/v1/messages"},
		{"/v1/v1/chat/completions", "/v1/chat/completions"},
		{"/v1/v1", "/v1"},
		{"/v1/messages", "/v1/messages"},
		{"/v1", "/v1"},
		{"/healthz", "/healthz"},
		{"/v1beta/models/x", "/v1beta/models/x"},
	}
	for _, c := range cases {
		var got string
		h := CollapseDoubleV1(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			got = r.URL.Path
		}))
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://x"+c.in, nil)
		require.NoError(t, err)
		h.ServeHTTP(nil, req)
		require.Equal(t, c.want, got, "input %s", c.in)
	}
}
