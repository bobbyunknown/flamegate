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
	ansiDim    = "\033[2m"
	ansiOrange = "\033[38;5;208m"
	ansiCyan   = "\033[36m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
)

type BannerConfig struct {
	Version   string
	Commit    string
	Mode      string
	AdminAddr string
	LLMAddr   string
	DBDriver  string
	Cache     string
	DataDir   string
	LogLevel  string
	LogDir    string
}

func PrintBanner(w io.Writer, cfg BannerConfig) {
	if f, ok := w.(interface{ Fd() uintptr }); ok {
		if !IsTerminal(f.Fd()) {
			return
		}
	}

	modeBadge := "production"
	modeColor := ansiGreen
	if cfg.Mode == "dev" || cfg.Mode == "development" {
		modeBadge = "development"
		modeColor = ansiYellow
	}

	commitStr := ""
	if cfg.Commit != "" && cfg.Commit != "unknown" {
		commitStr = " " + ansiDim + "(" + cfg.Commit + ")" + ansiReset
	}

	divider := ansiDim + strings.Repeat("─", 54) + ansiReset

	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s%sFLAMEGATE%s %sv%s%s%s  %s[%s]%s\n",
		ansiBold, ansiOrange, ansiReset,
		ansiBold, cfg.Version, ansiReset,
		commitStr,
		modeColor, modeBadge, ansiReset)
	fmt.Fprintln(w, "  "+divider)

	rows := []struct {
		label, value string
	}{
		{"Admin HTTP", cfg.AdminAddr},
		{"LLM Proxy", cfg.LLMAddr},
		{"Database", cfg.DBDriver},
		{"Cache", cfg.Cache},
		{"Data Dir", cfg.DataDir},
		{"Log Level", cfg.LogLevel},
	}
	if cfg.LogDir != "" {
		rows = append(rows, struct{ label, value string }{"Logs Dir", cfg.LogDir + " (http.log, llm.log)"})
	}

	for _, r := range rows {
		fmt.Fprintf(w, "  %s%-12s%s %s%s%s\n",
			ansiDim, r.label, ansiReset,
			ansiCyan, r.value, ansiReset)
	}

	fmt.Fprintln(w, "  "+divider)
	fmt.Fprintln(w)
}

func PrintBannerStdout(cfg BannerConfig) {
	PrintBanner(os.Stdout, cfg)
}
