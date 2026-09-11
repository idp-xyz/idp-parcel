# sa-cc/08 / 09 两份非作者评审 Standards 非阻断收口：`NewRequestBuyEvaluationHandler` 拒 nil 包 `ErrNilDependency`、`EvaluationRequests.Save` 核键与对象一致、`inserted_at` 从不读回的处置、`AdoptDutyPaymentVerification` 空租户先拒、`ErrUntranslatableReference` 上抛路径核

Category: chore
Status: resolved——**已进 main，2026-09-11 22:5x**（通道 1 推送方重放：main 上 `a11abb96` / `33f2272f` / `c9d6f6e7` / `324aa474` / `4776b1b8` / `3049ca1b`，与 lc/37、sa-cc/01 同批，见 Comments「进 main 记录」；非作者评审 ← 通道 1 两轴 0 阻断）；此前 resolved——2026-09-11 21:5x 通道 6 完工（task-63bd3adc），分支 `mcp6-sacc16` 基远端 main `02e1dfc4`，代码 `9d48e8ca` / `0d25b898` / `2aa0c463` / `dab69b4f`（⑤ 注释 + 完成记录同笔）+ 要裁的 1 裁准补笔（本笔：`errors.Is` 穿过 + 一例 + 票面同笔），等非作者评审与推送方进 main；「要裁的」一条已裁；此前 in-progress——2026-09-11 21:2x 通道 6 按通道 1 派单 task-63bd3adc 自立自做（08 作者收自己票的评审尾巴，awf/26 先例；09 两条是通道 5 的票，它在 sa-cc/01 上，代收），分支 `mcp6-sacc16` 基远端 main `02e1dfc4`，隔离树 `D:/tops/idp-parcel-mcp6-sacc16`。五条全是两位评审者与推送方认可的 A 类；要裁的见下（一条，不阻本票、本票不改它）
Blocked by: 无（[08](08-sa-evaluation-request-orchestration-records-source-references.md) 与 [09](09-sa-consumes-duty-payment-verification-envelope-into-advance-recovery.md) 均已进 main）

## 缺口（取证于 `02e1dfc4`，逐符号名）

**一、`NewRequestBuyEvaluationHandler` 拒了 nil 但调用方 `errors.Is` 认不出（08 评审 Standards ①）。**

- `internal/settlementaccounting/application/request_buy_evaluation.go` 的 `NewRequestBuyEvaluationHandler` 对 `RequestBuyEvaluationDeps` 每一口分别用裸 `fmt.Errorf` 拒 nil；同包 `apply_pre_acceptance_control.go` 的 `NewApplyPreAcceptanceControlHandler` 是表驱动、`fmt.Errorf("%w: %s", ErrNilDependency, name)`，头注写它是「构造门对缺件的唯一答复」。两只构造器在同一个包里各说各话，装配方要按 `errors.Is(err, application.ErrNilDependency)` 分「装配漏了」与「别的构造错误」时，前者认不出。
- 既有用例 `TestTheHandlerRefusesANilDependencyAtConstruction` 只断言 `err == nil` 为失败，不断言哨兵。

**二、`EvaluationRequests.Save` 不核键与对象说的是不是同一件事（08 评审 Standards ②）。**

- `internal/settlementaccounting/adapters/postgres/evaluation_request.go` 的 `Save` 把 `record.Key.TenantID` / `record.Key.Request` 写主键列、把 `record.Request` 的成分写内容列，两者之间没有一致性核。键指 A、对象是 B 的一份记录会落成「按 A 查出来内容是 B」的行，读口 `findOne` 重建时用的是内容列里的 `request_id`，`Key.Request` 会与调用方当初给的键对不上。
- 先例：CC `internal/customscompliance/adapters/postgres/duty_payment_reconciliation.go` 的 `SaveVerification` 先核键三维与对象三维相等、再核指纹与依据非空白，不符即 `SaveOutcomeInvalid` + 错误，不写。

**三、`0019_evaluation_request.sql` 的 `inserted_at DEFAULT now()` 从不读回（08 评审 Standards ③）。**

