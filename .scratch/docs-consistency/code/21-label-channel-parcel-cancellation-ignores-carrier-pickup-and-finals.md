# 21 PS 面单渠道服务包裹的取消不核实际承运商收寄、面单服务终局边界和既有终局

Category: bug
Status: ready-for-agent——R1（上线前必修）：答错且破坏终局不变量。逐包裹取消已在生产装配（`cmd/parcel-api/assemble_cancellation.go`）；面单渠道包裹在承运商收寄之后、甚至已有终局之后仍可能交出 `PARCEL_CANCELLED`。今天取消授权停在 `AUTHORITY_UNCONFIGURED` 暂时挡住，租户一登记取消授权（上线实施的常规一步）这条路就生效，没有绕法。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/ps.md](../audit/ps.md) PS-02，取证 [audit/ps-code.md](../audit/ps-code.md) 第 2 条（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在）。

## 现象

取消编排在提交边界上只重读网络收寄的采用链（`Adoptions.FindResponsibilityStart`），终局库和承运商首次有效收寄都不读。面单渠道包裹的终局来自面单服务终局判断，不经采用链，所以承运商收寄之后、终局之后都还能取消。

## 证据

- 文档：
  - PS CONTEXT Rules：「面单渠道服务包裹只允许在实际承运商收寄前且面单服务终局边界尚未成立时取消」；「面单渠道服务发生实际承运商收寄或其他约定终局边界后，包裹不得回退为“已取消”」。
  - UC-PS-006 `AT-PS-086`：「包裹已有终局后请求取消｜返回既有终局」。
- 代码：
  - `internal/parcelshipment/application/cancel_parcel.go` 的 `CancelParcelHandler.Handle`：提交边界只调 `deps.Adoptions.FindResponsibilityStart`。
  - 面单渠道终局由 `JudgeLabelServiceFinal` 形成；承运商首次有效收寄的采认在 `adapters/transportfulfillment/judge_on_carrier_first_effective_pickup.go` 一路，取消编排都不读。

## 三类

机制。产品策略：「其他约定终局边界」由合同声明（既有登记），未声明即只有承运商收寄一条边界。租户取值：取消授权、合同边界声明，未登记答未配置。

## 规模

M：取消编排在提交边界上加两道读——当前有效终局、按服务形态的边界事实；结果加「返回既有终局」一格（若尚无）；补 `AT-PS-086` 与面单渠道收寄后取消的用例。

## 最小的诚实修法

1. 提交边界上先读包裹当前有效终局：有终局就交既有终局，不取消。
2. 再按服务形态核边界：网络服务读采用链（今天的做法）；面单渠道服务读承运商首次有效收寄，以及合同声明的其他终局边界；越过边界答不可取消并指名边界。
3. 读不动按依赖不可用答未决，不放行取消。

## 要裁的

- 承运商收寄与取消并发（取消先提交、收寄消息后到）属 B（[audit/ps.md](../audit/ps.md) PS-03），归修订票 docs-consistency/06；本票只按提交那一刻已在册的事实判，不处理晚到。

## 裁向（2026-10-21，文档自洽 06）

- 晚到那一格已由 [ADR-0254](../../../docs/adr/0254-cancellation-and-boundary-fact-order-is-judged-by-their-business-times-and-a-late-earlier-boundary-fact-is-a-fact-conflict.md) 裁定：取消与边界事实按两者的业务时间比，取消已提交之后才到、发生不晚于取消的承运商首次有效收寄是事实冲突，面单终局判断停在未决。那一半连同冲突的解开决定面归 [code/35](./35-cancellation-and-boundary-fact-order-conflict-has-no-label-half-and-no-resolution-face.md)；本票照旧只管提交边界上已在册的事实（ADR-0254 决定二第 1 格），范围不变，Blocked by 仍为无。
