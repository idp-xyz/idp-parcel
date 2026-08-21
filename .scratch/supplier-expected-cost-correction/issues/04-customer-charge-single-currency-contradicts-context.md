# `CustomerCharge` 只带单一币种与金额，与 CONTEXT 的币种三件组对不上

Category: bug
Status: resolved

由 [ADR-0067](../../../docs/adr/0067-cost-correction-restates-the-whole-evaluation-result.md) 的
Consequences 划出：「`CustomerCharge` 只带单一币种与金额，没有原币/结算币这一对，与 CONTEXT 的
『每条费用分别保存原币金额、合同结算币金额及换算依据』对不上。本记录不裁那一处，另记。」本票即那处「另记」。

## 病灶

[SA CONTEXT](../../../docs/domain/settlement-accounting/CONTEXT.md)「赔付、追偿、税费、币种与法人」
一节明文：

> 每条费用分别保存原币金额、合同结算币金额及换算依据。

以及 ADR-0067 补进的同节新句：原币金额、合同结算币金额和换算依据是**从同一个评价采用来的一组**，
不是可以各自停在不同评价上的三件。

`internal/settlementaccounting/domain` 的 `CustomerCharge` 今天只有单一币种与单一金额，三件组
一件都放不下。供应商侧（`SupplierExpectedCost`）三件俱全，客户侧缺位。

## 不在本票内

- 本票只记录模型与 CONTEXT 的失配，不提方案——客户费用是否真的需要三件组、还是 CONTEXT 那句
  对客户侧另有解读，属领域 owner 裁断。
- ADR-0067 已明确**不裁**这一处；不要把该 ADR 当本票的答案引用。

## 影响面

`CustomerCharge` 有无应用层调用方、有无落库数据未取证；triage 时先补这两问。

## Comments

- 2026-08-20 · MCP-1：随 ADR-0067 入库创建，防「另记」落空。
- 2026-08-21 · MCP-1：triage 前置两问取证毕（对 `main=e6e094d`），裁断输入齐。
  1. 应用层调用方**有**，两处：`ConfirmChargeHandler`（`internal/settlementaccounting/application/confirm_charge.go`，找回 → `Confirm` → `SaveConfirmed` → 发布意图，其注释明言「金额不改，领域已钉，本编排连金额输入都没有」）；`CutOffPublishStatementHandler`（`cut_off_publish_statement.go`，按 ID 逐笔找回费用组账单草稿）。
  2. 落库面**有**：`settlement_accounting.customer_charge`（`migrations/settlement_accounting/0003_customer_charge_advance.sql`），列形状即单 `currency` + 单 `amount`，与域对象一致；适配器 `adapters/postgres/customer_charge.go` 双向读写，读回经 `rebuildCustomerCharge` 精确重放一次 `Confirm`。生产数据无（无租户），但机制半边的持久化面已活。
  3. 波及面补记：`ChargeAdjustment.Charge`、`CustomerClaimAmount.OriginalCharge`（退款贷记既有费用）、`StatementLine` / `SubsequentInclusion` / `StatementDispute` 均按 `CustomerChargeID` 引用不复制金额；但账单线自带 `AmountMinor`，草稿整只收 `[]CustomerCharge`。若客户侧补三件组，改动至少贯穿域对象、0003 表列、`rebuildCustomerCharge` 与确认编排四处。
  是否补三件组、还是 CONTEXT 那句对客户侧另有解读，仍属领域 owner 拍板，本条不裁。
- 2026-08-21 · MCP-1：owner 经队列拍板**补三件组**后，主通道补充「文档也要对齐」，核查完毕：SA CONTEXT 硬句（「每条费用分别保存原币金额、合同结算币金额及换算依据」与 ADR-0067 补进的同组句、同币种等值句）与 UC-SA-002（AT-SA-176/177、换算依据交接行）**本就以三件组表达，权威文档无需改动**——落后的一方是代码。文档侧对齐落在本票记录与新立的 [05](./05-charge-adjustment-currency-triple-unruled.md)（费用调整是否同受约束，owner 明确另票单裁）。实现由 MCP-2 在共享树 SA 包领工（广播预告：Spec 换五字段、`Amount()` 拆三口、迁移 0013），Status 已由其转 in-progress。
- 2026-08-21 · MCP-2：triage 完成并认领。票面两问的答案：**有调用方**——应用层
  `confirm_charge.go` 编排确认、`cut_off_publish_statement.go` 截单读费用；postgres 适配器
  `customer_charge.go` 与迁移 `0003_customer_charge_advance.sql` 是落库面。**无生产数据**——
  产品尚无租户，真库行仅由测试产生，改口径无清洗负担。
- 2026-08-21 · MCP-2：owner 经 IDP 队列拍板（对「Q1 客户费用是否受三件组约束，建议是，
  若是则只改代码不改文档」答复「按你的建议直接开工」）：**客户费用受币种三件组约束，
  CONTEXT「每条费用分别保存原币金额、合同结算币金额及换算依据」按字面执行；文档不动，
  代码补齐。**同币种两额相等、跨币种必备评价内换算步骤两条不变量随 CONTEXT
  「赔付、追偿、税费、币种与法人」一节的现行文句一并落到形成门与库内 CHECK。
  费用调整是否同受约束不在本票，另立本目录 `05`。实现范围：`CustomerCharge` 领域形状、
  对账单域读点改读结算金额、postgres 适配器与新迁移（0003 不可改写，出 `0013` 补列）、
  相关测试。
- 2026-08-21 · MCP-2：实现完成，随本提交置 resolved。落点：
  - 领域：`CustomerChargeSpec`/`CustomerCharge` 换三件组（原币一对、结算币一对、换算依据），
    `Amount()` 拆为 `OriginalAmount()`/`SettlementAmount()`/`Conversion()`；形成门补两条不变量——
    跨币种必备评价内换算步骤（复用 `ErrConversionStepMissing`）、同币种两额必须相等，理由与
    供应商侧形成门一字不差。
  - 对账单域：`CutStatementDraft` 入单币种比对、`NetTotalMinor` 与 `PublishStatement` 行快照
    改读结算币一对——对账单以合同结算币立单。
  - 落库面：迁移 `0013_customer_charge_currency_triple.sql`（rename 单币种一对为结算币一对、
    补原币一对与 `conversion_ref`、回填后钉 NOT NULL；同币种相等与跨币种换算必备两条 CHECK
    与供应商侧 0008/0012 同款）；适配器读写新列，读回仍走形成门重放确认。
  - 测试：域新增 `TestAChargeCarriesTheCurrencyTripleFromItsEvaluation`（三件组读回、缺换算拒、
    两额不等拒、同币种不带换算依据）；真库约束用例补「同币种两额不等」「跨币种缺换算」两条
    反例行；晋升用例断言原币一对不丢。
  - 验证：`go build ./...`、`go vet ./...`、`go test -count=1 ./...` 全绿，真库带 DSN 实跑
    （单跑费用库用例 `-v` 确认 PASS 非 SKIP）。
  - `ChargeAdjustment` 未动，归本目录 `05`（needs-triage）。
