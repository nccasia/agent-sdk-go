package memory

import "encoding/json"

// Scratchpad caps mirror the Python bounds.
const (
	capKeys       = 64
	capValueChars = 8000
	capList       = 64
)

// Scratchpad is turn-scoped key→value flash memory. JSON-serializable values,
// bounded. Reset every turn.
type Scratchpad struct {
	data  map[string]any
	order []string
}

// NewScratchpad builds an empty scratchpad.
func NewScratchpad() *Scratchpad {
	return &Scratchpad{data: map[string]any{}}
}

func (s *Scratchpad) ensure() {
	if s.data == nil {
		s.data = map[string]any{}
	}
}

func over(value any) bool {
	b, err := json.Marshal(value)
	if err != nil {
		return true
	}
	return len(b) > capValueChars
}

func capValue(value any) any {
	encoded, err := json.Marshal(value)
	if err != nil {
		value = stringify(value)
		encoded, _ = json.Marshal(value)
	}
	if len(encoded) <= capValueChars {
		var out any
		_ = json.Unmarshal(encoded, &out)
		return out
	}
	switch v := value.(type) {
	case string:
		keep := capValueChars
		runes := []rune(v)
		head := keep * 3 / 4
		tail := keep / 4
		if head > len(runes) {
			head = len(runes)
		}
		if tail > len(runes) {
			tail = len(runes)
		}
		return string(runes[:head]) + "\n…[+" + itoaInt(len(runes)-keep) + " chars elided]…\n" + string(runes[len(runes)-tail:])
	case []any:
		capped := make([]any, 0, len(v))
		for _, item := range v {
			capped = append(capped, capValue(item))
		}
		for len(capped) > 1 && over(capped) {
			capped = capped[:len(capped)-1]
		}
		dropped := len(v) - len(capped)
		if dropped > 0 {
			capped = append(capped, map[string]any{"_elided": dropped})
		}
		return capped
	case map[string]any:
		capped := map[string]any{}
		for k, val := range v {
			capped[k] = capValue(val)
		}
		capped["_truncated"] = true
		return capped
	default:
		s := string(encoded)
		if len(s) > capValueChars {
			s = s[:capValueChars]
		}
		return map[string]any{"_truncated": true, "preview": s}
	}
}

// Set writes a value (bounded). At capacity it refuses NEW keys.
func (s *Scratchpad) Set(key string, value any) {
	s.ensure()
	if _, exists := s.data[key]; !exists {
		if len(s.data) >= capKeys {
			return
		}
		s.order = append(s.order, key)
	}
	s.data[key] = capValue(value)
}

// Get reads a value (default if absent).
func (s *Scratchpad) Get(key string, def any) any {
	s.ensure()
	if v, ok := s.data[key]; ok {
		return v
	}
	return def
}

// Delete drops a key. Returns whether it existed.
func (s *Scratchpad) Delete(key string) bool {
	s.ensure()
	if _, ok := s.data[key]; !ok {
		return false
	}
	delete(s.data, key)
	for i, k := range s.order {
		if k == key {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	return true
}

// Append appends to a list-valued key (creating it as a list).
func (s *Scratchpad) Append(key string, value any) {
	s.ensure()
	cur := s.data[key]
	var list []any
	switch c := cur.(type) {
	case nil:
		list = []any{}
	case []any:
		list = append([]any{}, c...)
	default:
		list = []any{c}
	}
	if len(list) < capList {
		list = append(list, capValue(value))
	}
	s.Set(key, list)
}

// Keys returns the keys in insertion order.
func (s *Scratchpad) Keys() []string {
	s.ensure()
	out := make([]string, 0, len(s.order))
	for _, k := range s.order {
		if _, ok := s.data[k]; ok {
			out = append(out, k)
		}
	}
	return out
}

// Has reports whether key is present.
func (s *Scratchpad) Has(key string) bool {
	s.ensure()
	_, ok := s.data[key]
	return ok
}

// Bool reports whether the scratchpad holds anything.
func (s *Scratchpad) Bool() bool {
	s.ensure()
	return len(s.data) > 0
}

// AsList reads a key as a list. A scalar becomes a one-item list; missing → [].
func (s *Scratchpad) AsList(key string) []any {
	s.ensure()
	v, ok := s.data[key]
	if !ok || v == nil {
		return []any{}
	}
	if list, ok := v.([]any); ok {
		return append([]any{}, list...)
	}
	return []any{v}
}

// Snapshot returns a JSON-safe deep copy for the trace.
func (s *Scratchpad) Snapshot() map[string]any {
	s.ensure()
	b, err := json.Marshal(s.data)
	if err != nil {
		out := map[string]any{}
		for k, v := range s.data {
			out[k] = stringify(v)
		}
		return out
	}
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}
