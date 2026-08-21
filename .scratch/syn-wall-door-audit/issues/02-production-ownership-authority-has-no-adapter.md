# 生产归属权威端口无生产适配器,治理登记册配好也接不进提交链

Category: enhancement
Status: ready-for-agent

来源:SYN-WALL-DOOR-AUDIT 走通审计(基线 `49a2ab0`),对应清单 W03。

## 墙

`OWNERSHIP_UNRESOLVED` / `ADMISSION_PAUSED`(`parcelshipment/application/submit_shipment_request.go` 的 `SubmitOutcome`)、`AUTHORITY_UNRESOLVED`(`parcelshipment/domain/production_ownership.go`)。UC-PS-001 第二步「生产归属」在 `DecideProductionOwnership` 处停摆。

## 现状:半座桥

- PS 侧:`ports.ProductionOwnershipAuthority` 全库只有接口定义,无任何生产适配器(仅测试替身)。
- 治理侧:pilot-governance 的 `authority_interval` 表、INSERT 写入方(`governance_records.go`)与登记用例 `record_stage_review`、`govern_incident` 都在——PAR-GOV-03 的登记机制半边基本齐。
- 缺口:①两者之间没有桥接适配器(按权威区间+准入控制回答一份 `AdmissionScope` 归谁);②治理登记用例未接任何进程入口(cmd 无治理端点/CLI)。

## 缺的最小机制件

1. PS→pilot-governance 桥接适配器:实现 `ProductionOwnershipAuthority`,读权威区间与暂停/恢复记录,译成 `ProductionOwnershipDecision`(含 `ADMISSION_PAUSED` 一格,注意 `blockedOutcome` 三分)。
2. 治理登记的进程级入口(端点或受控 CLI),让 PAR-GOV-03..07 的实例登记有路可走。

## 红线

- 无权威区间登记时如实答 `AUTHORITY_UNRESOLVED`,不得默认「本产品承接」。
- 治理记录不可覆盖(既有票已钉);本票不改治理语义,只接线。

## 参照

`docs/product/PILOT-PARAMETER-REGISTER.md` PAR-GOV-03..07;UC-PS-001;ADR-0017。

## Comments

- 2026-08-20 MCP-2：对 `3324ecb` 重核四件，**结论不变：半座桥原样**。PS 侧——
  `ProductionOwnershipAuthority` 仍只有接口定义（`internal/parcelshipment/ports/ports.go`），
  实现仍全是测试替身（`submit_shipment_request_test.go`、`adapters/http/submit_shipment_request_test.go`、
  `cmd/parcel-dispatch/synthetic_v0_test.go` 三处 `ownershipAuthorityDouble`/`synSProductionOwnership`）；
  `internal/parcelshipment` 下无任何 `pilotgovernance` 引用——桥接适配器仍缺。治理侧——
  仓储在（`migrations/pilot_governance/0001_governance_records.sql` 含 `authority_interval`），
  写入方在（`governance_records.go`、`incident_records.go`），用例在（`record_stage_review.go`、
  `govern_incident.go`）。登记口——`cmd` 全树无 `pilotgovernance` 引用，治理用例仍未接任何
  进程入口。基线以来 pilot-governance 仅两笔（`81b50f2`/`3b37b5a`）改 handoff 信封分区与
  ID（OUTBOX-PK-STEP2），不动本票四件。票面与代码无矛盾。

- 2026-08-21 MCP-4：对 `0ec62ea` 连续性重核四件，**结论不变：半座桥原样**，票面与代码无矛盾，按原票开工。
  ①PS 侧——`ProductionOwnershipAuthority` 仍只有接口定义（`internal/parcelshipment/ports/ports.go`），
  生产实现仍为零，三处测试替身（`internal/parcelshipment/application/submit_shipment_request_test.go`、
  `internal/parcelshipment/adapters/http/submit_shipment_request_test.go`、
  `cmd/parcel-dispatch/synthetic_v0_test.go`）原样。②治理侧——`authority_interval` 在
  `migrations/pilot_governance/0001_governance_records.sql`；**暂停、恢复、接管三表在
  `migrations/pilot_governance/0003_suspension_resumption_takeover.sql`**，票面只点了 0001，
  实际比票面更全，确认不建新表。写入方与两份登记用例原样在。③登记口——`cmd` 全树仍无
  `pilotgovernance` 引用。④基线连续性——`git log 3324ecb..0ec62ea -- internal/pilotgovernance
  migrations/pilot_governance` 为空，上次重核以来治理侧零改动。

