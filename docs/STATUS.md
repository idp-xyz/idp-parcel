# 现状一页

取证于 main `4fa6a80d6`（2026-11-12）。本页只指路、不定义：每一格的权威在它链到的文件里，两处相抵以那边为准。维护方式见文末。

## 一句话

上线线已于 2026-10-28 达成；上线前与限量生产前必须做的票都已在 main。离第一家租户上线，产品这边没有欠着的代码，差的是租户取值与 PN-08 `Go`。现在唯一在做的是客户要求清单那一批（合同、价卡、计费、报价）。

## 现在按什么规矩干活

[AGENTS.md「交付模式」](../AGENTS.md#交付模式)：只做三种——冻结清单里必须做的、真实租户撞上的、用户点名的。

## 各切片做到哪

「机制半边」一栏照抄[开发主线「机制半边现状」](./product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md#机制半边现状)的定级；那张表之后的更正写在它下面的各条补记里，本页不复述。实例半边（某个租户能不能上生产）八个切片都还没有，只来自[参数登记册](./product/PILOT-PARAMETER-REGISTER.md)的证据与 PN-08 `Go`。

| 切片 | 是什么 | 主责上下文 | 机制半边 |
|---|---|---|---|
| PN-01 | 范围、责任与治理基础 | `pilotgovernance` | 达标 |
| PN-02 | 商业权威、生产归属、委托与包裹身份 | `party-commercial`、`parcel-shipment`、`network-routing` | 达标（留待项已认可） |
| PN-03 | 接受后路由、收寄与节点作业 | `network-routing`、`node-operations`、`transport-fulfillment` | 达标（留待项已认可） |
| PN-04 | 取消与处置、运输履约、交付与终局 | `transport-fulfillment`、`parcel-shipment` | 达标 |
| PN-05 | 关务合规控制 | `customs-compliance` | 达标 |
| PN-06 | 追踪、异常与客户披露 | `visibility-exception` | 达标（留待项已认可） |
| PN-07 | 计价、账单与运营结算 | `parcel-pricing`、`settlement-accounting` | 达标（留待项已认可） |
| PN-08 | 端到端试点与阶段准入 | `pilotgovernance` | 达标；真实候选与证据包等租户 |
| PN-09 | 仓配履约（可选，首发不含） | `warehouse-fulfillment` | 用例已写，main 上无生产代码；已搁置 |

## 一条规则现在是什么，看哪

先看上下文自己的 `CONTEXT.md`，那里写的是现行规则；用例的输入、结果与失败边界看 `UC-*`；ADR 只在要追「为什么这样定」时读，并且先看它的 Status 行有没有被后来的 ADR 停掉。

| 上下文 | 现行规则 | 用例 |
|---|---|---|
| 参与方与商业 | [CONTEXT](./domain/party-commercial/CONTEXT.md) | [UC-PC](./application/party-commercial/) |
| 小包托运 | [CONTEXT](./domain/parcel-shipment/CONTEXT.md) | [UC-PS](./application/parcel-shipment/) |
| 网络与路由 | [CONTEXT](./domain/network-routing/CONTEXT.md) | [UC-NR](./application/network-routing/) |
| 节点作业 | [CONTEXT](./domain/node-operations/CONTEXT.md) | [UC-NO](./application/node-operations/) |
| 运输履约 | [CONTEXT](./domain/transport-fulfillment/CONTEXT.md) | [UC-TF](./application/transport-fulfillment/) |
| 关务与贸易合规 | [CONTEXT](./domain/customs-compliance/CONTEXT.md) | [UC-CC](./application/customs-compliance/) |
| 全程追踪与异常 | [CONTEXT](./domain/visibility-exception/CONTEXT.md) | [UC-VE](./application/visibility-exception/) |
| 小包计价 | [CONTEXT](./domain/parcel-pricing/CONTEXT.md) | [UC-PP](./application/parcel-pricing/) |
| 结算与经营核算 | [CONTEXT](./domain/settlement-accounting/CONTEXT.md) | [UC-SA](./application/settlement-accounting/) |
| 代收与清分 | [CONTEXT](./domain/collection-remittance/CONTEXT.md) | [UC-CR](./application/collection-remittance/) |
| 仓配履约（可选） | [CONTEXT](./domain/warehouse-fulfillment/CONTEXT.md) | [UC-WF](./application/warehouse-fulfillment/) |

上下文之间谁拥有什么看 [CONTEXT-MAP](./domain/CONTEXT-MAP.md)，跨上下文的词看 [GLOSSARY](./domain/GLOSSARY.md)。

## 在做

- **客户要求清单**（[规格](../.scratch/commercial-four-module-requirements/spec.md)）：用户 2026-11-12 点名全部保留。票 15 已进 main（ADR-0380：客户合同版本声明销售价来源）；它的实现链 16 → 17、18 → 19 还没进 main，所以三个上下文的 `CONTEXT.md` 在这一处暂时走在代码前面。在途分支只有票 16 的 `cursor/cfmr-16-contract-sell-price-arrangement-r2`。

## 不做

- **冻结的票**：名单与解冻条件只在[上线线与冻结清单](../.scratch/launch-line-freeze-2026-10-27.md)。
- **2026-11-12 搁置的分支**：`cursor/wf-in-warehouse`、`cursor/reporting-source-adr`、`shelved/company-subject-profile`，都在 origin 上，等用户点名。
- 其余 feature 下开着的散票：没被点名，不开工。

## 离第一家租户上线还差

1. 一个真实租户的取值（服务商通道、账期等），登进[参数登记册](./product/PILOT-PARAMETER-REGISTER.md)；产品在这些格上按设计答`未配置`。
2. PN-08 阶段评审给出 `Go`（[交接](./design/pn-08-end-to-end-pilot-and-stage-admission-development-handoff.md)）。
3. 冻结清单「记录」里标了上线前的核对项：两本采用册登的是同一个范围；首个带 PR 的推送看一次 Actions；要对客展示 ETA 时先过它那条清单。

## 怎么维护

一笔进 main 的改动若让上面某一格变了（切片定级、在做、不做、还差），同一笔改这一格并把顶上的 SHA 与日期换成当时的。不在这里写经过、理由与票面细节——那些留在它们原来的地方。本页长过一屏就是写多了。
