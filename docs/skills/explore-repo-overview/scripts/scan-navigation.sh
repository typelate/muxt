#!/bin/bash
# Scan templates for navigation links: href, action, and $.Path usage.
set -e

echo "=== Hardcoded paths (href and action attributes) ==="
grep -rn 'href=\|action=' --include='*.gohtml' . || echo "No href or action attributes found."

echo ""
echo "=== Type-checked paths ($.Path usage) ==="
grep -rn '\.Path\.' --include='*.gohtml' . || echo "No $.Path usage found."
