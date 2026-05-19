package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

const (
	agentLabel    = "de.teams-music"
	agentFileName = "de.teams-music.plist"
)

// plist Template
const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{{ .Label }}</string>

    <key>ProgramArguments</key>
    <array>
        <string>{{ .BinaryPath }}</string>
        {{- if .ConfigPath }}
        <string>--config</string>
        <string>{{ .ConfigPath }}</string>
        {{- end }}
    </array>

    <key>RunAtLoad</key>
    <{{ .RunAtLoad }}/>

    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>

    <key>ThrottleInterval</key>
    <integer>10</integer>

    <key>ProcessType</key>
    <string>Background</string>

    <key>StandardOutPath</key>
    <string>{{ .LogDir }}/teams-music.out.log</string>

    <key>StandardErrorPath</key>
    <string>{{ .LogDir }}/teams-music.err.log</string>

    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/usr/local/bin:/usr/bin:/bin</string>
    </dict>
</dict>
</plist>
`

// LaunchAgentConfig contains data for plist generation.
type LaunchAgentConfig struct {
	Label      string
	BinaryPath string
	ConfigPath string
	RunAtLoad  string // "true" or "false"
	LogDir     string
}

// AgentPlistPath returns the path to the LaunchAgent plist.
func AgentPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", agentFileName)
}

// InstallAgent installs the LaunchAgent.
// - Resolves the path to the binary
// - Generates the plist
// - Loads the agent via launchctl
func InstallAgent(configPath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home-verzeichnis nicht gefunden: %w", err)
	}

	// Resolve binary path
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("binary-pfad nicht ermittelbar: %w", err)
	}
	binaryPath, _ = filepath.Abs(binaryPath)

	// Log directory
	logDir := filepath.Join(home, "Library", "Logs")
	os.MkdirAll(logDir, 0755)

	// Config path (empty = default)
	if configPath == "" {
		configPath = ""
	}

	cfg := LaunchAgentConfig{
		Label:      agentLabel,
		BinaryPath: binaryPath,
		ConfigPath: configPath,
		RunAtLoad:  "true",
		LogDir:     logDir,
	}

	// Generate plist
	tmpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		return fmt.Errorf("template parse: %w", err)
	}

	plistPath := AgentPlistPath()
	os.MkdirAll(filepath.Dir(plistPath), 0755)

	f, err := os.Create(plistPath)
	if err != nil {
		return fmt.Errorf("plist erstellen (%s): %w", plistPath, err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, cfg); err != nil {
		return fmt.Errorf("plist schreiben: %w", err)
	}

	fmt.Printf("✅ LaunchAgent plist erstellt: %s\n", plistPath)

	// Load agent
	if err := launchctlBootstrap(plistPath); err != nil {
		return err
	}

	fmt.Println("✅ LaunchAgent geladen – teams-music läuft jetzt im Hintergrund")
	fmt.Println()
	fmt.Println("   Nützliche Befehle:")
	fmt.Printf("   Status:      launchctl print gui/$(id -u)/%s\n", agentLabel)
	fmt.Printf("   Stoppen:     launchctl bootout gui/$(id -u)/%s\n", agentLabel)
	fmt.Printf("   Logs:        tail -f ~/Library/Logs/teams-music.out.log\n")
	fmt.Printf("   Deinstall.:  %s --uninstall\n", binaryPath)

	return nil
}

// UninstallAgent uninstalls the LaunchAgent.
func UninstallAgent() error {
	plistPath := AgentPlistPath()

	// Stop agent (ignore errors if not loaded)
	launchctlBootout()

	// Delete plist
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("plist löschen (%s): %w", plistPath, err)
	}

	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		fmt.Println("✅ LaunchAgent deinstalliert")
	}

	return nil
}

// StatusAgent prints the LaunchAgent status.
func StatusAgent() {
	uid := os.Getenv("UID")
	if uid == "" {
		out, err := exec.Command("id", "-u").Output()
		if err == nil {
			uid = strings.TrimSpace(string(out))
		}
	}

	target := fmt.Sprintf("gui/%s/%s", uid, agentLabel)
	cmd := exec.Command("launchctl", "print", target)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("ℹ️  LaunchAgent '%s' ist nicht geladen\n", agentLabel)
		fmt.Println("   Installieren mit: ./teams-music --install")
	}
}

// ─── launchctl Helpers ───────────────────────────────────

func getUID() string {
	out, err := exec.Command("id", "-u").Output()
	if err != nil {
		return "501" // fallback
	}
	return strings.TrimSpace(string(out))
}

func launchctlBootstrap(plistPath string) error {
	// First try unloading previous agent
	launchctlBootout()

	uid := getUID()
	target := fmt.Sprintf("gui/%s", uid)

	// Attempt 1: modern bootstrap
	cmd := exec.Command("launchctl", "bootstrap", target, plistPath)
	out, err := cmd.CombinedOutput()

	if err == nil {
		return nil
	}

	outStr := strings.TrimSpace(string(out))

	// Error 125 = "Domain does not support specified action"
	// -> agent may have already auto-loaded when plist was written
	if strings.Contains(outStr, "125") || strings.Contains(outStr, "already loaded") ||
		strings.Contains(outStr, "Domain does not support") {

		// Check whether the agent is actually running
		checkTarget := fmt.Sprintf("gui/%s/%s", uid, agentLabel)
		checkCmd := exec.Command("launchctl", "print", checkTarget)
		if checkCmd.Run() == nil {
			// Agent is loaded -> all good
			fmt.Println("ℹ️  LaunchAgent war bereits geladen")
			return nil
		}
	}

	// Attempt 2: fallback to legacy load (often more reliable)
	fmt.Println("ℹ️  bootstrap fehlgeschlagen, versuche legacy load...")
	loadCmd := exec.Command("launchctl", "load", "-w", plistPath)
	loadOut, loadErr := loadCmd.CombinedOutput()

	if loadErr == nil {
		return nil
	}

	loadOutStr := strings.TrimSpace(string(loadOut))

	// load can also return "already loaded" -> OK
	if strings.Contains(loadOutStr, "already loaded") ||
		strings.Contains(loadOutStr, "service already loaded") {
		fmt.Println("ℹ️  LaunchAgent war bereits geladen")
		return nil
	}

	return fmt.Errorf("launchctl fehlgeschlagen:\n"+
		"   bootstrap: %s\n"+
		"   load:      %s\n"+
		"   Manuell:   launchctl load %s", outStr, loadOutStr, plistPath)
}

func launchctlBootout() {
	uid := getUID()

	// Attempt 1: modern bootout
	target := fmt.Sprintf("gui/%s/%s", uid, agentLabel)
	cmd := exec.Command("launchctl", "bootout", target)
	if cmd.Run() == nil {
		return
	}

	// Attempt 2: legacy unload
	plistPath := AgentPlistPath()
	exec.Command("launchctl", "unload", plistPath).Run()
}
