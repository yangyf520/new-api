# 令牌发放 · 第三方集成 API 说明

> 设计细节见 [token-apply-design.md](../../design/token-apply-design.md)  
> 自动化测试见 [token-apply-test.md](./token-apply-test.md)

面向 **审批系统 / OA** 等集成方。模型调用（Relay）另见项目 `docs/openapi/relay.json` 或 [官方 API 文档](https://docs.newapi.pro/zh/docs/api)。

---

## 1. 环境与鉴权

```bash
cd /path/to/new-api
source .env

export BASE_URL="http://localhost:3000"   # 生产替换为实际域名
# X-Api-Key 来自 .env 的 TOKEN_API_KEY 或运营设置 token_apply_setting.api_key
```

| 用途 | 鉴权 | 说明 |
|------|------|------|
| 发放 / 变更（本文） | 请求头 `X-Api-Key: <TOKEN_API_KEY>` | 集成方密钥，**勿**写入前端 |
| 调模型 Relay | `Authorization: Bearer sk-...` | 发放响应中的 `token_key` |

**规则摘要**

- 一流程一 Key：每个 `ticket_no` 只发一个 `sk-...`
- 幂等：同 `ticket_no` 重复 `POST` 返回原 `token_apply_id` / `token_key`，不改额度
- 业务失败时 HTTP 仍为 **200**，看响应里 `success: false` 与 `message`

---

## 2. 发放 `POST /api/token-apply`

### 2.1 curl（全参）

```bash
curl -sS -X POST "${BASE_URL}/api/token-apply" \
  -H "X-Api-Key: ${TOKEN_API_KEY}" \
  -H 'Content-Type: application/json' \
  -d '{
    "ticket_no": "WO-2025-001",
    "record_id": "rec_8a1b2c",
    "email": "zhang@corp.com",
    "amount": 3000,
    "currency": "CNY",
    "org_code": "D001-T010",
    "org_name": "平台研发组",
    "org_budget": 30000,
    "cap_amount": 500,
    "period_type": "day",
    "project_code": "PRJ-2025-01",
    "project_budget": 5000,
    "token_type": "user",
    "quota_mode": "fixed",
    "scope_type": "team",
    "parent_org_code": "D001",
    "parent_org_budget": 100000,
    "parent_scope_type": "org",
    "work_no": "E10086",
    "user_name": "张三",
    "token_name": "WO-2025-001",
    "token_group": "default",
    "remark": "新项目立项"
  }'
```

### 2.2 请求字段

| 字段 | 必填 | 默认 | 说明 |
|------|:----:|------|------|
| `ticket_no` | ✓ | — | 流程工单号，幂等键 |
| `record_id` | | `""` | 外部记录 ID，便于对账 |
| `email` | ✓ | — | 申请人邮箱；`user` 类型 Key 归属该用户 |
| `amount` | ✓ | — | 本笔审批金额（元），换算为 Key 额度 |
| `currency` | | `CNY` | 币种 |
| `org_code` | ✓ | — | 部门/团队编码 |
| `org_name` | | `""` | 部门名称 |
| `org_budget` | | 不传则不建 | 部门审批总上限（元）→ 自动写入 `token_budget_policies` |
| `cap_amount` | | 不传则不建 | 部门周期消耗封顶（元）→ 自动写入 `token_spend_policies`（③） |
| `project_code` | | `""` | 项目编码 |
| `project_budget` | | 不传则不建 | 项目审批总上限（元），需有 `project_code` |
| `token_type` | | `user` | `user` 员工 / `app` 应用（Key 归 `app_user_email`） |
| `quota_mode` | | `fixed` | `fixed` 计入审批预算；`unlimited` 不计入①但仍按 `amount` 设 Key 额度 |
| `scope_type` | | `team` | 部门总包维度：`team` / `org` / `company` 等 |
| `period_type` | | `month` | 消耗封顶周期：`day` / `week` / `month` / `none` |
| `parent_org_code` | | — | 上级组织编码 |
| `parent_org_budget` | | — | 上级审批总上限（元） |
| `parent_scope_type` | | `org` | 上级总包 `scope_type` |
| `work_no` | `user` 时 ✓ | `""` | 工号 |
| `user_name` | | `""` | 申请人姓名 |
| `token_name` | | `ticket_no` | 令牌名称 |
| `token_group` | | `org_code` | Relay 分组 |
| `remark` | | `""` | 备注 |

> `org_budget` 等总包字段**不进** `token_apply_records` 表，只同步到 `token_budget_policies`。

### 2.3 成功响应

```json
{
  "success": true,
  "message": "",
  "data": {
    "token_apply_id": 43,
    "user_id": 8,
    "token_id": 46,
    "token_key": "sk-...",
    "token_name": "WO-2025-001",
    "remain_quota": 205479452
  }
}
```

集成方请保存：`token_apply_id`、`token_key`（后续变更与 Relay 使用）。

### 2.4 失败示例

```json
{
  "success": false,
  "message": "预算不足：team D001-T010 累计已批 28000，本次 3000 将超出上限 30000"
}
```

未带或错误的 `X-Api-Key` → HTTP **401**（通常无 JSON 体）。

---

## 3. 变更 `PUT /api/token-apply/:id`

`:id` = 发放返回的 `token_apply_id`。

### 3.1 curl（全参）

```bash
export TOKEN_APPLY_ID=43

curl -sS -X PUT "${BASE_URL}/api/token-apply/${TOKEN_APPLY_ID}" \
  -H "X-Api-Key: ${TOKEN_API_KEY}" \
  -H 'Content-Type: application/json' \
  -d '{
    "change_ticket_no": "WO-2025-001-CHG-1",
    "record_id": "rec_9d4e5f",
    "amount": 5000,
    "currency": "CNY",
    "org_budget": 50000,
    "cap_amount": 800,
    "period_type": "day",
    "project_budget": 8000,
    "scope_type": "team",
    "parent_org_code": "D001",
    "parent_org_budget": 120000,
    "parent_scope_type": "org",
    "remark": "项目扩容"
  }'
```

### 3.2 请求字段

| 字段 | 必填 | 说明 |
|------|:----:|------|
| `change_ticket_no` | ✓ | 变更单号，幂等键 |
| `record_id` | | 外部记录 ID |
| `amount` | ✓ | 变更后**总**审批额（元），非增量 |
| `currency` | | 默认沿用台账币种 |
| `org_budget` | | 更新部门总包（可选） |
| `cap_amount` | | 更新部门消耗封顶（可选） |
| `project_budget` | | 更新项目总包（可选） |
| `scope_type` | | 更新部门总包维度 |
| `period_type` | | 更新消耗封顶周期 |
| `parent_org_code` | | 更新上级组织 |
| `parent_org_budget` | | 更新上级总包 |
| `parent_scope_type` | | 上级 scope 类型 |
| `remark` | | 备注 |

**不可修改**：`email`、`work_no`、`token_type`、首次 `ticket_no`。

增额只把**差额**计入审批预算；减额时若低于已消耗则拒绝且不写审计。

### 3.3 成功响应

```json
{
  "success": true,
  "message": "",
  "data": {
    "token_apply_id": 43,
    "token_id": 46,
    "amount": 5000,
    "remain_quota": 342465753
  }
}
```

---

## 4. 消耗封顶（③）

消耗策略由发放/变更接口**随请求字段同步写入** `token_spend_policies`。Relay 调用时按策略行的 `used_amount` / `period_key` 做周期封顶校验；门户只读展示走 `GET /api/token-apply/consumption`。

---

## 5. 发放后调模型（Relay）

```bash
export TOKEN_KEY="sk-..."   # 发放响应中的 token_key

curl -sS -X POST "${BASE_URL}/v1/chat/completions" \
  -H "Authorization: Bearer ${TOKEN_KEY}" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "hi"}]
  }'
```

---

## 6. 对接清单

发给第三方时建议包含：

1. 本文档（或导出 PDF）
2. `BASE_URL`（测试 / 生产）
3. `TOKEN_API_KEY`（单独安全渠道，不写进文档仓库）
4. Relay 文档链接（上一节或官方文档）
