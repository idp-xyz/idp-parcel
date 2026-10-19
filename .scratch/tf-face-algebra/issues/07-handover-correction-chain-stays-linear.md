# 07 交接更正链不线性：同一前版能被更正成两个新版本，范围汇总把一个对象数两次

Category: bug
Status: in-progress——2026-10-18 会话 `idp-parcel-9b` 代决并认领（分支 `claude-oc41tfa07`，与 operator-channel/41 同一分支，排在其后）。此前 needs-triage——2026-10-18 会话 `idp-parcel-9b` 立（操作者与集成客户端渠道逐口核代数研判；实测发现未经对抗核验，裁定待做，动手前先复现）
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
