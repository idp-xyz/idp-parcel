# 10 横切文档：GLOSSARY、CONTEXT-MAP、SCENARIOS、产品文档、协作文档

Category: enhancement
Status: in-progress——10a 已进 main（见「10a 进 main 记录」）；10b 评审已修，待随 10a rebase 做词汇对齐后进 main。此前 in-progress——10a 完工待评审与进 main，分支 claude-dc10a；10b 待开。此前 ready-for-agent——2026-10-21 协调方立（文档自洽修订最后一张，见 [README](../README.md)「修订执行」）
Blocked by: 03、04、05、06、07、08、09 进 main（它们的「交横切」一节是本票的输入）
出处：[audit/cross.md](../audit/cross.md) 里 GLOSSARY（GL）、CONTEXT-MAP（CM）、SCENARIOS（SC）、产品基线诸件（PB）、试点三件（PL）、docs/agents 与 AGENTS.md（AG）各条，以及其中 B 类；03–09 各票的「交横切」一节；[01](./01-adr-suspension-chains.md)「交后续」交给本票的条目。

## 要什么

1. 按 main 现状逐条复核（03–09 进 main 后的现状）。
2. A 类改文档：GLOSSARY 单一权威（跨上下文术语只定义一次，各 CONTEXT 引用）；CONTEXT-MAP 的上下游与各 CONTEXT「拥有/不拥有」一致；SCENARIOS 不与现行规则相反；`docs/product/` 与 ADR 现行决定一致；`docs/agents/`、AGENTS.md 与现行做法一致（含 AG·H2：进 main 的步骤不能只写在单会话不读的地方）；20 条断链修好。
3. B 类代决立新 ADR（号段 0274–0278），裁完相关文档随之改；要改代码的在 [code/](../code/) 补票。
4. 03–09 交来的每一条都落实或写明不落的理由。

5. **跨组约定：CONTEXT 写成现行、代码尚未到的规则怎么让读者知道**（出自 05 的隔离评审）：不在 CONTEXT 的单条规则上挂票号（票号随票关闭而过时，也违背「只写一处」）。每份 `docs/domain/*/CONTEXT.md` 头部只放一句不含票号、各组同形的固定指针，例如「规则与实现的差距见各 UC『给开发的交接』」；各 UC 的「给开发的交接」未实现表与 AT 行的「未实现」注记由 03–09 各自维护。

## 不变式

- 已接受 ADR 正文不改；不改代码。
- AGENTS.md 是人与 Agent 共用的入口，改它照 `/writing-for-agents` 的写法，不删现行规则、不重述 docs/ 的权威内容。

## 分工（2026-10-21，协调方）

审计 cross.md 151 条加 03–09 交来的横切条目，按文件不相交拆两个作者、两条分支，先后或并行进 main：

| 半边 | 文件范围 | 新 ADR 号段 | 代码票号段 | 分支 |
|---|---|---|---|---|
| 10a 领域语言 | `docs/domain/GLOSSARY.md`、`docs/domain/CONTEXT-MAP.md`、`docs/domain/SCENARIOS.md`、各 `docs/domain/*/CONTEXT.md` 头部固定指针（及交来的 CONTEXT 级改动）、`docs/application/README.md`、新立 `docs/application/collection-remittance/UC-CR-001`（09 的 X5：迁 CR CONTEXT 交接表）、别组 UC 的交来改动（如 07 的 X11 指 PS 文档认`适用冲突`） | 0274–0276 | code/54–56 | claude-dc10a |
| 10b 产品与协作 | `docs/product/`（含参数登记册、PILOT-*、基线诸件）、`docs/design/`、`docs/agents/`、`AGENTS.md`、`docs/README.md`、已接受 ADR 的 Status 注记与 Links（交来的引用更正，如 08 的 X6：ADR-0233 越权风险点 1；07 的 X9：ADR-0146「第三条判据」） | 0277–0278 | code/57–59 | claude-dc10b |

两半边都要的条目（例如一处 GLOSSARY 改动牵动产品文档一句）由 10a 改领域语言、10b 照新词改产品文档；10b 开工时以 10a 已进 main 的词为准，若 10a 未进 main，就在返回里列出待对齐的句子。

## 10a 完成记录（2026-10-21，分支 claude-dc10a，rebase 于 main `37f9d7747`）

作者子代理按票面「分工」做 10a 领域语言半边。审计 [audit/cross.md](../audit/cross.md) 的 GL、CM、SC 各条与其待核，及 03–09「交横切」与 01「交后续」交给 10 的条目，按 main 现状逐条复核后落实；复核读的是各组进 main 之后的 CONTEXT、UC 与 ADR，不是审计钉的 `6b0074d0`。另用脚本比了审计之后各 CONTEXT 改过、又与 GLOSSARY 重名的词条，读出各组没交来的分歧：CR「差异事项」、PP「计价输入快照」、TF「容量预占」随本票改了 GLOSSARY；PS「客户原始资料版本」改的那一格 GLOSSARY 没写，不相抵。

### 审计条目 → 去向

