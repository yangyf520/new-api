#!/usr/bin/env bash
# 压测：MODE=smoke|seed|soak|burst|full  SCALE=dev|prod
# 说明：docs/tests/performance/pressure-test.md
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
REPORT_DIR="$SCRIPT_DIR/reports"

BASE_URL="${BASE_URL:-https://ai.sensetime-inc.com}"
MODE="${MODE:-smoke}"
SCALE="${SCALE:-dev}"
TEST_TAG="${TEST_TAG:-pt-$(date +%Y%m%d-%H%M%S)}"
RECONCILE="${RECONCILE:-}"
TOKEN_API_KEY="${TOKEN_API_KEY:-}"
SEED_FILE="${SEED_FILE:-}"
ORG_PREFIX="${ORG_PREFIX:-D001-T}"

RUN_APPLY_WRITE="${RUN_APPLY_WRITE:-false}"
RUN_APPLY_INCREASE="${RUN_APPLY_INCREASE:-false}"
RUN_PORTAL_READ="${RUN_PORTAL_READ:-false}"
PORTAL_AUTH_TOKEN="${PORTAL_AUTH_TOKEN:-}"
STREAM_RATIO="${STREAM_RATIO:-0}"
APPLY_WRITE_RPS="${APPLY_WRITE_RPS:-5}"
APPLY_WRITE_DURATION="${APPLY_WRITE_DURATION:-}"
INCREASE_VUS="${INCREASE_VUS:-10}"
INCREASE_DURATION="${INCREASE_DURATION:-}"
PORTAL_READ_RPS="${PORTAL_READ_RPS:-10}"
PORTAL_READ_DURATION="${PORTAL_READ_DURATION:-}"
INCREASE_DELTA="${INCREASE_DELTA:-100}"

case "$SCALE" in
  prod) ORG_COUNT=20; APP_PER_ORG=10; USER_COUNT=300; TARGET_RPS=120; SOAK_DURATION=2h; PEAK_VUS=500; SOAK_MAX_VUS=400 ;;
  *)    ORG_COUNT=2;  APP_PER_ORG=5;  USER_COUNT=10;  TARGET_RPS=20;  SOAK_DURATION=5m; PEAK_VUS=80;  SOAK_MAX_VUS=100 ;;
esac
ORG_COUNT="${ORG_COUNT:-$ORG_COUNT}"
APP_PER_ORG="${APP_KEYS_PER_ORG:-$APP_PER_ORG}"
USER_COUNT="${USER_KEY_COUNT:-$USER_COUNT}"
TARGET_RPS="${TARGET_RPS:-$TARGET_RPS}"
SOAK_DURATION="${SOAK_DURATION:-$SOAK_DURATION}"
PEAK_VUS="${PEAK_VUS:-$PEAK_VUS}"

# full + prod 默认开启扩展场景
if [[ "$MODE" == "full" && "$SCALE" == "prod" ]]; then
  RUN_APPLY_WRITE="${RUN_APPLY_WRITE:-true}"
  RUN_APPLY_INCREASE="${RUN_APPLY_INCREASE:-true}"
  STREAM_RATIO="${STREAM_RATIO:-30}"
  [[ -n "$PORTAL_AUTH_TOKEN" ]] && RUN_PORTAL_READ="${RUN_PORTAL_READ:-true}"
fi

[[ -z "$TOKEN_API_KEY" && -f "$REPO_ROOT/.env" ]] && \
  TOKEN_API_KEY="$(grep '^TOKEN_API_KEY=' "$REPO_ROOT/.env" | cut -d= -f2- | tr -d '"')"
[[ -z "$TOKEN_API_KEY" ]] && { echo "[error] TOKEN_API_KEY required"; exit 1; }

paths() {
  SEED_FILE="${SEED_FILE:-$REPORT_DIR/seed-${TEST_TAG}.json}"
  K6_SUMMARY="$REPORT_DIR/summary-${TEST_TAG}.json"
  BILL_BEFORE="$REPORT_DIR/billing-before-${TEST_TAG}.json"
  BILL_AFTER="$REPORT_DIR/billing-after-${TEST_TAG}.json"
  CALIBRATE="$REPORT_DIR/calibrate-${TEST_TAG}.json"
  RECON_OUT="$REPORT_DIR/reconcile-${TEST_TAG}.json"
  REPORT_MD="$REPORT_DIR/report-${TEST_TAG}.md"
}

