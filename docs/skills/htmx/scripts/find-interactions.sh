#!/bin/bash
# Find HTMX interactions in templates: hx-get/post, targets, swaps, and triggers.
set -e

echo "=== HTMX request attributes (hx-get, hx-post, etc.) ==="
grep -rn 'hx-get\|hx-post\|hx-put\|hx-patch\|hx-delete' --include='*.gohtml' . || echo "No HTMX request attributes found."

echo ""
echo "=== Swap targets (hx-target, hx-swap, hx-select) ==="
grep -rn 'hx-target\|hx-swap\|hx-select' --include='*.gohtml' . || echo "No swap targets found."

echo ""
echo "=== Triggers (hx-trigger, hx-confirm, hx-boost) ==="
grep -rn 'hx-trigger\|hx-confirm\|hx-boost' --include='*.gohtml' . || echo "No triggers found."