- 2026-08-21 MCP-4：碰撞判定（开工前报 MCP-1）——本票**不改** `internal/parcelshipment/ports/ports.go`。
  `ProductionOwnershipAuthority` 签名已完整，适配器直接实现即可；PS 读 PG 所需的三个依赖口按 ADR-0025
  「适配器为此需要的实例半边协作者，其接口定义在适配器包内，不进消费方 `ports`」写在适配器包内。
  与 MCP-5 票 10（`internal/parcelshipment/adapters/nodeoperations/`）文件级无交集。

- 2026-08-21 MCP-4：**本票只交「缺的最小机制件」第 1 条（桥接适配器），第 2 条（治理登记的进程级入口）不做。**
  理由是地盘：派单把地盘限在 `internal/parcelshipment/adapters/pilotgovernance/**`，并明令不碰
  `cmd/parcel-dispatch/assemble.go` 与 `cmd/parcel-api/endpoints.go`，而第 2 条按定义要落 `cmd`。
  第 2 条因此仍缺，另立票或由派单方指派——本票收口后 `cmd` 全树仍无 `pilotgovernance` 引用这一条不变。

- 2026-08-21 MCP-5（接手 MCP-4 死会话封存件 `92c8e92`）：**重核结论——封存件可用，留下推进。**
  它不是死在半途的现场：`cherry-pick` 到 `82aa1d3` 零冲突，gofmt / `go build ./...` / `go vet`
  全零信号，17 个顶层用例 41 条 PASS、0 红 0 跳过，测试文件完整收尾于 `hasReason`，无断句。
  实现把取舍写进了注释（三个读口为何不合并、为何按 ADR-0025 不进 `ports`、依赖故障为何
  不能折成未决、`revisionFor` 依赖治理侧哪条不变式），可读可验，重写会把这些理由一并丢掉。

- 2026-08-21 MCP-5（**本轮最要紧的一条：全绿掩着一个功能性缺口**）：封存件的 41 条用例
  全部是替身、包耗时 0.012 秒，**证的只是翻译层对**。实际上三个窄口里只有两个今天插得进
  真实仓储：`AuthorityIntervalSource.ListCurrent` ↔ `AuthorityIntervals.ListCurrent`、
  `OwnershipHandoffSource.FindByInterval` ↔ `Takeovers.FindByInterval` 逐字对得上；
  **暂停读口在治理侧根本没有实现**（`Suspensions` 只有 `FindByID`/`Save`，`Resumptions`
  只有 `FindBySuspension`/`Save`，没有任何查询回答「某范围在某时点是否仍被拦」）。
  而 `governanceScope()` 把 `Suspensions == nil` 归入「显式未配置」，于是**整座桥恒答
  `AUTHORITY_UNRESOLVED`，生产上永远走不到第二步**——41 条绿一条都照不到这里。
  记下来当样本：绿不等于对，尤其当绿全部来自替身时。

- 2026-08-21 MCP-5（本票交付三件）：
  1. **编译期断言**（`production_ownership_test.go`）：三个窄口逐个钉到真实仓储。封存件
     一条这类断言都没有，窄口形状与真仓储对不对得上此前无人守；不合当场编不过，而不是
     等装配时表现为一个看不出原因的`权威未确定`。
  2. **治理侧读口** `Suspensions.FindUnresumedSuspension(ctx, scope, at)`
     （`internal/pilotgovernance/adapters/postgres/incident_records.go`，经 MCP-1 扩地盘）。
     方法名与语义留在治理侧、不上提到 PS 的 `ports`；消费方窄口 `AdmissionSuspensionSource`
     改为跟它同名（Go 结构化类型要求名字一致，且按 ADR-0025 由消费方去适配提供方）。
  3. **真库用例三条**（`incident_records_test.go`），逐条对着下面三处边界。
     行为绿自此有登记册背书，不再只有替身。

- 2026-08-21 MCP-5（三处边界的出处，逐条——都不是自选的）：
  - **范围版本按字面相等匹配**。依据 `docs/product/PILOT-SCOPE.md`「后续证据支持扩大或缩小
    暂停范围时，应形成带新依据和生效时间的范围版本，不覆盖此前判断和实际阻断历史」——
    每条暂停只为它写明的那一版说话。另一半是类型上的：`ScopeVersionReference` 是不透明串，
    版本之间的先后与继承读不出来，**跨版本匹配根本无从写起**。
  - **生效时刻起算**（`effective_at <= at`），恢复同此。与本仓权威区间 `[From, To)` 的半开
    约定同向；暂停与恢复同刻时以恢复为准。这一条 PILOT-SCOPE 未明文，取的是同一文件里
    `matchInterval` 已在用的约定，照实记为「按邻近约定取齐」而非文档明文。
  - **同一范围多条暂停各自独立解除**。依据是恢复记录逐条引用一个暂停标识，加上
    PILOT-SCOPE「恢复准入必须……由试点业务责任角色依据证据明确决定；指标恢复或规则不再
    命中均不得自动恢复」——只要还有一条已生效且未恢复，范围就仍在暂停中。交回哪一条按
    （生效时间，标识）定序，保证同一登记册每次问都得到同一条。

