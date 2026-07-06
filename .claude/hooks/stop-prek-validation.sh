#!/usr/bin/env bash
#
# Stop Hook: Prek Validation
#
# Runs prek validation when Claude Code stops with smart triggering:
#
# Default mode (CLAUDE_LINT_ON_STOP not set):
#   - Only runs when there are uncommitted changes
#   - Skips validation for read-only queries (fast iteration)
#   - Validates when Claude modifies code (catch issues before commit)
#
# Strict mode (export CLAUDE_LINT_ON_STOP=true):
#   - Always runs validation on every stop
#   - Use when you want maximum quality enforcement
#   - Slower but catches issues immediately
#
# Performance:
#   - Validates changed files only (5-10s typical)
#   - Uses hack/prek.ci.toml (skips network-dependent hooks)
#
set -uo pipefail

# Ensure we're running from the git repository root
# This handles cases where Claude Code's CWD is in a subdirectory (e.g., .claude/skills/)
REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null)

# Check for jq dependency before any jq call
if ! command -v jq &> /dev/null; then
  cat <<'EOF'
{"decision": "block", "reason": "jq is not installed — required for hook processing.\n\nInstall it:\n  brew install jq         # macOS\n  apt-get install jq      # Debian/Ubuntu\n  yum install jq          # RHEL/CentOS\n\nRetry the action once installed."}
EOF
  exit 0
fi

if [[ -z "$REPO_ROOT" ]]; then
  jq -n '{"decision": "block", "reason": "Not in a git repository. Cannot run prek validation."}'
  exit 0
fi
cd "$REPO_ROOT" || exit 1

HOOK_INPUT=$(cat)

# Allow stop on retry to prevent infinite loops
STOP_HOOK_ACTIVE=$(echo "$HOOK_INPUT" | jq -r '.stop_hook_active // false')
if [[ "$STOP_HOOK_ACTIVE" == "true" ]]; then
  exit 0
fi

# Determine if validation should run:
# 1. If CLAUDE_LINT_ON_STOP=true → always run (user opt-in for strict mode)
# 2. Otherwise, only run if there are uncommitted changes (about to commit)
FORCE_LINT="${CLAUDE_LINT_ON_STOP:-false}"

if [[ "$FORCE_LINT" != "true" ]]; then
  # Check for uncommitted changes (staged, unstaged, or untracked)
  if git diff-index --quiet HEAD -- 2>/dev/null && [[ -z "$(git ls-files --others --exclude-standard)" ]]; then
    # No changes and not forced - skip validation
    exit 0
  fi
fi

# Check if prek is installed — block and nudge instead of silently passing
if ! command -v prek &> /dev/null; then
  jq -n \
    --arg reason "prek is not installed — required for quality checks before stopping.

Install it:
  uv tool install prek      # recommended
  pipx install prek         # alternative
  pip install --user prek   # fallback

Then wire up the git hook: prek install

Retry the action once installed so validation can run." \
    '{"decision": "block", "reason": $reason}'
  exit 0
fi

# Run prek validation (using CI config to skip network-dependent hooks)
# Validate changed files (staged + unstaged + untracked)
# Check for changes without storing NUL-delimited output in a variable (which strips NULs)
HAS_CHANGES=false
git diff --name-only --diff-filter=d HEAD | grep -q . && HAS_CHANGES=true
git ls-files --others --exclude-standard | grep -q . && HAS_CHANGES=true

if [[ "$HAS_CHANGES" == "false" ]]; then
  # No files changed — fall back to --all-files
  PREK_OUTPUT=$(prek run --all-files --config hack/prek.ci.toml 2>&1)
else
  # Stream NUL-delimited filenames directly to xargs to preserve filenames with spaces
  PREK_OUTPUT=$({ git diff -z --name-only --diff-filter=d HEAD; git ls-files -z --others --exclude-standard; } | xargs -0 prek run --config hack/prek.ci.toml --files 2>&1)
fi
PREK_EXIT=$?

if [[ $PREK_EXIT -eq 0 ]]; then
  exit 0
fi

# Extract only prek-structured hook/error lines — never forward raw tool output
# which could contain secrets, env vars, or internal paths.
SANITIZED_OUTPUT=$(echo "$PREK_OUTPUT" | grep -E "^(hook |Failed|Passed|ERROR|error|warning|Warning|\[)" | head -30)
if [[ -z "$SANITIZED_OUTPUT" ]]; then
  SANITIZED_OUTPUT="prek reported failures — run 'prek run --all-files --config hack/prek.ci.toml' locally to see details."
fi

# Block stop and tell Claude what to fix
jq -n \
  --arg reason "prek validation failed. Fix the issues below, then try again:

$SANITIZED_OUTPUT" \
  '{"decision": "block", "reason": $reason}'
