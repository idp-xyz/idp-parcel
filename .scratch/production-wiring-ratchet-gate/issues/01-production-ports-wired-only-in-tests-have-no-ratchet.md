# 只在测试里接线的生产端口没有棘轮，接线退化与新增未接线口都不会红

Category: enhancement
Status: ready-for-agent

来源：T3 分区键碰撞票（[partition-key-space-collision 票 01](../../partition-key-space-collision/issues/01-tf-object-partitions-collide-with-ve-parcel-partitions.md)）
第三问取证的副产物，基线 `9e5c5c0`。本票**不解**那张票，两者判据不同。

## 形状

**测试自己补上了生产缺的那半根线。** 某个生产端口的全部构造调用点都在 `_test.go` 里：适配器
自测与集成夹具各自手工接线，用例因此永远绿——但它们证的是「若接上则可用」，不是「已接上」。
生产进程里那根线不存在，而没有任何东西会因此变红。

**已知实例五个，横跨四个上下文、三张 T3 票，成因各不相同而形状相同**——这不是巧合，是本仓当前
阶段的常态：

- **TF 两口零生产装配**（本票来源）：`NewOutboxTransportHandoverRegistrationHandoff` 与
  `NewOutboxEffectiveDeliveryHandoff` 在全仓非测试代码里零构造调用点，只出现在各自适配器自测与
  `cmd/parcel-dispatch` 的集成夹具。分区键碰撞票据此把「今天就在发作」写成了事实，实际不可达。
- **PS←PG 桥恒答未解析**（syn-wall-door-audit 票 02，MCP-5 报）：某读口没实现，`nil` 被归进
  「显式未配置」，整座桥恒答未解析，四十一条用例全绿一条照不到。
- **`SubmitDeclarationHandler` 与 `EstablishCaseHandler`**（customs-declaration-case-link 票 01）：
  同样零生产调用点，唯一构造点各在自己的应用层测试里。
- **`FormChargeAdjustment`**（supplier-expected-cost-correction 票 05）：两个调用点都是领域测试，
  且 SA 应用层根本没有形成费用调整的编排。

**五个实例按族归位，这张表本身就是判据的取证**——三个上下文、三个族、同一形状：

| 实例 | 族 | 断在哪一跳 |
|---|---|---|
| `NewOutboxTransportHandoverRegistrationHandoff` / `NewOutboxEffectiveDeliveryHandoff` | 交接口 | 无 outbox 装配 |
| PS←PG 桥读口 | 端口实现 | 读口未实现，`nil` 折成「显式未配置」 |
| `SubmitDeclarationHandler` / `EstablishCaseHandler` | 应用层处理器 | 无进程入口 |
| `FormChargeAdjustment` | 领域工厂 | 连应用层调用方都没有 |

**只有第一行落在 `NewOutbox*Handoff` 里**——本票初稿的范围会漏掉其余五分之四。判据据此放宽为
三族，见下节。

## 现状：普查数字（对 `d5e5d20`，判据已放宽为三族）

判据：**某个生产端口、命令处理器或领域工厂的构造函数，其全部调用点是否都在 `_test.go` 里。**
三族分开数——它们的「接线」含义不同，混成一个数字会让下一个人按错的含义去核。

**一族：outbox 交接口 `NewOutbox*Handoff`。** `internal/` 下共 **46** 个构造函数；
`cmd/parcel-dispatch/assemble.go` 是全仓唯一装配处（`cmd` 下非测试代码里 `Handoff` 只出现在这一
个文件，`cmd/parcel-api`、`cmd/parcel-commercial`、`cmd/parcel-pricing-register` 三个进程零处），
装配 5 口、6 个调用点：`nrpostgres.NewOutboxInitialRouteHandoff`、
`vepostgres.NewOutboxCustomerViewHandoff`（两处）、`vepostgres.NewOutboxProjectionHandoff`、
`pspostgres.NewOutboxFinalOutcomeHandoff`、`pspostgres.NewOutboxNetworkIntakeHandoff`。
**41 个零生产调用点。**

**二族：应用层命令处理器 `New*Handler`。** `internal/*/application/` 下共 **62** 个构造函数；
`internal/` 非测试代码里零调用点（全部命中都是声明本身），生产调用点全在 `cmd/`，共 9 个：
`cmd/parcel-pricing-register/main.go` 两个（价卡、参考序列登记），
`cmd/parcel-commercial/main.go` 一个（商业授权发布），
`cmd/parcel-dispatch/assemble.go` 六个（初始路由、改路重评、客户视图派生、投影派生、终局形成、
收寄采用）。**53 个零生产调用点。**

