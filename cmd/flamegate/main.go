// Command flamegate is the FlameGate server entrypoint. It loads configuration,
// builds the application, and serves until interrupted.
//
// Usage:
//
//	flamegate [command] [flags]
//
// Commands:
//
//	start        start the server (default when no command is given)
//	status       check whether a local server is running and print its URL
//	bootstrap    create an initial API key and exit
//	version      print version and exit
//	help         show usage
//
// For backward compatibility the legacy flag form is still accepted, so
// `flamegate`, `flamegate -bootstrap`, `flamegate -healthcheck`, and
// `flamegate -version` keep working exactly as before.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/bobbyunknown/flamegate/internal/app"
	"github.com/bobbyunknown/flamegate/internal/config"
	"github.com/bobbyunknown/flamegate/internal/shared/prettylog"
	"github.com/bobbyunknown/flamegate/internal/shared/version"
)

// Version is set at build time via -ldflags "-X main.Version=...".
// Defaults to "dev" for local builds (air, go run). Release builds (make build,
// CI) inject a real tag like "v0.1.25".
var Version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		if !errors.Is(err, errSilent) {
			//nolint:errcheck // best-effort stderr output
			fmt.Fprintln(os.Stderr, "flamegate:", err)
		}
		os.Exit(1)
	}
}

// errSilent signals a non-zero exit without printing the default error prefix.
// Used by commands that already printed a user-friendly message.
var errSilent = errors.New("silent")

// run dispatches to a subcommand when the first argument is a known verb.
// Otherwise it falls back to the legacy flag form, which also serves the
// server — so `flamegate`, `flamegate -bootstrap`, etc. keep working.
func run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "help", "-h", "--help":
			printUsage(os.Stdout)
			return nil
		}
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd := args[0]
		rest := args[1:]
		switch cmd {
		case "start":
			return cmdStart(rest)
		case "status":
			return cmdStatus(rest)
		case "bootstrap":
			return cmdBootstrap(rest)
		case "ext":
			return cmdExt(rest)
		case "version":
			fmt.Println("0", version.Resolve(Version))
			return nil
		default:
			printUsage(os.Stderr)
			return fmt.Errorf("unknown command %q", cmd)
		}
	}
	// Legacy flag form (no subcommand). Equivalent to `start`, but also honors
	// -bootstrap, -healthcheck, and -version for backward compatibility.
	return runLegacy(args)
}

func printUsage(w *os.File) {
	//nolint:errcheck // best-effort write
	_, _ = fmt.Fprint(w, `FlameGate — self-hostable AI gateway

Usage:
  flamegate [command] [flags]

Commands:
  start        Start the server (default when no command is given)
  status       Check whether a local server is running and print its URL
  bootstrap    Create an initial API key and print it once
  ext          Manage WASM extensions (install, list, enable, disable)
  version      Print version and exit
  help         Show this help

Common flags:
  -c, --config <path>  Path to a TOML config file (optional, default: ~/.flamegate/flamegate.toml)
  --no-browser       (start) Do not open the dashboard in a browser
  -k, --key-name <n> (bootstrap) Name for the created API key

Examples:
  flamegate start -c config.yaml  # start with custom config
  flamegate start --no-browser    # start without opening a browser
  flamegate bootstrap             # mint your first API key
  flamegate bootstrap -k prod     # mint a named API key
  flamegate status                # is it running?
`) //nolint:errcheck // best-effort
}

// resolveConfigFlag registers -c and --config on fs, returns -c pointer.
func resolveConfigFlag(fs *flag.FlagSet) *string {
	c := fs.String("c", "", "")
	fs.String("config", "", "")
	return c
}

// configVal returns -c if set, else --config value.
func configVal(fs *flag.FlagSet, c *string) string {
	if *c != "" {
		return *c
	}
	if f := fs.Lookup("config"); f != nil {
		return f.Value.String()
	}
	return ""
}

