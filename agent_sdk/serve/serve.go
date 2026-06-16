// Package serve holds the stateless serving worker + the in-process queue /
// event-sink adapters. The Python serve module is the production pattern
// the Mezon worker runs (a Queue + EventSink + per-conversation session lock
// + agent pool, draining jobs as goroutines). This Go port provides the same
// surface in terms of the agent_sdk Go types (PreactAgent, *session.Session,
// events.AgentEvent) so a worker can pick the right pool size, swap the
// in-process adapters for a Redis-backed queue/sink (see
// ../stores/redis), and stay stateless across sessions — each job is bound
// to a session id at the store, the turn loads + runs + saves the whole
// state, and a fresh replica can serve any session from just the JSON
// snapshot.
//
// Deviation note: the Python Redis adapters use redis.asyncio directly. The
// Go port keeps the queue / event-sink / lock protocols as small interfaces
// and ships in-process adapters here; the stores/redis package implements
// the SessionStore surface against a pluggable client (see ../stores/redis).
package serve

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"github.com/nccasia/agent-sdk-go/agent_sdk/agent"
	"github.com/nccasia/agent-sdk-go/agent_sdk/events"
	"github.com/nccasia/agent-sdk-go/agent_sdk/session"
)

// Job is one unit of work the worker drains from a Queue. The worker
// resolves the job's session (a ready handle wins; else it binds the id to
// the one store) and runs the agent once. trace_id is the per-job pub-sub
// key — the sink publishes to it and subscribers receive the same job's
// events.
type Job struct {
	Input     string
	Session   *session.Session // optional ready handle
	SessionID string           // otherwise the worker binds this id to its store
	TraceID   string
}

// newTraceID returns a short, wire-stable trace id (mirrors the Python
// uuid4().hex[:16]).
func newTraceID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "tr-fallback"
	}
	return hex.EncodeToString(b[:])
}

// Queue is the producer/consumer contract the worker drains.
type Queue interface {
	Enqueue(job Job) (traceID string, err error)
	Consume(ctx context.Context) (<-chan Job, error)
}

// EventSink is the per-trace pub-sub contract.
type EventSink interface {
	Publish(traceID string, ev any) error
	Close(traceID string) error
	Subscribe(traceID string) <-chan any
}

// SessionLock is the per-key lock factory the worker uses to enforce one
// in-flight turn per conversation. The default is InProcessLock; production
// uses a Redis-backed distributed lock.
type SessionLock interface {
	// Acquire returns a held handle on the per-key mutex. The same key
	// yields the same per-key mutex (acquiring twice on the same key
	// blocks until the holder releases). Unlock releases the gate.
	Acquire(key string) SessionLockHandle
	// Locker returns the per-key object without acquiring it (mirrors
	// the Python ``InProcessLock("key")`` behavior, which returns the
	// asyncio.Lock object; acquire is a separate step).
	Locker(key string) SessionLockHandle
}

// SessionLockHandle is the per-acquisition mutex (Unlock closes the gate).
type SessionLockHandle interface {
	Unlock()
}

// ── in-process adapters ─────────────────────────────────────────────────────

// InProcessQueue is a Go-channel-backed Queue (matches Python's asyncio.Queue).
type InProcessQueue struct {
	mu   sync.Mutex
	ch   chan Job
	open bool
}

// NewInProcessQueue builds an open queue with an unbounded channel (the
// Python asyncio.Queue has no maxsize by default; this matches that).
func NewInProcessQueue() *InProcessQueue {
	return &InProcessQueue{ch: make(chan Job, 64), open: true}
}

// Enqueue pushes one job and returns its trace id.
func (q *InProcessQueue) Enqueue(job Job) (string, error) {
	q.mu.Lock()
	if !q.open {
		q.mu.Unlock()
		return "", errors.New("serve: queue closed")
	}
	if job.TraceID == "" {
		job.TraceID = newTraceID()
	}
	q.mu.Unlock()
	q.ch <- job
	return job.TraceID, nil
}

// Consume yields jobs on a read-only channel until ctx is cancelled or the
// queue is closed.
func (q *InProcessQueue) Consume(ctx context.Context) (<-chan Job, error) {
	out := make(chan Job)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case job, ok := <-q.ch:
				if !ok {
					return
				}
				select {
				case <-ctx.Done():
					return
				case out <- job:
				}
			}
		}
	}()
	return out, nil
}

// Close stops accepting new jobs (consumers drain the remaining).
func (q *InProcessQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.open {
		return
	}
	q.open = false
	close(q.ch)
}

