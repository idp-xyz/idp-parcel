# 07 PC+PP（party-commercial、parcel-pricing）：CONTEXT 写出现行规则、UC 与之一致、矛盾裁清

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dc07（2026-10-21 协调方立；作者子代理 rebase 于 main `3619f5c0e`）
Blocked by: 无（01 进 main 后开工）
出处：[audit/pc-pp.md](../audit/pc-pp.md)（pc-pp.md）；[02 的完成记录](./02-code-defects-from-the-audit.md)里「交修订票」一表中交给本票的条目；[01](./01-adr-suspension-chains.md) 的「交后续」一节里交给本票的条目。

## 要什么

1. 按 main 现状逐条复核审计报告里本组的 A、B 类条目（审计钉 `6b0074d0`；其后进 main 的票与文档自洽 01、02 可能已消解一些——消解的写「已由某票消解」并指 SHA）。
2. **A 类**：改文档。范围 docs/domain/party-commercial/、docs/domain/parcel-pricing/ 的 CONTEXT.md 与对应 docs/application/，以及本组新立 ADR 与它所停用的 ADR 的 Status 行与 Links。目标照 README「目标」1、2、4、5：CONTEXT 写出今天生效的语言、不变量与生命周期（已被 ADR 部分停用的句子不再以现行口吻留着，改写为现行规则并引用 ADR）；UC 同一场景只有一套结果，结果名与代码一致；不用行号、计数、位置引用。
3. **B 类**：按用户授权（「你作为业务和系统专家自决」）代决，立新 ADR（号段 0260–0262），Status 写法照近期 ADR，写越权风险点，部分停用照 `docs/adr/README.md` 顶部规则（文档自洽 01 改过的「一件事只写一处」）；裁完 CONTEXT 与 UC 随之改。裁定若要求改代码，不在本票改：在 [code/](../code/) 下补一张代码票（编号接 code/ 现有最大号），或在 02 已立的对应代码票里写明裁向并解除其 Blocked by。
4. **交横切**：需要改 GLOSSARY、CONTEXT-MAP、SCENARIOS、`docs/product/`、`docs/agents/`、别组 CONTEXT/UC 的，不在本票改，逐条写进本票「交横切」一节（位置、引文、改成什么、依据），由票 10 统一改。

## 不变式

- 已接受 ADR 正文不改；只动本组文档、本组新 ADR 及其停用对象的 Status 行与 Links。
- 不改代码；不改 GLOSSARY、CONTEXT-MAP 等横切文件。
- `go run ./tools/adr-index -check`（重生成后）退 0。

## 完成判据

- 审计报告本组每一条 A、B 都有去向（已改 / 已由某票消解 / 交横切 / 立代码票）。
- 完成记录里一张对照表：审计条目 → 去向。

## 完成记录（2026-10-21，分支 claude-dc07，rebase 于 main `3619f5c0e`）

审计 [audit/pc-pp.md](../audit/pc-pp.md) 24 条逐条按 main 现状复核（审计钉 `6b0074d0`；其间 PC、PP 的代码与本组文档没有被任何票改过，动过的只有 ADR 的 Status 行——票 demo-chain-gaps/88 的 ADR-0231 与文档自洽 01 的合并与补指针）。另接 [02](./02-code-defects-from-the-audit.md) 交来的 code/24、code/25 两裁与「cross B2-05 → 07 或 10」一条；[01](./01-adr-suspension-chains.md)「交后续」没有交给本票的条目。

### 审计条目 → 去向

