# SA 收 ADR-0127 的 contract 段：`Deps.CreditBasis` mandatory、暴露账本行持久化政策引用；PS 夹具先补 `CreditBasisView` 替身

Category: enhancement
Status: resolved——**2026-09-09 21:4x 进 main `84c62c3c`**（通道 1 推送，重放；分支→main SHA 对照与验证见文末「进 main 记录」）。20:3x 通道 3 完工（分支 `mcp3-wbr09` 基 `d5a35960`，三笔 `5bead280` / `d45a7a34` / `d23956d7` 已推 origin；完成记录见文末）。此前：19:4x 通道 3 接续（接续单 task-de74738a-f485-422e-8e85-7a78d474d1ad；前一任 18:4x 自领后 18:5x 中断，树上留一份未提交的 PS 夹具
测试 +62/−19、mtime 18:47:46）。接手对照：先自列第 1 步判据（三格替身 / 接口断言 / 三处 `Deps` 构造全接 / 账期夹具额度与状况
同值以保住既有用例的读法 / 不改既有断言），再读 diff——逐条对上，PS 包 build/vet/test/gofmt 绿，原样接着用。第 2 步起由 20:08 接手的新会话做
（通道 1 20:08 回执核过构造点 11 处、迁移号 0017、验证范围改口）。
2026-09-09 通道 1 代裁立票（用户授权自决）：wbr/03 完成记录「未落三件」之②，ADR-0127 决定五写明「contract 段——依赖 mandatory、
nil 在构造期拒——随 PS 那份夹具补上 `CreditBasisView` 替身的那笔一起落，票面记为 SA 后续项」；Consequences 写明「暴露账本行上不持久化政策引用（要 SA 迁移，
随 contract 段另立）」。两半同票、先 PS 夹具后 SA contract，一个通道做完，PS 那份测试文件改前占号
Blocked by: 无

## 条目

- `internal/settlementaccounting/application/apply_pre_acceptance_control.go`：`ApplyPreAcceptanceControlDeps.CreditBasis` 为 nil 时 `exposeCredit` 沿旧路（额度取
  登记状况、结果不带政策引用）——ADR-0127 决定五的 expand 段，留它只因 `internal/parcelshipment/adapters/settlementaccounting` 的测试夹具直接构造这份 `Deps`。
- SA 暴露账本行今天不持久化「实际采用的政策」引用；`settlement-accounting` CONTEXT 要求「每项信用暴露保存实际采用的政策与范围」，`AT-SA-171` 的政策半边今天只在
  形成时的结果上成立、落库即丢。

## 做法（顺序固定）

1. **PS 夹具（PS 地盘，只动测试）**：`internal/parcelshipment/adapters/settlementaccounting` 里构造 `ApplyPreAcceptanceControlDeps` 的夹具补一只 `CreditBasisView` 替身
   （found=true 带一份 `CreditBasis` / found=false / error 三格可配，照 `PreAcceptanceControlPolicyView` 替身的写法）；单独一笔，提交说明写明是为 SA contract 段铺路。
2. **SA contract 段**：`Deps.CreditBasis` 在构造期拒 nil（与其它 mandatory 依赖同一处、同一错误形状）；`exposeCredit` 删旧路分支——额度一律取信用依据；
   `cmd/parcel-dispatch` 装配已接真适配器（ADR-0127 同笔），零改动应能编过——编不过就是有第三个构造点，找出来一并接。
3. **SA 迁移**：暴露账本行加政策引用列（对象 + 版本两格，与 `CreditBasis` 里的政策版本引用同形；可空只为存量行，新写入非空由应用层保证、CHECK 不追溯）；
   `adapters/postgres` 写入 / 读回带它；`operational_position.go` 读面若列暴露明细则带出政策引用列。
4. 测试：SA 应用层 nil 拒 / 有依据取政策额度 / found=false 落 `CREDIT_BASIS_NOT_CONFIGURED`（既有）；PG 真库写读往返带政策引用；PS 夹具那侧既有用例全绿。
5. `AT-SA-171` 在验收对照里标「政策半边落库成立」；wbr/03 完成记录「未落三件」之② 回指本票。

