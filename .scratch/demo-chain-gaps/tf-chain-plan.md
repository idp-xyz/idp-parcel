# 运输履约演示链缺口：统一代决与落地计划

钉 `main 91ea0883`（2026-10-19）。授权：用户原话「你作为业务和系统专家自决，解决所有的问题」。下文各条都是会话代决，越权风险点留给各 owner 复核。实测证据一律记 `S`。

本文件是这批代决的单一权威，各票落地时把对应裁定写进各自 ADR（0201–0209，已占号），ADR 接受后本文件只作索引。

在途分支（2026-10-19 现状）：
- `claude-dcg08`（ADR-0200，BUY 按回指形成供应商预期成本）、`claude-dcg11`（ADR-0197，外部结果按层进 VE）已于 2026-10-19 进 main（`7bba9b85`）。
- `claude-nrs01`（ADR-0199）亦已进 main。

所以本计划的新 ADR 从 **0201** 起编。

---

## 代决（逐条）

### 跨票通则

- **X1 演示租户各段的履约方式**（租户取值，记 `S`）
  - 裁定：SYN-TENANT-01 各段如下。
    - `SHA-HUB→SZX-GATE` 自营。
    - `SZX-GATE→SIN-HUB` 外包给 `SYN-PARTY-CARRIER-TRUNK-01`，采用 `SYN-SUPPLIER-TRUNK-CN-SG-01` v1。
    - `SIN-HUB→SIN-LM` 与末端派送自营，经一线设备，与 psb/05 重走一致。
    - `SYN-SUPPLIER-LASTMILE-SG-01` 暂时不被任何委托采用，等 06 定末端。
    - 这几条写进 `docs/design/synthetic-demo-journey-script.md` 与 `scripts/demo-seeds/README.md`，不进参数登记册。
  - 理由：干线协议已在商业种子里发布，02、07 和 BUY 都靠它。CONTEXT 写明「自营段不建委托」「自营与外包不由委托有无推断」，所以这里是声明，不是从协议有无推出来的。
  - 越权风险点：末端协议闲置。06 若改成外包，要补委托与发生项（演示租户 / 06 的 owner）。

- **X2 入口族一律按 ADR-0151 定族判据**
  - 登记册配置（不搬包裹、不形成生产事实）走 `operatorRegistries.transport`，同一个登记用例同时出 CLI 和端点（ADR-0085）。
  - 操作者做出的生产决定走 `operatorDecisions.transport`，每口一种决定种类。种类本身是产品角色模型；授予、准入映射与权威区间是租户取值，演示用种子登 `SYN-`。
  - 外部系统交来的结果走集成客户端（ADR-0149）。
  - 越权风险点：登记册写授予没有逐册维度，持写权的配置员既能登班次，也能登凭证（access-identity owner）。

- **X3 信封上线次序**
  - 新事件类型的消费者先进路由表，生产方后上线（ADR-0049 Consequences）。
  - 没有消费者的新结果不入队，包括运输准备结果和关务动作准入判断。
  - `transport-commission.submitted` 照 ADR-0066 保持未登记。
  - 新信封一律用引用式载荷加 `outboxintent.FingerprintEventID`。
  - 越权风险点：每封委托信封都会撞 `dispatch.no_subscriber`，直到 ABANDONED，演示日志会出现红行（TF owner，作为 ADR-0066 的后继问题）。

- **X4 跨上下文读**：一律经消费侧适配器（ADR-0025）。新端口另立文件，不改 TF `ports/ports.go`，因为 peer 的文件在 import 它。

- **X5 已发生的物理事实照记**：依赖读不动也不挡落册（ADR-0186 先例）。读数与事实同一笔保存。

- **X6 未配置与未决分格**：答 `未配置` 的入账，不重投。只有依赖读不动才答未决，登哨兵后重投（ADR-0029、ADR-0094）。

- **X7 编号**：迁移号、ADR 号都在认领时占号。本计划给的是建议号。

### 01 运输机会与班次

- **D01-1 准备由什么触发**
  - 裁定：分两段。本链先做 TF 消费 `network-routing.initial-route.formed`：路由表改成 `FanOut(VE, TF)`，各自一个 inbox、各列未决哨兵。授权的运营再准备请求放到后继票 36，复用同一个应用用例。
  - 理由：UC-TF-003 的第一个触发是当前有效计划段，CONTEXT-MAP 的 NR→TF 边已有表述，派发注释也已把 TF 列为欠着的消费者。运营口要决定种类、权威区间与准入，量大，而且不挡演示。
  - 越权风险点：每包裹每段自动准备，事件放大归 PAR-NFR-03（TF owner；NR owner 会签这条消费边）。

- **D01-2 周期安排归谁**
  - 裁定：TF 自有「周期安排」登记册。文法是产品策略，内容是租户取值（PAR-NET-08）。NR 服务日历不当班次用。
  - 理由：班次生命周期「周期安排 → 具体班次」写在 TF CONTEXT 里，GLOSSARY 也把班次归 TF。出发时刻依赖具体网络，过不了 ADR-0146 参考配置的分界检验。
  - 越权风险点：租户若已在 NR 侧维护车次表，要录两遍（TF owner）。

- **D01-3 登记走哪个族**
  - 裁定：周期安排、具体班次、容量池走登记册配置写。容量预占、释放和再准备走运营决定。
  - 理由：建班次不搬包裹，也不形成生产事实，符合 ADR-0100 的豁免，与主单、凭证的先例同形。演示种子已经授了这一面。
  - 越权风险点：同 X2。

- **D01-4 班次的方向身份**
  - 裁定：班次固定（NR 连接代码〔不带版本〕、起点节点、终点节点），按 NR 拼写不透明保存。与计划段比对只在 TF 的 `adapters/networkrouting` 里做，对的是 `PlannedLeg.From/To` 和窗口依据 `CONNECTION/<code>@<ver>`。自由串方向退役。
  - 理由：自由串无法与段比对。只用节点对，遇到平行连接会有歧义。连接代码跨 NR 版本保持稳定。
  - 越权风险点：连接出新版本、改了起讫或含义时，仍按代码相认（拼写归 NR 定义，NR owner 确认）。

- **D01-5 空册与无候选怎么答**
  - 裁定：
    - 该连接没有任何周期安排或具体班次：准备结果记 `未配置`，确认消费，不重投。
    - 有登记，但段窗口内没有合格候选：记 `无当前运输机会`，带候选与淘汰原因。
    - 任何情况都不建占位班次。
  - 理由：在 FanOut 下，TF 一重投就会把每个没配班次租户的初始路由信封烧到 ABANDONED。缺租户取值是答案，不是依赖故障。UC-TF-003 把 `无当前运输机会` 留给「有候选被评过」的情形。
  - 越权风险点：租户补登周期安排后，已经形成的 `未配置` 结果不会自动重备，要靠票 36（TF owner）。

- **D01-6 需求量从哪来**
  - 裁定：
    - 收寄前取 PS 的申报测量（`DeclaredMeasurementView`，经 TF 消费侧适配器），快照标 `declaration` 与资料版本锚。
    - 只认三个维度：重量、体积（三边乘积）、件数。件数只给件数维的池用。
    - 袋位、托盘位等没有来源的维度答 `待补充`。
    - 同一维度内按量纲换算单位，跨维度不折算。
    - 节点实测后的再准备等 peer 收工，另立票。
  - 理由：UC-TF-003 的输入是「当前有效测量或声明数量版本」，并禁止用对象数或计费重量顶替。NO 是 peer 的地盘。
  - 越权风险点：用申报量做容量判断，实测偏大时只能靠再准备补救（TF owner；PS、NO 是提供方）。