| 条目 | 归类 | 去向 | 落在哪 |
|---|---|---|---|
| H1 收尾之后在途委托的发生项被挡 | B | 已由 demo-chain-gaps/88 消解（ADR-0231，`d238b6023`）：委托不随 PC 收尾改指，委托协议换版承接，运营约束停用；本票只补文档 | PC CONTEXT Rules 新增「收尾不改写既有快照，也不改指既有委托」一条，「供应商商业协议版本」「商业规则与政策版本」生命周期；UC-PC-001 一致性一节、`AT-PC-058` |
| H2 授权规则、合同收尾后授予与委派照旧生效 | C（按 code/24 的二选一改裁） | 代决 [ADR-0261](../../../docs/adr/0261-authority-grants-and-contract-delegations-follow-the-closure-of-their-version-and-several-hits-are-an-applicability-conflict.md)；已改；code/24 写明裁向、解除 Blocked by | PC CONTEXT Rules 授予与委派一条、「客户合同版本」「商业规则与政策版本」生命周期；UC-PC-003 全面改写、新立 `AT-PC-059`–`061`；ADR-0233 Status 行与 Links |
| H3 服务产品「自退役生效时间起」 | A | 已改 | PC CONTEXT「服务产品版本」生命周期、Rules 收尾一条补退役时刻取登记时刻 |
| H4 选卡的权威有两套 | B | 代决 [ADR-0260](../../../docs/adr/0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md) 决定一；已改；GLOSSARY、CONTEXT-MAP 交横切 X1、X2、X5 | PC CONTEXT「商业价格政策版本」「定价方案绑定」「已解析定价依据」与 Rules 绑定一条；PP CONTEXT「计价输入快照」、生命周期待判断一行、Boundaries；UC-PC-002 一致性一节与交接 |
| H5 SELL 体积系数与金额取整 | C（按 code/25 的二选一改裁） | 代决 ADR-0260 决定二；已改；code/25 写明裁向（认价卡）、解除 Blocked by | PP CONTEXT「金额取整策略」「体积系数」与 Rules 体积一条；PC CONTEXT「商业价格政策版本」；UC-PC-001 `AT-PC-062`；ADR-0107 Status 行与 Links |
| M1 `AT-PC-036` 两种结果 | A | 已改 | UC-PC-002 `AT-PC-036` |
| M2 锚点身份归输入未受理 | A | 已改 | UC-PC-002 结果表「输入未受理」一行与其后一段 |
| M3 UC-PC-001 结果语义与流程 | A | 已改 | UC-PC-001 元数据、结果表（去「适用冲突」、来源冲突改内容冲突、补未受理与已收尾）、首发结果码对照表、步骤 4、一致性一节、`AT-PC-003`、`006`、`007`、`011`、`033` |
| M4 UC-PC-003 现状过时 | A（未决五另由 ADR-0261 裁） | 已改 | UC-PC-003 状态行、「首切范围与现状」、「建模未决与已作的裁定」、`AT-PC-042`、交接一节 |
| M5 GLOSSARY「可执行价卡版本」 | A | 交横切 X1 | — |
| M6 计价参考序列实例两种对三种 | A | 交横切 X3、X5 | — |
| M7 ADR-0202 决定四与 ADR-0233 决定五 | A | 已由 docs-consistency/01 消解（`bc8283e33`）：ADR-0202 注记已挂「由 ADR-0233 决定五回答」，审计给的「至少补前向指针」那一支；本票复核同意按回答处理（0202 原句自带「若 PC owner 裁定要折入，由 PC 在点读口一处交出」） | — |
| M8 三份 UC 按「只许夹具」写 | A | 已改 | UC-PC-001、UC-PC-002、UC-PP-001 状态行与交接；UC-PC-002 首发试点叠加条件末条 |
| L1 受控案例校验门 | A | 已改 | PP CONTEXT Rules 价卡发布一条 |
| L2 「已替代」「已生效」与对象清单 | A | 已改 | PC CONTEXT Rules 共同不变量清单补客户服务规则；「客户服务规则版本」生命周期改同一判法；「商业规则与政策版本」生命周期写明`已替代`的写法另定、绑定不是独立对象；PP CONTEXT「定价方案与价表版本」生命周期写截止与退役 |
| L3 「计算目的」 | A | UC 已改；GLOSSARY 交横切 X4 | UC-PC-002 两阶段契约与一致性一节改称解析目的 |
| L4 Status 行两写 | A | 已由 docs-consistency/01 消解（`3221ed51f`），0209、0104、0207、0151 一并 | — |
| L5 ADR-0016 前向指针、「第三条判据」 | A | 前向指针已由 docs-consistency/01 消解（`bc8283e33`）；ADR-0146 Status 行的「第三条判据」交横切 X9 | — |
| L6 Go 注释用计数 | A（改代码注释） | 立 [code/40](../code/40-pc-pp-comments-count-things-in-other-files.md) | — |
| L7 跨文件计数与区间 | A | 已改；转录本一行交横切 X7 | UC-PC-001 `PAR-COM-*` 各行；PP CONTEXT 源完整性门禁去掉条数与案例数，改指转录本「证据层级不变」 |
| L8 「未形成」两用 | A | 已改 | PP CONTEXT Rules 两条、重放一条、「价格评价」生命周期改称计算失败（`FAILED`）并与评价未形成分开；UC-PP-001「已评价」一格 |
| L9 漏了接受前估价例外 | A | 已改 | UC-PP-001 目标与边界 |
| L10 「内部成本政策」未定义 | A | 已改 | PC CONTEXT Rules 收尾一条改写为「价格政策正文的版本点读」 |
| L11 正式与试算冲突判法不同 | A | 已改 | UC-PP-001「按评价请求形成评价」首段；PP CONTEXT Rules 评价请求一条 |
| cross B2-02、B2-03（02 登在 code/25） | B | 代决 ADR-0260 决定二、三；ADR-0148 Status 行与 Links、ADR-0157 注记 | PP CONTEXT Rules 换算一条与序列登记责任一条；PC CONTEXT「商业价格政策版本」与两阶段解析一条 |
| cross B2-05（02 交「07 或 10」） | B | 本票接下：代决 [ADR-0262](../../../docs/adr/0262-reference-series-and-catalogue-review-duty-separation-is-a-tenant-governance-rule.md)；立 [code/41](../code/41-reference-review-gate-reads-a-tenant-review-duty-rule.md) | PP CONTEXT「序列版本复核」；UC-PP-001 新增一节 `AT-PP-014`、`015`；ADR-0099 Status 行与 Links |

