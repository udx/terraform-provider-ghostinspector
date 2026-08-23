package gi

import (
	"encoding/json"
	"reflect"
	"sort"
)

// Step normalization shared by Read, plan comparison, and replacement
// payloads. The Ghost Inspector API returns step targets either as a plain
// selector string or as an array of selector candidates (objects with a
// `selector` field, or bare strings). Both shapes mean the same thing, so
// every consumer works with the canonical form produced here:
//
//	command:      string
//	targets:      []string (nil when absent)
//	value:        string (omitted when empty)
//	condition:    json.RawMessage (omitted when empty)
//	optional:     bool
//	variableName: string (omitted when empty)
//	notes:        string (omitted when empty)
//	private:      bool

// CanonicalStep is the normalized form of one Ghost Inspector step.
type CanonicalStep struct {
	Command      string          `json:"command"`
	Targets      []string        `json:"targets,omitempty"`
	Value        *string         `json:"value,omitempty"`
	Condition    json.RawMessage `json:"condition,omitempty"`
	Optional     bool            `json:"optional,omitempty"`
	VariableName *string         `json:"variableName,omitempty"`
	Notes        *string         `json:"notes,omitempty"`
	Private      bool            `json:"private,omitempty"`
}

// empty reports whether a value counts as empty under Ghost Inspector
// conventions (missing, "", or []).
func empty(v interface{}) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case []interface{}:
		return len(t) == 0
	case json.RawMessage:
		return len(t) == 0 || string(t) == `""` || string(t) == "[]" || string(t) == "null"
	}
	return false
}

// extractTargets converts any API target shape into a selector list.
func extractTargets(v interface{}) []string {
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		if s == "" {
			return nil
		}
		return []string{s}
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		if m, ok := v.(map[string]interface{}); ok {
			if s, ok := m["selector"].(string); ok {
				return []string{s}
			}
			if raw, err := json.Marshal(m); err == nil {
				return []string{string(raw)}
			}
		}
		return nil
	}
	if rv.Len() == 0 {
		return nil
	}
	out := make([]string, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		e := rv.Index(i).Interface()
		switch sel := e.(type) {
		case string:
			out = append(out, sel)
		case map[string]interface{}:
			if s, ok := sel["selector"].(string); ok {
				out = append(out, s)
			} else if raw, err := json.Marshal(sel); err == nil {
				out = append(out, string(raw))
			}
		default:
			if raw, err := json.Marshal(e); err == nil {
				out = append(out, string(raw))
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func strPtr(v interface{}) *string {
	if empty(v) {
		return nil
	}
	if s, ok := v.(string); ok {
		return &s
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	s := string(raw)
	return &s
}

// CanonicalizeStep converts one raw API step into canonical form.
func CanonicalizeStep(raw map[string]interface{}) CanonicalStep {
	out := CanonicalStep{}
	if c, ok := raw["command"].(string); ok {
		out.Command = c
	}
	out.Targets = extractTargets(raw["target"])
	out.Value = strPtr(raw["value"])
	if !empty(raw["condition"]) {
		if compact, err := compactJSON(raw["condition"]); err == nil {
			out.Condition = compact
		}
	}
	if b, ok := raw["optional"].(bool); ok {
		out.Optional = b
	}
	out.VariableName = strPtr(raw["variableName"])
	out.Notes = strPtr(raw["notes"])
	if b, ok := raw["private"].(bool); ok {
		out.Private = b
	}
	return out
}

// CanonicalizeSteps normalizes a full step list.
func CanonicalizeSteps(raw []map[string]interface{}) []CanonicalStep {
	out := make([]CanonicalStep, 0, len(raw))
	for _, s := range raw {
		out = append(out, CanonicalizeStep(s))
	}
	return out
}

// compactJSON re-encodes a decoded JSON value with sorted keys (encoding/json
// sorts map keys on marshal) and no insignificant whitespace.
func compactJSON(v interface{}) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

// StepsEqual compares two raw step lists semantically.
func StepsEqual(a, b []map[string]interface{}) bool {
	ca := CanonicalizeSteps(a)
	cb := CanonicalizeSteps(b)
	ja, erra := json.Marshal(ca)
	jb, errb := json.Marshal(cb)
	if erra != nil || errb != nil {
		return false
	}
	return string(ja) == string(jb)
}

// APIStep renders a canonical step back into the shape the import endpoint
// accepts: target as a bare string when there is a single selector, or as an
// array of {selector} objects when there are several candidates.
func (s CanonicalStep) APIStep() map[string]interface{} {
	out := map[string]interface{}{"command": s.Command}
	switch len(s.Targets) {
	case 0:
	case 1:
		out["target"] = s.Targets[0]
	default:
		candidates := make([]map[string]interface{}, 0, len(s.Targets))
		for _, t := range s.Targets {
			candidates = append(candidates, map[string]interface{}{"selector": t})
		}
		out["target"] = candidates
	}
	if s.Value != nil {
		out["value"] = *s.Value
	}
	if len(s.Condition) > 0 {
		var cond interface{}
		if err := json.Unmarshal(s.Condition, &cond); err == nil {
			out["condition"] = cond
		}
	}
	if s.Optional {
		out["optional"] = true
	}
	if s.VariableName != nil {
		out["variableName"] = *s.VariableName
	}
	if s.Notes != nil {
		out["notes"] = *s.Notes
	}
	if s.Private {
		out["private"] = true
	}
	return out
}

// APISteps renders a canonical list for import payloads.
func APISteps(steps []CanonicalStep) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.APIStep())
	}
	return out
}

// SortKeys is a helper for tests: returns sorted keys of a map.
func SortKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
