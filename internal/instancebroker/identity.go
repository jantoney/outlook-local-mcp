package instancebroker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/desek/outlook-local-mcp/internal/config"
)

// ProtocolVersion identifies the private proxy-to-broker contract. Changing
// incompatible endpoint or recovery semantics requires incrementing it.
const ProtocolVersion = 1

// Identity uniquely describes a compatible broker runtime and its state realm.
// It prevents executables or stores in different locations from accidentally
// sharing an in-memory registry or HTTP endpoint.
type Identity struct {
	// Key is the SHA-256 identity used for election and descriptor names.
	Key string `json:"key"`
	// StateKey identifies the mutable account and token-cache realm.
	StateKey string `json:"state_key"`
	// ExecutablePath is the canonical path of the running binary.
	ExecutablePath string `json:"executable_path"`
	// AccountsPath is the canonical persistent account registry path.
	AccountsPath string `json:"accounts_path"`
	// Protocol is the private broker protocol version.
	Protocol int `json:"protocol"`
}

// NewIdentity derives a deterministic identity from executable location,
// mutable state paths, cache namespace, and behavior-affecting configuration.
// It returns an error when a required path cannot be made absolute.
func NewIdentity(executablePath string, cfg config.Config) (Identity, error) {
	executable, err := canonicalPath(executablePath)
	if err != nil {
		return Identity{}, fmt.Errorf("canonical executable path: %w", err)
	}
	accounts, err := canonicalPath(cfg.AccountsPath)
	if err != nil {
		return Identity{}, fmt.Errorf("canonical accounts path: %w", err)
	}
	authRecord, err := canonicalPath(cfg.AuthRecordPath)
	if err != nil {
		return Identity{}, fmt.Errorf("canonical auth record path: %w", err)
	}

	stateMaterial, err := json.Marshal(struct {
		AccountsPath   string `json:"accounts_path"`
		AuthRecordPath string `json:"auth_record_path"`
		CacheName      string `json:"cache_name"`
		TokenStorage   string `json:"token_storage"`
	}{accounts, authRecord, cfg.CacheName, cfg.TokenStorage})
	if err != nil {
		return Identity{}, fmt.Errorf("encode state identity: %w", err)
	}
	stateKey := digest(stateMaterial)

	behaviorMaterial, err := json.Marshal(struct {
		Executable        string   `json:"executable"`
		StateKey          string   `json:"state_key"`
		Protocol          int      `json:"protocol"`
		ClientID          string   `json:"client_id"`
		TenantID          string   `json:"tenant_id"`
		AuthMethod        string   `json:"auth_method"`
		DefaultTimezone   string   `json:"default_timezone"`
		ReadOnly          bool     `json:"read_only"`
		MailEnabled       bool     `json:"mail_enabled"`
		MailManage        bool     `json:"mail_manage"`
		MailSend          bool     `json:"mail_send"`
		AttachmentRoots   []string `json:"attachment_roots"`
		MaxAttachmentSize int64    `json:"max_attachment_size"`
		ProvenanceTag     string   `json:"provenance_tag"`
		RequestTimeoutNS  int64    `json:"request_timeout_ns"`
		MaxRetries        int      `json:"max_retries"`
		RetryBackoffMS    int      `json:"retry_backoff_ms"`
		WebUIEnabled      bool     `json:"web_ui_enabled"`
		WebUIPort         int      `json:"web_ui_port"`
		LogLevel          string   `json:"log_level"`
		LogFormat         string   `json:"log_format"`
		LogFile           string   `json:"log_file"`
		LogSanitize       bool     `json:"log_sanitize"`
		AuditEnabled      bool     `json:"audit_enabled"`
		AuditPath         string   `json:"audit_path"`
		OTELEnabled       bool     `json:"otel_enabled"`
		OTELEndpoint      string   `json:"otel_endpoint"`
		OTELServiceName   string   `json:"otel_service_name"`
	}{
		executable, stateKey, ProtocolVersion, cfg.ClientID, cfg.TenantID,
		cfg.AuthMethod, cfg.DefaultTimezone, cfg.ReadOnly, cfg.MailEnabled,
		cfg.MailManageEnabled, cfg.MailSendEnabled, cfg.AttachmentRoots,
		cfg.MaxAttachmentSizeBytes, cfg.ProvenanceTag, int64(cfg.RequestTimeout),
		cfg.MaxRetries, cfg.RetryBackoffMS, cfg.WebUIEnabled, cfg.WebUIPort,
		cfg.LogLevel, cfg.LogFormat, cfg.LogFile, cfg.LogSanitize,
		cfg.AuditLogEnabled, cfg.AuditLogPath, cfg.OTELEnabled, cfg.OTELEndpoint,
		cfg.OTELServiceName,
	})
	if err != nil {
		return Identity{}, fmt.Errorf("encode broker identity: %w", err)
	}
	return Identity{Key: digest(behaviorMaterial), StateKey: stateKey, ExecutablePath: executable, AccountsPath: accounts, Protocol: ProtocolVersion}, nil
}

// DescriptorPath returns the private endpoint descriptor path adjacent to the
// account registry. It does not create directories or files.
func (i Identity) DescriptorPath() string {
	return filepath.Join(filepath.Dir(i.AccountsPath), ".broker", "endpoint-"+i.Key+".json")
}

// ElectionAddress returns the deterministic IPv4 loopback address on which
// compatible processes elect their authoritative broker.
func (i Identity) ElectionAddress() string {
	return hashedAddress(i.Key, 20000)
}

// StateAddress returns a separate deterministic guard address for the mutable
// state realm. A broker must hold this guard before account restoration, so
// incompatible executable/config identities cannot mutate one store at once.
func (i Identity) StateAddress() string {
	return hashedAddress(i.StateKey, 40000)
}

// canonicalPath makes path absolute and normalizes Windows case so equivalent
// paths produce the same identity. It does not require the target to exist.
func canonicalPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	cleaned := filepath.Clean(absolute)
	if runtime.GOOS == "windows" {
		cleaned = strings.ToLower(cleaned)
	}
	return cleaned, nil
}

// digest returns a lowercase SHA-256 digest for deterministic identity keys.
func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

// hexDigit converts one lowercase hexadecimal byte to its numeric value.
func hexDigit(value byte) int {
	if value >= 'a' {
		return int(value-'a') + 10
	}
	return int(value - '0')
}

// hashedAddress maps a SHA-256 key into one of twenty thousand ports starting
// at base. Election and state guards use disjoint port ranges.
func hashedAddress(key string, base int) string {
	value := hexDigit(key[0])
	for index := 1; index < 8; index++ {
		value = value*16 + hexDigit(key[index])
	}
	return fmt.Sprintf("127.0.0.1:%d", base+(value%20000))
}