| 条目 | 归类 | 去向 | 落在哪 |
|---|---|---|---|
| GL·G-01 出发门禁读数 | A | 已改 | GLOSSARY 词条收成一句定义、判法指 TF CONTEXT，所有者一条写明单元那一格由运输履约按成员折出（同 08 X3） |
| GL·G-02 价格规则版本 | A | 已改 + 立代码票 | GLOSSARY 词条只指 PC 商业价格政策；SA CONTEXT 同名词条改指针、「采购价格规则版本」改为评价命中的可执行价卡版本；代码命名与注释立 [code/55](../code/55-supplier-expected-cost-calls-the-price-card-version-a-purchase-rule-version.md) |
| GL·G-03 面单服务终局边界 | A | 已改 | GLOSSARY 词条与「面单渠道服务」一条改为接单规则包版本的终局规则定义（ADR-0058）；PS CONTEXT 同名词条改指针、Rules 那一句同改；SCENARIOS 场景二预期结果同改 |
| GL·G-04 客户费用退款 | A | 已改 | GLOSSARY 词条依据改为计价纠错、商业让利或客户索赔责任，渠道退款结果不是依据（同 09 X3） |
| GL·G-05 对账周期归谁 | B | 已由 09 消解（ADR-0269，`97f782d24`）+ 已改 | GLOSSARY「结算政策」去对账周期、新立「对账安排」、「截单快照」改按归属日；PC CONTEXT「结算政策」同改（同 09 X7、X10） |
| GL·G-06 当前客户资料版本采用判断取值 | A | 已改 | GLOSSARY 取值改为 PS CONTEXT 那一组，各格含义指 PS（同 06 X1） |
| GL·G-07 限制适用性判断取值 | A | 已改 | GLOSSARY 改为与 CC Language 同名的四格，技术未形成另成一类结果（同 03 X5）；CC Language 原文已一致，不改 |
| GL·G-08 计价参考序列实例 | A | 已改 | GLOSSARY 补定额附加金额（ADR-0110），口径一句按 ADR-0260 决定三（同 07 X3）；新立「计价参考目录」词条（CM·M-3 转来） |
| GL·G-09 同义重复 | A | 部分已改，其余不落 | 已改：NR、NO 的重复词条（05 X3）、VE「客户归属」「客户全程追踪视图」（04 第 2 条）、PS「歧义信号」「范围相容」「面单服务终局边界」、SA「价格规则版本」改为指针，定义并进 GLOSSARY；GLOSSARY 开头补一句：CONTEXT 新增或改写已定义的词只写指针与细化，仍各留一份的两处说法不一即是缺陷。不落：其余上下文的同义重复逐词合并要改写 03–09 刚进 main 的各份 Language，审计自评低、同义不改业务结果；审计之后改过的重名词条已逐条比过（见本节开头），分歧都已改齐 |
| GL·G-10 GLOSSARY 文档关系与 PP 开头 | A | 已改 | GLOSSARY「文档关系」补 PP；PP CONTEXT 开头补 GLOSSARY 那一句 |
| GL·G-11 总单的运输履约范围 | A | 已改 | GLOSSARY 改为实际履约段（ADR-0113） |
| GL·G-12 揽收交接结果、有效收寄 | A | 已改 | GLOSSARY、PS CONTEXT、NR CONTEXT、SCENARIOS 场景一统一用「场外揽收结果」；GLOSSARY 新立「有效收寄」词条；NO「节点收寄」一句改指有效网络收寄结果 |
| GL·G-13 实物控制、集运单元实例词名 | A | 已改 | GLOSSARY 词头改为「实物控制」并注「实物控制关系」是同一件事；「集运单元」注 NO 内称集运单元实例、成员含嵌套单元；「可复用载具」避免用词改写（同 05 X3） |
| GL·G-14 签约服务商 | A | 已改 | GLOSSARY 新立词条，写明它属外包运输一侧的角色集、面单交易一侧不含它；「承运商代理商」一条改用这个词 |
| GL·G-15 不带前缀的提交版本 | A | 已改 | PC、SA Boundaries 写全称「委托提交版本」，PC 关务一句写「申报提交版本」；CONTEXT-MAP 两处同改 |
| GL·G-16 修订意图没有出处 | A | 已改 | PS CONTEXT 新立「修订意图」词条；PC「资料修订允许声明」改指它 |
| GL·G-17 依据计算目的形成计价重量 | A | 已改 | GLOSSARY「当前有效实测」与 NO CONTEXT 两处改为按评价采用的重量策略 |
| GL·G-18 代收应收、实收、清分 | A | 已改 | PS、TF（两处）、SA（两处）Boundaries 与 SCENARIOS 场景十四、CONTEXT-MAP 改用 CR 的对象与资金位置词表 |
| GL·G-19 应追偿金额相对方 | A | 已改 | SA CONTEXT 词条相对方范围对齐 GLOSSARY |
| GL·待核 T-1 保价条件与金额规则册 | 待核 | 不落 | 要 SA、PC owner 说明设计意图才判得出 A 还是 B；审计未计入条数，本票不猜 |
| GL·待核 T-2 待识别实物的正式身份 | 待核 | 已改 | 同 05 X3，照 ADR-0210 的读法写 |
| CM·M-1 CC↔NR 关务适用性 | A | 已改 | 关系那半句改指 CC CONTEXT「关务适用性判断」；CC 拥有清单补这一项 |
| CM·M-2 唯一采用口与 CR 真实到账 | B | 代决 [ADR-0274](../../../docs/adr/0274-collected-principal-bank-arrival-enters-through-the-settlement-accounting-funds-adoption.md) + 立代码票 | CONTEXT-MAP 补 SA→CR 边与关系句；CR CONTEXT Rules 与 Boundaries、SA CONTEXT Boundaries、GLOSSARY「代收货款」「代收事实」随改；[code/54](../code/54-collection-bank-arrival-does-not-reference-an-adopted-funds-fact.md) |
| CM·M-3 PP 拥有清单的序列与目录 | A | 已改 | PP 拥有、不拥有两栏与「外部数值来源」一条（同 07 X5） |
| CM·M-4 NR 择优只消费 BUY、唯一选择 | A | 已改 | NR 拥有一栏与 `parcel-pricing → network-routing` 一条：外包 BUY、自营 INTERNAL，结果可以并列冲突（同 05 X2） |
| CM·M-5 自然键只守幂等 | A | 已改 | SA→PP 一条改为（自然键、代次）唯一（同 09 X2） |
| CM·L-1 箭头口径 | A | 已改 | 关系约束开头写明一律从提供方指向消费方；PP 读 TF、PS 的三条改写成提供方 → 消费方 |
| CM·L-2 图上缺边 | A | 已改 | 补 NR→PS、PP→NR、SA→CC、SA→PP、PP→PS、SA→CR |
| CM·L-3 PS 渠道择优消费 BUY | A | 已改 | 新立 `parcel-pricing → parcel-shipment`（渠道择优成本）一条 |
| CM·L-4 参数已登记只写一封 | A | 已改 | SA→PS 补受理前估价采用，PC 的运营登记完成一封同一续办口 |
| CM·L-5 派送要求两样引用 | A | 已改 | PC→TF 补交付条件引用与承运主体身份，PS→TF 补收件地点引用；图上标签同改 |
| CM·L-6 计费重量 | A | 已改 | 改为实重尺寸（随 09 X2 那一句改写） |
| CM·L-7 客户查询授权 | A | 已改 | 移出 PC 的提供清单，写由共享身份能力给出（同 04 第 1 条） |
| CM·L-8 只经客户包裹件号的「唯一命中」 | A | 已改 | 改为恰一个候选（不范围相容） |
| CM·L-9 早于当前位置不判 | A | 已改 | 交接触发复核那句补 ADR-0235 |
| CM·L-10 价卡收尾 | A | 已改 | 参数已登记的计价触发补价卡收尾（ADR-0234） |
| CM·L-11 收寄复核的触发 | A | 已改 | PS→NR 补有效网络收寄采用的信封（UC-NR-003）；NO→NR 注明实测与控制节点读口等 NO 实测登记册 |
| CM·L-12 pilotgovernance 的位置 | A | 已改 | 产品边界补一段：试点治理能力不是限界上下文，答生产归属与准入控制（同 06 X2） |
| CM·待核 1 不可计价与续办 | 待核 | 已改 | 同 09 X4 |
| CM·待核 2 PC 定价方案绑定是否被读 | 待核 | 已改 | 随 07 X5 改写 PC→PP 一条：方向授权与 INTERNAL 政策的方案绑定，计价不引用 PC 解析（ADR-0260） |
| CM·待核 3 NO→CC 执行事实交接 | 待核 | 不落 | 引用册由谁、经哪个口填没有核到，可能是代码缺消费者；不在本票读码范围，留协调方定要不要立代码票 |
| CM·待核 4 测量边 | 待核 | 已改 | 随 CM·L-11 在 NO→NR 一条注明 |
| SC·S-01 容量预占写成承运接受的结果 | A | 已改 | 场景十六里承运只接受部分申请量那一步（同 08 X7） |
| SC·S-02 阶梯方式 | A | 已改 | 场景十五里保底量与阶梯返利那一步按 ADR-0165 内置形态，数值另册登记 |
| SC·S-03 计价基准时点由合同约定 | A | 已改 | 场景十二前提与步骤（随 S-08 改写） |
| SC·S-04 合同委派可直接决定 | B | 代决 [ADR-0275](../../../docs/adr/0275-controlled-closure-and-reopening-are-decided-only-by-operator-roles-and-a-channel-stop-is-an-external-restriction.md) | PS CONTEXT 受控关闭与重开一条去掉例外；场景二里停止与恢复请求、重开请求两步与预期结果；GLOSSARY「面单继续尝试决定」补一句 |
| SC·S-05 渠道方叫停写成请求 | B | 代决 ADR-0275 | 同上；ADR-0183、ADR-0187 注记与 Links 各挂由 ADR-0275 回答的来向；代码注释与 Covers 跟改立 code/56 |
| SC·S-06 结算依据采购规则形成成本 | A | 已改 | 场景十六里形成运输收费发生项与供应商预期成本那一步 |
| SC·S-07 不予受理与不成立 | A | 已改 | 场景十八里客户提交两项索赔那一步改为通过资格审核、进入责任审核后的不成立结论 |
| SC·S-08 面单申请时预估并冻结 | B | 已由 09 消解（SA CONTEXT 冻结一条，`d52df6bf0`）+ 已改 | 09 已写明面单交易前的冻结今天不形成；场景十二改为网络服务委托的受理前估价冻结 → 包裹计费时点预估客户费用 → 复秤后重新评价形成新的预估版本。面单渠道服务包裹的计费时点仍是 ADR-0190 越权风险点 8，本票不裁 |
| SC·S-09 待识别实物 U 被交出 | A | 已改 | 场景十里后续交出那一步 |
| SC·S-10 结算按合同判断计费重量 | A | 已改 | 场景四里重新计价那一步与预期结果、场景十一里测量那一步 |
| SC·S-11 审核应付、客户运营应收直接形成 | A | 已改 | 场景一里全程追踪与费用那一步 |
| SC·S-12 外部资金事实的进入口 | A | 已改 | 场景十四里关务税费付款核对那一步；场景十二收款、场景十四与十七真实到账同补 |
| SC·S-13 改路的自动与建议分支 | A | 已改 | 场景三里单次分配失败那一步 |
| SC·S-14 身份冲突处置 | A | 已改 | 场景九里 U 与正式包裹建立关联那一步与预期结果（ADR-0210） |
| SC·S-15 「十个限界上下文」 | A | 已改 | 开篇改指 CONTEXT-MAP |
| SC·S-16 文档关系漏 PC、CR | A | 已改 | 补两行 |
| SC·待核 Q-01 场景十六软超配 | 待核 | 不落 | 现行 TF CONTEXT 仍写软容量按阈值与授权超配，场景与之相容；软池怎么表达等 psb/09，本票不裁 |
| SC·待核 Q-02 同级或更高重开权限 | 待核 | 不落 | PS CONTEXT 仍是「同级或更高」，pc-gaps/13 裁决 ① 的首发判法没有核到细节；不改场景，留 PS、PC owner |
| SC·待核 Q-03 ADR-0090 引的场景原句已不存在 | 待核 | 归 10b | 是 ADR-0090 Status 注记的事 |
| SC·待核 Q-04 TF CONTEXT 规则与边界一节的张力 | 待核 | 不落 | 是 TF CONTEXT 自身的张力，场景与规则一致；08 已进 main、没交来，留 TF owner |
| SC·待核 Q-05 场景十四「代收应收、已收」 | 待核 | 已改 | 随 G-18 与 ADR-0274 改写代收与清分形成分户账结果那一步 |
| 场景十七与 ADR-0271 | — | 核过，一致 | 报告 100、到账 96、认可抵扣 3、短款 1 照 ADR-0271 的短款公式成立；只补真实到账经结算采用一句 |
| X·X-08 断链 | A | 10a 范围内无断链；其余归 10b | 10a 改过的各文件链接与锚点全部可解析；0045、0054 的 Links 与 `docs/review/081701.md` 不在 10a 文件范围 |

