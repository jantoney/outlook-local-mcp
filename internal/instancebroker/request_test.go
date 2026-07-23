package instancebroker

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestReplayDecisionReplaysOnlyCataloguedReads verifies protocol reads and
// annotated tool reads are replayable while writes and unknowns fail closed.
func TestReplayDecisionReplaysOnlyCataloguedReads(t *testing.T) {
	catalog := map[string]struct{}{"calendar.list_events": {}}
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{"protocol read", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, true},
		{"catalogued read", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"calendar","arguments":{"operation":"list_events"}}}`, true},
		{"write", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"calendar","arguments":{"operation":"create_event"}}}`, false},
		{"unknown", `{"jsonrpc":"2.0","id":4,"method":"future/method"}`, false},
		{"notification", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := replayDecision([]byte(test.raw), catalog); got != test.want {
				t.Fatalf("replayDecision() = %v, want %v", got, test.want)
			}
		})
	}
}

// TestRetryRequiredResponsePreservesID verifies ambiguous writes produce an
// actionable error without changing the harness request ID.
func TestRetryRequiredResponsePreservesID(t *testing.T) {
	response := retryRequiredResponse([]byte(`{"jsonrpc":"2.0","id":"write-7","method":"tools/call"}`))
	var decoded struct {
		ID    string `json:"id"`
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ID != "write-7" || decoded.Error.Code != -32071 || !strings.Contains(decoded.Error.Message, "not replayed") {
		t.Fatalf("unexpected retry response: %s", response)
	}
}

// TestRetryRequiredResponseIgnoresClientResponses verifies a failed reply to a
// broker-initiated ping or elicitation does not create a spurious harness error.
func TestRetryRequiredResponseIgnoresClientResponses(t *testing.T) {
	if response := retryRequiredResponse([]byte(`{"jsonrpc":"2.0","id":9,"result":{}}`)); response != nil {
		t.Fatalf("client response unexpectedly produced retry error: %s", response)
	}
}
