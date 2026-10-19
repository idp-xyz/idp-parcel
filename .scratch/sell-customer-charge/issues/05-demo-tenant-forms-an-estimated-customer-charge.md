# 05 演示租户：登 SELL 计费时点采用、费用项目映射与价卡取整，演示包裹形成预估客户费用

Category: enhancement
Status: resolved——2026-10-19 进 main（全量钉 `864248ab`；与 02、03、04 同批，见文末「完成记录」）。
Blocked by: [02](./02-request-sell-evaluation-on-parcel-moment.md)、[03](./03-pricing-consumes-sell-evaluation-requests.md)、[04](./04-sell-evaluation-recorded-forms-estimated-customer-charge.md)
地盘：`scripts/demo-seeds`（seedgen 的售价卡、`data/settlement/` 新增两份、`seed.sh` 调用行、README）；演示动线脚本补记。
出处：只收 demo-chain-gaps/09 的 SELL 那一半与 10 的 SELL 那一半；[ADR-0190](../../../docs/adr/0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md)；[demo-chain-gaps/09](../../demo-chain-gaps/issues/09-demo-price-cards-declare-no-amount-rounding.md)、[10](../../demo-chain-gaps/issues/10-demo-tenant-buy-and-sell-trigger-registers-are-empty.md)。

## 做什么

- 售价卡声明金额取整（总额、0.01 CNY、四舍五入），就地改 v1 后用 `seed.sh --reset` 重灌；另登 v2 会撞适用冲突。
- `SYN-SETTLEMENT-ACCOUNT-01` 采用 `NETWORK_INTAKE_ADOPTED`，计价范围与受理前估价采用册登的那一个相同（ADR-0190 越权风险点 9）。
- `BASE_FREIGHT` 与 `REMOTE_AREA_SURCHARGE` 都映到同一个合成费用项目（`SYN` 简化，ADR-0190 越权风险点 2）。

## 完成判据

- 一次性库实走（`S`）：提交、受理、收寄、采用之后，演示包裹形成一笔预估客户费用，金额与受理前冻结同额（今天没有实测读口，两边都用申报，故同额）。走完删库。

## 完成记录

2026-10-19 Claude Code 会话落地（用户「你作为业务和系统专家自决」授权）。代码两笔（`7a090edc` 种子，`90f4800c` 模板导入夹具），与票 02–04 同批进 main：

- 售价卡声明金额取整：`seedgen` 的 `sellCardSnapshot` 加总额、0.01 CNY、四舍五入一格，`price-card-cn-sg.json` 重生成。模板导入的演示售价卡夹具同步声明，与种子快照仍是同一摘要。
- `data/settlement/` 新增三份：`05` SELL 计费时点采用（`SYN-SETTLEMENT-ACCOUNT-01`，`NETWORK_INTAKE_ADOPTED`，计价范围 `SYN-SCOPE-01`，与受理前估价采用同值）；`06`、`07` 两条费用项目映射（`BASE_FREIGHT`、`REMOTE_AREA_SURCHARGE` 都映到 `SYN-FEE-ITEM-FREIGHT-01`）。`seed.sh` 各加调用行，README 的 `data/settlement/` 行与参数登记册 `PAR-SET-02` 补登记口指针。

成本卡没动：BUY 那一半归 [demo-chain-gaps/09](../../demo-chain-gaps/issues/09-demo-price-cards-declare-no-amount-rounding.md)，上游还挡在 [07](../../demo-chain-gaps/issues/07-transport-charge-occurrences-have-no-production-path.md)、[08](../../demo-chain-gaps/issues/08-buy-evaluation-recorded-consumer-never-forms-supplier-expected-cost.md)。

### 实走（`S`）

一次性库 `sell05b`，在批次分支（票 02–05 叠在一起）上按 `seed.sh` 原样灌，退 0。`parcel-api` 与 `parcel-dispatch` 照演示动线「前置」起。

1. 委托 `SUBMITTED`，初始路由 `ROUTE_FORMED`。
2. 上海枢纽设备用客户包裹号收寄，答 `INTAKE_FORMED`；`parcel-shipment.network-intake.recorded` 发布，路由复核 `STILL_APPLICABLE`。
3. 同一封采用信封扇出到 SA：请求册落一份 `NETWORK_INTAKE_ADOPTED` 的 SELL 请求（结算币种 CNY，计价范围 `SYN-SCOPE-01`），`settlement-accounting.sell-evaluation-request.submitted` 发布。
4. PP 按请求形成 SELL 评价，`parcel-pricing.evaluation.recorded` 发布。BUY 与 SELL 两路 inbox 各收一次。
5. SA 形成预估客户费用 `SELLCC-<请求标识>`：费用项目 `SYN-FEE-ITEM-FREIGHT-01`，阶段`预估`，CNY 9600（最小币单位，即 96.00），与受理前冻结同额。请求记`已形成`并带这笔费用的标识。
6. 末端交接、派送发起、派送尝试、有效交付，PS 终局 `EFFECTIVE_DELIVERY` 发布。终局那一路入账：这一户只采用了收寄时点，没有第二份请求。走完请求册一行、客户费用一行。

派发日志里的失败只有 `dispatch.no_subscriber` 四类（治理暂停与恢复、可达判断、客户视图），与 2026-10-18 那次重走相同，不是本批引入的。走完已停进程、删库。

### 进 main

全量 `go test -p 1 -count=1 ./...`（带 55432 DSN）在干净游离检出上钉 `864248ab`：144 包 ok，0 FAIL。机制清点在同一 tip 上重生成。