// cmdStart parses start-specific flags and serves.
func cmdStart(args []string) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	configPath := resolveConfigFlag(fs)
	noBrowser := fs.Bool("no-browser", false, "do not open the dashboard in a browser")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, log, isPretty, err := setup(configVal(fs, configPath))
	if err != nil {
		return err
	}
	// Open the dashboard by default, but only on an interactive terminal so
	// headless/Docker runs stay quiet. --no-browser always wins.
	openBrowser := isPretty && !*noBrowser
	return serve(cfg, log, isPretty, openBrowser)
}

// keyNameFlag registers -k and --key-name on fs, returns -k pointer.
func keyNameFlag(fs *flag.FlagSet) *string {
	k := fs.String("k", "default", "")
	fs.String("key-name", "default", "")
	return k
}

// keyNameVal returns -k if explicitly set, else --key-name value.
func keyNameVal(fs *flag.FlagSet, k *string) string {
	if *k != "default" {
		return *k
	}
	if f := fs.Lookup("key-name"); f != nil && f.Value.String() != "default" {
		return f.Value.String()
	}
	return *k
}

func cmdBootstrap(args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	configPath := resolveConfigFlag(fs)
	keyName := keyNameFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, _, _, err := setup(configVal(fs, configPath))
	if err != nil {
		return err
	}
	return doBootstrap(cfg, keyNameVal(fs, keyName))
}
// cmdStatus reports whether a local server is reachable.
func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	configPath := resolveConfigFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(configVal(fs, configPath))
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	url := dashboardURL(cfg)
	if err := runHealthcheck(cfg); err != nil {
		fmt.Printf("FlameGate is not running (no response at %s)\n", url)
		return errSilent
	}
	fmt.Printf("FlameGate is running → %s\n", url)
	return nil
}

// runLegacy preserves the original flag-based entrypoint so existing usage
// (`flamegate`, `flamegate -bootstrap`, `flamegate -healthcheck`,
// `flamegate -version`) keeps working unchanged.
func runLegacy(args []string) error {
	fs := flag.NewFlagSet("0", flag.ContinueOnError)
	showVersion := fs.Bool("version", false, "print version and exit")
	configPath := resolveConfigFlag(fs)
	bootstrap := fs.Bool("bootstrap", false, "create an initial API key and exit")
	healthcheck := fs.Bool("healthcheck", false, "check the local HTTP health endpoint and exit")
	keyName := keyNameFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *showVersion {
		fmt.Println("0", version.Resolve(Version))
		return nil
	}

	cfg, log, isPretty, err := setup(configVal(fs, configPath))
	if err != nil {
		return err
	}

	if *healthcheck {
		return runHealthcheck(cfg)
	}
	if *bootstrap {
		return doBootstrap(cfg, keyNameVal(fs, keyName))
	}
	// Legacy plain `flamegate` keeps the original behavior: serve without
	// auto-opening a browser.
	return serve(cfg, log, isPretty, false)
}

