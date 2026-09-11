# UC-CC-003 步 7「记录凭证门禁」只判不记：`JudgeCredentialApplicability` 算得出四格，没有任何编排把它落成就绪判断里的一格

Category: enhancement
Status: resolved——**已进 main，2026-09-11 16:3x**（与 sa-cc/08 同批重放；main 上代码 `a32c80eb`、票面 `258ff7b3`、批清点 `dcb74785`；非作者评审 ← 通道 6 两轴 0 阻断 / Standards 2 + Spec 2 非阻断随票记，见 Comments）；此前 resolved——2026-09-11 16:1x 通道 4 完工，等非作者评审进 main（分支 `mcp4-sacc04`，基 main `7c37253f`，代码 tip `d69e18c7`；完成记录见文末）；此前 in-progress——2026-09-11 15:3x 通道 4 接单 task-f9d6cd97（接替 crash 的 task-42663c03），树 `D:/tops/idp-parcel-mcp4-sacc04`；此前 ready-for-agent——2026-09-10 17:3x 通道 5 按通道 1 派单 task-9a2ff746（用户授权代裁，CC owner 口径）写入裁决：一册（独立「凭证门禁判断」登记册，就绪判断按引用绑定）、评估请求到达时算一次（[ADR-0137](../../../docs/adr/0137-customs-gate-judgments-are-registered-facts-driven-by-assessment-requests-payment-gate-rule-is-registered-and-funds-facts-are-minted-only-in-settlement-accounting.md) 决定一 / 二，见「要裁的」下「裁决」），本票再无待裁问题。此前 draft——2026-09-10 通道 4 立票（task-9880bbc9），只写票面未动代码；取证锚 `3f485e97`
Blocked by: 无

## 缺口（取证于 `3f485e97`）

- `internal/customscompliance/application/judge_credential_applicability.go` 头注原句：「本用例只判、不记：步 7 写的『记录凭证门禁』那半留给就绪判断的编排——今天就绪判断是带依据引用登记进来的事实（`RegisterReadiness` 收 `ReadinessBasisReference`），UC-CC-003 步 3–10 没有逐门禁计算的编排，凭证门禁作为其中一格的持久化随那条编排一起落」。
- `git grep -n JudgeCredentialApplicability -- cmd/` 零：判断口没有生产调用方。
- mech/07「没做、留给后继票的」第 1 条：「UC-CC-003 步 7『记录凭证门禁』的持久化——等就绪判断的逐门禁编排。」

## 语言从哪里来

- UC-CC-003 步 7 行：「核验监管凭证身份、适用性、有效期和截至当前的可用依据 → 记录凭证门禁；不占用、释放或核销」（`CC-RULE`、`CC-LIFE`）；`AT-CC-056`：「凭证门禁满足并保存适用性和截至时点；本用例不占用或核销额度」。
- CC `CONTEXT.md`：本上下文拥有「监管凭证及其适用性和使用关系」；「放行门禁核对必须绑定当前有效的监管程序……门禁满足不生成放行」。

## 做法

1. 先答「要裁的」第 1 条——凭证门禁是就绪判断的**一格**（随 `RegisterReadiness` 的依据引用一并落、就绪判断本身仍是登记进来的事实）还是一条**独立记录**（`credential_gate` 登记册，就绪判断按引用指向它）。
2. 无论哪种：记录带凭证身份 / 版本 / 判断结论四格之一 / 截至时点 / 依据引用；同键同内容重放 `已存在`，换内容新版本不覆盖（与 CC 既有登记册代数一致）。
3. 新迁移序号（`migrations/customs_compliance/` 当前最大 `0017`，开工时重取）；postgres 适配器 + 真库用例。
4. `JudgeCredentialApplicability` 从此有生产调用方；`AT-CC-056` 有代码实现。

## 红线

- 只记门禁判断，不占用、不释放、不核销（那三件是 UC-CC-005 步 7/9、UC-CC-006 步 7 的凭证使用生命周期，时点由真实程序定 `PAR-CUS-04`）。
- 「未登记」与「不适用」两格不得压成一格（头注理由：租户上线前每一次判断都会读成「凭证不适用」）。
- 真实凭证与真实程序属实例半边，不写默认。

