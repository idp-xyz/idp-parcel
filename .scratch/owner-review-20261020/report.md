# 代决复核报告 · 2026-10-20 · 钉 `9045b6fd`

这份报告是 Claude 会话对自己代决的复核，**不是 owner 复核**。它不改变任何 ADR 的状态，也没有在任何原文旁写「owner 复核」。

**范围**：38 份代决 ADR 中除 ADR-0139 以外的 37 份（ADR-0139 已由用户亲自接受），即 ADR-0114、0149、0151、0183、0184、0186、0189、0190、0195、0196，以及 ADR-0198 至 0224。此外还复核了今天几张没有 ADR 的票和分诊裁定：[infra-speed/01] 至 [infra-speed/04] 与 [infra-speed/08] 的代决，以及 [triage-2026-10-19] 的分诊结果和 2026-10-20 的上线就绪评估。

**方法**：七组复核都在 main `9045b6fd` 上只读完成。复核单位是越权风险点、没有显式风险点的决定、复核记录和裁定，共 418 条：

- **须你裁定** 15 条，合并重复后 14 问
- **应修订** 26 条，合并修法相同的条目后 23 项
- **维持** 377 条

R1–R14、M1–M23 只是本报告的条目号。上线就绪评估的分级本报告一律写作「分级 R1/R2/R3」，两者无关。

## 须你裁定 → 已代决（2026-10-20）

用户 2026-10-20 原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」，以下十四问由本会话代决。这是代决，不是 owner 本人复核；每条的越权风险点就是「owner 若持相反意见，要撤回的是什么」，写在裁定里。立票号指 `.scratch/demo-chain-gaps/issues/`。

| 问 | 出处 | 问题 | 裁定 | 理由与后续 |
|---|---|---|---|---|
| R1 | ADR-0149 决定一、三 | 生产上是否允许租户的财务、关务人员在管理台手工采用外部资金事实与监管凭证（操作者渠道另开入口，按决定种类单独授予、记提交操作者，与集成客户端族并存）？ | **是** | 多数租户没有银行或报关行接口，照单人工录入是常态；单独授予加留痕把资金控制风险收在可审计范围。现状管理台三张签走集成客户端认证、实际用不了。立票 140（上线前必修）。越权风险点：若 owner 答否，撤掉那三张管理台签即可 |
| R2 | ADR-0190 越权风险点 8（ADR-0179 同此） | 计价基准时点是否认定为产品策略、由内置时点给出？ | **是** | 内置时点本身是业务时点，守住了规则要防的东西；价卡没有声明这一格的位置，硬要声明只会每张卡答未配置。立票 141 改 SA CONTEXT 与 GLOSSARY 的对应句 |
| R3 | ADR-0201 决定七 | `network-routing.initial-route.formed` 扇出给 VE 与 TF 两个消费者？ | **是** | NR 只多一行路由，语义不变；放大量计入 PAR-NFR-03。无后续 |
| R4 | ADR-0201 决定二 | NR 是否认这两条为对 TF 的契约：计划段窗口依据里恰有一格 `CONNECTION/<代码>@<版本>`；同一连接代码跨版本只改起讫、含义变就换代码？ | **是** | TF 准备与出发门禁读数都依赖它们。立票 142：NR 侧加一条守这个拼写的用例 |
| R5 | ADR-0201 越权风险点 28 | 采用判断处于待复核时，TF 把 PS 的「未定」当终局（只剩件数、重量体积记待补充），复核出结论后不自动重备？ | **是** | 复核没有时限，改答未决会把初始路由信封烧到 ABANDONED，更糟。无后续（重备归 36、44） |
| R6 | ADR-0215 越权风险点 1 | 接受 NR 经 PS 现有读法读回整份委托聚合、只取受理基线地理维（部分停用 ADR-0075 决定一）？ | **是，作为过渡** | 立票 143：PS 开「受理基线地理投影」端口，NR 进程不再加载收件人身份与地址本体 |
| R7 | ADR-0223 越权风险点 6、7 | 首方契约把包裹内部标识（`parcels[].parcelId`）定为客户长期持有的键、用于客户追踪视图定位？ | **是** | 标识永不复用、同一物理件不变；首方 API 发布兼容基线前仍可回头；按客户参考号定位由 119 增量补。无后续 |
| R8 | ADR-0202 决定二 | 承运答复口（`POST /transport-fulfillment/booking-answers`、五值 outcome、每次订舱至多一份候补加一份定局）按现形作为对承运商的首版集成契约？ | **是** | 首发前无真实承运商接入（证据 S），以后扩形只加取值。立票 144：集成说明写明候补位次更新、候补后分批接受目前进不来 |
| R9 | ADR-0202 越权风险点 14 | 对不是本服务方的承运商，是否继续把 `SOURCE_NOT_SERVICE_PROVIDER` 与 `BOOKING_NOT_FOUND` 分两格作答？ | **否**，一律答 `BOOKING_NOT_FOUND` | 同一租户的承运商之间常是竞争方，两种情形下承运方的续办都是找租户核对，分格只多泄露一位信息。立票 145，须在首版集成契约发布前落地 |
| R10 | ADR-0204 越权风险点 2；票 57 | 解除采用后，委托再无别的已采用段时，撤回「已开始」回到「已提交」（以更正事实记录，原开始与解除都留历史）？ | **是** | 那次开始派生自后来证明错误的采用连线，不是已发生的控制事实；不撤回则错采用的委托永远取消不了。还有别的已采用段时，开始改按其余段最早首个参与重新派生——触及 ADR-0204 决定二「只前移不后移」，与决定一末两句一并在 57 的新 ADR 里部分停用。并入票 57 |
| R11 | ADR-0219 越权风险点 5（连带 ADR-0205 越权风险点 13） | 出发前门禁问询与出发两口回给设备的逐对象那一格，趁一线客户端未接入，两口一起收窄成只列被阻断或未决的对象（落册仍记全）？ | **是**，并升为上线前必修 | 今天持 OFFSITE 授予的任一设备能对本租户任一段、任一时点问出段内全部对象（含别的客户的包裹）且不留痕——这是数据暴露。立票 146，排在 42 之后（同一读数内核） |
| R12 | ADR-0206 越权风险点 12（ADR-0213 越权风险点 7 同根） | 对已登记过至少一个节点关务区域的租户，不在册对象起讫有一端未登区域时，跨区那一道改答「未配置」（指名未登的节点），而不是不列？ | **是** | 纯境内租户不受影响；跨境租户漏登新节点时，静默放过变成指名的未配置。立票 147，排在关务在途票（12、psb/18）之后 |
| R13 | ADR-0212 决定十 | 首发接受：没有预报、也没有全部确认的交接单时，客户包裹件号送站一律落待识别，不另开自动识别的路？ | **是** | 已有「节点开送站交接单、经运营决定确认」这条路；错关联只能追加、PS 采用不回滚，代价远大于多一件待识别。无后续 |
| R14 | ADR-0213 越权风险点 20 | 派送出发前的交付门禁问询（票 94）列为上线前必修？ | **是** | 不做则交付侧关务门禁只能事后记账，受限或被扣的包裹照样交到收件人手里。票 94 升为上线前必修，排在 42 之后（照 ADR-0219 同形） |