cross 的 B2、B3 两簇 B 类（B2-04、B2-10、B3·1、B3·2、B3·3、B3·6、B3·8）不在本票「出处」所列的 GL、CM、SC、PB、PL、AG 之内，03–09 也没有接；B2-02、B2-03、B2-05 已由 07 裁（ADR-0260、0262，`45b6bc873`），B2-06 已由 demo-chain-gaps/88 裁（ADR-0231，`d238b6023`）。余下七条交协调方派。

### 交来条目 → 去向

| 条目 | 去向 | 落在哪 |
|---|---|---|
| 01 交后续（AGENTS.md 改文档、workflow.md 进 main 步骤、开发主线与产品故事引 ADR-0016、Bento 闸门、`docs/review/081701.md`、pp-pricing 设计文档引 ADR-0015） | 归 10b | — |
| 03 X1 内部合规限制、监管限制 | 已改 | GLOSSARY 新立「监管限制」，「内部合规限制」补互不改名、互不解除 |
| 03 X2 限制 | 已改 | GLOSSARY「限制」补两种关务限制 |
| 03 X3 动作准入判断 | 已改 | GLOSSARY |
| 03 X4 技术回执 | 已改 | GLOSSARY |
| 03 X5 GLOSSARY 四条 | 已改 | 限制适用性判断、税费付款核对、申报就绪判断、监管核定税费 |
| 03 X6 ADR-0213 Status 注记 | 归 10b | — |
| 03 X7 关务基线 | 归 10b | — |
| 04 第 1 条 CONTEXT-MAP 客户查询授权 | 已改 | 同 CM·L-7 |
| 04 第 2 条 客户归属、客户全程追踪视图 | 已改 | GLOSSARY 新立两条；VE CONTEXT 两条改为一句定义加指针 |
| 04 第 3 条 追偿事项同义两名 | 已改 | GLOSSARY 词头改「追偿事项」并注别名；「应追偿金额」避免用词改写 |
| 04 第 4 条 状态 | 已改 | GLOSSARY「状态」 |
| 04 第 5 条 事实类型词出处 | 已改 | GLOSSARY 与 TF CONTEXT「外部承运轨迹事实」句末指 VE CONTEXT「事实类型词」 |
| 04 第 6 条 UC-TF-005 `AT-TF-157`–`161` | 已改 | 这几行改为引用 UC-VE-002、UC-VE-004 的对应行；AT 盘点①的对应行不动（测试 Covers 仍点它们，改点归 code/32） |
| 04 第 7、8 条 ADR-0077、0078 前向指针 | 归 10b | — |
| 05 X1 作业位置与当前位置 | 已改 | GLOSSARY |
| 05 X2 NR 择优边 | 已改 | 同 CM·M-4 |
| 05 X3 GLOSSARY 与 NR、NO 同词重复 | 已改 | 见 GL·G-09、G-13、T-2 |
| 05 X4 实物控制的单元控制 | 已改 | GLOSSARY「实物控制」 |
| 05 X5 场外揽收不建立可控节点 | 已改 | CONTEXT-MAP NO/TF→NR 一条 |
| 05 X6 AT 盘点 | 已由 05 消解 | 05 评审轮已做 |
| 06 X1 当前客户资料版本采用判断 | 已改 | 同 GL·G-06 |
| 06 X2 生产归属、准入控制 | 已改 | GLOSSARY 新立两条；PS CONTEXT 那一条加链接；CONTEXT-MAP 产品边界交代 pilotgovernance（不进上下文清单） |
| 06 X3 歧义信号、范围相容 | 已改 | 进 GLOSSARY，正文取 PS 现文；PS 两条改指针，NO「身份冲突来由」链到 GLOSSARY |
| 06 X4 渠道账号使用授权「旧授权版本」 | 已改 | GLOSSARY 改「旧授权登记」并注 ADR-0093 |
| 06 X5 PS→NR 地址端口的适用场景 | 已改 | CONTEXT-MAP |
| 06 X6 申报测量 → 声明测量 | 已改 | CONTEXT-MAP 两条 |
| 06 X7 TF「授权处置」 | 已改 | TF CONTEXT 改「交授权角色裁决」 |
| 06 X8 AGENTS.md 当前默认切片 | 归 10b | — |
| 06 X9 README UC-PS-005 一行 | 已改 | `docs/application/README.md` |
| 06 X10、X11、X12 设计简报、开发主线、PILOT-SCOPE 与 design | 归 10b | — |
| 07 X1 可执行价卡版本 | 已改 | GLOSSARY，注明「价卡」是定价方案版本与价表版本的合称 |
| 07 X2 价格规则版本 | 已改 | 同 GL·G-02 |
| 07 X3 计价参考序列 | 已改 | 同 GL·G-08 |
| 07 X4 计算目的 | 已改 | GLOSSARY |
| 07 X5 CONTEXT-MAP 两条边 | 已改 | PC→PP 与外部数值来源 |
| 07 X6、X7、X8、X9 参数登记册、计价转录本、计价设计文档、ADR-0146 Status | 归 10b | — |
| 07 X10 B2-02、03、05 已裁 | 无须改 | 本票不再另裁 |
| 07 X11 PS 文档认适用冲突 | 已改 | UC-PS-002「未决」、UC-PS-005「撤回未决」两行并列授权适用冲突（ADR-0261），各自「给开发的交接」补一行指 code/24；UC-PS-001 的拒绝、复核与处置没有列这类结果码，照 X11「在列……的地方」不加 |
| 07 X12 NR CONTEXT 换算 | 已改 | NR CONTEXT 成本单维那一条 |
| 07 X13 README 索引 | 已改 | UC-PP-001、UC-PC-001、002 状态照 UC，补 UC-PC-003 |
| 07 X14 UC-SA-002 `PAR-SET-11` | 已由 09 消解（`4c9b2c4cc`） | — |
| 08 X1 订舱、承运接受 | 已改 | GLOSSARY |
| 08 X2 容量池 | 已改 | GLOSSARY；「容量预占」一并按 ADR-0264、0203 改 |
| 08 X3 出发门禁读数 | 已改 | 同 GL·G-01 |
| 08 X4 业务流程导览三处 | 归 10b | — |
| 08 X5 权威运输交接结果、监管运输处置承接决定 | 已改 | GLOSSARY |
| 08 X6 ADR-0233 越权风险点 1 引文 | 归 10b | — |
| 08 X7 SCENARIOS 350kg | 已改 | 同 SC·S-01 |
| 09 X1 计价基准时点 | 已改 | GLOSSARY |
| 09 X2 SA→PP | 已改 | 同 CM·M-5、L-6 |
| 09 X3 客户费用退款 | 已改 | 同 GL·G-04 |
| 09 X4 不可计价不重试 | 已改 | GLOSSARY 与 CONTEXT-MAP |
| 09 X5 UC-CR-001 | 已改 | 新立 [UC-CR-001](../../../docs/application/collection-remittance/UC-CR-001-REGISTER-COLLECTION-AND-POST-THE-SUBLEDGER.md)：结果码与退出码逐码取 `application.Outcome.String()` 与 CLI `answer`，词表取各 `String()`，验收 `AT-CR-001`–`023`；CR CONTEXT 交接表迁入其「给开发的交接」并补 code/54 一行，CR CONTEXT 头部改回固定句、删文末交接节；README 补一行；AT 盘点按桶补行 |
| 09 X6 README UC-SA-003、005 | 已改 | `docs/application/README.md` |
| 09 X7 截单快照 | 已改 | 同 GL·G-05 |
| 09 X8 资金冻结 | 已改 | GLOSSARY |
| 09 X9 当前有效客户预估费用 | 已改 | GLOSSARY |
| 09 X10 结算政策与对账安排 | 已改 | 同 GL·G-05 |
| 票面第 5 条 CONTEXT 头部固定指针 | 已改 | CC 补；CR 改回固定句；各份 CONTEXT 头部逐字同为「规则与实现的差距见各 UC「给开发的交接」。」 |

