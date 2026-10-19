# 43 演示成本卡绑分区目录：干线发生项往下的 BUY 评价越过 ZONE_UNRESOLVED，形成供应商预期成本

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcgrfc20（2026-10-19 作者子代理，与 routing-first-cut/20 同一分支、在它之后落，见文末「完成记录」）。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [27](./27-transport-charge-occurrences-are-registered-and-handed-off.md) 的隔离评审：27 的完成判据「BUY 评价 → ADR-0200 形成供应商预期成本」没有走通，移交本票。Blocked by 解除之前不认领。
Blocked by: 无（[routing-first-cut/20](../../routing-first-cut/issues/20-bound-cost-cards-fold-zones-through-the-parcel-postal-route.md) 同一分支先落）
地盘：同票 [30](./30-buy-cost-card-declares-amount-rounding.md)——`scripts/demo-seeds/data/pricing/price-card-cn-sg-cost.json`、`scripts/demo-seeds/seedgen` 里成本卡与模板夹具同摘要的那一处、`seed.sh` 一行（若需要）；README「已知边界」里票 27 记的那一条。
出处：票 27 完成记录「偏离与待裁」第 2 条（甲路）；票 30 完成记录「待裁」；[ADR-0189](../../../docs/adr/0189-leg-pricing-zones-are-supplied-per-card-and-unbound-cards-take-a-registered-leg-zone.md) 决定一第一条、越权风险点 3。

## 现象

演示走到干线 `ACTUAL_FULFILLMENT` 发生项登成（201）之后，结算发起一份 BUY 请求、计价形成一份评价，评价答 `PENDING` / `ZONE_UNRESOLVED`（「the price card binds no zone catalogue and the input names no zone」）。结算按 ADR-0200 把待判断入账，供应商预期成本 0 行。

## 证据（票 27 实测，一次性库，证据 `S`）

- 演示成本卡 `price-card-cn-sg-cost.json` 不绑分区目录。
- 计价按发生项造的输入取成员包裹的申报与邮编路线（`200000`→`018000`），不带分区。卡没绑目录、输入也没给分区时，分区解不出，评价不形成。

## 三类

租户取值（演示租户的成本卡绑哪一份目录）。机制已在：ADR-0189 决定一第一条「卡绑目录、经邮编路线解区」。规模 S。

## 最小的诚实修法（票 27 的甲路）

演示成本卡绑分区目录 `SYN-CAT-ZONE-CN-SG`。按发生项造的输入已带邮编路线，经目录解出 `Z1`，与今天三段各 `Z1` 同价。改成本卡与 seedgen 里同摘要的模板夹具；在用解析多于一版即适用冲突，所以照票 30 的先例就地改 `v1`，不另发一版。

**为什么要等 routing-first-cut/20**：在它落地之前，路由的候选成本取数不经读数补全，绑了目录的卡恒答待判断，初始路由会退回 `CANDIDATE_COSTS_PENDING`——修了 BUY 这一头，演示的第一步反而断了。

## 完成判据

- 演示动线：干线参与结束后登 `ACTUAL_FULFILLMENT` 答 201 → BUY 请求 → 计价评价形成（不再是 `ZONE_UNRESOLVED`）→ ADR-0200 形成一行供应商预期成本。证据 `S`。
- 初始路由仍形成，不退回 `CANDIDATE_COSTS_PENDING`。
- 一次性库 `seed.sh` 退 0；`./scripts/demo-seeds/...` 绿。
- README「已知边界」里票 27 记的那一条随之删去或改写。

## 不在本票

乙路（按发生项取段计价区域：实际履约段经计划段连到 NR 的线路段，再读段计价区域）是跨 PP、SA、NR 的产品机制，要新 ADR，留给那几位 owner。真实租户若用按线路计价、不绑目录的干线卡，迟早要它；ADR-0189 越权风险点 3 一直挂着。

## 既有票

