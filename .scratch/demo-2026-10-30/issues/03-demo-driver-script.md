# 03 演示驱动脚本

Category: task
Status: in-progress——2026-10-28 作者完工，待协调方复核与进 main，分支 claude-demo-drv（最后一笔见下方完成记录）
此前：in-progress——2026-10-28 tops-20（idp-parcel-91）立并认领，分支 claude-demo-drv
Blocked by: 无

## 现象

审计：提交、撤回、取消在管理台必 401（first-party-api/05，冻结），要用终端；班次、预占、装载分配、各次交接、干线委托、订舱、承运答复、段采用、派送、有效交付、放行结果、门禁核对、费用确认补拍、对账单发布、供应商账单、收寄后处置都没有现成脚本，载荷散在动线文档补记与 psb/05 各节。演示账户余额只够约两件在途。现场逐条敲风险高。

## 做什么

只改 `scripts/demo-seeds/`（新子目录如 `demo-run/`），不改 Go：

1. `up.sh`：照动线「前置」起发行方、`parcel-api`、`parcel-dispatch`（环境变量全齐、日志落文件、PID 记下）、灌种子到指名库；经生产口登一笔外部收款并映射，把演示账户可用余额补到能连发多件；`down.sh` 收干净。
2. 分段脚本，每段可单独跑、打印清楚（每步一行：做了什么 → 关键答复字段），失败即停：主链（提交 → 预报收寄 → 自营段 → 干线（委托先于交出）→ 尾程派送 → 有效交付 → 客户轨迹视图）；关务（立案、授权、提交 → 发送（含合成 broker 起停）→ 放行 → 门禁核对 → 交出读数）；结算（费用确认读数 → 对账单发布 → 供应商账单接收与审核）；异常（另一件包裹的退运决定 → 人工项清单 → 登记 → 读回；客户渠道撤回、取消）。班次日期用当天。
3. `README.md`：先跑哪几段预热、现场跑哪段、每段在管理台哪页看结果（引 `../../.scratch/demo-2026-10-30/ui-coverage-audit.md` 的页面 id）。

## 完成判据

- 在一次性库上从 `up.sh` 起依次跑完全部分段零失败、零绕法，`down.sh` 收干净；输出贴进完成记录（只记 `S`）。

## 完成记录（2026-10-28，作者子代理）

**起点** `origin/main` `5d5a4484b`。**改了什么**：只加 `scripts/demo-seeds/demo-run/`（`up.sh`、`down.sh`、`topup-funds.sh`、`10-main-chain.sh`、`20-customs.sh`、`30-settlement.sh`、`40-exceptions.sh`、共用的 `lib.sh` 与 `chain.sh`、`README.md`）和 `.gitignore` 一行（`.run/`）。不改 Go、不改种子、不改既有脚本。

**自验**：一次性库 `demo03`（55432 上 `idp-parcel-postgres-gate`）从 `up.sh demo03` 起依次跑完 `10`、`20`、`30`、`40`，零失败、零绕法（没直写库、没补登种子之外的行；唯一的受控 CLI 是映射余额用的 `parcel-settlement-register funds-mapping`，映射本来就没有在线口）；`20` 重跑被脚本自己拦下（一库一件）；`down.sh --drop-db` 收干净（进程停、库删、`.run/` 清）。共享集成客户端发行方这一轮没答 `invalid_client`，没起私有的（私有分支只写未实走）。证据只记 `S`。输出摘要：

