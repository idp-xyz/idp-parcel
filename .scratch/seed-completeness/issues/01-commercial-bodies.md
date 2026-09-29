# 01 商业发布批补齐正文

Category: enhancement
Status: done——2026-09-29 通道 2；同日通道 1 重放进 main，见文末「进 main 记录」。
Blocked by: 无
地盘：`scripts/demo-seeds/data/commercial/`、`scripts/demo-seeds/README.md` 的组成表与数据故事、`scripts/demo-seeds/seed.sh` 里商业发布那一行说明。不改网络、关务、计价、可见性、代收的种子。

## 事实

`data/commercial/publish-batch.json` 原先 12 项里只有接单规则包、客户合同、结算政策、取消授权带真正文；财务控制策略与两份供应商协议、两份服务产品、价格规则、两份方向授权是 `sha256:syn-…` 壳。发布口后来已经能写供应商协议、信用政策、客户服务规则、接受前财务控制策略、价格政策、合同委派、交付条件、资料修订允许，种子没跟上，对应正文表是空的。

本票把发布口能写、种子却没写的正文补进同一发布批，标识一律 `SYN-`，证据只记 S。摘要改成发布时 CLI 印出的 PCC-1，不再留占位串。两份方向授权仍是壳：授权规则的正文通道只有取消授权。

## 正文与对齐

- 服务产品两版各带交付条件。快递：当面与自提柜；经济：仅当面。
- 接受前财务控制策略 `SYN-FIN-CONTROL-01`：共同通过为全部通过；预付冻结、信用检查，都拒、都挂 `SYN-LE-01` 与 `SYN-CHARGE-PREPAID`。
- 接单规则包补资料修订允许：收件地址、已受理未收寄、更正、允许。摘要因此换过。
- 客户合同补合同委派（客户账户、来源资料修订）与收紧交付条件（快递产品收紧到当面）。摘要因此换过。
- 供应商协议两份都采购买入方案 `SYN-PLAN-CN-SG-COST-01`（网络线上唯一的 BUY 方案）。干线承运方 `SYN-PARTY-CARRIER-TRUNK-01`、末端承运方 `SYN-PARTY-CARRIER-LM-01` 加进 `register-parties.json`。线路 JSON 本身不点名协议。
- 价格规则补卖出价格政策正文与口径：未税、材积除数 5000、提交时点汇率，与卖出价卡同一套。结算政策摘要未改。
- 新增信用政策 `SYN-CREDIT-01`（额度 500000，合成值，不是生产默认）与客户服务规则 `SYN-SERVICE-RULE-01`（挂快递产品，首次索赔 14 个工作日）。

## 取证

一次性库 `idp-parcel-seed-mcp2`，`127.0.0.1:55442`，不碰共享门禁 `55432`。

- `seed.sh --reset` 退出 0。发布批 14 项全部 `PUBLISHED_EFFECTIVE`，上列声明通道全部 `SAVED`。
- 不带 `--reset` 再跑：商业发布 14 项全部 `REPLAYED`，声明 `ALREADY_REGISTERED`；停在网络目录节点版本主键（退出 3）。这是 README 已写的口径，不是本票的正文缺陷。
- 正文行数：`supplier_agreement` 2，`credit_policy` 1，`customer_service_rule` 1，`pre_acceptance_financial_control_policy` 1，`commercial_price_policy` 1，`price_policy_caliber` 1，`contract_delegation` 1，`delivery_condition` 3，`source_data_amendment_allowance` 1。

## 管理台读面

对照 `apps/admin-web/src/pages/party` 的列表入口与 `operations_catalogue` 的查询。本票没有把这套种子灌进共享门禁，所以没有在正在跑的管理台上点开页面。

| 页 | 结果 |
|---|---|
| 供应商协议 `GET /commercial-supplier-agreements` | 读面左连接 `supplier_agreement`，两行正文会列出 |
| 商业策略册：信用政策、商业价格政策、接受前财务控制策略、客户服务规则、接单规则包 | 各册按 kind 列表，对应正文表有行 |
| 服务产品、客户合同的交付条件 | 两页都渲染 `deliveryConditions`，目录查询连接 `delivery_condition`，三行会列出 |
| 合同委派 | 读面缺失。委派已落库，读口是裁定用的 `LoadContractDelegations` / `EffectiveContractDelegationView`，合同页没有委派列 |
| 资料修订允许 | 读面缺失。允许已落在接单规则包版本上，政策册目录查询不选这张表，页面没有这一列 |

两份方向授权在授权规则册上仍是没有取消授权正文的壳。这是发布口没有方向正文通道，不是页面没接。

## 进 main 记录（2026-09-29，通道 1 推送）

分支 `mcp2-seedcomp@33c1aae8`（已推 origin）。三笔在隔离树 `/tmp/idp-replay-seed` 顺序重放到 `e2ff20bb` 之上：`33c1aae8→cd9f767c`（`git range-diff` 为 `=`）、`c5d13025→5064da1a`（`!`：`scripts/demo-seeds/README.md` 两处文本冲突，两边正文都留）、`f0d16476→4feb8d2f`（`!`：README 组成表一处冲突，取 pricing/network 行来自 03、customs/visibility/collection 行来自 02）；另加 `2171a208` 按评审非阻断意见修 README 数据故事里信用政策/客户服务规则挂靠的措辞。
推送方验证：链 tip `2171a208` 上 gofmt 空，vet 与 build 退出 0，清点门 current，分片覆盖核对通过（148 个包）；一次性库 `127.0.0.1:55461` 上 `seed.sh --reset` 三笔合并后退出 0（供应商协议 2、分区目录 1、自动改路事实 1、回汇批次 2、监管凭证 2、索赔材料收讯 2）；含 DSN `go test -p 1 -count=1 ./...` 一次：**134 ok / 0 FAIL / 14 无测试**。三笔都不改 Go 生产/测试代码（03 只动 seedgen 生成器与产物），清点无变化。

## Comments

**评审 ← 通道 3 · 钉 `33c1aae8` · 无阻断**：私有库 55453 `seed.sh --reset` 退出 0，14/14 `PUBLISHED_EFFECTIVE`，入册摘要与 JSON 声明一致；两份方向授权仍是 `sha256:syn-` 壳、合同委派与资料修订允许已落库但管理台无列，属实；本差分不改 Go、无测试读 `publish-batch.json`，作者只跑 build/vet 够。非阻断：README 故事第 2 条「信用政策与客户服务规则都挂在同一法人与预付费用上」与数据不符（信用是 `SYN-LE-01`+`SYN-CHARGE-PREPAID`，客户服务规则是 `SYN-LE-01`+产品 `SYN-PROD-CN-SG-EXPRESS`）——已在 `2171a208` 改。
