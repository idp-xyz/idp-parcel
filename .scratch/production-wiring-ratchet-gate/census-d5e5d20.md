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

> **初版这里写 89，错了，已更正为 72（见文末「一次已发生的口径错误」）。** 零调用点那一半
> 未受影响，仍是 13，且名单一字未变。

**13 个零非测试调用点**：`EstablishCase`、`EstablishSegmentWithHandover`、
`EstablishSegmentWithPickup`、`FormAuditedPayable`、`FormChargeAdjustment`、`FormDutyCollaboration`、
`FormLoadAssignment`、`FormSupplierCreditNote`、`FormSupplierExpectedCost`、`OpenDispatchTask`、
`PublishChannelAccountUseAuthorization`、`RecordMovementFact`、`VerifyDutyPayment`。

## 四族：ports 适配器构造函数

**识别口径（这一族靠断言认，不靠命名认）**：`internal/*/adapters/**` 下**含编译期接口断言**
`^var _ <pkg>ports.<Iface> = ` 的文件里的 `^func New*(` 构造函数，**扣除已计入一族的
`NewOutbox*`**。用断言而不是名字，是因为适配器的命名没有统一前缀（`NewReadinessView`、
`NewCaseIdentities`、`NewProductionOwnershipAdapter` 毫无共同词根），**而「它实现了某个 ports
接口」这件事在本仓恰好有一个句法标记**。

符合的适配器文件 **97** 个，其中构造函数（扣除 `NewOutbox*`）共 **66** 个，
**43 个零非测试调用点**：

`NewAcceptanceContentDeclarations`、`NewAcceptanceDecisions`、`NewAcceptanceRulePackages`、
`NewActiveRejectionAdapter`、`NewAllocationRuleApplicability`、`NewAsOfPolicyDeclarations`、
`NewAuthorityGrants`、`NewCaseIdentities`、`NewCaseRequirementView`、
`NewChargeConfirmationConditions`、`NewClaimEligibilityRules`、`NewCommercialAuthority`、
`NewCommercialBasisAdapter`、`NewCommercialEligibility`、`NewCreditStandings`、
`NewCustomerContractContents`、`NewDeclarationVersions`、`NewDeliveryAttempts`、`NewETAVersions`、
`NewExceptionCases`、`NewExecutionFactView`、`NewGateConditionRegistrations`、
`NewGateConditionView`、`NewIntakeResultVersions`、`NewInterpretationRuleRegistrations`、
`NewInterpretationRuleView`、`NewManifestCandidateView`、`NewNotificationPolicies`、
`NewNotifications`、`NewObligationInventoryRegistrations`、`NewObligationInventoryView`、
`NewOperationalBalances`、`NewPreAcceptanceControlDeclarations`、**`NewProductionOwnershipAdapter`**、
`NewReadinessRegistrations`、`NewReadinessView`、`NewRecoveryMatters`、
`NewSubmissionAuthorityRegistrations`、`NewSubmissionAuthorityView`、`NewSubmissionIdentities`、
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
「写了接口断言的适配器里，有 43 个零调用点」，**不能读作「适配器里只有 43 个零调用点」**。

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
| ports 适配器 `New*`（按接口断言认） | 66 | 43 |
| 合计 | 246 | 150 |

**那个 246/150 被当成 KPI 就完了**，它没有业务含义。四族并排看，不相加。

四族数字均以 **git 侧匹配、大小写敏感**在 `d5e5d20` 上得出（`git grep <pattern> d5e5d20`）；
前三族另在当时 HEAD 上重跑过一遍，与基线一致——**期间无漂移**。

## 这 107 个不是 107 个缺陷

绝大多数是 SYN-WALL-DOOR-AUDIT 十八墙里还没建门的口，属**缺席**（门还没建）而非**在场且错**。
这条界线要写进门禁注释，否则下一个人会把清单长度当成待修工量。

## 三族的数字为什么不能横向比——判据是逐跳的，不是传递的

领域工厂只有 13/89 落网，看起来这一族「基本都接上了」。**不是。** 领域工厂的调用方是应用层命令
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
  `:(exclude)internal/**/*_test.go` 与 `:(exclude)cmd/**/*_test.go` 排掉测试；命中数 ≤ 1 即零调用点
  （那一次命中是声明本身）。
- 三族的前缀集是**判断**不是穷举：领域工厂那一族的十二个前缀取自本仓现有命名，新前缀出现时要补。
  **口径差异优先怀疑这里**，其次怀疑大小写，最后才怀疑数数。

## 一次已发生的口径错误：初版领域工厂总数 89 是错的，实为 72

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