## 完成判据

1. `git grep -w JudgeCredentialApplicability -- 'internal/*.go' ':(exclude)*_test.go'` 有 application 层以外的调用（编排或装配）。
2. 应用层：四格各一条落库路径；重放 / 换内容两格。
3. 真库往返；`AT-CC-056` 用例点名 Covers。
4. 基线不加宽；清点 tip 重生成。

## 地盘

`internal/customscompliance/application/`（新编排文件或 `register_readiness` 相邻处，按裁决）、`internal/customscompliance/ports/`、`internal/customscompliance/adapters/postgres/`、`migrations/customs_compliance/`（新序号）。不动 `parcel-api` 端点表（登记面归 [07](07-cc-credential-and-duty-reconciliation-registration-faces.md)）。

## 要裁的

1. **一格还是一册**：凭证门禁随就绪判断的依据引用落（就绪判断仍是登记进来的事实，只多一维），还是独立登记册由就绪判断引用——UC-CC-003 步 3–10 今天没有逐门禁计算编排，选后者等于先立第一册；选前者要动 `RegisterReadiness` 的形。归 CC owner。
2. **谁触发判断**：就绪评估请求到达时算一次，还是凭证登记 / 程序变更时重算——头注写「续办是由凭证责任流程形成有效依据后重新评估（UC-CC-003 门禁表第 4 行）」，触发面本票倾向「评估请求到达时」。归 CC owner。

### 裁决

（用户 17:0x 授权、通道 5 按 CC owner 口径代裁并落 [ADR-0137](../../../docs/adr/0137-customs-gate-judgments-are-registered-facts-driven-by-assessment-requests-payment-gate-rule-is-registered-and-funds-facts-are-minted-only-in-settlement-accounting.md)，2026-09-10 17:2x；task-b941ce87 分类两条均 B、task-9a2ff746 落笔。硬句不改；拿不准的在 ADR「越权风险点」1 / 2 / 5。）

- **1 → 一册：独立「凭证门禁判断」登记册，就绪判断按不可变引用绑定它，`RegisterReadiness` 的形不动**（ADR-0137 决定一）。理由：UC-CC-003 七道门禁今天没有一道机器算并落库，第一册定下「逐门禁判断 = 登记事实、就绪判断只引用不内嵌」的形，后面的门照抄；作就绪判断的一格则每加一道门改一次聚合。UC-CC-003 范围节「保存每项门禁的依据……和结果」由对不可变门禁记录版本的引用成立（越权风险点 1）。做法 1 由此定：新编排文件 + 新登记册（ports 写读一对、postgres、新迁移序号开工重取），不改 `register_readiness` 的形。
- **2 → 评估请求到达时算一次并登记；凭证登记 / 程序变更不后台重算**（ADR-0137 决定二）。理由：UC-CC-003「原判断失效后不得因为……凭证恢复……自动恢复，必须引用新依据形成新的判断版本」——后台重算把门禁翻成满足正是那句禁的；来源变化的正当作用是让既有判断`不再就绪`（失效规则，另一条编排），不是产出新判断。续办流程要不要自动发起一次新的评估请求是越权风险点 2，本票不做。

## 参照

[mech/07](../../mechanism-executor-triage/issues/07-cc-four-executors-behind-existing-uc-steps.md) CC-a 与「没做」第 1 条；UC-CC-003；[remaining-work-dd5ed934.md](../../unresolved-review-20260904/remaining-work-dd5ed934.md) 五-8 ①。

## Comments

- 2026-09-10 · 通道 4：立票。未动代码。
- 2026-09-11 15:3x · 通道 4：接单 task-f9d6cd97 开工，基 main `7c37253f`，按「裁决」一册 + 评估请求到达时算一次实现；本笔只改 Status。
- 2026-09-11 16:1x · 通道 4：完工，Status → resolved，完成记录见下。两轴评审子代理两次都以「Authentication error」空转（同通道 1 15:0x 所记），改由作者按 `/code-review` 正文串行自跑两遍、结果随记；**这不顶替非作者评审**，进 main 前仍要一位非作者评。

## 完成记录（通道 4，2026-09-11）

