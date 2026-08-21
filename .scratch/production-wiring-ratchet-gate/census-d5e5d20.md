# 三族普查数字（基线 `d5e5d20`）

> ## ⛔ 实现方：完成自己那一份普查之前不要打开本文件
>
> 本文件是[票 01](./issues/01-production-ports-wired-only-in-tests-have-no-ratchet.md) 的普查结果，
> 由 MCP-2 一人得出。票 01 的第一条方向约束**不可逆**，初始清单错一行就会一直锁在里面——
> **一个把错误吸收进基线的门禁永远不会红**，而那正是票 01 要治的病本身。
>
> 所以它被从票面拆出来单独存放：**你可以整篇读票，不必记行号、不必自律**。
> 顺序见票 01 的「开工条件」一节。读本文件是第 4 步，不是第 1 步。
>
> 拆成两份文件而不是在票里写一句「读到这里请停」，理由与票 01 通篇一致：
> **能用结构守的就不要留给纪律守**——「先读判据后读数」做到与没做到的产物长得一模一样。

判据：**某个生产端口、命令处理器或领域工厂的构造函数，其全部调用点是否都在 `_test.go` 里。**

三族分开数，**不做加总解读**——它们的「接线」含义不同（交接口接的是 outbox 装配，处理器接的是
进程入口，工厂接的是处理器），混成一个数字会让下一个人按错的含义去核。

## 一族：outbox 交接口 `NewOutbox*Handoff`

`internal/` 下共 **46** 个构造函数。

`cmd/parcel-dispatch/assemble.go` 是全仓唯一装配处——`cmd` 下非测试代码里 `Handoff` 只出现在这
一个文件，`cmd/parcel-api`、`cmd/parcel-commercial`、`cmd/parcel-pricing-register` 三个进程零处。
装配 5 口、6 个调用点：

- `nrpostgres.NewOutboxInitialRouteHandoff`
- `vepostgres.NewOutboxCustomerViewHandoff`（两处）
- `vepostgres.NewOutboxProjectionHandoff`
- `pspostgres.NewOutboxFinalOutcomeHandoff`
- `pspostgres.NewOutboxNetworkIntakeHandoff`

**41 个零生产调用点。**

## 二族：应用层命令处理器 `New*Handler`

`internal/*/application/` 下共 **62** 个构造函数。`internal/` 非测试代码里零调用点（全部命中都是
声明本身），生产调用点全在 `cmd/`，共 9 个：

- `cmd/parcel-pricing-register/main.go` 两个（价卡登记、参考序列登记）
- `cmd/parcel-commercial/main.go` 一个（商业授权发布）
- `cmd/parcel-dispatch/assemble.go` 六个（初始路由、改路重评、客户视图派生、投影派生、终局形成、
  收寄采用）

**53 个零生产调用点。**

## 三族：领域工厂

`internal/*/domain/` 下的 `Form*` / `Establish*` / `Fix*` / `Judge*` / `Grant*` / `Cut*` /
`Publish*` / `Accept*` / `Propose*` / `Verify*` / `Open*` / `Record*`，共 **72** 个构造函数。

> **初版这里写 89，错了，已更正为 72（见文末「第一次已发生的口径错误」）。** 零调用点那一半
> 未受影响，仍是 13，且名单一字未变。

**13 个零非测试调用点**：`EstablishCase`、`EstablishSegmentWithHandover`、
`EstablishSegmentWithPickup`、`FormAuditedPayable`、`FormChargeAdjustment`、`FormDutyCollaboration`、
`FormLoadAssignment`、`FormSupplierCreditNote`、`FormSupplierExpectedCost`、`OpenDispatchTask`、
`PublishChannelAccountUseAuthorization`、`RecordMovementFact`、`VerifyDutyPayment`。

### ⚠ 这个 13 是下界：前缀集实测不完备，而盲区里有货

**本族的 72 已由 MCP-3 用同一前缀集机械重数、逐位一致**（抓得住转写错与工具错，抓不住口径
错）。但前缀集本身是判断不是穷举，而它挡在外面的东西实测如下（同基线，`internal/*/domain/`
非测试，去重导出顶层函数）：

```
导出顶层函数        732
十二个前缀盖到       72
未盖到              660
  其中 New*         528   （值对象/ID 构造，多半不属本族）
  非 New*           132   ← 关切面
```

**那 132 个按动词分组后与已收前缀分不出道理**：`Judge*` 在册而 `Decide*` / `Conclude*` /
`Assess*` 不在；`Accept*` 在册而 `Adopt*` / `Receive*` 不在；`Record*` 在册而 `Declare*` 不在；
`Form*` / `Establish*` 在册而 `Derive*` / `Rehydrate*` 不在——**光 `Rehydrate*` 就 43 个，而
整族才 72。**

