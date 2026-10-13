# ADR-0180：准入范围的租户对照是接入身份的一本册——按（租户、能力、事实类型）对至多一个对象范围，带生效区间与依据；本产品在治理登记册里的权威串是 `parcel-api` 的一件部署形态参数，准入范围桥与 parcel-shipment 生产归属同读这一处；登记册上的能力与事实类型词表归产品策略

Status: Proposed（**草案**，2026-10-13 起草：通道 2 随票 [operator-channel/19](../../.scratch/operator-channel/issues/19-admission-scope-bridge-for-operator-and-integration-client-minters.md) 分诊，派单 `task-f2fa6dc4` ← 通道 1。**接受与否归用户**，接受前不是依据；草案期间 ADR-0083、ADR-0149 一字不动。起草方裁决能力边界：读过[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「产品就绪与试点就绪」「切片的机制半边与实例半边」两节，[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)、[ADR-0083](./0083-pilot-governance-read-face-carries-registry-dimensions-only.md)、[ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)、[ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 全文，[ADR-0151](./0151-unassigned-command-faces-get-their-families.md) 的 Context 后半与决定一至五，[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定二信任锚那一条与 Context 里信任锚那一句（按关键词取句，非全文）；票 operator-channel/14、19 全文，10 的做什么、完成判据与完成记录判断项，15 判断项 3 与进展记录里改挡点的那两句；[参数登记册](../product/PILOT-PARAMETER-REGISTER.md) `PAR-GOV-03` 整行，`PAR-INT-08..10` 与 `PAR-GOV-04..07` 各行的开头一段（截取，非整行）；`migrations/pilot_governance/0001` 抬头注，`migrations/access_identity/0001` 抬头注与授予、撤销两表的生效与撤销列（按列名取）、`0002` 的能力面约束、`0003` 抬头注与键。代码读过 `internal/accessidentity` 的 `AdmissionScope` 一族、`WorkFactMinter`、包注释、操作者能力面常量与 `DecisionKind`、`ExternalFactType` 两组词；`internal/pilotgovernance` 的 `AuthorityCoverage` 与领域 `AuthorityInterval`；parcel-shipment `adapters/pilotgovernance` 生产归属适配器的端口、依赖与 `governanceScope`；`cmd/parcel-api` 的 `buildOperatorMinter`、`buildIntegrationClientMinter`、`buildSubmissionOrchestration`、`buildProductionOwnership`、`buildIsolatedWriteAdmission` 与隔离形态的治理常量；各认证方取 `AdmissionRequirement` 的那一句；`internal/architecture` 的跨模块门禁；演示种子 `data/governance/` 各文件的对象范围、能力、事实类型与权威串（区间 01、02、03、07 读过全文，其余按键取），`seed.sh` 治理一段（按关键词）。2026-10-13 22:4x 通道 3 接手（通道 2 会话中断、用户转派）逐条复核引证，改正处记在票 19 分诊节末「接手核对」。没读：ADR-0025、ADR-0091、ADR-0100 全文，ADR-0078，`OperatorMinter` 与 `IntegrationClientMinter` 全文，`access_identity` 0002、0004 迁移，`cmd/parcel-access-register` 各子命令实现，`cmd/parcel-api` 各装配测试正文，`probe-operator-channel.sh`，CONTEXT-MAP 与 GLOSSARY 全文，`PILOT-SCOPE`，登记册其余各行。拿不准的列在文末「越权风险点」。）
Date: 2026-10-13

## Context

**桥的两端都在，中间那一格没有出处。** ADR-0149 决定四第二条要求铸信封前按（租户、对象范围、能力、事实类型）读生产权威区间。票 operator-channel/14 交了只读口 `AuthorityCoverage`——按对象范围 × 能力 × 事实类型三维问，因为治理登记册没有租户维（ADR-0083 决定一）——与「不在准入范围」一格，并把「租户到对象范围的对照」留给 `AdmissionScope` 的实现（14 判断项 1）。铸造侧的 `AdmissionScope.Admits` 只收（租户、能力与事实类型、时刻）。取证钉 `b3c8bd46`：非测试代码里 `AdmissionScope` 的实现只有 `UnconfiguredAdmissionScope`，生产装配里操作者与集成客户端两只铸造器都取它；`NewAuthorityCoverage` 没有生产调用点。

**挡着的是一串换口。** operator-channel/15 已换的运营决定口与 11 已换的集成客户端口，在设了发行方的生产装配里授予齐备也停在 `403 OUTSIDE_ADMISSION_SCOPE`；15 余下的口与 10 余下那一半换口要同笔撤隔离放行（ADR-0150 决定三），演示租户在这些口上随之退成 `403`。演示租户越过准入范围只有普通登记一条路（ADR-0150 决定一），不能按 `SYN-` 前缀开例外。

**要裁的是三格，外加一格词表。** 对照的行住在哪、按什么键；本产品在登记册里的权威串从哪来、parcel-shipment 那一处跟不跟；登记册上的能力与事实类型用谁的词。前两格一定就要落迁移、定参数来源，难逆转，故成本记录。

## 候选与反方

### 对照住在哪

- **甲 · 接入身份另立一本对照册**：`access_identity` 下一号迁移，`parcel-access-register` 加子命令，行带生效区间与依据。即决定一。
- **乙 · 试点治理另立一张带租户的对照表。** 反方：ADR-0083 决定一否决给治理登记册加租户维，理由是治理对象本是产品实例级；另立一张带租户的表，同样是给这个上下文添一维它领域里没有的东西。它还掀掉决定三的前提「`pilot_governance` 的最高（也是唯一）隔离边界就是产品实例本身」——治理读面的隔离放行不带租户，带租户的表一进读面就得按租户隔离。要另出记录部分停用 0083 决定一、三，换来的东西甲不付这个代价也有。否决。
- **丙 · 部署参数，同发行方那几件。** 反方：对照的每一行属于某个租户，是租户取值；ADR-0100 决定二把发行方地址与 JWKS 端点定为 `parcel-api` 的部署形态参数，其 Context 写明的理由恰是「谁签发操作者令牌是产品的部署形态参数，不是租户参数」，对照不满足这一条。部署参数没有依据、没有生效区间、没有历史，改一次要重启，PN-08 的记录能力里就缺了「哪个租户自何时、凭什么进了哪个对象范围」的答案；14 判断项 1 写的也是「租户随试点登记」。否决。

### 对照按什么键

- **只按租户。** 反方：演示种子里路由、计价、委托受理已分属不同对象范围，同一租户的不同能力本就落在不同试点范围。否决。
- **按（租户、能力）。** 反方：同一能力下的决定种类或外部事实类型分批进生产（先开装载分配、后开终止参与）就表达不了。否决。
- **按（租户、能力、事实类型），一键可对多个对象范围，任一覆盖即放行。** 反方：`Admits` 看不到这一笔改的是哪个对象，任一覆盖即放行会把试点范围外对象上的写也放进来——越权放行，正是准入范围要挡的。否决。
- **按（租户、能力、事实类型），任一时刻至多对一个对象范围。** 即决定二。
- **按对象分，Intake 把对象交给 `Admits`。** 这要改 14 的接口，票 19「不做」已排除。本记录不否决它，只不在此做，见越权风险点 2。

### 本产品的权威串从哪来

- **产品常量。** 反方：没了「未配置」这一态，产品会认领任何恰好写了这个串的区间；租户在自己的登记册里怎么称呼本产品，是那个部署的事。否决。
- **治理登记册里另登一条「本产品的权威串」。** 反方：多一道迁移、一个子命令，登的是一个部署至多改一次的值；还让「哪些区间算本产品的」随一条登记改动，与「本产品是谁」随部署走（发行方、受众同此）相悖。否决。
- **每条对照各带一个权威串。** 反方：把「全册一个约定」（`AuthorityCoverage` 与 PS 生产归属适配器两处头注同句）拆成逐租户，等于让租户的对照决定谁是本产品。否决。
- **`parcel-api` 的一件部署形态参数，两处同读。** 即决定三。

### 能力与事实类型用谁的词

- **租户登区间时自取词，产品去对齐**（15 判断项 3「首个租户登记区间时要对齐」的字面）。反方：准入要求是认证方里的产品代码定的，租户取什么词都得等于它，「对齐」只能是租户去抄产品的词；把它算作租户取值，只会让拼错一格的租户撞上与「没登」同一张脸的 `403`。否决。
- **词表归产品策略，随认证方定、随产品发布。** 即决定四。

## Decision

**一、对照是接入身份的一本册。** `access_identity` 另立「准入范围对照」：一行说某租户在（能力、事实类型）上的生产写落在治理登记册的哪个对象范围，带生效起止（止可空）与依据引用。只增不改，结束另登一行——同操作者授予与撤销的取法，被结束的对照是那段时间里它确实有效的证据。册的列由产品定，行是租户取值，经 `parcel-access-register` 受控登记，迁移不种任何行。对照只做坐标换算，不回答「谁在写生产」：那一问的唯一权威仍是治理登记册的生产权威区间（ADR-0149 决定四第二条，及其否决的候选丁「准入范围由各上下文在应用层各自判」）。治理登记册照旧没有租户维，ADR-0083 一字不动。

**二、键是（租户、能力、事实类型），任一时刻至多对一个对象范围。** 同键的生效区间重叠在登记时拒，与 parcel-shipment 生产归属适配器读治理区间时同一判据（其头注「同维两条区间本身是记录错误」）。对照没登，或登了而那个对象范围上此刻没有本产品的区间，都答不在准入范围（ADR-0149 越权风险点 3）；对照读不动答依赖故障，不折成不在准入范围（14 的分格）。

**三、本产品在治理登记册里的权威串是 `parcel-api` 的一件部署形态参数，桥与 parcel-shipment 生产归属同读这一处。** 参数名照现有 `IDP_PARCEL_*` 取法，建议 `IDP_PARCEL_GOVERNANCE_SELF_AUTHORITY`；值是部署取值，产品不给默认。`cmd/parcel-api` 只解析一次，交给 `NewAuthorityCoverage` 与 `buildProductionOwnership` 的生产分支。

- 未设即未配置：桥答不覆盖，各口答 `OUTSIDE_ADMISSION_SCOPE`；PS 归属答`权威未确定`。两者与今天逐字节相同。
- PS 跟，只跟参数来源：生产分支的 `Directory` 照旧留空，`governanceScope` 在目录缺席时先于读册就早退，PS 的答复不因本条改变；目录本身归 PS 生产归属那一票。
- 隔离分支不动，照旧取 `isolatedSelfAuthority`（ADR-0150 决定三：过渡期内隔离形态的现行规则不变）。隔离写开关开着、参数也设了而两值不等，进程启动即拒——一个进程里不许有两个「本产品」，与读写两开关取值不一即拒同一取法；隔离写开关删去的那一笔，这条检查随之删去。
- 演示部署取 `SYN-AUTH/idp-parcel-pilot`，与隔离形态常量、种子里委托受理那条区间同串。

**四、治理登记册上的能力与事实类型词表归产品策略。** 一个命令口的生产写登在哪个能力、哪个事实类型下，不看任何租户就能回答（ADR-0146 决定二）；租户取值是给哪几个（对象范围、能力、事实类型）登区间、登给谁。

- 运营决定口：能力 `OPERATION_DECISION`，事实类型取决定种类词（`accessidentity.DecisionKind` 的值）。外部结果与资金事实口：能力 `EXTERNAL_FACT`，事实类型取外部事实类型词（`accessidentity.ExternalFactType` 的值）。即两族认证方今天的取法，本条认下。
- 这些词不带 `SYN-` 前缀：它们是产品词，不是实例标识。演示区间照原词登，带 `SYN-` 的是对象范围与权威串。
- 一词一处定义：各认证方引用 `internal/accessidentity` 导出的那一个词。取证钉 `b3c8bd46`：`OPERATION_DECISION` 已是 `internal/accessidentity` 导出的操作者能力面 `CapabilityOperationDecision`（操作者册 `0002` 迁移的能力面约束同串），PS 与 TF 的操作者认证方却各自另定义了一个同串的不导出常量；`EXTERNAL_FACT` 在 CC 与 SA 的集成客户端认证方里各定义一次、都不导出，`internal/accessidentity` 里没有它。所以运营决定那一个词不新立，认证方改引 `CapabilityOperationDecision`：准入要求里的能力就是授予所在的那个能力面。外部事实那一个词在 `internal/accessidentity` 与 `AdmissionRequirement` 同处新导出，CC 与 SA 改引它。不收拢的代价：改了其中一处，同一族就分成两个词，登在旧词下的区间对改过的那一边无声失效。
- 别的能力面要判准入时（如作业事实登记），它的词同属本表，随那一面的票定，取法同此：一个能力词，事实类型逐口取。

## Consequences

- 票 operator-channel/19 据本记录收窄地盘、拆做什么各件，落点写在该票「分诊」节；本记录接受前该票停在 needs-info。
- 参数登记册增「准入范围对照」一行（随 19 做什么第 4 件）：册的列由产品定，行是租户取值，格里只写尚无租户。`PAR-GOV-03` 所列「能力、事实类型」按决定四拆注：词表归产品策略，登哪几格归租户取值。
- 演示种子的运营决定与外部事实区间照决定四的词登，对象范围与权威串带 `SYN-`。
- 15 判断项 3「首个租户登记区间时要对齐」由决定四作答；那张票面由派单方随 15 余下的口一并改。
- ADR-0083、ADR-0149 不需部分停用：本记录在两者原话之内补出对照的落点与键。
- [adr/README](./README.md) 增本记录一行。

## 越权风险点

1. **接入身份收进了一个治理坐标。** `accessidentity` 的包注释说它不拥有业务领域语言；对照里的对象范围是治理登记册里的脱敏引用，本册只当不透明串存、不解释，与操作者册里作业范围引用同等地位。若 owner 认为「租户参与哪个试点范围」该是试点治理自己的概念（比如日后 PN-01 要给试点范围登参与方），甲要改投那一处；迁移已落则另迁。
2. **键的粒度。** 同一租户同一能力与事实类型按线路或客户分批上线时，一行对照会把这个租户该类生产写全部放进那个对象范围。登记指引必须写明：只在这个租户该类生产写全部落在那个对象范围时才登对照。要更细，得让 Intake 把对象交给 `Admits`（改 14 的接口），另立。
3. **两本册要同一时刻切。** 试点范围换版本（如 `@v1` → `@v2`）时，对照行与治理区间分属两本册，错开的那一段答不在准入范围——朝拦不朝放，但演示与上线脚本要两边一起登。
4. **暂停不进准入范围。** 本记录照 ADR-0149 决定四第二条只读区间。暂停决定停的是新准入，运营决定与外部事实落在在途对象上，本记录据此不让暂停拦它们；要不要接，归 PN-08 owner。
5. **启动即拒是过渡期隔离形态上多出的一道装配检查。** 决定三第三项不改任何口的准入，只拒一种自相矛盾的装配；若 owner 把 ADR-0150 决定三「过渡期内隔离形态的现行规则……一字不变」读作连装配检查也不许加，改为启动日志出声。
6. **PS 生产归属的能力与事实类型词不在本记录内。** 隔离形态今天取 `SYN-CAP/shipment-intake` 与 `SYN-FACT/shipment-request`，那是合成坐标，随隔离形态退场；PS 生产目录立起时要的词同属决定四这一类，归 PS 生产归属那一票定。
7. **决定四第三项越出票 19 的原地盘**，要动 PS、TF、CC、SA 的认证方各一行；派单方可拆成先行的一小笔。运营决定的能力词取操作者能力面那一个常量，等于认下「准入要求里的能力就是授予所在的能力面」；集成客户端族没有能力面（授予按事实类型登），外部事实那个词因此是新立的。owner 若要治理登记册另有一套与能力面分开的能力词，改的是决定四第三项与认证方各一行。
8. **对照册的列与子命令照授予的样子推**，而 `access_identity` 0002、0004 迁移与 `parcel-access-register` 的实现没读；与那几处既有纪律有出入的，以实施时核对为准。

## Links

- [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)：决定四第二条与越权风险点 3，本记录补出其中「租户」一维的落点
- [ADR-0083](./0083-pilot-governance-read-face-carries-registry-dimensions-only.md)：决定一、三，治理登记册无租户维；本记录不动它
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：决定二的分界检验，决定四据以归类
- [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)：决定一、二、三，演示租户经普通登记越过准入范围、隔离形态过渡期规则不变
- [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)：决定二与 Context 信任锚那一句，部署形态参数与租户参数的分界
- [ADR-0151](./0151-unassigned-command-faces-get-their-families.md)：决定三，运营决定口要判准入范围
- 票 [operator-channel/14](../../.scratch/operator-channel/issues/14-admission-scope-read-and-grade.md)：只读口与答复格，判断项 1、2、4
- 票 [operator-channel/19](../../.scratch/operator-channel/issues/19-admission-scope-bridge-for-operator-and-integration-client-minters.md)：本记录承接的分诊票
