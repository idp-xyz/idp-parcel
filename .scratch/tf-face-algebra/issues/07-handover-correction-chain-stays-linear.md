# 07 交接更正链不线性：同一前版能被更正成两个新版本，范围汇总把一个对象数两次

Category: bug
Status: in-progress——2026-10-18 完工；评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-18 作者子代理完工，待评审。此前 in-progress——2026-10-18 会话 `idp-parcel-9b` 代决并认领（分支 `claude-oc41tfa07`，与 operator-channel/41 同一分支，排在其后）。此前 needs-triage——2026-10-18 会话 `idp-parcel-9b` 立（操作者与集成客户端渠道逐口核代数研判；实测发现未经对抗核验，裁定待做，动手前先复现）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：逐口核研判（workflow review26-face-algebra-sweep）：/transport-fulfillment/handover-corrections
地盘：
- migrations/transport_fulfillment/（新迁移，序号取落地时的下一个）
- internal/transportfulfillment/adapters/postgres/transport_handover_registry.go 及其测试
- internal/transportfulfillment/application/register_transport_handover.go（撞非主键索引时的读回）及其测试
- cmd/parcel-api/（新增真库回归测试文件）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 裁定（2026-10-18，会话 idp-parcel-9b 代决）

照「修复方向」三条做：链必须线性，一版最多被更正一次，照揽收更正的先例（迁移 0015 与票 tf-segment-lifecycle-closure/08）。编排先判「前版已有后继」，同内容答已有版本、异内容答冲突，比对用 operator-channel/41 交接更正那一半立起来的那套；库面部分唯一索引只作兜底，建索引前先查存量分叉，有就如实报出、不静默删。迁移取 `migrations/transport_fulfillment/` 下一个空号。属机制。

## 现象
首登 v1，更正 v1→v2，再以 v1 作前版更正成 v2-retry（换放行证据）：两次更正都答 201 `HANDOVER_CORRECTED`，链在 v1 处分叉。用生产 `TransportHandovers.ListByScope` 读该范围，再交 `domain.SummarizeHandovers` 汇总，得 handedOver=2、total=2；而该范围只有一个对象。

## 复现（S）
在 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/wt-decide2 下执行：
`IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' go test -overlay /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/handover-corrections/overlay_v2.json -count=1 -p 1 -run 'TestProbeDecide2HandoverCorrectionV2$' -v ./cmd/parcel-api/`
用例是 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/handover-corrections/zz_probe_v2_test.go 的 F 段。叠上候选修复（verify-amend/overlay_fix_and_probe.json）之后，这一段照样是 201、201，handedOver=2：〈交接更正冲突〉收不住这一格。

## 应当
链线性，同一前版最多被更正一次。揽收更正有先例：迁移 0015 头注「一版最多被更正一次」，票 tf-segment-lifecycle-closure/08 写「链必须线性（一版最多被更正一次，库内索引兜底）；指名已被更正过的版本，同内容答已有版本、异内容答冲突」。

## 根因
- `0005_pickup_handover_registries_and_delivery_attempt.sql` 只有主键 `transport_handover_pkey (tenant_id, object_ref, scope_ref, handover_version)`，`corrects_version` 上没有唯一约束。
- `Correct` 不检查前版是否已被更正过。

## 修复方向（机制，待 TF owner 裁）
1. 新迁移给 `transport_fulfillment.transport_handover` 加部分唯一索引「（tenant_id, object_ref, scope_ref, corrects_version）WHERE corrects_version IS NOT NULL」，写法照 0015。建索引前先查存量有没有分叉；有就如实报出，不静默删。
2. `Save` 用的是 `ON CONFLICT DO NOTHING`，撞上这条非主键索引时同样答「已登记」。这时 `commit` 按本次新键读回会读空，所以要改成按（前版）读回那份后继，再用〈交接更正冲突〉那套比对分格：同内容答已有版本，异内容答冲突。
3. 编排层先判「前版已有后继」，直接走第 2 条的分格，库面索引只作兜底。

## 是否潜伏
已可触发。生产渠道与〈交接更正冲突〉相同。

## 完成判据
- [ ] 迁移：在干净库上 `migrate` 顺序跑通；存量检查语句与结果写进完成记录。
- [ ] 编排单测：对已有后继的前版再更正——同内容答 `EXISTING_VERSION`，异内容答 `SOURCE_CONFLICT`。
- [ ] 真库回归（`cmd/parcel-api` 新测试文件，生产作业事实 Intake + `buildControlFactOrchestrations`）：v1→v2 之后再投 v1→v2-retry，答 `SOURCE_CONFLICT`；`SummarizeHandovers` 对单对象范围得 total=1；两份同前版的更正并发，恰好一份 201。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退出码 0，`gofmt -l .` 无输出；带 DSN 跑 `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/...` 全部 ok。
- [ ] 票 15 补记下追加回写行（只加行）。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：作者自验，非作者评审（两轴分开），推送方在干净检出上带 DSN 跑全量。

