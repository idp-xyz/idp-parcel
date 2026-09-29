# 02 种子补全：关务 / 可视与异常 / 代收 缺口

Category: enhancement
Status: done——2026-09-29 通道 3；同日通道 1 重放进 main，见文末「进 main 记录」。
Blocked by: 无
地盘：`scripts/demo-seeds/data/customs/`、`data/visibility/`、`data/collection/`，以及 `scripts/demo-seeds/seed.sh` 与 `README.md` 里这三段。不改登记 CLI，不改管理台页面，不碰 `data/commercial/`、`data/pricing/`、`data/network/`。

## 为什么

`./parcel.sh seed` 之后，三个上下文各有登记 CLI 已有、种子没调用的入口，对应管理页读到空册。演示租户 `SYN-TENANT-01` 的合成实例（证据层级 S）本来就该把这些故事灌进去；空着不是「租户取值未配置」，是种子没走到那条命令。

## 灌进去的故事

全部 `SYN-` 前缀，只记 S，引用既有 `SYN-TENANT-01`、`SYN-LE-01`、`SYN-BROKER-01`、`SYN-ACCOUNT-01`、`SYN-CASE-CN-SG-01` 这一套，不新开租户。

1. **关务。** `SYN-UNIT-CN-EXPORT-03` 先就绪再按 `SYN-CAUSE-READINESS-WITHDRAWN` 撤销（单元 01 仍有效，单元 02 只撤授权）。报关行两版监管凭证，程序都是 `SYN-PROC-CN-EXPORT`：`SYN-CRED-CN-EXPORT-01` 写明 12 次，`SYN-CRED-CN-EXPORT-02` 不写次数。税费协作两格：出口单元按核定 `SYN-DUTY-CN-SG-01` 形成，进口单元 `SYN-UNIT-SG-IMPORT-01` 明确无需付款。
2. **可视与异常。** 异常披露规则与分诊同一对信号：账户 01 的 `CUSTOMS_HOLD` 可披露、不自动发布；账户 02 的 `ETA_GAP` 明确不披露。冲突信号规则一租户一条，钉在 `CUSTOMS_HOLD`。索赔批次收照片与发票，发票次日撤销。
3. **代收。** CNY 分户账两笔回汇批次：`SYN-BATCH-CNY-OPEN` 停在已归集，`SYN-BATCH-CNY-HANDED` 交出汇付主张。SGD 账仍无批次。早先 seed.sh 写「回汇批次一笔不造——批次属实例半边，页面显未配置」：同段指令、事实、记账已经是演示租户的 `SYN-` 实例，这条理由今天不成立。空批次格留给 SGD 账，页面原文仍能显「未配置」。

## CLI 拒收、不绕过

`duty-payment-verification` 不进种子。核对要已接收的外部资金事实版本、同一范围上的协作事项、该程序的付款人规则。资金事实只经 settlement-accounting 的采用信封进关务，`parcel-customs-register` 没有这条命令；付款人规则也没有种子能调用的入口。协作事项落下之后，核对仍答前置未齐（退出码 3）。不直接 INSERT。

## 完成判据

- 隔离库 `seed.sh --reset` 两次都零报错（第二次是干净复灌，即本包写明的幂等口径）。
- 不碰 `idp-parcel-postgres-gate`（127.0.0.1:55432）。
- 逐页核对写在完成记录里：读模型对得上的页点名；读不到的写原因。

## 完成记录

隔离库 `idp-parcel-seed-mcp3`（127.0.0.1:55443，用完已删）。`seed.sh --reset` 两次退出码都是 0。门禁库 127.0.0.1:55432 未连接、未重置。批次与交出在已灌库上重放分别答 `EXISTING`、`ALREADY_HANDED_OVER`（退出码 0）。

管理台现在连的是门禁库上的 api，这次种子不在那只库里，浏览器打不开这些新行。下面按各页读的那张表核对；Intake 未配置时这些 GET 本来就 403，那是既有边界，不是这批种子没落下。

