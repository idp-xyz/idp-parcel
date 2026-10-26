# 25 PP SELL 方向的体积系数与金额取整：文档归商业价格政策，代码与演示种子只读价卡

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code25（基于 main `9ea060682`），隔离评审无阻断、修订轮 N1–N6 已处置，见文末「完成记录」「评审记录与修订轮」。此前 ready-for-agent——R2（上线后尽快）：SELL 评价已在生产路径上；PC 价格政策口径对 SELL 必填体积系数，PP 却只读卡上那一份，两处各自合法，合同政策上声明的那个静默不起作用。实施时两处登同值即可绕开（可靠，但全靠人），所以列 R2；租户若按合同政策调系数而不改卡，计费重量与金额就与预期不符。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无——2026-10-21 文档自洽 07 已裁（[ADR-0260](../../../docs/adr/0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md)）：**认价卡**。原 Blocked by：docs-consistency/07 先裁「SELL 评价的计价条款（金额取整、体积系数、汇率口径）从哪一份工件读」——即 audit/pc-pp.md H5 的 B 路与 audit/cross.md B2-02、B2-03 同一裁。
出处：[audit/pc-pp.md](../audit/pc-pp.md) H5（审计归 C，并注明 owner 若认定 SELL 卡自带系数与取整就是首发形态则改归 B）；同根的 [audit/cross.md](../audit/cross.md) B2-02（金额取整，B）、B2-03（汇率口径，B）在本票登记、不另立。审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，PC、PP 代码与种子在此期间未改。

## 现象

PP CONTEXT、PC CONTEXT 与 ADR-0107 都说销售方向的体积系数与金额取整属商业价格政策；PP 代码不分方向，一律读价卡上的系数与取整，`internal/parcelpricing` 里没有任何地方读 PC 价格政策口径。PC 价格政策口径只有税务、体积、汇率三格，没有金额取整。

## 证据

- 文档与决定：
  - PP CONTEXT「体积系数」：「采购方向的系数属价卡内容，销售方向的系数属商业价格政策」；「金额取整策略」：「采购方向的策略属价卡内容，销售方向的属商业价格政策」。
  - PC CONTEXT「商业价格政策版本」：「……以及销售方向的体积系数……`parcel-pricing` 按声明的口径解析取值」。
  - ADR-0107 决定一（同上，与体积系数两侧归属同构）。
  - 相反一侧（cross B2-02）：ADR-0190 三类把「价卡的金额取整声明」列为租户取值；ADR-0198 决定二「卡没声明金额取整，请求记`待判断`」；ADR-0179 决定三选 SELL 卡「不走 PC 价格政策的定价方案绑定」。
- 代码与种子：
  - PC `internal/partycommercial/domain/volumetric_caliber.go` 的 `NewVolumetricCaliber`：SELL 必须声明系数引用，BUY / INTERNAL 不得声明。
  - PP `domain.PricingPlanVersion` 不分方向携带卡自己的 `VolumetricFactor` 与金额取整；`domain/input.go` 的 `deriveVolumetricWeight` 只读 `policy.VolumetricFactor()`。
  - `scripts/demo-seeds/data/pricing/price-card-cn-sg.json`（`direction: SELL`）自带 `weightPolicy.volumetricFactor`（除数 5000）与 `amountRounding`（`HALF_UP`）。
  - PC `domain.PricePolicyCaliber` 只有 tax、volumetric、fx 三格。

## 三类

机制：SELL 评价从裁定的那一份工件取计价条款，另一处拒收或不再要求。产品策略：哪一份是权威（待 07 裁）。租户取值：系数、取整、汇率口径的具体值。

## 规模

M（裁为价卡：撤掉 PC 的 SELL 体积口径必填，改 PC 校验与登记；裁为商业价格政策：PP 的 SELL 评价读 PC 口径，SELL 卡拒收这几格，按 ADR-0179 决定三「两处同一笔改」一并改选卡，L）。

