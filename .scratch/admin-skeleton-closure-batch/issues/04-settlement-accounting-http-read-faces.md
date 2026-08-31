# 结算与核算四页读面——`adapters/http` 整个包从零建

Category: feature
Status: ready-for-agent——MCP-4
Blocked by: 无

本批最大一块，但**零设计裁决**：形状可逐字照抄。

## 现状（取证于 `65b6cf2`）

`internal/settlementaccounting` 的 `domain`、`ports`、`application`、`adapters/postgres`、
`adapters/partycommercial` 都在，写适配器很齐——**但整个 `adapters/http` 目录不存在**，
所以四页无处发请求。`migrations/settlement_accounting/` 下 25 张表、`tenant_id` 出现 78 处，
**ADR-0077 的读面形状逐字成立，不需要新 ADR**。25 张表当前全部 0 行。

样板取 `internal/collectionremittance`——它是最新一份从零建到接线的完整样板（域 → 端口 →
真库适配器 → `adapters/http` 查询处理器 → 隔离读准入 → 装配 → 页），逐层可比。

## 四页与表的对应（起点，需按页面栏目与 CONTEXT 复核后定稿）

| 页 | 落在哪些表 |
|---|---|
| `charges-billing` 费用与计费 | `customer_charge`、`charge_confirmation_basis`、`charge_confirmation_condition`、`cost_allocation`、`supplier_expected_cost` |
| `reconciliation` 对账单 | `customer_statement`、`statement_dispute`、`supplier_bill_reception`、`subsequent_inclusion` |
| `settlement-application` 收付款核销 | `settlement_application`、`external_funds_fact`、`funds_mapping`、`funds_freeze` |
| `operating-metrics` 经营核算 | `operating_result`、`operational_balance` |

这张表是我按表名与页名推的**起点，不是裁定**。定稿前请对着
`docs/domain/settlement-accounting/CONTEXT.md`、`docs/application/settlement-accounting/UC-SA-*`
与四张页现有骨架的栏目逐格复核；对不上就以文档与页面栏目为准，并在 Comments 里记明改了哪几格
与依据。

## 表会一直是空的，这是预期不是缺陷

25 张表的写入方是事务链（费用由计价评价形成、对账单由账期归集、核销由真实收付映射），而事务链
在接入渠道墙后面——见 [spec 事实基线](../spec.md)。四页接完**仍是空册**。

**完成判据只能写成「空态文案说的是『读取入口已配置、登记册为空』，而不是『尚未接线』」，
不得写成「页面有数据」。** 尤其不要为了让经营核算页好看去造毛利或损失数字：那是经营口径的
业务事实，造出来直接破「实例值留空拒默认」。读适配器的真库测试照常写（测试内插行再读回）。

## 地盘的一处例外

`internal/settlementaccounting/adapters/partycommercial/`（`pre_acceptance_control_policy.go`
及其测试）**此刻有他会话的在途未提交改动**，不在本票地盘内，不要碰、也不要把它当自己的验证
失败——碰到那里的编译错按 parallel-sessions 的规矩不修不报。本票只在 `ports`、
`adapters/postgres`、新建的 `adapters/http` 三处落笔。

## 两阶段与次序

- **阶段一**：四组读端口 + 真库读适配器（含真库测试）+ 新建 `adapters/http` 包（四个
  `query_*.go` + 隔离读准入）。自验绿后向频道交**已验 SHA** 与四条端点行；**不自改**
  `cmd/parcel-api` 装配四件（占号在票 07）。
- **阶段二**：收到 MCP-1「已装配」广播后，四页接真，`liveIds` 加四行——**只加自己那四行，
  不动邻行**。注意 `ReconciliationPage` 与 `SettlementApplicationPage` 在
  `apps/admin-web/src/pages/governance/` 下，不在 `settlement/`。

## 完成判据

四页转 live；空态文案说「读取入口已配置、登记册为空」而非「尚未接线」；含真库全仓绿（注明）；
新建的 `adapters/http` 包与 `collectionremittance` 同族（隔离读准入三条判据入格一致）。

## Comments
