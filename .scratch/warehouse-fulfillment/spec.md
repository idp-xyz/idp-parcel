# 仓配履约（PN-09）：从领域文档到可开工

Category: enhancement
Status: in-progress

依据：领域已定——[ADR-0368](../../docs/adr/0368-commodity-inventory-and-warehouse-fulfillment-join-the-product-as-a-new-context-and-node-operations-stays-non-wms.md)、[ADR-0370](../../docs/adr/0370-warehouse-outbound-dispatches-by-product-network-owner-label-or-owner-pickup-and-the-remaining-warehouse-gaps-are-decided.md)、[ADR-0371](../../docs/adr/0371-warehouse-fulfillment-acts-for-the-owner-on-warehouse-outbound-requests-and-every-decision-path-hands-off-the-acceptance-decision.md) 与[仓配履约 CONTEXT](../../docs/domain/warehouse-fulfillment/CONTEXT.md)（票 01 至 04）；切片与「可选能力」的口径见 [ADR-0375](../../docs/adr/0375-warehouse-fulfillment-is-an-optional-capability-on-its-own-pn-09-slice-off-per-deployment-unless-switched-on-and-unconfigured-per-tenant-until-registered.md)（票 05）。

用户 2026-11-11 原话「按你的定义开始吧，在我们的系统仓配实现的时候可以是可选的能力吗」。这是用户点名的新能力，不受[上线线与冻结清单](../launch-line-freeze-2026-10-27.md)约束（冻结清单管的是 2026-10-27 的存量票池）。

## 目标

PN-09 的机制半边可以开工：切片已立，仓储作业费的计价与结算已定，`UC-WF-*` 写全，首方客户面与一线作业端的契约写明，开发交接（工作包与合成演示动线）写成。

之后的实现按开发交接里的工作包认领（[workflow.md](../../docs/agents/workflow.md)「一个 PN 切片怎么走」），不在本 spec 里立实现票。

## 不在本轮

- 实现：`internal/**`、迁移、管理台与一线作业端，随票 12 写出的工作包做。
- 按租户的商业授权与计量（ADR-0375 决定六）。
- 保税仓，与运营企业在目的国自己组织本地派送（ADR-0370 决定七、ADR-0371 决定六）。

## 子票

都是文档票，走 workflow.md「进 main 的步骤」，进 main 前各要一份非作者评审。

| 票 | 内容 | Blocked by | 状态 |
|---|---|---|---|
| [01](./issues/01-write-the-warehouse-fulfillment-context.md) | 写仓配履约 CONTEXT | — | resolved |
| [02](./issues/02-close-the-warehouse-fulfillment-gaps.md) | 补齐首版差距（ADR-0370） | 01 | resolved |
| [03](./issues/03-align-the-warehouse-fulfillment-docs-with-the-current-implementation.md) | 文档与现实现对齐 | 02 | resolved |
| [04](./issues/04-carry-out-the-owner-review-conditions.md) | 落实 owner 评审条件（ADR-0371） | 03 | resolved |
| [05](./issues/05-stand-up-the-pn-09-slice-and-make-warehouse-fulfillment-an-optional-capability.md) | 立 PN-09 切片，定「可选能力」的口径（ADR-0375） | 04 | in-progress |
| [06](./issues/06-decide-how-warehouse-charge-occurrences-are-priced-and-settled.md) | 仓储作业费的计价与结算（ADR） | — | ready-for-agent |
| [07](./issues/07-write-the-inbound-use-cases.md) | `UC-WF`：主数据与入库 | 05 | ready-for-agent |
| [08](./issues/08-write-the-in-warehouse-use-cases.md) | `UC-WF`：库内作业、冻结、盘点与差异 | 05 | ready-for-agent |
| [09](./issues/09-write-the-outbound-use-cases.md) | `UC-WF`：出库请求到出库确认 | 05；平台备货一节另等 declared-goods-lines/01 进 main | ready-for-agent |
| [10](./issues/10-write-the-return-agent-warehouse-and-charge-occurrence-use-cases.md) | `UC-WF`：退货、代理仓与收费发生项 | 05、06、07、08、09 | ready-for-agent |
| [11](./issues/11-write-the-customer-faces-and-the-frontline-warehouse-work-contract.md) | 首方客户面、一线作业端与接入词表 | 07、08、09、10 | ready-for-agent |
| [12](./issues/12-write-the-pn-09-development-handoff-and-synthetic-task-pack.md) | PN-09 开发交接与合成任务包 | 06、07、08、09、10、11 | ready-for-agent |

06 与 07、08、09 互不依赖，可以并行。用例编号是各票的建议，以写成的 `UC-WF-*` 为准。

## 与在途分支的交叠

取证于 2026-11-11：本地分支 `docs/declared-goods-lines`（未进 main）带 ADR-0373、ADR-0374，同样改 ADR-0368 的 `Status` 行与仓配履约 CONTEXT 的平台备货各句。票 05 与它在 ADR-0368 的 `Status` 行与 Links 上会冲突，后落的一方把两条注记都留下即可；票 09 的平台备货一节等它进 main 之后照 ADR-0374 写。