- **D01-7 改路后的新计划**
  - 裁定：不在本链内。立 NR 票 34（复评新计划已形成信封），TF 消费者照 ADR-0049 先登路由。TF 不轮询 NR 计划目录。票 18 的完成记录写明这个缺口。
  - 理由：生产方归 NR。轮询等于把 NR 的适用性判断复制一份。
  - 越权风险点：34 落地前，改路后的段没有准备结果（NR owner）。

- **D01-8 内置择优形态**
  - 裁定：只内置一种。
    - 候选条件：连接代码相同、起讫相符，出发不早于段窗口起点，到达不晚于段窗口终点。
    - 排序：出发最早优先，同刻按班次标识。取第一个所有已登池维度容量都够的，答 `运输机会已准备`。
    - 有候选但没有池，或池的维度没有需求量：答 `容量待确认`。
    - 有候选但硬容量都不够：答 `当前机会不可用`。
    - 单对象请求不出 `部分准备`，这一格留给批量请求。
    - 不预占。
  - 理由：ADR-0146 决定五要求每个决定点至少有一种内置形。UC-TF-003 写明「本用例只形成机会……不提前写成预占」，也不得按默认容量推进。
  - 越权风险点：择优形态以后是否开成租户可选（TF owner）。

- **D01-9 周期文法与物化**（产品策略）
  - 裁定：
    - 文法只内置一种：按周循环（星期集合）、当地出发时刻、到达时刻或历时、IANA 时区、连接代码与起讫节点、服务范围引用、生效区间，外加可选的逐维容量声明（维度、单位、硬上限）。
    - 具体班次标识由（周期安排代码、当地运行日期）确定性派生。
    - 物化只发生在两处：准备时评估候选（UC 步 5），以及显式「建立具体班次」的登记。
    - 不按滚动窗口预生成，因为窗口是租户取值，现在没有。
    - 另可登记不出自模板的临时班次。
  - 理由：对应 AT-TF-027。固定日期的 SYN 班次会随提交时刻过期，模板加确定性物化让每次演示都能匹配上。
  - 越权风险点：按日历例外、按月循环等形态不内置（TF owner）。

- **D01-10 准备结果怎么存、发不发信封**
  - 裁定：每个（租户、计划段引用〔版本+序位〕、包裹、规则版本）只有一份当前有效结果。重放答 `已有结果`，内容不同答 `结果冲突`。不入队准备结果信封。
  - 理由：没有消费者（X3）。
  - 越权风险点：UC-TF-003 步 8 的发布意图暂缺，等 NR「分配失败重评」那条边（TF / NR owner）。

- **D01-11 拆票**：01 实测体量是 XL，拆成三票：16 周期安排与班次、17 容量池、18 准备结果与消费者。

### 02 委托、订舱、承运答复

- **D02-1 委托信封**
  - 裁定：接上 `OutboxTransportCommissionHandoff`，但路由表不登记，照 ADR-0066 的「缓」。同时把 handler 与端口注释里「交 SA 作成本预期源」改成现状：SA 的成本源是发生项加 BUY。
  - 理由：ADR-0049 决定三与 ADR-0066 已有裁定。不入队会丢掉后继消费者要用的意图（ADR-0043）。
  - 越权风险点：同 X3。

- **D02-2 承运答复走哪个族**
  - 裁定：集成客户端族。新外部事实类型 `CARRIER_BOOKING_ANSWER`。客户端绑定的来源身份必须等于该订舱所属委托的服务提供方，不等就答不受理。由 ADR-0202 补进 ADR-0149 的列举。
  - 理由：ADR-0151 的判据是「看提交方是谁」，承运答复的提交方是承运系统。psb/31 已收，演示可以发客户端令牌。
  - 越权风险点：靠邮件或电话答复的承运人没有人工转录口（TF / access-identity owner）。

- **D02-3 决定种类**
  - 裁定：每口一种：
    - `TRANSPORT_COMMISSION_SUBMISSION`
    - `BOOKING_REQUEST_SUBMISSION`
    - `TRANSPORT_COMMISSION_CANCELLATION`
    - `BOOKING_REQUEST_CANCELLATION`
    - `FULFILLMENT_SEGMENT_COMMISSION_ADOPTION`
  - 理由：沿 ADR-0151 先例（受控关闭与重开分成两种），授予的粒度等于权力的粒度。
  - 越权风险点：种类数属于产品角色模型，归 psb/07。

- **D02-4 协议怎么核**（内置判法，产品策略）
  - 裁定：
    - 三项检查：协议版本在册；在提交时点 `SupportsProcurementAt`；协议供应商等于服务提供方。
    - 不符时分格答 200 业务结果：`AGREEMENT_NOT_REGISTERED`、`AGREEMENT_NOT_EFFECTIVE`、`AGREEMENT_TERMINATED`、`PROVIDER_MISMATCH`。PC 读不动答 `UNDECIDED`。
    - 采购责任法人从协议版本快照到委托上（新加一列），不由调用方给。
    - 条件、角色、责任仍是调用方声明的引用，作为已知缺口记下，不做假核对。
  - 理由：照 PS `agreementOf` 的先例，只用 PC 拥有的事实。
  - 越权风险点：不核协议是否覆盖这段运输范围（TF owner；知会 PC owner）。

- **D02-5 PC 点读口**
  - 裁定：PC 新开一个窄读口，按（租户、协议对象、版本标签）答 Effective 版本壳加正文。
  - 理由：不把 PC 的商业范围概念带进 TF 快照，也不必每单加载整个范围登记册。管理台的列表口是受限口，不能借用。
  - 越权风险点：PC owner 若拒绝开口，退路是命令携带商业范围，走 `LoadForScope` + `Lookup`。

- **D02-6 开始连线与取消面**
  - 裁定：
    - 新运营决定「履约段采用运输委托」（ADR-0204）：声明（段、委托）这一对，段须在册，委托须已提交且服务方一致。
    - 采用即调用 `MarkTransportStarted`，依据取该段首个参与。
    - 取消委托、取消订舱的面随这张票一起开。已开始的答 `TRANSPORT_ALREADY_STARTED`，只取消尚可停止的范围。
    - 本条部分停用 ADR-0098 决定四，适用场景只限实际履约段，揽收↔委托仍不建。
  - 理由：没有开始连线就开取消面，会把已开始的运输答成已取消，违反 CONTEXT 与 AT-TF-092。不在交接登记里盖章，避开了 peer 的文件，也守住 ADR-0096 的声明式段身份。07 的采购上下文也要用这条线。
  - 越权风险点：采用登记晚于实际开始时，开始时点取段的首个参与（TF owner）。

- **D02-7 演示哪段外包**：见 X1。
- **D02-8 出站订舱连接器**：留在 psb/09 第 3 项，不并入本链。

- **D02-9 订舱引班次与答复词表**
  - 裁定：
    - 订舱必须引用一个在册的具体班次，不在册答 `SCHEDULE_NOT_FOUND`。
    - 承运答复的封闭集加 `WAITLISTED`（候补）。
    - `待确认`、`待补充` 是订舱请求还没有答复时的读法，不是答复值。
    - 逐对象接受、委托级接受都不做，记为缺口。
  - 理由：UC-TF-004 步 1 要校验运输机会。CONTEXT 把订舱与承运接受分开。
  - 越权风险点：确认订舱的容量关系另立票 37（TF owner）。

