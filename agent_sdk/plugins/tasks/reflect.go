package tasks

import "reflect"

// structReadField is a minimal field reader (we avoid `reflect` where
// the caller can pass a map; this path is only for SimpleNamespace-style
// structs the tests use).
func structReadField(v any, field string) any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}
	// Try the lower-cased name (for `simpleNamespace` Python shim).
	if f := rv.FieldByName(field); f.IsValid() {
		return f.Interface()
	}
	if f := rv.FieldByName(capitalize(field)); f.IsValid() {
		return f.Interface()
	}
	return nil
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'a' && b[0] <= 'z' {
		b[0] = b[0] - 32
	}
	return string(b)
}
