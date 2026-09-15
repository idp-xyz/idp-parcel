# CC 三只交接信封的 ID 是「租户 / 类型 / 范围 / 税费 / 资金 / 64 位指纹」串接，真长度引用下超过框架信封 ID 上限、`EnqueueOnce` 拒收；而 `handOffVerification` 把交接失败折成续办引用、核对不翻，重派路上那个续办引用无人读——核对行提交、信封没发、消费入账成功、无人知道

Category: bug
Status: resolved——**2026-09-15 14:2x 通道 4（二次改派）**（task-7d4346c0-75d0-4379-b69c-4e7d8a527019；分支 `mcp4-sacc29` 基 `cbffc269`，代码三笔 `976966ee`（裁决 1）/ `e8d73f09`（裁决 2）/ `b2229462`（装配 + 判据 (2)(4) 真库），代码 tip `b2229462`；本完成记录 + 05 一句紧随一笔；通道 6 / 2 先后 crash 未开工。逐条判据、裁决、判断项、验证与能力边界见下方「完成记录」）。此前 ready-for-agent——**2026-09-15 12:5x 通道 1 按用户「代裁」代裁（CC owner 口径），四条「要裁的」写入下方「裁决」节**：三只 `*EventID` 一次同改成定长指纹形（固定口名前缀 + `sha256` 十六进制，必在 `eventing.MaxEventIDLength` 内），载荷五维全量照旧；重派路上交接失败不再折成续办引用——依赖不可用归未决重投、信封不合法归硬失败，两格都整笔回滚；人重核路 05 的兜底本票不动；SA 侧靠采用登记册五维主键守幂等、真库用例钉住。上限已量实：`go.idp.xyz/idp-bento-go/eventing` `MaxEventIDLength = 128`，`Envelope.Validate` 对 `id` 查非空 / UTF-8 / 无控制符 / ≤ 128 字节。此前 draft——2026-09-15 12:4x 通道 1 立票（sa-cc/19 评审 ← 通道 6 Spec ② + 19 完成记录判断项 ③ ④ 转记；原作者会话 11:1x 广播量到 138 字节被拒是第一手）。只写票面未动代码；取证锚 main `36fb5437`
Blocked by: 无（[19](19-cc-new-funds-fact-version-forms-a-new-verification-version.md) 已进 main `49ffc96c`；本票是它量出来的、不归它重裁的那一件）

## 缺口（取证于 `36fb5437`，逐符号名）

- **ID 形**：`internal/customscompliance/adapters/postgres/duty_payment_verification_handoff.go` `dutyPaymentVerificationEventID(key)` = `dutyPaymentVerificationPartitionKey(key) + "/" + Duty + "/" + Funds + "/" + Digest`，分区键 = `TenantID + "/duty-payment-verification/" + Scope`；`Digest` 是 `verificationDigest` 的 64 位十六进制。同形还有 `gate_verification_handoff.go` `gateVerificationEventID` 与 `verification_handoff.go` `verificationEventID`（都含 64 位指纹）。
- **上限**：框架 eventing 对信封 ID 有长度上限，sa-cc/19 原作者实测 `tenant-a` + `SYN-UNIT-RD` + `SYN-DUTY-RD/v1` + `bank-fact-2` 拼出 138 字节即被 `outboxintent.EnqueueOnce` 里的 `store.Enqueue` 拒收（19 完成记录判断项 ④）。既有用例全用短指纹（`digest-v1`）或替身，从没碰到；19 装配第三格为绕开它把税费 / 范围取短（`SYN-D9` / `SYN-U9`）并显式断言 `HandoffReference()` 为空。**上限值与校验所在的框架符号本票立票人没读**——归作者开工第一步量（见能力边界）。
- **兜底把失败吞成静默**：`internal/customscompliance/application/reconcile_duty_payment.go` `handOffVerification` 在 `SaveVerification` 答`已登记`后同一 ctx 调 `Handoff.HandOffDutyPaymentVerification`；失败**不翻核对**，返回 `"CONT-DUTY-VERIFICATION/" + Scope + "/" + Digest[:8]` 续办引用，outcome 仍 `DutyVerificationFormed`（既有头注「失败不翻核对，留续办引用」——05 之形，人重核那条路调用方拿得到 `HandoffReference()`）。
- **重派路上续办引用无读者**：`RederiveDutyVerificationsOnFundsFactVersion` 把每条谱系的 `VerifyPayment` 结果原样放在 `Lineage.Result` 上、只看 `Outcome()`；消费侧 `adapters/settlementaccounting/receive_on_adopted_funds_fact.go` `rederivationConsumption` 对`已重派`一律 nil → 消费门提交。于是一次**不毒化事务**的交接失败（ID 超长正是这种）会让 (a′) 核对行随事务提交、信封没发、SA 永远收不到那一版、消费入账成功、续办引用只留在结果对象上。19 裁决 3 (1)「版本行、新核对版本、交接意图三者同生同灭」由此只对库级故障成立（19 评审 Spec ②、判断项 ③）。
- **不是 19 的范围**：19 只是让这条既有兜底多了一条无人看的路；ID 形与兜底形都是 05 定的。

