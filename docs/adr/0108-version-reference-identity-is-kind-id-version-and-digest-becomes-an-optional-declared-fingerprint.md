# ADR-0108: 版本引用的身份是（种类，标识，版本）三元；digest 退出身份与指纹，成为可选的「声明时附带的指纹」，不进规范化文档

Status: Accepted
Date: 2026-09-04

## Context

`parcel-pricing` 的 `VersionReference` 有四个槽：种类、标识、版本、`digest`。`NewVersionReference` 要求 `digest` 非空，规范化文档（`fingerprint.go` 的 `canonicalReference`）把它折进内容摘要与评价语义摘要，`compareVersionReferences` 也拿它参与排序。可是**这个槽从没有装过一份被比对过的内容指纹**。取证于 `main = 512b419`：

- 领域里没有任何读法拿 `VersionReference.Digest()` 与被引对象的内容比过；重放校验比的是 `PricingEvaluation.planContentDigest` 与方案自己的 `ContentDigest()`，那是另一个字段。
- 域内自造的引用装的是 `builtin:decimal-bigint-v1` 这样的常量串；seedgen 一律写 `sha256:syn-<id>-<version>` 占位；运营操作者路径（票 [pricing-reference-series-operations/08](../../.scratch/pricing-reference-series-operations/issues/08-series-write-face-needs-a-draft-and-a-version-read-face.md)）三处引用都给不出真摘要，MCP-3 于 2026-09-03 裁了过渡做法——铸 `declared:<kind>/<id>@<version>` 令牌（`adapters/http/reference_series_payload.go` 的 `DeclaredReferenceToken`），前缀自报「这是声明不是哈希」。
- 三处给不出的理由各不相同，且只有一处能补：**自身版本引用**装不进参考序列登记的内容摘要——`reference_series_register.go` 的快照文档把自身引用（连 digest）折进了 `ContentDigest`，装进去就自指循环；**口径 `QuoteBasis`** 引 `party-commercial` 的政策版本，PC 读口不透出 `content_digest`；**更正回指 `PriorVersion`** 有来源（前版登记时声明的引用）。
- **领域自己的身份判断早就不含 digest**：`NewVersionManifest` 的去重键是 `referenceIdentity{kind, id, version}`。同一清单里两条只差 digest 的引用被当成重复而拒——也就是说，领域已经认定 digest 不是身份的一部分，只是类型没跟上。

于是册上同时存在三种形状的「digest」（真摘要或占位、`builtin:` 常量、`declared:` 令牌），领域对三者一视同仁。今天没有读法依赖它们可比所以不出事；但一个叫 digest 的槽装着不是 digest 的东西，任何日后想「按引用 digest 校验被引对象没变」的读法都会在这一格上遇到三种语义。票 [09](../../.scratch/pricing-reference-series-operations/issues/09-version-reference-digest-has-no-source-on-the-operator-path.md) 列了三条互斥改法要 ADR 裁一条。

## Decision

**一、版本引用的身份是（种类，标识，版本）三元。** `VersionReference` 的相等、清单去重、排序、规范化文档一律只看这三元——把 `NewVersionManifest` 已经在用的 `referenceIdentity` 提成类型的定义，而不是让类型继续比它自己的去重键多一格。

**二、digest 退出身份与指纹，改为可选的「声明时附带的指纹」（`declaredFingerprint`，命名由实施定）。** 它记的是「引用方在声明这条引用时手上有什么」：有真摘要就带真摘要，没有就空。它进快照文档（重建时原样读回），**不进规范化文档**——内容摘要与评价语义摘要不再受它影响，同一份引用有没有附指纹、附的是哪一种，都不改变摘要。它也不参与 `VersionReference` 的相等。

**三、`NewVersionReference` 不再要求 digest 非空，但三元一格不许空。** 票 09 的红线「不改 `NewVersionReference` 的非空校验来放行空 digest」说的是**在 digest 仍是身份一部分时**放行——那是把问题从命名挪进数据；本记录先把它从身份里拿出来，非空校验随之失去对象，这不是绕过红线而是消除红线的前提。三元的非空与去空白校验原样保留。

**四、规范化版本换号一次（ADR-0014）。** 规范化文档里 `canonicalVersionReference` 去掉 `Digest` 字段，`compareCanonicalReferences` 去掉第四对。既有方案与评价按原版本重放，不追溯改写；旧规范化版本的快照按 `CANONICALIZATION_VERSION_UNSUPPORTED` 既有语义处置。若与 [ADR-0107](./0107-evaluation-amount-rounding-is-declared-by-the-price-card-like-weight-rounding.md) 的换号同期实施，**合并为一次换号**——PPC-2 把三处拓宽合成一次的先例适用。