**三族：领域工厂**（`internal/*/domain/` 下的
`Form*` / `Establish*` / `Fix*` / `Judge*` / `Grant*` / `Cut*` / `Publish*` / `Accept*` /
`Propose*` / `Verify*` / `Open*` / `Record*`）。共 **89** 个构造函数，**13 个零非测试调用点**：
`EstablishCase`、`EstablishSegmentWithHandover`、`EstablishSegmentWithPickup`、`FormAuditedPayable`、
`FormChargeAdjustment`、`FormDutyCollaboration`、`FormLoadAssignment`、`FormSupplierCreditNote`、
`FormSupplierExpectedCost`、`OpenDispatchTask`、`PublishChannelAccountUseAuthorization`、
`RecordMovementFact`、`VerifyDutyPayment`。

| 族 | 构造函数 | 零非测试调用点 |
|---|---|---|
| outbox 交接口 `NewOutbox*Handoff` | 46 | 41 |
| 应用层命令处理器 `New*Handler` | 62 | 53 |
| 领域工厂 `Form*` 等 | 89 | 13 |
| **合计** | **197** | **107** |

**这 107 个不是 107 个缺陷。** 绝大多数是 SYN-WALL-DOOR-AUDIT 十八墙里还没建门的口，属**缺席**
（门还没建）而非**在场且错**。这条界线要写进门禁注释，否则下一个人会把清单长度当成待修工量。

### 三族的数字为什么不能横向比——判据是逐跳的，不是传递的

领域工厂只有 13/89 落网，看起来这一族「基本都接上了」。**不是。** 领域工厂的调用方是应用层命令
处理器，而那一族有 53/62 自己没有进程入口。**一个被未接线处理器调用的领域工厂，在本判据下算
「已接线」**——它确实有非测试调用点，只是那个调用点自己到不了任何进程。

`FormChargeAdjustment` 之所以落网，是因为它连应用层调用方都没有（SA 应用层根本没有形成费用调整
的编排），断在更靠前的一跳。

**这一格必须写进门禁注释，不许略过**：本门禁答的是「**这一跳**有没有非测试调用点」，不是「这个
东西从某个进程可达」。后者要做可达性分析，不在本门禁范围内，也不必——三族分层扫，每一跳各自
有清单，把三张清单读在一起就看得出断在哪一跳。**但只读其中一张会得出错的结论**，所以三族的数字
并排放，不做加总解读。

**普查中撞到一个正面样本，值得写进清单口径**：`cmd/parcel-api` 的提交端点并非「忘了接」——
它有一个名为 `unwired_orchestration.go` 的文件，`endpoints.go` 显式装
`shipmenthttp.UnconfiguredIntake{}` 与 `unwiredSubmission{}`，后者返回 `errOrchestrationNotWired`。
**这正是清单该有的样子：未接线被显式命名并可读。** 差别只在它今天靠人写注释表达，没有任何机制
保证下一个未接线口也这么诚实——那就是本门禁要补的一格。

## 要建的

一条纯句法门禁，形状**部分**照抄 `internal/architecture/envelope_partition_gate_test.go` 的
`allowedSameExpression`：中心清单 + 可复核的判据前缀 + 一条证明门禁真能红的自测。

**「只许变短」那一条不抄**——理由见下节，本仓的清单会合法地变长，抄过来会把门禁变成盖章机。
换成三条方向性约束（已接线不得退回、接上要删行、新增要同笔添行写理由）。

判据：**某个生产端口、命令处理器或领域工厂的构造函数，其全部调用点是否都在 `_test.go` 里。**

**能力上现成，不需要类型信息**：`parseRepositorySources` 已经在遍历全仓 `.go` 并**跳过
`_test.go`**——那正好是这条门禁要的那一半，非测试调用点是否存在只需 AST。

## 它同时是一张迁移窗口清单——这一层比棘轮本身更值

**零生产调用点等价于「这个口的载荷、键与签名今天还能免费改」。** 接线之后再改，在途信封与已存
记录就要多一套兼容期。这条不是推论，本轮已经踩到一个具体实例：

> customs-declaration-case-link 票 01 若把案件维加进申报提交载荷并设为必填，**必须赶在提交口接上
> 生产装配之前做**——下游 VE 消费方已经接线，且 `decodeFormedDeclarationSubmission` 把任一键维
> 缺席判为毒丸。今天生产上一封在途信封都没有，迁移窗口免费；接线之后旧信封集体变毒丸。

所以本门禁的清单有两重读法：**向后读是进度（哪些门还没建），向前读是窗口（哪些改动现在还免费）。**
第二重读法有时效性，而今天没有任何东西在跟踪它——每接一根线，就有一批「免费改动」静默到期。

**这一维不必单设**：下节第 3 条已经要求每行写明「哪张票会接它」，而那句话本身就回答了「谁的窗口
会在那时关掉」。对已知有在途改造提案的口（如申报提交口）把窗口写实即可。

