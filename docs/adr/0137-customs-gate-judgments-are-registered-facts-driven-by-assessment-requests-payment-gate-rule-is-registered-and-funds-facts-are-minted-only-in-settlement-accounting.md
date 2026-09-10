# ADR-0137：关务就绪的逐门禁判断是 `customs-compliance` 登记的事实、就绪判断只按不可变引用绑定它（凭证门禁为第一册）；门禁判断由评估请求驱动、来源变化只让既有判断失效不后台重算；放行门禁里「税费付款」那一道的读法是按监管程序登记进来的规则（对覆盖 · 差额 · 有效性三态各自的接受集合），未登记答「规则未配置」、没有默认；外部资金事实进入本产品只有 `settlement-accounting` 采用这一口，`customs-compliance` 不开第二个铸造或补录入口

Status: Accepted（2026-09-10，通道 5 按通道 1 派单 task-9a2ff746「用户授权代裁，CC owner 口径，决定四连 SA owner 口径：硬句不改、拿不准的单列越权风险点」裁决。裁决能力边界：读过票 [sa-cc/spec](../../.scratch/sa-cc-funds-and-credential-seams/spec.md) 与 [01](../../.scratch/sa-cc-funds-and-credential-seams/issues/01-buy-evaluation-to-sa-inbox-consumer.md)–[07](../../.scratch/sa-cc-funds-and-credential-seams/issues/07-cc-credential-and-duty-reconciliation-registration-faces.md) 全文、[CC CONTEXT](../domain/customs-compliance/CONTEXT.md)「监管凭证」「税费付款核对」「放行门禁核对」词条、Rules「申报就绪、授权与提交」「监管凭证、限制与处置」「税费、放行与案件闭环」三节与 Lifecycles「税费付款与放行门禁」一节、Boundaries 末两句、[SA CONTEXT](../domain/settlement-accounting/CONTEXT.md)「实际代垫成立判断」「客户代垫回收」词条、Lifecycles「实际代垫与客户代垫回收」一节、Boundaries 关务与财务两句、[UC-CC-003](../application/customs-compliance/UC-CC-003-ASSESS-DECLARATION-READINESS.md) 范围、结果表、「就绪门禁」表与业务规则节、主流程步 4 / 6 / 7 / 10 / 11 那几行、[UC-SA-001](../application/settlement-accounting/UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md) 概述、输入表「外部资金事实」「付款核对」两行与业务规则「同一外部资金事实……只能被采用一次」句、[UC-SA-002](../application/settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md) 步 2 与「供应商预期成本已形成」行、[ADR-0069](./0069-customs-case-chain-ordering-absorbed-by-reread-and-retry.md) 决定二、[ADR-0074](./0074-tf-object-partitions-carry-a-port-segment-apart-from-ve-parcel-partitions.md) 决定五、[ADR-0013](./0013-pricing-owns-versioned-external-reference-series.md) 决定节、`internal/customscompliance/ports/ports.go` 的 `GateConditionCatalogueEntry` 头注与 `GateConditionRegistry` / `GateConditionCatalogueRead` 签名、`internal/settlementaccounting/adapters/postgres/supplier_bill_handoff.go` 与 `operating_handoff.go` 关于分区键的头注。没读 `verify_release_gate.go` / `register_readiness` / `judge_credential_applicability.go` 的实现体、CC postgres 适配器、SA `map_external_funds.go` 实现体。）
Date: 2026-09-10

## Context

[mech/07](../../.scratch/mechanism-executor-triage/issues/07-cc-four-executors-behind-existing-uc-steps.md) 收口时留下的四件 CC 机制与 [mech/06](../../.scratch/mechanism-executor-triage/issues/06-sa-four-executors-behind-existing-uc-steps.md) 留下的两件 SA 机制，2026-09-10 由通道 4 立成 [sa-cc-funds-and-credential-seams](../../.scratch/sa-cc-funds-and-credential-seams/spec.md) 七张票。七张里十三条「要裁的」按「推送方可代裁 / owner 口径代裁 / 只有产品能答」分过类（task-b941ce87），本记录裁其中动 CC 领域形状或跨上下文归属的四条；其余九条是落位与命名，裁决直接写进各票「裁决」小节，不进本记录。

四条题各自今天的形状：