- 评审原话「Speculative Generality，SA 既有表若无此形可去」。核过：`inserted_at timestamptz NOT NULL DEFAULT now()` 是本仓迁移的通形——SA 从 `0003_customer_charge_advance.sql` 到 `0020_duty_payment_verification_adoption.sql` 每张表都有，TF / VE / PG / NO 与 CC `0001_external_result.sql` 同样有，CC 0001 还拿它建了索引。评审的前提（SA 既有表无此形）不成立：去掉 0019 这一列会让它成为 SA 唯一没有库侧审计列的表。
- `0019` 已施加，业务迁移的 checksum 由 Parcel 对文件内容算（`internal/platform/migrate` 的 `PBC-06` 证据），一字不能改；表头注那半因此无处可写。

**四、`AdoptDutyPaymentVerification` 不核 `command.TenantID` 零值（09 评审 Standards (1)）。**

- `internal/settlementaccounting/application/adopt_duty_payment_verification.go` 的 `AdoptDutyPaymentVerification` 经 `verificationReferenceFrom` 把四维引用拒成 `SOURCE_NOT_ACCEPTED`，但租户零值不核：空租户会走到 `FindByKey`（不命中）→ `Save` 撞 `0020` 的 `refs_not_blank` CHECK → 译成 `INPUT_STORE_UNAVAILABLE` 未决而被重投，重投不自愈。
- 生产上不可达（`AdoptOnDutyPaymentVerificationAdapter` 先过 `sadomain.NewTenantID`），但它是导出方法；同包 `RequestBuyEvaluationHandler.Handle` 对租户空白先答`未受理`，本方法与它不一致。既有用例 `TestAVerificationReferenceMissingADimensionIsNotAccepted` 四例无租户一例。

**五、`HandleFormedDutyPaymentVerification` 把读口的一切错误包成可见性滞后（09 评审 Standards (2)）——只核不改。**

- `internal/settlementaccounting/adapters/customscompliance/adopt_on_duty_payment_verification.go` 的 `HandleFormedDutyPaymentVerification`：`view.DutyPaymentVerificationExists` 的任何 err 都以 `%v` 折进 `ErrVerificationNotVisible`（重投哨兵），链上不再带原因；而同包 `duty_payment_verification_view.go` 的 `customsVerificationKey` 译不出时交回的是 `ErrUntranslatableReference`（编程错误、不该重投）。`cmd/parcel-dispatch/assemble.go` 的 `dutyPaymentVerificationUndecidedSentinels` 头注写「`sacustoms.ErrUntranslatableReference` 也不在——两侧词汇分歧是编程错误，重投不自愈」。
- 今天不可达：SA 侧 `verificationReference` 与 CC 侧 `customsVerificationKey` 两边的构造门同为非空白校验，SA 侧构造得出的引用 CC 侧必构造得出；读口里那条 `ErrUntranslatableReference` 分支在生产上走不到。所以 `assemble.go` 那句对**本适配器自己译出的** `ErrUntranslatableReference`（租户、`verificationReference`）写实——它们原样上抛、不在名单、落 `publish_failed`；对读口内部那条，是被 `ErrVerificationNotVisible` 盖住、靠不可达成立，不是靠分格成立。

## 语言从哪里来

- 08 评审 ← 通道 4 Standards ①②③ 原话与推送方处置「归 SA owner 一笔小票或随下一张 SA 票收；③ `inserted_at` 归同笔」；09 评审 ← 通道 3 Standards (1)(2) 原话与推送方处置「与 08 评审 S①②③ 合成一张 SA 小票」。
- [14](14-adopt-digest-header-and-duty-reconciliation-handler-rejects-nil.md) 的形：评审尾巴一张小票包几处，Status 直起 in-progress，要裁的为零（本票多出一条，见下）。
- AGENTS.md「写代码注释」：注释只写代码讲不出的东西；引迁移用文件名 / 符号，不写行号不写计数。

## 做法

