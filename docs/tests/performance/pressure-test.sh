#!/usr/bin/env bash
set -euo pipefail

# Single-file pressure test runner:
# 1) issue token keys from /api/token-apply
# 2) run k6 load/pressure test with generated keys

BASE_URL="${BASE_URL:-https://ai.sensetime-inc.com}"
MODEL_NAME="${MODEL_NAME:-auto}" # set explicit model name in production if possible
PROFILE="${PROFILE:-capacity}"   # baseline|capacity|stability|burst
PEAK_VUS="${PEAK_VUS:-120}"
KEY_COUNT="${KEY_COUNT:-10}"
REQUEST_TIMEOUT="${REQUEST_TIMEOUT:-30s}"
TEST_TAG="${TEST_TAG:-token-pressure-$(date +%Y%m%d-%H%M%S)}"
TOKEN_GROUP="${TOKEN_GROUP:-default}"
ORG_CODE="${ORG_CODE:-D001-T010}"
ORG_BUDGET="${ORG_BUDGET:-50000}"
PERIOD_TYPE="${PERIOD_TYPE:-day}"
TOKEN_API_KEY="${TOKEN_API_KEY:-}"

if [[ -z "$TOKEN_API_KEY" && -f .env ]]; then
  TOKEN_API_KEY="$(python3 - <<'PY'
import os
v=""
with open(".env","r",encoding="utf-8") as f:
    for line in f:
        line=line.strip()
        if line.startswith("TOKEN_API_KEY="):
            v=line.split("=",1)[1].strip().strip('"').strip("'")
            break
print(v)
PY
)"
fi

if [[ -z "$TOKEN_API_KEY" ]]; then
  echo "[error] TOKEN_API_KEY is empty (export it or put in .env)."
  exit 1
fi

if ! command -v k6 >/dev/null 2>&1; then
  echo "[error] k6 not found in PATH."
  exit 1
fi

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT
TOKENS_FILE="$WORK_DIR/tokens.txt"
K6_SCRIPT="$WORK_DIR/pressure.k6.js"
K6_SUMMARY="docs/tests/performance/reports/pressure-summary-${TEST_TAG}.json"
mkdir -p "docs/tests/performance/reports"

echo "[info] issuing $KEY_COUNT token keys..."
for i in $(seq 1 "$KEY_COUNT"); do
  ticket_no="PT-${TEST_TAG}-${i}"
  work_no="PT$(printf "%05d" "$i")"
  email="pressure+${ticket_no}@example.com"
  body="$(cat <<EOF
{"ticket_no":"$ticket_no","email":"$email","amount":2000,"currency":"CNY","org_code":"$ORG_CODE","org_budget":$ORG_BUDGET,"period_type":"$PERIOD_TYPE","scope_type":"team","token_type":"user","work_no":"$work_no","token_name":"pressure-$ticket_no","token_group":"$TOKEN_GROUP","remark":"single script pressure test"}
EOF
)"
  resp="$(curl -sS -X POST "$BASE_URL/api/token-apply" \
    -H "X-Api-Key: $TOKEN_API_KEY" \
    -H "Content-Type: application/json" \
    -d "$body")"
  token_key="$(python3 - <<'PY' "$resp"
import json,sys
try:
    d=json.loads(sys.argv[1])
    print((d.get("data") or {}).get("token_key",""))
except Exception:
    print("")
PY
)"
  if [[ -n "$token_key" ]]; then
    echo "$token_key" >> "$TOKENS_FILE"
  else
    echo "[warn] issue failed: $ticket_no"
  fi
done

if [[ ! -s "$TOKENS_FILE" ]]; then
  echo "[error] no token key issued."
  exit 1
fi

TOKEN_KEYS="$(python3 - <<'PY' "$TOKENS_FILE"
import sys
with open(sys.argv[1],"r",encoding="utf-8") as f:
    print(",".join([x.strip() for x in f if x.strip()]))
PY
)"

cat > "$K6_SCRIPT" <<'EOF'
import http from 'k6/http';
import exec from 'k6/execution';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const okRate = new Rate('business_success_rate');
const BASE_URL = __ENV.BASE_URL;
const MODEL_NAME = __ENV.MODEL_NAME || 'auto';
const REQUEST_TIMEOUT = __ENV.REQUEST_TIMEOUT || '30s';
const PEAK_VUS = Number(__ENV.PEAK_VUS || 120);
const PROFILE = __ENV.PROFILE || 'capacity';
const TOKEN_KEYS = (__ENV.TOKEN_KEYS || '').split(',').map((s) => s.trim()).filter(Boolean);

function pickToken() {
  if (!TOKEN_KEYS.length) throw new Error('TOKEN_KEYS empty');
  const id = exec.vu.idInTest || 1;
  return TOKEN_KEYS[(id - 1) % TOKEN_KEYS.length];
}

function stages() {
  if (PROFILE === 'baseline') return [{ duration: '5m', target: PEAK_VUS }];
  if (PROFILE === 'burst') return [{ duration: '2m', target: Math.round(PEAK_VUS * 0.5) }, { duration: '2m', target: Math.round(PEAK_VUS * 1.8) }, { duration: '4m', target: PEAK_VUS }];
  if (PROFILE === 'stability') return [{ duration: '10m', target: Math.round(PEAK_VUS * 1.1) }];
  return [{ duration: '2m', target: PEAK_VUS }, { duration: '2m', target: Math.round(PEAK_VUS * 1.4) }, { duration: '2m', target: Math.round(PEAK_VUS * 1.8) }];
}

export const options = {
  scenarios: {
    pressure: {
      executor: 'ramping-vus',
      startVUs: 1,
      stages: stages(),
      gracefulRampDown: '20s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.02'],
    business_success_rate: ['rate>0.99'],
    http_req_duration: ['p(95)<6000', 'p(99)<12000'],
  },
};

function resolveModel(token) {
  if (MODEL_NAME !== 'auto') return MODEL_NAME;
  const r = http.get(`${BASE_URL}/v1/models`, { headers: { Authorization: `Bearer ${token}` }, timeout: REQUEST_TIMEOUT });
  const j = JSON.parse(r.body || '{}');
  return (j.data && j.data[0] && j.data[0].id) || '';
}

export function setup() {
  const t = TOKEN_KEYS[0];
  const model = resolveModel(t);
  if (!model) throw new Error('no model resolved');
  return { model };
}

export default function (data) {
  const token = pickToken();
  const r = http.post(`${BASE_URL}/v1/chat/completions`, JSON.stringify({
    model: data.model,
    stream: false,
    max_tokens: 128,
    messages: [{ role: 'user', content: '压力测试: 返回一句话' }],
  }), {
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    timeout: REQUEST_TIMEOUT,
  });
  const ok = (r.status >= 200 && r.status < 300) || r.status === 429;
  okRate.add(ok);
  check(r, { 'status ok/429': () => ok });
  sleep(0.12);
}
EOF

echo "[info] start pressure test, profile=$PROFILE, peak_vus=$PEAK_VUS"
BASE_URL="$BASE_URL" \
MODEL_NAME="$MODEL_NAME" \
PROFILE="$PROFILE" \
PEAK_VUS="$PEAK_VUS" \
TOKEN_KEYS="$TOKEN_KEYS" \
REQUEST_TIMEOUT="$REQUEST_TIMEOUT" \
k6 run --summary-export "$K6_SUMMARY" "$K6_SCRIPT"

echo "[done] report: $K6_SUMMARY"