## 语言从哪里来

CC `CONTEXT.md` 集成规则：核对形成后交 `settlement-accounting` 采用（05 落地的 `DutyPaymentVerificationHandoff`，一版一封）；`docs/agents/parallel-sessions.md`「不同的『绿』在输出上长着同一张脸」——**信封没发**与**信封已发**在消费门看来同一张脸，这正是那一节要求「要么做进结构、要么写证据」的形。`internal/platform/dispatch/dispatcher.go` 失败码注释「失败码按运维要做的动作取值……`publish_failed` 是几格里唯一必须人动手的」——ID 超长重投不自愈，属这一格。

## 做法候选（只列不选，归 CC owner）

**ID 形状**（三只 `*EventID` 同形，裁一次三处同改或只改 duty-payment）：

1. **指纹化**：ID = 固定前缀 + `sha256(租户 / 范围 / 税费 / 资金 / 指纹)` 的定长十六进制（或 base32），载荷 `dutyPaymentVerificationPayload` 五维照旧全量。定长、必在上限内；代价是 ID 不再可读，运维从 ID 反查要经载荷。SA 消费门按信封 ID 去重——新形只对新信封，已消费的旧 ID 不受影响；但**同一份核对若在换形前后各发一次**（重投场景）会被当两封，要量 SA `duty_payment_verification_adoption` 主键（五维照抄 CC 键、不含信封 ID）是否已把这层兜住。
2. **缩短组成**：去掉分区键里与载荷重复的段、`Digest` 取前 N 位。可读性保住，但「必在上限内」取决于四个引用的真长度——租户与范围引用是实例半边、长度不归本仓定，只能给上界不能给保证。
3. **长度校验前移**：`VerifyPayment` 形成核对**之前**先算 ID 长度，超长直接答未决 / `未受理`（人动手改引用），不落核对行也不发信封。治本在调用方，但把框架的限制变成了 CC 的业务结果格，且对已落库的核对无效。

**重派路上交接失败怎么响**（与 ID 形状独立）：

- (a) `RederiveDutyVerificationsOnFundsFactVersion` 任一谱系 `HandoffReference() != ""` → 整笔 `ErrDutyVerificationRederivationUndecided`，事务回滚、从接收重投。**重投治不了 ID 超长**——会以未决之名耗尽失败预算（`dispatcher.go` 头注点名的反例）。
- (b) 把它折成派发器能分格的**硬失败**（`publish_failed` 那一格的动作：人动手），事务回滚、不留半截。
- (c) 保留「核对行提交、信封没发」的既有形，但把续办引用**登下来**（登在哪：`duty_payment_collaboration` 今天 `kind` 封闭二值，加格要改 CHECK 与键；或新表；或消费侧结果对象加一格让 `rederivationConsumption` 至少把它写进日志 / 指标）。
- 人重核那条路 05 的兜底（调用方看 `HandoffReference()`）要不要一并改，随上面裁。

## 红线

- 三轴不由编排猜、前版一字不动（19 裁决 1）不受本票影响；本票不碰核对内容，只碰信封 ID 与失败的响法。
- 实例半边不写死：租户 / 范围引用长度不归本仓定，任何「够短」的判断要写成对上限的量法，不写成对某租户的假定。
- 迁移 `0016` / `0019` / `0021` / `0022` / `0023` 不改；SA 目录若要改（去重键）单独成笔、先量。

## 完成判据（待裁后写实；可 grep）

1. 真长度引用（≥ 上限的合成串）下 `VerifyPayment` 交接**不再静默**：要么信封发出（ID 形改）、要么结果可观测（未决 / 硬失败 / 登记），装配用例一格钉住。
2. 19 装配第三格把 `SYN-D9` / `SYN-U9` 取短的理由句随裁决改口或删掉；`HandoffReference() == ""` 的断言按裁决留或改。
3. 三只 `*EventID` 与 SA 消费门去重的关系写进判断项（换形对重投 / 去重的后果）。
4. 带 DSN CC postgres + `cmd/parcel-dispatch` + SA adapters/customscompliance 全 PASS 非 SKIP。

