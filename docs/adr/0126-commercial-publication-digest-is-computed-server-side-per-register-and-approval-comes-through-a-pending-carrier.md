# ADR-0126：商业发布的内容摘要由服务端按册规范化算出（规范化版本 `PCC-1`，摘要串自带版本）；受控批文仍声明摘要但必须与算出的相等，不等即`未受理`带两串；批准是操作者动作，经一版一行的待批准载体推进，审批职责规则未登记即批准门答未配置；预览与发布走同一份规范化与构造门

Status: Accepted（2026-09-08，用户经 IDP 队列通道 1 派单 `task-cc7313e8` 授权 owner「自决口径：硬句一字不改、每个决定写理由、拿不准或跨上下文归属的点单列越权风险点」，据此对票 [admin-write-faces/08](../../.scratch/admin-write-faces/issues/08-publication-form-path-common-half-server-side-digest-and-envelope-approval.md)「要裁的」三问一次答完。裁决能力边界：读过伞票 [07](../../.scratch/admin-write-faces/issues/07-commercial-publication-operator-main-paths-per-register.md) 硬句与票 08 全文、[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)、[ADR-0101](./0101-operator-facing-registration-payload-shape-is-product-defined.md)、[ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)、[ADR-0031](./0031-owned-repository-write-outcome-is-a-closed-algebra-not-an-error.md)、[ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md) 全文，PC `CONTEXT.md`「商业版本」词条与「商业规则与政策版本」生命周期，`internal/partycommercial/domain` 的 `CommercialVersionSpec` / `NewCommercialContentDigest` / `CommercialObjectKind` / `NewCreditPolicy` / `CreditLimit`，`application/publish_commercial_authority.go` 全文，`cmd/parcel-commercial/translate.go` 的 `publicationItemDocument`，`adapters/http/register_commercial_publication.go`，parcel-pricing 的 `fingerprint.go`（`PPC-5`、`hashCanonical`）与 `preview_reference_series.go` / `reference_series_payload.go`，parcel-shipment 的 `payload_canonicalization.go`（`PSC-1:<sha256>` 摘要串形）；**未重读**十类正文各自的领域构造门（只读了信用政策那一类）、`scripts/demo-seeds` 各批文正文、`internal/accessidentity` 全部——本记录因此只裁**摘要由谁算与摘要串的形、受控批文那一半的对账门、待批准载体的形与批准门、预览口的路径**，不改任何一册正文的形状，不裁其余九册各自的规范化文档（那是各子票逐册接时的事，同一版本号下加册不换号，见决定一））
Date: 2026-09-08

## Context

伞票 07 的硬句：「表单不算摘要、不裁任何门：`contentDigest` 与受理都由服务端答，表单只呈现。」今天的发布口做不到这一句——它**要求调用方声明摘要**：`publicationItemDocument.contentDigest` 是批文里一格字符串，领域 `NewCommercialContentDigest` 只查非空、不从正文算也不与正文对；「同键异内容答内容冲突」（ADR-0031）比的也是这个自报的串。批准是载荷里三格字符串加一格 `approvalRoleStanding`，谁填的、凭什么填，登记册答不上来——ADR-0101 Context 对价卡说的那句在这里逐字成立。`/commercial-publications` 挂的是字面量 `UnconfiguredIntake{}`，操作者身份（ADR-0100 `OperatorEnvelope`）还没到这个端点。

逐字段表单若照今天的口来，要么前端算摘要（硬句禁），要么让操作者手填摘要与批准人——那不是表单，是 JSON 镜像换了皮。九张子票（09–17）全部卡在这里。

已有的先例给了形状：parcel-pricing 的内容摘要由领域一处 `hashCanonical`（`json.Marshal` → SHA-256 → hex）按 `PPC-5` 规范化文档算出，预览口与登记口共用同一段载荷解码、解出同一个领域对象，摘要自然逐字节同答（`preview_reference_series.go` 头注）；parcel-shipment 的载荷摘要串自带版本前缀 `PSC-1:<sha256>`，兑现 ADR-0014「已保存摘要必须携带产生它的规范化版本」而不另开一列。ADR-0101 决定四、五、六已经裁了价卡首例的三件——校验与发布共用一份摘要、批准是独立的操作者动作、审批职责规则缺省朝拦——本记录把它们落到商业发布上，差别只在载体轻重。

## Decision

