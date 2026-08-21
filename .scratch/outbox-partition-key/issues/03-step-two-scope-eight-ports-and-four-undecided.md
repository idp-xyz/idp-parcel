# 第二步的确切范围：八口要改、四口待裁，剩下十四口不用动

Category: bug
Status: resolved

[01](./01-per-event-partition-keys-make-the-ordering-guarantee-vacuous.md) 说第二步「等口径
定完再动」，但没说第二步有多大。本票把它从估算变成清单：`a771bc3` 上逐行读完门禁例外清单的
二十六个 handoff，按 01 的判据（**后一条会不会改写或取代前一条说过的事**）各判一次。

判据逐行落在 `internal/architecture/envelope_partition_gate_test.go` 的 `allowedSameExpression`
里，**本票不复制第二套**。这里只装那份清单答不出的两件事：**要改的怎么改**，与**待裁的卡在
哪一句**。

## 二十六行分成四堆

| 堆 | 行数 | 意思 |
|---|---|---|
| 真风险（更正入口／版本进键／状态序列） | **8** | 第二步的全部工量 |
| 依赖前序 | 2 | 乱序会报错重投，不静默错——但自愈有前提，见末尾 |
| 无先后 | 12 | 不用动 |
| 待裁 | **4** | 未知，不是零 |

**待裁那四行不要记进无害。** 记进去工量看起来就收敛了，而它们恰恰是最可能藏着缺陷的四行
——01 里 `supplier_bill` 被误判成无害正是这么发生的。

## 八口真风险，按 01 的甲乙丙分

### 甲：区分维已在 ID 里，只改 `PartitionKey` 一行，意图契约不变

| 口 | ID 里已有的区分维 | 分区键应取 |
|---|---|---|
| `customscompliance/.../verification_handoff.go` | 事实集指纹 | `租户/决定` |
| `customscompliance/.../gate_verification_handoff.go` | 逐项判断指纹 | `租户/范围/动作/边界` |
| `nodeoperations/.../sealed_snapshot_handoff.go` | 封签 | `租户/单元` |
| `pilotgovernance/.../governance_handoff.go`（暂停/恢复那一对） | `suspension/` 与 `resumption/` 前缀天然错开 | 被解除的那个暂停标识 |

治理那一格值得多说一句：01 把它列为首例时判的是「恢复可能赶在暂停之前送到」，**那个判断
成立，但它不丢**——两种类型的信封 ID 前缀不同，两份都入队，只是落进两个分区。所以它是甲不
是丙，改一行就够。

### 乙：对象有版本，ID 加版本 + 改分区键

| 口 | 版本在哪 | 今天的后果 |
|---|---|---|
| `customscompliance/.../manifest_handoff.go` | `ReceiveManifestHandler.Revise` 推进版本，ID 只有舱单标识 | 修订版**静默不入队** |
| `settlementaccounting/.../operating_handoff.go` | `CostAllocation` 的分摊版本、`OperatingResult` 的结果版本，ID 都不含 | 重分摊与重派生各**静默丢一份** |

`operating_handoff.go` 一个文件坐着两个缺陷、共用一个 `shape.eventID`：**只改一支门禁依旧
红**，两支都改完才能删那一行。

### 丙：对象无版本但有状态判别子，ID 加状态后缀 + 改分区键

| 口 | 两拍是什么 | 后缀照谁写 |
|---|---|---|
| `customscompliance/.../follow_up_handoff.go` | `ManageFollowUpHandler.FormTarget` → `.RecordEffect`，同一目标键 | `statement_handoff.go` 的 `/voided` 是仓里跑着的现成形状 |
| `settlementaccounting/.../settlement_application_handoff.go` | `MapExternalFundsHandler.Reverse` | 同上 |

### 同一文件里的第二个缺陷：治理接管

`takeoverEventID` 取（对象范围+能力+事实类型），**不含 `Authority`**。同一区间由另一个权威
接管会算出同一个信封 ID，于是第二次接管静默不入队。这一格属乙或丙（取决于接管要不要版本
维），**与上面那一对暂停/恢复不是同一件事，改法也不同**。pilot-governance 当前无主。

## 四处待裁，每处只卡一句

**关务案件链三口**（`customs_case` / `declaration_submission` / `case_closure`）卡同一句：

> 同一案件的建立 → 申报提交 → 核对 → 关闭是否需要保序？

01 已经点出这条最容易漏的地方：**分区由键值定、不由 handoff 定**，四口不改成同一个键公式就
落不进同一分区，各自改成业务键也白改。`verification` 也在这条链上，但它另有独立成立的真风险
（事实集指纹换版），所以它列在甲类里、不等这一裁。

