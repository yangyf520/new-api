# 压测说明

> [pressure-run.sh](./pressure-run.sh) · [pressure-test.sh](./pressure-test.sh) · [pressure.k6.js](./pressure.k6.js) · [压测报告](./pressure-report.md)

## 一键跑

```bash
# 三个参数：Key 数、RPS、时长（先凑满 Key，再压测；未齐则中止）
KEYS=300 RPS=300 DURATION=10m bash docs/tests/performance/pressure-run.sh
```

依赖：`k6`、`python3`、`.env` 中 `TOKEN_API_KEY`。生产需 `RELAY_SKIP_MODEL_CALL=true`。

复用已有 Key 池：

```bash
SEED_FILE=$PWD/docs/tests/performance/cases/seed-xxx.json \
  KEYS=300 RPS=300 DURATION=10m bash docs/tests/performance/pressure-run.sh
```

产出在 `cases/`（本地，不提交 Git）：`seed-<TAG>.json`、`summary-<TAG>.json`。

---

## 参数

| 变量 | 必填 | 默认 | 说明 |
|------|------|------|------|
| `KEYS` | ✅ | — | app Key 总数（须为 10 的倍数，默认每部门 10 个） |
| `RPS` | ✅ | — | 目标请求速率 |
| `DURATION` | ✅ | — | 时长，如 `10m`、`3m` |
| `TEST_TAG` | | `pt-时间戳` | 本次标签 |
| `SEED_FILE` | | 自动 | 已有 seed 则跳过申请 |
| `BASE_URL` | | 生产地址 | |
| `ORG_PREFIX` | | `D001-SMOKE` | 部门编码前缀 |
| `SEED_INTERVAL` | | `1.2` | 每个 Key 申请间隔（秒），避开 180/3min 限流 |
| `SEED_BATCH_SIZE` | | `50` | 每批申请数，批间再暂停 |
| `SEED_BATCH_PAUSE` | | `5` | 批间暂停（秒） |
| `SEED_MAX_RETRY` | | `8` | 遇 429 退避重试次数 |
| `SOAK_MAX_VUS` | | `500` | k6 最大 VU |

---

## 压测内容

- 接口：`POST /v1/chat/completions`（mock，不打真实模型）
- Payload：8 轮历史 × 4KB + mock 输出 ~32KB
- 链路：鉴权 → 计费 → 大字段落库

底层脚本 `pressure-test.sh` 另支持 `MODE=smoke|seed|soak|cleanup` 等，见源码注释。

---

## 清理

```bash
TEST_TAG=pt-xxx FORCE_CLEANUP=true MODE=cleanup bash docs/tests/performance/pressure-test.sh
```
