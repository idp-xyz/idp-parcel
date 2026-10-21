# 05 NR+NO（network-routing、node-operations）：CONTEXT 写出现行规则、UC 与之一致、矛盾裁清

Category: enhancement
Status: resolved——2026-10-21 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-dc05；再此前 in-progress——2026-10-21 认领；原 ready-for-agent——2026-10-21 协调方立（文档自洽修订，见 [README](../README.md)「修订执行」）
Blocked by: 无（01 进 main 后开工）
出处：[audit/nr-no.md](../audit/nr-no.md)（nr-no.md）；[02 的完成记录](./02-code-defects-from-the-audit.md)里「交修订票」一表中交给本票的条目；[01](./01-adr-suspension-chains.md) 的「交后续」一节里交给本票的条目。

## 要什么

1. 按 main 现状逐条复核审计报告里本组的 A、B 类条目（审计钉 `6b0074d0`；其后进 main 的票与文档自洽 01、02 可能已消解一些——消解的写「已由某票消解」并指 SHA）。
2. **A 类**：改文档。范围 docs/domain/network-routing/、docs/domain/node-operations/ 的 CONTEXT.md 与对应 docs/application/，以及本组新立 ADR 与它所停用的 ADR 的 Status 行与 Links。目标照 README「目标」1、2、4、5：CONTEXT 写出今天生效的语言、不变量与生命周期（已被 ADR 部分停用的句子不再以现行口吻留着，改写为现行规则并引用 ADR）；UC 同一场景只有一套结果，结果名与代码一致；不用行号、计数、位置引用。
3. **B 类**：按用户授权（「你作为业务和系统专家自决」）代决，立新 ADR（号段 0249–0253），Status 写法照近期 ADR，写越权风险点，部分停用照 `docs/adr/README.md` 顶部规则（文档自洽 01 改过的「一件事只写一处」）；裁完 CONTEXT 与 UC 随之改。裁定若要求改代码，不在本票改：在 [code/](../code/) 下补一张代码票（编号接 code/ 现有最大号），或在 02 已立的对应代码票里写明裁向并解除其 Blocked by。
4. **交横切**：需要改 GLOSSARY、CONTEXT-MAP、SCENARIOS、`docs/product/`、`docs/agents/`、别组 CONTEXT/UC 的，不在本票改，逐条写进本票「交横切」一节（位置、引文、改成什么、依据），由票 10 统一改。

## 不变式

- 已接受 ADR 正文不改；只动本组文档、本组新 ADR 及其停用对象的 Status 行与 Links。
- 不改代码；不改 GLOSSARY、CONTEXT-MAP 等横切文件。
- `go run ./tools/adr-index -check`（重生成后）退 0。

## 完成判据

- 审计报告本组每一条 A、B 都有去向（已改 / 已由某票消解 / 交横切 / 立代码票）。
- 完成记录里一张对照表：审计条目 → 去向。

## 完成记录（2026-10-21，分支 claude-dc05，基 main `205eb254`）

审计 [audit/nr-no.md](../audit/nr-no.md) 26 条逐条按 main `205eb254` 复核（审计钉 `6b0074d0`；其间进 main 的票没有动 NR、NO 的代码与本组文档，只有文档自洽 01 动了 ADR 的 Status 行）。除 L13 外，现象全部仍在。

### 审计条目 → 去向