**盲区里确有本票要棘的那一种。** 用本文更正后的调用点判法打在那 132 个上，**20 个零非测试
调用点**，其中 18 个在测试里有调用——也就是「只被测试调用的生产领域函数」。已逐个复核四例：

| 名字 | 位置 | 非测试真调用 | 测试中 |
|---|---|---|---|
| `AssessSafeHandoff` | `parcelshipment/domain/production_handoff.go` | 0 | 9 |
| `DecideDisclosure` | `visibilityexception/domain/customer_disclosure.go` | 0 | 6 |
| `RehydratePricingPlanSnapshot` | `parcelpricing/domain/plan_snapshot.go` | 0 | 3 |
| `SubmitEvidence` | `visibilityexception/domain/evidence.go` | 0 | 1 |

> **哪些算领域工厂是口径判断，本文不替票主与人类定。** `DecimalFromInt64`、`ParseCanonical`、
> `MarshalPricingPlanSnapshot` 看着像值/解析助手；`Rehydrate*` 算不算工厂是个真问题。
> **所以 20 是候选数不是缺陷数，13 是下界不是结论。** 盲区形状由 MCP-3 跑出（初报 11），本文
> 这一跑得 **20**；差异已查清并经 MCP-3 复核认下：它那次把 **Go 文档注释当成了调用点**——
> **文档注释按惯例以函数名开头，于是几乎每个写了注释的导出函数都受影响，而那恰是最可能为真
> 领域入口的一批**——并叠上一次中文行经 PowerShell 合并。**20 作数。**
>
> 本文这一跑要求名字后紧跟 `(`，因此不吃那种注释；但同基线 `internal/*/domain/` 下另有 7 行
> 注释里带 `名字(` 的写法，**所以 20 自己也是下界**。

### 二十个已逐个读完：三族零调用点应为 **≥ 26**，正好翻倍

由 MCP-3 逐个读、按返回形态三分，本文复核其中四例的签名与全部十三例的上下文归属：

**甲、构造领域对象并带不变式校验（`(领域类型, error)`）——13 个，确属本族：**
`AssessSafeHandoff`、`ReplayPricingEvaluation`、`ResolveCreditPolicy`、`DecideDisclosure`、
`ResolveByBusinessTime`、`ReceiveReleaseOutcome`、`SummarizeHandovers`、
`IncludeAdjustmentInSubsequentPeriod`、`PrepareDisclosure`、`ChargeOccurrenceForFailedAttempt`、
`RaiseConflictSignal`、`RegisterCredential`、`SubmitEvidence`。
**横跨七个上下文**（`customscompliance`、`parcelpricing`、`parcelshipment`、`partycommercial`、
`settlementaccounting`、`transportfulfillment`、`visibilityexception`）——**不是某一处的局部异常。**

**乙、不属本族——4 个**：`ManualReviewRequirementFor`（返回 `bool`，谓词）、
`MarshalPricingPlanSnapshot`（序列化）、`ValidateBeforeDecision`（同型变换，不构造新对象）、
`DecimalFromInt64`（值助手）。

**丙、需票主定族界——3 个**：`RehydratePricingPlanSnapshot`（**`Rehydrate*` 算不算工厂，那一族
43 个**）、`ParseCanonical`（解析助手，造的是值对象）、`Evaluate`（见下）。

**所以本族零调用点 = 13 + 甲类 13 = 26，仍是下界**（丙类未定，7 行注释写法未排，只扫了导出顶层
函数）。

> **`Evaluate` 是另一类东西，判据把它和「只被测试接线」混在了一起。** 它是一行转发壳
> （`internal/parcelpricing/domain/evaluation.go`，`return EvaluatePricing(request)`），
> **非测试 0、测试也 0——全仓一个调用点都没有**。按本文的操作规则它照样落进零调用点，于是
> **门禁会把它报成「未接线的生产端口」，而它其实是死代码**：该做的是删掉，不是接线。
> **两者要人做的事相反，而在判据下长着同一张脸。**

**这一格对门禁实现直接有话说**：让门禁自己当场算初始清单、不从本文抄，**拦不住这一类**——
门禁算的时候用的还是这十二个前缀。**前缀集是写进门禁里的假设，不是它每次重算的输入**，得单独
守（例如把「本族前缀集之外的导出领域函数」也纳入门禁视野，或至少让新增前缀触发一次复核）。

## 四族：ports 适配器构造函数