关闭那口还带一个子问题：`CloseCustomsCaseHandler.Handle` 的注释写着「重开走 Reopen 不走
这里」，而重开后再关会撞同一个信封 ID。裁案件链时一并答。

申报提交那口的另一半已经有票：ID 缺版本维见
[declaration-envelope-version-dedup/01](../../declaration-envelope-version-dedup/issues/01-envelope-id-lacks-version-dimension.md)
——核证结论是机制坐实、今天无触发路径、引信是 CC CONTEXT 预期的「原案内更正」。**本票不重复
它，也不把它算进上面那个 8。**

**场外揽收登记一口**（`transportfulfillment/.../offsite_pickup_registration_handoff.go`）卡：

> 同一载运对象能否出现第二次成功的对象级揽收登记？

判不准的理由是证据本身留了余地：[02](./02-pod-correction-is-silently-swallowed-by-enqueue-once.md)
自查这一口时写的是「**一个键**只会有一份意图」，没写「一个对象一份」；而键里留着尝试维。
若同一对象能有第二次成功登记，两次就是同一条控制链的先后拍，该照 `transport_handover` 那口
的先例把主体取到对象——那口的注释已经把理由写死了：控制转移对一个载运对象是一条链，取到范围
就把链切成互不排队的两段。

## 两口依赖前序：不用改代码，但要给消费者留一条约束

`advance_recovery` 与 `claim_settlement` 的调整都**引用**先前的记录且不改写它
（`Adjust` 先核对目标在场）。乱序时消费方找不到被引用对象会报错回滚重投，前序到了就好。

**但这条自愈的前提今天还没人定**：它要求消费方把「引用对象尚未到达」当作可重试失败。
ADR-0049 定了毒丸显式拒收入账，哪个消费方把这一格归进毒丸，那份信封就真丢了。

settlement-accounting 眼下一个消费者都没有。**这条约束属于「建 SA 消费者」那张票，不属本票**
——现在记下来成本为零，等有了消费者再发现就是丢数据之后才发现。

## 本票不做的事

- 不改任何 `PartitionKey` 与信封 ID。本票的产出是范围，不是修复。
- 不替 pilot-governance 与 customs-compliance 拍板——它们无主，四处待裁要人裁。
- 不复制门禁清单里的逐行判据。那里是单一权威，这里只装修法与卡点。

## 一条门禁守不住、清单也不收的

`allowedSameExpression` 拦的是「ID 与 `PartitionKey` 同源」这个句法形状。**分区键在业务键上
拼得太细、却与 ID 不同源的口，既不进门禁也不进清单，用例还全绿。** 那一类只能靠人拿判据重扫，
本票的二十六行不覆盖它。这句话在门禁文件里已经写着，此处只是提醒：清单清空 ≠ 这一类清完。

## Comments

- 2026-08-20 MCP-1（triage）：**八口＋治理接管那格现在开工**，派 MCP-2（OUTBOX-PK-STEP2），
  票置 in-progress——三类改法票面已备齐、有仓内先例可照、系统无生产数据，修的是静默丢失，
  不等四处待裁（互不依赖，票面已论证）。接管那格乙或丙由实现者按证据判：判得动就修，
  判不动原样报回不猜。**四处待裁不随手裁**：关务案件链保序（含重开再关撞 ID 子问题）与场外
  揽收二次登记都是真领域问题，PG/CC 无主，须先取证再裁断，另行成轮；在此之前这四口一行
  例外清单都不许删。依赖前序两口的消费者约束照票面归「建 SA 消费者」那票，本轮不动。
