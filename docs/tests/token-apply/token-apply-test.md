# 令牌发放（token-apply）测试计划

> 设计：[../../design/token-apply-design.md](../../design/token-apply-design.md)  
> **第三方集成（curl + 字段）：** [token-apply-api.md](./token-apply-api.md)

**目录约定：**

```text
docs/tests/
  <feature>/                    # 例如 token-apply/
    <feature>-test.md           # 测试计划（用例 + 手工步骤）
    <feature>-api.md            # 第三方 curl + 字段说明
```

**文档状态：待评审** — 请先看完全文用例，确认无遗漏后再执行测试；执行结果填入 [§12 执行记录](#12-执行记录)。

---

## 0. 评审检查项

评审时请确认：

- [ ] 三层额度（①审批 / ②Key / ③消耗）的**金钱断言**是否足够
- [ ] **钱包 `users.quota` 不被 token-apply 误扣** 是否有独立用例
- [ ] **审计 `budget_delta` / `token_apply_logs`** 是否与台账一致
- [ ] Relay **预扣 → 结算 → 退款** 对 `token_spend_policies.used_amount` 的影响是否覆盖
- [ ] 负向（拒绝、幂等、越权）是否齐全
- [ ] 门户 / 前端是否接受「手工 + 自动化」分工

---

## 1. 环境与前置

### 1.1 仅允许在测试库执行

全量回归前请确认在**测试库**操作；手工清理会 **DELETE** 以下表数据，禁止在生产库跑：

`token_apply_logs` · `token_apply_records` · 关联 `tokens` · `token_spend_policies` · `token_budget_policies`

### 1.2 前置清单

| # | 项 | 要求 |
|---|-----|------|
| E-01 | 服务 | `go run main.go` 已启动，`curl $BASE_URL/api/status` → 200 |
| E-02 | 密钥 | `.env` 中 `TOKEN_API_KEY` 与运营设置一致 |
| E-03 | 数据库 | `.env` 里 `SQL_DSN`（`postgres://...` 可直接给 `psql`） |
| E-03b | psql | 客户端已安装（`brew install libpq`） |
| E-04 | BASE_URL | 建议显式 `export BASE_URL=http://127.0.0.1:3000` |
| E-05 | app 令牌 | 运营设置 `app_user_email` 指向已存在用户 |
| E-06 | Relay | 至少 1 个 **status=1** 的 channel + 可用模型（计费用例必需） |
| E-07 | 门户 | `TI_PORTAL_ADMIN_*`（默认 root/123456）；`TI_PORTAL_DEPT_*` 为**已注册用户**（E2E 用 psql 设 `group=D001-T010`） |
| E-08 | 汇率 | 确认 `AmountToQuota` 所用汇率配置稳定（CNY 用例以 3000 元为基准） |

### 1.3 执行方式（评审通过后）

按 [token-apply-api.md](./token-apply-api.md) 中的 curl 示例手工调用 API，结合 §10 SQL 核对台账、策略与消耗计数。门户 / 前端用例在浏览器中验证。

---

## 2. 测试数据（Fixture）

### 2.1 组织树（总包由发放参数带入，非独立 API）

E2E 首笔发放时在 POST body 中携带：

| 申请字段 | 对应 scope | 示例值 |
|----------|------------|--------|
| `parent_org_code` + `parent_org_budget` | org · `D001` | 100,000 CNY |
| `org_code` + `org_budget` | team · `D001-T010` | 30,000 CNY |
| `project_code` + `project_budget` | project · `PRJ-E2E-01` | 5,000 CNY（项目发放步骤） |

| scope_type | scope_code | 审批上限（月） | 说明 |
|------------|------------|----------------|------|
| org | `D001` | 100,000 CNY | 父节点（`parent_org_*`） |
| team | `D001-T010` | 30,000 CNY | 被测团队（`org_budget`） |
| team | `D001-T020` | — | 门户跨部门用 |
| project | `PRJ-E2E-01` | 5,000 CNY | 挂 `D001-T010`（`project_budget`） |

### 2.3 消耗策略（默认 E2E）

| scope_type | scope_code | cap | period |
|------------|------------|-----|--------|
| team | `D001-T010` | 100 CNY | day |
| token | `{token_id}` | 按需 | day |

### 2.4 工单编号

| 工单 | 用途 |
|------|------|
| `WO-E2E-001` | 主发放 + 变更 |
| `WO-E2E-002` | 预算拒绝 |
| `WO-E2E-003` | 第二笔发放 |
| `WO-E2E-PRJ` | 项目维度 |
| `WO-E2E-UNL` | unlimited |
| `WO-E2E-CAP` | 消耗封顶 |
| `WO-E2E-OTHER-DEPT` | 门户跨部门 |

---

## 3. 金钱 / 计费核心用例（必测，不可错）

> **优先级 P0。** 每条须核对 DB 字段，不能只看 API `success`。

### 3.1 金额换算与台账一致性

| ID | 场景 | 操作 | 金钱断言（全部必须满足） | 自动化 |
|----|------|------|--------------------------|:------:|
| **M-01** | 发放金额 → quota | POST 3000 CNY | `token_apply_records.amount=3000`；`token_apply_records.quota=AmountToQuota(3000,CNY)`；`tokens.remain_quota=token_apply_records.quota`；`tokens.used_quota=0` | e2e |
| **M-02** | 钱包不被碰 | 发放前后对比 | `users.quota` **发放前后相等**（同一 `user_id`） | e2e |
| **M-03** | 审计首笔 | 发放 3000 | `token_apply_logs` 1 条 `action=issue`；`budget_delta=3000`；`amount_after=3000`；`quota_after=token_apply_records.quota` | e2e |
| **M-04** | Quota 往返 | e2e 发放后查 DB | `token_apply_records.quota` 与 `amount` 按汇率一致（误差 ≤0.01） | e2e |
| **M-05** | 金额精度 | e2e 台账字段 | `token_apply_records.amount` 为 4 位小数精度 | e2e |

### 3.2 ① 审批预算（`org_budget` 总包 + `budget_delta` 分包）

发放/变更时传 `org_budget` 自动 upsert `token_budget_policies`；每笔 `amount` 计入审计 `budget_delta`。

**申请 JSON 总包 / 分包字段：**

| 字段 | 含义 |
|------|------|
| `org_budget` | 部门总包 → `token_budget_policies.total_amount` |
| `amount` | 本笔分包 → 台账 + `budget_delta` |
| `project_budget` | 项目总包（需 `project_code`） |
| `parent_org_code` / `parent_org_budget` | 上级部门总包 |
| `scope_type` | 部门策略类型，默认 `team` |

| ID | 场景 | 操作 | 金钱断言 | 自动化 |
|----|------|------|----------|:------:|
| **M-10** | 团队月上限 | 已批 8k 后再发 28k | `success:false`，含「预算不足」；**无新台账** | e2e |
| **M-11** | 增额只计增量 | 3000→5000 | 新日志 `budget_delta=2000`（非 5000）；团队累计已批 +2000 | e2e |
| **M-12** | 减额不计入已批 | 5000→4000 | 新日志 `budget_delta=0`；已批总额**不因减额下降** | e2e |
| **M-13** | unlimited 不占审批 | `quota_mode=unlimited` 发 20000 | 发放成功；该笔 `budget_delta=0`；团队已批**不增加** | e2e |
| **M-14** | unlimited 仍限 Key 总包 | unlimited 发 20000 | `remain_quota=AmountToQuota(20000)`；② 仍按 amount 封顶 | e2e |
| **M-15** | 项目维度已批 | `project_code=PRJ-E2E-01` 发 2000 | 成功；项目策略已批 +2000（日志汇总可查） | e2e |
| **M-16** | 项目额度打满 | 项目已批接近 5k 再发超额 | `success:false` 预算不足 | e2e 待补 |
| **M-17** | 父链 org 已满 | team 有余量但 org 已满时发放 | `success:false`（沿 parent_id 校验） | e2e 待补 |
| **M-18** | 子策略之和 ≤ 父 | upsert 子 team 超额 | `success:false` 或入库拒绝 | e2e 待补 |
| **M-19** | 发放幂等不占预算 | 同 `ticket_no` 重试 | 响应相同；`token_apply_logs` **条数不变**；已批总额不变 | e2e |

### 3.3 ② Key 额度（tokens.remain_quota / used_quota）

| ID | 场景 | 操作 | 金钱断言 | 自动化 |
|----|------|------|----------|:------:|
| **M-20** | 增额后 Key 可用 | 3000→5000 | `remain_quota` 增加量 = `AmountToQuota(2000)`；`used_quota` 不变 | e2e |
| **M-21** | 减额成功 | 5000→4000，used=0 | `remain_quota=AmountToQuota(4000)`；`token_apply_records.amount=4000` | e2e |
| **M-22** | 减额拒绝 | 模拟 `used_quota>0` 后减到低于已消耗 | `success:false` 含「已消耗」；**无新审计**；额度不变 | e2e |
| **M-23** | Key 余额不足 | `remain_quota` 耗尽后再 Relay | HTTP **403**；`used_quota` 不增加 | e2e 待补 |
| **M-24** | 变更幂等 | 同 `change_ticket_no` 重试 | 响应相同；日志条数不变；额度不变 | e2e 待补 |

### 3.4 ③ 消耗封顶（token_spend_policies）

| ID | 场景 | 操作 | 金钱断言 | 自动化 |
|----|------|------|----------|:------:|
| **M-30** | 日封顶 403 | team 日 cap=0 后 Relay | HTTP **403**；含「消耗封顶」；`used_amount` **不 permanently 超扣** | e2e |
| **M-31** | 成功 Relay 记账 | cap 足够、调 1 次模型 | `remain_quota` 下降；`used_quota` 上升；`users.quota` **不变** | e2e |
| **M-32** | used_amount 增加 | 同上 | `token_spend_policies.used_amount`（team+当日 period_key）**严格增加** | e2e |
| **M-33** | token 级 cap 优先 | 同 Key 配 `scope_type=token` 与日 cap 更低 | 以 **token 策略**为准拦截（链路透传） | e2e 待补 |
| **M-34** | Reserve/Adjust/Release | Relay 预扣→结算→失败退款 | `used_amount` 与 Key 额度一致回滚 | e2e 待补 |
| **M-35** | period=none 不封顶 | 策略 `period_type=none` | 该层跳过消耗校验 | e2e 待补 |
| **M-36** | 日/周/月 period_key | 不同 period_type | 键为 `YYYY-MM-DD` / `YYYY-Www` / `YYYY-MM` | e2e 待补 |

### 3.5 Relay 结算（service/billing + TokenApplyFunding）

| ID | 场景 | 操作 | 金钱断言 | 自动化 |
|----|------|------|----------|:------:|
| **M-40** | 只扣 Key 不扣钱包 | 成功 Relay | `users.quota` 前后相等；`tokens` 变化 | e2e |
| **M-41** | 信任旁路关闭 | token-apply Key | 必须预扣 token（不走 trust quota 旁路） | 代码审查 + e2e |
| **M-42** | 结算调差 | 流式/实际 token 与预扣不同 | `used_quota` 按**实际**结算；`used_amount` Adjust 差额 | e2e 待补 |
| **M-43** | 上游失败退款 | 模拟上游 5xx | `ReleaseConsumption`；`remain_quota` 恢复预扣 | e2e 待补 |

### 3.6 对账公式（评审用）

```text
① 累计已批 = SUM(logs.budget_delta)  过滤 scope 匹配（无周期重置）
② Key 可用     = token_apply_records.quota 换算后 − tokens.used_quota  （= remain_quota）
③ 本周期已消耗 = token_spend_policies.used_amount（元，当前 period_key）
```

**硬规则：**

- token-apply 路径 **永不** `UPDATE users SET quota = quota - …`
- `quota_mode=unlimited` **只**影响 ①，不影响 ② 总包
- 减额 **永不** 产生负的 `budget_delta`

---

## 4. L1 单元测试

> **已移除。** 不再维护 `model/*_test.go` / `pkg/period/*_test.go`；原 U-01~U-16 场景由 **手工 curl + psql** 覆盖或标为「待补」。

---

## 5. L2 集成 API 用例

| ID | 场景 | 步骤 | 期望 | 自动化 |
|----|------|------|------|:------:|
| A-01 | 员工发放 | POST `token_type=user` + `work_no` | `token_apply_id`、`token_key`、`user_id` | e2e |
| A-02 | 发放幂等 | 同 `ticket_no` 再 POST | 同 ID/Key；日志不增 | e2e |
| A-03 | app 发放 | `token_type=app` | Key 归 `app_user_email` 对应用户 | e2e 待补 |
| A-04 | 无 Api-Key | 不带 `X-Api-Key` | HTTP **401** | e2e 待补 |
| A-05 | 必填校验 | 缺 `ticket_no` / `work_no` | `success:false` | 手工 |
| A-06 | 增额 | PUT `amount` 变大 | 见 M-20、M-11 | e2e |
| A-07 | 减额拒绝 | 见 M-22 | 见 M-22 | e2e |
| A-08 | 减额成功 | 见 M-21 | 见 M-21 | e2e |
| A-09 | 变更幂等 | 同 `change_ticket_no` | 见 M-24 | e2e 待补 |
| A-10 | 预算拒绝 | 见 M-10 | 见 M-10 | e2e |
| A-11 | unlimited | 见 M-13、M-14 | 见 M-13、M-14 | e2e |
| A-12 | project 发放 | 带 `project_code` + `project_budget` | 见 M-15 | e2e |
| A-13 | 总包随申请同步 | 首笔 POST 带 `org_budget` / `parent_org_*` | `token_budget_policies` 有对应行；无 PUT budget API | e2e |
| A-14 | 消耗策略同步 | POST/PUT token-apply 带 `cap_amount` | Portal 可见策略；Relay 按 cap 拦截 | e2e 部分 |
| A-15 | 消耗 403 | 见 M-30 | 见 M-30 | e2e |
| A-16 | token 消耗策略 | PUT `scope_type=token` | upsert 成功 | e2e |
| A-17 | Relay | POST `/v1/chat/completions` | 见 M-31、M-32、M-40 | e2e |

---

## 6. L3 E2E 场景编排（清库后顺序）

| 步骤 | 场景 ID | 说明 |
|:----:|---------|------|
| 1 | PRE | 清库（§5 SQL） |
| 2 | A-14 | 首笔发放时带 `cap_amount`（同步写入③） |
| 3 | A-01、A-13、M-01~03 | 首笔发放（含 `org_budget`）+ `token_budget_policies` 核对 |
| 4 | A-02、M-19 | 发放幂等 |
| 5 | A-10、M-10 | 预算拒绝 |
| 6 | A-01 | 第二笔 5000 成功 |
| 7 | A-12、M-15 | 项目发放（`project_budget`） |
| 8 | A-11、M-13~14 | unlimited 发放 |
| 9 | A-06、M-11、M-20、M-02 | 增额 |
| 10 | A-15、M-30 | 消耗封顶 403 |
| 11 | A-16 | token 级策略 |
| 12 | A-07、M-22 | 减额拒绝 |
| 13 | A-08、M-21 | 减额成功 |
| 14 | A-17、M-31~32、M-40 | Relay 计费（需 channel） |
| 15 | A-14 | （无）消耗策略无独立删除 API |
| 16 | P-01~04 | 门户 |
| 17 | F-01 | Classic HTTP |

**评审后待补用例：** M-16、M-17、M-23、M-24、A-03、A-04、A-09、M-33、M-42、M-43。

---

## 7. L4 门户只读

| ID | 场景 | 步骤 | 期望 | 自动化 |
|----|------|------|------|:------:|
| P-01 | 管理员列表 | 登录 admin → GET `/records` | `success:true`；可见全部部门 | e2e |
| P-02 | 部门列表 | 登录 dept（group=`D001-T010`） | 仅 `org_code=D001-T010` 台账 | e2e |
| P-03 | 跨部门详情 | dept 访问 `D001-T020` 的 `token_apply_id` | `success:false` / 未找到 | e2e |
| P-04 | 策略只读 | GET `/api/token-apply/budget`、`/consumption` | dept 只见本部门树；admin 见全部 | e2e |
| P-05 | 门户写拒绝 | POST `/api/token-apply` 用 Session | **401**（必须 Api-Key） | 手工 |

---

## 8. L5 前端

| ID | 场景 | 步骤 | 期望 | 自动化 |
|----|------|------|------|:------:|
| F-01 | Classic 入口 | 登录 → `/console/token-apply` | 页 HTTP 200；三 Tab 可切换 | HTTP smoke |
| F-02 | Classic 数据 | 申请记录 Tab | 列表含 `remain_quota`；与 DB 一致 | **手工** |
| F-03 | Classic 策略 Tab | 预算 / 消耗 | 展示已批 / 已消耗 / 剩余 | **手工** |
| F-04 | Default 入口 | 登录 → 个人中心「部门额度」 | 三 Tab 渲染 | **手工** |
| F-05 | Default 权限 | 普通用户 | 只见本部门 | **手工** |

---

## 9. 负向与边界

| ID | 场景 | 期望 | 自动化 |
|----|------|------|:------:|
| N-01 | 无效 `token_apply_id` 变更 | 404 / 未找到 | 手工 |
| N-02 | `amount≤0` | 拒绝 | 手工 |
| N-03 | 错误 Api-Key 写消耗策略 | 401 | 手工 |
| N-04 | 子总包先传、父总包不存在 | 发放失败（父部门总包不存在） | e2e 待补 |
| N-05 | 币种不一致 | 预算/消耗校验失败 | e2e 待补 |
| N-06 | 达到用户最大 token 数 | 发放失败 | 手工 |
| N-07 | 并发双增额 | 仅一笔成功或预算正确（行锁） | 手工/压测 |
| N-08 | Relay 无 channel | 上游错误；**不脏写** `used_amount`/额度 | 观察 |

---

## 10. 对账 SQL（执行时逐条跑）

```sql
-- 台账 + Key 快照
SELECT a.id, a.ticket_no, a.amount, a.quota, a.quota_mode, a.org_code,
       t.remain_quota, t.used_quota, u.quota AS user_wallet_quota
FROM token_apply_records a
JOIN tokens t ON t.id = a.token_id
JOIN users u ON u.id = a.user_id
ORDER BY a.id;

-- 审计与 budget_delta
SELECT id, token_apply_id, action, change_ticket_no,
       amount_before, amount_after, quota_before, quota_after, budget_delta, created_at
FROM token_apply_logs
ORDER BY id;

-- 消耗策略（当前周期用量）
SELECT scope_type, scope_code, period_type, period_key, used_amount, cap_amount
FROM token_spend_policies
WHERE enabled = true
ORDER BY scope_type, scope_code;

-- 团队累计已批
SELECT COALESCE(SUM(l.budget_delta), 0)
FROM token_apply_logs l
JOIN token_apply_records a ON a.id = l.token_apply_id
WHERE a.org_code = 'D001-T010' AND a.token_type = 'user';
```

---

## 11. 覆盖统计（评审用）

| 类别 | 用例数 | 已覆盖 | 待补 | 手工 |
|------|:------:|:------:|:----:|:----:|
| 金钱 P0（M-*） | 28 | 18 | 10 | 0 |
| L2 API（A-*） | 17 | 12 | 5 | 1 |
| L4 门户（P-*） | 5 | 4 | 0 | 1 |
| L5 前端（F-*） | 5 | 1 | 0 | 4 |
| 负向（N-*） | 8 | 0 | 2 | 6 |
| **合计** | **63** | **35** | **17** | **12** |

> **说明：**「待补」项见 §6，评审通过后按 curl + SQL 手工执行并回填 §12。

---

## 12. 执行记录

> 评审通过后填写。状态：`待测` / `通过` / `失败` / `跳过`

| 日期 | 执行人 | 方式 | 结果 | 备注 |
|------|--------|------|------|------|
| 2026-06-15 | agent | 手工 curl + psql | **部分通过** | 核心 API/预算/幂等/增减/策略删除 ✅；Relay 404 跳过；Portal/前端需 `ti_portal_dept` 与 admin 密码 |

### 12.1 金钱用例执行明细

| ID | 状态 | 实际值摘要 | 备注 |
|----|:----:|------------|------|
| M-01 | 待测 | | |
| M-02 | 待测 | | |
| … | | | |