**五、过渡令牌退役；票 08 的三处引用各归其位。** 自身版本引用只带三元，指纹留空（登记的 `ContentDigest` 是它自己的字段，不必再塞进自身引用，自指循环随之消失）；口径 `QuoteBasis` 只带三元，指纹留空——PC 读口日后透出 `content_digest` 时可以补进指纹，那是可选增强不是前提；更正回指 `PriorVersion` 带三元，指纹带前版登记的内容摘要（有来源）。`DeclaredReferenceToken` 与 `declared:` 前缀删除；`builtin:decimal-bigint-v1` 那条引用改为三元。**预览与登记共用同一条构造**（ADR-0101 决定四）继续成立，且因为不再铸任何令牌而更简单。

**六、不做的两条。** 不采票 09 的改法 1（PRS-2 只排除引用槽）——它只解自身引用一处，口径与回指照旧缺来源，且留下「digest 是身份」这个不成立的前提；不把改法 2（PC 读口透出 `content_digest`）当本记录的一部分——它是可选增强，何时做归 PC owner。

## Consequences

- **类型名与它装的东西一致了**：身份三元、指纹可选，读的人不必再分辨一个 `digest` 字段里是哈希还是令牌。
- **清单去重、相等、排序三处从此同一套判据**，`NewVersionManifest` 里那个 `referenceIdentity` 不再是与类型定义不一致的局部真相。
- **运营操作者路径不再造任何假摘要**——票 08 的解码器与预览删掉铸令牌那一步，两条路的输出天然相同。
- **波及面**：`VersionReference` 值对象、`plan_snapshot.go` / `evaluation_snapshot.go` / `reference_series_register.go` 三处快照文档、`fingerprint.go` 规范化文档、`NewVersionReference` 的全部调用点（价卡、评价清单、参考序列登记、seedgen、测试夹具）。按 [parallel-sessions](../agents/parallel-sessions.md) 的判据这是「会让旧调用点对不上」的一类，实施走三步法（先加三元构造与可选指纹、迁调用点、再删旧签名）或单独 worktree，并在频道占号。
- **将来若要「按指纹校验被引对象没变」**，那是一条新读法：只在指纹在场时校验、缺席即跳过并如实记——它需要另一条决定，本记录不预设。
- 代价：一次规范化换号；旧快照的重放路径要多带一格「旧形状里的 digest 读回后放进可选指纹」的兼容读法，实施时钉测试。

## Alternatives considered

- **改法 1：PRS-2 规范化文档排除引用槽，自身引用装真摘要。** 否决：只解三处中的一处；口径与回指照旧无来源；且保留了「digest 是身份一部分」这一与 `NewVersionManifest` 去重键相悖的前提，令牌过渡得继续活着。
- **改法 2：PC 读口透出 `content_digest`。** 否决为本记录的一部分：跨上下文，且只解口径一处。它作为可选增强仍可做，落进可选指纹即可，不需要再改领域。
- **维持过渡令牌为长期做法。** 否决：三种形状共存的 digest 是本记录要消除的东西，把它写成长期做法等于宣布那个槽永远不可比。
- **digest 保留在身份里、允许为空。** 否决：那是票 09 红线点名的做法——空值进身份等于给「相等」多一种含义（空 = 任何？空 = 只与空相等？），把问题从命名挪进数据。
- **给三处各造一种「真」摘要来源（自身引用改算法、PC 加读口、回指照旧）。** 否决：三条工程各自成立，合起来仍然维持了一个没有人比对的指纹作为身份——付三份代价买一个不成立的语义。

## Links

- [票 pricing-reference-series-operations/09](../../.scratch/pricing-reference-series-operations/issues/09-version-reference-digest-has-no-source-on-the-operator-path.md)：三条改法与取证
- [票 pricing-reference-series-operations/08](../../.scratch/pricing-reference-series-operations/issues/08-series-write-face-needs-a-draft-and-a-version-read-face.md)：过渡令牌的出处与三处引用的来源核对
- [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：规范化版本换号的依据
- [ADR-0101](./0101-operator-facing-registration-payload-shape-is-product-defined.md)：决定四「预览与登记铸法同一条」——本记录使其在没有令牌的情况下继续成立
- [ADR-0107](./0107-evaluation-amount-rounding-is-declared-by-the-price-card-like-weight-rounding.md)：同期换号可合并为一次
- [ADR-0099](./0099-price-card-binds-series-identity-and-in-force-version-is-derived-from-review.md)：价卡对序列绑的是身份不是版本——与本记录「身份三元」同一立场
