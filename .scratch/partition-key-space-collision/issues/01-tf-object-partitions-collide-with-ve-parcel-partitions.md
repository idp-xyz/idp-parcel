# TF 的（租户+对象）分区与 VE 的（租户+包裹）分区是同一个键，跨上下文共链无人察觉

Category: bug
Status: ready-for-human

分区由**键值字符串**决定，不由上下文、事件类型或 handoff 决定（[票 01](../../outbox-partition-key/issues/01-per-event-partition-keys-make-the-ordering-guarantee-vacuous.md)
的判据原句）。两个上下文各自算出同一个字符串，它们的信封就进同一分区并因此互相排队——包括
互相**阻塞**。今天已有两口这样：

| 口 | PartitionKey |
|---|---|
| `transportfulfillment/.../transport_handover_registration_handoff.go` | `租户/对象`（`transportHandoverPartitionKey`） |
| `transportfulfillment/.../effective_delivery_handoff.go` | `租户/对象`（`effectiveDeliveryPartitionKey`） |
| `visibilityexception/.../projection_handoff.go` | `租户/包裹`（`Projection.Parcel()`） |
| `visibilityexception/.../triage_handoff.go`、`visibility_gap_handoff.go`、`eta_handoff.go` | 同为 `租户/包裹` |

而**载运对象引用与申报包裹标识是同一个字符串**——`cmd/parcel-dispatch` 的揽收采用链里，
`offsitePickupObject` 既用来构造 `tfdomain.NewCarriedObjectReference`，也用来构造
`psdomain.NewDeclaredParcelID` 反查已接受委托。所以 TF 那两口今天就与 VE 四口共分区。

> **末句已过时（2026-08-21 对 `9e5c5c0` 取证，见 Comments 第一节）。** 键表达确实相同，
> 但 TF 两口与 VE 四口中的三口在非测试代码里零构造调用点，装配后的进程里这 2×4 的
> 碰撞面实为 0×1。今天真正共 `租户/包裹` 的是另外三口。上面这段保留为原读法。

## 实测（2026-08-21，基线 `f47f698`）

`outbox-partition-key` 票 03 的裁断二本打算把 `offsite_pickup_registration_handoff.go` 的分区
主体也收窄到（租户+对象）。收窄后 `TestARegisteredOffsitePickupStopsAtUnprovenIntakeEligibility`
立刻红在重拍那句：

```
offsite_pickup_adoption_test.go:81: 重拍定稿 0 条, want 1（仅派生信封经视图链定稿）
```

把分区键换成（租户+对象+一个探针后缀）——仍是一对象一键，只是避开 VE 的键空间——同一用例
立刻转绿。**起因因此确切：不是揽收登记自身的顺序问题，是它并进了 VE 的包裹分区。**

后果的形状：一封停在 `dispatch.consumer_undecided` 的揽收信封会把同一包裹**已经由 VE 受理并
派生**的追踪投影一起堵在分区头，直到失败预算耗尽进 `ABANDONED` 才解冻。而「硬资格未证明」
是今天的常态（实例半边为空），不是罕见边缘。

## 为什么这一类没有任何东西在守

`internal/architecture/envelope_partition_gate_test.go` 拦的是「ID 与 `PartitionKey` 同源」这个
**句法**形状。跨上下文算出同一个键值是**语义**形状：两处代码各自合规、各自的用例全绿，只有
在两条链恰好同时有信封在途、且一条卡住时才看得见。票 03 末尾「一条门禁守不住、清单也不收的」
说的是键拼得太细那一半；这里是**同一个盲区的另一半——拼得太粗、跨上下文撞车**。

## 要答的

**第一问是本票的全部难点，下面这一节写成可以一坐下就拍的形状；取证明细在 Comments，
拍板不必先读完。**

