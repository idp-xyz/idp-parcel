# ADR-0342：申报由 dispatch 的一只消费者经登记的通道发出，越过发送边界另记一笔发送事实

Status: Accepted（2026-10-28，Claude Code 会话代 CC owner 决（代决）：用户 2026-10-28 对会话提出的四条决定与三笔拆分答「同意」；票 product-strategy-boundary/10 第 1 项。越权风险点待 CC owner 复核。）<!-- 索引注记 -->｜**入口**：提交执行口只把首次尝试记成`已登记待发送`，提交版本形成信封只有 VE 投影消费，没有任何东西把申报交出去，也没有地方收技术回执。**取法**：dispatch 上给那封信再挂一只发送消费者，经发送目标登记的通道发出；尝试行不改，另记发送事实，技术回执逐份追加，当前技术判断折出来。
Date: 2026-10-28

## Context

取证于 main `5678c6c8c`，只记 `S`。

- ADR-0227 决定四：提交执行口固定提交版本，首次尝试记 `REGISTERED_PENDING_SEND`、`sent_at` 为空，提交执行口不发送任何东西。越权风险点 7 留了一问：越过发送边界，是改这一份尝试的状态，还是另记一笔。迁移 `customs_compliance/0025` 之后，尝试行没有任何 UPDATE。
- 提交版本形成信封 `customs-compliance.declaration-submission.formed` 与版本同笔入 outbox，dispatch 上只有 VE 投影一条腿。
- ADR-0242 决定一至三：技术回执按它回应的那次尝试归属、记在提交一侧，由发起那次尝试的通道接收；后到的回执追加为新来源事实，派生新的当前判断，不改写原结果。落成哪种存储，与上一问同一问。
- CC CONTEXT「提交授权与就绪判断分别形成和失效」：登记之后、越过发送边界之前就绪或授权失效怎样处理，归申报发送通道。
- PILOT-SCOPE：首发要求授权确认后提交，并保留提交尝试与逐步外部回执；排除的是自动提交，不是提交。
- 本仓开发的是产品（AGENTS「红线」）：某个租户接哪个报关服务商、端点在哪、凭什么凭据，是租户取值；产品要带一种能对公开形状或模拟对端跑通的连接器形态。

## Decision

**一、发送意图就是提交版本形成信封。** 不另铸一封信。dispatch 在这封信上加第二条腿：VE 投影之后，申报发送消费者 `customs-compliance/send-declaration`。版本与信封同笔落库，信封进了 outbox 就有了持久化的发送意图；inbox 记账保证一份尝试只被这只消费者处理一次。消费者只做首次尝试：尝试行不是`已登记待发送`（受控重发形成时已带结果），或这一份尝试已有发送事实，都答已发过、不再发。

**二、越过发送边界另记一笔发送事实，尝试行不改。** 新表 `submission_send_fact`，主键是（租户、版本、尝试序号），一份尝试至多一笔。它记发出时刻、同步答复折成的结果、通道给的消息号与答复摘要。结果只有三种：
- 通道答收下 → 已回执；
- 通道答拒收 → 失败；
- 发出去了、没拿到可读的答复（超时、断在发出之后、5xx、读不懂）→ 待确认，不当失败。

只有确知没发出去（连通道都没拨通）才是未决，消费者交回未决哨兵，dispatch 续办重投。发出去之后不自动重发，再发走受控重发、要有安全再次发送判断。

发送之前再读一次单元此刻的就绪与授权：任一不在了或已失效，就不发，答 `DECLARATION_SEND_HELD`，尝试保持`已登记待发送`。不比成版时的那一份依据：重新取得的判断照样支持发送，与提交编排「双有效才成版」同一口径。

**三、连接器是对登记端点的 JSON POST；通道册是租户取值。** 新表 `declaration_channel`，按（租户、发送目标）登端点与可缺的凭据引用。凭据只登运行环境里的变量名，密钥不进库；端点只收 https，回环地址上的 http 也收，给本机的合成对端用。
- 登记走 `parcel-customs-register declaration-channel`。没登通道的目标答 `DECLARATION_CHANNEL_UNCONFIGURED`，什么都不发。凭据变量在运行环境里没有值，同答这一格。
- 线格式见 `HTTPSender` 头注：请求带 `Idempotency-Key: <租户>/<版本>/<尝试序号>`；同步答复 `{"outcome":"ACCEPTED"|"REJECTED","messageId":…}`。
- 某个服务商或单一窗口的私有格式，是另一种连接器，不在这里猜。
- 演示租户的通道登到本机的合成报关服务商（`scripts/demo-seeds/declaration-broker`），证据只记 `S`。