去向计数（按主去向，审计 24 条）：已改 17（A 14：H3、M1、M2、M3、M4、M8、L1、L2、L3、L7、L8、L9、L10、L11；B 与改裁的 C 3：H2、H4、H5）；已由某票消解 4（H1 由 88，M7、L4、L5 由 01；H1、L5 各有补文档或交横切的一半）；交横切 2（M5、M6）；立代码票 1（L6）。另 cross 三条随本票代决。新 ADR 3 份（0260–0262），新代码票 2 张（code/40、41），已立的 code/24、25 解除阻塞。

### 代决摘要与越权风险点

用户授权「你作为业务和系统专家自决」，三份新 ADR 全是会话代决，Status 写明代谁、待谁复核，越权风险点见各记录文末。

- **ADR-0260（H4、H5、cross B2-02、B2-03，代 PP 与 PC owner，待 PP owner 复核，第 1、2、5、7 点同待 PC owner，第 6 点同待 NR owner）**：正式评价、接受前估价与试算按范围从 PP 价卡登记册选卡，不读商业价格政策的定价方案绑定、不与选出的卡核对（路由自营段经 `INTERNAL` 政策的绑定得出方案那一处照 ADR-0148 不变）；体积系数与金额取整不分方向是价卡内容，PC 价格政策口径的体积一格三个方向一律不收；换算用卡绑定的汇率序列，期次按计价基准时点，口径的声明仍在政策上、由序列登记的口径引用指名，评价不读政策正文、不执行取值时点规则、不加点，路由比价同此。部分停用 ADR-0107 决定一一句、ADR-0148 决定四「比较币种与换算」一句（按口解下一句「该口径」、撤 Consequences 一项），回答 ADR-0157 越权风险点 3。越权风险点：客户差异化销售价首发表达不了；销售与采购政策的绑定可登而不决定选卡；取值时点规则不执行；口径引用不核；撤 PC 销售体积格改了登记面；NR 比价口径；不加点。
- **ADR-0261（H2、M4 未决五，代 PC owner，待 PC owner 复核，第 3 点同待 PS owner）**：授权授予随所挂授权规则版本、合同委派随所挂客户合同版本的收尾离开新的裁定——退役即不参与、截止即按截止点收窄判用区间终点；同一请求命中多于一条授予（或委派方不同的多条委派）答`适用冲突`，不取首个。部分停用（接上）ADR-0233 决定六一条，回答其越权风险点 6。越权风险点：授权裁定多一格；不同对象的重复授予退不了场；PS 各适配器要认这一格；截止只截终点。
- **ADR-0262（cross B2-05，代 PP owner，待 PP owner 复核）**：计价参考序列与目录的人工复核按租户登记的参考复核职责规则判须否不同主体、须持哪一格授予，规则未登记时复核不放行，与价卡发布审批职责规则分立、序列与目录共用一条；免人工复核声明不变。部分停用 ADR-0099 决定二一句；ADR-0109、0101、0126 无条款停用。越权风险点：单人租户可以自审；规则未登记时复核停住；一条规则不分种类。

