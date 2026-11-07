#!/bin/sh

set -eu
cd "$(dirname "$0")"

PORT=${PORT:-18090}
WORK=$(mktemp -d)
BIN=$WORK/api
DB=$WORK/smoke.db
B=http://127.0.0.1:$PORT
trap 'kill $PID 2>/dev/null || true' EXIT

echo "== build"
go build -o "$BIN" ./cmd/api

echo "== start (db: $DB)"
"$BIN" -addr "127.0.0.1:$PORT" -db "$DB" >"$WORK/api.log" 2>&1 &
PID=$!
for _ in 1 2 3 4 5 6 7 8 9 10; do
  curl -sf "$B/api/health" >/dev/null 2>&1 && break
  sleep 0.5
done
curl -sf "$B/api/health"; echo

echo "== import"
curl -sf -X POST "$B/api/import" --data-binary @testdata/example_records.json; echo

echo "== evaluate all"
curl -sf -X POST "$B/api/evaluate/batch" -d '{}' | head -c 300; echo

echo "== wait for jobs"
for _ in $(seq 1 600); do
  PENDING=$(curl -sf "$B/api/jobs" | tr -d ' \n' | grep -oE '"(queued|running)":[0-9]+' | grep -oE '[0-9]+$' | paste -sd+ - | bc 2>/dev/null || echo 0)
  [ "${PENDING:-0}" = "0" ] && break
  sleep 1
done
curl -sf "$B/api/jobs" | tr -d '\n' | grep -oE '"counts":\{[^}]*\}'; echo
if curl -sf "$B/api/jobs?status=failed" | grep -q '"status":"failed"'; then
  echo "!! some jobs failed:"; curl -sf "$B/api/jobs?status=failed" | grep -oE '"error":"[^"]*"'
fi

echo "== export"
curl -sf "$B/api/export?format=json" >"$WORK/export.json"
curl -sf "$B/api/export?format=csv" >"$WORK/export.csv"
echo "json: $(grep -c '"evaluation_id"' "$WORK/export.json") records, csv: $(wc -l <"$WORK/export.csv" | tr -d ' ') lines -> $WORK"

echo "== restart keeps data"
kill $PID; wait $PID 2>/dev/null || true
"$BIN" -addr "127.0.0.1:$PORT" -db "$DB" >>"$WORK/api.log" 2>&1 &
PID=$!
sleep 1
curl -sf "$B/api/conversations" | grep -oE '"latest_job_status":"[a-z]+"' | sort | uniq -c

echo "== repo hygiene"
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  TRACKED=$(git ls-files)
  echo "$TRACKED" | grep -E '\.db(-wal|-shm)?$|^backend/resources/|/resources/' && { echo "!! database or resources tracked"; exit 1; } || true
  echo "$TRACKED" | xargs grep -nIE 'X-Amz-(Signature|Credential)|[?&](sig|signature|token|sv)=|api[_-]?key\s*[:=]|BEGIN (RSA|OPENSSH|EC) PRIVATE KEY' 2>/dev/null \
    | grep -vE 'smoke\.sh' && { echo "!! possible secret in tracked files"; exit 1; } || true
  echo "tracked files: $(echo "$TRACKED" | wc -l | tr -d ' '), no databases, no signed links, no keys"
fi
grep -c SECRET "$WORK/api.log" >/dev/null 2>&1 && echo "!! log contains SECRET marker" || true
echo "== ok"
