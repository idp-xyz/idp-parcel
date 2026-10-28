# 演示驱动脚本（`demo-run/`）

## 共享发行方

`up.sh` 对已在跑的演示发行方 5556、5558 不重建（只探测 `/keys`，缺哪只才跑 compose）。若确需重建，必须从主检出 `/home/tops/workspace/idp-parcel` 跑 `docker compose -f scripts/demo-seeds/operator-channel/compose.yaml up -d`：挂载的绝对路径进配置哈希，换一棵 worktree 去跑会把容器改挂到那棵树上。重建会作废所有会话手里的令牌，先通知再动。

票 `demo-2026-10-30/03`。把首家客户演示要点的动线做成可重复跑的分段脚本，不再现场逐条敲 curl。**仅限本机隔离环境**，全部是 `SYN-` 合成值，证据只记 `S`。

## 起、收

**库名只用自己的**（建议 `demo_` 前缀）：`up.sh <库名> --reset` 会清空指名库的全部数据，别指别人的库。

**关联只适用单会话顺序跑**：客户费用与供应商预期成本两册的行不带包裹号，脚本靠「提交前记下已有行、终局后取新出现的一行」关联到这一件；并发跑，或别的会话同时写同一个库，会认错行。

不带 `--reset` 对已有库重跑 `up.sh`（先 `down.sh` 不删库）是幂等的：补余额用固定事实号与固定时刻，重放答 `200 EXISTING_FUNDS_FACT`、映射答 `EXISTING_MAPPING`，照常退 0。

```bash
scripts/demo-seeds/demo-run/up.sh <库名> [--reset]   # 例：up.sh demo_a --reset
scripts/demo-seeds/demo-run/down.sh [--drop-db]
```

- `up.sh` 依次：建库（不存在就建；已存在不带 `--reset` 则沿用已灌的种子）、起/确认演示发行方（5556、5558 已在跑就不碰；集成客户端发行方答 `invalid_client` 时改起私有的 5559，`iss` 串不变）、`seed.sh`、编译、起 `parcel-api`（`:19080`，三族发行方、治理自授权、隔离读写两个开关都是 `SYN-TENANT-01`）与 `parcel-dispatch`（七变量齐，拍间隔 2s），最后补演示账户余额（`topup-funds.sh`：合成银行客户端经 `/settlement-external-funds-fact-registrations` 登 100000，再用 `parcel-settlement-register funds-mapping` 映射；种子余额 20000，每件冻结 9600，补完约够十二件）。
- 运行态都在 `demo-run/.run/`（已进 `.gitignore`）：`bin/` 二进制、`parcel-api.log`、`parcel-dispatch.log`、`*.pid`、`env.sh`（进程与发行方坐标）、`state.env`（各分段写、后段读的状态）。每次 `up.sh` 清空 `state.env`。
- `down.sh` 停进程与合成报关服务商、删私有发行方容器、清 `.run/`；库默认保留，`--drop-db` 才删（只删 `up.sh` 自己新建的库）。共享发行方 5556/5558 不停。
- 余额再用尽：`scripts/demo-seeds/demo-run/topup-funds.sh 100000 02`（换一个后缀再补）。

## 分段（先后与是否可单跑）

每步打印一行「步骤 → 关键答复字段」，状态码或 `outcome` 不对就停并打印该步原始响应。班次日期取运输准备对该包裹选中的班次（第一段是上海当地当天）。每段都可单独跑，只有 `30` 需要先有 `10`。

| 脚本 | 做什么 | 耗时（约） |
|---|---|---|
| `10-main-chain.sh` | 提交 → 受理 → 初始路由 → 运输准备 → 送站预报 → 收寄 → 自营段（班次、两池预占、装载分配、交出、问询、出发、交进口岸）→ 干线（委托先于交出、订舱、承运答复、交给干线承运商、段采用、交进新加坡枢纽）→ 尾程 → 派送 → 有效交付 → 终局 → 客户轨迹视图。每跑一次一件新包裹 | 40 s |
| `20-customs.sh` | 关务专项包裹：同上到收寄，再立案、授权确认、申报提交 → 起合成报关服务商，dispatch 发送，送技术回执 → 放行结果 `FULL` → 放行门禁核对 `MET` → 自营段 → 干线（交给干线承运商，交出门禁读数 `ADMITTED`）→ 尾程 → 交付。**每库只能走一次**（见下） | 40 s |
| `30-settlement.sh` | 对最近一次主链：费用确认读数 → 补拍答 `ALREADY_CONFIRMED` → 发布对账单并读回 → 供应商账单接收（`MATCHED`）→ 审核成应付 → 读接收册。同一件主链只能走一次 | 5 s |
| `40-exceptions.sh` | 甲：另一件包裹收寄后退运决定 → 待人工清单 → 登记 `HANDLED`/`UNABLE` → 重登 → 读回；乙：客户渠道补充、撤回、修订、索赔、取消（`walk-customer-command-faces.sh`）| 25 s |

