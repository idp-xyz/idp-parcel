# 试点治理（`pilotgovernance`）

代码在 `internal/pilotgovernance`，登记入口在 `cmd/parcel-governance-register`。对照 main `1bdbc1b7b`（2026-11-12）写成。

**这份文件核到什么程度**：旧文档里没有这个包的专属上下文文档，全文是从代码与 Go 注释里读出来的。取值表、处理结果、入口三节照抄代码里的常量。「规则」一节每条只确认了代码里有对应的类型、函数或测试名，没有跑测试验证行为。词义参照了旧的 PN-08 交接与术语表，只取用词。

## 管什么，不管什么

它是产品级的治理记录与编排能力，不是限界上下文：不建运输状态机，不拥有任何物流业务事实。

管：候选版本组，阶段评审决定（`Go` / `No-Go`），生产权威区间，暂停与恢复新准入的决定，对象级接管记录，在途盘点，范围版本之间的覆盖关系，受控通道的执行留痕。对外答两问：某组坐标此刻是否落在本产品的生产权威区间内；某个范围版本此刻还拦不拦新准入。

不管：

- **某个真实租户能不能上生产。** 本包只把治理决定记下来。能不能上，只来自真实证据加一次阶段评审的 `Go`；合成（`S`）证据永不升级，代码里没有任何把它改记成别的层级的入口。
- 真实客户、线路、伙伴、阈值。记录里只有脱敏引用，真实值在参数登记册。
- 委托、包裹、在途对象的生命周期。暂停不取消、不迁移、不回退在途；盘点只是审计快照。
- 把治理答复变成「建不建委托」——`parcel-shipment` 经自己的适配器读登记册后自行决定。
- 租户到对象范围的对照——`accessidentity` 与 `parcel-shipment` 各自换好坐标再来问。登记册没有租户维。
- 谁被允许运行登记入口、真实角色名——租户取值，产品不设默认。

## 词

| 词 | 意思 |
|---|---|
| 候选版本组 | 一次阶段评审固定下来的三样引用：范围版本、参数快照、规则与契约版本。不可扩张 |
| 范围版本 | `pilotScopeVersion` 的脱敏引用。不透明串，推不出谱系 |
| 执行阶段 | 评审时所处的阶段，见取值表 |
| 阶段评审决定 | 对一个评审目标、一个候选版本组给出的 `Go` 或 `No-Go`，不可覆盖 |
| 保留的偏差 | 随 `Go` 留下的非关键偏差，六件齐全才能留 |
| 生产权威区间 | 对象范围 × 能力 × 事实类型在一段生效区间内的唯一写入方。终点为空即开放 |
| 权威冲突 | 同一对象范围、能力、事实类型上两条区间时间重叠 |
| 暂停决定 | 暂停指定范围版本的新委托纳入 |
| 恢复决定 | 解除一条指名的暂停。一条暂停至多一次恢复 |
| 在途盘点 | 逐对象的审计快照：身份、当前事实、当前权威方、责任方、下一行动、复核时间 |
| 对象级接管 | 原权威确实无法继续时，对明确对象范围建立新权威区间的记录 |
| 覆盖关系 | 两个范围版本之间登记的一条边：后继承继前代的暂停，或两版互不相干 |
| 续办引用 | 记录已入册但后续一步没做成时交回的引用，重跑同一命令补齐 |
| 通道技术身份 | 登记入口自取的 OS 进程属主与主机名，随登记同笔事务留痕 |

## 取值表（代码里的封闭集）

| 轴 | 取值 |
|---|---|
| 执行阶段 `ExecutionStage` | `NOT_YET_IN_EXECUTION` `HISTORICAL_REPLAY` `SHADOW_RUN` `LIMITED_PRODUCTION` |
| 评审结论 `ReviewVerdict` | `GO` `NO_GO` |
| No-Go 处理方式 `NoGoDisposition` | `KEEP_CURRENT_SCOPE` `SUSPEND_NEW_ADMISSION` `FIX_AND_REASSESS` `OBJECT_LEVEL_TAKEOVER` |
| 覆盖关系种类 `ScopeVersionRelationKind` | `INHERITS_SUSPENSIONS`（有向） `UNRELATED`（对称） |
| 准入暂停依据 `AdmissionSuspensionGround` | `NOT_SUSPENDED` `SUSPENDED_BY_NAMED_SCOPE` `SUSPENDED_BY_INHERITED_SCOPE` `SUSPENDED_BY_UNREADABLE_SCOPE_RELATION` |
| 受控通道命令 `ChannelCommand` | `authority-interval` `suspend` `resume` `stage-review` `takeover` |
| 查阅册 `register` | `authority-interval` `suspension` `resumption` `takeover` |

## 处理结果

