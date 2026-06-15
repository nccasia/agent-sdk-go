package activable

import "testing"

// stubBlock is a minimal Activable used to lock the interface shape.
type stubBlock struct {
	id, name, desc, useWhen string
	sig                     float64
}

func (s stubBlock) ID() string               { return s.id }
func (s stubBlock) Name() string             { return s.name }
func (s stubBlock) Description() string      { return s.desc }
func (s stubBlock) UseWhen() string          { return s.useWhen }
func (s stubBlock) Signal(_ Context) float64 { return s.sig }

func TestLayerValuesMatchActivationCore(t *testing.T) {
	cases := []struct {
		layer Layer
		want  int
	}{
		{LayerInstinct, 0},
		{LayerPerception, 1},
		{LayerMemory, 2},
		{LayerSkill, 3},
		{LayerCognition, 4},
		{LayerExpression, 5},
	}
	for _, c := range cases {
		if int(c.layer) != c.want {
			t.Errorf("layer %v = %d, want %d", c.layer, int(c.layer), c.want)
		}
	}
}

func TestActivableInterface(t *testing.T) {
	var a Activable = stubBlock{id: "synthesize", name: "Synthesize", sig: 1.0}
	if a.ID() != "synthesize" {
		t.Errorf("ID = %q", a.ID())
	}
	if a.Signal(Context{}) != 1.0 {
		t.Errorf("Signal = %v", a.Signal(Context{}))
	}
}
