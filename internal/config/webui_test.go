package config

import "testing"

// TestLoadConfigWebUIDefaults verifies the optional UI is disabled and uses
// port 8155 when no environment overrides are supplied.
func TestLoadConfigWebUIDefaults(t *testing.T) {
	t.Setenv("OUTLOOK_MCP_WEB_UI_ENABLED", "")
	t.Setenv("OUTLOOK_MCP_WEB_UI_PORT", "")
	cfg := LoadConfig()
	if cfg.WebUIEnabled || cfg.WebUIPort != DefaultWebUIPort {
		t.Fatalf("web UI defaults = (%v, %d), want (false, %d)", cfg.WebUIEnabled, cfg.WebUIPort, DefaultWebUIPort)
	}
}

// TestApplyCLIOverridesWebUI verifies explicit command-line values take
// precedence over environment-derived configuration.
func TestApplyCLIOverridesWebUI(t *testing.T) {
	cfg := Config{WebUIEnabled: false, WebUIPort: 9000}
	updated, err := ApplyCLIOverrides(cfg, []string{"--web-ui", "--web-ui-port", "8155"})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.WebUIEnabled || updated.WebUIPort != 8155 {
		t.Fatalf("CLI overrides = (%v, %d), want (true, 8155)", updated.WebUIEnabled, updated.WebUIPort)
	}
}

// TestValidateConfigRejectsWebUIPort verifies invalid listener ports fail
// normal configuration validation even when the UI is disabled.
func TestValidateConfigRejectsWebUIPort(t *testing.T) {
	cfg := LoadConfig()
	cfg.WebUIPort = 70000
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("ValidateConfig() succeeded for invalid web UI port")
	}
}
