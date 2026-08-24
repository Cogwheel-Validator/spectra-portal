package routeindex

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

var pathfinderLog zerolog.Logger

func init() {
	out := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	pathfinderLog = zerolog.New(out).With().Timestamp().Str("component", "routeindex").Logger()
}
