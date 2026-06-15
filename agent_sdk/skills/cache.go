// Ported from agent_sdk/skills/cache.py — SurfaceCache: lazy
// compile-on-activate persistence for compiled skill surfaces.
//
// A skill's surface is built the first time it is ACTIVATED (never before) and
// cached here keyed by content hash. Two layers: an in-process map (always on)
// and an optional SKILL.compiled.json sidecar next to the skill folder (when the
// pack carries a SourceDir). A sidecar whose content_hash no longer matches is
// stale: ignored and recompiled. Cache I/O is best-effort.
package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const sidecarName = "SKILL.compiled.json"

// SurfaceCache memoizes compiled surfaces by content hash, with an optional
// folder sidecar.
type SurfaceCache struct {
	mu      sync.Mutex
	mem     map[string]CompiledSkill // content_hash → compiled
	persist bool
}

// NewSurfaceCache builds a cache. persist controls whether the folder sidecar is
// read/written (off ⇒ in-process only, e.g. A/B runs).
func NewSurfaceCache(persist bool) *SurfaceCache {
	return &SurfaceCache{mem: map[string]CompiledSkill{}, persist: persist}
}

// Get returns a cached surface for pack at its CURRENT content hash, or
// (zero, false).
func (c *SurfaceCache) Get(pack *SkillPack) (CompiledSkill, bool) {
	chash := ContentHash(pack)
	c.mu.Lock()
	hit, ok := c.mem[chash]
	c.mu.Unlock()
	if ok {
		return hit, true
	}
	side, ok := c.readSidecar(pack)
	if ok && side.ContentHash == chash {
		c.mu.Lock()
		c.mem[chash] = side
		c.mu.Unlock()
		return side, true
	}
	return CompiledSkill{}, false
}

// Put stores compiled in memory and (when persisting) writes the sidecar.
func (c *SurfaceCache) Put(pack *SkillPack, compiled CompiledSkill) {
	c.mu.Lock()
	c.mem[compiled.ContentHash] = compiled
	c.mu.Unlock()
	c.writeSidecar(pack, compiled)
}

func (c *SurfaceCache) sidecarPath(pack *SkillPack) string {
	if !c.persist || pack.SourceDir == "" {
		return ""
	}
	return filepath.Join(pack.SourceDir, sidecarName)
}

func (c *SurfaceCache) readSidecar(pack *SkillPack) (CompiledSkill, bool) {
	p := c.sidecarPath(pack)
	if p == "" {
		return CompiledSkill{}, false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return CompiledSkill{}, false
	}
	var d map[string]any
	if json.Unmarshal(data, &d) != nil {
		return CompiledSkill{}, false
	}
	return CompiledSkillFromJSON(d), true
}

func (c *SurfaceCache) writeSidecar(pack *SkillPack, compiled CompiledSkill) {
	p := c.sidecarPath(pack)
	if p == "" {
		return
	}
	data, err := json.MarshalIndent(compiled.ToJSON(), "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(p, data, 0o644)
}
