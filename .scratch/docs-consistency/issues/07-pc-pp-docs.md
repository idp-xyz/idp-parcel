# 07 PC+PP（party-commercial、parcel-pricing）：CONTEXT 写出现行规则、UC 与之一致、矛盾裁清

Category: enhancement
Status: resolved——2026-10-21 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-dc07（2026-10-21 协调方立；作者子代理二轮修订后 rebase 于 main `899a45557`）
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

审计 [audit/pc-pp.md](../audit/pc-pp.md) 24 条逐条按 main 现状复核（审计钉 `6b0074d0`；其间 PC、PP 的代码与本组文档没有被任何票改过，动过的只有 ADR 的 Status 行——票 demo-chain-gaps/88 的 ADR-0231 与文档自洽 01 的合并与补指针）。另接 [02](./02-code-defects-from-the-audit.md) 交来的 code/24、code/25 两裁，与 02 交给 07 或 10 的 cross B2-05 一条；[01](./01-adr-suspension-chains.md)「交后续」没有交给本票的条目。

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
| L4 Status 行两写 | A | 已由 docs-consistency/01 消解，逐份：0104、0151 在 `3221ed51f`，0209 在 `bc8283e33`，0207 在 `caf5d8fe0`（`git log` 核过） | — |
| L5 ADR-0016 前向指针、「第三条判据」 | A | 前向指针已由 docs-consistency/01 消解（`bc8283e33`）；ADR-0146 Status 行的「第三条判据」交横切 X9 | — |
| L6 Go 注释用计数 | A（改代码注释） | 立 [code/40](../code/40-pc-pp-comments-count-things-in-other-files.md) | — |
| L7 跨文件计数与区间 | A | 已改；转录本一行交横切 X7 | UC-PC-001 `PAR-COM-*` 各行；PP CONTEXT 源完整性门禁去掉条数与案例数，改指转录本「证据层级不变」 |
| L8 「未形成」两用 | A | 已改 | PP CONTEXT Rules 两条、重放一条、「价格评价」生命周期改称计算失败（`FAILED`）并与评价未形成分开；UC-PP-001「已评价」一格 |
| L9 漏了接受前估价例外 | A | 已改 | UC-PP-001 目标与边界 |
| L10 「内部成本政策」未定义 | A | 已改 | PC CONTEXT Rules 收尾一条改写为「价格政策正文的版本点读」 |
| L11 正式与试算冲突判法不同 | A | 已改 | UC-PP-001「按评价请求形成评价」首段；PP CONTEXT Rules 评价请求一条 |
| cross B2-02、B2-03（02 登在 code/25） | B | 代决 ADR-0260 决定二、三；ADR-0148 Status 行与 Links、ADR-0157 注记 | PP CONTEXT Rules 换算一条与序列登记责任一条；PC CONTEXT「商业价格政策版本」与两阶段解析一条 |
| cross B2-05（02 交 07 或 10） | B | 本票接下：代决 [ADR-0262](../../../docs/adr/0262-reference-series-and-catalogue-review-duty-separation-is-a-tenant-governance-rule.md)；立 [code/41](../code/41-reference-review-gate-reads-a-tenant-review-duty-rule.md) | PP CONTEXT「序列版本复核」；UC-PP-001 新增一节 `AT-PP-014`、`015`；ADR-0099 Status 行与 Links |

去向计数（按主去向，审计 24 条）：已改 17（A 14：H3、M1、M2、M3、M4、M8、L1、L2、L3、L7、L8、L9、L10、L11；B 与改裁的 C 3：H2、H4、H5）；已由某票消解 4（H1 由 88，M7、L4、L5 由 01；H1、L5 各有补文档或交横切的一半）；交横切 2（M5、M6）；立代码票 1（L6）。另 cross 三条随本票代决。新 ADR 3 份（0260–0262），新代码票 2 张（code/40、41），已立的 code/24、25 解除阻塞。

### 代决摘要与越权风险点

用户授权「你作为业务和系统专家自决」，三份新 ADR 全是会话代决，Status 写明代谁、待谁复核，越权风险点见各记录文末。