1. **TF 的「载运对象」与 VE 的「包裹」在分区意义上是不是同一个排队主体？**

   取证收束成一句话：**身份相同不等于排队主体相同。** 载运对象在「是哪一件东西」这一维上
   按定义引用包裹身份（TF CONTEXT：「载运对象引用其来源上下文拥有的身份」）；在「哪些先后拍
   必须排一条队」这一维上却是另一个集合——GLOSSARY 的载运对象是**包裹与集运单元的并集**，而
   同一份 GLOSSARY 在「包裹」条目下显式写着「包裹不是……集运单元」。两边各自的原句、以及代码
   里那条已经成建制的立场（`NodeExecutionQualificationEvidence` 为「两侧编号偶然同名」专设的
   来源种类闸），见 Comments 第一节。

   **还要回答的第二半，不能省：那条队买到了什么。** ADR-0065 已定「投影的取代关系由来源给出」
   ——投影本就不靠到达先后排序，所以跨口保序在 VE 那一侧**买不到任何东西**。这不是支持某一边的
   第四条证据，是一个独立维度：**即使判为同一主体，共分区也是只有成本没有收益**，那就必须说清
   为什么仍要共。

   两个分支各自的代价：

   | 判 | 要做的 | 代价 |
   |---|---|---|
   | 同一主体 | 共链按特性接受，头端阻塞写进权威文档；`offsite_pickup_registration_handoff.go` 的类型段去掉，三口一起回到（租户+对象） | 要正面回答上面那句「买到了什么」；接受一封未决信封能扣住同一包裹已成立的可见性 |
   | 不同主体 | `transportHandoverPartitionKey`、`effectiveDeliveryPartitionKey` 与揽收登记口三口一起加维，形状取齐 | **改变跨上下文的保序语义，是难逆转取舍，按 AGENTS.md 要走一篇 ADR**；三口同笔改 |

2. 若判为不同主体：`transportHandoverPartitionKey` 与 `effectiveDeliveryPartitionKey` 两口
   要不要跟着加维？它们今天已经与 VE 共链，只是还没有用例把它照出来。

   > 取证补记：这两口今天在生产装配里零构造调用点，因此「已经共链」只在夹具接线上成立；
   > 今天真正共 `租户/包裹` 的是另外三口，且同样被 `ELIGIBILITY_UNDECIDED` 掩住。
   > **本问今天没有任何运行期证据能推动它**，判据只能来自第一问。详见 Comments 第二节。

3. ~~这一类要不要一条门禁。判据不是句法，可能得靠「同一 SHA 下全仓分区键表达求值后取交集」
   这类扫描，成本与收益要先估。~~ **已估完，答案是做不出来；两条替代门禁已另开两票。**

   「同一 SHA 下全仓分区键表达求值后取交集」**两层都堵死**，写在这里是为了拦下一个人：

   - **求不了值。** 分区键是运行期字符串拼接，成分是领域值对象的 `.String()`，值来自库行与
     信封载荷，静态没有值可求。
   - **退成类型层求交也不成立，而且换工具也救不了。** 现有门禁走 `go/parser` +
     `types.ExprString`，`parseRepositorySources` 只 `parser.ParseFile`，全程无类型检查。
     **就算整体换成 `go/packages` 全量类型检查，也答不出本票这个问题**——本票的碰撞正是两个
     **不同类型**承载**同一字符串**（`CarriedObjectReference` 与 `DeclaredParcelID`），类型层
     看它们是两回事。类型交集会把每一对 `租户/X` 都报出来，同时漏掉真碰撞。

   替代方案拆成两票，均不在本票里做：
   [棘轮门禁](../../production-wiring-ratchet-gate/issues/01-production-ports-wired-only-in-tests-have-no-ratchet.md)（先做）、
   [登记式分区主体声明](./02-partition-subject-registry-gate.md)（后做，不阻于第一问）。

## 已经定死的边界（不要在本票里重开）