[routing-first-cut/20](../../routing-first-cut/issues/20-bound-cost-cards-fold-zones-through-the-parcel-postal-route.md)（前置）；[30](./30-buy-cost-card-declares-amount-rounding.md)（同一张卡的金额取整）。

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcgrfc20`，基 `5f0aa66d`，叠在 routing-first-cut/20 之后）

授权：用户原话「你作为业务和系统专家自决……解决所有的问题」。提交 `0ee61489`、`4fbf0633`。

### 做了什么

- `scripts/demo-seeds/seedgen` 的 `buyCardSnapshot` 绑分区目录 `SYN-CAT-ZONE-CN-SG`，重跑生成器，只有 `price-card-cn-sg-cost.json` 变，摘要换新。
- 同摘要的几处跟上：
  - `pricecardtemplate` 夹具 `costCard` 补 `reference_catalogues` 一行，「模板导入与种子同摘要」照旧绿；
  - 模板规范第八节成本卡样例补那张表与一句说明。
- 依赖演示成本卡的两条用例改为经邮编路线读区：
  - SA `TestTheDemoCostCardYieldsAnAdoptableBuyEvaluation`：输入改成邮编路线 200000 → 018000，加上从演示目录解出的读数，形状照正式评价补齐读数的结果；
  - `cmd/parcel-dispatch` `TestADeclaredParcelOnZonedLegsIsPricedThroughProductionWiring`：登目录及其复核，请求带邮编路线。
- 种子 README：
  - 组成表计价一行写成本卡也绑这本目录；
  - 网络一行写明三段段计价区域自此没有读者；
  - 数据故事第 2 步改「两张卡都绑这本目录」；
  - 「已知边界」里票 27 记的那一条删去。
- 演示动线脚本补记一次性库实走。

### 代决

- **就地改 v1，不另发 v2**：照票 30 与 sell-customer-charge/05 的先例。在用解析多于一版即适用冲突，路由取候选遇同一方案两版报歧义。`seed.sh` 不加行。
- **段计价区域的种子三行保留、不删**：删要改 `seed.sh` 既有一行与 `data/network/`，那是在途票 64 的地盘。这三行对绑了目录的卡没有读者，README 写明了。
- **实走看自动形成的那一项，不人工登 `ACTUAL_FULFILLMENT`**：票面判据一写「登 `ACTUAL_FULFILLMENT` 答 201」，那是票 67 之前的走法；票 67 之后段采用了委托，参与结束即由派发自动形成，人工再登同一项只答 `200 EXISTING`。
- **立票 [104](./104-unbound-buy-cost-cards-get-no-zone-on-the-formal-evaluation.md)**：票面「不在本票」的乙路此前没有承接票，给它一个去处，`needs-triage`，要新 ADR。

### 越权风险点

1. 已灌过旧种子的库，再跑 `seed.sh` 会在成本卡那一行答内容冲突（退出码 2），只能 `--reset` 或换新库（演示租户 owner；与票 30 同一处境）。
2. 演示成本卡三段同引、按目的邮编分区，三段同区。中间段并不送到收件人，是 `SYN` 简化（ADR-0189 文末补记越权风险点 7）。
3. 种子里三段段计价区域成了没有读者的演示行（演示租户、NR owner），去留并入票 104 一起看。

### 验证

- 一次性库 `rfc20_walk43` 实走（实测，只记 `S`）：
  - 环境：`seed.sh` 灌入退 0。`parcel-api`（`:19181`）照动线「前置」起，`parcel-dispatch` 照「前置」七个变量起，拍间隔 2s。操作者发行方用共享的 5556。共享的 5558 对合成货主系统与干线承运客户端都答 `invalid_client`，照 psb/05 最新重走另起私有 Dex：同一份 `integration-dex.yaml`，宿主端口 5571，`iss` 不变，两族 JWKS 指 5571。
  - 提交与受理：`submit-one-shipment.sh` 答 `201`，委托 `SHR-7VZ6SKBOOHYFX3TZMBHX6BIDS4`，包裹 `DPCL-3QVTGZDHL5KDU6WVLDBMWMWWLI`，申报 1.5 KG。
  - 初始路由：下一拍 `ROUTE_FORMED`，不退回 `CANDIDATE_COSTS_PENDING`（判据二）。计划 `RPV-000000000001` 三条成本出处各 85 CNY、卡 `SYN-PLAN-CN-SG-COST-01/v1`，评价标识 `EST-67679ed3c509388e`。
  - 干线三步：委托 `SYN-COMMISSION-TRUNK-RFC20`（`v3`）答 `201 COMMISSION_SUBMITTED`；华东场外设备在 SZX 交接立段 `SYN-SEG-SZX-SIN-RFC20` 答 `201`，读数 `UNDECIDED`；采用答 `201 COMMISSION_ADOPTED`。
  - 参与结束：新加坡尾程场外设备在 SIN-HUB 交接答 `201`，`PARTICIPATION_ENDED`。
  - 往下几拍（判据一）：
    - 实际履约发生项 `TCO-4a2c0636…`，`ACTUAL_FULFILLMENT`、1 `PIECE`、协议 `SYN-SUPPLIER-TRUNK-CN-SG-01/v3`，登记者是系统；
    - BUY 请求 `EVREQ-TF6FCSCUDAGMCWIX6HXRH2VYBM`；
    - 计价评价 `EVAL-LH6SGYOKZP6PGVDZKH7J6CYVCQ` `COMPLETED`，没有问题项，解释里「reference catalogue ZONE resolved to Z1 from SYN-CAT-ZONE-CN-SG@v1 for destination 018000」；
    - 供应商预期成本一行 `BUYSC-EVREQ-…`，CNY 8500 分，购买规则 `SYN-PLAN-CN-SG-COST-01@v1`。
  - 信封：接受决定、初始路由、两封交接、发生项、评价请求、评价各信封都 `PUBLISHED`。dispatch 日志除已知的 `no_subscriber` 外没有告警。
  - 收尾：走完已停进程、删私有 Dex 容器、删库，二进制与令牌已清。
- 用例与包级测试见票 routing-first-cut/20 完成记录「验证」，同一次跑，含 `./scripts/demo-seeds/...` 与 `./internal/parcelpricing/adapters/pricecardtemplate/`。

### 偏离与待裁

- 偏离：判据一「登 `ACTUAL_FULFILLMENT` 答 201」改看自动形成的那一项，见代决。
- 待裁（协调方）：psb/05「重走（钉 `8197fa2f`）」停点四可据本票标为已解。psb/05 票面不在本票地盘，没改。
- ADR-0207 Consequences 里「评价停在待判断 `ZONE_UNRESOLVED`」是那张票落地时的事实，已接受 ADR 正文不改。
