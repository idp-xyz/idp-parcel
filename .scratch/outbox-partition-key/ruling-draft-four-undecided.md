# 四处待裁·裁断草案（**已落地，留作起草轮记录**）

> **2026-08-21 已落地，本文件不再是现行依据。** 现行依据：裁断一见 [ADR-0069](../../docs/adr/0069-customs-case-chain-ordering-absorbed-by-reread-and-retry.md)，裁断二见 TF CONTEXT 的跨段接续句与 [UC-TF-002](../../docs/application/transport-fulfillment/UC-TF-002-PERFORM-OFFSITE-PICKUP.md) 的补句加 `AT-TF-098`。落地时对本草案作了三处修正（关闭周期序数改派生不注入并补第四条成立前提、ADR-0049 链接文件名、PS 采用侧那行修完即为假），逐条见[票 03 Comments](./issues/03-step-two-scope-eight-ports-and-four-undecided.md)。**照抄本文件会抄进那三处，以票 03 Comments 与上述权威文档为准。**

- 性质：**草案**。方向由 owner 经队列拍板（「按此起草」，2026-08-21），专家建议与业务论证见队列记录；本文件是可落地的完整措辞，**owner 确认前不改任何权威文档、不改任何代码、不删例外清单行**。
- 基线：origin/main `f47f698`。四口键公式与消费侧分格已按该基线重验，与取证文件（[evidence-four-undecided.md](./evidence-four-undecided.md)，取证于 `3b9f212`）无出入。
- 覆盖：票 [03](./issues/03-step-two-scope-eight-ports-and-four-undecided.md) 的四处待裁——CC 案件链三口一并裁（裁断一），TF 场外揽收登记一口（裁断二）。

---

## 裁断一：案件链三口——跨口不设分区保序，链内版本各携判别维

**一句话**：建立 → 申报提交 → 核对 → 关闭的跨口到达顺序不由分区保证，由「指针载荷＋按键重读＋不可见可重试」消化；业务真正的刚性要求是**审计链完整**——每个关闭周期、每份申报版本各自成封不丢，链内保序。

### ADR 草案全文（落地时建 `docs/adr/0069-*.md`，编号以当时实际下一号为准）

