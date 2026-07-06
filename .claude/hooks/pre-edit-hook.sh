#!/usr/bin/env bash
# PreToolUse hook: extracts file_path from Claude Code tool JSON (stdin) and
# delegates to pre-edit.sh for generated/vendored/high-risk file protection.
set -euo pipefail

INPUT=$(cat)
FILE=$(python3 -c "
import sys, json
d = json.loads(sys.argv[1])
print(d.get('tool_input', {}).get('file_path', ''))
" "$INPUT" 2>/dev/null || echo "")

[ -z "$FILE" ] && exit 0

exec bash "$(dirname "$0")/pre-edit.sh" "$FILE"
