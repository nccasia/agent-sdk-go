#!/usr/bin/env bash
# check-upstream.sh — list Python agent-sdk commits not yet ported to this Go repo.
#
# A commit is "ported" when a Go commit carries an `Upstream: <sha>` trailer for it
# (see docs/UPSTREAM.md). Run from the repo root:
#
#   PYTHON_SDK=../agent-sdk ./scripts/check-upstream.sh
#
# PYTHON_SDK   path to a local clone of https://github.com/nccasia/agent-sdk (default ../agent-sdk)
# BASELINE     upstream sha the full port was built against (default 59feb07)
set -euo pipefail

PY="${PYTHON_SDK:-../agent-sdk}"
BASELINE="${BASELINE:-59feb07}"

if [ ! -d "$PY/.git" ]; then
  echo "error: PYTHON_SDK=$PY is not a git clone of nccasia/agent-sdk" >&2
  echo "       git clone https://github.com/nccasia/agent-sdk $PY" >&2
  exit 2
fi

git -C "$PY" fetch --quiet origin 2>/dev/null || true
UP="$(git -C "$PY" rev-parse --verify --quiet origin/main || echo main)"

pending=0
printf '%-9s  %-7s  %s\n' "STATUS" "SHA" "SUBJECT"
while read -r sha subject; do
  [ -n "$sha" ] || continue
  if [ -n "$(git log --grep="Upstream: ${sha}" -1 --format=%h 2>/dev/null)" ]; then
    printf '%-9s  %-7s  %s\n' "PORTED" "$sha" "$subject"
  else
    printf '%-9s  %-7s  %s\n' "PENDING" "$sha" "$subject"
    pending=$((pending + 1))
  fi
done < <(git -C "$PY" log --no-merges --reverse --format='%h %s' "${BASELINE}..${UP}")

echo
if [ "$pending" -eq 0 ]; then
  echo "in sync — no unported upstream commits."
else
  echo "$pending pending commit(s). Port each, commit with an 'Upstream: <sha>' trailer, then update docs/UPSTREAM.md."
fi
