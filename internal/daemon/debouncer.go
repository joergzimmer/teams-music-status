package daemon

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"teams-music/internal/musicwatcher"
)

// DebouncedTrack contains the track and whether it is a clear event.
type DebouncedTrack struct {
	Track   musicwatcher.TrackInfo
	IsClear bool
}

// Debouncer delays track events by a configurable duration.
// During rapid skipping, only the latest track is forwarded.
type Debouncer struct {
	delay  time.Duration
	output chan DebouncedTrack
	logger *slog.Logger

	mu       sync.Mutex
	timer    *time.Timer
	pending  *DebouncedTrack
	lastSent *musicwatcher.TrackInfo // last track that was actually sent
}

// NewDebouncer creates a new debouncer.
func NewDebouncer(delaySec int, logger *slog.Logger) *Debouncer {
	if delaySec <= 0 {
		delaySec = 3
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Debouncer{
		delay:  time.Duration(delaySec) * time.Second,
		output: make(chan DebouncedTrack, 8),
		logger: logger,
	}
}

// Output returns the channel with debounced events.
func (d *Debouncer) Output() <-chan DebouncedTrack {
	return d.output
}

// Run consumes events from the watcher and forwards them debounced.
// Blocks until ctx is canceled.
func (d *Debouncer) Run(ctx context.Context, input <-chan musicwatcher.TrackInfo, clearOnPause bool) {
	for {
		select {
		case <-ctx.Done():
			d.mu.Lock()
			if d.timer != nil {
				d.timer.Stop()
			}
			d.mu.Unlock()
			close(d.output)
			return

		case track, ok := <-input:
			if !ok {
				d.mu.Lock()
				if d.timer != nil {
					d.timer.Stop()
				}
				d.mu.Unlock()
				close(d.output)
				return
			}

			d.handleEvent(track, clearOnPause)
		}
	}
}

func (d *Debouncer) handleEvent(track musicwatcher.TrackInfo, clearOnPause bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Pause/Stop -> forward immediately (no debounce)
	if track.State == musicwatcher.StatePaused || track.State == musicwatcher.StateStopped {
		if d.timer != nil {
			d.timer.Stop()
			d.timer = nil
			d.pending = nil
		}

		if clearOnPause {
			d.logger.Debug("Sofort-Event: Musik gestoppt/pausiert", "state", track.State)
			d.lastSent = nil
			select {
			case d.output <- DebouncedTrack{Track: track, IsClear: true}:
			default:
				d.logger.Warn("Output-Channel voll, Clear-Event verworfen")
			}
		}
		return
	}

	// Playing -> debounce
	// Same as last sent track? -> ignore (no duplicate update)
	if d.lastSent != nil && d.lastSent.Equal(track) {
		d.logger.Debug("Track identisch mit letztem Update, ignoriert",
			"name", track.Name,
		)
		return
	}

	dt := DebouncedTrack{Track: track, IsClear: false}
	d.pending = &dt

	// Reset timer
	if d.timer != nil {
		d.timer.Stop()
	}

	d.logger.Debug("Debounce-Timer gestartet",
		"name", track.Name,
		"delay", d.delay,
	)

	d.timer = time.AfterFunc(d.delay, func() {
		d.mu.Lock()
		p := d.pending
		d.pending = nil
		d.timer = nil
		d.mu.Unlock()

		if p != nil {
			d.logger.Debug("Debounce abgelaufen, Event weitergeleitet",
				"name", p.Track.Name,
			)
			d.mu.Lock()
			sent := p.Track
			d.lastSent = &sent
			d.mu.Unlock()

			select {
			case d.output <- *p:
			default:
				d.logger.Warn("Output-Channel voll, Event verworfen",
					"name", p.Track.Name,
				)
			}
		}
	})
}
