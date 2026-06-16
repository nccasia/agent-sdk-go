// Package blocks — a flat re-export aggregation of the ported deterministic
// building blocks.
//
// This package gathers the framework primitives that were lifted from the
// reference SDK — contracts, the deterministic activation network, the
// lobe/flow frameworks + registries, metacognition, the ReAct funnel, skills,
// and inspection — under one import so downstream code (and the parity
// gateway) can reference them flatly, mirroring agent_sdk/_blocks.py's
// re-export role.
//
// Go has no runtime re-export, so the aggregation is expressed as type aliases
// + function value bindings: every name below resolves to the canonical
// definition in its owning package.
package blocks

import (
	"github.com/nccasia/agent-sdk-go/agent_sdk/contracts"
	"github.com/nccasia/agent-sdk-go/agent_sdk/core/activate"
	"github.com/nccasia/agent-sdk-go/agent_sdk/core/attention"
	"github.com/nccasia/agent-sdk-go/agent_sdk/core/feature"
	"github.com/nccasia/agent-sdk-go/agent_sdk/core/spec"
	"github.com/nccasia/agent-sdk-go/agent_sdk/flows"
	"github.com/nccasia/agent-sdk-go/agent_sdk/inspection"
	"github.com/nccasia/agent-sdk-go/agent_sdk/lobes"
	"github.com/nccasia/agent-sdk-go/agent_sdk/metacognition"
	"github.com/nccasia/agent-sdk-go/agent_sdk/react"
	"github.com/nccasia/agent-sdk-go/agent_sdk/skills"
)

// ── contracts (the dependency-free base) ────────────────────────────────────
type (
	LlmCall              = contracts.LlmCall
	LobeServices         = contracts.LobeServices
	TurnContext          = contracts.TurnContext
	PromptContribution   = contracts.PromptContribution
	LobeResult           = contracts.LobeResult
	StageResult          = contracts.StageResult
	ToolRuntime          = contracts.ToolRuntime
	CompositeToolRuntime = contracts.CompositeToolRuntime
	Citation             = contracts.Citation
	Claim                = contracts.Claim
	Memo                 = contracts.Memo
	FinalEnvelope        = contracts.FinalEnvelope
)

var (
	NewTurnContext          = contracts.NewTurnContext
	NewCompositeToolRuntime = contracts.NewCompositeToolRuntime
	IsPinned                = contracts.IsPinned
	StripMemoryFooter       = contracts.StripMemoryFooter
)

// ── activation network (pure, deterministic) ────────────────────────────────
type (
	LobeSpec          = spec.Lobe
	PathSpec          = spec.Path
	Blackboard        = attention.Blackboard
	ContextNode       = attention.ContextNode
	NetworkResolution = activate.NetworkResolution
)

var (
	Propagate        = activate.Propagate
	RecognizePaths   = feature.RecognizePaths
	ResolvePath      = activate.ResolvePath
	ValidateNetwork  = spec.ValidateNetwork
	MergeLobeWeights = activate.MergeLobeWeights
	BuildAttention   = attention.Build
)

// ── lobe framework ──────────────────────────────────────────────────────────
type (
	Lobe         = lobes.Lobe
	LobeRegistry = lobes.Registry
)

var ProductionPaths = lobes.ProductionPaths

// ── flow framework ──────────────────────────────────────────────────────────
type (
	Flow     = flows.Flow
	FlowStep = flows.FlowStep
)

var (
	NewFlow     = flows.NewFlow
	NewFlowStep = flows.NewFlowStep
)

// ── metacognition ───────────────────────────────────────────────────────────
type (
	MetaController  = metacognition.MetaController
	MetaDecision    = metacognition.MetaDecision
	MetaObservation = metacognition.MetaObservation
)

var (
	Monitor          = metacognition.Monitor
	CompileStatePlan = metacognition.CompileStatePlan
)

// ── react + skills ──────────────────────────────────────────────────────────
type (
	SkillRegistry = skills.SkillRegistry
	SkillPack     = skills.SkillPack
)

var (
	TierObservations      = react.TierObservations
	CompactObservations   = react.CompactObservations
	BuildSkillPromptBlock = skills.BuildPromptBlock
)

// ── inspection ──────────────────────────────────────────────────────────────
type (
	EngineSnapshot   = inspection.EngineSnapshot
	LobeAxisSnapshot = inspection.LobeAxisSnapshot
	FlowAxisSnapshot = inspection.FlowAxisSnapshot
	AxisOptimization = inspection.AxisOptimization
)

var SuggestAxisOptimizations = inspection.SuggestAxisOptimizations