| 条目 | 归类 | 去向 | 落在哪 |
|---|---|---|---|
| H1 初始路由整封回滚、并列不留痕 | C | 既有代码票承接；文档加未实现注记 | [demo-chain-gaps/155](../../demo-chain-gaps/issues/155-initial-route-unconfigured-undecided-burns-the-acceptance-partition.md)（02 已扩范围）；UC-NR-001 新「给开发的交接」表、`AT-NR-012` 注记 |
| H2 揽收复核拿揽收地点比计划节点 | B | 代决 [ADR-0249](../../../docs/adr/0249-offsite-pickup-reassessment-checks-hard-constraints-only-and-does-not-establish-a-controllable-node.md)；已改；代码票解除阻塞 | NR CONTEXT Language「当前可控节点」、「路由形成与选择」两条、「路由稳定性与受控改路」前缀一句、生命周期；UC-NR-003 复核点 1、输入契约、复核顺序层次 2、`AT-NR-034` 改写、新立 `AT-NR-084`；ADR-0172 Status 行与 Links 挂前向指针；[code/16](../code/16-pickup-reassessment-compares-the-pickup-place-with-plan-nodes.md) 写明裁向、Blocked by 改为无 |
| H3 复核不取候选成本 | C | 既有代码票承接；文档加注记 | [demo-chain-gaps/115](../../demo-chain-gaps/issues/115-route-reassessment-never-loads-candidate-costs.md)；UC-NR-003「给开发的交接」表、`AT-NR-039`、`AT-NR-053` 注记 |
| M1 UC-NO-006 漏了只因歧义信号的冲突 | A | 已改 | UC-NO-006 输入契约「归入」、步骤 7、`AT-NO-075`，引 NO CONTEXT 重核对象 |
| M2 UC-NR-003 用「未决」写重投推不动的格，缺「触发未受理」 | A | 已改 | UC-NR-003 首段、元数据、启动条件、步骤 3、5、`AT-NR-040`；结果表补「触发未受理」 |
| M3 「当前位置」同词异义 | A | 已改；GLOSSARY 一侧交横切 X1 | NR CONTEXT Language 定义「当前控制位置」，CONTEXT 与 UC-NR-003 一律改用，码值不改 |
| M4 仍适用线从不形成改路建议 | C | 既有代码票承接（复核：不走「限到失效线」那条路） | [code/17](../code/17-still-applicable-plan-never-forms-a-reroute-suggestion.md) 记复核结论；UC-NR-003 `AT-NR-042` 注记 |
| M5 承接决定缺「待补充」 | C | 既有代码票承接；文档加注记 | [code/18](../code/18-node-customs-collaboration-acceptance-lacks-pending-supplement.md)；UC-NO-001 状态行、`AT-NO-003` 注记 |
| M6 集运包含不查环 | C | 既有代码票承接；文档加注记 | [code/19](../code/19-consolidation-containment-is-not-checked-for-cycles.md)；UC-NO-003 阶段 3、`AT-NO-039` 注记 |
| M7 方向性作业受阻：UC 挡、CONTEXT 与代码照记 | B | 代决 [ADR-0251](../../../docs/adr/0251-directional-node-work-is-paused-outside-the-validity-interval-of-the-delivered-basis-and-recorded-moves-are-never-refused.md)（把 no-directional-basis/01「裁定修订」定为记录）；已改；代码不改 | NO CONTEXT「集运、封装与载具」括注、「作业授权与执行」暂停一条；UC-NO-003 启动条件、结果「方向性作业受阻」、阶段 3、`AT-NO-031`、`AT-NO-033`；读面随 no-directional-basis/01、/02 |
| M8 「适用性看不清」的来源都推不动 | B | 代决 [ADR-0250](../../../docs/adr/0250-unclear-plan-applicability-in-reassessment-is-settled-as-not-judged-and-resume-in-the-same-change-stays-in-the-acceptance-chain.md) 决定一；已改；立代码票 | ADR-0235 Status 行与 Links 挂前向指针；NR CONTEXT 复核前提缺口一条；UC-NR-003 结果表（「当前计划适用性未决」并入「触发未判」）、结果表后一段、步骤 6A、`AT-NR-051`、`AT-NR-067`；[code/28](../code/28-unclear-plan-applicability-in-reassessment-is-retried-instead-of-settled.md) |
| L1 重核对象两处口径不一，缺「信号没了、只缺范围」 | A | 已改 | NO CONTEXT 生命周期「待识别实物与身份关联」定义重核对象一处，「送站交接单」改为引用；规则与生命周期补第三支（ADR-0212 决定六） |
| L2 过时的实现现状句 | A | 已改 | UC-NR-003、UC-NO-002（交接一节与结果段那句）、UC-NO-003「给开发的交接」；UC-NO-006 状态行改已实现 |
| L3 以现行口吻写尚无代码的能力 | C（交本票注记） | 已改；无票的一块立代码票 | UC-NR-003、UC-NO-003「给开发的交接」各列一张未实现表，相关 AT 行加注记：路由指令、集运兼容 → no-directional-basis/02；偏离 → demo-chain-gaps/35；复评新计划信封 → demo-chain-gaps/34；实测触发 → pp-pricing-input-seams/04、product-strategy-boundary/08、demo-chain-gaps/44（NR 侧消费票不立，NO 没有实测登记册之前它没有生产者）；人工改路决定 → 新立 [code/29](../code/29-reroute-suggestions-and-ties-have-no-authorized-decision-face.md) |
| L4 AT-NR-033、036 缺冻结形态已声明的前提 | A | 已改 | UC-NR-003 `AT-NR-033`、`AT-NR-036` |
| L5 失效后自动改路仍比改善 | B | 按已接受的 ADR-0173 决定三写进文档，不另立 ADR | ADR-0173 决定三原句「失效后同样按本形态折自动条件」已明写，代码与之一致，相抵只在 CONTEXT 漏写；NR CONTEXT 自动改路一条与 `AT-NR-037` 补上条件与 `IMPROVEMENT_BELOW_THRESHOLD` |
| L6 币种不齐没提比较币种 | A | 已改 | NR CONTEXT「路由形成与选择」排序一条（ADR-0148 决定四） |
| L7 现行规则节里写历史经过 | A | 已改 | NR CONTEXT 删并列出口的历史一段，只留现行一句 |
| L8 CONTEXT-MAP 的 NR 条目只写 `BUY` 评价、「成本维度」 | A | 交横切 X2 | — |
| L9 UC-NR-002 写「读取」客户地址与申报资料 | A | 已改 | UC-NR-002 输入契约「包裹声明」行 |
| L10 GLOSSARY 与 NR/NO CONTEXT 同词重复定义 | A | 交横切 X3 | — |
| L11 「可以形成单元控制」以现行口吻写 | A | 已改；GLOSSARY 一侧交横切 X4 | NO CONTEXT「接收、控制与交接」那条补「规则尚未确立」；UC-NO-003 `AT-NO-038` 注记 |
| L12 AT-NO-062 一行叠两套结果 | A | 已改 | UC-NO-002 拆成 `AT-NO-062`（有范围或渠道件号）与新立 `AT-NO-131`（无范围、只经客户件号），去掉「ADR-0212 之后」 |
| L13 14 份 ADR 的 Status 行把同一次停用写两遍 | A | 已由 docs-consistency/01 消解 | 01 进 main（`205eb254`）后逐份复核：14 份 Status 行标记前段只剩去向，来向只在注记，`adr-index -check` 校这一条 |
| L14 ADR-0235 落`未判`不带续办，与 ADR-0094 决定四相抵 | B | 代决 ADR-0250 决定二 | ADR-0235 注记挂「由 ADR-0250 回答」（越权风险点 1，无条款停用）；ADR-0094 不动，决定二读的就是它注记里写的适用场景 |
| L15 两处 Go 注释与 ADR 相反或过时 | C | 既有代码票承接 | demo-chain-gaps/115（02 已并入） |

