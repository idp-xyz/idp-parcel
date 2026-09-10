# sa-cc/03 评审留下的两处小改：SA `adoptDigest` 头注补「0018 起含付款人、存量零不回算」；CC `NewDutyPaymentReconciliationHandler` 构造期拒 nil

Category: chore
Status: ready-for-agent——2026-09-10 22:0x 通道 4 立票（按通道 1 派单 task-d00c5556；sa-cc/03 非作者评审 Standards 非阻断 ① ② 的后继，一张小票包两处）。要裁的为零。只写票面未动代码；取证锚 main `9ddbafcf`
Blocked by: 无（[03](03-cc-inbox-consumer-receives-external-funds-fact.md) 已进 main）

## 缺口（取证于 `9ddbafcf`，逐符号名）

**一、`adoptDigest` 的摘要算法静默不兼容，只靠迁移头注的「存量零」兜住（03 评审 Standards ①）。**

- `internal/settlementaccounting/application/map_external_funds.go` 的 `adoptDigest` 自 sa-cc/03 起把 `strings.TrimSpace(command.Payer)` 以 `\x00` 分隔加进摘要；`AdoptFact` 把它存成 `ports.FundsFactRecord.ContentDigest`，重放时 `existing.ContentDigest != digest` 即答 `FundsFactConflict`。
- 于是任何在 0018 之前落下的行（旧摘要没有付款人那一元素）重投**同一内容**会答 `内容冲突` 而非 `已存在`——幂等不变式在版本边界上断了一格。它今天成立只因 `migrations/settlement_accounting/0018_external_funds_fact_payer.sql` 头注写着「存量为零（本上下文的采用今天没有生产入口），不回填」。
- `adoptDigest` 现头注只有一句「把付款人算进内容：同引用换付款人是另一份内容（冲突），不是重放」——没说这一句自哪一版起成立、旧行为什么不回算。读代码的人要跳到迁移头注才知道这不是 bug；改摘要算法的人下一次也看不到「这里改过一次、靠存量零过关」的先例。

**二、`NewDutyPaymentReconciliationHandler` 不拒 nil，漏装即运行期 panic（03 评审 Standards ② / 判断题 ⑦）。**

- `internal/customscompliance/application/reconcile_duty_payment.go` 的 `NewDutyPaymentReconciliationHandler(deps)` 直接 `return &DutyPaymentReconciliationHandler{deps: deps}`；`DutyPaymentReconciliationDeps` 四口（`Collaborations` / `Funds` / `Verifications` / `Clock`）任一 nil，到 `FormCollaboration` / `ReceiveFundsFact` / `VerifyPayment` 里解引用时才 panic，而不是具名停下。
- 生产调用点 `cmd/parcel-dispatch/assemble.go` `receiveExternalFundsFactConsumer`：同一只 `ccpostgres.DutyPaymentReconciliation` 接全三口 + `clock`（03 判断题 ⑦「接全比留 nil 诚实」）。今天是全的；本票要的是让「不全」在构造期就响，与 03 新增的四只构造器（`NewAdoptedFundsFactView` / `NewSettlementAdoptedFundsFactSource` / `NewReceiveOnAdoptedFundsFactAdapter` / `NewExternalFundsFactConsumer`）同一纪律。
- 同形先例：sa-cc/02 评审 Standards 1——SA `MapExternalFundsDeps.FactHandoff` 漏装即 panic，`NewMapExternalFundsHandler` 不校验；评审当时的处方是「宜在那笔或本编排构造期断言」。仓内已有构造期拒 nil 并返 error 的应用层先例：`internal/settlementaccounting/application/apply_pre_acceptance_control.go` 的 `NewApplyPreAcceptanceControlHandler(deps) (*Handler, error)`，逐口具名报缺。
- **CC application 其余构造器一律不拒 nil**（`NewEstablishCaseHandler` / `NewVerifyReleaseGateHandler` / `NewRegisterCredentialHandler` ……全是同一形）。本票只动被评审点名的这一只，不顺手改整目录——那是另一张风格票，且会碰十几处调用点。

## 语言从哪里来

- 03 评审 Standards ① 原话：「属摘要算法静默不兼容，建议 `adoptDigest` 头注补一句『0018 起含付款人、存量零不回算』（幂等不变式 / 证据诚实）」。
- 03 评审 Standards ② 原话：「该构造器不拒 nil Deps——既有代码非本票引入；本票新增四构造器均拒 nil。同形 sa-cc/02 Standards 1，归 CC application 另票。」
- AGENTS.md「写代码注释」：注释只写代码本身讲不出的东西——某条取舍为何如此、换个写法会破坏什么。「这一版起含付款人、旧行不回算」正是代码讲不出的那种。

## 做法