- **凭证门禁只判不记**（票 [04](../../.scratch/sa-cc-funds-and-credential-seams/issues/04-cc-credential-gate-persists-in-readiness-assessment.md)）。`JudgeCredentialApplicability` 算得出四格，头注写「本用例只判、不记：步 7 写的『记录凭证门禁』那半留给就绪判断的编排」；今天就绪判断是带依据引用登记进来的事实（`RegisterReadiness` 收 `ReadinessBasisReference`），UC-CC-003 步 3–10 没有逐门禁计算的编排。UC-CC-003 范围节写「形成不可覆盖的就绪判断版本，保存每项门禁的依据、适用时间、判断方式、责任角色和结果」，「就绪门禁」表列七道门，步 7 那行「核验监管凭证……→ 记录凭证门禁；不占用、释放或核销」。要裁的是：门禁记录是就绪判断的一格（改 `RegisterReadiness` 的形）还是独立登记册被就绪判断引用；以及谁触发判断——评估请求到达时算一次，还是凭证登记 / 程序变更时重算。
- **放行门禁不读付款核对**（票 [06](../../.scratch/sa-cc-funds-and-credential-seams/issues/06-cc-release-gate-reads-duty-payment-verification.md)）。`VerifyReleaseGate` 的依赖里没有 `DutyVerificationStore`；CC CONTEXT 要求「放行门禁核对必须绑定……税费付款核对」，又要求覆盖 / 差额 / 有效性「分别表达，不能实现为一组互斥总状态」，还说「税费支付是否是放行前置条件，取决于当前监管程序的适用规则；本上下文不得统一假设『先税后放』或『先放后税』」。门禁目录今天登记的是前置条件**认定**（`GateConditionCatalogueEntry.Findings`，三值封闭、整行缺席才是未决、空清单是「此动作在此边界本就不受门禁」）。要裁的是：三态怎么折成「税费付款」这一道门禁——折法是登记进来的规则还是编排常量。
- **CC 的资金事实人工补录口**（票 [07](../../.scratch/sa-cc-funds-and-credential-seams/issues/07-cc-credential-and-duty-reconciliation-registration-faces.md)）。票 [02](../../.scratch/sa-cc-funds-and-credential-seams/issues/02-sa-external-funds-fact-adoption-hands-off-an-envelope.md) + [03](../../.scratch/sa-cc-funds-and-credential-seams/issues/03-cc-inbox-consumer-receives-external-funds-fact.md) 落地后 CC 的 `ReceiveFundsFact` 由 SA 采用信封驱动；07 步一原拟给 `cmd/parcel-customs-register` 加 `external-funds-fact` 子命令作运维补录口。SA CONTEXT 与 CC CONTEXT 各有一句同义：「银行、支付或财务系统拥有实际付款、付款失败、追加付款、资金退回、付款撤销和外部资金事实更正」；SA 采用它（`map_external_funds.go`，UC-SA-001「同一外部资金事实通过回调、文件或人工核实重复到达，只能被采用一次；更正必须形成新来源版本」）；CC 只拥有「税费付款核对……及其与外部资金事实的可追溯关系」。要裁的是：那个人工口保留还是去掉——保留就得答「补录的事实与 SA 采用的事实撞键怎么办」。

用三个场景试过候选：

- **第二道门也要记。** 票 06 让「税费付款」成为放行门禁里一道被机器读出来的门；UC-CC-003 七道就绪门禁里今天没有一道是机器算并落库的。若凭证门禁作就绪判断的一格落进 `RegisterReadiness`，第二道门（比如口岸 / 渠道）落地时再改一次 `RegisterReadiness` 的形，第三道再改一次——就绪判断版本携带每道门的内部结构，每加一道门改一次聚合。若凭证门禁是独立登记册、就绪判断按引用绑定，第二道门照抄第一册的形，`RegisterReadiness` 不动。
- **凭证责任流程刚补齐一份新凭证版本。** UC-CC-003 门禁表第 4 行「失败后的责任入口」写「由凭证责任流程形成有效依据后重新评估」；业务规则写「原判断失效后不得因为缺口补齐、凭证恢复、账号续期……自动恢复。必须引用新依据形成新的判断版本」。若凭证登记触发后台重算并把门禁翻成满足，那就是「凭证恢复 → 自动恢复」；若只在下一次评估请求到达时算，重新评估是一个有人发起、有责任角色、有适用时点的动作，与「不得自动恢复」相容。来源变化让既有正向判断成为`不再就绪`是 UC-CC-003 范围节里另一条规则（失效），它不产出新的门禁判断。
- **某程序下付款根本不是放行前置条件。** CC CONTEXT「当前范围是否需要付款，与付款是否构成某个拟执行动作的放行前置条件必须分别判断」。若折法是编排常量（比如「已覆盖 · 无差额 · 有效才满足」），这个程序下每一份没付款的申报都会被门禁挡住——常量替租户裁了「先税后放」。若折法是登记进来的规则，租户为该程序登一行「不构成前置条件」，门禁如实放过这一道；没登时门禁答「规则未配置」停下，不猜。

