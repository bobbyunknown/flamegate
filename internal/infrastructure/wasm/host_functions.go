package wasm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tetratelabs/wazero/api"

	"github.com/bobbyunknown/flamegate/internal/application/ports"
	core "github.com/bobbyunknown/flamegate/internal/domain/provider"
	"github.com/bobbyunknown/flamegate/internal/shared/httputil"
	"github.com/bobbyunknown/flamegate/internal/shared/vault"
)

// HostFuncConfig holds dependencies injected into host function closures.
// Created per-request, captured by closures at InstantiateModule time.
type HostFuncConfig struct {
	Slug         string
	Logger       *logrus.Entry
	Vault        *vault.Vault
	AccountRepo  ports.AccountRepository
	AllowedHosts []string
	HTTPClient   *http.Client
	// Creds holds the pre-resolved credentials from the pipeline dispatch layer.
	// When set, get_credentials returns these directly — no AccountRepo/Vault lookup needed.
	Creds core.Credentials
}


// httpPostErrorResponse is returned on http_post failure.
type httpPostErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}


// credResponse is returned to guest from get_credentials.
type credResponse struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url,omitempty"`
}

// credResponseFrom maps resolved credentials to the guest-facing shape.
// OAuth accounts carry their token in AccessToken; the guest ABI only exposes
// api_key, so forward AccessToken there when no APIKey is set. Mirrors the
// native connectors (AccessToken preferred, APIKey fallback).
func credResponseFrom(creds core.Credentials) credResponse {
	token := creds.AccessToken
	if token == "" {
		token = creds.APIKey
	}
	return credResponse{APIKey: token, BaseURL: creds.BaseURL}
}

// hostHTTPPost implements the http_post host function.
// Guest calls: http_post(url_ptr, url_len, body_ptr, body_len, hdrs_ptr, hdrs_len) -> resp_ptr
// Returns a raw length-prefixed upstream body pointer in guest memory.
func hostHTTPPost(client *http.Client) func(context.Context, api.Module, uint32, uint32, uint32, uint32, uint32, uint32) uint32 {
	return hostHTTPRequest(client, http.MethodPost)
}

// hostHTTPGet implements the http_get host function (list_models / non-body GETs).
// Guest calls: http_get(url_ptr, url_len, hdrs_ptr, hdrs_len) -> resp_ptr
func hostHTTPGet(client *http.Client) func(context.Context, api.Module, uint32, uint32, uint32, uint32) uint32 {
	post := hostHTTPRequest(client, http.MethodGet)
	return func(ctx context.Context, mod api.Module, urlPtr, urlLen, hdrsPtr, hdrsLen uint32) uint32 {
		return post(ctx, mod, urlPtr, urlLen, 0, 0, hdrsPtr, hdrsLen)
	}
}