> # ADR-0069: 关务案件链乱序由重读与重试消化，不由分区保证；关闭信封携关闭周期维
>
> Status: Accepted
> Date: 2026-08-21
>
> ## Context
>
> [outbox-partition-key 票 03](../../.scratch/outbox-partition-key/issues/03-step-two-scope-eight-ports-and-four-undecided.md)悬置的问句：「同一案件的建立 → 申报提交 → 核对 → 关闭是否需要保序？」今天三口三个键公式，必然落三个分区，链上零顺序保证；且案件有两种身份表达（范围五维键与铸造 `CustomsCaseID`），链上两口各用一种，不统一改什么键都白改（取证：[evidence-four-undecided.md](../../.scratch/outbox-partition-key/evidence-four-undecided.md)）。
>
> 消费模型已由既有决定钉死：路由表显式清单（ADR-0049）、投影只增不改写且取代关系随来源给出（ADR-0065）、多对象信封消费侧拆分（ADR-0066）；两条 CC 消费路的载荷都是指针（只带键），处理方按键重读权威态，「按键重读不可见」登记为可重试未决哨兵。
>
> 业务侧（国际小包关务运营）的刚性要求是两条：下游读到的**当前案件状态可信**，与**审计链完整**——每份申报版本、每个关闭周期都必须到达且留痕；清关后稽查重开案件是常态（UC-CC-010 的多关闭周期即为此而写）。跨口到达顺序不在其中：因果先后在写入侧已被应用层钉死（申报只发生在已建立案件的进行中态），下游按键重读时读到的是当前态，不依赖信封先后。
>
> ## Decision
>
> **一、案件链四口不建立跨口同分区保序。** 本决定的成立前提有三，任一被打破须重裁：(1) 链上信封载荷保持指针式（只带键，不带可变业务态）；(2) 消费者按键重读权威态，不用信封先后拼装状态；(3) 「按键重读不可见」在消费侧归**可重试**未决哨兵，不归毒丸。
>
> **二、关闭信封身份携关闭周期维，分区键收窄到案件。** `caseClosureEventID = tenant/caseRef/关闭周期序数`（首次关闭为 1，重开后再次关闭递增；序数即「使当次关闭期成立的决定」的序号），`PartitionKey = tenant/caseRef`——同案各关闭周期同分区先后保序。今天域层只支持单周期，改动无行为差异；它拆掉的是 UC-CC-010 多周期落地那天 C2 与 C1 同 ID 被 `EnqueueOnce` 静默吞掉的引信（与 POD 更正票 02 同构）。
>
> **三、信封上的案件维引用统一用铸造 `CustomsCaseID`。** 五维范围键的职责收敛为**建案幂等**（「同一法律行为一案」），建立口 eventID 维持五维键不变；除此之外任何口要携带案件维（Subject、载荷、将来可能的分区维），一律用铸造 ID，不得再用范围键充当案件引用。建立口信封 Subject 已带铸造 ID，即现成形状。
>
> **四、申报口的案件维是建模欠账，不在本记录内解决。** 申报单元 → 案件的关联今天在域模型里缺席（文档「一个案件可以关联多个申报单元」只活在文档），另票排期；关联落地后申报意图在载荷/Subject 补案件引用（按第三条用铸造 ID），**不进分区键**。申报信封 ID 缺版本维一事继续归 `declaration-envelope-version-dedup` 票，两票互不吸收。
>
> ## Consequences
>
> - 门禁例外清单三行处置：建立行、申报行由「待裁」改写为「无先后」批注并引本记录；关闭行随第二条的修复删除（ID 与分区键不再同源，门禁自然放行）。
> - 关闭口修复与已落地的乙类同形（ID 加维＋分区键收窄到业务主体）；系统无生产数据，ID 公式变更无迁移负担。
> - 消费侧「不可见＝可重试」的分界从代码注释升格为本记录的成立前提；将来 CC 新增消费者时必须沿用，否则触发重裁。
>
> ## Alternatives considered
>
> - **四口统一案件分区（全链保序）。** 否决：申报口今天连案件维都取不出（关联缺席），建立口换键公式会丢掉建案幂等；换来的顺序保证在重读模型下没有任何现有或已规划消费者需要，而真正的业务不变量（版本不丢）它一条也没多保。
> - **关闭 ID 等 UC-CC-010 多周期实现时再改。** 否决：引信与拆弹分离，落地那天大概率忘——POD 更正被静默吞正是这样被发现的。
> - **案件维统一取五维范围键。** 否决：范围键是建案时的唯一性约束，不是引用身份；关闭口与建立口 Subject 已在用铸造 ID，反向统一改动更大且语义更差。
>
> ## Links
>
> - [UC-CC-010](../application/customs-compliance/UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md)：多关闭周期（AT-CC-327/337）
> - [ADR-0049](./0049-dispatch-routing-table-is-an-explicit-list.md)、[ADR-0065](./0065-projection-versions-are-append-only-and-supersession-is-source-given.md)、[ADR-0066](./0066-multi-object-envelope-unrolls-per-member-on-the-consumer-side.md)：消费模型三前提的出处

（README 索引条目一并补：`ADR-0069：关务案件链乱序由重读与重试消化，不由分区保证；关闭信封携关闭周期维`。）

### 随裁断一落地的代码改动

