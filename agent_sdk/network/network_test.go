// Golden routing/activation fixtures exported from Python (fixtures_production,
// fixtures_routing): Propagate must reproduce Python's activated-lobe lists and
// resolved path for the production network at default weights.
package network

import (
	_ "embed"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/nccasia/agent-sdk-go/agent_sdk/core/activate"
	"github.com/nccasia/agent-sdk-go/agent_sdk/core/feature"
)

//go:embed fixtures_production.json
var fixturesProductionJSON []byte

//go:embed fixtures_routing.json
var fixturesRoutingJSON []byte

type productionFixture struct {
	Name      string         `json:"name"`
	Ctx       map[string]any `json:"ctx"`
	Activated []string       `json:"activated"`
	PathName  string         `json:"path_name"`
}

type routingFixture struct {
	Name   string             `json:"name"`
	Ctx    map[string]any     `json:"ctx"`
	Scores map[string]float64 `json:"scores"`
	Path   map[string]any     `json:"path"`
}

func TestProductionNetworkShape(t *testing.T) {
	if got := len(ProductionLobes()); got != 15 {
		t.Errorf("production lobes = %d, want 15", got)
	}
	if got := len(ProductionPaths()); got != 5 {
		t.Errorf("production paths = %d, want 5", got)
	}
	if got := len(ProductionFlows()); got != 6 {
		t.Errorf("production flows = %d, want 6", got)
	}
	if got := len(ProductionStages()); got != 8 {
		t.Errorf("production stages = %d, want 8", got)
	}
}

func TestPropagateReproducesGoldenActivation(t *testing.T) {
	var fixtures []productionFixture
	if err := json.Unmarshal(fixturesProductionJSON, &fixtures); err != nil {
		t.Fatalf("unmarshal fixtures: %v", err)
	}
	lobes := ProductionLobes()
	paths := ProductionPaths()
	weights := DefaultWeights()

	for _, fx := range fixtures {
		fx := fx
		t.Run(fx.Name, func(t *testing.T) {
			res, err := activate.Propagate(lobes, fx.Ctx, weights, activate.PropagateOptions{Paths: paths})
			if err != nil {
				t.Fatalf("propagate: %v", err)
			}
			if !reflect.DeepEqual(res.Activated, fx.Activated) {
				t.Errorf("activated\n got=%v\nwant=%v", res.Activated, fx.Activated)
			}
			if name, _ := res.Path["name"].(string); name != fx.PathName {
				t.Errorf("path name = %q, want %q", name, fx.PathName)
			}
		})
	}
}

func TestRecognizeReproducesGoldenRouting(t *testing.T) {
	var fixtures []routingFixture
	if err := json.Unmarshal(fixturesRoutingJSON, &fixtures); err != nil {
		t.Fatalf("unmarshal fixtures: %v", err)
	}
	paths := ProductionPaths()

	for _, fx := range fixtures {
		fx := fx
		t.Run(fx.Name, func(t *testing.T) {
			scores := feature.RecognizePaths(fx.Ctx, paths)
			for name, want := range fx.Scores {
				if got := scores[name]; got != want {
					t.Errorf("score[%s] = %v, want %v", name, got, want)
				}
			}
			gotPath := activate.ResolvePath(scores, paths)
			wantName, _ := fx.Path["name"].(string)
			if name, _ := gotPath["name"].(string); name != wantName {
				t.Errorf("resolved path = %q, want %q", name, wantName)
			}
		})
	}
}

func BenchmarkPropagateProduction(b *testing.B) {
	lobes := ProductionLobes()
	paths := ProductionPaths()
	weights := DefaultWeights()
	ctx := map[string]any{"query": "compare a versus b in detail for the team across many factors and dimensions", "stages": []any{"classify"}, "route": "complex"}
	opts := activate.PropagateOptions{Paths: paths}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = activate.Propagate(lobes, ctx, weights, opts)
	}
}

func BenchmarkRecognizePaths(b *testing.B) {
	paths := ProductionPaths()
	ctx := map[string]any{"query": "what about that thing?", "has_history": true, "prev_path": "qna"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = feature.RecognizePaths(ctx, paths)
	}
}