### 03 装载分配与容量

- **D03-1 拆票**：03a 是票 19；03b 拆成票 20 和 21。

- **D03-2 班次不在册怎么答**
  - 裁定：先做版本重放和链头判定，再按租户读班次登记。不在册（含指向他租户的引用）答新格 `SCHEDULE_NOT_FOUND`，HTTP 200。读失败落现有的 `UNDECIDED`。
  - 理由：恢复动作是登班次或换班次，与修载荷不同（ADR-0029）。TF 已有同形先例 `SEGMENT_NOT_FOUND`。
  - 这条落在 UC-TF-004 步 1 的现行文字内，只补 AT 行，不需要 ADR。

- **D03-3 什么时候必须有容量保障**（产品策略，ADR-0203）
  - 裁定：分配所在的班次只要登记了至少一个容量池，就必须对每个池引用一份当前有效预占，并声明分配量。没登池的班次不要求。确认订舱用同一判据（票 37）。
  - 理由：已登记的池是硬限制，绕过它就是超配。这个判据不看任何租户，所以归产品策略。
  - 越权风险点：不登池的租户，分配不受保护（TF owner）。

- **D03-4 分配关系**
  - 裁定：在预占上新增分配关系，守恒为「已分配 ≤ 预占 − 释放 − 消耗」。分配撤回或改版时归还。形成分配时不调 `Consume`。池所在的班次必须等于分配所在的班次。
  - 理由：CONTEXT 把分配量和实装量分开存，未使用的范围要归还；`Consume` 是永久占用。

- **D03-5 分配量从哪来**
  - 裁定：逐池逐维度由运营声明，并带来源：运营声明版本，或引用票 18 准备结果的需求快照。只有件数维的池可以用成员数。
  - 理由：UC-TF-003 禁止用对象数顶替。
  - 越权风险点：声明量可能和实测不符（AT-TF-087）。

- **D03-6 预占与释放**
  - 裁定：两者都是运营决定，种类为 `CAPACITY_RESERVATION`、`CAPACITY_RELEASE`。预占必须声明需求量与来源。碰到硬上限答 `CAPACITY_EXCEEDED`，带余量，不能靠审批越过。`EstablishPool` 改为核对班次在册（票 17）。

- **D03-7 容量消耗交接**
  - 裁定：不进本链。并入 envelope-id-bounds/01 的运输履约半边：事件 ID 改成含消耗依据的指纹，`Consume` 按依据做幂等。仍不接线。消费者（NR 容量风险信号）等 NR owner 定。

- **D03-8 资源 / 载运位置粒度**：本链不扩。

### 05 移动事实与出发门禁

- **D05-1 门禁是读数，不拒收**（ADR-0205）
  - 裁定：出发一律落册，同一笔存关务门禁读数。读数是封闭集：`放行` / `阻断`（带原因与限制来源）/ `不受门禁` / `未决`（带缺什么）/ `读不动`。退役「`DEPARTURE_GATE_BLOCKED`、不落库」这一格。阻断读数随信封交 VE，作为异常信号。
  - 理由：现行做法是 200 加不落库，离线队列会因此出队，丢掉一次实际发生的出发。ADR-0186 与 9b 的 C1 规则已经定了「照记」。
  - 越权风险点：违规出发会被记成事实而不是被挡下。挡下是设备出发前读口的事，等 peer（TF owner）。

- **D05-2 TF 拉，不是 CC 推**
  - 裁定：TF 的消费侧端口 `DepartureGateView` 在写事务开始前同步问 CC。
  - 理由：ADR-0137 决定二规定门禁判断由请求驱动，出发登记本身就是请求。

- **D05-3 CC 一口作答**（ADR-0206）
  - 裁定：CC 自有一个「动作准入判断」查询，把限制、门禁核对和外部放行三源折叠成封闭答复。TF 只翻译，不再自己折。
  - 理由：门禁满足不等于放行；外部放行也不解除内部限制。TF 自己折就成了一个决策两处定义。
  - 越权风险点：CC 的限制今天没有生产写入，阻断只可能来自门禁或外部结果（CC owner）。

- **D05-4 对象到范围与程序**
  - 裁定：由 CC 按包裹引用解析，先例是 `ParcelDeclarationFactsView`。解析不到答未决。演示停在 `SYN-SCOPE-01` / `SYN-PROC-CN-EXPORT` 一级，逐件建案等 psb/18。

- **D05-5 适用哪些动作**（修正证据包的建议）
  - 裁定：
    - TF 只交出一个动作意图：「出发：自 A 节点到 B 节点」。
    - 由 CC 定适用动作：出发对应 `LOADING_DEPARTURE`。
    - CC 的节点关务区域登记册显示起讫区域不同时，另加 `CROSS_CUSTOMS_MOVEMENT`。
    - 节点未登记，而该对象有跨区门禁目录行：这一动作答 `未配置`。
    - 不采纳「每次出发都问两种」。
  - 理由：演示目录按出口程序登了 `CROSS_CUSTOMS_MOVEMENT`。每次都问两种，会让境内段 SHA→SZX 被出口门禁挡住。NR CONTEXT 和 `CustomsApplicabilitySource` 都写明「是否跨关务区域是 CC 的判断」。
  - 越权风险点：节点关务区域是一本新登记册，粒度按节点（CC owner）。

- **D05-6 移动事实怎么连到对象**
  - 裁定：移动事实带登记方声明的段引用，不加外键（迁移 0010 先例），承运对象取该段的当前参与。段不在册也照记，读数答 `未决：段未在册`。资源引用可选。不经装载分配推导对象。
  - 理由：CONTEXT 禁止「用计划覆盖事实」；ADR-0096 规定段身份是声明的；UC-TF-005 要求结果带对象/段/资源。

- **D05-7 外包干线的跨境门禁**：落在把货交给承运人的那次交接上，那是 peer 文件，等 peer。05 只修自营移动的机制。

- **D05-8 设备自报字段**
  - 裁定：`gateRequired` 不再决定是否设门。命令字段暂时保留但不读，等 peer 收工再删。`gateClearance` 只作为设备出示的证据原样保存。
  - 理由：设备不能自己把门关掉。删字段会破 peer 的 `work_fact_intake_test.go`。

- **D05-9 不核班次在册**：事实可以早于班次登记。

- **D05-10 谁消费移动事实信封**：`transport-fulfillment.movement-fact.recorded` 与 VE 消费者、路由行、里程碑映射参考配置在同一个变更上线（票 33）。NR 路由偏离另立票 35。

- **D05-11 外包轨迹那一半**：从 05 删掉，归 psb/09 第 1 项。

### 07 运输收费发生项

- **D07-1 发生条件放在哪**
  - 裁定：先上「登记者断言条件已满足」，断言记在运营决定上，带协议版本。随票 29（ADR-0209）上 PC 文法：逐原因声明是否计费，加数量形（`PER_SEGMENT` / `PER_PIECE` / `PER_ATTEMPT` / `PER_BOOKING`）。此后登记按声明判，协议没声明答 `未配置`。演示协议以 v2 重新发布。
  - 理由：UC-TF-004 的输入行把「收费发生条件引用」指给 PC。TF 自建条件册会成为第二个商业权威。
  - 越权风险点：断言期内，条件由人判断（TF / PC owner）。