- **ADR-0260（H4、H5、cross B2-02、B2-03，代 PP 与 PC owner，待 PP owner 复核；第 1、7 点同待 PC owner，第 2、5 点待 PC owner，第 6 点待 NR owner）**：正式评价、接受前估价与试算按范围从 PP 价卡登记册选卡，不读商业价格政策的定价方案绑定、不与选出的卡核对（路由自营段经 `INTERNAL` 政策的绑定得出方案那一处照 ADR-0148 不变）；体积系数与金额取整不分方向是价卡内容，PC 价格政策口径的体积一格三个方向一律不收；换算用卡绑定的汇率序列，期次按计价基准时点，口径的声明仍在政策上、由序列登记的口径引用指名，评价不读政策正文、不执行取值时点规则、不加点，路由比价同此。部分停用 ADR-0107 决定一一句、ADR-0148 决定四「比较币种与换算」一句（按口解下一句「该口径」、撤 Consequences 一项，回答其越权风险点 1）、ADR-0157 Consequences 一句（按口解，回答其越权风险点 3）。越权风险点：客户差异化销售价首发表达不了；销售与采购政策的绑定可登而不决定选卡；取值时点规则不执行；口径引用不核；撤 PC 销售体积格改了登记面；NR 比价口径；不加点。
- **ADR-0261（H2、M4 未决五，代 PC owner，待 PC owner 复核，第 3 点同待 PS owner）**：授权授予随所挂授权规则版本、合同委派随所挂客户合同版本的收尾离开新的裁定——退役即不参与、截止即按截止点收窄判用区间终点；同一请求命中多于一条授予（或委派方不同的多条委派）答`适用冲突`，不取首个。部分停用（接上）ADR-0233 决定六一条，回答其越权风险点 6。越权风险点：授权裁定多一格；不同对象的重复授予退不了场；PS 各适配器要认这一格；截止只截终点。
- **ADR-0262（cross B2-05，代 PP owner，待 PP owner 复核）**：计价参考序列与目录的人工复核按租户登记的参考复核职责规则判须否不同主体、须持哪一格授予，规则未登记时复核不放行，与价卡发布审批职责规则分立、序列与目录共用一条；免人工复核声明不变。部分停用 ADR-0099 决定二一句；ADR-0109、0101、0126 无条款停用。受控 CLI 复核走同一道门，证明不了授予；规则经 CLI 登记，种子在复核前登演示规则。越权风险点：单人租户可以自审；规则未登记时复核停住；一条规则不分种类；CLI 复核证明不了授予。

### 交横切（交 docs-consistency/10）