**一、摘要由服务端按册规范化算出，PC 有自己的规范化版本号 `PCC-1`，摘要串自带版本。** 领域 `CanonicalizePublicationContent` 按对象类别把该册正文折成规范化文档（JSON，字段顺序由结构体钉死，时刻一律 UTC RFC 3339），`json.Marshal` → SHA-256 → hex，摘要串形为 **`PCC-1:<hex>`**——版本随串走（先例 `PSC-1:<sha256>`），不给 `commercial_version` 加列：一列会让「没版本的旧串」与「有版本的新串」在库上长成同一种东西，而串里的前缀让两者一眼可分。规范化文档只盖**正文**（该册的声明正文与它自己的区间），不盖版本壳的身份四元与壳上的范围、区间——身份是键，范围与区间是 `SaveVersion` 逐列比对的项，盖进摘要只会让「同内容换范围」从`内容冲突`折成两个串不同。文档带 `kind`，一册的正文冒不了另一册的名。**首例只接信用政策**（`CREDIT_POLICY`：责任法人 × 权限等级 × 费用类型 × 额度恰一格 × 区间）；其余九册在各自子票里逐册接进同一个 `PCC-1`——**加册不换号**：换号只在既有册的文档形状变化时发生（ADR-0014），一册从「没接」到「接了」不改任何已算出的字节。没接的册答`本册尚未规范化`，是一格答案不是错误。

**二、受控批文那一半走甲：CLI 继续声明摘要，服务端算出后必须相等，不等即`未受理`带两串。** 发布用例 `PublishCommercialAuthorityHandler` 多一格结果 `NOT_ACCEPTED`：对已接规范化的册，声明的 `contentDigest` 与算出的 `PCC-1:<hex>` 逐字节比，不等即整项一行不写、结果带出「声明的 / 算出的」两个串。这是批量口第一道对账门——此前「调用方算错摘要」从来不可见，因为没有任何东西能算出对的那一个。**不改批文既有字段的语义**：`contentDigest` 仍是必填的声明，多的只是这一道相等校验；没接的册与正文缺席的版本没有可比对象，按今天的样子登记声明的串（那种版本没有正文可读，摘要盖住的是空，放它过去不是漏洞——预览与表单路径永远带正文）。乙路（CLI 也不声明、一律服务端算）否决：它要把 `scripts/demo-seeds` 各批文与已施加的登记重放一遍对照，且抹掉了对账门。**seed 一字不动**——今天没有任何 seed 发布信用政策；日后子票接某一册时，该册 seed 的 `contentDigest` 要换成 `PCC-1:<hex>`（`seedgen` 可调领域函数算），那是那张子票的活。**`NOT_ACCEPTED` 与 `CONTENT_CONFLICT` 是两格**：前者是「这一份输入内部不自洽」（声明的串与正文对不上），恢复动作是改批文；后者是「与册上已有的那一版不同」，恢复动作是换版本号。折成一格会让两种续办长成一张脸。

**三、批准是独立的操作者动作，经一版一行的待批准载体推进；发布 = 把已批准的载体交给既有发布用例。** 新增**待批准发布**（`PublicationDraft`）：键 = 版本身份四元（租户 × 类别 × 对象 × 版本号），一版一行；行上有版本壳（范围、区间、指名引用）、正文快照（声明的线格式）、服务端算出的规范化版本与摘要、录入者、状态三格封闭——`待批准` → `已批准` → `已发布`。**录入**（`SubmitPublicationDraft`）由 Intake 把 `OperatorEnvelope` 里的操作者主体作为录入者交进来，载荷里没有身份格，出现即按未知键拒（同 PP 预览口）；同版本同内容再录是重放，`待批准`期间换内容是修订（一版一行，行被替换，录入者与时刻随之更新——这是本上下文唯一一处就地更新的行，理由是它不是权威记录：权威记录是发布后的 `commercial_version`，那一册照旧只插不改），`已批准`之后内容固定、再录答`内容已固定`。**批准**（`ApprovePublicationDraft`）由另一个操作者动作推进，批准者身份同样取自信封，两身份都记且可比；批准门先读**审批职责规则**（租户治理参数：录入者与批准者须否为不同主体、批准者须持哪一格授予），**未登记即答`未配置`、不放行**——分界句同 ADR-0052 / ADR-0101 决定六：读一个空登记册并如实答未配置不是默认实现；不写死「必须双人」也不写死「单人可批」，两者都是替租户定治理规则。**发布**（`PublishPublicationDraft`）只接`已批准`的载体：把载体上的壳、摘要、正文交给既有 `PublishCommercialAuthorityHandler`——`ContentDigest` 即服务端算出的那一个（决定二的相等门由此恒成立），`ApprovalBasis` 的批准引用 = 批准者主体、来源 = 载体自己的引用、时刻 = 批准时刻，`ApprovalRoleStanding` = 已确认（角色是否够由批准门按审批职责规则判过了，这一格不再是载荷里一句自报）；发布用例的答案代数一格不改，发布落定后载体转`已发布`。受控 CLI 不经载体，照旧直接消费批文（ADR-0085 决定一、ADR-0101 决定五）。**不做**模板导入、草稿撤回、草稿查阅面：十册没有一类是矩阵；撤回与查阅是子票接表单时按需立，本记录不预开。

