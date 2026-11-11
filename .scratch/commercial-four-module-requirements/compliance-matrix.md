# 客户要求清单逐条对照（Contract / Pricing / Rating / Quotation）

本文件是 [spec](./spec.md) 的取证附件，不是规则。它只回答一件事：要求清单里的每一项，产品今天做到了哪一步，证据在哪。

**取证**：钉 `11bfa1ffb`（`origin/main`，2026-11-11 用 `git ls-remote origin main` 当场确认）。方法是对这个提交 `git grep` 与 `git show`，逐项读代码与文档。没有跑测试，没有起服务，没有在管理台上点过；下面的「满足」因此指「代码与页面在，读下来行为对得上」，不指「在演示环境里实走过」。

起草收尾时远端已前进到 `a276ddae2`。`git diff --stat 11bfa1ffb a276ddae2` 在本表引到的路径上，改动落在管理台法人资料页的复制图标、仓配履约的 ADR（新增一篇，另一篇的 `Status` 行与 ADR 索引随之变）与上下文地图里仓配的几行。本表引的代码符号与文档句子都不在其中。再往后的提交没有核。

**三种结论**：

- **满足**：有生产入口，有管理台页面或命令入口，行为与要求对得上。
- **部分满足**：做了一半，差的那一半写在「现状」里。
- **待开发**：没有对应代码。

本表逐行合计：满足 10 项，部分满足 5 项，待开发 6 项。

## Contract

| # | 要求 | 结论 | 现状 | 证据 | 票 |
|---|---|---|---|---|---|
| 1 | 合同创建 | 满足 | 客户合同是商业版本的一类，在管理台「客户与合同」页录入，形成待批准发布 | `internal/partycommercial/domain` 的 `CustomerContractBody`、`SubmitPublicationDraft`；`POST /commercial-publication-drafts`；页面 `party-contracts`（`CustomerContractPublicationForm.tsx`） | — |
| 2 | 签署证据 | 待开发 | 发布只记批准依据（批准引用、来源引用、时刻），没有签署方、签署时间或签署件索引 | `ApprovalBasis`；在 `internal/partycommercial` 的非测试代码里检索「签署、签章、signature、signed」，只命中联系人用途的一句注释 | [03](./issues/03-signing-evidence-on-contract-and-agreement-versions.md) |
| 3 | 独立审批 | 满足 | 批准与录入是两个操作者动作。两人须否不同、批准者须持哪一格授予，由租户登记的审批职责规则定；没登记不放行 | `PublicationDraft` 的 `Approve`、`ApprovalDutyRule`；`POST /commercial-publication-draft-approvals`；规则登记命令 `parcel-commercial register-approval-duty-rule`（ADR-0362）；页面 `pending-publications` | — |
| 4 | 生效 | 满足 | 发布时按有效区间答已生效或计划生效，到点生效；换版要显式收尾旧版 | `publish_commercial_authority.go` 的 `PUBLISHED_EFFECTIVE`、`PLANNED_EFFECTIVE`；`POST /commercial-publication-draft-publications`、`/commercial-version-successor-cuts`、`/commercial-version-retirements`；按身份读回 `GET /commercial-version-readbacks/{kind}/{objectId}/{version}`（ADR-0361） | — |
| 5 | 价卡授权 | 部分满足 | 授权有记录：价卡登记必须引用一条价格方向授权，销售与采购方向的价格政策各绑定一份定价方案。没有核对：计价不查那条授权是否已发布、方向是否相符；选卡也不读绑定 | `PriceCardRegistration` 的方向授权引用、`ArtifactCommercialAuthorization`（注释「这里只登引用不解析其内容」）；`CommercialPricePolicy` 的 `PricingPlan`；ADR-0260 决定一；`internal/parcelpricing/adapters/` 下没有读参与方与商业的适配器 | [04](./issues/04-price-card-registration-verifies-the-direction-authorization-it-cites.md) |

## Pricing

