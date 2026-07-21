package tools

import (
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/resource"
)

// referenceCodec returns the optional signing authority supplied at startup.
func referenceCodec(codecs []*resource.ReferenceCodec) *resource.ReferenceCodec {
	if len(codecs) == 0 {
		return nil
	}
	return codecs[0]
}

// addSharedEventReferences signs each event result. Summary maps receive a
// direct resource_ref; raw maps remain unchanged and references are returned
// as a position-aligned sidecar.
func addSharedEventReferences(events []map[string]any, target calendarReadTarget, codec *resource.ReferenceCodec, raw bool) (any, error) {
	if !target.isShared() {
		return events, nil
	}
	if codec == nil {
		return nil, fmt.Errorf("shared resource reference signing is unavailable")
	}
	provenance := make([]map[string]any, 0, len(events))
	for index, event := range events {
		reference, err := signCalendarEvent(codec, target.target, eventString(event, "id"))
		if err != nil {
			return nil, err
		}
		if raw {
			provenance = append(provenance, map[string]any{"item_index": index, "resource_ref": reference})
		} else {
			event["resource_ref"] = reference
		}
	}
	if raw {
		return map[string]any{"data": events, "provenance": provenance}, nil
	}
	return events, nil
}

// addSharedEventReference signs one event result. Summary maps receive the
// reference directly; raw data is returned unchanged beside a provenance map.
func addSharedEventReference(event map[string]any, target calendarReadTarget, codec *resource.ReferenceCodec, raw bool) (any, error) {
	if !target.isShared() {
		return event, nil
	}
	if codec == nil {
		return nil, fmt.Errorf("shared resource reference signing is unavailable")
	}
	reference, err := signCalendarEvent(codec, target.target, eventString(event, "id"))
	if err != nil {
		return nil, err
	}
	if raw {
		return map[string]any{"data": event, "provenance": map[string]any{"resource_ref": reference}}, nil
	}
	event["resource_ref"] = reference
	return event, nil
}

// signCalendarEvent creates one target-bound event reference and rejects
// missing Graph IDs instead of returning incomplete provenance.
func signCalendarEvent(codec *resource.ReferenceCodec, target resource.Target, eventID string) (string, error) {
	if eventID == "" {
		return "", fmt.Errorf("shared calendar event is missing its Graph ID")
	}
	return codec.Sign(resource.ReferenceClaims{
		AccountID: target.AccountID, ResourceID: target.ResourceID,
		ResourceKind: target.Kind, MailboxView: target.View,
		ItemKind:     resource.ItemKindEvent,
		GraphIDChain: []resource.GraphID{{Kind: resource.ItemKindEvent, ID: eventID}},
	})
}

// eventString returns a string field from a serialized event map.
func eventString(event map[string]any, name string) string {
	value, _ := event[name].(string)
	return value
}