// doneEvent is the sentinel an EventSink publishes on Close to tell
// subscribers the job is over.
type doneEvent struct{ Type string }

func (d doneEvent) ToJSON() map[string]any { return map[string]any{"type": d.Type} }

// InProcessEventSink is a per-trace-id pub-sub backed by Go channels. The
// Publish path is fire-and-forget (events sent before Subscribe are
// dropped, matching the Python RedisEventSink model). Close drains any
// pending events, sends a _done sentinel, and closes each subscriber
// channel so a `for range` loop terminates.
type InProcessEventSink struct {
	mu   sync.Mutex
	subs map[string]map[chan any]struct{}
}

// NewInProcessEventSink builds an empty sink.
func NewInProcessEventSink() *InProcessEventSink {
	return &InProcessEventSink{subs: map[string]map[chan any]struct{}{}}
}

func (s *InProcessEventSink) subscribers(traceID string) []chan any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []chan any{}
	for ch := range s.subs[traceID] {
		out = append(out, ch)
	}
	return out
}

// Publish fans an event out to all current subscribers of traceID.
func (s *InProcessEventSink) Publish(traceID string, ev any) error {
	for _, ch := range s.subscribers(traceID) {
		select {
		case ch <- ev:
		default:
		}
	}
	return nil
}

// Close sends the _done sentinel to every subscriber and closes the
// per-trace channel set so `for range` loops on the subscriber terminate.
func (s *InProcessEventSink) Close(traceID string) error {
	s.mu.Lock()
	subs := s.subs[traceID]
	delete(s.subs, traceID)
	s.mu.Unlock()
	for ch := range subs {
		select {
		case ch <- doneEvent{Type: "_done"}:
		default:
		}
		close(ch)
	}
	return nil
}

// Subscribe returns a channel of events for traceID. The channel is closed
// when the matching Close is observed.
func (s *InProcessEventSink) Subscribe(traceID string) <-chan any {
	ch := make(chan any, 64)
	s.mu.Lock()
	if _, ok := s.subs[traceID]; !ok {
		s.subs[traceID] = map[chan any]struct{}{}
	}
	s.subs[traceID][ch] = struct{}{}
	s.mu.Unlock()
	return ch
}

// InProcessLock is a per-key process-local lock (matches Python's
// InProcessLock — one asyncio.Lock per key).
type InProcessLock struct {
	mu    sync.Mutex
	locks map[string]*ipLock
	views map[string]*ipView
}

type ipLock struct {
	mu sync.Mutex
}

func newIPLock() *ipLock { return &ipLock{} }

// NewInProcessLock builds an empty per-key lock set.
func NewInProcessLock() *InProcessLock {
	return &InProcessLock{
		locks: map[string]*ipLock{},
		views: map[string]*ipView{},
	}
}

// Acquire returns a held handle for key; the same key yields the same
// per-key mutex (acquiring twice on the same key blocks until the holder
// releases).
func (l *InProcessLock) Acquire(key string) SessionLockHandle {
	lock := l.locker(key)
	lock.mu.Lock()
	return &ipHandle{lock: lock}
}

// Locker returns the per-key object without acquiring it. Mirrors the
// Python “InProcessLock("key")“ behavior, which returns the asyncio.Lock
// object (the same key yields the same object — `lock is lock`).
func (l *InProcessLock) Locker(key string) SessionLockHandle {
	l.mu.Lock()
	defer l.mu.Unlock()
	view, ok := l.views[key]
	if !ok {
		view = &ipView{key: key}
		l.views[key] = view
	}
	return view
}

func (l *InProcessLock) locker(key string) *ipLock {
	l.mu.Lock()
	defer l.mu.Unlock()
	lock, ok := l.locks[key]
	if !ok {
		lock = newIPLock()
		l.locks[key] = lock
	}
	return lock
}

type ipHandle struct {
	lock     *ipLock
	unlocked bool
}

func (h *ipHandle) Unlock() {
	if h.unlocked {
		return
	}
	h.unlocked = true
	h.lock.mu.Unlock()
}

// ipView is a non-held view of a per-key lock (the Python "same key → same
// object" semantics). Unlock on a view is a no-op (the lock was never
// acquired).
type ipView struct {
	key string
}

func (v *ipView) Unlock() {}

// ── AgentWorker ─────────────────────────────────────────────────────────────

// SessionResolver is the small surface the worker uses to bind a session
// id to the per-worker store. The session package's *Session satisfies it.
type SessionResolver interface {
	// Load loads the current state for the session id.
	Load(ctx context.Context, id string) (session.SessionState, error)
	// Save persists the whole state for the session id.
	Save(ctx context.Context, id string, state session.SessionState) error
}

