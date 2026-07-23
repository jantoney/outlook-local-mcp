package config

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// DefaultWebUIPort is the loopback TCP port used when the web UI is enabled
// without an explicit port.
const DefaultWebUIPort = 8155

// ApplyCLIOverrides parses the supported command-line flags and applies them
// over environment-derived configuration. The --web-ui boolean and
// --web-ui-port integer flags are accepted. Unknown flags or invalid values
// return an error; the function performs no network or filesystem I/O.
func ApplyCLIOverrides(cfg Config, args []string) (Config, error) {
	flags := flag.NewFlagSet("outlook-local-mcp", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	webUI := flags.Bool("web-ui", cfg.WebUIEnabled, "enable the local account-management web UI")
	webUIPort := flags.Int("web-ui-port", cfg.WebUIPort, "local account-management web UI port")
	if err := flags.Parse(args); err != nil {
		return cfg, fmt.Errorf("parse command-line flags: %w", err)
	}
	if flags.NArg() != 0 {
		return cfg, fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	cfg.WebUIEnabled = *webUI
	cfg.WebUIPort = *webUIPort
	return cfg, nil
}

// parseWebUIPort reads a decimal port string. Invalid input returns the
// documented default so validation and startup behavior remain deterministic.
func parseWebUIPort(value string) int {
	port, err := strconv.Atoi(value)
	if err != nil {
		return DefaultWebUIPort
	}
	return port
}