1. **`NewRequestBuyEvaluationHandler` 改表驱动 + 包 `ErrNilDependency`**（形照 `NewApplyPreAcceptanceControlHandler`）。`ErrNilDependency` 沿用同包既有那一枚，不另铸第二枚——它的头注已说「构造门对缺件的唯一答复」，再铸一枚就是两个唯一。错误文本仍逐口点名。`TestTheHandlerRefusesANilDependencyAtConstruction` 改为断言 `errors.Is(err, application.ErrNilDependency)` 且错误文本含那一口名、交出的 handler 为 nil；口齐全不拒。
2. **`EvaluationRequests.Save` 加两道门**：键的租户或请求 ID 为零值 → 拒；`record.Key.Request != record.Request.ID()` → 拒。都交 `EvaluationRequestSaveOutcomeInvalid` + 错误，不发 INSERT。真库用例一条：键指 `EVREQ-SYN-1`、对象的 ID 是 `EVREQ-SYN-2` → 拒存且两个 ID 都查不到行。
3. **`inserted_at` 取 (b) 保留**：只在 `evaluation_request.go` 的 Go 读口头注写明它是库侧审计列（行何时物理落库，`DEFAULT now()` 由库填）、不进领域、与 `recorded_at`（编排时钟给的登记时刻，随记录往返）不同义、本仓各表通形。不动 `0019`；不加迁移。
4. **`AdoptDutyPaymentVerification` 构造门前核租户**：`strings.TrimSpace(command.TenantID.String()) == ""` → `SettlementInputNotAccepted`，与 `RequestBuyEvaluationHandler.Handle` 同形。`TestAVerificationReferenceMissingADimensionIsNotAccepted` 加「缺租户」一例：未受理、零写。
5. **第五条只核**：核 `assemble.go` 头注写实与否（见「缺口」五）、在 `HandleFormedDutyPaymentVerification` 包装读口错误处补一句注释讲清为什么今天这样包不会吞掉词汇分歧；语义零改。改不改分格写进「要裁的」。

## 红线

- 除做法 1 / 2 / 4 的防律与其用例外零业务语义改动；做法 3 / 5 只注释。
- 不动 `internal/settlementaccounting/adapters/inbox/**`（通道 5 在 sa-cc/01 加文件）、不动 `cmd/**`（同一撞点）。
- 不改已施加迁移、不加迁移。
- 注释中文、不写行号不写计数；新增拒 nil 用例照 `errors.Is`。

## 完成判据

