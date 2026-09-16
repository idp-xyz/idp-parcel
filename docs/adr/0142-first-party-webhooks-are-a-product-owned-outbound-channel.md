# ADR-0142：首方 webhook 是产品自有的出向渠道，属机制半边——订阅绑定在客户渠道册的行上、载荷是契约拥有的投影而不是内部信封、投递由 Outbox → 派发 → 出向渠道消费者承担、至少一次、按事件 ID 幂等、按客户密钥签名、失败预算耗尽即暂停订阅并出声；它是 `NotificationChannelGateway` 在机器接收方上的第一份生产实现，结果层只诚实记到「渠道接受」；`PAR-INT-06` 收窄为「某客户实际启用哪条通知渠道」与既有消息渠道的适配

Status: Proposed（**草案**，2026-09-16。用户经 IDP 队列通道 1 令「按建议继续」，本记录是通道 1 复核所列六件缺项的第三件——「客户 API 今天只有请求-响应；轨迹更新、异常通知、受理结果都该能推；本仓已有 Outbox 与信封机制，缺的只是一份 ADR」。**接受与否归用户**，接受前不是依据。裁决能力边界：读过 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md)、[ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md)、[ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md) 全文，[ADR-0043](./0043-publish-intent-claimed-by-result-identity.md) 与 [ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md) 的 Decision，`internal/platform/dispatch/direct_publisher.go` 全文，`internal/visibilityexception/ports/ports.go` 里 `CustomerViewHandoff`、`NotificationChannelGateway`、`NotificationHandoff` 三个口与其注释，[UC-VE-006](../application/visibility-exception/UC-VE-006-FORM-CUSTOMER-DISCLOSURE-AND-NOTIFICATION.md) 的结果层与 `AT-VE-103..105`，[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)的 `PAR-INT-06` 行，[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)机制半边现状里 PN-06 那句「通知网关等真实渠道凭证」；未重读 VE CONTEXT 全文、未重读各上下文 `Outbox*Handoff` 的信封内容、未核对 `eventing.Envelope` 的全部字段——本记录因此只裁**出向渠道的归属、订阅模型、投递语义与结果层的诚实映射**，不裁 v1 事件目录的内容、不裁任何事件载荷的字段、不裁重试节奏的取值）
Date: 2026-09-16

## Context

**首方 API 只有一半：客户系统能问，产品不能说。** ADR-0139 立的首方客户渠道覆盖命令面与查阅面，全部是请求-响应。一个物流产品对客户系统最常被问到的能力恰恰是反向的：委托被接受或拒绝了、包裹到了某个里程碑、形成了客户可见的异常、终局成立了——这些今天只能靠客户系统反复轮询 `/customer-tracking-view`。轮询不是错，它是补偿查询的正当形态；缺的是推送这一半。

**推送在本仓早就有了机制，只是没有对外的那一段。** 各上下文的 `Outbox*Handoff` 适配器把「发布意图与业务结果同一提交」证在真实 PostgreSQL 上（[ADR-0043](./0043-publish-intent-claimed-by-result-identity.md)：信封 ID 由结果标识认领，重放重发同一份）；派发器按分区串行、失败到点重试、预算耗尽进 `ABANDONED`；[ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md) 让 `DirectPublisher` 按 `Envelope.Type` 把信封投给本进程的消费者，「`Publish` 返回 nil 只在消费者的 inbox 事务已提交后才成立」。VE 侧已经把客户视图版本（`CustomerViewHandoff`）与通知决定（`NotificationHandoff`）写进 Outbox；`NotificationChannelGateway` 的注释写着「真实渠道与其凭证属实例参数，今天没有实现，唯一实现是测试替身」——[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)把这一格记为 PN-06 唯一的余量。也就是说：事件在库里、派发器在跑、消费门四条已被复用多次，缺的只是**一个把信封变成对客户系统的 HTTP 投递的消费者**，和它该归谁。

**这一格今天挂在 `PAR-INT-06` 上，与 ADR-0139 拆 `PAR-INT-01` 之前的形状一模一样。** `PAR-INT-06`「客户异常通知渠道」的最低证据列是「合同通知义务、消息渠道能力、提交/接受/送达/失败/客户确认结果层、失败恢复和更正能力」——这是**客户既有消息渠道**（邮件、短信、IM）的属性：它们的送达回执长什么样、失败怎么恢复，各家各异，替它拟表就是替租户拟样子。但产品自己带一条机器渠道——产品定义的回调协议、产品签发的签名密钥、产品写的事件契约——这一形**只有一形，且是产品自己选的**。ADR-0139 对 `PAR-INT-01` 用过的那句判据逐字适用：一族渠道的证据由谁出，看它的三件（协议、密钥形态、订阅绑定模型）取决于谁；三件都取决于产品，就是机制半边。`PAR-INT-06` 的「已确认约束」（默认人工确认，仅批准范围可自动发布）管的是**什么内容可以发**——那是披露决定的事，`UC-VE-006` 拥有，本记录一字不碰。