**关务段一库一件的处理。** `declare-one-parcel.sh` 写死一个申报单元与义务引用，同库第二件关务包裹立案会答 `200 SCOPE_CONFLICT`。要换单元得往种子里再登一组单元就绪与门禁目录行，那是改种子，不在本票范围。所以 `20` 在一个库里只走一次，已走过会直接停并说明；再走先 `up.sh <库名> --reset`。申报单元只绑关务包裹，主链 `10` 的包裹不立案，不受影响。

**现场节奏建议。** 开场前 `up.sh <库名> --reset`，再跑一遍 `10`、`30`（预热，确认环境）；现场用的关务包裹留给 `20` 现场跑（预热不要跑它，否则要 `--reset`）。`10` 的耗时主要是 dispatch 拍子，现场想快可以先跑完再讲。

## 每段结果在管理台哪页看

页面 id 即 `#/<id>`；表依据 [`ui-coverage-audit.md`](../../../.scratch/demo-2026-10-30/ui-coverage-audit.md)，审计写明哪些页没有读面。管理台本地起法与登录见 `apps/admin-web/README.md`（dev 服务器从 WSL 起，`PARCEL_API_TARGET=http://127.0.0.1:19080`，用配置员 `syn-operator-config-01@syn-tenant-01.example.invalid` 登录）。

| 段 | 看哪页 | 看什么 |
|---|---|---|
| `10` 提交、受理 | `shipment-request-inquiry` | 委托状态 `ACCEPTED`，声明包裹，接受判断 |
| `10` 初始路由 | `route-plans` | 初始路由册（计划版本；各履约段页面不显示，口头讲或看 API） |
| `10` 预报、收寄 | `node-operations-review` | 送站预报区、收寄区 |
| `10` 班次、预占、交接、订舱 | `transport-fulfillment-review` | 班次、运力池（已预占）、权威交接结果、订舱区、交付证明 |
| `10` BUY 成本、SELL 费用 | `charges-billing` | 客户费用 `CONFIRMED`、供应商预期成本 CNY 8500 |
| `10` 终局 | `tracking-projection` | 最新里程碑 `EFFECTIVE_DELIVERY`、投影版本 |
| `10` 客户视图 | 无页面 | 终端那行输出（首方客户令牌读 `/v1/customer-tracking-view`）；`disclosure-policies` 页可当旁证 |
| `20` 立案、授权、申报单元、资料快照 | `customs-cases` | 「关务案件」「申报单元」「资料快照」「就绪与授权」「关闭义务」签 |
| `20` 申报发送、技术回执 | `customs-cases`「提交版本」签 | 提交尝试（`REGISTERED_PENDING_SEND`）、发送事实、技术回执（`ACKNOWLEDGED`） |
| `20` 放行结果 | `customs-cases`「监管结果与放行」签 | 放行结果 `FULL` |
| `20` 交出读数 | `customs-cases`「门禁判断册」签 | 按**包裹号**查（传申报单元号得空册）；交给干线承运商那次读数 `ADMITTED` |
| `20` 放行门禁核对 | `customs-restrictions`「门禁核对」签 | 门禁目录与前置认定 `MET` |
| `30` 费用确认 | `charges-billing` | 客户费用册阶段、确认依据 |
| `30` 对账单 | `reconciliation` | 客户对账单册（账户、期间、合计、行数）、供应商账单接收册（匹配数） |
| `30` 审核应付、冻结/暴露/余额 | 无页面 | 只看终端输出 |
| `40` 甲 退运决定、待人工清单 | `post-intake-disposition`（导航「收寄后处置决定」） | 待决请求、五向决定、待人工处理项清单、逐项登记、按包裹读回；决定与登记也可以直接在这页上点，脚本路径保留 |
| `40` 乙 撤回、取消 | `shipment-request-inquiry` | 看委托状态；管理台上的撤回、取消页必 401（操作者令牌，冻结），所以现场用终端 |

**管理台仍读不到的几处**：冻结与信用暴露、运营余额、审核应付、客户轨迹视图（首方客户令牌）。演示这些用终端输出讲。

## 不做

不改 Go、不改种子数据。现有 `submit-one-shipment.sh` 等脚本原样被调用；`declare-one-parcel.sh`、`send-one-declaration.sh`、`walk-customer-command-faces.sh` 仓里没有可执行位，本目录脚本用 `bash <脚本>` 调它们。
