# ADR-0031: 自有仓储端口的写入结果是封闭代数而不是 error，预期版本由聚合携带

Status: Accepted（**Consequences 登记的 `Insert` 已知缺口已关闭**：入口条件「已经建过了 ≠ 直接译成已有结果」已答为否；`ShipmentRequestInsertOutcome` 承接该格，编排按重放规则重答且不重复追加观察。）  
Date: 2026-08-11

## Context

[ADR-0028](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md) 写下「聚合携带版本，`Save` 收预期版本，而推进版本是仓储的事」，并声明这一条不是取舍，是框架合同规定的。聚合那一半已经落地——`ShipmentRequest.revision`、`Revision()`，以及守「状态转移一律不动版本」的那条反射性质用例。端口那一半一个字都没动，[ADR-0030](./0030-rehydration-admits-one-state-at-a-time-by-snapshot-expressiveness.md) 把它显式登记为遗留：「`ShipmentRequestRepository.Save` 仍未收预期版本。」

**ADR-0028 那句「不是取舍」经实测不成立，本记录据此改按取舍论证。** 逐符号核过 `idp-bento-go` 之后：框架全模块**没有任何符号叫 `Save`**；`testkit.RunRepositoryContract` 收的不是「一个仓储」，是 `RepositoryContractScenario[K,A]`，里面三个分开的接口——`Loader[K,A].Load`、`Inserter[A].Insert`、`VersionedUpdater[K,A].Update(ctx, key, aggregate, expectedRevision Revision) (Revision, error)`。更要紧的是，框架为「版本怎么随聚合走」给出的形状恰好**相反**：`Loaded[A]{Aggregate, Revision}` 存在的意义就是让聚合本身不必带版本。框架不禁止聚合带版本，但显然没有规定它。三个接口字段还可以由三个不同的值填，因此框架**看不见**本上下文的语义端口——跑得通那份合同并不要求 `Save` 改名或改签名。

成立的只有「推进版本是仓储的事」那半句：`Insert` 与 `Update` 都交回新的 `Revision`。

**这不动摇 ADR-0028 的决定，只取消它的免议地位。**「`Save` 收预期版本」仍然要做，而本记录下面为它给的理由是本仓自己的「一个事实一处表达」，不依赖框架合同，框架沉默不影响它。但它同时意味着**这确实是一次取舍**——而取舍要有记录，也就是本记录。

真去落地时，逐条摆开发现要定的**不是一件事，是两件**，而 ADR-0028 只说了第一件：

- **预期版本怎么进去。** 作独立参数、由聚合携带、还是另开一个带并发控制的方法。
- **冲突怎么出来。** 三种进法没有一种回答了它。

第二件才是会扩散的那个。今天四个 `Save` 调用点——`FormAcceptanceDecisionHandler.Handle`、`RejectShipmentRequestHandler.Handle`、`WithdrawShipmentRequestHandler.Handle`、`AmendCustomerSourceDataHandler.Handle`——一律是 `if err := ...Save(...); err != nil` 塌成一个未决常量，前三处给 `DecisionNotRecorded`，第四处给 `AmendedRequestNotSaved`。**一次版本冲突会落进「没落库」这一格。** 两者的运维含义相反：一个可能是库坏了，一个是正常竞争；而续办引用由原因与范围共同派生，压在一格就是让两种缺口共用同一条引用。

**本上下文对同形状的问题已经答过五次，答案每次相同。** `JudgmentAsOfFormation`、`PreAcceptanceControlAssessment`、`ReachabilityAssessment`、`CommercialRevalidation`、`SourceDataAmendmentAllowance` 全都不把业务答案压进 `error`。理由写在 `ports.go` 上，说的是可达性那一格，但它逐字适用于版本冲突：

> `请求冲突`与`未受理`不译成 error。它们是业务答案而非技术故障——调用方必须能据以纠正自己这次请求，而不是当作故障重试。译成 error 之后消费侧只剩「依赖答不出」一格，续办路径是内部重试，正是那句话禁止的事。

**但那五次全是跨上下文提供方端口，而 `ShipmentRequestRepository` 是本上下文自己拥有的。** [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md) 的「翻译必须是全函数」按字面只管跨上下文适配器对提供方封闭集合的翻译，够不到自有端口。所以「自有端口要不要也这么办」在本仓是第一次被问，而此前五次的一致答案**不构成先例**——它们答的是另一个问题。

