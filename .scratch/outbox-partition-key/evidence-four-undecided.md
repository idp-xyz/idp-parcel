# 四处待裁的裁断输入:只读取证(CC 案件链三口 + TF 场外揽收一口)

- 任务:`task-c0be1781-9b0c-441c-b414-95451c747383`(MCP-4,只读;除本文件外未改任何文件)
- 取证基线:origin/main `3b9f212`(detached 树检出,未读任何在途分支;MCP-2 的 OUTBOX-PK-STEP2 隔离分支不在取证对象内)
- 性质:**裁断输入,不是裁断**。四处待裁(票 [03](./issues/03-step-two-scope-eight-ports-and-four-undecided.md))PG/CC 无主,拍板另行成轮;此前四口在 `internal/architecture/envelope_partition_gate_test.go` 例外清单里的四行一行不删。

判据备取(票 [01](./issues/01-per-event-partition-keys-make-the-ordering-guarantee-vacuous.md) 原句):「分区由键值决定,不由 handoff 决定。两个 handoff 只要算出同一个键字符串,它们的信封就进同一分区并因此保序;算不出同一个键,改成业务键也白改。」

---

## A 关务案件链三口:`customs_case` / `declaration_submission` / `case_closure`

卡的一句(票 03):「同一案件的建立 → 申报提交 → 核对 → 关闭是否需要保序?」

### A1 文档硬句:案件生命周期的先后与并发

CC CONTEXT(`docs/domain/customs-compliance/CONTEXT.md`,Status: Confirmed):

- 案件定义:「围绕明确监管辖区、进出口方向、监管程序和法定义务范围建立的稳定业务案件。**一个案件可以关联多个申报单元和多次提交**」(Language·关务案件)。
- 身份固定:「关务案件建立时必须固定监管辖区、进出口方向、监管程序和法定义务范围;这些要素变化而形成独立监管义务时,必须建立新的关务案件并保留关系,不能原地改变案件身份。」(Rules·案件身份与申报范围首句)
- 生命周期序(Lifecycles·关务案件,逐句):
  - 「建立案件:固定监管辖区、进出口方向、监管程序和法定义务范围……」
  - 「**进行中:可以形成一个或多个申报单元、资料版本、提交**、监管决定、限制、税费结果和处置关系」——申报只发生在已建立案件的进行中态。
  - 「进行中 → 已关闭:只在当前关闭核对覆盖全部适用义务,且每项关闭依据均为已终结或已被有权接收方有效承接后,由有权责任角色形成关闭决定时成立。」——关闭在义务(含申报、核对)之后。
  - 「已关闭 → 重新打开:……原关闭记录和关闭期间事实继续保留。」
  - 「**重新打开 → 已关闭:必须重新盘点当次全部适用义务并形成新的关闭决定**;再次发生迟到事实时引用使最新关闭期成立的决定,不覆盖以前的关闭与重开周期。」
- 申报在案内:「监管规则允许的更正或补充**在原案件内**形成新的正式申报资料和提交版本,原提交及其结果永久保留。」(Rules·申报就绪、授权与提交)
- 核对(处置执行核对)定义:「`customs-compliance` 将节点、运输方或其他执行方提供的实际执行事实,与监管处置决定和适用证据规则所要求的维度进行的版本化比较判断。」(Language)——链上「核对」对应 `verification_handoff`(`disposition-verification.recorded`),该口另有独立成立的真风险已列甲类,不等本裁(票 03 明文)。

UC-CC-010(`docs/application/customs-compliance/UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md`):

- 并发节:「**已关闭、重开后再次关闭以及后续再次重开都形成新的决定和核对版本,历史关闭周期永久保留,不更新原关闭时间或原决定内容。**」
- `AT-CC-327`:「重开后再次完成义务并申请关闭 → 形成**新的关闭核对和关闭决定周期**,保留第一次关闭、重开和全部中间事实」。
- `AT-CC-337`:「原案已经历关闭 C1、重开、再次关闭 C2,随后又收到同程序迟到事实 → 新重开决定精确引用使当前关闭期成立的 C2 及受影响依据项;C1 和全部中间周期继续保留」——**C1 与 C2 是两份关闭决定**。
- 结果语义「已有关闭结果」的边界:「**相同请求和相同业务内容**已经形成关闭决定 → 返回已有……不得创建第二个决定或重置关闭时间」——它约束的是同请求重放,不是「案件曾经关过就永远只有一份」。
- 正常关闭步骤 8:「提交关闭决定、逐项依据、责任来源、审计和**发布意图**」——每个关闭决定带自己的发布意图。