- **D07-2 采购上下文怎么来**
  - 裁定：
    - 先由登记方给出服务方与协议版本（ADR-0098 决定二），经票 22 的适配器核 PC。采购责任法人取自协议。
    - 票 25 落地后，段已采用委托时从委托快照取；调用方给的值不一致答冲突。
    - 拒绝在建段时盖章。
    - 缺采购上下文答不受理，绝不答不适用。

- **D07-3 实际履约发生项由什么触发**
  - 裁定：先开显式运营决定 `TRANSPORT_CHARGE_OCCURRENCE_REGISTRATION`。TF 自消费交接 / 有效交付信封来自动形成，等 29、25 落地且 peer 收工。永不内联进交接或交付编排（ADR-0098 决定一）。

- **D07-4 信封与费用项目**
  - 裁定：`transport-fulfillment.charge-occurrence.formed` 用引用式载荷（租户、发生项、有效性版本），`FingerprintEventID`，与保存同一笔。消费者先登路由：票 26 先于票 27。费用项目归 SA。

- **D07-5 旅程**
  - 裁定：由登记方声明，演示约定 `SYN-JRN-<包裹号>`。
  - 越权风险点：同一包裹被不同登记者声明成不同旅程时无人核对（TF owner；GLOSSARY 里旅程是跨上下文词）。

- **D07-6 发生项身份**：由（租户、原因、事实依据、成员范围）确定性派生，首版有效性为 V1。重登答已存在，异内容答冲突。

- **D07-7 自营**：只有显式声明自营，或实际承运商判断为自营时，才答不适用（ADR-0098 决定三）。

- **D07-8 派送失败尝试**：经现有的 `DeliveryAttemptView` 读，新增领域构造 `ChargeOccurrenceForDeliveryAttempt`。不建新存储。

- **D07-9 演示先走哪个原因**：`ACTUAL_FULFILLMENT`，在干线段 SZX→SIN 的参与结束之后登记。`FAILED_ATTEMPT` 机制齐备，演示可选。

### BUY 发起（接 08，收 10）

- **D08-1 SA 怎么接发生项信封**（ADR-0208）
  - 裁定：
    - SA 在 `cmd/parcel-dispatch` 消费 `charge-occurrence.formed`，经 SA 消费侧适配器按引用回读发生项。
    - 发生项、原因、版本、发生时点、供应商协议都取自发生项本身。协议是发生项形成时固定下来的来源事实，不是从登记册推导出来的。这一点部分停用 ADR-0164 决定一末句，只限本信封路径。
    - BUY 触发册每行（租户、原因）加一格「费用项目」，属租户取值；缺这一格答 `未配置`。
    - 主要范围取协议版本的商业范围，经 SA→PC 读口；读不动答未决。
  - 越权风险点：一个原因只映一个费用项目，不分协议；主要范围等同协议的商业范围（SA / PC owner）。

- **D10-1 演示的 BUY 触发行**：登 `ACTUAL_FULFILLMENT → OCCURRENCE_FORMED`，费用项目 `SYN-FEE-ITEM-TRUNK-COST-01`。成本卡补声明金额取整，以新版本重发（票 30）。
  - 改判（2026-10-19，票 30 实施时）：成本卡**就地改 v1**，不另发 v2——另发一版会让在用解析答适用冲突。理由与越权风险点以[票 30](./issues/30-buy-cost-card-declares-amount-rounding.md)完成记录的「代决」「越权风险点」为准。上一句「以新版本重发」作废。

---

## 需要的 ADR

状态格写法沿 ADR-0200：「Accepted（会话代决）」，越权风险点待 owner 复核。已接受 ADR 的正文不动，只在它的 Links 加一行后继指针，并在 README 状态格注明「部分停用」。

| 号 | 题 | owner | 票 | 对已接受 ADR 的作用 |
|---|---|---|---|---|
| 0201 | 运输机会：周期安排按日期物化成班次，初始路由形成后逐段准备，空册答未配置 | TF（NR 会签） | 16/17/18 | 无 |
| 0202 | 运输委托、订舱与承运答复的入口族与协议核对 | TF + access-identity | 23/24 | 补 ADR-0149 列举（加后继指针）；引用 ADR-0066 |
| 0203 | 容量保障：班次挂池就必须引用预占并分配数量 | TF | 20/21/37 | 无 |
| 0204 | 履约段采用运输委托：声明式连线，开始后不可取消 | TF | 25/28 | 部分停用 ADR-0098 决定四（限实际履约段） |
| 0205 | 出发门禁是拉取的读数，与移动事实同笔落册，不拒收 | TF | 32/33 | 无（退役的拒收只出自票 tf-unwired-seven/06，不是 ADR） |
| 0206 | 关务动作准入判断：一口折叠三源，跨区由 CC 按节点区域判 | CC | 31 | 无 |
| 0207 | 运输收费发生项的显式登记与已形成信封 | TF | 27/28 | 落在 ADR-0098 决定二之内；决定一、三不变 |
| 0208 | BUY 评价由发生项已形成信封发起 | SA | 26 | 部分停用 ADR-0164 决定一末句（限本路径）；回答其越权点 2 |
| 0209 | 供应商协议声明收费发生条件 | PC + TF | 29 | 无 |

**ADR-0201**
- 机制：
  - 「周期安排 ≠ 班次」。具体班次标识由（周期安排代码、当地日期）确定性派生。
  - 方向身份 = NR 连接代码 + 起讫节点，按 NR 拼写不透明保存。
- 产品策略：
  - 周期文法只内置一种：按周、当地时刻 + IANA 时区、生效区间、可选逐维容量声明。
  - 物化只在两处：准备时评估候选，以及显式登记具体班次。
- 机制：周期安排、班次、容量池走登记册配置写，CLI 与端点同用例。
- 产品策略：TF 消费初始路由已形成；无路线判断零写入；该连接空册答 `未配置` 并入账。
- 产品策略：
  - 择优：最早合格出发，然后按标识，取首个容量够用的。
  - 无池答 `容量待确认`；全部不够答 `当前机会不可用`；有候选但窗口外答 `无当前运输机会`。
  - 不预占，不建占位班次。
- 产品策略：收寄前需求量取 PS 申报并标来源；只认重量、体积、件数（件数只配件数维池）；跨维不折算。
- 机制：结果按（租户、计划段引用、包裹、规则版本）唯一；不发信封；改路新计划的缺口交 NR。
- 越权风险点：事件放大；只有按周一种形；用申报量；未配置结果不自动重备；改路缺口；连接代码跨版本相认。

**ADR-0202**
- 提交委托、提交订舱、取消委托、取消订舱、段采用委托，各是一种运营决定。
- 承运答复走集成客户端：新外部事实类型 `CARRIER_BOOKING_ANSWER`，客户端来源身份必须等于服务提供方。
- 协议核对：在册、在提交时点生效、供应商等于服务方。四个业务否定格答 200，PC 读不动答未决。采购责任法人从协议快照。
- PC 新开窄读口：按（租户、对象、版本标签）答版本壳与正文。
- 订舱必须引用在册班次。承运答复加 `WAITLISTED`。`待确认`、`待补充` 是读法，不是答复值。
- 委托信封照 ADR-0066 不登记路由；改掉过时注释。
- 越权风险点：没有人工转录口；条件、角色、责任不核；运输范围覆盖不核；委托信封的红日志；种类粒度。