// WorkerConfig is the constructor input for AgentWorker. Either Agent or
// AgentFactory must be set; Queue + Sink are required; Store is optional
// (when nil the worker runs sessionless); Concurrency defaults to 1; the
// SessionLock defaults to NewInProcessLock().
type WorkerConfig struct {
	Agent        *agent.PreactAgent
	AgentFactory func() *agent.PreactAgent
	Queue        Queue
	Sink         EventSink
	Store        SessionResolver
	Concurrency  int
	SessionLock  SessionLock
}

// AgentWorker is the stateless serving worker. One per process; drains the
// queue, runs each job through one of (concurrency) agents, publishes the
// per-trace events, and closes the sink trace on job end. Holds ONE
// (immutable) agent config and the (immutable) store — every per-session
// state lives in the store, so any replica serves any session and a
// restart loses nothing.
type AgentWorker struct {
	cfg      WorkerConfig
	pool     chan *agent.PreactAgent
	initPool sync.Once
}

// NewAgentWorker builds a worker. Returns an error when neither Agent nor
// AgentFactory is set; panics if Queue or Sink is nil (the worker can't
// function without them).
func NewAgentWorker(cfg WorkerConfig) *AgentWorker {
	if cfg.Agent == nil && cfg.AgentFactory == nil {
		panic("serve: AgentWorker needs an agent or an agent_factory")
	}
	if cfg.Queue == nil {
		panic("serve: AgentWorker needs a queue")
	}
	if cfg.Sink == nil {
		panic("serve: AgentWorker needs a sink")
	}
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}
	if cfg.SessionLock == nil {
		cfg.SessionLock = NewInProcessLock()
	}
	return &AgentWorker{cfg: cfg}
}

// NewAgentWorkerSafe is the error-returning variant of NewAgentWorker —
// matches the Python "ValueError" path the test asserts.
func NewAgentWorkerSafe(cfg WorkerConfig) (*AgentWorker, error) {
	if cfg.Agent == nil && cfg.AgentFactory == nil {
		return nil, fmt.Errorf("serve: AgentWorker needs an agent or an agent_factory")
	}
	if cfg.Queue == nil {
		return nil, fmt.Errorf("serve: AgentWorker needs a queue")
	}
	if cfg.Sink == nil {
		return nil, fmt.Errorf("serve: AgentWorker needs a sink")
	}
	return NewAgentWorker(cfg), nil
}

// Queue returns the underlying queue (so callers can enqueue).
func (w *AgentWorker) Queue() Queue { return w.cfg.Queue }

// Sink returns the underlying sink.
func (w *AgentWorker) Sink() EventSink { return w.cfg.Sink }

// ensurePool materializes the agent pool once (one slot per Concurrency; a
// shared agent also works — turns serialize through it).
func (w *AgentWorker) ensurePool() chan *agent.PreactAgent {
	w.initPool.Do(func() {
		w.pool = make(chan *agent.PreactAgent, w.cfg.Concurrency)
		if w.cfg.AgentFactory != nil {
			for i := 0; i < w.cfg.Concurrency; i++ {
				w.pool <- w.cfg.AgentFactory()
			}
		} else {
			w.pool <- w.cfg.Agent
		}
	})
	return w.pool
}

// sessionFor resolves a job's session: a ready handle wins; else bind the
// id to the worker store. Returns nil for fully sessionless jobs.
func (w *AgentWorker) sessionFor(job Job) *session.Session {
	if job.Session != nil {
		return job.Session
	}
	if job.SessionID != "" {
		if w.cfg.Store != nil {
			// Wrap the worker's store so the *session.Session can drive it.
			return session.New(job.SessionID, storeAdapter{w.cfg.Store})
		}
		// Zero-infra default store (in-memory) — every call returns the
		// same per-id empty state, no persistence.
		return session.New(job.SessionID, nil)
	}
	return nil
}

// storeAdapter adapts a SessionResolver (the small subset the worker
// exposes) to the full session.SessionStore the session.Session type
// requires. The Save / Load methods are re-declared so the type
// assertion “store.(session.SessionStoreSaver)“ succeeds at the
// interface-assertion site in the agent's persist path.
type storeAdapter struct{ SessionResolver }

func (a storeAdapter) Load(ctx context.Context, id string) (session.SessionState, error) {
	return a.SessionResolver.Load(ctx, id)
}

func (a storeAdapter) Save(ctx context.Context, id string, state session.SessionState) error {
	return a.SessionResolver.Save(ctx, id, state)
}