### 代决摘要与越权风险点

用户授权「需要修改文档，确保自洽，正确」「你作为业务和系统专家自决」，两份都是会话代决，Status 写明待谁复核；号段 0276 未用。

- **[ADR-0274](../../../docs/adr/0274-collected-principal-bank-arrival-enters-through-the-settlement-accounting-funds-adoption.md)（CM·M-2，代 CR、SA owner）**：运营企业真实到账只经 settlement-accounting 的唯一采用口进入本产品；代收与清分登记这一层时证据引用必须指向一条已采用的外部资金事实，登记时按引用读回核在册与币种，同一采用事实下各条到账之和不超额。无条款停用（ADR-0137 决定四原样扩到 CR，ADR-0082、0271 的依据门之上加门）。代码 code/54。越权风险点：入口归属；结算一侧怎样处置被引用的那部分没裁，在那之前停在未分配；超额答冲突、不自动截断；登记时读一次、不跟随结算更正；COD 首发不在范围、code/54 落地前 CLI 照旧不核。
- **[ADR-0275](../../../docs/adr/0275-controlled-closure-and-reopening-are-decided-only-by-operator-roles-and-a-channel-stop-is-an-external-restriction.md)（SC·S-04、S-05，代 PS、PC owner）**：受控关闭与重开只由运营侧授权角色形成，合同委派不参与，不是「首发不开」而是规则上不存在；货主的要求作为请求证据落在决定上；渠道服务方或代理商（不是委托客户账户时）叫停记为外部限制、请求方缺席，恢复是来源责任方的解除；不另立请求册。无条款停用，回答 ADR-0183 越权风险点 1 与 Consequences「另行裁定」那一问、ADR-0187 Alternatives 候选丁；行为不改，注释、Covers 与解除门缺口随 code/56。越权风险点：例外不存在（要开自助路须另起记录）；渠道方叫停靠运营角色转成关闭；渠道方的恢复靠来源解除；未授权请求不留痕。

作者另作的判断（不另立 ADR，评审可挑）：

1. **GL·G-09 只做交来的那几族**，其余同义重复不合并，理由见上表；GLOSSARY 开头那句只约束此后的新增与改写，不宣称现状已无重复。
2. **CONTEXT 级改动不限于交来的条目**：GL、SC 各条与两份新 ADR 要改的 CONTEXT 句子（PS、PC、SA、TF、NR、NO、VE、CR、CC、PP 各有几处）在本票改了，因为审计把这些条目交给票 10、各组已进 main。改的都是术语对齐与指针，没有改任何一条规则的结论；ADR-0275 改 PS CONTEXT 那一条是裁定的直接后果。
3. **ADR-0183、ADR-0187 的 Status 注记与 Links 由本票挂**（ADR-0183 一半是评审轮补的）：分工说 10a 不改已接受 ADR 的 Status 行、本半边新 ADR 停用的除外。ADR-0275 是回答不是停用，但 README「一件事只写一处」要求被回答方注记挂来向，照 09 给 ADR-0190 挂 ADR-0270 回答的先例由本票做；ADR-0183、ADR-0187 去掉 Status 行与 Links 后与 main 逐字节相同。
4. **UC-TF-005 迁走的五行保留为引用行**，没删：AT 盘点①的五行与测试 Covers 仍点它们，删行会让盘点指向不存在的行；改点新号归 code/32。
5. **AT 盘点**：`AT-CR` 各行里编排已有、测试未点名的入②，未实现的入④，追记一条；总览计数照规矩进 main 时重算。

### 立的代码票

- [code/54](../code/54-collection-bank-arrival-does-not-reference-an-adopted-funds-fact.md) CR 真实到账的证据引用不指向结算已采用的外部资金事实（R3，COD 进入试点之前必修；要裁的一格归 SA owner）。
- [code/55](../code/55-supplier-expected-cost-calls-the-price-card-version-a-purchase-rule-version.md) SA 把评价命中的价卡版本叫采购规则版本（R3，只改命名与注释）。
- [code/56](../code/56-closure-and-reopening-comments-still-cite-the-deleted-delegation-exception.md) 受控关闭与重开的注释与 Covers 仍引已删的合同委派例外句，渠道方叫停形成的关闭今天可以直接重开（R3；解除门要裁的一格归 PS owner）。评审轮立。

### 归 10b、待它照新词对齐的

- 交来条目：01 交后续各条；03 X6、X7；04 第 7、8 条；06 X8、X10、X11、X12；07 X6–X9；08 X4、X6；SC·待核 Q-03；X-08 的 ADR-0045、0054 Links 与 `docs/review/081701.md`。
- 照 10a 新词要对齐的产品文档句子（10b 开工时以本分支进 main 后的词为准）：`docs/product/PRODUCT-BASELINE.md`「面单服务终局边界由适用服务产品和客户合同」一句（改为接单规则包版本的终局规则定义）；`docs/product/PILOT-SCOPE.md` COD 排除那一句里的「代收应收」（改用 CR 的对象与资金位置词表）。
- `docs/product/PRODUCT-BASELINE.md` 里「货主、渠道服务方和代理商只能提出请求，除非客户合同或明确授权把其纳入可直接形成决定的授权角色范围」一句与 ADR-0275 正面相反。照 ADR-0275 决定一、三改：货主只能提出请求，关闭与重开的正式决定只由运营侧授权角色形成、合同委派不参与；渠道服务方或代理商（不是委托客户账户时）叫停记为外部限制、请求方缺席，其恢复是来源责任方的有效解除，不是重开请求。

### 检查

- 在 `git archive claude-dc10a` 解出的副本上跑 `go run ./tools/adr-index` 生成后 `-check` 退 0；生成的索引多出 ADR-0274、0275 的行，ADR-0187 一行多出由 ADR-0275 回答的注记，标签派生为〔代决·待 CR、SA owner 复核〕〔代决·待 PS、PC owner 复核〕。
- `go run ./tools/at-coverage-counts -check` 只报计数过期（新增 `AT-CR` 前缀一行与②④两桶的数），未重算。
- 改过的 .md 相对链接与锚点全部可解析（dc09tools 的 `links.py`，基准 main..claude-dc10a）。
- 书名号引文用 `quotes.py` 比过分支与 main 两份语料：本票新写的转述已去掉书名号或改引原句；余下几处是同文件内的自指或嵌套书名号，逐字存在。
- 已接受 ADR 正文逐字节不变：本分支只动 ADR-0183、ADR-0187 两份已接受记录的 Status 行与 Links，去掉它们后与 main 相同；ADR-0274、0275 是新文件。
- 各份 `docs/domain/*/CONTEXT.md` 头部各有一处逐字的「规则与实现的差距见各 UC「给开发的交接」。」。
- 没改代码；没碰 `docs/product/`、`docs/design/`、`docs/agents/`、`AGENTS.md`、`docs/README.md` 与 `docs/adr/README.md`。

### 评审记录（隔离评审第 1 轮，2026-10-21）与作者回应

机械检查全过；阻断 4 条，建议 8 条，全部照改。

