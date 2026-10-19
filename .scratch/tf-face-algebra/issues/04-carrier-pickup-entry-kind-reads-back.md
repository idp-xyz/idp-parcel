# 04 凭实际承运商首次有效收寄入场的段按键读不回：`participationEntryKindFrom` 不认第三个入场词，段里一名这样的成员就让整段的终止、交接、揽收进段停在未决或 500

Category: bug
Status: needs-triage——2026-10-18 会话 `idp-parcel-9b` 立（操作者与集成客户端渠道逐口核代数研判；实测发现未经对抗核验，裁定待做，动手前先复现）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：逐口核研判（workflow review26-face-algebra-sweep）：/transport-fulfillment-participation-terminations, /transport-fulfillment-carrier-first-effective-pickup-judgments, /transport-fulfillment/handovers, /transport-fulfillment/offsite-pickups
地盘：
- internal/transportfulfillment/adapters/postgres/fulfillment_segment_registry.go
- internal/transportfulfillment/adapters/postgres/（新增真库往返测试文件）
- cmd/parcel-api/（新增真渠道回归测试文件）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 现象
- 领域 `EstablishSegmentWithCarrierPickup` 立段后，经生产仓储 `Save` 落库，结果为 `SegmentSaved`，库面参与行 `entry_kind=CARRIER_FIRST_EFFECTIVE_PICKUP`（迁移 0020 的 CHECK 放行）。但随后：
  - `FindByKey` 报「find fulfillment segment: unknown participation entry kind \"CARRIER_FIRST_EFFECTIVE_PICKUP\"」；
  - `FindActiveSegments` 却找得到这一段。
- 经生产承运商首次有效收寄判断口立段（先在 PC 登承运参与方），判断答 201 `PICKUP_FORMED`，同时带 `carrierJudgmentContinuationReference`：实际承运商判断那一半也读不回段。
- 在这类段上：
  - 终止答 200 `PARTICIPATION_END_UNDECIDED`，续办引用固定不变；重放还是同一格，永远形成不了。
  - 同一对象的下一次交接整笔回滚，答 500，错误为「end participation on handover: … the participation end did not settle」。
- 混段：A 凭揽收入场，B 经判断口加入同一段。之后终止 A 答 `PARTICIPATION_END_UNDECIDED`；另一对象凭揽收指名这一段，答复带 `segmentContinuationReference`，没有进段。也就是说，一名第三入场词的成员会拖垮整段。
- 读码可见，同样经 `FindByKey` 的关段、派送发起、参与重派生，在这类段上也会停住。这几项未逐一实测。

## 复现（S）
命令：在 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/wt-decide2 下执行
`IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' go test -overlay <overlay> -count=1 -p 1 -run '<用例>' -v ./cmd/parcel-api/`

探针：
- /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/transport-fulfillment-participation-terminations/zz_probe_test.go，用例 `TestProbeDecide2TerminationOnACarrierPickupSegment`、`TestProbeDecide2TerminationOnASegmentTheCarrierPickupPortEstablished`、`TestProbeDecide2NextHandoverOnACarrierPickupSegmentDirect`（同目录 overlay.json，日志 run-final.log）。
- 同目录 impl-verify/zz_implverify_test.go，用例 `TestImplVerifyPTJoiningACarrierPickupSegment`、`TestImplVerifyPTMixedSegmentPoisonedByOneCarrierPickupMember`（日志 run-iv3.log）。

## 应当
写进去的段，按键能读回来。凭第三入场词入场的参与，可以终止、可以被下一次交接结束，也可以与其他成员同段。

## 根因
`participationEntryKindFrom` 的 switch 只认 `EnteredByOffsitePickup` 与 `EnteredByTransportHandover`，其余一律报 `unknown participation entry kind`。

## 修复方向（机制）
1. `participationEntryKindFrom` 补上 `domain.EnteredByCarrierFirstEffectivePickup.String()` 这一格。
2. 同一文件里其余词表的译回（如 `participationEndKindFrom`），逐个核是否与领域 `String()` 及迁移 CHECK 同字，结果写进完成记录。
3. 补真库往返测试：第三入场词的首登版本、经 `Supersede` 写入的失效版本，写入后都要能经 `FindByKey` 读回。

## 是否潜伏
已可触发。演示配置员同时持有：
- 登记册配置写授予（能在 PC 登承运方）；
- `CARRIER_FIRST_EFFECTIVE_PICKUP_JUDGMENT` 授予，以及对应的准入对照与治理区间。

凭这些就能先登承运方、再判形成并立段。生产装配上这条路不问授权映射。

## 完成判据
- [ ] 真库往返（`adapters/postgres` 新测试文件）：第三入场词的首登段、失效版本，`Save` / `Supersede` 写入后，`FindByKey` 读回逐格等于写入。
- [ ] HTTP 真渠道回归（`cmd/parcel-api` 新测试文件；运营决定渠道、作业事实渠道都走生产 Intake，编排用生产装配）：
  - 经判断口立段后，终止该对象答 201 `PARTICIPATION_ENDED`；
  - 混段里终止 A 答 201；
  - 该对象的下一次交接答 201，且 `participationEnd=PARTICIPATION_ENDED`；
  - 另一对象揽收指名该段时进段：库面有行，答复不带段续办；
  - 判断口的答复不再因为读不回段而带 `carrierJudgmentContinuationReference`。
- [ ] 变异自检：撤掉新加的那一格，上述用例转红。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退出码 0，`gofmt -l .` 无输出；带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...` 全部 ok，`-v` 下 TF `adapters/postgres` 无 `--- SKIP`。
- [ ] 在 operator-channel/15 补记下追加回写行（只加行）。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：作者自验；非作者评审，两轴分开；推送方在干净检出上带 DSN 跑全量。