**载荷不能是内部信封。** `eventing.Envelope` 里装的是各上下文自己的领域语言与内部标识，直接推给客户系统等于把内部形状变成对外承诺，此后任何一个上下文改自己的事件就是破坏性变更。首方 API 的响应形状由各 `adapters/http` 拥有、由 ADR-0140 的描述符进契约；出向事件的载荷同理必须是**契约拥有的投影**，从内部信封翻译而来，翻译住在消费侧适配器里（ADR-0025「跨上下文调用的适配器落在消费侧，翻译职责由它独占」——出向渠道对各上下文而言就是一个消费方）。

**归属：它不是业务上下文，也不进 `platform`。** ADR-0072 为入向那一半立过同一判据：接入渠道登记、凭据验证、信封铸造归一个「共享接入身份技术能力」`internal/accessidentity`，不进 CONTEXT-MAP，不拥有业务语言；不归 `platform`，因为它携带租户隔离语义。出向渠道是同一判据的镜像：订阅绑在（租户，客户账户）上、密钥引用是敏感实例、投递记录要按租户隔离——带隔离语义，不进 `platform`；它把事件送到客户系统而不判断任何业务，不是限界上下文。

**结果层要诚实。** `UC-VE-006` 把通知结果分成提交、渠道接受、送达、失败、客户确认几层各自成立，`AT-VE-103`「只记录接受，不推定送达」、`AT-VE-104`「渠道返回送达，不推定客户确认」。webhook 协议能观察到的只有：投递发出（提交）、接收方答 2xx（渠道接受）、非 2xx 或超时（失败）。**它观察不到「送达」与「客户确认」**——2xx 说的是「你的服务器收下了」，不是「你的业务处理了」，更不是「你的人看到了」。把 2xx 记成送达是 `AT-VE-103` 明禁的推定。本记录因此只把 webhook 映到提交、渠道接受、失败，送达与客户确认留空并写明为什么。

## Decision

**一、首方 webhook 是产品自有的出向渠道族，属机制半边；`PAR-INT-06` 的适用面收窄为「某客户实际启用哪条通知渠道」与「客户既有消息渠道（邮件、短信、IM、非本产品协议的回调）的适配所需证据」。** 协议、签名密钥形态、订阅绑定模型三件由产品定义，现在就立。`PAR-INT-06` 的登记状态不改，其「已确认约束」（默认人工确认、仅批准范围可自动发布）继续管披露内容，不归本记录；接受本记录时在该行备注加一条指向本记录的引用（同 ADR-0139 对 `PAR-INT-01` 的处置）。

**二、出向渠道能力落点是 `internal/outboundchannel/`，与 `internal/accessidentity/` 对称：技术能力，不是限界上下文。** 它不进 CONTEXT-MAP、不拥有业务领域语言、没有 `domain` 子包；它拥有**订阅登记册**（谁订了什么、投到哪、用哪把密钥的引用）、**投递账**（每个事件对每个订阅的每次尝试与结果）和**投递执行**（签名、发送、记账、按预算重试）。不归 `platform`：订阅与投递账带（租户，客户账户）隔离语义，ADR-0072 Alternatives 对 `platform` 的界定「技术件不判业务」在此同样守不住。各业务上下文对它一无所知——它们只往 Outbox 写自己的信封，与今天一样。

**三、订阅绑定在客户渠道册的行上，是首方族的一部分。** 一条订阅 =（客户渠道册的一行引用，回调 URL，签名密钥的受控引用，事件类型过滤，生效区间，状态）。订阅**不能**独立于渠道行存在：它继承那一行的（租户，客户账户），一个客户系统只能订到它自己账户下的事件——ADR-0003 的隔离边界在出向这一侧的落法。密钥本体不落库、不进仓库，库里只登受控引用（AGENTS「敏感实例外置」，与 ADR-0139 Decision 二同款）；密钥由产品签发、可轮换，轮换期间新旧两把并存一个窗口，签名头同时携带两把的签名，接收方任一验过即认。订阅的登记走两口：客户系统经首方 API 自助（`/v1/webhook-subscriptions`，挂首方渠道 Intake，走 ADR-0139 的三格）与租户运营经管理台写面（ADR-0085 那一族）；两口消费同一登记用例。

