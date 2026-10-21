# ADR-0242：技术回执落在它所回应的那次提交尝试上，不是外部结果册的一层；外部结果接收口只收监管侧各层

Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「需要修改文档，确保自洽，正确」与常设授权「你作为业务和系统专家自决」；票 docs-consistency/03，代 CC owner 裁定审计 cc M6。不停用任何已接受记录的条款。越权风险点见文末，待 CC owner 复核。）<!-- 索引注记 -->｜**落点**：技术回执是申报渠道对一次提交尝试的连接、报文接收或技术校验反馈，按尝试归属、记在提交一侧；外部结果册（`ResultLayer`）是监管侧各层，不另加层，`/customs/external-results` 不收技术回执。**入口**：由申报发送通道随它发起的那次尝试接收，同步响应在同一次发送里、异步回执按渠道外部关联指回原尝试；保全、去重、同身份冲突与待关联照 UC-CC-006 来源接受规则。一份响应同时带技术与监管语义时，技术一半落尝试、监管一半进外部结果册，两份引同一来源记录。**当前技术判断**：后到的回执追加为该尝试的新来源事实、派生新的当前技术判断，不改写原结果；推进尝试状态还是另记一笔回执事实归票 psb/10。**首发**：没有发送通道，技术回执无入口
Date: 2026-10-21

## Context

- 审计 cc M6（`.scratch/docs-consistency/audit/cc.md`，钉 `6b0074d0`；2026-10-21 按 main `205eb254` 复核，现象仍在）：UC-CC-006「外部结果层次」表首行是「技术回执 | 申报渠道对连接、报文接收或技术校验的明确反馈 | 提交尝试及技术动作」，`AT-CC-140`、`141`、`144`、`153` 要本用例形成技术回执；代码 `domain.ResultLayer` 是「封闭六层……提交尝试与技术回执在提交对象上，这里是监管侧六层」，从 `REGULATORY_RECEIPT` 到 `DISPOSITION_DECISION`，没有技术回执。ADR-0197 决定二的类型词表也只有监管侧各层加全部、部分、附条件放行。
- 技术结果今天只是 `domain.AttemptResult` 的 `ACKNOWLEDGED` / `FAILED` / `PENDING_CONFIRMATION`（另有 ADR-0227 的 `REGISTERED_PENDING_SEND`），在形成尝试时就定了；尝试行没有 UPDATE（ADR-0227 越权风险点 7）。后到的技术回执今天无处可收——申报发送通道不存在（票 product-strategy-boundary/10「申报发送通道」一项）。
- CC CONTEXT Rules「外部结果与规则时效」：「提交尝试、技术回执、监管接收、业务受理、监管过程决定、监管核定税费、放行结果和监管处置决定必须分层保存」——要求分层，没说放在哪一册。Lifecycles「提交与外部监管结果」：「发起提交尝试后，技术结果可以是成功、失败或待确认；技术成功和失败都只表达技术层」。GLOSSARY「技术回执」：「`customs-compliance` 保存来源回执及其与提交尝试的关系」。
- 两种读法都说得通，没有 ADR 定过：代码按「尝试的状态」，UC-CC-006 按「外部结果的一层」。

## Decision

**一、技术回执按提交尝试归属，记在提交一侧。** 它回答的是「这一次发送，渠道收没收到、校验过没过」，同一提交版本的每次尝试各有各的回执。外部结果册按提交版本归属、按层判同层一致性、逐层投进 VE（ADR-0197），是监管侧各层；本记录不给它另加一层，`ResultLayer` 不变，外部结果接收口 `/customs/external-results` 只收监管侧各层，不收技术回执。CONTEXT 的「分层保存」由两册分担：提交尝试与技术回执在提交一侧，监管接收到监管处置决定在外部结果册。

**二、技术回执由申报发送通道接收。** 同步响应在发送那一次调用里拿到，异步回执按渠道的外部业务关联指回原尝试——能这样关联的只有发起那次尝试的通道（票 psb/10）。接收纪律与 UC-CC-006「来源接受与关联规则」同一套：先保全来源、同一来源消息身份与内容重到答已有、同身份异内容是来源响应冲突、关联不上唯一尝试即待关联。一份响应同时带技术语义与监管语义（如「报文接收成功」加「监管已接收」）时，技术那一半落在尝试上，监管那一半照 UC-CC-006 进外部结果册，两份都引同一来源记录；监管那一半不因与技术那一半同到而少过一道来源权威判断。