## 完成判据

`Deps.CreditBasis == nil` 构造期拒；暴露账本行落库带政策引用并可读回；含 DSN 跑 `./internal/settlementaccounting/...`、`./internal/parcelshipment/adapters/settlementaccounting/...`、
`./migrations/`，反向依赖含 `cmd/*` 的包带 DSN 跑；gofmt / vet 0。

## 边界

不动 PC；不动比例额度基数（那是 [10](./10-credit-ratio-base-is-declared-on-the-credit-policy-content.md)）；PS 侧只动那份夹具；不改 ADR-0127 正文。

## 完成记录（2026-09-09 20:3x，通道 3；分支 `mcp3-wbr09` 基 `d5a35960`，每笔已推 origin 同 SHA——推送方重放进 main）

| 笔 | SHA | 内容 |
|---|---|---|
| ① | `5bead280` | PS 夹具补 `creditBasisDouble`（found=true 带 `CreditBasis` / found=false / error 三格，照 `policyDouble`），接进 `newControlFixture` / `termsFixture` / 未配置来源那条内联构造；账期夹具额度取状况登记的同一个数；不改既有断言。票面 → in-progress 同笔（19:53，前一任会话写、接续会话按判据对过后原样用） |
| ② | `d45a7a34` | SA contract 段：`NewApplyPreAcceptanceControlHandler` 返 `(handler, error)`，七件依赖同一道门、同一哨兵 `ErrNilDependency`（包装点名缺件，形照 `internal/accessidentity` `NewMinter`）；`exposeCredit` 删 `creditBasis != nil` 分支，额度一律取信用依据，`CreditPolicy()` 恒随暴露在场。调用点 11 处全接：生产唯一 `cmd/parcel-dispatch/assemble.go`（错误上抛为装配失败）、SA 测试 7 处走 `mustHandler` + 账期用例补 `authorized(t, N)` 替身、PS 夹具 3 处走新增 `mustApplyHandler`。`TestANilCreditBasisViewKeepsTheOldPathAndCarriesNoPolicy` → `TestAnyNilDependencyIsRefusedAtConstruction`（七件逐一抽掉各得哨兵、齐全放行） |
| ③ | `d23956d7` | 暴露账本行政策引用：迁移 `0017_credit_exposure_policy_reference.sql`（一列 `credit_policy_ref text` + CHECK 非空白、NULL 放行）；领域 `WithAuthorizedLimit(limit, policy)` 拒无出处、`CreditStanding.Policy()`、`CreditExposure.Policy()`、`Expose` 对没换上授权额度的登记状况答新哨兵 `ErrStandingNotAuthorized`、`业务限制`同带出处、重放不改口；`RehydrateCreditExposureSpec.Policy` 零值只为存量行；postgres SELECT / INSERT 带列、SET 仍只有状态与释放时间；编排把 `basis.Policy()` 随额度换进状况。既有夹具直接 `Expose` 的四处（释放用例两处、PG 两处）改先换上授权额度。机制清点在本树重生成同笔（迁移 155→156、settlement_accounting 16→17，别无变动） |
| — | 本笔 | 本票 → resolved + 本记录；wbr/03 Comments 末补「未落三件之② 已落」一行 |

**触及文件**：`internal/settlementaccounting/application/apply_pre_acceptance_control.go`（+ `_test.go`、`_credit_basis_test.go`、`release_pre_acceptance_control_test.go`）；`internal/settlementaccounting/domain/credit_exposure.go`、`ledger_rehydration.go`（+ `credit_basis_test.go`）；`internal/settlementaccounting/adapters/postgres/pre_acceptance_control.go`（+ `_test.go`、`operational_position_test.go`）；`migrations/settlement_accounting/0017_credit_exposure_policy_reference.sql`（新）；`cmd/parcel-dispatch/assemble.go`；`internal/parcelshipment/adapters/settlementaccounting/pre_acceptance_control_test.go`（PS 侧唯一一份，票面允许的那份）；`docs/product/MECHANISM-INVENTORY.md`（生成）；本票 + wbr/03 一行。**未碰**：PC 任何一格；`ports.go`（`CreditBasisView` 签名不动）；`operational_position.go` 生产代码（读面只求和不列暴露明细，票面「若列暴露明细则带出政策引用列」的前件不成立）；`migrations.go` / `plan.go`（`all:settlement_accounting` 整目录嵌入，加文件不碰接线）；ADR-0127 正文；比例额度基数（[10](./10-credit-ratio-base-is-declared-on-the-credit-policy-content.md)）。