**难逆转的是这个第一次怎么答。** 八个不可重建类型、两个上下文的 Repository 都会照第一个仓储端口的写法来，这与 ADR-0028、ADR-0030 面对的是同一句话。

## Decision

**自有仓储端口的写入结果是封闭代数，不是 `error`。** `Save` 交回 `(ShipmentRequestSaveOutcome, error)`：`error` 留给「这次写入没能完成」——连接断了、事务回滚了、库不可达；`版本冲突`是一个**业务答案**，它意味着写入这条路走通了，只是有人先落了一步。

**分格判据是消费方的恢复动作，不是端口归谁拥有。** [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) 已经把这条维度写下来，但它自己划了管辖：「本记录只管「按标识取回原状态」这一步」。**本记录是一次显式扩用，不是援引。** 扩用成立的理由是那条维度的论证不依赖跨上下文这个前提——消费方要从取值里读出的是「我接下来做什么」，这句话对自有端口一字不变；而它在 ADR-0029 里之所以能同时满足「不泄露」，靠的是那一步恰好有隐蔽性要求，本处没有，因此只剩全函数那一半在承重，结论方向不变。

**扩用的只是这条分格维度。那份记录里的两格结论、以及「恢复动作相同必须合」那条仲裁规则，都不随之扩用。** 后者要单独点明，因为它看起来正好反对本记录下面「版本冲突自占一格」那一条。它在 ADR-0029 里立在两条腿上：合并「不抹掉任何调用方能用上的区别」，且合并之后「可区分性正好消失，所以满足不泄露」。**两条腿在本处都不成立**——区别用得上（原因参与续办派生与未决统计，理由见下），隐蔽性则根本没有对象。所以那条仲裁规则在这里推不出「该合」，不是被绕过，是前提不满足。

**预期版本由聚合携带，不作独立参数。** ADR-0028 已经写下「聚合只记自己是从哪一版读出来的」，那就是预期版本的定义；再开一个参数就是造第二个来源，而两个来源相等这件事由另一条不变式（转移一律不动版本）保证，不由这个签名保证。今天两者恒等，因此没有任何东西分辨得出哪个调用点写错了；哪天那条不变式松动，四个调用点会分成两派而不报错。**一个事实一处表达。**

**强制力由返回值提供，而不是由入参提供。** 这是本记录与「改签名收一个参数」的真正分界。改入参能把编译器的手按到四个调用点上，改返回值同样能——而且它顺带逼出第二个问题的答案：拿到一个新取值的人必须回答「这一格算什么」。只改入参的话，四个调用点会照旧 `if err != nil`，那格静默留空，没有编译错误、没有测试变红、没有评审提示。

**本记录的结果代数只用于仓储端口，不蔓延到聚合转移。** ADR-0028 把 `(ShipmentRequest, error)` 定为状态转移的**唯一**合法形状，而守它的那条反射用例会把别的形状判成违规，报文写明「别的形状不是漏覆盖，是形状本身违 ADR」。所以一个 `Decide(...) (ShipmentRequest, DecideOutcome)` 会当场撞上那道守卫。两者的分界说得清：**端口交回的是「外面发生了什么」，转移交回的是「本聚合成了什么样」**——后者的失败一律是不变式被破坏，而那正是 `error` 该在的地方。

**逐取值分派，不留 `default` 兜底。** 照 `asOfPendingReasons.forOutcome` 的既有写法：认不出的取值上抛哨兵错误，而不是静默继承某一格。ADR-0025 那句「兜底那一格会成为下一次『合成一格』的入口」在这里管的不是它字面的适用范围，是它的理由。

**版本冲突在应用层自占一格未决原因，续办路径仍是内部重试。** 后半句是判据明确的：四个 `Save` 调用点所在的 `Handle` **全都以 `FindBySourceIdentity` 开头**，所以一次内部续办重入天然就是「重读再重放」；三个等待态里另外两个（客户补件、人工复核）对版本冲突都无能为力——客户补不出一份被别人抢先写掉的版本。因此不新开 `ResumePath`，`resumePath()` 的 `default` 落点判对。

前半句要单说，因为它看起来与 ADR-0029「恢复动作相同必须合」相反。**恢复动作相同不等于原因相同**，本包已经这样判过一次：`CommercialBasisSuperseded` 与 `CommercialRevalidationBasisNotResolved` 共用同一个恢复函数，仍然分成两格，理由是「原因参与续办派生与未决统计，而那两样恰恰要求分得开」。ADR-0029 的合并规则要防的是可区分性泄露存在性，本处没有那个对象。

