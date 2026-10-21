# 38 PS 代码注释随文档自洽 06 改口

Category: chore
Status: needs-triage——建议 R3（不急）：只改注释，不改行为；注释今天指向已改写或被裁过的口径，读码的人会照着写错。2026-10-21 文档自洽 06 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc06。分级由协调方定稿。
Blocked by: 无
出处：[audit/ps.md](../audit/ps.md) PS-04、PS-09、PS-16、PS-17、PS-49 的「代码注释随票改」半边，与 [ADR-0255](../../../docs/adr/0255-a-submitted-request-is-regrouped-only-by-withdrawing-it-and-submitting-linked-new-requests.md)、[ADR-0257](../../../docs/adr/0257-a-lapsed-label-success-only-feeds-the-label-service-final-and-leaves-the-channel-parcel-number-relation-applicable.md) 后果节点名的两处。

## 逐处

| 位置（符号） | 今天写的 | 改成什么 | 依据 |
|---|---|---|---|
| `internal/parcelshipment/application/submit_shipment_request.go` 包头注 | 「它不含任何持久化、事务或事件机制，那些仍阻断在 Bento 闸门之后（ADR-0017）」 | 持久化与事件机制在 `adapters`，不进本包 | ADR-0026；UC-PS-001「开发拆分建议」末段 |
| 同文件 `OutcomeLinkIneligible` 头注 | 「客户该走的是资料修订（待决）或等决定」 | 普通纠错走同一委托的新提交版本；重组先撤回原委托，再以撤回重提方向关联 | ADR-0255 |
| `application/withdraw_shipment_request.go` 的 `releaseOccupation` 头注 | 「从未形成过冻结时」 | 没有任何控制项成立时 | PS CONTEXT「委托撤回不能伪装成运营企业拒绝」一条；ADR-0125 决定四 |
| `domain/customer_source_data.go` 的 `SourceDataAdoptionOutcome` 头注 | 「CONTEXT 声明的封闭集合有五项……`待复核`是 UC-PS-002 对 CONTEXT`待补充`那一格的点名叫法」 | CONTEXT 词条「当前客户资料版本采用判断」的取值，`待复核`是与`待补充`分开的一格，续办方是授权复核角色 | PS CONTEXT 词条 |
| `application/cancel_parcel.go` 的 `ErrIntakeBoundaryCrossed` 分支注释 | 「没有授权处置决定时保持待处置（AT-PS-082）」 | 没有收寄后服务处置决定时保持待处置 | UC-PS-006 `AT-PS-082` |
| `domain/external_identifier.go` 的 `ExternalIdentifierChangeKind` 头注 | 「已失效不在其中：有效期规则今天没有登记处，没有执行器会形成它（ADR-0184 决定七）」 | 外部标识关联只有作废与替代两种终态；面单有效期届满只进面单服务终局判断，不改号码关联（ADR-0257） | ADR-0257 |
| `application/judgment_continuation.go` 的 `SourceDataAmendmentStageUndetermined` 一段注释 | 「今天是关务与装袋的读面未接」 | 两只读面已接提供方真读面（ps-port-remainder/05）；判不出是六格事实里有答「不知道」的 | ADR-0118 Status 行注记 |
| `domain/declared_measurement*.go` 等处注释 | 「申报测量」 | 声明测量 | PS CONTEXT 词条「声明测量」 |
| `adapters/http/withdraw_shipment_request.go`、`amend_customer_source_data.go`、`form_new_submission_version.go` 头注 | 把 `!found` 读成「统一不可见结果不拆开」而答 5xx | 随 [code/22](./22-customer-face-http-contract-misses-accepted-adrs.md) 一起改，本票不动 | ADR-0022 |

## 三类

不涉。只改注释。

## 规模

S。
