// Package auth provides account persistence for multi-account configurations.
//
// This file implements the JSON-based persistence layer for per-account
// identity configuration (client_id, tenant_id, auth_method). The accounts
// file stores only non-secret metadata; tokens and credentials are managed
// separately by the OS-native token cache.
package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/desek/outlook-local-mcp/internal/resource"
)

// AccountConfig holds the identity configuration for a single account.
// These fields are persisted to the accounts JSON file and used to
// reconstruct credentials after a server restart.
type AccountConfig struct {
	// AccountID is the immutable local provenance identity for this persisted
	// account instance. Labels and UPNs may be reused after removal, but this
	// identity is never reused or changed in place.
	AccountID AccountID `json:"account_id,omitempty"`

	// Label is the unique human-readable identifier for this account.
	Label string `json:"label"`

	// ClientID is the OAuth 2.0 client (application) ID for this account's
	// app registration.
	ClientID string `json:"client_id"`

	// TenantID is the Entra ID tenant identifier for this account
	// (e.g., "common", "organizations", or a specific tenant GUID).
	TenantID string `json:"tenant_id"`

	// AuthMethod is the authentication method used for this account
	// (e.g., "auth_code", "browser", "device_code").
	AuthMethod string `json:"auth_method"`

	// CalendarPolicy is the explicit own-calendar permission selected by the
	// user. Nil is accepted only as pre-CR-0069 migration input.
	CalendarPolicy *CalendarPolicy `json:"calendar_policy,omitempty"`

	// ReauthenticationRequired records that the current token grant does not
	// match the account's configured deterministic scope union.
	ReauthenticationRequired bool `json:"reauthentication_required,omitempty"`

	// UPN is the User Principal Name (e.g., "alice@contoso.com") resolved
	// from the Microsoft Graph /me endpoint after authentication. It serves
	// as a stable, human-recognizable account identity that is persisted
	// across restarts so the registry can populate AccountEntry.Email
	// without a Graph API call at startup. Empty for accounts created
	// before CR-0056 or before EnsureEmail has run; backfilled lazily.
	UPN string `json:"upn"`

	// MailProfile is the stable per-account capability profile name. Empty
	// records are legacy entries and derive their profile from server defaults.
	// New writes use MailPolicy; this field remains read-only migration input.
	MailProfile string `json:"mail_profile,omitempty"`

	// MailPolicy is the independent own-mail action policy. A nil value marks
	// a legacy record that must be migrated before runtime registration.
	MailPolicy *MailActionPolicy `json:"mail_policy,omitempty"`

	// CalendarAliases is the account-scoped allowlist of shared calendar
	// targets. A missing field is the secure legacy default of no aliases.
	CalendarAliases *[]resource.CalendarAlias `json:"calendar_aliases,omitempty"`

	// MailAliases is the separate account-scoped allowlist of shared owner-view
	// mailboxes. A missing field is the secure legacy default of no aliases.
	MailAliases *[]resource.MailAlias `json:"mail_aliases,omitempty"`
}

// AccountsFile is the top-level structure of the persistent accounts JSON file.
// It wraps a slice of AccountConfig entries.
type AccountsFile struct {
	// SchemaVersion identifies the persisted account-policy schema. Version 2
	// introduces explicit own-calendar access and the breaking auth reset.
	SchemaVersion int `json:"schema_version"`

	// Accounts is the list of persisted account configurations.
	Accounts []AccountConfig `json:"accounts"`
}

// LoadAccounts reads account configurations from the JSON file at the given path.
// If the file does not exist, an empty slice is returned with no error.
//
// Parameters:
//   - path: absolute filesystem path to the accounts JSON file.
//
// Returns the loaded account configurations, or an error if the file exists
// but cannot be read or parsed.
func LoadAccounts(path string) ([]AccountConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []AccountConfig{}, nil
		}
		return nil, fmt.Errorf("read accounts file: %w", err)
	}

	var file AccountsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse accounts file: %w", err)
	}

	return file.Accounts, nil
}

