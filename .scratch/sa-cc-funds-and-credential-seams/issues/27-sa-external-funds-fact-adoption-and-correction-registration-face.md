# SA 外部资金事实采用与更正的登记面：`MapExternalFundsHandler` 零生产装配，首版采用与 sa-cc/20 新开的「更正」格今天都只有测试直写能到

Category: enhancement
Status: draft——2026-09-15 10:3x 通道 6 立票（sa-cc/20 裁决 2「更正面与首版面是同一张『SA 采用登记面』的题……本票作者顺手立一张 draft」；归 SA owner）。只写票面未动代码；取证锚 main `3a21dab7` 与分支 `mcp6-sacc20`
Blocked by: 20（更正格已在分支 `mcp6-sacc20`，进 main 前本票不开工）；要裁的三条归 SA owner

## 缺口（取证于 `3a21dab7`，逐符号名；sa-cc/20 通道 5 取证条与通道 6 认领笔各量过一遍）

- `internal/settlementaccounting/application/map_external_funds.go` `NewMapExternalFundsHandler` / `MapExternalFundsDeps`：`git grep -n -E 'NewMapExternalFundsHandler|MapExternalFundsDeps' -- cmd/ internal/ ':!*_test.go'` 除定义处外**零命中**——采用（`AdoptFact`）、映射（`Map`）、核销（`Apply` / `Reverse`）与 sa-cc/20 加的更正（`CorrectFact`）五格今天没有任何生产构造点。
- 逐类入口：**inbox**——`internal/settlementaccounting/adapters/inbox/` 只听 `parcel-pricing.evaluation.recorded` 与 `customs-compliance.duty-payment-verification.formed`，仓内没有任何银行 / 支付 / 财务系统的入向事件类型；**HTTP**——`internal/settlementaccounting/adapters/http/` 全是 `query_*` 与 intake，`cmd/parcel-api/endpoints.go` 装的 SA 端点全是 `settlementhttp.NewQuery*`；**CLI**——`cmd/` 下无 settlement 二进制。
- 后果：外部资金事实进产品只有 SA 采用这一口（ADR-0137 决定四），而这一口今天没有门——首版与更正都只有 `cmd/parcel-dispatch/assemble_test.go` 与 SA 各测试用 `sapostgres.NewExternalFundsFacts(db).Save` 直写。sa-cc/02 的信封、sa-cc/03 的 CC 消费者、sa-cc/13 的版本子表、sa-cc/20 的更正格，整条缝在生产依赖图上从 SA 这一头起就是断的。
- 装配纪律的一处暗礁（sa-cc/02 评审 Standards 非阻断 (1) 记过）：`MapExternalFundsDeps.FactHandoff` 漏装时 `handOffFact` 直接对 nil 调用 → panic；`NewMapExternalFundsHandler` 今天不像 `NewRequestBuyEvaluationHandler` 那样构造期逐口拒 nil。

## 语言从哪里来

- ADR-0137 决定四：「外部资金事实进入本产品只有 `settlement-accounting` 采用这一口……人工核实与更正也先登在 SA、再经采用信封到 CC」——去掉的是 CC 侧运维补录子命令，**保留的这一口要有门**才成立。
- UC-SA-001「同一外部资金事实通过回调、文件或人工核实重复到达，只能被采用一次；更正必须形成新来源版本」——回调 / 文件 / 人工核实三种到达方式各自对应一种入口形。
- UC-SA-005 状态行：真实财务系统来源仍 `No-Go / 待参数化`（`PAR-INT-05`）；输入表「外部资金事实」行「不复制为本地付款事实，不因接收回调自动采用」——**自动采用不在任何一票**，本票只做机制半边的门。
- ADR-0085 决定一「登记册配置写面属产品能力，进 `parcel-api` 端点表」、决定二「写准入不另立准入形」（`UnconfiguredIntake{}` 起步）、决定四「其余上下文逐册跟进，取舍按『登记频次 × 操作者角色』由实施票逐册裁」——SA 的采用与更正是不是「登记册配置写面」这一族，是要裁的 2。