### 逐笔 SHA（分支 `mcp4-sacc04`，基 main `7c37253f`）

| 笔 | SHA | 内容 |
|---|---|---|
| 1 | `d59015d3` | 票面 Status → in-progress |
| 2 | `a6874e0f` | domain `credential_gate.go`（`CredentialGateJudgment` / `CredentialGateConclusion` / `CredentialGateBasisReference` / `ResponsibleRoleReference` / `RecordCredentialGate`）+ ports（`CredentialGateKey` / `CredentialGateDigest` / `CredentialGateRecord` / `CredentialGateRegistry` 写口 / `CredentialGateView` 读口）+ application `record_credential_gate.go`（`RecordCredentialGateHandler`）与三份用例 |
| 3 | `2a503c00` | `migrations/customs_compliance/0018_credential_gate_judgment.sql` + postgres `credential_gate_registry.go`（`CredentialGateRegistrations` / `CredentialGateView`）+ 真库用例 |
| 4 | `3a2454d9` | 机制清点在 `2a503c00` 干净检出重生成（CC 生产 84→87 / 测试 84→87、应用编排 16→17、postgres 适配器 38→39、迁移 163→164、端口声明 391→393；缺口两口径不变） |
| 5 | `d69e18c7` | 两轴自评修：判断半边接进构造门（去掉只有一个实现的 `CredentialApplicabilityJudge` 接口，Deps 改收 `ports.CredentialView`）；postgres 用例头注去跨文件计数 |

代码 tip = 分支 tip = `d69e18c7`。动过的文件：上表五笔所列，另 `docs/product/MECHANISM-INVENTORY.md`（生成物）与本票面。**没动**：`register_case_configuration.go`（`RegisterReadiness` 的形）、`parcel-api` 端点表、`internal/architecture/*baseline.txt`、`reconcile_duty_payment.go`。

### 完成判据逐项

1. **`JudgeCredentialApplicability` 有 application 层以外的调用（编排或装配）**——**编排有、装配无，如实记**。编排：`record_credential_gate.go` 的构造门 `NewRecordCredentialGateHandler` 接上 `NewJudgeCredentialApplicabilityHandler`，`Handle` 调它算四格（`git grep -n -E "JudgeCredentialApplicability(Handler|Command)" -- 'internal/*.go' ':(exclude)*_test.go'` 在 `d69e18c7` 命中该文件的构造与调用两处，此前零）。票面写的 `git grep -w JudgeCredentialApplicability …` 那条以 `-w` 只匹配得到注释——生产标识符是 `…Handler` / `…Command`，`-w` 不认前缀，那条 grep 的字面结果与「有没有调用方」无关，评审请以上一条为准。装配：**今天没有装配点**。`cmd/parcel-dispatch/assemble.go` 的 CC 消费者一族全是 inbox 驱动，仓内没有「评估请求到达」的入向面（UC-CC-003 步 1–2 就绪接入尚无票）；`cmd/parcel-api/unwired_orchestration.go` 只列端点表上已登、编排未接的口，而本票不动端点表（且该文件正在通道 2 sa-cc/07 步二占号中），所以也没在那里列。装配点属「就绪接入」那张后继票：它接进来的是 `RecordCredentialGateDeps{Credentials, Registry, Clock}` 一口整步。
2. **应用层四格各一条落库路径；重放 / 换内容两格**——`TestEveryCredentialGateConclusionHasItsOwnRecordedWord`（不适用 / 凭证未登记 / 未决）+ `TestAnApplicableCredentialGateIsJudgedAndRecorded`（适用）四格各落一版；`TestReplayingACredentialGateSplitsExistingFromANewVersion` 重放`已存在`（时钟走了不算换内容）、换依据追加新版首版原样。
3. **真库往返；`AT-CC-056` 用例点名 Covers**——postgres 七条用例（往返、四格各自读回同一词、同键不顶替 + 换内容追加、键与判断不符拒、未登记 found=false / 空指纹拒、CHECK 旁路拒、无事务 `ErrTransactionRequired`）带 DSN 全 PASS 0 SKIP；`AT-CC-056` 在 domain / application / postgres 三份用例的 Covers 里各点名一次。
4. **基线不加宽；清点 tip 重生成**——`internal/architecture` 两份 baseline 零改动（`git diff --stat 7c37253f -- internal/architecture` 空），`./internal/architecture/...` 绿；清点 `3a2454d9` 在 `2a503c00` 干净 worktree 重生成。

