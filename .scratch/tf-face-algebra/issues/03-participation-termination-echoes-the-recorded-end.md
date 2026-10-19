# 03 终止参与：「已离场」不回显在册终点，并发输家不读回就答「已离场」，坏命令随参与状态改答——异依据重投、被别处结束与原样重放分不出

Category: bug
Status: needs-triage——2026-10-18 会话 `idp-parcel-9b` 立（操作者与集成客户端渠道逐口核代数研判；实测发现未经对抗核验，裁定待做，动手前先复现）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：逐口核研判（workflow review26-face-algebra-sweep）：/transport-fulfillment-participation-terminations
地盘：
- internal/transportfulfillment/application/end_fulfillment_participation.go 及其测试
- internal/transportfulfillment/adapters/http/terminate_fulfillment_participation.go 及其测试
- cmd/parcel-api/（新增真渠道真库回归测试文件）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 现象
1. 已离场之后，下列请求一律答 200 `PARTICIPATION_ALREADY_ENDED`，响应体逐字节相同：原样重放、异依据、异终止时刻、依据与时刻都空、终止早于入场、参与已被下一次权威交接结束。
   - 响应只带命令里的段与对象，不带在册终点的种类、依据和时刻。
   - 全仓也没有读面透出参与终点：`ReviewCatalogue` 与 `/transport-fulfillment-records` 的白名单里都没有段或参与。
2. 并发输家：终止读完段、还没做窄口更新时，另一事务把链尾更正替代掉（仍是已交接，产生替代版本，终点为空）。终止的更新命中 0 行，答 200 `PARTICIPATION_ALREADY_ENDED`。可此时库面两行的终点都为空，参与其实没有结束；无竞争再投一次，才答 201。
3. 坏命令随参与状态改答：只缺依据、依据全空白、只缺时刻、终止早于入场，落在已离场的参与上答「已离场」，落在在场的参与上答 `INPUT_NOT_ACCEPTED`。
4. 链尾被交接更正成失效版本后，同一终止重放改答 `OBJECT_NOT_IN_SEGMENT`；再更正回已交接，又答「已离场」。原研判对这一格只读了码，这次已实测。

## 复现（S）
命令：在 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/wt-decide2 下执行 `IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' go test -overlay <overlay> -count=1 -p 1 -run '<用例>' -v ./cmd/parcel-api/`。

探针（overlay 均在各自目录内）：
- /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/transport-fulfillment-participation-terminations/zz_probe_test.go 的 `TestProbeDecide2TerminationReplayAndSameIdentityDifferentContent`、`TestProbeDecide2TerminationAfterAnotherSourceEndedIt`（日志 run-final.log）。
- 同目录 adversary/zz_adv_probe_test.go 的 `TestProbeAdvPTReplayLeavesEveryTableAndSequenceUntouched`、`TestProbeAdvPTConcurrentLoserBehindASupersedingCorrection`、`TestProbeAdvPTReplayAfterTheEntryHandoverIsCorrectedAway`（日志 run-adv2.log）。
- 同目录 impl-verify/zz_implverify_test.go 的 `TestImplVerifyPTConcurrentLoserBehindACorrectionOfTheChainTail`、`TestImplVerifyPTReplayUnderTheProductionOperatorRegister`。后者用生产 postgres 操作者册，不落库按全库逐表内容、xmin 与序列取证（日志 run-iv3.log）。
- 同目录 verify-authority/zz_verify_test.go 的 `TestVerifyAuthorityMalformedTerminationsAnswerByParticipationState`、`TestVerifyAuthorityConcurrentTerminationsWithDifferentBases`（日志 run-verify-final.log）。

## 应当
照票 20 对关段的先例，原话是：「代价在于……异时刻重投的调用方看不到原时刻——补上回显」。
- 已离场的答复带在册终点的种类、依据和时刻，调用方能自己分出三种情形：同一次终止的重投、异依据或异时刻、被交接或交付结束。
- 并发输家读回之后再作答，绝不在没读回时宣称已离场。
- 坏命令不论参与状态，一律答 `INPUT_NOT_ACCEPTED`。