## 地盘

`internal/customscompliance/adapters/postgres/duty_payment_verification_handoff.go`（及 `gate_verification_handoff.go` / `verification_handoff.go` 若三处同改）、`internal/customscompliance/application/reconcile_duty_payment.go`（`handOffVerification`）、`internal/customscompliance/application/rederive_duty_verifications.go`、`internal/customscompliance/adapters/settlementaccounting/receive_on_adopted_funds_fact.go`（`rederivationConsumption`）、`cmd/parcel-dispatch/assemble_test.go`（第三格）。SA 侧 `internal/settlementaccounting/adapters/customscompliance/duty_payment_verification_consumer.go` 只在 ID 形变且去重依赖 ID 时才碰。撞点：无在途分支碰 CC。

## 要裁的（全归 CC owner）

1. **ID 形状取哪条**（指纹化 / 缩短 / 前移校验），以及三只同形 ID 是一次同改还是只改 duty-payment。
2. **重派路上交接失败响成什么**（未决重投 / 硬失败 / 登续办），登续办的话登在哪。
3. **人重核路的既有兜底**（`handOffVerification` 吞错留续办引用）是否随 2 同改——它是 05 定的形，改它要在 05 票面留一句。
4. **SA 消费门去重**若依赖信封 ID，换形对「同一核对在换形前后各发一封」怎么处置。

## 裁决（2026-09-15 12:5x 通道 1 按用户「代裁」代裁，CC owner 口径；依据是 19 评审 Spec ②、19 完成记录判断项 ③ ④、本票缺口节，外加本次量实的框架上限，钉 `e1ab9fb5`）

