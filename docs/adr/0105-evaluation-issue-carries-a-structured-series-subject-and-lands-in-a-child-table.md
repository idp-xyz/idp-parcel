# ADR-0105：评价问题项带结构化的「涉及序列」主体，进快照不进规范化文档；问题项落子表不落数组，读面按序列种类分组只读子表

Status: Accepted（2026-09-03，用户经 IDP 队列通道 3 授权本会话「有全部权限」自决并向各会话派工。裁决能力边界：读过票 [pricing-reference-series-operations/05](../../.scratch/pricing-reference-series-operations/issues/05-coverage-horizon-and-blocked-evaluations-read-face.md) 全文（含 MCP-1 摆出的两件待裁）、`internal/parcelpricing/domain` 的 `EvaluationIssue`、`resolveSeries`、`ReferenceSeriesBinding`、评价快照文档与规范化文档中问题项那一段、`ports/evaluation_read.go`、`migrations/parcel_pricing/0001_evaluation.sql`；未重读 parcel-pricing `CONTEXT.md` 全文与 [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md) 之外的规范化相关记录——本记录因此只裁**问题项的结构与落库形状**，不改任何评价语义、不动结果代数、不动状态五格）
Date: 2026-09-03

## Context

票 05 第 2 项要「从评价库按待判断且原因码属于 `REFERENCE_SERIES_UNRESOLVED` / `EXCHANGE_RATE_UNRESOLVED` 计数并按序列种类分组」，票面写的退路是「没有可复用列面就补最小读口，不扫 JSONB」。MCP-1 开工前核出这条退路预设了两件不存在的东西：

- **那一列不存在。** `parcel_pricing.evaluation` 只有 `status`；原因码只活在 `snapshot` 那份 jsonb 里，而票面自己禁止读面扫 JSONB。
- **「按序列种类分组」在领域里没有数据可分。** `EvaluationIssue` 只有 `code` 与 `message` 两格；产出待判断的地方是 `newEvaluationIssue("REFERENCE_SERIES_UNRESOLVED", seriesErr.Error())` 与 `EXCHANGE_RATE_UNRESOLVED` 那一处——序列种类只以自由文本混在 `message` 里。要分组，就得在领域上加一格结构化的东西，那是领域模型改动，不是加一列。

另一件形状问题一并摆了出来：一次评价带的是**一组**问题项不是一个（`withOutcome` 之前可能已追加过 `EXCLUDED_BY_RATE_CARD` 一类），所以「给评价表加一列 `reason_code`」也不对形——要么数组，要么子表。

本记录裁这两件。裁之前先看清一件会决定改动重量的事实：**问题项已经在规范化文档里**。`fingerprint.go` 的 `canonicalEvaluationIssue` 把 `code` 与 `message` 一起折进语义摘要；给规范化文档加一格就会改变每一份评价的摘要，按 ADR-0014 那要升规范化版本，且重放可比性在版本之间断开。而快照文档（`evaluation_snapshot.go` 的 `document.Issues`）与规范化文档是两份东西：前者供持久化与重建，后者供摘要；快照多一格可缺席的字段，旧行读回为空，重建后的整图重验与摘要自校都不受影响。

另一件事实：`resolveSeries` 在失败那一刻手上有整条 `ReferenceSeriesBinding`（`kind` 与 `seriesID`），只是把它们格式化进了错误文字；`EXCHANGE_RATE_UNRESOLVED` 那一处则只有种类没有绑定（结算币种需要汇率，而方案未必声明过汇率绑定）。所以结构化主体的形状是「种类必有、序列标识可缺」，两处产出点都填得出来，不需要从文字里反解析。

## Decision

**一、`EvaluationIssue` 增一格可缺席的结构化主体「涉及序列」。** 形状为「序列种类（`ReferenceSeriesKind`）必有、序列标识可缺」；只有 `REFERENCE_SERIES_UNRESOLVED` 与 `EXCHANGE_RATE_UNRESOLVED` 两处产出点填它（前者填绑定的种类与标识，后者只填 `EXCHANGE_RATE`），其余问题项该格为空。`code` 与 `message` 一字不改——`message` 里那段文字照旧，它是规范化文档的一部分。

**二、主体进快照文档，不进规范化文档；规范化版本不升。** 快照文档的问题项多一格可缺席字段，旧快照读回为空；规范化文档的 `canonicalEvaluationIssue` 不动，语义摘要因此对已落册的每一份评价保持不变。理由：主体是 `message` 里已有信息的**类型化重述**，不是新的评价语义——两份评价若主体不同则 `message` 必不同，摘要已经区分了它们；反之为主体升一次规范化版本，换来的只是同一件信息的第二个副本进摘要，代价却是重放可比性断一次。

**三、问题项落子表 `parcel_pricing.evaluation_issue`，不落数组、不落评价表的列。** 一行一条问题项：评价标识（外键回 `evaluation`）、序号、原因码、序列种类（可空）、序列标识（可空）；与父表同一事务写入、同父表一样追加不改。评价写侧（`EvaluationStore` 的真库适配器）在落评价行的同一事务里落全部问题项；权威内容仍在 `snapshot`，子表是**检索列面**——读回评价对象照旧只经快照装载与整图重验，不从子表重建问题项。种类列若非空须落在 `ReferenceSeriesKind` 封闭集内，由 CHECK 钉住，与 `0003` 对序列版本表种类列的做法同款。

