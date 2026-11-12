# 报表来源（PN-10）：让报表平台按协议读本产品的经营汇总

Category: enhancement
Status: in-progress

依据：产品 owner 2026-11-11 在报表平台一侧的决定——IDP Parcel 是报表平台 IDP Reporting 的第一个真实来源，Parcel 一侧在本仓做，先出本仓自己的 ADR 给 owner 看，再实现；报表查询里的「公司」是责任法人。出处与对本仓的只读核对在 `idp-xyz/idp-reporting` 仓的 `docs/ADR_0027_AFTER_THE_DESIGN_REVIEW.md` 与 `docs/REAL_SOURCE_DISCOVERY_PARCEL.md`，那个仓不是本仓的权威。

这是 owner 点名的新能力，不受[上线线与冻结清单](../launch-line-freeze-2026-10-27.md)约束（冻结清单管的是 2026-10-27 的存量票池）。

## 目标

报表平台经它发布的报表来源协议，替一名在册的操作者读到本产品冻结的经营汇总；他能看什么只按本产品的操作者册裁。做法写在 [ADR-0384](../../docs/adr/0384-reporting-source-is-an-optional-capability-a-reporting-platform-client-names-an-operator-and-reads-frozen-operating-summaries-judged-by-the-operator-register.md)（草案）。

## 现状

- ADR-0384 是草案，接受与否归产品 owner。接受之前不立实施票，不动 `internal/**`，不动任何已接受的记录与现行文档。
- owner 要先答的四条在 ADR-0384「越权风险点」里点了名：点名的信任、发行方是不是同一个、法人范围、服务凭据。

## 不在本轮

- 逐行明细的数据集、委托与件级事实的日计数、正式单据的出具与作废（ADR-0384 决定十）。
- 部署形态与真实租户：自检与对账只在合成租户上做，证据记 `S`。

## 子票

| 票 | 内容 | Blocked by | 状态 |
|---|---|---|---|
| [01](./issues/01-decide-how-parcel-serves-as-a-reporting-source.md) | 写 ADR-0384：本产品怎样做报表来源 | — | in-progress（草案已出，等 owner 作答） |

ADR-0384 被接受之后要立的票。现在不立文件：owner 的答复可能改掉其中几张的内容。

- 接受那一笔的文档改动（ADR-0384「接受时同笔改动的文件」）。
- 接入身份：「报表来源读」授予、能力面「经营报表查阅」与法人范围、信封与铸造、登记口。
- 冻结读取与协议四口（`catalog`、`reads`、`pages`、`authorize`）：端点表的新一族、部署开关、错误映射、清理一拍。
- 首个数据集：结算与经营核算的汇总读端口、口径文档、与对账单复算的验收场景。
- 公司目录。
- 为读取的范围担保，连同报表平台的接入合同要来源交的「自己的测试」。
- `UC-RS-*`、PN-10 开发交接与演示种子；协议的自检命令对演示租户全部通过（`S`）。

报表平台一侧要先补的两件不在本仓：它的通用适配器会取并续客户端凭据令牌；协议写明结果超限时来源答哪个码。