- 2026-08-21 MCP-5（**残余风险，已报 MCP-1，需领域裁定**）：范围版本字面相等匹配意味着
  **v1 上一条尚未恢复的暂停，在被问 v2 时是看不见的**——答准入开放。这正是暂停要拦的那件事
  反过来发生。今天无法在代码层收紧：版本谱系不可读，「v1 的暂停是否覆盖 v2」是领域问题
  不是查询问题。**不擅自选一个看上去安全的写法**（比如按前缀匹配），留待裁定。

- 2026-08-21 MCP-5（**`AdmissionScope` 无人装配——照实记缺，不给默认值**）：
  `domain.AdmissionScope` 今天只是 reference 与 digest 两个非空串的容器，由命令入参给入，
  **全仓无任何东西按 PAR-GOV-03..07 装配它**，而 `DecideProductionOwnership` 收的就是它。
  不并入本票：本票交的是「登记册怎么答」，范围怎么形成是另一件事，且它牵到 PS 接受链的
  入参来路。**不因为参数拿不到就给默认值**——本适配器对拿不到坐标的范围一律答
  `AUTHORITY_UNRESOLVED`（`GovernanceScopeDirectory` 交回 false 即停，不代拟坐标）。

- 2026-08-21 MCP-1 裁定 / MCP-5 执行（**上面「范围版本按字面相等匹配」那条出处引错了，
  更正如下；原文保留不改写**）：错在引了 `docs/product/PILOT-SCOPE.md` 硬风险暂停那段的
  **第三句**「后续证据支持扩大或缩小暂停范围时，应形成带新依据和生效时间的范围版本，不覆盖
  此前判断和实际阻断历史」，并把它读成「每条暂停只为它写明的那一版说话」。**「不覆盖此前
  判断」说的是此前判断仍然立着，不是它从此不适用**——按字面相等的写法，v1 那条判断在被问 v2
  时既没被恢复决定解除、也不再拦任何东西，那恰恰就是被覆盖，只不过是静默覆盖。这句话是反对
  字面相等的。真正管这件事的是同段**中间**那句：「如果共享依赖、共同原因或证据不足导致影响
  范围无法可靠隔离，必须保守暂停整个试点的新准入，不能仅拒绝当前报错的单个委托后继续放量。」
  「v1 的暂停覆不覆盖 v2」在不透明串上读不出来，正是「证据不足导致影响范围无法可靠隔离」。
  第二道依据是同文件那句「恢复准入必须在风险原因已经解除、必要一致性核对已经完成后，由试点
  业务责任角色依据证据明确决定；指标恢复或规则不再命中均不得自动恢复」：范围版本从 v1 升到
  v2 是一次**限量范围扩大的 `Go/No-Go`，不是恢复决定**（`RecordResumption` 要的解除证据、
  一致性核对、在途盘点一件都没有），字面相等让一次范围扩大顺带解除一条暂停，正是明文禁止的
  「规则不再命中即自动恢复」。**结论：这是缺陷不是待定参数**，因此上面那条「残余风险，需领域
  裁定」已裁完销账，不再挂待裁。

- 2026-08-21 MCP-5（按裁定改成三态；**拒绝前缀匹配这一点裁定确认是对的，未改**）：
  从不透明串里解析谱系是从脱敏引用组合发明实例事实，撞红线；按时间「最新版本胜出」同理。
  前缀、子串、版本号解析、时间序推断一律禁止。正确语义是保守回答而不是猜谱系，落为
  `pgdomain.AdmissionSuspensionGround` 三格，`FindUnresumedSuspension` 的第二个返回值由
  `bool` 换成它：①无已生效未恢复暂停 → `NOT_SUSPENDED`；②有且范围版本字面相等 →
  `SUSPENDED_BY_NAMED_SCOPE`；③有但覆盖关系不可判 → `SUSPENDED_BY_UNREADABLE_SCOPE_RELATION`。
  ②③都拦，但**在治理侧分得开**（运维动作不同：②去走恢复决定，③去把范围版本关系登进登记册），
  这一条按 ADR-0017「先分辨阻断理由的性质再决定它约束什么」办。命中优先于保守交回，暂停引用
  在③指向读不出关系的那条暂停本身。判断留在治理侧：PS 侧 `admissionControl` 只问
  `ground.Blocks()`，不分辨是哪一格，两格给出的都是同一个`暂停`——本包仍然只翻译。
  `Blocks()` 的失效方向朝拦：只有 `NOT_SUSPENDED` 放行，零值跟着拦，漏填一处不能变成默认放行。