## 完成记录（2026-10-18，会话 idp-parcel-9b 的实施子代理，分支 claude-oc41tfa07）

提交：`c4ee61e5`；机制清点 `d3d13eb5`（与 operator-channel/41 共用）。迁移号：`0022_transport_handover_corrects_once.sql`（fetch 后核过 origin/main 与在途分支，0022 无人用）。

**落点**
- 端口 `TransportHandoverRegistry` 加 `FindSuccessor`（按前版的键交回回指它的那一版）；生产适配器实现，HTTP 测试替身补桩。
- `RegisterTransportHandoverHandler.Correct` 在领域 `Correct` 与摘要之后先判「前版已有后继」，经 operator-channel/41 的 `answerOccupiedKey` 分格；新版本不进摘要，换新版本号原样重投仍是重放，交回已落下的后继。
- `commit` 撞键读回改走 `findWinner`：按新键读空且本记录是更正时，按前版读回先到的后继再分格。
- 迁移 0022：部分唯一索引（tenant_id, object_ref, scope_ref, corrects_version）WHERE corrects_version IS NOT NULL，写法照 0015/0021；之前的 DO 块查存量分叉，有就 `RAISE EXCEPTION` 列出「租户/对象/范围 的 前版（后继 …）」，不删任何行。

**各格对应用例**
- 编排：`TestAPredecessorThatAlreadyHasASuccessorIsNotCorrectedAgain`（同内容答 `EXISTING_VERSION` 并交回 v2，异内容答 `SOURCE_CONFLICT`）；`TestACorrectionThatLostTheRaceForThePredecessorIsComparedWithTheWinner`（撞索引后按前版读回）。
- 库面：`TestAPredecessorIsCorrectedAtMostOnce`（FindSuccessor 读回；第二个后继 Save 答已登记）。
- 真库：`TestTheWiredHandoverCorrectionChainStaysLinearAgainstARealDatabase`（v1→v2 后 v1→v2-retry 答冲突；`ListByScope` + `SummarizeHandovers` 得 total=1、handedOver=1；同一前版两份并发更正六轮，每轮恰好一份 `HANDOVER_CORRECTED`、一份冲突）。

**先红后绿**：真库用例先在未改代码上跑红——v1→v2-retry 答 201 `HANDOVER_CORRECTED`，复现票面现象；编排用例在替身按索引兜底后先红（答 `HANDOVER_UNDECIDED`）；库面用例在挪走 0022 时红（第二个后继落下）。实现后全绿。

**存量检查**：检查语句即 0022 的 DO 块（按 tenant_id, object_ref, scope_ref, corrects_version 分组，HAVING count(*) > 1）。干净库上为空，迁移顺序跑通（`pgtest.Pool` 每例全量施加）。另用一次性探针在已迁移库上删掉索引、植入 v1→v2、v1→v3 两个后继，再执行 0022：整笔失败，报「SYN-T/SYN-O/SYN-S 的 v1（后继 v2、v3）」，索引未建、三行都在。探针跑完即删，没有提交。

**没做或没单独复现的格**：汇总 handedOver=2 没单独观察到（未改代码时用例停在前一步的 201）；并发两份更正在未改代码下没单独跑红。票 15 补记回写行在地盘外，没写。

**门**：`gofmt -l` 无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN `go test -p 1 -count=1 ./internal/transportfulfillment/... ./cmd/parcel-api/... ./internal/architecture/... ./migrations/...` 全 ok。

证据层级：`S`。

评审：待会话读 diff 审。

## Comments

- **评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审）· 钉 `b9cb5c9a`（基线 `4e000051`）· 2026-10-18**：用户选「精简续做」，评审由会话本身读 diff 做；代码出自本会话起的实施子代理，不算非作者评审，如实记下。
  - **Spec**：阻断无。编排先判前版已有后继（新端口 `FindSuccessor`），同内容答已有版本、异内容答冲突；迁移 0022 加 `(tenant_id, object_ref, scope_ref, corrects_version) WHERE corrects_version IS NOT NULL` 部分唯一索引兜底，存量分叉整笔失败并列出、不删行。
  - **Standards**：阻断无。
  - **未单独观察到的格**：范围汇总 handedOver=2 与并发两份更正的红没单独跑出，由链线性的用例与索引兜底覆盖。