**四、异步技术回执走集成客户端渠道，凭消息号关联回尝试。** `POST /customs/technical-receipts`，认证复用「外部结果」那一种集成客户端授予：送回执的就是送外部结果的那一方。载荷是来源身份、原发送的消息号、`ACKNOWLEDGED` 或 `FAILED`、通道写下回执的时刻。回执逐份记进新表 `technical_receipt`，主键是（租户、来源身份），接收纪律照 UC-CC-006「来源接受与关联规则」：
- 同身份同内容答已有，同身份异内容是来源响应冲突，不覆盖；
- 消息号指不回任何一笔发送事实，答 `UNATTRIBUTABLE`，什么都不记。

答复带回执关联到的尝试，以及此刻的技术判断。当前技术判断由 `CurrentTechnicalJudgment` 折出：
1. 先取尝试行上的那一格；
2. 有发送事实取它；
3. 再按到达先后，逐份取明确回执。

回执不进外部结果册。

## 候选与反方

- **另铸一封发送意图信，与版本同笔入队。** 形成信封已经与版本同笔，内容也够；再铸一封，两封说的是同一件事，还得多一条「两封都在」的不变量。否决。
- **连接器扫待发送的尝试行。** 要另立一个扫描驱动与租约，而 inbox 已经给了「一份只处理一次」。否决。
- **把发送结果 UPDATE 回尝试行。** ADR-0242 决定三要后到的回执不改写原结果；尝试行一旦可改，发送与回执两路就要抢同一行。否决，另记事实、折出当前判断。
- **超时记失败、自动重发。** CC CONTEXT「提交尝试」明写结果未知时保持待确认，不得把超时直接解释为失败；自动重发可能双发。否决。
- **凭据密钥进库。** 敏感实例外置（AGENTS「红线」）。否决，只登变量名。

## Consequences

- CC 领域：`DeclarationChannel`、`SubmissionSendFact`、`TechnicalReceipt`、`CurrentTechnicalJudgment`；迁移 `customs_compliance/0030` 三张表。
- CC 应用：`SendDeclarationHandler`（`DeclarationSender` 口、`ErrDeclarationNotSent`、`ErrDeclarationCredentialMissing`）、`ReceiveTechnicalReceiptHandler`、`RegisterDeclarationChannelHandler`。
- CC 适配器：`adapters/declarationchannel` 的 `HTTPSender`；inbox 的 `DeclarationSendConsumer`；postgres 的 `SubmissionSends`；HTTP 的回执接收口；`registrationjson` 的通道登记载荷。
- 装配：parcel-dispatch 在形成信封上加申报发送一条腿；parcel-api 挂 `/customs/technical-receipts`；parcel-customs-register 多一个命令。
- 演示：`data/customs/33` 登演示目标的通道；`declaration-broker` 与 `send-one-declaration.sh` 走「授权确认 → 提交尝试 → 外部回执」。
- 票 10 第 2–7 项与离线服务商的回执不在本记录里。

## 越权风险点

1. **发送在 inbox 事务里做。** 发出去之后、事务提交之前进程死了或提交失败，发送事实回滚，dispatch 重投会再发一次。缓解靠幂等键：同一份尝试的键不变，通道据它认出重到。对端不认幂等键时，这一格可能双发；要不要先落「发送中」再发，等真实通道的语义定。
2. **通道登在提交之后不补发。** 没登通道时信封入账、答未配置；之后再登通道，已入账的那几份不会自己再发，没有补发的门铃。就绪或授权失效后再恢复，同理。今天只能人工重放或等受控重发口。
3. **同步答复里的 REJECTED 与 FAILED 回执记成失败。** ADR-0242 越权风险点 2 问过：明确的技术失败之后，这一份尝试是不是还要先过待确认。本记录取「明确的失败就是失败」，安全再次发送判断仍由 UC-CC-006 那一路做。
4. **关联不上的回执不留。** `UNATTRIBUTABLE` 什么都不记，送方要自己重送；UC-CC-006 的「待关联」要不要落一本暂存册，留给真实通道。
5. **同步答复不进来源保全册。** UC-CC-005「提交尝试与发送阶段」要求同步响应先作为外部来源保全；今天只把答复摘要记在发送事实上，监管层语义不从同步答复里解释。
6. **一份响应同时带技术与监管语义**（ADR-0242 决定二）**今天不拆。** 产品随附的线格式只有技术一半，监管一半仍走 `/customs/external-results`。

## Links

- [ADR-0227](./0227-customs-case-and-declaration-submission-enter-production-through-operator-decision-faces.md)：决定四（首次尝试`已登记待发送`）、越权风险点 7
- [ADR-0242](./0242-a-technical-receipt-belongs-to-its-submission-attempt-not-to-the-external-result-register.md)：决定一至三、越权风险点 2
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：连接器形态是产品策略，端点与凭据是租户取值
- [UC-CC-005](../application/customs-compliance/UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md)：`AT-CC-453`、`AT-CC-454`
- [UC-CC-006](../application/customs-compliance/UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md)：`AT-CC-455`
- 票 [product-strategy-boundary/10](../../.scratch/product-strategy-boundary/issues/10-cc-declaration-channel-and-public-regulatory-reference-configuration.md)
