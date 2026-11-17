# 17 销售价格来源随接受时固定的商业解析一起固定，结算有口读它

Category: enhancement
Status: in-progress——2026-11-17 Cursor 会话认领，分支 `cursor/cfmr-17-sell-price-sources-fixed-with-resolution`（worktree `idp-parcel-cfmr17`），基线 `7d6fcabea`。此前 ready-for-agent
父票：[spec](../spec.md)
Blocked by: 无。票 [16](./16-the-customer-contract-body-carries-the-sell-price-arrangement.md) 已于 2026-11-17 进 `main` `bdba71e9d`。
地盘：`internal/partycommercial`（闭包、快照与读口）、`internal/settlementaccounting/adapters/partycommercial`

对应要求：Contract「价卡授权」。规则在 ADR-0380 决定二。

## 现象

取证钉 `54b32c1b8`。接受时固定的闭包快照（`adoptedDocument`）带采用的文档种类与版本、形态、结算政策、信用依据与产品底座，没有销售价格来源。结算凭回指读这份闭包找结算账户（适配器里的 `AdoptedSettlementTerms`），读不到合同说的价。

## 要做的

1. **固定。** 一份已固定的解析采用了哪一版合同，读出的就是那一版的销售价格约定；合同之后发新版，已固定的解析不改口。办法二选一，由实现定：整份写进快照，或按快照里固定下来的那一版点读正文。两种在 `adoptedDocument` 上都有先例；选之前先看读回的门（ADR-0380「起草能力边界」写明没看）。
2. **读口。** 结算顺着找结算账户的同一条链多取一格。答复分得开三种：一份有序列表；显式不计销售价；这一版合同没有销售价格约定（旧规范化版本）。三种不合并，也不拿空列表顶。
3. **旧快照。** 没有这一格的旧快照读出来是第三种，不报错。

## 三类

全部是机制。

## 完成判据

- 委托接受之后合同发了新一版、列表不同：按原回指读到的仍是旧列表。
- 三种答复各有一条测试；跨租户读不到。
- 领域包不依赖 HTTP 与 `pgx`；结算一侧只经它自己的消费侧端口读。
- `UC-PC-002`「给开发的交接」里讲销售价格约定的那一段随之删去或改成现状。

## Comments

- 2026-11-12：由票 15 拆出。解析键上的锚点是登记行上的一个时刻（ADR-0380 越权风险点 2）：一位已登记的客户发了新一版合同，新委托能不能采用到它，取决于冻结着的 `product-strategy-boundary/06`，不在本票。
- 2026-11-12（激活）：用户在问题卡上选定激活票 16 至 21，本票转 `ready-for-agent`；此前 `draft`，等 ADR-0380 进 `main`（已在 `486a4e8fe` 进）。仍等票 16。
- 2026-11-17（票 16 的非作者评审带出的几件事，留给本票）：
  1. **票 16 没有立领域级的点读口。** 它只在客户合同目录行与按身份读回上带出销售价格约定，那是给管理台的。本票「固定」一条若选按快照里固定下来的那一版点读正文，点读口由本票立：约定在 `party_commercial.customer_contract_sell_price_arrangement`，来源在它的子表里，按声明的位置排。
  2. **本票落地之前固定下来的解析。** 票 16 进 `main` 之后、本票落地之前，带销售价格约定的合同已能发布并被解析采用，那时的闭包快照里没有这一格。本票「旧快照」一条写「没有这一格的旧快照读出来是第三种」；照字面，这些解析会被读成这一版合同没有销售价格约定，而它采用的那一版合同其实有。选整份写进快照，就要裁这一格怎样答；选按固定下来的那一版点读正文，不出现这个问题。今天只有演示库有数据，随种子重灌。
  3. **来源名的形。** 票 16 的构造门 `NewPricingScopeReference` 拒带首尾空白的名字，判据与计价一侧的 `NewPricingScopeID` 相同。两边只靠注释相连，没有测试钉在一起：领域包之间互不导入。读口把名字交给结算时照原串交，不要再规整。哪天两边判据分叉，参与方与商业收下的名字到了计价的构造门会被拒；那一格怎样答，归票 19。