## 最小的诚实修法

两种裁向都要做到「同一格只有一处合法声明」：

- 认价卡：PC 的 SELL 体积口径改为不要求（或撤掉），已登的行如实标「不参与计价」；PP CONTEXT 与 ADR-0107 由 07 改。
- 认商业价格政策：PP 的 SELL 评价从 PC 口径取系数、取整与汇率口径；SELL 卡上出现这几格即拒收；PC 口径补金额取整一格。

## 要裁的

全部在 Blocked by 那一裁里。

## 裁向（2026-10-21，文档自洽 07）

[ADR-0260](../../../docs/adr/0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md) 取「认价卡」：体积系数与金额取整不分方向是价卡内容，换算用卡绑定的汇率序列（汇率口径的声明仍在商业价格政策上，由序列取值登记的口径引用指名）；按范围选卡不读价格政策的定价方案绑定。PP 不改。照此实施：

1. PC：`domain.NewVolumetricCaliber` 对三个方向一律不收系数引用（`SELL` 不再必填，带了即拒，判法与今天的 `BUY`、`INTERNAL` 相同）；受控批文与管理台表单的价格政策口径去掉体积一格；已发布的口径行原样留在册上，读面（价格政策目录）把那一格标「不参与计价」。若价格政策正文的规范化文档形状因此变化，照 ADR-0014 判换不换号。
2. PC：`FxCaliber` 注释里「消费方据以停在未决，不得读成零加点」那半句（缺加点一格时的读法）改为 ADR-0260 决定三的口径（评价不读政策正文、不执行取值时点规则、不加点）；`VolumetricCaliber` 注释里「销售方向没有承运商卡，只能由商业政策声明」那条理由与它的测试头注随第 1 条改写。
3. 种子：`scripts/demo-seeds/data/commercial/publish-batch.json` 的 `SYN-PRICE-RULE-CN-SG` 口径去掉 `volumetricFactor`，摘要随之重算；种子卡 `price-card-cn-sg.json` 不动。同一版本号换了内容，在已灌过的库上重跑会答内容冲突，照 `scripts/demo-seeds/README.md` 的先例用 `--reset` 复灌，并在该 README 记一句。汇率序列 `reference-series-fx-cny-sgd.json` 的 `quoteBasis` 指的是同一对象与版本号，不随摘要变。
4. NR：路由策略版本的比较价格政策一格（`ports.RouteStrategyDefinitionVersion.ComparisonPricePolicy` 一路，迁移与 `adapters/postgres/network_catalog.go` 同改）不再作换算依据——删掉，或保留为历史列并在登记口拒收新值；ADR-0148 决定四那一句已由 ADR-0260 部分停用。
5. 补用例：`AT-PC-062`（UC-PC-001，PC 侧拒收）；`AT-PP-013`（UC-PP-001）今天的行为已对，补 Covers 点名即可。在 `.scratch/at-coverage-inventory.md` 把 `AT-PC-062` 从④移入①、`AT-PP-013` 从②移入①。

## 完成记录（2026-10-25，分支 claude-code25）

用户授权原话「你作为业务和系统专家自决」；下列「代决」由作者会话代 PC、PP、NR owner 作出，待复核。本票由两位作者接力：上一位作者写了红用例与 PC 领域、应用、端口、适配器的改动和 0041 迁移，未提交、未构建即中断；本位作者逐处读过、按裁向判过去留（见「对上一位作者半成品的处置」），再接着做完其余各步。

### 做了什么

