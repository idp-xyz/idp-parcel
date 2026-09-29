# 03 计价参考目录、自动改路事实，以及三处没有写入口的空表

Category: enhancement
Status: in-progress——2026-09-29 通道 4（分支 `mcp4-seedc`）
Blocked by: 无
地盘：`scripts/demo-seeds/data/pricing/`、`data/network/`、`seedgen/`、`README.md` 与 `seed.sh` 里这两段。不改 commercial / customs / collection。
出处：用户反馈 `./parcel.sh seed` 后页面业务数据不完整。派单 `task-b125d095` ← 通道 1。

## 块 1：CLI 已有、种子没调用

- `parcel-pricing-register -kind reference-catalogue` 与 `reference-catalogue-review`。
  生成器经 `MarshalReferenceCatalogueRegistration` 产出 `SYN-CAT-ZONE-CN-SG` v1（证据 S）：
  目的前缀粒度由这一版声明为 3，`018`→Z1、`238`→Z2，始发覆盖 `200` 与 `510`。
  Z1/Z2 与售价卡 `SYN-PLAN-CN-SG-01` 同名，卡不绑这本目录，评价仍走调用方给值。
  复核人 `SYN-PRICING-REVIEW-01`，与登记人不同。
- `parcel-network-register -kind auto-reroute-facts`。一行合成陈述，判断键对齐
  `SYN-TENANT-01` / `SYN-ACCOUNT-01` / `SYN-RS-CN-SG-01/v1`。库里没有对应委托。
  0009 存的就是登记方折算完的布尔，不存阈值；空清单是「没有未解除限制」这句陈述，
  与零行的「未登记」分开。

网络四张仍空的表，核对结果：

| 表 | 有没有登记入口 | 是什么 |
|---|---|---|
| `network_definition`（0007） | 没有。登记口不再读它 | 头注：没有写入方，读口恒答未配置 |
| `initial_route` | 没有 | 一次初始路由判断留下的运行时记录 |
| `plan_applicability` | 没有 | 计划版本的适用性，挂在初始路由之后 |
| `reachability_judgment` | 没有 | 可达性判断的运行时记录 |
| `auto_reroute_facts`（0009） | 有，本票调用 | 登记方的事实陈述，不是上面三张的产物 |

## 块 2：没有写入口的项（不擅自开机制）

### 价格政策

写入口已经有：`parcel-commercial publish` 在批文带 `pricePolicyBody` 时调用 `SavePricePolicy`。
本包 `publish-batch.json` 没有这份正文，所以页面仍空。补正文在 commercial 地盘，通道 2 的商业发布批会碰它。
通道 4 已在频道里问过：由谁补、页面读哪张表、种子要不要等。本票不改 commercial，也不自己拍板补一份政策。
`RehydrateAdoptedBasisSpec` 仍缺席，见 `commercial-closure-settlement-key/02`。

交回通道 1 的选项（用户裁）：

1. 等通道 2 在发布批里补 `pricePolicyBody`，种子不另做。
2. 通道 2 明确不覆盖种子后，另立一票只加批文正文（仍属 commercial 地盘）。

### 接管记录

`parcel-governance-register` 对未知种类答「接管未开」。ADR-0128 决定五：接管记录的语义与写侧不在该记录内。
`PAR-GOV-05..07` 仍待提供。这是刻意保护，不是漏调用。演示不造行。

交回通道 1 的选项（用户裁）：

1. 保持未开，页面继续如实空。推荐：开写入口要先有 ADR，不是种子票能做的。
2. 另立治理票，先定接管记录的语义再谈演示数据。

### 回汇批次

CLI 有 batch 入口。通道 3 负责复核，本票不管。

## 验证

一次性库 `idp-parcel-seed-mcp4`（`127.0.0.1:55444`，用完已删）。不碰 `55432`。
`seed.sh --reset` 两次都退出 0。目录与复核重放答 `ALREADY_REGISTERED` / `ALREADY_RECORDED`（退出码 0）。
自动改路同键再登答 `ALREADY_EXISTS`（退出码 2），所以整段重跑仍走 `--reset`。
灌后行数：目录 1、目录复核 1、自动改路事实 1；`network_definition`、`initial_route`、
`plan_applicability`、`reachability_judgment`、`takeover_record` 都是 0。

## 不做

- 不改价卡去绑目录。
- 不给 `network_definition` / 初始路由 / 适用性 / 可达性新开登记口。
- 不 INSERT。
- 不碰门禁库 `127.0.0.1:55432`。