**四、不回填。** 子表自迁移起写入；迁移之前落册的评价没有子表行，也不从 jsonb 里抠出来补——本仓尚无租户，生产库里没有评价；隔离环境重跑种子即可。读面文案对这一格如实：计数覆盖的是子表有行的评价。

**五、读面只读子表，按（状态 = 待判断，原因码 ∈ 那两格）筛、按序列种类分组计数。** 另立伴生读端口，不拓宽 `EvaluationCatalogueRead`（理由同该文件头注：扩写既有接口会拆全部替身）；不扫 JSONB，不在 SQL 里解释原因码之外的任何语义。这一格的数进票 05 第 3 项摘要条「挂起评价数」，与覆盖读口的 `asOf` 同一时刻回显。

**六、不做的，逐条写明。** 不给其他问题项发明主体（`EXCLUDED_BY_RATE_CARD` 的主体是排除规则，不是序列，不进本格）；不把 `message` 改成结构化模板；不给评价表加任何冗余的原因列；不开「按原因码任意筛」的通用读口——要什么列面按 ADR-0077 通例逐口另立。

## Consequences

- `internal/parcelpricing/domain`：`EvaluationIssue` 加主体与访问器；两处产出点填主体；快照文档与重建各多一格；规范化文档与 `hashPricingEvaluation` 不动。领域测试增：主体只在那两处非空、重建往返保主体、旧快照缺该字段可读回、摘要对加主体前后同一评价不变。
- `migrations/parcel_pricing`：新增子表迁移（结构 + 外键 + 种类 CHECK），不种行、不回填。
- `internal/parcelpricing/adapters/postgres`：评价写侧同事务落子表；新伴生读适配器按种类分组计数；真库用例证「一次评价多条问题项各成一行」「非序列问题项种类列为空」「分组计数不读快照」。
- `internal/parcelpricing/ports`：新伴生读端口；`EvaluationCatalogueRead` 不动。
- `cmd/parcel-api`：分组计数读口进端点表（共享接线文件，按频道占号纪律）；管理台摘要条补「挂起评价数」那一格，不摆 0 占位（0 会被读成「没有挂起」，票 05 的 MCP-4 Comment 已记这一格为何不摆）。
- 票 05 第 2 项由此可做；票面「补最小读口」那句退路由实施者按本记录改写为「加领域主体 + 子表 + 伴生读口」。

## Alternatives considered

- **主体进规范化文档并升规范化版本。** 否决：得到的是同一件信息在摘要里的第二个副本，付出的是重放可比性在版本间断开一次；ADR-0014 把升版留给「语义确实变了」的场合，本记录不是那种场合。
- **从 `message` 反解析种类。** 否决：把一段供人读的文字当接口用，文字一改读面就静默错——那是本仓反复记的「两种状态同一张脸」那一族；且 `EXCHANGE_RATE_UNRESOLVED` 的文字里根本没有种类字样。
- **评价表加 `reason_code text[]`。** 否决：数组表达不了「哪一条属于哪个种类」，分组要靠 `unnest` 再拼接第二个数组对位，索引也做不上；一次评价多条问题项本来就是多行的形状。
- **评价表加单列 `reason_code`。** 否决：一次评价带一组问题项，单列只能挑一个，挑哪个就是替读面裁一件领域没裁的事。
- **读面直接扫 `snapshot` jsonb。** 否决：票面红线，且与本上下文「目录只转写列面、权威内容在快照」的纪律相反（`ports/evaluation_read.go` 头注）。
- **回填历史评价。** 否决：无租户无生产评价；为一个空集合写反解析 jsonb 的回填脚本，是给上面第二条否决项开后门。

## Links

- 票 [pricing-reference-series-operations/05](../../.scratch/pricing-reference-series-operations/issues/05-coverage-horizon-and-blocked-evaluations-read-face.md)：两件待裁的出处与 05a/05b 拆法
- [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：规范化版本与重放可比性——本记录 Decision 二不升版的依据
- [ADR-0077](./0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)：读面通例——伴生读口另立、不拓宽既有口
- [ADR-0099](./0099-price-card-binds-series-identity-and-in-force-version-is-derived-from-review.md)：方案绑定序列标识——主体里「序列标识可缺」那一格指的就是它
- `internal/parcelpricing/domain`：`EvaluationIssue`、`resolveSeries`、`ReferenceSeriesBinding`、`canonicalEvaluationIssue`、评价快照文档——本记录在其上加一格，不改摘要
- `internal/parcelpricing/ports/evaluation_read.go`：头注「不拓宽既有写口、伴生读端口另立」的理由，本记录沿用
- `migrations/parcel_pricing/0001_evaluation.sql`：父表；本记录的子表外键回它
- 来源：IDP 队列通道 3 的授权（2026-09-03）
