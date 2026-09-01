package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kardianos/service"

	"github.com/bobbyunknown/flamegate/internal/app"
	"github.com/bobbyunknown/flamegate/internal/shared/version"
)

type serviceProgram struct {
	configPath string
	cancel     context.CancelFunc
	exitChan   chan struct{}
}

func (p *serviceProgram) Start(s service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.exitChan = make(chan struct{})

	go p.run(ctx)
	return nil
}

func (p *serviceProgram) run(ctx context.Context) {
	defer close(p.exitChan)

	cfg, log, isPretty, err := setup(p.configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "service setup failed: %v\n", err)
		return
	}

	resolvedVersion := version.Resolve(Version)
	application, err := app.Build(ctx, cfg, log, resolvedVersion)
	if err != nil {
		log.Errorf("service build failed: %v", err)
		return
	}

	if err := application.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Errorf("service run exited with error: %v", err)
	}
	_ = isPretty
}

func (p *serviceProgram) Stop(s service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	if p.exitChan != nil {
		<-p.exitChan
	}
	return nil
}

func newServiceConfig(configPath string) *service.Config {
	execPath, err := os.Executable()
	if err != nil {
		execPath = "flamegate"
	}
	// Resolve symlink to absolute target executable
	if resolved, err := filepath.EvalSymlinks(execPath); err == nil {
		execPath = resolved
	}

	args := []string{}
	if configPath != "" {
		absConfig, err := filepath.Abs(configPath)
		if err == nil {
			args = append(args, "-c", absConfig)
		} else {
			args = append(args, "-c", configPath)
		}
	}

	return &service.Config{
		Name:        "flamegate",
		DisplayName: "FlameGate AI Gateway",
		Description: "Self-hostable LLM proxy, router, and extension runtime",
		Executable:  execPath,
		Arguments:   args,
		Option: service.KeyValue{
			"KeepAlive":   true,
			"RunAtLoad":   true,
			"UserService": true, // User-level service (LaunchAgent on macOS, systemd user on Linux)
		},
	}
}

// cmdService handles `flamegate service [install|uninstall|start|stop|restart|status]`.
func cmdService(args []string) error {
	fs := flag.NewFlagSet("service", flag.ContinueOnError)
	configPath := resolveConfigFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		printServiceUsage(os.Stdout)
		return nil
	}

	action := remaining[0]
	cfgPath := configVal(fs, configPath)

	prg := &serviceProgram{configPath: cfgPath}
	svcConfig := newServiceConfig(cfgPath)

	s, err := service.New(prg, svcConfig)
	if err != nil {
		return fmt.Errorf("initialize service: %w", err)
	}

	switch action {
	case "install":
		if err := s.Install(); err != nil {
			return fmt.Errorf("install service: %w", err)
		}
		fmt.Println("✓ FlameGate background service installed successfully.")
		fmt.Println("  Run 'flamegate service start' to start the service.")
		return nil

	case "uninstall":
		_ = s.Stop()
		if err := s.Uninstall(); err != nil {
			return fmt.Errorf("uninstall service: %w", err)
		}
		fmt.Println("✓ FlameGate background service uninstalled successfully.")
		return nil

	case "start":
		if err := s.Start(); err != nil {
			return fmt.Errorf("start service: %w", err)
		}
		fmt.Println("✓ FlameGate background service started.")
		return nil

	case "stop":
		if err := s.Stop(); err != nil {
			return fmt.Errorf("stop service: %w", err)
		}
		fmt.Println("✓ FlameGate background service stopped.")
		return nil

	case "restart":
		if err := s.Restart(); err != nil {
			return fmt.Errorf("restart service: %w", err)
		}
		fmt.Println("✓ FlameGate background service restarted.")
		return nil

	case "status":
		status, err := s.Status()
		if err != nil {
			fmt.Printf("Service status: unknown or not installed (%v)\n", err)
			return nil
		}
		switch status {
		case service.StatusRunning:
			fmt.Println("Service status: RUNNING")
		case service.StatusStopped:
			fmt.Println("Service status: STOPPED")
		default:
			fmt.Println("Service status: UNKNOWN")
		}
		return nil

	case "run":
		// Called by service manager when running as daemon
		return s.Run()

	default:
		printServiceUsage(os.Stdout)
		return fmt.Errorf("unknown service action: %s", action)
	}
}

func printServiceUsage(w *os.File) {
	_, _ = fmt.Fprint(w, `Usage: flamegate service [action] [flags]

Actions:
  install      Install FlameGate as a background system/user service
  uninstall    Remove the FlameGate background service
  start        Start the FlameGate background service
  stop         Stop the FlameGate background service
  restart      Restart the FlameGate background service
  status       Check background service status (Running / Stopped)

Flags:
  -c, --config <path>  Path to configuration file to use for the service
`)
}