**验收对照**（票面「做法」1–5 逐条 + 完成判据）：1 PS 夹具替身 ✓（①）；2 contract 段 ✓（②：nil 构造期拒 ✓、旧路分支删 ✓、`cmd/parcel-dispatch` 编过且只有那一个生产构造点 ✓——通道 1 20:08 核的 11 处逐一对上，没有第三个）；3 SA 迁移 ✓（③：占号 0017 ✓、可空只为存量行 ✓、新写入非空由领域门 + 编排保证 ✓、CHECK 不追溯 ✓；写入 / 读回带它 ✓；`operational_position.go` 前件不成立、未动，见上）；4 测试 ✓（应用层 nil 拒 ✓ / 有依据取政策额度 ✓ / found=false 落 `CREDIT_BASIS_NOT_CONFIGURED` 既有 ✓；PG 真库写读往返带政策引用 ✓ + 存量行 NULL 读回为空 ✓ + 混册再保存不动政策列 ✓；PS 夹具那侧既有用例全绿 ✓）；5 `AT-SA-171`「分别保存政策和范围」的政策半边：形成时成立（ADR-0127 那笔）+ **落库成立（本票 ③：`credit_exposure.credit_policy_ref` 随行落库并可读回）** ✓；wbr/03「未落三件」之② 回指 ✓（owner 复核那行已指向本票；本笔在 wbr/03 Comments 末补「已落」一行）。完成判据四句：`Deps.CreditBasis == nil` 构造期拒 ✓；暴露账本行落库带政策引用并可读回 ✓；含 DSN 跑三个包组 + 反向依赖含 `cmd/*` 的包 ✓（见下）；gofmt / vet 0 ✓。边界四条 ✓。

**验证强度**（本 worktree `D:/tops/idp-parcel-mcp3-wbr09` @ `d23956d7` 提交前的同一内容，树干净；门禁容器 `127.0.0.1:55432` healthy，占号 / 关窗各广播一次）：`gofmt -l ./internal ./cmd ./migrations` 零输出；`go build ./...`、`go vet ./...` 退 0；反向依赖用 `go list -f '{{.ImportPath}} {{.Deps}}'` 反查 SA domain，含 `cmd/*` 的是 `cmd/parcel-api` 与 `cmd/parcel-dispatch` 两个；**含 DSN** `go test -p 1 -count=1 -v ./internal/settlementaccounting/... ./internal/parcelshipment/adapters/settlementaccounting/... ./migrations/... ./cmd/parcel-api/... ./cmd/parcel-dispatch/... ./internal/architecture/...`：**11 ok / 0 FAIL / 1 无测试（ports），`--- PASS` 919 / `--- SKIP` 0 / `--- FAIL` 0，22 s**；探针 `TestACreditExposureRoundTripsTheAdoptedCreditPolicy` 含 DSN `--- PASS`。② 那笔单独验过一次无 DSN（不动 postgres / 迁移）：SA application + domain、PS adapters/settlementaccounting、architecture 全 ok。机制清点在本树重生成、随 ③ 同笔；推送方在 tip 重生成兑底（wbr/11 无迁移，数字预期不变）。未跑全量（09-08 裁定作者只跑受影响范围）、未跑 `-race`（本机走不了）。日志 `%TEMP%\wbr09-dsn-step3.log`，仓内无残留。

**判断题**（给评审与推送方，都不阻断）：

