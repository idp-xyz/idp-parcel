# 首方 API：随首方 Intake 实施并行的三张机制票

Category: enhancement
Status: in-progress——2026-10-19 ADR-0139–0142 已由用户在 Claude Code 会话里答「接受」（按复核建议修订后接受），本批激活；首方 Intake 实施票立为 [00](./issues/00-first-party-customer-intake.md)。此前 draft

取证基线 `a608536d`（ADR-0139 草案修订落 main 的那一笔）。本批与 ADR-0140 / 0141 / 0142 三份草案同笔起草，全部 `Status: draft`，**在 ADR-0139 被接受之前不激活**——三张票都建在首方族的客户渠道册与两步校验上，那两件今天只是草案里的形状。

## 这一批要解决什么

通道 1 对「我们的内外 API 是不是科学、专业、先进的解决方案」的复核（2026-09-16）答了六件缺项。前三件是难逆转取舍，各立 ADR：契约生产与版本（[ADR-0140](../../docs/adr/0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md)）、集成方沙箱（[ADR-0141](../../docs/adr/0141-integrator-sandbox-is-a-deployment-form-of-the-first-party-api-on-synthetic-data.md)）、出向事件（[ADR-0142](../../docs/adr/0142-first-party-webhooks-are-a-product-owned-outbound-channel.md)）。后三件不改任何已接受的决定，是首方 Intake 落地那张实施票旁边就能做的机制工作，落成本批三票：

- 客户 API 的 scope 与 1:N 绑定——今天一个 client 拿到的是全部命令面；ADR-0139 风险点 5 把 1:1 / 1:N 留给了 owner。
- 按 client 的限流与配额——一个公开 API 没有 `429` + `Retry-After` 就没有对滥用与误配的第一道保护。
- 按 client 的可观测性——ADR-0022 已要求按 `outcome` 统计，加一个 client 维就是集成方自助排障与 SLA 的底。

三张票共用一个前提：**首方族的客户渠道册已经立了行**（ADR-0139 Decision 二）。scope 挂在行上、配额挂在行上、指标按行上的 client 维聚合。所以它们的阻塞边都指向 ADR-0139 的接受（01、02 还指向首方 Intake 的实施票，03 还指向 ADR-0140 的接受——`requestId` 进问题详情是 0140 定的），不互相阻塞；逐票的边见下表。

## 不在本批

- 契约字段、`outcome` 词表内容——各上下文 owner 的用例。
- ADR-0140 / 0141 / 0142 各自的实施票——各 ADR 接受后按其 Consequences 另立。
- 沙箱里的事实注入面——ADR-0141 风险点 4，单独裁。

## 子票

| 号 | 题 | 阻塞边 |
|---|---|---|
| [01](./issues/01-customer-api-scopes-and-account-binding-cardinality.md) | 客户 API 的 scope 模型与 1:N 账户绑定 | ADR-0139 接受；首方 Intake 实施票 |
| [02](./issues/02-per-client-rate-limit-and-quota.md) | 按 client 的限流与配额：`429` + `Retry-After`，缺省朝拦 | ADR-0139 接受；首方 Intake 实施票 |
| [03](./issues/03-per-client-observability.md) | 按 client 的可观测性：`outcome` × client 维、`requestId` 对账、暂停出声 | ADR-0139 接受；ADR-0140 接受（`requestId` 进问题详情） |
| [04](./issues/04-ps-production-ownership-directory-before-the-write-switch-retires.md) | PS 生产归属目录：隔离写开关退场的前件（00 立） | 无 |
| [05](./issues/05-admin-web-submission-page-has-no-customer-credential.md) | 管理台的委托提交页没有首方客户端凭据：提交答 401（00 立） | 无 |

## 激活条件

ADR-0139 的 `Status` 翻为 Accepted、首方 Intake 实施票立出之后，把本文 `Status` 改 `in-progress`、01 与 02 改 `ready-for-agent`；03 还要等 ADR-0140 翻为 Accepted 才改。此前任何一张票开工都是在草案上建东西。

## 评审记录

**评审 ← 通道 2（非作者）· 钉 `21b20938` · 基线 `a608536d` · 2026-09-16 17:52 / 17:55。** Standards 阻断 0 / 非阻断 4；Spec 阻断 1 / 非阻断 7。评审全文在任务 `task-54cee2f9` 的 report_task。

**阻断 1 已在同分支修**（下一笔）：ADR-0141 Decision 五 / Consequences 第三条 / 风险点 5 / Links 引的「ADR-0140 Consequences 里的契约集成测试」在 0140 里不存在——改为 0141 自己立「对真实例的契约集成测试」这一项，0140 只管生成与包内一致性测试。同笔顺带修了三条非阻断里不需要裁量的：0141 Decision 一 / 三 / 八首条收窄为「除 Decision 二那道只朝拦的登记前缀门外」（Spec 2）；0140 Decision 七在 Consequences 补落点（Spec 6）；本文正文阻塞边与子票表对齐（Spec 7）。

**留给 owner 在接受时裁的非阻断**（草案正文未动，接受前不是依据）：

