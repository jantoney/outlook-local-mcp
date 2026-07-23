package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/desek/outlook-local-mcp/internal/resource"
)

// CurrentAccountsSchemaVersion is the persisted schema introduced by CR-0069.
// Version 2 makes own-calendar permissions explicit and resets authentication.
const CurrentAccountsSchemaVersion = 2

// MigrateExplicitAccountPermissions upgrades an accounts file to schema 2.
// It preserves account identity, own-mail policy, and shared resource aliases,
// initializes own-calendar access to off, marks every account as requiring
// re-authentication, and clears old account token/auth-record state. The
// migration is idempotent and rewrites the file atomically before cleanup.
//
// The accountsPath parameter identifies accounts.json, cacheNameBase identifies
// per-account cache partitions, and authRecordDir contains per-account records.
// It returns an error when reading, parsing, persisting, or security cleanup
// fails; callers must not restore accounts after such an error.
func MigrateExplicitAccountPermissions(accountsPath, cacheNameBase, authRecordDir string) error {
	data, err := os.ReadFile(accountsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read accounts migration input: %w", err)
	}

	var file AccountsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("parse accounts migration input: %w", err)
	}
	if file.SchemaVersion >= CurrentAccountsSchemaVersion {
		return nil
	}

	off := CalendarPolicyOff
	for index := range file.Accounts {
		file.Accounts[index].CalendarPolicy = &off
		file.Accounts[index].ReauthenticationRequired = true
		file.Accounts[index].MailProfile = ""
	}
	if err := SaveAccounts(accountsPath, file.Accounts); err != nil {
		return fmt.Errorf("persist explicit account permission migration: %w", err)
	}

	var cleanupErrors []error
	for _, account := range file.Accounts {
		if err := ClearTokenCache(cacheNameBase + "-" + account.Label); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("account %q token cache: %w", account.Label, err))
		}
		recordPath := filepath.Join(authRecordDir, account.Label+"_auth_record.json")
		if err := os.Remove(recordPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("account %q auth record: %w", account.Label, err))
		}
	}
	return errors.Join(cleanupErrors...)
}

// SetAccountPermissions atomically persists one account's explicit own-calendar
// and own-mail policies plus its re-authentication marker. It returns an error
// when the account does not exist or persistence fails. It performs no token
// cleanup and does not mutate the runtime registry.
func SetAccountPermissions(path, label string, calendarPolicy CalendarPolicy, mailPolicy MailActionPolicy, reauthenticationRequired bool) error {
	accounts, err := LoadAccounts(path)
	if err != nil {
		return err
	}
	for index := range accounts {
		if accounts[index].Label != label {
			continue
		}
		accounts[index].CalendarPolicy = &calendarPolicy
		accounts[index].MailPolicy = &mailPolicy
		accounts[index].MailProfile = ""
		accounts[index].ReauthenticationRequired = reauthenticationRequired
		return SaveAccounts(path, accounts)
	}
	return fmt.Errorf("account %q not found in accounts file", label)
}

// SetAccountReauthenticationRequired atomically persists the marker for one
// account. It returns an error for a missing account or storage failure and has
// no effect on runtime registry state.
func SetAccountReauthenticationRequired(path, label string, required bool) error {
	accounts, err := LoadAccounts(path)
	if err != nil {
		return err
	}
	for index := range accounts {
		if accounts[index].Label != label {
			continue
		}
		accounts[index].ReauthenticationRequired = required
		return SaveAccounts(path, accounts)
	}
	return fmt.Errorf("account %q not found in accounts file", label)
}

// SetAccountSharedResources atomically persists both shared-resource families
// and the re-authentication marker for one account. It validates each alias via
// the existing family setters' invariants before writing one accounts file.
func SetAccountSharedResources(path, label string, calendars []resource.CalendarAlias, mailboxes []resource.MailAlias, reauthenticationRequired bool) error {
	aliases := make(map[string]struct{}, len(calendars)+len(mailboxes))
	for _, alias := range calendars {
		if err := alias.Validate(); err != nil {
			return err
		}
		if _, exists := aliases[alias.Alias]; exists {
			return fmt.Errorf("shared resource alias %q is duplicated", alias.Alias)
		}
		aliases[alias.Alias] = struct{}{}
	}
	for _, alias := range mailboxes {
		if err := alias.Validate(); err != nil {
			return err
		}
		if _, exists := aliases[alias.Alias]; exists {
			return fmt.Errorf("shared resource alias %q is duplicated", alias.Alias)
		}
		aliases[alias.Alias] = struct{}{}
	}
	accounts, err := LoadAccounts(path)
	if err != nil {
		return err
	}
	for index := range accounts {
		if accounts[index].Label != label {
			continue
		}
		calendarCopy := append([]resource.CalendarAlias(nil), calendars...)
		mailCopy := append([]resource.MailAlias(nil), mailboxes...)
		accounts[index].CalendarAliases = &calendarCopy
		accounts[index].MailAliases = &mailCopy
		accounts[index].ReauthenticationRequired = reauthenticationRequired
		return SaveAccounts(path, accounts)
	}
	return fmt.Errorf("account %q not found in accounts file", label)
}
