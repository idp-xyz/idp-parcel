# 35 PS 取消与边界事实的先后冲突：面单渠道一侧不比时间、冲突没有解开的决定面

Category: enhancement
Status: needs-triage——建议 R2（上线后尽快）：看得见的卡住。网络服务一侧，取消已提交之后才到、发生不晚于取消的收寄，采用停在未决 `CANCELLATION_ORDER_CONFLICT`，此后没有任何口能解开它，包裹在节点手里却没有责任起点，也不触发收寄后的路由复核；面单渠道服务一侧，同一情形今天答 `CANCELLATION_STANDS`，客户被告知已取消、承运商却已收走。今天取消授权停在未配置，取消形成不了，两种情形都还到不了；租户一登记取消授权即生效。2026-10-21 文档自洽 06 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc06。分级由协调方定稿。
Blocked by: 无（面单渠道提交边界那一半与 [code/21](./21-label-channel-parcel-cancellation-ignores-carrier-pickup-and-finals.md) 同一编排，宜同批或排在其后）
出处：[audit/ps.md](../audit/ps.md) PS-03（B），代决 [ADR-0254](../../../docs/adr/0254-cancellation-and-boundary-fact-order-is-judged-by-their-business-times-and-a-late-earlier-boundary-fact-is-a-fact-conflict.md)；UC-PS-003「给开发的交接」、UC-PS-004「给开发的交接」`AT-PS-108` 一行、UC-PS-006「给开发的交接」事实冲突一行。

## 现象

ADR-0254 定了取消与边界事实按两者的业务时间比、晚到的更早边界事实是事实冲突。代码在网络服务一侧已经这样判，但冲突之后无路可走；面单渠道一侧还不比时间。

## 证据

- 网络服务：`internal/parcelshipment/application/adopt_network_intake.go` 的 `AdoptNetworkIntakeHandler.Handle` 读到已成立的取消，`cancellation.RequestedAt()` 早于 `source.OccurredAt()` 即不采用（`CANCELLED_BEFORE_INTAKE/<取消标识>`），否则答 `ELIGIBILITY_UNDECIDED`、原因 `CANCELLATION_ORDER_CONFLICT`。注释写「收寄发生早于取消决定」，比的却是取消的业务时间。
- 冲突之后：没有命令口、运营决定面或续办信封能把这一格解开；同一来源再投仍落在同一格。
- 面单渠道：`internal/parcelshipment/domain/label_service_final.go` 的 `JudgeLabelServiceFinal` 在 `input.CancellationStands` 为真时直接答 `CANCELLATION_STANDS`；`application/judge_label_service_final.go` 只读「有没有有效取消」，不读取消的业务时间，也不比承运商首次有效收寄的发生时间。
- 文档：PS CONTEXT「形成取消决定前……」一条，UC-PS-003 `AT-PS-107`，UC-PS-004 `AT-PS-095`、`AT-PS-108`，UC-PS-006 决定矩阵。

## 三类

机制。产品策略：解开冲突的决定形状（谁有权、去向有哪几个）由本票落地时按 ADR-0254 决定三提出、交 PS owner 认可。租户取值：谁持有这个授权，属 `PAR-COM-14` 一类，未登记答未配置。

## 规模

M：面单渠道终局判断多比一次时间并多一格未决原因；一个授权决定面（命令、授权、只追加记录、续办信封）；收寄采用那段注释改口。

## 最小的诚实修法

1. 面单服务终局判断读到有效取消时，读出取消的业务时间，与承运商首次有效收寄的发生时间比：收寄晚于取消，照旧 `CANCELLATION_STANDS`；不晚于取消，答 `JUDGMENT_UNDECIDED` 并带一格新原因（例如 `CANCELLATION_ORDER_CONFLICT`），不形成终局。
2. 立一个授权决定面解开冲突，去向至少有：认定取消依据不成立、包裹转入收寄后服务处置（随后重驱收寄采用或终局判断）；认定边界事实有误、交来源方更正。决定只追加，不改写已提交的取消。
3. `adopt_network_intake.go` 那段注释的「取消决定」改为「取消的业务时间」。
4. 补用例：网络服务冲突后经决定解开；面单渠道收寄不晚于取消停在未决（`AT-PS-108`）。

## 要裁的

- 解开决定的去向集合与授权：本票落地时提出，交 PS owner 认可（ADR-0254 越权风险点 2）。