1. `adoptDigest` 头注加一句：付款人自 `settlement_accounting/0018` 起进摘要；0018 之前没有采用入口、存量为零，故不回算旧行的摘要；若日后再改摘要元素，要么回算要么在这里再记一版。引迁移用文件名 / 符号，不写行号不写计数。
2. `NewDutyPaymentReconciliationHandler` 改为 `(*DutyPaymentReconciliationHandler, error)`，四口逐一具名拒 nil（形照 `NewApplyPreAcceptanceControlHandler` 那张表）。调用点三处：`cmd/parcel-dispatch/assemble.go` `receiveExternalFundsFactConsumer`（接错误、照同函数其余 `New*` 的 `fmt.Errorf("parcel-dispatch: …: %w", err)` 形；共享文件，占号、只改这一块）、`internal/customscompliance/application/reconcile_duty_payment_test.go` 的夹具、`internal/customscompliance/adapters/settlementaccounting/receive_on_adopted_funds_fact_test.go` 的夹具——两处测试夹具今天装的是什么口，作者开工时看：装全的照改签名，只装部分口的按测试意图补替身或改成断言构造期拒。
3. 一条构造期用例：四口各缺一 → 错误里具名那一口。

## 红线

- 不改 `adoptDigest` 的算法与元素顺序（那会让 03 之后落下的行也不兼容）；只加注释。
- 不回填、不动 `0018`；不给 CC 其余构造器一并改签名。
- `assemble.go` 是共享接线文件：动前占号，只改 `receiveExternalFundsFactConsumer` 那一块。
- 不改任何业务判断、不加任何列。

## 完成判据

1. `adoptDigest` 头注含「0018」与「不回算」两个词（`git grep -n '不回算' -- internal/settlementaccounting/application/map_external_funds.go` 一处命中）；`go test -count=1 ./internal/settlementaccounting/application/` 绿、行为零变化。
2. `NewDutyPaymentReconciliationHandler` 返 `(*DutyPaymentReconciliationHandler, error)`；四口各缺一的用例断言到具名错误；`git grep -n 'NewDutyPaymentReconciliationHandler(' -- '*.go'` 的每一处都接了错误。
3. `cmd/parcel-dispatch` 真库装配用例（`TestAnAdoptedExternalFundsFactReachesTheCustomsRegisterThroughTheRouteTable`）原样绿；`assemble.go` 只有那一块 diff。
4. `gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；`go test -count=1` SA application + CC application + CC adapters/settlementaccounting + `cmd/parcel-dispatch`（带 DSN）+ `./internal/architecture/...` 绿；机制清点预期不变。
5. 完成记录逐笔 SHA（两处各一笔）。

## 地盘

`internal/settlementaccounting/application/map_external_funds.go`（一句注释）、`internal/customscompliance/application/reconcile_duty_payment.go` 及其测试、`internal/customscompliance/adapters/settlementaccounting/receive_on_adopted_funds_fact_test.go` 夹具、`cmd/parcel-dispatch/assemble.go` 一块（共享，占号）。**不动** 迁移、`ports`、CC 其余 application 文件。

## 要裁的

无。两处都是评审明写的处方；「只改这一只不改整目录」是范围取舍不是规则题，若 CC owner 要整目录统一，另立票。

## 参照

[03](03-cc-inbox-consumer-receives-external-funds-fact.md) Comments「评审 ← 通道 6」Standards ① ②、判断题 ⑦、「进 main 记录」候选后继；[02](02-sa-external-funds-fact-adoption-hands-off-an-envelope.md) Comments「评审 ← 通道 6」Standards (1)（`FactHandoff` 同形）；`internal/settlementaccounting/application/map_external_funds.go`（`adoptDigest` / `AdoptFact` / `MapExternalFundsDeps`）；`migrations/settlement_accounting/0018_external_funds_fact_payer.sql` 头注；`internal/customscompliance/application/reconcile_duty_payment.go`（`NewDutyPaymentReconciliationHandler` / `DutyPaymentReconciliationDeps`）；`internal/settlementaccounting/application/apply_pre_acceptance_control.go`（`NewApplyPreAcceptanceControlHandler`，构造期逐口拒 nil 的先例）；`cmd/parcel-dispatch/assemble.go`（`receiveExternalFundsFactConsumer`）。

## Comments

- 2026-09-10 22:0x · 通道 4（task-d00c5556，取证锚 main `9ddbafcf`）：立票，Status 直接 ready-for-agent（要裁的为零）。**只写票面，未动代码。** 能力边界：读过 `adoptDigest` / `AdoptFact` 一带与 `FundsFactRecord.ContentDigest` 的比法、`0018` 头注、`NewDutyPaymentReconciliationHandler` 与 CC application 全部 `New*Handler` 的形、`NewApplyPreAcceptanceControlHandler` 的拒 nil 表、`receiveExternalFundsFactConsumer` 全文、03 / 02 两票评审原话；**没读** 两处测试夹具今天装了哪几口——做法 2 因此写成两可，作者开工时看。sa-cc/02 Standards 1 点名的 `NewMapExternalFundsHandler` / `FactHandoff` 那一处**不在本票**：它在 SA，评审处方是「mech/06 SA 在线面接上时同时装它」，归那张票；这里只记同形，不顺手改。
