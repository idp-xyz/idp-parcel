# 71 派送发起的注释仍说拍频归调用方、作业事实口是生产入口，改指 ADR-0216

Category: enhancement
Status: resolved——2026-10-19 随 demo-chain-gaps 交接与派送 A 波合批进 main（协调方在合批分支上直接改，见「完成记录」）。此前 ready-for-agent——2026-10-19 票 [60](./60-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。同日按票 60 的隔离评审补齐清点，并改了完成判据。票 60 的地盘里本有 `buildDeliveryDispatchTrigger` 头注那一句；但下列文件在票 60 落地时都在别的会话或别的票的地盘里，票 60 没碰。
Blocked by: 无（地盘协调已解除：在途方已交还 TF 派送发起那几份文件，协调方确认可改。）

## 地盘

只改注释。下列文件在票 60 落地时各有在途方：

- 会话 idp-parcel-9b 的在途地盘：
  - TF 的 `trigger_delivery_dispatch.go`。在途地盘按文件名列，application 与 adapters/http 下的同名文件都算；
  - `cmd/parcel-api/assemble_delivery_dispatch.go`。
- 跨票热点 `cmd/parcel-api/endpoints.go`、`cmd/parcel-api/main.go`：波次在途时只许追加行，改既有注释行要等波次收口。
- operator-channel/48 也在改 `cmd/parcel-api` 若干测试与头注的注释。

## 做什么

只改注释，不改行为。

[ADR-0216](../../../docs/adr/0216-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope-per-object.md) 已定：
- 派送发起由派发进程随交接登记信封逐对象发起，任务成立时间取派发时钟；
- 作业事实口 `/transport-fulfillment-delivery-dispatch-triggers` 留作补拍；
- `PAR-NET-09` 的拍频格退役。

下面几类注释因此都已过期：
- 说派送发起的拍频归调用方或租户；
- 说谁按拍调还待定；
- 说作业事实口是执行器的生产入口；
- 说触发面不是进程内循环。

清点按票 60 评审时的搜索结果列出。做票时以「完成判据」里的搜索为准，不以这份清单为准。

- `cmd/parcel-api/assemble_delivery_dispatch.go` 的 `buildDeliveryDispatchTrigger` 头注：
  - 首句说「第一条派送要求缝的实施票立执行器的生产入口」；
  - 末段说「谁按拍调这一口、拍频多大，是租户运营节拍的实例半边，仓内不写默认」；
  - 改为：进程内的那一拍由派发进程随交接登记信封发起（ADR-0216），这一口是补拍。
- `cmd/parcel-api/main.go` 里 `buildDeliveryDispatchTrigger` 调用前那段注释：
  - 「拍频属调用方」已过期；
  - 同一段「时间窗与条件两缝留空让执行器如实停在下一处未接线」也已过期：三条缝都已接真（票 tf-segment-lifecycle-closure/13、14）。
- `cmd/parcel-api/endpoints.go` 里 `/transport-fulfillment-delivery-dispatch-triggers` 那一行上方的注释：
  - 说这一口是「内部触发执行器的生产入口」；
  - 说「谁按拍调、拍频多大属调用方（实例半边）」；
  - 说「不替调用方定拍频」。
- `internal/transportfulfillment/adapters/http/trigger_delivery_dispatch.go` 的包头注：
  - 说这一口是「内部触发执行器的生产入口」；
  - 说「触发面是端点不是进程内循环」；
  - 说「谁按拍调、拍频多大属调用方」；
  - 说「不猜一个拍频、不在进程里空转」。
- `internal/transportfulfillment/adapters/http/trigger_delivery_dispatch_test.go`：
  - 文件头注把这一口叫作「生产入口」；
  - `TestAnUnconfiguredTriggerIntakeRefusesWithoutReadingTheBody` 的 Covers 注释说「拍频作配置不作默认」。
- `internal/transportfulfillment/application/trigger_delivery_dispatch.go`：
  - `TriggerDeliveryDispatchCommand` 注释末句「谁按拍调本编排、拍频多大，随第一条派送要求缝的实施票立（ADR-0114 决定二末句），这里不预设」。改为指 ADR-0216：调用方是派发进程里派送发起那一路，以及作业事实口的补拍；
  - `ContinuationReference` 注释「供按拍调本编排的一方重跑同一拍」，按同一口径改成说调用方。
- `cmd/parcel-api/unwired_orchestration.go` 的 `unwiredDeliveryDispatchTrigger` 注释：只引了票 tf-segment-lifecycle-closure/12 的小节名「生产入口」，没有断言这一口是生产入口。改不改由做票者判。

## 完成判据

- 在 `cmd`、`internal` 下的 Go 文件里搜「拍频」「按拍调」「生产入口」「进程内循环」「空转」。命中只要落在派送发起上（执行器、作业事实口、它们的装配与用例），就都不再说：
  - 拍频归调用方或租户；
  - 谁按拍调还待定；
  - 作业事实口是生产入口；
  - 触发面不是进程内循环。
- 改后的说法指 ADR-0216：进程内的那一拍由派发进程随交接登记信封发起，作业事实口是补拍。
- 其他机制里的「生产入口」命中不归本票。
- `go build ./...`、`go vet`、`gofmt -l` 干净；所动文件所在各包的测试照过。

## 规模

S

## 完成记录（2026-10-19，协调方在合批分支 tmp-wavea 上直接改）

只改注释，不改行为。改后口径一律指 ADR-0216：进程内的那一拍由派发进程随交接登记信封逐对象发起，作业事实口 `/transport-fulfillment-delivery-dispatch-triggers` 是补拍口；一拍一对象，没有拍频可选。

- `cmd/parcel-api/assemble_delivery_dispatch.go`，`buildDeliveryDispatchTrigger` 头注：
  - 首段改为补拍口，并指向派发进程那一路另接的同一份三条缝（`cmd/parcel-dispatch` 的 `triggerDeliveryDispatchOnHandoverConsumer`），改缝要两处一起改；
  - 末段「谁按拍调、拍频多大是租户运营节拍的实例半边」改为「一拍一对象，没有拍频可选（ADR-0216 决定二）」，渠道与未配置那半句不动。
- `cmd/parcel-api/main.go`，`buildDeliveryDispatchTrigger` 调用前那段：删去「时间窗与条件两缝留空」「拍频属调用方，Intake 未配置」，改为只给补拍口用、接法见 `buildDeliveryDispatchTrigger`。
- `cmd/parcel-api/endpoints.go`，`/transport-fulfillment-delivery-dispatch-triggers` 那一行上方：改为补拍口（ADR-0216 决定四），写明它留给所有者补齐要求之后再发起的情形；渠道归属那半句不动。
- `cmd/parcel-api/unwired_orchestration.go`，`unwiredDeliveryDispatchTrigger`：判为改。票 tf-segment-lifecycle-closure/12「生产入口」小节名换成「补拍口背后那一个，ADR-0216 决定四」，免得读成断言。
- `internal/transportfulfillment/adapters/http/trigger_delivery_dispatch.go` 包头注：首段改为补拍口，写明进程内那一拍的来处、补拍留给什么情形、两边答复互相认（答`已在册`、交回同一项任务）；删去「触发面是端点不是进程内循环」「谁按拍调、拍频多大属调用方」「不猜一个拍频、不在进程里空转」。与手工建任务口区分的那一段不动。
- `internal/transportfulfillment/adapters/http/trigger_delivery_dispatch_test.go`：文件头注改为补拍口；`TestAnUnconfiguredTriggerIntakeRefusesWithoutReadingTheBody` 的 Covers 末句改为补拍口同样只对配好的渠道开。
- `internal/transportfulfillment/application/trigger_delivery_dispatch.go`：
  - `TriggerDeliveryDispatchCommand` 末句改为两个调用方：派发进程那一路给派发时钟，补拍给请求里报的时刻；
  - `ContinuationReference` 改为「供调用方重跑同一拍时对上号」：派发进程那一路随 inbox 重投重跑，补拍口交回报拍的一方。

完成判据里的搜索（`cmd/parcel-api`、`cmd/parcel-dispatch`、`internal/transportfulfillment` 下的 Go 文件）复跑：派送发起上的命中只剩两处「没有拍频可选」，都是 ADR-0216 的口径。其余命中不归本票：「生产入口」落在审核、处置、结算、有效时间规则、外部承运凭证等别的机制上；「空转」落在 VE 补派生那一腿。

验证：`go build ./...`、`go vet ./cmd/... ./internal/transportfulfillment/...` 通过；`gofmt -l cmd internal` 无输出；`go test -count=1` 跑 `./internal/architecture/`、`./internal/transportfulfillment/adapters/http/`、`./internal/transportfulfillment/application/`、`./cmd/parcel-api/` 均 ok（不带 DSN）。