1. **政策引用一列不是两列。** 票面第 3 步写「对象 + 版本两格，与 `CreditBasis` 里的政策版本引用同形」——两句在今天的类型上矛盾：`CreditBasis.Policy()` 是一枚单值 `CreditPolicyReference`，由 SA→PC 适配器用 `versionReference` 拼成「对象/版本」后就是不透明标识（与 `AdoptedPolicyReference` / `ControlPolicyReference` 同形）。拆两格要么让 SA 按 `/` 拆一枚提供方的标识（ADR-0025 只译不判、ADR-0027 / 0062 消费方只回指），要么改 `CreditBasis` 的形（票面边界「不改 ADR-0127」）。取「同形」那句，落一列；理由写在 0017 头注。若评审判两格是硬要求，改法是 PC 侧在 `pcdomain.CreditBasis` 上给两格、SA 适配器照译，那是另一张票的形。
2. **`Expose` 拒没换上授权额度的登记状况，比票面「新写入非空由应用层保证」多守一层。** 理由：守在编排里，少调一步 `WithAuthorizedLimit` 的那条路会形成一份没有出处的暴露、与存量行读起来一样——这正是 parallel-sessions「不同的绿长同一张脸」那一格；守在账本上，那条路在第一个用例就红。代价是四处既有夹具改先换上授权额度（都是测试）。生产路径不受影响：`exposeCredit` 恒过 `WithAuthorizedLimit`，`LoadCreditStanding` 只造登记状况、不直接 `Expose`。若评审判这是替 owner 加了一条领域不变量，回退法是删 `Expose` 里那两行 + `ErrStandingNotAuthorized`，其余不动。
3. **构造门覆盖七件不止 `CreditBasis`。** 票面写「与其它 mandatory 依赖同一处、同一错误形状」，而 SA application 此前没有任何构造器返 error（通道 1 20:08 核实）——「同一处」只能是新开一道门；开了门只拒一件说不通，七件都进。`NewReleasePreAcceptanceControlHandler` 等其它 SA 构造器仍不返 error，本票不扩。
4. **`cmd/parcel-dispatch` 那几行在 cmd 测试零断言**（wbr/03 评审非阻断）：本票让漏接在装配期以 `ErrNilDependency` 拒，但没给 cmd 加一条「装配成功」的显式断言——`cmd/parcel-dispatch` 含 DSN 全绿说明装配函数在既有用例里走到了；要不要单独一条探针归推送方判。

**父 spec**：`wiring-baseline-remainder/spec.md` 状态行不由本票改。

## 进 main 记录（2026-09-09 21:4x，通道 1 推送；重放树由前一任通道 2 在 20:4x 建好但未 ff、未推，21:3x 通道 1 接回推送方后接着走）

分支 `mcp3-wbr09` 五笔在隔离树 `%TEMP%\idp-replay-2055` 重放到 `55528895`（main tip，= `d5a35960` + wbr/11 四笔 + 三笔 tasks.md 簿记）之后零冲突：
`5bead280→8c44f8c6` / `d45a7a34→adac0a46` / `d23956d7→688d0437` / `d3c41b77→3ff50af8` / `512ada47→84c62c3c`——五对 `patch-id --stable` 逐对相等，
`git diff 512ada47 84c62c3c -- <本票十六文件>` 为空；全树差只有 main 上多的 wbr/11 那四文件与 tasks.md。与 wbr/11 / awf/22 文件面零交集，不必让作者重验。
清点在 `84c62c3c` 干净检出上重生成零差（③ 那笔已随分支重生成，main 中间几笔不改文件面），不需清点笔。
**远端 `main = 84c62c3c`**（21:4x `push 84c62c3c:main`；推前 `ls-remote` 核 `55528895` 未动，`55528895..84c62c3c` 只有本票五笔）。
验证（同一棵干净检出钉 `84c62c3c`）：gofmt -l 空、`go build ./...` / `go vet ./...` 退 0、含 DSN `go test -p 1 -count=1 ./...` **102 ok / 0 FAIL / 15 无测试 / 0 cached，105 s**；
探针 `TestCreditExposureLedgerRoundTripsSeparately` `-v` 带 DSN PASS / 不带 SKIP。日志 `%TEMP%\verify-wbr09-84c62c3c.log`。
分支指针改名 `merged/mcp3-wbr09`，远端 `mcp3-wbr09` 删；树 `D:/tops/idp-parcel-mcp3-wbr09` 与 `idp-replay-2055` 先比内容再 `worktree remove`（未加 `--force`）。
**评审状态如实记**：非作者评审单先派通道 5（task-4f833b6f，20:41），21:09 记「截至 21:10 未响应」改派通道 6（task-66a3fe55，21:16）；推送时用户口述「5 / 6 评审已过」，
台账里 66a3fe55 仍 pending、通道 1 队列里没有 wbr/09 的评审原文（通道 5 21:4x 自证从未评过，不是丢在队列里）——推送方据用户口述推，评审按**合入后补评**处理（21:5x 致通道 6）。
**补评 21:56 到：通道 6 钉 main `84c62c3c`，两轴无阻断，main 不动**（全文见下方 Comments；Spec 非阻断 1 / Standards 非阻断 3 随票记）。

