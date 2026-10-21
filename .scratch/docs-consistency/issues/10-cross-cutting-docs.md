# 10 横切文档：GLOSSARY、CONTEXT-MAP、SCENARIOS、产品文档、协作文档

Category: enhancement
Status: in-progress——10a 完工待评审与进 main，分支 claude-dc10a；10b 待开。此前 ready-for-agent——2026-10-21 协调方立（文档自洽修订最后一张，见 [README](../README.md)「修订执行」）
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
| SC·S-01 容量预占写成承运接受的结果 | A | 已改 | 场景十六第 1 步（同 08 X7） |
| SC·S-02 阶梯方式 | A | 已改 | 场景十五第 6 步按 ADR-0165 内置形态，数值另册登记 |
| SC·S-03 计价基准时点由合同约定 | A | 已改 | 场景十二前提与步骤（随 S-08 改写） |
| SC·S-04 合同委派可直接决定 | B | 代决 [ADR-0275](../../../docs/adr/0275-controlled-closure-and-reopening-are-decided-only-by-operator-roles-and-a-channel-stop-is-an-external-restriction.md) | PS CONTEXT 受控关闭与重开一条去掉例外；场景二第 10、16 步与预期结果；GLOSSARY「面单继续尝试决定」补一句 |
| SC·S-05 渠道方叫停写成请求 | B | 代决 ADR-0275 | 同上；ADR-0187 注记与 Links 挂由 ADR-0275 回答的来向 |
| SC·S-06 结算依据采购规则形成成本 | A | 已改 | 场景十六第 11 步 |
| SC·S-07 不予受理与不成立 | A | 已改 | 场景十八第 10 步改为通过资格审核、进入责任审核后的不成立结论 |
| SC·S-08 面单申请时预估并冻结 | B | 已由 09 消解（SA CONTEXT 冻结一条，`d52df6bf0`）+ 已改 | 09 已写明面单交易前的冻结今天不形成；场景十二改为网络服务委托的受理前估价冻结 → 包裹计费时点预估客户费用 → 复秤后计价纠错。面单渠道服务包裹的计费时点仍是 ADR-0190 越权风险点 8，本票不裁 |
| SC·S-09 待识别实物 U 被交出 | A | 已改 | 场景十第 6 步 |
| SC·S-10 结算按合同判断计费重量 | A | 已改 | 场景四第 5 步与预期结果、场景十一第 3 步 |
| SC·S-11 审核应付、客户运营应收直接形成 | A | 已改 | 场景一第 8 步 |
| SC·S-12 外部资金事实的进入口 | A | 已改 | 场景十四第 1 步；场景十二收款、场景十四与十七真实到账同补 |
| SC·S-13 改路的自动与建议分支 | A | 已改 | 场景三第 3 步 |
| SC·S-14 身份冲突处置 | A | 已改 | 场景九第 4 步与预期结果（ADR-0210） |
| SC·S-15 「十个限界上下文」 | A | 已改 | 开篇改指 CONTEXT-MAP |
| SC·S-16 文档关系漏 PC、CR | A | 已改 | 补两行 |
| SC·待核 Q-01 场景十六软超配 | 待核 | 不落 | 现行 TF CONTEXT 仍写软容量按阈值与授权超配，场景与之相容；软池怎么表达等 psb/09，本票不裁 |
| SC·待核 Q-02 同级或更高重开权限 | 待核 | 不落 | PS CONTEXT 仍是「同级或更高」，pc-gaps/13 裁决 ① 的首发判法没有核到细节；不改场景，留 PS、PC owner |
| SC·待核 Q-03 ADR-0090 引的场景原句已不存在 | 待核 | 归 10b | 是 ADR-0090 Status 注记的事 |
| SC·待核 Q-04 TF CONTEXT 规则与边界一节的张力 | 待核 | 不落 | 是 TF CONTEXT 自身的张力，场景与规则一致；08 已进 main、没交来，留 TF owner |
| SC·待核 Q-05 场景十四「代收应收、已收」 | 待核 | 已改 | 随 G-18 与 ADR-0274 改写第 6 步 |
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
| 04 第 6 条 UC-TF-005 `AT-TF-157`–`161` | 已改 | 五行改为引用 UC-VE-002、UC-VE-004 的对应行；AT 盘点①的五行不动（测试 Covers 仍点它们，改点归 code/32） |
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
- **[ADR-0275](../../../docs/adr/0275-controlled-closure-and-reopening-are-decided-only-by-operator-roles-and-a-channel-stop-is-an-external-restriction.md)（SC·S-04、S-05，代 PS、PC owner）**：受控关闭与重开只由运营侧授权角色形成，合同委派不参与，不是「首发不开」而是规则上不存在；货主的要求作为请求证据落在决定上；渠道服务方或代理商（不是委托客户账户时）叫停记为外部限制、请求方缺席，恢复是来源责任方的解除；不另立请求册。无条款停用，回答 ADR-0187 Alternatives 候选丁，不改代码。越权风险点：例外不存在（要开自助路须另起记录）；渠道方叫停靠运营角色转成关闭；渠道方的恢复靠来源解除；未授权请求不留痕。