**四、载荷是契约拥有的事件投影，从内部信封翻译，翻译住在 `outboundchannel` 的消费侧适配器里。** 每一种对外事件类型有：稳定的类型名、`schemaVersion`、契约拥有的载荷形状（进 ADR-0140 生成的首方 API 契约的 `webhooks` 节）。翻译器按 `Envelope.Type` 从内部信封产出对外事件，或判定「此类内部事件不对外」。**v1 对外事件目录本记录不定**——哪些内部信封对外、各对外事件的载荷字段，归各上下文 owner 按其用例与 CONTEXT 声明，写成 ADR-0140 的描述符；本记录只定「对外事件一定是翻译出来的投影，不是内部信封本体」。载荷里没有租户维——租户由订阅所属的行决定，客户系统不需要也不该看到它（同 `CustomerViewHandoffIntent` 注释「视图对象没有租户维」的处置）。载荷里带客户账户引用，因为一个客户系统将来可能代多个账户（ADR-0139 风险点 5），事件要说清是谁的。

**五、投递是 Outbox → 派发 → 出向渠道消费者，至少一次，按事件 ID 幂等。** `outboundchannel` 向派发器的路由表注册为一个 `Consumer`（ADR-0049 Decision 三「路由表是显式清单」）：收到内部信封 → 翻译（或判不对外、直接消费成功）→ 找出该事件所属（租户，客户账户）下活跃的订阅 → 为每条订阅落一笔投递账（事件 ID × 订阅 ID 唯一，撞即幂等跳过）→ 消费门事务提交。**发送不在消费门事务里做**：HTTP 调用的时长与结果不确定性进不了一个要提交的事务（ADR-0049 对「消费者慢会吃掉失败预算」的警告在这里更重——接收方是客户的服务器）。投递账落下之后由 `outboundchannel` 自己的投递循环按账逐条发送、记结果、按退避重试。对外事件 ID 从内部信封 ID 派生（ADR-0043：信封 ID 由结果标识认领，重放重发同一份），所以同一业务结果无论内部重投多少次，客户系统看到的是同一个事件 ID——接收方按它幂等，协议文档写明「不假定只到达一次，不假定有序」。

**六、签名与时间窗。** 每次投递带三个头：事件 ID、发送时间戳、签名。签名是 HMAC-SHA256，原文为「时间戳 + `.` + 原始请求体」，密钥取订阅上受控引用所指的那一把；头里可并列多把密钥的签名（轮换窗口）。接收方按契约文档核签名并拒绝时间戳超出窗口的投递（窗口长度进契约文档，是协议参数不是租户参数）。头名与签名原文的构造照 Standard Webhooks 惯例（`webhook-id` / `webhook-timestamp` / `webhook-signature`），不自造——集成方的通用接收库认得它。不做 mTLS 到接收方、不做接收方 IP 白名单发布，两者列为风险点。

**七、失败预算与暂停要出声。** 一条订阅连续投递失败达到预算（次数与退避序列是 `outboundchannel` 的节奏配置，形状同 `dispatch.Config`：装配方显式给出，不设默认）后进入**已暂停**：不再尝试新事件，既有未送出的事件留在投递账里可查；暂停是订阅状态的一个取值，可由租户运营或客户系统恢复，恢复后按投递账补发暂停期间的事件——**不静默丢**，理由与 ADR-0049「无订阅者即阻塞分区而不是静默丢弃」同一条。一条订阅的失败**不阻塞**其他订阅、不阻塞派发器的分区：消费门在落账那一刻就提交了，发送是它自己的循环。暂停必须出声：管理台的客户渠道页与客户系统自助的订阅查阅面都看得到状态与最近失败。

**八、它是 `NotificationChannelGateway` 在机器接收方上的第一份生产实现，结果层只诚实记到「渠道接受」。** VE 的通知编排把 `CustomerNotification` 交给 `SubmitToChannel`；当该客户的通知渠道是首方 webhook 时，实现把通知翻成对外事件、落投递账并返回——这一步对应 `UC-VE-006` 的**提交**层。接收方 2xx 对应**渠道接受**（`AT-VE-103`「只记录接受，不推定送达」）；非 2xx / 超时 / 预算耗尽对应**失败**。**送达与客户确认两层 webhook 观察不到，本记录不为它们造任何推定**：合同若要求送达或确认，通知义务在 webhook 这条渠道上停在「已接受」，`AT-VE-105`「合同要求客户确认但只有送达，通知义务保持待确认」的纪律在此照旧——客户确认若要成立，要靠客户系统调一个确认端点（另一个用例，不在本记录）。接受结果如何回写到 VE 的通知记录（异步、经 VE 自己的入口）归 VE owner，见风险点。

