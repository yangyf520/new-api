import http from 'k6/http';
import exec from 'k6/execution';
import { check, sleep } from 'k6';
import { Counter, Rate } from 'k6/metrics';
import { SharedArray } from 'k6/data';

const businessOk = new Rate('business_success_rate');
const mockOk = new Rate('mock_header_rate');
const chat200 = new Counter('chat_200_total');
const chat429 = new Counter('chat_429_total');
const chatStream = new Counter('chat_stream_total');
const capHit = new Counter('cap_429_total');

const BASE_URL = __ENV.BASE_URL;
const TIMEOUT = __ENV.REQUEST_TIMEOUT || '60s';
const MOCK = (__ENV.MOCK_MODE || 'true') === 'true';
const STREAM_RATIO = Number(__ENV.STREAM_RATIO || 0);
const CHAT_HISTORY_ROUNDS = Number(__ENV.CHAT_HISTORY_ROUNDS || 8);
const CHAT_CONTENT_KB = Number(__ENV.CHAT_CONTENT_KB || 4);
const CHAT_OUTPUT_KB = Number(__ENV.CHAT_OUTPUT_KB || 32);
const MAX_TOKENS = Number(__ENV.MAX_TOKENS || CHAT_OUTPUT_KB * 256);

function kbText(kb, tag) {
  const unit = `pressure-test-${tag}-`;
  const target = Math.max(1, kb) * 1024;
  let out = '';
  while (out.length < target) {
    out += unit;
  }
  return out.slice(0, target);
}

function buildChatMessages() {
  const messages = [];
  for (let i = 0; i < CHAT_HISTORY_ROUNDS; i++) {
    messages.push({ role: 'user', content: kbText(CHAT_CONTENT_KB, `user-${i}`) });
    messages.push({ role: 'assistant', content: kbText(CHAT_CONTENT_KB, `assistant-${i}`) });
  }
  messages.push({ role: 'user', content: kbText(CHAT_CONTENT_KB, 'final') });
  return messages;
}

const chatMessages = buildChatMessages();
const seed = JSON.parse(open(__ENV.SEED_FILE));
const orgCount = Number(seed.org_count || 20);
const orgPrefix = __ENV.ORG_PREFIX || 'D001-T';

const appPool = new SharedArray('app', () => seed.app || []);
const userPool = new SharedArray('user', () => seed.user || []);
const increaseTargets = new SharedArray('inc', () => {
  if (seed.increase_targets?.length) return seed.increase_targets;
  const seen = new Set();
  return (seed.user || []).filter((u) => {
    if (!u.token_apply_id || seen.has(u.org_code)) return false;
    seen.add(u.org_code);
    return true;
  });
});
const n07Target = seed.n07_target || null;
const increaseDelta = Number(__ENV.INCREASE_DELTA || seed.increase_delta || 100);

function pick(pool) {
  return pool[(exec.vu.idInTest - 1) % pool.length];
}

function orgCode(idx) {
  return `${orgPrefix}${String(idx).padStart(3, '0')}`;
}

function chat(token, model, tags) {
  const useStream = STREAM_RATIO > 0 && exec.scenario.iterationInTest % 100 < STREAM_RATIO;
  const body = {
    model,
    stream: useStream,
    max_tokens: MAX_TOKENS,
    messages: chatMessages,
  };
  const r = http.post(`${BASE_URL}/v1/chat/completions`, JSON.stringify(body), {
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    timeout: TIMEOUT,
    tags: { ...tags, stream: String(useStream) },
  });
  const ok = r.status === 200 || r.status === 429;
  businessOk.add(ok);
  if (r.status === 200) {
    chat200.add(1);
    if (useStream) chatStream.add(1);
    if (MOCK) mockOk.add(r.headers['X-Relay-Skip-Model-Call'] === 'true');
  }
  if (r.status === 429) {
    chat429.add(1);
    capHit.add(1);
  }
  check(r, {
    ok: () => ok,
    mock: () => !MOCK || r.status === 429 || r.headers['X-Relay-Skip-Model-Call'] === 'true',
  });
}

