// Package slimmer — truncate.go defines global truncation caps shared by every
// filter rule, matching the upstream RTK convention where different content
// categories get different caps based on actionability.
package slimmer

const (
	// CapErrors is the max number of error lines to show. Errors are the most
	// actionable category so they get the highest cap.
	CapErrors = 20

	// CapWarnings is the max number of warning / lint / test-failure lines.
	// Lower signal density than errors.
	CapWarnings = 10

	// CapList is the max for flat lists (PRs, services, packages, git status
	// entries). One line per item.
	CapList = 20

	// CapInventory is the max for exhaustive lookups (pip list, docker images,
	// npm ls). These are reference data, not actionable output.
	CapInventory = 50
)


// itoa converts a small int to string without importing strconv (avoids pulling
// a new dependency for a trivial conversion).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
