package pbai

import (
	"encoding/json"
	"strings"
)

// extractFormFill scans an assistant reply for a {"formFill":{...}} JSON
// object (optionally wrapped in prose or ```json fences) and returns the
// parsed field→value map with the reply text cleaned of that JSON block. It
// returns (nil, text) unchanged when no well-formed formFill object is found.
//
// The view-agent prompt asks the model to answer form requests with exactly
// such an object; extracting it server-side (instead of via a client regex)
// keeps the transcript free of raw JSON and lets the form UI take the
// structured path directly.
func extractFormFill(text string) (map[string]any, string) {
	idx := strings.Index(text, "formFill")
	if idx < 0 {
		return nil, text
	}
	// the JSON object we want is the one that directly contains "formFill" —
	// walk back from the key to its nearest opening brace.
	open := idx
	for open >= 0 && text[open] != '{' {
		open--
	}
	if open < 0 {
		return nil, text
	}
	obj := text[open:]
	depth, inStr, esc := 0, false, false
	closeIdx := -1
	for i := 0; i < len(obj); i++ {
		c := obj[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			if c == '\\' {
				esc = true
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				closeIdx = i
				i = len(obj)
			}
		}
	}
	if closeIdx < 0 {
		return nil, text
	}
	raw := obj[:closeIdx+1]
	var parsed struct {
		FormFill map[string]any `json:"formFill"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil || len(parsed.FormFill) == 0 {
		return nil, text
	}

	clean := text[:open] + text[open+closeIdx+1:]
	clean = strings.TrimSpace(strings.TrimPrefix(clean, "```json"))
	clean = strings.TrimSpace(strings.TrimPrefix(clean, "```"))
	clean = strings.TrimSpace(strings.TrimSuffix(clean, "```"))
	// collapse runs of whitespace left where the JSON block was removed
	clean = strings.Join(strings.Fields(clean), " ")
	return parsed.FormFill, clean
}