- **X1 GLOSSARY「可执行价卡版本」**（H4、M5）。引文：「依据已解析商业价格政策和定价方案绑定形成的不可变 `PricingPlanVersion`、`RateTableVersion` 及其计算策略版本」「发布后正文不可覆盖；变化、退役或更正形成新版本并保留历史引用」。改成什么：前句去掉「依据已解析商业价格政策和定价方案绑定形成」，改为「由 `parcel-pricing` 登记发布，评价按范围选卡（ADR-0260）」；后句改为「变化形成新版本；截止与退役是册上追加的收尾事实，不形成新版本、不改写版本行（ADR-0234）」；词头对齐 PP CONTEXT 的「定价方案版本」「价表版本」，或在 PP CONTEXT 里定义「价卡」是两者的合称。依据：ADR-0260 决定一、ADR-0234、PP CONTEXT 收尾一条。
- **X2 GLOSSARY「价格规则版本」**（H4）。引文：「`parcel-pricing` 依据该政策形成可执行价卡和纯评价」。改成什么：「`parcel-pricing` 按范围选卡、评价不读该政策（ADR-0260）；`INTERNAL` 方向政策的定价方案绑定由路由自营段用来得出评价所用的方案（ADR-0148 决定四）」。
- **X3 GLOSSARY「计价参考序列」**（M6、B2-03）。引文：「燃油费率与汇率是它的两个实例」与同词条下「商业口径归 `party-commercial`：汇率的牌价类型、取值时点规则和加点规则由商业价格政策版本化声明，计价不接受未声明口径的裸汇率。」。改成什么：实例补「按期公布的定额附加金额（ADR-0110）」，去掉计数；口径一句补「经汇率序列取值登记的口径引用指名；评价期次按计价基准时点取，不执行政策上的取值时点规则，不加点（ADR-0260 决定三）」。依据：PP CONTEXT「计价参考序列」与换算一条。
- **X4 GLOSSARY「计算目的」**（L3）。引文：「`party-commercial` 在解析商业依据时引用它」。改成什么：删去这一半句，注明 PC 的解析键用它自己的解析目的（`ACCEPTANCE_CONTROL` / `PRICING`），不引用 PP 的计算目的。依据：`partycommercial/domain.ResolutionPurpose`、UC-PC-002 两阶段契约。
- **X5 CONTEXT-MAP 两条边**（H4、M6）。引文：`party-commercial → parcel-pricing` 一边「计价校验引用闭包并形成可执行评价」；`parcel-pricing ↔ 外部数值来源` 一边「承运商公布燃油费率，财务侧提供汇率牌价，两者都是计价之外的事实」，以及外部数值一条「汇率牌价类型、取值时点和加点规则由 `party-commercial` 的商业价格政策声明」。改成什么：前者改为「计价按范围选卡、不引用 PC 解析；`INTERNAL` 方向政策的绑定经路由自营段得出方案（ADR-0260、ADR-0148）」；后者补第三个实例（定额附加金额，ADR-0110），口径一句同 X3。
- **X6 参数登记册**（ADR-0260、ADR-0262）。引文：`PAR-SET-11`「汇率口径由 `party-commercial` 的商业价格政策版本化声明」。改成什么：补「经序列取值登记的口径引用指名，评价按计价基准时点取期次（ADR-0260）」；`PAR-SET-11` 的复核责任方一格（或照 `PAR-SET-12` 另立一行）登参考复核职责规则的实例：须否不同主体、复核者须持哪一格授予，未登记时人工复核不放行（ADR-0262）。`PAR-SET-11` 另补一句：其中要求登记的取值时点规则与加点规则只作登记，不参与评价（ADR-0260 决定三）。
- **X7 计价治理案例转录本**（L7）。位置：`docs/design/pp-golden-case-transcript.md`「证据层级不变」表 `SOURCE_RATE_CARD` 一行。引文：「`SRC-DISC-001` 至 `004` 取得业务裁决前不得用于金额验证」。改成什么：与 PP CONTEXT 源完整性门禁一致——`001`、`003`、`004` 不成立、`005` 已撤销，阻断它的是 `002` 未决与金额保真度（`006`、`007`）。
- **X8 计价设计文档**（B2-02、B2-03）。位置与引文：`docs/design/pp-pricing-rule-model-final-design.md` 正文「系数的归属按价格方向不同：采购侧的系数是价卡内容，销售侧的系数属商业价格政策，两者都随各自工件版本化。」、概念表「体积系数」一行「由价卡或商业价格政策版本化声明、用于从尺寸得出体积重的除数」、小节「十、汇率口径由商业价格政策版本化声明」；同一文件「汇率比燃油多一条约束并已落地：口径（牌价类型、取值时点规则、加点规则）由商业价格政策版本化声明，读数必须携带该政策的版本引用，否则拒绝构造。」一句；`docs/design/pp-reference-design-absorption-coverage.md` 那一行里「系数归属按价格方向分立（采购侧属价卡、销售侧属商业价格政策）」。改成什么：系数不分方向是价卡内容（ADR-0260 决定二）；第十节与汇率约束那一句补决定三的读法（口径经序列取值登记的口径引用指名，评价不读政策正文、不执行取值时点规则、不加点）。与 01「交后续」交给 10 的那一处同一份文件。
- **X9 ADR-0146 Status 行**（L5 的另一半）。引文：Status 前段部分取代 ADR-0016 那一句里的「的机制半边定义与第三条判据」。改成什么：改为引文「参数显式未配置」那一条判据。ADR-0146 属治理类，不在本组。
- **X10 cross B2-02、B2-03、B2-05 已由本票裁定**（ADR-0260、ADR-0262），票 10 不再另裁；B2-05 的参数登记册一半见 X6。
- **X11 parcel-shipment 文档**（ADR-0261，协调方裁定，提示票 10；06 正在修订，本票不直接改 PS 文件）：code/24 落地后 `psports.AuthorizationOutcome` 多 `APPLICABILITY_CONFLICT` 一格；主动拒绝、撤回、资料修订、受控关闭与重开四只编排停在各自的未决并指名授权适用冲突，授权处置与完成人工复核两只各多 `AUTHORITY_APPLICABILITY_CONFLICT` 一码（逐只见 code/24）。PS 的 UC 结果表在列 `AUTHORITY_RULES_NOT_CONFIGURED` 一类结果码的地方并列补上这一格；本票检索 PS CONTEXT 与 UC 时没有列举 PC 授权答复格的句子。
- **X12 network-routing CONTEXT**（ADR-0260 决定三，评审阻断 B1）。位置：`docs/domain/network-routing/CONTEXT.md`「首发的排序形态是满足硬约束与时间可行性后按成本单维择优」那一条。引文：「换算由 `parcel-pricing` 按策略版本所引的价格政策完成」。改成什么：「换算用该段价卡绑定的汇率序列（ADR-0260 决定三），策略版本所引的价格政策不参与」。依据：ADR-0260 决定三、ADR-0148 Status 行的停用注记。
- **X13 `docs/application/README.md` 索引**（M8、M4）。引文：UC-PP-001 那一行「逐适用价卡各算一份、只算不存、结构上不交结算、不择优的机制已定（ADR-0152），待实现；真实价卡、目录与序列待提供」；UC-PC-001 一行「商业对象独立身份、不可覆盖发布、冲突和替代边界已形成，真实来源与批准参数待提供」、UC-PC-002 一行「独立商业选择锚点、唯一解析和逐项 `asOf` 两阶段机制已形成，真实锚点策略与商业版本待提供」。改成什么：UC-PP-001 去掉「待实现」，改为机制已实现、参考复核读规则那一格待 code/41；UC-PC-001、002 两行照 UC 状态行写机制已实现；补入 UC-PC-003 一行（裁定商业授权请求，`party-commercial`，裁定机制已实现，客户侧授权代表与撤回待评审）。
- **X14 settlement-accounting UC-SA-002**（ADR-0260 决定三，提示票 09 或 10）。位置：`docs/application/settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md` 参数表 `PAR-SET-11` 一行。引文：「汇率的牌价来源、牌价类型、取值时点与加点规则」。改成什么：补「取值时点规则与加点规则只作登记，评价期次按计价基准时点取、不加点（ADR-0260）」，与 X6 同口径。

