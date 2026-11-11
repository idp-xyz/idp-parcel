# 10 已批准的价卡草稿没有撤下的路——批准之后才发现不该发布的草稿，只能一直停在已批准，随时可以被人发布

Category: enhancement
Status: draft
父票：[spec](../spec.md)
Blocked by: 票 [09](./09-quotation-domain-language-and-use-case.md)（撤下这一格由票 08 的 ADR 定，由票 09 写进计价 `CONTEXT.md`）
地盘：`internal/parcelpricing`（领域、应用、`adapters/postgres`、`adapters/http`）、计价草稿册的迁移、`cmd/parcel-api` 端点表、`apps/admin-web/src/pages/pricing`

对应要求：Quotation「客户接受」的另一面——客户拒绝、报价过期或改了价之后，报出去的那份价格内容怎么收场。

## 现象

取证钉 `535202926`。

- 草稿的状态是`草稿`、`已校验`、`已批准`、`已发布`（`PriceCardDraftStatus`）。
- 批准之后内容固定，同一版本号换内容答 `CONTENT_FIXED`，要另起版本号（`PriceCardDraftResubmissionContentFixed`）。
- 已批准的草稿只有一个去处：发布。发布命令只按租户与方案版本指名草稿（`PublishPriceCardDraftCommand`）。
- 在 `internal/parcelpricing/domain` 与 `application` 讲草稿的非测试文件里检索「withdraw、discard、abandon、reject、cancel、撤回、撤下、作废、驳回」，无命中。
- 计价 `CONTEXT.md` Lifecycles「定价方案与价表版本」一节写的是草稿、已校验、已批准、已发布、已退役这一条线，没有批准之后不发布的一步。

后果：报给客户的一套价被拒绝或过期之后，那份草稿还在册上，状态与一份等着发布的草稿一模一样。

## 要做的

照 ADR 与票 09 落文后的规则实现。这里只列切片的边。

1. **草稿加一格终态`已撤下`**：只从`已批准`进入，记撤下人、时刻与原因。
2. **撤下的草稿不能发布**，答一格业务结论。它的版本号不再使用：同一版本号再录入答已撤下，不替换。
3. **读面**：草稿列表与详情显示已撤下、撤下人、时刻与原因，可按状态筛选。
4. **管理台**：已批准的草稿详情加「撤下」。
5. **不做**：撤下`已校验`的草稿（它可以被同一版本号的重新录入替换）；撤下已发布的版本（那走截止或退役，[ADR-0234](../../../docs/adr/0234-an-older-price-card-version-leaves-new-selections-only-through-an-explicit-closure-for-a-named-successor.md)）。

## 要裁的

- **谁可以撤下。** 录入者、批准者，还是持某一格授予的人。撤下只让一份草稿不能发布，不放行任何东西，建议不套审批职责规则。
- **撤下之后能不能恢复。** 建议不能；还要用就另起版本号重新录入、重新批准。
- **有发出记录的草稿被撤下**，那几条报价读出`所引价格已撤下`（票 [12](./12-customer-acceptance-of-a-quotation.md)）。撤下时要不要先提示有几条报价还在等答复。

## 三类

全部是机制。没有租户取值。

## 完成判据

- 已批准的草稿撤下之后，发布不放行并答已撤下；有真库用例。
- `已校验`与`已发布`的草稿撤不了，各答各的一格。
- 撤下的版本号再录入，答已撤下。
- 迁移之前已在册的草稿行读回不变。
- 管理台能撤下并看到撤下记录。
- `UC-PP-001`「价卡导入」一节与 AT 号同步。

## Comments

- 2026-11-11：本票原题「报价对象、录入与批准」，随 ADR 草案第二稿撤掉：报价不再有自己的对象状态与审批职责规则，创建与审批由价卡草稿的录入与批准回答，报给谁、有效到何时并进票 11。编号改给草案新增的撤下这一格。