| # | 评审意见 | 回应 |
|---|---|---|
| B1 | ADR-0274 Context 说两份已接受的记录给了相反的入口并引 ADR-0082，但 ADR-0082 没有相反的句子，与 Status「无条款停用」自相矛盾 | 改为 ADR-0137 对 CR CONTEXT 与 GLOSSARY；写明 ADR-0082 决定四只是必要条件、没有定入口 |
| B2 | ADR-0275 也回答了 ADR-0183 越权风险点 1 与 Consequences「另行裁定」，没挂来向 | ADR-0183 Status 注记与 Links 各补「由 ADR-0275 回答」；ADR-0275 Status 前段、「对已接受记录的作用」、Links 同步；截引的 Consequences 补全后半句；ADR-0183 正文逐字节不变 |
| B3 | 代码三处注释与 Covers 仍引已删的 PS CONTEXT 例外句 | 立 code/56（R3，只改注释与 Covers）；ADR-0275 Consequences 那句改为行为不改、注释随 code/56 |
| B4 | 产品基线有与 ADR-0275 正面相反的同句 | 「归 10b」一节补那一句与改法；ADR-0275 Consequences 点名产品基线 |
| 建议 1 | 决定三的解除今天不落门 | ADR-0275 Consequences 写明，缺口记进 code/56 |
| 建议 2 | UC-PS-002、UC-PS-005 新加交接行的现状写错 | 改为 PC 今天取首个命中、答已授权（`AT-PC-042`），补上适用冲突后 PS 适配器也还不译它（code/24） |
| 建议 3 | UC-CR-001 主流程结果不全、结果表未注出处、`AT-CR-020` 缺币种一支 | 各步补未受理、未决等格；结果表注明哪几格只出自哪只编排；`AT-CR-020` 补币种不同答受理门拒 |
| 建议 4 | PS CONTEXT「修订意图」与 UC-PS-002 输入表不一致 | CONTEXT 照 UC 输入表写规则（含来源更正与撤销关系），实现缺口进 UC-PS-002 交接；PC 允许声明那句的「意图三格」计数改为各格 |
| 建议 5 | 场景十二用词 | 复秤那一步改为形成新的预估版本；计费时点那一步用原词「计价基准时点」 |
| 建议 6 | GLOSSARY「签约服务商」新句没有出处 | 改为指明 TF CONTEXT 的角色集含它、PC CONTEXT 的不含 |
| 建议 7 | ADR-0274「扩到」是停用动词 | 改为「本就涵盖代收与清分」 |
| 建议 8 | 场景二用位置引用指下一步；CONTEXT-MAP 新挂票号；票面步号引用 | 场景二点名 PS CONTEXT 那条硬限制解除规则；去掉新挂的 `pp-pricing-input-seams/04`；票面场景步号改为点名那一步的内容 |

## 10a 进 main 记录（2026-10-21，协调方）

- 分支 `claude-dc10a` tip `7d30868cb` 在 main `37f9d7747` 上快进；隔离评审 4 条阻断全部消掉，协调方抽核 B1（ADR-0274 Context 不再把相反入口归到 ADR-0082）与 B2（ADR-0183 挂「由 ADR-0275 回答」）。
- 协调方随后在 tip 上单独提交：重生成 `docs/adr/README.md`、重算 AT 盘点总览计数（UC-CR-001 新增 `AT-CR-001`–`023`）；两者 `-check` 退 0。机制清点重生成无变化；`go build ./...` 干净。只动文档，免全量真库测试。
- 去向：代码票 code/54、55、56；归 10b 的条目见上。审计 cross.md B2、B3 两簇里 B2-04、B2-10、B3·1、B3·2、B3·3、B3·6、B3·8 七条不在本票出处范围、03–09 也没接，由协调方另立票。

## 10b 完成记录（2026-10-21，分支 claude-dc10b，基 main `4e7874ba0`）

作者：Claude Code 作者子代理，worktree `idp-parcel-dc10b`。按 main 现状逐条复核审计 cross.md 的产品基线诸件（PB）、试点三件（PL）、docs/agents 与 AGENTS.md（AG）各条，以及 03–09「交横切」与 01「交后续」落在 10b 范围的条目：审计条目在 `4e7874ba0` 上全部仍成立，没有一条已被别票消解（审计钉之后进 main 的票没有动这几份产品与协作文档，只动过首个消费者切片简报里与 code/39 相关的两句、演示动线两条补记）。

### 审计条目 → 去向

