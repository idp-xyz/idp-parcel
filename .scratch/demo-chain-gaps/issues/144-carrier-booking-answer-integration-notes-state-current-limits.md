# 144 承运答复口首版集成说明未写现形限制

Category: enhancement
Status: in-progress——2026-10-25 作者完工，待隔离评审与进 main，分支 claude-dcgdocs tip `ac6f096b1`（本票内容 `1951c6657`、`ac6f096b1`，票面那一笔在其上；见文末「完成记录」）。此前 in-progress——2026-10-25 会话 `tops-20`（Claude Code）认领，文档票同批（128、134、137、138、141、144），分支 `claude-dcgdocs`（worktree `idp-parcel-dcgdocs`）。此前 ready-for-agent——2026-10-20 代决复核立（文档），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 R8；ADR-0202 决定二

## 现象与修法

现象、证据与修法的完整写法以复核报告 R8 一条为准（单一权威），此处只记要点：

代决「是」：按现形作为首版集成契约；集成说明写明候补位次更新、候补后分批接受目前进不来。

## 补注（2026-10-21，票 [145](./145-booking-answers-from-a-non-provider-answer-booking-not-found.md) 的作者子代理）

票 145 改了答复口的一格，集成说明照 [ADR-0239](../../../docs/adr/0239-a-carrier-answer-from-a-non-provider-is-answered-booking-not-found-and-the-basis-is-kept-on-a-bounce-register.md) 写：答复方不是订舱所属委托的服务提供方时答 `200` `BOOKING_NOT_FOUND`，与订舱不在册同一格、同一副响应，线上不再有 `SOURCE_NOT_SERVICE_PROVIDER`；这一格的续办写「找租户核对订舱标识」，不写「这份答复不归你答」。凭什么挡回只记在租户侧的承运答复挡回册，不对承运方透出。

## 完成记录（2026-10-25，Claude Code 作者子代理，协调方 `tops-20`，分支 `claude-dcgdocs`，起点 `d3aa57fc3`）

### 提交

| SHA | 内容 |
|---|---|
| `1951c6657` | 新立 `docs/api/carrier-booking-answer-integration.md`；`docs/README.md` 补入口；UC-TF-004「给开发的交接」补一句指针 |
| `ac6f096b1` | 集成说明把文内位置词（「见下文」「下表」「上表」「上面几条」）改为按节名或「本文所列」 |
| 票面那一笔 | 本完成记录与 Status |

### 做了什么

- **找处所**：`docs/` 下没有任何承运方对接说明。grep `booking-answers` 只命中 ADR-0202、ADR-0239、ADR 索引、种子 README 与演示动线，票 145 的完成记录也写着「集成说明那份文件不存在」。于是新立一份，放在 `docs/api/`，与[首方客户 API 契约](../../../docs/api/first-party-api-contract.md)并列。
- **[承运订舱答复口集成说明](../../../docs/api/carrier-booking-answer-integration.md)**分六节：
  - 首版契约与证据层级：R8、证据 `S`。
  - 认证：集成客户端族，Bearer 令牌，按需要求证书绑定。认证失败各格写明码与续办。
  - 载荷：封闭解码，字段包括 `booking`、`answer`、`outcome`、`quantity`、`basis`、`decidedAt`，两个标识各以 512 字节为界。
  - 一次订舱能答几份：至多一份候补加一份定局；重放与冲突怎样分。
  - 响应：`201` 只给新登；各 `outcome` 的含义与续办。`BOOKING_NOT_FOUND` 照 ADR-0239 与「不是服务方」合为一格，续办写「找租户核对订舱标识」，正合票面补注。
  - 现形进不来的答复与以后怎样扩形：见下一节。
- **现形限制**（本票要点）：**候补位次更新**与**候补后分批接受**今天都答 `ANSWER_CONFLICT`，在册的仍是原来那一份，只能请租户另起订舱（ADR-0202 文末补记越权风险点 13）；没有候补、直接部分接受之后再补接受，同此。另写明答复只按一次订舱作答，不能逐对象接受，也没有委托级接受（ADR-0202 决定五）。
- **扩形只加取值**（R8）：已有取值的含义不改、不删，新形以新增的 `outcome` 取值进来。
- 各格逐一对过代码：`tfhttp.BookingAnswerPayload`、`writeBookingAnswerOutcome`、`commandEndpoint` 的认证格、`CommissionTransportHandler.AnswerBooking`、`domain.FormCarrierAcceptance`、`CarrierAcceptance.AdmitsSuccessor`。
- `docs/README.md` 在首方客户 API 契约之后补一行入口（AGENTS「新增权威文档」）。UC-TF-004「给开发的交接」补一句：改这一口的线格式、答复取值或续办时，同笔改集成说明。

### 与报告相抵处

无。报告 R8 只说「集成说明写明候补位次更新、候补后分批接受目前进不来」，没说写在哪；票面把集成说明当成已有的文件，实际没有，本票新立。

### 代决

1. 集成说明新立为 `docs/api/` 下的独立一份，不写进 UC-TF-004：UC 写给开发者，集成说明写给承运方的对接人，读者不同。新文件声明自己不另立规则，与出处相抵时以出处为准。
2. 「以后怎样扩形」一节加了一条对接方纪律：遇到不认识的响应 `outcome`，视为已形成答案、但本端不会处置，出队交人，不重发。R8 只说「扩形只加取值」；不加这一条，「加取值」对已上线的对接方就是破坏。写法照首方客户 API 契约同形的那一条（那一条出自 ADR-0140，只管首方族）。**请评审重点看**：这算不算替集成客户端族另立了一条规则。
3. UC-TF-004 那句反向指针不在报告修法里，是为了防集成说明随口子改动而变旧。
4. 认证失败那张表把 `ACCESS_CHANNEL_NOT_CONFIGURED`、`OUTSIDE_ADMISSION_SCOPE`、`IDENTITY_DEPENDENCY_UNAVAILABLE` 也列上：它们与操作者族同码，承运方同样会撞上（ADR-0202 文末补记「线格式」一条）。

### 自验

- 集成说明、`docs/README.md` 与 UC-TF-004 里新增的相对链接逐个 `test -e` 都在。所引节名都对过原文：ADR-0202 补记「订舱与答复都不入队」、种子 README「集成客户端渠道：演示发行方」、首方契约「`outcome` 新增取值不算破坏」。
- 没有新引入的行号或计数式引用，文内位置词已清（`ac6f096b1`）。
- `go run ./tools/adr-index -check` 退 0。