- 2026-11-17（认领）：票 16 已进 `main` `bdba71e9d`，本票阻塞解除。固定办法取「按快照里固定下来的那一版点读正文」（ADR-0380 决定二的两种先例之一；票 16 评审留给本票的第 2 条：写进快照会把票 16 进 main 之后、本票落地之前固定的解析读成第三种）。
- 2026-11-17（完工，待评审）：Cursor 会话在分支 `cursor/cfmr-17-sell-price-sources-fixed-with-resolution` 上做完，基线 `7d6fcabea`。
  1. **落点。**
     - 参与方与商业：点读端口 `ContractSellPriceArrangementView` 与真库实现 `ContractSellPriceArrangements`，按（租户、合同版本）取约定，重建走票 16 的构造门 `DeclareContractSellPriceArrangement`。没有约定的版本答没有；父子对不上的行上抛。
     - 结算：领域值 `AdoptedSellPriceArrangement`（三种答复），消费侧端口 `AdoptedSellPriceArrangementView`，适配器 `AdoptedSellPriceArrangement`：回指 → 已固定的闭包 → 它采用的那一版客户合同 → 点读约定。
     - 没有迁移，闭包快照 `adoptedDocument` 没有动。
     - 文档：`dox/contexts/party-commercial.md`、`dox/contexts/settlement-accounting.md`、`dox/work.md`。
  2. **完成判据逐条。**
     - 合同发了新一版，按原回指读到的仍是旧列表 ✅ `TestALaterContractVersionDoesNotChangeTheFixedResolution`（真库）；点读口自己的那一半 `TestLoadContractSellPriceArrangementReadsTheVersionItIsAsked`。新一版的约定是直接登进册的，没有走一遍委托接受与合同发布。
     - 三种答复各有一条测试 ✅ `TestTheFixedResolutionReadsOrderedSellPriceSources`、`TestTheFixedResolutionReadsAnExplicitNoSellPrice`、`TestAContractVersionWithoutAnArrangementAnswersNotDeclared`；跨租户读不到 ✅ `TestAForeignTenantCannotReadTheFixedSellPriceArrangement`。
     - 领域包不依赖 HTTP 与 `pgx`，结算只经自己的消费侧端口读 ✅ `internal/architecture` 各门通过。
     - `UC-PC-002` 交接段：`docs/` 已冻结（`AGENTS.md`「交付模式」），这一条改为改 `dox/`，见落点。会话起先照旧规矩改了 `UC-PC-002`，提交之前已还原。
  3. **票面之外定的。**
     - 固定办法取点读，理由见认领那一条。「旧快照」一条因此没有单独的一格：快照里本来就没有这一格，新旧解析都按采用的那一版点读，那一版没有约定就答第三种。
     - 点读为什么等于固定：约定随合同版本的内容摘要走（带约定的正文按 `PCC-3` 算摘要），同一版换一份约定时摘要不同，发布答冲突；约定的写口也不覆盖。这是读发布编排与票 16 的测试得来的，本票没有另写测试。
     - 闭包没采用客户合同答 error，不折成第三种（`TestAClosureWithoutACustomerContractIsNotASellPriceAnswer`）。
     - 读口没有接进生产装配，`cmd/` 下没有调用方；用它的是票 19。
     - 结算一侧的来源名用现成的 `EstimatePricingScope`，照原串收；不计销售价的依据在结算领域里另立同名值 `NoSellPriceBasis`。
  4. **自验。** 受影响范围，含真库（共用的门禁库 `idp-parcel-postgres-gate`）：`internal/partycommercial/...`、`internal/settlementaccounting/...`、`internal/architecture/...` 全部通过；`go build ./...` 与这两个上下文的 `go vet` 退 0。全量真库测试与种子未跑，属进 `main` 的步骤。
- 2026-11-17（非作者评审，一轮）：评审者是作者会话派出的两个隔离子代理，一个做 Standards 轴、一个做 Spec 轴，拿不到作者的对话，在隔离检出上只读。评审基点 `7d6fcabea`，钉 `7431d6d8c`。照 `AGENTS.md`「交付模式」只评这一轮、只修挡上线的。
  1. **Standards 轴：无阻断。** 非阻断四条：
     - 硬违规一处：`sellPriceArrangementFrom` 的头注跨文件用了序数（「票 16 留给本票的第 3 条」），违反 `AGENTS.md`「改文档」。**已改**，见本条末。
     - 判断题三处，没有改：`LoadContractSellPriceArrangement` 分两条语句读父行与子行，同包别的读口是一条语句取回（两张表只增不改，错配会落进「恰一在场」的 error，不会静默答错）；可能的重复代码（回指换闭包那一段在结算的这个适配器包里又抄了一份，父行查询与「恰一在场」的校验各抄自写口与目录行）；`existingBasis` 这个名字照搬自写口。
     - 核过无发现：红线、ADR-0025、ADR-0380 决定一二、冻结目录、`dox/` 行数上限。
  2. **Spec 轴：无阻断。** 非阻断三条，都没有改：
     - 第三种答复只凭约定行不在判，不看合同版本的规范化版本；约定行丢了的 `PCC-3` 合同会被读成没有约定。今天没有代码路径造得出这种行。
     - 父子行对不上即上抛那一支没有测试。
     - 「合同发新版不改原回指」那条测试弱一档：新一版的约定是直接登进册的，没走发布。
     - 另核了一件：点读等于固定，在生产路径上成立——约定的写口只有发布编排一个调用方；同一版加约定或换约定，声明旧摘要时对账门答`未受理`，声明新摘要时 `SaveVersion` 答冲突，两种都在写约定之前返回；两张表在非测试代码里只有读与插入。`UC-PC-002` 改为改 `dox/` 判为合 `AGENTS.md`。
  3. **评审者自报没做的。** 两轴都没跑真库用例、`go build` 与 `go vet`，只重跑了 `internal/architecture`（Spec 轴另跑了结算领域包）。Spec 轴没有核小包托运的解析键是否总把客户合同列为必需依据，也没有读待批准发布的载体编排（「同走发布编排」是它的推断）。
  4. **评审之后的一笔（本条所在的提交）。** 改掉那句带序数的注释，只动注释；Spec 轴两条与测试缺口记进 `dox/work.md`「评审留下、不挡上线的」。作者自验：`go build ./...`、结算读参与方与商业的那个适配器包（含真库，没有跳过的用例）与 `internal/architecture` 重跑通过。没有再送评审。