| 条目 | 归类 | 去向 | 落点 |
|---|---|---|---|
| PB·H1 导览说 CR 没有后端、没有 CONTEXT | A | 已改 | 导览 9.11 与 13.1：机制已落地、首发不启用分开说 |
| PB·H2 首发只有网络服务一种形态 | A | 已改 | PRODUCT-STORY 一句话定位；导览 2.1 |
| PB·H3 网络定义登记册写入方仍列为缺口 | A | 已改 | 开发主线新补记（一）；控制金额源一并收口 |
| PB·M1 Bento 闸门「已解除」与「仍阻断」 | B | 代决 ADR-0277 | 简报闸门表、开发主线三句、AGENTS.md 当前默认切片、PN-02 合成商业交接 |
| PB·M2 两道哨兵按旧条款写 | A | 已改 | 开发主线横切缺口投递一条（ADR-0064、ADR-0148 决定六） |
| PB·M3 HTTP 接入面仍写成 PAR-INT-01 实例半边 | A | 已改 | 开发主线两处；PRODUCT-STORY 生产路径一句 |
| PB·M4 SELL/BUY 补记仍写不进派发 | A | 已改 | 开发主线新补记（二） |
| PB·M5 PN-03 行写代码未实现 | A | 已改 | 覆盖缺口表 PN-03 行改指机制半边现状 |
| PB·M6 商业解析排在生产归属之前 | A | 已改 | 端到端主链阶段 1 |
| PB·M7 仍待提供整体归实例半边 | A | 已改 | 覆盖缺口表后那句与 PN-02、PN-05 两行 |
| PB·M8 切片表漏 UC | A | 已改 | 两张表补 UC-PC-003、UC-NO-004/005/006、UC-CC-013、UC-PP-001 |
| PB·M9 关务基线按十二个用例评估 | A | 已改 | 关务基线状态行、评估结论、两张表补 UC-CC-013 与它跟 UC-CC-009 的分工 |
| PB·M10 关务基线余量全归真实实例 | A | 已改 | 评估结论、规则取值、PAR-COM-13 并发规则归产品策略两处 |
| PB·M11 导览业务入口表当前行为与代码不符 | A | 已改 | 导览 8.6 改列接入面族、钉取证 |
| PB·M12 导览 PN 定级表 | A | 已改 | 导览 12.2 改引开发主线 |
| PB·M13 导览用例数与漏 UC | A | 已改 | 导览 9.0 改引应用用例索引并补行 |
| PB·M14 导览第 9 章进度句过时 | A | 已改 | 删去各模块「当前成熟度」块，操作面只写职责，进度只留第 13 章并钉 SHA |
| PB·M15 导览 UC-PC-003 授权范围 | A | 已改 | 导览 9.0、9.1 |
| PB·M16 产品基线「不同时铺开多个渠道」 | A | 已改 | 产品基线首发原则补作用面 |
| PB·L1 重定级结论未补 ADR-0100 落地、0139..0142 接受 | A | 已改 | 开发主线新补记（三） |
| PB·L2 全仓九份 contract test | A | 已改 | 就地注明取证 SHA |
| PB·L3 五条出口共七处（= 06 X11） | A | 已改 | 按符号指，不写数 |
| PB·L4 「紧接其上那句」 | A | 已改 | 改为点名 |
| PB·L5 关务基线 31 项 | A | 已改 | 改引切片 0 交接 |
| PB·L6 引已取代的 ADR-0018 | A | 已改 | 注明已取代、落位结论沿用（指 ADR-0022 Links） |
| PB·L7 ADR-0016 当完整决定引用 | A | 已改 | 开发主线、PRODUCT-STORY 各补部分取代；前向指针已由 01 消解（`bc8283e33`） |
| PB·L8 产品故事漏产品策略 | A | 已改 | PRODUCT-STORY 演进路线与凭什么赢 |
| PB·L9 导览把角色定义划给租户 | A | 已改 | 导览第 3 章 |
| PB·L10 导览说前端无 CI 门禁 | A | 已改 | 导览 13.2 |
| PB·L11 收费模型引文非原句 | A | 已改 | 照产品基线原句引 |
| PB·L12 择优归属不准 | A | 已改 | 开发主线首发范围约束、收费模型 |
| PB·L13 数据态要等隔离读准入 | A | 已改 | PRODUCT-STORY |
| PL·F1 PAR-INT-08 漏运营决定 | A | 已改 | 已确认约束与最低证据 |
| PL·F2 PAR-INT-11、PAR-GOV-03 只列两组词 | A | 已改 | 按族点名四组，词表以 `internal/accessidentity` 为准 |
| PL·F3 矩阵按三条判据 | A | 已改 | 改引开发主线（ADR-0146） |
| PL·F4 渠道或国家专属设计挡在租户参数后 | A | 已改 | 范围「进入详细设计的条件」；登记册门槛一节改为「生产启用前」并加约束说明 |
| PL·F5 41 例源价卡漏金额保真度 | A | 已改 | 矩阵计价证据规则改引源完整性门禁 |
| PL·F6 证据存储写死自建 MinIO | B | 代决 ADR-0278 | 登记册两处与 PAR-GOV-11、矩阵、docs/README、PN-08 交接与几份工作单 |
| PL·F7 PAR-GOV-10 标待核验 | A | 已改 | 改待提供并指拆分注 |
| PL·F8 缺 webhook 订阅行 | A | 已改 | 新增 PAR-INT-14 |
| PL·F9 PAR-COM-18 影响面漏一册 | A | 已改 | 改引 ADR-0126 所列各册 |
| PL·F10 路由复核触发漏交接进节点 | A | 已改 | 范围、PAR-NET-14、OPS-02 都改引 UC-NR-003「触发」一行 |
| PL·F11 限量生产门槛漏几行 | A | 已改 | PN-03 补 PAR-NET-17/18、治理补 PAR-INT-11 同时生效、接入渠道族各行 |
| PL·F12 范围几句等真实确认涵盖了判断方法 | A | 已改 | 范围三句只点名租户取值 |
| PL·F13 PAR-VIS-02「保守默认依据」 | A | 已改 | 改「经批准登记的保守参数及其依据」 |
| PL·F14 面单渠道服务必验条件三种 | A | 已改 | CPS-07 加租户条件与 N/A；范围那句加租户条件 |
| PL·F15 PAR-SET-03 混进单卡取证状态 | A | 已改 | 模板行只留证据要求；单卡答复移回 PP-S03-W01，范围那句改指它 |
| AG·H1 domain.md 说 CR 没有 CONTEXT | A | 已改 | domain.md 树、映射表、缺席一段；workflow.md「九个」 |
| AG·H2 进 main 步骤只写在单会话不读处 | A | 已改 | workflow.md 新增「进 main 的步骤」；AGENTS.md 与 parallel-sessions 头部写明单会话必读部分 |
| AG·M1 机制清点重生成两套口径 | B | 代决（docs/agents）+ 立代码票 | parallel-sessions「生成物不占号」改为三件同一条规矩；CI 门挂法交 code/57 |
| AG·M2 Spec 轴对照 UC 还是票面 | B | 代决（docs/agents） | 只在 workflow.md「进 main 的步骤」定义，workflow 上文与 parallel-sessions 改为链接 |
| AG·M3 gh 无 workflow scope | A | 已改 | workflow.md 当前宿主 |
| AG·M4 四条红线少一条 | A | 已改 | workflow.md 改指红线表并补敏感实例外置 |
| AG·M5 AGENTS.md 改文档没写部分停用（兼 01 交后续） | A | 已改 | AGENTS.md 改文档一句入口；workflow.md 那句改链接 |
| AG·M6 当前宿主只写 Cursor | B | 代决（docs/agents） | workflow.md 新增「当前宿主之二：WSL 里的 Claude Code 会话」；parallel-sessions 标明依赖 idp-mcp 的几节只对 Cursor 多通道 |
| AG·M7 「必要时再 /code-review」与评审门不一 | B | 代决（AGENTS.md） | AGENTS.md 本仓主路径补一句 |
| AG·L1 branch-state 跑法 | A | 已改 | parallel-sessions 改 pwsh 形 |
| AG·L2 位置引用方向写反 | A | 已改 | 改为节名加原句 |
| AG·L3 上方路由表与别处计数 | A | 已改 | workflow.md 两处、parallel-sessions「四片」 |
| AG·L4 `-p 1` 残留 | A | 已改 | parallel-sessions 步骤 4、workflow.md 前端切片理由 |
| AG·L5 autocrlf 前后两说 | A | 已改 | workflow.md 两处理由改为 `.gitattributes` |
| AG·L6 `blocked` 状态 | B | 代决（docs/agents）+ 立代码票 | 认 `Blocked by:`，不补角色；脚本交 code/58 |
| AG·L7 link-skills.sh 不在本仓 | A | 已改 | workflow.md |
| X·X-08 `docs/review/081701.md` 本机绝对路径（01 交后续） | A | 不落 | `docs/review/` 不在 10a、10b 任一半边的文件范围，交协调方定 |
| 附表 C1 断链 | — | 10b 范围内无断链 | 审计所报各条落在 ADR 与 `docs/review/`：ADR-0045、0054 的 Links 已由 01 改正（0054 Decision 正文那条记了引用更正），ADR-0011 是已取代记录、不动；`docs/review/` 见上一行 |

交来的条目：

| 条目 | 去向 | 落点 |
|---|---|---|
| 01 交后续：AGENTS.md 部分停用写法 | 已改 | 同 AG·M5 |
| 01 交后续：进 main 步骤旁写停用只写一处 | 已改 | workflow.md「进 main 的步骤」作者一步 |
| 01 交后续：开发主线、产品故事引 ADR-0016 | 已改 | 同 PB·L7 |
| 01 交后续：ADR-0017 指针交出的闸门现状（PB·M1） | 代决 ADR-0277 | ADR-0017 注记末句改为以 ADR-0277 决定三为准 |
| 01 交后续：pp 最终设计的 `Amended by` | 已改 | pp-pricing-rule-model-final-design 两处；同样写法的 pp-unratable-outcome-decision-brief 一处一并改 |
| 03 X6 ADR-0213 R12 读法注 | 已改 | ADR-0213 注记 |
| 03 X7 关务基线内部合规限制消费方 | 已改 | 补 UC-CC-013，实现现状不写进基线 |
| 04 第 7 条 ADR-0077 ← ADR-0100 | 已改 | 0077 注记与 Links、0100 前段与 Links |
| 04 第 8 条 ADR-0078 ← ADR-0139 理由收窄 | 已改 | 0078 注记与 Links、0139 Links |
| 06 X8 AGENTS.md 当前默认切片 | 已改 | 随 ADR-0277 |
| 06 X10 首个消费者切片简报 | 已改 | 布局块、SourceRequestKey、首个提交状态、`!found` 映射 |
| 06 X11 开发主线七处 `!found` | 已改 | 同 PB·L3 |
| 06 X12 「人工接受」 | 已改 | PILOT-SCOPE、矩阵 CPS-11、pn-02 两份设计文档 |
| 07 X6 PAR-SET-11 与参考复核职责规则 | 已改 | 已确认约束、最低证据、当前登记 |
| 07 X7 转录本 SOURCE_RATE_CARD 一行 | 已改 | 与源完整性门禁一致 |
| 07 X8 计价设计两份 | 已改 | 体积系数不分方向、汇率口径补 ADR-0260 决定三读法 |
| 07 X9 ADR-0146「第三条判据」 | 已改 | ADR-0146 注记「引用更正」，前段不动 |
| 08 X4 导览控制待确认/冲突三处 | 已改 | 照 ADR-0263 决定二 |
| 08 X6 ADR-0233 引文 | 已改 | ADR-0233 注记「引用更正」；ADR-0231 Context 同一引文一并更正 |
| 03 X1–X5、04 第 1–6 条、05 X1–X5、06 X1–X7 与 X9、07 X1–X5 与 X11–X14、08 X1–X3 与 X5、X7、09 X1–X10 | 归 10a | GLOSSARY、CONTEXT-MAP、SCENARIOS、各 CONTEXT、`docs/application/` 与别组 UC，本半边不改 |