0. **上限量实**（补缺口节「没读」那一格）：`go.idp.xyz/idp-bento-go@v0.1.0-rc.2` `eventing/types.go` `MaxEventIDLength = 128`（同表 `MaxSubjectLength` / `MaxPartitionKeyLength` = 512、`MaxFailureCodeLength` = 128）；`eventing/envelope.go` `Envelope.Validate` 对 `id` 走 `validateString`：非空、合法 UTF-8、无控制符、`len(value) > max` → `invalidEnvelope("id exceeds 128 bytes")`。校验在信封构造 / 入队时发生，是**确定性**失败——同一份输入重投永远同一个结果。
1. **要裁的 1——指纹化，三只一次同改。** ID = 固定口名前缀 + `sha256(租户 / 范围 / 税费 / 资金 / 指纹)` 的十六进制（前缀由本仓写死、哈希定长，总长必在 128 内——这是唯一一条不依赖实例半边长度就能保证的路）；三只 `dutyPaymentVerificationEventID` / `gateVerificationEventID` / `verificationEventID` 抽成一个 helper 同形同改——留两只带同一个潜伏缺口等于让「同一张脸」再长两回。**为什么不缩短、不前移校验**：缩短的「够短」取决于租户与范围引用的真长度，那是实例半边，只能给上界不能给保证；前移校验把框架限制变成 CC 的业务结果格、对已落库的核对无效，且治的仍是「够短」而非「必短」。载荷 `dutyPaymentVerificationPayload` 五维全量照旧——运维从 ID 反查走载荷，`Subject`（今天 `范围 / 税费`）与 `PartitionKey`（`租户 / 口名段 / 范围`）保留可读形不新增字段；两者上限 512，作者按 `TenantID` / `DecisionScopeReference` 构造门的限长量「能不能超」，能则同法取哈希（分区键哈希不改「同范围同分区」的顺序语义），不能则写进判断项。
2. **要裁的 2——重派路上交接失败不再折成续办引用，分两格、都整笔回滚。** `RederiveDutyVerificationsOnFundsFactVersion` 对每条谱系的交接结果：(a) **依赖不可用**（Outbox 存储错、事务错）→ 整笔 `ErrDutyVerificationRederivationUndecided`——与 `dependencyFailure()` 既有四个 `*Unavailable` 同格，消费门回滚、重投自愈；(b) **信封不合法**（框架 `Validate` 拒、确定性）→ 整笔**硬失败**：新哨兵**不进** `externalFundsFactUndecidedSentinels`，消费门回滚、inbox 无痕，重投同样失败直到派发器预算耗尽落账——响亮、人动手，正是 `dispatcher.go` 头注「硬失败重投不自愈，是几格里唯一必须人动手的」那一格。裁决 1 落地后 (b) 在 ID 上不再发生，留它是为 `Subject` / `PartitionKey` 与将来任何确定性校验错——不让「静默提交」这条路重新长出来。**为什么不登续办**：登在 `duty_payment_collaboration` 要改 `kind` 封闭集与键，是另一个词条；登在日志 / 指标不是结构，parallel-sessions「分不开的两态要么做进结构、要么写证据」——回滚就是结构。**落法**：`handOffVerification` 今天吞错返回续办引用的形不动（裁决 3），重派编排在拿到 `HandoffReference() != ""` 时**不接受**这个结果——它要的是交接错误本身来分格，所以 `handOffVerification` 要把原始错误也交出来（结果对象加一格或另一返回值，作者定形写判断项），人重核路照旧只看引用。
3. **要裁的 3——人重核路 05 的兜底本票不动。** `VerifyPayment` 直调时调用方（`parcel-api` 注册端点 / `parcel-customs-register`）拿得到 `HandoffReference()`，续办引用有读者、有响应格（CC 端点把它交回）；改它是 05 的题，不是 19 量出来的缺口。裁决 1 落地后这条路上的续办引用只剩依赖故障一种成因。
4. **要裁的 4——SA 侧靠采用登记册守幂等，不靠信封 ID。** Inbox 门（`inboxconsume`）按信封 ID 恰一次，只挡**同一 ID** 的重投；`duty_payment_verification_adoption` 主键是照抄 CC 核对键的五维（租户、范围、税费、资金、指纹），**不含信封 ID**——换形前后同一核对若各发一封（换形后重投 `EnqueueOnce` 查重查的是新 ID、查不到会再入队），Inbox 门放行第二封、采用编排撞五维主键答幂等。这一条今天是推断，**判据 (4) 用真库钉住**；已消费的旧 ID 不重算、不迁移。
5. **做法写实**：(1) `adapters/postgres`：新 helper（口名前缀 + `sha256` 十六进制）替三只 `*EventID` 的拼接体；载荷不动；`Subject` / `PartitionKey` 按裁决 1 量后定。(2) `application/reconcile_duty_payment.go`：`handOffVerification` 把交接错误随结果交出（形归作者），`VerifyPayment` 对外结果不变。(3) `application/rederive_duty_verifications.go`：按裁决 2 分两格，新硬失败哨兵；`adapters/settlementaccounting/receive_on_adopted_funds_fact.go` `rederivationConsumption` 与 `cmd/parcel-dispatch/assemble.go` 的未决哨兵集合**不**收新哨兵。(4) `cmd/parcel-dispatch/assemble_test.go` 第三格：`SYN-D9` / `SYN-U9` 取短的理由句删掉、引用改回该文件常规长度、`HandoffReference() == ""` 断言保留（它现在是「交接成功」的证据而非绕开）。(5) 三只 ID 的既有用例改断言形（定长、可重算），不断言字面串接。**不动**：迁移；SA 目录（除判据 (4) 的真库用例可放 SA adapters/customscompliance 测试）；05 的响应格。
6. **完成判据写实**（替换上方「待裁后写实」四条）：(1) 三只 ID 定长且 ≤ `eventing.MaxEventIDLength`，用例用**超长合成引用**（拼接形下 > 128 字节）构造信封仍入队成功；同一核对两次算 ID 相等。(2) 重派路：Outbox 存储替身答不可用 → `ErrDutyVerificationRederivationUndecided`、零核对行落库；信封校验错替身 → 硬失败哨兵、零核对行、不在未决集合（`errors.Is` 反断言）。(3) 装配第三格改回常规长度引用、`published == 1`、SA `FindByKey` 命中；19 那句取短理由不在。(4) 真库：同一核对以两个不同信封 ID 各投一封到 SA 消费者 → 采用登记册一行、第二封答幂等不报错。(5) `git grep -n 'SYN-D9\|SYN-U9' -- cmd/parcel-dispatch/assemble_test.go` 零命中（或作者留下并写明不再是绕开）；`0016` / `0019` / `0021` / `0022` / `0023` 零 diff；`gofmt -l` 空、`go vet` 0；带 DSN CC postgres + `cmd/parcel-dispatch` + SA adapters/customscompliance PASS 非 SKIP。(6) 05 票面 Comments 一句「ID 形已由 29 改为指纹形，兜底形不动」。
7. **能力边界**：裁的是 ID 形、失败分格、兜底归属、幂等靠谁；具体不变式（helper 的前缀字面与分隔、硬失败哨兵名、`handOffVerification` 交错的形、`Subject` / `PartitionKey` 能否超 512）归作者按代码定并写进判断项。读过：本票全文、19 评审 Spec ② 与判断项 ③ ④、`eventing/types.go` 常量表与 `envelope.go` `Validate` 校验段、`inboxconsume/consume.go` 包头（三态：重复跳过 / 毒丸 / 处理失败整体回滚）、`dispatcher.go` 失败码头注、`duty_payment_verification_handoff.go` 的 ID / Subject / PartitionKey 赋值行；**没读**：`EnqueueOnce` 正文、SA `duty_payment_verification_consumer.go` 处理方本体与 `AdoptOnDutyPaymentVerificationAdapter`、派发器对消费方硬失败的落账路径、三只 handoff 文件正文。作者量到与代码不符，以代码为准并写进判断项，不回头等我。