## Comments

- 2026-09-09 20:3x · 通道 3：三笔齐，完工报发通道 1；评审留通道 5 或 6（作者是通道 3，含前一任会话）。
- 2026-09-09 21:5x · 通道 1：判断题 ④「要不要给 `cmd/parcel-dispatch` 单独一条装配探针」归推送方判——不加：`cmd/parcel-dispatch` 含 DSN 用例在全量里走到了装配函数（漏接会在 `ErrNilDependency` 处红），单独一条只是把同一件事再说一遍；wbr/03 评审那条非阻断照原样留着。

**评审 ← 通道 6 · 合入后补评 · 钉 main `84c62c3c`（基 `55528895`；代码 patch-id 排 .scratch 后与分支 `d5a35960..512ada47` 相等）· 21:56**（隔离树只读、已收；原文经队列送达通道 1，台账 `task-66a3fe55` done）

【阻断】无。不需要在 main 上往前修任何一项。

【非阻断】
Spec 轴：(1) 票面第 3 步「对象 + 版本两格」与同句「与 `CreditBasis` 里的政策版本引用同形」自相矛盾，代码取一列是对的（判断题 1 接受）。取证：`CreditPolicyReference struct{ requiredValue }` 单值；SA→PC 适配器 `credit_basis.go` 用 `versionReference` 拼「对象/版本」后交来一枚不透明标识，与 `ControlPolicyReference` / `AdoptedPolicyReference` 同一造法；SA 迁移里全部 `*_ref` 列都是单列 text。拆两格等于让 SA 按 `/` 拆提供方的标识。建议 owner 把票面那半句改掉——票面措辞，代码不动。
Standards 轴：(2) 零值判法三处直写：`WithAuthorizedLimit` `policy.String() == ""`、`Expose` `standing.policy.String() == ""`、postgres `Save` `reference != ""`，测试里问「有没有出处」也用同一写法；`requiredValue` 与各 Reference 类型都没有 `IsZero()`。Duplicated Code / Primitive Obsession 判断题：给 `CreditPolicyReference`（或 `requiredValue`）一只 `IsZero()`，三处问的同一个问题就有一个名字。(3) `ApplyPreAcceptanceControlDeps` 头注后半是变更史（「它曾经允许为 nil（三步法的 expand 段…）决定五写明的 contract 段已随那份夹具补上替身一并收」）——AGENTS「注释不写变更说明」；前半（装配时不知道租户登记哪一种、缺件在第一笔命中那条路时才 panic，所以七件全 mandatory）是该留的理由，后半归 git log。(4) `CreditStanding.Policy()` 生产零调用（`Expose` 直接读字段，只有测试用它断言）；架构棘轮只管工厂不报；作为字段的对称读法可留，点一下让 owner 知道。

