# 18 NO 节点监管协作承接决定缺「待补充」一格

Category: bug
Status: in-progress——2026-10-27 作者完工，待隔离评审与进 main，分支 claude-code18（代码最后一笔 4afedd064）。此前 in-progress——2026-10-27 会话 tops-20（idp-parcel-91）接手 tops-26 断在中途的「上线前必须」一组（冻结清单「六」），分支 claude-code18。此前 ready-for-agent——R3（不急）：承接编排 `application/accept_collaboration.go` 在 `cmd/` 下没有装配，今天无生产路径；接线之前必须先修，否则资料不足只能被说成拒接或部分承接。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/nr-no.md](../audit/nr-no.md) M5（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在）。

## 现象与证据

- 文档：NO CONTEXT「节点监管协作承接决定」：「决定可以是已承接、部分承接、待补充或拒绝」；规则「对象、动作、节点、授权依据或必要作业条件不足时保持待补充」；GLOSSARY 同名词条；UC-NO-001 结果「节点协作待补充」、`AT-NO-002`、`AT-NO-003`。
- 代码：`internal/nodeoperations/domain/customs_collaboration.go` 的 `AcceptanceDecisionKind` 注释「封闭三值：接受、拒接、部分承接」（`CollaborationAccepted` / `CollaborationDeclined` / `CollaborationPartiallyAccepted`）；`application/accept_collaboration.go` 的结果里也没有待补充。

## 三类

机制。无产品策略；必要作业条件的取值归租户节点登记，未登记答未配置。

## 规模

S：加一格「待补充」，必须带缺口（缺哪几件）与补充入口；补后的决定作为新决定回指它。

## 最小的诚实修法

`AcceptanceDecisionKind` 加「待补充」：带缺口清单，不形成承接；补充到位后形成新决定并回指，原决定不改写。

## 要裁的

无（CONTEXT 已定）。TF 侧同名的监管承接「待补充」属 B（[audit/tf.md](../audit/tf.md) L8），不在本票。

## 完成记录

起点 SHA：`2afdd0b1e`。分支 `claude-code18`。

改了什么：
- 领域：`AcceptanceDecisionKind` 加第四值 `CollaborationPendingSupplement`（`PENDING_SUPPLEMENT`）；新增封闭五值 `SupplementGapKind`（对象、动作、节点、授权依据、必要作业条件）。待补充必带至少一项缺口、不带承接范围；其余三值不得带缺口。`RecordExecutionFact` 对待补充答 `ErrCollaborationPendingSupplement`（与拒接、越权分格）。
- 载荷规范化：`CanonicalizeAcceptancePayload` 加缺口参数，`gaps` 带 `omitempty`——其余三值的 NOC-1 文档逐字节不变，已存摘要不失配。
- 编排：命令加 `Gaps`；结果加 `CollaborationPendingSupplement`、`ExecutionOnPendingSupplement` 两格。当前决定是待补充且来了不同内容的新决定时，开下一轮（`Round+1`）而不是冲突；已决定（非待补充）的事项仍冲突。
- 持久化与迁移 `0024`：加 `round`（存量为 1）与 `gaps` 列，主键扩为（租户+事项+轮次），决定形状 CHECK 加第四值。`FindByKey` 交回最新一轮；旧轮行不改写。
- 发布意图：事件 ID 与载荷带轮次，待补充与其后的新决定各交一份。
- 文档：UC-NO-001 状态行与 `AT-NO-003` 去掉「未实现」注记，换成现状（四格已实现、`cmd/` 尚未装配）。

接线：票面「最小的诚实修法」不含接线，状态行写的是「接线之前必须先修」，故未接；`cmd/` 下仍无 `AcceptCollaborationHandler` 装配。

代决与越权风险点：
1. 不立 ADR：票面「要裁的无」，轮次链是机制。但「同一事项只决定一次」的口径放宽为「待补充除外」，是对 `CollaborationAcceptanceKey` 注释的改写，评审请确认不需 ADR。
2. 「回指」以轮次隐式表达（第 n+1 轮回指第 n 轮，且第 n 轮必为待补充），库面不设跨行约束，由编排保证。
3. 缺口取五类封闭值，取自 CONTEXT 规则原文；缺口的具体取值（必要作业条件）归租户节点登记，本票不判，未登记答未配置。
4. UC 要求的「责任方」未单独建模：缺口类别决定去哪补（对象、动作、授权依据在事项所有方，节点、作业条件在节点登记），暂不另存。
5. 补充入口 = 同一协作事项再交一次承接（同 ADR-0265 的 TF 口径），非另造入口字段。
6. 待补充上的 `Basis` 可有可无（不强制）。

判据对用例：`AT-NO-003`（缺对象、动作、节点或授权依据→待补充，不创建执行事实）由领域与编排测试覆盖。`AT-NO-002`「P1 已承接、P2 待补充」一轮里表达不了：待补充整件待补、不带承接范围，见 ADR-0333 越权风险点 1，本票不覆盖该条。

自验（含 PG 55432）：`go build ./...`；`go vet` nodeoperations 与 cmd；`gofmt -l` 无输出；`go test -count=1 -p 1` 通过 `./internal/nodeoperations/...`、`./cmd/...`、`./internal/architecture/...`、`./migrations/...`；`at-coverage-counts -check` 无报。

留给协调方：CC 侧消费 `node-operations.collaboration-acceptance.decided` 的一方（若有，在 customscompliance 地盘，我未碰）今后会收到待补充意图，载荷新增 `round`，读当前轮次决定即可。

评审修订（隔离评审钉 `088d9fec2`）：轮次链是难逆转取舍，补 ADR-0333（会话代 NO owner 接受，待 owner 复核）；`internal/architecture/envelope_partition_gate_test.go` 对承接意图的例外理由改成轮次链下的现行理由，全仓「只决定一次」注释对上（`ports.go`、应用测试、迁移 0002 头注）。`AT-NO-002` 措辞按上改为整件待补充，不再称覆盖其 P2 半边。ADR 索引未重生成。