### 交横切（交 docs-consistency/10）

- **X1 GLOSSARY「可执行价卡版本」**（H4、M5）。引文：「依据已解析商业价格政策和定价方案绑定形成的不可变 `PricingPlanVersion`、`RateTableVersion` 及其计算策略版本」「发布后正文不可覆盖；变化、退役或更正形成新版本并保留历史引用」。改成什么：前句去掉「依据已解析商业价格政策和定价方案绑定形成」，改为「由 `parcel-pricing` 登记发布，评价按范围选卡（ADR-0260）」；后句改为「变化形成新版本；截止与退役是册上追加的收尾事实，不形成新版本、不改写版本行（ADR-0234）」；词头对齐 PP CONTEXT 的「定价方案版本」「价表版本」，或在 PP CONTEXT 里定义「价卡」是两者的合称。依据：ADR-0260 决定一、ADR-0234、PP CONTEXT 收尾一条。
- **X2 GLOSSARY「价格规则版本」**（H4）。引文：「`parcel-pricing` 依据该政策形成可执行价卡和纯评价」。改成什么：「`parcel-pricing` 按范围选卡、评价不读该政策（ADR-0260）；`INTERNAL` 方向政策的定价方案绑定由路由自营段用来得出评价所用的方案（ADR-0148 决定四）」。
- **X3 GLOSSARY「计价参考序列」**（M6、B2-03）。引文：「燃油费率与汇率是它的两个实例」与同词条下「商业口径归 `party-commercial`：汇率的牌价类型、取值时点规则和加点规则由商业价格政策版本化声明，计价不接受未声明口径的裸汇率。」。改成什么：实例补「按期公布的定额附加金额（ADR-0110）」，去掉计数；口径一句补「经汇率序列取值登记的口径引用指名；评价期次按计价基准时点取，不执行政策上的取值时点规则，不加点（ADR-0260 决定三）」。依据：PP CONTEXT「计价参考序列」与换算一条。
- **X4 GLOSSARY「计算目的」**（L3）。引文：「`party-commercial` 在解析商业依据时引用它」。改成什么：删去这一半句，注明 PC 的解析键用它自己的解析目的（`ACCEPTANCE_CONTROL` / `PRICING`），不引用 PP 的计算目的。依据：`partycommercial/domain.ResolutionPurpose`、UC-PC-002 两阶段契约。
- **X5 CONTEXT-MAP 两条边**（H4、M6）。引文：`party-commercial → parcel-pricing` 一边「计价校验引用闭包并形成可执行评价」；`parcel-pricing ↔ 外部数值来源` 一边「承运商公布燃油费率，财务侧提供汇率牌价，两者都是计价之外的事实」，以及外部数值一条「汇率牌价类型、取值时点和加点规则由 `party-commercial` 的商业价格政策声明」。改成什么：前者改为「计价按范围选卡、不引用 PC 解析；`INTERNAL` 方向政策的绑定经路由自营段得出方案（ADR-0260、ADR-0148）」；后者补第三个实例（定额附加金额，ADR-0110），口径一句同 X3。
- **X6 参数登记册**（ADR-0260、ADR-0262）。引文：`PAR-SET-11`「汇率口径由 `party-commercial` 的商业价格政策版本化声明」。改成什么：补「经序列取值登记的口径引用指名，评价按计价基准时点取期次（ADR-0260）」；`PAR-SET-11` 的复核责任方一格（或照 `PAR-SET-12` 另立一行）登参考复核职责规则的实例：须否不同主体、复核者须持哪一格授予，未登记时人工复核不放行（ADR-0262）。`PAR-SET-02` 若写到销售方向体积系数归政策，同改为价卡。
- **X7 计价治理案例转录本**（L7）。位置：`docs/design/pp-golden-case-transcript.md`「证据层级不变」表 `SOURCE_RATE_CARD` 一行。引文：「`SRC-DISC-001` 至 `004` 取得业务裁决前不得用于金额验证」。改成什么：与 PP CONTEXT 源完整性门禁一致——`001`、`003`、`004` 不成立、`005` 已撤销，阻断它的是 `002` 未决与金额保真度（`006`、`007`）。
- **X8 计价规则模型设计**（B2-03）。位置：`docs/design/pp-pricing-rule-model-final-design.md` 小节「十、汇率口径由商业价格政策版本化声明」。改成什么：补 ADR-0260 决定三的读法（口径经序列取值登记的口径引用指名，评价不读政策正文、不执行取值时点规则、不加点）。与 01「交后续」交给 10 的那一处同一份文件。
- **X9 ADR-0146 Status 行**（L5 的另一半）。引文：Status 前段部分取代 ADR-0016 那一句里的「的机制半边定义与第三条判据」。改成什么：改为引文「参数显式未配置」那一条判据。ADR-0146 属治理类，不在本组。
- **X10 cross B2-02、B2-03、B2-05 已由本票裁定**（ADR-0260、ADR-0262），票 10 不再另裁；B2-05 的参数登记册一半见 X6。
- **X11 parcel-shipment 文档**（ADR-0261，提示票 06 或 10）：PC 授权裁定多了`适用冲突`一格，PS 各授权适配器要把它译成未决。本票检索 PS CONTEXT 与 UC 没有列举 PC 授权答复格的句子，暂无要改的文字；06 若新写这类列举，照 UC-PC-003 结果表。

