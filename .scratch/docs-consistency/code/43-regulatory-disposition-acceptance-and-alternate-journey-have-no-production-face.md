# 43 TF 监管运输处置承接与备用/退运旅程启动没有生产入口

Category: enhancement
Status: needs-triage——R3（不急）：两条用例在首发主链之外（监管退运、备用伙伴切换、普通退运都按受控异常走），今天生产上没有人能走到这两口；后果是 UC-TF-001、UC-TF-007 的结果契约与 AT 只在领域与编排测试里成立。2026-10-21 文档自洽 08 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc08。
Blocked by: 无
出处：[audit/tf.md](../audit/tf.md) M15（UC 状态行过时、UC-TF-007 没有端点）与 L8（承接「待补充」，裁定见 [ADR-0265](../../../docs/adr/0265-a-regulatory-disposition-supplement-required-is-an-answer-not-a-recorded-decision.md)）；文档侧见 [08](../issues/08-tf-docs.md)。

## 现象

- `application.AcceptRegulatoryDispositionHandler`（UC-TF-001 承接决定：三种决定，待补充与纯节点动作两格答复，一事项一决定，回执意图）与 `application.StartAlternateJourneyHandler`（UC-TF-007 与 UC-TF-001 共用的独立旅程启动）都已实现、有测试，`cmd/` 下都没有接线。
- 读一侧已经接上：派发进程把旅程启动信封投给全程追踪投影（`cmd/parcel-dispatch/routes_exception_journey.go`），实际履约发生项按旅程册挂旅程（UC-TF-005 `AT-TF-252`）。生产上没有写入方。
- 承接编排只核移动授权；协作事项本体（载运对象、移动动作、起止责任、当前有效性）不读，UC-TF-001 `AT-TF-004` 除移动授权以外的几种缺口今天走不到待补充。
- 关务协作事项没有交进本上下文的路；反方向的承接回执信封 `transport-fulfillment.regulatory-acceptance.recorded` 也没有消费者（`.scratch/outbox-handoff-consumption-map/report.md` 里 `RegulatoryAcceptanceHandoff` 那一行记作「应有但未开」）。

## 三类

- 机制：两口的命令面与装配；承接编排读协作事项并核对象、动作、起止责任与当前有效性，不足时照 ADR-0265 答待补充、不落决定；回执交关务协作链的消费者。
- 产品策略：两口各归哪一族、哪一种决定（按 ADR-0151 定族判据；承接与旅程启动都是运营人员在管理台上作的生产决定，按此应归操作者渠道「运营决定」能力面，接线时立 ADR 定）。
- 租户取值：谁被授予、退运路径、备用伙伴、责任与证据（`PAR-NET-10`、`PAR-NET-11`、`PAR-CUS-01..05`），留空答`未配置`，演示租户以 `SYN-` 登记、证据记 `S`。

## 规模

M–L：两口命令面、装配、授予种类、协作事项读口（CC 一侧的提供方用例可能也要补），加 UC-TF-001、UC-TF-007 各 AT 的生产装配用例。

## 最小的诚实修法

1. 先立 ADR 定两口的族与决定种类，照 ADR-0151 与 ADR-0149 的判据。
2. 承接口接上 `AcceptRegulatoryDispositionHandler`；协作事项按引用向 CC 读（消费侧适配器，ADR-0025），对象、动作、起止责任、当前有效性不足时答待补充，答复带一个原因、指名缺哪一项（缺移动授权、缺对象、缺动作、缺起止责任，ADR-0265 决定四）；关务侧的消费者把待补充记进自己的协作事项状态（ADR-0265 越权风险点 1）。
3. 旅程启动口接上 `StartAlternateJourneyHandler`。
4. 两口都记提交操作者，照票 demo-chain-gaps/126 的口径。
5. 落地时改 UC-TF-001、UC-TF-007 的状态行与「给开发的交接」，补生产装配用例的 Covers。

## 要裁的

- 两口归哪一族、几种决定（见「三类」产品策略一条）。
- 关务协作事项交进来的形态：关务推一封信封，还是本上下文按事项引用去读。
