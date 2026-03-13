#!/bin/bash
# Analyze template relationships: list routes, find duplicates, and shared sub-templates.
# Requires: jq
set -e

if ! command -v jq &>/dev/null; then
  echo "Error: jq is required but not installed." >&2
  exit 1
fi

echo "=== All route templates ==="
muxt list-template-callers --format json | jq '[.Templates[].Name]'

echo ""
echo "=== Duplicate route patterns ==="
muxt list-template-callers --format json | jq '
  [.Templates[].Name
   | select(test("^(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS) "))
  ] | group_by(split(" ") | .[0:2] | join(" "))
    | map(select(length > 1))
    | .[]
'

echo ""
echo "=== Sub-templates shared by multiple routes ==="
muxt list-template-calls --format json | jq '
  [.Templates[]
   | select(.Name | test("^(GET|POST|PUT|PATCH|DELETE) "))
   | {route: .Name, refs: [.References[].Name]}
  ] | [.[].refs[]]
    | group_by(.) | map(select(length > 1)) | map(.[0])
'