- **第 1 步（PC，`5830a819c`）**：删掉 `domain.VolumetricCaliber` 与 `domain.VolumetricFactorReference`，`PricePolicyCaliber` 只剩税务与可缺的汇率，三个方向同一形状；口径的方向由发布编排从政策正文抄给它。新哨兵 `domain.ErrVolumetricFactorOnPricePolicy`（Is `ErrInvalidPricePolicyCaliber`）是三个方向同一的拒因：受控批文给了 `volumetricFactor` 即整批在翻译时拒、一行不写（CLI 退出非零）；操作者面载荷答在 `pricePolicy.caliber.volumetricFactor` 上，预览为`未受理`带逐格问题；带系数的 `PCC-1` 快照折回时点名拒。迁移 `party_commercial/0041` 撤 0022 的方向耦合、以 `NOT VALID` 加「只收空」：已登的销售行原样留册，`LoadPricePolicyCaliber` 不把那一格折进领域口径，价格政策目录以 `historicalVolumetricFactor` 列出。管理台（`5e46177b4`）表单去掉体积一格；政策册口径列只剩税务与汇率两段，历史系数显为「历史体积系数 …（不参与计价）」。
- **第 2 步（PC，`5830a819c`）**：`FxCaliber` 注释改成 ADR-0260 决定三的读法——评价不读政策正文、不执行取值时点规则、按卡绑定序列的牌价换算且不加点，这一格缺席不让任何评价停在未决；原在 `VolumetricFactorReference` 上的「本上下文持引用、不持数值」立场一并落到 `FxCaliber` 注释里（`credit_limit.go` 的指针随之改指）。「销售方向没有承运商卡」那条理由随类型删去，「为何没有体积一格」写在 `PricePolicyCaliber` 的类型注释上。
- **第 3 步（种子，`5830a819c`、`66123f9a6`）**：`publish-batch.json` 的 `SYN-PRICE-RULE-CN-SG` 口径去掉 `volumetricFactor`，摘要由 `PCC-1:225bc6b9…` 重算为 `PCC-1:8e53f201fee195a08cb11323a88d13d49610b3b7c221b5e3cbb257bf26833204`（经 `cmd/parcel-commercial` 的摘要对账用例算出）；`price-card-cn-sg.json` 与 `reference-series-fx-cny-sgd.json` 未动。`scripts/demo-seeds/README.md` 记了已灌库重跑答 `CONTENT_CONFLICT`、用 `--reset`，并改了配价一节「口径与卖出价卡同一套」那句。
- **第 4 步（NR，`012ab0674`）**：`RouteStrategyDefinitionVersion` 与 `InitialRouteEvidence` 去掉 `HasComparisonPricePolicy`、`ComparisonPricePolicy`，`catalog_initial_route_evidence.go` 与 `adapters/postgres/network_catalog.go`（目录 jsonb、行形、登记 INSERT）同改；迁移 `network_routing/0027` 删 `comparison_price_policy` 列，库里有非空值即拒绝施加。比较币种一格不动。
- **第 5 步（`cf5d212da`）**：新用例 `internal/parcelpricing/application/sell_card_pricing_terms_test.go::TestASellCardIsPricedOnlyOnItsOwnTermsOnBothPaths` 点名 `AT-PP-013`；UC-PC-001「给开发的交接」那一行缺口随之不再成立（修订轮 N3 删去整张空表）；`.scratch/at-coverage-inventory.md` 把 `AT-PC-062` 由④移入①、`AT-PP-013` 由②移入①，只移行，总览计数未动。PP 产品代码未改。

### 对上一位作者半成品的处置