**四个调用点共用同一个未决原因，不按调用点拆。** 续办引用由原因**与范围**共同派生，而四处的范围本就不同（资料修订那一处取的是修订请求自己的身份加资料范围）。拆成四个原因不会让引用更可分，只会让未决统计多三行说的是同一件事。

**`Insert` 本轮不改，且这不是遗漏。** 判据是两者的结果代数**不同**而不是相同：`Save` 的失败答案是「有人先落了一步」，`Insert` 的是「这份已经建过了」，恢复动作也不同（重读再重放 vs 读回既有结果）。合成一个代数会让调用方拿一个取值去回答两个问题。`Insert` 的那一格由 `PBC-04`「并发重复不能创建第二份委托或第二个 EventID」驱动，它今天把冲突上抛成 `error`，与四个 `Save` 调用点的「转未决」本就不在同一格——**那条不对称在本记录之前就存在，本记录不制造也不修复它**，见 Consequences。

## Consequences

- `ports.ShipmentRequestRepository.Save` 的签名改变，波及四个应用层调用点与五个测试替身（`decidableRequestStore`、`rejectableRequestStore`、`amendableRequestStore`，以及 `application` 与 `adapters/http` 各一个同名的 `shipmentRequestRepositoryDouble`）。这正是 ADR-0028 Consequences 预告的那次波及，只是波及的是返回值而不是参数表。
- **`JudgmentPendingReason` 多一个取值。** 它必须插在 `judgmentPendingReasonEnd` 之前并补 `String()`；漏补当天两道门禁会红（`TestEveryEnumConstantIsNamedByItsStringMethod`、`TestEveryPendingReasonHasAStringAndAResumePath`），所以这一处不必靠人记得。
- **`Insert` 的冲突曾登记为已知缺口；现已关闭。** 入口条件已答：建单仓储的「已存在」**不是** `SubmitOutcome` 里既有的`已有结果`——还要按 `ClassifySourceSubmission` 比内容，同内容才答`已有结果`，不同内容答`接入冲突`。关闭落点是 `ShipmentRequestInsertOutcome`（与 Save 分代数）与提交编排的 `resolveAfterInsertConflict`（不重复 `AppendObservation`）。框架侧 `ErrAlreadyExists` 仍一对一翻译到`已存在`。原登记句保留作当时范围说明，不改写 Decision 史实。
- **真适配器仍阻断在 [ADR-0017](./0017-admission-gates-judged-by-blocking-cause.md) 的 Bento 持久化闸门后**，所以本记录今天只落端口、调用点与替身。框架侧的形状已实测：版本冲突是 `Update` 交回的哨兵 `repository.ErrConflict`（注释「表示条件更新未满足预期 Revision」），同包另有 `ErrNotFound` 与 `ErrAlreadyExists`。**适配器那一跳因此是一对一翻译，不压平也不生歧义**——`ErrConflict` 译成`版本冲突`，`ErrAlreadyExists` 译成`已存在`，其余仍作 `error` 上抛。
- **`expected = 0` 框架不定义，而这一格有活的后果。** 合同只跑过预期版本 1，`Insert` 被硬断言必须返回 1；`Revision` 是 `int64`，0 表达得出却没有既定语义。而今天全仓设 `revision` 的入口只有 `RehydrateShipmentRequest`（要求 ≥ 1），`SubmitShipmentRequest` 不设、转移不动它，于是**五个替身里的聚合版本恒为 0**，四个调用点在现有全部测试里递给仓储的都是 0——端口上没有任何东西表达「交给 `Save` 的聚合必须已持久化」。本记录的立场是适配器遇 0 **拒绝**而不是升格为插入：升格会让端口注释保护的「这是第一份还是第二份」在适配器内部失守，而签名上看不出任何痕迹。

  **「五个替身的聚合版本恒为 0」是一句会过期的现状断言，因此同样写成入口条件**：一旦有替身经重建门造出版本 ≥ 1 的聚合，上面这条论据就不再成立，届时要改的是这条立场的论证而不是悄悄留着它。重建面的导入门禁**不扫 `_test.go`**，所以没有任何东西会在那一天变红。它与上一条入口条件同形状，那里的理由一字不改地适用：比日后从一个死循环反推回来便宜。