**识别口径（这一族靠断言认，不靠命名认）**：`internal/*/adapters/**` 下**含编译期接口断言**
`^var _ <pkg>ports.<Iface> = ` 的文件里的 `^func New*(` 构造函数，**扣除已计入一族的
`NewOutbox*`**。用断言而不是名字，是因为适配器的命名没有统一前缀（`NewReadinessView`、
`NewCaseIdentities`、`NewProductionOwnershipAdapter` 毫无共同词根），**而「它实现了某个 ports
接口」这件事在本仓恰好有一个句法标记**。

符合的适配器文件 **97** 个，其中构造函数（扣除 `NewOutbox*`）共 **66** 个，
**46 个零非测试调用点**：

> **初版这里写 43，漏了三个，已更正为 46（见文末「第二次已发生的口径错误」）。** 漏因不在本族的
> 识别口径——97 与 66 两个数一字未动，错的是四族共用的**调用点排除法**。

`NewAcceptanceContentDeclarations`、`NewAcceptanceDecisions`、`NewAcceptanceRulePackages`、
`NewActiveRejectionAdapter`、`NewAllocationRuleApplicability`、`NewAsOfPolicyDeclarations`、
`NewAuthorityGrants`、`NewCaseIdentities`、`NewCaseRequirementView`、
`NewChargeConfirmationConditions`、`NewClaimEligibilityRules`、`NewCommercialAuthority`、
`NewCommercialBasisAdapter`、`NewCommercialEligibility`、`NewCreditStandings`、
`NewCustomerContractContents`、`NewDeclarationVersions`、`NewDeliveryAttempts`、
**`NewDispositionRequests`**、`NewETAVersions`、`NewExceptionCases`、`NewExecutionFactView`、
`NewGateConditionRegistrations`、
`NewGateConditionView`、`NewIntakeResultVersions`、`NewInterpretationRuleRegistrations`、
`NewInterpretationRuleView`、`NewManifestCandidateView`、`NewNotificationPolicies`、
`NewNotifications`、`NewObligationInventoryRegistrations`、`NewObligationInventoryView`、
`NewOperationalBalances`、`NewPreAcceptanceControlDeclarations`、**`NewProductionOwnershipAdapter`**、
`NewReadinessRegistrations`、`NewReadinessView`、`NewRecoveryMatters`、**`NewSignalEpisodes`**、
**`NewSourceDataVersions`**、`NewSubmissionAuthorityRegistrations`、`NewSubmissionAuthorityView`、
`NewSubmissionIdentities`、
`NewSubmissionIndex`、`NewSupplierExpectedCosts`、`NewTriageRules`。

> `NewProductionOwnershipAdapter` 就是逼出这一族的那个实例（见票 01「形状」一节取证表第二行，
> 及 [syn-wall-door-audit 票 13](../syn-wall-door-audit/issues/13-production-ownership-bridge-has-no-assembly-point.md)）。
> **它在这份基线上就已经是零调用点**——不是后来才变成的。所以那不是「断言过期」，是**扫描面
> 一直盖不到它**；两者对实现方的含义不同：前者重跑一遍就行，后者重跑多少遍都看不见。

### ⚠ 四族这个数是**下界**，不是适配器总数

**本族只看得见「自己写了接口断言」的适配器。一个实现了 ports 接口却没写 `var _` 的适配器，
对本族是隐形的。** 这一句必须声明，不能假定。实测（同基线）：

```
internal/*/adapters/**   非测试文件           223
其中带 ports 接口断言（本族识别口径）          97
无断言                                       126
无断言、但含 ^func New* 的                    110
```

**那 110 是盲区的上界，不是盲区本身**——其中大多数应是同包内断言写在别的文件、或本就不是端口
实现的辅助构造（行编解码、端点装配等）。**但「大多数」是推测，不是取证**：本族计数因此只能读作
「写了接口断言的适配器里，有 46 个零调用点」，**不能读作「适配器里只有 46 个零调用点」**。

要把下界收成实数，得换识别方式（例如用 `go/types` 逐个判类型是否满足某个 `ports` 接口）——那已
超出纯句法，成本与本票「不需要类型信息」的前提冲突。**留作已知缺口，不在本票内解决。**

> **97 与 98 的那一个差**：用宽模式 `^var _ .*ports\.` 会多出
> `internal/visibilityexception/adapters/http/query_customer_tracking_view.go`，其内容是
> `var _ TrackingViewReader = ports.CustomerViewStore(nil)`——**反方向断言**（断的是 ports 类型
> 满足本地窄接口，不是本地类型实现 ports 接口）。按本族口径应当排除，**97 是对的**。

## 汇总（只是表尾，不是结论）