## 完成记录（2026-09-15 14:2x 通道 4 二次改派会话，钉 `b2229462`）

代码三笔由通道 4 上一会话写成并逐笔推 origin：`976966ee`（裁决 1）/ `e8d73f09`（裁决 2 + 3）/ `b2229462`（装配第三格、判据 (2) 集合半边、判据 (4) 与 (2′) 真库）；该会话跑完带 DSN 测试、广播释号后结束，完成记录没落。本节由通道 4 新会话（无在途记忆）按 `git diff cbffc269 b2229462`（16 件全 M、无增删）与代码逐条对判据代写，**不改代码**；点名到符号名与用例名，判断项写代码实际怎么做的。

### 逐条对裁决 6「完成判据写实」(1)–(6)

**(1) 三只 ID 定长、在上限内、可重算** → `internal/customscompliance/adapters/postgres/duty_payment_verification_handoff_test.go` `TestOverlongReferencesStillProduceAnEnvelopeIDWithinTheFrameworkLimit`：税费 / 资金 / 范围各取「前缀 + 六十个 x」的合成串，夹具先自检旧串接形 > `eventing.MaxEventIDLength`（造不出超长就 `Fatal`，本格不许空证）；同一意图交两次 → outbox 行数 1（第二次被 `EnqueueOnce` 当同一份）；ID 长度 == `len("duty-payment-verification/") + hex.EncodedLen(sha256.Size)` 且 ≤ 上限；分区键仍是可读形「租户 / duty-payment-verification / 范围」。`gate_verification_handoff_test.go` `gateEventID` 与 `verification_handoff_test.go` `verificationHandoffEventID` 改为按同公式重算（测试侧 `fingerprintedEventID`），既有用例拿它去库里找行——断言形从「字面串接」换成「可重算」（裁决 5 (5)）。

**(2) 重派路两格、都不报成功**：

- 依赖不可用 → `internal/customscompliance/application/rederive_duty_verifications_test.go` `TestARederivationTreatsAnUnavailableHandoffAsUndecided`：交接替身答普通错误 → 整笔 `DutyVerificationRederivationUndecided`、点名新 reason `DutyVerificationHandoffUnavailable`、`Lineages()` 空。
- 信封被拒 → 同文件 `TestARederivationFailsLoudlyWhenTheHandoffEnvelopeIsRejected`：替身答包着 `ports.ErrHandoffEnvelopeRejected` 的错误 → 返回错误 `errors.Is` `application.ErrDutyVerificationHandoffRejected` 且 `Is` `ports.ErrHandoffEnvelopeRejected`，结果是零值（`DutyVerificationRederivationOutcomeInvalid`、零谱系）。
- 消费侧 → `internal/customscompliance/adapters/settlementaccounting/receive_on_adopted_funds_fact_test.go` `TestARejectedHandoffEnvelopeOnTheRederivationPathIsAHardFailureNotAnUndecided`：前者折成 `ErrDutyVerificationRederivationUndecided`；后者折成新哨兵 `ccsettlement.ErrDutyVerificationHandoffRejected`（带原因），与 `ErrDutyVerificationRederivationUndecided` / `ErrFundsFactReceiveUndecided` 互不 `errors.Is`（反断言）。
- 集合半边 → `cmd/parcel-dispatch/assemble_test.go` `TestARejectedDutyVerificationHandoffIsNotRegisteredAsUndecided`：遍历 `externalFundsFactUndecidedSentinels` **本身**（不在测试里重列一份），硬失败哨兵双向不 `Is` 任一项；重派未决哨兵仍在名单。
- 「零核对行落库」在替身层只证到「零谱系报出」（替身无事务，见判断项 ⑦）；行级回滚由下一格证。
- (2′) 真库回滚 → `cmd/parcel-dispatch/assemble_test.go` `TestARederivedVerificationWhoseEnvelopeIsRejectedRollsBackLoudlyInsteadOfCommittingHalf`：范围引用取 `"SYN-UNIT-LONG-" + MaxSubjectLength 个 x` 让 Subject 必超 `eventing.MaxSubjectLength`；v1 先经人重核路形成核对（断言 05 之形：`DutyVerificationFormed` + 续办引用非空 + `HandoffError()` `Is` `ErrHandoffEnvelopeRejected`，裁决 3 不动）；v2 信封到达那一拍 `published == 0`、`failure_code == "dispatch.publish_failed"`，`ListFundsFactVersions` 仍只有 v1、`ListVerificationsByFundsFact` 仍只有一版且 `Delta == DeltaNone`——版本行、(a′) 核对行、信封三者一起没落。

