# 26 SA 消费发生项已形成信封，发起 BUY 评价（收 10 的 BUY 半）

Category: enhancement
Status: resolved——2026-10-19 进 main（全量钉 `60f8d75c`：146 ok / 0 FAIL；TF 链第一波同批，见文末「进 main 记录」）。此前 in-progress——完工、评审已修订，待进 main，分支 claude-dcg26（与 [30](./30-buy-cost-card-declares-amount-rounding.md) 同一分支，30 先落）。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: 无
地盘：SA 新 `adapters/transportfulfillment/`、PC 范围读、新 inbox 消费者；触发册加费用项目（domain、ports、postgres、迁移 SA 新号）；`cmd/parcel-settlement-register`；`cmd/parcel-dispatch/assemble.go`(+_test)；种子 settlement BUY 行；ADR-0208（ADR-0164 加后继指针）、UC-SA-002、SA CONTEXT。
出处：[10](./10-demo-tenant-buy-and-sell-trigger-registers-are-empty.md)、[07](./07-transport-charge-occurrences-have-no-production-path.md)；方案「BUY 发起（接 08，收 10）」D08-1、D10-1，「跨票通则」X3、X6，「需要的 ADR」ADR-0208。

## 做什么

- SA 在 `cmd/parcel-dispatch` 消费 `transport-fulfillment.charge-occurrence.formed`，路由先登（生产方 [27](./27-transport-charge-occurrences-are-registered-and-handed-off.md) 后上线，方案 X3），经 SA 消费侧适配器按引用回读发生项（方案 D08-1）。
- 发生项各要素与供应商协议取自发生项本身；主要范围经 SA→PC 读口取协议版本的商业范围（方案 D08-1）。
- BUY 触发册每行加一格「费用项目」（租户取值）；未配置与未决分格照方案 D08-1、X6。
- 演示种子登 BUY 触发行与费用项目，取值照方案 D10-1。
- 起草 ADR-0208：部分停用 ADR-0164 决定一末句，范围照方案「需要的 ADR」；ADR-0164 Links 加后继指针。UC-SA-002、SA CONTEXT 补一句。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 路由先登（生产方未上线，ADR-0049 允许）。
- 无行或无费用项目答 `未配置` 并入账。
- 读不动登哨兵。
- 真库测试用手插的 TF 发生项行跑到 BUY 请求 `FIRED`。
- 端到端演示证据随 [27](./27-transport-charge-occurrences-are-registered-and-handed-off.md)。

## 规模

