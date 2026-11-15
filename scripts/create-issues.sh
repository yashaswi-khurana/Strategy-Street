#!/usr/bin/env bash
#
# Create one GitHub issue per file in docs/issues/SS-*.md.
#
#   ./scripts/create-issues.sh --dry-run     # print what would be created (default)
#   ./scripts/create-issues.sh --yes         # actually create the issues
#   ./scripts/create-issues.sh --yes --milestone-filter "Milestone | 2"   # subset
#
# Requires: gh CLI authenticated against the repo's origin.
# Idempotency: existing issue titles are skipped, so re-running is safe.
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ISSUE_DIR="$ROOT/docs/issues"

MODE="dry-run"
FILTER=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --yes) MODE="apply"; shift ;;
    --dry-run) MODE="dry-run"; shift ;;
    --milestone-filter) FILTER="${2:-}"; shift 2 ;;
    -h|--help) sed -n '2,12p' "$0"; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

command -v gh >/dev/null || { echo "gh CLI not found" >&2; exit 1; }
gh auth status >/dev/null 2>&1 || { echo "gh is not authenticated" >&2; exit 1; }

declare -A LABEL_FOR_PHASE=(
  [0]="phase:0-foundation"
  [1]="phase:1-data-layer"
  [2]="phase:2-python-engine"
  [3]="phase:3-orchestration"
  [4]="phase:4-paper-trading"
  [5]="phase:5-frontend"
  [6]="phase:6-hardening"
)

ensure_label() {
  gh label create "$1" --color "${2:-ededed}" --description "${3:-}" --force >/dev/null 2>&1 || true
}

existing_titles=""
if [[ "$MODE" == "apply" ]]; then
  existing_titles="$(gh issue list --state all --limit 500 --json title --jq '.[].title')"
fi

created=0
skipped=0

for file in "$ISSUE_DIR"/SS-*.md; do
  title="$(head -1 "$file" | sed 's/^# //')"
  phase="$(grep -m1 '^| Milestone |' "$file" | sed -E 's/.*\| *Milestone *\| *([0-9]+).*/\1/')"
  phase="${phase//[!0-9]/}"

  if [[ -n "$FILTER" ]] && ! grep -q "$FILTER" "$file"; then
    continue
  fi

  if [[ "$MODE" == "apply" ]] && grep -qxF "$title" <<<"$existing_titles"; then
    echo "skip (exists): $title"
    skipped=$((skipped + 1))
    continue
  fi

  if [[ "$MODE" == "dry-run" ]]; then
    echo "[dry-run] $title   (label: ${LABEL_FOR_PHASE[$phase]:-issue})"
    created=$((created + 1))
    continue
  fi

  label="${LABEL_FOR_PHASE[$phase]:-issue}"
  ensure_label "$label" "1d76db" "Phase $phase work"
  ensure_label "spec" "5319e7" "Derived from algorithmic-trading-platform.md"

  gh issue create \
    --title "$title" \
    --body-file "$file" \
    --label "$label" \
    --label "spec" >/dev/null
  echo "created: $title"
  created=$((created + 1))
done

echo
echo "$MODE complete — $created issue(s), $skipped skipped."
if [[ "$MODE" == "dry-run" ]]; then
  echo "Re-run with --yes to create them."
fi