**ADR-0203**
- 判据（产品策略）：班次挂了容量池，装载分配就必须对每个池引用一份有效预占并声明分配量；确认订舱同理。
- 分配关系守恒为「已分配 ≤ 预占 − 释放 − 消耗」，撤回或改版时归还。形成分配时不 `Consume`。池所在班次必须等于分配所在班次。
- 分配量逐池逐维度声明并带来源。只有件数维池可以用成员数。
- 预占与释放是两种运营决定。触硬上限答 `CAPACITY_EXCEEDED` 带余量，不能审批越过。
- 新格：`CAPACITY_RESERVATION_REQUIRED`、`RESERVATION_NOT_ON_SCHEDULE`、`ALLOCATION_EXCEEDED`。
- 越权风险点：无池即无保护；分配量是声明值；软超配仍归 psb/09；资源与载运位置粒度不支持。

**ADR-0204**
- 运营决定声明（段、委托）。段须在册，委托须已提交且服务方一致。一段至多采用一份当前委托。
- 采用即 `MarkTransportStarted`，依据取该段首个参与。
- 取消面就此开放：已开始答 `TRANSPORT_ALREADY_STARTED`，只取消尚可停止的范围。
- 连线是声明，不是推导。没有连线的段不能推断为自营。
- 揽收↔委托仍按 ADR-0098 决定四不建。
- 越权风险点：开始时点取首个参与；没有反向解除采用的面。

**ADR-0205**
- 出发一律落册。读数封闭五格（放行 / 阻断 / 不受门禁 / 未决 / 读不动），与事实同一笔保存。退役拒收格。
- 写事务前经消费侧端口同步拉 CC。
- 设备的 `gateRequired` 失去权威，`gateClearance` 只作证据。
- 移动事实带声明的段引用，不加外键。对象取段的当前参与。段不在册照记。不核班次在册。
- 信封与 VE 消费者在同一个变更上线。阻断读数派生异常信号。
- 越权风险点：违规出发被记录而不是被挡；读不动也照记；外包段的门禁在交接上，等 peer；出发前读口延后。

**ADR-0206**
- CC 的「动作准入判断」是请求驱动的（ADR-0137 决定二）。输入：租户、对象、动作意图（出发自 A 到 B / 交接 / 交付）、时点。
- 出发对应 `LOADING_DEPARTURE`；节点关务区域不同时另加 `CROSS_CUSTOMS_MOVEMENT`。
- 逐对象逐动作折叠限制可准入、门禁核对、外部放行三源。门禁满足不答放行；外部放行不解内部限制。
- 新登记册「节点关务区域」，值属租户取值。节点未登记而该对象有跨区目录行时答 `未配置`。
- 对象的范围与程序由 CC 解析，解析不到答未决。
- 判断记录可以重放，不入队。现有 `VerifyReleaseGate` 与 `gate-verification.recorded` 不变。
- 越权风险点：限制无生产写入；区域粒度按节点；演示只到 scope 一级。

**ADR-0207**
- 一个登记用例覆盖四种原因。事实依据与业务时间都取自事实本身：
  - 揽收或派送的失败尝试；
  - 已结束的参与（实际履约）；
  - 已接受的承运答复（订舱）；
  - 未开始范围的取消。
- 采购上下文由调用方给出，经 PC 核对，法人取自协议。段有采用连线时取委托快照，与调用方值不一致答冲突。缺采购上下文答不受理，不答不适用。
- 自营只能显式声明。
- 发生条件：ADR-0209 落地前，由登记者断言并留痕。
- 身份确定，首版 V1。旅程由登记者声明。
- 信封用引用式载荷与 `FingerprintEventID`，与保存同一笔。不在物理编排里内联。
- 越权风险点：断言期；旅程无人核；不自动形成。

**ADR-0208**
- SA 在派发里消费发生项信封，按引用回读。
- 发生项与协议取自发生项本身；费用项目来自触发册的新格；主要范围来自协议的商业范围。
- 未配置入账，读不动未决。
- 越权风险点：一个原因只对一个费用项目；范围等同协议范围；`Request` 仍可被直调（同 ADR-0164 点 3）。

**ADR-0209**
- 供应商协议正文加「收费发生条件」：逐原因声明计费与否，加数量形，定出数量与单位。
- 发布成新版本，旧版本不可变。
- TF 在登记时按快照判断：没声明答 `未配置`；声明不计费的原因答「协议不计此原因」。
- 越权风险点：文法偏简（无阈值、无免费额度）；演示协议要换版。

**不需要新 ADR，落在现行 CONTEXT / UC / 既有 ADR 之内的**：
- `SCHEDULE_NOT_FOUND`：UC-TF-004 步 1 + ADR-0029 / 0022，只补 AT 行。
- 委托信封不登路由：ADR-0066 已裁。
- psb/20 的触发策略：ADR-0164 已裁，触发面已在 `7c084917` 落地。
- 演示各段方式：租户取值，写进演示文档。
- 容量消耗交接的 ID：走 envelope-id-bounds/01。
- 订舱连接器、外包轨迹：psb/09。

**随票同步的文档**：
- TF CONTEXT：Language 补周期安排、运输机会、运输准备结果；Rules 补保障判据、段采用委托、门禁读数。
- CC CONTEXT：动作准入判断、节点关务区域。
- PC CONTEXT：收费发生条件。
- SA CONTEXT：一句话写明 BUY 由信封发起。
- CONTEXT-MAP：NR→TF 消费、TF→CC 拉取、TF→SA 信封、TF→VE 移动。
- UC-TF-003 / 004 / 005 / 006、UC-SA-002、UC-CC 准入判断。
- 参数登记册：PAR-NET-08 按 ADR-0146 拆成三类。

---

## 票拆分

票号接 `demo-chain-gaps/` 往后编。迁移号为建议值，认领时占号：TF 0024 起，CC / PC / SA 各取下一个空号。所有「真库测试」都用一次性库，按 `seed.sh` 原样灌。