M

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg26`，提交 `669b41a0` 至 `dc35a889`）

### 做了什么

- **触发册加费用项目**（`669b41a0`）：BUY 触发册每行多一格「费用项目」，可缺（迁移 `settlement_accounting/0040_buy_evaluation_trigger_fee_item.sql`）。领域 `BuyEvaluationTriggerRegistration.WithFeeItem` / `FeeItem`，重放比三格；`ports.BuyEvaluationTriggerView` 交回整行；`parcel-settlement-register buy-evaluation-trigger` 输入多一个可选键 `feeItem`，空白值拒。触发结果多出`未决`与「修订版不发起」两格，`未配置`点名缺原因还是缺费用项目。ADR-0164 那条触发面行为不变。
- **PC 协议范围读口**（`6e54440a`）：`ports.SupplierAgreementScopeView` 与 `postgres.SupplierAgreementScopes`，按「对象/版本」两段式指称答那一版协议正文上的适用范围。不拆指称：逐行用 `NewQualifiedVersionLabel` 那一处拼出来比。不核生效。新文件，不碰 22 的 `ports.go` 与 `supplier_agreement.go`。
- **结算发起链**（`d1df686e`）：
  - 消费门 `sainbox.ChargeOccurrenceFormedConsumer`，认 `transport-fulfillment.charge-occurrence.formed`，载荷三键 `tenantId`、`occurrence`、`version`，缺一即毒丸。
  - 消费侧适配器 `internal/settlementaccounting/adapters/transportfulfillment/`，按键经 TF 登记册的 `FindByKey` 回读发生项，译成发起命令。
  - `adapters/partycommercial.SupplierAgreementScope`，把协议的商业范围译成主要范围。
  - 编排 `application.TriggerBuyEvaluationOnOccurrenceFormedHandler`，请求方是固定名 `settlement-accounting/buy-evaluation-on-occurrence-formed`。
- **派发接线**（`2942adf3`）：路由表先登这一类，哨兵名单 `chargeOccurrenceFormedUndecidedSentinels` 只收发生项读不回与触发编排未决。生产接线在新函数 `requestBuyEvaluationOnChargeOccurrenceFormedConsumer` 里，`assemble.go` 只追加行。
- **种子**（`6a8c36b4`）：`data/settlement/08-buy-evaluation-trigger-actual-fulfillment.json`，`ACTUAL_FULFILLMENT` → `OCCURRENCE_FORMED`，费用项目 `SYN-FEE-ITEM-TRUNK-COST-01`；`seed.sh` 追加一行；README 结算一行补一句。
- **文档**（`dc35a889`）：
  - 起草 ADR-0208。
  - ADR-0164 的 Status 行与 Links 加前向指针；`docs/adr/README.md` 的 0164 行写被停条款与适用场景，并追加 0208 一行。
  - UC-SA-002 步 2 一句，加 `AT-SA-185` 至 `AT-SA-187`。
  - SA `CONTEXT` 一句；`CONTEXT-MAP` 加「运输履约 → 结算（发生项已形成的交接）」一条。
  - `PAR-SET-03` 补一处登口指针。

### 代决（方案未裁的细节）

1. **修订版不发起，入账**。有效性修订产生的那一版（失效或替代）答「修订版不发起」：失效的那一版若也发起，会给已证无效的范围估成本。纠错归票 [15](./15-occurrence-revision-does-not-drive-a-cost-correction.md)。票 27 只登首版，本路今天碰不到修订版。
2. **协议在 PC 查无是矛盾**，上抛为发布失败，不答未配置、不答未决。发生项登记时已经核过协议（票 22/27），两侧对不上不是缺租户取值，也不会因重投而改变。
3. **交信封失败在本路答未决**，整笔回滚。请求编排对事务外调用方留续办引用、照答已请求；那个形状放进消费事务，会让请求行提交而信封缺席。SELL 那只编排同样这样处理。
4. **未配置的两格先判，不问 PC**。没登的原因不必去读协议。
5. **触发册不设修订**。同原因换费用项目、给没登的行补登，都答冲突，沿 ADR-0164「不设修订」。
6. **协议指称串定为 PC 的「对象/版本」**，由 ADR-0208 决定六交给票 27 照写 `agreement_ref`。PS 引 PC 版本也是这个形。
7. **主要范围取协议正文上的适用范围**（`agreement_scope_ref`），不取版本壳上的范围。正文的范围是「这份协议在哪里约定」；演示里两者同值。
8. **PC 窄读口自立，不等票 22**。22 与本票是两条车道，本票照方案「Blocked by: 无」。PC 窄读口只交范围，22 的点读口交版本壳加正文，两口并存交 PC owner 裁（ADR-0208 越权风险点 8）。

### 越权风险点

以 ADR-0208 文末的越权风险点为准，此处不复述。要点：一个原因只映一个费用项目（SA）；主要范围等同协议商业范围（SA、PC）；`Request` 仍可直调（SA）；触发册不设修订（SA）；修订版不发起（SA、TF）；交信封失败不分确定性拒收（SA）；协议指称串形状由消费方先定（TF）；PC 两口并存（PC）。

### 用例

- 真库派发：`cmd/parcel-dispatch` 的 `TestAFormedChargeOccurrenceFiresABuyEvaluationRequestThroughTheRouteTable`。
  - 经 TF 真登记册手插几份首版发生项，PC 登干线协议版本与正文，触发册登实际履约（带费用项目）与取消（不带）。
  - 实际履约那份发起恰一份 BUY 请求（FIRED），范围 `SYN-SCOPE-01`、费用项目 `SYN-FEE-ITEM-TRUNK-COST-01`、协议 `SYN-SUPPLIER-TRUNK-CN-SG-01/v1`，请求信封一封。
  - 失败尝试（没登）、取消（没登费用项目）入账、零请求。
  - 协议不认的那份落 `dispatch.publish_failed`；指着没有的发生项的那封落 `dispatch.consumer_undecided`。
- 编排：`application` 包的 `trigger_buy_evaluation_on_occurrence_formed_test.go`，覆盖未配置两格、未决四口、矛盾、修订版与残缺命令、构造门。
- 适配器：`adapters/transportfulfillment` 的翻译、可见性未决、键不符、各格去向（真编排）；`adapters/partycommercial` 的范围翻译；`adapters/inbox` 的消费门（真库）；`adapters/postgres` 的触发册费用项目往返与拒改（真库）；PC `adapters/postgres` 的范围读口（真库）。

### 验证

- `go build ./...`、`go vet ./cmd/... ./internal/... ./scripts/... ./migrations/...`、`gofmt -l` 干净。
- 带真库跑 `go test -p 1 -count=1`：`./internal/architecture/`、`./migrations/...`、`./internal/settlementaccounting/...`、`./internal/partycommercial/adapters/postgres/`、`./cmd/parcel-settlement-register/`、`./cmd/parcel-dispatch/`（整包）、`./scripts/demo-seeds/...`、`./cmd/parcel-api/` 中评价请求两条，全绿。
- 一次性库 `dcg26_seed26` 上 `seed.sh` 退出码 0。触发册一行带费用项目；按 `SYN-SUPPLIER-TRUNK-CN-SG-01/v1` 查 PC 正文命中 `SYN-SCOPE-01`。用完已删库。
- 机制清点没有重生成（进 main 时统一做）。

### 偏离与待裁

- 「PC 范围读」落成新开的窄读口，不借 22 的点读口，见代决 8。
- 端到端演示证据随 [27](./27-transport-charge-occurrences-are-registered-and-handed-off.md)。走到供应商预期成本之前还有一处读码看到的缺口：演示成本卡没绑分区目录，按发生项造的快照不带分区，BUY 评价会停在分区解不出。详见 [30](./30-buy-cost-card-declares-amount-rounding.md) 完成记录的待裁。
- `cmd/parcel-api/assemble_evaluation_request.go` 头注「三件合格来源引用仍由调用方交进来，不从 TF 登记册推」对那条触发面仍然成立，所以没改。那个目录有 operator-channel/48 在改注释，也避开了。

## 评审记录（2026-10-19，隔离评审一轮；修订提交 `329fcd94` 至 `6005e30d`）

阻断 1 项，已修；非阻断 7 项，修 5 项，1 项立票，1 项按任务口径留到进 main。

### 阻断（已修）

- **CONTEXT-MAP 的 `settlement-accounting → parcel-pricing` 一条与 ADR-0208 字面矛盾**（`329fcd94`）。它不加限定地写「由结算作业或上游编排交进来，结算不从来源上下文的登记册推导」。现在这半句限到 ADR-0164 那条由调用方交齐的触发面。发生项已形成那一路注明不在此限：发生项与协议按引用回读发生项自身固定的事实，主要范围取协议的商业范围。注里链 ADR-0208，并按条目名指向「运输履约 → 结算（发生项已形成的交接）」一条。只改文档，代码没动。

### 非阻断

1. **协议范围读口的确定性错误被折成未决**（已修，`2da04f9a`、`59869497`）。按评审给的第一条路改：
   - PC `ports.ErrSupplierAgreementLabelAmbiguous`：一份指称比中不止一版协议时报它。
   - SA `ports.ErrSupplierAgreementScopeUntranslatable`：SA→PC 适配器把租户、指称、范围译不过去和指称歧义都包进它。
   - 触发编排见到这枚哨兵就原样上抛，落 `publish_failed`。只有读不动才答未决。
   - 派发哨兵名单的注释补了「不在名单里」一条。ADR-0208 决定四、五与 Consequences 把这件事写明，`AT-SA-187` 同步。
   - 新用例三条：PC 真库 `TestAQualifiedLabelNamingTwoAgreementVersionsIsAmbiguous`；SA 适配器 `TestAnAmbiguousAgreementLabelIsDeterministic`；编排 `TestAnUntranslatableAgreementScopeIsRaisedNotLeftUndecided`。读不动那条用例加一个断言：错误里不带确定性哨兵。
2. **领域层两条头注与 ADR-0208 矛盾**（已修，`a9bd35c9`）。`PrimaryScopeReference` 与 `EligibleSourceReferences` 的来源改为按发起路径分别写，两条路都不许的「按当前伙伴、线路或报表分组去配」保留为红线。
3. **方案 D10-1 仍写「以新版本重发」**（已修，`ceca1dec`）。D10-1 下追加一条改判，指向票 30 完成记录；票 30 的「做什么」「完成判据」注明已改判。
4. **ADR-0208 有两处遗漏**（已修，`59869497`）。
   - 「候选与反方」加一条：主要范围取版本壳上的 `scope_ref`，否决，并写明理由。
   - 决定四写明取的是正文上的适用范围；越权风险点 2 补一句两者分家时由 owner 另裁。
   - 「三类」一行补「七是机制」。
5. **注释里写了「生产方还没上线」这类时点状态**（已修，`5fad4ee0`）。改了三处：
   - 消费门 `ChargeOccurrenceFormedEventType` 头注、派发用例文件头、`assemble.go` 路由说明，都改成只陈述 ADR-0049 和「谁钉得住两串相等」；ADR-0208 Consequences 那一句也一并改。
   - 「测试改用真交接口入队并钉两串相等」写进票 [27](./27-transport-charge-occurrences-are-registered-and-handed-off.md) 的完成判据，同一条带上 `agreement_ref` 照 ADR-0208 决定六写。
6. **BUY 请求交接口没有确定性拒收哨兵**（未修，立票 [39](./39-buy-evaluation-request-handoff-does-not-separate-deterministic-rejection.md)，`6005e30d`）。
   - 不在本票修的理由：哨兵要从 `RequestBuyEvaluationHandler` 透出来。这只编排同时服务 ADR-0164 那条触发面和 `cmd/parcel-api` 的评价请求端点，两处今天对任何交信封失败都答「已请求、留续办引用」。改了会动那两处的答法，属 SA owner 的形状问题；`cmd/parcel-api` 目录也有别的会话在改注释。
   - ADR-0208 越权风险点 6 已指向票 39。
7. **机制清点没有重生成**（未做）。任务口径规定进 main 时统一重生成，分支上不做。本分支多了迁移 SA 0040，进 main 时须带上。

### 代决与越权风险点（评审修订新增）

- **确定性哨兵立在两侧 ports，没有选择收窄 ADR**。收窄就是承认 PC 一侧的坏答案可以按未决重投，等于把坏数据藏进失败预算；ADR-0208 决定五的本意是词汇表外即上抛。
- **PC 一侧只把「指称歧义」立成哨兵**。
  - PC 存储行译不回领域值（对象标识、版本、范围的构造失败）仍走普通 error，到 SA 按未决重投。
  - 越权风险点（PC owner）：这几种同样是确定性的，属 PC 存储不变量已破，但 PC 读口头注把「读取失败与坏数据」并作一格，那是 PC 的既有口径。要不要拆开归 PC owner；表上的非空白 CHECK 已经挡掉最常见的一种。
- **改了票 27 的完成判据**（越权风险点，归 27 的认领者与 TF owner）：追加的一条只钉两串相等与 `agreement_ref` 形状，两件都出自已写进 ADR-0208 的约定。
- **票 39 立为 `needs-triage`**，不标 `ready-for-agent`：哨兵从哪一层透出来，会改变 ADR-0164 触发面和 parcel-api 端点的答法，要先裁。

### 验证（修订后）

- `go build ./...`、`go vet ./cmd/... ./internal/... ./scripts/... ./migrations/...`、`gofmt -l` 干净。
- 带真库 `go test -p 1 -count=1` 全绿：`./internal/architecture/`、`./internal/settlementaccounting/...`、`./internal/partycommercial/...`（含 `adapters/postgres`）、`./cmd/parcel-dispatch/`（整包，含 `TestAFormedChargeOccurrenceFiresABuyEvaluationRequestThroughTheRouteTable` 与 `TestADeclaredParcelOnZonedLegsIsPricedThroughProductionWiring`）。
- 本轮没动种子与迁移，没有重跑 `seed.sh`。

## 进 main 记录（2026-10-19）

- TF 链第一波 16、19、22、26、30、31 合成一批，按 19 → 22 → 26/30 → 31 → 16 叠到 main 上，整批快进推送，与本记录同一次推送。
- 合批时的处理：
  - README 的 ADR 行按号合并。
  - `.scratch/at-coverage-inventory.md` 计数表：CC 行取 31，NO 行取 main，合计重算；两段追记都留。
  - 19 与 16 的接缝在 `d3c39a77`：19 的班次夹具改用 16 的方向与临时班次构造；19 在 `transport_schedule.go` 加的两行端口断言保留。
  - 26 立的 38 号票与 16 立的 38 号票撞号，26 那张改为 39，ADR-0208 风险点 6 同步。
- 合批种子在一次性新库上 `seed.sh` 灌通，退 0，用完删库。
- 全量 `go test -p 1 -count=1 ./...`（带 55432 DSN）在干净游离检出上钉 `60f8d75c`：146 包 ok，0 FAIL，0 SKIP。机制清点在同一 tip 上重生成。
- 推送前 main 又进了 no-reception-identity/05（NO 收寄，与本批文件不相交），本批重放到它上面：AT 覆盖盘点计数表再合一次（CC 行取本批，NO 行取 main，合计重算）；机制清点在推送 tip 上重生成；`internal/architecture`、TF、CC、SA、PC、PP 各包与 `cmd/parcel-api`、`cmd/parcel-dispatch`、三个登记 CLI、`scripts/demo-seeds/...`、`migrations/` 在推送 tip 上重跑，56 包 ok。没在推送 tip 上重跑全量。
