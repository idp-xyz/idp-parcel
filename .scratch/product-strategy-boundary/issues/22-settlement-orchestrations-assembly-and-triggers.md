# 22 结算其余编排的装配、入口与触发面

Category: enhancement
Status: in-progress——第 1 笔（确认）完工待评审，分支 claude-psb22（见文末「第 1 笔完成记录」）。此前 ready-for-agent——2026-10-27 用户同意由冻结 R3 上调为「限量生产前必须」（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)「一之二」，理由引 PILOT-SCOPE）：费用确认、截单发布、供应商账单接收与审核及确认与截单的触发上调；费用调整、成本分摊、赔付金额仍冻结；排在 tops-26 名下 6 张 code 票进 main 之后，先 10 后 22。此前 ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R3）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-09-24 通道 2 立票（用户令通道 2 独立承接）；接单时按编排拆笔
Blocked by: 无；赔付金额文法与分摊形态归[票 12](./12-sa-amount-grammars-allocation-forms-and-accounting-connectors.md) 第 1、2 项，结算四口的登记册与读口归[票 16](./16-mechanism-gaps-without-a-ticket.md) 第 3 项
地盘：`internal/settlementaccounting` 的入口与触发、`cmd/` 装配。
出处：[票 05](./05-demo-journey-criterion-evidence.md) 格 20 的装配与触发面那一半。

## 现象

`settlementaccounting/application` 的 `NewConfirmChargeHandler`、`NewCutOffPublishStatementHandler`、`NewRecordChargeAdjustmentHandler`、`NewAllocateCostsHandler`、`NewReceiveSupplierBillHandler`、`NewAuditSupplierBillHandler`、`NewSettleClaimAmountsHandler` 在 `cmd/` 零引用。这一类不在生产接线棘轮的网里（棘轮排除 `New*` 构造，见开发主线「2026-09-02 裁决」一段）。

## 做什么

- **机制半边**：各编排进生产装配，各有入口（运营命令面、受控 CLI 或消费门，按编排定）。
- **产品策略半边**：何时确认费用、何时截单发布的内置触发策略；账期是租户取值，经参考配置在演示租户上采用。

## 要裁的

1. 逐编排定入口形态：人发起的给命令面（生产形态等操作者渠道），事件驱动的给消费门。
2. 费用确认与截单的内置触发：到点批量、事件逐项，还是两者都给由租户显式采用。
3. 费用调整（`RecordChargeAdjustmentHandler`）开入口之前，先裁 [docs-consistency/code/46](../../docs-consistency/code/46-settlement-application-and-reversal-have-no-process-entry.md) 越权风险点「潜伏卡死：确认后的贷记调整让作用域账不平之后，撤销也撤不动」：入口一开，一笔确认后的贷记调整就能让作用域已确认应收低于已核销，此后该作用域的撤销、映射、核销全被重算拒绝。思路两条：放行不让账变得更坏的撤销，或允许一次撤多笔。（2026-10-26 文档自洽 code/46 第三轮轻评审 N-b 补）

## 完成判据

- 每份编排在生产进程里有入口；触发策略有内置执行器；票 05 格 20 的装配与触发面那一半越过（隔离形态实测只记 `S`）。

## 拆笔（2026-10-28，Claude Code 会话；用户 2026-10-27「按你的建议开始吧」）

1. **确认**：七项确认事实随预估客户费用同笔落下；「有效交付已终局」作内置确认依据，从终局信封与费用形成两头记；依据到了而这一户登了确认时点就当场确认；确认条件目录开受控登记口。裁决见 [ADR-0336](../../../docs/adr/0336-confirmation-facts-land-with-the-estimated-charge-and-an-effective-delivery-final-is-the-built-in-confirmation-basis.md)。
2. **运营决定口**：运营确认口（ADR-0336 决定三）与截单发布口。截单的内置触发另开 ADR：SA CONTEXT「对账周期的日历不在这本册里，本期边界由截单命令点名」，到点自动截单要先有周期册，第 2 笔里重新定。
3. **供应商账单**：接收与审核两个运营决定口。
4. **演示种子与重走**：演示租户登确认条件、两个结算时点、对账安排；重走动线到`已确认`与对账单发布。

## 第 1 笔完成记录（分支 claude-psb22，基于 main `c2a771cbb`）

- **做了什么**：`FormEstimatedCustomerChargeHandler` 的必备依赖 `FollowUp`（`FormedChargeConfirmationFollowUp`）在费用落下之后、记结论之前同笔登七项事实，PS 当前终局为已终局的有效交付就补记依据并按时点确认；新编排 `ConfirmChargeOnBasisHandler`；SA 新开第二路终局消费 `settlement-accounting/record-confirmation-basis-on-final-outcome`（`cmd/parcel-dispatch/routes_final_outcome.go`，名次 `positionBilling + 10`）；账户册 `FindByID`；依据册与条件目录的端口形写口；CLI 新命令 `charge-confirmation-condition`。无迁移。
- **文档**：ADR-0336；SA `CONTEXT.md`「费用形成与证据」两句；UC-SA-002 `AT-SA-217`–`220` 与交接一行（运营确认口归第 2 笔）。作者不重生成 ADR 索引、AT 盘点与机制清点。
- **自验**（受影响范围，真库 `idp-parcel-postgres-gate`）：`go test ./internal/settlementaccounting/... ./cmd/parcel-dispatch/ ./cmd/parcel-settlement-register/ ./cmd/parcel-api/ ./internal/architecture/... -count=1` 全过；`go vet ./...` 干净；`gofmt -l` 空。真库路由表用例 `TestAFormedChargeIsConfirmedOnTheEffectiveDeliveryFinalInEitherOrder` 两种先后都走到`已确认`、交出一封费用确认信封。全量与种子灌库留给推送方。
- **改动的既有用例**：dispatch 三条用例。两条直写 SELL 请求的补登了请求指名的结算账户（`sellRequestAccount`），因为形成之后要按标识读回那一户；终局扇出用例的 inbox 期望由两路改成三路。
- **代决与越权风险点**：见 ADR-0336「越权风险点」，待 SA owner 复核。
