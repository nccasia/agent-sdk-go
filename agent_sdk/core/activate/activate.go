// Package activate is the lobe-network propagation core — layered reasoning
// with context-driven activation. Ported from the propagation half of
// agent_sdk/network/activation.py.
//
// Activation:  a_j = prior_j + Σ_k w_k·signal_k(ctx) + Σ_i edge_{i→j}·a_i
//   - Σ_p path_bias_{p→j}·score_p
//
// where the edge sum ranges ONLY over upstream lobes that activated AND
// completed (no speculative cascade). Pure and deterministic.
package activate

import (
	"sort"
	"strings"

	"github.com/mezon/agent-sdk-go/agent_sdk/core/feature"
	"github.com/mezon/agent-sdk-go/agent_sdk/core/signal"
	"github.com/mezon/agent-sdk-go/agent_sdk/core/spec"
)

// MergeLobeWeights returns defaults with a sparse per-bot override applied —
// same semantics as the node-weight surface; only numeric overrides are kept.
func MergeLobeWeights(defaults, overrides map[string]float64) map[string]float64 {
	merged := make(map[string]float64, len(defaults))
	for k, v := range defaults {
		merged[k] = v
	}
	for k, v := range overrides {
		merged[k] = v
	}
	return merged
}

// LobeEntry is one candidate lobe's resolution record (the trace.lobes shape).
type LobeEntry struct {
	ID         string             `json:"id"`
	Layer      int                `json:"layer"`
	Behavior   string             `json:"behavior"`
	Signals    map[string]float64 `json:"signals"`
	InEdges    map[string]float64 `json:"in_edges"`
	Activation float64            `json:"activation"`
	Activated  bool               `json:"activated"`
	Pinned     bool               `json:"pinned"`
	Reason     string             `json:"reason"`
}

// NetworkResolution is the resolved network for one turn — trace-shaped.
type NetworkResolution struct {
	Lobes     []LobeEntry // one entry per candidate lobe
	Path      map[string]any
	Activated []string // activated lobe ids in execution order
}

// ByID returns the per-id lookup over the resolved lobe entries.
func (r NetworkResolution) ByID() map[string]LobeEntry {
	out := make(map[string]LobeEntry, len(r.Lobes))
	for _, e := range r.Lobes {
		out[e.ID] = e
	}
	return out
}

// ResolvePath turns recognition scores into the turn's resolved reasoning path
// (the trace.path shape). name is "emergent" when no named path cleared its
// recognition threshold.
func ResolvePath(scores map[string]float64, paths []spec.Path) map[string]any {
	thresholds := make(map[string]float64, len(paths))
	for _, p := range paths {
		thresholds[p.Name] = p.Threshold
	}
	// ranked: sort by (-score, name).
	type kv struct {
		name  string
		score float64
	}
	ranked := make([]kv, 0, len(scores))
	for n, s := range scores {
		ranked = append(ranked, kv{n, s})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].name < ranked[j].name
	})

	cleared := []kv{}
	for _, r := range ranked {
		th, ok := thresholds[r.name]
		if !ok {
			th = 0.5
		}
		if r.score >= th {
			cleared = append(cleared, r)
		}
	}

	if len(cleared) > 0 {
		win := cleared[0]
		var runnerUp map[string]any
		for _, r := range ranked {
			if r.name != win.name {
				runnerUp = map[string]any{"name": r.name, "score": r.score}
				break
			}
		}
		return map[string]any{"name": win.name, "score": win.score, "runner_up": runnerUp, "emergent": false}
	}
	var runnerUp map[string]any
	if len(ranked) > 0 {
		runnerUp = map[string]any{"name": ranked[0].name, "score": ranked[0].score}
	}
	return map[string]any{"name": "emergent", "score": 0.0, "runner_up": runnerUp, "emergent": true}
}

// PropagateOptions configures one propagation.
type PropagateOptions struct {
	Paths         []spec.Path
	MinActivation float64             // policy-level floor
	Failed        map[string]struct{} // lobes that activated but did not complete
}