去向计数（按主去向）：已改文档 17（A 11、B 5、L3 一条）；交横切 2（L8、L10；另 M3、L11 各有 GLOSSARY 一半交横切）；已由某票消解 1（L13）；既有代码票承接 6（H1、H3、M4、M5、M6、L15）。新立代码票 3（code/28 承 M8，code/29 承 L3 的人工改路，code/33 承评审补出的 NO 作业任务等无模型各块），已立的 code/16 解除阻塞。

### 代决与越权风险点

用户授权「你作为业务和系统专家自决」，三份新 ADR 全是会话代决，Status 写明代谁、待谁复核，越权风险点见各记录文末。

- **ADR-0249（H2，代 NR owner，待 NR owner 复核，第 1 点同待 TF owner）**：揽收触发的复核只按选中候选的硬约束判原计划适用性，揽收地点原样记入复核依据、不与计划节点比；揽收不建立当前可控节点——已执行前缀为空、不判冻结、不试改善切换，失效时只出建议（`NOT_AT_CONTROLLED_NODE`），决定从包裹交进的首个节点生效；揽收照复核、节点交给运输方照旧不复核；揽收复核计入当前控制位置。部分停用 ADR-0172 决定三、决定四各一句与 ADR-0173 决定三两句（均限揽收复核；0173 一处评审轮补上）。越权风险点：揽收地点与声明不符时 NR 不知；揽收时不试改善；失效只出建议、决定面未落地前停在无路由；计入当前控制位置；原先无路由时揽收就形成首个计划。
- **ADR-0250（M8、L14，代 NR owner，待 NR owner 复核）**：「适用性看不清」的来源（关务目录为空、受理基线缺端点国家、关务来源未接）重投都推不动，改答`未判`、原因 `PLAN_APPLICABILITY_UNCLEAR`，未判登记另记缺口与再判条件；真依赖故障在取数侧交错误、照旧重投。ADR-0094 决定四「续办触发同笔落地」只管受理判断链，路由复核每次控制触发都会重判，`未判`可以先于续办落地，续办仍归票 154。部分停用 ADR-0235 决定一一句的那一格，回答其越权风险点 1。越权风险点：关务来源未接也入账、不再有失败码；缺端点国家是永久缺口（补救在 PS 受理侧）；看不见的关务限制下原计划继续走；最后一个节点上的未判再无复核。
- **ADR-0251（M7，代 NO owner，待 NO owner 复核，第 3 点同待 NR owner）**：方向性作业的「暂停」由作业端按随对象下发、带有效区间的方向依据判断——据本地时钟，出区间或缺依据即暂停，依据有效而网络不通时照做，与 ADR-0024 一致（评审轮按协调方裁定改，初稿的「动作之前现问」是 ADR-0024 否决的路）；写口对已经发生的方向性作业（在线口、导入、离线回传一样）照记、同笔留方向依据记录，不答「受阻」；集运兼容判断是下发依据的一格。把票 no-directional-basis/01「裁定修订」定为记录，无条款停用。越权风险点：在线口也不拒；依据下发建成之前没有任何门；区间之内的路由变化看不见、不兼容的成员照记。
- 不另立 ADR 的 B 类：L5 按 ADR-0173 决定三写进 CONTEXT（理由见对照表）。M4 归 C，复核时不取审计给的「限到失效线」那条路，code/17 照立。