| 族 | 构造函数 | 零非测试调用点 |
|---|---|---|
| outbox 交接口 `NewOutbox*Handoff` | 46 | 41 |
| 应用层命令处理器 `New*Handler` | 62 | 53 |
| 领域工厂 `Form*` 等 | 72 | 13 |
| ports 适配器 `New*`（按接口断言认） | 66 | 46 |
| 合计 | 246 | 153 |

**那个 246/153 被当成 KPI 就完了**，它没有业务含义。四族并排看，不相加。

四族数字均以 **git 侧匹配、大小写敏感**在 `d5e5d20` 上得出（`git grep <pattern> d5e5d20`）；
前三族另在当时 HEAD 上重跑过一遍，与基线一致——**期间无漂移**。

## 这 153 个不是 153 个缺陷

绝大多数是 SYN-WALL-DOOR-AUDIT 十八墙里还没建门的口，属**缺席**（门还没建）而非**在场且错**。
这条界线要写进门禁注释，否则下一个人会把清单长度当成待修工量。

## 三族的数字为什么不能横向比——判据是逐跳的，不是传递的

领域工厂只有 13/72 落网，看起来这一族「基本都接上了」。**不是。** 领域工厂的调用方是应用层命令
处理器，而那一族有 53/62 自己没有进程入口。**一个被未接线处理器调用的领域工厂，在本判据下算
「已接线」**——它确实有非测试调用点，只是那个调用点自己到不了任何进程。

`FormChargeAdjustment` 之所以落网，是因为它连应用层调用方都没有（SA 应用层根本没有形成费用调整
的编排），断在更靠前的一跳。

## 取证方法（供重跑者对照口径，读到这里说明你已经跑完自己那份）

- **让 git 自己匹配、自己数**：`git grep -h -o -E '<pattern>' <基线SHA> -- <pathspec>`。不要把源文件
  交给 PowerShell 去读——理由见下节，那是一次真实的翻车。
- 构造函数声明：一至三族按 `^func <前缀>[A-Za-z0-9_]*\(` 在 `internal/` 上取，各自前缀集见上；
  **四族不按前缀取**——它按编译期接口断言认文件，再取那些文件里的 `New*`，口径见四族那一节。
- 调用点：在 `internal/` 与 `cmd/` 上找 `\b<名字>\(`，用
  `:(exclude)internal/**/*_test.go` 与 `:(exclude)cmd/**/*_test.go` 排掉测试；**扣除该名字的全部
  `^func <名字>(` 声明行之后仍为空**，才算零调用点。
  **不要用「命中数 ≤ 1」判。** 那条写法假定一个名字全仓只有一处声明，而**名字跨包可重**：第二处
  声明会被读成一次调用，于是真正的零调用点被判成已接线。初版即因此漏计三个（见文末第二节）。
  **一个构造函数要「包 + 名」两者才定得住，名字不是唯一键**——这一条同样是门禁实现的约束，不只是
  取证的约束。
- 三族的前缀集是**判断**不是穷举：领域工厂那一族的十二个前缀取自本仓现有命名，新前缀出现时要补。
- **口径差异的排查顺序**：初版在这里写过「优先怀疑前缀集，其次大小写，最后才怀疑数数」，
  **那个顺序已被两次真实差异证伪，勿再照用**。两次的成因分别是**声明侧的工具默认**（大小写，89→72）
  与**调用点排除法**（跨包重名，43→46），**前缀集至今一次都没错过**——把它排在第一位是初版的直觉，
  不是取证得来的。按已发生的事排：先查调用点排除法，再查工具的大小写默认，再查本族识别口径
  （前三族看前缀集、四族看断言模式），最后才疑数数。

## 第一次已发生的口径错误：初版领域工厂总数 89 是错的，实为 72

**成因：PowerShell 的 `Select-String` 默认大小写不敏感。** 初版这一族用它扫，于是
`^func (Form|Establish|…|Accept|…)` 把 **17 个未导出函数**一并算了进来——`Accept` 命中
`acceptanceDigest` / `acceptanceFrom` / `acceptedResolutionOf`，`Form` 命中 `formUnit` /
`formAttempt`，`Judge` 命中 `judgeContractScope`，等等。

**只有分母错，分子没错**：零调用点仍是 13，名单一字未变——那 13 个全是导出名、大小写本就对得上，
且已用 git 侧大小写敏感逐个复验（每个恰好 1 次非测试出现）。另两族原本走的是 ripgrep（默认大小写
敏感），复验后 46/41 与 62/53 均不变。

**分离过两个原因**：在 `d5e5d20` 上重验与在当时 HEAD 上重跑结果一致，所以这 17 个不是期间新增的
代码，纯粹是工具口径错。