| # | 要求 | 结论 | 现状 | 证据 | 票 |
|---|---|---|---|---|---|
| 6 | 销售/采购价卡 | 满足 | 方向分销售、采购、内部三种，互相隔离 | `PricingDirectionSell`、`PricingDirectionBuy`、`PricingDirectionInternal`；`PriceCards` 的 `Register`；页面 `price-card-catalog` | — |
| 7 | 版本 | 满足 | 已登记版本不可覆盖；同一版本号内容不同答内容冲突；旧版经截止或退役离开新的选择 | `RegisterPriceCardHandler` 的 `CONTENT_CONFLICT`；`/pricing-price-card-successor-cuts`、`/pricing-price-card-retirements`（ADR-0234）；内容读口 `GET /pricing-price-cards/content` | — |
| 8 | 草稿编辑 | 部分满足 | 草稿来自 Excel 模板导入或管理台新建向导，整份提交；批准前同一版本号重新提交即替换，批准后内容固定。不能逐格改一份已存的草稿，也不能把已登记的价卡读回编辑器改出新版 | `PriceCardDraft` 的 `ResubmissionOf`；`POST /pricing-price-card-drafts`、`GET /pricing-price-card-draft-views`；ADR-0349 决定八「已登记价卡反推成模板」一条；`apps/admin-web/src/pages/pricing/card-form/workbook-to-form.ts` 头注「编辑器只能新建」 | [01](./issues/01-registered-price-card-reads-back-as-an-editable-workbook.md) |
| 9 | 审核发布 | 满足 | 草稿经已校验、已批准、已发布三步；批准按租户的价卡审批职责规则裁；发布交既有的价卡登记 | `PriceCardDraft` 的 `Approve`、`PriceCardApprovalDutyRule`；`PublishPriceCardDraftHandler`；`/pricing-price-card-draft-approvals`、`/pricing-price-card-draft-publications` | — |
| 10 | 发布冲突检测 | 部分满足 | 查的是内容冲突：同一版本号内容不同，或规范化版本不可比；卡内的区间、单位、币种在构造时校验。两版适用期重叠不在预览、批准或发布时提示，到选卡时才答适用冲突 | `RegisterPriceCardHandler` 的 `CONTENT_CONFLICT`、`CANONICALIZATION_DIFFERS`；`PriceCardApplicabilityConflict`、`PriceCards` 的 `ResolveInForce`；在 `internal/parcelpricing/application` 与 `adapters/http` 的非测试代码里检索「重叠、overlap」无命中 | [02](./issues/02-price-card-preview-and-publication-show-overlap-with-registered-versions.md) |

## Rating

| # | 要求 | 结论 | 现状 | 证据 | 票 |
|---|---|---|---|---|---|
| 11 | 分票计费 | 部分满足 | 领域能按委托合计计价，导入模板能登记这样的卡。生产上没有形成票级输入的路径，销售计费请求按包裹发起。按件派生再求和的聚合方式已有决定，代码未到 | `AggregationPerShipment`、`NewAggregatePricingInputSnapshot`（生产代码无调用方）；`SellEvaluationRequestSpec` 的 `Parcel`；`internal/parcelpricing/adapters/transportfulfillment` 的 `missingSingleMember`；ADR-0364 决定三，与冻结清单「记录」里出处写它的那一行 | [06](./issues/06-shipment-level-weight-aggregation-lands-in-the-domain-and-the-import-template.md)、[07](./issues/07-shipment-level-sell-charging-enters-production.md)、[14](./issues/14-buy-side-multi-piece-charge-occurrences-cannot-be-priced.md) |
| 12 | 分件计费 | 满足 | 正式评价逐包裹形成：销售按包裹计费时点，采购按运输收费发生项 | `AggregationPerPackage`、`EvaluatePricing`；ADR-0190 | — |
| 13 | 体积重 | 满足 | 重量方法取实重，或实重与体积重的较大者；体积系数由价卡声明 | `PricingWeightActualOnly`、`PricingWeightMax`、`CalculatePricingWeight` | — |
| 14 | 阶梯运费 | 满足 | 价表有重量段乘分区、首重加续重、计费重乘单价三种 | `RateTableFamilyWeightZone`、`RateTableFamilyFirstContinue`、`RateTableFamilyUnitPrice` | — |
| 15 | 燃油附加费 | 满足 | 按基数百分比计收，费率取燃油费率序列当期值乘卡上的折扣系数；序列登记后经复核才在用 | `NewSeriesRateSurcharge`、`ReferenceSeriesFuelRate`；`/pricing-reference-series-registrations`、`/pricing-reference-series-reviews`；页面 `reference-series` | — |
| 16 | 毛利 | 部分满足 | 毛利归结算与经营核算，按口径由费用与成本的组成项派生；领域、落库、读口、页面都在。派生没有生产入口，经营结果册因此是空的。低毛利提示已有决定，代码未到 | `DeriveOperatingResult`；`AllocateCostsHandler` 的 `Derive`（只在 `cmd/parcel-api/assemble_settlement_orchestrations.go` 里构造，没有调用方）；`GET /settlement-operating-results`；页面 `operating-metrics`；ADR-0366 决定三；`UC-SA-006`「给开发的交接」 | [05](./issues/05-operating-results-are-derived-per-parcel-from-direct-charges-and-costs.md) |