**三、尝试的当前技术判断由技术回执派生，不改写。** 尝试形成时的技术结果是第一份技术事实；后到的技术回执追加为该尝试的新来源事实，派生新的当前技术判断（例如待确认之后到了明确的接收成功），原结果与原回执保留。这件事落成「推进尝试状态的一行新版本」还是「另记一笔回执事实、尝试行不动」，是持久化形状，与「越过发送边界是推进状态还是另记发送事实」同一问，一并归票 psb/10（ADR-0227 越权风险点 7）。

**四、首发：没有发送通道，技术回执无入口。** `AT-CC-140`、`141`、`144`、`153` 的技术那一半等票 psb/10；外部结果口的载荷层次只认监管侧各层，标作技术回执的载荷是词表外的坏报文，不进编排、不进外部结果册。

## 候选与反方

**甲 · 外部结果册另加「技术回执」一层。** UC-CC-006 原写法。反方：外部结果册的一致性判法按提交版本比同层（ADR-0226：监管接收、业务受理等「一版提交一个结论」），技术回执若进来，第二次尝试的技术失败会被判成与第一次的技术成功矛盾；ADR-0197 逐层投进 VE，技术层要么加一条不投的例外，要么把渠道技术细节漏进客户视图。外部结果册还按版本归属，技术回执却得指回具体的那一次尝试。否决。

**乙 · 技术回执只是尝试上的一个结果字段，不保全来源。** 代码今天的形状。反方：CONTEXT 要求来源先保全、分层保存，一份渠道响应可以同时承载技术与监管两层；只留一个字段，原始响应与它和监管那一半的关系就丢了，「同身份异内容」也判不出。否决（保留「记在尝试一侧」那一半）。

**丙 · 外部结果口按载荷里的层次一并接收技术回执，再在内部分流到尝试。** 反方：外部结果口按提交版本归属（ADR-0227 越权风险点 7 末句：尝试仍待发送时收到回执也按版本照收），说不出回执属于哪一次尝试；能指回尝试的只有发起它的通道。否决。

## Consequences

- UC-CC-006「外部结果层次」表首行与「后续业务交接」表首行、`AT-CC-140`、`141`、`144`、`153` 注明落点与首发无入口；UC-CC-005「提交尝试与发送阶段」表「技术结果已明确」一行改为发送通道接收。CC CONTEXT「技术回执」词条写明落点。
- 票 [product-strategy-boundary/10](../../.scratch/product-strategy-boundary/issues/10-cc-declaration-channel-and-public-regulatory-reference-configuration.md)「申报发送通道」一项按本记录接收技术回执，Comments 记裁向。
- 不改代码：`ResultLayer`、外部结果接收口与 ADR-0197 的词表都已是本记录的形状。

## 越权风险点

1. **报关服务方线下代发的技术回执没有入口**（CC owner）。本产品没有发起那次发送，就没有可指回的尝试；外部结果口照 ADR-0227 越权风险点 7 末句按版本收监管那一半，技术那一半今天无处可收。真实服务方若只回技术层，要等 `PAR-CUS-01/02` 给出它的语义再定。
2. **技术回执怎样驱动「原提交结果仍待确认」**（CC owner）。待确认与安全再次发送判断在 UC-CC-006，技术回执在尝试一侧；两边怎么接（技术失败而监管侧可能收到时继续待确认，`AT-CC-141`），归票 psb/10 与 UC-CC-006 原提交查询一并实现。
3. **GLOSSARY「技术回执」所有者一句与本记录同向**，不改；「申报提交尝试」条是否补「技术回执记在尝试一侧」，交票 docs-consistency/10 判。

## Links

- [ADR-0227](./0227-customs-case-and-declaration-submission-enter-production-through-operator-decision-faces.md)：决定四「已登记待发送」与越权风险点 7（越过发送边界的形状归票 psb/10）
- [ADR-0197](./0197-customs-external-results-enter-tracking-projection-per-layer.md)：外部结果按层投进 VE，类型词表只有监管侧各层
- [ADR-0226](./0226-successive-same-layer-customs-results-are-successors-unless-they-contradict.md)：同层一致性按提交版本判（候选甲的反方）
- [CC CONTEXT](../domain/customs-compliance/CONTEXT.md)：「技术回执」「提交尝试」词条；Rules「外部结果与规则时效」首条；Lifecycles「提交与外部监管结果」
- [UC-CC-005](../application/customs-compliance/UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md)、[UC-CC-006](../application/customs-compliance/UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md)
- 审计 [cc.md](../../.scratch/docs-consistency/audit/cc.md) M6；票 [docs-consistency/03](../../.scratch/docs-consistency/issues/03-cc-docs.md)、[product-strategy-boundary/10](../../.scratch/product-strategy-boundary/issues/10-cc-declaration-channel-and-public-regulatory-reference-configuration.md)