- 2026-08-21 MCP-5（**实例半边照旧留空，不回填**）：「v2 是否承继 v1 的暂停」是登记册事实
  不是查询技巧，没有租户就没有这份登记，因此**现在恒走第三态、恒答暂停**——这是诚实阻断不是
  缺陷。它不会把系统钉死：登记册里一条已生效未恢复的暂停都没有时第一态成立，照常开放；只有
  确实存在拦着的暂停时才保守。「范围版本之间的覆盖关系」需要登记册新增一项，按 AGENTS.md
  「改试点实例状态 → 参数登记册」另立票（`.scratch/syn-wall-door-audit/issues/11-...`，
  `needs-triage`），不并进本票。领域文档不动——规则已在 PILOT-SCOPE，不新开 ADR。

- 2026-08-21 MCP-5（**假绿反证：定性只能看 `-v` 下的 `--- PASS`/`--- SKIP`，秒表只配起疑**）：
  「报测试状态必须写明含不含 PG」此前只是一句要求，没人做过反证，这里补上两组实测数字。
  同一批用例、同一命令，只差 `IDP_PARCEL_POSTGRES_DSN`（`pgtest.DSNVariable`，值
  `postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`）设没设：
  - **未设**：`internal/pilotgovernance/adapters/postgres` 27 条全部 `--- SKIP`，包级仍是
    `ok ... 0.015s`。
  - **设上**：同包 `ok ... 4.477s`，三包合计 83 `--- PASS` / 0 `--- SKIP` / 0 `--- FAIL`；
    本轮两条新 PG 用例各实耗 0.19s、0.20s。
  两次的包级行都是 `ok`，**全跳过与全通过在默认输出里长得一模一样**，唯一差别是秒数，而秒数
  只够用来起疑不够定性。报绿前先做一次反证——拿掉 DSN 重跑同批用例，必须转 `SKIP`，转了才能
  证明刚才那份绿真的走了库。

- 2026-08-21 MCP-5（本轮交付与验证）：改动六个文件——
  `internal/pilotgovernance/domain/suspension_takeover.go`（新增 `AdmissionSuspensionGround`
  三格 + `Blocks()` + `String()`）、同包 `suspension_takeover_test.go`（钉「只有确实没有未恢复
  暂停那一格放行」与「命中/保守两格名字不得相同」）、
  `internal/pilotgovernance/adapters/postgres/incident_records.go`（三态 SQL：去掉
  `WHERE suspension.scope = $1`，改为 `ORDER BY (suspension.scope = $1) DESC` 让命中优先，
  并把该布尔作为 `names_asked_scope` 取回定格）、同包 `incident_records_test.go`（原三条改判
  `ground`；新增 `TestAnUnreadableScopeRelationSuspendsConservativelyInsteadOfOpening` 证
  v1 未恢复时问 v2 答暂停而非开放，与 `TestANamedScopeSuspensionOutranksAConservativeOne` 证
  命中优先且解除本版那条后退回保守而不是开放）、
  `internal/parcelshipment/adapters/pilotgovernance/production_ownership.go` 与其测试
  （窄口第二返回值跟治理侧换型；新增三条子用例钉「凡拦即`暂停`」含零值）。
  验证：`gofmt -l` 无输出；`go build ./...`、`go vet ./...` 零信号；三包真库 83 PASS / 0 SKIP
  / 0 FAIL（数字与反证见上一条）。**未跑全仓套件**——MCP-6 正改 `pgtest` 需要无人跑门禁的窗口，
  集成全量按派单由 MCP-1 在 detached verify 树上跑。

- 2026-08-21 MCP-5（第二件仍不做）：沿用 MCP-4 的理由，不推翻。另加一条今天才成立的：
  票面第二件里**端点那条路已被堵死**——八个业务端点今天全装 `UnconfiguredIntake{}`，
  无接入渠道即无可认证入口，而接入渠道登记册（票 01）已撞上 ADR-0055 明文否决的替代方案
  转 `ready-for-human` 等新 ADR。第二件因此只剩受控 CLI 或另拆票两条路，不得自造采信
  自报身份的口子（ADR-0003）。本票收口后 `cmd` 全树仍无 `pilotgovernance` 引用。