**(3) 装配第三格改回常规长度**：`TestAnAdoptedExternalFundsFactReachesTheCustomsRegisterThroughTheRouteTable` 的 `lineageDuty` / `lineageScope` 由 `SYN-D9` / `SYN-U9` 改回 `SYN-DUTY-RD/v1` / `SYN-UNIT-RD`——正是 19 原作者拼出 138 字节被拒的那一对，本格因此成了原始报告的回归格；取短理由段删掉、换成一句「自 29 起定长指纹形，引用多长都装得下」；`HandoffReference()` 为空、该拍 `published == 1`、SA `FindByKey` 命中三条断言未动（diff 不含那几行），语义由「绕开」变「正路证据」。

**(4) 真库 SA 幂等靠五维主键**：`TestAFormedDutyPaymentVerificationReachesTheSettlementInputThroughTheRouteTable` 末尾追一段——同一份五维载荷以另一个信封 ID `duty-verification-second-id-for-digest-v1` 再投一封 → 该拍 `published == 1`、`failure_code` 空、`settlement_accounting.duty_payment_verification_adoption` 按（租户、范围、税费、资金、指纹）数行仍为 1：Inbox 门只挡同一 ID，第二封放行到采用编排、撞五维主键答幂等不报错。SA 目录零 diff（`git diff --stat cbffc269 b2229462 -- internal/settlementaccounting` 空）。

**(5) 可 grep 的几条**（本会话在 `mcp4-sacc29` 工作树实测，树 = `b2229462` + 本票与 05 两份 .md）：`git grep -n -e 'SYN-D9' -e 'SYN-U9' b2229462 -- cmd/parcel-dispatch/assemble_test.go` 零命中（退 1）；`git diff --stat main..b2229462 -- migrations/` 空（`0016` / `0019` / `0021` / `0022` / `0023` 一并零 diff）；`gofmt -l ./internal/ ./cmd/` 空；`go vet ./...` 退 0。带 DSN 测试**本会话未重跑**（55432 已释、通道 1 按 `b2229462` 派评审），取上一会话释号广播的数（通道 1 14:1x 确认收到）：`go test -p 1 -count=1 -v ./internal/customscompliance/... ./cmd/parcel-dispatch/... ./internal/settlementaccounting/adapters/customscompliance/... ./internal/architecture/...` → `--- PASS` 886 / `--- FAIL` 0 / `--- SKIP` 0、退 0（22 s）；本节点名的用例在该广播里全 `PASS` 非 `SKIP`。

**(6) 05 票面一句**：[05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md) Comments 追「ID 形已由 29 改为指纹形，兜底形不动」一条，与本节同一笔提交。

### 逐条对裁决 1–4

**裁决 1——helper 与三只口**：`internal/customscompliance/adapters/postgres/external_result_handoff.go` `fingerprintEventID(portName, dimensions...)` = `portName + "/" + hex(sha256(strings.Join(dimensions, "\x00")))`，与 `ccEventSource` 同文件。三只口各自一个口名常量：`dutyPaymentVerificationEventIDPort = "duty-payment-verification"`（五维：租户、范围、税费、资金、指纹）、`gateVerificationEventIDPort = "gate-verification"`（五维：租户、范围、动作、边界、指纹）、`verificationEventIDPort = "disposition-verification"`（三维：租户、决定、指纹）。ID 长度是常量 `len(前缀) + 1 + 64`——90 / 82 / 89 字节，与任一维多长无关。载荷结构体、`Subject`、`PartitionKey` 三处 diff 未碰（`PartitionKey` 只在 (1) 用例里被断言仍是可读形）。