- **留下**：领域删 `VolumetricCaliber`、方向并进 `PricePolicyCaliber`；规范化结构保留 `volumetricFactor` 键、折回时拒带系数的快照（判 ADR-0014 不换号，理由见代决 4）；应用层由正文抄方向给口径；`registrationjson`、HTTP 载荷去掉体积格的构造；端口与目录读面的 `HistoricalVolumetricFactor` / `historicalVolumetricFactor`；四份红用例的意图。
- **0041 迁移原样留下**：撤耦合、`NOT VALID` 的「只收空」，不删列、不改存量行，存量行点读与上列都读得回——与裁向「已发布的口径行原样留册」相符，未改形。
- **改掉**：
  1. 受控批文与操作者面载荷原被做成「未知键整份拒」（HTTP 上是 400 畸形）。ADR-0260 决定二写的是「带了即`未受理`，与采购、法人间方向今天的判法相同」，而先前采购方向带系数在载荷上是答在 `volumetricFactor` 那一格上的`未受理`。改回认得键、点名拒，两面共用 `ErrVolumetricFactorOnPricePolicy`；红用例随之改断言。
  2. `ConsistentWithDirection` 与 `ErrPricePolicyCaliberDirectionMismatch`：main 的 `application.declarationWrites` 调 `caliber.ConsistentWithDirection(body.Direction)`，半成品里已删去这一调用、方法与哨兵却留着。编排从正文抄方向给口径之后，这道核恒真，删去；方向不一致由 0022 的复合外键 `price_policy_caliber_policy_fkey` 在库上拦，`TestAPricePolicyCaliberNeedsABodyRowOfTheSameDirection` 钉着。方法、哨兵与对应的领域用例一并删去。
  3. 库上用例原先「撤约束、落行、手写一遍约束加回去」，加回去的是抄本。改成退回 0041 之前的表（撤 0041 约束、按 0022 原文加回耦合）、落一行当年合法的带系数销售行，再**原样施加嵌入的 0041 正文**，证迁移在有存量的表上施加得了、存量行留册可读。
  4. 注释里的「ADR-0260 之前」「今天」等时间锚改成「0041 迁移之前」或删去；`ports.go` 里另三处仍写「体积」的口径注释补改；同文件里的位置词「见下一条」改成用例名；`FxCaliber` 注释原指向「判据同 FxQuoteTypeReference」，那里并没写那条立场，改为在本处写全。
  5. 应用测试的口径夹具 `sellCaliber` 已与方向无关，改名 `policyCaliber`。

### 代决

1. **删类型而不是留一个永远拒的构造门。** 裁向第 1 步写「`NewVolumetricCaliber` 三个方向一律不收」。一个永远只接受零值的值对象没有内容可携带，留下只是给后来者一个能写进口径的形状；删掉之后「政策上没有这一格」在类型上成立，拒的判法收在一个哨兵上，三处入口共用。
2. **入口认得键、点名拒，不当未知键。** 理由见上节第 1 条：守 ADR-0260 决定二「`未受理`、判法同采购方向」，也让照旧批文写来的人看见这一格去了价卡，而不是一句「不认识这个字段」。加点键、口径方向键仍按未知键拒——它们从来没有合法过。
3. **方向并进口径，撤方向核。** 口径行的复合外键要方向列，所以领域口径带方向；编排从正文抄方向给口径之后，输入面上没有第二个方向，编排里的方向核恒真，删去。
4. **ADR-0014：`PCC-1` 不换号。** 规范化结构体、键名、键序与编码都没变，`volumetricFactor` 仍是那个可缺键，本构建只是写不出它；不带系数的文档逐字节同前（`TestAStoredPricePolicyDocumentCarryingAVolumetricFactorDoesNotRehydrate` 断言折回再规范化与原文逐字节相等）。变的是正文的合法集合，ADR-0014 Decision「规范化版本不表达业务规则变化」；ADR-0267 读法里算结构变化的加键、删键、改键这里都没有，不需要写同号改形的前提。后果：已发布版本列上的摘要不动；同一版本号按新内容重发答内容冲突（种子即如此，用 `--reset`）；带系数的旧快照只在发布载体（非权威记录、只有按版本点读）上折不回来，见越权风险点 1。
5. **读面的标法。** 目录 API 的键改名 `historicalVolumetricFactor`、只在 0041 之前的销售行上在场，不沿用 `volumetricFactor`，免得照旧读法的调用方把它读成现行口径；「不参与计价」由管理台显在那一格旁，API 的含义写在 `ports.PricePolicyRow.HistoricalVolumetricFactor` 与 HTTP 行体注释上。不另加一个恒为假的「参与计价」布尔。
6. **NR 删列，不留历史列。** 比较价格政策一格从来没有登记口能写：路由策略批文形状（`registrationjson.strategyContent`）没有这一键，严格解码拒未知键，在线口与 CLI 共用这份翻译；也没有任何读方。留作历史列只多一格永远为空、还要另加 CHECK 拦新值的死列。万一有人直接写过 SQL，删列会抹掉一份当真写下的声明，所以 0027 先查非空值，有就拒绝施加，由登记方定去留后置空再重跑。
7. **`AT-PP-013` 立专门用例，不往零散用例上挂半边。** 盘点时找不到一条同时覆盖 SELL 卡、体积系数、金额取整与汇率序列的既有用例。新用例在正式评价（`EvaluatePricingHandler`）与试算（`FormEstimateEvaluationsHandler`）两条路上用同一张卡：计费重由卡上除数 5000 的体积重 4.8 KG 进位到 5 KG、落 2–10 KG 那一档，按绑定序列在用版本 v2 的牌价换算、在计价基准时点取期次，按卡上的 HALF_UP 0.01 合计，版本清单里只有卡与序列、没有商业政策或商业授权引用。修订轮 N2 另在 `cmd/parcel-api` 补了真库完整版（见「评审记录与修订轮」），应用层这条降为编排半边。
8. **用例与 AT 的落点。** `AT-PC-062` 由六条用例分面钉住（批文、载荷、快照、领域口径、发布编排、库与迁移），盘点①一行全部列上。