- `offsite_pickup_registration_handoff.go` 的分区键本轮取（租户+对象+类型段），
  理由与实测见票 03 的裁断落地 Comment 与该函数注释。本票若判 1 为「同一主体」，回来把类型段
  去掉即可；判为「不同主体」，则本口已经是对的形状，改的是另外两口。
- 同一对象两次成功揽收必须保序这一条已裁定，不因本票重开（`AT-TF-098`）。

## Comments

- 2026-08-21 MCP-2（T3 裁断取证，基线 `9e5c5c0`，零代码；`.go` / `.sql` 与该 SHA 逐字节一致）：

### 〇、先更正票面一处事实：「今天就在发作」不成立于装配后的进程

`cmd/parcel-dispatch/assemble.go` 是全仓唯一装配 outbox 交接口的地方——`cmd` 下非测试代码里
`Handoff` 只出现在这一个文件，`cmd/parcel-api`、`cmd/parcel-commercial`、
`cmd/parcel-pricing-register` 三个进程零处。它装配的交接口共五个（六个调用点）：
`nrpostgres.NewOutboxInitialRouteHandoff`、`vepostgres.NewOutboxCustomerViewHandoff`（两处）、
`vepostgres.NewOutboxProjectionHandoff`、`pspostgres.NewOutboxFinalOutcomeHandoff`、
`pspostgres.NewOutboxNetworkIntakeHandoff`。

票面点名的两口不在其中：`NewOutboxTransportHandoverRegistrationHandoff` 与
`NewOutboxEffectiveDeliveryHandoff` **在全仓非测试代码里零构造调用点**，只出现在各自的适配器
自测与 `cmd/parcel-dispatch` 的集成夹具（`transport_handover_projection_test.go`、
`effective_delivery_final_test.go`、`offsite_pickup_adoption_test.go`）。票面实测所用的
`NewOutboxOffsitePickupRegistrationHandoff` 同样零生产调用点，接线由
`offsite_pickup_adoption_test.go` 的 `recordRegisteredOffsitePickup` 手工搭。

准确说法因此是：**碰撞在代码层为真，在装配后的进程里今天发作不了**——参与碰撞的 TF 那一侧
一封都不产。票面 `Category: bug` 我建议保留（这是一个已经写进代码、接线即发作的缺陷），
但「今天就在发作」这半句要改。`f47f698` 那条实测本身没错，它照出的机制是真的，只是它跑在
夹具接线上而不是生产接线上——原读法已在正文就地标过时，未抹去。

### 一、【不拍板，只给判据】TF「载运对象」与 VE「包裹」是不是同一个排队主体

**支持「不同主体」的文本，三条：**

1. GLOSSARY「载运对象」：「可以被独立分配、交接并参与运输履约的明确实物范围，**例如包裹或
   集运单元**」，并注「`transport-fulfillment` 引用载运对象身份；包裹身份仍由
   `parcel-shipment` 拥有，集运单元及成员关系仍由 `node-operations` 拥有」。载运对象是**两个
   身份空间的并集**，包裹只是其中一支——两者在集合上就不相等。
2. GLOSSARY「包裹」：「在小包网络中具有独立身份并可被分别收寄、跟踪、处理和形成服务结果的
   物理服务件……**包裹不是委托行、面单交易、外部号码或集运单元**。」末句显式把集运单元排除在
   包裹之外，而集运单元恰恰在载运对象之内。
3. **代码里已有一条成建制的立场，这条最硬且不是文档。**
   `internal/parcelshipment/adapters/nodeoperations/intake_qualification_evidence.go` 的
   `NodeExecutionQualificationEvidence.ProveIntakeQualification` 专设一道来源种类闸，注释原文：
   「只有节点收寄这一路的来源对象才是节点的作业实物；场外揽收的对象来自
   transport-fulfillment，**两侧编号偶然同名就会误证成已证明**」。仓内已经把「TF 对象编号与
   另一侧编号同名」判成**偶然**并为它加了防线，而不是判成同一主体。

