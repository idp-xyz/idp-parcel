# HTTP 归属视图对「其他权威」决定不渲染未决原因、续办引用与确认引用：生产上唯一走得到的 Other 路在接入面缺格

Category: bug
Status: in-progress——2026-09-09 18:4x 通道 3 认领（用户 18:2x 经 IDP 队列指示由通道 3/4 自派；通道 3 18:29 点名后自领，分支 `mcp3-wbr11`，树 `D:/tops/idp-parcel-mcp3-wbr11`，基 main `d5a35960`；01 已进 main `8a1c403b`，阻断解除）。此前 ready-for-agent——2026-09-09 通道 1 代裁立票（用户授权自决）：wbr/01 评审（通道 3，钉 `a5bff461`）Spec 非阻断 1 点名「拆出物无票」；
作者完成记录「未落 ①」写「归 PS 另立票」，这就是那张票。**Blocked by [01](./01-ps-safe-handoff-is-assessed-nowhere-because-nothing-hands-over.md) 进 main**：本票改的
`newOwnershipView` 读的是 wbr/01 那五笔给 `ProductionOwnershipDecision` 加的评估格，01 未进 main 之前无处可接。PS 地盘。
Blocked by: 01

## 缺口

wbr/01 之后，`UC-PS-001` 步 3B 的「其他权威」路在生产上走得通了：编排在归属决定为 Other 时真调 `AssessSafeHandoff`，生产装配给出的通道是
`UnconfiguredOtherProductionAuthorityChannel{}`（ADR-0063 决定四「生产装配必须给出显式未配置实现」），于是今天每一个 Other 决定都以
「交接未决 · 通道未配置」收场，HTTP 答 `OWNERSHIP_UNRESOLVED`。

但 `adapters/http/submit_shipment_request.go` 的 `newOwnershipView` 只在 `decision.UnresolvedDetails()` 答 present 时才渲染 `unresolvedReason` 与
`continuationReference`，而 `UnresolvedDetails` 只对归属本身未决（Authority = Unresolved）的决定作答；Other 决定即使交接未决，它也答 false。结果：

- **Other + 交接未决**：HTTP 上是 `OWNERSHIP_UNRESOLVED` + `otherAuthority` + `handoffReference`，**没有未决原因、没有续办引用**——而 `UC-PS-001` 结果行
  「生产归属未决」要求带「安全续办引用」，编排层明明已经把 `CONT-PS-HANDOFF/<attemptID>` 那枚续办引用与 `CHANNEL_NOT_CONFIGURED` 那格原因写进了决定记录。
- **Other + 已确认**：HTTP 上是 `OTHER_PRODUCTION_AUTHORITY`，**没有确认引用**（渠道中立关联）；真通道接上那天这一格就会立刻被问到。

ADR-0128 决定五与 Consequences 把这一格点名为拆出物；wbr/01 完成判据 1–5 不含 HTTP，拆出成立，但拆出之后没有人认领——这就是本票。

## 要裁的形（实施者定，理由写完成记录）

JSON 契约两问：① Other + 交接未决 的原因与续办引用，是复用既有 `unresolvedReason` / `continuationReference` 两格（同一结果行、同一语义「未决 + 续办」），
还是另起名字；② Other + 已确认 的确认引用叫什么。倾向 ①复用、②另起一格（如 `handoffConfirmationReference`），但**不在此裁死**：读 ADR-0128 决定四
（决定记录分格）与 `ProductionOwnershipDecision` 上评估结果那两格的形之后定，管理台读这份 JSON 的那一侧（admin-web）若有既有字段名以它为准。

## 完成判据

1. `newOwnershipView` 对 **Other + 交接未决** 渲染未决原因（决定记录里评估结果那一格映射出的原因，今天生产上是 `CHANNEL_NOT_CONFIGURED`）与续办引用；
   对 **Other + 已确认** 渲染确认引用。字段名按上节裁定，写进 http 测试的字面断言。
2. http 测试补「未配置适配器 + Other」**一例**（wbr/01 评审 Spec 非阻断 2：加一例，不替换 `handoffChannelDouble` 给完整确认的那例）；
   `cmd/parcel-api` 装配测试在 `permittingOwnership` 之外补同一路一例，断言生产装配下 Other 决定的 HTTP 答复带续办引用。
3. 不改 domain / application / ports 的签名；不动 UC 文本；不改 ADR-0128（若裁定与决定五字面有出入，走 supersede 不改写）。
4. 验证（作者层）：`go build ./...` / `go vet ./...`；`go test -count=1` PS `adapters/http` + `application` + `./cmd/parcel-api/`（带 DSN）+ `./internal/architecture/...`。

## 边界

通往他方权威的真通道（`PAR-GOV-05..07` 实例半边）不归本票；决定记录落库（wbr/01「未落 ③」）不归本票；wbr/01 评审 Spec 非阻断 3
（`assessedAt` 取投递之前的 `decidedAt`，评估时刻系统性早于确认生效时刻）是作者记下的已知取舍，真通道接上时再看，不在此改。