## 应修订 → 去向（2026-10-20）

二选一的几格由本会话一并代决（同上节授权）：

- **M15（票 14）取第一条**：BUY 触发册纳入修订形，至少允许给费用项目为空的行追加一版带费用项目的登记，取代 ADR-0208 决定三「册不设修订」那句；「原因已登、费用项目未登」因此也能经「参数已登记」续办。
- **M17（票 50）取第一条**：上线前最小面扩成「区间截止，加上同起点重发时按决定引用退役」（领域已有 `Retire`），退役写口所需的 ADR 随之提前到上线前必修。理由：票 50 自己的动因与「更正错登的合同」多是同起点重发，排除它等于最小面不覆盖最常见的那一格。
- **M14（票 104）**：区域来源二选一属产品策略，留给 104 的分诊，由唯一协调方定，本节不代。

| 项 | 去向 | 分级 |
|---|---|---|
| M1 | 新票 126 | 上线前必修（真实租户上线后，没记下的决定无法补记） |
| M2 | 并入票 70 | 随 70 |
| M3 | 新票 127 | 上线后尽快 |
| M4 | 新票 128 | 文档 |
| M5 | 改票 121：撤掉用户决定阻断，按机制自决 | 上线后尽快 |
| M6 | 票 122（42 进 main 时自立，「交出读数不展开集运单元成员」）；ADR-0213 复核记录指向它 | 随 122 |
| M7 | 第一件与 M8 合为新票 129；第二件新票 130 | 129 上线前必修，130 上线后尽快 |
| M8 | 新票 129 | 上线前必修（每次证据更正都会自动另立第二项派送任务） |
| M9 | 改票 57 与分诊表 | 随 57 |
| M10 | 新票 131 | 上线前必修（与 14 同族） |
| M11 | 新票 132 | 上线前必修（较早触发可能据以失效或改路，违反「按业务时间裁决」） |
| M12 | 新票 133 | 上线后尽快 |
| M13 | 新票 134 | 文档 |
| M14 | 票 104 分诊 | — |
| M15 | 并入票 14（在途，经协调方转作者），裁定见上 | 随 14 |
| M16 | 并入票 50（在途，经协调方转作者） | 随 50 |
| M17 | 并入票 50（在途），裁定见上 | 随 50 |
| M18 | 新票 135 | 上线后尽快 |
| M19 | 新票 136 | 上线后尽快 |
| M20 | 新票 137 | 文档 |
| M21 | 新票 138 | 文档 |
| M22 | 本会话直接改 `docs/agents/parallel-sessions.md` | 已办 |
| M23 | 新票 139 | 文档与工具 |

十四问裁定另立的票：140（R1）、141（R2）、142（R4）、143（R6）、144（R8）、145（R9）、146（R11）、147（R12）；票 94 按 R14 升为上线前必修。