| 文件 | 改什么 |
|---|---|
| `internal/customscompliance/adapters/postgres/case_closure_handoff.go` | eventID 加周期序数维、PartitionKey 收窄为 `tenant/caseRef`、注释引 ADR-0069；序数今天恒为 1（域层单周期），取值方式随多周期实现定形但公式先钉死 |
| 同目录测试 | 断言新公式与分区键；两周期两封（用注入的序数）不同 ID 同分区 |
| `internal/architecture/envelope_partition_gate_test.go` | 关闭行删除；建立行、申报行改「无先后」批注引 ADR-0069（申报行保留 version-dedup 票引用） |
| 新票 | CC 申报单元 → 案件关联缺席（建模欠账，needs-triage） |

## 裁断二：同一载运对象允许第二次成功的对象级揽收登记；分区主体取到对象

**一句话**：退运再出、召回后再揽收是国际小包常态流程，产品机制不得假设租户为再入网换号；两次成功是同一条对象控制链上的先后两段，照 `transport_handover` 先例把分区主体取到对象。权威落点是 TF CONTEXT 与 UC-TF-002（补句），不另立 ADR；裁断记录落票 03 Comments。

### TF CONTEXT.md 补句（Rules · 场外揽收与末端派送，插在「场外揽收只有在……才建立履约参与关系」句后）

> 同一载运对象在前一段履约参与关系结束后，可以由新的履约尝试再次形成场外揽收成功，并按取得控制的边界开始新的履约参与关系；两段参与关系按时间先后构成该对象的控制链，各自保留成立与结束依据，后一段不重开、不覆盖、不吸收前一段。

（与既有句的分工：既有「同一实际控制范围不能因伙伴重投、任务重建或批量重试重复建立履约参与」管**同段去重**；本句管**跨段接续**。两句合起来，重复与再入网各有归属。）

### UC-TF-002 补句（一致性、幂等与并发一节，追加一行）

> - 同一载运对象跨尝试的再次成功揽收不是重复登记：对象级登记以（载运对象＋履约尝试）为身份逐拍成立，同一对象的各拍按发生先后构成控制链，登记与下游派发保序。

### UC-TF-002 新验收项（验收表追加）

> | `AT-TF-098` | 对象首段履约参与已结束（如退回交出方），改约后再次揽收成功 | 第二次成功按新尝试形成新的履约参与关系；两段按先后保留于同一对象控制链，对象级登记与下游派发保序，不覆盖首段 |

### 随裁断二落地的代码改动

| 文件 | 改什么 |
|---|---|
| `internal/transportfulfillment/adapters/postgres/offsite_pickup_registration_handoff.go` | `PartitionKey` 从 `=eventID` 收窄为 `tenant/object`（eventID 不变，幂等身份仍是对象+尝试+类型段）；注释引 `transport_handover` 先例「一个对象一条链」与本裁断 |
| 同目录测试 | 断言同对象两尝试两封：ID 不同、分区相同 |
| `internal/architecture/envelope_partition_gate_test.go` | offsite_pickup_registration 行删除（ID 与分区键不再同源） |
| PS 采用侧 | 同对象两份不同尝试来源输入的取舍：采用编排按 ADR-0060 重读当前已接受委托逐份判断，本裁断不改它；「后段成功是否顶替前段采用」留给采用语义，在票 03 Comments 记一句防丢 |

## 落地顺序（owner 确认后一笔一笔来）

1. ADR-0069 文件 + `docs/adr/README.md` 索引行（索引文件按占号协议先在频道说一声）。
2. TF CONTEXT.md 与 UC-TF-002 补句 + AT-TF-098。
3. 关闭口与揽收登记口两处代码修复 + 各自测试。
4. 门禁例外清单四行处置（两删两改写）。
5. 票 03 Comments 记裁断全文与 owner 授权出处，四处待裁清零后票转 resolved；新开申报关联欠账票。
6. 临时 worktree 按提交状态全门禁验证（含真库 PASS 非 SKIP），推已验 SHA。

## 等 owner 的点

- 确认裁断一（含三个成立前提与关闭周期序数公式）。
- 确认裁断二（含 CONTEXT/UC 三处措辞与 AT-TF-098）。
- 有措辞要改的，指句即可，改后再落。
