# 10 customs-compliance：申报发送通道与公开监管标准的参考配置

Category: enhancement
Status: ready-for-human——2026-10-28 第 1 项（申报发送通道）已进 main（[ADR-0342](../../../docs/adr/0342-a-declaration-is-sent-by-a-dispatch-consumer-through-a-registered-channel-and-crossing-the-send-boundary-is-an-appended-send-fact.md)），上调范围做完；第 2–7 项仍照[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)冻结，代理不认领。此前 ready-for-agent——2026-10-27 用户同意由冻结 R3 上调为「限量生产前必须」（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)「一之二」，理由引 PILOT-SCOPE）：第 1 项（申报发送通道）上调；第 2–7 项仍冻结；排在 tops-26 名下 6 张 code 票进 main 之后，先 10 后 22。此前 ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R3）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-09-24 通道 4 随票 02 立（登记册逐行拆分划出的产品策略，CC 一张）
Blocked by: 第 2–4 项的参考配置等 03
地盘：customs-compliance 的出向连接器适配器与参考配置（存放按 03）。
出处：[票 02](./02-split-parameter-register-and-retriage-deferrals.md)——[参数登记册](../../../docs/product/PILOT-PARAMETER-REGISTER.md) `PAR-CUS-01`、`PAR-CUS-02`、`PAR-CUS-04`、`PAR-CUS-06` 行内「〔ADR-0146 拆分〕」点名的部分；[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定二「公开标准的实例数据归参考配置」。已知缺口沿[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「按四项判据重定级」表 PN-05 行。

## 做什么

1. **申报发送通道**（`PAR-CUS-01/02`）。提交版本意图 `customs-compliance.declaration-submission.formed` 唯一的消费方是 VE 投影，没有把申报交给海关或报关服务商的连接器；首发要求授权确认后提交并保留提交尝试与逐步外部回执（PILOT-SCOPE），排除的是自动、不是提交。已知缺口：重定级表 PN-05 行第三项。
2. **公开监管结果代码与层次映射**（`PAR-CUS-01/02`「结果代码与层次映射版本」中属公开标准的部分）：按监管程序出参考配置；服务商私有代码的映射仍是租户取值。
3. **公开法规规则源、税则与税费版本、补充 / 更正 / 撤销 / 重报的法定适用条件**（`PAR-CUS-04`）：按地区出参考配置。
4. **法定义务目录与续办 / 独立义务的分类依据**（`PAR-CUS-06`「在册义务目录」「同一程序续办与独立义务的分类依据」）：前者按监管程序出参考配置，后者是判断方法。
5. **待核：重复识别、来源冲突、冲突前当前判断与依赖动作门禁的判断规则**（`PAR-CUS-01/02`「重复识别、来源响应冲突、外部结果冲突、冲突前当前判断及依赖动作门禁规则」）。外部结果分层与放行门禁已在 CC；核这几条是否都已有执行器，不齐的补。
6. **待核：内部合规限制的对象与动作词表、多个独立限制的并发与关闭关系**（`PAR-CUS-07`）。核 CC 的方向动作门禁与内部限制编排是否已覆盖。
7. **待核：资金事实与税费付款核对的关联方法**（票 02 暂缓清单：`sa-cc-funds-and-credential-seams/19` 判「真实关联规则属实例半边」）。核关联是按引用逐一匹配的方法（产品策略）还是某租户与银行的约定（租户取值），前者补内置形态。
8. **立案与提交申报的生产入口与触发面**（[票 05](./05-demo-journey-criterion-evidence.md) 格 9；2026-09-24 经用户授权自决补入）。据票 05 取证，`NewEstablishCaseHandler`、`NewSubmitDeclarationHandler` 在 `cmd/` 零引用，接受决定的扇出 `acceptanceFan` 只到 VE 与初始路由，生产进程里没有任何端点或消费门会建案或提交。装配是机制；谁在什么业务时点为哪些包裹建案、提交，这个触发面是产品策略。开发主线重定级表 PN-05 行第一项当时记「未见缺口」，此项补上。

首发随附哪些程序与地区的参考配置，归 CC owner 与用户定（与 ADR-0146 越权风险点 5 同一口径）；本票只立形态并带一份样板。

## 不做

- 不接任何真实服务商账号，不替租户定自动提交政策或凭证实例。
- 关务角色与权限（`PAR-CUS-03`）归票 07。

## 完成判据

- 申报发送通道至少一种形态能对合成对端跑通「授权确认 → 提交尝试 → 外部回执」；第 2–4 项各有一份可显式采用的参考配置样板或判断方法（带测试）；登记册对应行同步收短。

## Comments

- 2026-10-21 · Claude Code 会话（票 [18](./18-customs-case-and-declaration-submission-entry.md)，[ADR-0227](../../../docs/adr/0227-customs-case-and-declaration-submission-enter-production-through-operator-decision-faces.md)）：「立案与提交申报的生产入口与触发面」一项已由票 18 承接——建案、提交授权确认、申报提交执行三口挂操作者渠道的运营决定能力面，生产进程有了「授权确认 → 提交版本」那一段。提交执行口不发送任何东西，首次尝试记 `REGISTERED_PENDING_SEND`、`sent_at` 为空（迁移 `customs_compliance/0025`）；提交版本形成信封照旧只有 VE 投影消费。今天的「已登记待发送」只形成了 UC-CC-005 定义的一半：没有授权使用关系，唯一的意图 `customs-compliance.declaration-submission.formed` 只有 VE 投影消费、不是发送意图——**它不含发送意图**。「申报发送通道」一项的连接器接上时要先裁两件：发送意图从哪来（随提交版本同笔另入队一封给连接器的信，或连接器扫待发送的尝试行）；越过发送边界是推进这一份尝试的状态，还是另记一笔发送事实——尝试行今天没有 UPDATE（ADR-0227 越权风险点 7）。
- 2026-10-21 · Claude Code 会话（票 [docs-consistency/03](../../docs-consistency/issues/03-cc-docs.md)，[ADR-0242](../../../docs/adr/0242-a-technical-receipt-belongs-to-its-submission-attempt-not-to-the-external-result-register.md)）：「申报发送通道」一项的技术回执裁向——技术回执按它回应的那次提交尝试归属、记在提交一侧，由本通道随发起的尝试接收（同步响应在同一次发送里，异步回执按渠道外部关联指回原尝试）；外部结果册只有监管侧各层，`/customs/external-results` 不收技术回执。保全、去重、同身份冲突与待关联照 UC-CC-006「来源接受与关联规则」。一份响应同时带技术与监管语义时，技术一半落尝试、监管一半进外部结果册，两份引同一来源记录。后到的回执追加为该尝试的新来源事实、派生新的当前技术判断，不改写原结果；落成推进尝试状态还是另记一笔回执事实，与 ADR-0227 越权风险点 7 的「越过发送边界」同一问，本票一并裁。
- 2026-10-28 · Claude Code 会话（第 1 项，[ADR-0342](../../../docs/adr/0342-a-declaration-is-sent-by-a-dispatch-consumer-through-a-registered-channel-and-crossing-the-send-boundary-is-an-appended-send-fact.md)）：两处 2026-10-21 的问已裁——发送意图就是提交版本形成信封，dispatch 加申报发送一条腿；越过发送边界另记发送事实、尝试行不改，技术回执逐份追加、当前判断折出。连接器是对登记端点的 JSON POST（通道册是租户取值，凭据只登变量名）；异步回执走 `POST /customs/technical-receipts`，凭消息号关联。分支 `claude-psb10`，非作者评审两轮：第一轮阻断 B6（越过边界之后的失败回滚成没发、反复重发）、B7（跟随重定向把已发认作没发），第二轮阻断 B6′（超长消息号写不进唯一索引），均已修；其余建议或修或写进 ADR 越权风险点。进 main 前在隔离树 rebase 到 `bb36a1008`（其间 launch-wiring/08 落了 `data/customs` 34–42，README 那一行合并时改成十二册 42 份），重生成 ADR 索引、AT 计数、机制清点，`scripts/test-full.sh` 退 0，一次性库灌种子退 0（步骤 `461` 答 `REGISTERED`）。实走（一次性库 + 合成报关服务商 + parcel-dispatch + parcel-api，证据只记 `S`）：提交一件 → 接受 → 立案、授权确认、提交（`REGISTERED_PENDING_SEND`）→ dispatch 发出、服务商答 `ACCEPTED`、发送事实 `ACKNOWLEDGED` 带消息号 → `send-one-declaration.sh` 送异步回执，当前技术判断 `ACKNOWLEDGED`，重送答 `EXISTING_RECEIPT`；尝试行仍是 `REGISTERED_PENDING_SEND`。实走查出脚本末行打印的引号错（首跑回执已记下，只是打印崩了），评审之后修了这一行。已知缺口见 ADR-0342 越权风险点（发送在事务里、意图悄悄丢失的四格、回执只认租户等）；演示种子票 launch-wiring/08 加 `data/customs` 文件时注意 `33` 已占。