### 越权风险点

1. **带系数的旧发布载体折不回来，答成 5xx**（PC owner）。发布载体（`publication_draft`，非权威记录）里 0041 迁移之前录入、带体积系数的价格规则快照，`domain.RehydratePublicationDraft` 折不回来，`LoadDraft` 上抛，批准、发布与录入读回三口都答 5xx `NO_ANSWER_FORMED`。按状态：`待批准`的可按同一版本重新录入替换那一行（`SubmitDraft` 的 upsert 只替换`待批准`行）；`已批准`未发布的既发布不了、也不能按同一版本重录（upsert 不替换已批准行，零行命中后 `LoadDraft` 读回又撞同一个重建错误），只能换版本号；`已发布`的重放批准或发布不再答已发布一类的格，而答 5xx。这违反 ADR-0022（5xx 是「退避后重发」，这里重发不会好）与 ADR-0029（按恢复动作分格，恢复动作是重录或换版本号）。今天受影响的行为空：没有真实租户，演示种子不经载体发布价格规则。协调方裁本票不改代码，另立 [code/87](./87-a-publication-draft-this-build-cannot-rehydrate-answers-5xx-instead-of-a-business-grade.md)，把「本构建不认得的规范化版本」那一支同病一并归进去。
2. **目录 API 键名变了**（PC owner）。`caliber.volumetricFactor` 改为 `caliber.historicalVolumetricFactor`；仓内唯一读方管理台同分支已改，外部读方若照旧键读会读不到。
3. **入口为拒而认得一个已退场的键**（PC owner）。批文与载荷结构上保留 `volumetricFactor`，只为点名拒；日后若删掉它，拒法就退成一般的未知键（载荷上成 400），与 ADR-0260 决定二的「`未受理`」不再一致。
4. **库外调用方的方向核只剩外键**（PC owner）。绕开发布编排自己造口径的调用方，方向与正文不符时只在 `SavePricePolicyCaliber` 撞复合外键（答 `Invalid` 带外键原因），进程内不再先拒。
5. **0027 有值即停**（NR owner）。若某库直接写过比较价格政策，迁移拒绝施加、部署停在那一步，要人导出、置空再重跑。
6. **评价读不到政策，靠的是依赖图**（PP owner）。`AT-PP-013` 的真库用例构造了前提——同库同范围的销售价格政策带一个历史体积系数 6000、汇率格选「提交接收」——而结果只跟卡走；但「评价改读政策口径」在生产装配上够不着（PP 没有伸向 PC 的适配器，试算编排的依赖里也没有 PC 的口），所以那一处变异做不出来，由新架构断言 `TestParcelPricingHasNoPartyCommercialAdapter` 兜：谁加了 `internal/parcelpricing/adapters/partycommercial` 或让 parcel-pricing 导入 party-commercial，门禁即红。
7. **一处陈旧注释**（NR owner）。`cmd/parcel-network-register/main.go` 头注「路由策略的比较币种与所引价格政策两列（0016）本口还没有格」在 0027 之后半句过时；协调方放行只改这一句，修订轮已改为「路由策略的比较币种一列（0016）本口还没有格。」，该文件别处未动。