### 判断题（供评审）

- **幂等键含申报单元**：ADR-0137 决定一列的行内容没有「单元」，但就绪判断是按单元登记的（`RegisterReadinessCommand.Unit`），UC-CC-003「每个判断必须绑定……申报单元」；门禁记录若不带单元，两个单元同一张凭证同一截至时点会撞成一版，就绪判断绑引用时分不出谁的。故键取（租户、单元、凭证身份）+ 内容指纹。
- **责任角色进指纹、判断时刻不进**：同请求换角色重发读作另一版（那一版由谁负责是内容），时钟走了不是（重放比内容不比时刻，判据同 `sameCollaboration`）。
- **「凭证身份与版本」折成一列**：凭证册是「一身份一版」（0014 自注），没有第二个版本维可引；表与注释都写明这是折而不是漏。
- **`未决`也落一版**：判断口的`未决`是读凭证册的口故障，落成的是「这次评估请求上这道门没判出来」；重试是再发一次请求另成一版，不改这一版。按票面「四格各一条落库路径」做；若 owner 认为技术未决不该成为登记事实，改的是 `gateConclusionOf` 一处与 CHECK 一词。
- **「评估请求到达 → 判断 → 登记 → 就绪判断绑引用」里的最后一格不在本票**：ADR-0137 Consequences 把它写在票 04 一句里，但把单道门禁的结果绑成就绪判断等于替 UC-CC-003 步 10 的合取做决定（「所有适用门禁是合取关系」），今天没有逐门禁汇总的编排，本票只交出可绑定的键（`CredentialGateResult.Key`）。这一格归就绪接入 / 步 10–11 的后继票，票面「做法」1–4 与「完成判据」1–4 也没要它。

### 两轴自评（`/code-review`，基线 `7c37253f`，作者串行自跑，不顶替非作者评审）

**Standards**：① 判断半边原以 `CredentialApplicabilityJudge` 接口进 Deps、全仓只有一个实现——Speculative Generality，已修（`d69e18c7`），改收 `ports.CredentialView` 由构造门自己接判断口；② postgres 用例头注「九件 / 九列」数的是另一文件里的列——AGENTS「不数别处的东西」，已修（同笔）。其余：注释中文；无「硬句 NNN」行号引用；`String()` 四格齐（enum 门禁绿）；PBC08 负向证据 `TestCredentialGateWritesRefuseToRunOutsideATransaction` 在（架构门禁绿）；`WithinTransaction` 闭包内无 `t.Fatal`；夹具全 `SYN-`。0 未修。

**Spec**：① 判据 1 的「装配」半边今天接不上（见上），如实记，非阻断；② ADR Consequences 那句「就绪判断绑引用」不在本票（见判断题末条），非阻断；③ 键含单元 / 角色进指纹 / 未决落版三处是作者裁的形，已列判断题。缺失 0、越界 0、看着实现了但形不对 0（按作者读法）。

### 验证（tip `d69e18c7`）

- `gofmt -l internal/customscompliance` 空；`go build ./...`、`go vet ./internal/customscompliance/... ./migrations/...` 退 0。
- 带 DSN（55432 占号 / 释号各两轮）：`go test -count=1 -p 1 ./internal/customscompliance/... ./internal/architecture/... ./migrations/... ./cmd/...` 全绿 0 FAIL；`-run CredentialGate ./internal/customscompliance/adapters/postgres/ -v` 7 PASS 0 SKIP。不跑全量（派单如此）。
- 新 `.go` 全部 `gofmt -w` 后入库，`git ls-files --eol` 皆 `i/lf w/lf`；`0018_*.sql` CR 数 0、无 BOM，`migrations` 的 EOL 守卫绿。

### 评审 ← 通道 6 · 钉 `4c764f40`（代码 `d69e18c7`）· 基线 `7c37253f` · 2026-09-11 16:2x（机时；评审自标 16:5x 为估错）