**留下这一节而不是静默改数**，理由与本票通篇一致：**一个把错误吸收进基线的门禁永远不会红。**
双盲重跑的价值恰恰在于抓出这一类，而这次是我自己先抓到——把成因写明，重跑者对不上账时才知道
该往哪儿看。它同时是一个现成的反面样本：**默认值不出声，而「默认大小写不敏感」这件事没有任何
东西会提醒你。**

## 第二次已发生的口径错误：第四族零调用点初版写 43，实为 46

由 MCP-1 的重导抓出（记在票 01 Comments）。漏掉的三个是 `NewDispositionRequests`、
`NewSignalEpisodes`、`NewSourceDataVersions`；MCP-1 的集合是初版那 43 的严格超集。

**成因是调用点排除法，不是本族识别口径。** 这三个名字**在两个包里各声明一次**（一处在
`internal/<ctx>/adapters/identity/identity.go`，一处在 `internal/<ctx>/adapters/postgres/` 的对应
文件），真调用 0 处。初版按「命中数 ≤ 1」判零，于是第二处**声明**被当成一次调用，三个零调用点被
误判为已接线。识别口径那一半一字未错——97 个断言文件、66 个构造函数两个数重跑后完全一致。

**这一条与门禁实现直接相关**：门禁必须按包+名定位并扣除该名的**全部**声明。若门禁沿用初版那条
规则，它算出的仍是 43，与基线 43 相等因而**永远绿**——这三个会被结构性地永久隐形，正是本票所治
之病；若门禁算得对（46）而基线停在 43，它又会立刻**假红**三条，把人调去改三个本来就该是那样的
东西。两种坏法都由同一处错误产生，所以基线与门禁必须同时改。

### 另三族已实测免疫，不是推定

MCP-1 未重跑三族（领域工厂），理由正当：自拟前缀集会让差异分不清是口径还是数错。但
**这一族是否会犯同一个错，可以不重导计数就答**——问的不是「有几个」，而是「本族名字里有没有
跨包重名」，用的仍是本文件自己那套前缀集，因此不引入第二套口径。

同基线实测：把非测试 `.go` 的 `^func <名字>(` 或 `^func <名字>[` 建成索引（**1883** 条顶层声明），
四族名单逐一对查——

```
一族 outbox 交接口   46 个名字   多处声明的: 0
二族 应用层处理器     62 个名字   多处声明的: 0
三族 领域工厂         72 个名字   多处声明的: 0
四族 ports 适配器     66 个名字   多处声明的: 3   ← 即上述三个
```

所以 **41 / 53 / 13 三个数不受本次错误影响，且这是量出来的，不是「因为对上了所以没事」**；四族的
**至多只有那三个**会翻，MCP-1 那 46 是完整的，不存在第四个待发现。

> **这个索引初版建窄了两处，结论未变但描述错了，一并记下。** 初版写「全仓非测试 `.go`……1864 条」，
> 实际只扫了 `internal/` 与 `cmd/`（漏 `migrations/migrations.go` 的 13 条），且模式 `^func <名字>(`
> **看不见泛型声明** `func <名字>[T any](`（本仓 6 条）。放全后是 1883 条，去重 1804 个名字、
> 47 个名字有多处声明、多余声明 79 条。**四族对查的结果一格未变**（0/0/0/3，仍是同样那三个名字）。
>
> 由 MCP-1 追问「1864 复现不出来」抓出，它报的 1877（全仓不含泛型）与 1883（含泛型）都对。
> **值得记的不是那个数，是「我描述的范围比我实际量的宽」**——而这个索引的**唯一用处**就是当完整
> 参照系，它窄一格，「另三族一个重名都没有」就少一格依据。数错了会被人重跑撞上，**范围写宽了不会**。
>
> 泛型那一格 MCP-1 的直觉对了一半：全仓确实有一个泛型声明造成重名——`New`
> （`internal/platform/httpapi/router.go` 与 `internal/platform/inboxconsume/consume.go`）——
> 只是它落在四族之外，够不着本普查。**够不着不等于没有，下一个建索引的人仍要把 `[` 那一格算上。**

### 顺带记一次自己重犯

查这件事时我第一版探针**只在本族那 97 个文件里找重名，得「无」**——而那三个的第二处声明恰恰在
族外文件里。**这跟原错误是同一个形状：把「排除范围」缩到手边那一小片。** 我没看出来，是先跑
MCP-1 点名的那三个当阳性对照、发现对照与我的「无」直接矛盾才回头改的。
**阳性对照的用处不止于「报零命中之前」，也在「报无异常之前」。**