## 应修订

以下按建议立票的上下文分组。其中 M3、M7、M8、M10、M11、M12、M16、M17 是读代码证实的实缺陷；其余是文档前后相抵、指针失效、票面缺口，或分诊裁定不完整。

### TF

**M1**｜ADR-0151 越权风险点 5
- 问题：TF 的运营决定各口不记是谁做的决定。目前只有费用发生登记把操作者落成 `RegisteredBy`；关段、建派送任务、装载分配、终止参与、两个判断口和运输委托提交，都只收租户。PS 那边已由 ADR-0187 记下提交操作者。真实租户上线后，没记下的决定无法补记。
- 修法：出新 ADR，或在 ADR-0151 补复核记录并立票，定为「记」：把信封里的提交操作者作为操作证据记在决定记录上，待遇照 ADR-0187（只进审计，不进重放比对，也不进任何门）。在首个租户上线前落地，关掉风险点 5。
- 立票：TF，新票。

**M2**｜ADR-0151 越权风险点 7
- 问题：派送补拍口仍挂在 `workFactFace(WorkScopeOffsite)` 上（`endpoints_delivery_dispatch.go`），调度员要先有一台登记在场外作业范围的设备才能补拍。风险点依据的前提是「调用方是调度拍」，这一前提已被 ADR-0216 推翻。ADR-0216 决定四又写「渠道、Intake 不变」，两份记录在这一口上没有对齐。
- 修法：在 [demo-chain-gaps/70] 裁「所有者补齐要求后，谁来再发起」时，一并定这一口归哪一族。若由管理台调度员补拍：出新 ADR，把这一口移到运营决定能力面（加一个决定种类），并部分停用 ADR-0151 决定六末句和 ADR-0216 决定四那句。若只由一线设备补拍：在 ADR-0151 补复核记录，改写风险点 7 的前提。
- 立票：TF，并入 [demo-chain-gaps/70]。

**M3**｜ADR-0203 越权风险点 7
- 问题：准备消费者在 inbox 事务里经 `ListPoolsOnSchedules`、再经 `FindByKey` 读池，而 `FindByKey` 一律加 `FOR UPDATE`。结果是每封初始路由信封都把候选班次全部池的行锁一直持到整封提交，准备与预占、装载分配彼此串行。准备本身只算不存，不需要这把锁。风险点要求「票 18 合入时二选一并写进票面」，这一条没有兑现。
- 修法：给准备评估另开一个不加锁的池读口，只供 `PrepareOnPlannedLegsHandler` 使用；装载分配仍走加锁的那一口。若决定接受串行，就在 ADR-0201 的票 18 补记里写明这一点及其吞吐代价。
- 立票：TF，新票。

**M4**｜ADR-0204 决定二
- 问题：UC-TF-004 的验收表里 AT-TF-181 有新旧两行并存。旧行写「不改写首次开始」，与决定二「开始前移到更早的那一条」相抵。
- 修法：删掉写着「不改写首次开始」的那一行，并重算 [at-coverage-inventory] 的计数。
- 立票：TF 文档小改，可以不立票。

**M5**｜ADR-0205 越权风险点 2
- 问题：关务读不动时，出发照记、没有补问。承接这一格的 [demo-chain-gaps/121] 票面挂着「Blocked by 用户决定」，可票面自己把补问归为机制、把唤醒与次数归为产品策略、租户取值为无。按 AGENTS 的三类，这些都该由开发方现在就定。这个假阻断一直挂着，关务读不动期间的出发（包括本该阻断的那些）就永远不会补问，也不会形成信号。
- 修法：撤掉 121 的用户决定阻断。前置条件改为先把段册、班次册的「装不回」与「连不上」分开，这一步并进 [demo-chain-gaps/114] 或在 121 内做。两问按机制自决：补问追加一版读数，不改首版；补问得到的阻断信号以出发时点为业务时间。分诊时按合规后果至少归分级 R2。
- 立票：TF 与 CC，改 [demo-chain-gaps/121] 的票面。

**M6**｜ADR-0213 越权风险点 11
- 问题：交接对象是袋或集运单元时，境内交出答「不受门禁」，这是一格假的干净读数。风险点把成员展开指向 [demo-chain-gaps/42]，但 42 的现象、修法和完成判据只覆盖出发门禁读数。42 进 main 之后，交出读数的假干净格仍然在。
- 修法：在 42 的地盘和完成判据里补上交出门禁读数。交出与出发共用 `domain/gate_reading.go` 内核和同一个 NO 集运成员读口。或者另立一张同批票排在 42 之后。同时在 ADR-0213 文末补记，把「等票 42」改指真正承接的那张票。
- 立票：TF，并入 [demo-chain-gaps/42]。

