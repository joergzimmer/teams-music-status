package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"teams-music/internal/config"
	"teams-music/internal/daemon"
)

func main() {
	// ─── CLI Flags ────────────────────────────────────────
	configPath := flag.String("config", "", "Path to config file (default: ~/.config/teams-music/config.yaml)")
	initFlag := flag.Bool("init", false, "Creates an example config and exits")
	installFlag := flag.Bool("install", false, "Installs the macOS LaunchAgent (autostart)")
	uninstallFlag := flag.Bool("uninstall", false, "Uninstalls the macOS LaunchAgent")
	statusFlag := flag.Bool("status", false, "Shows the LaunchAgent status")
	flag.Parse()

	// ─── Subcommands ─────────────────────────────────────

	// --init: create example config
	if *initFlag {
		handleInit(*configPath)
		return
	}

	// --install: install LaunchAgent
	if *installFlag {
		handleInstall(*configPath)
		return
	}

	// --uninstall: uninstall LaunchAgent
	if *uninstallFlag {
		handleUninstall()
		return
	}

	// --status: LaunchAgent status
	if *statusFlag {
		daemon.StatusAgent()
		return
	}

	// ─── Normal start: run daemon ───────────────────────
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Config error: %v\n\n", err)
		fmt.Fprintf(os.Stderr, "   Tip: Run with --init to create an example config:\n")
		fmt.Fprintf(os.Stderr, "         ./teams-music --init\n\n")
		os.Exit(1)
	}

	logger := setupLogger(cfg.Behavior.LogLevel, cfg.Behavior.LogFile)
	logger.Info("Config loaded",
		"path", resolveConfigPath(*configPath),
		"tenant", cfg.Azure.TenantID[:8]+"...",
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	svc := daemon.NewService(cfg, logger)
	if err := svc.Run(ctx); err != nil {
		logger.Error("Daemon error", "error", err)
		os.Exit(1)
	}
}

// ─── Subcommand Handlers ─────────────────────────────────

func handleInit(configPath string) {
	path := config.DefaultConfigPath()
	if configPath != "" {
		path = configPath
	}

	created, err := config.WriteExample(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}

	if created {
		fmt.Printf("✅ Example config created: %s\n", path)
		fmt.Println("   Please fill in tenant_id and client_id, then start again.")
	} else {
		fmt.Printf("ℹ️  Config already exists: %s\n", path)
	}
}

func handleInstall(configPath string) {
	// Validate config before installing
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Config error: %v\n", err)
		fmt.Fprintf(os.Stderr, "   Please set up config first: ./teams-music --init\n")
		os.Exit(1)
	}
	_ = cfg // Config is valid

	fmt.Println("Installing LaunchAgent...")
	if err := daemon.InstallAgent(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Installation failed: %v\n", err)
		os.Exit(1)
	}
}

func handleUninstall() {
	fmt.Println("Uninstalling LaunchAgent...")
	if err := daemon.UninstallAgent(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Uninstallation failed: %v\n", err)
		os.Exit(1)
	}
}

// ─── Helper Functions ────────────────────────────────────

func setupLogger(level string, logFile string) *slog.Logger {
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: logLevel}

	var writer *os.File
	if logFile != "" {
		if strings.HasPrefix(logFile, "~/") {
			home, _ := os.UserHomeDir()
			logFile = filepath.Join(home, logFile[2:])
		}
		dir := filepath.Dir(logFile)
		os.MkdirAll(dir, 0755)

		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Log file not writable (%s): %v – using stdout\n", logFile, err)
			writer = os.Stdout
		} else {
			writer = f
		}
	} else {
		writer = os.Stdout
	}

	return slog.New(slog.NewTextHandler(writer, opts))
}

func resolveConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return config.DefaultConfigPath()
}
