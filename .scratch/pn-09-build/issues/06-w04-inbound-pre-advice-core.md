# 06 `PN09-W04` 第 1 段：入库预报的核心

Category: enhancement
Status: in-progress（代码写完；还没有非作者评审；没有进 main）
Blocked by: 04 第 1 段（商品）、02（仓）。都在本分支工作树里，还没进 main。

依据：[UC-WF-003](../../../docs/application/warehouse-fulfillment/UC-WF-003-ACCEPT-INBOUND-PRE-ADVICE.md)；[仓配履约 CONTEXT](../../../docs/domain/warehouse-fulfillment/CONTEXT.md)「入库预报」词条、「入库预报」生命周期与「入库与加工」里预报那几条；[ADR-0355](../../../docs/adr/0355-a-document-number-is-separate-from-internal-identity-and-is-assigned-in-the-issuing-transaction-under-built-in-rules-a-tenant-may-override.md)；[PN-09 交接](../../../docs/design/pn-09-warehouse-fulfillment-development-handoff.md) 的单据号表。

分工：拆包与验收由主会话做，代码与测试由 Grok 4.7 写。

## 本段做什么

受理、告知更新、撤回、取单据号、给收货侧留一个状态缝。不接任何接入面，不改库存账。

预报不是库存。本段落册之后，库存账的行数不变。

## 会话代决（可推翻）

1. **字面量。** 到仓方式：`FULL_CONTAINER`（整柜）、`PALLET`（托盘）、`LOOSE_CARTON`（散箱）。入库方式：`BY_GOODS`（商品入库）、`BY_CARTON`（整箱入库）。状态：`ADVISED`（已预报）、`RECEIVING`（收货中）、`CLOSED`（已完结）、`CANCELLED`（已取消）。提交途径沿用商品的 `OWNER`、`ON_BEHALF`。
2. **结果是平铺的**，不套登记册的 `NOT_ACCEPTED`：`ADVISED`、`NOTICE_UPDATED`、`CANCELLED`、`EXISTING_RESULT`、`SOURCE_CONFLICT`、`WAREHOUSE_NOT_FOUND`、`NOT_CONFIGURED`、`WAREHOUSE_NOT_ACCEPTING`、`CONTENT_INVALID`、`STATUS_MISMATCH`、`RECEIPT_STARTED`、`PRE_ADVICE_NOT_FOUND`、`NUMBERING_RULE_COLLISION`。`WAREHOUSE_NOT_ACCEPTING` 另带原因 `DEACTIVATED` 或 `AGENT`。`STATUS_MISMATCH` 与 `RECEIPT_STARTED` 回显当前状态。`CONTENT_INVALID` 逐行指出全部不合之处。
3. **代录提交时货主客户账户查不到，不是上面任何一格。** 交 `application.ErrOwnerAccountNotInTenant`，与商品代录同一哨兵。读不动走 error。
4. **批号与效期在预报上可缺。** 商品不跟踪时带了，是内容不合；跟踪时没带，照常受理（`AT-WF-028` 最后一种）。收到收货时再要求。
5. **单据号类型** `INBOUND_PRE_ADVICE`，默认模板 `IB-{yyyy}{mm}{dd}-{seq:5}`，日期时区来路是开立方（`docnumber.DateTimeZoneFromIssuer`）：交仓已登记的业务时区，没登记就留空，由编号组件改用租户的单据编号时区。两者都没有，编号组件答 `NUMBERING_TIME_ZONE_NOT_CONFIGURED`，预报照常受理，无号原因记下这一个字面量。
6. **预报标识由本上下文签发**，前缀 `IPA`（实际 `IPA-…`），放在 `internal/warehousefulfillment/adapters/identity`，与库存流水的签发器并列、不混进同一个类型。

## 身份与内容

- 身份是（租户，预报标识）。货主、仓、到仓方式、入库方式与明细在受理时固定，之后不改。
- 商品入库的一行：商品编码、计量单位、数量、批号（可缺）、效期（可缺）、箱数（可缺，只作告知）。数量为正。箱数给了就须为正。
- 整箱入库的一箱：箱号、箱单。箱号逐字保存，一份预报内不重复。箱单至少一行：商品编码与按基本单位计的正数量。
- 告知是单独的版本链，从 1 起只追加。第 1 版随受理落下。一版是预计到仓时间、到货线索、货主参考号。线索与参考号可空，逐字保存，不判唯一。更新是整组替换：没带的线索或参考号就是清空；预计到仓时间必带。
- 每笔提交、告知更新、撤回各记：途径、来源（货主口非空、代录为空）、键（货主口是来源请求键，代录是决定标识）、依据引用（只有代录有）、登记责任方、时刻。

## 判定次序

### 提交