作者另作的判断（不另立 ADR，评审可挑）：

1. **GL·G-09 只做交来的那几族**，其余同义重复不合并，理由见上表；GLOSSARY 开头那句只约束此后的新增与改写，不宣称现状已无重复。
2. **CONTEXT 级改动不限于交来的条目**：GL、SC 各条与两份新 ADR 要改的 CONTEXT 句子（PS、PC、SA、TF、NR、NO、VE、CR、CC、PP 各有几处）在本票改了，因为审计把这些条目交给票 10、各组已进 main。改的都是术语对齐与指针，没有改任何一条规则的结论；ADR-0275 改 PS CONTEXT 那一条是裁定的直接后果。
3. **ADR-0187 的 Status 注记与 Links 由本票挂**：分工说 10a 不改已接受 ADR 的 Status 行、本半边新 ADR 停用的除外。ADR-0275 是回答不是停用，但 README「一件事只写一处」要求被回答方注记挂来向，照 09 给 ADR-0190 挂 ADR-0270 回答的先例由本票做；ADR-0187 去掉 Status 行与 Links 后与 main 逐字节相同。
4. **UC-TF-005 迁走的五行保留为引用行**，没删：AT 盘点①的五行与测试 Covers 仍点它们，删行会让盘点指向不存在的行；改点新号归 code/32。
5. **AT 盘点**：`AT-CR` 各行里编排已有、测试未点名的入②，未实现的入④，追记一条；总览计数照规矩进 main 时重算。

### 立的代码票

- [code/54](../code/54-collection-bank-arrival-does-not-reference-an-adopted-funds-fact.md) CR 真实到账的证据引用不指向结算已采用的外部资金事实（R3，COD 进入试点之前必修；要裁的一格归 SA owner）。
- [code/55](../code/55-supplier-expected-cost-calls-the-price-card-version-a-purchase-rule-version.md) SA 把评价命中的价卡版本叫采购规则版本（R3，只改命名与注释）。
- 号段 code/56 未用。

### 归 10b、待它照新词对齐的

- 交来条目：01 交后续各条；03 X6、X7；04 第 7、8 条；06 X8、X10、X11、X12；07 X6–X9；08 X4、X6；SC·待核 Q-03；X-08 的 ADR-0045、0054 Links 与 `docs/review/081701.md`。
- 照 10a 新词要对齐的产品文档句子（10b 开工时以本分支进 main 后的词为准）：`docs/product/PRODUCT-BASELINE.md`「面单服务终局边界由适用服务产品和客户合同」一句（改为接单规则包版本的终局规则定义）；`docs/product/PILOT-SCOPE.md` COD 排除那一句里的「代收应收」（改用 CR 的对象与资金位置词表）。

### 检查

- 在 `git archive claude-dc10a` 解出的副本上跑 `go run ./tools/adr-index` 生成后 `-check` 退 0；生成的索引多出 ADR-0274、0275 的行，ADR-0187 一行多出由 ADR-0275 回答的注记，标签派生为〔代决·待 CR、SA owner 复核〕〔代决·待 PS、PC owner 复核〕。
- `go run ./tools/at-coverage-counts -check` 只报计数过期（新增 `AT-CR` 前缀一行与②④两桶的数），未重算。
- 改过的 .md 相对链接与锚点全部可解析（dc09tools 的 `links.py`，基准 main..claude-dc10a）。
- 书名号引文用 `quotes.py` 比过分支与 main 两份语料：本票新写的转述已去掉书名号或改引原句；余下几处是同文件内的自指或嵌套书名号，逐字存在。
- 已接受 ADR 正文逐字节不变：本分支只动 ADR-0187 一份已接受记录，去掉 Status 行与 Links 后与 main 相同；ADR-0274、0275 是新文件。
- 各份 `docs/domain/*/CONTEXT.md` 头部各有一处逐字的「规则与实现的差距见各 UC「给开发的交接」。」。
- 没改代码；没碰 `docs/product/`、`docs/design/`、`docs/agents/`、`AGENTS.md`、`docs/README.md` 与 `docs/adr/README.md`。
