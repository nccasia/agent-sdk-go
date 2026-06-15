---
id: 08-skills
title: Skills (progressive disclosure)
group: domains
inputs:
  - agent_sdk/tools/
  - agent_sdk/engine/
outputs:
  - agent_sdk/skills/
checks:
  - id: skills-tests
    cmd: go test ./agent_sdk/skills/...
    description: skills tests pass
mode: task
---
# Rung 08 — skills

Python → Go (`agent_sdk/skills/` → `skills`):
- `parser.py`, `compiler.py` (budget-bounded surface + chunk refs, lazy build), `runtime.py`, `packs.py`, `prompt.py` (`build_skill_prompt_block`), `loader.py` (SKILL.md folder → SkillPack), `cache.py`, `context.py`, `definition.py`, `lobes/` (skill_select, skill_active).
- `agent_sdk/{skill_def,skill_runtime}.py` → `skills` public types (Skill, SkillRegistry, SkillPack).

Translate: `tests/test_skill_activation.py`, `tests/test_skill_compiler.py`, `tests/test_skill_loader.py`, and the skill_def parts of `tests/test_extra_coverage.py`.

PARITY.md: Skill, SkillRegistry, SkillPack, build_skill_prompt_block.