### A2 代码事实:三口的信封 ID 与分区键公式(`3b9f212`)

| 口 | eventID 公式 | PartitionKey | Subject | 载荷 |
|---|---|---|---|---|
| `customs_case_handoff.go` | `tenant/jurisdiction/direction/procedure/obligation`(五维范围键) | =eventID | 铸造的 `Case.ID()` | 五维键 |
| `declaration_submission_handoff.go` | `tenant/unit/procedure`(幂等键三维) | =eventID | `Version.ID()` | 键三维+versionId |
| `case_closure_handoff.go` | `tenant/caseRef`(铸造 ID) | =eventID | CaseRef | tenant+caseRef |

三个公式互不相同 → 今天三口必然落三个分区,链上无任何顺序保证。且:

1. **案件有两种身份表达,链上两口各用一种。** `CustomsCaseKey` 是固定监管范围五维(ports.go 注释:「固定监管范围四维——同一法律行为一案」);`CustomsCaseID` 由 `EstablishCaseHandler` 经 `deps.Identity.MintCaseID(ctx)` **铸造**,从五维键推不出来。建立口的信封键用范围键(铸造 ID 只在 Subject),关闭口用铸造 ID。票 01 那句「各自改成业务键也白改」在此有具体形状:即便两口都改取"业务键",一个取范围键、一个取铸造 ID,仍不同分区——保序要求先统一"案件维"用哪种表达(建立口信封的 Subject 已携带铸造 ID,是现成的对齐材料;此为事实陈述,非方案)。
2. **申报口从信封里取不出任何案件维。** `DeclarationSubmissionKey` 三维(租户+申报单元+程序)与载荷均无案件引用;`submit_declaration.go` 全文对 `Case`/`案件` 零命中,申报单元→案件的关联在代码里不存在(文档上「一个案件可以关联多个申报单元」,该关联今天只活在文档)。要把申报信封归入案件分区,得先有 unit→case 的可查关联或在意图里携带案件引用——两者今天都没有。
3. 申报口 ID 缺版本维一事另有票(`.scratch/declaration-envelope-version-dedup/issues/01`,核证结论:机制坐实、今天无触发路径),本文件不重复。

例外清单注记原文(`envelope_partition_gate_test.go`,`allowedSameExpression`):

> 关务案件链是四个 handoff 各出一封(建立 → 申报提交 → 核对 → 关闭),而分区由键值定、不由 handoff 定:四口不改成同一个键公式就落不进同一分区,各自改成业务键也白改。这一句本上下文当前无主,三行因此待裁。verification 与 gate 另有各自独立成立的真风险,不等这一裁。

三行各自的尾注:关闭行「且 CloseCustomsCaseHandler.Handle 写明『重开走 Reopen』,重开后再关会撞同一 ID」;建立行「本口是链首,它取什么键公式决定了另外三口得跟着取什么」;申报行「ID 缺版本维一事已另有票……今天无触发路径」。

### A3 消费侧现状:按 `3b9f212` 重取证(对照 [outbox-handoff-consumption-map/report.md](../outbox-handoff-consumption-map/report.md))

report.md 的第 4 栏取证于 `60ea63c`(路由表两条,CC 21/22/29 三行均「应有但未开」),其保质期声明明写「消费本表前若 HEAD 已前进……应按新 HEAD 再取一次证」。新 HEAD 重取证结果:

- 路由表已长到**十二类**(`cmd/parcel-dispatch/assemble.go` 的 `wireDispatcher`,publisher map 逐条在 `NewDirectPublisher` 处)。其中 CC 两口已有消费者:
  - `customs-compliance.customs-case.established` → `visibility-exception/derive-projection-from-customs-case`(只投 VE,不 FanOut;装配注释:「案件建立是 CC 自家责任容器事实,PS 侧今天没有它的消费者——登记接不住的比不登记更糟(ADR-0049 第三条)」)。多成员信封按 ADR-0066 在消费侧循环拆分;载荷是五维键指针,案件本体由处理方按键重读(消费者注释:「权威事实留在 customs-compliance」)。
  - `customs-compliance.declaration-submission.formed` → `visibility-exception/derive-projection-from-declaration-submission`(同为只投 VE)。
- **`customs-compliance.case-closure.recorded` 仍无消费者**;`disposition-verification.recorded`(核对口)同样不在 publisher map。按 ADR-0049 第三条,这两类一旦发布即撞 `dispatch.no_subscriber`,显式失败并阻塞其分区,预算耗尽进 `ABANDONED`(有界,留痕)。
- 端口注释 `CaseClosureHandoffIntent`:「VE 的案件视图与**治理审计**消费它」——「治理审计」段查无文档出处(report.md 矛盾清单第 5 条),本次复核仍成立。
- 未决哨兵分格(ADR-0049 的重试/毒丸在这两路的落法,`assemble.go`):
  - 可重试(登记为未决哨兵):`ErrCustomsCaseNotVisible` / `ErrDeclarationSubmissionNotVisible`(按键重读不可见)、`ErrProjectionUndecided`。
  - 响亮失败(不在名单,塌 `dispatch.publish_failed`):`ErrCustomsCaseRecordInconsistent`(仓储不变量已破)、`ErrUntranslatableAnswer`(词汇表外)、`ErrProjectionHandoffPending`(要查 outbox 下游)。
  - 毒丸:「`veinbox.ErrPoisonEnvelope` 由消费门入账后交 nil,不要当未决哨兵」——五维键缺字段即毒丸(消费者注释:「缺了永远取不着,而重投同样内容不会长出字段来」),显式拒收入账,不重投。

### A4 子问题:「重开走 Reopen 不走这里」与重开后再关的撞 ID 路径

代码链(`3b9f212`,逐环):

1. `CloseCustomsCaseHandler.Handle` doc 注释原文:「已关案件重放返原关闭(**一案至多一份关闭记录,重开走 Reopen 不走这里**)→ 义务清单读取 → VerifyClosure 逐项核对 → 任一未解决阻止关闭带清单 → CloseCase 决定 → 提交与意图。」实现:`FindByCase` 查到已有关闭即答 `ALREADY_CLOSED` 并**重发同一份意图**(`alreadyClosed` → `handOffClosure`),不看重开与否。
2. `ports.CaseClosureStore` 注释:「一案至多一份关闭记录(**重开追加在记录内**)」;接口只有 `FindByCase` 与 `Save`,**没有 Update**。
3. `domain.CustomsCaseClosure.Reopen`:只往 `reopenings` 追加 `ControlledReopening`,「原关闭记录不动(closedAt 与 verification 原样)」。
4. **`Reopen` 在应用层零调用**:grep 全上下文,`Reopen` 只出现在 domain、适配器(序列化)、两者的测试与 Close 处理器的那句注释里——没有任何 handler 提供重开入口。
5. **重开没有自己的 handoff**:CC 九个 handoff 里无 reopen 口——即便重开落了库,下游也收不到任何信封。

叠加起来的现状,分两层照实记:

- **撞 ID 的机制成立,但今天无触发路径。** 若按 UC-CC-010(A1 引文:重开后再关「形成新的关闭核对和关闭决定周期」)实现第二份关闭决定 C2,今天的 `caseClosureEventID(tenant, caseRef)` 没有周期/决定维,C2 与 C1 算出同一 `(source, event_id)`,`outboxintent.EnqueueOnce` 查到已入队即静默返回——C2 对下游不存在。形状与票 [02](./issues/02-pod-correction-is-silently-swallowed-by-enqueue-once.md) 的 POD 更正被吞完全同构(那口的修法是 ID 加版本维)。
- **无触发路径的原因不是安全,而是代码还到不了那一步。** 文档要求多关闭周期(AT-CC-327/337),代码三处一致地只支持单份关闭(端口注释、域对象形状、Handle 的 ALREADY_CLOSED 短路),且重开既无入口、追加的 reopenings 也无持久化写口(店无 Update)。**Reopen 不改变可入队身份**:它不产生信封、不改 caseRef、关闭信封 ID 里没有任何随重开变化的维。裁案件链保序时,这个「文档要求 vs 代码单份」的缺口与信封维度缺失是同一处的两层,票 03 已要求一并答。

