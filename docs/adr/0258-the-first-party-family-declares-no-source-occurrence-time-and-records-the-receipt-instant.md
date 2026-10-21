# ADR-0258：首方客户 API 不收来源方声明的发生时间，信封的来源发生时间记本系统收到这次请求的时刻；客户要表达的业务时间只走客户请求生效时间

Status: Accepted（2026-10-21，Claude Code 会话代 PS owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/06，出处审计 [audit/ps.md](../../.scratch/docs-consistency/audit/ps.md) PS-30（取证 [audit/ps-code.md](../../.scratch/docs-consistency/audit/ps-code.md) 第 16 条）。填 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) 没写的一格（首方族信封的来源发生时间怎么取），无条款停用。代码不改。越权风险点见文末，待 PS owner 复核。）<!-- 索引注记 -->｜**取法**：首方族的来源信封由产品的接入适配器铸造，来源发生时间与系统接收时间都取本系统收到这次请求的时刻；首方契约不设客户声明发生时间的一格。**理由**：来源信封客户不可声明（UC-PS-001 输入语义契约，ADR-0139 决定一信封铸造归产品）；同步调用的发生就是送达；客户想表达的业务适用时间有`客户请求生效时间`一格，进规范化内容与摘要。**适用场景**：只限首方客户族；其余接入族各按自己的信封取法，作业事实的发生时间仍由设备签发（ADR-0023）
Date: 2026-10-21

## Context

- PS CONTEXT 词条「来源发生时间（`occurredAt`）」：「来源方声明请求或事件发生的时间。它属于来源信封元数据，用于溯源、迟到和因果判断，不进入规范化业务内容摘要」。
- UC-PS-001 输入语义契约「来源信封」行：客户可声明「否」，权威来源是「接入适配器与接入来源」。
- [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) 决定一把首方渠道的来源信封铸造判给产品；决定五把「接入请求标识」定为载荷里的一格。它没写首方族的来源发生时间取什么。
- 代码（取证于 main `f329f5059`）：首方提交 Intake `IntakeSubmission` 把 `OccurredAt` 与 `ReceivedAt` 取成同一个时钟值。首方提交的两类时间因此恒等。
- 「来源方声明」与「客户不可声明」在首方族上相抵：首方族的来源方就是客户系统本身。

## Decision

**一、首方族的来源发生时间记本系统收到这次请求的时刻。**

- 首方客户 API 的来源信封由产品的接入适配器铸造。来源发生时间与系统接收时间都取接入适配器收到这次请求时的本系统时刻。
- 首方契约不设「客户声明的发生时间」一格。

**二、客户要表达的业务时间只走客户请求生效时间。**

- 客户希望某份请求「从何时起在业务上适用」，写在`客户请求生效时间`（`requestEffectiveAt`）。它属于规范化业务内容，值与缺失或显式存在的状态进入 `PayloadDigest`，按 CONTEXT 各条参与判断。
- 来源发生时间在首方族上不承担迟到或因果判断：同步调用的发生就是送达，客户系统重发同一请求是同一来源身份下的重放，按 CONTEXT 只追加观察。

**三、只限首方客户族。**

- 其余接入族各按自己的信封取法。租户既有渠道（文件、门户、客户自有契约的 API）的来源发生时间若由来源方声明，照 CONTEXT 词条原义；作业事实的身份与发生时间由设备签发（[ADR-0023](./0023-work-fact-identity-and-time-are-minted-by-the-device.md)）。

## 候选与反方

- **首方契约收一格客户声明的发生时间，进信封、不进摘要。** 否决：来源信封客户不可声明，这一格等于让客户写信封；客户系统的时钟不受本系统约束，进信封后它就成了溯源与审计里的一个不可核实的值。客户真要表达的业务时间已有`客户请求生效时间`。
- **首方族不记来源发生时间，留空。** 否决：CONTEXT 要求分别保存两类时间，留空会让每个读信封的地方各自判空；与接收时刻同值、并写明取法，读者一看就知道这一格在首方族上不携带额外信息。

## 对已接受记录的作用

- 无条款停用。本记录填 ADR-0139 没写的一格，ADR-0139 正文与 Status 行不动。

## Consequences

- PS CONTEXT 词条「来源发生时间（`occurredAt`）」补一句首方族的取法，引本记录。
- 代码不改：`IntakeSubmission` 今天的取法即本记录的决定一。首方契约文档写明不收客户声明的发生时间，随首方契约的实施票（[docs-consistency/code/22](../../.scratch/docs-consistency/code/22-customer-face-http-contract-misses-accepted-adrs.md)）。

## 越权风险点

1. **客户系统离线攒批再发时，来源发生时间记的是送达时刻**（PS owner）。客户系统内部创建请求的时刻不进本系统；需要它的业务判断只能靠`客户请求生效时间`。
2. **日后首方契约若要收客户声明的发生时间**（PS owner），改本记录决定一并在契约里加一格；信封与摘要的分工不变。

## Links

- [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md)：首方族信封铸造归产品；本记录填其没写的一格
- [ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md)：首方契约
- [ADR-0023](./0023-work-fact-identity-and-time-are-minted-by-the-device.md)：作业事实时间由设备签发，不在本记录范围
- [PS CONTEXT](../domain/parcel-shipment/CONTEXT.md)：词条「来源发生时间」「客户请求生效时间」
- [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)：输入语义契约「来源信封」行
- 票 `.scratch/docs-consistency/issues/06-ps-docs.md`
