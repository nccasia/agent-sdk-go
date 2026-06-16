package lobes

import (
	"fmt"
	"sort"

	"github.com/nccasia/agent-sdk-go/agent_sdk/core/spec"
)

// Layer constants re-exported for row defaults (the deprecated cognition layer
// is the registry-row default landing layer, mirroring LAYER_COGNITION).
const layerCognition = spec.LayerCognition

// Registry is the per-turn view of the lobe network (mirrors SkillRegistry).
// Defaults are the degenerate network; rows (maps) override or extend by id/name.
// FromRows/AddRow are the G6 seam: a new capability is a registry row with
// signals + edges + a receptive field + write-back kinds — never an interpreter
// branch. Every mutation re-validates the forward DAG and the pinned-edge
// protection (spec.ValidateNetwork). Ported from agent_sdk/lobes/registry.py.
type Registry struct {
	lobes map[string]spec.Lobe
	paths map[string]spec.Path
}

// NewRegistry builds a registry over the given lobes/paths (nil ⇒ empty
// degenerate network for standalone SDK use). It validates the network and
// panics on a malformed default network (matching Python's import-time validation).
func NewRegistry(lobes []spec.Lobe, paths []spec.Path) (*Registry, error) {
	r := &Registry{
		lobes: make(map[string]spec.Lobe, len(lobes)),
		paths: make(map[string]spec.Path, len(paths)),
	}
	for _, l := range lobes {
		r.lobes[l.ID] = l
	}
	for _, p := range paths {
		r.paths[p.Name] = p
	}
	if err := spec.ValidateNetwork(r.Lobes()); err != nil {
		return nil, err
	}
	return r, nil
}

// FromRows builds a registry over the default network then applies declarative
// lobe/path rows. Ported from LobeRegistry.from_rows.
func FromRows(lobeRows, pathRows []map[string]any) (*Registry, error) {
	r, err := NewRegistry(nil, nil)
	if err != nil {
		return nil, err
	}
	for _, row := range lobeRows {
		if _, err := r.AddRow(row); err != nil {
			return nil, err
		}
	}
	for _, row := range pathRows {
		if _, err := r.AddPathRow(row); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Lobes returns the lobe specs sorted by (layer, order, id) — execution order.
func (r *Registry) Lobes() []spec.Lobe {
	out := make([]spec.Lobe, 0, len(r.lobes))
	for _, l := range r.lobes {
		out = append(out, l)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Layer != b.Layer {
			return a.Layer < b.Layer
		}
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		return a.ID < b.ID
	})
	return out
}

// Paths returns the registered paths (registration order is not guaranteed; the
// activation core does not depend on path order).
func (r *Registry) Paths() []spec.Path {
	out := make([]spec.Path, 0, len(r.paths))
	names := make([]string, 0, len(r.paths))
	for n := range r.paths {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		out = append(out, r.paths[n])
	}
	return out
}

// Get returns a lobe spec by id (ok=false when absent).
func (r *Registry) Get(id string) (spec.Lobe, bool) {
	l, ok := r.lobes[id]
	return l, ok
}

// GetPath returns a path by name (ok=false when absent).
func (r *Registry) GetPath(name string) (spec.Path, bool) {
	p, ok := r.paths[name]
	return p, ok
}

// Register adds/overrides a lobe spec and re-validates the network.
func (r *Registry) Register(l spec.Lobe) error {
	prev, had := r.lobes[l.ID]
	r.lobes[l.ID] = l
	if err := spec.ValidateNetwork(r.Lobes()); err != nil {
		if had {
			r.lobes[l.ID] = prev
		} else {
			delete(r.lobes, l.ID)
		}
		return err
	}
	return nil
}

// RegisterPath adds/overrides a path (no DAG validation — paths bias, never gate).
func (r *Registry) RegisterPath(p spec.Path) { r.paths[p.Name] = p }

// Remove drops a lobe by id and re-validates the network.
func (r *Registry) Remove(id string) error {
	delete(r.lobes, id)
	return spec.ValidateNetwork(r.Lobes())
}

// RemovePath drops a path by name.
func (r *Registry) RemovePath(name string) { delete(r.paths, name) }

// AddRow registers a lobe from a declarative registry row (no code). Ported from
// LobeRegistry.add_row. Defaults: behavior "custom", layer cognition, order 99,
// min_activation 0.5.
func (r *Registry) AddRow(row map[string]any) (spec.Lobe, error) {
	id, _ := row["id"].(string)
	if id == "" {
		return spec.Lobe{}, fmt.Errorf("lobe row missing id")
	}
	l := spec.Lobe{
		ID:            id,
		Behavior:      strOr(row["behavior"], "custom"),
		Layer:         intOr(row["layer"], layerCognition),
		Order:         intOr(row["order"], 99),
		Prior:         floatOr(row["prior"], 0.0),
		Pinned:        boolOr(row["pinned"], false),
		Signals:       CompileRowSignals(asMap(row["signals"])),
		SignalWeights: floatMap(row["signal_weights"]),
		Edges:         floatMap(row["edges"]),
		Writes:        strSlice(row["writes"]),
		MinActivation: floatOr(row["min_activation"], 0.5),
		Attends:       attendsFromRow(asMap(row["attends"])),
	}
	if err := r.Register(l); err != nil {
		return spec.Lobe{}, err
	}
	return l, nil
}

// AddPathRow registers a named path from a declarative registry row — promotion
// of a recurring emergent shape is exactly this call plus defaults.
func (r *Registry) AddPathRow(row map[string]any) (spec.Path, error) {
	name, _ := row["name"].(string)
	if name == "" {
		return spec.Path{}, fmt.Errorf("path row missing name")
	}
	p := spec.Path{
		Name:       name,
		Members:    strSlice(row["members"]),
		Recognizer: CompileRowRecognizer(asMap(row["recognizer"])),
		Bias:       floatMap(row["bias"]),
		Threshold:  floatOr(row["threshold"], 0.5),
	}
	r.RegisterPath(p)
	return p, nil
}

func attendsFromRow(m map[string]any) any {
	if len(m) == 0 {
		return nil
	}
	return ContextBound{
		Kinds:         strSlice(m["kinds"]),
		Scopes:        strSlice(m["scopes"]),
		BudgetTokens:  intOr(m["budget_tokens"], 1600),
		MinActivation: floatOr(m["min_activation"], 0.22),
	}
}

// ── row coercion helpers ──────────────────────────────────────────────────────

func strOr(v any, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}

func intOr(v any, def int) int {
	switch x := v.(type) {
	case int:
		return x
	case float64:
		return int(x)
	}
	return def
}

func floatOr(v any, def float64) float64 {
	if f, ok := asFloat(v); ok {
		return f
	}
	return def
}

func boolOr(v any, def bool) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return def
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func floatMap(v any) map[string]float64 {
	m := asMap(v)
	out := make(map[string]float64, len(m))
	for k, val := range m {
		if f, ok := asFloat(val); ok {
			out[k] = f
		}
	}
	return out
}

func strSlice(v any) []string {
	switch x := v.(type) {
	case []string:
		return append([]string(nil), x...)
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