// hostHTTPRequest is the shared non-stream outbound HTTP path for guest host functions.
func hostHTTPRequest(client *http.Client, method string) func(context.Context, api.Module, uint32, uint32, uint32, uint32, uint32, uint32) uint32 {
	return func(ctx context.Context, mod api.Module, urlPtr, urlLen, bodyPtr, bodyLen, hdrsPtr, hdrsLen uint32) uint32 {
		// Read URL from guest memory.
		rawURL, err := readGuestBytes(mod, urlPtr, urlLen)
		if err != nil {
			return writeHostError(ctx, mod, "HOST_READ_ERROR", err.Error())
		}

		// Validate URL scheme.
		parsedURL, err := url.Parse(string(rawURL))
		if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
			return writeHostError(ctx, mod, "INVALID_URL", "url must be http or https")
		}

		// Read body from guest memory (POST only typically).
		var bodyBytes []byte
		if bodyLen > 0 {
			bodyBytes, err = readGuestBytes(mod, bodyPtr, bodyLen)
			if err != nil {
				return writeHostError(ctx, mod, "HOST_READ_ERROR", err.Error())
			}
		}

		// Read headers from guest memory.
		var guestHeaders map[string]string
		if hdrsLen > 0 {
			hdrBytes, readErr := readGuestBytes(mod, hdrsPtr, hdrsLen)
			if readErr != nil {
				return writeHostError(ctx, mod, "HOST_READ_ERROR", readErr.Error())
			}
			if err := json.Unmarshal(hdrBytes, &guestHeaders); err != nil {
				return writeHostError(ctx, mod, "INVALID_HEADERS", "headers must be valid JSON")
			}
		}

		// Build HTTP request.
		var bodyReader io.Reader
		if len(bodyBytes) > 0 {
			bodyReader = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, string(rawURL), bodyReader)
		if err != nil {
			return writeHostError(ctx, mod, "HOST_BUILD_ERROR", err.Error())
		}
		if method == http.MethodPost || len(bodyBytes) > 0 {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range guestHeaders {
			req.Header.Set(k, v)
		}

		// Execute.
		if client == nil {
			client = httputil.NewClient(60 * time.Second)
		}
		resp, err := client.Do(req)
		if err != nil {
			return writeHostError(ctx, mod, "HOST_HTTP_ERROR", err.Error())
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
		if err != nil {
			return writeHostError(ctx, mod, "HOST_READ_ERROR", err.Error())
		}

		// Return raw upstream body with length prefix — extension parses it directly.
		// Works for both JSON (non-stream) and SSE text (stream).
		ptr, writeErr := writeGuestRawLenPrefix(ctx, mod, respBody)
		if writeErr != nil {
			return writeHostError(ctx, mod, "HOST_MEMORY_ERROR", writeErr.Error())
		}
		return ptr
	}
}

// hostGetCredentials implements the get_credentials host function.
// Guest calls: get_credentials(key_ptr, key_len) -> cred_ptr
func hostGetCredentials(cfg *HostFuncConfig) func(context.Context, api.Module, uint32, uint32) uint32 {
	return func(ctx context.Context, mod api.Module, keyPtr, keyLen uint32) uint32 {
		// Fast path: pipeline already resolved + decrypted credentials.
		if cfg.Creds.APIKey != "" || cfg.Creds.AccessToken != "" {
			cred := credResponseFrom(cfg.Creds)
			ptr, _, writeErr := writeGuestJSON(ctx, mod, cred)
			if writeErr != nil {
				return writeHostError(ctx, mod, "HOST_MEMORY_ERROR", writeErr.Error())
			}
			cfg.Logger.WithFields(logrus.Fields{
				"account_id": cfg.Creds.AccountID,
				"slug":       cfg.Slug,
			}).Info("get_credentials: credentials provided to extension")
			return ptr
		}

		// Fallback: lookup from AccountRepo + Vault (used by admin sync, list_models).
		keyBytes, err := readGuestBytes(mod, keyPtr, keyLen)
		if err != nil {
			return writeHostError(ctx, mod, "HOST_READ_ERROR", err.Error())
		}
		accountID := strings.TrimSpace(string(keyBytes))

		if cfg.AccountRepo == nil {
			return writeHostError(ctx, mod, "HOST_CONFIG_ERROR", "account repo not configured")
		}

		acct, err := cfg.AccountRepo.Get(ctx, accountID)
		if err != nil {
			cfg.Logger.WithError(err).Warn("get_credentials: account lookup failed")
			return writeHostError(ctx, mod, "ACCOUNT_NOT_FOUND", fmt.Sprintf("account %q not found", accountID))
		}

		if cfg.Vault == nil {
			return writeHostError(ctx, mod, "HOST_CONFIG_ERROR", "vault not configured")
		}

		vaultCreds, err := cfg.Vault.Open(acct)
		if err != nil {
			cfg.Logger.WithError(err).Warn("get_credentials: vault open failed")
			return writeHostError(ctx, mod, "DECRYPT_FAILED", "failed to decrypt credentials")
		}

		cred := credResponseFrom(vaultCreds)

		ptr, _, writeErr := writeGuestJSON(ctx, mod, cred)
		if writeErr != nil {
			return writeHostError(ctx, mod, "HOST_MEMORY_ERROR", writeErr.Error())
		}

		cfg.Logger.WithFields(logrus.Fields{
			"account_id": accountID,
			"slug":       cfg.Slug,
		}).Info("get_credentials: credentials provided to extension (fallback)")

		return ptr
	}
}

// hostEmitChunk implements the emit_chunk host function.
// Guest calls: emit_chunk(chunk_ptr, chunk_len)
func hostEmitChunk(ch chan<- interface{}) func(context.Context, api.Module, uint32, uint32) {
	return func(ctx context.Context, mod api.Module, chunkPtr, chunkLen uint32) {
		if ch == nil {
			return // no stream channel (non-stream request)
		}

		chunkBytes, err := readGuestBytes(mod, chunkPtr, chunkLen)
		if err != nil {
			return
		}
		// Copy out of guest linear memory before returning control to the guest.
		payload := append([]byte(nil), chunkBytes...)

		select {
		case ch <- payload:
		case <-ctx.Done():
		}
	}
}

// hostLog implements the fg_log host function.
// Guest calls: fg_log(level_ptr, level_len, msg_ptr, msg_len)
func hostLog(slug string) func(context.Context, api.Module, uint32, uint32, uint32, uint32) {
	return func(ctx context.Context, mod api.Module, levelPtr, levelLen, msgPtr, msgLen uint32) {
		levelBytes, err := readGuestBytes(mod, levelPtr, levelLen)
		if err != nil {
			return
		}
		msgBytes, err := readGuestBytes(mod, msgPtr, msgLen)
		if err != nil {
			return
		}

		level := strings.ToLower(strings.TrimSpace(string(levelBytes)))
		msg := string(msgBytes)

		log := logrus.WithField("slug", slug)
		switch level {
		case "debug":
			log.Debug(msg)
		case "info":
			log.Info(msg)
		case "warn", "warning":
			log.Warn(msg)
		case "error":
			log.Error(msg)
		default:
			log.Info(msg)
		}
	}
}

// writeHostError writes a JSON error response to guest memory and returns the pointer.
func writeHostError(ctx context.Context, mod api.Module, code, msg string) uint32 {
	errResp := httpPostErrorResponse{Error: msg, Code: code}
	ptr, _, err := writeGuestJSON(ctx, mod, errResp)
	if err != nil {
		return 0 // last resort: null pointer
	}
	return ptr
}