**四、预览与发布是同一条路径。** 新增不落库的预览口 `POST /commercial-publication-previews`：与录入口收**同一份载荷、走同一段解码**（`adapters/http` 的载荷解码与 `Command` 折装只有一处），解出同一组领域值对象，交给同一个 `CanonicalizePublicationContent`——摘要只在领域一处算，同一份载荷过预览与过录入逐字节同摘要（ADR-0101 决定四），由测试钉住。预览交回规范化版本、摘要与**逐格问题**：解码把每一格的构造门结果都收齐再答，而不是撞第一格就停——表单要的是「哪几格不对」，不是「有一格不对」。端点行进 `cmd/parcel-api/endpoints.go`，Intake 挂字面量 `UnconfiguredIntake{}`、未配置即拒（ADR-0085 两阶段），隔离读放行装不进它（编译期，同 PP 预览口）——拟登本体要信封里的租户才立得住，预览若采信自报租户，录入时换成信封里的那个，摘要就变了。**路径叫 `-previews` 不并进发布口加 dry-run 参数**：并进去之后装配点可以把预览编排接到发布端点上而编译仍绿，且「一个参数决定写不写库」是最容易被顺手改错的那种格（同 PP 的判据）。

**五、不在本记录。** 十册各自的规范化文档形状（子票逐册接，同号）；管理台表单页与草稿签（子票）；审批职责规则的登记入口——它是租户治理参数，实例半边，参数登记册增一行登「待提供」，机制半边只立读口与表（写口今天只给测试用，登记面归治理写面那一族按 ADR-0085 决定四另裁）；`OperatorEnvelope` 自身的落地（ADR-0100 那一族在 `internal/accessidentity`，本记录只在 PC 侧立「操作者主体引用 + 授予集」这一个消费面，Intake 到位那天把信封译成它）；旧串的迁移——今天册上的 `sha256:syn-…` 那种没版本的声明串按 ADR-0014 是「不完整」的，本记录不追溯改写它们，也不裁何时开始拒收没版本的串（那要等十册都接完、seed 都换完，归伞票收口时裁）。

## Consequences

- `internal/partycommercial/domain`：新增 `PCC-1` 规范化与 `CanonicalizePublicationContent`（首例信用政策正文 `CreditPolicyBody`）、`CommercialContentDigest.Canonicalization()`（串里有版本才答）、待批准发布 `PublicationDraft` 与其状态三格、审批职责规则 `ApprovalDutyRule` 与批准门 `Approve`、操作者主体引用。`NewCommercialContentDigest` 与既有各构造门一字不改。
- `application`：`PublishCommercialAuthorityOutcome` 多一格 `NOT_ACCEPTED`（结果带声明的 / 算出的两串与成因）；新增 `PreviewCommercialPublication`、`SubmitPublicationDraft`、`ApprovePublicationDraft`、`PublishPublicationDraft` 四个用例，后者转交既有发布用例。
- `ports`：新文件立 `PublicationDraftRegistry` 与 `ApprovalDutyRuleView`（读）/ 具名 Save（写，今天只给装配与测试用）；`ports.go` 不动。
- `adapters/postgres`：迁移 `party_commercial/0028` 立待批准发布表与审批职责规则表；两只适配器。
- `adapters/http`：运营操作者面的发布载荷解码（只有内容，没有身份；未知键拒）、预览端点、载体三口（录入 / 批准 / 发布）；`writePublicationAnswer` 长出 `NOT_ACCEPTED` 那一格。
- `cmd/parcel-api`：四行进端点表带未配置格，探针表各加一行；`cmd/parcel-commercial` 的 publish 子命令把 `NOT_ACCEPTED` 计入 attention 退出码并打印两串。
- PC `CONTEXT.md`：词条「发布规范化版本」「待批准发布」，Rules 加「批准是操作者动作」一句；参数登记册增「商业发布审批职责规则」一行（待提供）。
- 各子票（09–17）接自己那一册时：在 `CanonicalizePublicationContent` 加一册文档（不换号）、载荷解码加一节、该册 seed 的 `contentDigest` 换成算出的串。
- ADR-0127 预留号未用，释回。

## Alternatives considered

