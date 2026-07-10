#!/usr/bin/env bash
# 压测入口：先凑满 KEYS，再跑 RPS × DURATION
# 示例：KEYS=300 RPS=300 DURATION=10m bash docs/tests/performance/pressure-run.sh
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

KEYS="${KEYS:?KEYS required, e.g. KEYS=300}"
RPS="${RPS:?RPS required, e.g. RPS=300}"
DURATION="${DURATION:?DURATION required, e.g. DURATION=10m}"

APP_PER_ORG="${APP_PER_ORG:-10}"
ORG_COUNT="${ORG_COUNT:-$((KEYS / APP_PER_ORG))}"
[[ "$((ORG_COUNT * APP_PER_ORG))" -eq "$KEYS" ]] || {
  echo "[error] KEYS must be ORG_COUNT×APP_PER_ORG (default ×10)" >&2; exit 1
}

TEST_TAG="${TEST_TAG:-pt-$(date +%m%d-%H%M%S)}"
SEED_FILE="${SEED_FILE:-$DIR/cases/seed-${TEST_TAG}.json}"
BASE_URL="${BASE_URL:-https://ai.sensetime-inc.com}"

run() { env "$@" bash "$DIR/pressure-test.sh"; }

seed_count() {
  python3 -c "import json,sys; print(len(json.load(open(sys.argv[1])).get('app',[])))" "$1"
}

# 1) 申请 Key（已有 seed 且数量够则跳过；未齐则换新 TAG 重申）
have=0
[[ -f "$SEED_FILE" ]] && have="$(seed_count "$SEED_FILE")"
if [[ "$have" -eq "$KEYS" ]]; then
  echo "[seed] reuse $SEED_FILE ($have keys)"
else
  if [[ -f "$SEED_FILE" && "$have" -gt 0 ]]; then
    echo "[seed] incomplete $have/$KEYS, new tag (avoid duplicate tickets)"
    TEST_TAG="pt-$(date +%m%d-%H%M%S)"
    SEED_FILE="$DIR/cases/seed-${TEST_TAG}.json"
  fi
  echo "[seed] apply $KEYS keys ($ORG_COUNT×$APP_PER_ORG), batched -> $SEED_FILE"
  run MODE=seed SCALE=prod TEST_TAG="$TEST_TAG" SEED_FILE="$SEED_FILE" \
    ORG_COUNT="$ORG_COUNT" APP_KEYS_PER_ORG="$APP_PER_ORG" USER_KEY_COUNT=0 \
    ORG_PREFIX="${ORG_PREFIX:-D001-SMOKE}" \
    SEED_INTERVAL="${SEED_INTERVAL:-1.2}" SEED_BATCH_SIZE="${SEED_BATCH_SIZE:-50}" \
    SEED_BATCH_PAUSE="${SEED_BATCH_PAUSE:-5}" SEED_MAX_RETRY="${SEED_MAX_RETRY:-8}"
  have="$(seed_count "$SEED_FILE")"
fi

[[ "$have" -eq "$KEYS" ]] || {
  echo "[error] seed has $have/$KEYS keys — abort soak (Key 未齐不压测)" >&2
  exit 1
}
echo "[seed] ready $have/$KEYS — start soak"

# 2) Key 齐了再压测
echo "[soak] $KEYS keys · ${RPS} RPS × ${DURATION}"
run MODE=soak SCALE=prod RECONCILE=false TEST_TAG="$TEST_TAG" SEED_FILE="$SEED_FILE" \
  TARGET_RPS="$RPS" SOAK_DURATION="$DURATION" SOAK_MAX_VUS="${SOAK_MAX_VUS:-500}"

echo "[done] $DIR/cases/summary-${TEST_TAG}.json"