隔离检出 `%TEMP%\idp-review-sacc04`，只读、不带 DSN：`gofmt -l` 空、`go vet ./internal/customscompliance/...` 0、`go test` CC domain + application + postgres + architecture 四包 ok（PASS 325 / SKIP 242 / FAIL 0；本票 domain 4 + application 6 PASS，postgres 7 例 SKIP **未验**，推送方全量闭合）；`git diff --stat 7c37253f -- internal/architecture cmd register_case_configuration.go reconcile_duty_payment.go` 空。

- **Standards**：**阻断 无**。**非阻断 2**：(1) `ports/ports.go` `CredentialGateDigest` 把结论以 `strconv.Itoa(int(Conclusion()))` 枚举整数入指纹，而库存的是 `String()` 封闭词；枚举中间插一格所有旧指纹静默失配（重放不再答已存在而追加新版），换成 `String()` 即可；CC 既有 `verificationDigest` / `FindingsDigest` 也不带规范化版本前缀（ADR-0014 在 CC 未落），同形不单挑。(2) `application/record_credential_gate.go` `Handle` 登记面错误答 `CredentialGateRecordUndecided` 不带续办引用；CC 包内 `dutyUndecided` 同形，记为已知。**无发现（核过）**：注释全中文，无行号；跨文件引用全用符号名（`sameCollaboration` / `SaveVerification` / `NewDutyPaymentReconciliationHandler`），「八件」数的是同文件 struct；夹具全 `SYN-` / `tenant-a`，无真实凭证类型 / 程序；红线① 表与类型无额度 / 占用 / 释放 / 核销列，编排对凭证册只读；红线② `CREDENTIAL_NOT_REGISTERED` 与 `NOT_APPLICABLE` 在 domain 四格 / `gateConclusionOf` / CHECK / `TestEveryCredentialGateConclusionHasItsOwnRecordedWord` 四处分立；红线④ `NewRecordCredentialGateHandler` 逐口拒 nil 带 `ErrNilDependency` 点名（形照 `NewDutyPaymentReconciliationHandler`），用例 `TestTheCredentialGateHandlerNamesWhichDependencyIsMissing`；登记册代数 `ON CONFLICT DO NOTHING` 无 UPDATE，同键答 `CaseConfigurationAlreadyRegistered`、换内容换指纹追加，与 0016 `duty_payment_verification`（三维 + `version_digest` PK）同形；写口核键与对象一致；`CaseConfigurationSaveOutcome` 是 CC 登记册共用写入代数，复用合理；`ports` 里放指纹函数有 `FindingsDigest` 先例；迁移 0018 租户列 / not_blank / 封闭词 CHECK / 不外键到 `regulatory_credential`（「未登记」那格要落得进）口径一致；`WithinTransaction` 闭包内无 `t.Fatal`；读回经 `RecordCredentialGate` 重建；域层不导 pgx。
- **Spec**：**阻断 无**。**非阻断 2**：(1) 判据 1：票面 grep `-w JudgeCredentialApplicability` 字面只中注释（生产名是 `…Handler` / `…Command`），作者改用 Handler / Command 名核并如实记「编排有、装配无」——符合票面「编排**或**装配」二选一；`unwired_orchestration.go` 是端点占位、本编排无端点，不列是对的。若要一处「已装配未接线」可核，仓内同形先例是 label-channel/34 / sa-cc/08 的 `cmd/parcel-api` fail-fast 组合根（`RecordCredentialGateDeps{Credentials, Registry, Clock}` 三口都有真适配器），可另笔补，不阻。(2) 做法 2 写「凭证身份 / 版本」，实现折成 `credential_id` 一列（理由在 domain 头注与 0018 头注）——与 0014「一身份一版」一致，票面措辞与实现形不同，实现对。**无发现（判据逐项）**：2 ✓ 四格各一落库路（`TestEveryCredentialGateConclusionHasItsOwnRecordedWord` + `TestAnApplicableCredentialGateIsJudgedAndRecorded`），重放 `EXISTING` / 换依据新版（`TestReplayingACredentialGateSplitsExistingFromANewVersion`）；3 ✓ 真库 7 例在（往返 / 四词 / 同键不顶替 + 换内容追加 / 键不符拒 / 缺版 found=false / CHECK / 无事务）**未验**；`AT-CC-056` 在 domain / application / postgres 三份 Covers 各点名；4 ✓ architecture 两 baseline 零改，清点 `3a2454d9`。越界：`RegisterReadiness` 形 / `parcel-api` 端点表 / `reconcile_duty_payment.go` 零 diff；没做票面没要的。评价请求到达才算（无后台重算入口）符合 ADR-0137 决定二。
- **五条判断题**：① 键含单元——站得住（就绪判断按单元登 `RegisterReadinessCommand.Unit`，不带单元两单元同凭证同 as_of 撑成一版）。② 角色进指纹、时刻不进——站得住（责任是内容，重放比内容不比时钟，同 `sameCollaboration`）。③ 版本 = 身份折一列——站得住（0014 一身份一版，没有第二版本维可引）。④ 未决也落版——按票面「四格各一落库路」站得住，但 UNDECIDED 是读凭证册口故障（ADR-0029「等依赖」一格）落成不可变事实是个取舍——**后继「就绪绑引用」那票必须拒绑 UNDECIDED 版**，建议在本票或 ADR-0137 越权风险点补一句，不阻。⑤ 就绪绑引用留后继——站得住（做法 / 判据都没要，单门绑就绪 = 替步 10 合取拿主意）。
- **合计**：Standards 0 阻断 / 2 非阻断（最重：指纹用枚举整数）；Spec 0 阻断 / 2 非阻断（最重：判据 1 装配半边如实缺席）。真库 7 例未验，推送方重放时全量闭合。