1. 缺租户、货主、仓、到仓方式、入库方式、预计到仓时间、提交或登记责任方：装配错，交 error。
2. 代录先核货主客户账户，查不到交 `ErrOwnerAccountNotInTenant`，不读仓、不落册。货主口不问。
3. 按（租户，途径，来源，键）认重放与冲突。内容相同答 `EXISTING_RESULT` 并回显在册的那一份与原号；不同答 `SOURCE_CONFLICT`。重放不取号。
4. 仓：没登记（含本租户一个仓都没有）答 `WAREHOUSE_NOT_FOUND`；已停用答 `WAREHOUSE_NOT_ACCEPTING`/`DEACTIVATED`；代理仓答 `WAREHOUSE_NOT_ACCEPTING`/`AGENT`。
5. 明细逐行收齐：商品没登记；商品入库的计量单位不是当前修订的包装层级；数量不为正；不跟踪却带了批号或效期；箱号重复；箱单为空；箱单里的商品没登记或数量不为正。任一处不合答 `CONTENT_INVALID`，整份不落、不取号。
6. 同一事务里取号再落成 `ADVISED`，告知版本 1 同笔写入。撞号答 `NUMBERING_RULE_COLLISION`，不留预报，取过的流水不提交。

### 告知更新

1. 本租户一个仓都没登记：`NOT_CONFIGURED`。这一步在查找之前，不泄露这份预报在不在。
2. 按键认重放与冲突。
3. 预报在这个货主名下查不到：`PRE_ADVICE_NOT_FOUND`。货主口的货主对不上，与查不到同答。代录的货主取自预报，命令里不带货主。
4. `ADVISED` 或 `RECEIVING`：追加一版，答 `NOTICE_UPDATED`，回显新的现行版。状态、明细、单据号不动。与现行版内容相同也照留一版。
5. `CANCELLED` 或 `CLOSED`：`STATUS_MISMATCH`，回显状态。

### 撤回

1. 一个仓都没登记：`NOT_CONFIGURED`，同样在查找之前。
2. 查预报。查不到：`PRE_ADVICE_NOT_FOUND`。
3. 已是 `CANCELLED`：`EXISTING_RESULT`，不论这次的途径与键。
4. 这次的键已经落在另一份预报的撤回上：`SOURCE_CONFLICT`，那一份不变。
5. `ADVISED`：进入 `CANCELLED`，答 `CANCELLED`。单据号留在这份预报上，不释放。
6. `RECEIVING` 或 `CLOSED`：`RECEIPT_STARTED`，回显状态。

### 给收货侧的缝（本段不实现收货）

`NoteReceiptStarted`：`ADVISED` → `RECEIVING`，答 `STARTED`；已是 `RECEIVING` 答 `ALREADY_RECEIVING`；`CANCELLED` 或 `CLOSED` 答 `NOT_RECEIVABLE`。

`NoteClosed`：`ADVISED` 或 `RECEIVING` → `CLOSED`，答 `CLOSED`；已是 `CLOSED` 答 `ALREADY_CLOSED`；`CANCELLED` 答 `NOT_CLOSABLE`。

这两条与撤回、告知更新在同一份预报上串行。

## 一致性

- 同一键并发提交，至多一份预报、一个单据号，另一笔按内容答 `EXISTING_RESULT` 或 `SOURCE_CONFLICT`。
- 同一份预报上的告知更新按到达先后各成一版，现行的是最后到达的那一版。
- 撤回与 `NoteReceiptStarted` 并发，只成立一个：撤回先成立，收货缝答 `NOT_RECEIVABLE`；收货缝先成立，撤回答 `RECEIPT_STARTED`。

## 不在本段

接入面（客户面三个能力、代录决定词、端点、装配进 `parcel-api`）、待认领到货、收货确认本身、演示种子、管理台。类型声明要进 `cmd/internal/docnumbering.Catalogue`，否则开立时这个类型不存在；取号适配器先不接进 `parcel-api`。

## 验收

`AT-WF-026` 至 `AT-WF-032`、`AT-WF-082` 里不涉及接入面的部分。头注 `Covers: AT-WF-…（入库预报的核心）`。真库，带 `-race`。并发用已有的锁等待观察，不靠 sleep。受理之后查库存账，行数不变。

## 进展

受理、告知更新、撤回、取单据号，以及给收货侧的 `NoteReceiptStarted` 与 `NoteClosed` 做成了。预报受理不写库存账。类型 `INBOUND_PRE_ADVICE` 进了 `cmd/internal/docnumbering` 的 `Catalogue`。

与票面的判定次序和结果词一致。本段按票面没有接端点；取号适配器接进 `parcel-api`、收货入账、完结与待认领是后面的票。`NoteReceiptStarted` 在已取消或已完结时仍答 `NOT_RECEIVABLE`，那是这条缝的词，不是后来收货口的结果词。

代码与测试由 Grok 4.7 子代理写，主会话拆包与验收。

验证：全仓 `go test ./...` 连真库跑过并通过。仓配各包与 `cmd/parcel-api` 整包另带 `-race` 跑过。证据层级是隔离合成 `S`。演示种子没有另造。没有非作者评审。代码写完，没有进 main。