**M7**｜ADR-0216 越权风险点 2
- 问题：`CorrectWithGateReading` 只走重派生这一条路。前版是待确认、本版是已交接时，它答 `NO_PARTICIPATION_TO_REDERIVE`：对象既不结束先前段的参与，也不进新段。这与 TF CONTEXT「已交接使控制转入接收方」相抵。派送一路会替对象已经交出的先前派送段形成任务，这是它的下游后果；[demo-chain-gaps/60] 评审原本建议进 main 前堵住。
- 修法：分两件做。第一件是最小堵法：执行器命令加一个可选的「期望入场依据」，由 `TriggerDispatchOnHandover` 传入本版交接按 ADR-0221 铸出的依据；链尾入场依据不是它时，答「不是触发事实」。第二件另立票：前版是待确认或拒收、本版是已交接的交接更正，改走进段（结束先前段的参与，进入新段），先例是 [label-channel-service-first-release/31] 的分路。
- 立票：TF，两张票。

**M8**｜ADR-0216 越权风险点 7
- 问题：仍为已交接的更正会换一个入场依据，而执行器按（段，对象，链尾入场依据）铸任务引用，于是进程内会自动另立第二项派送任务，前一项不动。TF CONTEXT 的内部触发只认「载运对象凭已交接进入派送段」，同一段的替代版本不是进段。这一缺口让以前只在手工补拍时出现的重复任务，变成每次证据更正都会自动发生。
- 修法：在执行器查在册的那一步，沿参与替代链回看：链尾回指的前版参与如果已经形成任务，就答「已在册」并交回那一项，不另铸。[demo-chain-gaps/70] 的派送待形成册按同一认法处理。
- 立票：TF，可与 M7 的第一件同票。

**M9**｜[triage-2026-10-19] 对 [demo-chain-gaps/57] 的裁定（完整性）
- 问题一：裁定推翻了 ADR-0204 决定一「采用之后不改、不解除」，却没有写须新 ADR；同批的 14、70、88 都写了。
- 问题二：票 57 要裁的有五问，裁定只答了第一问。首个参与被更正后是否重新派生、取消的更正、「取消送达服务方」事实这三问都没答，Blocked by 改成「无」、转 ready-for-agent 为时过早。
- 问题三：裁定要求「已形成的发生项先走发生项失效」，可这条路在生产上不存在：TF 的 `ReviseValidity` 没有生产调用方。88 同批落地后，错采用的已结束段会随门铃立即形成发生项，解除随即被挡，评估理由里说的「改不回」仍然改不回。
- 修法：在裁定里补三件。其一，写明须新 ADR，部分停用 ADR-0204 决定一的末两句；R10 若答「是」，连带决定二。其二，把 TF 发生项失效的生产入口纳入第 3 批，并按分诊自己的注把 [demo-chain-gaps/15] 升 A，或写明成本纠错暂缺时的绕法；门铃消费者形成发生项之前，先复核采用是否仍在册。其三，其余三问本次补裁，或拆成 needs-triage 子票，57 只留解除采用。
- 立票：TF，改 [demo-chain-gaps/57] 与 [triage-2026-10-19]。

### NR

**M10**｜ADR-0215 越权风险点 9、10（两条合并）
- 问题：`reassessmentToConsumption` 不分原因，把复核未决一律折成重投哨兵。其中没有路由历史、证据未配置、冻结形态未配置这三种，重投不会自愈：它们会占住交接分区的队头，直到失败预算耗尽、落成 ABANDONED。ADR 说不入账是为了不丢掉这次复核，可落成 ABANDONED 同样丢掉了，还白白堵住分区。用户 2026-10-20 的上线评估已把同族问题在 SA、PP 两边归为分级 R1。
- 修法：在 `nrtransportfulfillment` 里按 `UndecidedReason` 分格。上面三格改为入账，留下原因（日志与 inbox 账），不登哨兵；复核登记册或历史读不回、看不清、取号依赖故障这几格照旧重投。日后可以并入 [demo-chain-gaps/14] 已裁的同族修法。
- 立票：NR，新票。

**M11**｜ADR-0215 越权风险点 11
- 问题：收寄采用与交接两种触发落在不同分区，复核不按业务时间排序。较早位置的触发可能排在较新的复核之后再判一次，并据此失效或改路。这与 UC-NR-003「一致性、幂等与并发」和 NR CONTEXT「按业务时间裁决，不按消息到达顺序」冲突。[demo-chain-gaps/63] 的评审写明「没有另立票」，这条冲突至今没有承接处。
- 修法：立后继票。复核前，先比较触发的业务时间与路由历史里最近一次控制触发的业务时间；较早的触发入账为「触发早于当前位置」，不判。收寄采用与交接两路同改，并在 ADR-0215 风险点 11 处补指针。
- 立票：NR，新票。

### SA 与 PP

**M12**｜ADR-0198 决定四与越权风险点 4（BUY 路 ADR-0200、AT-SA-184 同形）
- 问题：已完成但合计为零的评价，被当成两侧矛盾上抛：整笔回滚，发布失败，直到失败预算耗尽、刷告警，请求永远停在已提交。可零合计不是矛盾。PP 只在合计小于零时答失败（`ErrNegativeChargeTotal`），扣减恰好扣到零照样落成已完成；结算侧的 `FormCustomerCharge` 却拒收 `SettlementMinor<=0`。UC-SA-002 生命周期表里正好有对应的「负向结束」。
- 修法：把「已完成而合计为零」移出决定四的矛盾清单，在请求册加一格「负向结束（零额）」：入账、不重投、不建费用行。改动嫌大的话，至少做到入账不重投，并在请求册上可辨认。同步补 UC-SA-002 一行 AT。BUY 路按同形一并修。
- 立票：SA，新 ADR 加新票。