全仓检索（`所引价格政策`、`价格政策口径`、`政策取值时点`、`汇率时点`、`销售侧的系数`、`系数归属按价格方向` 等字样）在别组文件里另无命中；NR UC-NR-001 `AT-NR-072` 写「按参考序列换算」，与 ADR-0260 一致，不用改。

### 检查

- scratchpad 副本（`git archive claude-dc07` 解出）上 `go run ./tools/adr-index` 重生成后 `-check` 退 0；与 main 的 README 比，变的只有 0099、0107、0148、0157、0233 五行注记与新增的 0260–0262 三行。重生成的 README 不随本票提交。
- `go run ./tools/at-coverage-counts -check`：只报计数过期（退 1），计数由推送方重算。
- 本票改动的全部 `.md` 相对链接与锚点逐个解析，全部能解析。
- 被停原句在被停方正文逐字存在（去掉加粗后比）：ADR-0107 决定一一句、ADR-0148 决定四三处引文、ADR-0157 越权风险点 3、ADR-0233 决定六一条与越权风险点 6、ADR-0099 决定二一句。
- 已接受 ADR（0099、0107、0148、0157、0233）去掉 Status 行与 Links 后与 main 逐字节相同，Status 行只在末尾追加。
- 新写的书名号引文逐条对出处检索；转述而非原文的已去掉书名号。
- 编号：新 ADR 用 0260–0262；代码票用 code/40、41（code/42 未用）；AT 新号 `AT-PC-059`–`062`、`AT-PP-013`–`015`。`.scratch/at-coverage-inventory.md` 只加行与追记，总览计数未改。顺带发现：`AT-PC-037`–`053` 本就不在盘点里，本票只为改了预期的 `AT-PC-042` 加了一行。
- `git diff main --stat` 只含：PC、PP 两份 CONTEXT；UC-PC-001、002、003，UC-PP-001；新 ADR 0260–0262；ADR-0099、0107、0148、0157、0233 的 Status 行与 Links；`.scratch/at-coverage-inventory.md`；`.scratch/docs-consistency/` 下 code/24、25、40、41 与本票。未改代码。

