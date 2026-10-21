# ADR-0239：不是本服务方的承运答复答订舱不在册——对外与订舱不在册同一格同一副响应，挡回依据与答复同笔记进承运答复挡回册，两条挡回路做一样多的事

Status: Accepted（2026-10-21，Claude Code 会话代决：用户 2026-10-20 原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」，另有常设指令「你作为业务和系统专家自决……解决所有的问题」；代决复核报告 `.scratch/owner-review-20261020/report.md` 的 R9 裁「否」，票 demo-chain-gaps/145。回答 [ADR-0202](./0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md) 越权风险点 14；部分停用 ADR-0202 文末补记「承运答复的次序与格」一条里 `SOURCE_NOT_SERVICE_PROVIDER` 那一格（被停原句与适用场景见 ADR-0202 的 Status 行）。ADR-0202 正文不改，只在其 Status 行与 Links 加前向指针。越权风险点见文末，待 TF owner 复核，第 1 点知会接入身份 owner。）<!-- 索引注记 -->｜**已接受**（2026-10-21，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 TF owner 裁定；票 demo-chain-gaps/145）。**对外一格**：承运答复口对答复方不是订舱所属委托服务提供方的答复答 `BOOKING_NOT_FOUND`（200），与订舱在本租户下不在册同一格、响应体逐字相同；`SOURCE_NOT_SERVICE_PROVIDER` 退出线格式，应用层结果里也不留这一格；续办同是找租户核对；来源核对仍在重放之前。**对内分清**：TF 立承运答复挡回册（迁移 0050），（租户，答复方来源身份，订舱，答复标识，依据）一行，依据封闭两值（订舱不在册、不是服务提供方）存成定宽码，记首末挡回时刻与次数，同键再来只加次数；与那一次答复同笔，写不动答未决、整笔回滚，不答订舱不在册；台账不形成答复、不妨碍订舱录入之后的正当答复、不存报文内容；不叫「拒收」「不受理」，两词在 TF 已有别义，TF CONTEXT 立「承运答复挡回」词条。**一样多的事**：订舱连同委托的服务提供方一条语句读回，两条挡回路都是一读一写、写的键等长；订舱标识与答复标识设 512 字节界，超界在查库之前答`未受理`，两条路一样，迁移 0050 遇已有超界行拒绝施加；TF 命令载荷限读 1 MiB、超限答坏报文；读不动同答 `BOOKING_STORE_UNAVAILABLE`、册写不动同答 `ANSWER_STORE_UNAVAILABLE`，答复口不再单读委托，剩下的差别只有时差。部分停用 ADR-0202 补记那一格。越权风险点含挡回册没有查阅面（票 167）、剩余时差、数据违约 500 仍暴露存在、册无保留期与聚合（票 170）、未决借答复册的名、不存报文内容、字节界是产品常量、限读作用于 TF 全部命令口、键长余量靠租户与来源身份短，待 TF owner 复核

Date: 2026-10-21

## Context

- ADR-0202 文末补记「承运答复的次序与格」定了答复口的次序：来源取自集成客户端绑定 → 订舱在册，否则 `BOOKING_NOT_FOUND` → 答复方的来源身份等于订舱所属委托的服务提供方，否则 `SOURCE_NOT_SERVICE_PROVIDER` → 重放与冲突 → …。越权风险点 14 记下代价：分两格，同一租户下一个承运商据此判断得出某个订舱标识在本租户下存在。
- 代决复核 R9 裁「否」：一律答 `BOOKING_NOT_FOUND`。同一租户的承运商之间常是竞争方，两种情形下承运方的续办都是找租户核对，分格只多泄露一位信息；须在首版集成契约（R8：承运答复口作为对承运商的首版集成契约）发布前落地。
- 只改格名不够（读码钉 `4d181b01`，只记 `S`）：
  - 编排先读订舱、不在即答；在册才再读一次委托、比服务方。在册那一条比不在册的多一次往返，响应时刻把合并了的两种情形分开。
  - 委托册读不动答 `COMMISSION_STORE_UNAVAILABLE`，只有订舱在册才走得到这一步，未决原因同样分得开。
  - 今天两格都不落任何东西，区分只在答复里；合并之后不另记，运营也分不清「这个承运商答了不归它答的订舱」与「运营侧还没把订舱录进来」。