**M13**｜ADR-0198 越权风险点 2、3（两条合并）
- 问题：UC-SA-002 生命周期表里「待判断/冲突/未受理/未形成」那一行要求带缺口，AT-SA-177 要求记录缺口，AT-SA-175 要求保存排除依据与评价引用。SELL 这一路三处都不满足，UC 只为「未形成」记了例外（AT-SA-180）。BUY 路在 AT-SA-183 已把这些例外逐一点名，两处的现行说法相反。
- 修法：在 UC-SA-002 给包裹计费时点那一路补一句例外，可以并进 AT-SA-180，也可以新开一行 AT。写法仿照 AT-SA-183，点名生命周期表那一行、AT-SA-175 和 AT-SA-177：本路在请求册上不落这些，靠映射册重算，或经计价侧按回指读回。
- 立票：SA 文档小改。

**M14**｜ADR-0189 越权风险点 3 与 Consequences 第二条
- 问题：按线路计价、没绑目录的成本卡，在正式 BUY 评价上恒为待判断。ADR 给的去处是 psb/20，但 psb/20 已经 resolved，只核实了触发面，没有裁区域来源。这个缺口实际落在 [demo-chain-gaps/104]，状态 needs-triage。演示成本卡绑了目录，暂时不受影响，但真实租户一用不绑目录的干线卡就会碰上。
- 修法：给 ADR-0189 加一节复核记录，把去处改指 104。104 属产品策略，由开发方分诊二选一：正式 BUY 读段计价区域；或者规定正式成本卡必须绑目录，并在登记成本卡时给提示。
- 立票：PP 与 NR，分诊 [demo-chain-gaps/104]。

**M15**｜[triage-2026-10-19] 对 [demo-chain-gaps/14] 的裁定
- 问题：续办依赖一封「参数已登记」信封，可裁定列出的「费用项目未登」那一格永远发不出这封信。BUY 触发册的 feeItem 允许缺，册又不设修订：先登了一行没有费用项目的，再来补登，适配器答 CatalogueConflict（ADR-0208 决定三、越权风险点 4）。运营去续办口重问，也只会再答未配置。
- 修法：在 14 的裁定和新 ADR 里二选一。一是把触发册的修订形一并纳入，至少允许给费用项目为空的行追加一版带费用项目的登记，同时取代 ADR-0208 决定三「册不设修订」那句。二是把「原因已登、费用项目未登」明确排除在自动续办之外，并给出绕法。
- 立票：SA，改 [demo-chain-gaps/14]。

### PC

**M16**｜ADR-0202 越权风险点 5
- 问题：PC 点读口 `LoadSupplierAgreementVersion` 不读区间更正，`SupportsProcurementAt` 只看正文区间，违背已接受的 ADR-0038 决定二。照现状，2026-10-20 定的 [demo-chain-gaps/50] 上线前最小面「用区间更正把旧版截止在新版生效时点」，在委托提交和收费发生项登记两处核对上都不生效，旧版照样能用。
- 修法：在票 50 的完成判据里加一条：点读口交回折入最新有效性更正之后的区间，TF 适配器不动。再补一个 TF 用例：旧版被截止之后，按截止后的时点提交，答 `AGREEMENT_NOT_EFFECTIVE`。PS 的 `agreementOf` 按同一口径一并核。
- 立票：PC，并入 [demo-chain-gaps/50]。

**M17**｜[triage-2026-10-19] 对 [demo-chain-gaps/50] 的裁定
- 问题：新旧版起点相同时，「截在新版起点」就是零长区间。`NewEffectiveInterval` 要求终点晚于起点，这样的更正会被 `ErrValidityCorrectionInvalid` 拒掉。票 50 自己的动因（干线 v2 重发，区间与 v1 相同），以及评估理由点名的「第一次更正错登的合同」，偏偏多是同起点重发。
- 修法：二选一。一是把分级 R1 的最小面扩成「区间截止，加上同起点时按决定引用退役」（领域已有 `Retire`），退役写口需要的 ADR 随之提前到分级 R1。二是在裁定里明写同起点重发不在覆盖范围内，运营须让新版晚于旧版起点生效；这句本身有计费后果，要写进完成判据。
- 立票：PC，改 [demo-chain-gaps/50]。

### VE