## Decision

**一、关务就绪的逐门禁判断是 `customs-compliance` 登记的事实，各成一册；就绪判断按不可变引用绑定它们、不内嵌其内部结构。凭证门禁是第一册。** 凭证门禁登记册的一行带：凭证身份与版本、`JudgeCredentialApplicability` 四格之一的结论、截至时点、依据引用（凭证当前可用依据的来源）、判断时刻与责任角色；同键同内容重放答`已存在`，换内容成新版本不覆盖（与 CC 既有登记册代数一致）。「未登记」与「不适用」两格不得压成一格（票 04 红线；租户上线前每一次判断都会读成「凭证不适用」）。就绪判断版本上「每项门禁的依据、适用时间、判断方式、责任角色和结果」（UC-CC-003 范围节）通过对门禁记录版本的不可变引用成立——门禁记录版本不可变，引用它就是保存它；`RegisterReadiness` 的形不改。后面的门（口岸 / 渠道、限制、规则时效……）落地时照第一册的形各成一册，就绪判断多绑一条引用，不改聚合。`AT-CC-056`「凭证门禁满足并保存适用性和截至时点；本用例不占用或核销额度」由第一册的落地实现。

**二、门禁判断由评估请求驱动：评估请求到达时算一次并登记；凭证登记、程序变更等来源变化不触发后台重算。** 来源变化对既有判断的作用只有一种——按 UC-CC-003「任一适用前提在提交前变化时，原就绪判断不得继续作为当前提交依据」让它成为`不再就绪`，那是失效规则，不是重新判断；新的门禁判断只在新的评估请求到达时形成（UC-CC-003 门禁表第 4 行的「重新评估」读作「再发一次评估请求」）。理由是 UC-CC-003 业务规则「原判断失效后不得因为缺口补齐、凭证恢复……自动恢复，必须引用新依据形成新的判断版本」：后台重算把门禁翻成满足，正是那句禁的自动恢复；请求驱动让每一次判断有发起人、有适用时点、可重放，与仓内「判断不写、请求驱动」的纪律同形。

**三、放行门禁里「税费付款」那一道的读法是按监管程序登记进来的规则，不是编排常量；规则的形是对三态各自的接受集合；未登记答「规则未配置」，没有默认。** 规则挂在门禁目录既有的登记册（`GateConditionRegistry` 那一族，按申报范围 / 拟执行动作 / 监管边界三维键）：这一道的目录行不再由人登结论性的认定，而登一条规则，编排拿当前付款核对版本对着规则折出认定。规则正文两种形之一：「税费付款不构成本动作在本边界的前置条件」；或三个接受集合——覆盖 ⊆ {无覆盖, 部分覆盖, 已覆盖}、差额 ⊆ {无差额, 不足, 超额}、有效性 ⊆ {有效, 失效}，三态各自落在自己的接受集合内才满足，任一不在即未满足。`待确认` 与 `冲突` 不可登记为接受：三态里任一为待确认 / 冲突时该道门禁未决并指名等谁（UC-CC-003「未知不能当作可选、不适用、有效或已解除」）；没有付款核对版本同样未决。门禁记录带三态原值与付款核对版本引用，不带合成布尔（CC CONTEXT「分别表达，不能实现为一组互斥总状态」）。目录里没有这一道的规则行 → 该道门禁答「规则未配置」诚实停点，不取任何默认折法（AGENTS 红线「未确认参数与 `BD-*` 保持可配置或显式未决；不写死为生产默认」）。规则的取值——哪个程序下什么差额能放行——属实例半边 `PAR-CUS-0x`，本记录一个都不拟。