### A5 乱序后果逐口(以今天的消费者与 ADR-0049 分格)

前提:A2 已证三口三公式三分区,跨口零顺序保证;同口之内今天各是单条(建立/关闭一案一份;申报同键一份,版本维缺失另票)。载荷全部是指针式(只带键),消费者按键重读当前态——不存在「后一封改写前一封载荷」的通道,风险集中在「引用对象未达」与「重读读到晚于信封时点的状态」两种形状。

| 乱序形状 | 应然/现有消费者读到什么 | ADR-0049 分格 | 自愈还是丢 |
|---|---|---|---|
| 申报先于建立到达(两口都投 VE 投影) | 两路消费互不引用对方产物,各自按键重读 CC 库本体(生产侧同事务落库,自读必可见);申报投影先于案件投影形成 | 若消费方重读不可见:`*NotVisible` 已登记为**可重试**未决哨兵 → 重投,预算内自愈 | 自愈(预算内);后果限于 VE 投影版本的形成顺序不定——ADR-0065 投影只增不改写,顺序不定不产生改写,但读方看到的先后可能倒置 |
| 关闭先于建立/申报(今天) | 关闭**无消费者**:发布即 `dispatch.no_subscriber`,显式失败阻塞该分区,预算耗尽进 `ABANDONED` | ADR-0049 第三条(「路由表是显式清单,没有订阅者的事件类型显式失败并入账,不静默丢弃」);其 Consequences:「失败预算耗尽后该条进 ABANDONED,分区随即解冻,事件留在库里可查」 | 不消费,留痕可查;不属乱序丢失 |
| 关闭先于建立(将来 VE 案件视图接了关闭口) | 消费者按 caseRef 重读:案件未达则「引用对象未达」 | 归可重试 → 重投自愈;归毒丸 → 显式拒收入账,那封关闭对处理而言**丢**(留痕) | 取决于消费者把这一格归哪边——正是票 03 末尾「依赖前序两口」点名的同一条未定约束 |
| C2 关闭被 EnqueueOnce 吞掉(A4) | 下游只见 C1,永远不知道最新关闭期 | 不进派发——**在入队处就没了**,不属投递乱序,ADR-0049 各格都碰不到它 | 丢且无痕(EnqueueOnce 返回 nil,编排看到「交接成功」) |
| 核对(verification)与其它口互序 | 核对口无消费者(同 no_subscriber);其真风险(事实集指纹换版)在甲类已单独处理 | 同关闭口 | 不等本裁 |

毒丸判据的现状(供裁「引用对象未达」归格时参考):两个 VE 消费者今天只把**载荷缺字段/译不出**判毒丸;「按键重读不可见」判可重试。这个分界写在消费者与装配注释里,不在任何 CONTEXT/UC 文档里。

---

## B 场外揽收登记一口:`offsite_pickup_registration_handoff.go`

卡的一句(票 03):「同一载运对象能否出现第二次成功的对象级揽收登记?」

### B1 文档硬句:对象级登记与尝试维

UC-TF-002(`docs/application/transport-fulfillment/UC-TF-002-PERFORM-OFFSITE-PICKUP.md`):

