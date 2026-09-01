// Command flamegate is the FlameGate server entrypoint. It loads configuration,
// builds the application, and serves until interrupted.
//
// Usage:
//
//	flamegate [flags]
//	flamegate [command]
//
// Commands:
//
//	status       check whether a local server is running and print its URL
//	bootstrap    create an initial API key and exit
//	ext          manage WASM extensions (install, list, enable, disable)
//	version      print version and exit
//	help         show usage
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/bobbyunknown/flamegate/internal/app"
	"github.com/bobbyunknown/flamegate/internal/config"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/http/middleware"
	"github.com/bobbyunknown/flamegate/internal/shared/prettylog"
	"github.com/bobbyunknown/flamegate/internal/shared/version"
)

// Version metadata set at build time via ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = ""
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if !errors.Is(err, errSilent) {
			fmt.Fprintln(os.Stderr, "flamegate:", err)
		}
		os.Exit(1)
	}
}

// errSilent signals a non-zero exit without printing the default error prefix.
// Used by commands that already printed a user-friendly message.
var errSilent = errors.New("silent")

// run dispatches to a subcommand when the first argument is a known verb.
// Otherwise it directly parses flags and serves the gateway server.
func run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "help", "-h", "--help":
			printUsage(os.Stdout)
			return nil
		case "version", "-v", "--version", "-version":
			fmt.Printf("FlameGate v%s (%s)\n", version.Resolve(Version), Commit)
			return nil
		case "status":
			return cmdStatus(args[1:])
		case "bootstrap":
			return cmdBootstrap(args[1:])
		case "ext":
			return cmdExt(args[1:])
		case "start":
			// Allow `flamegate start` as optional synonym for `flamegate`
			return runServer(args[1:])
		}
	}

	return runServer(args)
}

func printUsage(w *os.File) {
	_, _ = fmt.Fprint(w, `FlameGate — self-hostable AI proxy & extension runtime

Usage:
  flamegate [flags]
  flamegate [command]

Commands:
  status       Check whether a local server is running and print its URL
  bootstrap    Create an initial API key and print it once
  ext          Manage WASM extensions (install, list, enable, disable)
  version      Print version and exit
  help         Show this help

Flags:
  -c, --config <path>  Path to a TOML config file (default: ~/.flamegate/flamegate.toml)
  -k, --key-name <n>   (bootstrap) Name for the created API key (default: default)
  -bootstrap           (flag) Create an initial API key and exit
  -healthcheck         (flag) Check local HTTP health endpoint and exit

Examples:
  flamegate                       # start server with default config (~/.flamegate/flamegate.toml)
  flamegate -c flamegate.toml     # start server with custom TOML config
  flamegate bootstrap             # mint your first API key
  flamegate bootstrap -k prod     # mint a named API key
  flamegate status                # check if server is running
`)
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

func runServer(args []string) error {
	fs := flag.NewFlagSet("flamegate", flag.ContinueOnError)
	configPath := resolveConfigFlag(fs)
	bootstrap := fs.Bool("bootstrap", false, "create an initial API key and exit")
	healthcheck := fs.Bool("healthcheck", false, "check the local HTTP health endpoint and exit")
	showVersion := fs.Bool("version", false, "print version and exit")
	keyName := keyNameFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *showVersion {
		fmt.Printf("FlameGate v%s (%s)\n", version.Resolve(Version), Commit)
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

	return serve(cfg, log, isPretty)
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
	url := fmt.Sprintf("http://%s:%d", loopbackHost(cfg), cfg.Server.Port)
	if err := runHealthcheck(cfg); err != nil {
		fmt.Printf("FlameGate is not running (no response at %s)\n", url)
		return errSilent
	}
	fmt.Printf("FlameGate is running → %s\n", url)
	return nil
}

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
	if cfg.Log.Dir != "" {
		if err := middleware.InitFileLogs(cfg.Log.Dir); err != nil {
			log.Warnf("failed to initialize log dir %s: %v", cfg.Log.Dir, err)
		}
	}
	return cfg, log, isPretty, nil
}

// serve builds the application and runs it until interrupted.
func serve(cfg config.Config, log *logrus.Logger, isPretty bool) error {
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
		mode := "production"
		if version.IsDev(Version) {
			mode = "development"
		}

		adminAddr := fmt.Sprintf("http://%s:%d", loopbackHost(cfg), cfg.Server.Port)
		llmAddr := adminAddr + "/v1"
		if cfg.ProxyAddr() != "" {
			llmAddr = fmt.Sprintf("http://%s:%d/v1", loopbackHost(cfg), cfg.Server.ProxyPort)
		}

		prettylog.PrintBannerStdout(prettylog.BannerConfig{
			Version:   resolvedVersion,
			Commit:    Commit,
			Mode:      mode,
			AdminAddr: adminAddr,
			LLMAddr:   llmAddr,
			DBDriver:  cfg.Database.Driver,
			Cache:     cacheLabel,
			DataDir:   dataDir,
			LogLevel:  cfg.Log.Level + " (pretty)",
			LogDir:    cfg.Log.Dir,
		})
	}

	// Cancel on SIGINT/SIGTERM for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if version.IsDev(Version) {
		cfg.Docs.Enabled = true
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
	url := fmt.Sprintf("http://%s:%d/healthz", loopbackHost(cfg), cfg.Server.Port)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("healthcheck %s: %w", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("healthcheck %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("healthcheck %s: %s", url, resp.Status)
	}
	return nil
}

func loopbackHost(cfg config.Config) string {
	host := cfg.Server.Host
	if host == "" || host == "0.0.0.0" || host == "::" {
		return "127.0.0.1"
	}
	return host
}

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