export function setup() {
  const token = appPool[0]?.token || userPool[0]?.token;
  const r = http.get(`${BASE_URL}/v1/models`, {
    headers: { Authorization: `Bearer ${token}` },
    timeout: TIMEOUT,
  });
  const model = JSON.parse(r.body).data?.[0]?.id;
  if (!model) throw new Error('no model');
  return { model };
}

const scenarios = {};
if (__ENV.RUN_SOAK === 'true') {
  scenarios.app_soak = {
    executor: 'constant-arrival-rate',
    rate: Number(__ENV.TARGET_RPS || 100),
    timeUnit: '1s',
    duration: __ENV.SOAK_DURATION || '10m',
    preAllocatedVUs: 50,
    maxVUs: Number(__ENV.SOAK_MAX_VUS || 300),
    exec: 'appSoak',
    tags: { workload: 'app' },
  };
}
if (__ENV.RUN_BURST === 'true') {
  const peak = Number(__ENV.PEAK_VUS || 400);
  scenarios.user_burst = {
    executor: 'ramping-vus',
    startVUs: 1,
    stages: [
      { duration: __ENV.BURST_RAMP_UP || '2m', target: Math.round(peak * 0.5) },
      { duration: __ENV.BURST_SPIKE || '2m', target: Math.round(peak * 1.8) },
      { duration: __ENV.BURST_HOLD || '4m', target: peak },
    ],
    gracefulRampDown: __ENV.BURST_RAMP_DOWN || '30s',
    exec: 'userBurst',
    tags: { workload: 'user' },
  };
}
if (__ENV.RUN_APPLY_WRITE === 'true') {
  scenarios.apply_write = {
    executor: 'constant-arrival-rate',
    rate: Number(__ENV.APPLY_WRITE_RPS || 5),
    timeUnit: '1s',
    duration: __ENV.APPLY_WRITE_DURATION || '5m',
    preAllocatedVUs: 5,
    maxVUs: 20,
    exec: 'applyWrite',
    tags: { workload: 'apply_write' },
  };
}
if (__ENV.RUN_APPLY_INCREASE === 'true' && increaseTargets.length) {
  const vus = Number(__ENV.INCREASE_VUS || 10);
  scenarios.apply_increase = {
    executor: 'constant-arrival-rate',
    rate: Math.max(1, Math.round(vus / 10)),
    timeUnit: '1s',
    duration: __ENV.INCREASE_DURATION || '5m',
    preAllocatedVUs: 5,
    maxVUs: Math.max(vus, 10),
    exec: 'applyIncrease',
    tags: { workload: 'apply_increase' },
  };
}
if (__ENV.RUN_APPLY_INCREASE === 'true' && n07Target) {
  scenarios.apply_n07 = {
    executor: 'shared-iterations',
    vus: 2,
    iterations: 2,
    maxDuration: __ENV.INCREASE_DURATION || '2m',
    exec: 'applyN07',
    tags: { workload: 'apply_n07' },
  };
}
if (__ENV.RUN_PORTAL_READ === 'true' && __ENV.PORTAL_AUTH_TOKEN) {
  scenarios.portal_read = {
    executor: 'constant-arrival-rate',
    rate: Number(__ENV.PORTAL_READ_RPS || 10),
    timeUnit: '1s',
    duration: __ENV.PORTAL_READ_DURATION || '5m',
    preAllocatedVUs: 5,
    maxVUs: 30,
    exec: 'portalRead',
    tags: { workload: 'portal_read' },
  };
}

export const options = {
  scenarios,
  thresholds: {
    http_req_failed: ['rate<0.02'],
    business_success_rate: ['rate>0.99'],
    mock_header_rate: MOCK ? ['rate>0.99'] : [],
    http_req_duration: ['p(95)<2000', 'p(99)<4000'],
  },
};

export function appSoak({ model }) {
  const item = pick(appPool);
  chat(item.token, model, { workload: 'app', org_code: item.org_code });
}