**四、外部资金事实进入本产品只有 `settlement-accounting` 采用这一口；`customs-compliance` 不开第二个铸造或补录入口，人工核实与更正也先登在 SA、再经采用信封到 CC。** SA 采用（`UC-SA-005` / `map_external_funds.go` 的四格）是事实进产品的唯一入口，UC-SA-001「同一外部资金事实通过回调、文件或人工核实重复到达，只能被采用一次」已把人工核实算作同一口的一种到达方式；CC 只拥有「与外部资金事实的可追溯关系」，它的 `ReceiveFundsFact` 是接收已采用事实的引用（票 03 的 inbox 消费者），不是铸造。若 CC 另开人工补录口，同一笔付款会有两个铸造点，「补录的事实与 SA 采用的事实撞键怎么办」这一问就是两个铸造点的冲突本身——去掉入口，问题不存在。因此票 07 步一的子命令是三个（凭证 / 协作 / 付款核对），不含 `external-funds-fact`；这一条只定口径，在票 02 / 03 进 main 之前不动代码。

## Consequences

- 票 04 立「凭证门禁判断」登记册（新迁移序号、ports 一对写读口、postgres 适配器）与「评估请求到达 → 判断 → 登记 → 就绪判断绑引用」的编排；`JudgeCredentialApplicability` 从此有生产调用方；`RegisterReadiness` 的形不动。
- 票 06 在门禁目录里给「税费付款」这一道加规则形（目录行正文从「认定」扩成「认定或规则」，既有认定行一字不变）；`VerifyReleaseGateDeps` 加付款核对读半边；门禁记录加付款核对版本引用一列（引用不快照，同上下文内 ADR-0013 不禁——票 06 要裁的 2 由此定）。
- 票 07 步一 CLI 三个子命令；步二端点 + 管理台照 ADR-0085 决定四做齐（三册同族一致），管理台写签等三册读面（票 [sa-cc/10](../../.scratch/sa-cc-funds-and-credential-seams/issues/10-cc-credential-collaboration-and-verification-read-faces.md)）。
- CC CONTEXT Rules「申报就绪、授权与提交」加一句（逐门禁判断是登记事实、就绪判断只引用、评估请求驱动）、「税费、放行与案件闭环」加一句（税费付款那一道的读法是登记规则、无默认）；SA CONTEXT Boundaries 加一句（资金事实单一采用口）。随本记录同笔。
- 不在本记录内：其余六道就绪门禁各册的字段形状（各自实施票）；`FoldGateConclusion` 对规则型目录行怎么折的实现；管理台上门禁规则登记面的形；票 02 / 05 的分区主体与 03 的读口选型（A 类，写在各票「裁决」小节：分区主体照 ADR-0069 决定二「收窄到业务主体」、主体名照 ADR-0074 决定五登记）。

## Alternatives considered

- **凭证门禁作就绪判断的一格，改 `RegisterReadiness` 的形。** 否决：每加一道门改一次聚合；就绪判断版本携带各门内部结构，与「就绪判断必须绑定其依据」那句里的「绑定」相比是「内嵌」。
- **凭证登记 / 程序变更时后台重算门禁。** 否决：撞 UC-CC-003「不得因为……凭证恢复……自动恢复」；重算没有发起人与适用时点。来源变化的正当作用是失效，已有规则。
- **折法写成编排常量（已覆盖 · 无差额 · 有效才满足）。** 否决：替租户裁了「先税后放」，CC CONTEXT 明禁统一假设；也撞 AGENTS 红线「不写死为生产默认」。
- **折法另立「付款门禁规则」登记册而不进门禁目录。** 否决（落位取舍，记为越权风险点 4）：门禁目录已按（范围 / 动作 / 边界）三维键伺候门禁编排，规则与认定是同一道门的两种登法，再开一册是同一把键的第二张表。
- **`待确认` / `冲突` 可登记为接受。** 否决：UC-CC-003「未知不能当作……有效」；待确认是「还没答」，不是一个能被接受的答案。
- **CC 保留 `external-funds-fact` 人工补录口，撞键时答 `已存在` / `内容冲突`。** 否决：两个铸造点的冲突不是幂等问题，是归属问题；SA CONTEXT 与 CC CONTEXT 同句把事实归财务系统、采用归 SA。
- **人工补录口保留到 02 / 03 落地再去。** 否决：口径现在就定，避免 07 步一先做四个子命令再拆一个；代码上本就不动（07 步一在 02 / 03 之后开工）。

