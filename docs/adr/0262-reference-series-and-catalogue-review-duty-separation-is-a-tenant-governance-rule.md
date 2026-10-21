# ADR-0262：计价参考序列与目录复核的职责分离归租户治理——复核者须否不同于登记者、须持哪一格授予，按租户登记的参考复核职责规则判；规则未登记时复核不放行

Status: Accepted（2026-10-21，Claude Code 会话代 PP owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/07，出处审计 [audit/cross.md](../../.scratch/docs-consistency/audit/cross.md) B2-05（文档自洽 02 交「07 或 10」，本票接下，因被裁的两句都在 PP 的记录与 CONTEXT 里）。部分停用 [ADR-0099](./0099-price-card-binds-series-identity-and-in-force-version-is-derived-from-review.md) 决定二一句，被停原句与适用场景见「对已接受记录的作用」；ADR-0099 正文不改，只在其 Status 行与 Links 加前向指针。代码随票 [docs-consistency/code/41](../../.scratch/docs-consistency/code/41-reference-review-gate-reads-a-tenant-review-duty-rule.md)。越权风险点见文末，待 PP owner 复核。）<!-- 索引注记 -->｜**判据**：录入者以外的第二个人这一类规则，与价卡发布、商业发布的批准同属租户治理（ADR-0101 决定六、ADR-0126 决定三）——产品不写死双人，也不写死单人。**形状**：一租户一条参考复核职责规则，声明复核者须否与该版本的登记者为不同主体、复核者须持哪一格授予；序列与目录共用这一条，与价卡发布审批职责规则分立。**缺省朝拦**：规则未登记时人工复核答未配置、不落复核行。**代写**：租户对来源声明免人工复核时，连接器的代写复核（`feed_reference_series.go` 复用同一只复核编排，复核者为连接器身份）带「免复核声明下的系统代写」一格，不读规则、规则未登记也不答未配置，写死的不同主体比较照做。**不变**：复核是追加记录、不改证据等级、退回不删
Date: 2026-10-21

## Context