- **新增保存调用点必须以重读开头，否则版本冲突不得走内部续办。** 这是入口条件，不是现状描述——「今天四处都以 `FindBySourceIdentity` 开头」是一句会过期的断言，而 `ResumePath` 自己的注释点名警告过它过期时的下场：「永远重试一件重试推不动的事」。一个聚合从别处传进来的编排一出现，内部续办会拿同一份过期聚合重放，冲突永远清不掉。形状与 ADR-0028 亲自解决过的「第七个转移」相同，那里的结论是「六处都要记得」守不到第七处；这里本期不加机制，但入口条件先写下来，比日后从一个死循环反推回来便宜。
- **`PBC-02` 今天一条都没证**：`RunRepositoryContract` 在本仓零次调用，`tests/bentocontract/` 只有 `PBC-01` 与 `PBC-06`；`migrations/` 也只有 `0001_source_submission.sql`，`shipment_request` 表还不存在。这不阻本记录（它只落机制半边），但它说明本记录关闭的是端口契约那一格，不是 `PBC-02`。
- 判错的方向是把「自有端口」读成「不必这么讲究」。那样第二个仓储端口会退回 `error`，而两个上下文的调用方从此一个能分辨冲突、一个不能——**而不能的那一个不会有任何东西变红**。

## Alternatives considered

- **`Save(ctx, identity, expected, request)`，预期版本作独立参数。** 否决：它造出第二个版本来源，而两者相等由「转移不动版本」那条不变式保证、不由签名保证；今天恒等，因此没有任何东西分辨得出哪个调用点写错。它还与 `SubmitShipmentRequestCommand.ExpectedRevision`（生产归属登记册修订，另一个类型、另一个含义）在同一批 handler 里撞名。它买到的编译器强制，返回值那一改一样买得到，还多买一个。
- **签名一字不改，仓储自读 `request.Revision()`，冲突仍走 `error`。** 否决：它单看入参那一半是对的（本记录采纳了这一半），但它让第二个问题静默留空。四个 `if err != nil` 没人访问，冲突就落进「没落库」，全程没有编译错误、没有测试变红。**编译器不逼任何调用点做选择，也就不逼任何人回答冲突算哪一格。**
- **另开 `SaveExpecting(...)`，旧 `Save` 留给不需要并发控制的路径。** 否决：那种路径今天一条都没有——四个调用点全是同一个 handler 内的 read-modify-write。它让「保存但不作并发控制」成为一条永久合法的路径，而赶时间的下一个人调它时两个方法都编得过、都返回 nil。ADR-0028 用同一条理由否掉 `EventBuffer`（「现在加是推测」）。它还有个反常性质：接口加方法逼着五个替身全部改，却不逼任何一个调用点改——**改动量最大，强制力最小**。
- **`Save` 与 `Insert` 共用一个写入结果代数。** 否决：两者的失败答案不是同一件事（「有人先落了一步」vs「这份已经建过了」），恢复动作也不同。按本记录自己的判据，恢复动作不同必须分。
- **不立记录，只改端口。** 否决：违反红线「单一权威」。「自有端口的结果代数按什么办」是本仓第一次回答，而另外七个不可重建类型与第二个上下文的 Repository 都会遇到同一个岔路；散在一个签名里，下一个人只能反推，而最容易反推出的正是上面被否决的第二条。

## Links

- [ADR-0028：聚合的重建与构造分属两扇门，重建只校验不重算](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)：本记录落它「`Save` 收预期版本」那一半，并回答它没有回答的另一问——冲突怎么出来
- [ADR-0029：按标识取回原状态失败时，结果代数按消费方的恢复动作分格](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：本记录把它的分格维度显式扩用到写入端，并写明扩用的是维度而非它那两格结论
- [ADR-0030：聚合重建按状态逐个开门](./0030-rehydration-admits-one-state-at-a-time-by-snapshot-expressiveness.md)：它把「`Save` 仍未收预期版本」登记为遗留，本记录关闭它；本记录对 `Insert` 沿用同一手法
- [ADR-0025：跨上下文调用的适配器落在消费侧，翻译职责由它独占](./0025-cross-context-adapters-live-on-the-consumer-side.md)：「翻译必须是全函数」按字面够不到自有端口，本记录取用的是它的理由而非它的适用范围
- [ADR-0017：实现准入闸门按阻断理由分别裁决](./0017-admission-gates-judged-by-blocking-cause.md)：真适配器仍阻断其后，本记录只落机制半边
- [Go 首个消费者切片决策简报](../design/parcel-go-first-consumer-slice-decision-brief.md)：`PBC-02` 的乐观版本冲突与 `PBC-04` 的并发重复要求所在处