**裁决 2——分格落在四层**：(a) `adapters/postgres` `OutboxDutyPaymentVerificationHandoff.HandOffDutyPaymentVerification` 对 `EnqueueOnce` 的错误 `errors.Is(err, eventing.ErrInvalidEnvelope)` 才包 `ports.ErrHandoffEnvelopeRejected`（新哨兵，头注写明「确定性 vs 重投会变」两格），其余原样返回；(b) `application/reconcile_duty_payment.go` `handOffVerification` 改返回 `(string, error)`，`DutyReconciliationResult` 加 `handoffErr` 与读口 `HandoffError()`（与 `HandoffReference` 同在场同缺席）；(c) `application/rederive_duty_verifications.go` 谱系循环里在既有 `dependencyFailure()` 判之后、`Formed / Existing` 记谱系之前插一格：`HandoffError() != nil` 且 `Is ErrHandoffEnvelopeRejected` → 返回零值结果 + `fmt.Errorf("%w: %w", ErrDutyVerificationHandoffRejected, handoffErr)`；否则 → `rederivationUndecided(DutyVerificationHandoffUnavailable)`（新 reason 进 `dependencyFailure()` 真集，`String()` 为 `DUTY_VERIFICATION_HANDOFF_UNAVAILABLE`）；(d) `adapters/settlementaccounting/receive_on_adopted_funds_fact.go` `HandleAdoptedExternalFundsFact` 对重派返回的错误 `Is ccapplication.ErrDutyVerificationHandoffRejected` → 包成 `ccsettlement.ErrDutyVerificationHandoffRejected` 交出；未决半边走既有 `rederivationConsumption` → `ErrDutyVerificationRederivationUndecided`。`cmd/parcel-dispatch/assemble.go` `externalFundsFactUndecidedSentinels` **名单本体一行未改**，diff 只有头注补「为什么新哨兵不在」。

**裁决 3——人重核路不动**：`handOffVerification` 仍吞错、仍返回 `"CONT-DUTY-VERIFICATION/" + 范围 + "/" + 指纹前八位`，`VerifyPayment` 的 outcome 仍 `DutyVerificationFormed`；既有 `reconcile_duty_payment_test.go` `TestAFailedHandoffLeavesTheVerificationFormedWithAContinuationReference` 只多一条 `HandoffError()` `Is` 替身错误的断言。(2′) 真库格顺手把同一条超长范围在人重核路上「形成 + 续办引用 + 原始错误可见」断了出来。

**裁决 4——见 (4)**：SA 采用编排、`duty_payment_verification_consumer.go`、迁移零 diff；幂等由 `duty_payment_verification_adoption` 既有五维主键承担，真库格钉住。

### 判断项（读代码写它实际怎么做的）

1. **维间分隔取 `\x00` 而不是 `/`**：引用本身含 `/`（`SYN-DUTY-RD/v1`），用 `/` 拼维会让「a/b + c」与「a + b/c」哈希前同串；`\x00` 不会出现在任何合法引用里（`Envelope.Validate` 拒控制符是对 ID 说的，这里是哈希输入不是 ID）。代码头注没写这一句，这是接手方的读法。测试侧 `fingerprintedEventID` 有意复述公式而不调生产那只未导出函数（头注原话：生产改了公式、这里就红）。
2. **口名前缀与分区键口名段的关系**：duty-payment 的前缀与 `dutyPaymentVerificationPartitionKey` 的口名段同词；gate 与 disposition 两只的分区键本来不带口名段，前缀是新词。三个前缀互异，`EnqueueOnce` 按（source, event_id）查重、三只口共用 `ccEventSource`，前缀让三只的 ID 空间不相交。
3. **`Subject` / `PartitionKey` 没改、也能超上限——裁决 1 让量的那一格**：上一会话没哈希、也没留理由句；(2′) 真库格经 `NewDecisionScopeReference` 造出 `"SYN-UNIT-LONG-" + 512 个 x` 且用例 PASS（构造门没把它挡在 512 以内），并拿到真实的 `ErrInvalidEnvelope`，所以 **Subject（范围 / 税费）今天确实能超 `MaxSubjectLength`**，同一条范围也会让 PartitionKey（租户 / 口名段 / 范围）超 `MaxPartitionKeyLength`。后果按裁决 2：人重核路折成续办引用（有读者）、重派路整笔硬失败 `publish_failed`（人动手），**不再静默**——但对引用长到那个程度的租户实例，交接会确定性失败直到有人改引用或改形。要不要把两者也哈希（分区键哈希不改「同范围同分区」）归 CC owner 一句或另票；这里只把「能超」量实。
4. **`ErrHandoffEnvelopeRejected` 的包裹只加在 duty-payment 一口**：`gate_verification_handoff.go` / `verification_handoff.go` 的 `EnqueueOnce` 错误仍原样返回，因为它们不在重派路上、今天没有调用方按它分格；日后哪条编排要分格，得在那一口补同一行 `errors.Is(err, eventing.ErrInvalidEnvelope)`。
5. **分格判据是「`Is ErrHandoffEnvelopeRejected`」，不是「是否确定性」**：重派路上凡不是它的交接错误一律归 `DutyVerificationHandoffUnavailable` 重投——包括 `HandOffDutyPaymentVerification` 对键维空白 / 核对时刻缺席那类装配缺陷错误（同样是确定性的）。实际到不了：重派路的键由编排从核对记录组、核对时刻由 `VerifyPayment` 取时钟；写下来是让下一个往交接口加确定性校验的人知道要顺手包一层。
6. **硬失败的结果形是零值 + 错误**：`RederiveDutyVerificationsOnFundsFactVersion` 返回 `DutyVerificationRederivationResult{}`（`Outcome()` 为 `DutyVerificationRederivationOutcomeInvalid`）与非 nil 错误，调用方先看错误；格的次序（`dependencyFailure` → `HandoffError` → 记谱系）保证带交接错误的 `Formed` 谱系不进 `Lineages()`——与头注「已形成的谱系会随之回滚，不把它们当成功报出去」一致；`Existing` 谱系从不带 `handoffErr`（`handOffVerification` 只在 `saved == CaseConfigurationRegistered` 时调）。
7. **替身层证不到「零核对行」**：`rederiveStores` / `newDutyStore` 没有事务，交接失败后核对行留在替身里，用例只断言 `Lineages()` 空；消费侧那格第二幕因此换 v3 到达（v2 已在替身登记册里、再投答 `已存在` 不触发重派——用例注释原话）。行级「同生同灭」只在 (2′) 真库格成立，那里 `inboxconsume` 的事务包着接收 + 重派 + 交接。
8. **硬失败到「人动手」之间只证了第一拍**：(2′) 断言 `published == 0` 与 `failure_code == dispatch.publish_failed`；失败预算耗尽、落账、告警那一段归派发器既有行为（`dispatcher.go` 头注），本票没加用例。
9. **与裁决字面不符、以代码为准**：裁决 2「结果对象加一格或另一返回值」→ 两样各做了一半：对外是结果对象加 `HandoffError()`，对内 `handOffVerification` 改成 `(string, error)`；裁决 1「`Subject` / `PartitionKey` 能超则同法取哈希」→ 能超（判断项 ③）但没哈希，改由裁决 2 (b) 兜成响亮硬失败。

