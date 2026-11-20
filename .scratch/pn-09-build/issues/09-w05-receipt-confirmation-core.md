# 09 `PN09-W05` 第 1 段：收货确认的核心（商品入库）

Category: enhancement
Status: in-progress（代码写完；还没有非作者评审；没有进 main）
Blocked by: 06、库存账核心。都在本分支工作树里。

依据：[UC-WF-004](../../../docs/application/warehouse-fulfillment/UC-WF-004-CONFIRM-RECEIPT-AND-CLOSE-OUT-AN-INBOUND-PRE-ADVICE.md) 步骤 2 至 6 里商品入库的那一半；`AT-WF-037` 至 `AT-WF-043`、`AT-WF-045`、`AT-WF-046`、`AT-WF-051` 里累计实收。不接作业事实登记面。

## 本段做什么

一条收货事实，对着一份商品入库的预报，把成立的行以收货流水入账，预报进入收货中，并形成这一次的收货差异。已有的预报读口带出累计实收和逐次收货。

整箱入库、完结、少收、差异结论、待认领到货、仓测清单，都不在本段。

## 结果

平铺：`CONFIRMED`、`EXISTING_RESULT`、`CONTENT_CONFLICT`、`CONTENT_INVALID`、`WAREHOUSE_NOT_FOUND`、`PRE_ADVICE_NOT_FOUND`、`NOT_RECEIVABLE`、`CARTON_RECEIPT_NOT_READY`。

- `CONTENT_INVALID`、`WAREHOUSE_NOT_FOUND`、`PRE_ADVICE_NOT_FOUND`、`CARTON_RECEIPT_NOT_READY` 不落册，不占事实身份。
- 预报已取消或已完结：事实落册，不入账，预报状态不变，答 `NOT_RECEIVABLE`。待认领到货那一件还没有，不要造，也不要把这个结果叫成「已转待认领到货」。
- 整箱入库的预报答 `CARTON_RECEIPT_NOT_READY`，不落册。整箱库存还没有。

行结果：`POSTED`、`UNKNOWN_GOODS`。认得出、但不在预报里的商品仍是 `POSTED`，同时记错货。

差异类型：`OVERAGE`、`WRONG_GOODS`、`DAMAGE`。待检不记破损。少收不在本段。

## 判定次序

1. 缺租户、事实身份、发生时刻、仓或预报：装配错，交 error。
2. 按事实身份认重放。内容相同答 `EXISTING_RESULT`，不再入账。内容不同答 `CONTENT_CONFLICT`，在册的与库存都不变。重放排在后面的校验之前。
3. 仓在本租户没登记：`WAREHOUSE_NOT_FOUND`。
4. 预报在本租户查不到：`PRE_ADVICE_NOT_FOUND`。预报的仓不是这一条事实的仓，或行的形态与预报的入库方式不符：`CONTENT_INVALID`，整条不落。
5. 预报是整箱入库：`CARTON_RECEIPT_NOT_READY`。
6. 整条先核，任一处不合就 `CONTENT_INVALID`，一行都不入账：库位在这个仓不存在，或所属库区不是收货暂存；认得出的商品计量单位不是当前修订的包装层级；跟踪批号或效期的没带，不跟踪的却带了；贴标别名不是该商品在事实发生时刻有效的平台商品码。库位已停用仍可入账。别名按事实发生时刻判，不按到达时刻。
7. 锁这份预报（与撤回同一把锁，先于库存锁），再 `NoteReceiptStarted`。已取消或已完结：落成 `NOT_RECEIVABLE`，不入账。
8. 同一事务里入账。成立的行合成一笔原因是收货的流水，来源事实引用是这条事实的身份。同一商品、同一批号与效期合成一个入库批次，不同库存状态各占一格。认不出的行不进流水。没有任何成立的行也答 `CONFIRMED`，预报仍进入收货中。
9. 同一笔里算差异。多收按基本单位：这一次入账之后，该商品累计实收超过预报数量的那一段，记在这一次上。错货：不在预报里但认得出的，以及认不出的（留下扫到的码或描述与数量）。破损：记为次品的数量。别的货主的别名不解析，当作认不出。

库存账的写入只用已有的 `PostStockMovementHandler`。入账失败整笔回滚，预报状态也不许单独推进。

## 读

货主口和运营查阅已有的预报读口带上：逐次收货（行结果、未入账的原因）、累计实收、这一次形成的差异（类型与数量，结论还没有就如实空着）。没有收过的预报，累计是 0，这是账算出来的，不是编的。不要加「另有到货待认领」。货主读口仍不带登记责任方；收货事实的设备与操作者只出现在运营查阅。

## 并发

同一份预报上的两次收货串行。并发各收一笔、合计超过预报时，多收记在后到的那一次。用已有的锁等待观察，不靠 sleep。

## 进展

商品入库的收货核心做成了：事实落册，成立的行入商品库存，预报进入收货中，形成多收、错货与破损。预报读口带累计实收与逐次收货。

出入：票面的结果词有 `NOT_RECEIVABLE` 与 `CARTON_RECEIPT_NOT_READY`。收货口的结果里没有这两格。预报已取消或已完结时答 `DIVERTED_TO_UNCLAIMED`，原事实保全为待认领到货，不入商品库存也不入整箱库存，预报状态不变；行上未入账的原因仍是 `NOT_RECEIVABLE`。`NoteReceiptStarted` 内部仍答 `NOT_RECEIVABLE`，那不是收货口的结果词。整箱入库不再答 `CARTON_RECEIPT_NOT_READY`，改走整箱收货。票面写整箱、完结、少收、差异结论、待认领都不在本段；整箱与转待认领后来做进同一个 `ConfirmReceiptHandler`。

代码与测试由 Grok 4.7 子代理写，主会话拆包与验收。

验证：全仓 `go test ./...` 连真库跑过并通过。仓配各包与 `cmd/parcel-api` 整包另带 `-race` 跑过。证据层级是隔离合成 `S`。演示种子没有另造。没有非作者评审。代码写完，没有进 main。
