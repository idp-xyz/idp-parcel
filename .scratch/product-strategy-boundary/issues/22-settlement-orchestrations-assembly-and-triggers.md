# 22 结算其余编排的装配、入口与触发面

Category: enhancement
Status: ready-for-agent——2026-10-27 用户同意由冻结 R3 上调为「限量生产前必须」（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)「一之二」，理由引 PILOT-SCOPE）：费用确认、截单发布、供应商账单接收与审核及确认与截单的触发上调；费用调整、成本分摊、赔付金额仍冻结；排在 tops-26 名下 6 张 code 票进 main 之后，先 10 后 22。此前 ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R3）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-09-24 通道 2 立票（用户令通道 2 独立承接）；接单时按编排拆笔
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
