# 27 CC 准入代码把监管限制叫作内部合规限制；内部合规限制没有模型

Category: enhancement
Status: resolved——2026-10-28 进 main（`282af38da`，与 code/37 同批；全量 `scripts/test-full.sh` 160 ok / 0 FAIL，种子灌一次性库退 0），见文末「进 main 记录」。此前 in-progress——2026-10-27 作者完工，待隔离评审与进 main，分支 claude-code27（代码最后一笔 d32a3bf3a）。此前：in-progress——2026-10-27 会话 tops-20（idp-parcel-91）接手 tops-26 断在中途的「上线前必须」一组（冻结清单「六」），分支 claude-code27（续 tops-26 已有分支与 worktree 半成品）。此前 ready-for-agent——R3（不急）：限制册在生产上没有写入（`ManageRestrictionHandler` 未装配，ADR-0206 越权风险点 1），内部合规限制没有任何入口，今天没有生产路径；**UC-CC-011 接线之前必须先做第 2 项**，否则内部合规限制要么被塞进 `RegulatoryRestriction`（让监管限制可以不凭监管结果解除），要么挡不住动作。R3 只说缺陷本身今天不出事；内部合规限制的模型是首发范围要求的实现缺口（PILOT-SCOPE 要求验证内部合规限制、关务专项基线切片 1 含它），不因 R3 而移出首发。2026-10-21 文档自洽 03 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc03。
Blocked by: 无（裁向已由 [ADR-0241](../../../docs/adr/0241-action-admission-restrictions-are-regulatory-or-internal-and-only-regulatory-restrictions-exist-today.md) 定）
出处：[audit/cc.md](../audit/cc.md) H3（B，审计钉 `6b0074d0`；2026-10-21 按 main `205eb254` 复核，现象仍在）；[02 的完成记录](../issues/02-code-defects-from-the-audit.md)「其余条件性 B 类」一句把它交给修订票 03 裁后按裁向立票。

## 现象

CC CONTEXT 与 ADR-0206 把动作准入的限制一源写作「内部合规限制」，代码里参与准入的 `domain.RegulatoryRestriction` 却是监管限制（挂监管决定、只凭责任来源接受的监管结果整条解除）。CONTEXT 里的内部合规限制（覆盖项、限制责任来源、解除条件、部分解除、四格适用性）在代码里没有模型。ADR-0241 裁定：两种限制各自成立、互不改名互不解除，`RegulatoryRestriction` 就是监管限制，内部合规限制另立对象并进准入。

## 证据

- 代码：`internal/customscompliance/domain/action_admission.go` 包注释「三源——内部合规限制、放行门禁核对、外部放行结果」、`AdmissionAnswer` 头注「外部放行不解内部限制」、`JudgeActionAdmission` 头注「外部放行不解内部限制」、`objectScope` 头注「内部限制可以直接落在一个包裹上」，测试注释 `domain/action_admission_test.go` 与 `adapters/postgres/action_admission_test.go` 里的「外部放行不解内部限制」同此；而 `ActionAdmissionInput.Restrictions` 的类型是 `[]RegulatoryRestriction`，`ports.RestrictionScopeView` 交回的也是它。
- 决定：ADR-0241 决定二、三、四；CC CONTEXT「监管限制」「内部合规限制」「动作准入判断」词条（文档自洽 03 改写后）。

## 三类

机制：两种限制的对象与册、准入的合成与内部合规限制待确认 / 冲突答未决。产品策略：无新的。租户取值：限制来源、动作矩阵与解除条件（`PAR-CUS-07`），没登的答未配置。

## 规模

1. 注释改名：S。
2. 内部合规限制模型、册与并进准入：L，随 UC-CC-011 实施（覆盖项逐项、限制责任来源解除决定、部分解除、四格适用性判断、迁移另取号），准入编排读两本册。

## 最小的诚实修法

1. **现在可做**：上列注释里的「内部合规限制」「内部限制」改称监管限制；「外部放行不解内部限制」改为「外部放行不自动解除限制——监管限制要一份引用已接受监管结果的解除」。不改类型名与行为。
2. **UC-CC-011 接线前必做**：另立内部合规限制的领域对象与登记册，不复用 `RegulatoryRestriction` 与限制册的表；准入的限制一源读两本册——任一种动作时点有效且约束本动作即阻断，内部合规限制适用性答待确认或冲突时该动作答未决并指名那条限制，技术未形成照读不动办（ADR-0241 决定三）；补 UC-CC-013 的 AT 一行（内部限制待确认答未决），AT 号接 UC-CC 现有最大号。

## 要裁的