### 交横切（交 docs-consistency/10）

- **X1 GLOSSARY「作业位置与当前位置」**（M3）。引文：「当前位置是依据当前有效位置和集运包含关系派生的结果。」改成什么：词条末加一条「避免与 `network-routing` 的『当前控制位置』（路由历史里业务时间最晚的已判控制触发，ADR-0235 决定四称『当前位置』）混用」。依据：NR CONTEXT Language「当前控制位置」。
- **X2 CONTEXT-MAP NR「拥有」与 `parcel-pricing → network-routing` 边**（L8）。引文：「择优消费 `parcel-pricing` 对每个候选形成的 `BUY` 评价作为成本维度」；「计价对每个候选分别形成 `BUY` 评价……网络与路由把这些评价作为成本维度并入择优」。改成什么：「`BUY` 评价或内部价格政策评价（ADR-0148 决定四），按比较币种合成为候选成本事实」，去掉「成本维度」，或注明首发排序形态是成本单维。依据：ADR-0148 决定四第 1、2 条；NR CONTEXT「候选成本事实」。
- **X3 GLOSSARY 与 NR/NO CONTEXT 同词重复定义**（L10）。按审计 L10 逐条：「实物控制」（NO CONTEXT）与「实物控制关系」（GLOSSARY）统一为一个名；GLOSSARY「待识别实物」「正式身份由 `parcel-shipment` 确认」半句改为「正式包裹身份由 `parcel-shipment` 给出，实物与包裹的关联与身份冲突处置归 `node-operations`」（依据 NO CONTEXT「身份冲突处置」、ADR-0210）；GLOSSARY「集运单元」「把多个包裹临时组合」改为「临时组合包裹或其他允许成员（含嵌套的集运单元）」（依据 NO CONTEXT「集运单元实例」、`ports.ConsolidationMemberAt.NestedUnit`）；NR、NO Language 里与 GLOSSARY 各有一份全文定义的词条（物流节点、无当前有效路由、路由指令、待识别实物、可验证交付范围等）只留 GLOSSARY 一处、CONTEXT 引用，合并时以 CONTEXT 现行措辞为准（例如「物流节点」的作业种类、「无当前有效路由」是「明确判断」）。
- **X4 GLOSSARY「实物控制关系」**（L11）。引文：「封签连续且相符的封闭集运单元被有效接收后，成员可以派生为“随单元受控”，但不形成逐件实收」。改成什么：句末加「单元控制规则尚未确立，确立之前集运单元的交接不推进成员控制（NO CONTEXT「接收、控制与交接」、UC-NO-005）」。
- **X5 CONTEXT-MAP `node-operations / transport-fulfillment → network-routing` 边**（H2）。引文：「运输履约提供场外揽收、权威运输交接和实际移动事实」。改成什么：补半句「场外揽收触发复核但不建立当前可控节点，交进声明的接收节点的权威运输交接才建立（ADR-0249、ADR-0215）」。依据：ADR-0249 决定二、三。
- **X6 `.scratch/at-coverage-inventory.md`**：评审轮按协调方安排改由本票做，已移行，见「评审记录」。