## 评审记录

### 隔离评审第 1 轮（2026-10-21，阻断 2 条）与作者回应（main 仍是 `3619f5c0e`，未 rebase）

阻断：

- **B1 NR CONTEXT 与 ADR-0260 决定三相抵、交横切没登。** 补 X12（位置、引文、改法、依据）；按评审要求全仓检索同类说法，另补 X14（UC-SA-002 `PAR-SET-11` 一行），设计文档两份并进 X8，NR 其余文件无命中。
- **B2 ADR-0157 那一句是按口解，不是只回答风险点。** 0157 注记改为部分停用（按口解），逐字引 Consequences「计价侧没有汇率时点执行器，那一格仍答`未配置`，不在本记录里补执行器。」一句，适用场景「评价换算的期次」，越权风险点 3 并进同一条；Links 同步。ADR-0260 Status 前段的去向、「对已接受记录的作用」与 Links 补这一项。

建议与顺手，全部照办：

1. L4 的消解 SHA 逐份改正：0104、0151 在 `3221ed51f`，0209 在 `bc8283e33`，0207 在 `caf5d8fe0`（`git log 6b0074d0..main` 逐份核过）。
2. X8 补设计文档里「销售侧的系数属商业价格政策」一句、概念表「体积系数」一行，与 `pp-reference-design-absorption-coverage.md` 那一句。
3. `docs/application/README.md` 索引登进交横切 X13。
4. X6 补 `PAR-SET-11` 的取值时点规则与加点规则只作登记、不参与评价。
5. ADR-0148 注记那条停用后加「越权风险点 1 随之回答」；ADR-0260 Status 前段、作用一节、Links 同步。
6. code/24 第 4 条照协调方裁定写具体：`psports.AuthorizationOutcome` 加 `APPLICABILITY_CONFLICT`；五只适配器（核过撤回也经 PC 裁定编排询问，`withdrawal_authorization.go` 调同一个 `Handle`）逐只列名；有未决格的四只编排加原因，没有未决格的授权处置、完成人工复核两只各加一码。ADR-0261 越权风险点 3 与 Consequences 改为同一份清单，并写明今天的事实（落 `default` 交回 error，即未形成，不被读成已授权或拒绝）。交横切 X11 提示票 10。
7. ADR-0262 决定一改准：操作者面取信封与授予集，受控 CLI 只带 `reviewer` 串、带不出授予；CLI 走同一道门，规则要求授予时一律答复核者不合格。规则经 `parcel-pricing-register` 新增一种登记登入，种子在复核之前登演示规则（登记者与复核者本就不同，动线不退成未配置）。越权风险点补第 4 条，code/41 逐项写具体。
8. code/25 补：种子政策撤掉 `volumetricFactor` 后在已灌过的库上重跑答内容冲突，照 README 先例用 `--reset`。
9. 去掉新文字里数别处的数：ADR-0260 Context 的口径格、ADR-0261 越权风险点 1 与 3、UC-PC-003「首切范围与现状」都改为列名。
10. UC-PC-001 结果码表改用符号 `RegisterPartyIdentityHandler`；UC-PC-003「首切范围与现状」补适用冲突今天不在 PS 适配器的翻译集合内（code/24）；ADR-0260 Status 的复核人标注与正文各点对齐（第 1、7 点 PP 与 PC，第 2、5 点 PC，第 6 点 NR）；票面去掉「cross B2-05 → 07 或 10」的书名号。

未采纳：无。

检查（修后）：scratchpad 副本（`git archive claude-dc07` 解出）上 `go run ./tools/adr-index` 重生成后 `-check` 退 0；改动的 `.md` 相对链接与锚点全部解析；ADR-0157、0148 注记与 ADR-0260 作用一节里新引的被停原句在被停方正文逐字存在（去加粗比）；已接受 ADR 去掉 Status 行与 Links 后与 main 逐字节相同。

### 隔离评审第 2 轮（2026-10-21，第 1 轮 B1、B2 已消；新阻断 3 条）与作者回应

阻断：

