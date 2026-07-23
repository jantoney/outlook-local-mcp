package instancebroker

import "encoding/json"

// requestEnvelope captures only the JSON-RPC fields required for routing and
// replay decisions while retaining the original message bytes unchanged.
type requestEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// toolCallParams captures the aggregate tool and operation identity.
type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// replayDecision reports whether raw is an explicitly replayable request. Tool
// calls require an exact catalog match; unknown protocol methods fail closed.
func replayDecision(raw []byte, catalog map[string]struct{}) bool {
	var message requestEnvelope
	if json.Unmarshal(raw, &message) != nil || len(message.ID) == 0 {
		return false
	}
	switch message.Method {
	case "ping", "tools/list", "resources/list", "resources/read", "resources/templates/list", "prompts/list", "prompts/get", "completion/complete":
		return true
	case "tools/call":
		var params toolCallParams
		if json.Unmarshal(message.Params, &params) != nil {
			return false
		}
		operation, ok := params.Arguments["operation"].(string)
		if !ok || operation == "" {
			return false
		}
		_, ok = catalog[params.Name+"."+operation]
		return ok
	default:
		return false
	}
}

// retryRequiredResponse creates a JSON-RPC error preserving the request ID. It
// tells an agent not to assume a write failed after an ambiguous disconnect.
func retryRequiredResponse(raw []byte) []byte {
	var message requestEnvelope
	if json.Unmarshal(raw, &message) != nil || len(message.ID) == 0 || message.Method == "" {
		return nil
	}
	response := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{JSONRPC: "2.0", ID: message.ID}
	response.Error.Code = -32071
	response.Error.Message = "The broker connection failed while this non-read operation was in flight. Its outcome is unknown, so it was not replayed. Inspect current state, then retry the operation if needed."
	encoded, _ := json.Marshal(response)
	return encoded
}
