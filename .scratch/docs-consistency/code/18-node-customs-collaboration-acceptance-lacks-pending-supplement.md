# 18 NO 节点监管协作承接决定缺「待补充」一格

Category: bug
Status: ready-for-agent——R3（不急）：承接编排 `application/accept_collaboration.go` 在 `cmd/` 下没有装配，今天无生产路径；接线之前必须先修，否则资料不足只能被说成拒接或部分承接。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
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