**M18**｜ADR-0205 越权风险点 22、23（两条合并）
- 问题：VE CONTEXT 已经规定：信号发作期按因果条件划分，条件解除即结束，再发生时开一个关联的新期；每个信号都必须保存事实依据。可 `RaiseSignalCommand` 既没有因果条件，也没有事实依据，生产代码里也没有一处结束发作期。于是出发门禁阻断信号的发作期永不结束，第二次违规不再分诊。这件事目前没有票。
- 修法：立票。出发门禁阻断信号以那一条移动事实为因果条件，每次违规各开一期、重新分诊。命令加上事实依据：`movement-fact/<对象>/<事实>` 引用、事实版本，以及逐对象那一格的关务判断标识。冲突信号缺恢复边界的存量问题，在同一张票里另列一格。
- 立票：VE，新票。

**M19**｜ADR-0224 越权风险点 1
- 问题：控制事实登记时，如果段那一半欠账（`SegmentContinuationReference`），续办补进段之后没有任何东西去唤醒补派生，那次出发就丢出了追踪。整组 ADR 里，这是唯一写了「修法二选一」却没有承接票的一条；[demo-chain-gaps/53] 的完成记录也只列了它，没有立票。
- 修法：立一张 demo-chain-gaps 后继票，在票里二选一：续办进段成功时交一封唤醒信封；或者补派生改按对象全扫。并在 ADR-0224 风险点 1 末尾和票 53 的完成记录里补上票号。
- 立票：VE（读 TF 段册），新票。

### NO 与领域文档

**M20**｜ADR-0210 越权风险点 2
- 问题：GLOSSARY「待识别实物」第三条写「正式身份由 `parcel-shipment` 确认」，与紧接着的第四条「身份冲突经身份冲突处置解除」字面相抵。新的读法只写在 ADR 里，违反单一权威。
- 修法：把 `docs/domain/GLOSSARY.md`「待识别实物」第三条改为「候选与正式包裹身份只由 `parcel-shipment` 给出；实物关联哪一件由本上下文按其答复判，冲突经身份冲突处置定」。NO CONTEXT 里讲唯一命中的那一句保留。
- 立票：NO 文档小改。

**M21**｜ADR-0201 越权风险点 29
- 问题：正文写「票 demo-chain-gaps/108」，链接实际指向 110，issues 目录下没有 108。`scripts/demo-seeds/README.md` 里 data/transport/07–23 那一行也两次写成 108。
- 修法：两处的 108 都改为 [demo-chain-gaps/110]。ADR 正文如果按惯例不动，就在 ADR-0201 补一则更正记录。
- 立票：文档小改。

### 工程文档与工具

**M22**｜[infra-speed/01] 之后的 [parallel-sessions]
- 问题：「别人分支上的活怎么进 main」步骤 4 仍写「全仓 -p 1 -count=1」，与同文「本机这一跑用 `scripts/test-full.sh`」相抵。步骤 2 补过一句，步骤 4 漏了。
- 修法：步骤 4 照步骤 2 的写法补一句「2026-10-19 起这一跑用 `scripts/test-full.sh`」。「再往下省时间」段末，在「以 ci.yml 那种带 run 号的实测为据」之后补半句：本机一侧已按本机实测放开，CI 一侧仍按此句。
- 立票：文档小改。

**M23**｜[infra-speed/03] 越权风险点 4
- 问题：在 `9045b6fd` 上实测，`docs/application` 下有 1380 个不同的 AT 号，盘点表行只有 1220 个，约 160 个没有表行（NO-101–128、PC-037–053、PS-094–105、SA-179–187、NR-054–063/070–072，以及 TF 的多段）。NO-101–112、126–128 的追记写着「各加 12/3」，但生成器不读追记，这些加数在 03 之后已经不生效，总览因此低报。风险点说这个缺口「不归本票」，却没有给去处。
- 修法：立 AT 盘点票，把这批补成分桶表行；在 [at-coverage-inventory]「方法与口径」里写明：新 AT 只按表行计数，追记里的加数不再生效。
- 立票：文档与工具，新票。

## 维持

