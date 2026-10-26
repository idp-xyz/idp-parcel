# 180 TF 其余运营决定口与派送重问、补拍不记提交操作者

Category: bug
Status: ready-for-agent——2026-10-26 票 126 隔离评审之后立（上线前必修：真实租户上线后，没记下的决定无法补记，理由同复核报告 M1），见票 [126](./126-tf-operation-decisions-record-the-submitting-operator.md)「评审记录」Spec 非阻断 1、2 与「修订轮」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；票 126 隔离评审；[ADR-0303](../../../docs/adr/0303-transport-fulfillment-operation-decisions-record-the-submitting-operator-as-operation-evidence.md) 决定五与越权风险点 1、4；代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 M1；[ADR-0230](../../../docs/adr/0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 决定七

## 现象（读码于票 126 分支）

票 126 按 ADR-0303 让关段、明确终止、建派送任务、装载分配、两个判断口与运输委托提交七口记下提交操作者。同在操作者渠道「运营决定」能力面上、同样形成决定的还有两组，Intake 仍走 `tfhttp.OperatorDecisionIntake.tenantFor`，认证出的操作者在 Intake 里就丢了：

- 形成决定记录的六口：订舱提交 `/transport-fulfillment-booking-request-submissions`、取消订舱 `/transport-fulfillment-booking-request-cancellations`、取消运输委托 `/transport-fulfillment-transport-commission-cancellations`、履约段采用运输委托 `/transport-fulfillment-segment-commission-adoptions`、容量预占 `/transport-fulfillment-capacity-reservations`、容量释放 `/transport-fulfillment-capacity-releases`。
- 派送补拍 `/transport-fulfillment-delivery-dispatch-triggers` 与按册重问 `/transport-fulfillment-delivery-dispatch-reasks`（决定种类「派送重问」，ADR-0230 决定七）。ADR-0303 决定五定了执行器形成的任务不记建任务人——任务的内容出自所有者给的派送要求；但「这一次再问」是哪一位按的，至今哪里都没留下。产品没有逐请求的接入日志，事后无从补记。

票 126 没并进来，是因为订舱与容量池两组的读写面当时正由票 127 改动（ADR-0303 决定五）。

## 修法

- **六口**：照 ADR-0303 把信封里的提交操作者（「发行方#sub」，`domain.RegistrantReference`）作为操作证据落在各自的决定记录上——订舱那一份、订舱的取消、委托的取消、段采用的那一代、那一份预占、那一次释放。待遇照 ADR-0303 决定三（即 ADR-0187 三条）：只进审计，不进重放与内容冲突比对，不进任何门；写面回执不带；可缺、不回填，存量读作「没记下是谁」。查阅面已展示这些记录的（如订舱册），补出操作者。
- **派送重问与补拍**：仍不记在任务上（ADR-0303 决定五），另记在重问决定处——落点待作者定：例如派送待形成册上「最近一次问」旁记问的人，或另立一份重问记录；补拍对单件、按册重问批量，两口都要留下是谁、何时按的。重问的执行结果与答复形状不变。
- **文档**：扩 ADR-0303 的口径到这几口并定重问的落点（新 ADR 或在 ADR-0303 文末补复核记录，按停用规则挂指针）；相关 UC 立 AT 行（号取当时空号，全仓与在途分支核过）；AT 盘点①补行；迁移号取当时下一个空号。

## 完成判据

- 六口每口：命令收操作者、记录加列、读回带出；同一决定由另一操作者重放答复不变、在册的不被顶替（每口一条用例钉）；不带操作者照样形成；库面拒空白与错位。
- 补拍与重问：按的人可查，带 PG 的用例钉住；执行器形成的任务仍不记建任务人。
- 生产装配真库用例：两个令牌各投一次，库里是首投者，回执不含操作者。