- 「到 `transport-fulfillment` 建立揽收任务、记录每次实际尝试,并**逐载运对象**形成场外揽收成功、失败、拒收或待确认结果为止。」
- 「后续重试、改约或成功揽收是**新的尝试**和新的成本来源范围,不能覆盖第一次失败尝试,也不能把多个尝试压缩为一笔无来源成本。」
- 幂等三句:「同一尝试来源身份和内容返回已有结果;同一身份不同内容形成冲突」「**同一对象在同一尝试中只能有一个当前有效结果**;更正或来源撤销形成新判断版本」「**同一实际控制范围**不能因伙伴重投、任务重建或批量重试重复建立履约参与」。
- `AT-TF-021`:「首次尝试失败,后续改约后第二次成功 → 两次尝试都保留,成功只在第二次形成控制」;`AT-TF-094` 同形(失败尝试费一节)。
- 未受理句:「提交自身矛盾或最小身份不全(如**成功缺控制依据**、失败带控制依据、对象集为空),未进入处理」——失败尝试没有控制证据,**登记口只登成功**(`register_offsite_pickup.go` 注释同句:「失败到访没有控制证据可供——缺控制在受理处就是未受理」)。

TF CONTEXT(`docs/domain/transport-fulfillment/CONTEXT.md`):

- 「每次实际上门形成新的履约尝试。尝试必须保存计划窗口、实际到场、执行方、地点、对象范围、证据和失败原因;改约、**再次揽收**或重新派送不能重开或覆盖旧尝试。」——「再次揽收」是硬句词汇。
- 「场外揽收只有在明确载运对象形成有效收寄或权威交接**并由运输方取得控制**时,才建立履约参与关系。」
- 「实际履约段持续到载运对象形成有效交付、下一次权威交接或**其他明确结束当前运输控制的有效事实**。」——控制可以结束;结束后同对象再次被揽收,文档没有禁止句。
- 交接更正一句(控制链语义):「必须**重算整段控制来源链**并保留后续事实;不能直接把当前控制回退给早期交出方。」

**判读输入(不裁)**:文档明文允许同对象多尝试、允许「再次揽收」,且控制有明确的结束事实;对「同一对象第二次*成功*登记」既无禁止句、也无明许句。票 03 的问句悬在这个空档上。

### B2 代码事实:登记键、幂等语义与三层都不挡

- **键形状**:`OffsitePickupKey = (TenantID, Object, Attempt)`——尝试维在键里。信封 `eventID = tenant/object/attempt + "/offsite-pickup-registration"`(注释:类型段把本口与交付生效错开,「避免 EnqueueOnce 把另一口的已入队当成『同一份』」);`PartitionKey = eventID`(逐事件分区);载荷=键三维。
- **幂等语义**(`register_offsite_pickup.go`,唯一入口 `Register`):`FindByKey` 同键同内容指纹=重放(`EXISTING_VERSION` + 重发同一意图);同键异指纹=`SOURCE_CONFLICT` **不落库**(注释:「同一(对象+尝试)携带不同控制/地点/时间:首登不顶替,来源更正走新版本」);无更正、撤销或对象级查重入口。→ 票 02 自查句「**一个键**只会有一份意图」逐字成立;票 03 指出的余地同样成立——键含尝试维,**同对象换尝试即全新键**,走 `PickupRegistered` 全流程。
- **两次成功登记今天挡不挡**,三层逐层:
  - 域层:`FormOffsitePickup` 校验的是七件输入完整性,无「一对象至多一次成功」不变量;
  - 应用层:`Register` 只按(对象+尝试)查重,不按对象查;
  - 库层:`transport_fulfillment.offsite_pickup` 的 `PRIMARY KEY (tenant_id, object_ref, attempt_ref)`(迁移 `0005_pickup_handover_registries_and_delivery_attempt.sql`)——(对象, 尝试₂) 是合法新行。
  - **结论性事实:三层都不挡。** 若第二次成功登记发生,两封信封 ID 不同、分区不同,互无顺序保证。
- **消费侧**(`3b9f212`,对照 report.md 第 19 行「应有但未开」已过时):`transport-fulfillment.offsite-pickup.registered` 已有 **FanOut 双消费者**——先 VE 揽收投影,再 PS 来源采用。装配注释(`adoptOffsitePickupConsumer`):「按(租户+对象+尝试)重读登记 → 按包裹反查当前已接受委托(ADR-0060)→ 同一个来源采用编排」;「只接对象级的 `offsite-pickup.registered`,不接尝试级的 `offsite-pickup.formed`:后者一封信带一批成功对象,而采用判断逐对象成立,两条都登记会让同一份揽收结果被采用两次」。哨兵:可重试=`ErrPickupNotVisible`/`ErrParcelTargetNotFound`/`ErrAdoptionUndecided`;响亮=`ErrPickupRecordInconsistent`/`ErrAdoptionHandoffPending`/`ErrAmbiguousParcelTarget`。若两次成功登记成立,PS 采用编排会收到**两份不同尝试的来源输入**,而两封信封分属两个分区、到达先后不定——采用侧如何分辨先后与取舍,属裁断范围,此处只记通道形状。

