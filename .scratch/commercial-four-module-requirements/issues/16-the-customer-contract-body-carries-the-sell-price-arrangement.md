# 16 客户合同版本的正文带销售价格约定：有序的销售价格来源，或显式不计销售价

Category: enhancement
Status: in-progress——2026-11-12 Cursor 会话认领，分支 `cursor/cfmr-16-contract-sell-price-arrangement-r2`（worktree `idp-parcel-cfmr16`），基线 `9bf6e8603`。它是 2026-11-13 重切的一版：原分支 `cursor/cfmr-16-contract-sell-price-arrangement` 从 `2130a37b2` 起，其间 `main` 进了票 declared-goods-lines/11 对客户合同正文的改动，重切时并进。此前 ready-for-agent
父票：[spec](../spec.md)
Blocked by: 无
地盘：`internal/partycommercial`（领域、规范化、发布与受控批文）、`cmd/parcel-commercial`、`docs/application/party-commercial`

对应要求：Contract「价卡授权」。规则在参与方与商业 `CONTEXT.md`「客户合同版本」词条与 ADR-0380 决定一、七。

## 现象

取证钉 `54b32c1b8`。`CustomerContract` 的正文只有货主客户账户、接单规则包引用与按费用范围的财务控制约定。合同不说价；一位客户用哪套销售价，今天由他的结算账户在两本采用册上登的计价范围定。

## 要做的

1. **正文加一格销售价格约定**，二选一：一份有序的销售价格来源列表（至少一项，每项指名一个计价范围，同一范围不出现两次）；或显式声明不计销售价并留依据。形照同一份正文里的 `FinancialControlBinding`。新形状下不存在「没说」。
2. **换规范化版本号**（[ADR-0267](../../../docs/adr/0267-a-key-added-to-an-already-canonicalized-body-is-a-shape-change-even-when-old-documents-stay-byte-identical.md)）。旧版本的合同照旧读得回、摘要不变，读出来是「没有销售价格约定」，不补默认。
3. **录入、批准与发布面带上这一格**：在线口、受控批文与待批准发布的正文快照。批准与发布绑定的内容摘要随之包含它。
4. **读面带出这一格**，供管理台（票 [20](./20-admin-web-contract-page-declares-sell-price-sources.md)）与闭包固定（票 [17](./17-sell-price-sources-are-fixed-with-the-acceptance-time-closure-and-read-by-settlement.md)）用。
5. **不核范围在计价一侧有没有卡。** 参与方与商业不读计价的册（ADR-0380 决定一「参与方与商业不核列表里的范围在计价一侧有没有卡」）。

## 要裁的

- 新规范化版本要不要照计价那一侧的做法设部署开关（计价是 [ADR-0352](../../../docs/adr/0352-registering-a-price-card-in-a-canonicalization-newer-than-the-base-one-needs-an-explicit-deployment-switch-that-is-off-by-default.md)）。参与方与商业今天有没有同类的门，开工时先核。
- 「显式不计销售价」留不留，见 ADR-0380 越权风险点 9。PC owner 复核之前照 ADR 做。

## 三类

正文的形、规范化与发布面是机制。每份合同写哪几个范围、排什么顺序，是租户取值：产品不预选范围，不给默认的标准价。

## 完成判据

- 领域构造门拒收：列表为空、同一范围出现两次、两种都没声明、两种都声明、不计销售价而没有依据。
- 一份带销售价格约定的合同版本经录入、批准、发布后读回，列表顺序与录入相同；摘要随列表的内容与顺序变。
- 旧规范化版本的合同读回不变，摘要不变。
- `UC-PC-001`「给开发的交接」里讲销售价格约定的那一段随之删去或改成现状。

## Comments

