# 客户要求清单（Contract / Pricing / Rating / Quotation）：逐条对照、缺口与排序

Category: enhancement
Status: needs-triage——子票全部 `draft`，没有激活。激活等于把这些项从冻结里解出来，归用户（见「与冻结清单的关系」）。
出处：2026-11-11 用户在 Cursor 会话里贴出一张四个模块的清单，问「我们的现有实现是这样的吗」。对照之后用户说「我没有方向了，基于现在的实现，你的专家做法呢」；会话建议不追这张表、只点名解冻毛利，并说明这张表的来源会改变结论。用户随后选定「这张表是客户或招标的要求，按它重排优先级」，再答「都要做」：逐条对照表、报价的 ADR 草稿与范围文档改动、规格和票。这张表在仓库与历史会话里查不到出处。同日稍后，会话请用户向对方确认四处口径，用户答「没有可问的人，分析，参考行业头部软件的做法」；会话查了公开文档后作答，用户选定把调研笔记与结论写进本目录。

切片：合同与报价属 PN-02，主责 `party-commercial`；价卡、计费与毛利属 PN-07，主责 `parcel-pricing` 与 `settlement-accounting`。非关务专项。三类归属写在各票。证据层级 `S`（演示租户）。

取证：钉 `11bfa1ffb`（`origin/main`，2026-11-11 用 `git ls-remote origin main` 当场确认）。**起草时的工作树不在这个提交上**：它停在分支 `codex/company-subject-profile`（`871f3cdd4`），当时 `git rev-list --count HEAD..origin/main` 为 750。本目录下的文件都照 `11bfa1ffb` 写，随后提交在从 `origin/main` 起的分支 `cursor/commercial-four-module-requirements` 上，没有进那条旧分支；那棵旧工作树里留着一份未跟踪的副本，在那里点指向较新 ADR 的链接会找不到文件。起草收尾时远端已前进到 `a276ddae2`，这一段没有碰到本规格引的代码与文档句子（核法见[对照表](./compliance-matrix.md)开头）。同日稍后的增补（行业做法笔记、票 14 与几张票的改动）里新引的代码与文档句子，核在 `48ca93e2d`。

## 要求清单（用户原文）

| 模块 | 要求 |
|---|---|
| Contract | 合同创建、签署证据、独立审批、生效、价卡授权 |
| Pricing | 销售/采购价卡、版本、草稿编辑、审核发布、发布冲突检测 |
| Rating | 分票/分件计费、体积重、阶梯运费、燃油附加费、毛利 |
| Quotation | 报价创建、审批、发出、客户接受与价格生效申请 |

## 对照结论

逐条结论与证据在[对照表](./compliance-matrix.md)，这里只说形状。

- **产品里没有这四个模块。** 前三块的能力分在三个限界上下文里：合同在 `party-commercial`，价卡与评价在 `parcel-pricing`，毛利在 `settlement-accounting`。这是 [ADR-0012](../../docs/adr/0012-parcel-pricing-context-within-idp-parcel.md) 定的划分，本规格不动它；四个模块只当要求清单用。
- **做了一半的**：价卡授权、草稿编辑、发布冲突检测、分票计费、毛利。
- **没有的**：签署证据，以及 Quotation 一行的全部。
- **其余各项今天就有。**

## 两处读法

要求清单只有一行字，下面两处是读出来的。没有可以去确认的对方；2026-11-11 会话对着行业头部软件的公开做法核过一遍，两处读法都站得住，依据在[行业做法笔记](./industry-practice-notes.md)。

- **Quotation 读作报一套价。** 依据是「客户接受与价格生效申请」：接受之后申请的是价格生效，不是锁价或下单。这样读，报价可以搭在现有的价卡与合同发布流程上，计价 `CONTEXT.md` 点名的两个难题（承诺哪一期参考序列取值、跨期差价归谁）不出现。理由全文在 [ADR 草案](./adr-draft-quotation-offers-a-set-of-prices.md)。
- **分票读作一票多件按整票合计计费。** 依据是「分票/分件」并列。