## Quotation

| # | 要求 | 结论 | 现状 | 票 |
|---|---|---|---|---|
| 17 | 报价创建 | 待开发 | 没有报价对象 | [10](./issues/10-quotation-object-entry-and-approval.md) |
| 18 | 报价审批 | 待开发 | 同上 | [10](./issues/10-quotation-object-entry-and-approval.md) |
| 19 | 报价发出 | 待开发 | 同上；对客通知的出向连接器也还没有实现 | [11](./issues/11-quotation-is-sent-as-an-exported-rate-sheet.md) |
| 20 | 客户接受 | 待开发 | 同上 | [12](./issues/12-customer-acceptance-of-a-quotation.md) |
| 21 | 价格生效申请 | 待开发 | 同上 | [13](./issues/13-price-effectiveness-application.md) |

这五项的证据是同一组：

- 在 `internal`、`cmd`、`apps/admin-web/src` 的非测试代码里检索 `quotation`，没有业务命中。
- 现行文档写着不做：`docs/product/PILOT-SCOPE.md`「首发不对客户形成价格承诺：报价有效期、锁价和报价接受不进入首发。」；`docs/domain/parcel-pricing/CONTEXT.md`「正式报价接受、锁价及 Quote 生命周期仍不进入首发」；`docs/application/README.md`「它不单独建立报价、合同或账务用例。」
- 最接近的能力是运营试算 `POST /pricing-estimates`（`FormEstimateEvaluationsHandler`，页面 `pricing-estimate`）：只算不存，不进结算，不形成客户承诺（ADR-0152）。

## 读这张表要注意的五处

1. **「生效」没有单独的人工动作。** 发布时带有效区间，到点自动生效。客户要的若是批准之后另有一步人工生效，第 4 项要改判部分满足。
2. **「毛利」不在计费里。** 计价只出评价金额；毛利由结算与经营核算从费用和成本派生。毛利也不是接单或择优的门槛（ADR-0366）。客户要的若是计费时随每票给出销售价减采购价，那是另一项能力。
3. **「分票」读作一票多件按整票计费。** 这是从「分票/分件」的并列读出来的。没有可以去确认的对方，会话对着行业做法核过一遍（[spec](./spec.md)「待用户确认」Q2，依据在[行业做法笔记](./industry-practice-notes.md)）。
4. **「价卡授权」读作合同一侧对价卡的授权。** 今天与它最近的两样是价格方向授权和价格政策的定价方案绑定，两样都只留痕。照行业做法，要补的是登记门上的核对，不是让评价去读合同（Q4）。
5. **演示时登记新形态的价卡要先开部署开关。** 方案记着基础版之外规范化版本的价卡，登记口在开关关着时答 `NEWER_CANONICALIZATION_NOT_ENABLED`（ADR-0352）。它不影响第 9 项的结论，但会影响现场。
