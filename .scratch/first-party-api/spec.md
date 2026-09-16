# 首方 API：随首方 Intake 实施并行的三张机制票

Category: enhancement
Status: draft

取证基线 `a608536d`（ADR-0139 草案修订落 main 的那一笔）。本批与 ADR-0140 / 0141 / 0142 三份草案同笔起草，全部 `Status: draft`，**在 ADR-0139 被接受之前不激活**——三张票都建在首方族的客户渠道册与两步校验上，那两件今天只是草案里的形状。

## 这一批要解决什么

通道 1 对「我们的内外 API 是不是科学、专业、先进的解决方案」的复核（2026-09-16）答了六件缺项。前三件是难逆转取舍，各立 ADR：契约生产与版本（[ADR-0140](../../docs/adr/0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md)）、集成方沙箱（[ADR-0141](../../docs/adr/0141-integrator-sandbox-is-a-deployment-form-of-the-first-party-api-on-synthetic-data.md)）、出向事件（[ADR-0142](../../docs/adr/0142-first-party-webhooks-are-a-product-owned-outbound-channel.md)）。后三件不改任何已接受的决定，是首方 Intake 落地那张实施票旁边就能做的机制工作，落成本批三票：

- 客户 API 的 scope 与 1:N 绑定——今天一个 client 拿到的是全部命令面；ADR-0139 风险点 5 把 1:1 / 1:N 留给了 owner。
- 按 client 的限流与配额——一个公开 API 没有 `429` + `Retry-After` 就没有对滥用与误配的第一道保护。
- 按 client 的可观测性——ADR-0022 已要求按 `outcome` 统计，加一个 client 维就是集成方自助排障与 SLA 的底。

三张票共用一个前提：**首方族的客户渠道册已经立了行**（ADR-0139 Decision 二）。scope 挂在行上、配额挂在行上、指标按行上的 client 维聚合。所以它们的阻塞边都指向 ADR-0139 的接受与首方 Intake 的实施票，不互相阻塞。

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

## 激活条件

ADR-0139 的 `Status` 翻为 Accepted、首方 Intake 实施票立出之后，把本文 `Status` 改 `in-progress`、三张子票改 `ready-for-agent`。此前任何一张票开工都是在草案上建东西。
