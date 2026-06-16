package codingagent

import (
	"github.com/mezon/agent-sdk-go/agent_sdk/clients"
)

// Initial sandbox contents (the demo/test writes these to a temp dir). Mirror
// coding_agent.fakes.
const CalculatorPy = `"""A tiny calculator."""


def add(a, b):
    return a + b


def subtract(a, b):
    return a - b
`

const TestCalculatorPy = `from calculator import add, subtract


def test_add():
    assert add(2, 3) == 5


def test_subtract():
    assert subtract(5, 2) == 3
`

const subtractSrc = "def subtract(a, b):\n    return a - b\n"
const multiplySrc = "\n\ndef multiply(a, b):\n    return a * b\n"
const newTestSrc = `from calculator import multiply


def test_multiply():
    assert multiply(2, 4) == 8
`

// fakeCodingModel is a stateful handler for FakeClient — one realistic
// 'add multiply' session driving explore → plan → implement → verify →
// summarize. Mirrors coding_agent.fakes.FakeCodingModel.
type fakeCodingModel struct {
	hops map[string]int
}

func (m *fakeCodingModel) call(stage, _ string, _ []map[string]any, _ []map[string]any) any {
	n := m.hops[stage]
	m.hops[stage] = n + 1

	switch stage {
	case "explore":
		if n == 0 {
			return map[string]any{"text": "Let me see the layout.",
				"tools": []map[string]any{{"name": "LS", "input": map[string]any{"path": "."}}}}
		}
		if n == 1 {
			return map[string]any{"text": "Reading the calculator module.",
				"tools": []map[string]any{{"name": "Read", "input": map[string]any{"file_path": "calculator.py"}}}}
		}
		return "calculator.py defines add and subtract. I'll add multiply plus a test."
	case "plan":
		return "1. Add multiply(a, b) to calculator.py after subtract.\n" +
			"2. Add test_multiply in a new test_multiply.py."
	case "implement":
		if n == 0 {
			return map[string]any{"text": "Adding multiply().",
				"tools": []map[string]any{{"name": "Edit", "input": map[string]any{
					"file_path": "calculator.py", "old_string": subtractSrc,
					"new_string": subtractSrc + multiplySrc}}}}
		}
		if n == 1 {
			return map[string]any{"text": "Adding a test.",
				"tools": []map[string]any{{"name": "Write", "input": map[string]any{
					"file_path": "test_multiply.py", "content": newTestSrc}}}}
		}
		return "Implemented multiply() and added test_multiply.py."
	case "verify":
		if n == 0 {
			return map[string]any{"text": "Running the tests.",
				"tools": []map[string]any{{"name": "Bash", "input": map[string]any{"command": PytestCmd}}}}
		}
		return "The full test suite passes."
	}
	// summarize (single) and answer flow
	return "Added `multiply(a, b)` to calculator.py (after `subtract`) and a new " +
		"test_multiply.py. Ran the suite with pytest — all tests pass."
}

// MakeFakeClient builds a FakeClient wired to the fakeCodingModel handler.
func MakeFakeClient() *clients.FakeClient {
	m := &fakeCodingModel{hops: map[string]int{}}
	return clients.Scripted(m.call)
}

const archMD = `# Architecture — tiny calculator

## Overview
A minimal arithmetic library: pure functions over numbers, with a matching test
suite. No external dependencies.

## Subsystems
- **calculator.py** — the core operations (` + "`add`, `subtract`" + `). Each is a pure
  function of two numbers.
- **test_calculator.py** — the test suite covering each operation.

## How it fits together
Callers import the operation functions directly from ` + "`calculator`" + `. The tests
import the same functions and assert their results. There is no runtime wiring,
config, or state — the public surface is the operation functions.

## Entry points
` + "`from calculator import add, subtract`" + `
`

// fakeUnderstandModel drives the codebase-understanding flow (survey → plan →
// investigate → document). Mirrors coding_agent.fakes.FakeUnderstandModel.
type fakeUnderstandModel struct {
	hops map[string]int
}

func (m *fakeUnderstandModel) call(stage, _ string, _ []map[string]any, _ []map[string]any) any {
	n := m.hops[stage]
	m.hops[stage] = n + 1

	switch stage {
	case "survey":
		if n == 0 {
			return map[string]any{"text": "Mapping the repo.",
				"tools": []map[string]any{{"name": "LS", "input": map[string]any{"path": "."}}}}
		}
		if n == 1 {
			return map[string]any{"text": "Finding the Python files.",
				"tools": []map[string]any{{"name": "Glob", "input": map[string]any{"pattern": "**/*.py"}}}}
		}
		return "Structure: a flat package — calculator.py (the operations) and " +
			"test_calculator.py (the tests). One subsystem to study."
	case "plan":
		return "Plan: 1) study calculator.py (the operations). 2) note the test " +
			"coverage. Save each finding to memory, then write ARCHITECTURE.md."
	case "investigate":
		switch n {
		case 0:
			return map[string]any{"text": "Reading the operations.",
				"tools": []map[string]any{{"name": "Read", "input": map[string]any{"file_path": "calculator.py"}}}}
		case 1:
			return map[string]any{"text": "Noting the finding.",
				"tools": []map[string]any{{"name": "memory", "input": map[string]any{
					"action": "remember", "scope": "conversation",
					"key":   "finding:operations",
					"value": "calculator.py defines pure add(a,b) and subtract(a,b)."}}}}
		case 2:
			return map[string]any{"text": "Checking the tests.",
				"tools": []map[string]any{{"name": "Read", "input": map[string]any{"file_path": "test_calculator.py"}}}}
		case 3:
			return map[string]any{"text": "Noting the test finding.",
				"tools": []map[string]any{{"name": "memory", "input": map[string]any{
					"action": "remember", "scope": "conversation",
					"key":   "finding:tests",
					"value": "test_calculator.py covers add and subtract."}}}}
		}
		return "Investigated both files; findings saved to memory."
	case "document":
		if n == 0 {
			return map[string]any{"text": "Recalling all findings.",
				"tools": []map[string]any{{"name": "memory", "input": map[string]any{
					"action": "recall", "scope": "conversation", "query": "finding"}}}}
		}
		if n == 1 {
			return map[string]any{"text": "Writing the architecture document.",
				"tools": []map[string]any{{"name": "Write", "input": map[string]any{
					"file_path": "ARCHITECTURE.md", "content": archMD}}}}
		}
		return "Wrote ARCHITECTURE.md — an overview, the calculator.py + " +
			"test_calculator.py subsystems, how they fit, and the entry points."
	}
	return "Done."
}

// MakeUnderstandClient builds a FakeClient wired to the fakeUnderstandModel handler.
func MakeUnderstandClient() *clients.FakeClient {
	m := &fakeUnderstandModel{hops: map[string]int{}}
	return clients.Scripted(m.call)
}