**九、不做的，逐条写明。**

- 不把内部 `eventing.Envelope` 直接推给客户系统。
- 不在业务事务里同步调用客户系统的 URL——ADR-0043 / ADR-0049 否决「取消 Outbox 在事务里直接调消费者」的理由在此更强。
- 不为出向引入消息中间件——ADR-0049 的判据「无负载证据不分布」原样成立；投递循环是 `outboundchannel` 自己的进程内循环，与派发器同形。
- 不替租户预建任何既有消息渠道（邮件、短信、IM）的适配——照 `PAR-INT-06` 原句等证据，到时为那一族立它自己的渠道实现，与首方 webhook 在 `NotificationChannelGateway` 后面并列。
- 不定 v1 事件目录、不定任何载荷字段、不定重试次数与退避序列的取值、不定时间窗长度的值——各归风险点或配置。
- 不做「客户确认」的回写端点——它是 `UC-VE-006` 客户确认那一层的另一个用例。

## Consequences

- 新增 `internal/outboundchannel/`（技术能力，无 `domain` 子包）：订阅登记册与投递账的 PostgreSQL 实现与迁移模块 `migrations/outbound_channel/`、翻译器注册表、签名器、投递循环；`cmd/parcel-dispatch` 的路由表增一个消费者（或另起 `cmd/parcel-outbound` 进程跑投递循环——进程划分归实施票）。
- `NotificationChannelGateway` 得到第一份生产实现；`ports.go` 里「今天没有实现，唯一实现是测试替身」一句随实施改写为「首方 webhook 渠道已按 ADR-0142 立；客户既有消息渠道的适配仍等 `PAR-INT-06`」。开发主线 PN-06 那句余量随下一轮盘点更新，不在本记录代改。
- 首方 API 契约（ADR-0140）增 `webhooks` 节与 `/v1/webhook-subscriptions` 一族端点；对外文档增「Webhook：订阅、验签、去重、乱序、重试与补偿查询」一章（方案稿对外文档必备项里的「Webhook」一项由此有权威落点）；补偿查询就是既有的 `/customer-tracking-view` 等查阅面，不新造。
- 管理台增客户渠道页里的订阅子面（状态、最近失败、恢复），走 ADR-0085 写面纪律。
- 沙箱（ADR-0141）里集成方能收到事件的前提是沙箱里有事实发生——ADR-0141 风险点 4 由此变成实施前必答。
- 参数登记册：`PAR-INT-06` 行加引用；增行「客户 webhook 订阅」（实例半边：某租户的客户账户 × 回调地址 × 事件过滤 × 区间）。
- 各上下文 owner 得到一件新工作：声明本上下文哪些内部信封对外、对外载荷长什么样（ADR-0140 描述符）。没有声明的内部事件默认**不对外**——缺省朝拦。

## Alternatives considered

- **只提供轮询，不做推送。** 否决：轮询是补偿查询的正当形态，保留；但集成方对物流 API 的默认预期是事件推送，没有它每个客户系统都要自己写轮询循环并承担频率与配额的摩擦——那正是限流那张票要治的病的来源之一。
- **直接推内部信封。** 否决：内部形状变成对外承诺，任何上下文改自己的事件都成了破坏性变更；ADR-0025 把翻译放在消费侧，出向渠道就是那个消费侧。
- **把出向渠道放进 `visibility-exception`。** 否决：VE 是业务上下文，拥有披露与通知的**决定**；投递是技术能力，且要投的不只是 VE 的事件（PS 的接受决定、终局，TF 的里程碑都可能对外）。与 ADR-0072 不让业务上下文各自做凭据验证同一条判据。
- **放进 `internal/platform/`。** 否决：订阅与投递账带（租户，客户账户）隔离语义，ADR-0072 Alternatives 对 `platform` 的界定不容它。
- **在消费门事务里同步发送 HTTP。** 否决：客户服务器的响应时长进了本产品的事务与派发预算；ADR-0049 对慢消费者的警告在此成倍。
- **每客户一个消息队列 / broker 主题。** 否决：ADR-0049「无负载证据不分布」；且把部署要求压给客户系统。
- **把 2xx 记成「送达」。** 否决：`AT-VE-103` 明禁；2xx 只证明接收方收下了请求。
- **签名自造格式。** 否决：Standard Webhooks 惯例免费且集成方的接收库认得它；自造格式让每个集成方多写一次验签。
- **失败即丢、不暂停不补发。** 否决：与 ADR-0049「不静默丢弃」、消费门「拒收也是账」同一条纪律。

