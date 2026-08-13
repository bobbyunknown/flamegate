package prettylog

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiOrange = "\033[38;5;208m"
)

type BannerConfig struct {
	Version  string
	Addr     string
	DBDriver string
	Cache    string
	DataDir  string
	LogLevel string
}

const (
	logoW  = 53
	innerW = logoW - 2
)

const logo = "█████ █      ███  █   █ █████  ███   ███  █████ █████\n" +
	"█     █     █   █ ██ ██ █     █     █   █   █   █\n" +
	"████  █     █████ █ █ █ ████  █  ██ █████   █   ████\n" +
	"█     █     █   █ █   █ █     █   █ █   █   █   █\n" +
	"█     █████ █   █ █   █ █████  ███  █   █   █   █████"

func PrintBanner(w io.Writer, cfg BannerConfig) {
	if f, ok := w.(interface{ Fd() uintptr }); ok {
		if !IsTerminal(f.Fd()) {
			return
		}
	}

	rows := []struct {
		label, value string
	}{
		{"HTTP", "http://" + cfg.Addr},
		{"DB", cfg.DBDriver},
		{"Cache", cfg.Cache},
		{"Data", cfg.DataDir},
		{"Log", cfg.LogLevel},
	}

	hLine := strings.Repeat("─", innerW)

	fmt.Fprintln(w) //nolint:errcheck // best-effort write
	for _, line := range strings.Split(logo, "\n") {
		fmt.Fprintf(w, "  %s%s%s\n", ansiOrange, line, ansiReset) //nolint:errcheck // best-effort write
	}
	fmt.Fprintln(w) //nolint:errcheck // best-effort write

	fmt.Fprintf(w, "  %s╭%s╮%s\n", ansiOrange, hLine, ansiReset) //nolint:errcheck // best-effort write
	boxRow(w, innerW, "FlameGate "+cfg.Version, false)
	boxRow(w, innerW, "", false)
	for _, r := range rows {
		boxRow(w, innerW, r.label+"  "+r.value, true)
	}
	fmt.Fprintf(w, "  %s╰%s╯%s\n", ansiOrange, hLine, ansiReset) //nolint:errcheck // best-effort write
	fmt.Fprintln(w) //nolint:errcheck // best-effort write
}

func boxRow(w io.Writer, innerW int, content string, bold bool) {
	pad := innerW - 1 - len(content)
	if pad < 0 {
		pad = 0
	}
	if bold {
		_, _ = fmt.Fprintf(w, "  %s│ %s%s%s%s%s│%s\n",
			ansiOrange, ansiBold, content, ansiReset, ansiOrange, strings.Repeat(" ", pad), ansiReset) //nolint:errcheck // best-effort write
	} else {
		_, _ = fmt.Fprintf(w, "  %s│ %-*s│%s\n", ansiOrange, innerW-1, content, ansiReset) //nolint:errcheck // best-effort write
	}
}

func PrintBannerStdout(cfg BannerConfig) {
	PrintBanner(os.Stdout, cfg)
}
