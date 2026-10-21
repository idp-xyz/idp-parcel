# 31 VE 客户追踪查询对本账户已归属、视图未形成的包裹答 `VIEW_NOT_FOUND`，与无权查询分不开

Category: enhancement
Status: ready-for-agent——R2（上线后尽快）：会撞（委托接受后、初始路由投影形成前，客户拿首方提交答复里的件号去查，或 NR 一直形不成投影时），但有可靠绕法：客户系统按受理状态稍后重查。首方 API 兼容性基线发布前落地最省（ADR-0246 越权风险点 5）。2026-10-21 文档自洽 04 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc04。
Blocked by: 无
出处：[audit/ve.md](../audit/ve.md) VE-05（归 B）；裁定见 [ADR-0246](../../../docs/adr/0246-the-customer-tracking-query-answers-no-visible-result-for-a-parcel-attributed-to-the-requesting-account-before-its-view-forms.md)。取证于 main `205eb254`。

## 现象

`internal/visibilityexception/adapters/http/query_customer_tracking_view.go` 的业务结果是封闭两格 `CURRENT_VIEW` / `VIEW_NOT_FOUND`，头注写「包裹不存在、不属于请求账户、或视图尚未形成，一律这一格」。端点只读客户视图库，读不到就答 `VIEW_NOT_FOUND`。归属方查自己已受理、还没形成视图的包裹，拿到的答复与无权查询相同。

## 证据

- 文档：ADR-0246 决定一至四；UC-VE-008 结果语义「当前无可见结果」一行与 `AT-VE-188`（本票同批改写与新立）；ADR-0223 越权风险点 3。
- 代码：`NewQueryCustomerTrackingViewEndpoint`、`TrackingViewReader`；归属反查口 `ports.ParcelCustomerAccountView`（今天只接在视图派生上）。

## 三类

产品策略：三格答法（ADR-0246 决定一、五）。机制：读不到视图时问一次归属。租户取值：无。

## 规模

S–M：端点加一个依赖（归属反查口）与一格 `NO_VISIBLE_RESULT`；`cmd/parcel-api` 的装配接上反查口；首方契约描述加取值；补用例；演示文档两处同步（见修法第 6 条）。

## 最小的诚实修法

1. 先读视图；读到照旧答 `CURRENT_VIEW`。
2. 读不到视图，经 `ParcelCustomerAccountView.FindCustomerAccount` 问一次归属：答出唯一账户且等于查询键的账户 → `NO_VISIBLE_RESULT`；答零份、答别的账户、答多于一份的具名错误 → `VIEW_NOT_FOUND`。
3. 反查读不动 → 与视图读不回同一格（`NO_ANSWER_FORMED`），不降成任何业务格。
4. 头注「一律这一格」改为三格；`queryResponse` 的取值说明随之改。
5. 用例：归属方查视图未形成的包裹答 `NO_VISIBLE_RESULT`；别的账户查同一件、查不存在的号都答 `VIEW_NOT_FOUND` 且响应体逐字相同；反查读不动答未形成。Covers 点 UC-VE-008 `AT-VE-188`、`AT-VE-151`。
6. 演示文档同步，与代码同一笔：
   - `scripts/demo-seeds/README.md` 客户追踪视图那一条「视图在委托接受、初始路由投影形成之后由 dispatch 派生，之前查它答 `VIEW_NOT_FOUND`」改为：委托接受之前答 `VIEW_NOT_FOUND`；接受之后、视图派生出来之前，本账户查它答 `NO_VISIBLE_RESULT`。
   - `docs/design/synthetic-demo-journey-script.md` 首方查询那张「请求 | 答复」表加一行「首方令牌，本账户已受理、视图尚未形成的包裹 | `200 NO_VISIBLE_RESULT`」；「别的账户的包裹，或不存在的包裹」一行不变。

## 要裁的

无（ADR-0246 已裁）。