统计：审计 PB、PL、AG 共 63 条——已改 56、代决 7（立 ADR 2：PB·M1 → ADR-0277、PL·F6 → ADR-0278；在 docs/agents 与 AGENTS.md 里代决 5：AG·M1、M2、M6、M7、L6，其中 2 条另立代码票）；外加 X·X-08 一条不落。交来条目 10b 半边全部落实，没有不落的。新立代码票 2 张：code/57、code/58（code/59 未用）。新 ADR 2 份：0277、0278（协调方给的备用号段 0267–0268 未用）。

待核条目（审计未计数）：

| 条目 | 去向 |
|---|---|
| PB·待核 1 关务基线「七类业务门禁」对不上 UC-CC-003 | 不落：未逐类核到，不作为发现；CC 组改 UC-CC-003 时一并看 |
| PB·待核 2 收费模型「API 用量的事实不存在」与 ADR-0140 的 `QUOTA_EXCEEDED` | 不落：是否在收费模型里提额度计量，归产品 owner |
| PB·待核 3 产品基线开篇与「纯面单转售」客户层的张力 | 不落：定位句放不放宽归产品 owner |
| PB·待核 4 导览「本机 `pnpm build` 不能稳定复现」 | 不落：本机环境事实，未复现，导览 13.2 照留 |
| PB·待核 5 开发主线「冻着 32 条」 | 不落：带日期的补记，历史数 |
| PL·待核 1 `PAR-SET-03`「加宽不使历史回放失效」与只认当前规范化版本 | 不落：要先核库里有没有旧版本评价，归 PC+PP 组 |
| PL·待核 2 范围「客户看金额走 SA」与 ADR-0152 试算答询价 | 不落：句意归 PS、PP owner 定 |
| PL·待核 3 价表族「激活」挡在租户证据后 | 不落：属 PP CONTEXT 口径，归 PC+PP 组 |
| AG·待核 Q1 Bento 闸门两说 | 即 PB·M1，ADR-0277 |
| AG·待核 Q2 Claude Code 会话调不调得到 idp-skills | 已改：写进 workflow.md「当前宿主之二」技能一条（按名调不到时读 idp-skills 仓的 `SKILL.md` 照做） |
| AG·待核 Q3 前端切片直推 main 与推送方快进同时在途 | 不落：没撞到实例；撞到时照 parallel-sessions「别人分支上的活怎么进 main」推送方先核中间那几笔再重放 |
| SC·待核 Q-03（10a 交来）ADR-0090 引的 SCENARIOS 原句已不存在 | 已改：ADR-0090 注记「引用更正」，写出现行对应句 |

### 代决摘要与越权风险点

- **ADR-0277（PB·M1，代架构 owner，待架构 owner 复核，第 2 点同待 Bento owner）**：Bento 持久化闸门分 Parcel 半边（锁定候选 `v0.1.0-rc.2` 的空缓存下载与 `PBC-01` 至 `PBC-09` 对同一 checksum 全部通过，自 `ccbf79e` 成立）与登记半边（绑 `B-06`、宣称闸门通过，等双消费者证明）；Parcel 半边成立之后持久化进生产装配不再限于取证用途，约束与常驻门禁照旧；文档统一写「Parcel 半边已过、持久化进生产装配获准；闸门本身未通过、`B-06` 未登记」。部分停用 ADR-0017 两句、ADR-0026 一句。越权风险点：RC 进生产装配；登记半边可能长期不发生；租户生产与 `B-06` 的关系未裁；08-17（`64bae1288`）至 08-25（`ccbf79e`）之间进装配的实现事后追认；越过 2026-08-25 受托裁定刻意保留的「本裁定不改结论格」；候选变化时已铺开的实现不回退。ADR-0043 决定四、ADR-0025 与 ADR-0031 的 Consequences 里「闸门挡持久化」的句子是写下时的现状，按决定三读，三份各挂读法注、无条款停用；ADR-0022、ADR-0090 的「本记录不解除任何闸门」说的是它们自己的范围，照旧成立，判为不挂。
- **ADR-0278（PL·F6，代产品 owner，待产品 owner 复核）**：证据存储的形态（S3 兼容对象存储、按对象版本引用、`idp-parcel/pilot` 命名空间约定）归产品策略，用哪一个平台、谁建谁运维是租户取值，登在该租户的 `PAR-GOV-11`。部分停用 ADR-0008 Decision 一句的一半与 Consequences 两句。越权风险点：开发方若托管证据存储；命名空间约定跨租户同名；连接器尚无执行器（psb/13 第 2 项）。ADR-0008 Alternatives 否决托管 S3 的理由另挂「理由限定」。
- **协作规程的五条 B 类在 docs/agents 与 AGENTS.md 里代决，不立 ADR**：AGENTS.md 把 ADR 留给「难逆转技术或产品取舍」，进 main 的步骤、评审轴、宿主说明与票面状态写法都随时可改，权威在 docs/agents（parallel-sessions 里 2026-09-08 等用户裁定也是这样记的）。各处就地标了「会话代决，待用户复核」。
  - AG·M1：三件生成物同一条规矩，票不重生成、推送方在 tip 上各单独一笔。风险：要走 PR 形态之前 CI 的机制清点门得先按 code/57 改，否则 PR 上按设计红。
  - AG·M2：Spec 轴对照票面判据与裁决加所引 UC，相抵以 UC 为准并记阻断。风险：评审要多读一份 UC，票面裁决与 UC 不一的票会被判阻断，作者要先改 UC 或票面。
  - AG·M6：两种宿主并存，Claude Code 一节照实际作业写；idp-mcp 依赖的点名、度量五数只对 Cursor 多通道成立。风险：Cursor 多通道若已退场，那几节该整节退役，本票没有裁「退场」，留给用户。
  - AG·M7：代码票与改现行规则的文档票落 main 前都要一份非作者评审，单会话也一样。风险：把 09-08 只说代码票的门扩到改规则的文档票，是照本轮各票的做法扩的。
  - AG·L6：阻塞只认 `Blocked by:`，不补 `blocked` 状态角色。风险：脚本修好之前复核队列第二节在 WSL 里一直是空的（code/58）。
- **进 main 的步骤写成现行**：照本轮各票「进 main 记录」与派单写下，只动 `.md` 的票可免全量真库测试与灌种子、进 main 记录写「未跑」。风险：这一免是照 09 等票的实际做法定的，用户若要文档票也全跑，改的是那两步的一句。

### 归 10a 与待随 10a 对齐

- 归 10a（交来的，本半边不改）：见上表最后一行。另有两处落在别组 UC、建议 10a 一并看：UC-PS-001「开发拆分建议」末段说持久化「按 ADR-0026 可写，登记为发布基线仍在 Bento 闸门之后」，可补引 ADR-0277；UC-PS-001 业务决策简报「保留人工接受门槛」一句与 06 X12 同类，是否改由 PS 口径定。
- 待随 10a 对齐（产品与设计文档里照 10a 将改的词改的句子，以 10a 进 main 后的词为准）：
  - 开发主线 ADR-0171 那条补记「申报测量与地址要素走小包托运」：06 X6 让 CONTEXT-MAP 改用「声明测量」（PS CONTEXT 已立此词）；补记是日期记录，随 10a 一起改。
  - PRODUCT-BASELINE「实物控制关系」：05 X3 要统一「实物控制」与「实物控制关系」的词名。
  - 「供应商或保险追偿」在 PILOT-SCOPE、PRODUCT-BASELINE、参数登记册与导览：04 第 3 条把 GLOSSARY 词头改为「追偿事项」、旧名列同义。
  - 「可执行价卡版本」在 PRODUCT-BASELINE、PN-07 交接与 PN-02 合成商业交接：07 X1 要对齐 PP CONTEXT 的「定价方案版本」「价表版本」或定义「价卡」。
  - PRODUCT-BASELINE「关闭和重开决定必须由……」一句里「货主、渠道服务方和代理商只能提出请求，除非客户合同或明确授权把其纳入可直接形成决定的授权角色范围」：与 10a 新立的 ADR-0275 相反（合同委派不参与，渠道方叫停记为外部限制），照 ADR-0275 决定一、三改写（协调方补）。
  - 协调方评审另列入同一轮的：PRODUCT-BASELINE「面单服务终局边界由适用服务产品和客户合同」、PILOT-SCOPE「代收应收」、UC-PS-001 两处。
  - 导览与开发主线写到 `UC-CR-001` 的地方：10a 立 UC-CR-001 后，导览 9.0 用例索引与开发主线切片表是否列它（COD 不在首发 PN 切片里）由合并时定。