// Propagate resolves the turn's activated subgraph. Pure and deterministic.
//
// ctx is the B1 signal substrate; weights is the merged flat surface; opts
// carries the paths, policy floor, and failed-lobe set. Returns an error if the
// network is malformed (forward-DAG / pinned violations).
func Propagate(lobes []spec.Lobe, ctx map[string]any, weights map[string]float64, opts PropagateOptions) (NetworkResolution, error) {
	if err := spec.ValidateNetwork(lobes); err != nil {
		return NetworkResolution{}, err
	}
	failed := opts.Failed
	if failed == nil {
		failed = map[string]struct{}{}
	}
	paths := opts.Paths

	pathScores := feature.RecognizePaths(ctx, paths)
	pathTrace := ResolvePath(pathScores, paths)

	// The winning path's grounding flag, gating the output-contract lobes.
	pathGrounds := false
	if winName, ok := pathTrace["name"].(string); ok {
		for _, p := range paths {
			if p.Name == winName {
				pathGrounds = p.Grounds
				break
			}
		}
	}

	// Biases from every RECOGNIZED path (score >= its threshold).
	bias := map[string]float64{}
	biasReason := map[string]string{}
	for _, p := range paths {
		score := pathScores[p.Name]
		if score < p.Threshold {
			continue
		}
		for _, member := range p.Members {
			def := p.Bias[member]
			w := weightOr(weights, "path_"+p.Name+"__"+member, def)
			if w == 0.0 {
				continue
			}
			contribution := w * score
			bias[member] += contribution
			if contribution > 0 {
				if _, exists := biasReason[member]; !exists {
					biasReason[member] = "path:" + p.Name
				}
			}
		}
	}

	activation := map[string]float64{}
	completed := map[string]bool{}
	entries := []LobeEntry{}
	activatedOrder := []string{}

	ordered := executionOrder(lobes)
	for _, lobe := range ordered {
		rawSignals := map[string]float64{}
		if lobe.Signals != nil {
			for k, v := range lobe.Signals(ctx) {
				rawSignals[k] = v
			}
		}
		signalSum := 0.0
		for name, value := range rawSignals {
			w := weightOr2(weights, "w_"+name, lobe.SignalWeights, name, 1.0)
			signalSum += w * value
		}

		inEdges := map[string]float64{}
		edgeSum := 0.0
		for _, src := range lobes {
			edge := weightOr(weights, "edge_"+src.ID+"__"+lobe.ID, src.Edges[lobe.ID])
			if edge == 0.0 {
				continue
			}
			if !completed[src.ID] {
				continue
			}
			contribution := edge * activation[src.ID]
			inEdges[src.ID] = signal.Round4(contribution)
			edgeSum += contribution
		}

		prior := weightOr(weights, "prior_"+lobe.ID, lobe.Prior)
		a := prior + signalSum + edgeSum + bias[lobe.ID]
		threshold := weightOr(weights, "min_"+lobe.ID, lobe.MinActivation)
		if opts.MinActivation > threshold {
			threshold = opts.MinActivation
		}

		var activated bool
		var reason string
		if _, isOutput := spec.OutputContractLobes[lobe.ID]; isOutput {
			activated = pathGrounds
			if pathGrounds {
				reason = "grounding_path"
			} else {
				reason = "non_grounding_path"
			}
		} else if lobe.Pinned {
			activated, reason = true, "pinned:invariant"
		} else if a >= threshold {
			activated = true
			reason = activationReason(rawSignals, inEdges, biasReason[lobe.ID])
		} else {
			activated, reason = false, "below_threshold"
		}

		if activated {
			activation[lobe.ID] = a
		} else {
			activation[lobe.ID] = 0.0
		}
		_, isFailed := failed[lobe.ID]
		completed[lobe.ID] = activated && !isFailed
		if activated {
			activatedOrder = append(activatedOrder, lobe.ID)
		}

		entryReason := reason
		if isFailed {
			entryReason = "failed"
		}
		entries = append(entries, LobeEntry{
			ID:         lobe.ID,
			Layer:      lobe.Layer,
			Behavior:   lobe.Behavior,
			Signals:    round4Map(rawSignals),
			InEdges:    inEdges,
			Activation: signal.Round4(a),
			Activated:  activated,
			Pinned:     lobe.Pinned,
			Reason:     entryReason,
		})
	}

	return NetworkResolution{Lobes: entries, Path: pathTrace, Activated: activatedOrder}, nil
}

func executionOrder(lobes []spec.Lobe) []spec.Lobe {
	out := make([]spec.Lobe, len(lobes))
	copy(out, lobes)
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

// activationReason yields a human-readable reason: own signals first, then path
// bias, then edges, then prior.
func activationReason(signals map[string]float64, inEdges map[string]float64, pathReason string) string {
	lit := []string{}
	for name, value := range signals {
		if value > 0 {
			lit = append(lit, name)
		}
	}
	if len(lit) > 0 {
		sort.Strings(lit)
		return strings.Join(lit, "+")
	}
	if pathReason != "" {
		return pathReason
	}
	if len(inEdges) > 0 {
		// strongest in-edge (ties -> first by name for determinism)
		var best string
		var bestVal float64
		names := make([]string, 0, len(inEdges))
		for k := range inEdges {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			if best == "" || inEdges[k] > bestVal {
				best, bestVal = k, inEdges[k]
			}
		}
		return "edge:" + best
	}
	return "prior"
}

func weightOr(w map[string]float64, key string, def float64) float64 {
	if v, ok := w[key]; ok {
		return v
	}
	return def
}

// weightOr2 resolves a signal weight: the flat surface key first, else the
// lobe's own signal_weights, else 1.0.
func weightOr2(w map[string]float64, flatKey string, sigWeights map[string]float64, name string, def float64) float64 {
	if v, ok := w[flatKey]; ok {
		return v
	}
	if v, ok := sigWeights[name]; ok {
		return v
	}
	return def
}

func round4Map(m map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(m))
	for k, v := range m {
		out[k] = signal.Round4(v)
	}
	return out
}
