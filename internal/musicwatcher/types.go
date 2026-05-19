//go:build darwin

package musicwatcher

import (
	"fmt"
	"strings"
	"time"
)

type PlayerState string

const (
	StatePlaying PlayerState = "Playing"
	StatePaused  PlayerState = "Paused"
	StateStopped PlayerState = "Stopped"
	StateUnknown PlayerState = "Unknown"
)

type TrackInfo struct {
	Name       string
	Artist     string
	Album      string
	State      PlayerState
	Duration   time.Duration
	ReceivedAt time.Time
}

func (t TrackInfo) IsEmpty() bool {
	return t.Name == "" && t.Artist == ""
}

func (t TrackInfo) Equal(other TrackInfo) bool {
	return t.Name == other.Name &&
		t.Artist == other.Artist &&
		t.Album == other.Album
}

func parseState(s string) PlayerState {
	switch s {
	case "Playing":
		return StatePlaying
	case "Paused":
		return StatePaused
	case "Stopped":
		return StateStopped
	default:
		return StateUnknown
	}
}

func parseDuration(s string) time.Duration {
	s = strings.TrimSpace(s)
	var secs float64
	fmt.Sscanf(s, "%f", &secs)
	return time.Duration(secs * float64(time.Second))
}