### 检查

- 新 ADR 两份与改了 `Status` 行或 Links 的已接受 ADR 十一份：正文（去掉 `Status` 行与 Links 节）对 `4e7874ba0` 逐字节相同（scratchpad `dc10b/tools/adrbody.py`）。
- `git archive claude-dc10b` 解到 scratchpad 后 `go run ./tools/adr-index` 生成、`-check` 退 0；索引里 0277 标〔代决·待架构 owner 复核〕、0278 标〔代决·待产品 owner 复核〕。`go run ./tools/at-coverage-counts -check` 退 0。本分支不手改 `docs/adr/README.md`、不重生成机制清点。
- 改过的 `.md` 与 `docs/product/`、`docs/design/`、`docs/agents/`、`AGENTS.md`、`docs/README.md` 全部文件的相对链接与锚点全部解析（`dc09tools/links.py`）。
- 改过的行里的书名号引文对 `4e7874ba0` 与本分支全仓逐字核过（`dc09tools/quotes.py`），新写的引文都中；报不中的都是改行里原样留着的旧引文、建议改写稿或代码注释跨行。
- 与 main 的重叠：本分支开工后 main 前进到 `37f9d7747`（code/52），两边改的文件不相交。
- 未跑 Go 测试：本半边不改代码。

### 评审记录（隔离评审第 1 轮，2026-10-21，阻断 2 条）与作者回应

评审结论：ADR-0277、0278 的事实与停用写法站得住；阻断两条都是改 docs/agents 时新造的同事两说。AG 五条 B 类不立 ADR 的判断协调方认可。

| 评审项 | 处置 |
|---|---|
| 阻断 1 「合入前独立评审」与「进 main 的步骤」口径不一 | parallel-sessions 那一节把门扩到改现行规则的文档票并标会话代决；「不评什么」的 .md 簿记限定为不改现行规则的（票面、台账、生成物） |
| 阻断 2 「全量只跑一次」推送方一条没有文档票的例外 | 补「只动 `.md` 的票除外」并链到 workflow「进 main 的步骤」 |
| 建议 3 机制清点没有 `-check`、无变化也成笔 | workflow 第 4 步照 ci.yml 写重生成命令与 `git status --porcelain` 判法；三件改为有变化才单独成笔 |
| 建议 4 ADR-0277 时间线与风险 | Context 补 `64bae1288`（08-17）先于 `ccbf79e`（08-25）进装配；越权风险点补事后追认与越过 MCP-1「本裁定不改结论格」两条；开发主线依赖一句改为随 ADR-0026 取证放行进来 |
| 建议 5 别份 ADR 里「闸门挡持久化」的句子 | ADR-0277「对已接受记录的作用」点名 0043、0025、0031、0022、0090，不停用；0043、0025、0031 注记挂读法注、两头 Links；0022、0090 判为不挂（理由见代决摘要） |
| 建议 6 ADR-0139 前段缺去向 | 前段照 ADR-0100 的写法补一条「另：」去向，指它对 ADR-0078 Decision 一那句理由的收窄 |
| 建议 7 序数禁令的例外 | AGENTS.md 补「已接受 ADR 的「决定几」编号不在此列」 |
| 顺手 8–12 | parallel-sessions Spec 一格只留链接；PRODUCT-STORY 改为多数读面经真渠道、其余仍走隔离读；docs/README 关务基线改 `UC-CC-001..013`；PAR-INT-14 当前登记去掉实现现状；PN-02 合成商业交接改为按 ADR-0017 与 ADR-0277 |
| 顺手 13–15 | 票面号段措辞；补待核条目去向（含 SC·待核 Q-03 → ADR-0090 引用更正）；ADR-0008 注记补「理由限定」 |
| 词汇对齐 | 本轮不动，等 10a 进 main 后统一处理（清单见上节「待随 10a 对齐」） |

重跑：scratchpad 副本上 adr-index 生成后 `-check` 退 0；`at-coverage-counts -check` 退 0；改过的 `.md` 链接与锚点全部解析；改过的已接受 ADR 正文对 `4e7874ba0` 逐字节相同；新写引文逐字核过。


### 词汇对齐（2026-10-21，rebase 于 main `c0e4d2f12`，10a 已进 main）

照 10a 在 main 上的 GLOSSARY、CONTEXT 词名与 ADR-0274、0275 改，提交 `47256e399`。

| 待对齐的句子 | 处置 |
|---|---|
| 开发主线 ADR-0171 补记「申报测量与地址要素走小包托运」 | 改「声明测量」，注原词 |
| PRODUCT-BASELINE「实物控制关系」 | 改「实物控制」（GLOSSARY 词头；「实物控制关系」是它认的别称） |
| 「供应商或保险追偿」：PILOT-SCOPE 三处、PRODUCT-BASELINE、参数登记册 PAR-SET-03、导览两处 | 改「追偿事项」；登记册那句改为「追偿事项的应追偿金额」 |
| 「可执行价卡版本」：PRODUCT-BASELINE、PN-07 交接、PN-02 合成商业交接 | 不改：GLOSSARY 保留此词头并定义为价卡的合称，三句与新定义不相抵 |
| UC-CR-001 | 导览 9.0 新增「代收与清分」一表、9.11 补主要用例；开发主线写明它不归任何首发切片（首发不启用 COD）；切片表不列 |
| PRODUCT-BASELINE「面单服务终局边界由适用服务产品和客户合同定义」 | 改为由所采用接单规则包版本的面单服务终局规则定义，服务产品与客户合同版本是采用方（ADR-0058，与 GLOSSARY 同词条一致） |
| PILOT-SCOPE COD 排除句「代收应收」 | 改为代收与清分机制已在产品内（CR 资金位置原词）、首发试点不启用 |
| PRODUCT-BASELINE 关闭与重开一句 | 照 ADR-0275 决定一、二、三改写：只由运营角色凭授权规则决定、合同委派不参与，货主要求是请求证据，渠道服务方或代理商叫停记外部限制、不作请求方；同级或更高重开权限那半句不动 |
| UC-PS-001「开发拆分建议」末段 | 补引 ADR-0277 |
| UC-PS-001 业务决策简报「保留人工接受门槛」 | 改「人工复核门槛」，写明人工复核完成本身不形成决定（同 06 X12） |

全仓再查 docs/product、docs/design 与 10a 改过的词条、ADR-0274、0275 正面相反的句子，另改五处：
- 导览 9.11 输入：运营企业真实到账改为经 settlement-accounting 采用进入、代收与清分凭采用引用登记（ADR-0274）。
- 开发主线端到端主链阶段 3「揽收交接」改「场外揽收结果」（GLOSSARY 收寄一条的新词）。
- 参数登记册 PAR-SET-11「燃油与汇率两个实例」、计价规则模型最终设计「两个实例」：补定额附加金额（ADR-0110），不再写死两个。
- 参数登记册 PAR-SET-02、PAR-SET-03 与 PN-02 W03 工作单把「计价基准时点」列为向价卡或价格规则取的证据：改为产品内置、由发起评价的那一路给出（ADR-0190 决定二，与 GLOSSARY「计价基准时点」一致），不取证。
- PN-04 交接「封签连续的集运单元可形成单元控制及随单元受控的派生」：改为派生规则尚未确立、确立前单元交接不为成员形成参与或控制判断（ADR-0263 决定四、GLOSSARY「实物控制」）。

查过、不改的：「多维容量」（一班多维是一组池，与 GLOSSARY「容量池」相容）；「截单时刻」各处（都在说对账安排或归属日判定的登记项，与 ADR-0269 一致）；「计算目的」各处（都指 PP 的计算目的，GLOSSARY 已去掉 PC 引用它那半句）。

检查：rebase 冲突只在本票面末尾，两节都留、10a 在前，Status 行取 main；scratchpad 副本上 adr-index 生成后 `-check` 退 0，`at-coverage-counts -check` 退 0；改动的 `.md` 链接全部解析；已接受 ADR 正文对 `4e7874ba0` 逐字节相同。