- **同一类第二个人的规则，一处写死、两处禁止写死。** [ADR-0099](./0099-price-card-binds-series-identity-and-in-force-version-is-derived-from-review.md) 决定二写死序列复核者不得是登记者，[ADR-0109](./0109-zip-classification-facts-are-owned-by-parcel-pricing-as-a-versioned-reference-catalogue.md) 决定二让目录的复核与在用照序列那一套。代码照写：`domain.NewSeriesReview` 遇同一人答 `ErrSeriesReviewerIsRegistrant`，目录侧 `ErrCatalogueReviewerIsRegistrant`，编排落 `NEEDS_ANOTHER_REVIEWER`。同一天立的 [ADR-0101](./0101-operator-facing-registration-payload-shape-is-product-defined.md) 决定六与后来的 [ADR-0126](./0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md) 决定三，把价卡发布与商业发布的「录入者与批准者须否为不同主体」定为租户治理，并明写两个方向都不许写死；PP 已有价卡发布审批职责规则（`PriceCardApprovalDutyRule`，参数登记册 `PAR-SET-12`），PC 有商业发布审批职责规则（`PAR-COM-18`）。
- 后果：单人运营的租户无法让一版手工登记的序列或目录进在用，同一租户的价卡批准却可以登成单人可批；[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 的分界检验在这件事上出了两个答案。

## 候选与反方

- **归产品策略：复核写死双人，把 ADR-0101 决定六、ADR-0126 决定三里两个方向都不许写死的那一句收窄到发布批准门。** 理由是复核确认的是转录、同一个人复核等于没复核。反方：发布批准确认的也是录入，ADR-0101 正是对同一条理由作的相反结论；分开对待只能说清两处的对象不同，说不清为什么谁来做第二双眼睛在一处是产品的事、在另一处是租户的事。否决。
- **复用价卡发布审批职责规则。** 形状相同，可租户对价卡与对序列的要求可以不同——燃油率每周一期、常由一个人照公布页转录，价卡一年几版、要经商务批准；`PAR-SET-12` 与 `PAR-COM-18` 分立的理由（参数登记册原句「租户对价卡与对商业版本的审批要求可以不同」）在这里同样成立。否决，另立一条。
- **序列与目录各一条规则。** 两者同族（ADR-0109 决定二），复核的对象都是一次外部事实的转录；分成两条只多一处要登的格。否决，共用一条。

## Decision

**一、人工复核门按租户的参考复核职责规则判。**（产品策略）

- 一租户一条**参考复核职责规则**，声明两件：复核者须否与该版本的登记者为不同主体；复核者须持哪一格操作者授予（可不要求）。形状照价卡发布审批职责规则。计价参考序列与计价参考目录的人工复核共用这一条。
- 复核者与登记者都记且可比。操作者面上复核者取自操作者信封，连同信封里的授予集；受控 CLI（`cmd/parcel-pricing-register -kind reference-series-review / reference-catalogue-review`）的复核者是批文里的 `reviewer` 串，带不出授予集。规则要求不同主体时，同一人复核答须换人（`NEEDS_ANOTHER_REVIEWER` 的格保留，判据从写死改为读规则），两条路同判；规则要求某一格授予时，操作者面按信封里的授予判，CLI 这一路证明不了授予，一律答复核者不合格——CLI 不绕过规则，要求授予的租户只能经操作者面复核。
- 规则经受控 CLI 登记（`parcel-pricing-register` 增一种登记，一租户一条、同内容重放、异内容冲突），与价卡、序列登记同一族；在线登记面随治理写面那一族另裁。演示租户由种子在序列与目录复核之前登这条规则。
- 规则未登记时，人工复核答`未配置`、不落复核行——不以默认双人或默认单人代替（ADR-0101 决定六「缺省朝拦」同一判法）。
- 规则只管人工复核。今天的事实：租户对某一来源显式声明免人工复核时，连接器的代写复核由 `application/feed_reference_series.go` 调同一只 `ReviewReferenceSeriesHandler`，复核者是 `binding.ConnectorIdentity()`，走的就是这道门，也受复核者不得是登记者那一道比较（PP CONTEXT「一版序列取值只有经复核通过才进入在用」那一条）。裁定（协调方 2026-10-21）：代写复核**不读**复核职责规则，规则未登记时也**不答**未配置——租户对该来源的免人工复核声明本身就是租户对这一来源的复核决定；写死的不同主体比较在代写这一路**保留**，防止租户把登记责任方填成连接器身份、自审自批，这也是现行行为。落法：复核命令带一格「免复核声明下的系统代写」，带这一格时跳过规则读取，不同主体比较照做。

**二、复核其余各条不变。**（机制）

复核是追加在版本之旁的独立事实、不改登记行、不改证据等级、退回不删、同一版本可再复核；在用版本按复核时刻派生（ADR-0099）。

三类：一里规则的形状与「缺省朝拦」是**产品策略**，门按规则判是**机制**，某个租户要不要双人、复核者持哪格授予是**租户取值**。演示租户由种子登一条要求不同主体、不要求授予的 `SYN-` 规则，演示动线的行为与今天相同，证据只记 `S`。

## Consequences

- 代码随票 docs-consistency/code/41：领域规则类型、读口与表（形照 `PriceCardApprovalDutyRule`，写口给受控 CLI、装配与测试用——这里有意不照那一套的先例，那一套没有 CLI 登记口）；序列复核与目录复核两只编排先读规则再判，`ErrSeriesReviewerIsRegistrant`、`ErrCatalogueReviewerIsRegistrant` 改为按规则给出；新增未配置与复核者不合格两格；受控 CLI 增规则登记一种、CLI 复核路照规则判；种子在复核之前登演示规则，演示动线不退成未配置。
- PP CONTEXT「序列版本复核」词条改写；UC-PP-001 新增 `AT-PP-014`、`AT-PP-015`、`AT-PP-016`。
- ADR-0099 的 Status 行与 Links 挂前向指针。
- 参数登记册 `PAR-SET-11` 的复核责任方一格补参考复核职责规则的实例登记，交文档自洽 10 改。
- 在 code/41 落地之前，生产上的人工复核照旧写死不同主体。

## 对已接受记录的作用

- **部分停用** ADR-0099 决定二「复核责任方不得与该版本的登记责任方相同（`PAR-SET-11` 分列两种责任的机制落点）。」一句。**适用场景**：计价参考序列与目录的全部人工复核——复核者须否不同于登记者改按租户的参考复核职责规则判，规则未登记时复核不放行。决定二其余（复核是追加的独立事实、四件内容、不改证据等级、退回不删、可再复核）与 ADR-0099 其余各条不变。
- ADR-0109：无条款停用。决定二让目录的复核与在用照 ADR-0099 给序列立的那一套，这一句照旧成立，目录的门随序列的门一起改。
- ADR-0101、ADR-0126：无条款停用。本记录把它们给发布批准定的判据扩到参考复核。

## 越权风险点

1. **单人租户可以自审转录**（PP owner）。规则登成不要求不同主体时，抄错的数没有第二个人拦——那是租户的选择，取值凭证与登记责任方照旧记在登记行上，事后查得到是谁登、谁复核。
2. **规则未登记时复核停住**（PP owner）。真实租户上线前必须先登这一条，否则手工登记的序列与目录一版都进不了在用、评价停在待判断；上线清单要列它。
3. **一条规则管序列与目录、管所有序列种类**（PP owner）。燃油率、汇率与金额序列的复核要求可能不同；本记录不按种类分，要分时扩规则的形状。
4. **CLI 复核证明不了授予**（PP owner）。受控批文只带复核者串，规则要求授予时 CLI 这一路一律答复核者不合格。序列可以改走操作者面（`/pricing-reference-series-reviews`）逐版复核；目录复核今天只有 CLI，`cmd/parcel-api` 没有目录复核端点，要求授予的租户因此根本无法复核目录——code/41 一并补目录复核的操作者面端点，补上之前这一格如实停住。租户若既要求授予、又要批量复核，得等批文能带上经认证的操作者身份。
5. **代写一路不受租户复核职责规则约束**（PP owner）。租户对某一来源声明免人工复核之后，那一来源的代写复核不看规则里的不同主体与要求授予两格，只保留登记者不得是连接器身份那一道比较；租户要收紧，就撤掉对该来源的免复核声明。

## Links

- [ADR-0099](./0099-price-card-binds-series-identity-and-in-force-version-is-derived-from-review.md)：部分停用决定二「复核责任方不得与该版本的登记责任方相同（`PAR-SET-11` 分列两种责任的机制落点）。」一句
- [ADR-0109](./0109-zip-classification-facts-are-owned-by-parcel-pricing-as-a-versioned-reference-catalogue.md)：目录的复核与在用照序列那一套
- [ADR-0101](./0101-operator-facing-registration-payload-shape-is-product-defined.md)：决定六审批职责规则是租户治理参数、缺省朝拦
- [ADR-0126](./0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md)：决定三同一判据用于商业发布
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：三类分界
- [parcel-pricing CONTEXT](../domain/parcel-pricing/CONTEXT.md)「序列版本复核」、[UC-PP-001](../application/parcel-pricing/UC-PP-001-FORM-ESTIMATE-EVALUATIONS.md)（`AT-PP-014`、`AT-PP-015`、`AT-PP-016`）
- 票 `.scratch/docs-consistency/issues/07-pc-pp-docs.md`；代码票 `.scratch/docs-consistency/code/41-reference-review-gate-reads-a-tenant-review-duty-rule.md`