// SaveAccounts writes account configurations to the JSON file at the given path.
// The write is atomic: data is written to a temporary file in the same directory,
// then renamed to the target path. This prevents corruption from crashes during write.
//
// Parameters:
//   - path: absolute filesystem path to the accounts JSON file.
//   - accounts: the account configurations to persist.
//
// Returns an error if the file cannot be written.
//
// Side effects: creates or overwrites the file at path. Creates the parent
// directory if it does not exist.
func SaveAccounts(path string, accounts []AccountConfig) error {
	return saveAccountsWithRename(path, accounts, os.Rename)
}

// saveAccountsWithRename implements [SaveAccounts] with an injected final
// rename operation. The seam permits deterministic cross-platform verification
// that a failed replacement preserves the original accounts file.
//
// Parameters:
//   - path: absolute filesystem path to the accounts JSON file.
//   - accounts: the account configurations to persist.
//   - rename: final atomic replacement operation.
//
// Returns an error from serialization, temporary-file I/O, or replacement.
// Side effects match [SaveAccounts]; failed replacements remove the temporary
// file and leave any existing target untouched.
func saveAccountsWithRename(path string, accounts []AccountConfig, rename func(string, string) error) error {
	file := AccountsFile{SchemaVersion: CurrentAccountsSchemaVersion, Accounts: accounts}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal accounts file: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create accounts directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "accounts-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()        //nolint:errcheck // best-effort cleanup on write failure
		os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup on write failure
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup on close failure
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup on rename failure
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// AddAccountConfig appends an account configuration to the persistent accounts
// file. The file is loaded, the new config is appended, and the file is saved
// atomically.
//
// Parameters:
//   - path: absolute filesystem path to the accounts JSON file.
//   - config: the account configuration to add.
//
// Returns an error if the file cannot be loaded or saved, the supplied
// identity is malformed, or secure randomness cannot generate a missing
// identity.
//
// Side effects: modifies the accounts file at path.
func AddAccountConfig(path string, config AccountConfig) error {
	accounts, err := LoadAccounts(path)
	if err != nil {
		return err
	}
	if config.AccountID == "" {
		config.AccountID, err = NewAccountID()
		if err != nil {
			return err
		}
	} else if err := config.AccountID.Validate(); err != nil {
		return err
	}

	accounts = append(accounts, config)

	return SaveAccounts(path, accounts)
}

// RemoveAccountConfig removes an account configuration by label from the
// persistent accounts file. If the label is not found, no error is returned
// and the file is unchanged.
//
// Parameters:
//   - path: absolute filesystem path to the accounts JSON file.
//   - label: the label of the account to remove.
//
// Returns an error if the file cannot be loaded or saved.
//
// Side effects: modifies the accounts file at path if the label is found.
func RemoveAccountConfig(path string, label string) error {
	accounts, err := LoadAccounts(path)
	if err != nil {
		return err
	}

	filtered := make([]AccountConfig, 0, len(accounts))
	for _, a := range accounts {
		if a.Label != label {
			filtered = append(filtered, a)
		}
	}

	// Only write if something was actually removed.
	if len(filtered) == len(accounts) {
		return nil
	}

	return SaveAccounts(path, filtered)
}

// SetAccountMailProfile updates one persisted account's mail profile.
// It rewrites accounts.json atomically through SaveAccounts and returns an
// error when the label does not exist or the file cannot be read or written.
func SetAccountMailProfile(path string, label string, profile MailProfile) error {
	accounts, err := LoadAccounts(path)
	if err != nil {
		return err
	}
	for i := range accounts {
		if accounts[i].Label != label {
			continue
		}
		accounts[i].MailProfile = profile.String()
		return SaveAccounts(path, accounts)
	}
	return fmt.Errorf("account %q not found in accounts file", label)
}

// UpsertAccountConfig replaces a persisted account with the same label or
// appends config when the runtime account was previously implicit. It writes
// through SaveAccounts, preserves the ordering of existing records, and never
// replaces an existing immutable account identity.
//
// Parameters:
//   - path: absolute filesystem path to the accounts JSON file.
//   - config: the complete account configuration to insert or replace.
//
// Returns an error when loading or saving fails, an identity is malformed,
// secure randomness cannot generate a missing identity, or the caller tries
// to replace an existing identity. Its side effect is an atomic accounts-file
// rewrite on success.
func UpsertAccountConfig(path string, config AccountConfig) error {
	accounts, err := LoadAccounts(path)
	if err != nil {
		return err
	}
	for index := range accounts {
		if accounts[index].Label == config.Label {
			existingID := accounts[index].AccountID
			if existingID == "" && config.AccountID == "" {
				config.AccountID, err = NewAccountID()
				if err != nil {
					return err
				}
			} else if existingID == "" {
				if err := config.AccountID.Validate(); err != nil {
					return err
				}
			} else if config.AccountID == "" {
				config.AccountID = accounts[index].AccountID
			} else if config.AccountID != existingID {
				return fmt.Errorf("account %q identity cannot be changed", config.Label)
			}
			accounts[index] = config
			return SaveAccounts(path, accounts)
		}
	}
	if config.AccountID == "" {
		config.AccountID, err = NewAccountID()
		if err != nil {
			return err
		}
	} else if err := config.AccountID.Validate(); err != nil {
		return err
	}
	return SaveAccounts(path, append(accounts, config))
}

// MigrateAccountIDs assigns and persists immutable identities for legacy
// account records that predate account provenance. Existing identities are
// validated and preserved. The migration is idempotent and rewrites the file
// only when at least one identity is missing.
//
// Parameters:
//   - path: absolute filesystem path to the accounts JSON file.
//
// Returns an error when the file cannot be loaded or saved, secure randomness
// is unavailable, or an existing identity is malformed. Its only side effect
// is an atomic accounts-file rewrite when migration is required.
func MigrateAccountIDs(path string) error {
	accounts, err := LoadAccounts(path)
	if err != nil {
		return err
	}
	changed := false
	for index := range accounts {
		if accounts[index].AccountID != "" {
			if err := accounts[index].AccountID.Validate(); err != nil {
				return fmt.Errorf("account %q: %w", accounts[index].Label, err)
			}
			continue
		}
		accounts[index].AccountID, err = NewAccountID()
		if err != nil {
			return fmt.Errorf("account %q: %w", accounts[index].Label, err)
		}
		changed = true
	}
	if !changed {
		return nil
	}
	return SaveAccounts(path, accounts)
}

// FindByIdentity searches accounts for the first entry whose ClientID and
// TenantID both match the supplied arguments. It is used by main.go at startup
// to determine whether the env-cfg identity is already covered by a named entry
// in accounts.json, so that the implicit "default" registration can be skipped.
//
// Parameters:
//   - accounts: the account configurations to search.
//   - clientID: the OAuth 2.0 client ID to match.
//   - tenantID: the Entra ID tenant identifier to match.
//
// Returns the first matching AccountConfig and true, or (AccountConfig{}, false)
// when no match is found. Empty clientID or tenantID arguments always return
// (AccountConfig{}, false) to prevent spurious matches against zero-value fields.
func FindByIdentity(accounts []AccountConfig, clientID, tenantID string) (AccountConfig, bool) {
	if clientID == "" || tenantID == "" {
		return AccountConfig{}, false
	}
	for _, a := range accounts {
		if a.ClientID == clientID && a.TenantID == tenantID {
			return a, true
		}
	}
	return AccountConfig{}, false
}

// UpdateAccountUPN sets the UPN field for the account identified by label in
// the persistent accounts file. The file is loaded, the matching entry's UPN
// is replaced, and the file is saved atomically. If the label is not found,
// no error is returned and the file is left unchanged (callers can choose to
// treat this as a silent no-op migration path).
//
// Parameters:
//   - path: absolute filesystem path to the accounts JSON file.
//   - label: label of the account whose UPN should be updated.
//   - upn: the User Principal Name to persist.
//
// Returns an error if the file cannot be loaded or saved. Returns nil if the
// label is not found.
//
// Side effects: rewrites the accounts file at path when the label matches.
func UpdateAccountUPN(path string, label string, upn string) error {
	accounts, err := LoadAccounts(path)
	if err != nil {
		return err
	}

	changed := false
	for i := range accounts {
		if accounts[i].Label == label {
			if accounts[i].UPN == upn {
				return nil
			}
			accounts[i].UPN = upn
			changed = true
			break
		}
	}

	if !changed {
		return nil
	}

	return SaveAccounts(path, accounts)
}