### 迁移

- `party_commercial/0041_price_policy_caliber_volumetric_factor_retired.sql`：撤 `price_policy_caliber_volumetric_factor_coupled`，加 `price_policy_caliber_volumetric_factor_retired CHECK (volumetric_factor_ref IS NULL) NOT VALID`。
- `network_routing/0027_route_strategy_comparison_price_policy_retired.sql`：有非空 `comparison_price_policy` 即 `RAISE EXCEPTION`，否则删列。
- 取号前核过 main（PC 到 0040、NR 到 0026）与在途分支、各 worktree，两个号都没被占。

### 无新写者

没有新增写口：改动的写者 `CommercialPublications.SavePricePolicyCaliber` 与 `NetworkCatalog.RegisterRouteStrategyVersion` 照旧经 `RequireExecutor` 取事务内执行器、事务外拒跑，锁与写的形状未变。确定性拒绝（带系数）答`未受理`或翻译拒，不进任何未决哨兵（ADR-0029）。

### 验证（初版，分支 claude-code25 当时 rebase 到 main `d19eeaafc`，`git rebase main` 退 0、无冲突；修订轮的验证见下节）

- `go build ./...`、`go vet ./...`、`gofmt -l` 干净。
- 红证：在 `5cc309c5b` 的临时 detached worktree 上放与新用例同输入、断言改用当时已有符号的探针，四面全红——批文上 SELL 带系数被收下、只有税务的 SELL 口径被拒；载荷上 SELL 带系数无问题、SELL 不带系数答在 `volumetricFactor` 上；带系数的快照折得回；库上 SELL 不带系数撞 `price_policy_caliber_volumetric_factor_coupled`。NR 迁移用例在该提交上答「没有找到迁移 0027」。用后删了探针与 worktree。
- 变异四处，均被杀：删载荷的点名拒 → `TestAVolumetricFactorInAPricePolicyPayloadIsAnsweredOnItsField` 三方向红；删批文的点名拒 → `TestAPricePolicyCaliberCarryingAVolumetricFactorIsNotAccepted` 三方向红；0041 去掉 `NOT VALID` → `TestMigration0041KeepsHistoricalFactorsListedAndRefusesNewOnes` 施加即报约束被存量行违反；0027 去掉非空守卫 → `TestMigration0027DropsTheComparisonPricePolicyOnlyWhenNothingWasDeclared` 报「照样删了列」。变异后文件按备份复原，`git status` 干净。
- `TMPDIR=… scripts/test-full.sh <改动包与 go list -test 算出的反向依赖，共 79 个> ./internal/architecture/... ./migrations/... ./tests/bentocontract/...`：退 0，78 个包 ok，其余无测试文件；含 `cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-commercial`、`internal/architecture`。
- 新用例与改过的用例另跑 `-count=3`：`cmd/parcel-commercial`、PC 领域、应用、HTTP、postgres，NR postgres，PP 应用，全 ok。
- 管理台三道门（Node 22）：`tsc -b --noEmit` 退 0；`node scripts/run-tests.mjs` 601 通过 0 失败；`pnpm exec vite build` 退 0。`pnpm install --frozen-lockfile` 未改 lockfile。
- 演示种子：一次性库 `c25seed` 上按分支首灌退 0，`SYN-PRICE-RULE-CN-SG` 口径行系数为空，`route_strategy_version` 只剩 `comparison_currency`；用后 `DROP DATABASE … WITH (FORCE)`。另走一次「先按 main 灌、再切分支」：临时 detached worktree（main `d19eeaafc`）灌通，口径行系数 `5000`；切到分支不带 `--reset` 重跑，两份迁移在已灌库上照常施加（`applied=2`），发布批那一项答 `CONTENT_CONFLICT`、`seed.sh` 退 2 中止，库里那一行系数仍是 `5000`、`price_policy_caliber_volumetric_factor_retired` 未校验（`convalidated=f`）；再带 `--reset` 复灌退 0，行上系数为空。用后删库、删 worktree。
- 未跑全量，未重生成 MECHANISM-INVENTORY、ADR 索引与 AT 盘点计数。ADR-0148、ADR-0107 的 Status 行已挂 ADR-0260 的前向指针，未改。

