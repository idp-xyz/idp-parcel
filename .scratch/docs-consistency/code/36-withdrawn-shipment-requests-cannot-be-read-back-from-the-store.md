# 36 PS 已撤回委托在真库上读不回：撤回决定不进快照，重复撤回与撤回重提关联走不通

Category: bug
Status: needs-triage——建议 R2（上线后尽快）：答错且挡路。撤回在真库上能写下去，但再读那份委托就报错：客户重发同一撤回答 5xx 而不是原撤回；以 `WITHDRAWN_RESUBMISSION` 关联原委托的新提交答 5xx——而 ADR-0255 把它定为决定前重组的唯一路径。撤回决定本身（决定标识、授权、请求方、原因）也不进快照，CONTEXT 要求保留的撤回历史只剩一个状态码。今天客户面撤回口在生产装配下仍是未配置 Intake，首方族接上撤回口即生效。2026-10-21 文档自洽 06 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc06。分级由协调方定稿。
Blocked by: 无
出处：文档自洽 06 修订时复核 UC-PS-005 `AT-PS-068`、`AT-PS-076` 与 [ADR-0255](../../../docs/adr/0255-a-submitted-request-is-regrouped-only-by-withdrawing-it-and-submitting-linked-new-requests.md) 的路径时读码所得，不在审计报告条目里；UC-PS-001、UC-PS-005「给开发的交接」。

## 现象

委托撤回之后，仓储按来源身份读它，重建门以 `ErrRehydrationStateNotSupported` 拒绝。读那份委托的每一条路——重复撤回、撤回后关联重提、查询——在真库上都报错。

## 证据

- `internal/parcelshipment/domain/rehydration.go` 的 `admitRehydratedState` 只放行`已提交`、`已接受`、`已拒绝`，`已撤回`答 `ErrRehydrationStateNotSupported`；头注写明撤回在规格里还没有字段可填（ADR-0061 决定一的`已撤回`那一半至今未推进）。
- `internal/parcelshipment/adapters/postgres/shipment_request.go` 头注与测试（`shipment_request_test.go` 里把状态改成`已撤回`后 `FindBySourceIdentity` 报错的那一例）钉住这一点。
- 读它的路：`WithdrawShipmentRequestHandler.resolvePreserved` 与 `existing`（`AT-PS-068`）；`SubmitShipmentRequestHandler.resolvePriorClaim` 以 `LinkWithdrawnResubmission` 建关联（`AT-PS-076`，ADR-0255 决定一）。两处读不回都上抛，HTTP 答 5xx。
- 应用层测试用内存仓储，读得回撤回，所以这一格在单元测试里看不出来。
- 文档：PS CONTEXT 委托生命周期「原提交、版本、判断、撤回决定和补偿关系继续保留」。

## 三类

机制。无产品策略、无租户取值。

## 规模

M：快照规格加撤回决定一格；重建门照 ADR-0030 → ADR-0061 → ADR-0188 的先例开到`已撤回`（立一份推进记录）；仓储往返与坏数据判据；真库用例覆盖重复撤回与撤回重提关联。

## 最小的诚实修法

1. `RehydrateShipmentRequestSpec` 与快照文档加撤回决定（决定标识、授权、请求方、原因、决定时刻）。
2. 重建门开到`已撤回`，照 ADR-0188 的写法立一份推进 ADR-0061 决定一`已撤回`那一半的记录。
3. 真库用例：撤回后重复撤回答原撤回与原补偿关联；撤回后以撤回重提方向提交关联新委托成立。

## 要裁的

- 存量：首发前只有演示租户，`--reset` 重灌即可；若已有撤回行没带撤回决定，按坏数据拒绝还是推回，随推进记录定。