1. `NewRequestBuyEvaluationHandler` 每口各缺一 → `errors.Is(err, application.ErrNilDependency)` 且文本含口名；`go test -count=1 ./internal/settlementaccounting/application/` 绿。
2. `EvaluationRequests.Save` 键与对象不符 → `EvaluationRequestSaveOutcomeInvalid` + 错误、库上零行；真库用例一条绿（带 DSN）。
3. `evaluation_request.go` 读口头注含「审计列」与「recorded_at」两个词；`0019` 零 diff；`migrations/` 零新文件。
4. `AdoptDutyPaymentVerification` 空租户 → `SOURCE_NOT_ACCEPTED`、登记册零写；用例一例。
5. 第五条：`HandleFormedDutyPaymentVerification` 语义零改（diff 只有注释行）；`assemble.go` 零 diff；判断写进「要裁的」。
6. `gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；`go test -count=1` SA application + SA adapters/postgres（带 DSN）+ SA adapters/customscompliance + `./internal/architecture/...` 绿；机制清点零差。
7. 完成记录随最后一笔代码同笔提交（通道 1 21:2x 新纪律），逐笔 SHA、五条各对应哪笔、③ 选了哪边与为什么。

## 地盘

`internal/settlementaccounting/application/request_buy_evaluation.go` 及其测试、`internal/settlementaccounting/application/adopt_duty_payment_verification.go` 及其测试、`internal/settlementaccounting/adapters/postgres/evaluation_request.go` 及其测试、`internal/settlementaccounting/adapters/customscompliance/adopt_on_duty_payment_verification.go`（只注释）、本票面、sa-cc spec.md 一行。**不动** `adapters/inbox/**`、`cmd/**`、迁移、`ports`、`domain`。

## 要裁的

1. **（已裁——2026-09-11 21:5x 通道 1 推送方：准，同分支再一笔收）** ~~`HandleFormedDutyPaymentVerification` 包装读口错误时要不要让 `ErrUntranslatableReference` 原样穿过~~（09 评审 Standards (2) 的处方）。本票作者的判断：**该穿**——包装前一句 `errors.Is(err, ErrUntranslatableReference)` 原样上抛，代价一行，换来 `assemble.go` 头注「`ErrUntranslatableReference` 不在名单」对读口内部那条也成立，而不是靠两侧构造门恰好同形。今天不可达、不阻任何票。**裁决**：准。理由：ADR-0029 按恢复动作分格——译不出是编程 / 数据错误，重投不自愈，折进重投哨兵一旦可达就是永远重投；与同函数对自己译出的那几处原样上抛同形；不可达的防律与本票 ④ 同一类。落法：一行 `errors.Is` 穿过 + 一例（替身读口回 `ErrUntranslatableReference` → 结果 `errors.Is` 它且不是 `ErrVerificationNotVisible`），⑤ 那句注释随之改口；不带 DSN。

## 参照

[08](08-sa-evaluation-request-orchestration-records-source-references.md) Comments「评审 ← 通道 4」Standards ①②③ 与「进 main 记录」推送方处置；[09](09-sa-consumes-duty-payment-verification-envelope-into-advance-recovery.md) Comments「评审 ← 通道 3」Standards (1)(2) 与「进 main 记录」推送方处置；[14](14-adopt-digest-header-and-duty-reconciliation-handler-rejects-nil.md)（同形小票先例）；`internal/settlementaccounting/application/apply_pre_acceptance_control.go`（`ErrNilDependency` / `NewApplyPreAcceptanceControlHandler`）；`internal/customscompliance/adapters/postgres/duty_payment_reconciliation.go`（`SaveVerification` 键与对象一致的形）；`migrations/settlement_accounting/0019_evaluation_request.sql`、`0020_duty_payment_verification_adoption.sql`（`refs_not_blank`）；`cmd/parcel-dispatch/assemble.go`（`dutyPaymentVerificationUndecidedSentinels` 头注，只读）；`internal/platform/migrate`（业务迁移 checksum 纪律）。

## Comments

- 2026-09-11 21:2x · 通道 6（task-63bd3adc，基远端 main `02e1dfc4`）：立票，Status 直接 in-progress，本笔只票面 + spec 16 行，未动代码。能力边界：读过五处目标符号全文、`NewApplyPreAcceptanceControlHandler` 拒 nil 表、CC `SaveVerification` 前两道门、全仓迁移里 `inserted_at` 的分布、`pgtest` 的 DSN 跳过纪律、08 / 09 两份评审原话与推送方处置；**没读** `assemble.go` 除 `dutyPaymentVerificationUndecidedSentinels` 头注以外的部分（本票不动它）。
- 2026-09-11 21:5x · 通道 6（task-63bd3adc）· **完成记录**，分支 `mcp6-sacc16` 基远端 main `02e1dfc4`（推送方重放后 main 上 SHA 会换，对照由推送方在「进 main 记录」补）：
  - 立票笔 `2cef827c`（票面 + spec 16 行，无代码）。
  - **① `9d48e8ca`**：`request_buy_evaluation.go` `NewRequestBuyEvaluationHandler` 改表驱动、每口缺一 `fmt.Errorf("%w: %s", ErrNilDependency, name)`，沿用同包既有 `ErrNilDependency` 不另铸（其文本仍是「pre-acceptance control dependency is nil」，口名跟在后面点名——名字偏窄是既有哨兵的事，改它要动 `apply_pre_acceptance_control.go`，不在本票地盘）；`TestTheHandlerRefusesANilDependencyAtConstruction` 改断言 `errors.Is` + 文本含口名 + handler 为 nil，另加口齐不拒一句。
  - **④ `0d25b898`**：`adopt_duty_payment_verification.go` `AdoptDutyPaymentVerification` 构造门前 `strings.TrimSpace(command.TenantID.String()) == ""` → `SettlementInputNotAccepted`，注释写为什么不能靠调用方先过 `NewTenantID`；`TestAVerificationReferenceMissingADimensionIsNotAccepted` 加「缺租户」一例（去掉这道门该例红：替身登记册会把它存下、答`已采用`）。
  - **② + ③ `2aa0c463`**：`evaluation_request.go` `Save` 写前两道门——键的租户 / 请求 ID 零值拒、`record.Key.Request != record.Request.ID()` 拒，都交 `EvaluationRequestSaveOutcomeInvalid` + 错误、不发 INSERT；真库用例 `TestSavingAnEvaluationRequestWhoseKeyDisagreesWithItsRequestIsRefused`（键指 `EVREQ-SYN-1`、对象是 `EVREQ-SYN-2` → 拒且两个 ID 都无行；零值键同门）。**③ 取 (b)**：`inserted_at timestamptz NOT NULL DEFAULT now()` 是本仓迁移通形——SA 各表、TF / VE / PG / NO 与 CC `0001_external_result.sql` 都带，评审「SA 既有表若无此形可去」的前提不成立，去掉反让 0019 成为 SA 唯一没有库侧审计列的表；`0019` 已施加、业务迁移 checksum 按文件内容算，表头注无处可写；为一句注释加 `COMMENT ON COLUMN` 迁移不值——与推送方倾向同向。注释落 `evaluationRequestColumns` 头注：库侧审计列、不进领域、与 `recorded_at`（编排时钟的登记时刻、随记录往返）不同义。`migrations/` 零 diff、零新文件。
  - **⑤ 本笔**：`adopt_on_duty_payment_verification.go` `HandleFormedDutyPaymentVerification` 包装读口错误处加注释、语义零改（diff 只有 `+//` 行）：读口一切错误折进 `ErrVerificationNotVisible`（含 `customsVerificationKey` 可能交回的 `ErrUntranslatableReference`），今天不吞词汇分歧是因为两侧构造门同为非空白校验、那条分支生产上走不到；`assemble.go` `dutyPaymentVerificationUndecidedSentinels` 头注「`ErrUntranslatableReference` 也不在」对本函数自己译出的那几处（租户、`verificationReference`）写实——原样上抛、不在名单、落 `publish_failed`；对读口内部那条是靠不可达成立。**要不要改分格写进「要裁的」1**（作者判断：该穿，一行 `errors.Is` 换头注对所有路径成立；按派单不自己改）。`cmd/**` 零 diff。
  - **判据逐项**：1 ✓（SA application ok；`-v -run TestTheHandlerRefusesANilDependencyAtConstruction` PASS）。2 ✓（带 DSN SA adapters/postgres **PASS 139 / SKIP 0 / FAIL 0**，含新用例；21:4x 占号 / 释号已广播）。3 ✓（`git grep -n '审计列' -- internal/settlementaccounting/adapters/postgres/evaluation_request.go` 一处、同段含 `recorded_at`；`git diff --stat 02e1dfc4 -- migrations/` 空）。4 ✓（`-v -run TestAVerificationReferenceMissingADimensionIsNotAccepted` 五例 PASS 含缺租户）。5 ✓（`adopt_on_duty_payment_verification.go` diff 只注释；`git diff --stat 02e1dfc4 -- cmd/` 空；要裁的 1 已写）。6 ✓（`gofmt -l ./internal ./cmd` 空；`go build ./...` / `go vet ./...` 退 0；`go test -count=1 ./internal/settlementaccounting/... ./internal/architecture/...` 全 ok（未设 DSN，PG 用例跳过；SA adapters/postgres 另带 DSN 跑过一次见判据 2）；机制清点 `tools/mechanism-inventory` 在本树重生成 `git status --porcelain -- docs/product/MECHANISM-INVENTORY.md` 空）。7 ✓（本条即；完成记录与 ⑤ 同笔）。
  - **未动**：`adapters/inbox/**`、`cmd/**`、迁移、`ports`、`domain`、`apply_pre_acceptance_control.go`（`ErrNilDependency` 文本）。
  - **判断题**：(a) ③ 取 (b) 而非新迁移——理由见上，与推送方倾向同向。(b) `Save` 的零值门放在相等核之前——零值对象的 ID 与零值键相等，单比相等拦不住。(c) ⑤ 的注释写在包装处而不是 `ErrVerificationNotVisible` 头注——读到 `%v` 折链那一行的人才会问「里面有没有别的哨兵」。(d) 一笔收 ② + ③：同一文件、③ 只注释，拆两笔要 patch 级暂存，收益不抵。
  - **要裁的 1 裁准后补笔（本笔，21:5x）**：`adopt_on_duty_payment_verification.go` `HandleFormedDutyPaymentVerification` 读口错误包装前加 `errors.Is(err, ErrUntranslatableReference)` 原样上抛，⑤ 那句注释改口为「唯有它穿过、为什么、今天不可达是防律」；新例 `TestAnUntranslatableReferenceFromTheViewIsNotDressedAsInvisibility`（替身读口回 `ErrUntranslatableReference` → 结果 `errors.Is` 它、不是 `ErrVerificationNotVisible`、零采用）。`gofmt -l` 空、`go vet` 该包 0、`go test -count=1` SA adapters/customscompliance + `cmd/parcel-dispatch`（未设 DSN）ok；不带 DSN（纯应用层适配器）。判据 5「语义零改」自此改为「按裁决改一格」，`cmd/**` 仍零 diff——`assemble.go` 头注「`ErrUntranslatableReference` 也不在」现在对读口路径也靠分格成立，不再靠不可达。
- **评审 ← 通道 1（推送方，非作者；作者通道 6 已无会话、其余通道无人空闲）· 钉 `ffbe1f14` · 22:3x**。对象 `git diff main...mcp6-sacc16 -- internal/`，只读，未跑测试（验证由重放 tip 全量闭合，见下）。
  - **Standards**：阻断 无。非阻断 ① `NewRequestBuyEvaluationHandler` 缺件错误文本是「settlement accounting: pre-acceptance control dependency is nil: evaluation request registry」——沿用既有哨兵是对的（两个「唯一答复」不能并存），但哨兵文本里的「pre-acceptance control」对本编排是一句错话；改文本要动 `apply_pre_acceptance_control.go`，不在本票地盘，归 SA owner 一句（作者完成记录已自报）。无发现：五处改动全落票面地盘，`cmd/**`、`adapters/inbox/**`、`migrations/`、`ports`、`domain`、`apply_pre_acceptance_control.go` 零 diff（`git diff --stat main...mcp6-sacc16` 逐路径核）；`Save` 两道门在 `RequireExecutor` 之前、零值门先于相等核（判断题 (b) 成立——零值对象的 ID 与零值键相等）；`ErrUntranslatableReference` 穿过后 `assemble.go` `dutyPaymentVerificationUndecidedSentinels` 头注「不在名单」对读口路径也成立、落 `publish_failed`；注释中文、无行号无计数，引 ADR-0029 与 CC `SaveVerification` 的形。
  - **Spec**：阻断 无。非阻断 无。无发现：做法 1–5 与要裁的 1 裁决逐条对得上 diff；判据 3 `evaluationRequestColumns` 头注含「审计列」「recorded_at」，`0019` 零 diff、`migrations/` 零新文件；判据 4 缺租户一例入 `TestAVerificationReferenceMissingADimensionIsNotAccepted`；判据 5 改为「按裁决改一格」且 `cmd/**` 零 diff；判据 7 完成记录随 ⑤ 同笔（`dab69b4f`）、补笔票面同笔（`ffbe1f14`）。判断题 (a) 取 (b) 不加迁移、(c) 注释写在包装处、(d) ② + ③ 一笔——同意。
- **进 main 记录（通道 1 推送方，22:2x–22:5x）**：原通道 1 21:5x 已在 `%TEMP%\idp-replay-sacc16`（detached `6a4fd7c8`）`cherry-pick main..mcp6-sacc16` 六笔零冲突到 `3049ca1b`，随后 crash（未全量、未 ff、未推）；本会话接着在其上叠 lc/37 六笔 + sa-cc/01 两笔（批 tip `3c57811d`），本票文件对分支 tip 只差同批 sa-cc/01 的 spec 行。清点重生成本票零差（推送方清点笔 `fe975e1b` 的数字全来自 sa-cc/01）；22:41 占号，`fe975e1b` 带 DSN 全量 **110 ok / 0 FAIL / 15 无测试 / 0 cached**（22:41:35→22:43:49），`-v` 探针 PASS 59 / SKIP 0（含 `TestSavingAnEvaluationRequestWhoseKeyDisagreesWithItsRequestIsRefused` 真库实跑、`TestAnUntranslatableReferenceFromTheViewIsNotDressedAsInvisibility`、`TestTheHandlerRefusesANilDependencyAtConstruction`、`TestAVerificationReferenceMissingADimensionIsNotAccepted`）；22:44 释号。SHA 对照：`2cef827c→a11abb96` / `9d48e8ca→33f2272f` / `0d25b898→c9d6f6e7` / `2aa0c463→324aa474` / `dab69b4f→4776b1b8` / `ffbe1f14→3049ca1b`。`mcp6-sacc16` → `merged/mcp6-sacc16`（指针留，远端删），`D:/tops/idp-parcel-mcp6-sacc16` 比内容后拆。**候选后继（归 SA owner）**：`ErrNilDependency` 文本去掉「pre-acceptance control」限定（它已是两只构造器的共用哨兵）。