- 隔离评审（2026-10-21）在真库与生产装配上复现了一条漏回的路：挡回册的依据存成长短不一的词，订舱号与答复标识没有长度上限，PostgreSQL 的 btree 索引行又有上限。答复方挑一个恰好卡在上限边上的答复标识，「订舱不在册」那一行写得进、「不是服务方」那一行写不进，一条路答订舱不在册，一条路答未决。写失败的那一笔整笔回滚、不留行。
- 留痕先例：本仓的留痕都在库里，与那一拍同笔（[ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 派送待形成册：一件一行、首末时刻）。parcel-api 不记请求日志。

## Decision

**一、对外一格：不是本服务方的答复答订舱不在册。**（产品策略）

- 答复方的来源身份不是该订舱所属委托的服务提供方时，答 `BOOKING_NOT_FOUND`（HTTP 200，ADR-0022），与订舱在本租户下不在册（含只登在他租户名下）同一格、同一副响应：响应体只有那一格，不回显在册答复，不带续办引用。
- `SOURCE_NOT_SERVICE_PROVIDER` 退出线格式；应用层的结果也去掉这一格，编排交回的结果里没有分得开两种情形的东西。区分只在挡回册（决定二），任何一个接这只编排的适配器都无从说出去。
- 续办合为一句：找租户核对订舱标识（与改载荷的`未受理`仍分开，ADR-0029）。
- 次序不变：来源核对仍在重放之前。别的承运商拿别人的答复标识原样重放，答订舱不在册，读不回别人的答复。来源与服务方照旧同一串字面比、不归一。
- 订舱在册而它的委托读不回，照旧是数据违约（500），不答订舱不在册，也不记挡回。

**二、对内分清：承运答复挡回册。**（机制）

- TF 立承运答复挡回册（迁移 transport_fulfillment `0050`）。一行是（租户，答复方来源身份，订舱，答复标识，依据），记首次与最近一次挡回时刻、次数。
  - 依据封闭两值：`BOOKING_NOT_REGISTERED`（载荷指名的订舱在本租户下不在册）、`SOURCE_NOT_SERVICE_PROVIDER`（订舱在册，答复方不是它所属委托的服务提供方）。库里存定宽码（1、2），理由见决定三。
  - 订舱与答复标识照载荷原样记；答复方取集成客户端绑定的来源身份。
  - 时刻是本上下文记下它的时刻，不是答复方给的答复时刻。
- 同键再来不另起一行，只加次数、推后最近一次时刻。键里没有答复的内容：同一答复标识换了载荷再交，也并进同一个计数。依据进键：订舱起先不在册、后来录在别的承运商名下，同一份答复前后两次被挡回的依据不同，两行都是实情。
- 名字叫「挡回」，不叫「拒收」，也不叫「不受理」。TF 里「拒收」已指交接的`已拒收`与收件人拒收，英文 Refusal 也已用于段进入拒收、交接拒收与承运答复的 `REFUSED`；「不受理」在 TF 又泛指构造门上的输入不受理（`未受理`）。表与代码用 bounce。TF CONTEXT 的 Language 一节立「承运答复挡回」词条（先例是 ADR-0230 立的「派送待形成」）。
- 写它的只有答复编排，与那一次答复同笔。册写不动答未决（`ANSWER_STORE_UNAVAILABLE`），装配整笔回滚，不答订舱不在册：留痕不全，运营就又分不清那一次是哪一种。
- 本册是台账，不是事实来源：
  - 挡回不形成答复，订舱的当前读法（`待确认`、`候补`）不读本册。
  - 不妨碍后来的正当答复：订舱录入之后，服务方重交同一份答复照常受理，那一行照留。
  - 不存被挡回报文的内容（五值、接受量、答复时刻）。
- 查阅面不在本记录，另票 [demo-chain-gaps/167](../../.scratch/demo-chain-gaps/issues/167-booking-answer-bounce-register-has-no-read-face.md)，与订舱、答复两册的查阅面（票 48）同一车道。

**三、两条挡回路做一样多的事。**（机制）

- **一读**：订舱连同所属委托的服务提供方一条语句读回（端口 `BookingWithProviderReader`；委托以左连接取，读不回时交回订舱、标出委托不在）。订舱不在册与答复方不是服务方两条路，都是这一读加挡回册一写，之后才分开。服务方自己的答复也走这一读：受理那一条从两次读变成一次。
- **一写，键等长**：挡回册的键上，唯一由路决定的那一格是依据，所以依据存定宽码。答复方挑什么长度的订舱标识与答复标识，两条路写的键都一样长。只靠字节界、依据照存词，两条路的键长差就只是离索引上限远而已——余量由租户与来源身份的长度决定，不由本口守。
- **字节界**：订舱标识与承运答复标识各以 512 字节为界（UTF-8 字节，`domain.ErrValueTooLongToStore`）。先例是 parcel-shipment 外部标识号码的字节界。只给进键的两个标识设界：答复依据不进任何键、不进挡回册，只在服务方那条路、来源核对之后才判，与合并掉的那一位无关，照旧只过既有构造门（非空、无 NUL）；给它设界只会让一格可能装自由文本的值约 171 个汉字就被拒。
  - 界在构造门上，构造门排在查库之前：超界答`未受理`（200），两条路一样，一次库都不碰。这与 TF 对全空白、带 NUL 的值是同一个先例——存不进去的输入在构造期答「改输入」。
  - 界让挡回册、订舱册与答复册的键都落在索引上限之下，两条路都写得进。写失败只剩依赖故障一种，超长标识也不再永远答「未决，稍后再交」。
  - 界只由领域构造门守，库上不另加 CHECK，与 parcel-shipment 那道界同一先例。界立起来之前已经落库、超过界的行，此后从门里读不回来，所以迁移 0050 遇到这样的行拒绝施加，不替任何人截短一个标识。
- **载荷限读**：TF 各命令口的载荷经同一个解码门（`decodeClosedPayload`）读，限读 1 MiB，超限答坏报文（400），先例是各上下文登记口的读取上限。它是字节界之前的第一道闸，不让一个调用方把整段请求读进内存。
- **未决原因**：读不动两条路都答 `BOOKING_STORE_UNAVAILABLE`，册写不动都答 `ANSWER_STORE_UNAVAILABLE`。答复口不再单读委托，`COMMISSION_STORE_UNAVAILABLE` 在这一口不再出现。
- 有了这几条，两条路的状态、响应体与未决原因都相同，剩下的差别只有时差一格。不加人为延时，也不做常数时间答复：那条语句命中不命中一行的差别，见越权风险点 2。

三类：决定一是**产品策略**（内置一种答法：对答复方不分这两格）；决定二、三是**机制**；哪个集成客户端绑定哪个来源身份、哪份委托由谁服务是**租户取值**（同 ADR-0202），演示租户以 `SYN-` 登记，证据只记 `S`。

## 候选与反方

- **只在 HTTP 层把 `SOURCE_NOT_SERVICE_PROVIDER` 译成 `BOOKING_NOT_FOUND`，应用层留两格。** 编排结果里留着那一位，下一个接这只编排的适配器（出站订舱连接器、人工转录口）照样会说出去；运营要的是看得到依据，不是适配器知道。否决。
- **只改格名，不动读法。** 在册那条多一次读委托，委托读不动还另答一个未决原因，两处都把合并了的情形重新分开。否决。
- **留痕写日志。** 本仓不记请求日志，日志也不是审计依据；运营要的是按订舱、按承运商查得出的记录。否决。
- **每次挡回一行（追加流水）。** 同一份答复的重交不是新的信息，按重试策略自动重交的承运系统会把册写满；次数与首末时刻已足够看出谁在反复试。否决。
- **挡回册写不动时照答订舱不在册（留痕尽力而为）。** 写口报 SQL 错误后 PostgreSQL 事务已中止，照答只换回提交错误；留痕不全也正是这条要防的。否决。
- **依据照存词，只设字节界。** 界离索引上限够远时，两条路照样都写得进；可两条路的键仍差几个字节，余量随租户与来源身份变短而变小，这一格守的东西就落到了别处。否决，依据存定宽码。
- **超界的标识答坏报文（400）。** TF 对存不进去的值（全空白、带 NUL）一律在构造门上答`未受理`，续办同是改输入；坏报文留给解不出、不成形的载荷。否决，超界答`未受理`；只有整份载荷超过读取上限答坏报文。
- **加随机延时或答复凑成常数时间。** 两条路已同读同写，剩下的差异远小于网络抖动；人为延时要每一份正当答复都付。否决。
- **按来源分别作答：服务方看得到依据，别人看不到。** 服务方自己答不在册订舱时依据就是「订舱不在册」，无需分；非服务方本就不该知道。否决。

## Consequences

- TF：
  - 端口 `ports/booking_answer_bounce.go`：`BookingWithProviderReader`、`BookingAnswerBounceLedger`、挡回依据两值。
  - 适配器 `BookingRequests.FindBookingWithProvider`（与 `FindByKey` 共用一份行重建）、`BookingAnswerBounces`。
  - 迁移 transport_fulfillment `0050`：承运答复挡回册，依据存 smallint；已有超界的订舱标识或答复标识时拒绝施加。
  - TF CONTEXT Language 一节「承运答复挡回」词条。
  - 领域 `domain/stored_reference.go`：`NewBookingReference`、`NewCarrierAcceptanceReference` 过 512 字节界，超界 `ErrValueTooLongToStore`；`NewAcceptanceBasisReference` 不变。
  - HTTP 解码门 `decodeClosedPayload` 限读 1 MiB。
  - 编排 `AnswerBooking` 改用连读口与挡回册；`CommissionTransportDeps` 多 `BookingsWithProvider`、`AnswerBounces` 两格；结果去掉 `BookingSourceNotProvider`。
- 装配：`buildBookingFaces` 接上连读口与挡回册，线格式不变形、只少一个取值。
- 文档：[UC-TF-004](../application/transport-fulfillment/UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md) 改写 `AT-TF-149`、`AT-TF-153` 的结果，增 `AT-TF-326`–`328`；演示种子 README 与演示动线脚本补记那一答。
- 每次挡回多一条写；受理那一条少一次读。

## 对已接受记录的作用

**部分停用** ADR-0202 文末补记「承运答复的次序与格」一条里「答复方的来源身份等于订舱所属委托的服务提供方，否则 `SOURCE_NOT_SERVICE_PROVIDER`」的「否则 `SOURCE_NOT_SERVICE_PROVIDER`」那一格：改答 `BOOKING_NOT_FOUND`（决定一）。**适用场景**：只限承运答复口 `/transport-fulfillment/booking-answers`。来源取自绑定、来源核对排在重放之前、字面比、不回显在册答复、委托读不回为数据违约，与同条其余各句不变；ADR-0202 决定二「不等答不受理」不变，变的只是那一格的名字。越权风险点 14 由本记录回答。ADR-0202 正文不改，Status 行与 Links 加前向指针。

## 越权风险点

1. **挡回册今天没有查阅面**（TF owner；知会接入身份 owner）。承运方拿到订舱不在册来问租户时，运营要回答「这个订舱不归你」还是「我们还没录」，今天只能经库查挡回册；承运方的续办实际落在运营能不能查到。去处是票 167，与票 48 同一车道。首版集成说明（票 144）须写这一格的续办是「找租户核对」，不写「不归你答」。
2. **剩余时差**（TF owner）。两条路同一条语句一读一写、写的键等长，差别只剩那条语句命中一行与否、以及命中时多重建一条订舱（微秒级），远小于网络抖动；按统计攒足够多的样本仍可能分得开。界内的每一次试探，两条路都在挡回册上留一行或加一次，反复试的承运商看得出来；超界的答`未受理`、不留行，也分不开两条路。owner 若要求常数时间，去处是在装配点给两条挡回路补齐到同一时长，代价落在每一次挡回上。
3. **数据违约的 500 仍暴露订舱存在**（TF owner）。订舱在册而委托行不在时答没形成答案、不记挡回，任何答复方都看得出那个订舱在。只在库被绕过编排改坏时出现；改答订舱不在册会让服务方去核一个确实存在的订舱（ADR-0202 补记的同一条理由）。
4. **挡回册没有保留期与聚合**（TF owner）。一个（答复方、订舱、答复标识、依据）一行，探测者每试一个新订舱标识、或重试时换一个新答复标识，都多一行；字节界只限住了单行的大小。保留期与聚合（例如按（租户、答复方、依据、日）计数，或按答复方设上限）另票 [demo-chain-gaps/170](../../.scratch/demo-chain-gaps/issues/170-booking-answer-bounce-register-has-no-retention-or-aggregation.md)。
5. **册写不动借答复册的未决名**（TF owner）。挡回册写不动答 `ANSWER_STORE_UNAVAILABLE`；服务方答在册订舱时答复册读不动也是这一格。承运方的续办相同（稍后原样重交），另立一个原因只是把挡回册的存在告诉外面。
6. **被挡回报文的内容不留**（TF owner）。五值、接受量与答复时刻都不进挡回册，运营核对时要请承运方重发。审计若要留原文，去处是挡回册加列或另立原文册，按（租户、答复方、订舱、答复标识）对得上。
7. **字节界是产品常量**（TF owner）。订舱标识与答复标识超过 512 字节答`未受理`，不由租户配；承运系统或运营给的标识若真有更长的，界要抬，抬之前先核挡回册键长离索引上限的余量。
8. **限读作用于 TF 全部命令口**（TF owner）。解码门是 TF 各命令口共用的那一个，1 MiB 不只管订舱两口。真正可能撞上的是带列表的载荷：运输委托提交与装载分配的成员列表 `members`，按演示长度的对象标识约 4 万个到顶；承运总单的关联列表 `associations` 每条约 52 字节，约 2 万条到顶，逐件关联包裹的总单最可能先撞上。超限答 400 `MALFORMED_REQUEST`（命令口统一把坏报文映成 400）。
9. **键长余量靠租户与来源身份短**（TF owner；知会接入身份 owner）。租户标识与答复方的来源身份来自集成客户端登记，本口不设界；两者加上两个满界的标识仍远在索引上限之下，前提是登记时给的身份不长到千字节。依据定宽之后，租户与来源身份在两条挡回路上是同一个值，再长也分不开两条路；过长的后果只是两条路一起答未决——是可用性问题，不是泄露的残余。

## Links

- [ADR-0202](./0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md)：文末补记「承运答复的次序与格」那一格部分停用；越权风险点 14 由本记录回答；决定二（来源取自集成客户端绑定）不变
- [ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md)：同笔落库、首末时刻的台账先例
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)、[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：订舱不在册答 200，与`未受理`按续办分开
- [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)：决定三，集成客户端族
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：三类
- [TF CONTEXT](../domain/transport-fulfillment/CONTEXT.md)：Language「承运答复挡回」
- [UC-TF-004](../application/transport-fulfillment/UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md)：`AT-TF-149`、`AT-TF-153`、`AT-TF-326`–`328`
- 代决复核报告 `.scratch/owner-review-20261020/report.md`：R8、R9
- 票 [145](../../.scratch/demo-chain-gaps/issues/145-booking-answers-from-a-non-provider-answer-booking-not-found.md)；后继票 [167](../../.scratch/demo-chain-gaps/issues/167-booking-answer-bounce-register-has-no-read-face.md)、[170](../../.scratch/demo-chain-gaps/issues/170-booking-answer-bounce-register-has-no-retention-or-aggregation.md)；相关票 [144](../../.scratch/demo-chain-gaps/issues/144-carrier-booking-answer-integration-notes-state-current-limits.md)、[48](../../.scratch/demo-chain-gaps/issues/48-booking-requests-and-carrier-answers-have-no-read-face.md)