**支持「同一主体」的文本，两条：**

1. TF CONTEXT「载运对象」：「载运对象**引用其来源上下文拥有的身份**；成为载运对象不会改变
   包裹身份、集运成员关系或客户责任范围。」当载运对象就是一个包裹时，它用的字符串**按定义**
   就是那个包裹的身份，不是另铸的编号。同文件 Boundaries 亦写「本上下文**引用包裹身份**并提供
   场外揽收、实际承运商首次有效收寄、交接、交付和履约事实」。
2. 物理上确是同一件实物，且两边有真实因果：VE 的投影正由 TF 的交接与交付事实派生
   （`internal/visibilityexception/adapters/inbox` 的 `NewTransportHandoverConsumer`、
   `NewEffectiveDeliveryConsumer`）。

**两边的张力可以一句话收住，但这句话归 owner 拍：身份相同不等于排队主体相同。** 载运对象在
「是哪一件东西」这一维上引用包裹身份（支持同一），在「哪些先后拍必须排一条队」这一维上是
另一个集合（含集运单元，支持不同）。另可计入支持「不同主体」的第三条间接证据：ADR-0065 的
「投影的取代关系由来源给出，本就不靠到达先后」说明跨口保序在 VE 这一侧买不到东西——该句已
写在 `offsite_pickup_registration_handoff.go` 的函数注释里。

### 二、【取证】两口今天的键表达，与具体共链场景

TF 两口今天的实际表达，两者逐字相同：

- `transportHandoverPartitionKey(key)` = `key.TenantID.String() + "/" + key.Object.String()`
- `effectiveDeliveryPartitionKey(key)` = `key.TenantID.String() + "/" + key.Object.String()`

票面所列 VE 四口今天的实际表达：

- `projection_handoff.go`：`intent.TenantID.String() + "/" + intent.Projection.Parcel().String()`
- `triage_handoff.go`：`intent.TenantID.String() + "/" + intent.Parcel.String()`
- `visibility_gap_handoff.go`：`intent.TenantID.String() + "/" + intent.Gap.Parcel().String()`
- `eta_handoff.go`：`intent.TenantID.String() + "/" + intent.Prediction.Parcel().String()`

**票面那张表要补一条**：VE 四口里只有 `projection_handoff.go` 有生产装配
（`newTenantBoundProjectionDerive`）；`triage`、`visibility_gap`、`eta` 三口的 `NewOutbox*`
在非测试代码里零调用点。

**具体场景（票面要的那个）**：今天在生产装配里真正共 `租户/包裹` 的是**三口**，而且一口都不是
票面点名的那两口——

| 口 | 装配点 | 分区键 |
|---|---|---|
| `vepostgres.OutboxProjectionHandoff` | `newTenantBoundProjectionDerive` | 租户/包裹 |
| `pspostgres.OutboxNetworkIntakeHandoff` | `networkIntakeAdoption` | 租户/包裹 |
| `pspostgres.OutboxFinalOutcomeHandoff` | `adoptEffectiveDeliveryConsumer` | 租户/包裹 |

共链路径是 FanOut：一封 NO 节点收寄形成信封同时投给 `adoptNodeIntakeConsumer`（PS 采用）与
`deriveProjectionConsumer`（VE 派生），两条链各自入队一封，两封落进同一个 `租户/包裹` 分区。

**但这条链今天同样照不出来**，成因与第〇节是同一件的另一半：两处装配点的硬资格证据口都给
`pspartycommercial.UnconfiguredIntakeQualificationEvidence{}`，PS 采用恒停在
`ELIGIBILITY_UNDECIDED`，PS 那两口一封不发，同分区里今天只有 VE 投影一家。

**所以「要不要加维」这一问今天没有任何运行期证据能推动它**，两边都不产信封，判据只能来自
领域（第一问）。这一点票面没说，建议写进票面：它决定了本票是「等 owner 裁」而不是「等取证」。

