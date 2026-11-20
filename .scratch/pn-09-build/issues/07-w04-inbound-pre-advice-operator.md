# 07 `PN09-W04` 第 2 段：入库预报的运营代录与运营查阅

Category: enhancement
Status: in-progress（代码写完；还没有非作者评审；没有进 main）
Blocked by: 06

依据：[UC-WF-003](../../../docs/application/warehouse-fulfillment/UC-WF-003-ACCEPT-INBOUND-PRE-ADVICE.md)；[ADR-0384](../../../docs/adr/0384-warehouse-access-words-customer-faces-and-the-switched-off-answer.md) 决定二、决定四、决定八。

货主口（`/v1/warehouse/inbound-pre-advices` 与三个客户面能力）不在本段。

## 本段做什么

一种运营决定「入库预报代录」，三个写口与三个读口，接到 `parcel-api`。取号适配器一并接上，否则受理取不到号。

## 词与路径

- 决定种类 `INBOUND_PRE_ADVICE_RECORD_ON_BEHALF`。迁移重建 `operator_grant_decision_kind` 的 CHECK，只加这一个词，不删已有的。
- 准入的能力词与事实类型词都是这同一个字面量。`checkAdmissionRequirement` 认这一对，别的决定种类仍走能力面那个词。
一路径只登记一行，一行只有一个方法、一个接入面（ADR-0222 决定三、决定六）。写口与读口因此各占各的路径，不在同一路径上用方法区分。

- 写口都挂这一种决定，都判准入：
  - `POST /warehouse-fulfillment-inbound-pre-advice-on-behalf` 代录提交。载荷带货主客户账户、决定标识、依据引用、仓、到仓方式、入库方式、明细、预计到仓时间，线索与参考号可缺。
  - `POST /warehouse-fulfillment-inbound-pre-advice-notice-on-behalf` 代录告知更新。载荷带预报标识、决定标识、依据引用、预计到仓时间，线索与参考号可缺（没带就是清空）。带了货主客户账户即坏载荷。
  - `POST /warehouse-fulfillment-inbound-pre-advice-withdrawal-on-behalf` 代录撤回。载荷带预报标识与依据引用。不带决定标识：键用预报标识本身。带了货主客户账户或决定标识即坏载荷。
- 读口挂主数据与运营查阅读，不判准入：
  - `GET /warehouse-fulfillment-inbound-pre-advices?ownerAccount=` 一个货主名下的预报，分页。
  - `GET /warehouse-fulfillment-inbound-pre-advice?ownerAccount=&preAdviceId=` 一份。找不到 `found: false`，不答 404。
  - `GET /warehouse-fulfillment-inbound-pre-advice-by-number?documentNumber=` 按单据号逐字找，只在本租户。没有号的预报不出现在这一口。找不到 `found: false`。
- 代录提交时货主客户账户查不到：`404 OWNER_ACCOUNT_NOT_VISIBLE`，不带 outcome。告知更新与撤回查不到预报：`200`，outcome `PRE_ADVICE_NOT_FOUND`。
- 新建答 201，其余业务答复 200。问题体用仓配既有的 `{"error":{"code"}}`。开关关着仍是 `403 WAREHOUSE_FULFILLMENT_NOT_ENABLED`。
- 日志不带依据引用、到货线索、货主参考号。

## 单据号那一句

ADR-0355 决定五的表仍是原来的十一类，正文不动。`Status` 行索引注记与 Links 各加一条前向指针：那十一类不变，另加 `INBOUND_PRE_ADVICE`（模板 `IB-{yyyy}{mm}{dd}-{seq:5}`，按日，时区来路是开立方），出处是 PN-09 交接与本票。改完跑 `go run ./tools/adr-index -check`。

## 不在本段

货主口、三个客户面能力、来源请求键前缀进保留表、待认领标记、演示种子。

## 进展

三个代录写口、三个运营查阅和取号适配器接进 `parcel-api`。决定种类 `INBOUND_PRE_ADVICE_RECORD_ON_BEHALF`，准入的能力词与事实类型词都是这个字面量。

出入：票面要求改 ADR-0355 的 `Status` 索引注记与 Links，并跑 `go run ./tools/adr-index -check`。那些 `docs/` 改动已经撤回，`docs/` 与基线一致，没有新 ADR。单据号类型写在代码和 `dox/contexts/warehouse-fulfillment.md` 的入库预报一节。

同一批读口后来带上累计实收、逐次收货、完结与少收、差异结论和 `unclaimedArrival`。那是收货、完结与待认领加在同一路径上，不是本段当时的读口。

代码与测试由 Grok 4.7 子代理写，主会话拆包与验收。

验证：全仓 `go test ./...` 连真库跑过并通过。仓配各包与 `cmd/parcel-api` 整包另带 `-race` 跑过。证据层级是隔离合成 `S`。演示种子没有另造。没有非作者评审。代码写完，没有进 main。
