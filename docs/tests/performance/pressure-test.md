# 压测说明

> [pressure-test.sh](./pressure-test.sh) · [pressure.k6.js](./pressure.k6.js)

---

## 快速开始

```bash
pip install psycopg2-binary
MODE=smoke bash docs/tests/performance/pressure-test.sh
MODE=full SCALE=prod PORTAL_AUTH_TOKEN="<jwt>" bash docs/tests/performance/pressure-test.sh
```

---

## 功能清单

| 功能 | MODE / 开关 |
|------|-------------|
| 连通性 smoke（非流式+流式 chat + mock header） | `MODE=smoke` |
| 多部门并发 seed（app + user + parent_org + n07_target） | `MODE=seed` |
| app 底噪 soak | `MODE=soak` |
| user 突发 burst | `MODE=burst` |
| 全量 full | `MODE=full` |
| 流式 chat 混合 | `STREAM_RATIO=30`（30% 请求 stream=true） |
| 压测中并发发 Key | `RUN_APPLY_WRITE=true` |
| 多部门并发增额 PUT | `RUN_APPLY_INCREASE=true` |
| N-07 同一笔双增额 | `RUN_APPLY_INCREASE=true`（`apply_n07` 场景） |
| 门户只读 | `RUN_PORTAL_READ=true` + `PORTAL_AUTH_TOKEN` |
| 压测前 quota 校准 + 精确对账 | `RECONCILE=true`（默认 soak/burst/full） |
| 分部门 used_quota 报告 | 自动写入 `report-<TAG>.md` |
| 清理测试数据 | `MODE=cleanup FORCE_CLEANUP=true TEST_TAG=...` |

`MODE=full SCALE=prod` 默认开启：`RUN_APPLY_WRITE`、`RUN_APPLY_INCREASE`、`STREAM_RATIO=30`；有 `PORTAL_AUTH_TOKEN` 时开启门户只读。写/增额/门户场景时长与 soak 对齐（默认 2h）。

---

## SCALE=prod 默认

| 项 | 值 |
|----|-----|
| 部门 / app / user | 20 / 10 / **300** |
| soak | 120 RPS × 2h |
| burst | 500 VU |
| 增额幅度 | `INCREASE_DELTA=100`（元） |

---

## 产出物

`reports/` 目录：

- `seed-<TAG>.json` — Key 池 + `increase_targets` + `n07_target`
- `calibrate-<TAG>.json` — 单次 chat 的 `quota_per_chat`
- `summary-<TAG>.json` — k6 摘要（含 p95/p99）
- `billing-before/after-<TAG>.json` — DB 快照
- `reconcile-<TAG>.json` — 对账（`pass: true` 为通过）
- `report-<TAG>.md` — 人类可读报告（含分部门 delta）

---

## 对账规则

- `used_quota` 随 `chat_200` 增长；`remain_quota` 守恒
- `quota_exact`：`used_quota_delta ≈ chat_200 × quota_per_chat`（±5%）
- `users.quota` 钱包不变
- `used_amount` 不超 cap；429 不导致异常扣费
- `budget_delta` 无负值（减额拒绝）
- k6：`http_req_failed < 2%`，`mock_header_rate > 99%`，`p95 < 2s`，`p99 < 4s`

详见 [token-apply-test.md §3.6 / §10](../token-apply/token-apply-test.md)。

---

## 环境变量

| 变量 | 说明 |
|------|------|
| `TOKEN_API_KEY` | `.env` 或 export |
| `SQL_DSN` | 对账 / 校准 / cleanup 用 PostgreSQL |
| `PORTAL_AUTH_TOKEN` | 部门管理员 JWT（非 API Key） |
| `USER_KEY_COUNT` | 覆盖 user 抽样数（可设 500） |
| `SEED_FILE` | 复用已有 seed |
| `RECONCILE` | 默认 soak/burst/full 为 true |
| `INCREASE_DELTA` | 增额 PUT 的目标增量（元），默认 100 |
| `FORCE_CLEANUP` | `MODE=cleanup` 时必须为 `true` |

---

## 清理测试数据

```bash
TEST_TAG=pt-20260702-120000 FORCE_CLEANUP=true MODE=cleanup \
  bash docs/tests/performance/pressure-test.sh
```

仅删除 `ticket_no LIKE 'PT-<TAG>%'` 的台账、日志、消耗策略与关联 token。