## 为什么它不是一张「先放着」清单——棘轮钉在方向上，不钉在长度上

第一天 107 行例外，看起来正是 `envelope_partition_gate_test.go` 点名否掉的那种清单。**换个叫法
救不了它**：叫「进度盘点」而它仍然永远不会红，那就是同一份文件里那句「一个从来不会失败的门禁
比没有门禁更坑人，因为它还会取信」。

### 「只许变短」在本仓不成立——这一条已被否掉，不要照抄现有门禁

`allowedSameExpression` 的「只许变短」能成立，是因为它那份清单上的每一行都是**缺陷**，缺陷只会
越修越少。本清单不是：**本仓按 ADR-0016 就是机制半边先写、接线在后**，所以每写一个新的未接线
处理器都会**合法地**让清单变长。

一条「只许变短」的门禁在这种仓里只有两种下场——**拦住正常开发，或者被人顺手添一行绕过**。后者
几乎必然发生，而那一刻它就变成盖章机：**又一个从来不会失败的门禁**，绕一圈回到本节开头那个坑。

### 改成三条，钉方向

1. **已接线的不得退回未接线** → 红。这是真棘轮，不可逆，且与正常开发不冲突。**今天完全无人守**：
   `assemble.go` 删掉一行接线，全仓依旧全绿。
2. **接上一口却忘了从清单删** → 红。保清单与现实一致，与现有门禁后半段同一机制。
3. **新增未接线口允许，但必须同笔添行并写明理由** → 红只在「加了口不加行」时触发。
   变长不禁止，**但不得静默**。理由写：哪张票会接它，或为何暂不接。

三条合起来仍然全是真红，且不会因为仓里 87% 未接线就失效。

**第 3 条顺带兼掉了迁移窗口那一层**：每行既然要写「谁会接它」，就自然带上了「谁的窗口会在那时
关掉」——不必再单设一维（见上一节）。

十八墙重核变成派生的（今天由人工每轮重核，如 `ee58c1a` / `d6453a7` 那一轮）是**附赠收益，不是
建它的理由**。按方向建，不按长度建，也不按盘点建。

### 目标形态：仓内已有一个叫得出名字的正确写法

一条门禁只说「什么是错的」很难落地，得有一个现成的正确样本才知道往哪儿收。本门禁的目标形态就是
`cmd/parcel-api` 那一处：**未接线被显式命名且可读**——`unwired_orchestration.go` 这个文件名、
`endpoints.go` 里显式装的 `shipmenthttp.UnconfiguredIntake{}` 与 `unwiredSubmission{}`、以及
`errOrchestrationNotWired` 这个错误名，三者合起来让「这里没接」成为读得出来的事实。

**门禁补的正是那一格：让下一个未接线口也这么诚实。** 今天这份诚实全靠写它的人自觉，没有任何机制
要求第二个人照做。

## 边界

- **本票不接任何一根线。** 只建门禁与清单；把某口从缺席改成在场是各自那张墙票的事。
- 清单每行按 `allowedSameExpression` 的口径带一句**可复核的判据**（这一口在等哪张票 / 哪堵墙，
  或为何暂不接），不写「无害」这类复核不了的结论。这既是判据也是第 3 条方向约束的载体。
- 不改 `cmd/parcel-dispatch/assemble.go`。
- **扫描范围三族**：outbox 交接口（`NewOutbox*Handoff`）、应用层命令处理器
  （`internal/*/application/` 的 `New*Handler`）、领域工厂（`internal/*/domain/` 的 `Form*` 等）。
  初稿只写了第一族，而五个已知实例里有四个落在另外两族——**限在一族会漏掉五分之四**。
  **可读性不构成限范围的理由**：清单长不是问题，清单遗漏才是。一条只盖五分之一的棘轮会让人以为
  其余已经守住，那正是 `envelope_partition_gate_test.go` 那句话的同一个失效模式。
- 清单按族分段，三族各自计数。**不要把三族混成一个数字**：它们的「接线」含义不同（交接口接的是
  outbox 装配，处理器接的是进程入口，工厂接的是处理器），且判据逐跳不传递（见上节）。**没有这句
  话，下一个人会把三个数加起来当一个 KPI 看**，而那个和没有任何业务含义。

## 参照

`internal/architecture/envelope_partition_gate_test.go`（`allowedSameExpression`、
`TestEveryExceptionCarriesACheckableVerdict`、`TestTheEnvelopePartitionGateCanActuallyCatchAViolation`）；
`internal/architecture/rehydration_gate_test.go` 的 `parseRepositorySources`；
`.scratch/syn-wall-door-audit/report.md` 十八墙清单。