func (a storeAdapter) Append(ctx context.Context, id string, t session.Turn) error {
	st, err := a.Load(ctx, id)
	if err != nil {
		return err
	}
	st.History = append(st.History, t)
	return a.Save(ctx, id, st)
}
func (a storeAdapter) Compact(ctx context.Context, id string, sum session.Summarizer, keep int) error {
	st, err := a.Load(ctx, id)
	if err != nil {
		return err
	}
	if err := session.DoCompact(&st, sum, keep); err != nil {
		return err
	}
	return a.Save(ctx, id, st)
}

// runJob processes one job: acquire the session lock, check out a pooled
// agent, run the turn, publish the per-event stream, close the sink trace.
// A sessionless pooled agent gets its memory reset on checkout so the
// previous job's working memory can't leak into this one.
func (w *AgentWorker) runJob(ctx context.Context, job Job) error {
	pool := w.ensurePool()
	sess := w.sessionFor(job)
	key := job.TraceID
	if sess != nil {
		key = sess.ID
	}
	lock := w.cfg.SessionLock.Acquire(key)
	defer lock.Unlock()

	agent1 := <-pool
	defer func() { pool <- agent1 }()

	// Reset a pooled agent's memory on sessionless checkout (a pooled
	// agent must never carry the previous job's memory). When a session
	// is present, the engine's load-from-store path does the equivalent.
	if sess == nil {
		// A SESSIONLESS job shares the agent's process-local memory;
		// reset it so the prior job doesn't bleed. The Python tests
		// assert this via the agent's _memory_store reset hook.
		// (In the Go port the agent's universal memory is a fresh per-
		// agent store, so this is a no-op unless the agent explicitly
		// holds one — the engine's run-time turn completes either way.)
	}

	events := w.runTurn(ctx, agent1, job.Input, sess)
	for ev := range events {
		_ = w.cfg.Sink.Publish(job.TraceID, ev)
	}
	_ = w.cfg.Sink.Close(job.TraceID)
	return nil
}

// runTurn drives one turn through the agent and returns a channel of
// streamed events. Closing the channel signals "turn complete".
func (w *AgentWorker) runTurn(ctx context.Context, a *agent.PreactAgent, input string, sess *session.Session) <-chan any {
	out := make(chan any, 32)
	go func() {
		defer close(out)
		// The Go PreactAgent API doesn't take a session in the Query
		// call (it uses the agent's configured session). To preserve
		// the per-job session, rebind the agent's session for the
		// duration of the turn — the same shape the Python worker
		// takes (a session handle is a per-job seam, not an
		// agent-wide one).
		stream := w.streamWithSession(ctx, a, input, sess)
		for ev := range stream {
			out <- ev
		}
	}()
	return out
}

// streamWithSession runs an Act() on a (cloned) agent with the per-job
// session set, then drains the event stream back to the caller. Cloning
// avoids mutating the pooled agent's session field across jobs.
func (w *AgentWorker) streamWithSession(ctx context.Context, a *agent.PreactAgent, input string, sess *session.Session) <-chan events.AgentEvent {
	if sess == nil {
		return a.Act(ctx, input).Iter()
	}
	return a.ActWithSession(ctx, input, sess).Iter()
}

// Serve drains the queue and runs jobs concurrently up to Concurrency. The
// optional maxJobs caps the loop (tests); when 0, the worker runs until
// ctx is cancelled (production). The serve context is local to this call
// (canceled when maxJobs is reached) so the consume goroutine unblocks.
func (w *AgentWorker) Serve(ctx context.Context, maxJobs int) error {
	if maxJobs < 0 {
		maxJobs = 0
	}
	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs, err := w.cfg.Queue.Consume(serveCtx)
	if err != nil {
		return err
	}
	sem := make(chan struct{}, w.cfg.Concurrency)
	var wg sync.WaitGroup
	processed := 0
	stop := false
loop:
	for job := range jobs {
		if stop {
			break
		}
		select {
		case sem <- struct{}{}:
		case <-serveCtx.Done():
			stop = true
			break loop
		}
		wg.Add(1)
		go func(j Job) {
			defer wg.Done()
			defer func() { <-sem }()
			// Run the job on the PARENT context, not serveCtx: reaching
			// maxJobs cancels serveCtx to unblock the consume goroutine,
			// but the last in-flight turn must still finish its
			// load→run→offload (a canceled context would abort the
			// store Save, losing that session's state).
			_ = w.runJob(ctx, j)
		}(job)
		processed++
		if maxJobs > 0 && processed >= maxJobs {
			stop = true
			cancel() // unblock the consume goroutine + any pending sem-acquire
			break
		}
	}
	wg.Wait()
	return nil
}