1. **ADR-0262 写代写复核照旧不经此门，与代码不符。** 核过：`application/feed_reference_series.go` 的免复核分支调同一只 `ReviewReferenceSeriesHandler`，复核者为 `binding.ConnectorIdentity()`，今天就受不同主体比较。照协调方裁定改：代写复核不读规则、规则未登记也不答未配置，写死的不同主体比较在代写一路保留，落法是复核命令带「免复核声明下的系统代写」一格。ADR-0262 决定一末条与索引注记写明今天的事实与这条裁定，越权风险点补第 5 条「代写一路不受租户复核职责规则约束」；code/41 第 4 条点名 `feed_reference_series.go` 写出绕行方式；UC-PP-001 新增 `AT-PP-016`，AT 盘点入②。
2. **ADR-0261 越权风险点 3 与 Consequences 和 code/24 写成同一份清单**：主动拒绝、撤回、资料修订、受控关闭与重开四只编排停在未决并指名授权适用冲突，授权处置与完成人工复核两只各加 `AUTHORITY_APPLICABILITY_CONFLICT`。
3. **ADR-0260 Consequences** 改为「ADR-0107、ADR-0148、ADR-0157（按口解）的 Status 行与 Links 挂前向指针」。

建议与顺手，全部照办：

- N1 ADR-0260 Consequences 末条交横切清单补 NR CONTEXT 排序那一条（X12）与 UC-SA-002 `PAR-SET-11`（X14）。
- N2 ADR-0262 Consequences 与 code/41 第 2 条：写口给受控 CLI、装配与测试用，注明有意不照 `PriceCardApprovalDutyRule` 的先例。
- N3 核过 `cmd/parcel-api` 只有 `/pricing-reference-series-reviews`、没有目录复核端点：ADR-0262 越权风险点 4 如实写要求授予的租户无法复核目录；目录复核的操作者面端点（`/pricing-reference-catalogue-reviews`）并进 code/41 第 4a 条，不另立票——它是同一道门的另一个入口，门改了而入口缺着，code/41 的验收就不完整。
- N4 `AT-PP-015` 补 CLI 那一支（规则要求授予时一律答复核者不合格），code/41 第 6 条随之改。
- N5 code/24 补：`dispose_shipment_request` 在生产上装 `UnconfiguredAuthorizedDispositionAuthorizer`（不问 PC，ADR-0132 越权风险点 2），给它加的那一码今天走不到，只为闭集穷尽。
- N6 PP CONTEXT 里 `006` 见下条金额保真度的写法改为点名「金额保真度：不可默认成立」一条；UC-PC-001、UC-PC-003、UC-PP-001 的「规则如上文」改为点名小节。
- N7 UC-PC-003 `AT-PC-061` 委派缺席答不允许的出处改为 ADR-0116 决定三（折法仍指 ADR-0261 决定二）；受控关闭与重开各有授权适配器的说法改为两者共用 `continued_attempt_decision_authorization.go` 一只。
- N8 X6 去掉 `PAR-SET-02` 那句条件句；设计文档里汇率比燃油多一条约束的那一句并进 X8。

未采纳：无。

检查（修后，rebase 于 main `899a45557`，AT 盘点与 06 的行与追记取并集）：scratchpad 副本上 `go run ./tools/adr-index` 重生成后 `-check` 退 0；改动的 `.md` 相对链接与锚点全部解析；0099、0107、0148、0157、0233 注记里的被停原句在被停方正文逐字存在，五份去掉 Status 行与 Links 后与 main 逐字节相同；本轮新写的书名号引文逐条对出处检索，引旧写法或转述的去掉了书名号。

## 进 main 记录（2026-10-21，协调方）

- 分支 `claude-dc07` tip `9e36491e9` 在 main `899a45557` 上快进；两轮隔离评审的阻断全部消掉，第二轮修订由协调方抽核（ADR-0262 代写复核一路的裁定、ADR-0261 越权风险点 3 与 Consequences 与 code/24 同一份清单、ADR-0260 Consequences 改为按口解）。
- 协调方随后在 tip 上单独提交：重生成 `docs/adr/README.md`、重算 AT 盘点总览计数；两者 `-check` 退 0。机制清点重生成无变化；`go build ./...` 干净。本票只动文档，未跑全量真库测试。
- 去向：代码票 code/40、41；code/24、25 已写裁向、解除 Blocked by；交横切 X1–X14 交票 10。
