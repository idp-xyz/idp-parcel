# 98 网络服务的交付终局采用不读已成立的取消：取消在册时，迟到的有效交付仍形成非取消终局

Category: bug
Status: needs-triage——建议 R2（上线后尽快）：一件包裹可能同时带着取消终局与非取消终局，委托完成汇总把它算成履约终局。顺序发生即可到达，与并发无关。今天取消授权与终局规则都停在未配置，取消形成不了、交付终局也形成不了，这一格还到不了；租户把两样都登记之后即生效。2026-10-26 code/77 落地时核「交付终局要不要进取消边界锁」所得。
Blocked by: 无
出处：[code/77](./77-cancellation-intake-adoption-and-label-final-are-not-serialized-per-parcel.md)「要裁的」一节的核查（完成记录代决 1）。

## 现象

网络服务的有效交付经 `AdoptOnEffectiveDeliveryAdapter` 交给终局采用编排 `FormParcelFinalHandler`。编排的「既有终局分派」只问终局库的当前有效终局（`Finals.FindCurrentFinal`），不问取消库里已成立的取消；取消读口只用在委托完成摘要的派生上。

一件包裹在收寄之前已经合法取消，实物却仍进了网络（收寄采用答 `SOURCE_NOT_ADOPTED`/`CANCELLED_BEFORE_INTAKE`，或停在 `CANCELLATION_ORDER_CONFLICT`，都没有责任起点）。此后 transport-fulfillment 若登记了这件包裹的有效交付，终局采用照常判规则、形成非取消终局：

- 包裹同时带着取消终局与非取消终局；
- `deriveCompletion` 先读终局库，这件包裹在汇总里算履约终局、不算取消；
- 停在冲突上的包裹（ADR-0254 决定二第 3 格「不形成责任起点也不形成非取消终局」）由交付一路绕过冲突形成终局，冲突册上那一行没人再看。

这不是并发问题：交付一路不读取消写下的任何东西，先后怎么排它都照样形成终局。code/77 的取消边界锁因此没有收它（见那张票的完成记录）。

## 证据

- 代码：`internal/parcelshipment/application/form_parcel_final.go` 的 `Handle`，既有终局分派只调 `Finals.FindCurrentFinal`；`Cancellations` 只在 `deriveCompletion` 里读。`internal/parcelshipment/adapters/transportfulfillment/adopt_on_effective_delivery.go` 按包裹反查当前已接受委托，不看取消。
- 文档：
  - UC-PS-004 应用流程「校验包裹身份、接受基线、服务类型、谱系和既有终局｜被替代/越权来源不采用」；
  - PS CONTEXT「取消或依据适用终局规则形成的包裹级失败结果可以在责任起点或正式承诺尚未形成时成为终局」——取消本身就是一份终局；
  - UC-PS-006 `AT-PS-080`「取消不被覆盖」；ADR-0254 决定二第 3 格；
  - UC-PS-003 启动条件「已经取消、已经终局或被身份谱系替代的包裹仍可产生真实节点或运输事实。该事实必须保全，但不能自动重开服务」。

## 三类

机制（既有终局分派漏读一处）。交付到达时落哪一格（不采用，还是并入冲突）可能要 PS owner 定，见「要裁的」。

## 规模

S：终局采用编排在既有终局分派处读已成立的取消；交付一路由此读了取消写下的东西，要与取消一起进 code/77 那把按包裹的取消边界锁（锁在这一道读之前取）；补用例（顺序一条、真库并发一条）。

## 最小的诚实修法

1. `FormParcelFinalHandler` 的既有终局分派同时读已成立的取消：取消在册即不采用，依据指名那份取消（如 `PARCEL_CANCELLATION/<取消标识>`），来源与物理事实保留。
2. 交付一路在这道读之前取 `ports.CancellationBoundaryLock`，与取消、收寄采用、面单渠道终局判断排同一个队；面单渠道一路已在判断前取过，同一事务重入。
3. 补用例：取消在册后到达的有效交付不形成终局；取消与交付并发时两种先后各一遍。

## 要裁的

- 取消在册时到达的有效交付答哪一格：按取消在先不采用，还是当收寄冲突仍未解开时并入冲突、停着等决定（code/76 的决定面）。
