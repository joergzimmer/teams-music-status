package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"

	"teams-music/internal/auth"
	"teams-music/internal/config"
	"teams-music/internal/musicwatcher"
	"teams-music/internal/teams"
)

// Service is the central daemon that orchestrates all modules.
type Service struct {
	cfg    *config.Config
	logger *slog.Logger

	tokenMgr    *auth.TokenManager
	graphClient *teams.GraphClient
	watcher     *musicwatcher.Watcher
	debouncer   *Debouncer
}

// NewService creates a new daemon service.
func NewService(cfg *config.Config, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}

	return &Service{
		cfg:    cfg,
		logger: logger,
	}
}

// Run starts the full service. Blocks until ctx is canceled.
func (s *Service) Run(ctx context.Context) error {
	// ─── 1. Initialize auth ───────────────────────────────
	s.logger.Info("Initialisiere Authentifizierung...")

	tokenPath := filepath.Join(config.DefaultConfigDir(), "token.json")
	s.tokenMgr = auth.NewTokenManager(auth.AzureConfig{
		TenantID: s.cfg.Azure.TenantID,
		ClientID: s.cfg.Azure.ClientID,
		Scopes:   []string{"Presence.ReadWrite"},
	}, tokenPath, s.logger)

	if _, err := s.tokenMgr.GetAccessToken(ctx); err != nil {
		return fmt.Errorf("authentifizierung fehlgeschlagen: %w", err)
	}
	s.logger.Info("Authentifizierung erfolgreich")

	// ─── 2. Initialize Graph client ───────────────────────
	s.graphClient = teams.NewGraphClient(s.tokenMgr, teams.ClientConfig{
		TimeZone:      s.cfg.Status.TimeZone,
		ExpiryMinutes: s.cfg.Status.ExpiryMin,
	}, s.logger)

	// Connectivity check
	presence, err := s.graphClient.GetPresence(ctx)
	if err != nil {
		return fmt.Errorf("graph API verbindungstest fehlgeschlagen: %w", err)
	}
	s.logger.Info("Graph API verbunden",
		"availability", presence.Availability,
		"activity", presence.Activity,
	)

	// ─── 3. Initialize music watcher ──────────────────────
	s.watcher, err = musicwatcher.New(s.logger, 16, s.cfg.Behavior.DebounceSec)
	if err != nil {
		return fmt.Errorf("music watcher erstellen: %w", err)
	}

	// ─── 4. Initialize debouncer ──────────────────────────
	s.debouncer = NewDebouncer(s.cfg.Behavior.DebounceSec, s.logger)

	// ─── 5. Start pipeline ────────────────────────────────
	var wg sync.WaitGroup

	// Debouncer: Watcher-Events → debounced Events
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.debouncer.Run(ctx, s.watcher.Events(), s.cfg.Status.ClearOnPause)
	}()

	// Status updater: debounced events -> Graph API
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.processEvents(ctx)
	}()

	// Start watcher (blocks until ctx is canceled)
	s.logger.Info("🎶 teams-music Daemon gestartet",
		"debounce_sec", s.cfg.Behavior.DebounceSec,
		"expiry_min", s.cfg.Status.ExpiryMin,
		"clear_on_pause", s.cfg.Status.ClearOnPause,
		"template", s.cfg.Status.Template,
	)

	watcherErr := s.watcher.Start(ctx)

	// Wait until all goroutines are done
	wg.Wait()

	s.logger.Info("Daemon beendet")
	return watcherErr
}

// processEvents handles debounced events and updates Teams status.
func (s *Service) processEvents(ctx context.Context) {
	for event := range s.debouncer.Output() {
		if ctx.Err() != nil {
			return
		}

		if event.IsClear {
			s.logger.Info("Musik gestoppt/pausiert – lösche Status",
				"state", event.Track.State,
			)
			if err := s.graphClient.ClearStatusMessage(ctx); err != nil {
				s.logger.Error("Status löschen fehlgeschlagen", "error", err)
			}
			continue
		}

		// Set status
		msg := formatTemplate(s.cfg.Status.Template, event.Track)
		update := teams.NewStatusUpdate(msg, s.cfg.Status.ExpiryMin)

		s.logger.Info("Aktualisiere Teams-Status",
			"message", msg,
			"track", event.Track.Name,
			"artist", event.Track.Artist,
		)

		if err := s.graphClient.SetStatusMessage(ctx, update); err != nil {
			s.logger.Error("Status setzen fehlgeschlagen",
				"error", err,
				"track", event.Track.Name,
			)
		}
	}
}

// formatTemplate renders the status template using track data.
func formatTemplate(tmpl string, track musicwatcher.TrackInfo) string {
	if tmpl == "" {
		tmpl = "🎵 {{.Name}} – {{.Artist}}"
	}

	// Fast string replacement (robust enough for our template format)
	replacer := map[string]string{
		"{{.Name}}":   track.Name,
		"{{.Artist}}": track.Artist,
		"{{.Album}}":  track.Album,
	}

	result := tmpl
	for placeholder, value := range replacer {
		for {
			i := indexOf(result, placeholder)
			if i < 0 {
				break
			}
			result = result[:i] + value + result[i+len(placeholder):]
		}
	}
	return result
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