- `up.sh`：新建库、seed.sh 退 0、编译、`parcel-api`（:19080）与 `parcel-dispatch` 起，日志出现隔离读、隔离写、治理自授权、操作者渠道、集成客户端渠道各声明；补余额 `201 FUNDS_FACT_ADOPTED` + `FUNDS_MAPPED`。
- `10`：`201 SUBMITTED` → `ACCEPTED` → `ROUTE_FORMED`（`RPV-000000000001`）→ 三段 `OPPORTUNITY_PREPARED`（#1 `SYN-SCHED-SHA-SZX-DAILY/2026-10-28`、#2 `SYN-SCHED-SZX-SIN-EXPRESS-1800/2026-10-29`）→ 预报 `DELIVERY_PRE_ADVICE_REGISTERED` → 收寄 `201 INTAKE_FORMED`/`VERIFIED` → SELL `ESTIMATED` 9600 → 班次 `201`、两池预占 `201`、装载分配 `201` → 交出 `201`/`NOT_GUARDED`、问询 `200`、出发 `201`、交进口岸 `201`/`PARTICIPATION_ENDED` → 干线委托 `v3` `201`、订舱 `201`、承运答复 `201`、交给干线承运商 `201`、段采用 `201`、交进新加坡枢纽 `201` → 尾程交接 `201`、派送任务、问询 `200`、尝试 `201`、有效交付 `201` → 投影含 `EFFECTIVE_DELIVERY` → BUY CNY 8500 → 客户视图 `200 CURRENT_VIEW`（终局 `SHOWN`，ETA `PENDING_CONFIRMATION`）。
- `20`：同上到收寄，再 立案/授权确认/申报提交（`REGISTERED_PENDING_SEND`）→ 合成服务商收到、技术回执 `RECEIPT_RECORDED`/`ACKNOWLEDGED` → 放行结果 `201 RESULT_RECORDED` → 放行门禁核对 `201 RECORDED`、`MET` → 交给干线承运商 `gateReading.answer=ADMITTED` → 走到有效交付与客户视图。
- `30`：费用 `CONFIRMED` 9600 → 补拍 `200 ALREADY_CONFIRMED` → 对账单 `201 STATEMENT_PUBLISHED`（合计 9600，册里读回行数 1）→ 供应商账单 `201 BILL_RECEIVED` → `201 PAYABLE_FORMED`（8500 CNY）→ 接收册 `matchCount` 1。
- `40`：退运决定 `201 DECIDED`（三项所有者请求）→ 清单两项（NETWORK_ROUTING、NODE_OPERATIONS）→ 登记 `HANDLED`、`UNABLE` 各 `201` → 重登 `200 ALREADY_REGISTERED` → 读回清单清空；客户渠道五口各一次成立一次重放，两条 404、一条 400 反例符合契约。

**代决与注意**（越权风险点）：
1. 关务段一库一件按「不改种子」处理——本段每库只走一次，再走先 `up.sh … --reset`；脚本拦重跑。若要现场多次演示关务，需另票往种子里登第二个申报单元及其门禁目录行。
2. `up.sh` 判断两只共享发行方已在跑就不跑 `docker compose up`：第一次试跑时我直接 `up -d --wait`，compose 因挂载路径（本 worktree）与原容器不同把共享的两只发行方重建了（令牌作废）。已改成先探测再决定。两只容器现在挂载的是本 worktree 的 `dex.yaml`/`integration-dex.yaml`，worktree 清理后重启这两只容器前须重新 `docker compose -f scripts/demo-seeds/operator-channel/compose.yaml up -d` 从主检出路径重建。
3. 客户费用与供应商预期成本两册的行不带包裹号，脚本用「提交前记下已有行、终局后新出现的那一行」关联到这一件包裹；本机顺序跑没问题，并发多会话同库跑会错配。
4. 登记册里的 `declare-one-parcel.sh`、`send-one-declaration.sh`、`walk-customer-command-faces.sh` 仓里没有可执行位，本目录用 `bash` 调它们，没改文件模式。

**判据对用例**：完成判据两条（一次性库从 `up.sh` 起依次跑完全部分段零失败、零绕法；`down.sh` 收干净、输出贴进记录）满足。

**留给协调方**：管理台读不到的几处（申报发送与回执、放行结果、交出读数、冻结/暴露/余额、审核应付、退运清单）README 已写「无页面，看终端输出」，不立票；上条越权风险点 2 的发行方容器重建需知悉。
