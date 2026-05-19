//go:build darwin

package musicwatcher

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
)

// Watcher observes Apple Music track changes via polling.
type Watcher struct {
	events    chan TrackInfo
	logger    *slog.Logger
	running   atomic.Bool
	done      chan struct{}
	interval  time.Duration
	lastTrack *TrackInfo
}

// New creates a new watcher.
// pollInterval in seconds (default: 3).
func New(logger *slog.Logger, bufferSize int, pollIntervalSec ...int) (*Watcher, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if bufferSize <= 0 {
		bufferSize = 16
	}

	interval := 3
	if len(pollIntervalSec) > 0 && pollIntervalSec[0] > 0 {
		interval = pollIntervalSec[0]
	}

	return &Watcher{
		events:   make(chan TrackInfo, bufferSize),
		logger:   logger,
		done:     make(chan struct{}),
		interval: time.Duration(interval) * time.Second,
	}, nil
}

// Start starts the watcher. Blocks until ctx is canceled or Stop() is called.
func (w *Watcher) Start(ctx context.Context) error {
	if !w.running.CompareAndSwap(false, true) {
		return fmt.Errorf("watcher läuft bereits")
	}

	w.logger.Info("Music Watcher gestartet (Polling-Modus)",
		"interval", w.interval,
	)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Run first check immediately
	w.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Context abgebrochen, stoppe Watcher...")
			w.cleanup()
			return nil
		case <-w.done:
			w.logger.Info("Stop-Signal empfangen")
			w.cleanup()
			return nil
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}

// poll queries the current track and emits an event on change.
func (w *Watcher) poll(ctx context.Context) {
	track, err := w.queryCurrentTrack(ctx)
	if err != nil {
		w.logger.Debug("Track-Query fehlgeschlagen", "error", err)
		return
	}

	// No track -> check whether one was previously playing (-> Stopped event)
	if track == nil {
		if w.lastTrack != nil && w.lastTrack.State == StatePlaying {
			stopped := TrackInfo{
				State:      StateStopped,
				ReceivedAt: time.Now(),
			}
			w.sendEvent(stopped)
			w.lastTrack = &stopped
		}
		return
	}

	// Compare with last track
	if w.lastTrack == nil || !w.lastTrack.Equal(*track) || w.lastTrack.State != track.State {
		w.logger.Debug("Track-Änderung erkannt",
			"name", track.Name,
			"artist", track.Artist,
			"state", track.State,
		)
		w.sendEvent(*track)
		w.lastTrack = track
	}
}

// queryCurrentTrack queries the current track via osascript.
func (w *Watcher) queryCurrentTrack(ctx context.Context) (*TrackInfo, error) {
	// First check whether Music is running (without starting it)
	running, err := w.isMusicRunning(ctx)
	if err != nil || !running {
		return nil, nil
	}

	script := `
        tell application "Music"
            set pState to player state as string
            if pState is "playing" then
                return "Playing||" & name of current track & "||" & artist of current track & "||" & album of current track & "||" & (duration of current track as string)
            else if pState is "paused" then
                return "Paused||" & name of current track & "||" & artist of current track & "||" & album of current track & "||" & (duration of current track as string)
            else
                return "Stopped"
            end if
        end tell
    `

	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(queryCtx, "osascript", "-e", script).Output()
	if err != nil {
		return nil, fmt.Errorf("osascript: %w", err)
	}

	result := strings.TrimSpace(string(out))
	if result == "" || result == "Stopped" {
		return nil, nil
	}

	return w.parseResult(result), nil
}

// parseResult parses osascript output.
func (w *Watcher) parseResult(result string) *TrackInfo {
	parts := strings.Split(result, "||")
	if len(parts) < 5 {
		w.logger.Debug("Unerwartetes osascript-Format", "result", result)
		return nil
	}

	state := parseState(parts[0])
	dur := parseDuration(parts[4])

	return &TrackInfo{
		Name:       parts[1],
		Artist:     parts[2],
		Album:      parts[3],
		State:      state,
		Duration:   dur,
		ReceivedAt: time.Now(),
	}
}

// isMusicRunning checks whether Apple Music is running (without starting it).
func (w *Watcher) isMusicRunning(ctx context.Context) (bool, error) {
	queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	script := `tell application "System Events" to (name of processes) contains "Music"`
	out, err := exec.CommandContext(queryCtx, "osascript", "-e", script).Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

func (w *Watcher) sendEvent(track TrackInfo) {
	select {
	case w.events <- track:
	default:
		w.logger.Warn("Event-Channel voll, Event verworfen",
			"name", track.Name,
		)
	}
}

func (w *Watcher) cleanup() {
	w.running.Store(false)
	close(w.events)
	w.logger.Info("Music Watcher gestoppt")
}

// Stop signals the watcher to terminate.
func (w *Watcher) Stop() {
	select {
	case w.done <- struct{}{}:
	default:
	}
}

// Events returns the read channel for track events.
func (w *Watcher) Events() <-chan TrackInfo {
	return w.events
}

// IsRunning checks whether the watcher is active.
func (w *Watcher) IsRunning() bool {
	return w.running.Load()
}