export function userBurst({ model }) {
  const item = pick(userPool);
  chat(item.token, model, { workload: 'user', org_code: item.org_code });
  sleep(0.05);
}

export function applyWrite() {
  const n = `${Date.now()}-${exec.vu.idInTest}-${exec.scenario.iterationInTest}`;
  const org = orgCode((exec.vu.idInTest % orgCount) + 1);
  const body = {
    ticket_no: `PT-WRITE-${n}`,
    email: `write+${n}@example.com`,
    amount: 1000,
    currency: 'CNY',
    org_code: org,
    org_budget: 500000,
    period_type: 'day',
    scope_type: 'team',
    token_type: 'user',
    work_no: `W${n}`.slice(0, 20),
    token_name: `write-${n}`,
    token_group: org,
    remark: 'pressure apply write',
  };
  const r = http.post(`${BASE_URL}/api/token-apply`, JSON.stringify(body), {
    headers: { 'X-Api-Key': __ENV.TOKEN_API_KEY, 'Content-Type': 'application/json' },
    timeout: TIMEOUT,
    tags: { workload: 'apply_write', org_code: org },
  });
  businessOk.add(r.status >= 200 && r.status < 300);
  check(r, { apply_write: (x) => x.status >= 200 && x.status < 300 });
}

export function applyIncrease() {
  const t = increaseTargets[exec.scenario.iterationInTest % increaseTargets.length];
  const n = `${Date.now()}-${exec.vu.idInTest}-${exec.scenario.iterationInTest}`;
  const amount = Number(t.increase_to || (t.current_amount || 0) + increaseDelta);
  const r = http.put(`${BASE_URL}/api/token-apply/${t.token_apply_id}`, JSON.stringify({
    change_ticket_no: `PT-INC-${n}`,
    amount,
    currency: 'CNY',
    remark: 'pressure concurrent increase',
  }), {
    headers: { 'X-Api-Key': __ENV.TOKEN_API_KEY, 'Content-Type': 'application/json' },
    timeout: TIMEOUT,
    tags: { workload: 'apply_increase', org_code: t.org_code },
  });
  businessOk.add(r.status >= 200 && r.status < 300);
  check(r, { apply_increase: (x) => x.status >= 200 && x.status < 300 });
}

export function applyN07() {
  const t = n07Target;
  const n = `${Date.now()}-n07-${exec.vu.idInTest}`;
  const amount = Number(t.increase_to || (t.current_amount || 0) + increaseDelta);
  const r = http.put(`${BASE_URL}/api/token-apply/${t.token_apply_id}`, JSON.stringify({
    change_ticket_no: `PT-N07-${n}`,
    amount,
    currency: 'CNY',
    remark: 'pressure N-07 same-record concurrent increase',
  }), {
    headers: { 'X-Api-Key': __ENV.TOKEN_API_KEY, 'Content-Type': 'application/json' },
    timeout: TIMEOUT,
    tags: { workload: 'apply_n07', org_code: t.org_code },
  });
  const ok = r.status >= 200 && r.status < 300;
  businessOk.add(ok);
  check(r, { apply_n07: () => ok });
}

export function portalRead() {
  const org = orgCode((exec.vu.idInTest % orgCount) + 1);
  const paths = [
    `/api/token-apply/records?org_code=${org}&page=1&page_size=20`,
    `/api/token-apply/budget?org_code=${org}`,
    `/api/token-apply/consumption?org_code=${org}`,
  ];
  const path = paths[exec.scenario.iterationInTest % paths.length];
  const r = http.get(`${BASE_URL}${path}`, {
    headers: { Authorization: `Bearer ${__ENV.PORTAL_AUTH_TOKEN}` },
    timeout: TIMEOUT,
    tags: { workload: 'portal_read', org_code: org },
  });
  businessOk.add(r.status >= 200 && r.status < 300);
  check(r, { portal_read: (x) => x.status >= 200 && x.status < 300 });
}