### 未决

- 越权风险点 1 的答法，归 [code/87](./87-a-publication-draft-this-build-cannot-rehydrate-answers-5xx-instead-of-a-business-grade.md)。
- 文档自洽 10 负责的旁处旧口径（GLOSSARY、CONTEXT-MAP、参数登记册等）不在本票。

## 评审记录与修订轮（2026-10-25，分支 claude-code25）

隔离评审无阻断，提了六条 N；协调方转来逐条处置要求。修订轮前先 `git rebase main` 到 `9ea060682`（多了 code/31，VE 范围），退 0、无冲突；旧提交未改写，修订落在新提交上。

- **N1 旧载体折不回来答成 5xx**：协调方裁走 (b)，本票不改代码。越权风险点 1 按`待批准`、`已批准`未发布、`已发布`三种状态如实改写，并写明违反 ADR-0022、ADR-0029、今天受影响的行为空。另立 [code/87](./87-a-publication-draft-this-build-cannot-rehydrate-answers-5xx-instead-of-a-business-grade.md)（取号前 ls 过本目录、各 worktree 与在途分支，87 空闲），R3，把「本构建不认得的规范化版本」那一支同病归进去，修法照评审 (a)：`%w` 保链，批准、发布、录入读回三口认出折不回来的哨兵答业务格。
- **N2 AT-PP-013 前提没构造出来**：
  - `cmd/parcel-api/assemble_pricing_estimate_test.go::TestAnEstimateReadsOnlyTheCardWhileASellPolicyDeclaresOtherwiseAgainstARealDatabase`，真库，前提三步：
    - 经运营操作者面的发布编排（录入 → 批准 → 发布）登一份销售价格政策，汇率格选 `REFCFG-1:parcel-shipment/as-of-semantics/submission-receipt@1`；
    - 照 `TestMigration0041KeepsHistoricalFactorsListedAndRefusesNewOnes` 的退表手法，给它的口径行落历史系数 6000，再原样施加 0041；
    - 经生产装配的登记编排登除数 5000 的 SELL 卡与两期汇率序列，序列口径引用指名那一版政策，并复核使之在用。
  - 用生产装配的试算编排声明 1 KG、40×30×20 CM 的包裹，断言：
    - 体积重按卡上除数 5000 得 4.8 KG（按 6000 会是 4 KG）；
    - 落 20 CNY 那一档；
    - 换算取计价基准时点所在的后一期 0.18765；
    - 合计按卡上的 HALF_UP 0.01 得 3.75 SGD；
    - 版本清单只有卡与序列，没有商业政策或商业授权引用。
  - 新架构断言 `internal/architecture/pricing_terms_source_gate_test.go::TestParcelPricingHasNoPartyCommercialAdapter`：`internal/parcelpricing/adapters/partycommercial` 不得存在，parcel-pricing 不得导入 party-commercial，中文头注写了为何（ADR-0260 认价卡）。
  - 「评价改读政策口径系数」在装配上够不着：PP 没有伸向 PC 的口，试算编排的依赖里也没有。所以做了两处替代变异，都被杀：
    - 在 PP 的 `deriveVolumetricWeight` 里把除数硬换成 6000，模拟「读到政策上的系数」，新用例红在体积重一格；
    - 临时建 `internal/parcelpricing/adapters/partycommercial/probe.go` 并导入 PC 端口，新架构断言两格都红。
  - 应用层用例 `TestASellCardIsPricedOnlyOnItsOwnTermsOnBothPaths` 的 Covers 降为编排半边；盘点①的 `AT-PP-013` 一行补上真库用例。