### 验证

见 (5)：`gofmt` / `go vet` / grep / 迁移零 diff 为本会话实测于 `mcp4-sacc29` 工作树（代码 = `b2229462`）；带 DSN 测试取上一会话释号广播的数（PASS 886 / FAIL 0 / SKIP 0，退 0），本会话未重跑。**清点预报**：本会话在 `mcp4-sacc29` 工作树对 `b2229462` 跑 `tools/mechanism-inventory`（`-out` 到 `%TEMP%`，不落树）与已提交的 `docs/product/MECHANISM-INVENTORY.md` **零差**——三笔无增删文件、无迁移、端口与端点声明数不变，推送方在 tip 重生成预期同样零差。

### 能力边界

读了：本票全文含裁决 0–7；`git diff cbffc269 b2229462` 全部 16 件逐 hunk；19 完成记录（格式与判断项 ③ ④ 的出处）。**没读**：`eventing/envelope.go` `Validate` 正文与 `ErrInvalidEnvelope` 的定义（`errors.Is` 能接上由 (2′) 真库格证，不是读出来的）；`outboxintent.EnqueueOnce` 正文；`inboxconsume/consume.go`；SA `duty_payment_verification_consumer.go` 与采用编排本体；`dispatcher.go` 预算耗尽路径；`0016`–`0023` 正文（零 diff 只按 `--stat` 认）。没写代码、没改测试、没重跑带 DSN 测试、没改 `spec.md` / `tasks.md`。

## 参照

[19](19-cc-new-funds-fact-version-forms-a-new-verification-version.md) Comments「评审 ← 通道 6」Spec ②、「完成记录」判断项 ③ ④；[05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md)；`internal/platform/dispatch/dispatcher.go` 失败码头注；`docs/agents/parallel-sessions.md`「不同的『绿』在输出上长着同一张脸」；AGENTS「只实现已确认规则」「敏感实例外置」。

## Comments

- 2026-09-15 12:4x · 通道 1：立票（19 评审 Spec ② 与判断项 ③ ④ 转记，推送方处置时点名「归 CC owner 立票」）。只写票面，未动代码。**能力边界**：读了 `dutyPaymentVerificationEventID` / `handOffVerification` 的续办引用一行、`dispatcher.go` 失败码头注、19 评审与完成记录；**没读**框架 eventing 的 ID 长度校验源码（上限值与符号名以 19 原作者实测转记，作者开工第一步先量）、SA 消费门去重实现、三只 handoff 文件正文。