side_duration() {
  local soak="$1" burst="$2"
  if [[ "$soak" == "true" ]]; then echo "$SOAK_DURATION"
  elif [[ "$burst" == "true" ]]; then echo "9m"
  else echo "5m"; fi
}

py() { REPO_ROOT="$REPO_ROOT" python3 - "$@" <<'PY'
import argparse, json, os, sys, urllib.error, urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import datetime
from pathlib import Path
from urllib.parse import unquote, urlparse

REPO = Path(os.environ["REPO_ROOT"])

def load_dsn():
    if os.environ.get("SQL_DSN"):
        return os.environ["SQL_DSN"].strip()
    for line in (REPO / ".env").read_text(encoding="utf-8").splitlines():
        if line.strip().startswith("SQL_DSN="):
            return line.split("=", 1)[1].strip().strip('"').strip("'")
    raise SystemExit("[error] SQL_DSN not set")

def pg():
    try:
        import psycopg2
    except ImportError:
        raise SystemExit("[error] pip install psycopg2-binary")
    u = urlparse(load_dsn())
    return psycopg2.connect(host=u.hostname, port=u.port or 5432,
        user=unquote(u.username or ""), password=unquote(u.password or ""),
        dbname=(u.path or "/").lstrip("/"))

def post_apply(base, key, body):
    import urllib.error, urllib.request
    req = urllib.request.Request(f"{base.rstrip('/')}/api/token-apply", method="POST",
        headers={"X-Api-Key": key, "Content-Type": "application/json"},
        data=json.dumps(body).encode())
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            raw = r.read().decode()
    except urllib.error.HTTPError as e:
        raw = e.read().decode()
    doc = json.loads(raw)
    if not doc.get("success"):
        return None, doc.get("message") or raw[:200]
    d = doc.get("data") or {}
    if not d.get("token_key"):
        return None, "empty token_key"
    return {"token": d["token_key"], "org_code": body["org_code"], "token_type": body["token_type"],
        "ticket_no": body["ticket_no"], "token_apply_id": d.get("token_apply_id"), "token_id": d.get("token_id")}, None

def cmd_seed(a):
    jobs, seq = [], 0
    extra = {"parent_org_code": a.parent_org_code, "parent_org_budget": a.parent_org_budget}
    for o in range(1, a.org_count + 1):
        oc = f"{a.org_prefix}{o:03d}"
        for i in range(1, a.app_per_org + 1):
            seq += 1
            t = f"PT-{a.test_tag}-APP-{o:03d}-{i:03d}"
            jobs.append({"ticket_no": t, "email": f"app+{t}@example.com", "amount": a.key_amount,
                "currency": "CNY", "org_code": oc, "org_budget": a.org_budget, "period_type": a.period_type,
                "scope_type": "team", "token_type": "app", "work_no": f"APP{seq:06d}",
                "token_name": f"app-{t}", "token_group": oc, **extra})
    npu = max(1, a.user_count // a.org_count)
    exu = a.user_count - npu * a.org_count
    for o in range(1, a.org_count + 1):
        oc = f"{a.org_prefix}{o:03d}"
        n = npu + (1 if o <= exu else 0)
        for u in range(1, n + 1):
            seq += 1
            t = f"PT-{a.test_tag}-USR-{o:03d}-{u:03d}"
            jobs.append({"ticket_no": t, "email": f"user+{t}@example.com", "amount": a.key_amount,
                "currency": "CNY", "org_code": oc, "org_budget": a.org_budget, "period_type": a.period_type,
                "scope_type": "team", "token_type": "user", "work_no": f"USR{seq:06d}",
                "token_name": f"user-{t}", "token_group": oc, **extra})
    app, user, err = [], [], []
    with ThreadPoolExecutor(max_workers=a.workers) as ex:
        futs = {ex.submit(post_apply, a.base_url, a.api_key, j): j for j in jobs}
        for fut in as_completed(futs):
            try:
                item, e = fut.result()
            except Exception as x:
                err.append({"ticket": futs[fut]["ticket_no"], "error": str(x)}); continue
            if item is None:
                err.append({"ticket": futs[fut]["ticket_no"], "error": e})
            elif item["token_type"] == "app":
                app.append(item)
            else:
                user.append(item)
    inc, seen = [], set()
    for item in user:
        oc = item.get("org_code")
        if oc and oc not in seen and item.get("token_apply_id"):
            seen.add(oc)
            inc.append({"token_apply_id": item["token_apply_id"], "org_code": oc, "ticket_no": item["ticket_no"],
                "current_amount": a.key_amount, "increase_to": a.key_amount + a.increase_delta})
    n07 = None
    if user and user[0].get("token_apply_id"):
        n07 = {"token_apply_id": user[0]["token_apply_id"], "org_code": user[0]["org_code"],
               "current_amount": a.key_amount, "increase_to": a.key_amount + a.increase_delta}
    out = {"test_tag": a.test_tag, "org_count": a.org_count, "org_prefix": a.org_prefix, "key_amount": a.key_amount,
           "increase_delta": a.increase_delta, "app": app, "user": user, "increase_targets": inc,
           "n07_target": n07, "errors": err}
    Path(a.out).parent.mkdir(parents=True, exist_ok=True)
    Path(a.out).write_text(json.dumps(out, indent=2, ensure_ascii=False))
    print(f"[seed] app={len(app)} user={len(user)} inc={len(inc)} err={len(err)} -> {a.out}", file=sys.stderr)
    sys.exit(0 if app else 1)

def cmd_snapshot(a):
    like = f"PT-{a.test_tag}%"
    conn = pg()
    try:
        cur = conn.cursor()
        cur.execute("""SELECT a.id,a.ticket_no,a.org_code,t.id,t.remain_quota,t.used_quota,u.id,u.quota
            FROM token_apply_records a LEFT JOIN tokens t ON t.id=a.token_id LEFT JOIN users u ON u.id=a.user_id
            WHERE a.ticket_no LIKE %s ORDER BY a.id""", (like,))
        tokens = [{"token_apply_id": r[0], "ticket_no": r[1], "org_code": r[2], "token_id": int(r[3] or 0),
            "remain_quota": int(r[4] or 0), "used_quota": int(r[5] or 0), "user_id": int(r[6] or 0),
            "user_wallet_quota": int(r[7] or 0)} for r in cur.fetchall()]
        cur.execute("""SELECT scope_type,scope_code,token_type,cap_amount,used_amount,period_key
            FROM token_spend_policies WHERE enabled=true AND (scope_code LIKE %s OR token_apply_id IN
            (SELECT id FROM token_apply_records WHERE ticket_no LIKE %s))""", (f"{a.org_prefix}%", like))
        policies = [{"scope_type": r[0], "scope_code": r[1], "token_type": r[2], "cap_amount": float(r[3] or 0),
            "used_amount": float(r[4] or 0), "period_key": r[5]} for r in cur.fetchall()]
        cur.execute("""SELECT COALESCE(SUM(l.budget_delta),0) FROM token_apply_logs l
            JOIN token_apply_records a ON a.id=l.token_apply_id WHERE a.ticket_no LIKE %s AND l.budget_delta < 0""", (like,))
        neg_budget = float(cur.fetchone()[0] or 0)
    finally:
        conn.close()
    snap = {"test_tag": a.test_tag, "tokens": tokens, "spend_policies": policies, "neg_budget_delta": neg_budget}
    Path(a.out).parent.mkdir(parents=True, exist_ok=True)
    Path(a.out).write_text(json.dumps(snap, indent=2))
    print(f"[snapshot] tokens={len(tokens)} -> {a.out}", file=sys.stderr)

def mcount(k6, name):
    m = (k6.get("metrics") or {}).get(name) or {}
    return int(m.get("count") or (m.get("values") or {}).get("count") or 0)

def mrate(k6, name):
    m = (k6.get("metrics") or {}).get(name) or {}
    v = m.get("values") or {}
    return float(v.get("rate", m.get("rate", 0)) or 0)

def cmd_smoke(a):
    import time
    tag = f"smoke-{int(time.time())}"
    fail = 0

    def go(method, url, headers=None, body=None):
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(url, method=method, headers=headers or {}, data=data)
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                return resp.status, {k.lower(): v for k, v in resp.headers.items()}, resp.read().decode()
        except urllib.error.HTTPError as e:
            return e.code, {k.lower(): v for k, v in e.headers.items()}, e.read().decode()

    def check(ok, msg):
        nonlocal fail
        print(f"  [{'PASS' if ok else 'FAIL'}] {msg}")
        if not ok:
            fail += 1

    print(f"[smoke] base={a.base_url} tag={tag}")
    code, _, _ = go("GET", f"{a.base_url.rstrip('/')}/api/status")
    check(code == 200, f"GET /api/status -> {code}")

    code, _, raw = go("POST", f"{a.base_url.rstrip('/')}/api/token-apply",
        {"X-Api-Key": a.api_key, "Content-Type": "application/json"}, {
            "ticket_no": tag, "email": f"{tag}@example.com", "amount": 5000, "currency": "CNY",
            "org_code": "D001-SMOKE", "org_budget": 50000, "period_type": "day", "scope_type": "team",
            "token_type": "user", "work_no": "S1", "token_name": tag, "token_group": "default",
            "parent_org_code": "D001", "parent_org_budget": 100000})
    apply = json.loads(raw) if raw else {}
    tok = (apply.get("data") or {}).get("token_key", "")
    check(code == 200 and apply.get("success") and tok, "POST /api/token-apply -> issued key")

    if not tok:
        print(f"[{'PASS' if fail == 0 else 'FAIL'}] smoke aborted")
        sys.exit(1 if fail else 0)

    auth = {"Authorization": f"Bearer {tok}"}
    code, _, raw = go("GET", f"{a.base_url.rstrip('/')}/v1/models", auth)
    model = (json.loads(raw).get("data") or [{}])[0].get("id", "") if raw else ""
    check(code == 200 and model, f"GET /v1/models -> {model or 'empty'}")

    if not model:
        print(f"[{'PASS' if fail == 0 else 'FAIL'}] smoke aborted")
        sys.exit(1 if fail else 0)

    for stream in (False, True):
        label = "stream" if stream else "non-stream"
        code, hdrs, raw = go("POST", f"{a.base_url.rstrip('/')}/v1/chat/completions",
            {**auth, "Content-Type": "application/json"},
            {"model": model, "stream": stream, "max_tokens": 16, "messages": [{"role": "user", "content": "ping"}]})
        skip = hdrs.get("x-relay-skip-model-call", "")
        check(code == 200, f"POST /v1/chat/completions ({label}) -> {code}")
        check(str(skip).lower() == "true", f"mock header ({label}) -> {skip or 'missing'}")
        if not stream:
            usage = (json.loads(raw).get("usage") or {}) if raw else {}
            check(usage.get("total_tokens", 0) > 0, f"usage ({label}) -> total={usage.get('total_tokens', 0)}")
        elif raw and "data:" in raw:
            check(True, f"usage ({label}) -> sse chunks present")

    print(f"[{'PASS' if fail == 0 else 'FAIL'}] smoke done")
    sys.exit(1 if fail else 0)

def chat_once(base, token, model):
    import urllib.error, urllib.request
    body = json.dumps({"model": model, "stream": False, "max_tokens": 32,
        "messages": [{"role": "user", "content": "ping"}]}).encode()
    req = urllib.request.Request(f"{base.rstrip('/')}/v1/chat/completions", method="POST",
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"}, data=body)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return resp.status
    except urllib.error.HTTPError as e:
        return e.code

def cmd_calibrate(a):
    seed = json.loads(Path(a.seed).read_text())
    ticket = (seed.get("app") or [{}])[0].get("ticket_no") or (seed.get("user") or [{}])[0].get("ticket_no")
    token = (seed.get("app") or [{}])[0].get("token") or (seed.get("user") or [{}])[0].get("token")
    if not ticket or not token:
        raise SystemExit("[error] calibrate: empty seed pool")
    conn = pg()
    try:
        cur = conn.cursor()
        cur.execute("""SELECT t.id, t.used_quota FROM token_apply_records a
            JOIN tokens t ON t.id=a.token_id WHERE a.ticket_no=%s""", (ticket,))
        row = cur.fetchone()
        if not row:
            raise SystemExit(f"[error] calibrate: ticket not found {ticket}")
        token_id, before = int(row[0]), int(row[1] or 0)
        r = urllib.request.Request(f"{a.base_url.rstrip('/')}/v1/models",
            headers={"Authorization": f"Bearer {token}"})
        with urllib.request.urlopen(r, timeout=30) as resp:
            model = (json.loads(resp.read().decode()).get("data") or [{}])[0].get("id", "")
        if not model:
            raise SystemExit("[error] calibrate: no model")
        code = chat_once(a.base_url, token, model)
        if code != 200:
            raise SystemExit(f"[error] calibrate: chat HTTP {code}")
        cur.execute("SELECT used_quota FROM tokens WHERE id=%s", (token_id,))
        after = int((cur.fetchone() or [0])[0] or 0)
    finally:
        conn.close()
    delta = after - before
    out = {"ticket_no": ticket, "token_id": token_id, "quota_per_chat": delta, "model": model}
    Path(a.out).parent.mkdir(parents=True, exist_ok=True)
    Path(a.out).write_text(json.dumps(out, indent=2))
    print(f"[calibrate] quota_per_chat={delta} -> {a.out}", file=sys.stderr)

def org_used_deltas(before, after):
    def by_org(tokens):
        m = {}
        for t in tokens:
            oc = t.get("org_code") or "unknown"
            m[oc] = m.get(oc, 0) + int(t.get("used_quota") or 0)
        return m
    b, aft = by_org(before.get("tokens") or []), by_org(after.get("tokens") or [])
    return {oc: aft.get(oc, 0) - b.get(oc, 0) for oc in sorted(set(b) | set(aft))}

def cmd_cleanup(a):
    if os.environ.get("FORCE_CLEANUP") != "true":
        raise SystemExit("[error] set FORCE_CLEANUP=true to delete PT-* data for this tag")
    like = f"PT-{a.test_tag}%"
    conn = pg()
    try:
        cur = conn.cursor()
        cur.execute("SELECT id, token_id FROM token_apply_records WHERE ticket_no LIKE %s", (like,))
        rows = cur.fetchall()
        if not rows:
            print(f"[cleanup] no records for {like}", file=sys.stderr)
            return
        ids = [r[0] for r in rows]
        token_ids = [r[1] for r in rows if r[1]]
        ph = ",".join(["%s"] * len(ids))
        cur.execute(f"DELETE FROM token_apply_logs WHERE token_apply_id IN ({ph})", ids)
        cur.execute(f"DELETE FROM token_spend_policies WHERE token_apply_id IN ({ph})", ids)
        cur.execute(f"DELETE FROM token_apply_records WHERE id IN ({ph})", ids)
        if token_ids:
            tph = ",".join(["%s"] * len(token_ids))
            cur.execute(f"DELETE FROM tokens WHERE id IN ({tph})", token_ids)
        conn.commit()
        print(f"[cleanup] removed records={len(ids)} tokens={len(token_ids)} tag={a.test_tag}", file=sys.stderr)
    finally:
        conn.close()

def cmd_reconcile(a):
    before, after = json.loads(Path(a.before).read_text()), json.loads(Path(a.after).read_text())
    k6 = json.loads(Path(a.k6_summary).read_text()) if a.k6_summary else {}
    bt = {t["token_id"]: t for t in before["tokens"] if t.get("token_id")}
    at = {t["token_id"]: t for t in after["tokens"] if t.get("token_id")}
    used_d = sum(at[i]["used_quota"] - bt[i]["used_quota"] for i in at if i in bt)
    remain_d = sum(at[i]["remain_quota"] - bt[i]["remain_quota"] for i in at if i in bt)
    bp = {(p["scope_type"], p["scope_code"], p["token_type"], p["period_key"]): p for p in before["spend_policies"]}
    used_amt, cap_bad = 0.0, 0
    for key, p in {(x["scope_type"], x["scope_code"], x["token_type"], x["period_key"]): x for x in after["spend_policies"]}.items():
        if key not in bp: continue
        used_amt += float(p["used_amount"]) - float(bp[key]["used_amount"])
        if float(p.get("cap_amount") or 0) > 0 and float(p["used_amount"]) > float(p["cap_amount"]) + 1e-6:
            cap_bad += 1
    chat200, chat429 = mcount(k6, "chat_200_total"), mcount(k6, "chat_429_total")
    fail_rate, mock_rate = mrate(k6, "http_req_failed"), mrate(k6, "mock_header_rate")
    wb = {t["user_id"]: t["user_wallet_quota"] for t in before["tokens"] if t.get("user_id")}
    wa = {t["user_id"]: t["user_wallet_quota"] for t in after["tokens"] if t.get("user_id")}
    wallet_bad = [u for u in wb if u in wa and wb[u] != wa[u]]
    neg_budget = float(after.get("neg_budget_delta") or 0)
    cal = json.loads(Path(a.calibrate).read_text()) if getattr(a, "calibrate", "") and Path(a.calibrate).is_file() else {}
    qpc = int(cal.get("quota_per_chat") or 0)
    checks = [
        ("tokens", len(after["tokens"]) > 0, f"count={len(after['tokens'])}"),
        ("used_quota", chat200 == 0 or used_d > 0, f"chat200={chat200} delta={used_d}"),
        ("balance", used_d == -remain_d or used_d == 0, f"used={used_d} remain={remain_d}"),
        ("used_amount", used_amt >= -1e-6, f"delta={used_amt:.4f}"),
        ("cap", cap_bad == 0, f"violations={cap_bad}"),
        ("wallet", not wallet_bad, f"changed={wallet_bad[:3]}"),
        ("neg_budget_delta", neg_budget >= -1e-6, f"sum={neg_budget}"),
        ("chat_429", chat429 == 0 or used_amt >= -1e-6, f"429={chat429} used_amt={used_amt:.4f}"),
    ]
    if qpc > 0 and chat200 > 0:
        expected = chat200 * qpc
        tol = max(chat200, int(expected * 0.05))
        checks.append(("quota_exact", abs(used_d - expected) <= tol,
            f"used={used_d} expected={expected} qpc={qpc} tol={tol}"))
    if k6:
        checks += [
            ("http_req_failed", fail_rate < 0.02, f"rate={fail_rate:.4f}"),
            ("mock_header", mock_rate == 0 or mock_rate > 0.99, f"rate={mock_rate:.4f}"),
        ]
    passed = all(c[1] for c in checks)
    for name, ok, detail in checks:
        print(f"  [{'PASS' if ok else 'FAIL'}] {name}: {detail}")
    report = {"pass": passed, "checks": [{"name": n, "pass": o, "detail": d} for n, o, d in checks],
        "delta": {"used_quota": used_d, "remain_quota": remain_d, "used_amount": round(used_amt, 4),
                  "chat_200": chat200, "chat_429": chat429, "quota_per_chat": qpc},
        "org_used_delta": org_used_deltas(before, after)}
    Path(a.out).write_text(json.dumps(report, indent=2, ensure_ascii=False))
    print(f"[reconcile] {'PASS' if passed else 'FAIL'} -> {a.out}", file=sys.stderr)
    sys.exit(0 if passed else 1)

def cmd_report(a):
    k6 = json.loads(Path(a.k6_summary).read_text())
    rec = json.loads(Path(a.reconcile).read_text()) if Path(a.reconcile).is_file() else {}
    m = k6.get("metrics") or {}
    def mc(n): return int(((m.get(n) or {}).get("values") or {}).get("count", (m.get(n) or {}).get("count", 0)) or 0)
    def mp(n, p):
        v = (m.get(n) or {}).get("values") or {}
        return v.get(p, (m.get(n) or {}).get(p))
    lines = [f"# Pressure Report — {a.test_tag}", "", f"Generated: {datetime.now().isoformat(timespec='seconds')}", "",
        "## k6", "", f"- http_reqs: {mc('http_reqs')}", f"- chat_200: {mc('chat_200_total')}",
        f"- chat_429: {mc('chat_429_total')}", f"- chat_stream: {mc('chat_stream_total')}",
        f"- http_req_duration p95: {mp('http_req_duration', 'p(95)')}", f"- http_req_duration p99: {mp('http_req_duration', 'p(99)')}", ""]
    org_delta = rec.get("org_used_delta") or {}
    if org_delta:
        lines += ["## Per-org used_quota delta", ""]
        for oc, d in sorted(org_delta.items()):
            lines.append(f"- {oc}: {d}")
        lines.append("")
    if rec:
        lines += [f"## Reconcile: **{'PASS' if rec.get('pass') else 'FAIL'}**", ""]
        for c in rec.get("checks") or []:
            lines.append(f"- {'✅' if c.get('pass') else '❌'} {c.get('name')}: {c.get('detail')}")
    Path(a.out).write_text("\n".join(lines) + "\n")
    print(f"[report] -> {a.out}", file=sys.stderr)

p = argparse.ArgumentParser()
sub = p.add_subparsers(dest="cmd", required=True)
s = sub.add_parser("seed")
for k, t, d in [("--base-url", str, None), ("--api-key", str, None), ("--test-tag", str, None),
    ("--org-count", int, None), ("--app-per-org", int, None), ("--user-count", int, None),
    ("--org-budget", int, 500000), ("--key-amount", int, 50000), ("--workers", int, 10), ("--out", str, None)]:
    kw = {"type": t}
    if d is not None: kw["default"] = d
    s.add_argument(k, **kw)
s.add_argument("--org-prefix", default="D001-T")
s.add_argument("--period-type", default="day")
s.add_argument("--parent-org-code", default="D001")
s.add_argument("--parent-org-budget", type=int, default=1000000)
s.add_argument("--increase-delta", type=int, default=100)
s = sub.add_parser("snapshot"); s.add_argument("--test-tag"); s.add_argument("--org-prefix", default="D001-T"); s.add_argument("--out")
s = sub.add_parser("smoke"); s.add_argument("--base-url"); s.add_argument("--api-key")
s = sub.add_parser("calibrate"); s.add_argument("--base-url"); s.add_argument("--seed"); s.add_argument("--test-tag"); s.add_argument("--out")
s = sub.add_parser("cleanup"); s.add_argument("--test-tag")
s = sub.add_parser("reconcile"); s.add_argument("--before"); s.add_argument("--after"); s.add_argument("--k6-summary", default=""); s.add_argument("--calibrate", default=""); s.add_argument("--out")
s = sub.add_parser("report"); s.add_argument("--test-tag"); s.add_argument("--k6-summary"); s.add_argument("--reconcile"); s.add_argument("--out")
a = p.parse_args()
{"seed": cmd_seed, "snapshot": cmd_snapshot, "smoke": cmd_smoke, "calibrate": cmd_calibrate,
 "cleanup": cmd_cleanup, "reconcile": cmd_reconcile, "report": cmd_report}[a.cmd](a)
PY
}

run_seed() {
  paths; mkdir -p "$REPORT_DIR"
  py seed --base-url "$BASE_URL" --api-key "$TOKEN_API_KEY" --test-tag "$TEST_TAG" \
    --org-count "$ORG_COUNT" --app-per-org "$APP_PER_ORG" --user-count "$USER_COUNT" \
    --org-prefix "$ORG_PREFIX" --increase-delta "$INCREASE_DELTA" --out "$SEED_FILE"
}

run_k6() {
  local soak="$1" burst="$2"
  local side; side="$(side_duration "$soak" "$burst")"
  local write_dur="${APPLY_WRITE_DURATION:-$side}"
  local portal_dur="${PORTAL_READ_DURATION:-$side}"
  local inc_dur="${INCREASE_DURATION:-$side}"
  paths; mkdir -p "$REPORT_DIR"
  [[ "$RECONCILE" == true ]] && py snapshot --test-tag "$TEST_TAG" --org-prefix "$ORG_PREFIX" --out "$BILL_BEFORE"
  if [[ "$RECONCILE" == true ]]; then
    py calibrate --base-url "$BASE_URL" --seed "$SEED_FILE" --test-tag "$TEST_TAG" --out "$CALIBRATE" || true
  fi
  RUN_SOAK="$soak" RUN_BURST="$burst" \
  RUN_APPLY_WRITE="$RUN_APPLY_WRITE" RUN_APPLY_INCREASE="$RUN_APPLY_INCREASE" \
  RUN_PORTAL_READ="$RUN_PORTAL_READ" PORTAL_AUTH_TOKEN="$PORTAL_AUTH_TOKEN" \
  STREAM_RATIO="$STREAM_RATIO" ORG_PREFIX="$ORG_PREFIX" INCREASE_DELTA="$INCREASE_DELTA" \
  APPLY_WRITE_RPS="$APPLY_WRITE_RPS" APPLY_WRITE_DURATION="$write_dur" \
  INCREASE_VUS="$INCREASE_VUS" INCREASE_DURATION="$inc_dur" \
  PORTAL_READ_RPS="$PORTAL_READ_RPS" PORTAL_READ_DURATION="$portal_dur" \
  BASE_URL="$BASE_URL" SEED_FILE="$SEED_FILE" TOKEN_API_KEY="$TOKEN_API_KEY" \
  TARGET_RPS="${TARGET_RPS:-100}" SOAK_DURATION="${SOAK_DURATION:-10m}" PEAK_VUS="${PEAK_VUS:-400}" \
  SOAK_MAX_VUS="${SOAK_MAX_VUS:-300}" MOCK_MODE="${MOCK_MODE:-true}" \
  k6 run --summary-export "$K6_SUMMARY" "$SCRIPT_DIR/pressure.k6.js"
  if [[ "$RECONCILE" == true ]]; then
    py snapshot --test-tag "$TEST_TAG" --org-prefix "$ORG_PREFIX" --out "$BILL_AFTER"
    py reconcile --before "$BILL_BEFORE" --after "$BILL_AFTER" --k6-summary "$K6_SUMMARY" \
      --calibrate "$CALIBRATE" --out "$RECON_OUT" || true
    py report --test-tag "$TEST_TAG" --k6-summary "$K6_SUMMARY" --reconcile "$RECON_OUT" --out "$REPORT_MD"
    [[ -f "$RECON_OUT" ]] && ! python3 -c "import json,sys; sys.exit(0 if json.load(open('$RECON_OUT'))['pass'] else 1)" && exit 1
  fi
  echo "[done] $K6_SUMMARY $REPORT_MD"
}

case "$MODE" in
  smoke) py smoke --base-url "$BASE_URL" --api-key "$TOKEN_API_KEY" ;;
  seed) run_seed ;;
  soak)  [[ -z "$RECONCILE" ]] && RECONCILE=true; [[ -f "${SEED_FILE:-}" ]] || run_seed; paths; run_k6 true false ;;
  burst) [[ -z "$RECONCILE" ]] && RECONCILE=true; [[ -f "${SEED_FILE:-}" ]] || run_seed; paths; run_k6 false true ;;
  full)  RECONCILE=true; run_seed; paths; run_k6 true true ;;
  cleanup)
    [[ -z "$TOKEN_API_KEY" ]] && { echo "[error] TOKEN_API_KEY required"; exit 1; }
    paths; py cleanup --test-tag "$TEST_TAG"
    ;;
  *) echo "MODE=smoke|seed|soak|burst|full|cleanup  SCALE=dev|prod"; exit 1 ;;
esac