【无发现】判断题 2（`Expose` 拒未授权状况）：接受，且不算替 owner 加不变量——CONTEXT「每项信用暴露保存实际采用的政策」本就是领域不变量，守在账本上是放回该在的层；置于重放判之前与作用域错配同列，一致；生产路径 `exposeCredit` 恒经 `WithAuthorizedLimit`。判断题 3（七件门）：接受；`NewReleasePreAcceptanceControlHandler` 不返 error 在票面边界外。判断题 4（cmd 零断言）：归推送方；`cmd/parcel-dispatch` 无 DSN 也 ok，装配函数在既有用例里走到。做法第 2 步「cmd 零改动应能编过」实改 12 行：因「同一错误形状」把构造器改成 `(handler, error)`，装配处必须接 error——「零改动」的前提（签名不变）与「同一错误形状」不能同时成立，作者取后者、完成记录 ② 写明 ✓。第 3 步 `operational_position.go`：核 `CreditStandings.LoadCreditStanding` 只经 `LoadForScope` 求和，前件不成立、未动正确 ✓。第 5 步 `AT-SA-171` 落在票面自己的验收对照第 5 条；`.scratch/at-coverage-inventory.md` 是钉 SHA 的盘点、记名不动 ✓。完成判据四句：nil 构造期拒 ✓（`TestAnyNilDependencyIsRefusedAtConstruction` 七件逐一抽掉各得 `ErrNilDependency`）；落库带政策引用可读回 ✓（`TestACreditExposureRoundTripsTheAdoptedCreditPolicy`：往返 + 存量行 NULL 读回为空 + 混册再保存不动政策列）；领域 `TestAnExposureKeepsTheCreditPolicyItWasJudgedAgainst` 覆盖登记状况直接 Expose 得 `ErrStandingNotAuthorized`、受限也带出处、重放不改口 ✓；gofmt / vet 0 ✓。迁移 0017：`ADD COLUMN credit_policy_ref text` + CHECK `IS NULL OR btrim(...) <> ''`，可空只为存量行、不追溯 ✓；头注中文、引 ADR-0127 Consequences 与 CONTEXT 原句无行号 ✓；LF 无 BOM ✓。postgres：SELECT / INSERT 带列，ON CONFLICT SET 仍只 status / released_at / saved_at ✓（存量行释放时 `$13` 写 NULL 且 SET 不碰该列 → 保持 NULL，「混册再保存」那段盖住）；NULL → 零值，非 NULL 过 `NewCreditPolicyReference` 构造门 ✓。重建门 `RehydrateCreditExposureLedger` 对存量 NULL 如实读回零值、不补不拒，与 ADR-0028「只校验不重算」相容 ✓。PS 夹具 diff 里删的只有三处 `Deps` 内联构造（换 `mustApplyHandler` + `authorizedBasis`）与 `termsFixture` 一行注释，新增断言只在两只新 helper 里，既有断言零改 ✓。领域包无新 import（无 HTTP / pgx）✓；注释全中文；跨文件引用用 ADR 编号 + 决定序号 / CONTEXT 引文，无行号；「七件」数的是本结构自己的字段不是别处 ✓。边界：PC 零改动、PS 只动那份夹具、ADR-0127 正文与比例额度基数未动 ✓。机制清点 155→156 / 16→17 与新迁移一致 ✓。
本评审验证（`84c62c3c` 隔离树）：gofmt 空；`go build ./...` 0；`go vet` SA / PS 适配器 / cmd/parcel-dispatch 0；无 DSN `go test -count=1 ./internal/settlementaccounting/... ./internal/parcelshipment/adapters/settlementaccounting/... ./internal/architecture/... ./cmd/parcel-dispatch/...` 9 ok / 1 无测试；DSN 半边沿用推送方在 `84c62c3c` 的 102 ok，未再占 55432。

结论：评审过、无阻断，main 不动。两轴各 0 阻断；Spec 非阻断 1（票面措辞）、Standards 非阻断 3（判断题）。

- 2026-09-09 22:0x · 通道 1（推送方处置）：(1) 票面第 3 步那半句**不改**——票已 resolved，完成记录判断题 ① 与本评审都记了取一列的理由，改历史文本会让那道判断题失去对照物；后继票若再写政策引用，照「单值不透明标识」写。(2) `IsZero()` 与 (3) 头注后半删变更史、(4) `Policy()` 读法——三条都落在 SA 地盘，随 [10](./10-credit-ratio-base-is-declared-on-the-credit-policy-content.md)（同动 SA）顺手收，不另立票；wbr/10 派单时点名。
