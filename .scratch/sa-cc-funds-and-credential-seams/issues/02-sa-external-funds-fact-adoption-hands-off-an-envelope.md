# SA 的外部资金事实采用不发信封：`map_external_funds.go` 采用一条事实后 Outbox 里什么都没有，CC 的税费付款核对拿不到它

Category: enhancement
Status: in-progress——2026-09-10 18:0x 通道 5 认领（task-c947d9a1；分支 `mcp5-sacc02` 基 `76932b38`，隔离树 D:/tops/idp-parcel-mcp5-sacc02）。此前 ready-for-agent——2026-09-10 17:3x 通道 5 按通道 1 派单 task-9a2ff746（用户授权代裁）写入裁决：分区主体取租户 / 资金事实引用、更正 / 撤销复用同一事件类型带回指（见「要裁的」下「裁决」），本票再无待裁问题。此前 draft——2026-09-10 通道 4 立票（task-9880bbc9），只写票面未动代码；取证锚 `3f485e97`
Blocked by: 无

## 缺口（取证于 `3f485e97`）

- `internal/settlementaccounting/application/map_external_funds.go` 的采用编排交 `FundsFactAdopted / FundsFactExisting / FundsFactConflict`，写 `ports.ExternalFundsFactStore`；**不调任何 `HandOff*`**（`git grep -n -i handoff -- internal/settlementaccounting/application/map_external_funds.go` 零）。
- SA 的事件类型常量表（`git grep -n 'eventing.EventType' -- internal/settlementaccounting/adapters/postgres`）：代垫回收 / 回收调整 / 费用确认 / 索赔金额 / 应收 / 确认 / 索赔调整 / 成本分摊 / 经营结果 / 核销两型 / 对账单三型 / 供应商账单三型——**没有资金事实采用**。
- CC 那一半已按「应用层入口为缝」落（mech/07 CC-c：`ReceiveFundsFact` + `external_funds_fact` 登记册，`616646d`），等的正是这封信。

## 语言从哪里来

- SA `CONTEXT.md`：「银行、支付或财务系统拥有实际付款、付款失败、追加付款、资金退回、付款撤销和外部资金事实更正」；「外部资金事实撤销、更正或退回时，`UC-SA-005` 追加映射更正或核销撤销。两类历史都不得删除。」
- CC `CONTEXT.md` 生命周期：「外部资金事实接入 → 税费付款核对：按真实程序逐范围分别形成覆盖状态、差额状态和有效性状态；资金退回、付款撤销或外部资金事实更正只作为重新核对的来源事实，不能成为关务核对状态。」
- mech/07 CC-c 原句：「SA 的 `AdoptFundsFact` 不发信封，SA 适配器无 `external-funds-fact.*` 事件类型……SA 事实采用发信封 + CC inbox 消费者接进本口，另立票归 SA（MCP-3 建议：那封的分区主体取租户/资金事实引用，因为事实的更正/撤销在 SA 是新事实回指原事实）。」

## 做法

1. `ports.ExternalFundsFactHandoff`（`HandOffExternalFundsFact(ctx, intent)`）+ `adapters/postgres/external_funds_fact_handoff.go`，事件类型 `settlement-accounting.external-funds-fact.adopted`，载荷只带引用（租户、资金事实引用、采用版本），内容由消费方按引用回查——与 SA 既有 handoff 同形（`supplier_bill_handoff.go` 那一族）。
2. 采用编排在 `FundsFactAdopted` 那一格同事务交意图；`FundsFactExisting` 不重发（重放靠 `EnqueueOnce` 认领）；`FundsFactConflict` 不发。
3. 更正 / 撤销在 SA 是新事实回指原事实（ADR-0029 / CONTEXT），同一事件类型再发一封、载荷带回指——不另开「更正」事件类型（见「要裁的」）。
4. 分区主体：租户 / 资金事实引用（MCP-3 建议）——见「要裁的」。

## 红线

- 信封只带引用不带金额；金额与币种由消费方按引用读 SA 读口（单一权威）。
- 不改采用编排的四格负向代数；`UnfundableFactOutcome` 不发。
- 不在本票接 CC 消费者（那是 [03](03-cc-inbox-consumer-receives-external-funds-fact.md)）。

## 完成判据

1. 应用层：采用成功交意图一封；重放不交；冲突不交；交接失败按仓内既有形（版本已落、意图未入队 → 技术未形成 + 续办引用）。
2. 真库：`external_funds_fact_handoff_test.go` 入队一封且分区键如裁；`EnqueueOnce` 同键不翻倍。
3. `docs/agents` / 机制清点：直投路由表不改（消费者在 03）；清点 tip 重生成。

## 地盘

`internal/settlementaccounting/ports/`、`internal/settlementaccounting/application/map_external_funds.go`、`internal/settlementaccounting/adapters/postgres/`（新文件）、`cmd/parcel-api` 装配处（`map_external_funds` 的装配点，先核在哪个 assemble 文件）。

## 要裁的

1. **分区主体**：租户 / 资金事实引用（MCP-3 建议）还是租户 / 申报范围——前者让同一事实的更正链有序，后者让同范围多笔事实有序；CC 核对按范围逐笔判，本票倾向前者。归 SA owner，一句。
2. **更正 / 撤销复用同一事件类型**还是另开 `*.superseded`：仓内先例是核销「applied / reversed」两型分开；本票倾向同型带回指（事实本身就是新版本）。归 SA owner。

### 裁决

（通道 1 推送方裁、通道 5 写入，2026-09-10 17:0x；task-b941ce87 分类两条均 A、task-9a2ff746 落笔。A 类不落 ADR。）

- **1 → 租户 / 资金事实引用。** 口径是仓内既有的「ID 管幂等、分区键管顺序：同一主体的版本链排一条队」（[ADR-0069](../../../docs/adr/0069-customs-case-chain-ordering-absorbed-by-reread-and-retry.md) 决定二「分区键收窄到业务主体」；SA `supplier_bill_handoff.go` / `operating_handoff.go` 头注同句）。SA CONTEXT「外部资金事实撤销、更正或退回时……追加映射更正」——更正链是同一事实的版本链，主体就是资金事实；同范围多笔事实之间没有先后可言（CC 按范围逐笔核对，每笔各自成核对来源）。主体名「租户 / 资金事实」按 [ADR-0074](../../../docs/adr/0074-tf-object-partitions-carry-a-port-segment-apart-from-ve-parcel-partitions.md) 决定五进中心登记表，随本票落地同笔。
- **2 → 同一事件类型带回指。** 两侧 CONTEXT 已定语义：SA 更正 / 撤销是新事实回指原事实；CC「资金退回、付款撤销或外部资金事实更正只作为重新核对的来源事实」——消费方对三者一视同仁地重新核对，不需要按类型分路。核销 applied / reversed 两型是两个生命周期态而非同一事实的新版本，不构成反例。载荷带回指（原事实引用）由消费方按引用读 SA 只读口取事实内容与更正关系。

## 参照

[mech/06](../../mechanism-executor-triage/issues/06-sa-four-executors-behind-existing-uc-steps.md)「不在本票」末段；[mech/07](../../mechanism-executor-triage/issues/07-cc-four-executors-behind-existing-uc-steps.md) CC-c；[remaining-work-dd5ed934.md](../../unresolved-review-20260904/remaining-work-dd5ed934.md) 五-7；ADR-0043（EventID 认领）、ADR-0049（分区）；`internal/partycommercial/adapters/postgres/operator_registration_completed_handoff.go`（跨上下文「已登记」信封先例，`542ebc3c`）。

## Comments

- 2026-09-10 · 通道 4：立票。未动代码。