- **N3**：UC-PC-001「给开发的交接」删去写着「已补」的 `AT-PC-062` 一行。表空了，「现状缺口（…）：」引句与表头一起删（先例 code/11 的 `2ff8d15b9`）。
- **N4**：「对上一位作者半成品的处置·改掉 2」与代决 3 改成实情。main 的 `application.declarationWrites` 调着 `caliber.ConsistentWithDirection(body.Direction)`；编排从正文抄方向给口径之后，这道核恒真，删去。方向不一致由 0022 的复合外键 `price_policy_caliber_policy_fkey` 在库上拦，`TestAPricePolicyCaliberNeedsABodyRowOfTheSameDirection` 钉着。初版回报里「编排早已不调」一句不实，以此为准。
- **N5**：`TestMigration0041KeepsHistoricalFactorsListedAndRefusesNewOnes` 末尾，对 `price-historical` 用只有税务的同向口径调一次 `SavePricePolicyCaliber`，断言答 `PricePolicyCaliberContentConflict`、行上系数仍是 5000。变异：删掉 `scannedPricePolicyCaliber.sameAs` 里的系数比较，用例答 `ALREADY_REGISTERED` 而红；已复原。
- **N6 注释**：
  - `domain.ErrVolumetricFactorOnPricePolicy` 去掉计数与「仍」，点名 `registrationjson.pricePolicyCaliberDocument`、`http.PricePolicyCaliberPayload`、`canonicalPricePolicyCaliber`；
  - 三条测试注释里的「不再」改成现行规则句；
  - `domain.PricePolicyCaliber`、`canonicalPricePolicyCaliber` 与快照用例注释的「先前」改「0041 迁移之前」；
  - 迁移 0041 头注「ADR-0260 之前按 0022 登下的销售行」改「按 0022 登下的销售行」，并说明 0022 头注提到的 `ConsistentWithDirection` 已删、方向一致只由 `price_policy_caliber_policy_fkey` 守（0022 不可改）。0041 字节因此变了，尚未进 main，不受校验和约束。
- **协调方放行的 main.go 注释**：`cmd/parcel-network-register/main.go` 头注那一句改为「路由策略的比较币种一列（0016）本口还没有格。」，该文件别处未动；越权风险点 7 与「未决」随之改。

### 修订轮验证（分支 claude-code25，rebase 到 main `9ea060682`）

- `go build ./...`、`go vet ./...`、`gofmt -l` 干净；0041 与新票面量过无 CR。
- `TMPDIR=… scripts/test-full.sh ./internal/partycommercial/... ./internal/parcelpricing/... ./internal/networkrouting/... ./cmd/parcel-commercial/... ./cmd/parcel-network-register/... ./cmd/parcel-api/ ./internal/architecture/... ./migrations/...`：退 0，36 个包 ok，3 个无测试文件。
- 新改用例另跑 `-count=3`：`cmd/parcel-api`、`internal/architecture`、PC 领域、HTTP、postgres，PP 应用，`cmd/parcel-commercial`，全 ok。
- 变异三处，均被杀，见上（N2 两处、N5 一处）；变异后文件按备份复原。
- 演示种子：一次性库 `c25seed2` 上按分支首灌退 0，`SYN-PRICE-RULE-CN-SG` 口径行系数为空；用后 `DROP DATABASE … WITH (FORCE)`。
- 未改 admin-web，三道门不重跑。
