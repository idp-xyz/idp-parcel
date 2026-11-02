# ADR-0354：客户对账单争议经两种运营决定登记与处理，客户不自助提出，相对方确认不记

Status: Accepted（2026-11-01，Claude Code 会话代 SA owner 决（代决）：用户 2026-11-01 经协调方裁定「做管理台入口：客户电话或邮件提出异议，客服代登记，财务核实后处理；按授予收起；不做客户自助口；待确认应收（客户确认）不做」；票 home-today-todo/03。取舍由作者落笔，越权风险点待 SA owner 复核。）<!-- 索引注记 -->｜**入口**：争议的开立与处理（`OpenDispute`、`ResolveDispute`）只到编排，生产上没有口，库上永远没有争议。**取法**：开登记、处理两个运营决定口，各一种授予；作废之后提出的争议不登记在这张单上；两份记录各记操作证据；客户自助与相对方确认不做。
Date: 2026-11-01

## Context

取证于 main `4aade8e49`，只记 `S`。

- ADR-0170 决定一把截单编排 `CutOffPublishStatementHandler` 装在 parcel-api，开立与处理是它的 `OpenDispute`、`ResolveDispute`。`cmd/` 下没有任何口调用这两个方法，生产库上 `statement_dispute` 只能是空的。
- UC-SA-003 输入契约「争议请求」的来源是「客户或授权运营方」；「权限、隔离与审计」要求争议处理单独授权、审计保留争议方与决定方。
- UC-SA-003 结果契约的对账状态有相对方未确认、部分确认、已确认、存在争议四格；代码里没有相对方确认这件事。
- 管理台首页「今日待办」的待对账一格数的是挂着未了结争议的单（票 home-today-todo/01）；争议进不来，那一格在生产上恒为 0。
- 首发的客户主要经电话与邮件对账；客户门户另议（设计源「九、怎么落地」第三步）。

## Decision

**一、登记争议口。** `POST /settlement-statement-disputes`，挂操作者渠道「运营决定」能力面，决定种类 `STATEMENT_DISPUTE_REGISTRATION`。客服照客户来电或来信代登记：载荷点名争议标识（调用方在第一次发出前定下，重发落回同一项）、对账单号、费用、争议金额（最小币单位的整数串）、理由（人写的话，至多 1000 字）与客户提出的时刻（业务时刻，不是录入时刻）。租户取认证结果。不过结算时点那一道门：门管确认与截单什么时候能动，客户什么时候提出异议不由它定。答复：
- 开立了答 `201 DISPUTE_OPENED`，带那一项。
- 同一份重放答 `EXISTING_DISPUTE`，同标识换了内容答 `DISPUTE_CONFLICT`——两份同一编号的登记并发到达、输的那一份同样按内容比，不因落库时撞了主键就一律答已存在；单不在册、费用不在单上、金额越行、提出早于发布答 `SOURCE_NOT_ACCEPTED`，提出时刻在单的作废之后答 `STATEMENT_VOIDED_TAKES_NO_DISPUTE`（决定三）——都是答案（`200`）。
- 册读不动是没形成答案（`500`）。

**二、处理争议口。** `POST /settlement-statement-dispute-resolutions`，同一能力面，决定种类 `STATEMENT_DISPUTE_RESOLUTION`，与登记分开授予：能代客户提出的人不因此能裁定，能裁定的人也不因此能替客户提出。财务核实后点名争议、走向（接受、部分接受、拒绝、待复核）、依据与处理时刻；不带金额——接受与部分接受只产生请金额所有者复核的依据（UC-SA-003「争议与既有调整纳入」）。答复：
- 形成处理答 `201 DISPUTE_RESOLVED`；待复核之后可再处理，再处理也答 `201`。
- 已是终局答 `DISPUTE_ALREADY_RESOLVED` 并交回那一份终局；读到未处理、写时另一人先落了终局，同样答这一格，不答未受理。不在册、处理时刻早于提出答 `SOURCE_NOT_ACCEPTED`。都是答案（`200`）。
- 册读不动是没形成答案（`500`）。

两口的载荷形状写坏——必填格缺、金额为零或带符号、理由或依据空白或超过 1000 字、走向不是那四个词、时刻写不出、自报租户——在入口就按请求写坏拒收，答 `400 MALFORMED_REQUEST`、不进编排，与本上下文别的运营决定口一致；它们不是业务答案，不答 `200`。

**三、作废之后提出的争议不登记在这张单上。** 按客户**提出时刻**与这张单的**作废时刻**比：提出时刻早于或等于作废时刻的照收——客户在作废之前来电、客服在作废之后才代登记，这项异议说的是当时有效的那张单，应当登得上；提出时刻在作废之后的拒，答 `STATEMENT_VOIDED_TAKES_NO_DISPUTE`。作废之前已开的争议照旧在、照旧能处理到终局（待对账一格照样数它）。提出时刻早于这张单的发布时刻，说的就不是这张单，也不收。
- 出处：SA CONTEXT 生命周期「对账单」的「已发布 → 已作废：只适用于整单不应继续有效的明确情况」与「已作废 → 已替代：需要重新出具时形成新对账单并关联原单」。CONTEXT 对作废单收不收新争议没有说，本记录把这一格定下：作废之后整单已不应继续有效，作废之后才提出的异议没有一张有效的单可以落；替代关系今天还没做（UC-SA-003 交接 `AT-SA-069` 一行），所以也不把它转到哪一张单上。
- 归类（ADR-0146）：这是**机制**，不是租户取值——它由作废的含义推出，对每个租户都一样，不留成可配置。UC-SA-003 的 `PAR-SET-09`「争议规则」是租户参数（哪些费用可争议、争议期限之类），与这一条分开：那些取值没登时由各自的口答未配置，不靠这一条。

