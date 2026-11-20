# 10 `PN09-W05` 第 2 段：收货确认的作业事实口

Category: enhancement
Status: in-progress（代码写完；还没有非作者评审；没有进 main）
Blocked by: 09

依据：[UC-WF-004](../../../docs/application/warehouse-fulfillment/UC-WF-004-CONFIRM-RECEIPT-AND-CLOSE-OUT-AN-INBOUND-PRE-ADVICE.md) 的收货确认口；[ADR-0384](../../../docs/adr/0384-warehouse-access-words-customer-faces-and-the-switched-off-answer.md) 决定三、决定八。

只接这一口。完结、差异结论、整箱收货、待认领到货都不在本段。

## 词

- 作业范围种类加 `WAREHOUSE`。一台设备仍只登记一个范围。节点设备拿来收货，答设备未登记或已停用。
- 作业事实类型加 `RECEIPT_CONFIRMATION`，并且这一口在铸造侧判准入。能力词沿用作业事实登记面已有的 `WORK_FACT_REGISTRATION`，事实类型词是 `RECEIPT_CONFIRMATION`。不要把两格都写成事实类型。

## 路径

`POST /warehouse-fulfillment-receipt-confirmations`，挂在仓配那一份端点登记里。一路径一行。开关关着在认证之前答 `403 WAREHOUSE_FULFILLMENT_NOT_ENABLED`。

设备、作业范围种类、作业范围引用走作业事实既有的请求头。种类必须是 `WAREHOUSE`，引用必须等于载荷里的仓。事实身份与发生时刻在载荷里，服务端不重签。租户、操作者、设备不在载荷里，自带即坏载荷。

调用已有的 `ConfirmReceiptHandler`。新建 `201`，其余业务结果 `200`。问题详情用作业事实族已有的码：未授予、设备未登记或已停用、不在准入范围、令牌无效、渠道未配置。

## 进展

`POST /warehouse-fulfillment-receipt-confirmations` 接上。作业范围种类 `WAREHOUSE`，引用须等于载荷里的仓。铸造侧能力词是 `WORK_FACT_REGISTRATION`，事实类型是 `RECEIPT_CONFIRMATION`。新建的 `CONFIRMED` 答 201，其余业务结果答 200。

出入：票面写完结、差异结论、整箱收货、待认领都不在本段，调用当时已有的 `ConfirmReceiptHandler`。端点仍是这一个。这个处理器后来会把整箱入库写入整箱账，并把已取消或已完结的收货答 `DIVERTED_TO_UNCLAIMED`。

代码与测试由 Grok 4.7 子代理写，主会话拆包与验收。

验证：全仓 `go test ./...` 连真库跑过并通过。仓配各包与 `cmd/parcel-api` 整包另带 `-race` 跑过。证据层级是隔离合成 `S`。演示种子没有另造。没有非作者评审。代码写完，没有进 main。
