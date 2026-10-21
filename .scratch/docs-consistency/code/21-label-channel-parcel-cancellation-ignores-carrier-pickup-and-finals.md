# 21 PS 面单渠道服务包裹的取消不核实际承运商收寄、面单服务终局边界和既有终局

Category: bug
Status: resolved——2026-10-21 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-code21（见文末「完成记录」）。此前 ready-for-agent——R1（上线前必修）：答错且破坏终局不变量。逐包裹取消已在生产装配（`cmd/parcel-api/assemble_cancellation.go`）；面单渠道包裹在承运商收寄之后、甚至已有终局之后仍可能交出 `PARCEL_CANCELLED`。今天取消授权停在 `AUTHORITY_UNCONFIGURED` 暂时挡住，租户一登记取消授权（上线实施的常规一步）这条路就生效，没有绕法。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
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

- 晚到那一格已由 [ADR-0254](../../../docs/adr/0254-cancellation-and-boundary-fact-order-is-judged-by-their-business-times-and-a-late-earlier-boundary-fact-is-a-fact-conflict.md) 裁定：取消与边界事实按两者的业务时间比，取消已提交之后才到、发生不晚于取消的承运商首次有效收寄是事实冲突，面单终局判断不形成终局、指名冲突（`LABEL_SERVICE_NOT_FINAL` 加一格原因）。那一半连同冲突的解开决定面归 [code/35](./35-cancellation-and-boundary-fact-order-conflict-has-no-label-half-and-no-resolution-face.md)；本票照旧只管提交边界上已在册的事实（ADR-0254 决定二第 1 格），范围不变，Blocked by 仍为无。

## 完成记录（2026-10-21，分支 claude-code21，rebase 于 main `8d68c7bab`；与 code/35 同批）

### 做了什么

- `654f442d8` 取消编排在提交边界上加读（`CancelParcelHandler.Handle`，授权之后、形成取消之前）：
  - 先读当前有效终局（`CurrentFinalView`）与已成立的取消（`ParcelCancellationView`）：有终局答新结果 `FINAL_OUTCOME_STANDS`，`Basis` 指名 `FINAL_OUTCOME/<终局版本>` 或 `PARCEL_CANCELLATION/<取消标识>`，不落取消记录（`AT-PS-086`）。
  - 再读网络服务的采用链（今天的做法，越过答 `DISPOSITION_PENDING`）。
  - 未越过时读面单渠道服务的实际承运商首次有效收寄链尾（新端口 `CarrierPickupBoundaryView`，TF 登记册之上的消费侧适配器 `pstf.CarrierPickupBoundaryAdapter`）：已形成答 `CANCELLATION_REFUSED`，依据 `CARRIER_FIRST_EFFECTIVE_PICKUP/<收寄事实>@<版本>`，拒绝记录落库、重放答原结果；待确认答未决 `CARRIER_PICKUP_PENDING`；链尾失效与从未有过同答没有。
  - 读不动按依赖未决：`FINAL_OUTCOME_STORE_UNAVAILABLE`、`CARRIER_PICKUP_UNAVAILABLE`；已成立取消读不动沿用 `CANCELLATION_STORE_UNAVAILABLE`。不放行取消。
  - HTTP 响应加 `standingFinal`；parcel-api 取消装配接真终局库与 TF 收寄登记册，授权缝拆出 `cancellationOrchestrationWith` 供真库用例换一个已登记的授权答复。
- `6471fa9a9` 管理台取消页认 `FINAL_OUTCOME_STANDS`、三个新未决原因与「既有终局」一行。
- `7d6c1d7f2` UC-PS-006 状态行、结果语义契约（新增「既有终局」一行，拒绝与未决两行的含义与原因码补齐）、决定矩阵冲突一行与交接表 code/21 一行改为已补。
- `2b799b8d2` AT 盘点 `AT-PS-086` 由④移入①。

### 代决

用户授权「你作为业务和系统专家自决……解决所有的问题」。

1. 提交边界上的边界事实全读，不按服务形态挑：本上下文不在包裹上存服务形态的判别位，CONTEXT「此刻已在册的收寄或终局使取消不成立」对两种形态同写，全读是它的直译。读序是终局 → 网络服务采用链 → 面单渠道服务首次有效收寄。
2. 「返回既有终局」另立结果 `FINAL_OUTCOME_STANDS`，不并进`已有结果`：后者说的是同一请求身份的重放，前者说的是包裹本身已终局。不落记录：它说的是此刻在册的终局；终局失效后重放，按失效后的边界重判。
3. 已成立的取消也是既有终局。取消键是（租户，委托来源请求键，包裹），同一委托下第二份取消在键上就撞了；这一格只在包裹号跨委托复用时到得了，读它是为不让同一包裹形成第二份取消。
4. 面单渠道服务越过首次有效收寄答`取消被拒`、依据指名那一代，不另立结果格：面单渠道服务没有收寄后处置这条路，收寄就是它的终局边界（CONTEXT「面单渠道服务发生实际承运商收寄……后，包裹不得回退为“已取消”」），ADR-0254 决定二第 1 格写的是「答既有终局或不可取消」。记录落库，与网络服务的`待处置`同样是已提交的判断。
5. 待确认的收寄答取消未决（UC-PS-006 决定矩阵「收寄事实存在但有效性待确认 → 取消/处置未决」）；失效链尾当作没有（ADR-0135 决定六「链尾失效即该对象当前无首次有效收寄」）。
6. 授权仍在边界之前：授权未配置时，已终局的包裹照旧答 `AUTHORITY_UNCONFIGURED`。票面「先读终局」读作提交边界上的先后，不改授权在前的既有顺序。
7. 载运对象按包裹标识原串查 TF 链尾，与收寄判断那一路反查包裹用同一个串；集运单元那一格同样不猜。
8. 「其他约定终局边界」不另读：合同声明的边界成立即形成终局（CONTEXT 生命周期），已由终局那一道读覆盖；未声明即只有承运商收寄一条。