- **乙路：CLI 不再声明摘要，一律服务端算。** 否决：要重放一遍 seed 与已施加登记作对照，且抹掉了刚立起来的对账门；甲路多的只是一道相等校验。
- **摘要盖住版本壳（范围、区间、引用）。** 否决：范围与区间已是 `SaveVersion` 逐列比对的项，盖进去会让「同内容换范围」从`内容冲突`折成两个串不同，两种续办长成一张脸。
- **给 `commercial_version` 加 `canonicalization` 列。** 否决：一列让没版本的旧串与有版本的新串在库上同形；串里带前缀两者一眼可分，且不动版本册的形。
- **待批准载体按修订号只插不改。** 否决（暂）：一版一行是票面的形；修订的痕迹今天没有任何读面要，等某一册真要「谁改过什么」再按修订号拆，那时改的是载体不是权威记录。
- **无载体，预览后一键发布。** 否决：录入与批准必然同人同刻，任何审批职责规则都立不住（ADR-0101 Alternatives 第三条）。
- **审批职责规则缺省单人可批 / 缺省双人。** 否决：两者都是替租户定治理规则，红线两个方向都禁。
- **预览做成发布口的 dry-run 参数。** 否决：见决定四。

## 裁决方的能力边界

本记录裁的是**摘要由谁算与串的形、批量口的对账门、载体的形与批准门、预览口的路径**。越权风险点单列供 owner 复核：（1）决定一「文档只盖正文不盖壳」是从 `SaveVersion` 逐列比对推的，没逐册核过十类正文里有没有哪一类把范围或区间当正文的一部分——若有，那一册接进来时文档要带它，不换号；（2）决定三把批准引用 = 批准者主体、来源 = 载体引用写进 `ApprovalBasis`，是对既有三格的解释而非改形——若 owner 认为「来源」该指外部批准文件而不是载体，改的是折装那一处；（3）决定三让`待批准`期间的修订就地替换行，是本上下文唯一一处就地更新，与「只插不改」的口径有张力，理由写在决定里，owner 不认可则按修订号拆载体；（4）决定二「正文缺席的已接册按声明串登记」放过了一类没有正文的版本，理由是它没有正文可读，若 owner 要求已接册必带正文，那是一行改动。

## Links

- [ADR-0101](./0101-operator-facing-registration-payload-shape-is-product-defined.md)：校验与发布共用一份摘要（决定四）、批准是独立的操作者动作（决定五）、审批职责规则缺省朝拦（决定六）——本记录把三件落到商业发布上
- [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)：录入者与批准者身份的出处 `OperatorEnvelope`；本记录在 PC 侧只立消费面
- [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：规范化版本号；`PCC-1` 与 `PPC-*` / `PSC-*` 并列，摘要串自带版本的落法照 `PSC-1:<sha256>`
- [ADR-0031](./0031-owned-repository-write-outcome-is-a-closed-algebra-not-an-error.md)：写入结果是封闭代数——`NOT_ACCEPTED` 与 `CONTENT_CONFLICT` 分格的依据
- [ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)：写面进端点表带未配置格、两阶段接线；CLI 为受控批量口
- [ADR-0052](./0052-network-evidence-catalogue-has-an-unconfigured-grade.md)：「读空册如实答未配置不是默认实现」——批准门未配置格的分界句
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：状态码只说答案有没有形成
- 票 [admin-write-faces/08](../../.scratch/admin-write-faces/issues/08-publication-form-path-common-half-server-side-digest-and-envelope-approval.md) 与伞票 [07](../../.scratch/admin-write-faces/issues/07-commercial-publication-operator-main-paths-per-register.md)：三问与硬句的出处
- [PC CONTEXT](../domain/party-commercial/CONTEXT.md)：「商业版本」「发布规范化版本」「待批准发布」词条
- `internal/parcelpricing/domain/fingerprint.go`、`internal/parcelshipment/domain/payload_canonicalization.go`：规范化与摘要串形的先例

## owner 复核记录

- owner 复核 2026-09-09 认可（用户 2026-09-09 12:3x 经 IDP 队列通道 1 授权「你自决，目标是全部解决」，通道 1 代裁，四条逐条）：1. 「文档只盖正文不盖壳」——伞票 07 子票 09–17 逐册接进后核过：把范围 / 区间当正文一部分的册（结算政策六维、价格政策七格）都在**自己的正文**里带着它们，文档盖的仍是正文，没有一册要把壳折进文档；2. `ApprovalBasis` 批准引用 = 批准者主体、来源 = 载体引用——既有三格的解释，来源指外部批准文件那天改折装一处；3. `待批准`期间就地替换——载体是草稿不是版本，「只插不改」守的是版本，张力可接受；4. 正文缺席的已接册按声明串登记——壳单独发布是合法形态（服务产品无正文；结算政策 seed 也曾只带壳），对账门对壳不比对是 ADR-0126 决定二的既有语义。票 admin-write-faces/08 的两处越权点即此。