| 页 | 读到什么 |
|---|---|
| 关务案件 · 申报就绪 | `readiness_judgment` 三行。`SYN-UNIT-CN-EXPORT-03` 的 `revoked_by` 是 `SYN-CAUSE-READINESS-WITHDRAWN`，页上写成「已撤销」。01、02 仍有效 |
| 关务案件 · 监管凭证 | 两行。`SYN-CRED-CN-EXPORT-01` 的 uses=12；02 的 uses=0，读口把 0 收成缺席，页上计入「来源未提供次数额度」 |
| 关务限制 · 税费付款协作 | 两行：出口 `ASSESSED_DUTY` / `SYN-DUTY-CN-SG-01`，进口 `EXPLICITLY_NOT_REQUIRED`。页上「明确无需付款」计 1 |
| 关务限制 · 税费付款核对 | 0 行。CLI 前置未齐，故意不灌。页显空册 |
| 披露策略 · 异常披露规则 | 两行：账户 01 的 `CUSTOMS_HOLD` 可披露、不自动发布；账户 02 的 `ETA_GAP` 不披露 |
| 判断规则 · 冲突信号规则 | 一行：`CUSTOMS_HOLD` / `SYN-VE-CONFLICT-V1` |
| 代收分户账 | CNY 账 `SYN-BATCH-CNY-OPEN`=`COLLECTED`、`SYN-BATCH-CNY-HANDED`=`HANDED_FOR_PAYMENT`（页词「已归集」「已交付汇付」）。SGD 账 0 笔，页显「未配置」 |
| 索赔材料收讫 | 收讫两行、撤销一行（发票）。管理台没有这一册的页 |

## 进 main 记录（2026-09-29，通道 1 推送）

分支 `mcp3-seedb@f0d16476`（本地分支，未推 origin，随重放进 main）。三笔在隔离树 `/tmp/idp-replay-seed` 顺序重放到 `e2ff20bb` 之上：`33c1aae8→cd9f767c`（`git range-diff` 为 `=`）、`c5d13025→5064da1a`（`!`：`scripts/demo-seeds/README.md` 两处文本冲突，两边正文都留）、`f0d16476→4feb8d2f`（`!`：README 组成表一处冲突，取 pricing/network 行来自 03、customs/visibility/collection 行来自 02）；另加 `2171a208` 按评审非阻断意见修 README 数据故事里信用政策/客户服务规则挂靠的措辞。
推送方验证：链 tip `2171a208` 上 gofmt 空，vet 与 build 退出 0，清点门 current，分片覆盖核对通过（148 个包）；一次性库 `127.0.0.1:55461` 上 `seed.sh --reset` 三笔合并后退出 0（供应商协议 2、分区目录 1、自动改路事实 1、回汇批次 2、监管凭证 2、索赔材料收讯 2）；含 DSN `go test -p 1 -count=1 ./...` 一次：**134 ok / 0 FAIL / 14 无测试**。三笔都不改 Go 生产/测试代码（03 只动 seedgen 生成器与产物），清点无变化。

## Comments

**评审 ← 通道 4 · 钉 `f0d16476` · 无阻断**：可丢弃库 55454 独立重放退出 0；`regulatory-credential` 两次 REGISTERED、`duty-collaboration` 两次 COLLABORATION_FORMED、`exception-disclosure-rules` 与 `conflict-signal-rule` REGISTERED、`claim-material-receipt` 两次 REGISTERED 后 revocation REVOKED、`batch` 两次 REGISTERED、`hand-over` HANDED_OVER；关务 26 份 JSON 与 README 一致；凭证 02 `uses=0` 被读口收成「来源未提供次数」、admin-web 无索赔材料页、`duty-payment-verification` 不登资金事实且 diff 无 INSERT，作者结论成立；`SYN-ACCOUNT-02` 复用既有可视种子。作者未改 Go、未跑包测试，够。提示：seed.sh 与 README 同 03 改的不同段落，回放有文本冲突、语义不冲突（已按此解）。