### 越权风险点（待 PS owner 复核）

1. 全读不分形态：网络服务包裹若在 TF 册上有首次有效收寄，会答`取消被拒`而不是`待处置`，且拒绝落库后同一请求被钉住（重放答已有结果）。TF 的「非首次」规则下（ADR-0135 决定四）已参与的对象不形成首次有效收寄，这一格不应出现；若 owner 要按形态分读，需要先在 PS 立服务形态的判别。
2. 两种取舍并列，一起交 owner 裁：
   - 面单渠道越界的拒绝落库，会冻结答复——收寄日后失效（ADR-0135 决定六），同一请求重放仍答原来的拒绝；
   - 终局在册不落记录，每次重读——终局日后重派生或失效（ADR-0138 仍 Proposed），同一请求重放按失效后的边界重判，可能答出不同的结果。
   两条方向相反，都没有立 ADR-0266 那样「答原结果」的统一纪律。
3. 面单渠道越界与授权拒绝同用 `CANCELLATION_REFUSED`，客户端靠依据前缀分辨。若 owner 要求单列一格，改的是结果枚举一格与记录种类一格（要迁移）。
4. 授权未配置时已终局的包裹不答既有终局（代决 6）。
5. 取消提交、收寄采用、面单终局形成三处之间不按包裹串行：read committed 下两笔并发时可能同时落下取消与非取消终局，此后不会再有信封重新比对。归 [code/77](./77-cancellation-intake-adoption-and-label-final-are-not-serialized-per-parcel.md)（R2）。

### 迁移

无（本票）。冲突册迁移 0029 属 code/35。

### AT

`AT-PS-086` 入①：`TestAStandingFinalIsReturnedInsteadOfACancellation`、真库 `TestTheWiredCancellationStopsAtTheBoundaryFactsAgainstARealDatabase`。面单渠道越界另由 `TestALabelChannelParcelCannotBeCancelledPastTheCarrierPickup`、`TestTheCarrierPickupBoundaryReadsTheChainTail` 守；结果码与原因码逐码对照 String() 在 `TestTheCancellationOutcomeAndReasonSetsAreClosed`。

### 验证（分支 claude-code21）

与 code/35 同批跑，记在 code/35 完成记录「验证」一节。

### 未决

- 管理台改动没有跑类型检查：本 worktree 没有 `node_modules`。
- AT 盘点总览计数未改，由推送方重算。

## 评审记录

### 第 1 轮（2026-10-21，隔离评审；协调方转达，无阻断）

判法与冲突册与 ADR-0254 逐格一致，变异全红；admin-web 的 tsc、测试与 vite build 由评审方跑过。本票修订：

- N3：`CancellationFinalOutcomeStands` 头注与代决 2 照实写「不落记录：它说的是此刻在册的终局；终局失效后重放，按失效后的边界重判」——`91e6aaa4f`。
- N5：越权风险点 1 补「拒绝落库后同一请求被钉住」。
- N6：越权风险点 2 并列写两种取舍（越界拒绝落库冻结答复；终局在册不落记录每次重读），一起交 owner 裁。
- N4：另立 [code/77](./77-cancellation-intake-adoption-and-label-final-are-not-serialized-per-parcel.md)（R2），越权风险点 5 指向它——`cfc3472d2`。
- 收口前 `git rebase main`（`8d68c7bab`），无冲突；完成记录里的 SHA 已换成 rebase 后的。

修订后验证见 code/35 评审记录。

## 进 main 记录（2026-10-21，协调方）

- 分支 `claude-code21`（code/21、35 同一分支）由协调方 rebase 到 main `c83f8e8e6`（无冲突）后快进，tip `e57a2f271`；隔离评审无阻断，按协调方裁定补了交易与关闭两路照冲突册作答（ADR-0254 第 3 格字面），其余建议已收。
- 协调方在 tip 上单独提交重算的 AT 计数与重生成的机制清点；ADR 索引无变化；检查均退 0。
- 全量 `scripts/test-full.sh` 退 0，158 个包 ok；`go build ./...`、`go vet ./...` 干净；admin-web `tsc -b --noEmit`、`run-tests.mjs`、`vite build` 都过；演示种子在一次性库上灌通（迁移 `parcel_shipment/0029` 一并施加），用后删库。
- 去向：code/76（解开冲突的业务决定面，待 PS owner 认可形状）、code/77（取消、采用、面单终局按包裹串行，R2）。