// setup loads configuration and initializes the default logger. In dev mode
// (Version == "dev") the log level defaults to debug unless the user
// explicitly sets FLAMEGATE_LOG__LEVEL.
// resolveConfigPath returns configPath if non-empty, otherwise checks for
// a default config at ~/.flamegate/flamegate.toml.
func resolveConfigPath(configPath string) string {
	if configPath != "" {
		return configPath
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	candidate := filepath.Join(home, ".flamegate", "flamegate.toml")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

func setup(configPath string) (config.Config, *logrus.Logger, bool, error) {
	cfg, err := config.Load(resolveConfigPath(configPath))
	if err != nil {
		return config.Config{}, nil, false, fmt.Errorf("load config: %w", err)
	}

	// Dev mode: default to debug unless the user explicitly set a log level.
	if version.IsDev(Version) {
		if _, set := os.LookupEnv("FLAMEGATE_LOG__LEVEL"); !set {
			cfg.Log.Level = "debug"
		}
	}

	log, isPretty := newLogger(cfg.Log)
	return cfg, log, isPretty, nil
}

// serve builds the application and runs it until interrupted. When openBrowser
// is true it launches the dashboard once the server reports healthy.
func serve(cfg config.Config, log *logrus.Logger, isPretty, openBrowser bool) error {
	resolvedVersion := version.Resolve(Version)

	// Print the startup banner in pretty mode (TTY only).
	if isPretty {
		cacheLabel := "disabled"
		if cfg.Cache.Enabled {
			cacheLabel = cfg.Cache.Backend
		}
		dataDir := cfg.Data.Dir
		if dataDir == "" {
			dataDir = "~/.flamegate"
		}
		bannerVersion := resolvedVersion
		if version.IsDev(Version) {
			bannerVersion = resolvedVersion + " (dev)"
		}
		prettylog.PrintBannerStdout(prettylog.BannerConfig{
			Version:  bannerVersion,
			Addr:     cfg.Addr(),
			DBDriver: cfg.Database.Driver,
			Cache:    cacheLabel,
			DataDir:  dataDir,
			LogLevel: cfg.Log.Level + " (pretty)",
		})
	}

	// Cancel on SIGINT/SIGTERM for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if openBrowser {
		go openDashboardWhenReady(ctx, cfg)
	}

	if version.IsDev(Version) {
		cfg.Docs.Enabled =true
		log.Info("dev build: API docs auto-enabled at /docs")
	}

	application, err := app.Build(ctx, cfg, log, resolvedVersion)
	if err != nil {
		return fmt.Errorf("build app: %w", err)
	}

	return application.Run(ctx)
}

// doBootstrap creates an initial API key and prints it once.
func doBootstrap(cfg config.Config, keyName string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	plaintext, err := app.Bootstrap(ctx, cfg, keyName)
	if err != nil {
		return err
	}
	fmt.Println("Created API key (copy it now, it will not be shown again):")
	fmt.Println(plaintext)
	return nil
}

func runHealthcheck(cfg config.Config) error {
	url := healthURL(cfg)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("healthcheck %s: %w", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("healthcheck %s: %w", url, err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("healthcheck %s: %s", url, resp.Status)
	}
	return nil
}

// loopbackHost normalizes a wildcard/empty bind address to a reachable
// loopback host for local client connections.
func loopbackHost(cfg config.Config) string {
	host := cfg.Server.Host
	if host == "" || host == "0.0.0.0" || host == "::" {
		return "127.0.0.1"
	}
	return host
}

func healthURL(cfg config.Config) string {
	return fmt.Sprintf("http://%s:%d/healthz", loopbackHost(cfg), cfg.Server.Port)
}

// dashboardURL is the friendly URL to show users and open in a browser.
func dashboardURL(cfg config.Config) string {
	host := loopbackHost(cfg)
	if host == "127.0.0.1" {
		host = "localhost"
	}
	return fmt.Sprintf("http://%s:%d", host, cfg.Server.Port)
}

// openDashboardWhenReady polls the health endpoint until the server is up (or
// ctx is canceled), then opens the dashboard in the default browser. Failures
// are best-effort: the user can always open the URL printed in the banner.
func openDashboardWhenReady(ctx context.Context, cfg config.Config) {
	client := &http.Client{Timeout: 1 * time.Second}
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline:
			return
		case <-ticker.C:
			resp, err := client.Get(healthURL(cfg))
			if err != nil {
				continue
			}
			resp.Body.Close() //nolint:errcheck // best-effort close
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				openBrowser(dashboardURL(cfg))
				return
			}
		}
	}
}

// openBrowser opens url in the system default browser. Best-effort; errors are
// ignored because the URL is also printed to the console.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

// newLogger builds a structured logger per config. Returns the logger and
// whether pretty (colorized) output is active.
func newLogger(cfg config.LogConfig) (*logrus.Logger, bool) {
	var level logrus.Level
	switch cfg.Level {
	case "debug":
		level = logrus.DebugLevel
	case "warn":
		level = logrus.WarnLevel
	case "error":
		level = logrus.ErrorLevel
	default:
		level = logrus.InfoLevel
	}

	logger := logrus.New()
	logger.SetLevel(level)
	pretty := false
	if cfg.Format == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{})
	} else {
		pretty = prettylog.IsTerminal(os.Stdout.Fd())
		logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true, ForceColors: pretty})
	}
	return logger, pretty
}