**推荐（标明是推荐）**：若第一问判为不同主体，TF 两口应跟着加维，且**应与
`offsite_pickup_registration_handoff.go` 取同一形状**（类型段）——三口同属 TF 对象链，形状不
一致会让下一个人以为其中某口是特意的。若判为同一主体，则按该文件注释预留的那句把类型段去掉，
三口一起回到光秃秃的（租户+对象）。

### 三、【估门禁】「全仓分区键表达求值后取交集」做不做得出来

**先答那个具体提法：做不出来。** 卡在两处，第一处是硬的。

1. **求不了值。** 分区键是运行期字符串拼接，成分是领域值对象的 `.String()`，值来自库行与
   信封载荷，静态没有值可求。
2. **退成类型层求交也不成立，而且是两头都不成立。** 现有门禁
   `internal/architecture/envelope_partition_gate_test.go` 走 `go/parser` +
   `types.ExprString`——`parseRepositorySources` 只调 `parser.ParseFile`，**全程没有类型检查**，
   `types.ExprString` 是纯 AST 打印。要区分「这一段是 `CarriedObjectReference` 还是
   `DeclaredParcelID`」得整体换成 `go/packages` 全量类型检查。**而即便换了也答不出本票这个
   问题**：本票的碰撞正是两个**不同类型**承载**同一字符串**——类型层看是两回事，值层是同一个。
   类型交集会把每一对 `租户/X` 都报出来（今天光生产装配就有三口同形），同时漏掉真碰撞。

照 `internal/architecture` 自己的立场，这里应当明写做不出来，而不是建一条扫不到东西的门禁：
该包原话是「一个从来不会失败的门禁比没有门禁更坑人，因为它还会取信」，而
`envelope_partition_gate_test.go` 已经预先写下了这一格——「本清单清空 ≠ 这一类缺陷清完了……
那一类只能靠人拿上面那句判据重扫」。本票这一类正落在那句话里。

**能建的是另一条，而且更值：登记式主体声明。** 形状照抄 `allowedSameExpression` 的现成做法
（中心清单 + 判据前缀 + 只许变短 + 自测能红）：每个交接口在一张中心表里声明自己的分区主体
（`租户/包裹`、`租户/载运对象`、`租户/客户`……），门禁只查两件——表是否覆盖全部交接口（新口
进不来就红），以及是否有两个不同上下文声明了同一个主体名。它守不住「两个主体名其实是同一个
字符串」，但它把这个判断**从代码里挪到一张人能一眼扫完的表上**，而那恰是第一问要 owner 拍的
东西。成本估在一张票以内：清单是一次性手写（四十六行），门禁本体与现有那条同构。

### 四、与并行线索合并：「全绿但照不到」是不是一类通用形状

**是三类，不是一类。硬并成一类会把能扫的那一类也判成扫不出来。**

> 本节原写两类（2026-08-21 首次落笔）。ADR-0071 报到后补入丙类，原两类一字未改。

**形状甲：测试自己补上了生产缺的那半根线。** MCP-5 在 syn-wall-door-audit 票 02 报的那个实例
（读口没实现、nil 归为「显式未配置」、整座桥恒答未解析、四十一条全绿一条照不到），与本轮我在
本票撞到的「TF 两口零生产装配」是同一形状：用例证的是「若接上则可用」，生产里那根线不存在，
而用例自带接线所以永远绿。**这一类扫得出来，判据是纯句法的**——某个生产端口的全部构造调用点
是否都在 `_test.go` 里。现有门禁框架够用，不需要类型信息，而且 `parseRepositorySources` 本来
就跳过 `_test.go`，正好是这条门禁要的那一半。

**普查数字先摆出来再决定要不要建**：`internal/` 下共 46 个 `NewOutbox*Handoff` 构造函数，
**41 个在全仓非测试代码里零调用点**（生产只用到 5 个，全在 `assemble.go`）。所以这条门禁
**不是缺陷检测，是进度盘点**——41 行例外不是 41 个缺陷，是十八墙审计里那些还没建门的口。

