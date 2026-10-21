# 文档自洽审计 · 横切组报告

- **取证版本**：只读 worktree `/home/tops/workspace/idp-parcel-audit`，detached @ `6b0074d0`（2026-10-21）。全程只读，没改仓库文件、没提交、没用 stash。文中所有「实测」「共 N 处」都是对这个 SHA 而言。
- **口径**：`.scratch/docs-consistency/README.md`（目标 1–6、A/B/C 归类、严重度）。每条发现写了类别、位置（文件 + 引文，不用行号）、证据、归类、严重度与建议修法。不报：生成物是否最新、纯措辞偏好、`docs/archive/`。
- **范围**：CONTEXT-MAP、GLOSSARY、SCENARIOS 与各 CONTEXT 的一致性；`docs/product/` 与 ADR 现行决定的一致性；`docs/adr/` 停用链全量与 infra-speed/07 的重复清单；两份已接受 ADR 互相矛盾而无停用关系的情形（B 类）；`docs/agents/` 与 `AGENTS.md`；全仓文档的引用规范与断链。各上下文 CONTEXT / UC 的逐条一致性归别组，这里只在横切处碰到。
- **做法**：停用链、Status 行重复、断链、行号 / 计数 / 位置引用由协调者写只读脚本抽取后逐条人工核（脚本在 `（审计会话的临时目录，未入库）docs-audit/cross-x/scripts/`）；GLOSSARY、CONTEXT-MAP、SCENARIOS、试点三件、产品基线诸件、`docs/agents/`、三簇 ADR 互相矛盾由子代理分头审，各自用 `grep -F` 核过引文，协调者回原文抽核了 GL·G-01、AG·H1、CM·M-2、SC·S-01、SC·S-04、PB·M1、B3·1 与 X-01（B2·B2-08 同条），并对 GLOSSARY、CONTEXT-MAP、SCENARIOS 三份报告里的引号引文做了机械比对（不在仓内文档里的多为建议改写稿、标题转述或代码注释，非位置引文）；其余条目以子代理各自的 `grep -F` 自核为准。下文编号前缀标明出处。

## 计数

共 151 条发现（另有待核若干，单列于后、不计数）。

| 严重度 | A 文档错 | B 要裁 | C 代码错 | 合计 |
|---|---|---|---|---|
| 高 | 8 | 1 | 0 | 9 |
| 中 | 48 | 20 | 0 | 68 |
| 低 | 72 | 2 | 0 | 74 |
| 合计 | 128 | 23 | 0 | 151 |

按来源：

| 来源（本报告编号前缀） | 范围 | 高 | 中 | 低 | 合计 |
|---|---|---|---|---|---|
| ADR 停用链与引用规范（协调者自审）（X） | `docs/adr/` 停用链全量、infra-speed/07、全仓断链与引用规范 | 1 | 3 | 6 | 10 |
| ADR 互相矛盾 · 商业与资金簇（B2） | PS、PC、PP、SA、CR 相关 ADR | 2 | 6 | 5 | 13 |
| ADR 互相矛盾 · 作业簇（B3） | TF、NR、NO、CC、VE 相关 ADR | 0 | 9 | 4 | 13 |
| GLOSSARY ↔ CONTEXT（GL） | GLOSSARY 与十个 CONTEXT | 1 | 7 | 11 | 19 |
| CONTEXT-MAP（CM） | CONTEXT-MAP 与 CONTEXT、ADR、代码 | 0 | 5 | 12 | 17 |
| SCENARIOS（SC） | SCENARIOS 与 CONTEXT、UC、ADR | 0 | 9 | 7 | 16 |
| 产品基线诸件（PB） | 首发基线、产品基线、关务基线、业务流程导览、产品故事、收费模型、竞品台账 | 3 | 16 | 13 | 32 |
| 试点三件（PL） | 参数登记册、试点范围、验收矩阵 | 0 | 6 | 9 | 15 |
| docs/agents 与 AGENTS.md（AG） | AGENTS.md 与 docs/agents/*.md | 2 | 7 | 7 | 16 |

按类别（以每条标的第一个目标号计）：

| 类别 | 条数 |
|---|---|
| 目标 1（CONTEXT 是现行规则唯一出处） | 57 |
| 目标 2（UC 与 CONTEXT、ADR 一致） | 7 |
| 目标 3（ADR 停用链完整） | 40 |
| 目标 4（文档与代码一致） | 15 |
| 目标 5（引用规范） | 15 |
| 目标 6（GLOSSARY 单一权威） | 17 |

## 覆盖缺口与去重

- **B1 簇（横切技术与治理 ADR 的互相矛盾专审）没有出报告**：子代理因额度中断，只留下候选抽取（`（审计会话的临时目录，未入库）docs-audit/b1-x/`）。协调者按协调方要求不再派分叉，自己过了它抽出的「自认相抵 / 改判而未停用」候选句：多数是 Alternatives 里的否决理由或已有「事实依据已更正」类 Status 注记（例如 0028 ← 0031），没有新的 B 类。这一簇的已知矛盾由别处报出：CM·M-2（0137 决定四 ↔ 0082）、PB·M1（Bento 持久化闸门）、X-11（0055 ← 0149）、B3·9 / B3·13（0078 ← 0091、0149、0150）。0022 / 0055 / 0072 / 0085 / 0100 / 0139 / 0140 / 0149 / 0150 这串的停用范围只做到附表 A 的逐对核对，没做「两份都现行的说法互相打架」的话题级通读——修订票开工前建议补这一项。
- **试点三件**：子代理报告写完后中断在交回前，协调者接手核过它的汇总与待核，原样并入（PL 前缀）。
- **去重**：B2·B2-08 与 X-01 是同一条（0181 ← 0182），只计 X-01；PB·L2 与协调者原拟的「全仓九份 contract test」一条相同，只计 PB·L2；PB·L7 与试点组「范围外线索」同为 0016 ← 0146，只计 PB·L7；产品基线组 Q、B2 组 Q-1、agents 组 Q1 三处待核的「Bento 闸门」即 PB·M1，只计一次。
- **与其他组交叠（修订立票时对一下，免得同一件事两张票）**：SC·S-03、SC·S-04、SC·S-05、SC·S-08 与 PS、PC、SA 组；GL·G-02、GL·G-05、X-01、B2 各条与 SA+CR、PC+PP 组；GL·G-01 与 TF、VE 组；CM·M-1、M-4 与 NR、CC 组；B3 各条与 TF、NR+NO、VE 组；PL·F1、F2 与 PC+PP 组。依赖：GLOSSARY、CONTEXT-MAP 的修订要等在途票 146+94 进 main（本轮 README 分组表）。
- **不在仓库里、但要转告的**：agents 组 AG·M3 核到 `gh auth status` 已带 `workflow` scope、`9045b6fd6` 已推上 origin/main——用户 auto-memory 里「Push cannot touch .github/workflows」那条已过期，本审计不改 memory，由协调方决定是否更新。


## 发现索引（高→低；同级内 B、C 在前）

| 编号 | 严重度 | 归类 | 一句话 |
|---|---|---|---|
| B2·B2-02 | 高 | B | SELL 方向的金额取整归谁：0107（与 PP CONTEXT）归商业价格政策，0190 与 0198（与代码）当作价卡声明 |
| X·X-01 | 高 | A | ADR-0182 收窄了 ADR-0181 决定五、决定六两句，ADR-0181 的 Status 行与 Links 都没有前向指针 |
| B2·B2-01 | 高 | A | ADR-0047 的「方式即选路」和额度来源已被 0122、0127 换掉，0047 没有任何停用指针 |
| GL·G-01 | 高 | A | GLOSSARY「出发门禁读数」仍写「每个在控对象逐个问关务」，ADR-0225 已对集运单元那一格部分停用这条规则 |
| PB·H1 | 高 | A | 导览说 `collection-remittance` 没有后端、没有 `CONTEXT.md`，与代码和开发主线都相反 |
| PB·H2 | 高 | A | 产品故事说首发只有网络服务一种对客形态，与 ADR-0088 决定一正相反 |
| PB·H3 | 高 | A | 开发主线 2026-09-30 的三条补记把「网络定义登记册的写入方」列为未满足的机制缺口，而那本册自 ADR-0148 决定六起已经不再被读 |
| AG·H1 | 高 | A | `domain.md` 说 `collection-remittance` 没有 CONTEXT.md，其实它 2026-08-28 起就有，状态是 Confirmed |
| AG·H2 | 高 | A | 生成物重生成、进 main 步骤只写在 `parallel-sessions.md`，而 AGENTS.md 和该文自己都说单会话不必读 |
| B2·B2-03 | 中 | B | 汇率口径：0148（与 0013、PP CONTEXT）要 PP 按价格政策声明的口径换算，0189 补记与代码只认价卡绑定的在用序列 |
| B2·B2-04 | 中 | B | 规范化版本：同样是「新加一格、旧文档逐字节不变」，0129 不换号，0209 换号 |
| B2·B2-05 | 中 | B | 双人复核：参考序列与目录复核写死「复核方≠登记方」，价卡发布与商业发布的批准却定为租户治理、明令两个方向都不许写死 |
| B2·B2-06 | 中 | B | 收尾供应商协议版本对在途委托的作用：0233 自认与 PC CONTEXT 相抵，只用运营约束挡住，没有裁定 |
| B3·1 | 中 | B | 等登记的格能不能先入账、续办触发以后再补：各上下文给了相反的答案（B） |
| B3·2 | 中 | B | 合同版继承的产品底座在索赔时现选，不随接受时的闭包冻结（B，fork 核出） |
| B3·3 | 中 | B | 0176 决定四按「查询携带的合同」点核，与 0136、代码相反，还丢了 0104 决定四的产品那一半（B，fork 核出） |
| B3·6 | 中 | B | 交接转入立起的控制，收寄不认，同一节点会另起第二个控制起点（B，fork 核出） |
| B3·8 | 中 | B | 代决复核 R11 改了设备答复的逐对象范围，却写「正文与 Decision 不变」，又与 0225 的 `members` 相抵（B） |
| GL·G-05 | 中 | B | 「对账周期」归谁两处说法相反：GLOSSARY 与 PC 说它是结算政策的一项，ADR-0158 与 SA 说它属对账安排；截单快照的「结算账户的时区和截单时刻」在账户上也没有 |
| CM·M-2 | 中 | B | 「结算是外部资金事实进入本产品的唯一采用口」与代收与清分直接接受「运营企业真实到账」相抵（两份已接受记录，互不停用） |
| SC·S-04 | 中 | B | 场景二：受控关闭与重开保留「合同委派可直接决定」的例外，与 PC CONTEXT 和代码相反 |
| SC·S-05 | 中 | B | 场景二：渠道服务方或代理商的叫停写成「请求」，ADR-0183 复核写成外部限制 |
| SC·S-08 | 中 | B | 场景十二：面单申请时形成预估费用并冻结，现行机制里没有这一路 |
| PB·M1 | 中 | B | 开发主线说 Bento 持久化闸门「已解除」，技术切片权威、ADR-0017/0026 和 AGENTS.md 说它仍阻断 |
| PL·F6 | 中 | B | 证据存储写死为「公司自建 MinIO」，与租户取值模型相抵 |
| AG·M1 | 中 | B | 机制清点「各笔在自己那笔上重生成」，与实际做法「进 main 时统一做」相反 |
| AG·M2 | 中 | B | `/code-review` 的 Spec 轴对照什么：workflow.md 说对照 UC、不对照工单，parallel-sessions 说对照票面 |
| AG·M6 | 中 | B | 「当前宿主」与多会话规程只描述 Cursor + idp-mcp 通道；10-16 起实际作业的 Claude Code 会话不在其中 |
| AG·M7 | 中 | B | AGENTS.md「必要时再 `/code-review`」与「代码票进 main 前必须有一份非作者评审」口径不一 |
| X·X-02 | 中 | A | 部分停用的现行写法仍在制造 infra-speed/07 那类重复：2026-10-20/21 新接受的几次停用，被停方 Status 行照旧前段一份、索引注记一份 |
| X·X-03 | 中 | A | Status 行之外还有第三种停用载体：头部的「Partially suspended by:」「Amended by:」行；其中 ADR-0012 ← ADR-0013 的限定只 |
| X·X-11 | 中 | A | ADR-0149 决定四把 ADR-0055 决定五「仍然拦着」的两项未决按口解了，ADR-0055 的 Status 行与 Links 没有前向指针 |
| B2·B2-07 | 中 | A | ADR-0080 改了闭包键上结算选择器的齐备判据，ADR-0044 决定二没有指针 |
| B2·B2-09 | 中 | A | ADR-0081 决定六「四格留 nil」里有三格已被后来的记录换掉，只挂了 0158 一条，索引注记反而写「其余各格仍有效」 |
| B3·4 | 中 | A | 0146 把候选生成、过滤、排序改判为产品策略，0052、0053、0068 仍写它们属租户取值，没挂指针（A） |
| B3·5 | 中 | A | 派送发起口已改挂运营决定，0149 的索引注记仍写它归作业事实（A） |
| B3·7 | 中 | A | 0230 改了晚到交接信封的处理，只停了 0216 决定四，没停决定一那句理由（A，fork 核出） |
| B3·9 | 中 | A | 0150 停用了 0091、0100 里「隔离形态不退场」的同义句，漏了 0078 决定四（A，fork 核出） |
| GL·G-02 | 中 | A | 「价格规则版本」在 SA CONTEXT 里多了「可执行价卡版本」一种含义，与 GLOSSARY、PC 的归属不同，代码已跟着分成两种用法 |
| GL·G-03 | 中 | A | GLOSSARY「面单服务终局边界」写成由服务产品和客户合同定义、PC 拥有「产品和合同规则版本」；ADR-0058 已裁定规则正文归接单规则包，产品与合同只是采用方 |
| GL·G-04 | 中 | A | GLOSSARY「客户费用退款」把「渠道结果」列为依据，SA 写明非索赔退款只来自计价纠错或商业让利、渠道退款结果不是这一调整 |
| GL·G-06 | 中 | A | 「当前客户资料版本采用判断」的取值集合：GLOSSARY 列五格、PS Language 列四格，两边都用的 `待复核` 却不在任一集合里 |
| GL·G-07 | 中 | A | 「限制适用性判断」的取值集合：GLOSSARY 五格（含「技术未形成」），CC Language 四格 |
| GL·G-08 | 中 | A | GLOSSARY「计价参考序列」说只有两个实例，ADR-0110 与 PP、代码已有第三种 |
| CM·M-1 | 中 | A | CC↔NR 关系仍按「关务适用性判断口尚未补上」写，CC 拥有清单也漏了这项判断 |
| CM·M-3 | 中 | A | parcel-pricing 拥有清单只认「燃油费率与汇率」两种序列，漏了第三种序列与计价参考目录 |
| CM·M-4 | 中 | A | 网络与路由择优写成「只消费 BUY 评价」且「形成唯一的候选选择」，与 ADR-0148 决定四和 NR 的并列规则相抵 |
| CM·M-5 | 中 | A | 结算→计价那条仍写「自然键只守幂等」，ADR-0229 已把唯一约束换成（自然键、代次） |
| SC·S-01 | 中 | A | 场景十六：容量预占写成承运接受的直接结果 |
| SC·S-02 | 中 | A | 场景十五：阶梯返利写成按合同声明的「阶梯方式」计算 |
| SC·S-03 | 中 | A | 场景十二：计价基准时点写成由客户合同约定 |
| SC·S-06 | 中 | A | 场景十六：供应商预期成本写成结算「依据采购规则」直接形成，跳过了计价 |
| SC·S-07 | 中 | A | 场景十八：不符合责任条件写成「不予受理」，混淆了资格审核与责任审核 |
| SC·S-09 | 中 | A | 场景十：待识别实物 U 被列为可交出的对象 |
| PB·M2 | 中 | A | 横切缺口里「两道哨兵」那段仍按 ADR-0064 之前和 ADR-0053 决定四写 |
| PB·M3 | 中 | A | HTTP 接入面仍被写成「`PAR-INT-01` 实例半边」「被 ADR-0055 决定五两项拦着」，与 ADR-0100、0139、0149 相反 |
| PB·M4 | 中 | A | 开发主线的 SELL/BUY 补记仍写「不进派发」，ADR-0190 已取代那几句，ADR-0208 已把 BUY 触发接进派发 |
| PB·M5 | 中 | A | 开发主线 PN-03 覆盖缺口一行仍写「当前代码尚未实现这些上下文」 |
| PB·M6 | 中 | A | 开发主线把商业解析排在生产归属之前，与 UC-PS-001、导览和代码的顺序相反 |
| PB·M7 | 中 | A | 「仍待提供」的列表把产品策略项整体归进实例半边（三类） |
| PB·M8 | 中 | A | 开发主线的 PN 切片两张表漏了后来新增的五个 UC |
| PB·M9 | 中 | A | 关务基线仍按「十二个关务用例」评估，没有 UC-CC-013 |
| PB·M10 | 中 | A | 关务基线把剩余阻塞全部归为真实实例，与 ADR-0146 和开发主线 PN-05 的重定级不一致（三类） |
| PB·M11 | 中 | A | 导览 §8.6 业务入口对照表的「当前行为」一列与代码、ADR-0100/0149/0150 不符 |
| PB·M12 | 中 | A | 导览 §12.2 的 PN 定级表与开发主线的两次定级都对不上 |
| PB·M13 | 中 | A | 导览 §9.0 说用例目录有 49 份，并漏掉 5 个 UC |
| PB·M14 | 中 | A | 导览第 9 章各模块的「产品操作面 / 当前成熟度」大面积过时，而且违反导览自己的维护规则 |
| PB·M15 | 中 | A | 导览对 UC-PC-003 授权范围的描述与 ADR-0116 和代码相反 |
| PB·M16 | 中 | A | 产品基线「首发原则」仍写「不同时铺开多个…渠道」，与 ADR-0088 决定三不符 |
| PL·F1 | 中 | A | `PAR-INT-08` 列举可授能力面时漏了「运营决定」 |
| PL·F2 | 中 | A | `PAR-INT-11` 与 `PAR-GOV-03` 只列了两组准入能力词，代码已收四组 |
| PL·F3 | 中 | A | 矩阵仍按 ADR-0016 的三条判据写产品就绪 |
| PL·F4 | 中 | A | 范围与登记册仍把「渠道或国家专属设计、容量维度」挡在租户参数之后 |
| PL·F5 | 中 | A | 矩阵对 41 例 `SOURCE_RATE_CARD` 的放行条件漏了金额保真度那道门 |
| AG·M3 | 中 | A | workflow.md 说凭据没有 `workflow` scope、改 `.github/workflows/` 的推送会被拒，这句已过期 |
| AG·M4 | 中 | A | workflow.md「四条红线」少了 AGENTS.md 的第五条「敏感实例外置」 |
| AG·M5 | 中 | A | AGENTS.md「改文档」只写「新 ADR 或 supersede」，没写部分停用怎么做，也没写回改 CONTEXT |
| B2·B2-10 | 低 | B | 计价范围登在两本册上：0179 估价采用册与 0190 SELL 计费时点采用册，和 0179 决定三「同一条规则选卡」相抵 |
| AG·L6 | 低 | B | `blocked` 状态：parallel-sessions 和脚本认它，triage-labels 没定义 |
| X·X-04 | 低 | A | 「部分被 ADR-0190 取代」不是 README 的机制：0169、0170 的注记没链接、没原句、没写其余各条 |
| X·X-05 | 低 | A | 索引注记里打了引号的「被停原句」有几处不是原句 |
| X·X-06 | 低 | A | Links 节前向指针缺失 |
| X·X-07 | 低 | A | Status 前段里「见 adr/README 该行」成了自指 |
| X·X-08 | 低 | A | 断链：三份 ADR 的相对链接指向不存在的文件，一份评审记录用本机绝对路径加行号 |
| X·X-09 | 低 | A | 「回答」类前向指针时有时无 |
| B2·B2-11 | 低 | A | ADR-0169「内置时点只有发生项形成」已被 0190 扩成三种，0169 的指针只挂了「不进派发」 |
| B2·B2-12 | 低 | A | 接受前控制链上三处取代只写在正文的复核记录里，Status 行与索引都没有指针 |
| B2·B2-13 | 低 | A | ADR-0017「PostgreSQL 实现…仍然阻断」已被 0026 的「即刻解锁的范围」收窄，0017 没有指针 |
| B2·B2-14 | 低 | A | ADR-0079 决定五写释放只认冻结账本，与 0047 决定四、0122 决定三的「两本账各认领一次」不符 |
| B3·10 | 低 | A | 0148 改了`未配置`的判法，0052 决定一的第二格定义仍自称不变（A） |
| B3·11 | 低 | A | 自动形成那一路的旅程已经派生，0207 决定四仍写「由登记方声明」（A，fork 核出） |
| B3·12 | 低 | A | 0204 说「自营只能显式声明」并称这是 0207 的读法，0207 却另有按承运商判断认自营一格（A） |
| B3·13 | 低 | A | 0078 决定五「照旧拦着……一切命令面」已被 0091 与 0149 推翻（A，fork 核出） |
| GL·G-09 | 低 | A | GLOSSARY 已定义的词，十个 CONTEXT 的 Language 节逐字或换字又各定义一遍，共 188 处（实测于 `6b0074d0`） |
| GL·G-10 | 低 | A | GLOSSARY「文档关系」没有 parcel-pricing；PP CONTEXT 开头也没有「跨上下文术语以统一领域语言为准」一句 |
| GL·G-11 | 低 | A | GLOSSARY「总单」的「运输履约范围」已由 ADR-0113 落定为「实际履约段」，GLOSSARY 未跟改 |
| GL·G-12 | 低 | A | 「揽收交接结果」只在 GLOSSARY、PS、NR 里出现，所有者 TF 从不用这个词；「有效收寄」在 GLOSSARY 里没有定义 |
| GL·G-13 | 低 | A | GLOSSARY 与 NO 对同一概念用了不同的词名：「实物控制关系 / 实物控制」「集运单元 / 集运单元实例」 |
| GL·G-14 | 低 | A | 「签约服务商」：TF 的交易角色集里有，PC 的角色集里没有，GLOSSARY 只在「承运商代理商」里顺带叫作「签约物流服务商」 |
| GL·G-15 | 低 | A | 不带前缀的「提交版本」：GLOSSARY 规定只在 CC 内作「申报提交版本」的简称，PC、SA 的 Boundaries 却拿它指 PS 的「委托提交版本」 |
| GL·G-16 | 低 | A | PC 引用 PS「资料修订阶段」词条里的「修订意图」原词，PS CONTEXT 并没有定义修订意图 |
| GL·G-17 | 低 | A | GLOSSARY「当前有效实测」与 NO 说 PP「依据计算目的形成计价重量」，GLOSSARY「计价重量」与 PP 说按重量策略 |
| GL·G-18 | 低 | A | PS、TF、SA 描述 CR 的产物时用「代收应收、实收/已收、清分」，这些词既不在 CR 的封闭词表里，也不在 GLOSSARY |
| GL·G-19 | 低 | A | 「应追偿金额」的相对方范围：GLOSSARY、VE 含「其他责任相对方」，SA 只到「供应商、实际承运商或保险方」 |
| CM·L-1 | 低 | A | 关系约束的箭头方向两种口径混用 |
| CM·L-2 | 低 | A | mermaid 图缺几条已有正文约束、代码里也有消费适配器的边 |
| CM·L-3 | 低 | A | 小包托运的渠道择优消费计价 BUY 评价，MAP 没有这条边，还把「多卡择优」整体写成 NR 的事 |
| CM·L-4 | 低 | A | 进 parcel-shipment 的「参数已登记」类交接只写了结算账户一封 |
| CM·L-5 | 低 | A | PC→TF、PS→TF 两条关系漏了派送要求的两样引用 |
| CM·L-6 | 低 | A | 结算→计价那条在计价的输入里用了「计费重量」 |
| CM·L-7 | 低 | A | 「客户查询授权」被写成 party-commercial 提供 |
| CM·L-8 | 低 | A | PS→NO 那条把只经客户包裹件号的命中也叫「唯一命中」 |
| CM·L-9 | 低 | A | 交接触发路由复核的那句漏了 ADR-0235 的「早于当前位置不判」 |
| CM·L-10 | 低 | A | 「参数已登记」的计价触发漏了价卡收尾（ADR-0234） |
| CM·L-11 | 低 | A | 收寄复核的触发其实经 PS 的「有效网络收寄采用」到 NR，MAP 写成节点作业直接提供 |
| CM·L-12 | 低 | A | MAP 没有交代 `pilotgovernance` 的位置，而 parcel-shipment 的生产归属依赖它 |
| SC·S-10 | 低 | A | 场景四、十一：计费重量写成由结算按合同规则判断，漏掉了计价侧形成计价重量 |
| SC·S-11 | 低 | A | 场景一：审核应付、客户运营应收写成「采用评价」直接形成 |
| SC·S-12 | 低 | A | 场景十四：外部资金事实的进入口没写，「不由结算生成」容易误读 |
| SC·S-13 | 低 | A | 场景三：改路写成 `network-routing` 直接形成新版本，漏掉了自动改路与改路建议的分支 |
| SC·S-14 | 低 | A | 场景九：没写身份冲突处置，「由 PS 确认后建立关联」与 ADR-0210 的读法有落差 |
| SC·S-15 | 低 | A | 开篇用「十个限界上下文」计数别处的东西 |
| SC·S-16 | 低 | A | 「文档关系」漏了两个上下文 |
| PB·L1 | 低 | A | 开发主线四项判据重定级的结论没有补记 ADR-0100 已落地、ADR-0139..0142 已接受 |
| PB·L2 | 低 | A | 「全仓九份 `*_contract_test.go`」已与现状不符 |
| PB·L3 | 低 | A | 「五条编排出口……共七处」的计数已变 |
| PB·L4 | 低 | A | 「紧接其上那句」的位置引用已经断了 |
| PB·L5 | 低 | A | 关务基线抄了「31 项」这个别处的计数 |
| PB·L6 | 低 | A | 开发主线引用已被取代的 ADR-0018 作为端点落位依据 |
| PB·L7 | 低 | A | ADR-0016 的 Status 行没有 ADR-0146 的前向指针，而开发主线和产品故事都把它当「完整决定」引用 |
| PB·L8 | 低 | A | 产品故事对产品就绪的复述漏了产品策略（三类） |
| PB·L9 | 低 | A | 导览把角色定义也划给租户登记（三类） |
| PB·L10 | 低 | A | 导览说前端没有 CI 和测试门禁，与 `ci.yml` 不符 |
| PB·L11 | 低 | A | 收费模型给产品基线加引号引了一句原文里没有的话 |
| PB·L12 | 低 | A | 择优的归属写得不准：面单渠道择优归 PS，BUY 评价由计价形成 |
| PB·L13 | 低 | A | 产品故事说数据态展示还要等「隔离环境读面准入」，ADR-0150 后这一说法已过时 |
| PL·F7 | 低 | A | `PAR-GOV-10` 标`待核验`，与「全部行保持空模板」及本行拆分注相抵 |
| PL·F8 | 低 | A | ADR-0142 要求登记册增「客户 webhook 订阅」一行，没有增 |
| PL·F9 | 低 | A | `PAR-COM-18` 影响面写「09–17」，漏了第十册的表单票 18 |
| PL·F10 | 低 | A | 三处列举路由复核触发，都漏了 ADR-0215 的「交接进节点」 |
| PL·F11 | 低 | A | 「进入限量生产前」的逐切片门槛没收后来增的几行会挡生产写的参数 |
| PL·F12 | 低 | A | 范围有几句「等待真实…确认」，涵盖了登记册拆分后已划为产品策略的判断方法 |
| PL·F13 | 低 | A | `PAR-VIS-02` 最低证据写「保守默认依据」，与「数据不足时保持未配置」用词相抵 |
| PL·F14 | 低 | A | 面单渠道服务是否必验：范围、登记册与矩阵两行给出三种条件 |
| PL·F15 | 低 | A | `PAR-SET-03` 把参考设计里某一张源价卡的取证状态混进了模板行 |
| AG·L1 | 低 | A | parallel-sessions 给的 `branch-state.ps1` 跑法在现宿主上跑不起来，和 workflow.md 相抵 |
| AG·L2 | 低 | A | parallel-sessions 文内七处位置引用方向写反（写「上面」「上节」，所指内容在后文） |
| AG·L3 | 低 | A | workflow.md 跨文件写「上方路由表」，并有不带锚的别处计数 |
| AG·L4 | 低 | A | 「-p 1」的残留措辞 |
| AG·L5 | 低 | A | workflow.md「当前宿主」里 `core.autocrlf` 前后两说 |
| AG·L7 | 低 | A | workflow.md 指的 `scripts/link-skills.sh` 不在本仓 |

## 高

### B2·B2-02 SELL 方向的金额取整归谁：0107（与 PP CONTEXT）归商业价格政策，0190 与 0198（与代码）当作价卡声明

来源：ADR 互相矛盾 · 商业与资金簇

- **类别**：3
- **位置**：
  - `docs/adr/0107-evaluation-amount-rounding-is-declared-by-the-price-card-like-weight-rounding.md` 决定一：「采购方向的策略属价卡内容，销售方向的属商业价格政策——与体积系数的两侧归属逐字同构」。
  - `docs/domain/parcel-pricing/CONTEXT.md`「金额取整策略」词条的写法与此相同：「采购方向的策略属价卡内容，销售方向的属商业价格政策」。
- **证据**：
  - `docs/adr/0190-…-estimated-customer-charge.md` 三类划分：「**租户取值**：…费用代码映到哪个费用项目；价卡的金额取整声明。」
  - `docs/adr/0198-sell-request-conclusions-on-the-parcel-billing-moment-path.md` 决定二：「**二、卡没声明金额取整，请求记`待判断`，入账不重投。**」以及「取整声明与映射行同属租户取值。」
  - ADR-0229 决定五那张表里，「取整未声明」只能由「价卡」登记续办。
  - ADR-0179 决定三选 SELL 卡时明写：「不走 PC 价格政策的定价方案绑定（ADR-0034）」。SELL 评价因此根本不经过商业价格政策。
  - 代码里金额取整只挂在价卡上：`internal/parcelpricing/adapters/pricecardtemplate/decode.go` 的 `readAmountRounding`，`domain/evaluation.go` 的 `applyAmountRounding`。PC 的 `PricePolicyCaliber`（`internal/partycommercial/domain/price_policy_caliber.go`）只有税务、体积、汇率三格，没有取整格。
  - 同根的 C 性质事实：PC 的 `PricePolicyCaliber.volumetric` 对销售方向必填，但 PP 的 `VolumetricFactor`（`domain/plan.go`）只从价卡读，`internal/parcelpricing` 里没有任何代码读 PC 的价格政策口径。租户登在销售价格政策上的体积口径对 SELL 评价不起作用。
- **归类**：B。两份已接受 ADR 在同一格上结论相反：CONTEXT 站在 0107 一边，代码站在 0190、0198 一边，谁也没有停用谁。
- **严重度**：高。照 CONTEXT 实现 SELL 取整的作者会把它放进 PC。租户在商业价格政策上登 SELL 体积口径（以及将来的取整）不会生效，计费重量与金额会与租户的预期不一致。
- **建议修法**：立新 ADR 裁定「SELL 评价的计价条款（金额取整、体积系数、汇率口径，见 B2-03）从哪一份工件读」。
  - 若认价卡：部分停用 0107 决定一后半句；PP CONTEXT「金额取整策略」「体积系数」两个词条改口；PC 价格政策口径的体积格明写为不参与计价，或者撤掉。
  - 若认商业价格政策：立代码票，让 SELL 选卡与评价读 PC 口径，同时按 0179 决定三「正式路径将来若改走商业价格政策的绑定，两处同一笔改」一并改。

---

### X·X-01 ADR-0182 收窄了 ADR-0181 决定五、决定六两句，ADR-0181 的 Status 行与 Links 都没有前向指针

来源：ADR 停用链与引用规范（协调者自审）

- **类别**：目标 3（停用链缺一头）
- **位置**：`docs/adr/0181-posted-balance-is-recomputed-in-the-funds-mapping-transaction.md` 的 `Status` 行没有任何一处提到 ADR-0182，索引注记反而照旧写「信用状况行在到期日有处可读之前不写」；Links 节列的是 ADR-0007、0047、0127、0146、0150、SA CONTEXT、UC-SA-005 与票 product-strategy-boundary/29，也没有 ADR-0182。被收窄的两句原文：「**五、信用状况行在算得出逾期之前不写。无行仍是信用状况不可用，不读成未逾期。**」与「**六、首切只挂决定一在客户收款映射及其撤销上的那一支。**」
- **证据**：`docs/adr/0182-receivable-account-is-a-funds-mapping-target.md` 正文：「决定五、决定六里「到期日有处可读之前不写信用状况」与「首切只挂对账单映射」这两句，由本记录在上面三条的条件下收窄。ADR-0181 的求法与「无行不是未逾期」不改。」；同记录决定一：「**一、应收结算账户成为第四种映射目标 `ACCOUNT`。**」。另：0182 所引两句都不是 0181 的原句（0181 原文见上），0182 也没有 Links 节。SA CONTEXT 已按 0182 写（「该作用域没有已确认费用则逾期为假并写行」），代码 `internal/settlementaccounting/adapters/postgres/customer_receipt_posting.go` 有 `INSERT INTO settlement_accounting.credit_standing`——0181 一侧是唯一没跟上的。B2 簇子代理独立核出同一条（其 B2-08，本报告并入此条、不重复计数）。
- **归类**：A（0182 是已接受的后来者，收窄范围写在它正文里；缺的是被停方那一头）
- **严重度**：高。只读 0181 的作者会认为首切只有对账单映射、第一张对账单之前的预收进不了入账余额、信用状况行要等到期日可读才写——三件都已被 0182 改掉，读者会误判业务结果。
- **建议修法**：0181 `Status` 行的索引注记加一条部分停用：被停条款逐字引 0181 决定五首句与决定六首句，**适用场景**照 0182「在上面三条的条件下」写明，并写「求法与『无行不是未逾期』不变，其余各条不变」；0181 Links 补 ADR-0182 一条。0182 正文不改（已接受），其引文非逐字一事在 0181 的注记里以 0181 原句为准即可。

### B2·B2-01 ADR-0047 的「方式即选路」和额度来源已被 0122、0127 换掉，0047 没有任何停用指针

来源：ADR 互相矛盾 · 商业与资金簇

- **类别**：3
- **位置**：`docs/adr/0047-terms-control-forms-credit-exposure-not-a-freeze.md`
  - Decision 二：「**二、账期分支形成信用暴露，走自己的账本。**」以及「预付读运营余额，账期读 `CreditStanding`（当前额度、已占用暴露、是否逾期）」。
  - Consequences：「SA 施加编排按方式分支」；「同一委托「信用校验+预付冻结」并行（`AT-PS-035` 的同时要求）作用在**不同控制范围**上」。
  - 标题本身就是「账期方式的接受前控制形成信用暴露」。Status 行只有 `Accepted`，索引行没有注记。
- **证据**：
  - ADR-0122 决定二：「控制种类决定走哪本账」「ADR-0047 的两本账互不借用不变，变的只是选路开关」。Context 认定可以有组合：「一份账期合同同时要求预付保证金冻结、一份预付合同同时要求信用校验」，也就是同一范围里两种控制并存。Links 里自认：「「方式即选路」是被本记录换掉的那一半」。
  - ADR-0127 决定四：「额度取政策授权的金额（`CreditStanding.WithAuthorizedLimit`）」；「`credit_standing.limit_minor` 自此只是登记状况的一列，不再是额度依据的来源」。
  - ADR-0125 的 owner 复核写了「ADR-0047 两格降为投影而保留、不 supersede」。它只说了决定三的 `HELD` / `CREDIT_EXPOSED` 词汇，没有说到选路与额度来源。
  - 代码：`internal/settlementaccounting/application/apply_pre_acceptance_control.go` 里的 `ApplyPreAcceptanceControlHandler`，逐项按 `switch item.Kind()` 分路。`exposeCredit` 先调 `creditBasis.LoadCreditBasis`，再调 `standing.WithAuthorizedLimit`。
- **归类**：A。代码与 0122、0127 一致，0047 漏挂了停用指针。
- **严重度**：高。读者从索引或标题看到的规则是「账期 → 暴露、预付 → 冻结」，照着写会按结算方式选账本，与现行按控制种类逐项执行的实现相反。
- **建议修法**：在 0047 的 Status 行、Links 与索引注记各加两条部分停用：
  - ADR-0122 决定一、二停用以下几句：决定二里把「账期分支」当作选路判据的那一句；Consequences「SA 施加编排按方式分支」；Consequences「作用在**不同控制范围**上」。
  - ADR-0127 决定四停用决定二括号里「当前额度」取自 `CreditStanding` 的那半句。
  - 写明「其余各条不变」：两本账互不借用、决定一要求带方式与采用政策、决定三的词汇、决定四两本账都要认领。停用方那一头，以 0122 Links 里已有的那句为准。

### GL·G-01 GLOSSARY「出发门禁读数」仍写「每个在控对象逐个问关务」，ADR-0225 已对集运单元那一格部分停用这条规则

来源：GLOSSARY ↔ CONTEXT

- **类别**：目标 4（GLOSSARY 条目仍以现行口吻写着已被 ADR 部分停用的规则），兼目标 6。
- **位置**：`docs/domain/GLOSSARY.md`「出发门禁读数」
  - 「一条出发落册时与它同一笔保存的、逐个在控对象向动作准入判断拉来的答复记录：放行、阻断、不受门禁、未决或读不动，整条出发取最重的一格。」
  - 「所有者：`transport-fulfillment` 拥有读数记录；答复与依据属 `customs-compliance` 的动作准入判断，读数只翻译、原文保存。」
- **证据**：
  - `docs/domain/transport-fulfillment/CONTEXT.md` 同名词条：「在控对象是集运单元时，单元本身不问关务，按出发那一刻的直接成员逐件问，单元那一格取成员里最重的一格，成员各自一格」。
  - ADR-0225 的 Status 行：「部分停用 ADR-0205 决定一「对象与它的格」与决定二「关务的答复译成本上下文的五格」，仅限单元那一格」。
  - ADR-0225 Context 写了旧读法出过的事：「一袋装着被扣包裹的境内出发读成干净的不受门禁」。
  - ADR-0225 还写了「阻断信号只落在问过关务的那几格，单元那一格不另形成」。
  - 代码与 TF CONTEXT 都按 ADR-0225 做（ADR-0225 点名的 NO 读口 `ConsolidationMembershipAtView`），实现没错。
- **归类**：A。
- **严重度**：高。现在 GLOSSARY 的原话是「每个在控对象都向动作准入判断拉答复，读数只翻译」。照这句写，集运单元也会去问关务，ADR-0225 修掉的那个漏判就回来了：一袋里有被扣包裹，出发却读成不受门禁。消费方（VE 据阻断派生信号）照这句写，还会把单元那一格也当成关务的答复，另外多出一条信号。
- **建议修法**：给 GLOSSARY 该词条补一句集运单元的读法：单元不问关务，取出发那一刻直接成员里最重的一格，成员各自一格。所有者一条改成「成员各格属关务答复，单元那一格由运输履约按成员折出」，并指向 ADR-0225。

---

### PB·H1 导览说 `collection-remittance` 没有后端、没有 `CONTEXT.md`，与代码和开发主线都相反

来源：产品基线诸件

- **类别**：目标 4（文档与代码），兼目标 1（与开发主线 2026-09-04 补记相抵）
- **位置**：导览 §9.11「首发边界与当前状态」：「首发明确不包含 COD，不建设该模块的后端实现。当前只有上下文地图中的边界定义和管理台骨架，不应据此宣称已有代收能力。真实范围确认前也不创建空 `CONTEXT.md` 或假资金流程。」；§13.1：「领域地图中的 `collection-remittance` 只有边界定义和骨架，因首发排除 COD 不在首发实现范围」
- **证据**：
  - `docs/domain/collection-remittance/CONTEXT.md` 存在，首行是 `Status: Confirmed`。
  - `internal/collectionremittance/` 下 `adapters`、`application`、`domain`、`ports` 四层齐全；有 `cmd/parcel-collection-register`；`apps/admin-web/src/page-registry.tsx` 的 `liveIds` 含 `'cod-ledger',`。
  - 开发主线 2026-09-04 补记：「`collection-remittance`（代收与清分，按 `PA-CR-01` 准入，批票 04）自 `e125c2a` 起有生产代码」；ADR-0082（Accepted）定了代收分户账的形状。
- **归类**：A
- **严重度**：高。售前和实施照这段会对外说「没有代收能力」，研发会以为这个上下文不存在。
- **建议修法**：把 §9.11 和 §13.1 改成两件分开的事：机制已经按 `PA-CR-01` 准入并落地；首发试点不启用 COD（PILOT-SCOPE）。删掉「不创建空 `CONTEXT.md`」这一句。

### PB·H2 产品故事说首发只有网络服务一种对客形态，与 ADR-0088 决定一正相反

来源：产品基线诸件

- **类别**：目标 1
- **位置**：`PRODUCT-STORY.md`「一句话定位」：「首发对客服务形态是网络服务;」。附带：导览 §2.1「首发试点聚焦跨境出口网络服务」。
- **证据**：ADR-0088（已接受）Decision：「1. **首发服务形态改为网络服务与面单渠道服务的明确组合。**」。开发主线「已确认的首发范围约束」也写着「首发对客服务形态是网络服务与面单渠道服务的明确组合（ADR-0088 修订…）」。
- **归类**：A
- **严重度**：高。这份文档是对外叙事口径，专门服务商务洽谈。照它讲，会把首发的面单渠道转售说成不卖，而这正是候选租户的两条主力业务之一（ADR-0088 Context）。
- **建议修法**：改成「网络服务与面单渠道服务的明确组合」，并链到 ADR-0088。导览 §2.1 一并补上面单渠道服务。

### PB·H3 开发主线 2026-09-30 的三条补记把「网络定义登记册的写入方」列为未满足的机制缺口，而那本册自 ADR-0148 决定六起已经不再被读

来源：产品基线诸件

- **类别**：目标 1（引用的 ADR-0053 决定四已被部分停用，文档仍按旧条款写）
- **位置**：开发主线「机制半边现状」的三条补记：
  - ADR-0156 那条：「网络定义登记册的写入方、结算账户登记册、面单择优链的接受时解析回指仍是本行未满足的部分」
  - ADR-0159 那条：「网络定义登记册的写入方、结算账户登记册仍是本行未满足的部分」
  - ADR-0158 那条：「网络定义登记册的写入方、控制金额源仍是本行未满足的部分」
- **证据**：
  - ADR-0148 决定六：「定义登记册（迁移 `0007`）不再承载`未配置`，读口不再读它；表按迁移不可改的纪律留着，不删。」它部分停用了 ADR-0053 决定四，ADR-0053 的 Status 行已有前向指针。
  - 开发主线自己在 2026-09-24 那条补记里写了「`NetworkDefinitions` 已删」，日期早于这三条。
  - 代码里 `NetworkDefinitions` 和 `ErrNetworkDefinitionUnresolvable` 都已经搜不到。
  - 另外，「控制金额源」那半句已被 2026-10-13 的 ADR-0179 补记判为过时，但那条补记只点名了 2026-10-12 的「上一条」，没有覆盖这一条。
- **归类**：A
- **严重度**：高。这三句以现行口吻列出「剩余缺口」，作者可能照着去给一本已退役、不会被读的表做写入方。
- **建议修法**：加一条补记，说明自 routing-first-cut/07（ADR-0148 决定六）起「网络定义登记册的写入方」不再是缺口；ADR-0158 那条里的「控制金额源」一并标注由 ADR-0179 收口。

---

### AG·H1 `domain.md` 说 `collection-remittance` 没有 CONTEXT.md，其实它 2026-08-28 起就有，状态是 Confirmed

来源：docs/agents 与 AGENTS.md

- **类别**：1（规程单一权威）；兼 5（指向的现状失真）
- **位置**：`docs/agents/domain.md`
  - 「If a mapped `CONTEXT.md` is intentionally absent (currently」接下一行「`collection-remittance` — first-release COD out of scope), record map-only」，后面接着要求 「do not invent empty context docs」
  - 映射表：「| `collection-remittance` | map-only until COD enters product scope |」
  - File structure 树只列了九个上下文，最后一项是「│   │   └── settlement-accounting/CONTEXT.md」
  - 同一事实的连带位置：`docs/agents/workflow.md`「九个 `CONTEXT.md` 与 [ADR 索引](../adr/README.md)里已成文的全部记录都已经存在」。这句写死了一个别处的计数，紧接着的一句又在批评写死份数（「此前写死的「十一份」正是这样烂掉的」），而这个数现在已经错了。
- **证据**：`docs/domain/collection-remittance/CONTEXT.md` 存在，头部有「Status: Confirmed」，由 `2ad600c8e`（2026-08-28，「CR 上下文文档从零」）立档。`docs/domain/CONTEXT-MAP.md` 有「### [`collection-remittance`](./collection-remittance/CONTEXT.md)：代收与清分」。`ls docs/domain/*/CONTEXT.md | wc -l` 得 10。
- **归类**：A
- **严重度**：高。`domain.md` 是工程 skills 读领域文档的入口（「How the engineering skills should consume…」）。照它做，碰到 COD 或代收的 agent 不会去读 CR 的权威 CONTEXT，会按「map-only」只记边界，甚至以为不该有这份文件。
- **建议修法**：把 CR 加进 File structure 树和映射表，删掉「intentionally absent」那一段，或改成「目前没有缺席的上下文」。workflow.md 里的「九个」去掉数字，改成「各上下文的 `CONTEXT.md`」。

### AG·H2 生成物重生成、进 main 步骤只写在 `parallel-sessions.md`，而 AGENTS.md 和该文自己都说单会话不必读

来源：docs/agents 与 AGENTS.md

- **类别**：1（规程单一权威）
- **位置**：
  - `AGENTS.md`「Parallel sessions」节：「Single-session work does not need it.」
  - `docs/agents/parallel-sessions.md` 文首：「单人单会话不必读本文。」
- **证据**：机制清点、ADR 索引、AT 盘点计数什么时候重生成、由谁重生成，现行规则只在 `parallel-sessions.md` 有。一处是「生成物不占号」节：「它们与机制清点差一处：**票不在自己的笔上重生成它们**」。另一处是进 main 第 2 步：「2026-10-19 起同一步再加两件，各自单独成笔、口径同机制清点」。`docs/adr/README.md` 也把时机指到这里：「带新 ADR 或改 `Status` 行的票不碰本页，重生成的时机见 [parallel-sessions「生成物不占号」]」。CI 只在 main 推送上核：`.github/workflows/ci.yml` 的 `ADR index and AT coverage counts are current` 一步带 `if: github.event_name == 'push' && github.ref == 'refs/heads/main'`。所以一个单会话作者照 AGENTS.md 不读这份文档，带新 ADR 或改 AT 行的票直接落 main，就会在 main 上红一次，「Mechanism inventory is current」同理。本文件自带的全量口径（推送方用 `scripts/test-full.sh`）也只写在这里。
- **归类**：A
- **严重度**：高（照着做会把 main 的 CI 弄红，漏跑生成物）
- **建议修法**：把「落 main 前必做」（三件生成物重生成、全量用 `scripts/test-full.sh`）抽成一小节。放进 `workflow.md`，或在 AGENTS.md 写明「不论单会话还是多会话，落 main 都照 parallel-sessions『别人分支上的活怎么进 main』与『生成物不占号』」。两处「单会话不必读」随之改成「多会话的地盘、占号、点名部分单会话不必读」。

---

## 中

### B2·B2-03 汇率口径：0148（与 0013、PP CONTEXT）要 PP 按价格政策声明的口径换算，0189 补记与代码只认价卡绑定的在用序列

来源：ADR 互相矛盾 · 商业与资金簇

- **类别**：3
- **位置**：
  - `docs/adr/0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md` 决定四第 2 条：「换算所用的汇率口径（牌价类型、取值时点规则、加点规则）由策略版本所引的一份价格政策声明——PP 不接受未声明口径的裸汇率」。
  - ADR-0013 Decision：「汇率的牌价类型、取值时点规则和加点规则由 `party-commercial` 的商业价格政策版本化声明」。
- **证据**：
  - `docs/adr/0189-leg-pricing-zones-…md` 补记，越权风险点 8：「比价与正式评价只认 parcel-pricing 的在用序列」。
  - ADR-0179 决定二：「要以结算币种出价，靠价卡自己声明的计价参考序列」。
  - ADR-0157 Consequences：「价格政策汇率格可以选用同一引用。计价侧没有汇率时点执行器，那一格仍答`未配置`」。
  - 代码：NR 登记了 `ComparisonPricePolicy`（`internal/networkrouting/ports/catalog_registration.go`、`adapters/postgres/network_catalog.go`），一路带到 `application/catalog_initial_route_evidence.go`，此后没有任何代码读它。PP 的比较币种换算是 `domain/evaluation.go` 里的 `convertAmount(pricedTotal, reading, comparison)`，读的是卡上绑定的序列。`internal/parcelpricing` 里没有口径读取，评价实际是按裸汇率换算的。
- **归类**：B。0148 与 0189 补记结论相反：文档一侧还有 0013 与 PP CONTEXT「不接受未声明口径的裸汇率」，代码与 0189 一致。
- **严重度**：中。今天演示只有一种序列，看不出差别；租户一旦登了多种牌价或加点规则，换算结果与租户声明的口径不符，也没有任何地方报错。
- **建议修法**：与 B2-02 合并立一份 ADR，裁定汇率口径进不进评价。
  - 若进：立代码票，PP 接收口径，NR 传 `ComparisonPricePolicy`，SELL 路径同样传入。
  - 若不进：部分停用 0148 决定四第 2 条那半句，并收窄 0013 那一句的适用场景；PP CONTEXT 那句改口；NR 删掉这一格死数据。

### B2·B2-04 规范化版本：同样是「新加一格、旧文档逐字节不变」，0129 不换号，0209 换号

来源：ADR 互相矛盾 · 商业与资金簇

- **类别**：3
- **位置**：`docs/adr/0129-credit-ratio-base-is-declared-on-the-credit-policy-content.md` 决定一：「PCC-1 不换号：`canonicalCreditPolicyBody` 加键 `ratioBase,omitempty`（ADR-0126 决定一「各册接进同一个 `PCC-1` 不换号」同法）」，以及「金额正文的规范化文档因此一字不变，既有摘要不动」。
- **证据**：
  - `docs/adr/0209-…md` 决定三：「**三、带条件的正文以 `PCC-2` 规范化，不带的仍是 `PCC-1`、逐字节不变；本构建两版都认。**」以及「改的是供应商协议一节的形状，照 ADR-0014 换号。」这是同一种情形（只有带新内容的文档会变），做法却相反。
  - ADR-0126 决定一：「换号只在既有册的文档形状变化时发生（ADR-0014）」。0129 援引的「加册不换号」说的是一册从没接到接上，不是在已接的册里改形状。
  - ADR-0014 Consequences：「每次变化都要新增一个规范化版本并长期支持它」。ADR-0015「与 ADR-0014 的分工」：「只有当新词汇需要规范化文档增删改字段、或把标量换成集合时才升」。
  - 0014 已有的几次部分停用（0177、0201、0202）都按停用机制挂了指针，并写明「此后任何一册再改规范化形状照本记录换号」。0129 不在其列。
  - ADR-0176 决定五反把 0129 当作先例来引：「若要改则按加 omitempty 键不换号的先例另核」。
  - 代码：`internal/partycommercial/domain/publication_canonicalization.go` 里 `RatioBase string json:"ratioBase,omitempty"` 仍在 `PCC-1` 之下；同一文件另有供应商协议用的 `PCC-2`。
- **归类**：B。仓里事实上存在两条换号规则：0014、0126、0209 一条，0129、0176 一条，没有记录裁过它们的先后。
- **严重度**：中。下一位给 PCC 或 PPC 加可缺键的作者，无论照哪一条做，都能找到一份已接受 ADR 撑腰。摘要可比性的口径因此不唯一。
- **建议修法**：新 ADR 裁定「`omitempty` 加键、旧文档字节不变」算不算形状变化。
  - 若算：把 0129 补记为对 0014 Consequences 那一句的部分停用，适用场景仅限信用政策加 `ratioBase` 那一次，前提与 0177 相同（没有存量摘要）；0176 决定五括号里的先例改指这条停用。
  - 若不算：部分停用 0014 那句与 0126 决定一那半句，并说明 0209 为什么仍然换号。

### B2·B2-05 双人复核：参考序列与目录复核写死「复核方≠登记方」，价卡发布与商业发布的批准却定为租户治理、明令两个方向都不许写死

来源：ADR 互相矛盾 · 商业与资金簇

- **类别**：3
- **位置**：
  - `docs/adr/0099-price-card-binds-series-identity-and-in-force-version-is-derived-from-review.md` 决定二：「复核责任方不得与该版本的登记责任方相同（`PAR-SET-11` 分列两种责任的机制落点）」。
  - ADR-0109 决定二规定目录：「复核与在用的门照 ADR-0099 给序列立的那一套」。
- **证据**：
  - `docs/adr/0101-operator-facing-registration-payload-shape-is-product-defined.md` 决定六：「录入者与批准者须否为不同主体、批准者须持哪一格授予，属租户的治理规则」，以及「不写死「默认必须双人」，也不写死「默认单人可批」——两者都是替租户定治理规则」。0101 与 0099 是同一天（2026-09-03）立的。
  - ADR-0126 决定三：「不写死「必须双人」也不写死「单人可批」，两者都是替租户定治理规则」。
  - 代码按 0099 写死了：`internal/parcelpricing/domain/reference_series_review.go` 的 `ErrSeriesReviewerIsRegistrant`；`reference_catalogue_review.go` 的 `ErrCatalogueReviewerIsRegistrant`。
- **归类**：B。同一类「录入人以外的第二个人」规则，一份当作机制写死，两份当作租户取值、禁止写死。
- **严重度**：中。单人运营的租户无法自审序列或目录的版本，而同一租户的价卡批准可以登记成单人可批。分界检验（ADR-0146）在这件事上出了两个答案。
- **建议修法**：新 ADR 按 ADR-0146 的分界检验裁定职责分离归哪一类。
  - 若归租户治理：部分停用 0099 决定二那一句（连同 0109 的继承），复核门改读审批职责规则（`PAR-SET-12` 那一类），并立代码票。
  - 若归产品策略：收窄 0101 决定六、0126 决定三「两个方向都禁」的适用场景，限定在发布批准门。

### B2·B2-06 收尾供应商协议版本对在途委托的作用：0233 自认与 PC CONTEXT 相抵，只用运营约束挡住，没有裁定

来源：ADR 互相矛盾 · 商业与资金簇

- **类别**：3
- **位置**：`docs/adr/0233-an-older-commercial-version-is-closed-for-a-named-successor-by-a-cut-or-a-same-start-retirement.md`
  - 决定五：「运输履约的核对按既有分格把截止点之后读作未生效、把已退役读作已终止」。
  - 越权风险点 1(c) 自认：「这与 PC CONTEXT「已退役……既有委托、交易和结算继续引用原版本」相抵」。
  - 风险点里给的处置只是一条运营约束：「**运营约束：票 88 落地前，不得截止或退役仍被在途运输委托指名的供应商协议版本。**」
- **证据**：
  - 0233 决定三自己写的是：「正文、批准与原区间保留，既有委托、交易与结算继续引用它」。
  - 0233 决定五又写：「为既有引用点读的口（按回指取已固定的解析、按已固定的快照取正文）不折收尾」。
  - ADR-0204 决定一：「服务方、协议快照与采购责任法人的权威仍是委托那一行」。
  - ADR-0218：「未生效与已终止可能就是委托那一版在业务时间的如实答案」。
  - PC CONTEXT 生命周期：「已发布 → 已退役：自退役生效时间起不再供新的商业选择，但已经接受的委托继续引用原版本。」
  - 代码：TF `application/form_actual_fulfillment_on_participation_end.go` 把 `ChargeOccurrenceAgreementTerminated` 判为 `FormationSettled`，信封入账，此后成本不再形成。
- **归类**：B。0233 的决定三、决定五与它自己的越权风险点互相抵触，与 ADR-0204 和 PC CONTEXT 也有张力，没有走部分停用，也没有立裁定。
- **严重度**：中。运营违反这条约束时，在途委托的供应商成本会静默缺失，系统不留痕。
- **建议修法**：新 ADR（可与票 88 合并）裁定：在途委托下按委托快照登记的发生项，算「既有引用」还是「新决定」。
  - 算既有引用：TF 协议核对改走不折收尾的口，立代码票。
  - 算新决定：PC CONTEXT 生命周期两句加例外，并部分停用 0233 决定三「既有委托…继续引用它」半句的适用场景。

### B3·1 等登记的格能不能先入账、续办触发以后再补：各上下文给了相反的答案（B）

来源：ADR 互相矛盾 · 作业簇

- **类别**：3。
- **位置**：`docs/adr/0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md`
  - 决定四原句：「**四、第三格必须与它的续办触发同笔落地，否则不许落地。**」
  - 理由：「只把回滚改成提交而不给触发，结果是把 `ABANDONED` 换成一个**更安静的永久停滞**」。
  - 续办方式：「不采用定时扫描重驱，理由与 ADR-0086 否决轮询那条相同」。
  - 0094 的索引注记只写了分格判据的适用场景：「**适用场景**：接受判断链的未决处置」。决定四有没有越出小包托运，没有任何一处写明。
- **证据**：同一种恢复动作（等运营或租户登记），各份记录的处置如下。

  **入账，同笔不给续办触发：**
  - `0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md`（NR 路由复核）。
    - 决定一：「重投推不动的答`未判`（`ReassessTriggerNotJudged`）：没有路由历史、证据未配置、冻结形态未配置」。
    - Links：「续办触发同笔落地（决定四）本记录未做，见越权风险点 1」。
    - 越权风险点 1 自己承认相抵：「owner 若坚持 ADR-0094 决定四「续办触发同笔落地，否则不许落地」，撤回的是决定一的`未判`一格」。
    - 0235 没有部分停用 0094，0094 的 Status 行也没有指向 0235 的指针。
  - `0201-…-preparation-answers-not-configured-on-an-empty-register.md`（TF 运输准备）。
    - 决定六：「准备结果记`未配置`、确认消费、不重投（ADR-0094 判据；X6）」。
    - 越权风险点 4：「`未配置`结果不自动重备」。
  - `0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md`（TF 实际发生项）越权风险点 6：「未配置、不计费、协议否定格、缺采购上下文都入账；补齐之后没有东西回头重跑」。
  - `0216-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope-per-object.md`：「要求缺失也入账：所有者答「没有」，重投也不会变成「有」」，只靠人工补拍。
    - 后来 `0230-…` 加了待形成册与重问，又计划一个按间隔自动去问的驱动：「按间隔自动去问的驱动不在本记录，另票」（票 152）。
    - 这个驱动正是 0094 否掉的定时扫描。`0229-…` 在 SA、PP 一侧照 0094 否过一次：「定时扫描停着的请求重驱。** ADR-0094 决定四否决过」。

  **把决定四读成通则：**
  - `0132-…` 原句：「ADR-0094 决定四写死「只把回滚改成提交而不给触发是被禁止的」，一格没有触发就不许开」，并据此不开`等待资金到位`一格。
    - 这句引文在 0094 里不存在（`grep -F` 零命中），是转述冒充原句。
  - `0229-…` 把它当先例给 SA、PP 补了续办：「先例：ADR-0094 决定四在小包托运那一侧立过「参数已登记」」。

- **代码**：
  - NR 按 0235 落了：`internal/networkrouting/adapters/postgres/route_reassessment.go` `RecordNotJudged`，迁移 `network_routing/0021`，没有续办。
  - PS 按 0094 落了：`internal/partycommercial/adapters/postgres/operator_registration_completed_handoff.go` 交「参数已登记」，PS `operator_registration_queue.go`。
- **归类**：B。各份都已接受，在「等登记的格可否先入账、续办另票」这一件事上答案相反，没有一份记录裁过决定四是只管小包托运接受判断链，还是全产品通则。
- **严重度**：中。票 154（NR 未判续办）、155（初始路由未配置）以及以后每一个新的「未配置」格，作者都会面对两份相反的先例。今天 NR 未判的触发补齐配置后不会再判，读 0094 的人会以为「等运营登记」的格一律有续办。
- **建议修法**：立一份新 ADR，裁决定四的适用场景。
  - 选甲：限小包托运接受判断链。这时在 0094 的 Status 行写明适用场景，把 0235 越权风险点 1 的措辞改为指向新 ADR。
  - 选乙：定为通则。这时由新 ADR 部分停用 0094 决定四，适用场景写成「NR 路由复核、TF 运输准备、TF 实际发生项、TF 派送待形成在续办票落地之前，允许先入账并留原因」，并把 0230 计划的定时驱动与「不采用定时扫描」的冲突一并裁掉。
  - 两种都要顺手把 0132 那句伪引文改成 0094 的原句。

### B3·2 合同版继承的产品底座在索赔时现选，不随接受时的闭包冻结（B，fork 核出）

来源：ADR 互相矛盾 · 作业簇

- **类别**：3。
- **位置**：`0136-claim-rule-resolution-key-is-the-acceptance-time-commercial-resolution-reference.md`
  - 决定一：「适用规则版本，是该索赔项目标包裹所属委托被接受时固定的商业依据里已采用的客户服务规则版本」；
  - 决定二：「VE 不再走 PC 第一阶段解析」，以及「读一份已固定的闭包没有`适用冲突`」。
- **证据**：`0176-customer-service-rule-contract-tier-selection-and-inheritance.md`
  - 决定二：「底座版在层次读口内选「同范围挂服务产品、锚点生效」的版本，多候选照`适用冲突`纪律答」。也就是说，合同版没写的那几行取自一份不在接受时闭包里、读的时候才选出来的版本，而且这一选可能答`适用冲突`。
  - 0176 的 Consequences 又写「已接受的委托按接受时固定的版本读取，不受后续发布影响」。
  - 0176 全文不提 0136，0136 也没有指向 0176 的指针（两边 `grep -F` 都是 0 次）。
  - 代码与 0176 一致，并自认会漂：`internal/visibilityexception/adapters/partycommercial/claim_service_rules.go` 的注释写「底座不随闭包冻结，按锚点读时选；有效性更正改了底座区间时这里会改口」。同一文件 `adoptedRuleVersion` 的头注却照抄 0136 决定二那句，代码内部两说并存。
  - 能改动底座区间的写口是现成的：PC `close_commercial_version.go`（ADR-0233）与 `validity_correction.go`。
- **归类**：B。
- **严重度**：中，本簇里最接近「高」的一条。包裹接受之后，底座版若被更正或退役，客户合同继承来的索赔期限与最低材料会在事后改口，正是 0136 要防的「客户拿到一份下单时不存在的规则」。
- **建议修法**：立新 ADR 二选一。
  - 甲：接受时把产品底座版本作为闭包成员一并冻结。
  - 乙：认读时选，部分停用 0136 决定一（限合同版继承来的行）与决定二那句，并订正 0176 Consequences 里的「不受后续发布影响」。

### B3·3 0176 决定四按「查询携带的合同」点核，与 0136、代码相反，还丢了 0104 决定四的产品那一半（B，fork 核出）

来源：ADR 互相矛盾 · 作业簇

- **类别**：3。
- **位置**：`0176-…` 决定四：「VE 适配器点读后核「正文挂合同必须等于查询携带的」（折行续「合同」），以及「挂产品的版本不核」。
- **证据**：
  - `0136-…` 决定二说 `EligibilityQuery.Contract`「本记录不动它、不拿它去 PC 过滤」。越权风险点写「`EligibilityQuery.Contract` 与闭包采用的合同版本不核一致」。
  - `0104-customer-service-rule-content-is-owned-by-party-commercial-and-first-ships-two-items.md` 决定四要求「消费方点读之后核「这一版挂的是不是我手上这份产品 / 合同」」。0176 只停用过 0104 的一个前提，没有碰决定四。
  - 代码 `tiedToAdoptedContract(rule, adoption.contract)` 比的是闭包采用的合同，不是查询携带的合同。
- **归类**：B。代码跟的是 0136。
- **严重度**：中。照 0176 的字面实现，会把核对对象换成调用方可控的一格。
- **建议修法**：
  - 给 0176 的 Status 行加勘误，把「查询携带的合同」改读为「闭包采用的合同」（形照 0078 Status 行的「勘误」）。
  - 由 0176 部分停用 0104 决定四的「产品」那一半，适用场景限产品版本（由解析唯一性保证）。
  - 可与第 2 条并进同一份新 ADR。

### B3·6 交接转入立起的控制，收寄不认，同一节点会另起第二个控制起点（B，fork 核出）

来源：ADR 互相矛盾 · 作业簇

- **类别**：3。
- **位置**：`0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md`
  - 决定四：「**四、同一作业实物在同一节点不另起控制起点（第四期）。**」；
  - 正文：「有当前收寄时，同一实物在同一节点的任一交付观察都并入它」，当前收寄的定义带「且那一版的控制没有转出」。
- **证据**：`0214-node-control-transfers-out-and-in-on-an-authoritative-transport-handover.md`
  - 文末补记：「否则在接收节点立 HANDOVER_IN 控制」；
  - 越权风险点 17「收寄不认交接转入立起的控制」：「收寄编排只看收寄链，会再立一份」。
  - 两份都已接受，谁也没停用谁。代码与 0214 的描述一致：收寄只认 `Receptions.FindCurrentJudgmentsAtNode`，看不到转移册（票 `demo-chain-gaps/92` 现象节）。
- **归类**：B。
- **严重度**：中。读 0199 的人会以为同一节点只有一个控制起点，实际在转入之后会有两份。
- **建议修法**：随票 92 立新 ADR。二选一：把「当前收寄」改成「当前在身控制」，或明写 HANDOVER_IN 不受这一条约束。同时部分停用 0199 决定四里「当前收寄」那句定义，适用场景限交接转入立起的控制。

### B3·8 代决复核 R11 改了设备答复的逐对象范围，却写「正文与 Decision 不变」，又与 0225 的 `members` 相抵（B）

来源：ADR 互相矛盾 · 作业簇

- **类别**：3。
- **位置**：
  - `0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md` 决定一：「执行方要知道车上被挡的是哪一件，所以逐对象那一格照给」。
  - `0219-a-device-previews-the-departure-gate-before-departing-and-reads-what-the-departure-will-record.md` 决定一：「`gateReading` 与移动事实口是同一个线格式：整格、起讫、停点、逐对象那一格」。
- **证据**：
  - 两份同日（2026-10-20）的复核记录都开头写「正文与 Decision 不变」，下面却裁了「是」。
    - 0205 复核记录：「出发口回给设备的逐对象那一格收窄成只列被阻断或未决的对象，落册仍记全」。
    - 0219 复核记录：「出发前门禁问询与出发两口回给设备的逐对象那一格，趁一线客户端未接入一起收窄成只列被阻断或未决的对象」。
    - 两处都没写停用将由哪份 ADR 记。同期的 R9、R10、R12 都写明了停用随某票的 ADR 记。
  - `0225-the-departure-gate-reading-asks-through-consolidation-unit-members-at-the-moment-of-departure.md` 决定六（同日接受）：「回给报事实一方的逐对象那一格，在单元上带 `members`，列出它在同一份读数里各有一格的直接成员」。照 R11 收窄之后，放行的成员不在答复里，单元的 `members` 就指向答复里没有的格。
  - 代码今天照 Decision 全列：`internal/transportfulfillment/adapters/http/record_movement_fact.go` `departureGateReadingViewOf` 逐条 `Findings()` 全写。票 146 是 `ready-for-agent`。
- **归类**：B。两条已接受的话相反。R11 是更新的裁定，但没有停用落点，而且它和 0225 决定六之间还缺一个裁决：收窄之后 `members` 列全部成员，还是只列答复里的那几件。
- **严重度**：中。做票 146 的人照 0225 或照 R11，会写出两种线格式。
- **建议修法**：随票 146 立 ADR，部分停用 0205 决定一、0219 决定一各一句与 0225 决定六的 `members` 语义（适用场景限回给设备的答复，落册不变），把两份复核记录里「正文与 Decision 不变」改成指向它。

### GL·G-05 「对账周期」归谁两处说法相反：GLOSSARY 与 PC 说它是结算政策的一项，ADR-0158 与 SA 说它属对账安排；截单快照的「结算账户的时区和截单时刻」在账户上也没有

来源：GLOSSARY ↔ CONTEXT

- **类别**：目标 6、目标 5（同一项归属不同），兼目标 3（两份已接受记录相抵，谁也没停用谁）。
- **位置**：
  - `docs/domain/GLOSSARY.md`「结算政策」：「商业协议对结算币种、收付方向、预付或账期方式、对账周期、付款条件、信用约束、税务处理依据和允许归集范围作出的版本化约定。」PC CONTEXT 同名词条同句。
  - `docs/domain/GLOSSARY.md`「截单快照」：「在结算账户的明确时区和截单时刻，对尚未出账且已经具备结算资格的费用范围形成的不可覆盖集合。」SA Language 同句。
- **证据**：
  - `docs/domain/settlement-accounting/CONTEXT.md`：「对账周期、业务时区、截单时刻属于对账安排，付款条件属于商业结算政策。结算账户登记册不另存一份。」
  - ADR-0158（已接受）决定二：「对账周期、业务时区、截单时刻、付款条件不进这一行：前三格是对账安排，付款条件是商业结算政策，再存一份就有了第二权威。」
  - 「对账安排」的拥有方，任何 CONTEXT 与 GLOSSARY 都没写。
  - 代码两头都不完整：
    - PC 的 `SettlementPolicy`（`internal/partycommercial/domain/settlement_policy.go`）只有结算方式与适用范围，没有对账周期。
    - SA 结算账户行不存这三格（`internal/settlementaccounting/domain/settlement_account.go` 注释）。
    - 截单编排的 `Period` 与 `CutOffAt` 由调用方带进命令（`cut_off_publish_statement.go`）。
    - 时区与截单分钟登在按费用项目的归属登记上（`ChargeAttributionRegistration`）。
- **归类**：B。GLOSSARY 与 PC CONTEXT 是一份记录，ADR-0158 与 SA CONTEXT 是另一份，各执一词，代码两头都不完全对。
- **严重度**：中。要实现对账周期、截单的作者，不知道这几格该登在 PC 结算政策、SA 某本对账安排册，还是费用项目上。
- **建议修法**：立 ADR 代决「对账安排」归谁、登在哪本册（周期、业务时区、截单时刻），并写越权风险点。随后统一三处：GLOSSARY「结算政策」删去或保留「对账周期」，「截单快照」的「结算账户的」改成实际挂载对象，PC「结算政策」照改。

### CM·M-2 「结算是外部资金事实进入本产品的唯一采用口」与代收与清分直接接受「运营企业真实到账」相抵（两份已接受记录，互不停用）

来源：CONTEXT-MAP

- **类别**：目标 3（两份已接受 ADR 矛盾，彼此没有停用关系），兼目标 1。
- **位置**：`CONTEXT-MAP.md` 的 `settlement-accounting → customs-compliance`：「结算是外部资金事实进入本产品的唯一采用口」。同一文件的 `parcel-shipment / transport-fulfillment → collection-remittance`：「代收与清分关联适用证据及银行或支付事实后形成代收本金的应收、已收、在途、清分、应付客户、短款、溢款和汇付分户账结果」。
- **证据**：
  - ADR-0137 决定四（2026-09-10）是全称句：「外部资金事实进入本产品只有 `settlement-accounting` 采用这一口」。
  - ADR-0082（Date: 2026-08-28）、CR CONTEXT 与 GLOSSARY 都让 CR 自己接受真实到账：
    - CR CONTEXT「代收事实」列出四层：「收件人付款（来自 `transport-fulfillment` 的履约侧代收证据）、渠道代收报告、渠道回款通知、运营企业真实到账」；Boundaries 写「本上下文只登记汇付主张与已接受的资金事实引用」。
    - GLOSSARY「代收货款」写「银行或支付系统拥有运营企业真实到账事实；`collection-remittance` 关联这些事实形成代收分户账结果」。
  - 代码：`internal/collectionremittance/domain/collection_fact.go` 的「OperatorBankCredit 是运营企业真实到账。只有它支撑本金推进到`应付客户`。」。这一层直接带证据引用登进 CR，不经 SA 的采用信封；MAP 图中也没有 SA→CR 的资金事实边。
  - SA CONTEXT 写入账余额时有「COD 代收本金不进」。这暗示 COD 的到账也会经过 SA 的采用，但没有任何地方写它怎样到达 CR。
  - ADR-0137 没有部分停用 ADR-0082，也没有限定适用场景。
- **归类**：B（需立 ADR 裁）。
- **严重度**：中。COD 首发排除，眼下不阻断；但作者照 MAP 会在两处各开一个铸造点，或者把 CR 的真实到账层堵死。
- **建议修法**：立 ADR 二选一，并在 ADR-0137 的 Status 行补注记。
  - (a) 把 ADR-0137 决定四的适用场景限定为税费付款核对与 SA 自身的收付映射，CR 的真实到账层另成一口。
  - (b) CR 的真实到账层改为接收 SA 采用信封的引用，MAP 补 SA→CR 一条资金事实交接。

### SC·S-04 场景二：受控关闭与重开保留「合同委派可直接决定」的例外，与 PC CONTEXT 和代码相反

来源：SCENARIOS

- **类别**：目标 1（两份 CONTEXT 各执一词，场景跟着 PS 一侧走）
- **位置**：`docs/domain/SCENARIOS.md` 场景二：
  - 过程第 10 步：「除非客户合同或明确授权已经把请求方委派为可直接决定的角色，否则仍须由运营企业责任法人授权角色形成正式决定」
  - 过程第 16 步：「渠道服务方或代理商提出的恢复请求仍只是一项请求，除非其已被合同明确委派为决定角色」
  - 预期结果：「货主或渠道请求不会在缺少适用合同委派时直接改变包裹级继续尝试判断」
- **证据**：
  - 赞成例外：`docs/domain/parcel-shipment/CONTEXT.md`：「货主、渠道服务方和代理商只能提出请求，除非客户合同或明确授权把其纳入可直接形成决定的授权角色范围」
  - 反对例外：`docs/domain/party-commercial/CONTEXT.md`：「两者都是运营侧凭授权规则形成的决定，合同委派不参与。」
  - 裁定只落在票里，没有 ADR：`.scratch/party-commercial-context-gaps/issues/13-authorized-action-lacks-controlled-closure-and-reopening.md`：「- **③ 「货主经合同或明确授权直接形成决定」例外支 → 首发不开、留位**」。「首发不开、留位」比 PC CONTEXT 的「合同委派不参与」弱：前者是首发暂不开放，后者是规则上不存在。
  - 代码：`internal/partycommercial/domain/authority_grant.go` 头注「人工复核、主动拒绝、受控关闭与重开是运营侧凭授权规则自己作的决定，委派不参与。」，`formedOnlyByOperatorRole` 对这两种动作返回真，客户账户请求方答 `ErrCustomerAccountCannotDecide`。
- **归类**：B（PS CONTEXT 与 PC CONTEXT 两份已接受记录相矛盾，谁也没停用谁；裁定只在票里）
- **严重度**：中。读者看不出货主的直接请求能不能直接形成关闭或重开决定。
- **建议修法**：立 ADR 二选一：例外长期存在但首发不开（PC CONTEXT 改成「首发不开」并写明留位），或者例外不存在（PS CONTEXT 删掉这个例外）。SCENARIOS 那三处随裁定改；裁定之前至少给这三处标上「首发不开」。

### SC·S-05 场景二：渠道服务方或代理商的叫停写成「请求」，ADR-0183 复核写成外部限制

来源：SCENARIOS

- **类别**：目标 1
- **位置**：`docs/domain/SCENARIOS.md` 场景二·过程第 10 步：「货主、渠道服务方或代理商要求停止或恢复尝试时，`parcel-shipment` 先保存请求方、请求原因和依据；请求本身不改变包裹级继续尝试判断」；第 16 步：「渠道服务方或代理商提出的恢复请求仍只是一项请求」
- **证据**：
  - `docs/adr/0183-operator-channel-addresses-controlled-closure-and-reopening-by-parcel-and-derives-the-shipper-from-the-accepted-request.md` 决定：「操作者渠道上不开「渠道服务方或代理商作请求方」的路。」；owner 复核记录（代决）：「承运方或渠道方叫停是外部限制（`EXTERNAL-RESTRICTION`），不需要请求方。」
  - `docs/adr/0187-controlled-closure-and-reopening-record-request-and-operation-evidence-on-the-decision.md`：「**一、请求证据落在决定上，不另立请求册。**」；Alternatives：「**另立请求册**（候选丁）：未授权的请求、渠道方或代理商的请求要不要先留痕，归 PS owner；本记录不立。」
  - 场景和 PS CONTEXT（S-04 引的那一句）把渠道方的叫停当作有请求方、先留痕的请求；ADR-0183 复核把它归为外部限制。两种归法下重开的路不一样：外部限制要先由来源责任方解除（场景二第 17 步），请求则只需授权角色重开。
- **归类**：B（PS CONTEXT 与场景是一套说法，ADR-0183 复核记录是另一套，没有停用关系）
- **严重度**：中
- **建议修法**：与 S-04 同一份 ADR 裁：渠道服务方或代理商叫停时，是作为请求留痕（是否另立请求册，ADR-0187 交给了 PS owner），还是一律走 `EXTERNAL-RESTRICTION`。裁完改 PS CONTEXT 与场景二第 10、16 步。

### SC·S-08 场景十二：面单申请时形成预估费用并冻结，现行机制里没有这一路

来源：SCENARIOS

- **类别**：目标 1
- **位置**：`docs/domain/SCENARIOS.md` 场景十二·前提：「客户申请一笔面单交易，初始资料形成预估费用 50 元」；过程第 2 步：「`settlement-accounting` 采用该评价形成 50 元预估费用和完整金额展开，并在该结算账户冻结 50 元」
- **证据**：
  - 支持场景的一侧：`docs/domain/settlement-accounting/CONTEXT.md`：「面单或订单预估费用可以按合同与信用规则形成资金冻结。」
  - 相反的一侧：
    - 同一文件：「发起 SELL 评价有两条路：按发生项原因另册登记；或按包裹计费时点，由结算账户采用内置时点之一（「有效网络收寄已采用」「有效交付已终局」）」以及「两册上都没有，不发起。」面单申请不在这两条路里。
    - `docs/domain/parcel-pricing/CONTEXT.md`：受理前冻结来自估价——「`settlement-accounting` 可以把针对委托当前提交版本、逐件试算对象的评价合计用作冻结与信用暴露的控制金额」，并且「估价不是费用、预估费用或应收应付，费用照旧要另有一次针对已受理包裹的评价」。
    - `docs/adr/0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md` 越权风险点：「面单渠道服务包裹：责任起点是面单交易，终局是面单服务终局，两种时点都不会触发。」
  - 场景写的是「面单申请 → 一份 SELL 评价 → 预估费用 → 冻结」这一个时序；现行规则把它拆成了受理前估价冻结和包裹计费时点预估两段，而且面单申请这个时点两段都不发起。
- **归类**：B（SA CONTEXT 自己的两句话在这里方向相反；ADR-0190 把面单渠道服务的计费列为越权风险点，没有裁定）
- **严重度**：中
- **建议修法**：立 ADR 裁：面单申请（或面单交易受理）是否算第三种 SELL 发起时点，以及冻结跟着谁走。如果不算，场景十二改写为「受理前估价冻结 → 计费时点形成预估客户费用 → 复秤后重新评价」，SA CONTEXT 那句「面单或订单预估费用可以……形成资金冻结」也一并收窄。与 S-03 同属场景十二，可以一张票修。

### PB·M1 开发主线说 Bento 持久化闸门「已解除」，技术切片权威、ADR-0017/0026 和 AGENTS.md 说它仍阻断

来源：产品基线诸件

- **类别**：目标 1
- **位置**：开发主线「机制半边现状」：「持久化闸门已解除、投递侧四十六口全部换真」；横切缺口那一条：「Bento 闸门解除（`v0.1.0-rc.2` 经 `PBC-01`/`PBC-06` 锁定）后」「未落库的端口按批次推进，不再有闸门阻断」；「参数显式未配置」那段：「后两个随持久化闸门解除进来」
- **证据**：
  - `docs/design/parcel-go-first-consumer-slice-decision-brief.md` 闸门表的 Bento 行：「登记前不得宣称闸门已通过」「闸门本身未通过」；同一行的「仍禁止范围」一格：「宣称闸门已通过或据此改写开发主线的横切缺口状态」。
  - ADR-0017（Accepted，无停用）：「PostgreSQL Repository 实现、迁移、事务与 Outbox 仍然阻断」。ADR-0026 只放行「用途限于产出消费者证明」的持久化实现。
  - AGENTS.md「当前默认切片」：「租户取值与 Bento 持久化闸门仍阻断」。
  - 代码站在开发主线一边：`internal/*/adapters/postgres` 进了生产装配，已不只是取证用途。
- **归类**：B。两份已接受的权威记录对同一道闸门的现状说法相反，谁也没停用谁；代码与「仍阻断」一方不一致。
- **严重度**：中
- **建议修法**：立 ADR 代决闸门现状。或者宣布 Parcel 侧生产使用持久化获准（B-06 双消费者绑定另计），据此改简报闸门表和 AGENTS.md；或者把开发主线的「已解除」改回「取证放行、闸门未过」，并说明生产装配为何已在用。

### PL·F6 证据存储写死为「公司自建 MinIO」，与租户取值模型相抵

来源：试点三件

- **类别**：目标 3（两份已接受记录之间的矛盾，没有停用关系）
- **位置**：
  - 登记册 `PAR-GOV-11` 行：
    > | PAR-GOV-11 | 证据存储与访问 | 采用自建 MinIO S3 兼容对象存储，逻辑命名空间为 `idp-parcel/pilot` |
  - 登记册「权威证据存储」一节：
    > 正式试点证据和脱敏映射采用自建 MinIO S3 兼容对象存储
    
    同节又写：
    > 当前没有可复用的企业 S3 兼容对象存储平台；MinIO 的建设和运维由基础设施责任方负责
  - 矩阵：
    > 正式验收的证据位置必须指向 `PAR-GOV-11` 登记的自建 MinIO S3、`idp-parcel/pilot` 逻辑命名空间及稳定对象版本
- **证据**：
  - 两份文档都照 ADR-0008：
    > 为 `idp-parcel` 建设公司自建的 MinIO S3 兼容对象存储
    
    以及：
    > 当前公司没有可复用的企业级对象存储平台。
    
    这是「开发方即运营方」时期的写法。
  - 与之相抵的有三处。AGENTS.md：
    > **本仓开发的是卖给物流企业的软件产品，开发方不是运营企业。**
    
    ADR-0146 决定二（连接器形态归产品策略，具体实例归租户取值）。登记册自己的拆分注：
    > S3 兼容证据存储的连接器形态与命名空间约定归产品策略 → [票 13][psb-13]（待核）；平台实例引用、访问责任与生效时间仍是租户取值
  - ADR-0008 的 Status 只写 `Accepted`，没有任何停用指针。
- **归类**：B。ADR-0008（某个具体平台，由「公司」自建）与 ADR-0146、AGENTS 三类口径（平台实例是租户取值）真矛盾，谁也没停用谁。登记册与矩阵照的是 ADR-0008，所以不能只改文档。
- **严重度**：中。某租户用自己的 S3 兼容存储时，照矩阵这句它的证据就「不能成为权威证据源」，会误判正式验收能否成立。
- **建议修法**：立 ADR 代决，部分停用 ADR-0008 的「自建 MinIO」与「公司」两句：产品只定「S3 兼容对象存储连接器加命名空间约定」（票 product-strategy-boundary/13 第 2 项），平台选型与实例归 `PAR-GOV-11` 的租户取值。随后把登记册「已确认约束」与那一节、矩阵那句里的「自建 MinIO」改成「S3 兼容对象存储」。

---

### AG·M1 机制清点「各笔在自己那笔上重生成」，与实际做法「进 main 时统一做」相反

来源：docs/agents 与 AGENTS.md

- **类别**：1（规程单一权威）
- **位置**：
  - `docs/agents/parallel-sessions.md`「生成物不占号」：「收成两句：**各笔在自己那笔的干净检出上重生成、随笔提（或紧随一笔）；推送方在 tip 上重生成兑底。**」，以及「它们与机制清点差一处：**票不在自己的笔上重生成它们**」
  - 同文进 main 第 2 步写 ADR 索引与 AT 计数「口径同机制清点」，又和上一句「差一处」互相拉扯
  - `.github/workflows/ci.yml` 注释：「# ADR 索引与 AT 盘点计数也是生成物，但约定与上一步相反：票不在自己那笔上重生成，由推送方」
  - `.scratch/infra-speed/issues/08-…md`：「机制清点的约定是各笔在自己那笔上重生成，所以 PR 上跑它的 -check 是对的。」
- **证据**：实际做法是票不重生成机制清点。
  - `.scratch/demo-chain-gaps/issues/25-…md`：「5. **机制清点没重生成**（建议进 main 前）——不采纳：协调方派单明令「不要重生成机制清点（进 main 时统一做）」」
  - `.scratch/infra-speed/issues/02-…md`：「协调方明令「不要重生成，进 main 时统一做」」
  - `.scratch/demo-chain-gaps/issues/31-…md`：「编排指令本分支不重生成机制清点」
  - 另有十余张票写「机制清点未重生成（进 main 时统一做）」。评审照文档把它提成「建议进 main 前修」，作者照派单拒收。同一步骤两套口径，每张票都要争一次。
- **归类**：B（文档规则、CI 注释和派单实践各执一词，要裁哪一边算现行）
- **严重度**：中（评审照文档提、作者照派单拒，每张票都争一次；照文档在分支上重生成，同批的票又会撞同一段）
- **建议修法**：如果按实践裁，就把机制清点并进 ADR 索引与 AT 计数那一类（票不重生成，推送方在 tip 上做），「生成物不占号」改成一条规则，ci.yml 那句「约定与上一步相反」改成「同上一步」。如果按文档裁，就要撤掉派单里的那条明令。

### AG·M2 `/code-review` 的 Spec 轴对照什么：workflow.md 说对照 UC、不对照工单，parallel-sessions 说对照票面

来源：docs/agents 与 AGENTS.md

- **类别**：1（规程单一权威）
- **位置**：`docs/agents/workflow.md`：「**`/code-review` 的 Spec 轴**对照的是 `UC-*` 用例文档，不是工单描述。」（2026-08-07）
- **证据**：`docs/agents/parallel-sessions.md`「合入前独立评审（2026-09-08 起，用户裁定）」：「`/code-review <基线 SHA>`，spec 指到票面。两轴照 skill：**Standards**（AGENTS.md 红线、该上下文 CONTEXT/词表、」接下一行「所引 ADR 的决定）与 **Spec**（票面完成判据、裁决节、验收对照）。」实际评审记录多数写「Spec 轴（对照票面裁决四条）」「Spec（三条裁决、完成判据 1–7）」，例如 `.scratch/demo-intake-admission-paused/issues/01-…md`、`.scratch/party-commercial-context-gaps/issues/13-…md`。
- **归类**：B。两份 agents 规程正面相抵；后者出自用户 09-08 裁定的评审门，但 UC 优先于票面的道理（AGENTS.md「冲突时」那段）也成立，要裁成一句话。
- **严重度**：中（评审照哪一份做，都会漏另一侧的判据）
- **建议修法**：统一写成「Spec 轴 = 票面完成判据与裁决 + 所引 UC 的输入/结果/失败边界；两者相抵时以 UC 为准，并记成阻断」，两处只留一处定义，另一处链接过去。

### AG·M6 「当前宿主」与多会话规程只描述 Cursor + idp-mcp 通道；10-16 起实际作业的 Claude Code 会话不在其中

来源：docs/agents 与 AGENTS.md

- **类别**：1（规程单一权威）
- **位置**：
  - `docs/agents/workflow.md`：「### 当前宿主：Windows 上的 Cursor + WSL 里的仓库」，正文说「agent 的 Shell 工具（Windows PowerShell 5.1）里 git、go、docker、pnpm 一律经 `wsl -d Ubuntu-24.04 -e bash -lc '<命令>'` 跑」。同节自定的规矩是「**换宿主时整节删掉下面的「当前宿主」，不要逐条判断**」。
  - `docs/agents/parallel-sessions.md`：「接收方那半（收到 `[点名]` 必须立刻按固定格式应答）写在 `.cursor/rules/idp-mcp-messenger.mdc`——」接「它 `alwaysApply`，每个会话都加载，应答纪律只有放在那里才轮得到每个通道。」以及度量节的「此后 tasks.md 每个会话节」末尾固定写五个数。
- **证据**：
  - 2026-10-16 起的票面记的作者是 Claude Code 会话与子代理，例如 `.scratch/product-strategy-boundary/issues/18-…md`「2026-10-21，Claude Code 作者子代理，分支 `claude-psb18`」；infra-speed/02–04 的「作者子代理，worktree `idp-parcel-is03`」。这些会话跑的是 WSL 里的 bash，不经 PowerShell。
  - `.cursor/rules/idp-mcp-messenger.mdc` 被 `.gitignore` 忽略（「/.cursor/rules/idp-mcp-messenger.mdc」），只在主树本地有，各 worktree 里没有，Claude Code 也不加载 `.cursor/rules`。
  - `.scratch/tasks.md` 最后一节是 2026-09-21，`list_sessions`、`broadcast`、`dispatch_task` 式的点名派单自那以后没有记录。
  - 宿主观察（非仓内）：2026-10-21 `~/.claude/skills` 下只有 `synced/`，没有 idp-skills。AGENTS.md 路由的 `/implement`、`/tdd`、双轴 `/code-review` 在 Claude Code 会话里能不能调到，文档没交代，见待核 Q2。
- **归类**：B（是把「当前宿主」改写成两种 agent 宿主并列，还是认定 Cursor 形态退场，要用户定；`parallel-sessions` 的点名、五数、messenger 规则是否仍是现行规程同此）
- **严重度**：中（Claude Code 会话照着做会去套 `wsl -d …`、找不到 messenger 规则、补一份没人维护的 tasks.md 五数）
- **建议修法**：「当前宿主」下分「Cursor 会话（PowerShell → wsl）」与「Claude Code 会话（WSL bash）」两小节，各写 shell、技能根与子代理。`parallel-sessions` 里依赖 idp-mcp 工具的节（点名、度量、`set_channel_lock`）标明「仅 Cursor 多通道形态」，或者随裁定退场。

### AG·M7 AGENTS.md「必要时再 `/code-review`」与「代码票进 main 前必须有一份非作者评审」口径不一

来源：docs/agents 与 AGENTS.md

- **类别**：1（规程单一权威）
- **位置**：`AGENTS.md`「本仓主路径」：「默认：边界清晰 → `/implement`（或 `/tdd`）→ 必要时再 `/code-review`。」
- **证据**：`docs/agents/parallel-sessions.md`（2026-09-08 用户裁定）：「**代码票进 main 前必须有一份非作者的评审报告**」。`docs/agents/workflow.md`「本仓允许子代理」把它当成通用门：「评审门回到[并行会话](./parallel-sessions.md)「合入前独立评审」的原样」。可这份门禁所在的文件，AGENTS.md 又说单会话不必读（见 H2）。所以单会话的代码票要不要非作者评审，两处答案不同。
- **归类**：B（09-08 的评审门是否及于单会话、及于不经推送方的落 main，要裁）
- **严重度**：中
- **建议修法**：裁定后在 AGENTS.md 写一句：「代码票落 main 前要一份非作者评审（门的定义见 parallel-sessions「合入前独立评审」；前端切片按 workflow.md 第 5 步）」，「必要时」改成只指作者自己额外的评审。

---

### X·X-02 部分停用的现行写法仍在制造 infra-speed/07 那类重复：2026-10-20/21 新接受的几次停用，被停方 Status 行照旧前段一份、索引注记一份

来源：ADR 停用链与引用规范（协调者自审）

- **类别**：目标 3
- **位置**：`docs/adr/README.md` 顶部规则：「一条记录只有个别条款被后续记录停用、整体仍然有效时，标为**部分停用**：改被停用记录的 `Status` 行与 Links 节各加一条前向指针；`Status` 行那一条写进它的索引注记」。规则没说前段不再写，作者于是两边都写。
- **证据**：本轮最新的几次停用全部两写（实测于 `6b0074d0`）：
  - ADR-0215 前段「……已由 [ADR-0235](./0235-…) 回答（2026-10-21）：没有路由历史、证据未配置、冻结形态未配置三格入账未判……」，注记「**部分停用**：其决定四「重投」一条里……已由 [ADR-0235](./0235-…) 停用」。
  - ADR-0202 前段「越权风险点 14 已由 [ADR-0239](./0239-…) 回答（2026-10-21）……由此**部分停用**文末补记「承运答复的次序与格」一格（被停原句见索引注记）」，注记再写一遍并给出被停条款。
  - ADR-0216 ← ADR-0230、ADR-0151 ← ADR-0230 / 0236、ADR-0149 ← ADR-0236 同此。
  - 接受出处同样两写：0222 至 0239 新立的每一份 ADR 都是前段「Accepted（2026-10-2x，……）」、注记「**已接受**（2026-10-2x，……）」各一份（共 55 份，见附表 B）。
- **归类**：A（规则文字不全，不是两份决定相抵）
- **严重度**：中。07 合并完之后，下一张带停用的票会把重复再造出来；两份措辞已出现事实不一致（见 X-05 的 0022、附表 B 的「只在一边的事实」列）。
- **建议修法**：07 合并的同一笔里，把 README 顶部那句改成「来向停用（本记录被谁停用）只写在索引注记，标记前段不写；前段只留状态词、接受出处与去向」，并在 `docs/agents/workflow.md` 里写到「进 main 步骤」旁；接受出处是否只留一处由 07 一并裁。可考虑让 `tools/adr-index` 在前段出现「已由 [ADR-…] … 停用」时报错，免得再靠纪律。

### X·X-03 Status 行之外还有第三种停用载体：头部的「Partially suspended by:」「Amended by:」行；其中 ADR-0012 ← ADR-0013 的限定只在这一行，索引里看不到

来源：ADR 停用链与引用规范（协调者自审）

- **类别**：目标 3
- **位置**：
  - `docs/adr/0012-parcel-pricing-context-within-idp-parcel.md` 头部：「Amended by: [ADR-0013](./0013-pricing-owns-versioned-external-reference-series.md)（限定下文 Decision 中「它只消费其他上下文提供的事实快照」一句；本记录正文不改写，边界划分不变）」；其 `Status` 行只有「Status: Accepted」，没有索引注记，Links 也没有 ADR-0013。
  - `docs/adr/0015-closed-grammar-open-vocabulary-for-multi-carrier-rating.md` 头部「Amended by: [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md) 的 Decision（限定下文 Decision 中「词汇按证据增长」一段末尾「并按 ADR-0014 形成新的规范化版本」这半句……）」，`Status` 行同样没有注记。
  - `docs/adr/0014-…`、`0048-…`、`0126-…`、`0184-…` 头部各有一行「Partially suspended by:」，与各自索引注记里的部分停用是同一件事的第三份写法（0014 的四次停用、0048 ← 0179、0126 ← 0178、0184 ← 0185 / 0195 / 0212）。
- **证据**：README 顶部「`Status` 行那一条写进它的索引注记……本节该行由它生成」；`tools/adr-index` 只读 `Status` 行，不读头部行。ADR-0013 正文：「本记录只限定该句的适用范围，不取代 ADR-0012，也不改变它划定的边界。」——这是一次部分停用，索引上 ADR-0012 那一行看不到。
- **归类**：A
- **严重度**：中（0012 那一条：按索引查「计价只消费事实快照」是否仍成立的作者会读漏）；其余四份是重复，低。
- **建议修法**：0012 的索引注记补「**部分停用**：Decision 中「它只消费其他上下文提供的事实快照」一句的适用范围已由 ADR-0013 限定……其余各条不变」，Links 补 ADR-0013；0015 的「Amended by」若判为部分停用同此，若判为释义可保留头部行。四份「Partially suspended by:」头部行并进 07 一起合，README 写明头部行不再新增。

### X·X-11 ADR-0149 决定四把 ADR-0055 决定五「仍然拦着」的两项未决按口解了，ADR-0055 的 Status 行与 Links 没有前向指针

来源：ADR 停用链与引用规范（协调者自审）

- **类别**：目标 3
- **位置**：`docs/adr/0055-business-endpoint-intake-has-an-unconfigured-grade.md` 决定五：「**五、本记录不解锁 Intake 的另两项未决。** 载荷规范化摘要与准入范围装配（`PAR-GOV-03..07`）仍然拦着真渠道 Intake」。其 `Status` 行只有「**数目勘误**」一条，没有指向 ADR-0149（也没有指向按客户面各口延用它的 ADR-0139 决定八）。
- **证据**：`docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md` 决定四：「**四、ADR-0055 决定五两项未决按口逐一解。**」，其 Links：「[ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)：决定五两项未决，本记录给出按口解法」。产品基线组 PB·M3 报的「HTTP 接入面仍被写成『被 ADR-0055 决定五两项拦着』」就是读 0055 读出来的旧口径。
- **归类**：A
- **严重度**：中（作者照 0055 会以为真渠道 Intake 仍被两项未决整体拦住）
- **建议修法**：0055 索引注记加一条：决定五「仍然拦着真渠道 Intake」已由 ADR-0149 决定四按口解（作业事实、外部结果两族），客户面各口由 ADR-0139 决定八照办；其余各条不变。Links 补两条。

### B2·B2-07 ADR-0080 改了闭包键上结算选择器的齐备判据，ADR-0044 决定二没有指针

来源：ADR 互相矛盾 · 商业与资金簇

- **类别**：3
- **位置**：`docs/adr/0044-settlement-basis-adopts-via-settlement-policy.md` 决定二：「**二、键上新增 `SettlementSelector`（相对方、合同版本、费用范围、币种）。** 纪律与 `PriceDirection` 相同：请求结算依据必填，其余请求必缺，部分给出即`输入未受理`。」Status 行只有 `Accepted`。
- **证据**：
  - ADR-0080 决定一：闭包键上「带上合同维则整份键**不受理**」，同时「单依据键（`ResolutionKey`）仍要四维，判据不变」。
  - 代码：`internal/partycommercial/domain/settlement_policy.go` 的 `SettlementSelector.declaredWithoutContract`，由 `reference_closure.go` 调用。
  - 已知停用清单里没有 `0044<-0080`。
- **归类**：A。代码与 0080 一致。
- **严重度**：中。照 0044 构造闭包键的作者会带上合同维，结果被整份拒收。
- **建议修法**：在 0044 的 Status、Links 与索引注记加部分停用：决定二「部分给出即`输入未受理`」一句在闭包解析键上由 0080 决定一停用。适用场景：只限闭包键（合同维由闭包解出，见 0080 决定二）；单依据键不变；其余各条不变。

### B2·B2-09 ADR-0081 决定六「四格留 nil」里有三格已被后来的记录换掉，只挂了 0158 一条，索引注记反而写「其余各格仍有效」

来源：ADR 互相矛盾 · 商业与资金簇

- **类别**：3
- **位置**：`docs/adr/0081-acceptance-judgment-is-envelope-driven.md`
  - 决定六：「**六、入站半边的实例半边一格不填。** 时点取值源、可达性闭包标识、结算账户目录、控制金额源全部留 nil」。
  - 索引注记：「决定六其余各格仍有效」。
- **证据**：
  - 可达性闭包标识：ADR-0156 Consequences 写「生产装配不再传入 nil 身份映射」；`cmd/parcel-dispatch/assemble.go` 里装了 `nrpartycommercial.NewCommercialEligibility(resolutions)`。
  - 控制金额源：ADR-0179 决定一写「parcel-shipment 发起控制时不再交金额」，Consequences 写「`ControlAmountSource`、`EstimationAmountSource`、`EstimationBasisSource` 随实施退场」；dispatch 装了 `sapostgres.NewPreAcceptanceEstimateAdoptions`。
  - 时点取值源：dispatch 的 `acceptanceCommercialBasis` 接了 `pspartycommercial.NewSubmissionReceiptAsOf`，注释写「时点取值源接「提交接收」」，对应 ADR-0157。
- **归类**：A。
- **严重度**：中。索引注记把三条已经不成立的装配状态说成现行。
- **建议修法**：在 0081 的 Status 与索引注记补三条部分停用指针，分别指向 0156、0179、0157，并删掉「其余各格仍有效」。若 0157 不认作那一格的停用方，就在 Status 行注明那一格已由「提交接收」执行器接上，按符号名定位。

---

### B3·4 0146 把候选生成、过滤、排序改判为产品策略，0052、0053、0068 仍写它们属租户取值，没挂指针（A）

来源：ADR 互相矛盾 · 作业簇

- **类别**：3。
- **位置**：
  - `0052-network-evidence-catalogue-has-an-unconfigured-grade.md` 决定五：「逐候选事实的生成、过滤与排序规则仍属 `PAR-NET-14`，本记录一条都不定」。
  - `0053-network-fact-families-are-derived-not-registrable.md` 决定二：「事实族由解析层从原语派生，解析层属 `PAR-NET-14`」。
  - `0068-versioned-network-catalog-structure-precedes-rule-content.md`：
    - 决定三：「路由策略的规则正文（过滤、排序、冻结边界、改善阈值、改路条件与权限）全属 `PAR-NET-14`」；
    - 决定六：「把目录折成逐候选事实的解析层属 `PAR-NET-14`」。0148 停用的只是这一条的另两句，这半句没在停用范围里。
- **证据**：
  - `0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md` 决定七：「`PAR-NET-14` 拆成产品策略（候选生成、硬约束过滤、排序策略族、冻结边界与已执行前缀的判定、并发裁决、自动 / 人工改路的判断结构）」。
  - 0146 的 Consequences 写的是「以「实例半边」为由暂缓的 ADR 与票逐份重新定性」，但 0052、0053、0068 三份的 Status 行都没有指向 0146 的指针。
  - `0148-…` 照 0146 落了首个候选生成形态。
  - 代码与 0146 一致。迁移 `network_routing/0010_route_strategy_ranking_form.sql` 的头注写明「0008 建表时写「规则正文全属 PAR-NET-14…」；ADR-0146 把排序形态判为产品策略」。领域 `RankingForm` 也已落地。
- **归类**：A。应由 0146（或它的重定性票 product-strategy-boundary）对上面四句挂部分停用。被停条款引原句，适用场景写「属 0146 决定七所列产品策略的那几项；阈值、日历与截单、目录内容、选哪种排序仍是租户取值」。
- **严重度**：中。读 0052、0068 的人会以为候选生成与排序「不得先写一份默认实现」，结果是拒做产品策略，或者把它误登成租户参数。
- **建议修法**：如上。三份的正文不改。

### B3·5 派送发起口已改挂运营决定，0149 的索引注记仍写它归作业事实（A）

来源：ADR 互相矛盾 · 作业簇

- **类别**：3。
- **位置**：
  - `0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md` 决定一：「作业事实登记（节点收寄、场外揽收与尝试、交接、移动、派送任务、交付、段关闭、有效时间判断等）的提交者是运营企业的一线人员」。
  - 它的 Status 行（由 0151 停用写入）：「派送发起口与其余各条不变」。
- **证据**：
  - `0230-…` 决定七「**七、补拍口改挂运营决定（代决复核 M2）。**」。
  - 0230 停用的只有两处：「部分停用 ADR-0151 决定六末句「派送发起口……与 ADR-0149 其余各条不变」里派送发起口那一项」，以及 0216 决定四。0149 本身没有挂停用，所以 0149 的 Status 行仍断言派送发起口按作业事实登记。
  - 代码与 0230 一致：`cmd/parcel-api/endpoints_delivery_dispatch.go` 把 `/transport-fulfillment-delivery-dispatch-triggers` 挂在 `decisionFace(accessidentity.DecisionDeliveryDispatchReask)`。
  - `known-suspensions.md` 里的 `0149<-0230` 是误报：0149 全文没有 `0230` 字样。
- **归类**：A。应由 0230 部分停用 0149 决定一「派送任务」一项对 `/transport-fulfillment-delivery-dispatch-triggers` 的适用，并改掉 0149 Status 行里「派送发起口与其余各条不变」那半句。
- **严重度**：中。照 0149 接 Intake 会把这一口接回作业事实面，一线设备的令牌又能打它，与 0230 越权风险点 8 相反。
- **建议修法**：如上。

### B3·7 0230 改了晚到交接信封的处理，只停了 0216 决定四，没停决定一那句理由（A，fork 核出）

来源：ADR 互相矛盾 · 作业簇

- **类别**：3。
- **位置**：`0216-…` 决定一：「信封晚到时，对象可能已经被下一次交接带走。这时此刻在场的段才是该派送的那一段」。
- **证据**：
  - `0230-…` 决定六：「给了时，链尾不是凭它入场，答`不是触发事实`」，以及「信封晚到时，对象已被下一次交接带进后一段，后一段的链尾不是这一版」。
  - 0230 的「对已接受记录的作用」只停用了 0216 决定四。
  - 代码与 0230 一致：`internal/transportfulfillment/application/trigger_dispatch_on_handover.go` 传 `ExpectedHandover`；`trigger_delivery_dispatch.go` 不是凭这一版入场时答 `ENTRY_NOT_THIS_HANDOVER`。
- **归类**：A。应由 0230 部分停用 0216 决定一这一句，适用场景限随交接信封发起的那一路。
- **严重度**：中。照 0216 写，晚到的信封会替后一段形成派送任务。
- **建议修法**：在 0216 的 Status 行与 Links 补前向指针。

### B3·9 0150 停用了 0091、0100 里「隔离形态不退场」的同义句，漏了 0078 决定四（A，fork 核出）

来源：ADR 互相矛盾 · 作业簇

- **类别**：3。
- **位置**：`0078-isolated-environment-operations-reads-admit-by-assembly-injection.md` 决定四：「隔离读面准入不因真渠道出现自动退场」。决定一、三还枚举了八行查阅面。
- **证据**：
  - `0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md` 决定三：「某一口在装配点换上真渠道 Intake 的同一笔，撤下该口的隔离放行」。
  - 0150 只停用了 0091 决定六与 0100 决定五的同义句，并写「（ADR-0078 与 ADR-0091 其余各条）——一字不变」。0078 的 Status 行与索引注记里都没有 0150（`grep -F` 0 次）。
  - 代码跟 0150：`cmd/parcel-api/endpoints.go` 只剩委托查阅一族走隔离读；`cmd/parcel-api/assemble_isolated_read.go` 写未设即那一族未配置即拒。其余各行已撤。
- **归类**：A。应由 0150 部分停用 0078 决定四那一句，以及决定一、三的八行枚举作为现行放行面的效力。适用场景：某一口已换上真渠道 Intake。`/customer-tracking-view` 的排除判据不变。
- **严重度**：中。照 0078 会以为换上真渠道之后隔离读照旧放行。
- **建议修法**：在 0078 的 Status 行与 Links 补前向指针。

---

### GL·G-02 「价格规则版本」在 SA CONTEXT 里多了「可执行价卡版本」一种含义，与 GLOSSARY、PC 的归属不同，代码已跟着分成两种用法

来源：GLOSSARY ↔ CONTEXT

- **类别**：目标 6（同一词定义不同，两个 CONTEXT 各下一份定义，且归属上下文不同）。
- **位置**：`docs/domain/settlement-accounting/CONTEXT.md` Language「价格规则版本」：「由 `party-commercial` 拥有的商业价格政策或由 `parcel-pricing` 拥有的可执行价卡版本。结算只引用已解析商业依据和实际 `PricingEvaluation`，不发布或修改任一版本。」SA Rules 里的「采购价格规则版本」也按这个口径用：「整组重述：评价引用、采购价格规则版本、供应商协议引用」。
- **证据**：
  - `docs/domain/GLOSSARY.md`「价格规则版本」：「由 `party-commercial` 拥有、在明确适用范围和期间内授权销售价格、采购价格、法人间结算价格或其他商业金额的商业政策版本。可执行价卡和费率表版本由 `parcel-pricing` 拥有。」所有者一条是 `party-commercial`。
  - `docs/domain/party-commercial/CONTEXT.md` 同名词条：「商业价格政策的兼容简称。它只表示 `party-commercial` 拥有的商业适用性、授权和绑定规则；可执行的价卡、费率表和计算工件由 `parcel-pricing` 拥有。」
  - 代码分成两种用法：
    - SA 的 `domain.PurchaseRuleVersionReference` 由 `internal/settlementaccounting/adapters/parcelpricing/buy_evaluation.go` 填成 `evaluation.PlanReference().ID() + "@" + evaluation.PlanReference().Version()`，也就是 PP 的定价方案版本。
    - `internal/settlementaccounting/ports/ports.go` 的注释写「RuleVersion 是评价命中的可执行价卡版本（GLOSSARY「价格规则版本」的 PP 半边）」。
    - PC 代码（`internal/partycommercial/domain/price_policy.go` 等）里，「价格规则版本」只指商业价格政策。
- **归类**：A。GLOSSARY 与 PC 一致，SA 是孤例。附带一项代码命名与注释的跟改：`PurchaseRuleVersionReference` 及上面那句注释。只改名、不改行为，可以并进本轮的文档票，也可以另立小票。
- **严重度**：中。读供应商预期成本时，看到「采购价格规则版本」会以为指 PC 的采购价格政策版本，实际存的是 PP 的定价方案版本，溯源会对错对象。
- **建议修法**：SA CONTEXT 删掉自己那份定义，改为引用 GLOSSARY。评价命中的版本改叫「可执行价卡版本」（GLOSSARY 已有这个词条）或「定价方案版本」。SA 的类型名与注释随后跟改。

### GL·G-03 GLOSSARY「面单服务终局边界」写成由服务产品和客户合同定义、PC 拥有「产品和合同规则版本」；ADR-0058 已裁定规则正文归接单规则包，产品与合同只是采用方

来源：GLOSSARY ↔ CONTEXT

- **类别**：目标 6，兼目标 5（GLOSSARY 指认的拥有对象与 PC CONTEXT 的「拥有/不拥有」相抵）。
- **位置**：`docs/domain/GLOSSARY.md`「面单服务终局边界」
  - 「由适用服务产品和客户合同定义的、使面单渠道服务包裹从可取消服务阶段进入非取消终局结果的业务边界。」
  - 「所有者：`party-commercial` 拥有产品和合同规则版本；`parcel-shipment` 保存适用快照并判断具体包裹是否达到边界。」
  - PS CONTEXT 同名词条照抄了首句，Rules 里另有「面单服务终局边界必须来自适用服务产品和客户合同」。
- **证据**：
  - `docs/domain/party-commercial/CONTEXT.md`「面单服务终局规则」：「接单规则包版本对面单渠道服务终局边界、适用结果作出的商业定义；关闭或重开与取消授权目录归授权规则版本。服务产品版本和客户合同版本是采用方：委托被接受时固定所采用的规则版本，并不因此取得这两份正文的所有权。」
  - ADR-0058：「产品与合同是采用方：接受时固定闭包引用这些规则版本，并不因此取得正文所有权。」它的备选方案里，「**挂在服务产品或客户合同上。** 否决」。
  - 代码：`internal/partycommercial/domain/service_stage_content.go` 的 `NewFinalRuleContent` 要求拥有对象是 `AcceptanceRulePackageObject`。
- **归类**：A。
- **严重度**：中。照 GLOSSARY 建模，会把终局规则挂到服务产品或客户合同上，正是 ADR-0058 否决的那条路。代码现在是对的。
- **建议修法**：GLOSSARY 首句改成「由所采用接单规则包版本的面单服务终局规则定义（服务产品版本与客户合同版本是采用方）」。所有者一条改成「`party-commercial` 拥有规则正文（挂在接单规则包版本下；关闭、重开与取消授权目录挂在授权规则版本下）」。PS CONTEXT 那份同义定义改为引用 GLOSSARY，Rules 里那半句跟着改。

### GL·G-04 GLOSSARY「客户费用退款」把「渠道结果」列为依据，SA 写明非索赔退款只来自计价纠错或商业让利、渠道退款结果不是这一调整

来源：GLOSSARY ↔ CONTEXT

- **类别**：目标 6。
- **位置**：`docs/domain/GLOSSARY.md`「客户费用退款」：「依据合同、渠道结果或其他有效责任，对已经形成的客户费用追加的退款或贷记金额。」
- **证据**：
  - `docs/domain/settlement-accounting/CONTEXT.md` 调整类型表：「| 非索赔客户费用退款 | 仅作为计价纠错或商业让利的结果，减少既有普通客户费用 | `UC-SA-002` | 渠道退款结果不是该调整；真实退回由 `UC-SA-005` 映射 |」。
  - SA Boundaries：「不能从渠道退款结果自动推导」。
  - SA Language 同名词条把它分成两种：计价纠错或商业让利导致的非索赔退款，和索赔导致的退款。GLOSSARY 没有这层区分。
  - UC-SA-002 `AT-SA-043`：「渠道交易作废但计价纠错或商业让利条件尚未成立」时，不形成客户费用退款。
- **归类**：A。
- **严重度**：中。照 GLOSSARY 写，会拿渠道退款结果直接当依据形成客户费用退款，违反 SA 的唯一创建用例划分。
- **建议修法**：GLOSSARY 改成「依据计价纠错、商业让利（`UC-SA-002`）或客户索赔责任（`UC-SA-007`）…」，再补一句「渠道退款结果只能触发重判，本身不是依据」。SA Language 那份定义改为引用 GLOSSARY。

### GL·G-06 「当前客户资料版本采用判断」的取值集合：GLOSSARY 列五格、PS Language 列四格，两边都用的 `待复核` 却不在任一集合里

来源：GLOSSARY ↔ CONTEXT

- **类别**：目标 6（取值集合不同）。
- **位置**：
  - `docs/domain/GLOSSARY.md`「当前客户资料版本采用判断」：「它可以表示已采用、待下游判断、待补充、冲突或未决，不是可手工覆盖的全局状态。」
  - 同一份 GLOSSARY 的「收件地点引用」：「`待复核`时不给引用，对象不属任何已接受委托时答没有收件地点。」
- **证据**：
  - `docs/domain/parcel-shipment/CONTEXT.md` Language 同名词条：「它可以表示已采用、待补充、冲突或未决，不是可手工覆盖的全局资料状态。」少了「待下游判断」。
  - PS 生命周期又写「当前判断可以是已采用、待下游判断、待补充、冲突或未决。」
  - PS Rules 里的收件地点引用同样用 `待复核`。
  - UC-PS-002 结果表把它写成一格「待补充/待复核」。
  - 代码 `internal/parcelshipment/domain/customer_source_data.go` 的 `SourceDataAdoptionOutcome` 只有 `SourceDataAdopted` 与 `SourceDataAwaitingReview`。注释说「`待复核`是 UC-PS-002 对 CONTEXT`待补充`那一格的点名叫法……客户补不出东西来，要人来并」。
- **归类**：A。代码与 UC 一致，文档的取值名没有对齐。
- **严重度**：中。照 GLOSSARY 写的人会做出「待补充」一格、把它交给客户补，可代码与 UC 的那一格是要人工复核的 `待复核`。GLOSSARY 在别的词条里引用的取值，又不在它自己的集合里。
- **建议修法**：GLOSSARY 把集合写成「已采用、待下游判断、待补充/待复核、冲突或未决」，与 UC-PS-002 同口径，并说明 `待复核` 指要人工合并、不是客户补件。PS Language 那份定义删掉，改为引用 GLOSSARY。

### GL·G-07 「限制适用性判断」的取值集合：GLOSSARY 五格（含「技术未形成」），CC Language 四格

来源：GLOSSARY ↔ CONTEXT

- **类别**：目标 6（取值集合不同）。
- **位置**：`docs/domain/customs-compliance/CONTEXT.md` Language「限制适用性判断」：「它可以是有效限制、无内部限制、待确认或冲突，供下游重新评估，不是可手工修改的全局状态。」
- **证据**：
  - `docs/domain/GLOSSARY.md` 同名词条：「判断至少区分有效限制、当前无内部限制、适用待确认、冲突和技术未形成；不得用单一可手改状态表示。」
  - CC 自己的 Rules 也有第五格：「限制适用性不确定、来源不可确定、范围冲突或技术未形成时分别保持相应待确认/冲突/未形成结果」。
  - 代码里这个判断没有立成枚举，`internal/customscompliance` 下没有对应类型，是文档内部的分歧。
- **归类**：A。
- **严重度**：中。照 CC Language 写，会漏掉「技术未形成」，把读不动并进「待确认」或「冲突」。三处的取值名也不一样：「当前无内部限制 / 无内部限制」「适用待确认 / 待确认」。
- **建议修法**：取值集合只在 GLOSSARY 定一次，名字统一。CC Language 改为引用 GLOSSARY，或至少补上「技术未形成」，并与 GLOSSARY 用同一套名字。

### GL·G-08 GLOSSARY「计价参考序列」说只有两个实例，ADR-0110 与 PP、代码已有第三种

来源：GLOSSARY ↔ CONTEXT

- **类别**：目标 6（取值集合不同），兼目标 4（GLOSSARY 停在 ADR-0110 之前的说法）。
- **位置**：`docs/domain/GLOSSARY.md`「计价参考序列」：「按业务时点解析、不可覆盖、被版本清单引用的外部数值序列。燃油费率与汇率是它的两个实例。」
- **证据**：
  - `docs/domain/parcel-pricing/CONTEXT.md` 同名词条：「燃油费率、汇率与按期公布的定额附加金额（高峰 / 需求附加费一类）是它的三个实例」，并写明取值可以是带币种的金额，窗外行为要声明。
  - ADR-0110（已接受）把它定为第三种计价参考序列。
  - 代码：`internal/parcelpricing/domain/plan_structures.go` 有 `ReferenceSeriesFuelRate`、`ReferenceSeriesExchangeRate`、`ReferenceSeriesPublishedAmount` 三种。
- **归类**：A。
- **严重度**：中。读 GLOSSARY 的人会以为序列只装费率，金额序列不在模型里。「数值来自承运商公布费率或财务侧牌价」一句也漏了公布金额。
- **建议修法**：GLOSSARY 改成三个实例，补「取值可以是费率或带币种的金额」，并指向 ADR-0110。GLOSSARY 与 PP 同时各写一份完整定义，属目标 6 的重复，见 G-09；收敛时留一份权威，另一份改为引用。

---

### CM·M-1 CC↔NR 关系仍按「关务适用性判断口尚未补上」写，CC 拥有清单也漏了这项判断

来源：CONTEXT-MAP

- **类别**：目标 1（被后续落地答过的过渡口径还以现行口吻留着），兼目标 4。
- **位置**：`docs/domain/CONTEXT-MAP.md` 的 `customs-compliance ↔ network-routing` 一条：「路由按候选取关务适用性、判断口补上之前怎么答，见 [ADR-0148]」。MAP「`customs-compliance` 拥有」清单只到「禁限运、归类、原产地、申报价值、监管凭证适用性等规则化合规判断及其规则版本、依据和决定方式。」，没有关务适用性判断。
- **证据**：
  - ADR-0148 决定三写的是缺执行器时的退路：「在它补上之前，取数侧对候选的关务一格如实答状态未知」。
  - 这个口已经补上：票 `.scratch/routing-first-cut/issues/12-cc-customs-applicability-judgment-for-route-candidates.md` 是「Status: resolved」，2026-10-10 进 main。
  - 代码：`internal/customscompliance/ports/customs_applicability.go` 的 `CustomsApplicabilityQuery`，以及 `internal/networkrouting/adapters/customscompliance/customs_source.go` 头注「关务适用性判断口（票 routing-first-cut/12，ADR-0148 决定三）」。
  - CC CONTEXT 已立「**关务适用性判断**」词条，Boundaries 也写了「本上下文拥有合规候选区域、口岸、申报路径和关务适用性判断」。
- **归类**：A。
- **严重度**：中。读者会以为判断口还缺，照退路去实现「一律答状态未知」。
- **建议修法**：把那半句改为引 CC CONTEXT「关务适用性判断」，写明三格答复（可用 / 不可用 / 状态未知）是 CC 的口；ADR-0148 决定三只作历史引用。CC 拥有清单补「关务适用性判断」。

### CM·M-3 parcel-pricing 拥有清单只认「燃油费率与汇率」两种序列，漏了第三种序列与计价参考目录

来源：CONTEXT-MAP

- **类别**：目标 1（MAP 落后于 PP CONTEXT 与 ADR-0109、ADR-0110）。
- **位置**：`CONTEXT-MAP.md`「`parcel-pricing` 拥有」：「计价参考序列（燃油费率与汇率）的登记、版本化、发布治理，以及按计价基准时点的解析结果。」。同文件 `parcel-pricing ↔ 外部数值来源`：「承运商公布燃油费率，财务侧提供汇率牌价」。
- **证据**：
  - PP CONTEXT「计价参考序列」：「燃油费率、汇率与按期公布的定额附加金额（高峰 / 需求附加费一类）是它的三个实例」（ADR-0110，Accepted）。
  - PP CONTEXT「计价参考目录」：「承运商的分区表（目的邮编 → 分区）与偏远档位表（目的邮编 → 档位）是它的两个实例」（ADR-0109，Accepted）。
  - MAP 的 PP 拥有与不拥有两栏都没有提到计价参考目录。读者可能把「邮编 → 分区」误归到 `network-routing` 的「版本化服务区域」下。
- **归类**：A。
- **严重度**：中。
- **建议修法**：PP 拥有一栏改为「计价参考序列（燃油费率、汇率、按期公布的定额附加金额）与计价参考目录（分区表、偏远档位表）的登记……」；「外部数值来源」一条补上承运商公布的定额附加金额与分区 / 档位表。
- **转 GLOSSARY 组**：GLOSSARY「计价参考序列」仍写「燃油费率与汇率是它的两个实例。」，GLOSSARY 也没有「计价参考目录」词条，同属目标 6。

### CM·M-4 网络与路由择优写成「只消费 BUY 评价」且「形成唯一的候选选择」，与 ADR-0148 决定四和 NR 的并列规则相抵

来源：CONTEXT-MAP

- **类别**：目标 1，兼目标 4。
- **位置**：
  - `CONTEXT-MAP.md`「`network-routing` 拥有」：「多份供应商价卡并存时，为计划履约段选定外包伙伴的择优决定。」与「择优消费 `parcel-pricing` 对每个候选形成的 `BUY` 评价作为成本维度，按版本化策略裁决」。
  - `parcel-pricing → network-routing`：「同一票存在多份适用供应商价卡时，计价对每个候选分别形成 `BUY` 评价」与「网络与路由把这些评价作为成本维度并入择优，形成唯一的候选选择」。
- **证据**：
  - ADR-0148 决定四：外包段引 BUY 价卡，「自营段（含集团内另一法人承运的段）引内部价格政策下的价卡」。
  - 代码 `internal/networkrouting/adapters/parcelpricing/cost_source.go` 分两支取评价：`case nrports.SupplierBuyPlanBasis` 与 `case nrports.InternalPolicyBasis:`，后者要求 `PricingDirectionInternal`。
  - NR CONTEXT：「最低成本并列、选不出唯一一条时为冲突，交授权角色裁决」，并且「不按候选标识或任何无业务依据的次序收尾」。
- **归类**：A。
- **严重度**：中。照「形成唯一的候选选择」实现的人会补一条并列收尾，正是 NR CONTEXT 禁止的；照「只消费 BUY」实现的人会让自营段缺成本依据。
- **建议修法**：NR 拥有一栏改写为「路由候选按成本单维择优：外包段取 BUY 评价、自营段取 INTERNAL 评价（ADR-0148 决定四）」；`parcel-pricing → network-routing` 一条把「形成唯一的候选选择」改为「择优结果可以是唯一选中或并列冲突，并列交授权角色」，方向也写成 BUY / INTERNAL 两路。

### CM·M-5 结算→计价那条仍写「自然键只守幂等」，ADR-0229 已把唯一约束换成（自然键、代次）

来源：CONTEXT-MAP

- **类别**：目标 1（MAP 写着已被 ADR-0229 改掉的规则）。
- **位置**：`CONTEXT-MAP.md` 的 `settlement-accounting → parcel-pricing`：「评价请求的身份是铸造标识，自然键（范围、目的、引用集合摘要）只守幂等」。
- **证据**：
  - ADR-0229 决定一：「BUY 与 SELL 两本评价请求册各加三格：代次、前一代、续办依据。唯一约束从自然键换成（自然键、代次）」。SELL 那一路的同义句（ADR-0190 决定一「自然键（租户、包裹、计费时点）只守幂等」）已由 ADR-0229 部分停用；MAP 里 BUY 那一路的同形句没有跟着改。
  - 代码 `internal/settlementaccounting/adapters/postgres/evaluation_request.go`：注释「（自然键、代次）拦另一个 ID 带同一组成分在同一代再来」，查询 `ORDER BY generation DESC LIMIT 1`；迁移 `migrations/settlement_accounting/0042_evaluation_request_generation_and_stop_cause.sql`。
- **归类**：A。
- **严重度**：中。照 MAP 只在自然键上建唯一约束，续办出来的下一代就落不进册。
- **建议修法**：改为「自然键加代次唯一（ADR-0229），按自然键读回最新一代」，并指向下一条「评价未形成与参数已登记」。

---

### SC·S-01 场景十六：容量预占写成承运接受的直接结果

来源：SCENARIOS

- **类别**：目标 2（场景结果与 UC 的 AT 行、ADR 现行决定相反）
- **位置**：`docs/domain/SCENARIOS.md` 场景十六·过程第 1 步：「申请量为 `500kg`，外部服务方只接受并确认 `350kg`，因此先形成 `350kg` 的有效容量预占」
- **证据**：
  - `docs/adr/0203-capacity-assurance-requires-a-reservation-on-every-pool-of-the-schedule-and-reservations-are-operation-decisions.md` 决定三：「**三、预占与释放是两种运营决定，都声明需求量（本票）。**」，需求「来源是封闭的两种：`OPERATOR_DECLARATION`（运营人员声明的一版需求）与 `PREPARATION_DEMAND_SNAPSHOT`（运输准备结果固定下来的需求快照）」。承运接受不在这两种来源里。
  - 同一 ADR 决定一：「装载分配或确认订舱所在的具体班次只要登记了至少一个容量池，就必须对该班次的每个池引用一份当前有效预占，并逐池声明分配量」。也就是说预占要在确认订舱之前就有，确认订舱去引用它，而不是从确认订舱派生出来。
  - `docs/application/transport-fulfillment/UC-TF-004-COMMISSION-AND-ACCEPT-TRANSPORT.md`：「| `AT-TF-041` | 订舱确认但无容量预占依据 | 保留订舱结果，不能伪造有效预占 |」
  - 代码：`internal/transportfulfillment/domain/capacity_demand.go` 的需求来源只有 `OPERATOR_DECLARATION`、`PREPARATION_DEMAND_SNAPSHOT` 两个值。
- **归类**：A（代码和 ADR-0203 一致，错在场景）
- **严重度**：中。作者照场景写，会做出「承运接受后自动建预占」，正好落进 AT-TF-041 禁止的「伪造有效预占」。
- **建议修法**：第 1 步改成：运营决定按声明的需求来源先形成预占，确认订舱引用这份预占；承运方只接受 `350kg` 时，接受量与预占量各记各的，超出部分的预占另行释放或调整。第 9 步「有效预占……`380kg`」与第 2 步的超配写法同步复核（见待核 Q-01）。

### SC·S-02 场景十五：阶梯返利写成按合同声明的「阶梯方式」计算

来源：SCENARIOS

- **类别**：目标 1（场景复述了与产品策略封闭形态相反的说法）
- **位置**：`docs/domain/SCENARIOS.md` 场景十五·过程第 6 步：「保底量和阶梯返利分别按合同声明的考核对象、合格范围、阶梯方式、周期、结算方向和确认时点计算」
- **证据**：
  - `docs/adr/0165-periodic-fees-are-minimum-spend-volume-floor-and-tiered-rebate.md`：「**一、形态封闭为三套，另加本期不适用。**」与「阶梯返利：把基数切成互不重叠的档，每档只乘自己的万分比再向下取整，然后相加」
  - `docs/domain/settlement-accounting/CONTEXT.md`：「周期费用的计算形态封闭为三套，另加明确的本期不适用。」与「数值与是否适用按租户和规则引用另册登记。」
  - 代码：`internal/settlementaccounting/domain/periodic_fee.go` 只有一种分档形态（`NewRebateTier` / `NewOpenRebateTier`，万分比）。
- **归类**：A
- **严重度**：中。「阶梯方式」由合同声明，读起来像有多种算法可选（全额累进、超额累进……），会让人去做一个可配置的阶梯算法；而现行决定是产品只内置一种形态，数值另册登记。
- **建议修法**：去掉「阶梯方式」，写成「按产品内置的分档形态（每档只乘本档比例）计算，考核对象、档位与比例等数值按租户另册登记」；同步把「合同声明的」改成「已登记的」。

### SC·S-03 场景十二：计价基准时点写成由客户合同约定

来源：SCENARIOS

- **类别**：目标 1 / 目标 3（场景以现行口吻保留了已被代决改判的说法）
- **位置**：`docs/domain/SCENARIOS.md` 场景十二·前提：「客户合同明确计价基准时点、价格规则版本、冻结政策、对账周期和余额不足时的业务限制」；过程第 2 步：「`parcel-pricing` 按约定计价基准时点形成 50 元 SELL `PricingEvaluation`」
- **证据**：
  - `docs/adr/0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md` 复核记录：「**R2**：计价基准时点认定为产品策略、由内置时点给出；SA CONTEXT 与 GLOSSARY 的对应句随之改（票 demo-chain-gaps/141）。」
  - `.scratch/demo-chain-gaps/issues/141-pricing-basis-moment-is-product-strategy-in-sa-context.md`：「代决「是」：计价基准时点为产品策略、由内置时点给出；改 SA CONTEXT「每类……可执行价卡必须声明自己的计价基准时点」与 GLOSSARY 对应定义。」这张票的修订范围只有 SA CONTEXT 和 GLOSSARY，没有 SCENARIOS。
  - 代码：`internal/settlementaccounting/domain/parcel_billing_moment.go` 的 `BillingMomentNetworkIntakeAdopted`、`BillingMomentEffectiveDeliveryFinalized`。
- **归类**：A
- **严重度**：中。照场景写，会给合同或价卡加一格「计价基准时点」；R2 的理由正是这一格「价卡没有声明这一格的位置，硬要声明只会每张卡答未配置」。
- **建议修法**：并进票 141 一起改。前提里删去「计价基准时点」；第 2 步改成「按产品内置的计价时点」。

### SC·S-06 场景十六：供应商预期成本写成结算「依据采购规则」直接形成，跳过了计价

来源：SCENARIOS

- **类别**：目标 1
- **位置**：`docs/domain/SCENARIOS.md` 场景十六·过程第 11 步：「`settlement-accounting` 依据采购规则形成供应商预期成本」
- **证据**：
  - `docs/domain/transport-fulfillment/CONTEXT.md`：「`parcel-pricing` 依据采购商业依据形成 BUY 纯评价，`settlement-accounting` 再判断费用金额责任」
  - `docs/domain/settlement-accounting/CONTEXT.md`：「供应商预期成本首版按 BUY 评价回指的那份评价请求形成」；开篇：「它消费 `parcel-pricing` 的纯计价评价，但不拥有可执行价卡或第二套算价规则」
  - 场景十三的前提写对了（先由 `parcel-pricing` 形成 BUY 评价，再由结算采用），两个场景对同一条链给了两种写法。
- **归类**：A
- **严重度**：中。「结算依据采购规则形成」读起来就是结算自己按规则算价，正好违反「不拥有第二套算价规则」。
- **建议修法**：改成「`parcel-pricing` 依据采购商业依据和这些发生项形成 BUY 评价，`settlement-accounting` 采用评价形成供应商预期成本」。

### SC·S-07 场景十八：不符合责任条件写成「不予受理」，混淆了资格审核与责任审核

来源：SCENARIOS

- **类别**：目标 2（场景结果名与 CONTEXT 生命周期、UC 主流程不合）
- **位置**：`docs/domain/SCENARIOS.md` 场景十八·过程第 10 步：「P1 因不符合适用责任条件形成有依据的不予受理」
- **证据**：
  - `docs/domain/visibility-exception/CONTEXT.md`：「收到客户索赔、通过资格审核和确认赔偿责任是不同判断。」；生命周期：「- 资格审核 → 等待补充、不予受理或进入责任审核：结果保存合同、首次索赔期限、授权、重复关系和材料依据。」与「- 责任审核 → 全部成立、部分成立、不成立或当前无法认定：结论按明确责任范围和证据形成，金额结算独立处理。」；资格的判据是「系统必须先保留原始提交事实，再按申请人授权、客户账户、合同版本、索赔时限、目标范围、重复关系和最低材料要求判断资格。」
  - `docs/application/visibility-exception/UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md` 主流程：「6. 对符合资格的索赔项形成全部成立、部分成立、不成立或当前无法认定的责任结论」
- **归类**：A
- **严重度**：中。「不予受理」（资格层的负向结果）和「不成立」（责任层的结论）是两个不同的业务结果。照场景写，责任条件的判断会被放进资格审核，复核期限、撤回规则等后续处理也会跟着走错。
- **建议修法**：二选一写清。如果原意是合同不覆盖这种索赔类型或已经超期，就写「因不符合合同索赔资格（如……）形成有依据的不予受理」；如果是责任条件不满足，就写「进入责任审核后形成不成立的责任结论」。

### SC·S-09 场景十：待识别实物 U 被列为可交出的对象

来源：SCENARIOS

- **类别**：目标 1
- **位置**：`docs/domain/SCENARIOS.md` 场景十·过程第 6 步：「后续交出 P1、U 或 H 时，`transport-fulfillment` 逐对象形成交接结果」（U 在第 3 步作为「待识别实物 U」被发现）
- **证据**：
  - `docs/domain/node-operations/CONTEXT.md`：「获得候选标识 → 保存候选关联和依据；候选尚未确认时不得据此执行方向性作业。」以及「分拣到下一方向、集包出运、装载和交出等方向性作业必须暂停，直至取得有效依据。」（U 没有正式身份，也就没有路由指令）
  - 同一文件「节点控制」一节：权威运输交接认作业实物时，如果对象不是集运单元，就「否则按正式包裹身份读本上下文此刻的当前关联，恰对上一件作业实物才推进，零件或多件不推进。」交出 U 不会转出控制。
  - 同场景第 5 步：「节点按当前有效路由和集运兼容判断移入、移出成员，并重新封装 H」——U 没有路由，也不能据此装回 H 随单元交出。
- **归类**：A
- **严重度**：中。场景把待识别实物写成了可以正常进入方向性交接的对象，和 NO 的方向性作业门禁相反。
- **建议修法**：第 6 步删去 U；或者写明「U 只有在取得正式身份和有效路由之后，或依据明确的退回/移交处置决定，才交出」。

---

### PB·M2 横切缺口里「两道哨兵」那段仍按 ADR-0064 之前和 ADR-0053 决定四写

来源：产品基线诸件

- **类别**：目标 1
- **位置**：开发主线「横切缺口」中「投递侧、派发循环与消费侧均已换真」一条：「第一道是**商业适用性**——把判断键折成解析标识的映射属实例半边」，以及「才轮到第二道**网络定义登记册**按 ADR-0053 答`未配置`」，后接「那条路今天根本走不到」。
- **证据**：
  - ADR-0064 决定一：「**一、适用性闭包标识的权威是已接受决定上的解析标识，作为命令附加字段，不是判断维。**」开发主线自己的四项判据重定级表 PN-03 格也写了初始路由消费门的商业适用性「已按 ADR-0064 从已接受解析回指（`NewRoutingApplicability`），不是缺口」。
  - ADR-0053 决定四「查无登记 → `未配置`」已由 ADR-0148 决定六停用：`未配置`改由目录修订锚和路由策略版本答。
- **归类**：A
- **严重度**：中。这段明说是给排障的人看「先后要写准」，而两道哨兵的成因都已经换了。
- **建议修法**：改写或加注：第一道按 ADR-0064 由已接受解析回指；第二道按 ADR-0148 决定六由目录修订锚和策略版本答。删掉「属实例半边」这一句。

### PB·M3 HTTP 接入面仍被写成「`PAR-INT-01` 实例半边」「被 ADR-0055 决定五两项拦着」，与 ADR-0100、0139、0149 相反

来源：产品基线诸件

- **类别**：目标 1
- **位置**：
  - 开发主线「机制半边只剩一处」段末：「HTTP 接入面不在此列：它的机制半边已成型，缺的 Intake 认证方式属 `PAR-INT-01`，是实例半边，按本节规则不计入缺口」
  - 横切缺口「进程对外的业务 HTTP 面已经打开」一条：「缺口因此收敛为「逐端点等真渠道 Intake」」，以及「载荷规范化摘要与准入范围装配两项未决仍拦着真渠道 Intake（ADR-0055 Decision 五）」
  - 附带：`PRODUCT-STORY.md`「生产路径在接入渠道登记(`PAR-INT-01`,见[参数登记册](./PILOT-PARAMETER-REGISTER.md))之前一律 403 未配置」
- **证据**：
  - ADR-0139 决定一：「**一、首方客户接入渠道是产品自有的接入渠道族，属机制半边；`PAR-INT-01` 的适用面收窄为」……
  - ADR-0149：「**四、ADR-0055 决定五两项未决按口逐一解。**」
  - ADR-0100 把操作者渠道判为机制半边。开发主线自己在重定级段也写了「ADR-0100 已把操作者身份判归机制半边，上表第一项因此不能沿用」，但叙述段没跟着改。
  - 代码：`cmd/parcel-api/endpoints.go` 已有 `首方客户族（ADR-0139）`、操作者四个能力面和集成客户端族。
- **归类**：A
- **严重度**：中
- **建议修法**：给这两条加补记：接入渠道族的机制半边由 ADR-0100、0139、0149 承担，`PAR-INT-01` 只剩「哪位客户启用哪条渠道」和「既有渠道的适配」；ADR-0055 决定五按 ADR-0149 决定四逐口解。产品故事那句改成「在租户登记接入渠道（产品自有渠道族的登记册）之前」。

### PB·M4 开发主线的 SELL/BUY 补记仍写「不进派发」，ADR-0190 已取代那几句，ADR-0208 已把 BUY 触发接进派发

来源：产品基线诸件

- **类别**：目标 1（引用的 ADR-0169、0170 部分被取代，ADR-0164 部分停用）
- **位置**：开发主线：
  - 「2026-09-30 补记（票 `product-strategy-boundary/12` 第 9 项，ADR-0169）」末句「不把这套编排接进进程」
  - 「2026-09-30 补记（票 `product-strategy-boundary/12` 第 10 项，ADR-0170）」末句「SELL 形成编排仍不进派发」
  - 「2026-09-30 补记（票 `product-strategy-boundary/12` 第 8 项，ADR-0164）」：「BUY 评价请求的触发面是 `evaluationRequestOrchestration.Trigger`，时点只有发生项形成」
- **证据**：
  - ADR-0190 的 Status 行：「取代 [ADR-0169](./0169-sell-evaluation-forms-a-customer-charge.md) Consequences「形成编排不进 `cmd/parcel-dispatch`」」，以及 ADR-0170 的同类句子。
  - ADR-0164 的 Status 行注明决定一末句由 ADR-0208 部分停用；ADR-0208 决定一让结算在派发里消费发生项已形成信封。
  - 代码：`cmd/parcel-dispatch/routes_final_outcome.go` 与 `routes_evaluation_request.go` 已接 SELL 请求链；`routes_charge_occurrence.go` 已接 BUY on charge occurrence formed。
- **归类**：A
- **严重度**：中。作者会以为 SELL 客户费用链不在派发进程里。
- **建议修法**：加补记，指向 ADR-0190 与 ADR-0208，说明这三句自那两票起过时。

### PB·M5 开发主线 PN-03 覆盖缺口一行仍写「当前代码尚未实现这些上下文」

来源：产品基线诸件

- **类别**：目标 4
- **位置**：开发主线「当前应用用例覆盖缺口」表 PN-03 行：「真实收寄模式、资格、路由策略、节点/设备/SOP、外部来源和非功能参数仍待提供，当前代码尚未实现这些上下文」
- **证据**：`internal/networkrouting`、`internal/nodeoperations`、`internal/transportfulfillment` 都有完整四层代码；同文「机制半边现状」把 PN-03 定为「达标（显式留待已认可）」。同表 PN-02 行已经自承「此前本行复述实现进度，已经因此烂过一次」，本行是同一种烂法。
- **归类**：A
- **严重度**：中
- **建议修法**：按 PN-02 行的做法删掉实现进度那半句，指向「机制半边现状」。

### PB·M6 开发主线把商业解析排在生产归属之前，与 UC-PS-001、导览和代码的顺序相反

来源：产品基线诸件

- **类别**：目标 2，兼目标 4
- **位置**：开发主线「端到端主链」阶段 1：「`party-commercial` 先从不可覆盖的已发布商业版本唯一解析客户、法人、产品、合同和规则；完整拟受理范围再取得唯一生产归属」
- **证据**：
  - UC-PS-001「应用流程」的步骤 3B「| 3B | 试点准入控制 | 对完整拟受理范围形成版本化生产归属结果」排在步骤 4A「| 4A | `parcel-shipment` / `party-commercial` | 按独立商业选择锚点执行」之前。
  - 导览 §5.1 先列「| 4. 生产归属 | 判断完整范围是否由本产品生产处理 |」，再列阶段 5「接受前判断」；§5.6：「一份已归属本产品的请求先建立最小委托候选并进入`已提交`，随后由 `parcel-shipment` 编排接受判断。」
  - 代码：`internal/parcelshipment/application/submit_shipment_request.go` 在提交时判生产归属，商业解析在其后由接受判断（ADR-0081 信封驱动）完成。
- **归类**：A。开发主线的位次高于 UC，但代码与 UC 一致，按 A 改主线措辞。
- **严重度**：中
- **建议修法**：阶段 1 改为「先取得完整拟受理范围的唯一生产归属，本产品取得权威后，`party-commercial` 唯一解析……再逐包裹形成可达性」。

### PB·M7 「仍待提供」的列表把产品策略项整体归进实例半边（三类）

来源：产品基线诸件

- **类别**：目标 1（三类）
- **位置**：开发主线「当前应用用例覆盖缺口」：
  - 表后：「上表各行「仍待提供」的真实参数属**实例半边**」
  - PN-02 行：「真实商业版本、规则包、财务策略、角色、时点和试点参数仍待提供」
  - PN-05 行：「真实程序、来源、角色、规则和复杂验证参数仍未确认」
- **证据**：
  - ADR-0146 决定二：「角色模型（有哪些角色、各能做什么）归产品策略，把人分派到角色归租户取值」。
  - 同文重定级表把「时点」改判为「标准语义可内置折法，租户截点才是取值——改判，待 PC owner 复核」，把授权坐标改判为「从请求方与委托推出法人、权限等级与商业范围是方法，授权授予与原因目录才是租户取值」。
  - PN-05 第三项：「**未满足**：申报发送通道」属产品策略缺执行器。
- **归类**：A
- **严重度**：中。表后那句以权威口吻把这些项判成「无租户不可能取得」，会让作者推迟本该现在做的产品策略。
- **建议修法**：表后那句改成「其中的租户取值属实例半边；判断方法、角色模型、连接器形态等产品策略按 ADR-0146 现在就做」。PN-02、PN-05 两行把「角色」「时点」「规则」限定为取值。

### PB·M8 开发主线的 PN 切片两张表漏了后来新增的五个 UC

来源：产品基线诸件

- **类别**：目标 2
- **位置**：开发主线「首发纵向开发切片」与「当前应用用例覆盖缺口」两表：
  - PN-03：「`UC-NR-001/003`、`UC-NO-002/003`、`UC-TF-002`、`UC-PS-003`」
  - PN-05：「`customs-compliance`、`UC-CC-001..012` 及专项协作用例」，以及「| PN-05 | `UC-CC-001..012` |」
  - PN-07：「`parcel-pricing`、`settlement-accounting`、`UC-SA-001..007`、`SET-01..11`」
- **证据**：`docs/application/` 现有 `UC-CC-013`（ADR-0206，状态「开发基线已形成；判断方法、节点关务区域登记册与判断记录已交付」）、`UC-NO-004/005/006`（ADR-0210、0214、0212）、`UC-PP-001`（ADR-0152），`docs/application/README.md` 也已登记它们。AGENTS「开工顺序」第一步要求先从这份主线定出 PN 切片，这五个 UC 在主线里没有归属。
- **归类**：A
- **严重度**：中
- **建议修法**：把它们补进对应切片：`UC-CC-013` 归 PN-05；`UC-NO-004/005/006` 归 PN-03 或 PN-04（UC-NO-005 跨交接）；`UC-PP-001` 归 PN-07。

### PB·M9 关务基线仍按「十二个关务用例」评估，没有 UC-CC-013

来源：产品基线诸件

- **类别**：目标 2
- **位置**：关务基线「状态」行与「评估结论」：「本文评估现有十二个关务应用用例」「十二个现有用例没有需要删除的完整重复」；「用例级开发交付清单」止于 UC-CC-012。附带：导览 §9.6「`UC-CC-001..012` 覆盖建案、资料、就绪、授权、提交、结果、后续动作、监管处置、税费/放行、关闭重开、内部限制和外部监管舱单。」
- **证据**：`docs/application/customs-compliance/UC-CC-013-JUDGE-ACTION-ADMISSION.md`（ADR-0206）。它与 UC-CC-009、011、006 各有分工：本用例不登记放行门禁核对，那是 UC-CC-009 的事。关务基线的「无重复」判断没有覆盖到它。
- **归类**：A
- **严重度**：中
- **建议修法**：评估结论与交付清单补 UC-CC-013（动作准入判断、PAR-CUS-08），并写明它与 UC-CC-009 门禁核对的分工；去掉「十二」这一计数。

### PB·M10 关务基线把剩余阻塞全部归为真实实例，与 ADR-0146 和开发主线 PN-05 的重定级不一致（三类）

来源：产品基线诸件

- **类别**：目标 1（三类）
- **位置**：关务基线：
  - 「评估结论」：「剩余阻塞不在于继续增加“清关状态”或空用例，而在于真实程序实例和证据尚未填充」
  - 「关务专项开发切片」后：「任何未取得真实来源、规则或责任依据的分支都必须保持显式未配置」
  - §3：「首发入口、允许字段、阶段、授权、并发和迟到语义通过 `PAR-COM-13` 与 `PAR-INT-01` 确认」
  - 「生产分支启用阻塞清单」：「`PAR-COM-13` 中客户原始资料补充/更正的生产入口、允许字段、阶段、授权、并发和迟到语义」
- **证据**：
  - 开发主线重定级 PN-05 第三项：「**未满足**：申报发送通道」——连接器形态属产品策略（ADR-0146 决定一）；ADR-0227 Context：「申报发送连接器不存在」。
  - 参数登记册 `PAR-COM-13` 已按 ADR-0146 拆分：「「基础版本/并发合并规则」「与制签/收寄/装袋/申报/关闭后续办的交接规则」是判断方法，划为产品策略」。
  - ADR-0146 决定一：判断方法归产品，现在就做；只有租户取值留空。
- **归类**：A
- **严重度**：中
- **建议修法**：评估结论补一句「产品策略侧余量（申报发送连接器等）按 ADR-0146 现在就做」；「规则」限定为租户取值；并发语义从 PAR-COM-13 的阻塞清单里移出，指向 psb/06。

### PB·M11 导览 §8.6 业务入口对照表的「当前行为」一列与代码、ADR-0100/0149/0150 不符

来源：产品基线诸件

- **类别**：目标 4，兼目标 1
- **位置**：导览 §8.6：
  - 「| `/shipment-requests` | 命令 | `parcel-shipment` | 提交委托、来源保全、生产归属和接单判断 | 真实渠道未配置时拒绝 |」
  - 「| `/node-operations/receptions` | 命令 | `node-operations` | 节点收寄 | 真实渠道未配置时拒绝 |」
  - 「| `/customs/external-results` | 命令 | `customs-compliance` | 接收外部监管结果 | 真实渠道未配置时拒绝 |」
  - 「| `/tracking-projections` | 查阅 | `visibility-exception` | 运营追踪投影 | 仅在显式隔离读准入时可读 |」，各目录读行同此
  - 表下注：「“仅在显式隔离读准入时可读”指开发/演示环境中注入带 `SYN` 前缀的合成运营作用域」
- **证据**：`cmd/parcel-api/endpoints_*.go`：
  - `/shipment-requests` 挂 `customerClientFace(...)`（ADR-0139 首方客户族）
  - `/node-operations/receptions` 挂 `workFactFace(accessidentity.WorkScopeNode)`；交付两口挂 `workFactFace(...Offsite)`（ADR-0149 作业事实登记）
  - `"/customs/external-results": {method: http.MethodPost, face: integrationClientFace(accessidentity.FactCustomsExternalResult)}`
  - `/tracking-projections`、`/pricing-price-cards`、`/network-catalog`、`/commercial-*`、`/customs-*-rules|registers`、`/visibility-catalogues` 一律挂 `operationsReadFace()`（ADR-0100 操作者渠道）
  - `endpoints.go` 头注：「faceIsolatedRead：委托查阅及其子集视图，只有隔离读准入换得了（ADR-0078）。」
  - ADR-0150 决定三：隔离形态逐口退场。
  - 只有撤回、逐包裹取消、`/claims` 仍是 `unconfiguredFace()`，`/shipment-request-views` 仍是隔离读。另外，代码登记处现有 176 个路径，这张标为「当前登记的业务入口快照」的表只列了 20 来个。
- **归类**：A
- **严重度**：中。实施和联调人员会按错误的渠道族去配凭证。
- **建议修法**：这一列改成「接入面族」，按端点表的 `face` 写；或者整表改为只引 `cmd/parcel-api` 登记处与契约生成物（ADR-0140），不再在导览里抄快照。

### PB·M12 导览 §12.2 的 PN 定级表与开发主线的两次定级都对不上

来源：产品基线诸件

- **类别**：目标 1
- **位置**：导览 §12.2：
  - 「| PN-02 | 商业权威、生产归属、接受前判断、委托与包裹身份 | 部分 |」（PN-03/06/07 同为「部分」）
  - 「| PN-05 | 关务合规控制 | 达标 |」（PN-04 同为「达标」）
  - 「这里的“达标”只指首发开发主线定义的机制半边，不表示真实租户已上线。」
  - 附带 §9.6：「关务机制半边是当前达标切片之一，十二个用例、持久化和多条交接均已形成」
- **证据**：
  - 开发主线按三条判据：「| PN-02 | `party-commercial`、`parcel-shipment`、`network-routing` | 达标（显式留待已认可） |」，PN-03/06/07 同为「达标（显式留待已认可）」。
  - 按四项判据重定级：「上表与上段「产品就绪里程碑就此成立」是按当时三项判据的如实定级，原样保留；判据自本日起改为四项（见上节），**按新判据产品就绪暂不成立**。」，结论「**结论：按四项判据产品就绪不成立。**」，PN-04、PN-05 的第三项都「未满足」。
  - 导览的「部分 / 达标」两套判据都对不上；主线现在定义的产品就绪是「**产品就绪 = 八个切片的机制半边与产品策略全部达标**」。
- **归类**：A
- **严重度**：中
- **建议修法**：删掉这张表，改为引开发主线「机制半边现状」；如果保留，按四项判据如实写「不成立」，并分列第一项和第三项。§9.6 那句同改。

### PB·M13 导览 §9.0 说用例目录有 49 份，并漏掉 5 个 UC

来源：产品基线诸件

- **类别**：目标 5（数别处的东西），兼目标 2
- **位置**：导览 §9.0：「应用目录当前有 49 份业务用例文档」。§9.0 索引与 §9.4、9.6、9.8「主要用例」都没有 `UC-NO-004/005/006`、`UC-CC-013`、`UC-PP-001`。§9.8「计价作为 `UC-SA-002` 的纯计算前置能力工作」没提到 UC-PP-001。
- **证据**：`find docs/application -name 'UC-*.md'` 得 55 份，扣掉 `UC-PS-001-BUSINESS-DECISION-BRIEF.md` 为 54 份；`docs/application/README.md` 已登记上述五份。
- **归类**：A
- **严重度**：中
- **建议修法**：去掉数目，改为「以[应用用例索引](../application/README.md)为准」，并补齐五行。导览自己第 17 章说过「不使用易失效行号和计数作为长期锚点」。

### PB·M14 导览第 9 章各模块的「产品操作面 / 当前成熟度」大面积过时，而且违反导览自己的维护规则

来源：产品基线诸件

- **类别**：目标 4
- **位置**（导览第 9 章，均为逐字引文）：
  - §9.2「面单交易、人工复核和更多处置页仍处于骨架或待接线阶段」
  - §9.3「路由计划与改路页仍以骨架为主」
  - §9.4「管理台有节点作业查阅骨架」「真实节点、设备、SOP、身份读口和一线作业端仍待接入」
  - §9.5「管理台有运输履约查阅骨架」
  - §9.7「异常分诊、案件、索赔追偿和通知工作流多为骨架」「真实消息渠道、资格规则的租户读面、材料归集和外部来源映射仍是主要缺口」
  - §9.8「价格评价页仍在产品化」
  - §9.9「管理台已有费用、对账、核销和经营结果骨架」
  - §9.10「管理台有阶段准入与暂停恢复骨架」
- **证据**：
  - `apps/admin-web/src/page-registry.tsx` 的 `liveIds`（PRODUCT-STORY 指定的接线唯一权威）含 `label-transactions`、`acceptance-review`、`authorized-disposition`、`route-plans`、`node-operations-review`、`transport-fulfillment-review`、`exception-triage`、`exception-cases`、`claims-recovery`、`pricing-evaluation`、`charges-billing`、`reconciliation`、`settlement-application`、`operating-metrics`、`stage-admission`。
  - 身份读口：开发主线 2026-10-16 补记「`parcel-api` 与 `parcel-frontline-import` 的身份核对改读这本册，两处未配置替身退场」（ADR-0184）。
  - 资格规则读面与材料归集：开发主线 PN-06 格「资格规则视图带租户维读法与材料归集面两件已收」，`.scratch/ve-claims-read-seams/issues/01`、`02` 均为 resolved。
  - 导览第 17 章：「模块实现进度变化只更新第 13 章的观察性快照」。
- **归类**：A
- **严重度**：中
- **建议修法**：第 9 章只保留模块职责，进度句一律删掉或改指 `liveIds`、开发主线；第 13 章快照重写一次并注明取证钉。

### PB·M15 导览对 UC-PC-003 授权范围的描述与 ADR-0116 和代码相反

来源：产品基线诸件

- **类别**：目标 1，兼目标 4
- **位置**：导览 §9.0 UC-PC-003 行：「（草案）按已确认范围读取动作授权规则，返回已授权、不允许、规则未配置或未形成；当前首切仅支撑主动拒绝授权，客户侧撤回/资料修订、取消、调整等动作的授权仍待业务决策和参数登记。」；§9.1：「`UC-PC-003`（草案）：按已确认范围形成商业动作授权裁定；当前首切仅支撑主动拒绝授权，其他动作授权待业务决策和参数登记。」
- **证据**：
  - ADR-0116（Accepted）：「**一、`AuthorizedAction` 加「资料修订」一格（`SOURCE_DATA_AMENDMENT`），与人工复核、主动拒绝并列。**」
  - 代码 `internal/partycommercial/domain/authority_grant.go` 的 `AuthorizedAction` 现有 `ManualReviewAction`、`ActiveRejectionAction`、`SourceDataAmendmentAction`、`ControlledClosureAction`、`ReopeningAction` 五格；只有撤回仍留给 `PAR-COM-14`。
- **归类**：A。`UC-PC-003` 正文「首切范围」一节同样停在二值，属 PC 组范围，见文末附带。
- **严重度**：中
- **建议修法**：改成「已裁人工复核、主动拒绝、资料修订（ADR-0116）、受控关闭与重开五个动作；客户撤回的授权仍待 `PAR-COM-14`」。

### PB·M16 产品基线「首发原则」仍写「不同时铺开多个…渠道」，与 ADR-0088 决定三不符

来源：产品基线诸件

- **类别**：目标 1
- **位置**：`PRODUCT-BASELINE.md`「首发原则」：「首发选择一条真实国际小包线路完成端到端闭环验证，不同时铺开多个国家、线路和渠道。」
- **证据**：
  - ADR-0088：「3. **「一条线路」约束的作用面。**」——一条线路只约束网络服务主链路，面单渠道服务的首发范围以登记的渠道产品与产品—渠道映射为界；Consequences 写明「多家渠道的适配与择优取证全部前置」。
  - PILOT-SCOPE 已改成「该约束作用于网络服务主链路；面单渠道服务……不以线路表达」；开发主线「已确认的首发范围约束」也有同一限定。
  - ADR-0088 Consequences 列出的「随本 ADR 修订的权威文本」不含产品基线，这一处因此被漏改。
- **归类**：A
- **严重度**：中。「不铺开多个渠道」与首发面单渠道服务的多渠道择优相抵。
- **建议修法**：照 PILOT-SCOPE 的措辞补上作用面限定，并链到 ADR-0088。

---

### PL·F1 `PAR-INT-08` 列举可授能力面时漏了「运营决定」

来源：试点三件

- **类别**：目标 1（与 ADR 现行决定相抵），兼目标 4（与代码相抵）
- **位置**：登记册 `PAR-INT-08` 行，「已确认约束」格：
  > 登记册配置写、主数据与运营查阅读两格可授，治理登记只预留（按 ADR-0085 决定四另裁），作业事实登记一格按租户 × 作业范围授予、同登本行
- **证据**：
  - ADR-0151 决定一：
    > **一、操作者渠道新增能力面「运营决定」，覆盖委托侧五口与 TF 在管理台上作决定或判断的各口。**
    
    同一决定又写：
    > 某个租户把哪位操作者授予哪种决定，是租户取值。
  - ADR-0212 决定九：
    > 谁持有「送站交接确认」运营决定授予（`PAR-INT-08`）与它的准入范围（`PAR-INT-11`）。
  - ADR-0210：
    > 谁持有这一种授予是租户取值（参数登记册 `PAR-INT-08` 的行）
  - ADR-0236 的三个手工口也：
    > 三口挂操作者渠道「运营决定」能力面
  - 代码 `internal/accessidentity/operator.go`：`checkGrantable` 放行四格，其中有
    > CapabilityOperationDecision CapabilityFace = "OPERATION_DECISION"
- **归类**：A。代码与 ADR-0151、ADR-0210、ADR-0212、ADR-0236 一致，是登记册这一格停在 ADR-0100 那时候的三格加 ADR-0149 的作业事实。
- **严重度**：中。实施方照这一行收证据，就收不到「谁持哪种运营决定」：复核完成、主动拒绝、受控关闭与重开、关段、建派送任务、送站交接确认、派送重问、身份冲突处置、外部资金事实采用、监管凭证登记，以及关务建案与提交各口，上线后都会答无授予。
- **建议修法**：「已确认约束」格补「运营决定一格按租户 × 决定种类授予（ADR-0151 决定一），决定种类是产品的角色模型」；「最低证据」格补「每笔运营决定授予的决定种类」。

### PL·F2 `PAR-INT-11` 与 `PAR-GOV-03` 只列了两组准入能力词，代码已收四组

来源：试点三件

- **类别**：目标 4（与代码相抵），兼目标 1
- **位置**：
  - 登记册 `PAR-INT-11` 行，「已确认约束」格：
    > 能力与事实类型照认证方的词登（ADR-0180 决定四：运营决定口取 `OPERATION_DECISION` 与决定种类，外部结果与资金事实口取 `EXTERNAL_FACT` 与外部事实类型），集合外拒收
    
    同格又写：
    > 对照没登的租户，运营决定与外部事实各口答「不在准入范围」
  - 登记册 `PAR-GOV-03` 行，「最低证据」格：
    > 〔ADR-0180 拆分〕「能力、事实类型」两项的词表归产品策略：运营决定与外部事实各口照认证方的词登（ADR-0180 决定四）
- **证据**：
  - ADR-0180 决定四末项早就预告了词表会扩：
    > 别的能力面要判准入时（如作业事实登记），它的词同属本表，随那一面的票定，取法同此：一个能力词，事实类型逐口取。
  - ADR-0139 Decision 八：
    > 其余生产写口（撤回、逐包裹取消、受控补充、资料修订、索赔提交）在铸造侧按 ADR-0149 决定四第二条判
  - 代码 `internal/accessidentity/customer_client_pre_advice.go`：
    > const CustomerCommandAdmissionCapability = "CUSTOMER_COMMAND"
    
    送站预报提交与撤回、送站交接确认三口在铸造侧判准入。
  - 代码 `internal/accessidentity/admission_scope_mapping.go` 的 `checkAdmissionRequirement` 有
    > case CapabilityWorkFactRegistration.String():
    
    收送站交接单那几种作业事实。这两组词由 2026-10-19 的提交 `aa0a8ab4`、`4a9ac524` 加入（票 ps-external-mark-relations/09、12）。
- **归类**：A。代码照 ADR-0180 决定四末项与 ADR-0139 Decision 八扩了词，登记册这两格没跟上。
- **严重度**：中。租户上线照这两行只登运营决定与外部事实的对照，首方客户的送站预报与交接确认、节点的送站交接单作业事实，上线后会一律答 `OUTSIDE_ADMISSION_SCOPE`。读者还会误以为「没登对照答不在准入范围」只管两族口。
- **建议修法**：两格都改成按族点名四组：运营决定 `OPERATION_DECISION` 加决定种类、外部事实 `EXTERNAL_FACT` 加外部事实类型、首方客户命令 `CUSTOMER_COMMAND` 加客户面能力词、作业事实登记 `WORK_FACT_REGISTRATION` 加作业事实类型；具体收哪些词交给 `internal/accessidentity` 那一份集合作权威，格里不再枚举。

### PL·F3 矩阵仍按 ADR-0016 的三条判据写产品就绪

来源：试点三件

- **类别**：目标 1（ADR 现行决定）
- **位置**：矩阵导言：
  > 产品交付走另一条并行轨道，判据为骨架完整、受控案例可复算、参数显式未配置
- **证据**：
  - ADR-0146：
    > **五、产品就绪判据改为四条**
    
    其中：
    > 第三条由「参数显式未配置」改为「租户取值显式未配置，产品策略有执行器」
    
    并新增「演示动线走通」。
  - ADR-0146 的 Status 行写它「部分取代」ADR-0016 的第三条判据。开发主线「切片的机制半边与实例半边」一节已按四条写。
- **归类**：A。
- **严重度**：中。读者照矩阵判产品就绪，会把「没有判断方法」与「没有租户取值」当成同一件事，这正是 ADR-0146 Context 点名的误判。
- **建议修法**：改成「判据见开发主线『切片的机制半边与实例半边』（四条，ADR-0146）」，只引不复述，免得下次再改时又留一份旧的。

### PL·F4 范围与登记册仍把「渠道或国家专属设计、容量维度」挡在租户参数之后

来源：试点三件

- **类别**：目标 1 / 3（ADR 现行决定）
- **位置**：
  - 范围「进入详细设计的条件」：
    > 在真实客户、国家或关务区域、线路、服务形态、关键渠道和业务规模完成确认前，只能继续完善不依赖这些参数的稳定领域模型，不应确定渠道专属流程、国家专属规则、容量账本的具体维度与阈值或性能架构。
  - 登记册「参数就绪门槛」：
    > ### 进入渠道或国家专属详细设计前
    
    其下第二条：
    > `PAR-NET-01` 至 `PAR-NET-09`、`PAR-NET-12` 至 `PAR-NET-15` 已确认适用真实值
- **证据**：
  - ADR-0146 决定一：
    > 机制与产品策略同属产品交付轨道，开发方现在就做
    
    决定二：
    > 公开标准的实例数据（国家与地区码、各国公开注册号格式与校验位算法、公开承运商接口）归参考配置
  - ADR-0201 补记已把容量维度定下：
    > **容量维度封闭词**（产品策略）：维度只认这六个词
  - 登记册自己的 `PAR-INT-02` 写：
    > 面单出向连接器形态（公开承运商面单接口归参考配置）与账号使用授权的选法归产品策略
  - 开发主线：
    > **`No-Go / 待参数化` 只约束生产结论，不约束能力范围。**
  - 范围自己也说机制与实例的划分以开发主线为准：
    > 两半的划分见[开发主线的机制半边与实例半边](./PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)，那里是唯一权威，本文不复制第二套口径。
- **归类**：A。两处都是 ADR-0146 之前的「等租户」口径；范围自己声明开发主线是划分的唯一权威，所以按 A 修，不用另立 ADR。
- **严重度**：中。作者照这两节办，会拒做 ADR-0146 要求现在就做的连接器形态、公开标准参考配置与容量维度，还会把已落地的 ADR-0201 补记读成越界。
- **建议修法**：两节改成只约束「租户取值进入生产」，例如「阈值、具体维度取值与渠道、国家实例的生产启用」；判断方法与参考配置的范围引开发主线。登记册那一节的标题改成「进入渠道或国家专属生产启用前」之类，删去「详细设计」。

### PL·F5 矩阵对 41 例 `SOURCE_RATE_CARD` 的放行条件漏了金额保真度那道门

来源：试点三件

- **类别**：目标 1（CONTEXT 是现行规则出处）
- **位置**：矩阵「计价证据规则」：
  > 参考设计交付的 41 例 `SOURCE_RATE_CARD` 在自述源价卡差异取得业务裁决前隔离
  
  其后：
  > 以这些案例支撑 `P` 级金额验收时必须一并接受该强度。
- **证据**：`docs/domain/parcel-pricing/CONTEXT.md`「源完整性门禁」：
  > 因此本卡可作为**结构来源**——存在哪些费用、由什么触发、按什么方法计算——但**不作为生产金额来源**
  
  以及：
  > 在 `SRC-DISC-002` 取得承运方归属裁决、且金额保真度按上条闭合之前，该案例集的任何案例都不能作为生产价卡金额、正式 `P` 验收或客户账单依据
- **归类**：A。
- **严重度**：中。照矩阵，`SRC-DISC-002` 一裁完，就可以「接受断言强度」后拿这 41 例支撑 `P` 级金额验收；CONTEXT 则要求金额保真度也闭合，而且该卡本身不作为生产金额来源。
- **建议修法**：改成「在 CONTEXT『源完整性门禁』所列各条闭合之前隔离」，只引不复述；删去「以这些案例支撑 `P` 级金额验收时必须一并接受该强度」，或改成「即使闭合，源身份仍只有断言强度」。

### AG·M3 workflow.md 说凭据没有 `workflow` scope、改 `.github/workflows/` 的推送会被拒，这句已过期

来源：docs/agents 与 AGENTS.md

- **类别**：1（规程单一权威）；兼 4（文档与实况）
- **位置**：`docs/agents/workflow.md`「当前宿主」：「`gh auth status` 为账号 `idpxyz`、scope `gist`、`read:org`、`repo`，没有 `workflow`——按 GitHub 的规则，改 `.github/workflows/` 的推送会因此被拒（未实测）。」
- **证据**：
  - `.scratch/infra-speed/issues/01-…md`：「凭据补上 `workflow` 权限后与票 08 同一分支推上去」（2026-10-20）
  - 改 ci.yml 的 `9045b6fd6`（「ci: 票 infra-speed/08……」）已在 `origin/main`
  - 2026-10-21 本机 `gh auth status` 显示「Token scopes: 'gist', 'read:org', 'repo', 'workflow'」
- **归类**：A
- **严重度**：中（作者会把能自己做的 CI 改动当成推不上去，记「待补」交给用户）
- **建议修法**：改成「2026-10-20 起 token 带 `workflow` scope，改 workflow 文件可直接推（`9045b6fd6` 为证）」。
- **附带**（不属仓库文件，转告调用方）：用户 auto-memory 里「Push cannot touch .github/workflows … record as 待补」同样已过期。

### AG·M4 workflow.md「四条红线」少了 AGENTS.md 的第五条「敏感实例外置」

来源：docs/agents 与 AGENTS.md

- **类别**：5（跨文件计数）；兼 1
- **位置**：`docs/agents/workflow.md`「红线在哪一步生效」：「四条红线不是审查清单，是流程里的具体动作。」下面只列了证据层级诚实、租户取值留给租户、所有权清晰、单一权威四条。
- **证据**：`AGENTS.md`「红线」表有五行，第五行是「| 敏感实例外置 | 仓库只登脱敏标识与证据索引；真实映射留在约定的受限存储 |」。
- **归类**：A
- **严重度**：中（照着 workflow.md 走流程的人看不到这道门禁在哪一步生效）
- **建议修法**：去掉「四条」这个数（跨文件计数本身违反「改文档」那条规矩），补一段「敏感实例外置——在写票面、登记册与夹具那一步」，或者写成「AGENTS.md 红线表的每一行」。

### AG·M5 AGENTS.md「改文档」只写「新 ADR 或 supersede」，没写部分停用怎么做，也没写回改 CONTEXT

来源：docs/agents 与 AGENTS.md

- **类别**：3（停用链完整）；兼 1
- **位置**：
  - `AGENTS.md`：「- 改难逆转技术或产品取舍 → 新 ADR 或 supersede；不改写已接受 ADR 历史。」
  - `docs/agents/workflow.md` 同口径：「ADR 只新增或 supersede，不改写已接受的历史。」
- **证据**：本仓最常见的形态是部分停用，`docs/adr/README.md` 的现行规则是「改被停用记录的 `Status` 行与 Links 节各加一条前向指针」，`Status` 那条写进索引注记。本轮 README 的目标 1 与目标 3 要求被停句子不再以现行口吻留在 CONTEXT 里、停用链两头都在。照 AGENTS.md 那一句做的作者只会立新 ADR。读到「不改写已接受 ADR 历史」，还可能以为旧 ADR 的 Status 行也碰不得，于是留下单边停用和 CONTEXT 里的旧句。本轮统计的 91 份带停用指针、各处漏改，就是这种形状。
- **已核**：AGENTS.md 与 `docs/agents/*.md` 里没有「去 docs/adr/README.md 该行手写被停条款」的旧指引（infra-speed/03 要求清理的那类）。`parallel-sessions.md` 已写明「`docs/adr/README.md` 的两个列表后来改成了生成物（见「生成物不占号」），新增 ADR 不再碰它」。缺的不是旧指引，是新规则的入口。
- **归类**：A
- **严重度**：中
- **建议修法**：这一条改成三分支：新 ADR、supersede、部分停用（「被停 ADR 的 `Status` 行写索引注记、Links 加前向指针，规则见 [adr/README](./docs/adr/README.md) 顶部」），再加一句「同笔改 CONTEXT 里被停的句子」。workflow.md 那句改成链接到这里，不再复述。

## 低

### B2·B2-10 计价范围登在两本册上：0179 估价采用册与 0190 SELL 计费时点采用册，和 0179 决定三「同一条规则选卡」相抵

来源：ADR 互相矛盾 · 商业与资金簇

- **类别**：3
- **位置**：`docs/adr/0179-…md`
  - 决定三：「估价与随后形成的客户费用因此按同一条规则选卡」。
  - 决定五：采用册的值是「（形态、计价范围）」。
- **证据**：
  - ADR-0190 决定三：另一本册的值是「（计费时点、计价范围）」。
  - 0190 越权风险点 9 自认：「与 ADR-0179 决定三「同一条规则选卡」的意图有张力」。
  - 0190 Consequences 的验收句却以两册同值为前提：「验收：同一件包裹的预估客户费用与受理前冻结同额」。
- **归类**：B。自认有张力，没有裁定。
- **严重度**：低。演示租户在两本册上登的是同一个范围。
- **建议修法**：SA owner 复核或立新 ADR 二选一：
  - 两册共用一份计价范围，一册引另一册；
  - 明认两份可以不同：收窄 0179 决定三「因此」一句的适用场景，并改 0190 那句验收。

### AG·L6 `blocked` 状态：parallel-sessions 和脚本认它，triage-labels 没定义

来源：docs/agents 与 AGENTS.md

- **类别**：1（规程单一权威）
- **位置**：`docs/agents/parallel-sessions.md`：「把 ADR 与票面里的「越权风险点」、`needs-info` / `blocked` 的票汇成一页」
- **证据**：`docs/agents/triage-labels.md` 的状态角色只有 `needs-triage`、`needs-info`、`ready-for-agent`、`in-progress`、`ready-for-human`、`wontfix`。`scripts/owner-review-queue.ps1` 匹配 `^Status:\s*(needs-info|blocked)`。票面里实际出现过 `Status: block`（1 张），从没出现过 `blocked`。AGENTS.md 的阻塞写法是 `Blocked by:` 行。
- **归类**：B（是在 triage-labels 补一个 `blocked` 角色，还是脚本与文档都改认 `Blocked by:`，要定）
- **严重度**：低
- **建议修法**：二选一后两处同改。若不补角色，parallel-sessions 改成「`needs-info` 与带 `Blocked by:` 的票」。

### X·X-04 「部分被 ADR-0190 取代」不是 README 的机制：0169、0170 的注记没链接、没原句、没写其余各条

来源：ADR 停用链与引用规范（协调者自审）

- **类别**：目标 3
- **位置**：`docs/adr/0169-sell-evaluation-forms-a-customer-charge.md` 与 `docs/adr/0170-settlement-orchestrations-enter-through-parcel-api.md` 的索引注记都写「**部分被 ADR-0190 取代**（「不进派发」那几句）」；两份正文里都没有「不进派发」这几个字。
- **证据**：ADR-0190 决定六：「取代 ADR-0169 Consequences「形成编排不进 `cmd/parcel-dispatch`」、决定四「也不在本记录里把形成编排接进进程」，以及 ADR-0170 Context「SELL 评价形成客户费用的编排不进 `cmd/parcel-dispatch`」与它否决「把 SELL 形成编排接进派发」那一条」。README 只有「部分停用」与「已被取代」两种，没有「部分取代」。
- **归类**：A
- **严重度**：低（注记至少点了名，Links 节写得清楚）。
- **建议修法**：两份注记改成标准部分停用：加 ADR-0190 链接、逐字引上面四句、写「其余各条不变」；0169 前段不再提。

### X·X-05 索引注记里打了引号的「被停原句」有几处不是原句

来源：ADR 停用链与引用规范（协调者自审）

- **类别**：目标 3（所引被停原句须逐字存在）
- **位置与证据**（左为注记引文，右为被停 ADR 正文原句；实测于 `6b0074d0`）：
  - ADR-0022 注记「同段「禁止在两项未决前落地任何默认实现」不变」 ↔ 正文「并禁止在它们决定前落地任何默认实现」（同一行前段引的是对的）。
  - ADR-0053 注记「同条「登记了而解不出 → 响亮报错」与「禁止带空事实族答已配置」两格不变」 ↔ 正文「**查到登记但本进程没有解析层 → 响亮报错。**」「**禁止：查到登记就答 `configured=true` 带空事实族。**」
  - ADR-0048 注记「决定三第三增量的落点（金额缝装配器放在 PS 消费侧）」 没有引原句，是转述。
  - ADR-0081 注记「决定六「结算账户目录留 nil」」 ↔ 正文「时点取值源、可达性闭包标识、结算账户目录、控制金额源全部留 nil」。
  - ADR-0169、ADR-0170 注记「「不进派发」那几句」（见 X-04）。
  - ADR-0214 注记「转移册的主键与「同一版同一方向只处理一次」不受影响」 ↔ 正文「同一版在同一方向上只处理一次」。
  - ADR-0199 注记「（结论「记身份冲突的不重核」维持……）」 ↔ 正文「记身份冲突的（多候选、一码两物）不重核」。
  - ADR-0182 引 ADR-0181 的两句（见 X-01）。
- **归类**：A
- **严重度**：低。
- **建议修法**：07 合并时一并把这几处引文换成逐字原句（只动 `Status` 行）。

### X·X-06 Links 节前向指针缺失

来源：ADR 停用链与引用规范（协调者自审）

- **类别**：目标 3（README：「改被停用记录的 `Status` 行与 Links 节各加一条前向指针」）
- **位置与证据**：
  - 被停方 Links 没有停用方：ADR-0053（缺 ADR-0068）、ADR-0083（缺 ADR-0155）、ADR-0181（缺 ADR-0182，见 X-01）、ADR-0012（缺 ADR-0013，见 X-03）。
  - 停用方 Links 没列被停方：ADR-0201、ADR-0202（各缺 ADR-0014，停用写在文末补记里）、ADR-0199（缺 ADR-0186、ADR-0196，它的 Links 只列了「部分停用于」三条来向）、ADR-0185（整份没有 Links 节，停用了 ADR-0184 决定一）。
  - 另，ADR-0202 Links 仍写「ADR-0204（票 25 起草，号已占）」，ADR-0204 早已接受、无链接。
- **归类**：A
- **严重度**：低。
- **建议修法**：逐条补 Links（Links 不属正文红线，README 与本轮 README 都允许改）。

### X·X-07 Status 前段里「见 adr/README 该行」成了自指

来源：ADR 停用链与引用规范（协调者自审）

- **类别**：目标 3 / 5
- **位置**：ADR-0075 前段「适用场景与停用前提见 adr/README 该行」；ADR-0175 前段「适用场景与原句见 adr/README 该行」；ADR-0217 前段「被停原句与适用场景见 adr/README 各该行」；ADR-0201 前段「适用场景见 adr/README 该行」；ADR-0186、0195、0196、0199、0210 前段「原句见本记录文末「复核记录……」与 adr/README 该行」。
- **证据**：票 infra-speed/03 之后 README 的每一行由该 ADR `Status` 行的索引注记生成（README 顶部「行尾注记原样取自 `Status` 行里 `<!-- 索引注记 -->` 之后到行末的文字」），「见 README 该行」指回的是同一行的后半。
- **归类**：A
- **严重度**：低。
- **建议修法**：07 合并时删掉这些「见 adr/README 该行」，改为「见索引注记」或直接并进注记。

### X·X-08 断链：三份 ADR 的相对链接指向不存在的文件，一份评审记录用本机绝对路径加行号

来源：ADR 停用链与引用规范（协调者自审）

- **类别**：目标 5
- **位置与证据**（脚本扫全部 `docs/` 非 archive 与 `AGENTS.md` 的相对链接与锚点，只这几处解析不了）：
  - `docs/adr/0045-new-submission-version-keeps-history-and-reestablishes-the-task.md` Links：`./0028-aggregate-revision-moves-only-at-the-commit-boundary.md`、`./0030-rehydration-admits-only-the-submitted-state.md`——实际文件是 `0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md`、`0030-rehydration-admits-one-state-at-a-time-by-snapshot-expressiveness.md`。
  - `docs/adr/0054-pre-acceptance-control-policy-view-has-an-unconfigured-grade.md` Decision 二与 Links 两处 `./0029-recovery-action-is-the-error-algebra.md`——实际是 `0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md`。
  - `docs/adr/0011-…`（已被取代）Links 三条 `../../foo/…` 指仓外。
  - `docs/review/081701.md` 13 处 `/D:/tops/idp-parcel/…:行号` 形式的链接（本机绝对路径 + 行号，两条规矩都犯）。
  - 带锚点的相对链接（约 280 条）全部解析得到。
- **归类**：A
- **严重度**：低。
- **建议修法**：0045、0054 的 Links 改正文件名；0054 正文那处链接目标属正文，按 README「同一机制也用于标注正文中已失效的引用坐标」在 `Status` 行注明，或判为只改链接目标不改字（由修订票裁）。`docs/review/081701.md` 若仍是现行文档，改成仓内相对链接与符号名；若只追溯，移入 `docs/archive/`。

### X·X-09 「回答」类前向指针时有时无

来源：ADR 停用链与引用规范（协调者自审）

- **类别**：目标 3
- **位置**：ADR-0233「对已接受记录的作用」：「ADR-0202、ADR-0209 正文不改，它们的越权风险点 5 由决定五回答」；ADR-0234 同节：「ADR-0229 越权风险点 6 由决定四作答」。ADR-0202、0209、0229 的 `Status` 行都没有指向 0233 / 0234。
- **证据**：同类关系在别处都挂了前向指针，例如 ADR-0205 前段「14「段日后才登记，那一条事实不回补投影」已由 [ADR-0224](./0224-…) 回答（2026-10-20）」、ADR-0206 前段「一句与越权风险点 9 已由 [ADR-0213](./0213-…) 回答（2026-10-19）」。README 没规定「回答」要不要挂指针。
- **归类**：A
- **严重度**：低（不停用条款，读漏只是不知道风险点已答）。
- **建议修法**：在 README 部分停用那段后补一句「回答越权风险点、不停用条款的，也在被回答方注记里挂一条『由 ADR-… 回答』」，然后给 0202、0209、0229 补上；或明写不挂。二选一。

### B2·B2-11 ADR-0169「内置时点只有发生项形成」已被 0190 扩成三种，0169 的指针只挂了「不进派发」

来源：ADR 互相矛盾 · 商业与资金簇

- **类别**：3
- **位置**：`docs/adr/0169-sell-evaluation-forms-a-customer-charge.md` 决定三：「**三、何时发起是可登记的触发。** 内置时点只有发生项形成。」索引只注了「部分被 ADR-0190 取代（「不进派发」那几句）」。
- **证据**：
  - ADR-0190 决定二：「**二、内置两种包裹计费时点（产品策略）。**」
  - 0190 决定六：「ADR-0169 的消费门、形成编排与发生项原因触发册不动，与本记录的包裹时点册并存」。
  - SA CONTEXT 已改为「发起 SELL 评价有两条路」。
  - 两路是否互斥、会不会重复计费，由 0190 越权风险点 4 留给后继记录裁。
- **归类**：A。
- **严重度**：低。CONTEXT 已经是对的。
- **建议修法**：在 0169 的 Status 与索引注记加一条：决定三那一句的适用场景收窄为发生项原因触发册那一路（0190 决定二）。

### B2·B2-12 接受前控制链上三处取代只写在正文的复核记录里，Status 行与索引都没有指针

来源：ADR 互相矛盾 · 商业与资金簇

- **类别**：3
- **位置与证据**：
  - **ADR-0122 决定四**：「所以 SA 只把每项结果与条件原样交出，折叠在 PS 的 `adapters/settlementaccounting`」。ADR-0125 决定三说折叠「自此位置变、归属不变」。0122 只在文末复核记录写了「字面不改，以本条与 ADR-0125 为准」，Status 行没写。
  - **ADR-0125**：Consequences「**过渡状态明记**：票 04 落地前 `RESTRICTED` 仍译确定性不通过 → 拒绝」；决定一里受限项只带自己的原因。两者已由 ADR-0132 决定三取代，0132 越权风险点 3 写「扩了 ADR-0125 决定一的形——ADR-0125 正文不改，以本记录为准」。0125 的索引注记仍写着「`RESTRICTED` 一律拒绝是票 04 落地前的过渡状态，不是产品口径」，没有指向 0132。
  - **ADR-0127 决定四**：「遇到比例额度停在新增的`待判断`格 `CREDIT_RATIO_BASE_UNDECIDED`，不折成金额、不默认」。ADR-0129 决定三把这一格收窄为「`CREDIT_RATIO_BASE_UNDECIDED` 保留：它的唯一来路是重建门读回的」存量。0127 的索引注记仍写「比例额度基数未裁停在 `CREDIT_RATIO_BASE_UNDECIDED`」。
- **归类**：A。三处代码都已按取代方实现。
- **严重度**：低。正文里有复核记录可以追到。
- **建议修法**：在三份记录的 Status 行与索引注记补前向指针：0122 指向 0125，0125 指向 0132，0127 指向 0129。

### B2·B2-13 ADR-0017「PostgreSQL 实现…仍然阻断」已被 0026 的「即刻解锁的范围」收窄，0017 没有指针

来源：ADR 互相矛盾 · 商业与资金簇

- **类别**：3
- **位置**：`docs/adr/0017-admission-gates-judged-by-blocking-cause.md`：「**Bento 持久化技术闸门原样保留。** 它卡在外部不可变工件不存在，与租户参数无关。PostgreSQL Repository 实现、迁移、事务与 Outbox 仍然阻断」。
- **证据**：
  - ADR-0026：「**即刻解锁的范围**：针对精确 SemVer 候选编写 PostgreSQL Repository、迁移、事务与 Outbox 实现及其合同测试」。
  - 开发主线已记闸门解除：「Bento 闸门解除（`v0.1.0-rc.2` 经 `PBC-01`/`PBC-06` 锁定）」。
  - AGENTS.md「当前默认切片」仍写「租户取值与 Bento 持久化闸门仍阻断」，见待核 Q-1。
- **归类**：A。
- **严重度**：低。闸门已经解除，两份记录都只剩历史意义。
- **建议修法**：在 0017 的 Status 行加一条：「仍然阻断」一句由 0026 收窄为「发布基线登记仍在闸门后」；闸门后来已解除，以开发主线为准。

### B2·B2-14 ADR-0079 决定五写释放只认冻结账本，与 0047 决定四、0122 决定三的「两本账各认领一次」不符

来源：ADR 互相矛盾 · 商业与资金簇

- **类别**：3
- **位置**：`docs/adr/0079-pre-acceptance-control-policy-view-asks-by-commercial-resolution-reference.md` 决定五：「释放命令不带回指：释放不问控制策略，它按原请求身份在冻结账本上认领。」
- **证据**：
  - ADR-0047 早于 0079，其决定四是「四、释放按原关联双账本认领。」
  - ADR-0122 决定三：「释放两本账各认领一次」。
  - 代码：`internal/settlementaccounting/application/release_pre_acceptance_control.go` 的结果同时带 `Freeze()` 和 `Exposure()`。
- **归类**：A。这句表述在 0079 成文时就已经不准确。
- **严重度**：低。这句话的论点是「释放不问控制策略、不用回指」，这个论点仍然成立，错的只是「冻结账本」这几个字。
- **建议修法**：在 0079 的 Status 行注明：决定五「在冻结账本上认领」的表述以 0122 决定三为准；其余各条不变。

---

### B3·10 0148 改了`未配置`的判法，0052 决定一的第二格定义仍自称不变（A）

来源：ADR 互相矛盾 · 作业簇

- **类别**：3。
- **位置**：`0052-…`
  - 决定一：「`configured=false` 即**网络定义登记册对这个判断范围未配置**」；
  - 其 Status 行：「其余各条（三格端口、专格未决原因、修订由登记册派生、空册与空集合可分辨）不变」。
- **证据**：`0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md` 决定六。
  - 「证据视图答`未配置`，当且仅当该租户的目录修订锚不存在，或判断时点没有适用于该判断范围（租户 + 服务目的）的路由策略版本」；
  - 「定义登记册（迁移 `0007`）不再承载`未配置`，读口不再读它」。
  - 0148 只停用了 0053 决定四与 0068 决定六，没有停 0052 决定一。
  - 代码以目录与策略版本判：`internal/networkrouting/adapters/postgres/network_catalog.go` `LoadDefinitionsAt`；`network_definition` 在 `internal/`、`cmd/` 下零引用。
- **归类**：A。应由 0148 部分停用 0052 决定一第二格的定义（适用场景：自 routing-first-cut/07 起），并改 0052 Status 行里「其余各条……不变」那一串。
- **严重度**：低。0053、0068 的停用链读得出新判法。
- **建议修法**：如上。

### B3·11 自动形成那一路的旅程已经派生，0207 决定四仍写「由登记方声明」（A，fork 核出）

来源：ADR 互相矛盾 · 作业簇

- **类别**：3。
- **位置**：`0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md` 决定四：「旅程由登记方声明，本上下文没有它的权威来源，不核」。其 Status 行写「决定四其余各句不变」。
- **证据**：`0218-…` 决定四「就按对象的正式包裹身份确定性派生」，又写「「旅程由登记方声明」对人工登记不变」。0218 只停用了演示约定那一句。代码 `domain/actual_fulfillment_formation.go` 按 0218 派生旅程。
- **归类**：A。0218 应部分停用 0207 决定四这一句，适用场景限实际履约发生项自动形成那一路。
- **严重度**：低。
- **建议修法**：在 0207 的 Status 行补一条。

### B3·12 0204 说「自营只能显式声明」并称这是 0207 的读法，0207 却另有按承运商判断认自营一格（A）

来源：ADR 互相矛盾 · 作业簇

- **类别**：3。
- **位置**：`0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md`：「自营只能显式声明（ADR-0207 的读法不变）」。
- **证据**：`0207-…` 决定二：「**不适用只有两格**（ADR-0098 决定三）：登记方显式声明自营……或那一段当前的实际承运商判断是自营法人」。代码 `ActualCarrierJudgedSelfOperated` 与 0207 一致。
- **归类**：A。0204 那句括注误转述了 0207。0204 的本意（不由有无委托推断自营）不变。
- **严重度**：低。
- **建议修法**：给 0204 的 Status 行加注，说明那句只指「不由委托有无推断」，按承运商判断认自营照 0207 决定二。

### B3·13 0078 决定五「照旧拦着……一切命令面」已被 0091 与 0149 推翻（A，fork 核出）

来源：ADR 互相矛盾 · 作业簇

- **类别**：3。
- **位置**：`0078-…` 决定五：「载荷规范化摘要与准入范围装配两项未决照旧拦着真渠道 Intake 与一切命令面」。
- **证据**：
  - `0091-isolated-form-extends-to-the-write-path-by-graded-switches.md` 经隔离写开关放行了命令面，但只停用了 0078 决定四一句。
  - `0149-…` 标题写「ADR-0055 决定五两项按口逐一解」，两项未决已不成立。
- **归类**：A。
- **严重度**：低。
- **建议修法**：在 0078 的 Status 行补一条：「一切命令面」由 0091 停用（适用场景是隔离写开关逐口放行的口），「两项未决」由 0149 按口解。

---

### GL·G-09 GLOSSARY 已定义的词，十个 CONTEXT 的 Language 节逐字或换字又各定义一遍，共 188 处（实测于 `6b0074d0`）

来源：GLOSSARY ↔ CONTEXT

- **类别**：目标 6（只在 GLOSSARY 定义一次，CONTEXT 引用）。
- **位置与证据**：下表由脚本比对 GLOSSARY 的 `### 词条` 与各 CONTEXT Language 节的 `**词**` 标题得出。计数是这条发现的论据，按 AGENTS.md「改文档」锚在取证 SHA `6b0074d0`。上面 G-01 至 G-08 以外的各对，人工比过，都是同义，只差措辞或详略。

| CONTEXT | 重定义数 | 词 |
|---|---|---|
| collection-remittance | 5 | 代收指令、代收事实、代收分户账、回汇批次、差异事项 |
| customs-compliance | 35 | 关务案件、申报单元、受控跨客户合报、正式申报资料快照、申报就绪判断、提交授权、技术回执、监管接收、业务受理、监管过程决定、关务执行协作事项、处置执行核对、放行结果、监管凭证、内部合规限制、限制责任来源、解除条件、限制覆盖项、限制解除决定、限制适用性判断、监管处置决定、监管核定税费、监管补缴要求、监管税费退回决定、税费付款协作事项、税费付款核对、放行门禁核对、动作准入判断、节点关务区域、关务案件关闭核对、关闭依据项、剩余关务责任移交、关务案件关闭决定、受控重开决定、后续关务案件 |
| network-routing | 9 | 物流节点、服务区域、网络连接、线路、可达性判断、路由计划、无当前有效路由、路由指令、外部不透明履约段 |
| node-operations | 15 | 作业实物、待识别实物、身份冲突处置、可验证交付范围、送站预报、送站交接单、节点侧交接证据、作业任务、节点监管协作承接决定、作业批次、可复用载具、物理状况观察、在场核对、受控开封、内部作业标签 |
| parcel-pricing | 7 | 价格方向、计算目的、计价输入快照、计价重量、计价参考序列、换算步骤、不可计价 |
| parcel-shipment | 26 | 提交批次、委托、委托接受、委托撤回、委托接受基线、客户原始资料、保价要求、客户原始资料版本、资料补充/更正请求、当前客户资料版本采用判断、收件地点引用、包裹、包裹身份谱系、当前有效包裹、面单交易、面单交易包裹结果、面单交易定案、面单继续尝试决定、权威业务截断边界、外部标识、责任起点、面单服务终局边界、终局服务结果、包裹取消决定、收寄后服务处置决定、委托完成 |
| party-commercial | 17 | 法人资料、货主客户账户、服务产品、网络服务产品、面单渠道服务、服务产品版本、渠道产品、产品—渠道映射、客户合同版本、保价条件、客户服务规则版本、供应商商业协议版本、收费发生条件、价格规则版本、结算政策、渠道约束、渠道账号使用授权 |
| settlement-accounting | 32 | 结算账户、费用项目、价格规则版本、主要计费范围、计价基准时点、计算展开、费用明细、当前有效客户预估费用、客户运营应收、客户费用退款、客户赔付、实际代垫成立判断、客户代垫回收、客户计费重量、供应商计费重量、供应商预期成本、供应商账单主张、审核应付、供应商费用贷项、应追偿金额、追偿认可金额、成本分摊结果、未分摊余额、结算归属日、截单快照、对账单、费用争议、服务税金额、运营结算余额、资金冻结、核销、经营毛利 |
| transport-fulfillment | 25 | 载运对象、履约参与关系、实际履约段、实际承运商、实际承运商判断、运输委托、订舱、承运接受、监管运输处置承接决定、运输资源、容量池、容量预占、装载分配、权威运输交接结果、揽派任务、履约尝试、有效交付结果、交付证明、总单、外部承运凭证、轨迹源、外部承运轨迹事实、运输收费发生项、履约侧代收证据、出发门禁读数 |
| visibility-exception | 17 | 全程追踪投影、标准追踪里程碑、追踪摘要、预计到达时间（ETA）、可见性缺口、异常信号、异常分诊、异常案件、严重度、处置优先级、处置请求、客户可见异常、客户异常通知、证据项、责任结论、索赔提交批次、客户索赔项 |

- **归类**：A。
- **严重度**：低（同义重复本身不改业务结果）。G-01 至 G-08 正是这种双份写法下，其中一份没跟上改动的实例。
- **建议修法**：逐词判一次「跨上下文」还是「本上下文专用」。
  - 跨上下文的词：GLOSSARY 留唯一定义，CONTEXT Language 改成一行指针，照 SA 现有的 `[停因](../GLOSSARY.md#停因)` 写法，再补本上下文的细化。
  - 本上下文专用的词（例如关务案件关闭的那一族、集运单元那一族）：只留 CONTEXT 一处，GLOSSARY 删掉或改为指针。

### GL·G-10 GLOSSARY「文档关系」没有 parcel-pricing；PP CONTEXT 开头也没有「跨上下文术语以统一领域语言为准」一句

来源：GLOSSARY ↔ CONTEXT

- **类别**：目标 5、目标 6。
- **位置**：`docs/domain/GLOSSARY.md`「## 文档关系」列了九个上下文，没有 `parcel-pricing/CONTEXT.md`。全文唯一一处指向它的链接在「计算目的」词条里。
- **证据**：
  - 其余九个 CONTEXT 开头都有「跨上下文术语以[统一领域语言](../GLOSSARY.md)为准」。
  - `docs/domain/parcel-pricing/CONTEXT.md` 开头只有「上下文边界和与其他上下文的关系以[领域上下文地图](../CONTEXT-MAP.md)和 [ADR-0012](../../adr/0012-parcel-pricing-context-within-idp-parcel.md) 为准。」，没有 GLOSSARY 那一句。
- **归类**：A。
- **严重度**：低。
- **建议修法**：「文档关系」补一行 `parcel-pricing`。PP CONTEXT 开头补 GLOSSARY 那一句，与其余九份同形。

### GL·G-11 GLOSSARY「总单」的「运输履约范围」已由 ADR-0113 落定为「实际履约段」，GLOSSARY 未跟改

来源：GLOSSARY ↔ CONTEXT

- **类别**：目标 4、目标 6。
- **位置**：`docs/domain/GLOSSARY.md`「总单」：「一个总单可以关联一个或多个集运单元、包裹或运输履约范围，但关联必须明确且可追溯。」
- **证据**：
  - ADR-0113：「**「运输履约范围」落成实际履约段**：GLOSSARY 那三个字也可能读作运输委托或订舱所覆盖的范围；本记录选实际履约段」。owner 已复核认可。
  - TF CONTEXT：「总单的关联逐条明确到集运单元、包裹或实际履约段」。
- **归类**：A。
- **严重度**：低。
- **建议修法**：GLOSSARY 那三个字改成「实际履约段」。

### GL·G-12 「揽收交接结果」只在 GLOSSARY、PS、NR 里出现，所有者 TF 从不用这个词；「有效收寄」在 GLOSSARY 里没有定义

来源：GLOSSARY ↔ CONTEXT

- **类别**：目标 6（同一事物在不同文档里名字不同）。
- **位置**：
  - `docs/domain/GLOSSARY.md`「有效网络收寄结果」：「客户送达物流节点时，`node-operations` 拥有节点收寄结果；上门揽收或场外接货时，`transport-fulfillment` 拥有揽收交接结果。」
  - PS CONTEXT：「上门揽收或场外接货时由 `transport-fulfillment` 提供揽收交接结果」。
  - NR CONTEXT：「场外揽收时使用 `transport-fulfillment` 的揽收交接结果」。
- **证据**：
  - TF CONTEXT 全文找不到「揽收交接结果」（`grep -c` 为 0）。TF 管这件控制事实叫「场外揽收」，并另有「实际承运商首次有效收寄」。
  - GLOSSARY「实际履约段」写「由有效收寄或权威运输交接事实形成」，TF 又说「它是有效收寄的一种，与场外揽收、权威运输交接并列为控制事实」。「有效收寄」没有 GLOSSARY 词条，容易被读成「有效网络收寄结果」。后者含节点收寄，而节点收寄不形成实际履约段。
- **归类**：A。
- **严重度**：低。
- **建议修法**：GLOSSARY 统一用 TF 的词「场外揽收」，或在 TF 里正式立「揽收交接结果」并说明二者关系。GLOSSARY 补「有效收寄」一句，说明它指 TF 拥有的场外揽收与实际承运商首次有效收寄，不含节点收寄。

### GL·G-13 GLOSSARY 与 NO 对同一概念用了不同的词名：「实物控制关系 / 实物控制」「集运单元 / 集运单元实例」

来源：GLOSSARY ↔ CONTEXT

- **类别**：目标 6。
- **位置**：`docs/domain/GLOSSARY.md` 的 `### 实物控制关系` 与 `### 集运单元`。
- **证据**：
  - `docs/domain/node-operations/CONTEXT.md` 的词条是 `**实物控制**` 与 `**集运单元实例**`。两边定义首句基本相同，GLOSSARY 是「明确节点或运输履约范围当前对作业实物承担保管和后续作业责任」，NO 同句。
  - GLOSSARY「可复用载具」的避免用词里还列着「集运单元实例」，与 NO 把它当正式词条名相冲。
- **归类**：A。
- **严重度**：低。
- **建议修法**：统一词名，以拥有方 NO 的为准，或 GLOSSARY 加别名说明。「可复用载具」的避免用词改写成「把载具叫作集运单元实例」一类，免得读成禁用这个词。

### GL·G-14 「签约服务商」：TF 的交易角色集里有，PC 的角色集里没有，GLOSSARY 只在「承运商代理商」里顺带叫作「签约物流服务商」

来源：GLOSSARY ↔ CONTEXT

- **类别**：目标 6（同一角色集在两个 CONTEXT 里定义不同，GLOSSARY 收了第三种说法）。
- **位置**：`docs/domain/GLOSSARY.md`「承运商代理商」：「代理商销售自有物流产品并承担端到端责任时，还在该笔交易中扮演签约物流服务商角色」。
- **证据**：
  - TF CONTEXT：「渠道账号持有人、渠道服务方、签约服务商、合同与结算相对方、底层承运商、责任承担方和实际承运商必须分别保存」，共七个角色。
  - PC CONTEXT：「渠道账号持有人、渠道服务方、合同与结算相对方、底层承运商、实际承运商和责任承担方必须分别表达」，共六个角色。
- **归类**：A。
- **严重度**：低。
- **建议修法**：GLOSSARY 立「签约服务商」词条，或在「承运商代理商」里统一用这个词名。PC 的角色集补上它，或说明为什么不含。

### GL·G-15 不带前缀的「提交版本」：GLOSSARY 规定只在 CC 内作「申报提交版本」的简称，PC、SA 的 Boundaries 却拿它指 PS 的「委托提交版本」

来源：GLOSSARY ↔ CONTEXT

- **类别**：目标 6。
- **位置**：
  - `docs/domain/party-commercial/CONTEXT.md`：「`parcel-shipment` 拥有客户委托、提交版本、接受判断任务、接受或拒绝决定」。紧接着的下一条又写「`customs-compliance` 按关务案件和提交版本形成具体货物相关主体」，同一节两种含义。
  - `docs/domain/settlement-accounting/CONTEXT.md`：「`parcel-shipment` 拥有委托、提交版本、接受或拒绝决定、包裹、面单交易」。
- **证据**：`docs/domain/GLOSSARY.md`「申报提交版本」：「一次正式对外提交所采用的不可覆盖申报内容、范围和依据快照，在 `customs-compliance` 内简称“提交版本”。」
- **归类**：A。
- **严重度**：低。
- **建议修法**：PC、SA 指 PS 对象的地方写全称「委托提交版本」。

### GL·G-16 PC 引用 PS「资料修订阶段」词条里的「修订意图」原词，PS CONTEXT 并没有定义修订意图

来源：GLOSSARY ↔ CONTEXT

- **类别**：目标 6（跨上下文镜像的词没有权威出处）。
- **位置**：`docs/domain/party-commercial/CONTEXT.md`「资料修订允许声明」：「阶段六格与意图三格的词取 `parcel-shipment` 词条「资料修订阶段」与其修订意图的原词，本上下文只镜像不另定、一格不拆不加」。
- **证据**：
  - `docs/domain/parcel-shipment/CONTEXT.md`「资料修订阶段」只列了阶段六格。PS 全文与 GLOSSARY 都没有「修订意图」（`grep` 只命中 PC 这一处）。
  - 三格（补充、更正、显式清空）只出现在 UC-PS-002 与代码 `internal/parcelshipment/domain/customer_source_data.go` 的 `AmendmentIntent`，代码注释说「三格原词的权威在 parcel-shipment」。
- **归类**：A。
- **严重度**：低。
- **建议修法**：PS CONTEXT 补「修订意图」词条，列补充、更正、显式清空三格及以后的扩充条件。PC 那句指向它。

### GL·G-17 GLOSSARY「当前有效实测」与 NO 说 PP「依据计算目的形成计价重量」，GLOSSARY「计价重量」与 PP 说按重量策略

来源：GLOSSARY ↔ CONTEXT

- **类别**：目标 6。
- **位置**：
  - `docs/domain/GLOSSARY.md`「当前有效实测」：「`parcel-pricing` 依据计算目的形成计价重量」。
  - `docs/domain/node-operations/CONTEXT.md` Boundaries：「`parcel-pricing` 拥有依据计算目的形成的计价重量和纯评价」。
- **证据**：
  - `docs/domain/GLOSSARY.md`「计价重量」：「`parcel-pricing` 在一次评价内按重量策略从计价输入快照派生、用于查表和后续费用基数的重量。」
  - PP CONTEXT「计价重量」写明由重量方法、条件最低重量和取整策略得出。计算目的首发与方向一一对应，不选重量方法。
- **归类**：A。
- **严重度**：低。
- **建议修法**：两处改成「按评价采用的重量策略形成计价重量」。

### GL·G-18 PS、TF、SA 描述 CR 的产物时用「代收应收、实收/已收、清分」，这些词既不在 CR 的封闭词表里，也不在 GLOSSARY

来源：GLOSSARY ↔ CONTEXT

- **类别**：目标 6（同一组对象在三个 CONTEXT 里叫法各异，GLOSSARY 收的是第四种）。
- **位置**：
  - `docs/domain/parcel-shipment/CONTEXT.md`：「`collection-remittance` 拥有代收货款的应收、实收、渠道在途、清分、应付客户和汇付结果」。
  - `docs/domain/transport-fulfillment/CONTEXT.md`：「`collection-remittance` 接受适用证据后形成代收应收、已收、渠道在途、待清分、应付客户、短款、溢款和汇付结果」。
  - `docs/domain/settlement-accounting/CONTEXT.md`：「代收、渠道在途、清分、汇付和短款由独立的 `collection-remittance` 上下文拥有」。
- **证据**：
  - `docs/domain/collection-remittance/CONTEXT.md`「资金位置」：「代收本金在分户账内所处的受托状态，封闭词表：`渠道在途`」…`待清分`、`应付客户`、`已汇付`、`短款`、`溢款`。
  - CR 的对象是代收指令、代收事实、分户账、回汇批次、差异事项，没有「代收应收」「实收」位置。
  - GLOSSARY「代收分户账」用的是 CR 的封闭词表。
- **归类**：A。
- **严重度**：低。
- **建议修法**：三处 Boundaries 改成引用 CR 的对象与资金位置词表，或直接写「代收指令、代收事实与代收分户账结果（见 GLOSSARY）」。

### GL·G-19 「应追偿金额」的相对方范围：GLOSSARY、VE 含「其他责任相对方」，SA 只到「供应商、实际承运商或保险方」

来源：GLOSSARY ↔ CONTEXT

- **类别**：目标 6（取值范围不同）。
- **位置**：`docs/domain/settlement-accounting/CONTEXT.md`「应追偿金额」：「依据 `visibility-exception` 当前有效的追偿责任结论和适用商业规则，对供应商、实际承运商或保险方形成的运营企业应收金额。」
- **证据**：
  - `docs/domain/GLOSSARY.md` 同名词条：「依据当前有效外部责任结论和适用商业规则，对供应商、实际承运商、保险或其他责任相对方形成的运营企业应收金额。」
  - VE「追偿事项」也写「供应商、实际承运商、保险或其他外部责任关系」。
- **归类**：A。
- **严重度**：低。
- **建议修法**：SA 那份定义改为引用 GLOSSARY，相对方范围以 GLOSSARY 为准。

---

### CM·L-1 关系约束的箭头方向两种口径混用

来源：CONTEXT-MAP

- **类别**：目标 4（MAP 内部对集成方向的表达自相矛盾）。
- **位置**：`CONTEXT-MAP.md` 中这三条用「消费方 → 提供方」：「`parcel-pricing → transport-fulfillment`：计价按运输收费发生项的键读取」、「`parcel-pricing → parcel-shipment`：计价按声明包裹身份读取申报测量与地址要素」，以及 `parcel-pricing → parcel-shipment`（提交版本申报）。同一份 MAP 另有反向的 `transport-fulfillment → parcel-pricing / settlement-accounting`；mermaid 图画的也是 `TF --> PP` 与 `PS --> PP`。
- **证据**：
  - 其余各条都是「提供方 → 消费方」。ADR-0120 明写「CONTEXT-MAP 的箭头是 PS 消费 PC，不是反向」。
  - 混用的源头是 ADR-0171 的「`CONTEXT-MAP` 增加计价到运输履约、计价到小包托运两条消费边」。
  - 同类还有 `settlement-accounting → parcel-pricing`：它表达的是请求方向，而代码里是计价消费结算交来的请求（`internal/parcelpricing/adapters/settlementaccounting/`）。
- **归类**：A。
- **严重度**：低。正文都说清了是谁读谁，只是箭头会误导人查依赖方向。
- **建议修法**：三条 `parcel-pricing → …` 改写成 `transport-fulfillment → parcel-pricing（发生项成员读口）`、`parcel-shipment → parcel-pricing（申报测量与地址要素读口）` 等提供方 → 消费方写法；地图开头补一句箭头口径。

### CM·L-2 mermaid 图缺几条已有正文约束、代码里也有消费适配器的边

来源：CONTEXT-MAP

- **类别**：目标 4。
- **位置**：`CONTEXT-MAP.md` mermaid 块。有正文约束、图上却没画的边：
  - `network-routing → parcel-shipment`（NR→PS）；
  - `parcel-pricing → network-routing`（PP→NR）；
  - `settlement-accounting → customs-compliance`（SA→CC）；
  - `settlement-accounting → parcel-pricing` 两条（SA→PP）。
  - 另外 PP→PS 没有正文，见 L-3。
- **证据**：每条边都有对应的消费侧适配器：
  - `internal/parcelshipment/adapters/networkrouting/reachability.go`
  - `internal/networkrouting/adapters/parcelpricing/`
  - `internal/customscompliance/adapters/settlementaccounting/receive_on_adopted_funds_fact.go`
  - `internal/parcelpricing/adapters/settlementaccounting/`
  - `internal/parcelshipment/adapters/parcelpricing/`
- **归类**：A。
- **严重度**：低。
- **建议修法**：在图上补 `NR --> PS`、`PP --> NR`、`SA --> CC`、`SA --> PP`、`PP --> PS` 五条边并配标签。

### CM·L-3 小包托运的渠道择优消费计价 BUY 评价，MAP 没有这条边，还把「多卡择优」整体写成 NR 的事

来源：CONTEXT-MAP

- **类别**：目标 4。
- **位置**：`CONTEXT-MAP.md` 的 `parcel-pricing → network-routing`：「同一票存在多份适用供应商价卡时，计价对每个候选分别形成 `BUY` 评价」。MAP 既没有 PP→PS 的关系约束，也没有这条图边。
- **证据**：
  - 代码 `internal/parcelshipment/adapters/parcelpricing/cost_bridge.go` 头注：「渠道择优按候选的 `BUY` 价卡要成本」。
  - ADR-0148 Context 也写了「面单渠道择优」（label-channel-service-first-release/01、/13）。
  - PP CONTEXT 说「择优由 `network-routing` 形成，本上下文不知道也不决定谁被选中」，与 MAP 同一口径，同样漏了 PS 这一路（属 PP 组）。
- **归类**：A。
- **严重度**：低。MAP 已明写「具体渠道选择」归 PS，缺的只是它向 PP 取成本这条边。
- **建议修法**：补 `parcel-pricing → parcel-shipment（渠道择优成本）` 一条：PP 逐候选答已出价、待判断或不可计价，出局与并列在 PS 的渠道择优比较器；`parcel-pricing → network-routing` 一条改说「路由候选择优」，不再泛指「同一票多卡」。

### CM·L-4 进 parcel-shipment 的「参数已登记」类交接只写了结算账户一封

来源：CONTEXT-MAP

- **类别**：目标 4（MAP 写的跨上下文事件与代码对不上）。
- **位置**：`CONTEXT-MAP.md` 的 `settlement-accounting → parcel-shipment`（结算账户已登记的交接）：「结算登记一笔结算账户后，与账户行同一事务经 Outbox 交出只带租户与登记种类的「参数已登记」类信封」。
- **证据**：代码里另有两封同类信封交给 PS，MAP 两封都没写：
  - `party-commercial.commercial-authority.operator-registration-completed`：生产方 `internal/partycommercial/adapters/postgres/operator_registration_completed_handoff.go`（注释称其为「参数已登记」意图，ADR-0094 决定四），消费方 `internal/parcelshipment/adapters/inbox/operator_registration_completed_consumer.go`。
  - `settlement-accounting.pre-acceptance-estimate-adoption.registered`：生产方 `.../pre_acceptance_estimate_adoption_registered_handoff.go`，消费方 `internal/parcelshipment/adapters/inbox/pre_acceptance_estimate_adoption_registered_consumer.go`。
- **归类**：A。
- **严重度**：低。
- **建议修法**：在 `party-commercial → parcel-shipment` 补「商业授权运营登记已完成」一封；把结算那条交接改成「结算账户或受理前估价采用已登记」两种登记种类。

### CM·L-5 PC→TF、PS→TF 两条关系漏了派送要求的两样引用

来源：CONTEXT-MAP

- **类别**：目标 4。
- **位置**：`CONTEXT-MAP.md` 图边「PC -->|供应商商业协议和采购条件| TF」「PS -->|面单交易、底层承运网络与服务要求| TF」，以及 `party-commercial → transport-fulfillment`、`parcel-shipment → transport-fulfillment` 两条关系约束的正文。
- **证据**：
  - TF CONTEXT「派送要求」：「收件地点引用归 `parcel-shipment`，计划履约段的时间窗口归 `network-routing`，交付方式、收件范围与合同责任的条件引用归 `party-commercial`」。实际承运商判断的承运主体身份也引 PC 的参与方册。
  - 代码：`internal/transportfulfillment/adapters/partycommercial/delivery_condition_source.go`、`carrier_identity_directory.go`，以及 `internal/transportfulfillment/adapters/parcelshipment/delivery_place_source.go`。
- **归类**：A。
- **严重度**：低。
- **建议修法**：PC→TF 补「交付条件引用、承运主体参与方身份」；PS→TF 补「收件地点引用」。

### CM·L-6 结算→计价那条在计价的输入里用了「计费重量」

来源：CONTEXT-MAP

- **类别**：目标 6（术语与 GLOSSARY 不一致）。
- **位置**：`CONTEXT-MAP.md` 的 `settlement-accounting → parcel-pricing`：「合格来源引用指不到包裹主体、分区或计费重量时」。
- **证据**：GLOSSARY「计价重量」：「避免使用：计费重量、客户计费重量、供应商计费重量、实测重量。」。客户 / 供应商计费重量归 `settlement-accounting`，计价缺的其实是实重与尺寸；同一句后文也写「节点作业 / 小包托运的实重尺寸」。
- **归类**：A。
- **严重度**：低。读者可能以为计价要读结算的计费重量，方向就反了。
- **建议修法**：改为「指不到包裹主体、分区或实重尺寸时」。

### CM·L-7 「客户查询授权」被写成 party-commercial 提供

来源：CONTEXT-MAP

- **类别**：目标 1（MAP 写的归属与 MAP 自身的产品边界、VE CONTEXT、代码都相抵）。
- **位置**：`CONTEXT-MAP.md` 的 `party-commercial / parcel-shipment → visibility-exception`：「参与方与商业提供货主客户账户、责任法人、服务产品、客户合同、客户查询授权及适用异常服务规则」。
- **证据**：
  - MAP 产品边界：「身份认证与通用授权能力只负责凭据验证和授权作用域结果」。
  - VE CONTEXT：「认证与授权由共享身份能力提供并传入结果」。
  - 代码：VE 的客户追踪查阅授权走 `internal/visibilityexception/adapters/accessidentity/customer_tracking_view_authenticator.go`；VE 对 PC 的适配器只有 `claim_service_rules.go`。全库只有 MAP 这一处用「客户查询授权」一词。
- **归类**：A。
- **严重度**：低。
- **建议修法**：删去「客户查询授权」，或改成「查询授权作用域由共享身份能力签发」，并移出 PC 的提供清单。

### CM·L-8 PS→NO 那条把只经客户包裹件号的命中也叫「唯一命中」

来源：CONTEXT-MAP

- **类别**：目标 1（MAP 复述 CONTEXT 规则时自相矛盾）。
- **位置**：`CONTEXT-MAP.md` 的 `parcel-shipment → node-operations`：「只经客户包裹件号的唯一命中建立待识别实物、不记冲突」。同一句前文刚定义过「恰一个候选、没有歧义信号且候选范围相容才是唯一命中」。
- **证据**：NO CONTEXT：「恰一个候选、没有信号、只是不范围相容（只经客户包裹件号命中）时，缺的是证据而不是有反证」。PS CONTEXT：「没有时，只有恰一个候选、没有歧义信号、且这个候选范围相容，才是唯一命中」。按这两处，这种情形不是唯一命中。
- **归类**：A。
- **严重度**：低。
- **建议修法**：改为「只经客户包裹件号命中的恰一个候选（不范围相容）建立待识别实物、保存该候选、不记冲突」。这条整段复述 PS / NO CONTEXT 与 ADR-0212，可酌情压成引用（AGENTS.md「单一权威」）。

### CM·L-9 交接触发路由复核的那句漏了 ADR-0235 的「早于当前位置不判」

来源：CONTEXT-MAP

- **类别**：目标 1（写着已被 ADR-0235 部分停用的句子，没带新的适用范围）。
- **位置**：`CONTEXT-MAP.md` 的 `node-operations / transport-fulfillment → network-routing`：「权威运输交接以`已交接`把包裹交进声明的接收节点时，网络与路由从该节点复核」。
- **证据**：
  - ADR-0215 决定三、决定四已由 ADR-0235 部分停用（见 ADR-0215 Status 行）：「触发早于当前位置」的触发只入账、不判。
  - NR CONTEXT：「这次触发只入账为「触发早于当前位置」，不据以失效或改路」。
- **归类**：A。
- **严重度**：低。
- **建议修法**：句末补「触发的业务时间早于路由历史里最近一次已判控制触发时只入账、不判（ADR-0235）」。

### CM·L-10 「参数已登记」的计价触发漏了价卡收尾（ADR-0234）

来源：CONTEXT-MAP

- **类别**：目标 1（MAP 落后于最新 ADR）。
- **位置**：`CONTEXT-MAP.md` 的 `parcel-pricing → settlement-accounting`（评价未形成与参数已登记）：「价卡登记与序列、目录复核通过时同笔交「参数已登记」」。
- **证据**：ADR-0234（2026-10-21，Accepted）的「收尾同笔交「参数已登记」」；PP CONTEXT 已写「收尾同笔交[参数已登记](../GLOSSARY.md#参数已登记)」。
- **归类**：A。
- **严重度**：低。
- **建议修法**：把那半句改为「价卡登记、价卡收尾与序列、目录复核通过时……」。

### CM·L-11 收寄复核的触发其实经 PS 的「有效网络收寄采用」到 NR，MAP 写成节点作业直接提供

来源：CONTEXT-MAP

- **类别**：目标 4。
- **位置**：`CONTEXT-MAP.md` 的 `node-operations / transport-fulfillment → network-routing`：「节点作业提供节点收寄、当前有效实测、节点控制、物理拆分合并、非计划节点和物理装载事实」，以及图边「NO -->|当前有效实测、控制节点、非计划节点与装载事实| NR」。`parcel-shipment → network-routing` 一条没有提到 PS 向 NR 交收寄采用。
- **证据**：
  - NR 只在 `internal/networkrouting/adapters/inbox/network_intake_consumer.go` 收 `parcel-shipment.network-intake.recorded`，经 `adapters/parcelshipment/reassess_on_network_intake.go` 复核。
  - UC-NR-003 也写「收寄采用与交接两路触发分属不同分区」。
  - `internal/networkrouting/adapters/` 下没有 `nodeoperations` 子包。节点作业的实测登记册今天也不存在（票 `pp-pricing-input-seams/04` 仍是 draft）。
- **归类**：A。
- **严重度**：低。所有权说法本身成立：收寄事实归 NO / TF，采用归 PS。
- **建议修法**：`parcel-shipment → network-routing` 补一句「有效网络收寄采用后，小包托运经信封交采用结果，触发收寄复核（UC-NR-003）」；NO→NR 一条注明实测与控制节点的读口尚待 NO 实测登记册。

### CM·L-12 MAP 没有交代 `pilotgovernance` 的位置，而 parcel-shipment 的生产归属依赖它

来源：CONTEXT-MAP

- **类别**：目标 4（MAP 列出的上下文与代码包对不上，没有说明）。
- **位置**：`CONTEXT-MAP.md` 产品边界：「产品只复用具有明确版本和兼容性约束的公司级技术组件」，以及「本地图中的限界上下文表达语言、规则、生命周期和数据所有权边界」。全图没有提到试点治理能力。
- **证据**：
  - `internal/pilotgovernance/` 有自己的 domain / ports / postgres；PS 经 `internal/parcelshipment/adapters/pilotgovernance/production_ownership.go` 消费它（UC-PS-001「生产归属」）。
  - 开发主线写「PN-08 是产品级试点治理与跨上下文应用编排，不是新的限界上下文」；模块指南写「不是另一个拥有物流业务事实的全局限界上下文」。
  - `accessidentity` 的同类处置有 ADR-0072 / ADR-0142「不进 CONTEXT-MAP」可依。
- **归类**：A。
- **严重度**：低。
- **建议修法**：在产品边界补一句：「`pilotgovernance` 是 PN-01 / PN-08 的治理记录与编排能力，不是限界上下文；它答生产归属与准入，不拥有业务事实，小包托运消费它的答复」。

---

### SC·S-10 场景四、十一：计费重量写成由结算按合同规则判断，漏掉了计价侧形成计价重量

来源：SCENARIOS

- **类别**：目标 1
- **位置**：`docs/domain/SCENARIOS.md`
  - 场景四·预期结果：「结算使用各自合同规则判断计费重量」
  - 场景四·过程第 5 步：「客户计费重量和供应商计费重量分别计算，任何一方都不覆盖另一方」
  - 场景十一·过程第 3 步：「`settlement-accounting` 另行形成客户计费重量和供应商计费重量」
- **证据**：
  - `docs/domain/GLOSSARY.md`：「`parcel-pricing` 依据计算目的形成计价重量，客户计费重量和供应商计费重量由 `settlement-accounting` 分别形成财务采用结果。」
  - `docs/domain/node-operations/CONTEXT.md`：「`parcel-pricing` 拥有依据计算目的形成的计价重量和纯评价；`settlement-accounting` 拥有客户计费重量、供应商计费重量的财务采用、费用和成本分摊结果」
  - 场景十二第 3 步写的是对的（计价形成评价与计价重量，结算保存计费重量的财务采用），各场景之间不一致。
  - 附注：`docs/domain/settlement-accounting/CONTEXT.md` 规则里那句「客户计费重量和供应商计费重量分别依据各自合同与计量规则形成。」同样含糊，是这一读法的来源，归 SA 组处理。
- **归类**：A
- **严重度**：低（场景十二与 GLOSSARY 已经写清楚）
- **建议修法**：场景四、十一统一写成「`parcel-pricing` 按各自商业依据形成计价重量，`settlement-accounting` 分别形成客户和供应商计费重量的财务采用」。

### SC·S-11 场景一：审核应付、客户运营应收写成「采用评价」直接形成

来源：SCENARIOS

- **类别**：目标 1
- **位置**：`docs/domain/SCENARIOS.md` 场景一·过程第 8 步：「`settlement-accounting` 再采用评价形成当前有效客户预估费用、客户运营应收、供应商预期成本和审核应付」
- **证据**：`docs/domain/settlement-accounting/CONTEXT.md`：审核应付是「对供应商账单主张按合同、履约和费用依据审核后确认的应付金额结果。」；客户运营应收是「依据已经确认的客户费用及其有效调整形成、用于客户对账和业务余额管理的运营金额责任。」两者都不是由采用评价直接形成的。
- **归类**：A
- **严重度**：低（同一步后半句已经说明不能拿审核应付替代预期成本）
- **建议修法**：改成「采用评价形成当前有效客户预估费用和供应商预期成本；客户费用确认后形成客户运营应收，供应商账单主张经审核形成审核应付」。

### SC·S-12 场景十四：外部资金事实的进入口没写，「不由结算生成」容易误读

来源：SCENARIOS

- **类别**：目标 1
- **位置**：`docs/domain/SCENARIOS.md` 场景十四·过程第 1 步：「关务只引用资金事实形成税费付款核对」……「两类来源事实也都不由 `settlement-accounting` 生成」
- **证据**：`docs/domain/CONTEXT-MAP.md`：「结算是外部资金事实进入本产品的唯一采用口」；`docs/domain/settlement-accounting/CONTEXT.md`：「外部资金事实进入本产品只有本上下文的采用这一口」（ADR-0137 决定四、ADR-0236）。
- **归类**：A
- **严重度**：低。字面不算错（事实本身确实由财务或支付系统产生），但读者会以为关务可以直接从财务系统拿资金事实。
- **建议修法**：补一句「外部资金事实经 `settlement-accounting` 唯一采用口进入本产品，关务按采用信封接收事实引用」。

### SC·S-13 场景三：改路写成 `network-routing` 直接形成新版本，漏掉了自动改路与改路建议的分支

来源：SCENARIOS

- **类别**：目标 1
- **位置**：`docs/domain/SCENARIOS.md` 场景三·过程第 3 步：「已经没有可行机会时才由 `network-routing` 对明确受影响包裹从当前可控节点形成新的剩余路由版本」
- **证据**：`docs/domain/network-routing/CONTEXT.md`：「是否允许自动、改善阈值的形态由策略版本声明。」与「不满足自动改路条件时，只形成改路建议，由适用授权的运营角色决定。」（ADR-0173）
- **归类**：A
- **严重度**：低
- **建议修法**：改成「由 `network-routing` 重评剩余旅程：满足策略版本的自动改路条件时形成改路决定和新版本，否则形成改路建议交授权角色决定」。

### SC·S-14 场景九：没写身份冲突处置，「由 PS 确认后建立关联」与 ADR-0210 的读法有落差

来源：SCENARIOS

- **类别**：目标 1
- **位置**：`docs/domain/SCENARIOS.md` 场景九·过程第 4 步：「`parcel-shipment` 根据可验证依据确认正式包裹身份后，建立 U 与正式包裹的版本化关联」；预期结果：「也不让节点岗位取得正式包裹身份决定权」
- **证据**：
  - `docs/adr/0210-identity-conflicts-are-disposed-by-an-operation-decision-and-the-identity-basis-reads-chain-heads.md` 越权风险点 2 点名了这一场景：「本记录把那几句读成：正式包裹及其身份由 PS 给出与确认，实物与包裹之间的版本化关联归节点」。
  - `docs/domain/node-operations/CONTEXT.md`「身份冲突处置」：「对此刻判身份冲突的作业实物，由持有这一种运营决定授予的人作出的身份结论」；UC-NO-004 以此为准。
- **归类**：A
- **严重度**：低。第 3 步的条码冲突今天就是走身份冲突处置，场景却只写了「PS 确认」一条路。
- **建议修法**：第 3 步之后补一句「冲突经持授予的人作出身份冲突处置（在 PS 交回的候选里指定一件或确认都不是），关联由节点追加版本」；预期结果改成「节点不创建、合并或替换正式包裹身份」。

### SC·S-15 开篇用「十个限界上下文」计数别处的东西

来源：SCENARIOS

- **类别**：目标 5（计数引用）
- **位置**：`docs/domain/SCENARIOS.md` 开篇：「本文档以端到端业务结果验证十个限界上下文的语言、所有权和关系」
- **证据**：AGENTS.md「改文档」：「计数与行号同构……数的都是别处的东西，被数那一侧增减一项是完全正当的改动，而改的人不会路过你这句话」。上下文的集合由 CONTEXT-MAP 定义。
- **归类**：A
- **严重度**：低
- **建议修法**：改成「验证 [CONTEXT-MAP](./CONTEXT-MAP.md) 所列各限界上下文的……」。

### SC·S-16 「文档关系」漏了两个上下文

来源：SCENARIOS

- **类别**：目标 5（引用不全）
- **位置**：`docs/domain/SCENARIOS.md`「文档关系」一节，例如「- 上下文所有权和协作关系见 [CONTEXT-MAP.md](./CONTEXT-MAP.md)。」这一节只链了 PS、NR、NO、TF、CC、VE、PP、SA 的 `CONTEXT.md`。
- **证据**：正文多处以 `party-commercial` 为责任方（场景一、二、四、五、七、十二、十三），也多处以 `collection-remittance` 为责任方（场景十四、十七），但「文档关系」里没有 `./party-commercial/CONTEXT.md` 和 `./collection-remittance/CONTEXT.md` 的链接（grep 结果为 0）。现有链接都能解析。
- **归类**：A
- **严重度**：低
- **建议修法**：补两行：参与方、合同、商业授权与渠道产品见 party-commercial；代收本金、分户账与汇付见 collection-remittance。

---

### PB·L1 开发主线四项判据重定级的结论没有补记 ADR-0100 已落地、ADR-0139..0142 已接受

来源：产品基线诸件

- **类别**：目标 1
- **位置**：开发主线重定级结论：「头一件是 ADR-0100 操作者渠道，它挡着全部运营面」；表中「客户侧接入渠道的形态待 ADR-0139 至 0142 接受，归用户」（两处）；PN-06 格「（ADR-0142 首方 webhook 仍 Proposed）」。
- **证据**：`internal/accessidentity/operator_envelope.go` 已有 `OperatorEnvelope`，端点表有操作者四个能力面；ADR-0139、0140、0141、0142 的 Status 行都是「Accepted（2026-10-19 **修订后接受**…」。表下其他各格都按「上表单元格不改写」加了补记，唯独这两件没有。
- **归类**：A
- **严重度**：低。这一段钉了 `5445341c`，属历史定级。
- **建议修法**：补两条补记，分别指向 operator-channel 系列票与 ADR-0139..0142 的接受记录。

### PB·L2 「全仓九份 `*_contract_test.go`」已与现状不符

来源：产品基线诸件

- **类别**：目标 5（数别处的东西）
- **位置**：开发主线「契约测试不计入骨架完整」：「全仓九份 `*_contract_test.go` 里，仍不 import 任何生产包的只剩 PC 两份与 SA 一份」；「数 PC 时当心 `customer_contract_test.go`」。
- **证据**：`find internal cmd -name '*_contract_test.go'` 现为 12 份，其中带 `customer_contract_test.go` 后缀的有 4 份。本节开头有一句整节锚定（「本节正文里仍留着的数是历史叙述，锚在 `f6f0029`」），但这一句本身用现行口吻（「仍…只剩」「这一类现在一个都不剩」），读者看不出它是历史数。
- **归类**：A
- **严重度**：低
- **建议修法**：就地注明「（`f6f0029` 时）」，或删掉计数只留论点。

### PB·L3 「五条编排出口……共七处」的计数已变

来源：产品基线诸件

- **类别**：目标 5
- **位置**：开发主线横切缺口：「其中撤回与资料修订各有两处（重放路径上走的是同一出口），因此共七处」；「七处 `!found` 分支里六处旁边就地记下了该约束」。
- **证据**：`internal/parcelshipment/application/` 里 `ErrInvalidShipmentRequest` 现在出现在 7 个文件的 9 处，新增了 `complete_manual_review.go`、`dispose_shipment_request.go`，都在这「五条」之外。
- **归类**：A
- **严重度**：低
- **建议修法**：删掉计数，改为「凡 `!found` 上抛 `ErrInvalidShipmentRequest` 的出口」。如果数本身是论点，就锚上取证 SHA。

### PB·L4 「紧接其上那句」的位置引用已经断了

来源：产品基线诸件

- **类别**：目标 5
- **位置**：开发主线「2026-09-02 裁决」：「上表各行的差量格与紧接其上那句「**无一张可派的机制工作票**」」。
- **证据**：那句话在「八个切片全数达标」段，它与本节之间后来插进了 09-04、09-24 两条补记、四项判据重定级表和十余条补记。现在「上表」最近指向的是重定级表，「紧接其上」也已不成立。
- **归类**：A
- **严重度**：低
- **建议修法**：改用名字引用：「三条判据定级表与『八个切片全数达标』段的『无一张可派的机制工作票』」。

### PB·L5 关务基线抄了「31 项」这个别处的计数

来源：产品基线诸件

- **类别**：目标 5
- **位置**：关务基线切片 0 行：「参数登记册中渠道或国家专属详细设计前的 31 项精确门槛参数」。
- **证据**：这个数来自 `docs/design/customs-slice-0-business-development-handoff.md` 的枚举（PAR-COM 5 + PAR-NET-01..09 + PAR-CUS-01..07 + PAR-INT-01..05 + PAR-NFR-01..05）。登记册之后经 ADR-0146 拆分、ADR-0216 退役了 `PAR-NET-09` 的拍频格，又新增了 `PAR-CUS-08`；关务基线只抄了数，没写枚举，被数的一侧改了，它不会跟着变。
- **归类**：A
- **严重度**：低
- **建议修法**：改为「切片 0 交接所列的门槛参数」，不写数。

### PB·L6 开发主线引用已被取代的 ADR-0018 作为端点落位依据

来源：产品基线诸件

- **类别**：目标 3 / 5
- **位置**：开发主线横切缺口：「端点按 ADR-0018 落在各上下文自己的 `adapters/http/`」。
- **证据**：ADR-0018 的 Status 行以「Superseded by」开头，指向 ADR-0019；而且它讲的是 `apps/` 客户端落位，不是端点。ADR-0022 的 Links 写的是「已被取代，但业务端点归各上下文 `adapters/http` 的落位结论沿用至今」；跨上下文适配器落位的权威是 ADR-0025。
- **归类**：A
- **严重度**：低
- **建议修法**：改引 ADR-0022 或 ADR-0025，或者注明「ADR-0018（已被 ADR-0019 取代，落位结论沿用）」。

### PB·L7 ADR-0016 的 Status 行没有 ADR-0146 的前向指针，而开发主线和产品故事都把它当「完整决定」引用

来源：产品基线诸件

- **类别**：目标 3
- **位置**：开发主线：「两条轨道的完整决定与被否决的替代方案见 [ADR-0016]」；PRODUCT-STORY：「两轨制的完整决定见 [ADR-0016]」。
- **证据**：ADR-0146 的 Status 行写的是「本记录**部分取代**」ADR-0016，后接「的机制半边定义与第三条判据，0016 的两条轨道与其余决定不变、正文不改写」（链接从略）；ADR-0016 的 Status 行只有「Status: Accepted」。
- **归类**：A
- **严重度**：低
- **建议修法**：在 ADR-0016 的 Status 行补前向指针（横切组 ADR 停用链一并处理）；两份产品文档在引用处加「其机制半边定义与第三条判据由 ADR-0146 部分取代」。

### PB·L8 产品故事对产品就绪的复述漏了产品策略（三类）

来源：产品基线诸件

- **类别**：目标 1（三类）
- **位置**：PRODUCT-STORY「演进路线」：「产品就绪(机制半边达标、可演示)」；「凭什么赢」：「**机制与实例分两半。**」。
- **证据**：开发主线：「**产品就绪 = 八个切片的机制半边与产品策略全部达标**」，判据四项；ADR-0146 决定一是三分。
- **归类**：A
- **严重度**：低
- **建议修法**：改成「产品就绪（机制与产品策略达标、演示动线走通）」，并把「两半」写成「机制、产品策略归产品，租户取值归租户」。

### PB·L9 导览把角色定义也划给租户登记（三类）

来源：产品基线诸件

- **类别**：目标 1（三类）
- **位置**：导览第 3 章：「实际角色、授权等级和有效期必须由租户实例登记」。
- **证据**：ADR-0146 决定二：「角色模型（有哪些角色、各能做什么）归产品策略，把人分派到角色归租户取值」；ADR-0227 决定一：「有哪些种类是产品策略，谁持哪一种是租户取值」。
- **归类**：A
- **严重度**：低
- **建议修法**：改成「角色模型由产品定义；把人分派到角色、授权等级取值与有效期由租户登记」。

### PB·L10 导览说前端没有 CI 和测试门禁，与 `ci.yml` 不符

来源：产品基线诸件

- **类别**：目标 4
- **位置**：导览 §13.2：「前端 CI、自动测试和可复现依赖安装需要完善」「且尚无前端单元/E2E 测试门禁」。
- **证据**：`.github/workflows/ci.yml` 有 `admin-web:` job（`pnpm install --frozen-lockfile`、tsc 类型检查、`node scripts/run-tests.mjs`、vite build）；`apps/admin-web/src` 下有 89 个 `*.test.*`。E2E 门禁确实没有。
- **归类**：A
- **严重度**：低
- **建议修法**：改为「前端已有类型检查、单元测试与构建门禁，尚无 E2E 门禁」。

### PB·L11 收费模型给产品基线加引号引了一句原文里没有的话

来源：产品基线诸件

- **类别**：目标 5（引文失配）
- **位置**：`PRODUCT-MONETIZATION-MODEL.md`「不建议依赖的收入」：「使[产品基线](./PRODUCT-BASELINE.md)「本产品是具有独立产品责任的软件产品」这条定位失效」。
- **证据**：产品基线的原句是「而是具有独立产品责任和领域模型的国际小包网络运营系统」；`grep -rF '本产品是具有独立产品责任的软件产品' docs/` 只命中收费模型自己。
- **归类**：A
- **严重度**：低
- **建议修法**：照原句引，或去掉引号改为转述。

### PB·L12 择优的归属写得不准：面单渠道择优归 PS，BUY 评价由计价形成

来源：产品基线诸件

- **类别**：目标 1
- **位置**：开发主线「已确认的首发范围约束」：「首发允许多份供应商价卡并存，对同一票逐候选形成 `BUY` 评价后按成本单维择优，择优由 `network-routing` 形成」；收费模型：「`network-routing` 对同一票逐候选形成 `BUY` 评价后按成本单维择优」。
- **证据**：CONTEXT-MAP 把「具体渠道选择与锁定结果」列在 `parcel-shipment` 名下，计划履约段的外包伙伴择优才归 `network-routing`。代码里「渠道择优决定」在 `internal/parcelshipment/domain/channel_selection_decision.go`，`/channel-selection-decisions` 读口也在 PS。BUY 评价由 `parcel-pricing` 形成（CONTEXT-MAP「`parcel-pricing → network-routing`」）。
- **归类**：A
- **严重度**：低
- **建议修法**：开发主线补「面单渠道服务的渠道择优由 `parcel-shipment` 形成」；收费模型改为「`parcel-pricing` 逐候选形成 `BUY` 评价，`network-routing` 按成本单维择优」。

### PB·L13 产品故事说数据态展示还要等「隔离环境读面准入」，ADR-0150 后这一说法已过时

来源：产品基线诸件

- **类别**：目标 1，兼目标 4
- **位置**：PRODUCT-STORY「今天能演示什么」：「数据态展示候合成种子与隔离环境读面准入」。
- **证据**：ADR-0150：「**三、隔离形态是真渠道缺位时的过渡，逐口退场。**」运营读面已经挂在操作者渠道（`operationsReadFace()`），隔离读只剩委托查阅一族；合成种子已在 `scripts/demo-seeds/`。
- **归类**：A
- **严重度**：低
- **建议修法**：改成「数据态经合成租户的真渠道（操作者渠道）读取，种子见 `scripts/demo-seeds/`」。

---

### PL·F7 `PAR-GOV-10` 标`待核验`，与「全部行保持空模板」及本行拆分注相抵

来源：试点三件

- **类别**：目标 1
- **位置**：登记册：
  > | PAR-GOV-10 | 独立项目运行边界 | 不共享业务数据库、部署单元或发布流程 | 待核验 |
- **证据**：
  - 登记册头注：
    > 本文是**每租户上线核对清单模板**，当前尚无租户实例，全部行保持空模板
  - 本行拆分注：
    > 留册的是该租户部署实例的数据存储、部署与发布清单
  - 登记册列出的例外行只有这一句点名的三行：
    > `PAR-COM-04`、`PAR-COM-11`、`PAR-COM-12` 三行是例外
  - `git log -S` 显示这个`待核验`是首次提交（`26cfd0a5`）时留下的，那时这一行还指产品自身架构。
- **归类**：A。
- **严重度**：低。读者会以为已经有一份租户部署清单在等核验。
- **建议修法**：改成`待提供`。产品自身不共享库表那一半留在拆分注里，或者把本行也列进例外句。

### PL·F8 ADR-0142 要求登记册增「客户 webhook 订阅」一行，没有增

来源：试点三件

- **类别**：目标 1 / 3
- **位置**：登记册 `PAR-INT-01`…`PAR-INT-13` 里没有 webhook 订阅行。`PAR-INT-06` 只改了注：
  > （首方 webhook 见 ADR-0142，已接受）
- **证据**：ADR-0142 Consequences：
  > 参数登记册：`PAR-INT-06` 行内注的「Proposed」改为已接受；增行「客户 webhook 订阅」（实例半边：某租户的客户账户 × 回调地址 × 事件过滤 × 区间）。
  
  同批接受的 ADR-0139 要求的 `PAR-INT-13` 在接受时就增了，可见增行不等实施。
- **归类**：A。
- **严重度**：低。webhook 订阅代码尚未实现；但上线核对清单里缺了这一项租户取值。
- **建议修法**：照 `PAR-INT-13` 的形状增一行，「当前登记」写`待提供：尚无租户`，最低证据写客户账户、回调地址、事件过滤与生效区间。

### PL·F9 `PAR-COM-18` 影响面写「09–17」，漏了第十册的表单票 18

来源：试点三件

- **类别**：目标 5（引用与票号）
- **位置**：登记册：
  > | 管理台各商业发布表单（admin-write-faces/09–17） |
- **证据**：`.scratch/admin-write-faces/issues/07-commercial-publication-operator-main-paths-per-register.md`：
  > 十一张子票 08–18 全 resolved
  
  以及：
  > 第十册由子票 18 补
  
  票 18 就是 `CUSTOMER_SERVICE_RULE` 的发布表单，走同一条待批准载体（ADR-0126）。
- **归类**：A。
- **严重度**：低。
- **建议修法**：改成「admin-write-faces/09–18」；或不写范围，改成「ADR-0126 所列十册的管理台发布表单」，免得再加一册时又漏。

### PL·F10 三处列举路由复核触发，都漏了 ADR-0215 的「交接进节点」

来源：试点三件

- **类别**：目标 2（同一场景只有一套结果），兼目标 1
- **位置**：
  - 范围参数表：
    > 机制已确认：接受后、有效网络收寄后和节点当前有效实测形成后分别判断
  - 登记册 `PAR-NET-14`：
    > 接受后、有效网络收寄后和形成当前有效节点实测后分别判断
  - 矩阵 `OPS-02`：
    > `UC-NR-003` 覆盖有效收寄和节点实测触发的重评
- **证据**：
  - NR CONTEXT：
    > 权威运输交接把包裹交进一个节点（`已交接`且交接声明了接收节点）时，同样从该节点复核剩余旅程
  - UC-NR-003 的输入表：
    > | 触发 | 有效网络收寄、节点控制、当前有效实测、权威运输交接把包裹交进声明的接收节点
    
    并有 `AT-NR-064..067`（ADR-0215）。
- **归类**：A。
- **严重度**：低。不是写反了，是列漏了。按 `OPS-02` 写验收的人会漏掉交接触发的复核与它的入账分格。
- **建议修法**：三处都补「权威运输交接把包裹交进声明的接收节点之后」；或者三处都改成引用 UC-NR-003「触发」一行，不再各自枚举。

### PL·F11 「进入限量生产前」的逐切片门槛没收后来增的几行会挡生产写的参数

来源：试点三件

- **类别**：目标 1
- **位置**：登记册「参数就绪门槛·进入限量生产前」：
  > PN-03 的生产候选分支必须已经确认 `PAR-COM-16`、`PAR-NET-13`、`PAR-NET-14` 和适用的 `PAR-NET-04..07`
  
  以及：
  > `PAR-GOV-03` 至 `PAR-GOV-07` 已生效，且同一范围不存在两个生产权威写入方
- **证据**：
  - 登记册 `PAR-INT-11` 自己写：
    > 对照没登的租户，运营决定与外部事实各口答「不在准入范围」
    
    （ADR-0180：治理区间要配上这本对照才生效）。
  - ADR-0212 决定十：
    > 决定四新增的那一格是一次有意的翻转：今天所有经客户包裹件号的送站识别，此后都要有预报或交接单。
    
    也就是说，客户送站一旦启用，`PAR-NET-17/18` 就是 PN-03 的前置。
  - `PAR-INT-08`（操作者与授予）同样是一切管理台决定的前置。
- **归类**：A。
- **严重度**：低。开头那句「所有 `P` 证据场景的前置参数已经确认」按「主要影响场景」一列能兜住一部分，但逐切片清单会让核对的人以为列全了。
- **建议修法**：PN-03 一条补「启用客户送站时的 `PAR-NET-17/18`」，治理一条补「`PAR-INT-11` 与 `PAR-GOV-03` 同时生效」，再加一条「`PAR-INT-08..10`、`PAR-INT-13` 按启用的渠道族确认」。

### PL·F12 范围有几句「等待真实…确认」，涵盖了登记册拆分后已划为产品策略的判断方法

来源：试点三件

- **类别**：目标 1 / 3
- **位置**：范围：
  - 参数表「初始路由及收寄/实测后复核」一行：
    > 具体策略、日历、截单、冻结边界、自动/人工规则和证据由参数登记册维护
  - 自动暂停一条：
    > 具体检测方式和阈值等待真实线路与接口能力确认
  - 分层建案一条：
    > 具体信号分类、规则阈值、响应目标和披露策略等待真实线路与客户合同确认
- **证据**：
  - 登记册 `PAR-NET-14` 拆分注：
    > 「冻结边界」「已执行前缀和并发裁决」「自动/人工改路条件与权限」的判断结构（怎么判冻结与已执行前缀、并发怎么裁、自动改路要满足什么）同属判断方法
    
    ADR-0172、0173 已接受并实施（冻结边界首版形态、自动改路由策略版本折出）。
  - 登记册 `PAR-GOV-05`：
    > 「检测条件」里不看任何租户就能判的硬风险检测（跨客户、事实完整性）划为产品策略 → [票 13][psb-13]（待核）
  - 登记册 `PAR-VIS-04`：
    > 通用异常类型目录与各类型的识别条件形态归参考配置 → [票 11][psb-11]
- **归类**：A。范围自己声明划分以开发主线为准，属于口径没跟上。
- **严重度**：低。各句都还能读成「租户选形态、填取值」，所以不是正面相反；但照字面，会把检测方式、分类目录与改路判断结构当成等租户。
- **建议修法**：三句各改成只点名租户取值，例如「选用哪种形态、阈值、启用哪些类型」；判断方法引登记册对应行的拆分注，或引开发主线。

### PL·F13 `PAR-VIS-02` 最低证据写「保守默认依据」，与「数据不足时保持未配置」用词相抵

来源：试点三件

- **类别**：目标 1
- **位置**：登记册：
  > | 历史事件分布、伙伴上报能力和保守默认依据 |
- **证据**：
  - 范围：
    > 数据不足时保持未配置或进入人工分诊；只有经过明确批准、登记并版本化的保守参数才可用于其生效范围
  - ADR-0146 决定三：没采用的判断点照旧答`未配置`。
  - AGENTS 红线：租户取值不写死为生产默认。
- **归类**：A。
- **严重度**：低。用词会诱导实现方写一个缺省观察窗口。
- **建议修法**：改成「经批准登记的保守参数及其依据」。

### PL·F14 面单渠道服务是否必验：范围、登记册与矩阵两行给出三种条件

来源：试点三件

- **类别**：目标 2（同一场景只有一套结果）
- **位置**：
  - 范围：
    > 面单渠道服务的终局规则进入本次试点的真实业务主链路；本文中以“如果包含面单渠道服务”为前提的验证条目全部适用
    
    这里不带租户条件。
  - 矩阵 `CPS-07` 的前置只看产品范围，计划证据没有 `N/A` 支：
    > | CPS-07 | 独立面单渠道服务规则 | 面单渠道服务已纳入首发服务形态（`PAR-COM-12`，[ADR-0088](../adr/0088-label-channel-service-enters-the-first-release-service-forms.md)） | `P`；未自然发生的关闭、重开与更正分支 `S` |
- **证据**：
  - 登记册 `PAR-COM-12`：
    > 该租户卖不卖本形态由其服务产品版本（`PAR-COM-05`）体现，本行留作范围核对
  - 矩阵 `CPS-06` 带租户条件和 `N/A`：
    > | CPS-06 | 标签作为业务交易 | 该租户销售面单渠道服务（`PAR-COM-12` 已纳入首发形态），或锚点线路实际使用外部标签或面单 | `P`；两者均不成立时 `N/A` |
- **归类**：A。ADR-0088 决定一定的是「`CPS-07` 由 `N/A` 改为条件阻断下的 `P` 级验证」，加上租户条件不违背它。范围那句是 ADR-0088 落文时对当时那一个试点写的。
- **严重度**：低。`CPS-07` 门槛是条件阻断，未纳入时本可走 R/S/N/A，实际影响有限。
- **建议修法**：`CPS-07` 前置改成与 `CPS-06` 同一条件（该租户销售面单渠道服务），并补「不销售时 `N/A`」；范围那句改成「该租户销售面单渠道服务时……全部适用」。

### PL·F15 `PAR-SET-03` 把参考设计里某一张源价卡的取证状态混进了模板行

来源：试点三件

- **类别**：目标 1
- **位置**：登记册 `PAR-SET-03`「当前登记」格：
  > **价格形式已由业务答复为「价卡上的绝对金额」，登记为待核验**
  
  以及：
  > `0.54`／`0.432` 只是该卡的来源痕迹
- **证据**：
  - 登记册头注：
    > 本文是**每租户上线核对清单模板**，当前尚无租户实例，全部行保持空模板
    
    又写：
    > 租户出现后按本模板逐项建立该租户的实例登记，一租户一份，不在本文里混填。
  - ADR-0146 决定四：
    > 采用记录与演示数据都不进参数登记册
  - 「该卡」是 `PP-S03-W01` 里参考设计的 UPS 源价卡（`×0.432` 在那里的表里），不是任何租户的卡。
- **归类**：A。
- **严重度**：低。它不改变本行的`待提供`状态，但模板里出现单卡事实，正是头注说不做的「混填」。
- **建议修法**：把价格形式的答复与 `0.54`／`0.432` 移回 `PP-S03-W01` 或 PP CONTEXT「源完整性门禁」。本行只留「每份价卡须证明价格形式（绝对金额或折扣派生）」这一条证据要求。范围那句（「该答复按登记规则仍为`待核验`」）随之改引 `PP-S03-W01`。

---

### AG·L1 parallel-sessions 给的 `branch-state.ps1` 跑法在现宿主上跑不起来，和 workflow.md 相抵

来源：docs/agents 与 AGENTS.md

- **类别**：1（规程单一权威）
- **位置**：`docs/agents/parallel-sessions.md`「派发前先点名」的代码块：「powershell -NoProfile -ExecutionPolicy Bypass -File scripts/branch-state.ps1 -Path <地盘目录>」
- **证据**：`docs/agents/workflow.md`：「**`scripts/*.ps1` 在 WSL 里用 `pwsh` 跑**」，以及「Windows 侧没有 git」。`scripts/branch-state.ps1` 自己的头注也给了 `pwsh -NoProfile -File scripts/branch-state.ps1 [同上参数]` 这条 WSL 跑法。
- **归类**：A
- **严重度**：低（脚本会报「不在 git 仓库里」后退出，看得见）
- **建议修法**：代码块改成 `pwsh -NoProfile -File scripts/branch-state.ps1 -Path <地盘目录>`，旁注「Windows 形见脚本头注」。

### AG·L2 parallel-sessions 文内七处位置引用方向写反（写「上面」「上节」，所指内容在后文）

来源：docs/agents 与 AGENTS.md

- **类别**：5（引用规范）
- **位置**（都在 `docs/agents/parallel-sessions.md`）：
  - 「**同一个漏法还有第二个面：开窗通知发出之后发现做不了。** 上面「会让旧调用点对不上」那条要求」，以及「**第三个面：在共享树上做 TDD 的 red 中间态，同样要开窗。** 上面「会让旧调用点对不上」那条」：所指段落在后文「工作树也会被阻断，而它不按文件所有权发生」节
  - 「这与上面「这一查要贴着 `push` 跑，不是贴着」：所指在后文「推送：推经验证的确切 SHA」
  - 「它归上面「共享接线文件」那条的处方」与「上面三类（地盘、索引文件、共享接线文件）分的都是**文件归谁**。」：「共享接线文件：占号、同笔、逐块核」在后文
  - 「**评审**：只跑派单点名的包组（上节「评审者只读……不跑全仓」那句已经这么说）。」：那句在后两节「合入前独立评审」
  - 「重放到 tip 之后带 DSN 全量**一次**（上节步骤 4）」：步骤 4 在下一节「别人分支上的活怎么进 main」
- **证据**：照文内标题次序取证于 `6b0074d0`。
- **归类**：A
- **严重度**：低（引文带了原句，搜得到，但方向词会误导）
- **建议修法**：方向词去掉，统一写成「「<节名>」节的「<原句>」」。

### AG·L3 workflow.md 跨文件写「上方路由表」，并有不带锚的别处计数

来源：docs/agents 与 AGENTS.md

- **类别**：5（引用规范）
- **位置**：
  - `docs/agents/workflow.md`「与宿主无关」：「上方路由表提到的 `/which-skill`」。workflow.md 里没有路由表，路由表在 `AGENTS.md`。
  - 同条：「`~/.cursor/skills` 那 37 个里有 22 个在 `SKILL.md` 前置声明了 `disable-model-invocation: true`」。数的是另一个仓（idp-skills）的东西，没锚 SHA。2026-10-21 复数仍是 37 / 22，所以现在不假，但守不住。
  - `docs/agents/parallel-sessions.md`：「四片并行约 8 分钟」，数的是 `scripts/ci/test-shards.sh` 的片数，现在仍是 4。
- **证据**：AGENTS.md「改文档」：「**计数与行号同构**……数本身就是论点时必须写，且必须锚住取证时的 SHA」。
- **归类**：A
- **严重度**：低
- **建议修法**：「上方路由表」改成「[AGENTS.md 的技能路由表](../../AGENTS.md#skills-路由)」。「37 / 22」改成「不少」，或者锚 idp-skills 的 SHA。「四片」改成「各片」。

### AG·L4 「-p 1」的残留措辞

来源：docs/agents 与 AGENTS.md

- **类别**：1（规程单一权威）
- **位置**：
  - `docs/agents/workflow.md` 前端切片理由：「另开一条路的理由：那条路上的每一样——隔离树、`-p 1` 全量、占号」
  - `docs/agents/parallel-sessions.md` 进 main 第 4 步：「4. 在隔离树上对那个 SHA 跑全仓 `-p 1 -count=1`（含 DSN），绿了 `git push origin <sha>:main`。2026-10-19 起这一跑用 `scripts/test-full.sh`」。先写旧做法，再追一句覆盖，照第一句执行就错了。
- **证据**：`.scratch/infra-speed/issues/01-…md`。本机全量 2026-10-19 起改走 `scripts/test-full.sh`（专用实例、socket、默认 `-p 8`）。`parallel-sessions.md`「`-p 1` 量过之后」写「本机放开，CI 不动」。`ci.yml` 的 Test 步仍是 `go test -race -count=1 -p 1 $packages`，注释已在 `9045b6fd6` 改准。
- **结论**：口径本身各处一致：本机全量用 `test-full.sh`，CI 用 `-p 1`，作者跑受影响范围。这里只是措辞残留。
- **归类**：A
- **严重度**：低
- **建议修法**：第 4 步直接写「在隔离树上对那个 SHA 跑 `scripts/test-full.sh`（含 DSN、`-count=1`）」。workflow.md 那句改成「全量」，或者加「（当时）」。

### AG·L5 workflow.md「当前宿主」里 `core.autocrlf` 前后两说

来源：docs/agents 与 AGENTS.md

- **类别**：1（规程单一权威）
- **位置**：`docs/agents/workflow.md`
  - 节首「文件工具」条：「WSL 侧 `core.autocrlf` 未设，检出一律 LF。」
  - 同节 `hash-object` 条：「不带则落到本机 `core.autocrlf`，而它在这台机器上恰好是 `true`，于是也对。」
  - `gofmt` 条：「`core.autocrlf=true` 在**暂存时**就把 CRLF 收成 LF」
- **证据**：2026-10-21 在 WSL 里跑 `git config --get core.autocrlf`，输出为空。实测 `git hash-object`（不带 `-w`，不写库）：CRLF 临时文件不带 `--path`、带 `--path internal/x.go`、等价 LF 文件，三者哈希相同（`422c2b7a…`）。结论在现宿主上照样成立，起作用的是 `.gitattributes` 的 `* text=auto`，不是 autocrlf。
- **归类**：A
- **严重度**：低（结论对，理由错。按该文自己的话，「一条理由错的纪律，第一个验出它不必要的人会把它整条丢掉」）
- **建议修法**：两处理由改成「`.gitattributes` 的 `* text=auto` / `eol=lf` 在暂存与 hash 时规范化」，并注明「上一宿主为 `core.autocrlf=true`」。

### AG·L7 workflow.md 指的 `scripts/link-skills.sh` 不在本仓

来源：docs/agents 与 AGENTS.md

- **类别**：5（断链）
- **位置**：`docs/agents/workflow.md`：「开发机 `idp-110-dev`（`/workspace/idp/`）上技能装在 `~/.claude/skills` 与 `~/.agents/skills`，是 `scripts/link-skills.sh` 建的符号链接」
- **证据**：本仓 `scripts/` 下只有 `branch-state.ps1`、`ci/`、`demo-seeds/`、`git-hooks/`、`owner-review-queue.ps1`、`test-full.sh`。这个脚本在 idp-skills 仓：`/home/tops/workspace/idp-skills/scripts/link-skills.sh`。
- **归类**：A
- **严重度**：低
- **建议修法**：改成「idp-skills 仓的 `scripts/link-skills.sh`」。

---

## 待核（未计入条数）

### B2·待核

- **Q-1（横切组）**：AGENTS.md「当前默认切片」写「租户取值与 Bento 持久化闸门仍阻断」，开发主线记的是闸门已解除，两份现行文档说法相反。不属于 ADR 对 ADR，转 `audit/cross.md`。
- **Q-2**：ADR-0152 决定八把「给结算、路由或任何下游消费试算结果的口」列为不做的，但 0179（SA 消费受理前试算）与 0189 补记（NR 走 `FormPlanGivenEstimateEvaluationsHandler`）都在消费试算结果。0189 越权风险点 5 的解释是那句只管 UC-PP-001。决定八是「不做的」范围清单，不是禁令，所以没有列为矛盾。PP owner 若不认可这个读法，再立一条。
- **Q-3**：ADR-0234 越权风险点 1：价卡收尾不过审批职责规则，而价卡发布要过（0101 决定五、六）。两份并没有作出相反的决定，只是同类门在两处宽严不一。可与 B2-05 一并裁。
- **Q-4**：ADR-0190 越权风险点 4：发生项原因与包裹计费时点两路 SELL，可能给同一件包裹形成两笔预估客户费用。今天发生项那一路没有请求面，走不到；后继记录要裁两路是否互斥。
- **Q-5**：ADR-0050 的 Open questions 以「独立面单渠道服务被 `PAR-COM-12` 排除在首发外」为前提，推出「无论哪种读法都不改变任何首发行为」。ADR-0088 已把面单渠道服务纳入首发，这个前提不再成立。「网络使用资格」这一问是否因此变成阻断，没有核。

### B3·待核

1. **同一时刻的两次控制触发，最终计划状态由到达先后决定**（NR，B 低）。
   - `0174-loading-concurrency-is-judged-by-business-time.md` 决定四：「到达顺序不许用来打破平局」。
   - `0235-…` 决定四同一时刻照判。它的越权风险点 6 承认「两次控制落在同一时刻、位置不同时，两次都判，最后落库的那次决定计划状态，取决于到达先后」。Links 写了「复核触发的同一时刻照判，理由见决定四」。
   - 两份管的对子不同：0174 管改路对装载或交接，0235 管控制触发对当前位置。但 NR CONTEXT「不按消息到达顺序」那条原则落到 0235 这一格就被打破了。要不要给同一时刻的两次控制另设一格，归 NR owner。
2. **「班次无池」在准备与分配两处意思相反**（TF，B 低）。
   - `0201-…` 决定六：「有候选但没有池或池的维度没有需求量答`容量待确认`」。代码 `capacityVerdictOf` 在无池时答 `CandidateCapacityUnconfirmed`。
   - `0203-…` 决定一：「没有登记容量池的班次不要求」「不要保障的租户不登池即可，不需要第二个开关」。
   - 照 0203 选择不登池的租户，准备永远停在`容量待确认`，而装载分配畅通。两份没有谁对谁错可判，要 TF owner 定「无池」到底是未知还是不约束。
3. **集运单元号与包裹号同串时，各消费方认法不一**（fork，B 低）。
   - 0214 决定二与 0225 都按单元判：「包裹号与集运单元号同串时按集运单元判」「标识与包裹号同串时按单元判」。
   - 0215 决定一只按「对象不是当前已接受包裹」判，NR 适配器 `reassess_on_handover.go` 不查单元册。
   - 结果是同一封交接信封上，NO 和关务当单元处理，NR 当包裹复核。
4. **0113 决定五的前提已过时**（TF，低）。
   - 「单为总单开一个 CLI 会让 TF 有两种登记入口」。
   - 但 `0201-…` 决定一后来开了「CLI 是 `parcel-transport-register`」。总单仍不进 CLI（`cmd/parcel-transport-register` 没有总单子命令），结论没有被推翻，只是理由失效。总单要不要并入 CLI，待 TF owner。
5. **非服务方的承运答复答哪一格**（fork，低）。
   - 0202 决定二写「不等答不受理」。
   - 0239 写那一格只改了名，实际答 `BOOKING_NOT_FOUND`，又说「不受理」在 TF 泛指构造门。照 0202 的字面读会以为答`未受理`。建议在 0202 的 Status 行注明。
6. **演示协议版本说法过时**（fork，低，证据 `S`）。0209 决定六：「演示的段采用与登记都指名 `v2`」。种子按 ADR-0233 已退役 v1、v2，动线指名 v3。
7. **ADR-0233 的退役语义会不会让已接受包裹的产品底座事后换版**（fork，低）。这是第 2 条的实证缺口：0233 有「已退役不参选」，退役时刻又取登记时刻，要 PC owner 确认 `CustomerServiceRuleProductBase` 按锚点是否仍选得回已退役的那一版。
8. **复核裁定已改结论，停用落在在途票**（不算矛盾，列出备查）。下面三条要等对应票进 main 才算停用链闭合：
   - `0204-…` R10：「解除采用后……撤回「已开始」回到「已提交」」，停用随在途票 57 的新 ADR。
   - `0206-…`、`0213-…` 的 R12：跨区未登区域改答未配置，停用随票 147。
   - `0205-…` 复核记录：越权风险点 2 应修订，随票 121。

### GL·待核

- **T-1 保价条件的「赔付金额规则引用」与 SA「金额规则版本册」的关系。**
  - GLOSSARY「保价条件」写「保价条件是形状与规则引用：赔付金额只由 `settlement-accounting` 在形成客户赔付时采用其规则形成」，PC 同义。
  - SA CONTEXT 没有提到保价条件。SA 的赔付金额规则写「责任结论适用的金额规则版本按租户与责任结论登记。册上没有，不形成赔付或退款金额。」（ADR-0160、ADR-0161）。
  - 代码与 SA 的 UC 里都搜不到「保价」。
  - 存疑处：同一笔保价赔付的金额规则可能有两个来源，一个是 PC 保价条件里的引用，一个是 SA 按责任结论登记的册，而且没有一处写二者怎么衔接。需要 SA、PC owner 说明设计意图，才能判 A 还是 B。
- **T-2 GLOSSARY「待识别实物」里「正式身份由 `parcel-shipment` 确认」与身份冲突处置的关系。**
  - GLOSSARY 原句：「同一条码对应多个实物或同一实物出现冲突标识时，各实物保持独立并形成身份冲突，正式身份由 `parcel-shipment` 确认。」
  - 身份冲突处置由 NO 的人在 PS 交回的候选里指定。ADR-0210 越权风险点 2 把这句读成「正式包裹及其身份由 PS 给出与确认，实物与包裹之间的版本化关联归节点」，并写了 PS owner 若坚持由 PS 确认就另起记录。
  - 这一条要看 PS owner 复核的结论，现在不判错，只建议 GLOSSARY 照 ADR-0210 的读法把这半句写清楚。

### CM·待核

1. **「不可计价不反复重试」与 ADR-0229 的续办。** `CONTEXT-MAP.md` 写「计价返回`不可计价`时，结算保存排除依据但不形成任何费用行（含零金额），也不把它当作可补齐的缺口反复重试」，PP CONTEXT 与 GLOSSARY 同句。ADR-0229 决定五的对照表写明「不可计价与评价失败只认价卡登记」：价卡一登记，就另起新一代请求再问。我倾向两者一致：PP 生命周期写了「结论改变只能来自新的规则版本，且属于另一次评价」。但 MAP 那句没写这个例外，ADR-0229 也没有对这句作停用或补注。请 SA / PP 组确认是否在 MAP 补「新一版价卡登记后经新一代请求再问，不算重试（ADR-0229）」。
2. **计价选卡与 PC「定价方案绑定」的关系。** `party-commercial → parcel-pricing` 写计价「不得选择或修改合同、政策、绑定或授权」，可计价按（租户、范围、方向、目的、时点）在自己的价卡册里解析在用价卡，`internal/parcelpricing/adapters/` 下也没有 `partycommercial` 子包（价卡登记只引用 PC 签发的方向授权标识）。PC 的「定价方案绑定」今天在评价路径上是否被读、MAP 那条的提供清单是否已经过时，没有核完，归 PC+PP 组。
3. **节点作业 → 关务的执行事实交接。** MAP 有 `node-operations → customs-compliance` 一条，NO 也交出 `node-operations.execution-fact.recorded`，但 `internal/customscompliance/adapters/inbox/` 只收 SA 的信封，`ExecutionFactView` 读的是 CC 自己的引用册。引用册由谁、经哪个口填，没有核到，可能是 C（代码缺消费者），也可能经登记面填。
4. **图上的测量边。** 图边 `NO --> PP`「当前有效实测和来源版本」与 `NO --> NR` 的实测部分，今天没有 NO 侧登记册（票 `pp-pricing-input-seams/04` 为 draft）。MAP 只在 PP 那条注了「等 `pp-pricing-input-seams/04`」，NR 那条没注。这是在描述模型，不判文档错，列出供横切组决定是否加注。

### SC·待核

- **Q-01 场景十六第 2、9 步的软超配**：场景写「超出已确认范围的 `30kg` 只有在相应容量维度属于软限制、仍在配置阈值内并取得适用授权后，才能扩大有效预占」和「有效预占和获批分配 `380kg`」。这与 TF CONTEXT「商业可售容量或运营缓冲是软限制，只能在按班次、线路、供应商和容量维度配置的阈值及审批范围内超配。」以及 UC-TF-004「| `AT-TF-043` | 软容量超配获授权 | 形成超配缺口和授权引用，不覆盖原可用量 |」大体相容；但 ADR-0203 写「已登记的池是硬限制，绕过它就是超配」，又写「软容量的受控超配仍归 psb/09，不在本记录。」，代码头注也是「软容量超配走版本化阈值与缺口处置，另票。」（`internal/transportfulfillment/domain/transport_schedule_capacity.go`）。现行模型能不能表达软池、扩大预占还是只记缺口，要等 psb/09（needs-triage）定；建议 TF 组一并核。
- **Q-02 场景二第 18 步「同级或更高重开权限」**：PS CONTEXT 写「重开必须由同级或更高授权角色形成并带原因」，pc-gaps/13 裁决①「首发机制不算等级序」（`permits` 用 `==` 比较）。首发是否只认同级、场景要不要标注，没有核到判断细节。
- **Q-03 ADR-0090 引用的 SCENARIOS 原句已不存在**：ADR-0090 引用「防重复、结果待确认、查询、重打、作废、替换和退款分别处理」，现行 SCENARIOS 的对应句是「查询、重打、渠道作废、替换和渠道退款分别判断，不能以重新生成文件代替渠道业务操作。」ADR 正文不能改，交 ADR 停用链或横切组判断是否需要注记。
- **Q-04 场景三第 3 步与场景一第 6 步「由谁判断同一线路是否还有运输机会」**：场景写由 TF 判断，与 TF CONTEXT 规则「同一线路仍有满足时间窗口和客户承诺的后续机会时，可以重新分配；已经没有可行机会时才请求 `network-routing` 重评剩余旅程。」一致；但 TF CONTEXT 边界一节写「单次分配失败只提供容量和机会结果，由路由上下文判断是否重评剩余旅程。」。这是 TF CONTEXT 自身的张力，不是场景的问题，交 TF 组。
- **Q-05 场景十四第 6 步把「代收应收、已收」列为分户账结果**：场景原句「`collection-remittance` 关联四层事实后形成代收应收、已收、渠道在途、待清分、应付客户、短款、溢款和汇付分户账结果」，CR CONTEXT 的资金位置是「封闭词表」，里面没有这两格。TF、SA CONTEXT 用的是同一串词，属于跨文件措辞问题，交 GLOSSARY / CR 组核。

### PB·待核

1. 关务基线「申报就绪 | UC-CC-003 | 七类业务门禁完整」里的「七类」：UC-CC-003 正文没有直接写「七类」，没能逐类对上。
2. 收费模型「API 用量的事实不存在」：代码里确实没有调用计量；但 ADR-0140 Decision 三已定了 `QUOTA_EXCEEDED`（「等下个周期或找租户运营提额」），等于默认存在按周期的额度计量概念。是否应在收费模型里提一句，待判。
3. 产品基线开篇「面向自己经营国际小包网络的物流企业……并组织实际网络履约」与开发主线「目标客户群」第一层「不一定持有干线运力或场站……纯面单渠道转售……无自营现场作业」之间有张力。ADR-0088 之后纯面单转售也是首发形态，定位句是否要放宽，待产品 owner 判。
4. 导览 §13.2「Windows 与 WSL 生成的命令 shim 不完全兼容；本机 `pnpm build` 不能稳定复现」：属本机环境事实，没有复现。
5. 开发主线「2026-09-02 裁决」：「`internal/architecture/production_wiring_baseline.txt` 冻着 32 条」——该文件现在 633 行（含注释），没有逐行数条目。补记带日期，不作为发现。

### PL·待核

1. **`PAR-SET-03` 说「加宽不使历史回放失效」，代码只认当前一个规范化版本。**
   - 登记册：
     > 内容摘要此后携带规范化版本、只在同一版本内可比，价表族可随证据分批加宽而不使历史回放失效
   - ADR-0014 Consequences：
     > 回放路径按版本分支
   - 代码只有：
     > const canonicalizationVersion = "PPC-5"
     
     `evaluation.go` 写：
     > 重放路径会显式拒绝它，而不是静默当成数据损坏。见 ADR-0014。
     
     PPC-2 到 PPC-4 的快照都在重建门被拒。
   - 若库里没有按旧版本存下来的评价，这与 `fingerprint.go` 头注「按它记录的评价还能重放期间持续支持」并不矛盾。是否有存量评价、算 C 还是 B，交 PC+PP 组核。
2. **范围说客户看金额走 SA，ADR-0152 让运营用试算答客户询价。**
   - 范围：
     > 需要让客户看到金额时，使用 `settlement-accounting` 的客户预估费用，它明确不是运营应收
   - ADR-0152 Context：
     > 运营却没有任何入口：客户问「这个重量、这个尺寸、寄到这个邮编多少钱」，产品答不了。
     
     它据此立了 PP 试算，口径是不形成承诺、不进结算。
   - 范围那句若只指已成形的包裹，两边不冲突；若指一切对客金额，就与 ADR-0152 相抵。需要 PS/PP owner 定句意。
3. **价表族「激活」被挡在租户证据之后。**
   - 登记册：
     > 没有取得某一族的真实价卡证据，该族不得被激活，也不得进入 PN-07 生产候选
   - PP CONTEXT：
     > 哪些价表族在首发被激活由真实参数决定，不由本词条声明。
   - 代码里没有按族激活的门（三族都已实现、都可登记）。ADR-0146 与开发主线写「不约束能力范围」。若「激活」指某租户的价卡能否进生产候选，两边一致；若指产品是否支持该族，就与 ADR-0146 相抵。属 PP CONTEXT 的口径，交 PC+PP 组。

### AG·待核

- **Q1（交横切或产品组）**：`AGENTS.md`「当前默认切片」写「租户取值与 Bento 持久化闸门仍阻断」，与 `docs/design/parcel-go-first-consumer-slice-decision-brief.md` 闸门表的「保持阻断：……登记 `B-06` 是另一步闸门动作」一致。但 `docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md` 写「Bento 闸门解除（`v0.1.0-rc.2` 经 `PBC-01`/`PBC-06` 锁定）」。AGENTS.md 只是复述，矛盾在产品基线和设计简报之间，不在本组范围。
- **Q2**：Claude Code 会话能否调用 AGENTS.md 路由的 idp-skills（`/implement`、`/tdd`、双轴 `/code-review`）。本机观察：`~/.claude/skills` 下没有，项目里也没有 `.claude/skills`。Claude Code 自带的 `/code-review` 是另一个 skill，没有 Standards/Spec 两轴。没在 Claude Code 会话里实测调用，归 M6 的裁定。
- **Q3**：前端切片在共享树 `main` 上「做一条提一笔，提了就推」（`git push origin main`），后端推送方又要「3. 把共享 `main` **快进**到那个 tip（ff，不产生新对象）」，而规矩是「`main` 只有推送方推，**自己的分支自己推，每笔提交后就推**」。两边都承认前端例外（parallel-sessions「2026-09-21 起，diff 全在 `apps/admin-web/**` 的票不走本文这条路」），可两者同时在途时共享树的 main 怎么快进、谁的推送先后，没有写。没撞到实例，列在这里，不算发现。

## 附表 A　ADR 停用链全量核对（来向停用 72 次，实测于 `6b0074d0`）

取法：脚本从全部 228 份 ADR 的 `Status` 行、头部「Partially suspended by / Amended by」行与正文里抽出「X 的条款被 Y 停用 / 收窄 / 限定」的每一对，误报（只是「读过 ADR-X」、引文里带到 ADR-X、写明「不停用」的）人工剔掉；「回答越权风险点、无条款停用」的指针不算停用，不列。逐项含义：**被停方指针所在**——前段＝`<!-- 索引注记 -->` 之前，注记＝之后（README 由它生成），头部行＝Status 下面那行英文标签；**注记里的被停原句**——注记引号里的被停条款能否在被停 ADR 正文逐字找到；**停用方的声明**——停用方正文（含「对已接受记录的作用」「与已接受决定的关系」、补记、复核记录）有没有写这次停用。凡有缺口的格都对应正文某条发现（X-01、X-03 至 X-06）。

| 被停方 | 停用方 | 被停方指针所在 | 注记里的被停原句 | 注记写「其余…不变」 | 停用方的声明 | 被停方 Links 指停用方 | 停用方 Links 列被停方 |
|---|---|---|---|---|---|---|---|
| ADR-0012 | ADR-0013 | 头部行 | — | — | 正文（Context「只限定该句的适用范围」） | 否 | 是 |
| ADR-0014 | ADR-0177 | 注记、头部行 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0014 | ADR-0201 | 注记、头部行 | 有，逐字 | 是 | 正文 | 是 | 否 |
| ADR-0014 | ADR-0202 | 注记、头部行 | 有，逐字 | 是 | 正文 | 是 | 否 |
| ADR-0014 | ADR-0213 | 注记、头部行 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0022 | ADR-0055 | 前段、注记 | 有，一处非逐字（X-05） | 是 | 正文 | 是 | 是 |
| ADR-0022 | ADR-0140 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0025 | ADR-0027 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0027 | ADR-0029 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0048 | ADR-0179 | 注记、头部行 | 无引文，转述（X-05） | 只写「决定一、二不变」 | 正文 | 是 | 是 |
| ADR-0052 | ADR-0053 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0053 | ADR-0068 | 前段、注记 | 有，两处非逐字（X-05） | 是 | 正文 | 否 | 是 |
| ADR-0053 | ADR-0148 | 前段、注记 | 有，两处非逐字（X-05） | 是 | 正文 | 是 | 是 |
| ADR-0064 | ADR-0156 | 前段、注记 | 有，逐字 | 等效（「决定一至四…不变」） | 正文 | 是 | 是 |
| ADR-0068 | ADR-0148 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0072 | ADR-0100 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0072 | ADR-0139 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0075 | ADR-0215 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0078 | ADR-0091 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0081 | ADR-0158 | 前段、注记 | 有，逐字 | 等效（「决定六其余各格仍有效」） | 正文 | 是 | 是 |
| ADR-0083 | ADR-0155 | 注记 | 有，逐字 | 等效（「阶段评审格与无租户维仍有效」） | 正文 | 否 | 是 |
| ADR-0085 | ADR-0100 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0085 | ADR-0139 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0086 | ADR-0106 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0091 | ADR-0150 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0094 | ADR-0106 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0098 | ADR-0204 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0100 | ADR-0139 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0100 | ADR-0149 | 前段、注记 | 有，「同条…不变」一处非逐字 | 是 | 正文 | 是 | 是 |
| ADR-0100 | ADR-0150 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0101 | ADR-0139 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0104 | ADR-0176 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0106 | ADR-0139 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0126 | ADR-0178 | 注记、头部行 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0149 | ADR-0151 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0149 | ADR-0236 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0151 | ADR-0187 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0151 | ADR-0230 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0151 | ADR-0236 | 前段、注记 | 有，逐字 | 是 | 正文（对已接受记录的作用列表） | 是 | 是 |
| ADR-0164 | ADR-0208 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0169 | ADR-0190 | 注记 | 有，非逐字（X-04） | 是 | 正文 | 是 | 是 |
| ADR-0169 | ADR-0198 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0170 | ADR-0190 | 注记 | 有，非逐字（X-04） | 否 | 正文 | 是 | 是 |
| ADR-0175 | ADR-0217 | 前段、注记 | 有，逐字 | 是 | 仅 Status 行 | 是 | 是 |
| ADR-0181 | ADR-0182 | 无 | — | — | 正文 | 否 | 无 Links 节（正文有链接） |
| ADR-0183 | ADR-0187 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0184 | ADR-0185 | 注记、头部行 | 有，逐字 | 是 | 正文（Context/Decision 改键） | 是 | 无 Links 节 |
| ADR-0184 | ADR-0195 | 注记、头部行 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0184 | ADR-0212 | 注记、头部行 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0186 | ADR-0199 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 否 |
| ADR-0190 | ADR-0229 | 注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0195 | ADR-0211 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0195 | ADR-0212 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0196 | ADR-0199 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 否 |
| ADR-0196 | ADR-0210 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0198 | ADR-0229 | 注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0199 | ADR-0210 | 前段、注记 | 有，「结论…维持」一处非逐字 | 是 | 正文 | 是 | 是 |
| ADR-0199 | ADR-0211 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0199 | ADR-0212 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0200 | ADR-0229 | 注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0201 | ADR-0217 | 前段、注记 | 有，逐字 | 是 | 仅 Status 行 | 是 | 是 |
| ADR-0202 | ADR-0239 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0205 | ADR-0225 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0207 | ADR-0209 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0207 | ADR-0218 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0208 | ADR-0229 | 注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0209 | ADR-0218 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0210 | ADR-0211 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0214 | ADR-0220 | 前段、注记 | 有，「不受影响」一处非逐字 | 是 | 正文 | 是 | 是 |
| ADR-0215 | ADR-0235 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0216 | ADR-0230 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |
| ADR-0218 | ADR-0221 | 前段、注记 | 有，逐字 | 是 | 正文 | 是 | 是 |


结论：脚本按「停用 / 收窄 / 改指 / 消解 / 限定」字样能抽到的 72 次里，被停方完全没有前向指针的 1 次（0181 ← 0182，X-01）；指针只在头部行、不进索引的 1 次（0012 ← 0013，X-03）；其余 70 次被停方注记都在。注记引文非逐字的见 X-05；Links 缺口见 X-06。停用方只在 Status 行声明、正文无等效段的 2 次（0175、0201 ← 0217）——0217 是已接受记录，正文不改，被停方注记已写全原句与适用场景，不另立发现。

**这张表不是缺口的全部。** 后来的记录用「改为 / 接上 / 扩成 / 按口解 / 换成」改掉前一份的条款、却不用「停用」二字的，脚本抽不到，由 ADR 互相矛盾两簇与产品基线组按话题读出，全部是「被停方无前向指针」：B2·B2-01（0047 ← 0122、0127）、B2·B2-07（0044 ← 0080）、B2·B2-09（0081 决定六另三格 ← 0156、0179、0157）、B2·B2-11（0169 ← 0190 时点）、B2·B2-12（0122→0125、0125→0132、0127→0129 只记在复核记录）、B2·B2-13（0017 ← 0026）、B3·4（0052、0053、0068 ← 0146 决定七）、B3·5（0149 ← 0230）、B3·7（0216 决定一 ← 0230）、B3·9（0078 决定四 ← 0150）、B3·10（0052 决定一 ← 0148）、B3·11（0207 决定四 ← 0218）、B3·13（0078 决定五 ← 0091、0149）、PB·L7（0016 ← 0146）、X-11（0055 决定五 ← 0149 决定四）。修订时这些与表里的缺口同一种修法：被停方注记写被停条款原句与适用场景、Links 补指针。B1 簇（横切技术与治理）的互相矛盾专审因额度中断没有出报告，见报告开头「覆盖缺口」。

## 附表 B　infra-speed/07：Status 行里同一件事写了两遍

### B1　来向停用两写（07 要合的那一类）：40 份 ADR、54 次停用

ADR：0022、0025、0027、0052、0053、0064、0068、0072、0075、0078、0081、0085、0086、0091、0094、0098、0100、0101、0104、0106、0149、0151、0164、0169、0175、0183、0186、0195、0196、0199、0201、0202、0205、0207、0209、0210、0214、0215、0216、0218。

其中 0202、0205、0215、0216 的前段写的是「越权风险点 N 已由 ADR-… 回答……由此部分停用……（被停原句见索引注记）」，注记里另写被停条款——同一次停用，前段只是简述；合并时保留注记那一份即可。0151、0149、0202、0215、0216 的重复是 2026-10-20/21 新近写下的（见 X-02）。下面逐次列两份措辞；「脚本提示」只比对了日期、「适用场景」「其余各条不变」三样是否只在一边出现，注记把几次停用写在同一段时提示会串，合并时以人工核对为准。

#### ADR-0022（HTTP 状态码只回答「有没有形成答案」，业务判别一律进响应体）

- **← ADR-0055**
  - 标记前段：**部分停用**：Decision 中「两项未决期间，业务端点可以存在并由测试替身驱动，但不进 `cmd/parcel-api` 的装配」的末段「但不进 `cmd/parcel-api` 的装配」已由 [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) 停用——端点改为带「未配置即拒」Intake 进装配；该句要防的「默认实现铸造信封」由未配置格保留并加强，同段「禁止在它们决定前落地任何默认实现」原文有效，其余各条不变）
  - 索引注记：**部分停用**：其 Decision 中「两项未决期间，业务端点可以存在并由测试替身驱动，但不进 `cmd/parcel-api` 的装配」的末段「但不进 `cmd/parcel-api` 的装配」已由 [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) 停用——业务端点改为带「未配置即拒」Intake 进装配，未配置自成一格如实作答。该句前段（端点可存在并由测试替身驱动）与同段「禁止在两项未决前落地任何默认实现」不变，其余各条不变。
  - 脚本提示（只在一边的事实，合并时人工核）：无

- **← ADR-0140**
  - 标记前段：出队，交给人，不自动重发」对 `429` 的适用已由 [ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md) Decision 三停用，**适用场景**仅限首方与集成两族的限流答复（`429` + `Retry-After`，按 `Retry-After` 重发即正当）；其余 4xx 照旧。
  - 索引注记：出队，交给人，不自动重发」对 `429` 的适用已由 [ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md) Decision 三 停用，**适用场景**仅限首方与集成两族的限流答复；其余各条不变。
  - 脚本提示（只在一边的事实，合并时人工核）：无

#### ADR-0025（跨上下文调用的适配器落在消费侧，翻译职责由它独占）

- **← ADR-0027**
  - 标记前段：Status: Accepted（Decision 中「适配器为此需要的实例半边协作者，其接口定义在适配器包内，**不进消费方 `ports`**」一句已由 [ADR-0027](./0027-multi-step-cross-context-protocol-state-held-by-the-provider.md) **在跨上下文多步协议场景**停用，其余各条不变）
  - 索引注记：**部分停用**：其 Decision 中「适配器为此需要的实例半边协作者，其接口定义在适配器包内，不进消费方 `ports`」一句已在跨上下文多步协议场景由 [ADR-0027](./0027-multi-step-cross-context-protocol-state-held-by-the-provider.md) 停用——该协作者已由提供方以第二阶段用例提供，适配器直接调用，不另定接口。其余各条不变。
  - 脚本提示（只在一边的事实，合并时人工核）：无

#### ADR-0027（跨上下文多步协议的中间状态由提供方按解析标识保留，消费方端口按协议阶段分方法）

- **← ADR-0029**
  - 标记前段：Status: Accepted（Decision 中「不一致按`输入未受理`处理」一句所指定的取值已由 [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) 改指为`依据未解析`，仅限「按标识取回原状态失败」这一步；该句要防的「解析标识不得成为一张能力凭证」保留且被加强，本记录其余各条不变）（Context 原按行号引用的那些坐标已在本记录被实现时移位，现已逐条改写为符号名与引文——**只改坐标，未动任何决定文字**。
  - 索引注记：**部分停用**：其 Decision 中「不一致按`输入未受理`处理」一句所指定的取值已由 [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) 改指为`依据未解析`——该句要防的「解析标识不得成为一张能力凭证」不但保留且被加强，原先只挡读取，现在连回答也不泄。**适用场景**：仅限「按标识取回原状态失败」这一步；该句其余部分与本记录各条不变。
  - 脚本提示（只在一边的事实，合并时人工核）：「适用场景」只在注记；「其余各条不变」只在前段

#### ADR-0052（网络证据端口增设「未配置」格，网络定义登记册的模式属机制半边）

- **← ADR-0053**
  - 标记前段：**部分停用**：Decision 四的第一句「它要能承载……全部事实族」已由 [ADR-0053](./0053-network-fact-families-are-derived-not-registrable.md) 停用——九族是每次判断的推导结果，不可登记；其余各条不变）
  - 索引注记：**部分停用**：其 Decision 四中「它要能承载 `NetworkEvidence` 与 `InitialRouteEvidence` 两个结构今天已声明的全部事实族」一句已由 [ADR-0053](./0053-network-fact-families-are-derived-not-registrable.md) 停用——九族以每次判断生成的 `CandidateID` 为轴、且判断键里没有目的地维，它们是推导结果不是可登记的目录行。其余各条（三格端口、专格未决原因、修订由登记册派生、空册与空集合可分辨）不变。
  - 脚本提示（只在一边的事实，合并时人工核）：「其余各条不变」只在前段

#### ADR-0053（网络事实族是推导结果不可登记；登记册只登定义存在，无解析层时一律答未配置）

- **← ADR-0068**
  - 标记前段：**部分停用**：Decision 三的「现在不建表」半句已由 [ADR-0068](./0068-versioned-network-catalog-structure-precedes-rule-content.md) 停用——目录结构由 CONTEXT 硬句定死、不依赖 `PAR-NET-*` 取值，先行落表；Decision 四「查无登记 → `未配置`」一格已由 [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定六停用，自票 routing-first-cut/07 落地的那一笔起——`未配置`改由目录修订锚与判断时点适用于该服务目的的路由策略版本答，定义登记册（`0007`）不再被读，表按迁移不可改的纪律留着；同条第二格「查到登记但本进程没有解析层 → 响亮报错」（今天读作「登记了而解不出」）与第三格「禁止：查到登记就答 `configured=true` 带空事实族」不变。
  - 索引注记：**部分停用**：其 Decision 三中「现在不建表」半句已由 [ADR-0068](./0068-versioned-network-catalog-structure-precedes-rule-content.md) 停用——目录**结构**由 CONTEXT 硬句定死不依赖取值，先行落表；「不从事实族倒推」「内容形态不替租户拟」与其余各条不变。
  - 脚本提示（只在一边的事实，合并时人工核）：无

- **← ADR-0148**
  - 标记前段：**部分停用**：Decision 三的「现在不建表」半句已由 [ADR-0068](./0068-versioned-network-catalog-structure-precedes-rule-content.md) 停用——目录结构由 CONTEXT 硬句定死、不依赖 `PAR-NET-*` 取值，先行落表；Decision 四「查无登记 → `未配置`」一格已由 [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定六停用，自票 routing-first-cut/07 落地的那一笔起——`未配置`改由目录修订锚与判断时点适用于该服务目的的路由策略版本答，定义登记册（`0007`）不再被读，表按迁移不可改的纪律留着；同条第二格「查到登记但本进程没有解析层 → 响亮报错」（今天读作「登记了而解不出」）与第三格「禁止：查到登记就答 `configured=true` 带空事实族」不变。
  - 索引注记：其 Decision 四「查无登记 → `未配置`」一格已由 [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定六停用（自 routing-first-cut/07 落地起）——`未配置`改由目录修订锚与适用于该服务目的的路由策略版本答，定义登记册不再被读；同条「登记了而解不出 → 响亮报错」与「禁止带空事实族答已配置」两格不变，其余各条不变。
  - 脚本提示（只在一边的事实，合并时人工核）：无

#### ADR-0064（初始路由适用性闭包标识从已接受解析标识回指）

- **← ADR-0156**
  - 标记前段：部分停用：后果里「可达性那条链的 `ReachabilityClosureIdentity` 不变」一句，以及 Context 里「`ReachabilityClosureIdentity` 仍属实例半边（可达性判断键同样没有解析标识，且本记录不管 UC-NR-002）」一句，已由 [ADR-0156](./0156-reachability-closure-identity-comes-from-the-adopted-resolution.md) 停用。
  - 索引注记：**部分停用**：后果里「可达性那条链的 `ReachabilityClosureIdentity` 不变」一句，以及 Context 里「`ReachabilityClosureIdentity` 仍属实例半边（可达性判断键同样没有解析标识，且本记录不管 UC-NR-002）」一句，已由 [ADR-0156](./0156-reachability-closure-identity-comes-from-the-adopted-resolution.md) 停用。
  - 脚本提示（只在一边的事实，合并时人工核）：无

#### ADR-0068（版本化网络目录结构先行——七表由 CONTEXT 硬句推导，内容列与折叠规则等 ）

- **← ADR-0148**
  - 标记前段：**部分停用**：决定六「三个证据视图仍不读本目录」一句与其半句「目录修订与视图修订的合流属解析层设计，届时再裁」已由 [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定六停用，自票 routing-first-cut/07 落地的那一笔起——可达性证据视图从本目录折出事实，`未配置`改由目录修订锚与判断时点适用于该服务目的的路由策略版本答，视图修订就是目录修订锚，0007 的定义登记册不再被读；初始路由证据视图同一条判法答`未配置`，已配置时在 routing-first-cut/09 之前照旧响亮上抛。
  - 索引注记：**部分停用**：其决定六「三个证据视图仍不读本目录」一句（连同「目录修订与视图修订的合流属解析层设计，届时再裁」半句）已由 [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定六停用（自 routing-first-cut/07 落地起）——可达性证据视图从本目录折出事实，视图修订就是目录修订锚。其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：无

#### ADR-0072（接入渠道与凭据验证归共享接入身份能力，登记册形状等真渠道证据）

- **← ADR-0100**
  - 标记前段：**部分停用**，2026-09-03：Decision 2 中「登记册表结构与凭据形态在 `PAR-INT-01` 最低证据（该租户渠道的现行流程）到位前不立」一句已由 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 收窄——**适用场景**限客户接入渠道的登记行与凭据；管理台运营操作者渠道的登记册结构与凭据形态由产品定义、现在就立。
  - 索引注记：**部分停用**：其 Decision 2「登记册表结构与凭据形态在 `PAR-INT-01` 最低证据（该租户渠道的现行流程）到位前不立」一句已由 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 收窄——**适用场景**限客户接入渠道的登记行与凭据；管理台运营操作者渠道的登记册结构与凭据形态由产品定义、现在就立。
  - 脚本提示（只在一边的事实，合并时人工核）：日期只在前段 2026-09-03

- **← ADR-0139**
  - 标记前段：**部分停用**，2026-10-19：Decision 2「登记册表结构与凭据形态在 `PAR-INT-01` 最低证据到位前不立」的适用场景再由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一收窄为**租户既有渠道的适配**；首方客户族的册与凭据形态由 ADR-0139 Decision 二定。
  - 索引注记：**部分停用**（2026-10-19）：Decision 2「登记册表结构与凭据形态在 `PAR-INT-01` 最低证据到位前不立」已由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一 停用，**适用场景**再收窄到租户既有渠道的适配；其余各条不变。
  - 脚本提示（只在一边的事实，合并时人工核）：无

#### ADR-0075（客户地址随判断请求在请求期携带过界，network-routing 不建读取端口）

- **← ADR-0215**
  - 标记前段：部分停用：[ADR-0215](./0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md) 决定二（被停条款：决定一「`network-routing` 不建立读取 `parcel-shipment` 地址的端口」；适用场景只限受理之后、不由小包托运信封触发的路由复核，NR 经消费侧适配器读受理基线那一版提交的地理投影、只到地理维；适用场景与停用前提见 adr/README 该行，其余各条不变，本记录正文不改写）
  - 索引注记：**部分停用**（2026-10-19）：决定一「`network-routing` 不建立读取 `parcel-shipment` 地址的端口」已由 [ADR-0215](./0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md) 决定二停用。**适用场景**：仅限受理之后、不由小包托运信封触发的路由复核（今天只有权威运输交接进入节点这一种），NR 经消费侧适配器按（租户，包裹）取受理基线那一版提交的地理投影，只取用地理维，地址本体不取用、不落——停用理由的前提是受理基线一经固定不再改写，读回的那一版与判断键上的基线同版，本记录防的异版窗口在这里不存在；可达性判断、初始路由与收寄采用触发的复核照旧随请求携带。决定二、决定三与决定一其余部分不变，其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：日期只在注记 2026-10-19

#### ADR-0078（隔离环境运营查阅按装配注入放行——合成租户显式入参、缺省朝拦，写路径与客户查阅面）

- **← ADR-0091**
  - 标记前段：**部分停用**，2026-09-02：Decision 四中「按环境选择的只有装配点上查阅行的 Intake 一件事」一句已由 [ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 停用，适用面改由入格判据裁定；同条另两句与其余各条不变）
  - 索引注记：**部分停用**：其 Decision 四中「按环境选择的只有装配点上查阅行的 Intake 一件事」一句已由 [ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 停用——适用面由枚举改为入格判据（注入值全为 `SYN-` 合成、不采信自报身份、生产装配无此路径），写路径的命令面 Intake 与生产归属范围目录据此入格。同条另两句（不得据此再添 demo/mode 类全局开关或第二个 main；pgtest 环境处理通例不变）与其余各条不变。**适用场景**：仅限满足那三条判据的缝；`/customer-tracking-view` 的排除判据（客户维是调用方自己的身份主张）不因判据化而松动
  - 脚本提示（只在一边的事实，合并时人工核）：日期只在前段 2026-09-02；「适用场景」只在注记

#### ADR-0081（接受判断由信封驱动——提交落库即交出「委托已提交」，推进落在派发一拍；提交事务随）

- **← ADR-0158**
  - 标记前段：部分停用：决定六里「结算账户目录留 nil」这一格由 [ADR-0158](./0158-settlement-account-register-is-an-immutable-tuple.md) 停用——生产装配接结算账户登记册。
  - 索引注记：**部分停用**：决定六「结算账户目录留 nil」由 [ADR-0158](./0158-settlement-account-register-is-an-immutable-tuple.md) 停用；决定六其余各格仍有效
  - 脚本提示（只在一边的事实，合并时人工核）：无

#### ADR-0085（登记册配置写面进端点表带未配置格——写表单属产品能力，CLI 保留为受控批量口；）

- **← ADR-0100**
  - 标记前段：**部分停用**，2026-09-03：Decision 二补记中「两族各自要过 `PAR-INT-01` 的真证据门」一句的前半，与 Decision 二「本记录新增的登记端点也在被拦之列」一句，已由 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 停用——**适用场景**限管理台操作者族与登记册配置写面；「不得互相顶替」后半保留，客户业务面照旧过 `PAR-INT-01` 并被 ADR-0055 Decision 五两项拦着。
  - 索引注记：**部分停用**：其 Decision 二补记「两族各自要过 `PAR-INT-01` 的真证据门」一句的前半，与 Decision 二「本记录新增的登记端点也在被拦之列」一句，已由 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 停用——**适用场景**限管理台操作者族与登记册配置写面；「不得互相顶替」后半保留，客户业务面照旧过 `PAR-INT-01` 并被 ADR-0055 Decision 五两项拦着。
  - 脚本提示（只在一边的事实，合并时人工核）：日期只在前段 2026-09-03

- **← ADR-0139**
  - 标记前段：**部分停用**，2026-10-19：Decision 二同日补记「两族各自要过 `PAR-INT-01` 的真证据门，不得互相顶替」的客户半由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一收窄为**租户既有渠道的适配**（首方族不过该门）；「不得互相顶替」照旧。
  - 索引注记：**部分停用**（2026-10-19）：Decision 二补记「两族各自要过 `PAR-INT-01` 的真证据门」的客户半已由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一 停用，**适用场景**收窄到租户既有渠道的适配；其余各条不变。
  - 脚本提示（只在一边的事实，合并时人工核）：「适用场景」只在注记

#### ADR-0086（等待人工复核是入账暂停——暂停与等待态同事务落库，续办由「复核已完成」信封另行驱）

- **← ADR-0106**
  - 标记前段：部分停用：Context 把`等待受控补充`判在「回滚重投是对的」一侧的那一半，已由 [ADR-0106](./0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md) 停用——该等待态改为提交入账、由「新提交版本已形成」信封驱动续办；`等待内部续办`的分侧与 Decision 一至四不变）
  - 索引注记：**部分停用**：其 Context 把`等待受控补充`判在「回滚重投是对的」那一侧的分侧——原句「等待受控补充 / 等待内部续办：重投重跑同一轮，等的依赖（客户新提交版本、抖动的内部依赖）会自己回来」中关于**客户新提交版本**的那一半——已由 [ADR-0106](./0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md) 停用，理由是代码证实新版本不会自己回来、回来了也不是在途那封信封受益。**适用场景**：只停`等待受控补充`那一半；`等待内部续办`的分侧与 Decision 一至四不变，其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：「适用场景」只在注记

#### ADR-0091（隔离形态从查阅面扩到写路径，按分级开关放行——入格判据取代枚举；写面有持久化，可）

- **← ADR-0150**
  - 标记前段：**部分停用**：决定六「隔离形态不因真渠道出现而自动退场」一句已由 [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 停用——隔离形态是真渠道缺位时的过渡，某一口真渠道 Intake 落地即在同一笔撤下该口的隔离放行；同句「也不因它出现而获得任何真实租户」与其余各条不变
  - 索引注记：**部分停用**：其决定六「隔离形态不因真渠道出现而自动退场」一句已由 [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 停用——隔离形态是真渠道缺位时的过渡，某一口真渠道 Intake 落地即在同一笔撤下该口的隔离放行。
  - 脚本提示（只在一边的事实，合并时人工核）：「其余各条不变」只在前段

#### ADR-0094（未决重投与否由领域的 `ResumePath` 决定，并增设「等运营登记」第四格）

- **← ADR-0106**
  - 标记前段：部分停用：Decision 三对 `ResumeByCustomerSupplement` 的「维持回滚重投」一格，已由 [ADR-0106](./0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md) 改为提交入账——那一格写明「另立取证票，不在本记录裁」，取证票 first-tenant-runway/09 的三问已从代码答出；Decision 一、二、四、五与其余三格不变）
  - 索引注记：ADR-0086 的 Context 判过这一格……本记录**不改它**」——已由 [ADR-0106](./0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md) 停用；该格当时写明「另立取证票，不在本记录裁」，取证票即 first-tenant-runway/09，证据齐后改为提交入账。**适用场景**：只停那一格的处置；Decision 一、二、四、五与其余三格不变，其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：「适用场景」只在注记

#### ADR-0098（失败尝试费发生项不在揽收编排里形成，采购上下文显式给出而不推导）

- **← ADR-0204**
  - 标记前段：**部分停用**：决定四「**今天不建**：建它等于在没有任何租户实证的情况下先定一种采购组织方式（揽收是挂在委托下、还是各自独立由第三方关系相连）」已由 [ADR-0204](./0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md) 决定五停用，适用场景只限实际履约段与运输委托之间的连线；揽收任务与尝试 ↔ 委托仍不建，其余各条不变，本记录正文不改写。
  - 索引注记：**部分停用**：其决定四「**今天不建**：建它等于在没有任何租户实证的情况下先定一种采购组织方式（揽收是挂在委托下、还是各自独立由第三方关系相连）」已由 [ADR-0204](./0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md) 决定五停用。**适用场景**：仅限实际履约段与运输委托之间的连线——停用理由的前提是「先定一种采购组织方式」，而那条连线由登记方逐段声明、产品不替租户定组织方式；揽收任务与尝试 ↔ 委托仍不建（失败尝试不成段，段级连线够不着它）。其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：无

#### ADR-0100（管理台运营操作者身份是产品自有的接入渠道族，属机制半边——信任锚、校验方式与操作）

- **← ADR-0139**
  - 标记前段：**部分停用**，2026-10-19：决定四末段「客户业务命令面（委托提交与撤回、作业事实登记、外部结果接收）照旧被两项未决拦着」余下的客户委托一半由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一停用，**适用场景**：首方族的客户面各口，ADR-0055 Decision 五两项按 ADR-0139 Decision 八按口解。
  - 索引注记：**部分停用**（2026-10-19）：决定四末段「客户业务命令面……照旧被两项未决拦着」余下的客户委托一半已由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一 停用，**适用场景**首方族的客户面各口；其余各条不变。
  - 脚本提示（只在一边的事实，合并时人工核）：无

- **← ADR-0149**
  - 标记前段：**部分停用**：决定四末段与决定五第一条对「作业事实登记、外部结果接收」的适用已由 [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 停用（适用场景：作业事实登记面与外部结果、外部资金事实接收面），其余各条不变；决定五第二条中「隔离形态（ADR-0091）继续靠 `SYN-` 写开关服务演示，它与操作者渠道是装配点上的两行，互不替代、不合并；ADR-0091 Decision 六……照旧」一段已由 [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 停用——过渡期内两行并存，某一口真渠道落地即在同一笔撤下该口的隔离行；同条「不做绕过 OIDC 的『开发用』本地账号」不变
  - 索引注记：**部分停用**：决定四末段「客户业务命令面（委托提交与撤回、作业事实登记、外部结果接收）照旧被两项未决拦着」与决定五第一条「不给客户业务命令面开操作者渠道」对作业事实登记与外部结果接收的适用，已由 [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 停用；**适用场景**：作业事实登记面与外部结果、外部资金事实接收面；客户委托那一半与其余各条不变。
  - 脚本提示（只在一边的事实，合并时人工核）：无

- **← ADR-0150**
  - 标记前段：**部分停用**：决定四末段与决定五第一条对「作业事实登记、外部结果接收」的适用已由 [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 停用（适用场景：作业事实登记面与外部结果、外部资金事实接收面），其余各条不变；决定五第二条中「隔离形态（ADR-0091）继续靠 `SYN-` 写开关服务演示，它与操作者渠道是装配点上的两行，互不替代、不合并；ADR-0091 Decision 六……照旧」一段已由 [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 停用——过渡期内两行并存，某一口真渠道落地即在同一笔撤下该口的隔离行；同条「不做绕过 OIDC 的『开发用』本地账号」不变
  - 索引注记：决定五第二条中隔离形态与操作者渠道「互不替代、不合并」一段已由 [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 停用——过渡期内两行并存，某一口真渠道落地即撤下该口的隔离行；同条「不做『开发用』本地账号」不变
  - 脚本提示（只在一边的事实，合并时人工核）：「适用场景」只在前段；「其余各条不变」只在前段

#### ADR-0101（运营操作者面的登记载荷形状由产品定义，属机制半边——「渠道原始载荷 → 登记快照）

- **← ADR-0139**
  - 标记前段：**部分停用**，2026-10-19：决定一「适用场景收窄为客户渠道载荷」由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一**再收窄为客户既有渠道的载荷**；首方 API 的载荷形状是产品契约（ADR-0140）。
  - 索引注记：**部分停用**（2026-10-19）：决定一「适用场景收窄为客户渠道载荷」已由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一 停用，**适用场景**再收窄到客户既有渠道的载荷；其余各条不变。
  - 脚本提示（只在一边的事实，合并时人工核）：无

#### ADR-0104（客户服务规则版本的正文归 `party-commercial`，首发只进「索赔期）

- **← ADR-0176**
  - 标记前段：**部分停用**：Alternatives「允许显式空版本」条目中「客户服务规则没有对应的回退物——产品没有默认期限」的前提已由 [ADR-0176](./0176-customer-service-rule-contract-tier-selection-and-inheritance.md) 消解，见本文件 Links 节前向指针；其余各条不变
  - 索引注记：**部分停用**：其 Alternatives「允许显式空版本」条目中「客户服务规则没有对应的回退物——产品没有默认期限」这一句的前提已由 [ADR-0176](./0176-customer-service-rule-contract-tier-selection-and-inheritance.md) 消解——回退物＝同范围挂服务产品的版本的那一行；该条目其余否决理由与 Decision 三「子行至少一项、不允许显式空版本」对版本级继续成立，其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：无

#### ADR-0106（等待受控补充是入账暂停——与人工复核、运营登记同组提交入账，续办由「新提交版本已）

- **← ADR-0139**
  - 标记前段：**部分停用**，2026-10-19：决定四「客户渠道的采信身份属 `PAR-INT-01` 接入契约，是实例半边」的适用场景由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一收窄为**租户既有渠道的适配**；首方族的采信身份由 ADR-0139 Decision 二定。
  - 索引注记：**部分停用**（2026-10-19）：决定四「客户渠道的采信身份属 `PAR-INT-01` 接入契约，是实例半边」已由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一 停用，**适用场景**收窄到租户既有渠道的适配；其余各条不变。
  - 脚本提示（只在一边的事实，合并时人工核）：无

#### ADR-0149（主链业务命令面按提交者分两族——一线作业事实走操作者渠道族新增的「作业事实登记」）

- **← ADR-0151**
  - 标记前段：**部分停用**：决定一列举里「派送任务」「段关闭」「有效时间判断」三项对管理台手工建派送任务、关段与有效时间判断三口（`/transport-fulfillment-dispatch-task-registrations`、`/transport-fulfillment-segment-closures`、`/transport-fulfillment-effective-time-judgments`）的适用已由 [ADR-0151](./0151-unassigned-command-faces-get-their-families.md) 停用——三口是管理台上的运营决定与判断，归操作者渠道的「运营决定」能力面；派送发起口与其余各条不变；
  - 索引注记：**部分停用**：决定一列举里「派送任务」「段关闭」「有效时间判断」三项对管理台手工建派送任务、关段与有效时间判断三口的适用已由 [ADR-0151](./0151-unassigned-command-faces-get-their-families.md) 停用；**适用场景**：`/transport-fulfillment-dispatch-task-registrations`、`/transport-fulfillment-segment-closures`、`/transport-fulfillment-effective-time-judgments`；派送发起口与其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：「适用场景」只在注记

- **← ADR-0236**
  - 标记前段：**部分停用**：决定一「外部结果与外部资金事实（关务外部结果、监管凭证、承运商轨迹与首次有效收寄证据、外部资金事实）的提交者是外部系统」一句与决定三对外部资金事实、监管凭证两类的独占已由 [ADR-0236](./0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md) 决定六停用，适用场景只限租户人员经三个手工口手工采用这一格；外部系统送来的那一格与其余各条不变
  - 索引注记：**部分停用**：决定一「外部结果与外部资金事实（……）的提交者是外部系统」一句与决定三「外部结果与资金事实走集成客户端族」对外部资金事实、监管凭证两类的独占已由 [ADR-0236](./0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md) 决定六停用——租户的财务、关务人员手工采用另走操作者渠道「运营决定」能力面。**适用场景**：只限 `/settlement-external-funds-fact-manual-registrations`、`/settlement-external-funds-fact-correction-manual-registrations`、`/customs-regulatory-credential-manual-registrations` 三口，理由是这一格的提交者是租户自己的人；外部系统送来的那一格照旧归集成客户端族，其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：无

#### ADR-0151（未归族的命令口定族——委托侧五个运营决定口与 TF 在管理台上作决定或判断的各口）

- **← ADR-0187**
  - 标记前段：**部分停用**：决定二受控关闭与重开一项「不进命令的任何一格」半句已由 [ADR-0187](./0187-controlled-closure-and-reopening-record-request-and-operation-evidence-on-the-decision.md) 决定七停用，适用场景只限受控关闭口与重开口；
  - 索引注记：**部分停用**：决定二受控关闭与重开一项「提交操作者只作操作证据，不进命令的任何一格」中「不进命令的任何一格」半句已由 [ADR-0187](./0187-controlled-closure-and-reopening-record-request-and-operation-evidence-on-the-decision.md) 决定七停用——提交操作者进且只进操作证据一格。**适用场景**：只限受控关闭口 `/shipment-requests/continued-attempt-closures` 与重开口 `/shipment-requests/continued-attempt-reopenings`，理由是 PS CONTEXT 把操作证据写进这两种决定的定义；「只作操作证据」与同项其余各句不变，TF 各口要不要记提交操作者仍归 TF owner，其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：无

- **← ADR-0230**
  - 标记前段：**部分停用**：决定六末句里派送发起口「与 ADR-0149 其余各条不变」那一项已由 [ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 决定七停用，补拍口改挂运营决定，适用场景只限那一口（被停原句见索引注记）；
  - 索引注记：**部分停用**（2026-10-21）：决定六末句「派送发起口 `/transport-fulfillment-delivery-dispatch-triggers`（内部触发执行器的按拍入口，与手工建任务是两件事）与 ADR-0149 其余各条不变」里派送发起口那一项已由 [ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 决定七停用——它改挂「运营决定」能力面，决定种类「派送重问」与按派送待形成册批量重问共用；越权风险点 7 的前提「调用方是调度拍」随 ADR-0216 改成进程内那一拍，再发起的人是管理台上的调度员。**适用场景**：只限这一口；「与手工建任务是两件事」与 ADR-0149 其余各条不变，其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：日期只在注记 2026-10-21

- **← ADR-0236**
  - 标记前段：**部分停用**：决定四「外部资金事实更正随原事实归集成客户端族（operator-channel/11）」一句已由 [ADR-0236](./0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md) 决定六停用，适用场景只限手工更正口
  - 索引注记：**部分停用**：决定四「外部资金事实更正随原事实归集成客户端族（operator-channel/11）」一句已由 [ADR-0236](./0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md) 决定六停用——外部资金事实的更正也可由持「外部资金事实采用」授予的操作者经手工更正口提交，且不绑原采用族。**适用场景**：只限 `/settlement-external-funds-fact-correction-manual-registrations` 一口；交接、揽收、POD 更正与决定四其余各句不变，其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：无

#### ADR-0164（BUY 评价请求的触发面是发生项形成，原因没登记就不发起）

- **← ADR-0208**
  - 标记前段：**部分停用**：决定一末句「不从运输履约登记册推导三件来源引用」已由 [ADR-0208](./0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md) 决定二在发生项已形成信封那一路停用，其余各条不变。
  - 索引注记：**部分停用**：其决定一末句「不从运输履约登记册推导三件来源引用」已由 [ADR-0208](./0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md) 决定二停用。**适用场景**：仅限发生项已形成信封那一路——停用的理由是按引用回读的是那一份发生项自己固定的事实（协议是发生项形成时固定下来的来源事实），不是按当前伙伴、线路或报表分组去配；由调用方交齐三件引用的触发面照旧不读运输履约。其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：「适用场景」只在注记

#### ADR-0169（SELL 评价形成客户费用，何时发起另册登记）

- **← ADR-0198**
  - 标记前段：部分停用：决定四「信封上还没有费用项目时，消费门指名缺这一引用」在包裹计费时点那一路由 [ADR-0198](./0198-sell-request-conclusions-on-the-parcel-billing-moment-path.md) 决定三停用。
  - 索引注记：**部分停用**：决定四「信封上还没有费用项目时，消费门指名缺这一引用」已由 [ADR-0198](./0198-sell-request-conclusions-on-the-parcel-billing-moment-path.md) 决定三停用，**适用场景**仅限包裹计费时点那一路（回指 SELL 请求的评价），其余各条不变。
  - 脚本提示（只在一边的事实，合并时人工核）：「适用场景」只在注记

#### ADR-0175（服务日历的截单、处理时长与衔接缓冲按绝对时刻折进时间投影）

- **← ADR-0217**
  - 标记前段：部分停用：[ADR-0217](./0217-route-plan-carries-a-delivery-leg-after-the-delivery-node.md)（被停条款：决定一「三格都是分钟，都可空」里的「三格」，节点日历多一格派送时长；决定四「线路的连接序列是候选段链」，交付节点登了派送时长时连接序列之后接一段交付段；适用场景与原句见 adr/README 该行，其余各条不变，本记录正文不改写）
  - 索引注记：**部分停用**：决定一「三格都是分钟，都可空」里的「三格」与决定四「线路的连接序列是候选段链」已由 [ADR-0217](./0217-route-plan-carries-a-delivery-leg-after-the-delivery-node.md) 停用，**适用场景**：前一句只限节点日历多出的派送时长一格（交付节点登了它，计划在交付节点之后带交付段），截单、处理时长、衔接缓冲的读法不变；后一句只限交付节点日历登了派送时长、且候选有交付范围时，连接序列之后接一段交付段，连接序列本身照旧是节点间段；自票 demo-chain-gaps/64 进 main 起生效；其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：无

#### ADR-0183（操作者渠道上受控关闭与重开按包裹寻址，请求方与货主账户取包裹所属已接受委托的客户）

- **← ADR-0187**
  - 标记前段：**部分停用**：决定五首句「两条命令没有提交操作者的格；本记录不给它们加格」已由 [ADR-0187](./0187-controlled-closure-and-reopening-record-request-and-operation-evidence-on-the-decision.md) 决定七停用，适用场景只限受控关闭口与重开口
  - 索引注记：**部分停用**：决定五首句「两条命令没有提交操作者的格；本记录不给它们加格」已由 [ADR-0187](./0187-controlled-closure-and-reopening-record-request-and-operation-evidence-on-the-decision.md) 决定七停用。**适用场景**：只限受控关闭口与重开口；其余各条不变。 / 「复核记录二」的比较格不变，由 ADR-0187 决定四补明请求证据与操作证据不在比较格内
  - 脚本提示（只在一边的事实，合并时人工核）：无

#### ADR-0186（节点收寄遇跨上下文读不动——服务结果读不动照常形成收寄并记「读不动」，身份核对读）

- **← ADR-0199**
  - 标记前段：部分停用：决定二「身份核对读不动：不形成答案」与决定七「身份读口不隔离」两处由 [ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md) 决定五停用，随票 no-reception-source/05 进 main 生效；被停的原句见本记录文末「复核记录（2026-10-19，ADR-0199 决定五部分停用两处）」与 adr/README 该行。
  - 索引注记：不落册」与决定七「身份读口不隔离」（连同理由「身份读口读不动本就不形成答案，整笔回滚正是要的结果」）已由 [ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md) 决定五停用，自票 no-reception-source/05 进 main 起生效：身份核对读不动落`待识别`加身份核对态「未核对」、同笔自投重核门铃，身份读口经 savepoint；决定二「不拿空候选顶替」照旧，由核对态兑现；Consequences 里随决定二的「身份核对读不动现在答 5xx，设备留队重投」一句同停；其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：日期只在前段 2026-10-19

#### ADR-0195（唯一命中要没有歧义信号——号码同时被一份`已提交`委托声明时，节点答身份冲突；声）

- **← ADR-0211**
  - 标记前段：本记录自身决定四一句由 [ADR-0211](./0211-an-ambiguity-only-identity-conflict-releases-by-recheck-once-the-declaration-is-gone.md) 停用，只限冲突来由为只因歧义信号的身份冲突，随票 no-reception-identity/07 进 main 生效；原句见本记录文末「复核记录二（2026-10-19，ADR-0211 部分停用一句」与 adr/README 该行。
  - 索引注记：……冲突行不经重核解除……」里「信号消失不交信封」与「冲突行不经重核解除」两处，连同越权风险点「信号消失后，已记的冲突不自动解除」，已由 [ADR-0211](./0211-an-ambiguity-only-identity-conflict-releases-by-recheck-once-the-declaration-is-gone.md) 停用，**适用场景**：只限冲突来由为只因歧义信号的身份冲突——PS 在声明它的委托被拒绝或撤回时交声明撤除门铃，这类冲突经重核解开；同句「零候选的待识别又本就不看信号」照旧，自票 no-reception-identity/07 进 main 起生效；决定一「不按客户账户、分配方或交付方收窄」与决定三起句「NO 只认「恰一个候选、且没有信号」为唯一命中」及其分格已由 [ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 停用，**适用场景**：决定一只限有可验证交付范围时（信号只认范围内客户账户的声明）；决定三在有可验证交付范围时由 ADR-0212 决定四的表取代，没有时只「恰一个候选、无信号」一格收窄为候选范围相容才算；自票 ps-external-mark-relations/11 进 main 起生效；其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：日期只在前段 2026-10-19；「适用场景」只在注记

- **← ADR-0212**
  - 标记前段：部分停用：[ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md)（被停条款：决定一「不按客户账户、分配方或交付方收窄」，适用场景是有可验证交付范围时——信号只认范围内客户账户的声明；决定三起句「NO 只认「恰一个候选、且没有信号」为唯一命中」与它下面的分格，有可验证交付范围时由 ADR-0212 决定四的表取代，没有时只「恰一个候选、无信号」一格收窄为候选范围相容才算；其余各条不变，本记录正文不改写）
  - 索引注记：……冲突行不经重核解除……」里「信号消失不交信封」与「冲突行不经重核解除」两处，连同越权风险点「信号消失后，已记的冲突不自动解除」，已由 [ADR-0211](./0211-an-ambiguity-only-identity-conflict-releases-by-recheck-once-the-declaration-is-gone.md) 停用，**适用场景**：只限冲突来由为只因歧义信号的身份冲突——PS 在声明它的委托被拒绝或撤回时交声明撤除门铃，这类冲突经重核解开；同句「零候选的待识别又本就不看信号」照旧，自票 no-reception-identity/07 进 main 起生效；决定一「不按客户账户、分配方或交付方收窄」与决定三起句「NO 只认「恰一个候选、且没有信号」为唯一命中」及其分格已由 [ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 停用，**适用场景**：决定一只限有可验证交付范围时（信号只认范围内客户账户的声明）；决定三在有可验证交付范围时由 ADR-0212 决定四的表取代，没有时只「恰一个候选、无信号」一格收窄为候选范围相容才算；自票 ps-external-mark-relations/11 进 main 起生效；其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：无

#### ADR-0196（一码两物按关联到的包裹认——后到那件答身份冲突，先到那件只追加一版身份冲突判断）

- **← ADR-0199**
  - 标记前段：另有两句由 [ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md) 决定四停用，只限同一节点上有当前收寄的情形，随票 no-reception-source/04 进 main 生效；原句见本记录文末「复核记录三」与 adr/README 该行。
  - 索引注记：**部分停用**：决定四「先到那件有原判断的关联行加追加的冲突行，落成缺少·身份冲突」与同条「方向依据代码不改」已由 [ADR-0210](./0210-identity-conflicts-are-disposed-by-an-operation-decision-and-the-identity-basis-reads-chain-heads.md) 决定五停用，**适用场景**：前一句只限指名了前版的冲突版，先到那件改落缺少·{待识别, 身份冲突}，自票 no-reception-identity/05 进 main 起生效；决定六「按租户与包裹关联读别的作业实物的收寄，只认收寄本身带的关联」对被身份冲突处置版接续过的关联停用（ADR-0210 决定八），自票 no-reception-identity/06 进 main 起生效；决定一末句「同一件实物换个来源再收一次照常形成收寄」与「能力边界」里冲突形成之后先到那件换来源再收「照常答`节点收寄已形成`、交发布意图」一句已由 [ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md) 决定四停用，**适用场景**：只限同一节点上有当前收寄（当前有效版本是形成或待识别、控制没有转出），这时再来的交付并入那条收寄、答已有结果、不交意图，自票 no-reception-source/04 进 main 起生效；其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：「适用场景」只在注记

- **← ADR-0210**
  - 标记前段：部分停用：几句由 [ADR-0210](./0210-identity-conflicts-are-disposed-by-an-operation-decision-and-the-identity-basis-reads-chain-heads.md) 停用，随票 no-reception-identity/05、06 进 main 各自生效；被停的原句见本记录文末「复核记录二」与 adr/README 该行。
  - 索引注记：**部分停用**：决定四「先到那件有原判断的关联行加追加的冲突行，落成缺少·身份冲突」与同条「方向依据代码不改」已由 [ADR-0210](./0210-identity-conflicts-are-disposed-by-an-operation-decision-and-the-identity-basis-reads-chain-heads.md) 决定五停用，**适用场景**：前一句只限指名了前版的冲突版，先到那件改落缺少·{待识别, 身份冲突}，自票 no-reception-identity/05 进 main 起生效；决定六「按租户与包裹关联读别的作业实物的收寄，只认收寄本身带的关联」对被身份冲突处置版接续过的关联停用（ADR-0210 决定八），自票 no-reception-identity/06 进 main 起生效；决定一末句「同一件实物换个来源再收一次照常形成收寄」与「能力边界」里冲突形成之后先到那件换来源再收「照常答`节点收寄已形成`、交发布意图」一句已由 [ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md) 决定四停用，**适用场景**：只限同一节点上有当前收寄（当前有效版本是形成或待识别、控制没有转出），这时再来的交付并入那条收寄、答已有结果、不交意图，自票 no-reception-source/04 进 main 起生效；其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：「适用场景」只在注记

#### ADR-0199（节点收寄保全交付来源——待识别实物按存下的号重核，同一作业实物在同一节点不另起控）

- **← ADR-0210**
  - 标记前段：部分停用：几句由 [ADR-0210](./0210-identity-conflicts-are-disposed-by-an-operation-decision-and-the-identity-basis-reads-chain-heads.md) 停用，随票 no-reception-identity/05、06 进 main 各自生效；被停的原句见本记录文末「复核记录（2026-10-19，ADR-0210 部分停用几句」与 adr/README 该行。
  - 索引注记：**部分停用**：决定三「重核对象」一项里「册上已有的冲突行在 `JudgeIdentityBasis` 里一直判身份冲突，重核解不开」一句与决定五末项里「方向依据代码不改」「照现有折法」已由 [ADR-0210](./0210-identity-conflicts-are-disposed-by-an-operation-decision-and-the-identity-basis-reads-chain-heads.md) 决定五停用（结论「记身份冲突的不重核」维持，理由改为冲突只经人工处置解除），自票 no-reception-identity/05 进 main 起生效；决定三分格里「唯一，且包裹没有关联到别的作业实物」与「唯一，但包裹已关联到别的作业实物」两支已由 ADR-0210 决定九停用，**适用场景**：只限被重核那一版是「不是其中任何一件」的身份冲突处置版、唯一命中的包裹在它排除之列，自票 no-reception-identity/06 进 main 起生效；其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：日期只在前段 2026-10-19；「适用场景」只在注记

- **← ADR-0211**
  - 标记前段：另有决定三两句由 [ADR-0211](./0211-an-ambiguity-only-identity-conflict-releases-by-recheck-once-the-declaration-is-gone.md) 停用，只限冲突来由为只因歧义信号的身份冲突，随票 no-reception-identity/07 进 main 生效；原句见本记录文末「复核记录（2026-10-19，ADR-0211 部分停用两句」与 adr/README 该行。
  - 索引注记：**部分停用**：决定三「触发只有三处，不做后台扫表」里「只有三处」，与「重核对象」一项里「记身份冲突的（多候选、一码两物）不重核」，已由 [ADR-0211](./0211-an-ambiguity-only-identity-conflict-releases-by-recheck-once-the-declaration-is-gone.md) 停用，**适用场景**：只限冲突来由为只因歧义信号的身份冲突（ADR-0195 决定四把带信号的一个候选落进这一句）——触发多一处 PS 声明撤除门铃，这类冲突是重核对象；「不做后台扫表」与多候选、一码两物、来由缺席的冲突不重核照旧，自票 no-reception-identity/07 进 main 起生效；决定一「内容：作业实物、外部标识观察、接收观察、证据、拒收原因、交付方、节点、发生时间」与「证据、拒收原因、交付方只进来源文档、不进指纹」两句的列举、决定三「触发只有三处」里「只有三处」已由 [ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 停用，**适用场景**：前两句各多一格交付范围依据引用（指纹口径不变），自票 ps-external-mark-relations/11 进 main 起生效；后一句在 ADR-0211 的声明撤除门铃之外再多一处「交付范围成立」的自投门铃（不做后台扫表照旧），自票 ps-external-mark-relations/12 进 main 起生效；其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：日期只在前段 2026-10-19；「适用场景」只在注记

- **← ADR-0212**
  - 标记前段：另有决定一、决定三各有句子由 [ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 停用，随票 ps-external-mark-relations/11、12 进 main 各自生效；原句见本记录「复核记录（2026-10-19，ADR-0212 部分停用几句」与 adr/README 该行。
  - 索引注记：**部分停用**：决定三「触发只有三处，不做后台扫表」里「只有三处」，与「重核对象」一项里「记身份冲突的（多候选、一码两物）不重核」，已由 [ADR-0211](./0211-an-ambiguity-only-identity-conflict-releases-by-recheck-once-the-declaration-is-gone.md) 停用，**适用场景**：只限冲突来由为只因歧义信号的身份冲突（ADR-0195 决定四把带信号的一个候选落进这一句）——触发多一处 PS 声明撤除门铃，这类冲突是重核对象；「不做后台扫表」与多候选、一码两物、来由缺席的冲突不重核照旧，自票 no-reception-identity/07 进 main 起生效；决定一「内容：作业实物、外部标识观察、接收观察、证据、拒收原因、交付方、节点、发生时间」与「证据、拒收原因、交付方只进来源文档、不进指纹」两句的列举、决定三「触发只有三处」里「只有三处」已由 [ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 停用，**适用场景**：前两句各多一格交付范围依据引用（指纹口径不变），自票 ps-external-mark-relations/11 进 main 起生效；后一句在 ADR-0211 的声明撤除门铃之外再多一处「交付范围成立」的自投门铃（不做后台扫表照旧），自票 ps-external-mark-relations/12 进 main 起生效；其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：日期只在前段 2026-10-19；「适用场景」只在注记

#### ADR-0201（运输机会——周期安排按日期物化成班次，初始路由形成后逐段准备，空册答未配置）

- **← ADR-0217**
  - 标记前段：部分停用：[ADR-0217](./0217-route-plan-carries-a-delivery-leg-after-the-delivery-node.md)（被停条款：决定二末句「与计划履约段的比对……对的是 `PlannedLeg.From/To` 与窗口依据里的连接代码」只对节点间段成立，计划末尾的交付段不比对、不准备；适用场景见 adr/README 该行，其余各条不变，本记录正文不改写）
  - 索引注记：**部分停用**：决定二末句「与计划履约段的比对只在 TF 的 `adapters/networkrouting` 里做，对的是 `PlannedLeg.From/To` 与窗口依据里的连接代码（票 18）」已由 [ADR-0217](./0217-route-plan-carries-a-delivery-leg-after-the-delivery-node.md) 停用，**适用场景**：只限计划末尾的交付段——它没有终点节点、窗口依据不指连接，不比对、不形成准备结果；节点间段照原句；自票 demo-chain-gaps/64 进 main 起生效；其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：无

#### ADR-0202（运输委托、订舱与承运答复的入口族与协议核对——委托与订舱各口每口一种运营决定，承）

- **← ADR-0239**
  - 标记前段：）越权风险点 14 已由 [ADR-0239](./0239-a-carrier-answer-from-a-non-provider-is-answered-booking-not-found-and-the-basis-is-kept-on-a-bounce-register.md) 回答（2026-10-21）：不是本服务方的答复方改答订舱不在册，挡回依据记进承运答复挡回册；由此
  - 索引注记：**非服务方答复并入订舱不在册**（2026-10-21）：越权风险点 14 由 [ADR-0239](./0239-a-carrier-answer-from-a-non-provider-is-answered-booking-not-found-and-the-basis-is-kept-on-a-bounce-register.md) 回答——答复方的来源身份不是订舱所属委托的服务提供方时答 `BOOKING_NOT_FOUND`，与订舱不在册同一格、响应体逐字相同；挡回依据（`BOOKING_NOT_REGISTERED`、`SOURCE_NOT_SERVICE_PROVIDER`）与那一次答复同笔记进承运答复挡回册；订舱连同服务方一条语句读回，两条挡回路读写不动答同一个未决原因。**被停条款**：文末补记「承运答复的次序与格」里「答复方的来源身份等于订舱所属委托的服务提供方，否则 `SOURCE_NOT_SERVICE_PROVIDER`」的「否则 `SOURCE_NOT_SERVICE_PROVIDER`」那一格。**适用场景**：只限承运答复口 `/transport-fulfillment/booking-answers`；来源取自绑定、来源核对排在重放之前、字面比、不回显在册答复、委托读不回为数据违约与同条其余各句不变，决定二「不等答不受理」不变，其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：「适用场景」只在注记

#### ADR-0205（出发门禁是拉取的读数——与移动事实同笔落册，不拒收；移动事实带声明的段）

- **← ADR-0225**
  - 标记前段：越权风险点 5「对象只取段里的参与，不展开集运单元」已由 [ADR-0225](./0225-the-departure-gate-reading-asks-through-consolidation-unit-members-at-the-moment-of-departure.md) 回答（2026-10-20）：段里在控的集运单元按出发那一刻的直接成员逐件问关务，单元本身不问，它那一格取成员里最重的一格；由此
  - 索引注记：**部分停用**（2026-10-20）：越权风险点 5「对象只取段里的参与，不展开集运单元」由 [ADR-0225](./0225-the-departure-gate-reading-asks-through-consolidation-unit-members-at-the-moment-of-departure.md) 回答——段里在控的集运单元按出发那一刻的直接成员逐件问关务，单元那一格取成员里最重的一格，成员各自一格进逐对象明细，出发前问询同受。**被停条款**：决定一「回给报事实一方的答复只带整格、起讫、停点与逐对象那一格（对象与它的格）」——单元那一格另带它的成员；决定二「逐对象问，每件一格：关务的答复译成本上下文的五格」——单元那一格不问关务、由成员推出。**适用场景**：仅限段里在控或展开出来的对象是集运单元时那一件；包裹照旧逐件问关务、答复照旧只有对象与格。其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：「适用场景」只在注记

#### ADR-0207（运输收费发生项由运营决定显式登记——事实取自事实本身，采购上下文由登记方给出、经）

- **← ADR-0209**
  - 标记前段：**部分停用**：决定三「登记方交一句原话断言……缺了答未受理 `CONDITION_NOT_ASSERTED`」与「数量与单位同由登记方声明」两句、决定四「查与随后的落册在同一事务里按（租户、原因、事实依据）串行化」一句的键，已由 [ADR-0209](./0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md) 停用，适用场景见该记录「对已接受记录的作用」；其余各条不变，本记录正文不改写。 / **部分停用**（2026-10-19）：决定七后半句——标题里「也暂不自动形成」与正文「运输履约自消费交接与有效交付信封来自动形成，等 ADR-0209 与票 demo-chain-gaps/25 落地后另定」——与越权风险点 3「不自动形成」已由 [ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 停用，**适用场景**只限有采用连线的段；决定四末句「演示约定 `SYN-JRN-<包裹号>`」同由 ADR-0218 停用，演示租户改用 `JRN/<包裹号>`。决定七前半句、决定四其余各句不变，本记录正文不改写。
  - 索引注记：操作者渠道「运营决定」能力面加决定种类「运输收费发生项登记」；一个用例覆盖各原因，依据、范围与业务时间取自事实——失败尝试取尝试结果（派送经现有读口），实际履约取段内已结束的参与、业务时间取最晚终点，订舱与取消随票 28；采购上下文由登记方给出、按业务时间经 PC 核对，法人与「对象/版本」快照取自协议，四个否定格各成一格，缺了答未受理绝不答不适用，不适用只认显式声明自营与实际承运商判为自营；发生条件在 ADR-0209 之前由登记者断言并留痕；身份由（租户、原因、依据、成员集合）派生、首版 V1，同一事实上成员与另一项仍有效的发生项有交集答冲突；`transport-fulfillment.charge-occurrence.formed` 引用式三键、指纹 ID、与发生项同笔；不内联进物理编排、暂不自动形成。 / **部分停用**：决定三「登记方交一句原话断言『采用协议的发生条件已满足』……缺了答未受理 `CONDITION_NOT_ASSERTED`」与「数量与单位同由登记方声明」两句已由 [ADR-0209](./0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md) 决定四停用，**适用场景**：本记录之后新形成的发生项——停用的理由是条件与数量形已由采用的协议声明，这一前提只对新登记成立，断言期已形成的项照旧按断言读；决定四「查与随后的落册在同一事务里按（租户、原因、事实依据）串行化」一句的键改为（租户、原因、主要业务范围），**适用场景**：全部登记——按范围计的一项要挡住同一范围上别的事实依据。其余各条不变 / **部分停用**（[ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md)）：决定七后半句——「也暂不自动形成」与「运输履约自消费交接与有效交付信封来自动形成，等 ADR-0209 与票 demo-chain-gaps/25 落地后另定」——与越权风险点 3「不自动形成」，**适用场景**：只限有采用连线的段——停用的理由带前提「采购上下文已由委托给出、条件已由协议声明」，没有连线的段仍由人工登记；决定四末句「演示约定 `SYN-JRN-<包裹号>`」，**适用场景**：演示租户自 ADR-0218 起改用 `JRN/<包裹号>`。
  - 脚本提示（只在一边的事实，合并时人工核）：日期只在前段 2026-10-19

- **← ADR-0218**
  - 标记前段：**部分停用**（2026-10-19）：决定七后半句——标题里「也暂不自动形成」与正文「运输履约自消费交接与有效交付信封来自动形成，等 ADR-0209 与票 demo-chain-gaps/25 落地后另定」——与越权风险点 3「不自动形成」已由 [ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 停用，**适用场景**只限有采用连线的段；决定四末句「演示约定 `SYN-JRN-<包裹号>`」同由 ADR-0218 停用，演示租户改用 `JRN/<包裹号>`。决定七前半句、决定四其余各句不变，本记录正文不改写。
  - 索引注记：**部分停用**（[ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md)）：决定七后半句——「也暂不自动形成」与「运输履约自消费交接与有效交付信封来自动形成，等 ADR-0209 与票 demo-chain-gaps/25 落地后另定」——与越权风险点 3「不自动形成」，**适用场景**：只限有采用连线的段——停用的理由带前提「采购上下文已由委托给出、条件已由协议声明」，没有连线的段仍由人工登记；决定四末句「演示约定 `SYN-JRN-<包裹号>`」，**适用场景**：演示租户自 ADR-0218 起改用 `JRN/<包裹号>`。
  - 脚本提示（只在一边的事实，合并时人工核）：日期只在前段 2026-10-19

#### ADR-0209（供应商协议声明收费发生条件——逐原因计不计费、计费带数量形，发生项登记按采用的那）

- **← ADR-0218**
  - 标记前段：**部分停用**（2026-10-19）：决定五「登记者取自操作者信封的主体」一句已由 [ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 停用，**适用场景**只限实际履约发生项自动形成那一路（没有操作者信封，登记者记本上下文的系统身份）；运营决定那一路与其余各条不变，本记录正文不改写。
  - 索引注记：**部分停用**（[ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md)）：决定五「登记者取自操作者信封的主体」，**适用场景**：只限实际履约发生项自动形成那一路——那一路没有操作者信封，登记者记本上下文的系统身份；运营决定那一路与其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：日期只在前段 2026-10-19

#### ADR-0210（身份冲突经运营决定处置——在冲突候选里指定一件或确认都不是，逐条判断链追加处置版）

- **← ADR-0211**
  - 标记前段：部分停用：决定一两句与「能力边界」一项由 [ADR-0211](./0211-an-ambiguity-only-identity-conflict-releases-by-recheck-once-the-declaration-is-gone.md) 停用，只限冲突来由为只因歧义信号的身份冲突，随票 no-reception-identity/07 进 main 生效；被停的原句见本记录文末「复核记录（2026-10-19，ADR-0211 部分停用几句）」与 adr/README 该行。
  - 索引注记：**」里重核那半句，以及「能力边界」首项里「不做自动解除」，已由 [ADR-0211](./0211-an-ambiguity-only-identity-conflict-releases-by-recheck-once-the-declaration-is-gone.md) 停用，**适用场景**：只限冲突来由为只因歧义信号的身份冲突——声明它的委托被拒绝或撤回后，PS 交声明撤除门铃，重核答唯一命中即追加关联版、冲突随链头解开；再次收寄不解除冲突、多候选与一码两物只经处置照旧，自票 no-reception-identity/07 进 main 起生效；其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：日期只在前段 2026-10-19；「适用场景」只在注记

#### ADR-0214（节点控制按权威运输交接转出与转入——交接对象按正式包裹身份经节点自己的当前关联对）

- **← ADR-0220**
  - 标记前段：」一句已由 [ADR-0220](./0220-node-control-reads-the-handover-correction-chain-by-the-transport-fulfillment-back-pointer.md) 停用——更正链只按运输履约的更正回指认，正文与文末补记凡说「同链」的各格都按它读；其余各条不变。
  - 索引注记：」一句已由 [ADR-0220](./0220-node-control-reads-the-handover-correction-chain-by-the-transport-fulfillment-back-pointer.md) 停用——更正链只按运输履约的更正回指认，同一（对象，范围）下另起的首登是另一次交接。**适用场景**：节点控制转出与转入两半凡认「同链」之处（同链前版已推进、撤回控制的更正、一次交接不转出它自己立起的控制）；转移册的主键与「同一版同一方向只处理一次」不受影响。其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：「适用场景」只在注记

#### ADR-0215（交接进入节点时路由复核——只对`已交接`且声明了接收节点的交接复核、位置取接收节）

- **← ADR-0235**
  - 标记前段：）越权风险点 9「没有路由历史的已接受包裹会重投到预算耗尽」、10「本路未决会推迟同一对象后续交接的追踪投影与派送发起」与 11「两种触发分属不同分区，复核不按业务时间排序」已由 [ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 回答（2026-10-21）：没有路由历史、证据未配置、冻结形态未配置三格入账未判，原因落 NR 的未判触发登记，不再重投；业务时间早于路由历史里最近一次已判控制触发的触发入账为「触发早于当前位置」，不判。
  - 索引注记：**部分停用**：其决定四「重投」一条里「复核停在未决（复核登记册或路由历史读不回、证据未配置、适用性看不清、取号依赖故障）」一句的未决范围已由 [ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 停用——证据未配置，以及随之同归未决的没有路由历史、冻结形态未配置，改为入账未判、原因落未判触发登记。**适用场景**：NR 路由复核的收寄采用与交接两路；交接读不回、小包托运读不动与其余未决照旧重投，其余各条不变 / 」一句已由 [ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 停用。**适用场景**：更正的那次交接业务时间（承继原交接的判断时刻）早于路由历史里最近一次已判控制触发，这时更正入账为「触发早于当前位置」、不判；更正当前位置那一次交接照判、另成一次复核，更正成拒收或待确认照旧入账不复核，其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：日期只在前段 2026-10-21；「适用场景」只在注记

#### ADR-0216（派送发起随交接信封在进程内逐对象发起——TF 自消费交接登记信封，按对象此刻在场）

- **← ADR-0230**
  - 标记前段：）越权风险点 1「要求缺失不会自愈」、7「交接更正会自动另立一项任务」与 2 的第一种情形已由 [ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 回答（2026-10-21）：要求缺失同笔落派送待形成册、运营按册重问，随交接的一拍核期望交接、同段替代参与不另立任务；由此
  - 索引注记：**派送待形成留痕与补拍改族**（2026-10-21）：越权风险点 1「要求缺失不会自愈」、7「交接更正会自动另立一项任务」由 [ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 回答——执行器答`要求缺失`与这一拍同笔落「派送待形成」册、形成了结、离场作罢，运营按册批量重问走同一个执行器，到期重问凭租户登记的间隔、未登记答未配置；随交接信封的一拍把那一版交接交给执行器核链尾（`ENTRY_NOT_THIS_HANDOVER`），查在册沿替代链认前版任务。
  - 脚本提示（只在一边的事实，合并时人工核）：无

#### ADR-0218（实际履约发生项由交接与有效交付信封自动形成——只对已采用委托的段，找出被这件事实）

- **← ADR-0221**
  - 标记前段：**部分停用**（2026-10-19）：决定一「是`已交接`的，终点依据取交接自己交出的转出依据」与「多于一条：库面不一致，响亮失败，不挑一段去计费」两句、决定六「响亮失败」下「同一件事实在多个段里都结束了参与」一条，已由 [ADR-0221](./0221-a-handover-participation-basis-is-minted-from-the-handover-key.md) 停用——终点依据改由交接键铸，存量按版本加时刻认；后两处只停在旧拼法存量那一格（对上多于一条答`结束的参与认不出`、入账），逐字对上多于一条仍响亮失败；其余各条不变。
  - 索引注记：**部分停用**：决定一「是`已交接`的，终点依据取交接自己交出的转出依据」与「多于一条：库面不一致，响亮失败，不挑一段去计费」两句、决定六「响亮失败」下「同一件事实在多个段里都结束了参与」一条，已由 [ADR-0221](./0221-a-handover-participation-basis-is-minted-from-the-handover-key.md) 停用。**适用场景**：第一句全部停用——终点依据改由交接键铸，存量按版本加终点时刻认；后两处只停在 ADR-0221 之前只带版本的存量终点依据那一格，对上多于一条改答`结束的参与认不出`（`ENDED_PARTICIPATION_AMBIGUOUS`）、入账，逐字对上多于一条仍是库面不一致、响亮失败；其余各条不变
  - 脚本提示（只在一边的事实，合并时人工核）：日期只在前段 2026-10-19；「适用场景」只在注记


### B2　去向两写（07 口径：前段写「本记录停用了谁」的不是重复、不动；注记里的去向是否保留由 07 判）：26 份

0139→0072/0085/0100/0101/0106；0140→0022；0148→0053/0068；0150→0091/0100；0151→0149；0185→0184；0187→0151/0183；0195→0184；0198→0169；0204→0098；0209→0207；0210→0196/0199；0211→0195/0199/0210；0212→0184/0195/0199；0213→0014；0215→0075；0217→0175/0201；0218→0207/0209；0220→0214；0221→0218；0225→0205；0229→0190/0198/0200/0208；0230→0151/0216；0235→0215；0236→0149/0151；0239→0202。


### B3　接受出处两写（前段「Accepted（日期，……）」与注记「**已接受**（日期，……）」）：55 份

0139、0140、0141、0142、0179、0180、0182、0183、0184、0185、0186、0187、0188、0189、0190、0195、0196、0197、0198、0199、0200、0201、0202、0203、0204、0205、0206、0207、0208、0209、0210、0211、0212、0213、0214、0215、0216、0217、0218、0219、0220、0221、0222、0223、0224、0225、0226、0227、0229、0230、0233、0234、0235、0236、0239。票 03 完成记录当时数的是 42 份（0139–0142、0179、0180、0182–0190、0195–0221）；之后新立的 0222 至 0239 每一份都照样两写，说明这不是存量问题，是现行写法（X-02）。


### B4　Status 行之外的第三份：头部「Partially suspended by:」「Amended by:」行

| ADR | 头部行 | 与索引注记的关系 |
|---|---|---|
| 0012 | Amended by ADR-0013 | 注记里没有，索引上看不到（X-03） |
| 0014 | Partially suspended by ADR-0177、0201、0202、0213；Amended by 开发主线 | 四次停用注记里都有，重复 |
| 0015 | Amended by ADR-0014 | 注记里没有；是释义还是停用待判（X-03） |
| 0048 | Partially suspended by ADR-0179 | 注记里有，重复；头部行写得比注记全（「决定三前两个增量不变」只在头部行） |
| 0126 | Partially suspended by ADR-0178 | 注记里有，重复 |
| 0184 | Partially suspended by ADR-0185、0195、0212 | 注记里有，重复；头部行带被停条款原句 |


## 附表 C　引用规范（目标 5，扫 `docs/` 非 archive 与 `AGENTS.md`，实测于 `6b0074d0`）

### C1　断链

相对链接约 5400 条中解析不了的只有下表 20 条；带锚点的约 280 条全部解析得到（锚点按 GitHub 规则：小写、去标点、空格变 `-`、中文保留、重名加序号）。反引号里的仓内路径另扫了一遍，缺的 8 处都是历史说明（「原位于 `docs/ux/…`」）或别处机器上的脚本，不算断链。

| 文件 | 链接目标 | 说明 |
|---|---|---|
| `docs/adr/0011-parcel-pricing-context-within-idp-parcel.md` | `../../foo/国际小包计费领域模型_V1.0.1_终审版.md` | 仓外 |
| `docs/adr/0011-parcel-pricing-context-within-idp-parcel.md` | `../../foo/Rating_Runtime计算语义规范_V1.0.1_终审版.md` | 仓外 |
| `docs/adr/0011-parcel-pricing-context-within-idp-parcel.md` | `../../foo/international-parcel-rating-golden-cases-v1.0.1.json` | 仓外 |
| `docs/adr/0045-new-submission-version-keeps-history-and-reestablishes-the-task.md` | `./0028-aggregate-revision-moves-only-at-the-commit-boundary.md` | 文件名写错 |
| `docs/adr/0045-new-submission-version-keeps-history-and-reestablishes-the-task.md` | `./0030-rehydration-admits-only-the-submitted-state.md` | 文件名写错 |
| `docs/adr/0054-pre-acceptance-control-policy-view-has-an-unconfigured-grade.md` | `./0029-recovery-action-is-the-error-algebra.md` | 文件名写错 |
| `docs/adr/0054-pre-acceptance-control-policy-view-has-an-unconfigured-grade.md` | `./0029-recovery-action-is-the-error-algebra.md` | 文件名写错 |
| `docs/review/081701.md` | `/D:/tops/idp-parcel/cmd/parcel-dispatch/assemble.go:28` | 本机绝对路径 + 行号 |
| `docs/review/081701.md` | `/D:/tops/idp-parcel/docs/adr/0049-publish-channel-is-in-process-delivery-until-load-evidence.md:20` | 本机绝对路径 + 行号 |
| `docs/review/081701.md` | `/D:/tops/idp-parcel/cmd/parcel-api/endpoints.go:16` | 本机绝对路径 + 行号 |
| `docs/review/081701.md` | `/D:/tops/idp-parcel/internal/platform/httpapi/router.go:39` | 本机绝对路径 + 行号 |
| `docs/review/081701.md` | `/D:/tops/idp-parcel/internal/partycommercial/domain/commercial_version.go:100` | 本机绝对路径 + 行号 |
| `docs/review/081701.md` | `/D:/tops/idp-parcel/migrations/party_commercial/0001_commercial_publication.sql:36` | 本机绝对路径 + 行号 |
| `docs/review/081701.md` | `/D:/tops/idp-parcel/internal/visibilityexception/adapters/postgres/claim_recovery.go:478` | 本机绝对路径 + 行号 |
| `docs/review/081701.md` | `/D:/tops/idp-parcel/internal/visibilityexception/application/handle_claim.go:273` | 本机绝对路径 + 行号 |
| `docs/review/081701.md` | `/D:/tops/idp-parcel/internal/networkrouting/adapters/inbox/acceptance_consumer.go:141` | 本机绝对路径 + 行号 |
| `docs/review/081701.md` | `/D:/tops/idp-parcel/internal/networkrouting/adapters/parcelshipment/route_on_acceptance.go:82` | 本机绝对路径 + 行号 |
| `docs/review/081701.md` | `/D:/tops/idp-parcel/internal/partycommercial/domain/service_product.go:47` | 本机绝对路径 + 行号 |
| `docs/review/081701.md` | `/D:/tops/idp-parcel/internal/partycommercial/adapters/postgres/commercial_authority.go:22` | 本机绝对路径 + 行号 |
| `docs/review/081701.md` | `/D:/tops/idp-parcel/docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md:155` | 本机绝对路径 + 行号 |

### C2　跨文件行号引用

- `docs/review/081701.md`：13 处 `…/文件:行号` 链接（见 X-08）。
- `docs/adr/0137-…` 正文两处「UC-CC-003 门禁表第 4 行」——已接受 ADR 正文，不改；UC-CC-003 那张表若再加行，这两句会指错，CC 组修订 UC 时留意。
- 其余扫到的 `L40`、`L32:R39`、「第 1 行是列键」都是电子表格的单元格坐标，不是行号引用。


### C3　数别处的东西

可改文档里协调者扫到的已数错一处：首发基线「全仓九份 `*_contract_test.go`」（今为 12 份，PB·L2；PB·L3、PB·L5、AG·L3 另报了几处）。另有几处未锚 SHA、今天还对的计数，修订时顺手改成定性说法即可：首发基线「全仓直接依赖三个」「全仓十处「今天没有实现」注释」、`docs/design/synthetic-demo-journey-script.md`「外部资金事实与监管凭证另有三个手工口」。`AGENTS.md`「全仓抽验六处行号引用四处已经指错（实测于 `1dbc680`）」锚了 SHA，合规。


### C4　位置引用

没有找到跨文件的「上一条 / 下一节」位置引用。文件内的位置引用（「上一条」「见下文」「上面那段」等）在可改文档里约 136 处，集中在 `docs/agents/parallel-sessions.md`（16）、首发基线（15）、`docs/design/pp-pricing-rule-model-final-design.md`（15）、PP CONTEXT（8）、`docs/agents/workflow.md`（7）；其余文件各 1–7 处。它们不违反 AGENTS.md「改文档」（那条管跨文件），但在只追加的日志式段落里（首发基线的补记、CONTEXT-MAP「上一条的两口那时答「无」」）一旦插条就会指错，修订这些段落时改成点名。ADR 正文里另有约 90 处，已接受记录正文不改，不列。

