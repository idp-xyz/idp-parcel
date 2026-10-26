# 144 承运答复口首版集成说明未写现形限制

Category: enhancement
Status: in-progress——2026-10-26 评审阻断已修、修订轮完，待复评与进 main，分支 claude-dcgdocs tip `867b157d2`（修订轮本票 `867b157d2`，票面那一笔在其上；见文末「评审记录」「修订轮」）。此前 in-progress——2026-10-25 作者完工，待隔离评审与进 main，分支 claude-dcgdocs tip `ac6f096b1`（本票内容 `1951c6657`、`ac6f096b1`，票面那一笔在其上；见文末「完成记录」）。此前 in-progress——2026-10-25 会话 `tops-20`（Claude Code）认领，文档票同批（128、134、137、138、141、144），分支 `claude-dcgdocs`（worktree `idp-parcel-dcgdocs`）。此前 ready-for-agent——2026-10-20 代决复核立（文档），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
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

## 评审记录（2026-10-26，tops-20 隔离子代理，钉 `472dae3aa`）

照抄评审全文（`评审 ← tops-20 隔离子代理 · 钉 `472dae3aa` · 00:07`）：

结论：阻断 2 条（Standards 144 一条、Spec 141 一条），非阻断 3 条。

### Standards
阻断：
- `docs/api/carrier-booking-answer-integration.md` 把不带 `/v1` 的路径与 `{"error":{"code"}}` 问题体当首版契约写出，没引 ADR-0140。ADR-0140 决定二写明「集成客户端族各口同样加」`/v1`，决定三写明 4xx/5xx「一律 `application/problem+json`」，缺口已立票 docs-consistency/code/82。承运方照这份说明对接，code/82 一落地就断。修法：补引 ADR-0140，并在这两格注明「这是现状，随 code/82 改，不在『只加取值』承诺之内」。
非阻断：
- 同文件四处「ADR-0202 文末补记／文末复核记录」是跨文件位置词（AGENTS「改文档」不许），且已不准：ADR-0202 现末节是「复核记录（2026-10-20…）」，补记在其前。`6e53ee84f` 在 UC-TF-004 已把同一个词改成小节名，照办。
- 「不认识的 outcome 出队交人」：集成族无 ADR 明文授权（ADR-0140 决定五主语是首方契约）。判非阻断（不改服务端行为；是 R8「只加取值」对调用方成立的前提；与 ADR-0022 同向）。问题在出处：文首说「本文不另立规则」，所列权威里却没有这一条。改为注明依据是 R8 加 ADR-0140 决定五的写法，延到集成族待 TF owner 复核。
无发现：docs/README 入口与 UC-TF-004 反向指针合规。

### Spec
阻断：
- 141：R2 点名两句确已落地，但还有相抵残句：UC-SA-002「客户商业依据」行把「计价基准时点」列为 `party-commercial` 交的项；同 UC `PAR-SET-02` 行把「计价基准」列为待取证项，而登记册已写「不向价卡取证」；PC CONTEXT「商业价格政策版本」写「它规定…基准时点」，UC-PC-001 的政策行也一样。ADR-0179 决定三救不了：那里的时点是 PS 随控制交来的判断时点（「财务控制请求」行的「有效时点」），形态在规则包时点格里选（ADR-0157 决定二），不在价格政策上。141 核对结果「没有一句还要求…」不实。修法：四处都删掉这一项，改正 141 核对结果。依据：SA CONTEXT「商业价格政策与可执行价卡不另声明」；ADR-0190 复核记录 R2；ADR-0179 决定一；代码 `partycommercial/adapters/http.PricePolicyBodyPayload` 无基准时点格，只有 `caliber.fx.asOfSemantics`（汇率取值时点）。作者 grep 只搜「声明」，漏了写作「规定」的那句与两张表的行。改 PC CONTEXT 是 R2 直接后果，可在本分支做。
非阻断：
- 134 `AT-SA-214` 两处措辞：「`AT-SA-175` 保存的…是例外」用旧句（现行 AT-SA-175 已写「留在评价上」，AT-SA-183 写「照 AT-SA-175」）；「待判断缺的是什么（含读不出的序列）留在评价上」与 `AT-SA-189`「按缺的是什么分格」字面冲突（分格已由 `pendingJudgmentCauseOf` 记进请求停因）。建议改为「照 AT-SA-175」与「缺的具体哪一项」。
无发现：128、138 已落地属实；134 结论与代码一致、括注有据、号不撞；137 新句不引入矛盾（比 10a 略准）；144 各格与代码对得上。

## 修订轮（2026-10-26，作者子代理，同一分支 `claude-dcgdocs`）

| SHA | 内容 |
|---|---|
| `867b157d2` | 集成说明补引 ADR-0140（评审 Standards 阻断），小节名与对接方纪律的依据（两条非阻断）；`docs/README.md` 入口同句改 |
| 票面那一笔 | 本节、评审记录与 Status |

- **阻断**：集成说明新立「路径前缀与错误体是现状」一节。写明两件事：路径今天不带 `/v1`，ADR-0140 决定二要把 `/v1` 延到集成族；错误体今天是 `{"error":{"code"}}`、`application/json`，ADR-0140 决定三要求 `application/problem+json`，`code` 的取值原样保留。这两格**不在「只加取值」承诺之内**，随票 docs-consistency/code/82 改，改时承运方跟着改。文首的路径、认证错误体、`400` 那一句各挂一个指向该节的指针；「以后怎样扩形」也写明这一承诺不管路径前缀与错误体。文首权威列表补 ADR-0140 决定二、三。
- **非阻断一**：四处「ADR-0202 文末补记／文末复核记录」改为小节名，照 `6e53ee84f` 在 UC-TF-004 的写法，即「补记：票 24 落地时的补充裁定」一节与「复核记录（2026-10-20…）」一节。
- **非阻断二**：「不认识的 `outcome` 出队交人」那一条写明依据。依据是 R8「只加取值」，加上 ADR-0140 决定五的写法；那一条的主语是首方契约，延到集成族这一口是代决，待 TF owner 复核。这一条不改服务端行为，与 ADR-0022 同向。文首「本文不另立规则」改为「除『以后怎样扩形』一节那条对接方纪律另注依据之外，本文不另立规则」，两处不再自相矛盾。
- `docs/README.md` 入口补「路径前缀与错误体是现状、不在其内」，以及「去向归 ADR-0140」。
- 自验：集成说明全部相对链接逐个 `test -e` 都在，新增的 code/82、ADR-0140 两个也在；没有新引入的行号、计数或跨文件位置词；`adr-index -check` 退 0。
