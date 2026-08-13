package proxy

import (
	"fmt"
	"strconv"
	"strings"
)

// This file holds small presentation helpers used to render the human-readable
// console log shown on the dashboard. They format raw numbers (bytes, durations,
// token counts) into something a person can scan at a glance.

// humanBytes formats a byte count using binary units (B, KB, MB, ...).
func humanBytes(n int) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := int64(n) / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// humanDuration formats a millisecond duration as a compact, readable string.
func humanDuration(ms int) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	secs := float64(ms) / 1000
	if secs < 60 {
		return fmt.Sprintf("%.1fs", secs)
	}
	mins := int(secs) / 60
	rem := secs - float64(mins*60)
	return fmt.Sprintf("%dm %.0fs", mins, rem)
}

// humanInt formats an integer with thousands separators (e.g. 45119 -> 45,119).
func humanInt(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// plural returns "s" when n != 1, for simple pluralization.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