## 根因
- `EndFulfillmentParticipationResult` 没有终点字段。`participationTerminationAnswer` 回显的是命令里的段与对象，不是在册的那一份。
- `End` 里 `case ports.ParticipationAlreadyEnded:` 直接答已离场，没有读回。窄口更新命中 0 行，可能是因为别人已结束，也可能是因为链尾刚被替代，两种情况它分不开。
- `End` 里「已离场」那一支排在 `applyEnd` 校验依据与时刻之前。

## 修复方向（机制）
1. `EndFulfillmentParticipationResult` 在已离场时带上在册终点的种类、依据和时刻，取自读回链尾的 `FulfillmentParticipation.End`。
2. 并发输家照 `OpenDispatchTaskHandler.Open` 与关段并发输家的先例，先读回再作答：
   - 读回链尾已有终点：答已离场，并回显终点；
   - 读回链尾仍在场（刚被替代）：在新链尾上重走一次，仍不成就答`未决`，带续办；
   - 读回不通：答`未决`。
3. 依据与时刻的成形校验挪到读段之前，放在 `participationEndTargetFrom` 或 `End` 开头。
4. HTTP 的 `participationTerminationResponse` 加 `endKind` / `endBasis` / `endedAt` 三格；时刻经票 31 收拢的格式化助手输出，即先过 `AtRegistryPrecision`，再按 `RFC3339Nano`。
5. 备选（归 TF owner）：如果裁定依据是判断语义（类比票 20 对承运商判断的裁定），就改为补 TFC-1 形状 `END_FULFILLMENT_PARTICIPATION`：
   - 内容为依据和按登记册精度的终止时刻，两侧都从在册链尾现算；
   - 只在在册终点种类为 `CONTROL_TERMINATED` 时比较；
   - 异内容答新格 `CONTENT_CONFLICT`，并回显终点。

   默认取回显这一支，即照关段的先例。

## 不做
- 链尾失效后重放答 `OBJECT_NOT_IN_SEGMENT`：这是 `End` 里 `current.Voided()` 先判的结果，去留归 TF owner。
- 与交接更正并发时终点丢失：归 〈终止与更正并发丢终点〉。
- 凭实际承运商首次有效收寄入场的段读不回：归 〈第三入场词读回〉。

## 是否潜伏
已可触发。生产装配上，端点行直接挂 `operatorDecisions.transport` 与 `segmentOps.enderOf`，不问授权映射。演示租户的配置员授予、对照和区间都已配齐，`1777a0ec` 的取证里有「配置员终止参与 200 SEGMENT_NOT_FOUND」。

## 完成判据
- [ ] 编排单测：
  - 已离场的答复带终点三件；
  - `ports.ParticipationAlreadyEnded` 支会读回链尾：有终点答已离场并回显，仍在场不答已离场；
  - 坏命令在已离场与在场的参与上都答 `INPUT_NOT_ACCEPTED`。
- [ ] HTTP 单测：答复体含 `endKind`、`endBasis`、`endedAt`；整秒值与今天的格式逐字节相同。
- [ ] 真渠道真库回归（`cmd/parcel-api` 新测试文件，用 `buildOperatorDecisionIntakes`、生产准入桥、`buildSegmentOperations`）：
  - 首次终止答 201；
  - 原样重放、异依据、异时刻各答 200 已离场，回显在册那一份，依据仍是首投的；
  - 参与先被下一次交接结束时，终止答已离场，`endKind` 为下一次交接；
  - 坏命令在已离场参与上答 200 `INPUT_NOT_ACCEPTED`；
  - 全库表（含 `bento.outbox`）的内容与序列都不变。
- [ ] 并发真库回归：用钩子确定性复现「读段之后、窄口更新之前，另一事务把链尾更正替代」，做法照 `TestProbeAdvPTConcurrentLoserBehindASupersedingCorrection`。终止不答已离场，事后库面链尾有终点，或答复为`未决`。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退出码 0，`gofmt -l .` 无输出；带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...` 全部 ok。
- [ ] 在票 15 补记下追加回写行（只加行）。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：作者自验，非作者评审（两轴分开），推送方在干净检出上带 DSN 跑全量。