**而这恰是它的价值所在，只是价值不在原先设想的地方**：十八墙清单今天由人工维护、每轮人工重核
（T1-REVERIFY 五票四件那一轮就是这么核的，`ee58c1a` / `d6453a7`）。一条「哪些口有生产调用点」
的机械扫描能把这份重核变成派生的，且不会过期。**要建就按这个理由建，不要按「抓缺陷」建**——
按抓缺陷建的话第一天就是 41 行例外，那正是 `envelope_partition_gate_test.go` 点名否掉的那种
「先放着」清单。

**形状乙：性质跨越两个各自合规的单元。** 本票的分区键碰撞属这一类，判据不是句法、也不在任一
单个单元里，扫不出来，理由见第三节。

**形状丙：判定基准不存在。** 首个实例是
[ADR-0071](../../../docs/adr/0071-catalogue-views-carry-tenant-in-the-method-signature.md)（MCP-3
报）：目录/策略视图在组合根构造期绑租户，各手写一道零值守卫；**漏抄守卫的后果是视图恒答未配置，
而未配置正是首发唯一走得到的正确分支——正确接线与漏接线的可观察行为逐字节相同。**

丙与前两类都不同，三项都不成立：**代码可达**（不是甲）、**单元内且单一跳**（不是乙）。它不可
检测的原因是第三种——**判定基准不存在**：实例半边为空，把「接对了」与「接错了」归到同一个可观察
结果上。

已取证：`NewMilestoneMappings` 与 `NewDisclosurePolicies` 在 `cmd/parcel-dispatch/assemble.go` 的
两个租户绑定包装的 `Handle` 里**确有生产调用点**，所以[棘轮门禁](../../production-wiring-ratchet-gate/issues/01-production-ports-wired-only-in-tests-have-no-ratchet.md)
会把它们判为「已接线」并**永远绿**——那张票已按此在「守不住什么」里点名本类。

**三类的分界线：甲问「这根线接了没有」，答案在语法树里；乙问「两个值撞不撞」，答案在运行期
数据里；丙问「这个答案对不对」，而今天没有能回答它的基准。**

**丙还有一条前两类没有的性质，它有时效。** 甲乙是静态的——今天扫不出，明天也一样；丙的不可见性
来自「实例半边为空」，**而那是一个会结束的状态**。推论：**这一类会在第一个租户到来那一刻集中
显形，而不是陆续被发现**；今天每多一处靠人抄守卫的接线，就多一发存货。

这条推论与 T4 重盘那个里程碑相关——十八墙清单盘的是「有没有门」，盘不到「门接对了没有」，而后者
今天全靠人。**但产品就绪判据的唯一权威是[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)，
改它属产品基线变更，不在本票范围**；此处只记事实与推论，由协调岗上报用户定夺。

### 五、附带：MCP-4 票 10-B 接线后，「硬资格未证明是常态」会不会变

**不变。** 只读核过 `internal/parcelshipment/adapters/nodeoperations/`（未碰任何 PS 文件），
三重理由，任一重单独成立即足够：

1. `KnownPrefixIntakeQualificationEvidence.ProveIntakeQualification`：前缀未登记即答
   `IntakeQualificationUnproven`。该类型注释原话：「生产装配不登记任何前缀——不接假关务身份，
   也不把 UC-NR-002 禁限运登记进来。」
2. 即便前缀登记上，`NodeExecutionQualificationEvidence` 的 `qualifying` 表为空 →
   `registered == false` → `Unproven`。该类型注释原话：「前缀与登记表都在装配期给，没有租户
   就没有资格声明，因此生产上为空。空表答未证明。」
3. 即便登记表有行，还要 `facts.FindByKey` 找得回一件节点执行事实、且
   `record.Fact.PerformedAt()` 不晚于 `asOf`；无租户时节点执行事实表同样为空。