## 越权风险点

1. **UC-CC-003 范围节「保存每项门禁的依据……和结果」读作不可变引用而不是内嵌快照。** 若 CC owner 认为就绪判断版本必须自带每道门的快照（比如为了跨版本对比不依赖门禁册存活），决定一改成「引用 + 快照」，`RegisterReadiness` 的形要动。
2. **续办流程要不要自动发起重评。** 决定二只说「不后台重算」；凭证责任流程形成新依据后由谁、何时发起下一次评估请求，本记录没定（今天读作人发起）。若 owner 要「新依据落地即自动发起一次评估请求」，那仍是请求驱动，决定二不变，多一条触发编排。
3. **`待确认` / `冲突` 永远未决、不可登记为接受。** 我按 UC-CC-003「未知不能当作有效」读死；若某真实程序把「差额待确认」视为可放行，那是实例半边与 CONTEXT 那句的冲突，要先改 CONTEXT。
4. **规则挂门禁目录既有登记册而不另立一册。** 落位取舍；目录行正文从「认定」扩成「认定或规则」会让 `GateConditionCatalogueEntry` 长一格。若 owner 更愿意规则单独成册，决定三的规则形不变、只换住处。
5. **凭证门禁的形推及其余六道就绪门禁。** 决定一说「后面的门照第一册的形各成一册」；若 owner 认为应是一册「逐门禁判断」带门禁种类列而不是七册，改的是各册的落位，就绪判断按引用绑定这一点不变。
6. **决定四请 SA owner 复核「单一采用口」。** 我按 UC-SA-001「人工核实……只能被采用一次」把人工核实归进 SA 采用口；若 SA owner 认为运维补录该有一条不经采用四格的旁路（比如 SA 采用口不可用时的应急），那是 SA 的一格，不是 CC 的入口。另：在 02 / 03 进 main 前 CC 的 `ReceiveFundsFact` 只有测试路径能触发，这是如实的空档，不拿人工口填。

## Links

- [CC CONTEXT](../domain/customs-compliance/CONTEXT.md)：「监管凭证」「税费付款核对」「放行门禁核对」词条；Rules「申报就绪、授权与提交」「税费、放行与案件闭环」（本记录两句的落地处）；Boundaries 末两句
- [SA CONTEXT](../domain/settlement-accounting/CONTEXT.md)：「实际代垫成立判断」词条；Boundaries 关务与财务两句（本记录一句的落地处）
- [UC-CC-003](../application/customs-compliance/UC-CC-003-ASSESS-DECLARATION-READINESS.md)：范围节、「就绪门禁」表第 4 / 7 行、业务规则「不得自动恢复」「未知不能当作……」两句、步 7 / 10 / 11
- [UC-SA-001](../application/settlement-accounting/UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md)：「同一外部资金事实……只能被采用一次」
- [ADR-0069](./0069-customs-case-chain-ordering-absorbed-by-reread-and-retry.md) 决定二、[ADR-0074](./0074-tf-object-partitions-carry-a-port-segment-apart-from-ve-parcel-partitions.md) 决定五：票 02 / 05 分区主体的口径（A 类，写在票面）
- [ADR-0013](./0013-pricing-owns-versioned-external-reference-series.md)：引用 vs 快照的既有判据（票 06 要裁的 2）
- [ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md) 决定四：票 07 要裁的 1 的判据
- 票 [sa-cc/spec](../../.scratch/sa-cc-funds-and-credential-seams/spec.md)、[04](../../.scratch/sa-cc-funds-and-credential-seams/issues/04-cc-credential-gate-persists-in-readiness-assessment.md)、[06](../../.scratch/sa-cc-funds-and-credential-seams/issues/06-cc-release-gate-reads-duty-payment-verification.md)、[07](../../.scratch/sa-cc-funds-and-credential-seams/issues/07-cc-credential-and-duty-reconciliation-registration-faces.md)（四问出处）；[mech/06](../../.scratch/mechanism-executor-triage/issues/06-sa-four-executors-behind-existing-uc-steps.md)、[mech/07](../../.scratch/mechanism-executor-triage/issues/07-cc-four-executors-behind-existing-uc-steps.md)（缺口出处）
