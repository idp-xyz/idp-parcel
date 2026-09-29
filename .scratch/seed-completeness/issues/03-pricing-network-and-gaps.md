# 03 计价参考目录、自动改路事实，以及三处没有写入口的空表

Category: enhancement
Status: block 1 done、block 2 已取证——2026-09-29 通道 4；同日通道 1 重放进 main，见文末「进 main 记录」。接管写侧另立 `.scratch/takeover-write-side`（用户裁决要开，待选项）。
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

## 进 main 记录（2026-09-29，通道 1 推送）

分支 `mcp4-seedc@c5d13025`（已推 origin）。三笔在隔离树 `/tmp/idp-replay-seed` 顺序重放到 `e2ff20bb` 之上：`33c1aae8→cd9f767c`（`git range-diff` 为 `=`）、`c5d13025→5064da1a`（`!`：`scripts/demo-seeds/README.md` 两处文本冲突，两边正文都留）、`f0d16476→4feb8d2f`（`!`：README 组成表一处冲突，取 pricing/network 行来自 03、customs/visibility/collection 行来自 02）；另加 `2171a208` 按评审非阻断意见修 README 数据故事里信用政策/客户服务规则挂靠的措辞。
推送方验证：链 tip `2171a208` 上 gofmt 空，vet 与 build 退出 0，清点门 current，分片覆盖核对通过（148 个包）；一次性库 `127.0.0.1:55461` 上 `seed.sh --reset` 三笔合并后退出 0（供应商协议 2、分区目录 1、自动改路事实 1、回汇批次 2、监管凭证 2、索赔材料收讯 2）；含 DSN `go test -p 1 -count=1 ./...` 一次：**134 ok / 0 FAIL / 14 无测试**。三笔都不改 Go 生产/测试代码（03 只动 seedgen 生成器与产物），清点无变化。

## Comments

**评审 ← 通道 2 · 钉 `c5d13025` · 无阻断**：重跑 seedgen 后 `git diff` 为空；私有库 55452 `seed.sh --reset` 两次退出 0，`reference_catalogue_version` 1、review 1、`auto_reroute_facts` 1，`network_definition`/`initial_route`/`plan_applicability`/`reachability_judgment`/`takeover_record` 均为 0；四张路由表无登记入口属实；价格政策由 `publish` 承接属实（在 `mcp2-seedcomp` 已补）；接管未开与 ADR-0128 决定五属实。非阻断：① 自动改路那行键对不上任何委托，只展示形状（README 已写明）；② 初始路由证据已改读目录，「0007 恒未配置」属实但不挡目录那条链；③ 票 04 遗漏：隔离写同时换上节点收寄、运输履约一批口、关务结果、外部资金事实，步 2 不要按「这些口都没开」排；④ 票面写 55444 已删而评审时仍在（作者容器，评审没动）；⑤ 与 `mcp2-seedcomp` 在 README「配价」段与已知边界价格政策条撞，回放两边正文都留（已按此解）。
