# 11 `PN09-W05` 后四段：完结与少收、收货差异结论、整箱库存与整箱收货、取消或完结后到达的收货转待认领

Category: enhancement
Status: in-progress（代码写完；还没有非作者评审；没有进 main）
Blocked by: 09、10

这四段当时没有先写票面，直接按任务说明做的。本票是事后对着代码补记，不是事先的规则稿。

依据：[UC-WF-004](../../../docs/application/warehouse-fulfillment/UC-WF-004-CONFIRM-RECEIPT-AND-CLOSE-OUT-AN-INBOUND-PRE-ADVICE.md) 的完结、少收与收货差异结论；[UC-WF-005](../../../docs/application/warehouse-fulfillment/UC-WF-005-REGISTER-CLAIM-AND-DISPOSE-OF-UNCLAIMED-ARRIVALS.md) 里取消或完结后到达的收货转成待认领那一格；[ADR-0384](../../../docs/adr/0384-warehouse-access-words-customer-faces-and-the-switched-off-answer.md) 决定二、决定八；[PN-09 交接](../../../docs/design/pn-09-warehouse-fulfillment-development-handoff.md)。认领与处置不做。

分工：代码与测试由 Grok 4.7 子代理写，主会话拆包与验收。

## 做了什么

四段都挂在已有的预报读口和收货确认口上，另加完结口与结论的两个写口。路径都在 `cmd/parcel-api/endpoints_warehouse_fulfillment.go`。

- **完结与少收。** `POST /warehouse-fulfillment-inbound-pre-advice-closures`，运营决定 `INBOUND_PRE_ADVICE_CLOSE`。准入的能力词与事实类型词都是这个字面量。只完结收货中的预报。商品少收按基本单位，只留下大于 0 的；整箱少收是还没入账的预报箱号，一箱一行，数量是一箱。少收种类 `SHORTAGE`，有少收才另签一个差异标识，不写进这一次收货的差异行，也不改商品库存或整箱库存。结果：`CLOSED`（201）、`EXISTING_RESULT`、`STATUS_MISMATCH`、`PRE_ADVICE_NOT_FOUND`、`NOT_CONFIGURED`。已完结再完结回显当初那一份，说明不同也不是另一份。
- **收货差异结论。** 货主口 `POST /v1/warehouse/receipt-discrepancy-conclusions`，能力 `RECEIPT_DISCREPANCY_CONCLUSION`，准入两词都是这个字面量，来源请求键前缀 `receipt-discrepancy:`。代录口 `POST /warehouse-fulfillment-receipt-discrepancy-conclusion-on-behalf`，决定 `RECEIPT_DISCREPANCY_RECORD_ON_BEHALF`，准入两词都是这个字面量。结论是 `CONFIRM` 或 `OBJECT`，不改库存。收货形成的差异与完结形成的少收都能作结论。
- **整箱库存与整箱收货。** 没有单独的端点。整箱入库仍走 `POST /warehouse-fulfillment-receipt-confirmations`，写入另一本整箱账，不进商品库存。一箱是一件，不分库存状态，余额由流水现算。这一段流水原因只有 `RECEIPT`。一箱一个入库批次。
- **待认领。** 没有认领口。取消或完结之后到达的收货与一件待认领到货在同一笔事务里落下，收货口答 `DIVERTED_TO_UNCLAIMED`。原事实原样保全，两本库存都不入，预报状态不变。预报读口有这种到货时多一格 `unclaimedArrival`，不带内容、标识与数量；没有就不出现这一格。转成待认领的收货不放进逐次收货。

## 会话代决（可推翻）

对着代码记，当时没有先写进票面。

1. **谁取代谁。** 一份差异现行至多一条结论。还没有结论时，货主本人或代录都记下，答 `CONCLUSION_RECORDED`（201）。现行是代录的：货主本人的结论取代它，不论相同还是不同，被取代的照留。代录不取代现行：相同答 `EXISTING_RESULT`，不同答 `CONCLUSION_HELD`。现行是货主本人的不再改。货主口对不上这份差异所属的货主，与查不到同答 `DISCREPANCY_NOT_FOUND`。
2. **整箱另立一本账。** 整箱流水与整箱入库批次不并进商品库存的签发器。前缀分别是 `CMV`、`CIB`。锁是（租户，货主，仓）的整箱账，键里写明是整箱，不拿商品库存的货主锁。锁序是先预报、再整箱账。
3. **整箱一行怎么判。** 预报没列，或这份预报上已经收过，不入账，差异种类 `WRONG_GOODS`，行上原因 `WRONG_CARTON`。要入账的箱号已经在这个货主这个仓的在库箱号里，不入账，不形成差异，行上原因 `CARTON_NUMBER_IN_USE`。破损的入账箱另记 `DAMAGE`。收货库位须是收货暂存。
4. **少收另签差异标识。** 标识前缀 `RDP`，与这一次收货的差异共用签发器，但是完结自己的一行，不进 `receipt_discrepancy` 那种按收货事实挂的差异行。都到齐就不签。
5. **待认领只标有无。** 到货标识前缀 `UCA`，由本上下文签发。预报读口只回答有没有指着这份预报的待认领到货，不把到货的内容、标识与数量交出来。
6. **结果词换过。** 收货口不使用票 09 写过的 `NOT_RECEIVABLE` 与 `CARTON_RECEIPT_NOT_READY`。已取消或已完结走 `DIVERTED_TO_UNCLAIMED`。整箱入库走整箱收货，答 `CONFIRMED` 或同一口上的其他结果。

## 没做的

- 待认领到货的认领与处置。不指向一份预报的无主到货登记。
- 开箱。商品库存账对开箱仍答 `REFUSED` / `REASON_NOT_SUPPORTED`。
- 整箱的取走、占用，以及整箱或商品库存的读口端点。
- 结论不改库存，也不做开箱带来的差异。
- 演示种子、管理台页面。

## 验证

全仓 `go test ./...` 连真库跑过并通过。仓配各包与 `cmd/parcel-api` 整包另带 `-race` 跑过。证据层级是隔离合成 `S`。演示种子没有另造。没有非作者评审。代码写完，没有进 main。