- 2026-11-12：由票 15 拆出。
- 2026-11-12（激活）：用户在问题卡上选定激活票 16 至 21，本票转 `ready-for-agent`；此前 `draft`，等 ADR-0380 进 `main`（已在 `486a4e8fe` 进）。
- 2026-11-12（完工，待评审）：Cursor 会话在分支 `cursor/cfmr-16-contract-sell-price-arrangement` 上做完，基线 `2130a37b2`，代码 tip `819b28e59`（其后只改票面）。照 `/implement` › `/tdd` 逐片先红后绿。
  1. **落点。**
     - 领域：`ContractSellPriceArrangement` 与构造门 `DeclareContractSellPrice`（来源列表与不计销售价的依据两格恰一），`PricingScopeReference`、`NoSellPriceBasis`。
     - 规范化：带销售价格约定的客户合同文档写成 `PCC-3`，不带的仍是 `PCC-1`、逐字节不变。测试里那份期望串取自加这一格之前的构建。
     - 发布：声明通道 `CONTRACT_SELL_PRICE`。受控批文、录入载荷与载体快照三条路同键 `sellPriceArrangement`：`sources` 里每项一个 `pricingScope`、按顺序，或 `noSellPriceBasis`。
     - 持久化：迁移 `0049` 父子两表，来源按声明的位置保存；写口按位置比对，同一组范围换了顺序答`内容冲突`。
     - 读面：客户合同目录行与按身份读回带出这一格，没有这一层的合同答没有、不补默认。
     - 文档：`UC-PC-001`、`UC-PC-002`「给开发的交接」改成现状。
  2. **完成判据逐条。**
     - 领域构造门的拒收 ✅ `TestASellPriceArrangementDeclaresExactlyOneOfSourcesOrNoSellPrice`。「不计销售价而没有依据」与「两种都没声明」是同一格输入。
     - 经录入、批准、发布后读回顺序相同，摘要随内容与顺序变 ✅ 编排 `TestACustomerContractDraftCarriesItsSellPriceArrangementThroughApprovalAndPublication`；真库 `TestCustomerContractCatalogueReadsSellPriceSourcesBackInTheDeclaredOrder`、`TestAContractDraftWithSellPriceSourcesRoundTripsThroughTheRehydrationGate`；摘要 `TestAContractWithASellPriceArrangementIsCanonicalizedAsPCC3`。各段分别有测试，没有在起着的服务上从头到尾实走一遍。
     - 旧规范化版本的合同读回不变、摘要不变 ✅ `TestAContractWithoutASellPriceArrangementStaysByteIdenticalUnderPCC1`。
     - `UC-PC-001` 交接段改成现状 ✅。
  3. **「要裁的」两条怎样办的。**
     - 部署开关：核下来参与方与商业没有同类的门，`PCC-2` 也没有。本票没有加：加它是一项新的运维机制，要像 ADR-0352 那样另立记录。要不要加，留给 PC owner 与运维定。
     - 显式不计销售价：照 ADR-0380 做了。
  4. **票面之外定的三处。**
     - 迁移取号 0049，跳过 0048。0048 在未进 `main` 的分支上已被占用（`0048_customer_contract_rule_package_optional.sql`）。
     - 没有另立领域级的点读端口。票 17 的地盘写明读口；未接线的构造函数也过不了 `internal/architecture` 的生产接线棘轮。
     - 受控批文的翻译层自己判两个键恰一，判据同信用额度的两格。
  5. **与在途分支的重叠。** 另一会话未合入的分支（如 `feat/goods-details-admin`）带着「客户合同不必指名接单规则包」的改动，动的是客户合同正文、它的规范化、受控批文、合同载荷与合同目录查询这一批文件。本票把新代码尽量放进新文件；后进 `main` 的一方重放时合并。
  6. **自验。** 受影响范围，含真库：`internal/partycommercial/...`、`cmd/parcel-commercial`、`cmd/parcel-api`、`migrations`、`internal/platform/migrate`、`internal/architecture`，以及结算、小包托运、计价三处读参与方与商业的适配器，全部通过；`go build ./...` 与 `go vet` 退 0。全量真库测试与种子未跑，属进 `main` 的步骤。
  7. **还没有做的。** 非作者评审已派出两轴隔离子代理，未回；进 `main`。
