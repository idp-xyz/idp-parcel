# 76 PS 取消先后冲突留在册上，却没有解开它的业务决定面

Category: enhancement
Status: needs-triage——建议 R2（上线后尽快）：看得见的卡住。冲突已经记进 `parcel_shipment.cancellation_order_conflict`、消费门入账，按包裹查得出；但没有任何口能解开它：网络服务的冲突包裹没有责任起点，也不触发收寄后的路由复核，实物照旧在节点手里；面单渠道服务的冲突包裹没有终局，客户手里是`包裹已取消`，承运商却已收走。今天取消授权停在未配置，取消形成不了，冲突还到不了；租户一登记取消授权即生效。2026-10-21 自 [code/35](./35-cancellation-and-boundary-fact-order-conflict-has-no-label-half-and-no-resolution-face.md) 拆出（用户「你作为业务和系统专家自决」授权），分支 claude-code21。分级由协调方定稿。
Blocked by: 无（判法与冲突册已随 code/35 落地；决定的去向集合与授权形状要 PS owner 认可，见「要裁的」）
出处：[ADR-0254](../../../docs/adr/0254-cancellation-and-boundary-fact-order-is-judged-by-their-business-times-and-a-late-earlier-boundary-fact-is-a-fact-conflict.md) 决定三与越权风险点 2；code/35「最小的诚实修法」第 2 条。

## 为什么从 code/35 拆出

code/35 有两半：判法与入账（面单渠道一侧比时间、收寄采用消费门对冲突入账、冲突留下），以及解开冲突的决定面。前一半是机制，ADR-0254 已经把每一格定死，随 code/35 落地。后一半要先定一个 ADR-0254 有意没定的形状——谁有权、去向有哪几个、每个去向驱动什么重判——它是要 PS owner 认可的产品策略，体量本身就是一张 M 票：命令、授权读口、只追加的决定记录、续办信封，外加各条消费链的重判入口。两半捆在一票里，前一半要等后一半的认可才能进 main，而前一半不进 main，冲突会照旧回滚重投到失败预算耗尽、在库里不留痕。

## 现象

- 冲突册只追加、按包裹列（`CancellationOrderConflictRegister.ListByParcel`），没有任何编排读它。
- 冲突那一格的消费结论是入账：网络服务的采用答 `ELIGIBILITY_UNDECIDED`、原因 `CANCELLATION_ORDER_CONFLICT`；面单渠道服务的终局判断答 `LABEL_SERVICE_NOT_FINAL`、原因 `CANCELLATION_ORDER_CONFLICT`。同一来源再投仍落同一格，冲突册不翻倍。
- 已提交的取消照旧成立，`ParcelCancellationView` 照旧交出它；下游已收到释放意图。

## 证据

- 代码：`internal/parcelshipment/domain/cancellation_order.go` 的 `JudgeCancellationOrder`；`internal/parcelshipment/ports/cancellation_boundary.go` 的 `CancellationOrderConflictRegister`；`internal/parcelshipment/adapters/postgres/cancellation_order_conflict.go`；迁移 `migrations/parcel_shipment/0029_cancellation_order_conflict.sql`。
- 文档：UC-PS-003 `AT-PS-107`、UC-PS-004 `AT-PS-108`、UC-PS-006 决定矩阵「事实冲突」一行与 `AT-PS-081` 后一种，「给开发的交接」各指本票。

## 三类

机制：决定面的命令、只追加记录、续办信封与各条消费链的重判入口。产品策略：决定的去向集合、每个去向驱动的重判、谁有权（授权规则的形状）——本票提出、交 PS owner 认可。租户取值：谁持有这个授权，属 `PAR-COM-14` 一类，未登记答未配置。

## 规模

M：一个授权决定面（命令、授权、只追加的冲突解开决定记录、续办信封）；收寄采用、面单终局判断与有效交付的终局采用各认一次「这份冲突已由某决定解开」；冲突册加「已解开」的读法（另起一行回指冲突，不改写冲突行）。

## 最小的诚实修法（提案，待认可）

1. 决定挂在一行冲突上（租户，包裹，边界种类，边界那一代，取消），只追加，不改写冲突行，也不改写已提交的取消。
2. 去向至少两个：
   - **认定取消依据不成立，包裹转入收寄后服务处置**：取消记录保留，另记一份「取消被业务决定认定在边界之后」的决定；随后由这份决定驱动重判——网络服务重驱收寄采用（采用编排读到这份决定即不再按取消判先后，照常判资格与责任起点），面单渠道服务重驱终局判断（形成非取消终局）。客户侧那份`包裹已取消`怎么改口、下游已释放的东西怎么补回，属收寄后处置（code/37）那一面，本票只交出入口。
   - **认定边界事实有误，交来源方更正**：不在本上下文改任何事实；来源方更正之后新的一代按 ADR-0254 决定二重判（冲突册按那一代另起一行或不再起行）。
3. 授权未登记答未配置，不默认任何角色可解开。
4. 重判入口除收寄采用重驱与面单终局判断重驱之外，还有**网络服务履约结果（有效交付）的终局采用**：包裹上取消在册时到达的有效交付，`FormParcelFinalHandler` 答 `SOURCE_NOT_ADOPTED`、依据 `PARCEL_CANCELLATION/<取消标识>`，不采用记录按（包裹、结果种类、来源版本）永久保全。决定面把冲突解开（判「取消不成立」）后，必须能把依据该取消的不采用记录作废或重判，使交付重新进入终局规则判断。只等运输履约重投不够：同一版本重投只答「已有结果」，原不采用记录原样返回，这件包裹会一直没有终局。来源：code/98 评审 N1。上线影响：取消授权在生产未配置、没有决定面，冲突形成不了，今天不触发。
5. 补用例：网络服务冲突经决定解开后形成责任起点；面单渠道冲突经决定解开后形成终局；依据该取消的有效交付不采用记录经决定解开后作废或重判、交付形成终局；授权未配置停在未决。

## 要裁的

- 去向集合与每个去向驱动的重判（ADR-0254 越权风险点 2，PS owner）。
- 「认定取消依据不成立」之后，取消在客户侧与委托汇总里怎么读——是取消失效的一种（需要新的取消状态）还是只由收寄后处置覆盖其后果；前者动 CONTEXT 生命周期，可能要另立 ADR。