## 待用户确认

Q1 至 Q4 原本要向给出要求清单的一方问。没有可问的人，2026-11-11 由会话参考行业头部软件的公开做法代答，待用户认可；依据逐条写在[行业做法笔记](./industry-practice-notes.md)。Q5、Q6 仍是用户自己的决定。

| # | 问题 | 会话的答案 | 影响 |
|---|---|---|---|
| Q1 | 报价是报一套价，还是逐票报价并锁价 | 报一套价。按运单的事后改价不归报价，归费用调整 | ADR 草案的决定一不变 |
| Q2 | 分票是不是一票多件按整票计费；是合计后算计费重，还是逐件算计费重再相加；是不是主营 | 是一票多件按整票计费。两种算法都要，由价卡声明。不是小包的主营，主要在快递代理与专线渠道；这半句是判断，没有一手来源 | 票 06、07 排在报价之后；采购一侧另立票 14 |
| Q3 | 签署证据要到哪一步 | 登记签署信息与签署件索引。电子签章以后作为可选的对接另做 | 票 03 不变，证据多记一格来源 |
| Q4 | 价卡授权要到哪一步 | 不改评价的选卡路径。在两道登记门上核对：价卡登记时核授权，登记客户用哪个销售计价范围时核合同 | 票 04 多一道门；不重开 [ADR-0260](../../docs/adr/0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md) |

| # | 仍待用户决定 | 影响 |
|---|---|---|
| Q5 | 报价进首发试点范围，还是试点范围之外的产品能力 | 决定 `PILOT-SCOPE.md` 那一句怎么改 |
| Q6 | 激活哪几张票 | 见「与冻结清单的关系」 |

## 排序

| 顺序 | 票 | 补上要求里的 | 为什么排这里 |
|---|---|---|---|
| 1 | 01、02 | 草稿编辑、发布冲突检测 | 不等任何客户确认。01 也是报价「发出」的底座：对客价格表从价卡读回口导出 |
| 2 | 03、04 | 签署证据、价卡授权 | Q3、Q4 已有答案，各随票立一篇 ADR |
| 3 | 05 | 毛利 | 等用户点名解冻。它也是低毛利提示（[ADR-0366](../../docs/adr/0366-margin-is-neither-an-acceptance-nor-a-selection-gate-and-transit-time-is-an-eligibility-constraint-not-a-ranking-dimension.md) 决定三）的前置 |
| 4 | 08 → 09 → 10 → 11、12 → 13 | Quotation 一行 | 文档先行。08、09 可以与上面三批同时走，否则报价会成为最长的一条线 |
| 5 | 06 → 07、14 | 分票 | 照 Q2 的答案排在报价之后。依的是「小包多为一票一件」这句判断；租户的主营若是快递代理或专线，这一批要提到报价前面 |

## 子票

