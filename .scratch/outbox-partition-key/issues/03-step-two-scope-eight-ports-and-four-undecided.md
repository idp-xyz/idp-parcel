# 第二步的确切范围：八口要改、四口待裁，剩下十四口不用动

Category: bug
Status: needs-triage

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