| 出处 | 维持 | 最值得一提的一句 |
|---|---|---|
| ADR-0114 | 6 | 风险点 4 已由 ADR-0130 关闭；复核记录里 [tf-face-algebra/09] 的两处裁定也维持 |
| ADR-0149 | 5 | 风险点 3 是红线「登记册证据 + PN-08 Go」在代码上的落点，有意为之。另见 R1 |
| ADR-0151 | 6 | 风险点 3 的「能登不能改」在 TF 没有成真，同类问题在资金事实一侧成了真，归 R1。另见 M1、M2 |
| ADR-0183 | 5 | 风险点 4「没读原文」已由同日复核记录补读 |
| ADR-0184 | 9 | 复核记录 3（号码去两端空白）还没到期，但 label-channel 各票都没记这一问，接渠道包裹件号触发面的票立票时应写进去 |
| ADR-0186 | 4 | 决定二、七已由 ADR-0199 决定五停用，并已进 main（`e2c6f836`） |
| ADR-0189 | 7 | 另见 M14 |
| ADR-0190 | 10 | 风险点 3（请求卡死）由 [demo-chain-gaps/14]（分级 R1）承接。另见 R2 |
| ADR-0195 | 6 | 多数风险点已被 ADR-0210、0211、0212 收窄 |
| ADR-0196 | 5 | VE 协调路径至今无票（风险点 5，与 ADR-0210 风险点 9 是同一缺口），可在 VE 积压里补挂一行 |
| ADR-0198 | 2 | 另见 M12、M13 |
| ADR-0199 | 9 | 「能力边界」里「控制转出没有生产接线」一句已过时，ADR-0214 已经接上 |
| ADR-0200 | 5 | [demo-chain-gaps/15] 仍是 needs-triage，也不在上线就绪评估名单里 |
| ADR-0201 | 26 | 风险点 12「运行方向」列读空的问题仍在，[demo-chain-gaps/38] 还是 needs-triage。另见 R3、R4、R5、M21 |
| ADR-0202 | 15 | 风险点 17 的括注「尚未立票」已过时，[demo-chain-gaps/37] 已立。另见 R8、R9、M16 |
| ADR-0203 | 18 | 另见 M3 |
| ADR-0204 | 9 | 另见 R10、M4 |
| ADR-0205 | 22 | 风险点 13（执行方看得到同段每件对象）与 R11 是同一个答复形状，R11 答「是」时随之改。另见 M5、M18 |
| ADR-0206 | 11 | 风险点 1：`ManageRestrictionHandler` 在生产里零引用，限制这一格今天只在测试里出现。另见 R12 |
| ADR-0207 | 15 | 风险点 11 的纠正靠更正采用，即 [demo-chain-gaps/57]（分级 R1） |
| ADR-0208 | 8 | 风险点 4 建议上线前随 PAR-SET-03 补触发册的修订形，与 M15 是同一件事 |
| ADR-0209 | 9 | 风险点 5（v1、v2 并存，没有收尾入口）宜在真实租户第一次换版前补票 |
| ADR-0210 | 8 | 另见 M20 |
| ADR-0211 | 7 | 风险点 6 所述事实已不成立，管理台已经认得 `STILL_AMBIGUOUS` |
| ADR-0212 | 13 | 另见 R13 |
| ADR-0213 | 22 | 风险点 7 与 R12 同根。另见 R14、M6 |
| ADR-0214 | 19 | 风险点 10（收寄晚于交接到达时，陈旧控制永不转出）归 [demo-chain-gaps/81]（分级 R2），复看 81 时宜把离线补登算进频次 |
| ADR-0215 | 7 | 另见 R6、M10、M11 |
| ADR-0216 | 5 | 风险点 5 让拍频格退役，没有吞掉租户取值：按班次批量派送的需要归派送任务合并规则。另见 M7、M8 |
| ADR-0217 | 8 | |
| ADR-0218 | 9 | Status 里写待 SA、PC 会签的几点，核实后不新增计费口径，所以没有升为须裁 |
| ADR-0219 | 8 | 风险点 4：问询按微秒在 CC 册上留行，没有保留期；今天还没有客户端轮询。另见 R11 |
| ADR-0220 | 6 | |
| ADR-0221 | 4 | |
| ADR-0222 | 6 | 风险点 6（自建 Intake 的行声明错了照样绿）是可补的测试缺口 |
| ADR-0223 | 6 | 风险点 7 已并入 R7。另见 R7 |
| ADR-0224 | 8 | 另见 M19 |
| [infra-speed/01] | 1 | 本机放开 -p 1 有本机实测支撑，CI 照旧。另见 M22 |
| [infra-speed/02] | 9 | 路由与行为零变化已由逐字节对照证明 |
| [infra-speed/03] | 7 | 风险点 2 的重复注记已由 [infra-speed/07] 承接。另见 M23 |
| [infra-speed/04] | 6 | |
| [infra-speed/08] | 2 | 两道 -check 只挂 main 推送，与「生成物不占号」的约定一致 |
| [triage-2026-10-19] | 4 | A/B/C/D 分组、分级 R1/R2/R3 归组，以及票 70、88 的裁定都维持。另见 R10、M9、M15、M17 |

## 各组附注

**复核方式与已有记录的性质**：各组都在 `9045b6fd` 上只读复核，没有改任何文件。风险点里写的事实都抽查过，包括迁移号、生产接线、grep 结果，以及票 06、07 的管理台补丁。要提醒的是，ADR-0114、0183、0184 里已有的「owner 复核」行，本身也是同一授权下的会话代裁，不是 owner 亲审。

**真缺陷集中的几处**：
- SA 计费：零合计被当成矛盾上抛，SELL、BUY 两路同形（M12）。
- TF 交接更正链：待确认更正为已交接时不进段；仍为已交接的更正会自动另立派送任务（M7、M8）。
- NR 交接复核：未配置类的未决拿去重投，且复核不按业务时间排序（M10、M11）。
- PC 点读口：不折入区间更正，导致票 50 的最小面在 TF 核对上不生效（M16）。
- TF 准备：读池时持行锁（M3）。
- 分诊裁定：上线就绪评估方向是对的，但 14、50、57 三条有可证的缺口，开工前应先补（M15、M17、M9）。

