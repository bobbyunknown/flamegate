package tunnel

import (
	"context"
	"net"
	"time"
)

// publicDNSServers are the DNS servers tried first to bypass OS negative cache
// (e.g. mDNSResponder holds NXDOMAIN on macOS).
var publicDNSServers = []string{"1.1.1.1:53", "1.0.0.1:53", "8.8.8.8:53"}

// ResolveDNS checks whether hostname resolves via public DNS servers first,
// falling back to the OS resolver. Returns true if resolution succeeds within
// the timeout.
func ResolveDNS(hostname string, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Try public DNS servers first.
	for _, server := range publicDNSServers {
		dialer := &net.Dialer{Timeout: 2 * time.Second}
		r := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				return dialer.DialContext(ctx, "udp", server)
			},
		}
		addrs, err := r.LookupHost(ctx, hostname)
		if err == nil && len(addrs) > 0 {
			return true
		}
	}

	// Fall back to OS resolver.
	r := &net.Resolver{}
	addrs, err := r.LookupHost(ctx, hostname)
	return err == nil && len(addrs) > 0
}
