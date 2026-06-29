# 压测说明（token-apply + Relay）

> 脚本：[pressure-test.sh](./pressure-test.sh)  
> 设计：[../../design/token-apply-design.md](../../design/token-apply-design.md)  
> 功能测试：[../token-apply/token-apply-test.md](../token-apply/token-apply-test.md)

---

## 0. 环境与凭据

| 项 | 值 / 来源 | 说明 |
|----|-----------|------|
| **测试地址** | `https://ai.sensetime-inc.com` | `pressure-test.sh` 默认 `BASE_URL`；可用环境变量覆盖 |
| **集成方 Key** | 项目根 `.env` → `TOKEN_API_KEY` | 脚本自动读取；与运营设置 `token_apply_setting.api_key` 一致 |
| **Relay Key** | 压测时脚本现场发放 | `POST /api/token-apply` 返回的 `sk-...`，非 Api-Key |

```bash
# 本地已有 .env 时，一般只需：
cd /path/to/new-api
bash docs/tests/performance/pressure-test.sh

# 显式指定（连其他环境时）
export BASE_URL="https://ai.sensetime-inc.com"
export TOKEN_API_KEY="<与 .env 或运营后台一致>"
```

> **安全：** `TOKEN_API_KEY` **不入库**（见 `.env.example` 注释）；文档与脚本均不写明文 Key。

---

| 维度 | 目标 |
|------|------|
| 人员 | ~5000 人 |
| 部门 | 20 个 `org_code` |
| 应用 | 每部门多个 `token_type=app` Key，**7×24 常驻调用** |
| 用户 Key | 每人 1 个 `token_type=user` Key（约 5000，交互式、偏突发） |

### 1.1 负载模型（两类流量）

| 类型 | `token_type` | 特征 | 压测侧重 |
|------|--------------|------|----------|
| **应用** | `app` | 服务常驻、持续打 Relay；是**底噪 + 主力吞吐** | 长时间 soak、恒定 QPS |
| **用户** | `user` | 人工偶发、上下班波峰 | 短时 burst、峰值 VU |

**估算示例（按每部门 10 个应用、每应用均值 0.5 QPS）：**

```text
20 部门 × 10 应用 × 0.5 QPS ≈ 100 QPS  sustained（底噪）
+ 用户峰时突发（如 200～500 并发会话）→ 合计峰值更高
```

压测验收应覆盖：**应用持续负载（stability/soak）** + **用户峰时叠加（burst）**，不能只按「5000 人 × 在线率」估算。

---

## 2. 当前脚本能力

**流程：** 串行 `POST /api/token-apply` 发 Key → k6 压 `POST /v1/chat/completions`

| 参数 | 默认 | 说明 |
|------|------|------|
| `KEY_COUNT` | 10 | 发放 Key 数量 |
| `ORG_CODE` | `D001-T010` | 单一部门 |
| `PEAK_VUS` | 120 | k6 峰值虚拟用户 |
| `PROFILE` | `capacity` | `baseline` / `capacity` / `stability` / `burst` |
| `MODEL_NAME` | `auto` | `auto` 时 setup 调 `/v1/models` 取首个模型 |

**通过阈值（k6）：**

- `http_req_failed` < 2%
- `business_success_rate` > 99%（含 429 计成功）
- `http_req_duration` p95 < 6s，p99 < 12s

**报告输出：** `docs/tests/performance/reports/pressure-summary-<TEST_TAG>.json`

---

## 3. 覆盖 vs 缺口

| 场景 | 当前 | 目标规模 |
|------|:----:|:--------:|
| Relay chat 并发 | ✅ 少量 Key | 需数百 app Key 轮询 |
| 应用持续负载（soak） | ❌ 仅短时 ramp | 需 `constant-arrival-rate` + 长时运行 |
| 单部门 | ✅ | 需 20 部门 |
| `token_type=app` | ❌ 仅 user | **主力**，需占 seed 大部分 |
| token-apply 写并发 | ❌ 压测前串行 | 需 POST/PUT 并发 |
| 门户只读 API | ❌ | `/records` `/budget` `/consumption` |
| 父子总包 | ❌ | `parent_org_code` |
| 并发增额 N-07 | ❌ | 行锁 + 预算正确性 |

**结论：** 现有脚本 = **Relay 小规模冒烟**；**不能**作为 5000 人 / 20 部门 / 多应用验收依据。

---

## 4. 快速执行

```bash
# 依赖：k6、curl、python3
# 凭据见 §0：默认 BASE_URL + 项目根 .env 的 TOKEN_API_KEY

# 默认（10 Key、120 VU、单部门）
bash docs/tests/performance/pressure-test.sh

# 提高 Relay 并发
PEAK_VUS=400 PROFILE=capacity bash docs/tests/performance/pressure-test.sh

# 稳定性（长时间，模拟应用底噪）
PEAK_VUS=300 PROFILE=stability bash docs/tests/performance/pressure-test.sh
```

> 当前 `PROFILE=stability` 仅 10 分钟；真实应用常驻需 **≥2h soak**（脚本待扩展）。

---

## 5. 目标规模压测清单（待实现）

### 5.1 数据准备（seed）

```text
20 部门 × (org_budget + spend_policy + 日/月消耗封顶)
每部门：10～50 个 app Key（常驻）+ 少量 user Key（抽样即可）
app 合计：200～1000（按实际应用数；这是压测主力 Key 池）
user 合计：可抽样 200～500，不必全量 5000（功能测试已覆盖）
```

### 5.2 场景拆分

| 场景 | 路径 | 模型 | 关注点 |
|------|------|------|--------|
| **应用底噪** | `/v1/chat/completions` | `constant-arrival-rate`，每 app Key 绑定固定 QPS，≥2h | ③消耗累计、内存/连接泄漏 |
| **用户突发** | 同上 | `ramping-vus`，叠加在底噪之上 | 峰时延迟、429 比例 |
| token-apply 写 | `POST` / `PUT` | 低 QPS 并发 | 幂等、行锁、①预算 |
| 门户读 | `/records` 等 | 部门管理员定时刷新 | 分页、隔离 |

**k6 建议：** app 场景用 `constant-arrival-rate`（按总 QPS 驱动）；每个 VU 固定绑定一个 app Key，模拟真实「一应用一 Key 持续打」。

### 5.3 观测

- k6 按 `token_type` / `org_code` 分组延迟、QPS、429 率
- soak 期间监控：goroutine、DB 连接池、Redis、上游 channel 错误率
- pprof：`go tool pprof -http=:8080 pprof/cpu-*.pprof`
- 对账：`token_spend_policies.used_amount` 与日志是否线性增长、无跳变

### 5.4 建议起步参数

```bash
# 阶段 1：app 底噪（200 Key × 0.5 QPS ≈ 100 QPS）
APP_KEY_COUNT=200
TARGET_RPS=100
SOAK_DURATION=2h

# 阶段 2：叠加用户突发
PEAK_VUS=400 PROFILE=burst

# 脚本待扩展：app/user 分池、constant-arrival-rate、20 org_code
```

---

## 6. 与功能测试关系

| 类型 | 文档 | 职责 |
|------|------|------|
| 金钱 / 逻辑正确性 | [token-apply-test.md](../token-apply/token-apply-test.md) | M-* / N-* 用例 |
| 并发 / 容量 | 本文 + `pressure-test.sh` | 吞吐、延迟、稳定性 |

压测通过 ≠ 业务正确；须先过功能测试，再跑容量压测。