- 2026-11-17（重切到最新的 `main`）：上一条写在原分支上，其中三处此后变了，记在这里，上一条不回改。
  1. **为什么重切。** 原分支从 `2130a37b2` 起。2026-11-12 晚 `main` 进了票 declared-goods-lines/11（客户合同不必指名接单规则包，迁移 `0048`），动的正是上一条第 5 点说的那一批文件。新分支 `cursor/cfmr-16-contract-sell-price-arrangement-r2` 从 `2da964d63` 起，把原分支的四笔代码逐笔重放。
  2. **重放的结果。** 文本上没有冲突。两边都改过的几处我逐处读过：合同目录查询里正文在不在场改看声明时刻、规范化文档里规则包成了可缺键、合同载荷与受控批文里规则包可缺，都与本票加的那一节各在各的位置，互不改写。「不带销售价格约定的合同逐字节不变」那条测试在新的底上照过。
  3. **SHA 对应**（原分支 → `-r2`）：`79d41c7ac` → `566309193`，`ca79c900e` → `5618fe40f`，`44d730960` → `99cf36a99`，`819b28e59` → `7f32f4f5a`。原分支留在远端不删。
  4. **随重切改的一处。** 迁移 `0049` 头注里讲跳过 `0048` 的那一句删了：`0048` 已在 `main`，`0049` 就是下一号。上一条第 4 点的第一项与第 5 点因此不再成立。
  5. **评审。** 2026-11-12 派出的两轴隔离子代理都在读代码途中中断（宿主报资源耗尽），没有交出报告。在 `-r2` 的 tip 上重派。
  6. **自验（在 `7f32f4f5a` 上重跑）。** 经 `scripts/test-full.sh` 起的专用一次性库跑，共用的门禁库容器当时没在跑。范围：`internal/partycommercial/...`、`cmd/parcel-commercial`、`cmd/parcel-api`、`migrations`、`internal/platform/migrate`、`internal/architecture`，以及结算、小包托运两处读参与方与商业的适配器，全部通过；`go build ./...` 与 `go vet` 退 0。全量真库测试与种子未跑，属进 `main` 的步骤。
- 2026-11-17（非作者评审与修订）：评审者是作者会话派出的两个隔离子代理，一个做 Standards 轴、一个做 Spec 轴，拿不到作者的对话，在隔离检出上只读。评审基点 `2da964d63`。
  1. **第一轮钉 `2c59eb672`，两轴都无阻断。**
     - Standards 非阻断五条：新写的跨文件计数注释；加了这一层之后失实的「三键」「三层」「另两层」；一条旧测试拿 `PCC-3` 当本构建不认的版本；同一个词有 `SellPrice` 与 `SellPriceArrangement` 两种叫法；折回时的标签核对抄了一份。
     - Spec 非阻断三条：不加部署开关的后果没记全——票 17、19 落地之前带销售价格约定的合同已能发布，而结算仍读采用登记；没有立领域级点读口，判为不算缺口，建议记到票 17；来源按原串判重，带尾空格的同名不算同一范围。
  2. **修订 `61e826497`，八条都改了。** 改名之后，完工那一条记录里的 `DeclareContractSellPrice` 现在叫 `DeclareContractSellPriceArrangement`，通道 `CONTRACT_SELL_PRICE` 现在叫 `CONTRACT_SELL_PRICE_ARRANGEMENT`，正文与目录行上的字段叫 `SellPriceArrangement`；JSON 键一个没动。来源判重那一条的改法是让 `NewPricingScopeReference` 拒带首尾空白的名字（先红后绿）：计价一侧的范围标识本来就不带首尾空白。这是票面之外新加的一条领域规则，会话定的。部署开关的后果写进了 `UC-PC-001` 交接段；点读口与窗口内固定的解析记在票 17 的 Comments。
  3. **复核钉 `61e826497`（增量 `2c59eb672..61e826497`），两轴都无阻断。** 第一轮各条判为已解决，只有改名漏了一处：迁移 `0049` 头注里还是旧名。新的非阻断：`NewPricingScopeReference` 的注释对本包别处作了一句不成立的全称断言；`UC-PC-001` 新句里有一处对别处的计数，且只说了来源不起作用、没说显式不计销售价同样不起作用；范围名的判据在两个上下文各写了一份，只靠注释相连。
  4. **再修的一笔（本条所在的提交）。** 迁移头注改成新名；那句注释改成点名 `NewPricingScopeID` 与 `NewExternalReference`；`UC-PC-001` 那句去掉计数并补上不计销售价。范围名判据各写一份没有改：领域包之间互不导入，要钉在一起得进架构门，记在票 17 的 Comments 里留给读口那张票。
  5. **评审者自报没做的。** 两轴都没跑真库用例；Standards 轴没跑测试，只对相关包跑过 `go vet`；Spec 轴只跑了不连库的几个包。「结算仍读采用登记上的计价范围」与票 17 Comments 里对窗口的推演，Standards 轴没有对着结算代码核，Spec 轴判为属实。