## 做法（待裁后写实）

1. 装配点（见「要裁的」1）：a) `cmd/parcel-api` 端点表加 SA 第一个写端点（照 ADR-0085 形：Intake 接口 + 处理器接口 + 封闭响应形状，`UnconfiguredIntake{}` 起步；管理台写签随两阶段）；b) 新建登记 CLI（先例 `parcel-customs-register` / `parcel-pricing-register`）或在既有二进制加子命令；c) 两者都做（ADR-0085 决定一「CLI 与端点消费同一登记用例」）。
2. 命令形：首版 `AdoptFundsFactCommand` 与更正 `CorrectFundsFactCommand` 各一格，答案代数照 `FundsOutcome` 封闭集转写；`registrationjson` 若沿用 CC 那套先例则同笔。
3. 装配纪律（见「要裁的」3）：`NewMapExternalFundsHandler` 构造期逐口拒 nil（形照 `NewRequestBuyEvaluationHandler` 与 CC `NewDutyPaymentReconciliationHandler`），`FactHandoff` 装 `OutboxExternalFundsFactHandoff`，同事务 Outbox。
4. 真库装配用例一正一反在 `cmd/` 侧：采用 → 版本行 + 信封；更正 → 第二行 + 第二封。

## 红线

- 不自动采用：任何回调 / 文件到达都不在本票；本票只开人工 / 受控批量的门（UC-SA-005「不因接收回调自动采用」）。
- 不预填任何真实来源、账户、币种；夹具全 `SYN-`。
- 不改 SA 采用四格与更正格的答案代数；不改 CC。
- 共享接线文件（`cmd/parcel-api/endpoints.go`、`main.go`）动前占号。

## 完成判据（待裁后写实）

1. 装配点在：`git grep -n NewMapExternalFundsHandler -- cmd/ ':!*_test.go'` 非零。
2. 首版与更正各能从生产入口进：真库装配用例一正一反。
3. 漏装 `FactHandoff` 在构造期被拒，不再 panic。
4. ADR-0085 那套三态（未配置 / 登记册答案 / 未决）在 SA 写面上照样分得开（若走端点）。

## 地盘

`internal/settlementaccounting/{application,adapters/http}`、`cmd/parcel-api/{endpoints.go,main.go}`（共享，占号）或新建 `cmd/parcel-settlement-register`、`apps/admin-web` 写签（若走端点）。不动 `internal/customscompliance/**`。

## 要裁的

1. 入口长在哪：CLI / HTTP 端点 / 两者——归 SA owner。三选各自要新建什么见 sa-cc/20 通道 5 取证条「入口三选各自要新建什么」。
2. ADR-0085 决定四怎么读：SA 的采用与更正算不算「登记册配置写面」这一族（它登的是外部事实的引用，不是配置）；若不算，写面的准入与管理台归属另裁——归 SA owner。
3. `FactHandoff` 漏装即 panic 的装配纪律：构造期拒 nil 是不是 SA 全部 handler 的统一形（16 已给 `NewRequestBuyEvaluationHandler` 做过一处）——归 SA owner。

## 参照

[20](20-sa-external-funds-fact-holds-one-row-per-fact-and-cannot-store-a-correction.md) 裁决 2 与通道 5 取证条「要裁的 2」；[02](02-sa-external-funds-fact-adoption-hands-off-an-envelope.md) 完成记录「`cmd/parcel-api` 装配处经核不存在」与评审 Standards 非阻断 (1)；[16](16-sa-evaluation-request-and-duty-verification-adoption-standards-tails.md)（构造期拒 nil 的 SA 先例）；ADR-0137 决定四与越权风险点 6；ADR-0085；UC-SA-001；UC-SA-005。

## Comments

- 2026-09-15 10:3x · 通道 6：立票（sa-cc/20 实施中按裁决 2 顺手立）。只写票面，未动代码。
