# 19 准入范围桥：操作者与集成客户端铸造器的 `AdmissionScope` 由「租户对照 + `AuthorityCoverage`」实现，接进生产装配，演示租户登 `SYN-` 对照

Category: enhancement
Status: in-progress——2026-10-13 22:56 通道 3 认领实施（用户「开工」；照用户此前裁定免非作者评审、作者自重放），分支 `mcp3-oc19`，基 `00c899fe`。此前 ready-for-agent——2026-10-13 22:5x [ADR-0180](../../../docs/adr/0180-admission-scope-tenant-mapping-is-an-access-identity-register-and-self-authority-is-a-deployment-parameter.md) 已接受（用户授权通道 3 自决，接受记录在该 ADR 文末），头部地盘照「分诊」收窄；照「分诊」节「做什么四件」拆笔。此前 needs-info——2026-10-13 22:4x 分诊与 ADR-0180 草案进 main（重放 tip `7e7b6863`，用户裁定免评审、通道 3 直接重放，记录在「接手核对」末）；本票仍等 ADR-0180 接受。此前 needs-info——2026-10-13 22:4x 通道 3 接手（通道 2 会话中断，用户转派）：逐条复核 ADR-0180 与「分诊」节的引证，改正处见「分诊」节末「接手核对」；推荐与归类不变，只有词表一格的落法随取证改。此前 needs-info——2026-10-13 22:2x 通道 2 分诊完（派单 `task-f2fa6dc4` ← 通道 1，分支 `mcp2-oc19t`，基 `b3c8bd46`）：三问与词表一格都有推荐；问题一、三一定就要落迁移、定参数来源，起草成 [ADR-0180](../../../docs/adr/0180-admission-scope-tenant-mapping-is-an-access-identity-register-and-self-authority-is-a-deployment-parameter.md)（Proposed），待用户接受。归类、推荐、做什么各件落点与收窄后的地盘见文末「分诊」。此前 in-progress——2026-10-13 22:07 通道 2 分诊认领（派单 `task-f2fa6dc4` ← 通道 1），分支 `mcp2-oc19t`，基 `b3c8bd46`（开工前 ls-remote 核过远端 main 未变）。此前 needs-triage——2026-10-13 通道 4 立（通道 1 派单 `task-40698906`，照裁定 A：[15](./15-operation-decision-faces-take-operator-intake.md) 余下四口不换，桥另立）。桥的骨架有出处：ADR-0149 决定四第二条定了按（租户、对象范围、能力、事实类型）读生产权威区间，[14](./14-admission-scope-read-and-grade.md) 交了读口 `AuthorityCoverage` 与「不在准入范围」一格。没有出处的是对照本身——行住在哪、按什么键、本产品的权威串从哪来；第一件定了就要落迁移，见「要裁的问题」。
Blocked by: 无——ADR-0180 已接受（2026-10-13 通道 3 改）。此前：ADR-0180（Proposed，待用户接受，2026-10-13 通道 2 改）；再此前：无——要裁的是本票自己的形状，裁完转 ready-for-agent
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施
地盘（ADR-0180 接受后照「分诊」收窄，2026-10-13 通道 3 换）：`internal/accessidentity` 的对照类型与端口、`adapters/postgres` 的对照适配器、新包 `adapters/pilotgovernance`；`migrations/access_identity/` 下一号；`cmd/parcel-access-register` 两个子命令；`cmd/parcel-api` 的 `buildOperatorMinter`、`buildIntegrationClientMinter`、`buildProductionOwnership`、`buildIsolatedWriteAdmission`、权威串解析与 `main.go` 相应几行；`scripts/demo-seeds/data/access/`、`data/governance/`、`seed.sh` 两段与 `README.md`；参数登记册新的一行与 `PAR-GOV-03`。能力词一处定义若随本票做，另加 PS、TF、CC、SA 认证方各一行。此前（裁前）：`AdmissionScope` 的生产实现与对照的读口、两只铸造器与 `main.go` 那几行、演示种子治理区间与对照种子、参数登记册一行。
出处：[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定四第二条、越权风险点 3；[ADR-0151](../../../docs/adr/0151-unassigned-command-faces-get-their-families.md) 决定三；[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定一；[ADR-0083](../../../docs/adr/0083-pilot-governance-read-face-carries-registry-dimensions-only.md) 决定一；14 判断项 1、2、4；`cmd/parcel-api/assemble_operator_decisions.go` 的 `buildOperatorMinter` 头注。

## 为什么

14 判断项 4 把桥定为「随第一个消费者一起立」。第一个消费者是 15 之四：`buildOperatorMinter` 取了 `UnconfiguredAdmissionScope{}`，头注写「对照登进来的那一笔，把这里换成『对照 + pilot-governance 的 AuthorityCoverage』桥」；[11](./11-integration-client-register-and-client-credentials.md) 的 `buildIntegrationClientMinter` 同样。桥是机制；各租户的对照、区间与权威串是租户取值，对照没登的租户答「不在准入范围」（ADR-0149 越权风险点 3）。

演示租户也只有这一条路：ADR-0150 决定一不许按 `SYN-` 前缀分支，演示动线要越过准入范围，只能给 `SYN-TENANT-01` 登一份普通的对照与区间。取证钉 `1e02d2ea`：全仓非测试代码里 `AdmissionScope` 的实现只有 `UnconfiguredAdmissionScope`；`NewAuthorityCoverage` 没有生产调用点；演示种子里没有能力为 `OPERATION_DECISION` 或 `EXTERNAL_FACT` 的区间。

## 挡在本票上的

- 15 余下四口——关段、建派送任务、有效时间判断、承运商首次有效收寄判断：换口同笔撤隔离放行（ADR-0150 决定三）之后，演示租户在这几口上答 `403 OUTSIDE_ADMISSION_SCOPE`，管理台有效时间判断页（`effective-time-judgment-api.ts` 的 `judgeEffectiveTime`）随之退成 403。关段、建派送任务与收寄判断另挡在 [20](./20-tf-closure-dispatch-task-and-pickup-judgment-canonicalization.md)。
- 15 之四已换的复核完成、主动拒绝、授权处置、装载分配、终止参与：设了发行方的生产装配里，授予齐备也停在 `403 OUTSIDE_ADMISSION_SCOPE`（`TestSwappedDecisionFacesAnswerFromTheOperatorChannel` 的 "granted, admission not registered" 一格钉着）。越过它，装载分配与终止参与答业务结果，委托侧三口到达编排、再停在各自的授权缺口（15 完成判据第三条）。
- [10](./10-device-register-and-operation-fact-capability-face.md) 余下那半（作业事实各口换口同笔撤隔离放行）：`WorkFactMinter` 同样收 `AdmissionScope`，请求带准入要求即判；`cmd/parcel-api` 今天还没有它的装配点，接上时取 `UnconfiguredAdmissionScope` 就与 15 余下四口同一处境。
- 11 已换的集成客户端各口（关务外部结果、监管凭证登记、外部资金事实登记与更正）：同一格，`TestIntegrationClientFacesAnswerTheChannelGrades` 的 "granted, admission not registered" 钉着。

登记册配置写面与目录查阅面不受影响：它们的请求不带准入要求，`MintOperator` 只在 `Admission` 非空时判。

## 要裁的问题

1. **对照的行住在哪、归哪个上下文。** ADR-0083 决定一否决给治理登记册加租户维（治理是产品实例级机制）；14 判断项 1 只说对照是「租户随试点登记的实例半边，由 `AdmissionScope` 的实现去换」，没说行在哪。parcel-shipment 生产归属的 `GovernanceScopeDirectory` 同样只有隔离实现，生产形态留空（`buildProductionOwnership` 头注）。候选：
   - 甲 · access-identity 另立一本对照册，与操作者册、设备册、集成客户端册同处：`access_identity` 下一号迁移，`parcel-access-register` 加子命令，行带生效区间与依据。
   - 乙 · pilot-governance 另立一张带租户的对照表：与 ADR-0083 决定一相抵，要另出记录部分停用它。
   - 丙 · 部署参数，同发行方那几件：不落库、不过受控登记。

   甲、乙落迁移，只进不退；丙每改一次要重启，也没有登记轨迹。
2. **对照按什么键。** `Admits` 只收（租户、能力与事实类型、时刻），看不到这一笔改的是哪个对象；演示种子里路由、计价、委托受理各是一个试点范围。拟按（租户、能力、事实类型）→ 一个对象范围。同一租户同一能力分属几个试点范围（比如按线路分批上线）就表达不了，要让 Intake 把对象交给 `Admits`——那是改 14 的接口，另立。
3. **本产品的权威串从哪来，PS 那一处跟不跟。** 14 判断项 2：`AuthorityCoverage` 的 selfAuthority 与 parcel-shipment 生产归属的 `SelfAuthority` 应取同一值。PS 那一格今天生产形态留空，隔离形态取 `isolatedSelfAuthority`（`SYN-AUTH/idp-parcel-pilot`）。拟取部署参数一件、两处同读；PS 那一处若随本票同笔接上，地盘就越进 PS 的装配（`buildProductionOwnership`）。

另有一格属产品策略，可随上面一并认：治理登记册上的能力与事实类型词表。两族认证方今天取 `OPERATION_DECISION` + 决定种类、`EXTERNAL_FACT` + 外部事实类型，15 判断项 3 记着「首个租户登记区间时要对齐」；演示区间照这两组常量登。

## 做什么（裁完照此拆笔）

1. `AdmissionScope` 的生产实现：按问题一、二读租户对照，换成 `AuthorityCoordinates` 问 `AuthorityCoverage.Covers`。对照没登答不在准入范围；对照或区间读不动答 `ErrAdmissionScopeUnavailable`，不折成不在准入范围（14 的分格）。
2. 生产装配：`buildOperatorMinter`、`buildIntegrationClientMinter` 改收这只实现，`main.go` 建它（区间读口接真库，selfAuthority 按问题三）。selfAuthority 没配时，各口答复与今天逐字节相同。10 接 `WorkFactMinter` 时取同一只。
3. 演示租户：`SYN-TENANT-01` 的对照，与 `OPERATION_DECISION`、`EXTERNAL_FACT` 下各种类的区间，经普通登记进种子，标识带 `SYN-` 前缀；演示部署配上 selfAuthority。取证要用的授予若种子里还没有，随本票补，只加自己那几条。
4. 问题一取甲或乙时，参数登记册增一行（某租户的对照，租户取值），格里只写尚无租户。

## 不做

- 任何真实租户的对照、区间与权威串（租户取值）。
- 15 余下四口的换口与撤放行、10 的换口（各在其票）。
- 改 `AdmissionScope` 接口让它看得到对象（问题二若裁为要，另立）。

## 完成判据

- 对照已登且区间覆盖 → 铸成；对照未登，或登了而区间不覆盖（别的权威、已关、未始、别的对象范围）→ 不在准入范围；对照读不动、区间读不动 → 依赖故障格，且不被「不在准入范围」认出（带测试）。
- 生产装配：selfAuthority 未配时，`TestSwappedDecisionFacesAnswerFromTheOperatorChannel` 与 `TestIntegrationClientFacesAnswerTheChannelGrades` 的准入那一格照旧；配了且对照与区间齐备时，装载分配与终止参与答业务结果（`cmd/parcel-api` 装配测试带 DSN）。
- 演示环境灌完种子后，带运营决定授予的合成操作者在装载分配或终止参与上不再答 `OUTSIDE_ADMISSION_SCOPE`（`scripts/demo-seeds/operator-channel/probe-operator-channel.sh` 那一套取一次证据，证据层级 `S`）。

## 分诊（2026-10-13 通道 2；派单 `task-f2fa6dc4` ← 通道 1；取证钉 `b3c8bd46`）

**结论**：三问与词表一格逐条过了分界检验（[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md#切片的机制半边与实例半边)「切片的机制半边与实例半边」、ADR-0146 决定二）。问题一、三一定就要落迁移、定参数来源，难逆转，起草成 [ADR-0180](../../../docs/adr/0180-admission-scope-tenant-mapping-is-an-access-identity-register-and-self-authority-is-a-deployment-parameter.md)（Proposed，接受归用户）；问题二与词表一格随它写进同一篇。都不落在已接受 ADR 的原话里——ADR-0149 决定四第二条只说「按（租户、对象范围、能力、事实类型）读」，没说租户那一维住哪；14 判断项 1、2 只说「由 `AdmissionScope` 的实现去换」「两处将来装配时应取同一个值」——所以不直接转 ready-for-agent。理由与否决的候选都在 ADR-0180，这里只记归类、推荐与落点。

### 归类与推荐

| 格 | 归类 | 推荐 |
|---|---|---|
| 问题一 · 对照住哪 | 册的存在、列、落点与答复格是机制；行是租户取值 | 甲：`access_identity` 另立对照册（ADR-0180 决定一） |
| 问题二 · 键 | 机制 | （租户、能力、事实类型），任一时刻至多对一个对象范围；同键区间重叠在登记时拒（决定二） |
| 问题三 · 权威串 | 参数这一格是机制，值是部署取值 | `parcel-api` 一件部署形态参数，桥与 PS 生产分支同读；PS 跟，只跟参数来源；隔离分支不动，两值不等拒启动（决定三） |
| 词表 | 产品策略 | 认 `OPERATION_DECISION` + 决定种类、`EXTERNAL_FACT` + 外部事实类型，不带 `SYN-`；能力词一处定义（决定四） |

### 问题一：甲、乙、丙逐条核

| 出处 | 甲 · 接入身份对照册 | 乙 · 治理侧带租户的表 | 丙 · 部署参数 |
|---|---|---|---|
| ADR-0083 决定一 | 相容：治理册照旧没有租户维，带租户的行落在本就按租户绑定的 `access_identity` | 相抵：给治理上下文添租户维，要另出记录部分停用，还掀掉决定三「`pilot_governance` 的最高（也是唯一）隔离边界就是产品实例本身」的前提 | 相容：不落库 |
| ADR-0149 决定四第二条 | 相容：铸信封前读一次，区间仍是「谁在写生产」的唯一权威，对照只换坐标 | 相容 | 形式相容：「按租户读」的那一维成了进程参数 |
| ADR-0149 越权风险点 3 | 相容：没对照、没区间都答不在准入范围 | 相容 | 相容：未设即不在准入范围 |
| ADR-0150 决定一 | 相容：演示租户登一行普通对照，与真实租户同一条路（决定二「走普通登记，不另造旁路」） | 相容：经 `parcel-governance-register` 登 | 代码路径相容；演示与真实租户都靠部署参数进来，没有登记轨迹 |
| 14 判断项 1 | 兑现：「租户随试点登记」，换在 `AdmissionScope` 的实现里 | 相容 | 相抵：参数不是登记——没依据、没生效区间，改一次要重启 |
| 14 判断项 2 | 不涉及：权威串另由问题三定 | 不涉及 | 不涉及 |
| 14 判断项 4 | 兑现：桥随第一个消费者（15 之四、11）立；各口准入要求已在认证方里 | 同甲 | 同甲 |

**问题二的理由**：`Admits` 看得见的只有（租户、能力与事实类型、时刻）；粗一档的键表达不了已分属不同对象范围的能力和分批进生产的决定种类；一键多范围而任一覆盖即放行，会把试点范围外对象上的写放进来。按对象分要改 14 的接口，照本票「不做」另立；在那之前登记指引的约束见 ADR-0180 越权风险点 2。

**问题三的理由**：对照是租户取值，权威串是部署取值，两者不同类（ADR-0100 对发行方的分界：决定二把发行方地址与 JWKS 端点定为部署形态参数，理由在其 Context「谁签发操作者令牌是产品的部署形态参数，不是租户参数」）；做成产品常量就没有「未配置」一态。PS 不跟，桥与 PS 各有一处来源，14 判断项 2 只能靠约定守；跟的代价只在 `cmd/parcel-api`——PS 生产分支的目录照旧为空，`governanceScope` 先于读册早退，答复不变。

### 做什么四件：ADR-0180 接受后的落点

1. **`AdmissionScope` 的生产实现**
   - 对照册：`internal/accessidentity` 的对照类型与读写端口；`migrations/access_identity/` 下一号迁移（钉 `b3c8bd46` 时最高是 `0004`）；`internal/accessidentity/adapters/postgres` 的读写适配器，带真库测试；`cmd/parcel-access-register` 登记、结束两个子命令，照操作者授予与撤销的样子；登记子命令的帮助文本写明 ADR-0180 越权风险点 2 的粒度约束（见其接受记录）。
   - 桥：新包 `internal/accessidentity/adapters/pilotgovernance/` 实现 `accessidentity.AdmissionScope`——读对照 → `AuthorityCoordinates` → `AuthorityCoverage.Covers`。跨模块翻译只能落在 `internal/<consumer>/adapters/<provider>/`（`internal/architecture` 的 `TestBusinessModulesDoNotReachIntoEachOther`），所以不放 `cmd/`，也不放 `accessidentity` 根包。分格照本票「做什么」原第 1 条。
   - 能力词一处定义（ADR-0180 决定四第三项）：运营决定沿用 `internal/accessidentity` 已导出的操作者能力面 `CapabilityOperationDecision`，外部事实在那里与 `AdmissionRequirement` 同处新导出一个；PS、TF、CC、SA 的认证方各改一行去引用。越出原地盘，可拆成先行的一小笔。
2. **生产装配**：`cmd/parcel-api` 的 `buildOperatorMinter`、`buildIntegrationClientMinter` 改收桥，`main.go` 建它（区间读口取 `pgpostgres.NewAuthorityIntervals`，对照读口接真库）。权威串参数在 `cmd/parcel-api` 解析一次，交 `NewAuthorityCoverage` 与 `buildProductionOwnership` 的生产分支；`buildIsolatedWriteAdmission` 那一侧加两值不等即拒。头注随改：两只铸造器头注里「换成桥」那句，`buildSubmissionOrchestration` 头注「生产形态下留空的两样」改为只剩目录，`buildProductionOwnership` 里 Directory 与 SelfAuthority 那句。10 接 `WorkFactMinter` 时取同一只，不在本票。
3. **演示租户**：对照种子进 `scripts/demo-seeds/data/access/`，与操作者、授予的种子同处——对照住接入身份，不进 `data/governance/`。`OPERATION_DECISION`、`EXTERNAL_FACT` 下所需各词的区间进 `data/governance/`，对象范围带 `SYN-`，权威串取 `SYN-AUTH/idp-parcel-pilot`。`seed.sh` 的接入身份与治理两段；`scripts/demo-seeds/README.md` 的演示部署加权威串参数。取证要用的授予若缺，进 `data/access/02-grants.json`，只加自己那几条。
4. **参数登记册**：增「准入范围对照」一行，编号取下一个 `PAR-INT-*`（钉 `b3c8bd46` 时最高是 `PAR-INT-10`），格里只写尚无租户；`PAR-GOV-03` 加 ADR-0180 拆分注——「能力、事实类型」两项的词表归产品策略，登哪几格归租户取值。

**收窄后的地盘**（ADR-0180 接受后替换本票头部那行）：`internal/accessidentity` 的对照类型与端口、`adapters/postgres` 的对照适配器、新包 `adapters/pilotgovernance`；`migrations/access_identity/` 下一号；`cmd/parcel-access-register` 两个子命令；`cmd/parcel-api` 的 `buildOperatorMinter`、`buildIntegrationClientMinter`、`buildProductionOwnership`、`buildIsolatedWriteAdmission`、权威串解析与 `main.go` 相应几行；`scripts/demo-seeds/data/access/`、`data/governance/`、`seed.sh` 两段与 `README.md`；参数登记册新的一行与 `PAR-GOV-03`。能力词一处定义若随本票做，另加 PS、TF、CC、SA 认证方各一行。

**并行**（22:1x 取证，只作此刻）：在途分支 `mcp5-psb26` 改了 `scripts/demo-seeds/seed.sh` 与 `scripts/demo-seeds/README.md`（计价与结算段），与第 3 件同文件不同段；实施方开工前看它进没进 main。

### 顺带的事实：10 余下那一半接 `WorkFactMinter`，是否与 15 余下四口同一处境

只报事实，取证钉 `b3c8bd46`：

- `NewWorkFactMinter`、`MintWorkFact` 没有非测试调用点，`WorkFactEnvelope` 只出现在 `internal/accessidentity` 里。`internal/transportfulfillment/adapters/accessidentity/` 只有操作者认证方（运营决定的 `operator_authenticator.go`、登记写面的 `operator_registry_authenticator.go`）；`internal/nodeoperations/adapters/` 下没有 `accessidentity` 适配器。作业事实请求的防腐认证方在 TF 与 NO 都还不存在，没有任何代码给作业事实请求带 `AdmissionRequirement`。
- 非测试代码里构造 `AdmissionRequirement` 的只有 PS、TF 的操作者认证方（`OPERATION_DECISION` + 决定种类）与 CC、SA 的集成客户端认证方（`EXTERNAL_FACT` + 外部事实类型）；作业事实登记面没有能力词。
- `MintWorkFact` 只在 `request.Admission` 非空时判准入范围。所以同一处境是有条件的：认证方带上准入要求（ADR-0149 决定四第二条对生产写的要求），装配取 `UnconfiguredAdmissionScope` 时，就与 15 余下四口一样答 `403 OUTSIDE_ADMISSION_SCOPE`，换成本票的桥后同样越过。比 15 余下四口多一件：作业事实登记面的能力词与事实类型词还没有（ADR-0180 决定四末项把它们归同一张词表、随 10 定），演示区间也要等这组词定了才登得了。另外 10 余下各口分在 TF 与 NO 两个上下文，认证方两边各要立一只。

### 接手核对（2026-10-13 22:4x 通道 3；取证仍钉 `b3c8bd46`）

通道 2 交了分诊与 ADR-0180 草案（`2d7c4ab3`），在逐条核引证时会话中断；用户转派通道 3 接着做。通道 3 在 `b3c8bd46` 上把 ADR-0180 与本节引用的原句、符号与取证逐条对过源头，改正如下，余下各条核对成立。

- **ADR-0100 那句理由出自 Context，不在决定二。**「谁签发操作者令牌是产品的部署形态参数，不是租户参数」在 ADR-0100 的 Context；决定二信任锚那一条的原话是「发行方地址与 JWKS 端点是 `parcel-api` 的部署形态参数」。ADR-0180 候选丙、Links 与本节「问题三的理由」都改成两处分引。
- **ADR-0149 候选丁按原句引**：「准入范围由各上下文在应用层各自判」，原稿缩写成了「各上下文各自判」。
- **ADR-0083 决定三按原句引**：「`pilot_governance` 的最高（也是唯一）隔离边界就是产品实例本身」，原稿本节表里是一句改写。
- **ADR-0150 决定三按原句引**：越权风险点 5 引的是「过渡期内隔离形态的现行规则……一字不变」，原稿写成「现行规则不变」。
- **「同维两条区间本身是记录错误」出自 parcel-shipment 生产归属适配器的头注**，不是治理侧的原话；ADR-0180 决定二照实写出处。
- **能力词的取证不全（实质改动）。** 原稿说 `OPERATION_DECISION` 只在 PS、TF 两个认证方里各定义一次、都不导出。其实它已是 `internal/accessidentity` 导出的操作者能力面 `CapabilityOperationDecision`，操作者册 `0002` 迁移的能力面约束也是同一个串；不导出的只有 PS、TF 各自另定义的那两个同串常量。`EXTERNAL_FACT` 原稿的说法成立：只在 CC、SA 两处各一次，`internal/accessidentity` 里没有它。ADR-0180 决定四第三项与本节做什么第 1 件随之改为：运营决定那一个词沿用 `CapabilityOperationDecision`，外部事实那一个词新导出；越权风险点 7 补上这条取法背后认下的东西。
- **起草方的读过范围说窄了几处**：参数登记册只有 `PAR-GOV-03` 读了整行，`PAR-INT-08..10` 与 `PAR-GOV-04..07` 只读了各行开头一段；`access_identity` `0001` 授予与撤销两表按列名取，另读了 `0002` 的能力面约束；演示种子的治理区间只有 01、02、03、07 读过全文，其余按键取。ADR-0180 Status 行照此改。

核对成立、未改的各条，取证命令与输出不入票：`AdmissionScope` 非测试实现只有 `UnconfiguredAdmissionScope`，两只铸造器在生产装配里都取它，`NewAuthorityCoverage` 没有生产调用点；`Admits` 的签名；两条被点名的装配测试与其 "granted, admission not registered" 一格；`AuthorityCoverage` 与 PS 生产归属适配器两处头注都写「全册一个约定」；`governanceScope` 在目录缺席或权威串为空时先于读册返回；隔离形态的三个合成常量与种子里委托受理那条区间的权威串；读写两开关取值不一即拒启动；`IDP_PARCEL_*` 的命名；非测试代码里构造 `AdmissionRequirement` 的只有 PS、TF 的操作者认证方与 CC、SA 的集成客户端认证方，操作者、集成客户端与作业事实的铸造器都只在 `Admission` 非空时判；`WorkFactMinter` 一族没有非测试调用点，TF 的 `accessidentity` 适配器只有操作者认证方、NO 没有 `accessidentity` 适配器；跨模块门禁的测试名、`NewAuthorityIntervals` 的落点；`access_identity` 迁移最高 `0004`、登记册最高 `PAR-INT-10`；演示种子里路由、计价、委托受理分属不同对象范围，没有 `OPERATION_DECISION` 或 `EXTERNAL_FACT` 的区间。

**进 main**（2026-10-13 22:4x 通道 3）：用户裁定免非作者评审、由通道 3 直接重放；改动只有本票、ADR-0180 与 `docs/adr/README.md` 一行，纯 `.md`。在隔离树 `idp-parcel-replay-oc19t-mcp3` 把分支四笔原样重放到 main `aefbcaa4` 之上——`90f0ef3c`→`60eee1a0`、`27671572`→`466d297f`、`2d7c4ab3`→`32bd3f80`、`d88780ed`→`7e7b6863`；分支上的 SHA 作封存出处，本地分支指针改名 `merged/mcp2-oc19t`。main 自 `b3c8bd46` 起动过的文件与本分支不重叠，main 上没有别的 `0180`；清点在重放 tip 上重生成零差。本票不收口：ADR-0180 接受前停在 needs-info。