- 0140 Decision 五把「什么算破坏性变更」外包给 docs/README 登记为非权威的方案稿那张表——接受前内联判据，或指定生成契约文档为权威（Standards 1）。
- 0141 Decision 二的「只接受合成标识」参数形状上是按环境生效的开关，0141 以「只朝拦」豁免 ADR-0078 Decision 四——是否认可这条豁免（Standards 2）；末句「与 ADR-0091 Decision 三同一形状」说宽，0091 还有来源维（Standards 3）。
- 0142 Decision 六「时间窗长度进契约文档」与 Decision 九 / 风险点 4「登记册增行」同一个值两种归属，接受前定一处（Spec 3）；Decision 五单一路径与 Decision 八 `SubmitToChannel` 实现「落投递账并返回」是否第二条入账路，以及「失败」层如何回到 VE，接受前说清（Spec 4）；Decision 四「写成 ADR-0140 的描述符」而 0140 只定了端点描述符，事件描述符形状待 0140 接（Spec 5）。
- 票 02 正文「倾向未配置即不限流但出声」与标题「缺省朝拦」相反，且逆仓内 ADR-0052 / 0054 / 0055 / 0078 / 0091 一贯的缺省朝拦；票 02 把 `429` 作为 ADR-0022 的例外只写进契约文档即第二处口径，按 AGENTS 应另立小记录（Standards 4、Spec 8）。两条都归 owner，票激活前一并裁。

## 文档自洽审计复核（2026-10-21）

[docs-consistency/audit/ps.md](../docs-consistency/audit/ps.md) PS-24（取证 [ps-code.md](../docs-consistency/audit/ps-code.md) 第 12 条）点名：ADR-0140 已接受、首方 Intake 已接上提交口，`/v1`、`application/problem+json` 与端点描述符都没落地，也没立实施票。复核于 main `67438a0f`，现象仍在（`cmd/parcel-api/endpoints_submission.go` 仍挂 `"/shipment-requests"`，`adapters/http/submit_shipment_request.go` 仍写自造的 `problemResponse`）。实施票立为 [docs-consistency/code/22](../docs-consistency/code/22-customer-face-http-contract-misses-accepted-adrs.md)（连同 PS-07 的客户面 `!found` 答 404，分级 R1：基线前必须定形），作为 ADR-0140 的实施票挂在本规格「子票」之外的承接处，不改上表。


## 五口上线（2026-10-26 定，会话 tops-20）

出处：用户 2026-10-26「都需要」（首个租户上线时客户经首方 API 要能撤回、取消、修订、补充、索赔）、「每个客户系统只代表它自己的账户（直客），只考虑这个吧」、「首发不要客户状态读口」、「首发不做限流」；技术取舍由会话 `tops-20` 代决（用户「你帮我决定吧」）。 摸底报告：[customer-faces-scoping-2026-10-26.md](./customer-faces-scoping-2026-10-26.md)。

**范围**：首方客户 API 在首个租户上线前接通撤回、取消包裹、资料修订、补充、索赔五口（今天 `cmd/parcel-api` 全挂 `UnconfiguredIntake{}`）。**只做直客**：一个客户系统只代表它自己的账户（1:1，ADR-0139 首发口径），册上绑定即授权；代理人/平台 1:N（first-party-api/01）不在本轮。**首发不做**客户委托状态读口（靠写口回执、同请求重放与客户追踪视图）、不做按客户端限流（first-party-api/02）。

**取舍（代决，落在票 06 立的 ADR-0310 里）**：
- A 请求键空间：撤回、修订、补充、取消、索赔的来源请求键推导时加命令前缀（如 `withdrawal:<接入请求标识>`），同一标识跨命令不撞；契约写明标识在命令内唯一。
- B 授权：直客 1:1。补充只靠账户绑定 + 授予 + 基准版本核对；修订保留 PC 裁定（UC-PS-002、ADR-0116），请求方即客户本人、不涉委派。
- C 取消授权规则：租户取值，产品不代拟；演示租户登参考配置「客户系统可在收寄前取消」，首个真实租户上线实施时登记本租户的取值，没登答`未配置`、取消口对该租户不开。
- D 不做客户状态读口（用户定）。
- E 补充的基准版本：提交回执返回提交版本标识，补充载荷带回；回执今天若没有就在票 06 补上。
- F 索赔唯一性：以（账户、批次、项）为自然键，接入请求标识只作传输重放。

**拆票与归属**（tops-20 = 接线与 accessidentity；tops-26 = PS 应用层）：
| 票 | 内容 | 规模 | 依赖 | 归属 |
|---|---|---|---|---|
| [06](./issues/06-customer-command-faces-common-base.md) | 五口共用底座：Mint 骨架、能力词、请求键推导、access_identity 迁移、形状版本、演示授予与准入范围、ADR-0310 | M | — | tops-20 |
| [07](./issues/07-customer-withdrawal-face.md) | 撤回口 | M | 06；code/36、code/44（tops-26） | tops-20 |
| [08](./issues/08-customer-supplement-face.md) | 补充口 | M | 06 | tops-20 |
| [09](./issues/09-customer-claim-face.md) | 索赔口（并 demo-chain-gaps/102） | M | 06 | tops-20 |
| [10](./issues/10-customer-source-data-amendment-face.md) | 资料修订口 | L | 06；code/44 | tops-20 |
| [11](./issues/11-customer-parcel-cancellation-face.md) | 取消口 | M | 06；取消生命周期闸门、code/98（tops-26） | tops-20 |
| [12](./issues/12-customer-command-faces-demo-and-contract.md) | 五口演示种子收尾与客户契约补章 | S | 07–11 | tops-20 |
| code/36、44、98、99、dc/76、取消生命周期闸门（新立） | PS 应用层前置 | — | — | tops-26 |