| 票号建议 | 标题 | 地盘 | Blocked by | 完成判据 | 规模 |
|---|---|---|---|---|---|
| 16 | 周期安排与具体班次登记（01·一） | TF：`domain/transport_schedule_capacity.go`、新 `schedule_template.go`、`payload_canonicalization.go`；`adapters/postgres/transport_schedule.go`、新 `schedule_template.go`、`review_catalogue.go`；`ports/catalogue_read.go`、新 `ports/transport_opportunity.go`；tfhttp 新 `register_transport_schedule*.go`、`query_transport_fulfillment_records.go`；迁移 TF 0024；新 `cmd/parcel-transport-register`；新 `cmd/parcel-api/assemble_transport_schedule.go` + `endpoints.go`/`main.go` 行；种子 `data/transport/`、`seed.sh`、README；文档 ADR-0201、TF CONTEXT、UC-TF-003、PAR-NET-08 拆分 | 无 | 周期安排、具体班次两口和 CLI 同用例。同键同内容答已存在，异内容成新版本。（模板、日期）物化确定且幂等（真库）。自由串方向退役（表无生产行）。查阅口新增 `transport-schedule-template`。生产空册答 `未配置`。演示：seed 后查阅口列出三条 SYN 周期安排；显式登一个班次答 201 | L |
| 17 | 容量池登记（01·二） | TF：`domain` 池与模板容量声明；`application/prepare_transport_opportunity.go` 的 `EstablishPool`；`postgres/capacity_pool.go`、`review_catalogue.go`；迁移 TF 0027；tfhttp 新 `register_capacity_pool.go`；CLI 加一个 kind；装配与 `endpoints.go` 行；种子 | 16 | 池登记口与 CLI。`EstablishPool` 对不在册班次答 `SCHEDULE_NOT_FOUND`。模板的逐维容量随物化建池且幂等。多维分存（AT-TF-030）。真库测试。演示：查阅口 `capacity-pool` 列出物化班次上的 SYN 池 | M |
| 18 | 运输准备结果与初始路由消费者（01·三） | TF：新 `domain/transport_opportunity.go`；新 `application/prepare_opportunity_on_planned_legs.go`；新 `adapters/networkrouting/planned_leg_source.go`；新 `adapters/parcelshipment/declared_demand_source.go`；新包 `adapters/inbox/`；`postgres` 新 `transport_opportunity.go` + `review_catalogue.go`；迁移 TF 0028；`cmd/parcel-dispatch/assemble.go`(+_test) | 16；dcg08、dcg11 进 main（派发令牌）；软依赖 17 | 无路线判断零写入。`未配置`、`无当前运输机会`、`容量待确认`、`当前机会不可用`、`已准备` 各有真库测试。重放答 `已有结果`。TF 未决重投时 VE inbox 不重复投影。路由测试改成两路。完成记录登改路缺口（→34）。演示：提交一单后，`registry=transport-preparation` 列出三段，至少一段 `运输机会已准备`；窗口对不上就调 SYN 模板时刻，不改匹配规则 | L |
| 19 | 装载分配核班次在册（03a） | `application/form_load_assignment.go`(+_test)；`cmd/parcel-api/assemble_segment_operations.go`(+_test)、`load_assignment_single_chain_test.go`、`storable_values_channel_test.go`；tfhttp `form_load_assignment_test.go`、`segment_operations_test.go`；UC-TF-004 AT 行 | 无（演示一步要 16 或 18 的在册班次） | 新格 `SCHEDULE_NOT_FOUND`（200），放在重放与链头判定之后。读失败答 `UNDECIDED`。他租户引用落同一格。真库测试先补登班次行。不碰 `OpenDispatchTaskDeps`。演示：对物化班次分配答 201，对乱填班次答 `SCHEDULE_NOT_FOUND` | S |
| 20 | 容量预占与释放运营决定面 | `internal/accessidentity/operation_decision.go`(+_test)；tfhttp `operator_decision_intake.go`(+test)、新 `reserve_capacity.go`/`release_capacity.go`；`application` 的 Reserve/Release 带需求来源；新 `cmd/parcel-api/assemble_capacity_reservation.go` + `endpoints.go` 行；种子 access 02/03、governance 新区间；ADR-0203、TF CONTEXT 一句 | 17 | 两种决定种类。预占必须带需求量与来源。`CAPACITY_EXCEEDED` 带余量，没有审批越过（AT-TF-031）。重放幂等。真库测试。演示：配置员对 SYN 池预占答 201 | M |
| 21 | 装载分配的容量关系（03b） | `domain/load_assignment_movement.go`、`load_assignment_rehydration.go`、`transport_schedule_capacity.go`；`ports/load_assignment.go`（不改 `ports.go` 签名）；`postgres/load_assignment_registry.go`、`capacity_pool.go`；迁移 TF 0031；tfhttp `form_load_assignment.go`；`assemble_segment_operations.go`；UC-TF-004 AT | 19、20 | 三个新格：`CAPACITY_RESERVATION_REQUIRED`、`RESERVATION_NOT_ON_SCHEDULE`、`ALLOCATION_EXCEEDED`（带余量）。撤回或改版归还，守恒有真库测试。不调 `Consume`。无池班次照旧形成。演示：带预占引用的分配答 201，超量答 `ALLOCATION_EXCEEDED` | L |
| 22 | 供应商协议版本点读口与 TF 协议核对适配器 | PC `ports/ports.go`（加窄口）、`adapters/postgres/supplier_agreement.go`；TF 新 `ports/supplier_agreement.go`、新 `adapters/partycommercial/supplier_agreement_source.go` | 无 | 按（租户、对象、版本标签）答版本壳与正文。适配器译成：在册 / 未生效 / 已终止 / 服务方不符 / 法人。真库测试覆盖 SYN 两份协议。不动 cmd | S |
| 23 | 运输委托提交面 | `operation_decision.go`；tfhttp `operator_decision_intake.go`、新 `submit_transport_commission.go`；`application/commission_transport.go`（核对、法人快照、改注释）；`domain/transport_commission.go`、`commission_rehydration.go`；`postgres/transport_commission.go`；迁移 TF 0025（法人列）；新 `cmd/parcel-api/assemble_transport_commission.go` + `endpoints.go` 行；种子；ADR-0202、UC-TF-004 AT、演示文档 | 22 | 四个否定格加未决格。委托与 Outbox 同一笔（真库）。路由不登，`no_subscriber` 如实可见。演示：干线委托答 `SUBMITTED`；服务方填末端承运商答 `PROVIDER_MISMATCH` | M |
| 24 | 订舱提交（引班次）与承运答复集成客户端面 | `operation_decision.go`、`integration_client.go`；tfhttp 新 `submit_booking_request.go`、`answer_booking.go`、`integration_client_intake.go`；TF 新 `adapters/accessidentity/integration_client_authenticator.go`；domain、postgres 订舱（班次引用、`WAITLISTED`）；迁移 TF 0029；`assemble_transport_commission.go`、`assemble_integration_client.go`、`endpoints.go` 行；种子 access 02/03/05/06、governance | 23、16 | 引用不在册班次答 `SCHEDULE_NOT_FOUND`。客户端来源身份不等于服务方答不受理。答复五值。幂等。真库测试。演示：对 SZX→SIN 物化班次订舱，合成承运客户端用令牌回 `ACCEPTED` | L |
| 25 | 履约段采用运输委托；取消委托与订舱 | `operation_decision.go`；tfhttp 新 `adopt_segment_commission.go`、`cancel_transport_commission.go`、`cancel_booking_request.go`；`application` 新 `adopt_segment_commission.go` + `CancelBooking`；`domain` 新 `segment_commission_adoption.go`；ports 新文件（含 `BookingStore.Replace`）；postgres 新表，迁移 TF 0030；装配与 `endpoints.go` 行；种子；ADR-0204（ADR-0098 加后继指针）、TF CONTEXT | 23、24 | 段不在册答 `SEGMENT_NOT_FOUND`。服务方不一致答冲突。采用即标开始，随后取消答 `TRANSPORT_ALREADY_STARTED`（AT-TF-092）。未开始的可以取消。真库测试。演示：用现有交接口立干线段后采用委托答 201，再取消委托答已开始 | M |
| 26 | SA 消费发生项已形成信封，发起 BUY 评价（收 10 的 BUY 半） | SA 新 `adapters/transportfulfillment/`、PC 范围读、新 inbox 消费者；触发册加费用项目（domain、ports、postgres、迁移 SA 新号）；`cmd/parcel-settlement-register`；`cmd/parcel-dispatch/assemble.go`(+_test)；种子 settlement BUY 行；ADR-0208（ADR-0164 加后继指针）、UC-SA-002、SA CONTEXT | dcg08 进 main（ADR-0200、派发令牌） | 路由先登（生产方未上线，ADR-0049 允许）。无行或无费用项目答 `未配置` 并入账。读不动登哨兵。真库测试用手插的 TF 发生项行跑到 BUY 请求 `FIRED`。端到端演示证据随 27 | M |
| 27 | 运输收费发生项显式登记与已形成信封（失败尝试、实际履约） | TF `domain/transport_charge_occurrence.go`；`application` 新 `register_charge_occurrence.go` + `register_failed_attempt_charge.go`；新 `ports/charge_occurrence.go`；新 `postgres/charge_occurrence_handoff.go`；新 tfhttp `register_charge_occurrence.go`；`operation_decision.go`；新 `cmd/parcel-api/assemble_charge_occurrence.go` + `endpoints.go` 行；`internal/architecture/production_wiring_baseline.txt`；种子；ADR-0207、UC-TF-005 步与 AT、UC-TF-006/002 指向 | 22、26 | 结果格 `REGISTERED` / `EXISTING` / `NOT_APPLICABLE`（仅显式自营）/ `UNDECIDED` / `INPUT_NOT_ACCEPTED`，加协议否定格。保存与 Outbox 同一笔（真库），用指纹 ID。棘轮剪掉一条。不碰交接、交付编排。演示：干线参与结束后登 `ACTUAL_FULFILLMENT` 答 201 → 26 → BUY 请求 → 计价评价 → ADR-0200 形成供应商预期成本（需 30） | L |
| 28 | 订舱与取消发生项；实际履约经段采用取采购上下文 | TF `domain/transport_charge_occurrence.go`；`register_charge_occurrence.go`；新读端口文件；tfhttp `register_charge_occurrence.go` | 24、25、27 | 订舱的依据是已接受的答复；取消的依据是未开始范围。段已采用委托时可以省略采购上下文，给了但不一致答冲突。真库测试。演示：干线订舱被接受后登订舱费发生项 | M |
| 29 | 供应商协议声明收费发生条件 | PC `domain/publication_canonicalization_supplier_agreement.go`、`supplier_agreement.go`、发布用例、http 草稿载荷、postgres + 迁移 PC 新号；TF 登记时判条件；种子 `commercial/publish-batch.json` v2；ADR-0209、PC CONTEXT | 27 | 新摘要文法，v1 不变。没声明答 `未配置`。不计费的原因有业务格。数量形定出数量与单位。真库测试。演示：用 v2 干线协议登实际履约，不再需要断言 | L |
| 30 | BUY 成本卡声明金额取整（09 余半） | `scripts/demo-seeds/data/pricing/` 成本卡新版本、`seed.sh` 一行 | 无 | 成本卡以新版本重发并声明取整。`seed.sh` 退出码 0。走 ADR-0200 路径时不再落「金额未声明」格 | S |
| 31 | 关务动作准入判断与节点关务区域 | CC 新 `application/judge_action_admission.go`；ports 新文件（对象→范围/程序、节点区域）；`domain` 新 `node_customs_zone.go`；postgres 新册 + 迁移 CC 新号；CC http 登记口；`cmd/parcel-customs-register` 新 kind；`cmd/parcel-api` 装配 + `endpoints.go` 行；种子 customs 四个节点区域；ADR-0206、CC CONTEXT、UC-CC、CONTEXT-MAP | 无 | 按区域定适用动作。三源折叠成封闭答复，门禁满足不答放行。节点未登记答 `未配置`。判断记录可重放、不入队。真库测试覆盖放行 / 阻断 / 不受门禁 / 未决 / 未配置。演示：SHA→SZX 只适用 `LOADING_DEPARTURE`；SZX→SIN 另加 `CROSS_CUSTOMS_MOVEMENT` | L |
| 32 | 移动事实声明段与出发门禁读数 | TF `domain/load_assignment_movement.go`、`movement_fact_rehydration.go`、`payload_canonicalization.go`、新 `departure_gate_reading.go`；`application/record_movement_fact.go`；新 `ports/departure_gate.go`；新 `adapters/customscompliance/departure_gate.go`；`postgres/movement_fact_registry.go`；迁移 TF 0026；tfhttp `record_movement_fact.go`；`cmd/parcel-api/assemble_movement_fact.go`(+_test)、`movement_fact_same_identity_test.go`、`transport_fulfillment_command_lines_test.go`、`storable_values_channel_test.go`；ADR-0205、TF CONTEXT、UC-TF-005 | 31；oc48 进 main（同一测试文件） | 出发一律落库。五格读数与事实同一笔（真库）。CC 读不动也落库，记 `读不动`。`gateRequired=false` 照样问 CC。段不在册照记。不改 `work_fact_intake*.go`、`assemble_operator_decisions.go`。演示：自营 SHA→SZX 出发答 201，读数 `不受门禁` | L |
| 33 | 移动事实已形成信封与 VE 投影 | TF 新 `postgres/movement_fact_handoff.go` 与端口、`record_movement_fact.go`；VE 新 inbox 消费者与派生；`cmd/parcel-dispatch/assemble.go`(+_test)；`cmd/parcel-api/assemble_movement_fact.go`；种子 visibility 映射（参考配置 + SYN 采用） | 32；dcg11 进 main（同映射种子与派发） | 生产方与消费者在同一个变更。哨兵登列。阻断读数派生异常。真库测试。演示：出发后 `/tracking-projections` 出现 DEPARTED | M |