### 检查

- `go run ./tools/adr-index` 重生成后 `-check` 退 0。重生成的 `docs/adr/README.md` 不随本票提交：照 [parallel-sessions「生成物不占号」](../../../docs/agents/parallel-sessions.md#生成物不占号但只在干净检出上生成)，带新 ADR、改 Status 行的票只改 ADR 文件，索引由推送方在 tip 上重生成。
- `go run ./tools/at-coverage-counts -check`：初轮退 0；评审轮移行之后只报计数过期（退 1，ErrStale），计数由推送方重算。
- 本票改动的全部 `.md` 里的相对链接与锚点用脚本逐个解析，全部能解析。
- `git diff main --stat` 只含（第二轮评审后按现状重写）：本组两份 CONTEXT；NR、NO 的 UC（UC-NR-001、002、003，UC-NO-001、002、003、005、006）；新 ADR 0249、0250、0251；被停的 ADR-0172、ADR-0173、ADR-0235 的 Status 行与 Links；`.scratch/at-coverage-inventory.md`（评审轮受托移行）；`.scratch/no-directional-basis/issues/01-…`（评审轮受托加一条更正）；`.scratch/docs-consistency/` 下本票与 code/16、17、28、29、33。
- 编号：新代码票原取 code/26、27，评审轮按协调方分配改为 code/28、29（03 用 26–27、04 用 30–32），另立 code/33；分支内引用已同步。

## 评审记录（2026-10-21，隔离评审第 1 轮 → 作者修订）

阻断三条、建议三条、顺手两条、协调方安排两条，全部照办：

| 条 | 评审意见 | 处置 |
|---|---|---|
| 1 阻断 | ADR-0249 收窄了 ADR-0173 决定三「原计划仍可执行时，先看冻结。」与「未越过且改善超过阈值，才自动切换。」两句，却写「无条款停用」，与 ADR-0172 决定四的处理不一 | 改为部分停用：ADR-0249 Status 前段、「对已接受记录的作用」、Consequences、Links 列出，适用场景限场外揽收复核；ADR-0173 Status 注记加来向与逐字原句、Links 加后继 |
| 2 阻断 | ADR-0251 决定一「在线时现问；离线时用缓存……灰掉」落在 ADR-0024 否决的路上，全文没引 0024 | 照协调方裁定取 ADR-0024：方向依据随对象下发、带有效区间，作业端据本地时钟判，出区间或缺依据即暂停，有效而断网照做；改写 ADR-0251 标题、注记、Context、决定一、三、四、候选与反方、作用、越权风险点、Links（补 0024）；NO CONTEXT「作业授权与执行」那条、UC-NO-003 启动条件、结果「方向性作业受阻」、阶段 3、`AT-NO-031`、`AT-NO-033`、交接表，UC-NR-003「节点安全作业与方向性作业」与 `AT-NR-038` 同口径 |
| 3 阻断 | UC-NR-003 删了「当前计划适用性未决」，代码 `ReassessUndecided` 仍在，首段、元数据、步骤 5、`AT-NR-067` 仍说判断未决 | 结果表补「复核未决」一行：依赖读不动或提交不确定，答 `UNDECIDED`，列原因码与续办引用，消费侧回滚重投；禁止落复核、入账未判、默认保留或失效；注明今天另有 `PLAN_REVIEW_INCONCLUSIVE`，随 code/28 移入触发未判 |
| 4 建议 | 「触发未受理」行与 `AT-NR-040` 的「不回滚重投」与两路适配器相反 | 限到「复核编排答 `TRIGGER_NOT_ACCEPTED`、不落复核、不入账」；注明两路触发由适配器从权威记录译出，答到这一格即适配器响亮失败、信封回滚 |
| 5 建议 | UC-NR-002「声明测量不由本上下文读取」与 `adapters/parcelpricing/declared_parcel_facts.go`、ADR-0148 决定四第 7 条相反 | 改为声明测量只在组装候选成本的计价输入时由 NR 侧 PP 消费方适配器取客户声明快照；货物与申报经关务适用性判断引用 |
| 6 建议 | UC-NO-003 交接表末行「无票；随 PAR-NET-04..06 排」——这几块是机制 | 立 [code/33](../code/33-node-operations-has-no-work-task-batch-scan-location-condition-or-presence-check-model.md)，表里改指它并写明是机制、租户取值另登 |
| 7 顺手 | X6 补 `AT-NO-031` | 并入第 10 条一起做：`AT-NO-031` 由③移入④ |
| 8 顺手 | ADR-0249 越权风险点 3 与 code/29「绕法」说交接复核会在节点上自动改路，不准 | 改准：原计划失效后是无当前有效路由，交进首个节点时走「形成首个计划」那条线，唯一合格候选即成，不比改善阈值；未决或并列时仍无路由 |
| 9 安排 | 03、04、05 撞号 | code/26、27 改名 code/28、29，另立 code/33；票面、ADR-0249、0250、UC-NR-001、UC-NR-003、code/16 等分支内引用全部同步 |
| 10 安排 | AT 盘点移行由本票做 | `.scratch/at-coverage-inventory.md`：`AT-NR-034`、`AT-NR-051` 由②、`AT-NO-031` 由③移入④；新加 `AT-NR-084`（④）、`AT-NO-131`（①，`AT-NO-062` 行末句那三条 Covers 移来）；`AT-NR-033`、`036`、`037`、`040` 改场景文字；末尾加追记一条；总览计数不改 |

另照 [10](./10-cross-cutting-docs.md) 新增的跨组约定第 5 条（出自本票评审），NR、NO 两份 CONTEXT 头部各加一句不含票号的固定指针「规则与实现的差距见各 UC『给开发的交接』」；两份 CONTEXT 的规则条目里没有挂票号。收口前已 `git rebase main`（main `f928c5ba`）。

修订后的检查：scratchpad 副本上 `go run ./tools/adr-index` 后 `-check` 退 0（README 不随票提交）；`go run ./tools/at-coverage-counts -check` 只报计数过期；改动的 `.md` 相对链接全部解析；ADR-0249、0250 引的被停原句在 ADR-0172、0173、0235 正文里逐字核过。

## 评审记录（第 2 轮，2026-10-21）

第 1 轮三条阻断评审确认已消。残留阻断一条、建议三条、顺手七条，全部照办：

| 条 | 评审意见 | 处置 |
|---|---|---|
| B1 阻断 | UC-NO-003 交接表指向 no-directional-basis/01「读面半边」，那张票「裁定修订」第 1 条仍写「在线时现问」 | 经协调方授权，在该票「裁定修订」之下加 2026-10-21 更正一条：「现问」与「服务端答此刻能不能做」的读法作废，照 ADR-0024、ADR-0251 改为方向依据随对象下发、带有效区间、作业端据本地时钟判，「读面」一律改指下发半边；UC-NO-003 交接表与 ADR-0251 的指称同改 |
| 1 | UC-NO-002 `AT-NO-022` 与 `AT-NO-031` 同一场景 | `AT-NO-022` 措辞对齐并指向 `AT-NO-031`；AT 盘点里同样由③移入④，追记同步 |
| 2 | UC-NO-003 交接表「移入只拒……」漏了 `UnitNotFound`、`MemberElsewhereContained` | 补上，`AT-NO-035` 一并点名；另写明不拒成员就是单元本身 |
| 3 | 完成记录「检查」里的 diff 清单过期 | 按现状重写 |
| 4 | 三个测试的 Covers 注释仍写「`AT-NO-062` 末句」 | 本轮不改代码，登在下面，并并入 code/33 的修法：`internal/nodeoperations/application/delivery_scope_judgment_test.go::TestAnAmbiguityOnlyConflictWithoutAScopeIsReleasedIntoAnUnverifiedPendingUnit`、`internal/nodeoperations/adapters/http/reception_scope_basis_test.go::TestAManualRecheckThatReleasesAnAmbiguityIntoAnUnverifiedUnitAnswersCreated`、`cmd/parcel-dispatch/node_identity_ambiguity_release_test.go::TestTheWiredDispatchReleasesAnUnscopedAmbiguityIntoAnUnverifiedPendingUnit` 三处 Covers 改指 `AT-NO-131` |
| 5 | code/29 缺「证据」小节 | 补上，「现象」一节改为一句概述 |
| 6 | code/28「三个来处」 | 去掉「三个」 |
| 7 | ADR-0249 Consequences 漏了 ADR-0172 | 补上 |
| 8 | ADR-0251 文件名带初稿含义 | `git mv` 为 `0251-directional-node-work-is-paused-outside-the-validity-interval-of-the-delivered-basis-and-recorded-moves-are-never-refused.md`，全仓引用同步，旧名 grep 为 0 |
| 9 | UC-NR-003 首段与元数据「其他结束」仍写「判断未决」 | 改为「复核未决、替代候选评估未决」，与结果表一致 |
| 10 | code/33「`domain/` 只有……」穷举不全 | 改为「有的是……等」的不穷举说法，补上关务协作承接、送站预报与预报时窗、身份冲突处置 |

检查：scratchpad 副本上 `go run ./tools/adr-index` 后 `-check` 退 0；改过的 `.md` 相对链接全部解析；全仓 grep 旧的 0251 文件名为 0。

## 进 main 记录（2026-10-21，协调方）

- 分支 `claude-dc05` tip `d99096267` 在 main `f928c5ba` 上快进；两轮隔离评审的阻断全部消掉（第 2 轮残留的 no-directional-basis/01「现问」口径已加更正）。
- 协调方随后在 tip 上单独提交：重生成 `docs/adr/README.md`（`go run ./tools/adr-index`）、重算 AT 盘点总览计数（`go run ./tools/at-coverage-counts`）；两者 `-check` 退 0。机制清点重生成无变化。
- 本票只动文档，`go build ./...` 干净；未跑全量真库测试（无代码改动）。
- 去向：代码票 code/28、29、33；code/16 已解阻（ADR-0249）。