### B3 先例对照:`transport_handover` 把主体取到对象的理由原文

`transport_handover_registration_handoff.go` 两段注释,逐字:

> transportHandoverPartitionKey 取(租户+载运对象),不取整个判断键。
>
> 分区键与信封 ID 管的不是一回事:ID 管幂等(每个判断版本一份意图,更正因而不丢),分区键管顺序(同一对象的先后拍排队)。把 ID 直接当分区键会让每份信封自成一个分区,框架的顺序保证于是落空——更正版本可以先于它更正的那一版送达。
>
> 主体取到对象而不取到(对象+范围):**控制转移对一个载运对象是一条链,先从节点交出、再由承运方接收,两次交接分属不同范围却必须保序。取到范围就把链切成互不排队的两段**,而 node-operations 的控制转移正是按这条链推进的。

键对照:

| 口 | 信封 ID | PartitionKey |
|---|---|---|
| `transport_handover_registration` | `tenant/object/scope/version` | `tenant/object` |
| `offsite_pickup_registration` | `tenant/object/attempt/类型段` | =eventID(逐事件) |

例外清单注记原文(`offsite_pickup_registration_handoff.go` 行):

> 同一载运对象能否出现第二次成功的对象级揽收登记——能则两次是同一条控制链的先后拍,而交接登记那口已按「一个对象一条链」把主体取到对象

---

## 与票面的出入(照实记,不修)

1. **消费图 report.md 的三行现状已过时**:第 21 行(customs-case)、22 行(declaration-submission)在 `3b9f212` 已有 VE 投影消费者,第 19 行(offsite-pickup.registered)已有 VE+PS FanOut 双消费者。report.md 自己的保质期声明预告了这类失效;其文档判据栏(CONTEXT-MAP 边、UC)不受影响。`case-closure.recorded` 与 `disposition-verification.recorded` 维持无消费者。
2. **「治理审计」消费段仍查无文档出处**(report.md 矛盾清单第 5 条维持成立),现存唯一出处是 `ports.CaseClosureHandoffIntent` 注释。
3. **UC-CC-010 与代码存在文档-代码张力**:文档要求重开后再关「形成新的决定和核对版本」且「历史关闭周期永久保留」(AT-CC-327/337),代码三处一致只支持一案一份关闭记录(端口注释、`CustomsCaseClosure` 形状、`Handle` 的 ALREADY_CLOSED 短路),重开无应用入口、无持久化写口(店无 Update)、无信封。撞 ID 子问题(A4)嵌在这个更大缺口里:机制坐实、今天无触发路径,引信是把 UC-CC-010 的多周期真正实现出来的那一天。
4. **票 02 的「一个键只会有一份意图」与票 03 的存疑并存且都对**:按键(含尝试维)的幂等语义逐字成立;同对象跨尝试的第二次成功登记在域/应用/库三层都不被阻止(B2),文档对此既无禁止句也无明许句(B1)。

## 取证方法与保质期

- 代码与文档引文一律取自 detached 树检出的 origin/main `3b9f212`;共享树本地 main 落后属已知状态,未作为取证对象。
- 本文所有「今天/现状」断言随 HEAD 前进失效,消费侧与例外清单尤其如此(MCP-2 正在同票八口上动刀);文档硬句(CONTEXT、UC)不随代码变化。
- 复核入口:三口 CC handoff 与 TF 两口的公式看各自 `*_handoff.go` 的 `*EventID`/`PartitionKey` 字段;路由表看 `cmd/parcel-dispatch/assemble.go` 的 `NewDirectPublisher` map;例外清单看 `internal/architecture/envelope_partition_gate_test.go` 的 `allowedSameExpression`。
