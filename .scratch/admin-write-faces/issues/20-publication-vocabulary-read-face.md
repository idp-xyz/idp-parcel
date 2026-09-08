# 20 商业发布词表读口：一口按 kind 答各册正文的封闭集，表单不内置枚举

Category: enhancement
Status: ready-for-agent——形状由通道 1 代裁（用户 2026-09-08 18:3x「你自决」授权，见伞票 07 Comments 同刻那条）：**一个通用读口按 kind 答**，不是一册一口；只答码不答中文；集合的唯一权威在 PC 领域枚举。本票是第 2 波 12/13/15/17 的前置公共半边，与 16 之于第 1 波同一角色
Blocked by: 无（08 与 16 已进 main：五步路径四口与前端公共半边都在场）

## 缺什么

第 2 波四张表单票（[12](./12-acceptance-rule-package-form.md)、[13](./13-pre-acceptance-financial-control-policy-form.md)、
[15](./15-settlement-policy-form.md)、[17](./17-authorization-rule-form.md)）的票面都写着「封闭集由服务端词表读口供下拉，表单
不内置枚举」——理由是 ADR-0126 之后正文由服务端按册规范化，表单若自带一份枚举就是同一封闭集的第二份写法，漂了无人报。
可是这个读口今天不存在：`cmd/parcel-api/endpoints.go` 里商业发布只有五步路径的四口（预览 / 录入 / 批准 / 发布，
`/commercial-publication-*`），admin-web 里封闭集的中文词表各页自持（`presentation.ts` / `*-rows.ts` 那种「集外取值原样示出」的写法），
没有一处向服务端问「这一册正文的某格今天允许哪些词」。四张票同时开工就会四个人各造一口或各内置一份枚举。

## 裁决（2026-09-08，通道 1 代裁）

1. **一口，按 kind 答。** `GET /commercial-publication-vocabularies?kind=<CommercialObjectKind 的线上名>`，答该册正文里所有封闭集：
   每个集合一个名字（就是载荷里那格的键名，例如接单规则包的 `stage` / `intent`，结算政策的 `method`，授权规则的请求方那格）和
   按领域枚举顺序排列的**码**。理由：五步路径已经是一套载荷 `CommercialPublicationPayload` 带 `kind` 分册正文（ADR-0126），词表
   跟着同一条分派走是同一形；一册一口要在 `endpoints.go` 加四行、四张票四个人改同一文件，正是第 1 波刻意避开的撞点。
2. **只答码，不答中文。** 中文留在 admin-web 各页词表（既有约定：集外取值原样示出，不冒充既有一格）。服务端给中文会让呈现层的词
   进 PC 上下文，所有权错位；给码则表单拿到的正是规范化器接受的那些词，「表单内置枚举漂了」那一格从此不存在。
3. **集合的权威在领域。** 各册的封闭集今天已经是领域枚举（规范化器按它拒「集外取值」）；本票在 `internal/partycommercial/domain`
   给每册一个 `PublicationVocabulary(kind)` 式的只读口，从**同一个枚举**列出词——不另写一份常量表。某册没有封闭集（服务产品、
   客户合同的正文没有枚举格）就答空集合列表，不答 404：kind 合法、只是没词。
4. **未知 kind 答问题不答空。** kind 不在 `CommercialObjectKind` 内 → 与预览口同一种载荷问题格式（`PayloadProblemRecord`），不静默。
5. **前端一处取。** `party/publication-draft-api.ts` 加一个 `fetchPublicationVocabulary(kind)` 与类型；表单票各自只消费，不各写 fetch。
   下拉的选项 = 服务端码 × 本页中文词表；词表没收录的码原样示出（沿用 `channel-selection-decisions.ts` 那句判据）。

## 要做的

- 领域：`internal/partycommercial/domain` 新文件 `publication_vocabulary.go`——`PublicationVocabulary(kind CommercialObjectKind) ([]VocabularySet, error)`，
  逐册从既有枚举列词（接单规则包：`stage`、`intent`、判断类型 / 来源 / 终局种类等票 12 点名的那几格；接受前财务控制策略：
  票 13 点名的封闭集；结算政策：`method`；授权规则：请求方那格——各以票面「要做的」为准，**不多列票面没要的**）；测试钉「每个词
  都被该册规范化器接受、规范化器接受的每个词都在集合里」（双向，防漂）。无封闭集的册答空列表。
- http：`internal/partycommercial/adapters/http` 新文件 `publication_vocabulary.go`——`GET`，query `kind`，答 `{kind, sets:[{name, codes:[...]}]}`；
  未知 kind 答 `PayloadProblemRecord`；测试三格（有集 / 空集 / 坏 kind）。
- `cmd/parcel-api/endpoints.go` **加一行**（PC 组，紧跟四口之后；改前广播占号；不动别的行）；端点表测试跟一行。
- admin-web：`party/publication-draft-api.ts` 加 `fetchPublicationVocabulary(kind)` + `VocabularySetRecord` 类型；一个小纯函数
  `vocabularyOptions(codes, labels)`（码 × 中文词表 → 下拉选项，集外原样）+ node:test。不改任何表单。
- 广播落点（函数名与类型名）给 12/13/15/17 的认领人；票 07 子票表加本票一行。

## 边界

不动五步路径四口与其载荷；不动各册规范化器（只读它接受的词）；不加迁移；不动 `ports.go`；不碰 `CommercialPoliciesPage.tsx`
与任何表单文件（那是 12–17 的地盘）；不给中文；不给「默认选中」——票面硬句「表单不给默认、不预选」照旧归表单票。

## 完成判据

`GET /commercial-publication-vocabularies?kind=ACCEPTANCE_RULE_PACKAGE` 答票 12 点名的那几个集合、码序同领域枚举；`kind=SERVICE_PRODUCT`
答空列表；坏 kind 答问题格；领域测试双向钉住集合与规范化器同词；admin-web tsc / run-tests 绿；`endpoints.go` 只多一行；
含 DSN 全仓由推送方重放后跑一次（本票不带 DSN 也应全绿：无 postgres 改动）。

## Comments

- 2026-09-08 18:3x · 通道 1：立票并代裁形状（用户「你自决」授权）。第 2 波派单顺序：本票 + 14（14 不依赖词表）先开；本票落点广播后
  12/13/15/17 再点名派。
