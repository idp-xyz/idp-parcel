# CC 其余五只交接信封 ID（`customsCaseEventID` / `manifestEventID` / `declarationSubmissionEventID` / `caseClosureEventID` / `externalResultEventID`）仍是引用串接形——与 29 修掉的三只同一潜伏缺口：真长度引用下超 `eventing.MaxEventIDLength` 被确定性拒

Category: bug
Status: draft——**2026-09-15 14:4x 通道 1 立票**（sa-cc/29 评审 ← 通道 6 Spec ④ 转记；归 CC owner）。只写票面未动代码；取证锚第六批 tip `de822820`
Blocked by: 无（[29](29-cc-handoff-envelope-id-exceeds-framework-limit-and-rederive-path-swallows-handoff-failure.md) 已进 main——`fingerprintEventID` helper 已在 `internal/customscompliance/adapters/postgres/external_result_handoff.go`，本票只是让其余五只也走它）

## 缺口（评审 ← 通道 6 钉 `b2229462`，逐符号名；推送方未复量）

- 29 把 `dutyPaymentVerificationEventID` / `gateVerificationEventID` / `verificationEventID` 三只改成 `fingerprintEventID(口名前缀, 维…)`（口名前缀 + `sha256` 十六进制，定长 ≤ 128）。同包里 `customsCaseEventID`（五个引用串接）、`manifestEventID`、`declarationSubmissionEventID`、`caseClosureEventID`、`externalResultEventID` **仍是串接形**——ID 长度随租户 / 案件 / 申报等引用的真长度增长，那些是实例半边，本仓给不出上界（29 裁决 1 的同一句理由）。
- 29 裁决 1 的字面只裁了「三只核对交接口」；这五只当时不在 19 量出来的路上，所以不在 29 的地盘，评审把它记为「同一张脸的余数」。
- 这五只的**失败响法**各是什么（折续办 / 翻结果 / 硬失败）本票立票时**没读**——它们不都在重派路上，29 裁决 2 的两格不能照抄，作者开工先逐只量。

## 做法候选（只列不选，归 CC owner）

1. **五只一次同改** `fingerprintEventID`，各给口名前缀常量、维度全进哈希；载荷 / `Subject` / `PartitionKey` 不动。要量：每只的消费方按信封 ID 去重时换形前后同一事实会被当两封——各消费方是否有 ID 之外的幂等键。
2. **只改今天能从生产到达的那几只**，其余等它们的写面接上再改——省的是当下的量，欠的是「同一张脸再长几回」。

## 红线

- 不改载荷与可读形；不改迁移；不改任何消费方语义。
- 实例半边不写死。
- 与 [33](33-sixth-batch-review-standards-tails-test-currency-code-count-words-sentinel-count-and-unanchored-byte-claim.md) 同碰 `external_result_handoff.go`（33 只改一句头注），先后进即可。

## 完成判据（待裁后写实）

1. 五只 ID 定长且 ≤ `eventing.MaxEventIDLength`，超长合成引用下仍入队成功；同输入同 ID。
2. 每只的既有用例改断言形（定长、可重算），不断言字面串接。
3. 带 DSN CC postgres + 各自生产调用方所在 `cmd/` 包全 PASS 非 SKIP；清点零差。

## 地盘

`internal/customscompliance/adapters/postgres/` 五只 `*EventID` 所在文件与测试；各消费方只读不改（若要改幂等键另票）。

## 参照

[29](29-cc-handoff-envelope-id-exceeds-framework-limit-and-rederive-path-swallows-handoff-failure.md) 裁决 0 / 1 与完成记录「裁决 1——helper 与三只口」、Comments「评审 ← 通道 6」Spec ④；[32](32-sa-external-funds-fact-envelope-id-has-no-length-bound-and-a-rejected-envelope-folds-into-a-continuation.md)（SA 侧同款）。

## Comments

- 2026-09-15 14:4x · 通道 1：立票（29 评审 Spec ④ 转记）。只写票面，未动代码。**能力边界**：五只符号名取自评审原文，推送方**未读**这五只的定义、调用方与失败响法；作者开工第一步在 `de822820` 逐只量。