MCP-5 已在票 10 的 Comments 里把这件写成留给 B 的决策，并自判「两者行为等价（都恒答未证明）」
——与我逐口读出来的结论一致，构成独立的第二次取证。

**因此票面「硬资格未证明是今天的常态（实例半边为空），不是罕见边缘」这句在 10-B 之后仍然成立**，
不必改。但票面由它推出的下一句「因此头端阻塞是常态」不成立：常态未证明让 PS 采用链**根本不发
信封**，同分区里今天只有一家，碰撞被同一个原因掩住了。这是票面里唯一一处需要改结论的推理。

### 六、Status 与未决

`needs-triage` → `ready-for-human`，已改到票面。理由：第一问是领域裁断（归 owner）；第二、
三问取证已齐，且都指向「今天没有运行期证据能推动它」；不存在可由 agent 直接开工的机制件。

`Category` 保留 `bug` 未动：接线即发作，缺陷是真的，错的只是「今天就在发作」那半句。若 owner
认为「装配后不可达」应当降为 `enhancement`，那一改与第一问的裁断应同笔落，我不单方面动。

未发现难逆转取舍，故不报 ADR——第一问虽是领域裁断，但它是在 ADR-0065、ADR-0069 已定的框架内
选主体粒度，不是新开取舍。若 owner 判为「不同主体」并决定给分区键加上下文维，那一步会改变
跨上下文的保序语义，届时应走 ADR，本票不预判。

- 2026-08-21 MCP-2（协调岗三条裁定落地，仍零代码）：

  **裁一：`Category` 保留 `bug`，且不必等第一问**——两者无关。判据用第四节那条界线：41 个零
  调用点的交接口是**缺席**（门还没建），本票是**在场且错**（两扇已建的门用了同一把锁孔）。
  另一层更硬：本仓没有任何东西接着租户跑，若「装配后不可达」能降级，`bug` 会塌缩成「只有弄坏
  合成路径的才算」，那会系统性掩掉本仓产量最大的一类缺陷。票面维持原样，未改。

  **裁二：`ready-for-human` 确认，第一问协调岗同样不代拍**，理由即本节上一条——判为「不同
  主体」会改变跨上下文保序语义，属难逆转取舍，须走 ADR，已向用户上报。**按裁定把综合句
  「身份相同不等于排队主体相同」从本 Comments 中段提到票面「要答的」第一问旁**，并补两分支
  代价表；另补一个此前没明写的独立维度：ADR-0065 使跨口保序在 VE 侧买不到东西，因此**即使判
  同一主体，共分区也是只有成本没有收益**，那就得正面回答「那条队买到了什么」。

  **裁三：门禁要建，但救它的是「只许变短」而不是改叫法。** 我原先把它记为「进度盘点」是软了
  ——一条永远不会红的门禁，叫什么都还是那句「比没有门禁更坑人」。加上只许变短之后，41 行不是
  清单是棘轮，三种真红今天一种都没人守：新增第 47 个零调用点的口、某口从已接线退回未接线、
  接上一口忘了删行。第二种尤其——今天 `assemble.go` 删掉一行接线，全仓依旧全绿。

  据此拆两票，均不往本票里塑：
  [棘轮门禁](../../production-wiring-ratchet-gate/issues/01-production-ports-wired-only-in-tests-have-no-ratchet.md)（`ready-for-agent`，先做）与
  [登记式分区主体声明](./02-partition-subject-registry-gate.md)（`ready-for-agent`，后做，
  **不阻于第一问**——表存在本身就是为了让人能拍第一问，互相等会死锁）。后者票面已明写它
  **守不住「两个主体名其实是同一个字符串」**，并注「实现时不许删这一节」，否则下一个人会把它
  当成已经守住了本票。「求值取交集做不出来」的两层论证已按裁定搬进票面第三问，拦下一个人。