**后继票**（不在演示主链上，按需立）：

| 票号建议 | 标题 | Blocked by | 规模 |
|---|---|---|---|
| 34 | NR：复评新计划已形成信封 + TF 准备消费者（TF 先登路由） | 18 | M |
| 35 | NR：路由偏离消费移动事实 | 33 | M |
| 36 | TF：授权再准备请求面（运营决定，复用 18 的用例） | 18 | M |
| 37 | TF：确认订舱的容量关系（ADR-0203 同一判据） | 21、24 | M |
| — | 容量消耗交接的事件 ID 与重放幂等，并入 envelope-id-bounds/01 运输履约半边 | — | S |

原票 01、02、03、05、07 改成「拆分」，指向上表；08、11 随各自分支收口；09、10 由 30、26 收余半。

---

## 并行车道

**车道**（代码可以并行写，进 main 按下面的令牌串行）：

| 车道 | 票序 | 车道内独占的文件 |
|---|---|---|
| A 机会与容量 | 16 → 17 → 18；19 可随时并行；20 → 21 | TF 班次 / 池 / 准备 / 装载分配的 domain、postgres；`review_catalogue.go`、`ports/catalogue_read.go`、`query_transport_fulfillment_records.go`；`assemble_segment_operations.go` |
| B 委托与订舱 | 22 → 23 → 24 → 25 | TF 委托 / 订舱 domain、postgres、`commission_transport.go`；`assemble_transport_commission.go`；PC 点读口 |
| C 金额 | 30（任意时刻）；26 → 27 → 28；29 在 27 之后 | TF 发生项 domain、handoff；SA 全部；PC 文法 |
| D 门禁与移动 | 31 → 32 → 33 | CC 全部；TF 移动事实 domain、postgres、`assemble_movement_fact.go`；VE 移动消费者 |

**串行令牌**（跨车道共用的热点文件。一次只有一张票持令牌落地，后到的 rebase，热点文件只追加行）：

