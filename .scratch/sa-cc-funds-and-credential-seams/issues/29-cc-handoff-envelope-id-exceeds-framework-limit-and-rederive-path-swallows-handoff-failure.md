# CC 三只交接信封的 ID 是「租户 / 类型 / 范围 / 税费 / 资金 / 64 位指纹」串接，真长度引用下超过框架信封 ID 上限、`EnqueueOnce` 拒收；而 `handOffVerification` 把交接失败折成续办引用、核对不翻，重派路上那个续办引用无人读——核对行提交、信封没发、消费入账成功、无人知道

Category: bug
Status: ready-for-agent——**2026-09-15 12:5x 通道 1 按用户「代裁」代裁（CC owner 口径），四条「要裁的」写入下方「裁决」节**：三只 `*EventID` 一次同改成定长指纹形（固定口名前缀 + `sha256` 十六进制，必在 `eventing.MaxEventIDLength` 内），载荷五维全量照旧；重派路上交接失败不再折成续办引用——依赖不可用归未决重投、信封不合法归硬失败，两格都整笔回滚；人重核路 05 的兜底本票不动；SA 侧靠采用登记册五维主键守幂等、真库用例钉住。上限已量实：`go.idp.xyz/idp-bento-go/eventing` `MaxEventIDLength = 128`，`Envelope.Validate` 对 `id` 查非空 / UTF-8 / 无控制符 / ≤ 128 字节。此前 draft——2026-09-15 12:4x 通道 1 立票（sa-cc/19 评审 ← 通道 6 Spec ② + 19 完成记录判断项 ③ ④ 转记；原作者会话 11:1x 广播量到 138 字节被拒是第一手）。只写票面未动代码；取证锚 main `36fb5437`
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

## 参照

[19](19-cc-new-funds-fact-version-forms-a-new-verification-version.md) Comments「评审 ← 通道 6」Spec ②、「完成记录」判断项 ③ ④；[05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md)；`internal/platform/dispatch/dispatcher.go` 失败码头注；`docs/agents/parallel-sessions.md`「不同的『绿』在输出上长着同一张脸」；AGENTS「只实现已确认规则」「敏感实例外置」。

## Comments

- 2026-09-15 12:4x · 通道 1：立票（19 评审 Spec ② 与判断项 ③ ④ 转记，推送方处置时点名「归 CC owner 立票」）。只写票面，未动代码。**能力边界**：读了 `dutyPaymentVerificationEventID` / `handOffVerification` 的续办引用一行、`dispatcher.go` 失败码头注、19 评审与完成记录；**没读**框架 eventing 的 ID 长度校验源码（上限值与符号名以 19 原作者实测转记，作者开工第一步先量）、SA 消费门去重实现、三只 handoff 文件正文。