## 越权风险点（归 owner 复核）

本记录能力边界之外、实施时必须先答的几件：

1. **v1 对外事件目录**——哪些内部信封对外（委托已接受 / 已拒绝 / 已撤回、包裹终局、客户视图版本、客户可见异常通知……），各上下文 owner 按 CONTEXT 与用例逐个声明并写描述符；未声明即不对外。
2. **接受结果回写 VE**——接收方 2xx 如何成为 VE 通知记录上的「渠道接受」节点：`outboundchannel` 发一份内部信封由 VE 消费，还是 VE 轮询投递账；归 VE owner，纪律是「不推定送达」。
3. **投递循环的进程归属**——挂在 `cmd/parcel-dispatch` 里还是另起进程；影响失败隔离与部署形态，归部署 owner。
4. **重试节奏与失败预算取值、时间窗长度**——形状同 `dispatch.Config` 不设默认；取值属运营参数，参数登记册增行。
5. **接收方安全的两件可选项**——mTLS 到接收方、发布本产品的出向 IP 段；集成方常问，本记录不预裁。
6. **1:N 客户账户下的订阅**——一个 client 代多个账户时订阅按账户还是按 client；随 ADR-0139 风险点 5 一并裁。
7. **沙箱里的事实来源**——ADR-0141 风险点 4；没有它集成方在沙箱里收不到任何事件。
8. **客户确认的回写用例**——`UC-VE-006` 的客户确认层在 webhook 渠道上怎么成立（客户系统调确认端点），另立用例与票。

## Links

- [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md)：首方族、客户渠道册、三格答复、风险点 5——订阅绑定在其行上，自助订阅端点挂其 Intake
- [ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md)：契约生成物、`webhooks` 节、描述符——对外事件载荷进契约的通道
- [ADR-0141](./0141-integrator-sandbox-is-a-deployment-form-of-the-first-party-api-on-synthetic-data.md)：集成方在沙箱里测出向事件；其风险点 4
- [ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md)：显式路由表、`Publish` 的最严含义、不静默丢弃、慢消费者警告——本记录 Decision 五、七的依据与镜像
- [ADR-0043](./0043-publish-intent-claimed-by-result-identity.md)：信封 ID 由结果标识认领——对外事件 ID 从它派生、重投同 ID 的依据
- [ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)：技术能力不进 CONTEXT-MAP、不归 `platform` 的判据——本记录 Decision 二的镜像依据
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：翻译住在消费侧——载荷翻译器落在 `outboundchannel` 的依据
- [ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)：管理台写面纪律——订阅的运营口
- [ADR-0003](./0003-group-tenant-legal-entity-customer-account.md)：订阅继承渠道行的（租户，客户账户）所依的边界
- [UC-VE-006](../application/visibility-exception/UC-VE-006-FORM-CUSTOMER-DISCLOSURE-AND-NOTIFICATION.md)：提交 / 渠道接受 / 送达 / 失败 / 客户确认各自成立的结果层与 `AT-VE-103..105`——Decision 八诚实映射的出处
- [参数登记册](../product/PILOT-PARAMETER-REGISTER.md)：`PAR-INT-06` 原文；本记录不改其状态，只收窄适用面并预告新增一行
- [开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)：PN-06「通知网关等真实渠道凭证」那句余量——本记录把它拆成首方 webhook（机制）与既有渠道适配（实例）两半
- `internal/visibilityexception/ports/ports.go`：`CustomerViewHandoff`、`NotificationChannelGateway`、`NotificationHandoff` 的现状与注释
- `internal/platform/dispatch/direct_publisher.go`：`Consumer` 接口与三种不成功各自成格——`outboundchannel` 消费者要接的形状
- [Standard Webhooks](https://www.standardwebhooks.com/)：签名头与原文构造的惯例来源；[RFC 2104](https://www.rfc-editor.org/rfc/rfc2104.html)：HMAC
- 来源：IDP 队列通道 1 的复核与用户「按建议继续」指令（2026-09-16）