- **派发令牌**：`cmd/parcel-dispatch/assemble.go`、`assemble_test.go`。顺序：dcg08 → dcg11 → 26 → 18 → 33 → 34 / 35。
- **装配令牌**：`cmd/parcel-api/endpoints.go`、`main.go`。新接线一律另立 `assemble_*.go`。顺序：16 → 31 → 17 → 23 → 27 → 20 → 24 → 25 → 32。
- **决定种类令牌**：`internal/accessidentity/operation_decision.go`、tfhttp `operator_decision_intake.go`(+test)。顺序：23 → 27 → 20 → 24 → 25。`integration_client.go` 只有 24 改。
- **种子令牌**：`scripts/demo-seeds/seed.sh`、README、`access/02-grants.json`、`03-admission-scope-mappings.json`、`governance/`。只追加。顺序：30 → 16 → 31 → 17 → 26 → 23 → 27 → 20 → 24 → 25 → 33。每次落地后把 SHA 通知 9b（psb/30、31 惯例）。
- **TF 规范化令牌**：`domain/payload_canonicalization.go`。顺序：16 → 17 → 23 → 32 → 24 → 21。新聚合（准备结果、采用连线）的规范化另立文件。
- **文档令牌**：TF CONTEXT 与 UC-TF-00x。顺序：16 → 23 → 20 → 25 → 32 → 27。`docs/adr/README.md` 只追加。
- **其他**：`docs/product/MECHANISM-INVENTORY.md` 每次落地重新生成，冲突时以重新生成为准。迁移号、ADR 号认领时占号。

**波次建议**：

- **波 0（在途分支）**：dcg08、dcg11、oc48、oc54 进 main；envelope-id-bounds/01 运输履约半边进 main（已吸收 D03-7）。
- **波 1**：16、19、22、30、31、26。落地顺序 19 → 22 → 30 → 26 → 16 → 31。
- **波 2**：17、23、27、32。落地顺序 17 → 23 → 27 → 32。
- **波 3**：18、20、24、29、33。落地顺序 20 → 24 → 18 → 33 → 29。
- **波 4**：21、25，之后 28。

**演示动线终态**（补进 `synthetic-demo-journey-script.md`，证据 `S`）：
1. 提交 → 初始路由 → 三段准备结果。
2. SHA 收寄 → 用现有交接口立 SHA→SZX 自营段 → 对物化班次装载分配（带预占）→ 出发，门禁读数 `不受门禁` → VE 出现 DEPARTED。
3. 干线委托 → 订舱 → 合成承运商回 `ACCEPTED` → 在 SZX 交给承运商立干线段 → 段采用委托。
4. 在 SIN-HUB 交接，参与结束 → 登 `ACTUAL_FULFILLMENT` 发生项 → BUY 请求 → 供应商预期成本。
5. 末端照 psb/05 现行走法到终局。

---

## 等 peer 的

| 事项 | 为什么等 | 前置 | 收工后去处 |
|---|---|---|---|
| 04 交接后节点控制转移，NR 复核 | 在 `internal/nodeoperations/**` 和交接路径上 | — | 原票 04（交接信封扇出到 NO、NR） |
| 06 派送发起与末端段 | 在 `trigger_delivery_dispatch.go`、`open_dispatch_task.go`、`assemble_delivery_dispatch.go` 上，`claude-tfa09` 也在动这些文件 | — | 原票 06；末端是否外包随它定（X1） |
| 外包干线交接的跨境门禁 | 门禁要挂在 `register_transport_handover.go` 上 | 31 | 交接登记读 ADR-0206 准入判断，按 ADR-0205 照记读数 |
| 有效交付的 `FINAL_DELIVERY` 门禁 | 在 `register_effective_delivery.go` 上 | 31 | 同上 |
| 删掉移动命令与载荷里的 `gateRequired` / `gateClearance`；设备出发前门禁读口 | 在 `work_fact_intake*.go` 上（peer 的测试钉着透传） | 32 | 32 的后续小票 |
| NO 实测触发的再准备 | 要读 `internal/nodeoperations` 的端口，现在还在变 | 18 | 新 TF 消费者票 |
| TF 自消费交接 / 有效交付信封，自动形成 `ACTUAL_FULFILLMENT` | 路由行与 04、06 落在同一类型上 | 25、29 | 新票（ADR-0207 第七条的后继） |
| admin-web 各页（班次、池、委托、订舱、发生项、门禁读数） | 在 `apps/admin-web/**` 上，走前端 main 直做流程 | 各对应票 | 前端切片 |
| 19 与 `buildSegmentOperations` 同函数相邻 | peer 若改 `OpenDispatchTaskDeps` 会碰同一函数 | — | 不阻断；落地前 rebase |
| `cmd/parcel-api/assemble_operator_decisions.go`（`frontlineMovementSource`、`buildWorkFactIntakes`） | 与 9b 的收寄接线相邻 | — | 本计划所有票都不碰 |

---

## 已不成立或已修

- **01**
  - 「没有班次、容量池、预占端点」只对写入成立。读面 `/transport-fulfillment-records?registry=transport-schedule|capacity-pool` 已接 postgres，只是空册。
  - 「缺一个 DecisionKind」轴不对：登记班次和池走登记册配置写，种子已授。
  - ADR-0131 越权点 4 不在消费者路径上，适配器能按计划版本加序位自己造引用。
  - 「硬停」只相对演示判据。实物主链在 `851e9da7` 已不靠班次走到终局。
  - 体量 L 实为 XL，已拆成 16、17、18。
- **02**
  - psb/31 已在 `ae257777` 收口，集成客户端不再受阻。
  - 「`transport-commission.submitted` 不登路由」ADR-0066 早已裁过。
  - 「交 SA 作成本预期源」的注释与 SA CONTEXT、ADR-0164 不符。
  - NR `plannedLeg`「没有责任方列」不在本票关键路径上。
- **03**
  - 方法名是 `PrepareTransportOpportunityHandler.Consume`，不是 `ConsumeCapacity`。
  - 「经 ConsumeCapacity 同笔转换数量」是错的：`Consume` 是永久占用，撤回时会漏容量。
  - 证据包说「加班次核对后每次演示分配都会变成 `SCHEDULE_NOT_FOUND`」，与 psb/05 重走对不上：`851e9da7` 那次的步 1–8 没有装载分配这一步。受影响的只有三个真库测试，所以 19 可以独立落地，不让演示倒退。
- **05**
  - 「集成客户端在演示上配不了」已过时（psb/31 已收）。外包轨迹那一半与 psb/09 第 1 项重复。
  - 「消费者就位之前不登路由」说反了：ADR-0049 要求消费者先进路由表。
  - 用装载分配成员连对象，违反 TF CONTEXT。
  - 演示干线外包，所以修自营移动不会给 SZX→SIN 加门禁。
  - 证据包建议「每次出发都问两种动作」会让境内段被出口门禁挡住，已按 D05-5 改为由 CC 按节点区域判。
- **07**
  - 「采购法人、服务方、协议无处可取」说过头了：PC `LoadSupplierAgreement` 能答供应商和法人，委托上已有协议快照引用。
  - psb/20 的触发策略已由 ADR-0164 裁定，代码已在 `7c084917` 落地，psb/20 的 needs-triage 状态过时。
  - 「Blocked by: 无」不对：订舱、取消两种原因依赖 02。
  - 派送失败尝试不需要新存储，`DeliveryAttemptView` 已在。
  - 票面没提 ADR-0098 决定四，本计划由 ADR-0204 部分停用。
  - 证据核于 `81a4c418`，比其余几票旧，认领时要在新 main 上复核。
- **08**：已在 `claude-dcg08`（ADR-0200）解决，待进 main。cmd 注释里「两头没接上」已过时。
- **09**：SELL 半已收。BUY 成本卡确实没声明 `amountRounding`（种子实测），由 30 收。
- **10**：SELL 半已收。BUY 半的原因定为 `ACTUAL_FULFILLMENT`，由 26 收。
- **11**：已在 `claude-dcg11`（ADR-0197）解决，待进 main。
- **psb/05 那句话**：「ADR-0098 决定三让自营包裹永远形不成客户费用」已由 ADR-0190 的 SELL 计费时点路径解除。SELL 不经发生项。