| 用例 | 结果 |
|---|---|
| 固定候选版本组 `FixCandidateSetHandler` | `FIXED` `ALREADY_FIXED` `CONTENT_CONFLICT` `NOT_ACCEPTED` `UNDECIDED` |
| 记录阶段评审 `RecordStageReviewHandler` | `RECORDED` `EXISTING_DECISION` `AUTHORITY_CONFLICT` `COVERAGE_CONFLICT` `NOT_ACCEPTED` `UNDECIDED` |
| 登记权威区间 `RegisterAuthorityIntervalHandler` | `REGISTERED` `ALREADY_REGISTERED` `AUTHORITY_CONFLICT` `NOT_ACCEPTED` `UNDECIDED` |
| 暂停、恢复、接管 `GovernIncidentHandler` | `SUSPENSION_RECORDED` `SUSPENSION_EXISTING` `RESUMPTION_RECORDED` `RESUMPTION_EXISTING` `SUSPENSION_NOT_FOUND` `TAKEOVER_RECORDED` `TAKEOVER_EXISTING` `TAKEOVER_CONFLICT` `NOT_ACCEPTED` `UNDECIDED` |

四类答案不得互相替代：`NOT_ACCEPTED` 是改请求才会好（形状不合、引用悬空）；两种 `CONFLICT` 是治理答案，要人看过再走；`UNDECIDED` 是依赖故障，落没落不知道，重跑同一命令即可。

续办引用的前缀：`CONT-INTERVAL/` `CONT-COVERAGE/` `CONT-TAKEOVER-INTERVAL/` `CONT-GOV-SUSPENSION` `CONT-GOV-RESUMPTION` `CONT-GOV-TAKEOVER`。

查阅口的问题码：`ACCESS_CHANNEL_NOT_CONFIGURED` `OPERATOR_CREDENTIAL_REJECTED` `OPERATOR_NOT_GRANTED` `IDENTITY_DEPENDENCY_UNAVAILABLE` `MALFORMED_REQUEST` `METHOD_NOT_ALLOWED` `INTAKE_FAILED` `NO_ANSWER_FORMED`。

## 规则

**候选版本组与阶段评审**

- 候选版本组三样引用加形成时间一次进入，没有改写入口。同标识同内容是重放，同标识不同内容拒；范围扩大或版本变更要换新组（`FixCandidateVersionSet`、`TestFixCandidateSetRefusesToWidenAFixedSet`）。
- 评审决定必须引用在册的候选版本组，引用不存在的组不受理。
- 同一评审目标对同一候选版本组只决定一次；再来一次交回在册那份，不翻（`ReviewKey`）。
- `Go` 不带处理方式，保留的每件偏差要有范围、控制、责任人、关闭期限、残余风险、接受引用六件。`No-Go` 必须显式写明四种处理方式之一，且不得保留偏差；系统不代推（`RecordStageReview`、`TestVerdictShapesAreMutuallyExclusive`）。
- 评审可以附带一条授予的权威区间和若干覆盖关系声明。权威冲突与覆盖关系相悖都在决定落库前预检，撞上就整次阻断并交回全部冲突对。
- 决定落库后区间或关系边没追加上的，决定不翻，交回续办引用；重跑同一命令只补那一步。
- 注释说授予区间只在 `Go` 进限量生产时在场。代码没有检查这一点：任何结论、任何阶段带上区间都会走预检与追加。

**生产权威区间**

- 区间四维与起点必填；终点为空即开放，有终点必须晚于起点。
- 同一对象范围、能力、事实类型上时间重叠即冲突，权威方相同也算（`DetectAuthorityConflicts`）。区间含起点、不含终点。
- 登记先判全等重放，再判冲突；冲突不落库。区间册只追加。
- 「本产品是否持有这段权威」由 `AuthorityCoverage.Covers` 答：本产品的权威串未配置、坐标不全、册里没有命中的区间，都答不覆盖。读不动区间册是依赖故障，不折成不覆盖。

**暂停与恢复**

- 暂停决定九件缺一不可：标识、触发来源、依据、证据、范围版本、执行人、发生时间、生效时间、在途处置说明。
- 恢复决定必须指名一条在册的暂停，带原因解除证据、一致性核对、在途盘点、决定人与两个时间。暂停不在册答 `SUSPENSION_NOT_FOUND`。
- 一条暂停至多一次恢复；再暂停是新的暂停决定。原暂停记录不回写。
- 没有恢复决定就仍然拦着。指标回落、规则不再命中、范围版本升版都不解除暂停。
- 同一范围多条暂停各自独立解除；还有一条已生效未恢复，就仍在暂停中。
- 生效时点那一刻起算已生效；暂停与恢复同刻，以恢复为准。
- 「还拦不拦」按四格作答，只有 `NOT_SUSPENDED` 放行，零值也算拦（`AdmissionSuspensionGround.Blocks`、`TestOnlyAnAbsentSuspensionOpensAdmission`）。暂停写明的正是所问版本，按命中拦；登有承继边，按承继拦；什么关系都没登，保守拦；登有互不相干的边，这条暂停不及于所问版本（`Suspensions.FindUnresumedSuspension`）。

