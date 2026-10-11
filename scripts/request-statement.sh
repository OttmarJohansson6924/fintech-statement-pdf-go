#!/bin/sh
set -eu

curl --fail-with-body \
  --request POST \
  --header 'Content-Type: application/json' \
  --data @examples/august-statement.json \
  "${STATEMENT_URL:-http://localhost:8080}/statements"
