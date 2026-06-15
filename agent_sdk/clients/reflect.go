package clients

import (
	"context"
	"reflect"
)

// callAnthropic invokes the anthropic SDK via reflection. The SDK is not
// imported at compile time (no third-party deps in the Go module); production
// wires the real client at startup via SetClient.
func callAnthropic(ctx context.Context, client any, model string, system any, messages []map[string]any, maxTokens int, temperature float64, tools []map[string]any) (any, error) {
	c := reflect.ValueOf(client)
	messagesField := c.MethodByName("Messages")
	if !messagesField.IsValid() {
		return nil, errNoAnthropicClient
	}
	create := messagesField.MethodByName("Create")
	if !create.IsValid() {
		return nil, errNoAnthropicClient
	}
	kwargs := map[string]any{
		"Model":       model,
		"System":      system,
		"Messages":    messages,
		"MaxTokens":   maxTokens,
		"Temperature": temperature,
	}
	if len(tools) > 0 {
		kwargs["Tools"] = tools
	}
	results := create.Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(kwargs)})
	if len(results) != 2 {
		return nil, errNoAnthropicClient
	}
	if e := results[1].Interface(); e != nil {
		if err, ok := e.(error); ok {
			return nil, err
		}
	}
	return results[0].Interface(), nil
}

// callOpenAI invokes the openai SDK via reflection (same pattern as anthropic).
func callOpenAI(ctx context.Context, client any, model string, messages []map[string]any, maxTokens int, temperature float64, tools []map[string]any) (any, error) {
	c := reflect.ValueOf(client)
	chats := c.MethodByName("Chat")
	if !chats.IsValid() {
		return nil, errNoOpenAIClient
	}
	completions := chats.MethodByName("Completions")
	if !completions.IsValid() {
		return nil, errNoOpenAIClient
	}
	create := completions.MethodByName("Create")
	if !create.IsValid() {
		return nil, errNoOpenAIClient
	}
	kwargs := map[string]any{
		"Model":       model,
		"Messages":    messages,
		"MaxTokens":   maxTokens,
		"Temperature": temperature,
	}
	if len(tools) > 0 {
		kwargs["Tools"] = tools
	}
	results := create.Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(kwargs)})
	if len(results) != 2 {
		return nil, errNoOpenAIClient
	}
	if e := results[1].Interface(); e != nil {
		if err, ok := e.(error); ok {
			return nil, err
		}
	}
	return results[0].Interface(), nil
}

// errNoAnthropicClient / errNoOpenAIClient are returned when the SDK is not
// wired (no real client injected). Tests use FakeClient instead.
var (
	errNoAnthropicClient = &noClientError{provider: "anthropic"}
	errNoOpenAIClient    = &noClientError{provider: "openai"}
)

type noClientError struct{ provider string }

func (e *noClientError) Error() string {
	return "clients: no " + e.provider + " provider wired; inject via SetClient or use FakeClient for tests"
}

// getField pulls a named field off an `any` via reflection; used for
// duck-typed provider responses (we do not import the SDKs).
func getField(obj any, name string) (any, bool) {
	if obj == nil {
		return nil, false
	}
	v := reflect.ValueOf(obj)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil, false
		}
		v = v.Elem()
	}
	f := v.FieldByName(name)
	if !f.IsValid() {
		return nil, false
	}
	return f.Interface(), true
}

// getInt coerces a field value to int.
func getInt(obj any, name string) (int, bool) {
	v, ok := getField(obj, name)
	if !ok {
		return 0, false
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(r.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int(r.Uint()), true
	case reflect.Float32, reflect.Float64:
		return int(r.Float()), true
	}
	return 0, false
}

// getString coerces a field value to string.
func getString(obj any, name string) (string, bool) {
	v, ok := getField(obj, name)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// reflectValue returns the underlying reflect.Value of an `any`, unwrapping
// pointer indirection. Returns a zero Value if `v` is nil.
func reflectValue(v any) reflect.Value {
	if v == nil {
		return reflect.Value{}
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return reflect.Value{}
		}
		rv = rv.Elem()
	}
	return rv
}

// reflectSliceKind reports whether a reflect.Value is a slice (kind == reflect.Slice).
func reflectSliceKind(v reflect.Value) bool { return v.Kind() == reflect.Slice }