| # | 标题 | Blocked by | Status |
|---|---|---|---|
| 01 | [已登记价卡与价卡草稿读回成可编辑的工作簿](./issues/01-registered-price-card-reads-back-as-an-editable-workbook.md) | 无；随票立 ADR | draft |
| 02 | [价卡预览、批准与发布时提示与在册版本的适用期重叠](./issues/02-price-card-preview-and-publication-show-overlap-with-registered-versions.md) | 无 | draft |
| 03 | [合同与协议版本的签署证据](./issues/03-signing-evidence-on-contract-and-agreement-versions.md) | 无；随票立 ADR | draft |
| 04 | [价卡登记时核对它引用的价格方向授权，登记客户用哪套价时核对合同](./issues/04-price-card-registration-verifies-the-direction-authorization-it-cites.md) | 无；随票立 ADR | draft |
| 05 | [经营结果按包裹从直接费用与成本派生](./issues/05-operating-results-are-derived-per-parcel-from-direct-charges-and-costs.md) | 用户点名解冻；随票立 ADR | draft |
| 06 | [票级计价重量的聚合方式落到领域与导入模板](./issues/06-shipment-level-weight-aggregation-lands-in-the-domain-and-the-import-template.md) | 无（ADR-0364 已接受） | draft |
| 07 | [票级销售计费接进生产](./issues/07-shipment-level-sell-charging-enters-production.md) | 06；随票立 ADR | draft |
| 08 | [ADR 草案「对客报价」的非作者评审、编号与接受](./issues/08-adr-draft-quotation-review-numbering-and-acceptance.md) | Q5 | draft |
| 09 | [报价的领域语言与用例落文](./issues/09-quotation-domain-language-and-use-case.md) | 08 | draft |
| 10 | [报价对象、录入与批准](./issues/10-quotation-object-entry-and-approval.md) | 09 | draft |
| 11 | [报价发出：导出对客价格表并登记发出](./issues/11-quotation-is-sent-as-an-exported-rate-sheet.md) | 10、01 | draft |
| 12 | [客户接受报价](./issues/12-customer-acceptance-of-a-quotation.md) | 10 | draft |
| 13 | [价格生效申请](./issues/13-price-effectiveness-application.md) | 10、12 | draft |
| 14 | [采购一侧一票多件的运输收费发生项算不出成本](./issues/14-buy-side-multi-piece-charge-occurrences-cannot-be-priced.md) | 06；随票立 ADR | draft |

## 与冻结清单的关系

[上线线与冻结清单](../launch-line-freeze-2026-10-27.md)是冻结范围的单一权威。它「新发现怎么记」一节写着：不挡上线的不立票、不派代理，只在「记录」追加一行。本规格立了票，是因为用户点名要票；但没有一张激活，也没有改冻结清单。

激活时要同笔做三件事：

1. 在冻结清单「记录」追加一行，建议写法：

   `- 2026-11-11｜用户点名：客户或招标的要求清单（Contract / Pricing / Rating / Quotation）；逐条对照、报价 ADR 草案与子票已立为草稿｜[commercial-four-module-requirements/spec](./commercial-four-module-requirements/spec.md)｜用户点名，激活时逐票定级`

2. 票 05 只解冻清单「一之二」表里 product-strategy-boundary/22 那一行「留在冻结里的」所列成本分摊之中，派生经营结果的那一半。共享成本的分摊照旧冻结。
3. 冻结清单「记录」里出处写 ADR-0364 决定三的那一行，由票 06 承接。出处写 ADR-0366 决定三的那一行（低毛利提示）以票 05 为前置，本规格不承接。

## 不在本 spec

- 逐票报价与锁价。
- 让评价的选卡路径去读合同。Q4 的答案是在登记门上核对，ADR-0260 不动。
- 电子签章对接。Q3 的答案是先登记，对接以后作为可选的连接器另做。
- 按运单的事后改价。它归费用调整，入口仍在冻结。
- 主单级计费。它等运输履约的承运总单登记册。
- 共享成本分摊，以及低毛利提示本身。
- 把四个模块做成代码里或导航上的四块。

## 红线

- **租户取值留给租户**：各条审批职责规则、要不要以签署证据为发布前提、采用哪种经营口径、每份报价的价格与有效期，都不写死；没登记如实答未配置。
- **单一权威**：报价不立第二套批准与发布，价卡与合同照旧各走各的流程；毛利只在写口派生，读口不重算。
- **所有权清晰**：报价归 `party-commercial`，价格内容归 `parcel-pricing`，毛利归 `settlement-accounting`。
- **敏感实例外置**：签署件与客户的接受确认只在产品里存文件名、哈希与索引，文件放受限证据库。
- **证据层级诚实**：演示租户走通只记 `S`。

## 这份规格没有做的

没有改任何现行文档，没有提交，没有跑测试，没有在管理台上实走。对照表的取证方法与边界写在它的开头。