**跨组张力**：ADR-0205 风险点 13 在一组里维持，ADR-0219 风险点 5 在另一组升为须裁，两者是同一个答复形状，以 R11 的答案为准。ADR-0204 风险点 2 与分诊对票 57 的裁定落在同一问上，已合并为 R10。

**过时的说法，可顺手清**：
- ADR-0199「能力边界」的控制转出一句。
- ADR-0202 风险点 17 的「尚未立票」。
- ADR-0189 指向 psb/20 的去处。
- 管理台资金、关务登记签旁两处「挂 `UnconfiguredIntake{}`，随 PAR-INT-01 点亮」的前端注释（随 R1 一起处理）。

**建议补挂的积压**：
- VE 协调路径（ADR-0196 风险点 5、ADR-0210 风险点 9）。
- 换版收尾入口（ADR-0209 风险点 5）。
- label-channel 接渠道包裹件号触发面时的去空白约定（ADR-0184 复核记录 3）。

[demo-chain-gaps/14]: ../demo-chain-gaps/issues/14-buy-request-stuck-after-an-unadoptable-evaluation.md
[demo-chain-gaps/15]: ../demo-chain-gaps/issues/15-occurrence-revision-does-not-drive-a-cost-correction.md
[demo-chain-gaps/36]: ../demo-chain-gaps/issues/36-tf-authorized-re-preparation-request-face.md
[demo-chain-gaps/37]: ../demo-chain-gaps/issues/37-confirmed-booking-capacity-relationship.md
[demo-chain-gaps/38]: ../demo-chain-gaps/issues/38-admin-web-transport-schedule-section-reads-connection-and-nodes.md
[demo-chain-gaps/42]: ../demo-chain-gaps/issues/42-departure-gate-reading-does-not-expand-consolidation-unit-members.md
[demo-chain-gaps/44]: ../demo-chain-gaps/issues/44-tf-re-prepares-on-node-actual-measurement.md
[demo-chain-gaps/50]: ../demo-chain-gaps/issues/50-commercial-versions-have-no-closing-entry-so-agreement-versions-pile-up.md
[demo-chain-gaps/53]: ../demo-chain-gaps/issues/53-movement-facts-recorded-before-their-segment-never-reach-tracking.md
[demo-chain-gaps/57]: ../demo-chain-gaps/issues/57-segment-commission-adoption-has-no-correction-and-start-does-not-rederive.md
[demo-chain-gaps/60]: ../demo-chain-gaps/issues/60-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope.md
[demo-chain-gaps/63]: ../demo-chain-gaps/issues/63-network-routing-reassesses-on-a-handover-into-a-node.md
[demo-chain-gaps/70]: ../demo-chain-gaps/issues/70-delivery-dispatch-left-unformed-has-no-trace.md
[demo-chain-gaps/81]: ../demo-chain-gaps/issues/81-control-source-chain-recompute-has-no-disposition-face.md
[demo-chain-gaps/94]: ../demo-chain-gaps/issues/94-a-courier-previews-the-final-delivery-gate-before-dispatching.md
[demo-chain-gaps/104]: ../demo-chain-gaps/issues/104-unbound-buy-cost-cards-get-no-zone-on-the-formal-evaluation.md
[demo-chain-gaps/110]: ../demo-chain-gaps/issues/110-demo-transport-seeds-have-no-guard-for-three-leg-preparation-coverage.md
[demo-chain-gaps/114]: ../demo-chain-gaps/issues/114-deterministic-failures-answered-as-retryable-undecided-across-consumers.md
[demo-chain-gaps/119]: ../demo-chain-gaps/issues/119-customer-tracking-view-locates-by-the-customers-own-parcel-reference.md
[demo-chain-gaps/121]: ../demo-chain-gaps/issues/121-departures-recorded-before-their-segment-are-never-re-asked-at-the-gate.md
[triage-2026-10-19]: ../demo-chain-gaps/triage-2026-10-19.md
[infra-speed/01]: ../infra-speed/issues/01-full-suite-runs-packages-concurrently-on-a-dedicated-socket-instance.md
[infra-speed/02]: ../infra-speed/issues/02-composition-roots-register-per-file.md
[infra-speed/03]: ../infra-speed/issues/03-adr-index-and-at-coverage-counts-are-generated.md
[infra-speed/04]: ../infra-speed/issues/04-demo-seed-steps-load-from-ordered-files.md
[infra-speed/07]: ../infra-speed/issues/07-adr-status-lines-carry-a-suspension-twice.md
[infra-speed/08]: ../infra-speed/issues/08-ci-gate-for-generated-adr-index-and-at-counts.md
[tf-face-algebra/09]: ../tf-face-algebra/issues/09-dispatch-trigger-task-reference-is-a-faithful-identity.md
[label-channel-service-first-release/31]: ../label-channel-service-first-release/issues/31-carrier-first-effective-pickup-fact-registry-and-handoff.md
[ps-external-mark-relations/11]: ../ps-external-mark-relations/issues/11-identity-check-judges-the-verifiable-delivery-scope.md
[at-coverage-inventory]: ../at-coverage-inventory.md
[parallel-sessions]: ../../docs/agents/parallel-sessions.md