**覆盖关系**

- 关系只能登记，不能从引用串的前缀、子串、版本号或时间序推出来。
- 关系只随一次阶段评审决定登记，后继一律是该次评审的范围版本，没有独立登记口。
- 自指的边拒。同一次评审对同一前代只能说一句话。
- 与在册的边相悖（同一有序对不同种类，或互不相干撞上任一方向的承继）即阻断；同一事实已在册则跳过；双向承继是两条各自成立的边（`CompareScopeVersionRelations`）。

**接管与在途盘点**

- 接管记录必须带原权威停止写入的证据、新权威区间、已接受事实、待处理的外部事项、实际控制、责任、下一行动、在途盘点与生效时间。
- 新区间先过冲突预检：撞上仍开着的既有区间即 `TAKEOVER_CONFLICT`，要先关原区间。
- 入册后先追加区间，再交出意图；区间没追加上就不发意图。
- 在途盘点至少一条，每条六件齐全，对象身份不重复（`TakeInventory`）。

**交接与留痕**

- 暂停、恢复、接管入册后各写一份意图进 Outbox，事件类型 `pilot-governance.suspension.recorded` `pilot-governance.resumption.recorded` `pilot-governance.takeover.recorded`。交出失败不翻记录，重放重发同一份。
- 每次到达登记册的执行（落册或已在册）与登记同一笔事务留一行痕，记命令、记录引用、通道技术身份、答案与时间。留痕失败则整笔撤回。拒绝与未决不留痕。
- 通道技术身份由入口从运行环境自取，输入里没有这一格。执行人、决定人是登记内容，只算「登记者声明了谁」。
- 登记输入里出现未知字段即拒；进程不内置任何默认值。

## 生命周期

- **候选版本组**：固定即定，不再变。
- **阶段评审决定**：记录即定，不可覆盖。
- **权威区间**：登记后只追加；没有关闭或修改既有区间的用例。
- **暂停**：已记录 →（到生效时点）拦新准入 →（恢复决定生效）不再拦。两条记录都不改。
- **接管**：记录入册 → 区间追加 → 意图交出；中途断了由重放从断点续齐。

## 入口

离线口 `cmd/parcel-governance-register`（运营在数据库网络内手跑，不监听端口），用法 `<子命令> -input <JSON 文件>`，连接串取 `IDP_PARCEL_POSTGRES_DSN`：

| 子命令 | 做什么 |
|---|---|
| `authority-interval` | 登记一条生产权威区间 |
| `suspend` / `resume` | 记录暂停、恢复决定 |
| `stage-review` | 同一笔事务里固定候选版本组并记录评审；评审没落成，候选组也不留 |
| `takeover` | 记录对象级接管 |

退出码：`0` 已登记或幂等重放；`1` 用法或输入不合法；`2` 治理答案（冲突阻断，或已入册但有续办引用）；`3` 未决。

在线口（`cmd/parcel-api`）：

- `GET /governance-registers?register=<册>`：只查阅权威区间、暂停、恢复、接管四册。`register` 缺席或不在封闭集内按坏请求拒。走操作者查阅读认证；渠道未配置时一律答 403。筛选维：权威区间册 `open`，暂停册 `resumed`。
- 进程内读口，不是 HTTP：`AuthorityCoverage` 供 `accessidentity` 的准入范围桥使用，只在环境变量 `IDP_PARCEL_GOVERNANCE_SELF_AUTHORITY` 设了时接上；`parcel-shipment` 的生产归属适配器读区间册、暂停册与接管册。

## 有代码、但没有生产入口的

四个用例处理器与 `AuthorityCoverage` 都有 `cmd/` 下的非测试装配，没有发现悬空的用例。下面几项是写得进、但没有下游的：

- **阶段评审决定与候选版本组没有读方。** 除记录评审时的幂等查找外，`cmd/` 下没有任何查阅口或消费方读它们；查阅口的 `register` 封闭集不含阶段评审。准入实际读的是权威区间与暂停，不是 `Go`。
- **覆盖关系与执行留痕没有查阅口。** 覆盖关系只在「还拦不拦」的查询里被连表读到；留痕表只写不读。
- **三种治理意图没有订阅方。** 意图写进 Outbox，仓内没有找到按这三个事件类型消费的代码。
- **`RecordStageReviewDeps.Clock` 与 `GovernIncidentDeps.Clock`** 被装配传入，应用层没有用到。
