# 09 `SERVICE_PRODUCT` 版本的运营主路径：逐字段表单（版本壳 + 引用）

Category: enhancement
Status: in-progress——2026-09-08 14:2x MCP-3 认领（task-7fc5a960；分支 `mcp3-awf09`，隔离树基 main `0ef63897`）；08 已 resolved、阻塞边解除。此前 ready-for-agent——形状已裁清（逐字段表单，本票无待裁问题），Blocked by 08 未 resolved 前不在前沿；伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`；接管会话 2026-09-07 于 `92579b0a` 逐句核过票面，见伞票 Comments）
Blocked by: 08（已 resolved）

## 册与载荷

显示在**服务产品页**。发布载荷只有版本壳：`kind / objectId / version / scope / effectiveStartsAt / effectiveEndsAt? /
references{}`，`declarations` 里没有为它开的正文通道（`translate.go` 的 `declarationsDocument` 十几格里没有服务产品
正文）。服务产品的属性与渠道映射走另一条登记路（`register_products`，票 admin-remainder-mechanism-batch/02），
这里发布的是**商业版本壳**——它给合同、接单规则包、价格政策一个可引用的版本身份。

## 选形与理由（ADR-0101 决定八）

**逐字段表单。** 频次低（一个产品一年几版）、操作者是运营配置员、载荷是几格标识与一个区间——没有矩阵、没有
子表。表单字段：对象标识、版本号、范围引用、有效起止、引用表（键值对可加行，键的词汇由服务端答、表单不代填）。
摘要与批准照 08 的机制来，表单只呈现服务端答的摘要。

## 硬句

伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md)「硬句」一节逐字适用（不在此复述，那一节是唯一口径）；另一条本册特有：**表单不得替操作者拟引用键**——`references` 是开放词汇，键名从哪来、指向什么
由发布用例与领域答，表单给的是「加一行」不是「从这几个里挑」，除非服务端提供了词表读口。

## 本册规范化判断（2026-09-08，MCP-3；票内小裁决，不改 ADR-0126 任何一句）

**问题**：[ADR-0126](../../../docs/adr/0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md)
Decision 一说规范化文档「只盖正文不盖壳」，而本册**没有正文**——载荷就是壳。文档里放什么？两个候选：
(a) 文档 = `{canonicalization, kind}`，本册每一版摘要同一个串；(b) 把壳上的 `references` 读进文档当正文。

**选 (a)。** 理由：

1. `references` 是壳的一格，不是正文。它在 `PublicationDraftShell` 与 `CommercialVersionSpec` 上都与范围、区间并列，
   `sameReleasedContent`（发布登记册判重放 / 冲突）与 `SameSubmissionAs`（载体判重放 / 修订）都已经把它逐项比进去。
   (b) 让同一格既进壳的逐项比对又进摘要，「同引用换范围」与「同范围换引用」两种修订会一个折成壳不同、一个折成两串
   不同——ADR-0126 Alternatives 否决「摘要盖住版本壳」的那条理由在这里逐字成立。且 (b) 要让对账门从 `Spec.References`
   而不是 `Declarations` 读正文，那是把壳读作正文，对账门会对每一条服务产品批文开门。
2. (a) 下的串是诚实的：本册的正文就是「空」，摘要盖住的就是空（Decision 二原话「正文缺席……摘要盖住的是空」）。它不是
   退化——`PCC-1:<sha256 of {"canonicalization":"PCC-1","kind":"SERVICE_PRODUCT"}>` 与信用政策的串一样带版本、可重算、可比。
   同键重发（壳同）→ `DRAFT_REPLAYED`；换范围 / 区间 / 引用 → `DRAFT_REVISED`（待批准）或 `CONTENT_FIXED`（已批准）；
   落到 `commercial_version` 后由 `sameReleasedContent` 同一套判据分重放 / 冲突。判据没有一处为本册另开。
3. 「只盖正文」对无正文的册读作「正文为空，文档只剩规范化版本与 kind 两格」，是那一句的边界情形不是例外；**不换号**
   （加册不换号，Decision 一）。本册也没有 `ErrPublicationContentAbsent` 那一格——它没有正文槽位，缺席与在场是同一件事；
   `PublicationContent`、`canonicalPublicationDocument`、`CommercialPublicationPayload` 因此**不加格**，`RehydratePublicationContent`
   对本册跳过「正文缺席」判断。
4. **受控批文那一半**：`IsRegisterCanonicalized(SERVICE_PRODUCT)` 答真，但批文 `declarations` 里没有本册的正文通道，
   `publicationContentOf` 对本册答「不在场」——按 Decision 二「正文缺席的版本没有可比对象，照今天登记声明的串」放行。
   因此 `scripts/demo-seeds` 里两条 `sha256:syn-SYN-PROD-*` 一字不动、`cmd/parcel-commercial` 不动。这一格是**有意留的**：
   逼 CLI 对无正文册也声明 `PCC-1:<常量>`，属「何时开始拒收无版本旧串」那一问（Decision 五归伞票收口时裁），不在本票自裁。
   同一对象先经 CLI 以 `sha256:syn-…` 发布、再经表单以 `PCC-1:…` 发布同一版本号，落到登记册答`内容冲突`——两个声明串
   不同，本来就是冲突，与其余各册一致。

**给通道 1 复核的一点**：第 4 条把「已接的册」与「CLI 必须声明算出的串」拆开了——对有正文的册两者同时成立，对本册只有前者。
若通道 1 认为接进规范化就该同时对 CLI 开门（seed 两行随之换串），那是 `publicationContentOf` 加一支 + seed 两行，本票可接，
但要等 10 / 11 重放后再动 seed 文件以免三方撞同一份 JSON。

## 完成判据

服务产品页多一签「发布版本」（表单 → 预览摘要 → 存为待批准 → 批准 → 发布），结果在同页的目录读面立刻可见；
tsc / run-tests 绿；Go 侧只在 08 落的机制上加本册的规范化一格。

## 边界

不动 `register_products` 那条登记路；不动服务产品目录读面。