- 2026-08-21 MCP-1（裁断落地）：**四处待裁清零，票转 resolved。** 授权出处——owner 经 IDP
  队列先批「按此起草」（草案 [ruling-draft-four-undecided.md](../ruling-draft-four-undecided.md)），
  再于本轮批「你现在是主控制方，你来决定，当前没有任何工人工作了」，据此由本会话直接落地。
  取证是只读轮的 [evidence-four-undecided.md](../evidence-four-undecided.md)（取于 `3b9f212`），
  落地前按 `f47f698` 重核，两处代码公式与取证逐字相符。

  **裁断一（案件链三口）**：全文见 [ADR-0069](../../../docs/adr/0069-customs-case-chain-ordering-absorbed-by-reread-and-retry.md)。
  四口不建立跨口同分区保序，乱序由指针载荷、按键重读与「不可见即可重试」消化；关闭信封 ID
  加关闭周期序数、分区键收窄到 `租户/案件`；信封上的案件维统一用铸造 `CustomsCaseID`；申报口
  的案件维是建模欠账，另票。

  **裁断二（场外揽收登记口）**：同一载运对象允许第二次成功的对象级登记，两次是同一条控制链
  的先后两段；权威落点是 TF CONTEXT 的跨段接续句与 UC-TF-002 的补句加 `AT-TF-098`，不另立 ADR。
  分区主体取到对象。与既有去重句的分工：**UC-TF-002**「一致性、幂等与并发」一节的「同一实际
  控制范围不能因伙伴重投、任务重建或批量重试重复建立履约参与」管**同段去重**，新增的 CONTEXT
  句管**跨段接续**——那句去重语在 UC 不在 CONTEXT，两句分属两层文档，引用时勿混。

  **落地时对草案的三处修正**（草案措辞未经修正不可直接照抄）：
  1. 关闭周期序数改为从 `CustomsCaseClosure.Reopenings()` 条数加一**派生**，不由意图注入。
     `CaseClosureHandoffIntent` 只有租户与关闭记录两个字段，注入要加宽契约，而今天没有调用方
     给得出 1 以外的值——那只是把常量挪进编排，正是 ADR 自己否决「等实现时再改」的那种分离。
     派生的成立条件写进 ADR 决定一的**第四条成立前提**：多周期若改成一案多条关闭记录、新记录
     从零条重开起算，序数退回 1、撞 ID 复活，届时须同时给出新序数来源，否则重裁。
     顺带更正取证 A4 的一句：`ports.CaseClosureStore` 接口虽只有 `FindByCase`/`Save`，但重开
     **是**持久化的——`CaseClosures.Save` 对已有关闭走一条只写 `reopenings` 列的 UPDATE，
     `rebuildCaseClosure` 逐条重建，`TestCaseClosureRoundTripsAndReopeningAppendsInPlace` 守着
     这条往返。「店无 Update」只在方法名上成立，能力上不成立，派生因此跨库往返不丢。
  2. 草案 ADR 正文把 ADR-0049 链成 `0049-dispatch-routing-table-is-an-explicit-list.md`，
     该文件不存在；实际是 `0049-publish-channel-is-in-process-delivery-until-load-evidence.md`。
  3. 草案落地表 PS 采用侧那行「两封信封分属两个分区、到达先后不定」修完即为假：分区键收窄到
     `租户/对象` 后，同对象两尝试同分区且保序，**顺序那一半正是本次修复解决的**，剩下的只有
     采用语义（后段成功是否顶替前段采用）。那一半仍属采用语义，不在本裁断内，记此防丢。

  **揽收登记的分区键最终取（租户+对象+类型段），不是光秃秃的（租户+对象）。** 评审时我一度
  主张并进后者，理由是 TF 已有 `transportHandoverPartitionKey` 与 `effectiveDeliveryPartitionKey`
  两口在那里、揽收 → 交接 → 交付可以端到端保序。落地实测把这条推翻了：并进去之后
  `TestARegisteredOffsitePickupStopsAtUnprovenIntakeEligibility` 红在重拍那句（派生投影定稿 0 条，
  want 1）；换成（租户+对象+探针后缀）——仍是一对象一键，只避开 VE 的键空间——立刻转绿。
  起因确切：VE 的投影/triage/gap/eta 四口按 `租户/包裹` 分区，而载运对象引用与申报包裹标识是
  同一个字符串，并进去就与 VE 共分区，一封未决的揽收会把同一包裹**已经由 VE 受理并派生**的
  追踪投影堵在分区头，直到预算耗尽进 `ABANDONED`。而「硬资格未证明」是今天的常态。

  取舍据此重定：裁断二真正要的顺序只有「同一对象两次成功揽收之间的先后」，类型段保得住；
  跨口链到 VE 那一段既非需求、也换不来新保证——投影的取代关系由来源给出（ADR-0065），本就
  不靠到达先后。用已经成立的客户可见性去换一个用不上的顺序不划算。

  由此分出的更大问题——TF 的（租户+对象）与 VE 的（租户+包裹）是不是同一个排队主体、
  handover 与 delivery 两口今天已经共链要不要跟着改、这一类要不要门禁——见
  [partition-key-space-collision/01](../../partition-key-space-collision/issues/01-tf-object-partitions-collide-with-ve-parcel-partitions.md)，
  实测证据已带过去。它是票 03 末尾那句盲区的另一半：那里说的是键拼得太细，这里是拼得太粗。

  **门禁例外清单四行已清**：关闭行与揽收登记行随各自修复删除（ID 与分区键不再同源），建立行与
  申报行由「待裁」改写为「无先后」并引 ADR-0069。`allowedSameExpression` 中已无「待裁」条目。

  **新开票**：CC 申报单元 → 案件关联在域模型里缺席，见
  [customs-declaration-case-link/01](../../customs-declaration-case-link/issues/01-declaration-unit-has-no-case-association.md)。

  票面「一条门禁守不住、清单也不收的」那一段继续成立：清单清空 ≠ 这一类缺陷清完。