### 进 main 记录（通道 1 推送方，2026-09-11 16:3x；与 sa-cc/08 同批）

`%TEMP%\idp-replay-w5a` detached `807de571`（= 远端 main），先 `cherry-pick` 本票五笔（跳过作者清点 `3a2454d9`）再叠 08 六笔，零冲突（本票与 main 自 `7c37253f` 起的改动只重叠 `spec.md` 与清点两件簿记，代码零重叠；本票与 08 代码零重叠）；本票文件对分支 tip 零差。SHA 对照：`d59015d3→cb9eec19` / `a6874e0f→2dd947fd` / `2a503c00→f3305906` / `d69e18c7→a32c80eb` / `4c764f40→258ff7b3`；作者清点 `3a2454d9` 在单票底座上生成、叠加后不再成立，跳过，两票由推送方在栈 tip 重生成一笔 **`dcb74785`**（CC 85→88、迁移 17→18；SA 一并兑）。**验证钉 `dcb74785`**：`gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；16:17 占号，带 DSN `go test -p 1 -count=1 ./...` **108 ok / 0 FAIL / 15 无测试 / 0 cached**（16:17:14→16:19:23，129 s；包数 107→108 是 08 的新包 `adapters/identity`）；`-v` 探针 `-run 'CredentialGate|EvaluationRequest'` CC postgres + application + SA postgres + `cmd/parcel-api` PASS 21 / SKIP 0（评审侧「未验」的真库 7 例在此实跑）；16:19 释号。簿记一笔在其上（本票 Status + 评审 + 本条、票 08 同、sa-cc spec 04 / 08 / 11 / 15 行、票 11 / 15 Blocked by 句、tasks.md），纯 .md 自审；`ls-remote` 核 `807de571` 未动 → `merge --ff-only` → `push <sha>:main`。分支 `mcp4-sacc04@4c764f40` 作封存出处、改名 `merged/`、远端删；`D:/tops/idp-parcel-mcp4-sacc04` 比内容后拆。**推送方对评审的处置**：两轴 0 阻断；四条非阻断随票记——Standards (1) 指纹用 `String()` **随 sa-cc/06 同通道同目录一并改**（派单里写明），(2) 记为已知同形；Spec (1) 不另笔（编排无触发面，同 04 判据 1 原句「编排或装配」），(2) 票面措辞与实现形之别记在此不改票面；判断题 ④ 归 CC owner：就绪绑引用那票立票时写明**拒绑 UNDECIDED 版**。**下游**：sa-cc/06 可派（通道 4 同通道接）；sa-cc/15 软阻三票全部进 main，解除。