**四、操作证据只作审计。** 两份命令各带一格操作证据，运营决定口写成 `OPERATOR/<发行方>#<sub>`，落在争议行新加的 `operation_evidence`（代登记的人）与 `resolution_evidence`（形成当前那一次处理的人）两列上（迁移 `settlement_accounting/0052`）。判据同 ADR-0340 决定三：不进内容摘要、不进重放比对、不进任何门；另一位重放留下的仍是首次那一位。处理那一列随裁定三件一起被待复核之后的再处理换掉。争议方是客户本人，不另立一格。

**五、客户自助与相对方确认不做。** 不开首方客户渠道上的争议口；客户确认对账单没有登记口，也不在任何读面上答「待确认」。管理台首页因此不数设计源写的「待确认应收」。

## Alternatives considered

- **登记与处理共用一种授予。** UC-SA-003 要求争议处理单独授权，客服与财务也不是同一批人。否决。
- **开客户自助口。** 用户裁定不做；客户门户另议，到时另立记录。
- **作废单照收新争议，不论何时提出。** 作废之后才提出的那项会挂在一张已不再有效的单上，财务处理完也落不到替代单里。否决，见决定三。
- **作废之后一律不收。** 客户作废前来电、客服作废后才录入的那一项会被挡掉，异议的提出时刻明明在单有效期间。否决，见决定三。
- **记相对方确认。** 用户裁定不做；它要回答谁来记、部分确认按什么分，以及「多久不回复算待催」是不是租户取值，都没有出处。

## Consequences

- access-identity：决定种类 `STATEMENT_DISPUTE_REGISTRATION`、`STATEMENT_DISPUTE_RESOLUTION`；迁移 `access_identity/0037`。
- SA：领域 `OpenStatementDispute` 拒提出早于发布、拒提出在作废之后（`ErrDisputeOnVoidedStatement`），`DisputeResolutionKindFromName`；编排新答复 `STATEMENT_VOIDED_TAKES_NO_DISPUTE`，登记输掉并发时按内容判已存在或冲突，处理输掉竞争时答已处理；两份命令与 `DisputeRecord` 的操作证据；`registrationjson` 两份载荷；`adapters/http` 两口与 `OperatorDecisionIntake` 的两个 Intake；迁移 `settlement_accounting/0052`。
- SA 读面：`GET /settlement-statement-lines?statement=` 读一张单自己的费用行（单上的不可覆盖快照，连同币种、发布与作废时刻），管理台登记争议从这里选费用——目录行只带行数，按页上列的费用册会漏掉页外的费用、也认不出作废单上的行。
- parcel-api：`endpoints_settlement_decisions.go` 多挂两口、`endpoints_settlement_catalogue.go` 多挂费用行一口；`settlementOrchestrations` 多两个包事务的方法。
- 管理台：对账单页客户对账单签右栏两张表单，按两种授予各自收起（ADR-0315）。
- 演示种子：步骤 `510` 给配置员登两种授予、准入对照与权威区间。

## 越权风险点

1. **作废之后提出的争议不收是本记录加的规则。** UC-SA-003 没写；SA CONTEXT 同笔补了这一句。若 SA owner 认为这类异议应当留痕再转替代单，要等替代关系（code/13）做出来再改决定三。
2. **处理人只留最后一位。** 待复核之后再处理，前一位处理人与依据都被换掉（裁定三件本就如此）；审计要看每一次处理，得另立处理历史。
3. **操作证据只写不读。** 对账单读口还没透出登记人与处理人，审计要直查库。
4. **理由与依据是一格自由文字。** UC-SA-003 输入契约还列了证据；附件与证据册不在本记录里。

## Links

- [ADR-0170](./0170-settlement-orchestrations-enter-through-parcel-api.md)：决定一（开立与处理是截单编排上的方法）
- [ADR-0340](./0340-supplier-bill-reception-and-audit-are-two-operator-decisions-and-each-records-the-operator.md)：决定三（操作证据只作审计）
- [ADR-0315](./0315-operators-can-read-their-own-effective-grants-and-the-admin-console-hides-registry-write-entries-only-when-it-knows-the-grant-is-absent.md)：管理台按授予收起入口
- [UC-SA-003](../application/settlement-accounting/UC-SA-003-CUT-OFF-PUBLISH-AND-RECONCILE-CUSTOMER-STATEMENT.md)：`AT-SA-228`、`AT-SA-229`
- 票 [home-today-todo/03](../../.scratch/home-today-todo/issues/03-statement-dispute-registration-and-resolution-faces.md)