- 监管限制的部分解除（ADR-0241 越权风险点 1）：等 `PAR-CUS-01/02` 给出扣留与解除的范围语义，不在本票。
- 谁来形成监管限制（UC-CC-008 驱动还是 UC-CC-006 接收扣留时派生，ADR-0241 决定五）：归限制写入口接线那张票。

## 完成记录（2026-10-27，接手作者）

- 起点：main `2afdd0b1e`（分支 rebase 其上，文档提交 `b4fad7323` 为前作者的 ADR-0334、迁移 0028、UC-CC-011/013、AT-CC-449 至 452；取号核过 origin/main 与各 claude-* 分支未被占）。提交：`f980fad3d`（WIP 接手，前作者未提交的领域对象与准入测试）、`d32a3bf3a`（其余全部代码）。
- 做了什么：领域准入输入多 `InternalRestrictions`，依据多 `INTERNAL_RESTRICTION_IN_FORCE`（阻断）、`_CONFLICTING`、`_UNCONFIRMED`（未决，各指名那条限制），监管限制沿用 `RESTRICTION_IN_FORCE`；端口 `InternalRestrictionScopeView`/`InternalRestrictionStore`；postgres 适配器 `InternalRestrictions`（四张表，限制头行 `FOR UPDATE`，决定按 sequence 经领域 `Decide` 重演读回）；`ManageInternalRestrictionHandler`（建立、决定，重放先于核对，非责任来源与决定不成立分格，不发布意图）；准入编排多一口，读不动答 `INTERNAL_RESTRICTION_VIEW_UNAVAILABLE`；`parcel-api`、`parcel-dispatch` 装配接上读口（漏接构造即炸）。票面第 1 项注释改称同批做掉；盘点里 AT-CC-345 至 348、370 改指内部合规限制的测试，原监管限制编排测试注释不再冒称这些 AT。
- 代决与越权风险点：全部随 ADR-0334 的越权风险点 1 至 9，本批没有新增；写入编排在生产没有端点（ADR-0334 决定七、风险点 1），故装配只接读口，`ManageInternalRestrictionHandler` 只在测试里被构造。
- 判据对用例：AT-CC-449、450、451（域与真库）、452（域、应用、真库）、AT-CC-424 的内部合规限制读不动口（应用测试）。
- 自验（含 PG，DSN 55432）：`go build ./...`、`go vet`、`gofmt -l` 无输出；`go test -count=1 -p 1` 通过 `./internal/customscompliance/...`、`./cmd/parcel-api`、`./cmd/parcel-dispatch`、`./cmd/parcel-customs-register`、`./internal/architecture/...`、`./migrations/...`、`./internal/transportfulfillment/adapters/customscompliance`。`at-coverage-counts -check` 只报生成物过期。
- 过程里被门禁逼出的两处：`pbc08` 写口无事务负向证据（补 `TestInternalRestrictionWritesRefuseToRunOutsideATransaction`）；类型可达性棘轮（`InternalApplicability`、`RestrictionApplicability` 入基线，理由是等 UC-CC-003/009/010 接线）。
- 留给协调方：进 main 时重生成 ADR 索引与 AT 盘点计数；ADR-0241 的 Status 与 Links 前向指针已在文档提交里，进 main 前评审核一眼；未做（均不挡上线、已在 ADR-0334 风险点里）：生产写入入口、决定方授权核验、下游（就绪、门禁核对、关闭）读内部合规限制。

## 评审记录（2026-10-28，隔离子代理，钉 `03daa7979`）

两轴无阻断。Spec 轴非阻断四条：ADR 索引与 AT 计数待推送方重生成（已做）；ADR-0334 决定五把「无内部限制」改称「册上无记录」，对已接受记录的作用里没列这条措辞差（记入冻结清单）；测试 `Covers:` 点名的 AT-CC-339、340、342、352–357 盘点是否另有登记未逐条核（记入冻结清单）；类型可达性基线新增两行理由站得住。Standards 轴非阻断：`judge_action_admission.go` 头注一句未折行，不改。评审带 PG 跑过 customscompliance、architecture、cmd 与 TF 的 CC 适配器测试。

## 进 main 记录（2026-10-28，推送方 tops-20 / idp-parcel-91）

- 分支 `claude-code27` tip `03daa7979` 作出处；在隔离树上与 code/37 一起重放到 main `f1cccc964`（psb/22 第 1 笔之后），无冲突。
- 重放后的 tip 上重生成 ADR 索引、AT 盘点总览计数与机制清点，各成一笔；`adr-index -check`、`at-coverage-counts -check` 退 0。
- 全量 `scripts/test-full.sh`（专用一次性 PG，`-count=1`）：160 ok / 0 FAIL。种子 `scripts/demo-seeds/seed.sh` 灌一次性库 `seed2737` 退 0，用后删库。
- 推确切 SHA `282af38da` 到 main。

