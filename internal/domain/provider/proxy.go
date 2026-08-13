package provider

import "context"

type proxyKey struct{}

// WithProxy returns a context carrying proxy configuration from credentials.
// Call this in the pipeline before dispatching to a connector:
//
//	ctx = core.WithProxy(ctx, attempt.Creds)
func WithProxy(ctx context.Context, creds Credentials) context.Context {
	if creds.ProxyURL == "" && creds.RelayURL == "" {
		return ctx
	}
	return context.WithValue(ctx, proxyKey{}, creds)
}

// ProxyFromContext extracts proxy credentials from context, or returns false.
func ProxyFromContext(ctx context.Context) (Credentials, bool) {
	creds, ok := ctx.Value(proxyKey{}).(Credentials)
	return creds, ok
}