### 检查

- scratchpad 副本（`git archive claude-dc07` 解出）上 `go run ./tools/adr-index` 重生成后 `-check` 退 0；与 main 的 README 比，变的只有 0099、0107、0148、0157、0233 五行注记与新增的 0260–0262 三行。重生成的 README 不随本票提交。
- `go run ./tools/at-coverage-counts -check`：只报计数过期（退 1），计数由推送方重算。
- 本票改动的全部 `.md` 相对链接与锚点逐个解析，全部能解析。
- 被停原句在被停方正文逐字存在（去掉加粗后比）：ADR-0107 决定一一句、ADR-0148 决定四三处引文、ADR-0157 越权风险点 3、ADR-0233 决定六一条与越权风险点 6、ADR-0099 决定二一句。
- 已接受 ADR（0099、0107、0148、0157、0233）去掉 Status 行与 Links 后与 main 逐字节相同，Status 行只在末尾追加。
- 新写的书名号引文逐条对出处检索；转述而非原文的已去掉书名号。
- 编号：新 ADR 用 0260–0262；代码票用 code/40、41（code/42 未用）；AT 新号 `AT-PC-059`–`062`、`AT-PP-013`–`015`。`.scratch/at-coverage-inventory.md` 只加行与追记，总览计数未改。顺带发现：`AT-PC-037`–`053` 本就不在盘点里，本票只为改了预期的 `AT-PC-042` 加了一行。
- `git diff main --stat` 只含：PC、PP 两份 CONTEXT；UC-PC-001、002、003，UC-PP-001；新 ADR 0260–0262；ADR-0099、0107、0148、0157、0233 的 Status 行与 Links；`.scratch/at-coverage-inventory.md`；`.scratch/docs-consistency/` 下 code/24、25、40、41 与本票。未改代码。

