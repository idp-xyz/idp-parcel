# SA 收 ADR-0127 的 contract 段：`Deps.CreditBasis` mandatory、暴露账本行持久化政策引用；PS 夹具先补 `CreditBasisView` 替身

Category: enhancement
Status: resolved——2026-09-09 20:3x 通道 3（分支 `mcp3-wbr09` 基 `d5a35960`，三笔 `5bead280` / `d45a7a34` / `d23956d7` 已推 origin；完成记录见文末，
进 main 记录归推送方）。此前：19:4x 通道 3 接续（接续单 task-de74738a-f485-422e-8e85-7a78d474d1ad；前一任 18:4x 自领后 18:5x 中断，树上留一份未提交的 PS 夹具
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

## Comments

- 2026-09-09 20:3x · 通道 3：三笔齐，完工报发通道 1；评审留通道 5 或 6（作者是通道 3，含前一任会话）。
