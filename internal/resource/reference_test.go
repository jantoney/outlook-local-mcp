package resource

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/auth"
)

// TestReferenceCodecRoundTripBindsClaims verifies that a signed reference
// preserves every immutable provenance claim required for later authorization.
func TestReferenceCodecRoundTripBindsClaims(t *testing.T) {
	t.Parallel()

	key, err := LoadOrCreateSigningKey(filepath.Join(t.TempDir(), "reference.key"))
	if err != nil {
		t.Fatalf("LoadOrCreateSigningKey() error = %v", err)
	}
	codec := NewReferenceCodec(key)
	want := ReferenceClaims{
		AccountID:    auth.AccountID("11111111-1111-4111-8111-111111111111"),
		ResourceID:   "resource-42",
		ResourceKind: ResourceKindMailbox,
		MailboxView:  MailboxViewOwner,
		ItemKind:     ItemKindAttachment,
		GraphIDChain: []GraphID{
			{Kind: ItemKindMessage, ID: "message-id"},
			{Kind: ItemKindAttachment, ID: "attachment-id"},
		},
	}

	reference, err := codec.Sign(want)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	if !strings.HasPrefix(reference, ReferenceVersion+".") {
		t.Fatalf("reference = %q, want version prefix %q", reference, ReferenceVersion+".")
	}
	got, err := codec.Verify(reference)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Verify() = %+v, want %+v", got, want)
	}
}

// TestReferenceCodecRejectsTampering verifies that changing self-contained
// payload data without the signing key invalidates the reference.
func TestReferenceCodecRejectsTampering(t *testing.T) {
	t.Parallel()

	key, err := LoadOrCreateSigningKey(filepath.Join(t.TempDir(), "reference.key"))
	if err != nil {
		t.Fatalf("LoadOrCreateSigningKey() error = %v", err)
	}
	codec := NewReferenceCodec(key)
	reference, err := codec.Sign(validReferenceClaims())
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	parts := strings.Split(reference, ".")
	if parts[1][0] == 'A' {
		parts[1] = "B" + parts[1][1:]
	} else {
		parts[1] = "A" + parts[1][1:]
	}
	if _, err := codec.Verify(strings.Join(parts, ".")); err == nil {
		t.Fatal("Verify() error = nil, want tamper rejection")
	}
}

// TestSigningKeyRotationInvalidatesReference verifies that rotation revokes
// every reference issued under the previous local authority.
func TestSigningKeyRotationInvalidatesReference(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "reference.key")
	oldKey, err := LoadOrCreateSigningKey(path)
	if err != nil {
		t.Fatalf("LoadOrCreateSigningKey() error = %v", err)
	}
	reference, err := NewReferenceCodec(oldKey).Sign(validReferenceClaims())
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	newKey, err := RotateSigningKey(path)
	if err != nil {
		t.Fatalf("RotateSigningKey() error = %v", err)
	}
	if _, err := NewReferenceCodec(newKey).Verify(reference); err == nil {
		t.Fatal("Verify() error = nil after signing-key rotation")
	}
}

// TestReferenceCodecRejectsMissingGraphProvenance verifies that a reference
// cannot be issued without the Graph ID chain needed to address its item.
func TestReferenceCodecRejectsMissingGraphProvenance(t *testing.T) {
	t.Parallel()

	key, err := LoadOrCreateSigningKey(filepath.Join(t.TempDir(), "reference.key"))
	if err != nil {
		t.Fatalf("LoadOrCreateSigningKey() error = %v", err)
	}
	claims := validReferenceClaims()
	claims.GraphIDChain = nil
	if _, err := NewReferenceCodec(key).Sign(claims); err == nil {
		t.Fatal("Sign() error = nil, want missing-provenance error")
	}
}

// TestReferenceCodecRejectsIncompleteAttachmentProvenance verifies that an
// attachment reference cannot omit the parent message needed to route it.
func TestReferenceCodecRejectsIncompleteAttachmentProvenance(t *testing.T) {
	t.Parallel()

	key, err := LoadOrCreateSigningKey(filepath.Join(t.TempDir(), "reference.key"))
	if err != nil {
		t.Fatalf("LoadOrCreateSigningKey() error = %v", err)
	}
	claims := validReferenceClaims()
	claims.ItemKind = ItemKindAttachment
	claims.GraphIDChain = []GraphID{{Kind: ItemKindAttachment, ID: "attachment-id"}}
	if _, err := NewReferenceCodec(key).Sign(claims); err == nil {
		t.Fatal("Sign() error = nil, want missing-parent-message error")
	}
}

// TestReferenceCodecRejectsUnrelatedGraphProvenance verifies that references
// cannot carry arbitrary prefixes unrelated to the addressed item.
func TestReferenceCodecRejectsUnrelatedGraphProvenance(t *testing.T) {
	t.Parallel()

	key, err := LoadOrCreateSigningKey(filepath.Join(t.TempDir(), "reference.key"))
	if err != nil {
		t.Fatalf("LoadOrCreateSigningKey() error = %v", err)
	}
	claims := validReferenceClaims()
	claims.GraphIDChain = []GraphID{
		{Kind: ItemKindCalendar, ID: "unrelated-calendar"},
		{Kind: ItemKindMessage, ID: "message-id"},
	}
	if _, err := NewReferenceCodec(key).Sign(claims); err == nil {
		t.Fatal("Sign() error = nil, want unrelated-prefix error")
	}
}

// validReferenceClaims returns fixed, independently known provenance for tests.
func validReferenceClaims() ReferenceClaims {
	return ReferenceClaims{
		AccountID:    auth.AccountID("11111111-1111-4111-8111-111111111111"),
		ResourceID:   "resource-42",
		ResourceKind: ResourceKindMailbox,
		MailboxView:  MailboxViewOwner,
		ItemKind:     ItemKindMessage,
		GraphIDChain: []GraphID{{Kind: ItemKindMessage, ID: "message-id"}},
	}
}
