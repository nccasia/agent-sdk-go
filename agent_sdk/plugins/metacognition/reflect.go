package metacognition

import "reflect"

// structStringField is a tiny case-insensitive struct-field reader
// used by the meta_context lobe (tests pass SimpleNamespace-style
// structs).
func structStringField(v any, field string) string {
	if v == nil {
		return ""
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return ""
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return ""
	}
	if f := rv.FieldByName(field); f.IsValid() {
		if s, ok := f.Interface().(string); ok {
			return s
		}
	}
	return ""
}

// structReadField returns the value of a struct field by lower- or
// upper-case name. See tasks.reflect for the parallel helper.
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
	if f := rv.FieldByName(field); f.IsValid() {
		return f.Interface()
	}
	return nil
}
