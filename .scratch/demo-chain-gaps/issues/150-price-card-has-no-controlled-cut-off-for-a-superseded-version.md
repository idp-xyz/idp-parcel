# 150 价卡没有截止旧版的受控口，补登卡上取值只能靠期间不重叠的新版

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg150。此前 in-progress——2026-10-21 认领。此前 ready-for-agent——2026-10-21 协调方分诊升为上线前必修（R1），见 [triage-2026-10-19](../triage-2026-10-19.md) 末节「票 14 落地时追加的上线前必修（2026-10-21）」。此前 needs-triage——2026-10-21 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权），出自票 [14](./14-buy-request-stuck-after-an-unadoptable-evaluation.md) 实现时的发现。
Blocked by: 无
出处：[ADR-0229](../../../docs/adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 越权风险点 6。

## 现象

计价按（租户、计价范围、方向、目的、计价基准时点）选卡，同范围期间重叠的两版答`适用冲突`。卡上没声明金额取整、不可计价、评价冲突、价表上没有这一段（缺重量段）、没绑汇率序列这类停因，要靠新一版卡解开；租户登一版同范围、同期间的新卡，旧卡不会退场，续办出的下一代请求停在`适用冲突`；登期间不重叠的新一版，又不覆盖停着的请求的计价基准时点。今天唯一的绕法是新版从某个时点起生效、旧版期间不覆盖它——可已登的旧版期间改不了，计价没有截止旧版的受控口。

## 三类

机制（受控截止口）；截止到哪个时点是租户取值。规模 S–M。

## 最小的诚实修法

照参与方与商业那一侧区间更正的做法（票 [50](./50-commercial-versions-have-no-closing-entry-so-agreement-versions-pile-up.md)、ADR-0038、ADR-0056）：给价卡一个受控登记入口，把旧版截止在新版生效时点，旧版行留历史、只追加截止事实；截止落库同笔交「参数已登记」（种类价卡），停在`适用冲突`上的请求按 ADR-0229 续办。不以新版生效自动收尾旧版。

## 要裁的

截止口归登记册配置写还是运营决定；截止之后已在旧版下形成的评价是否要标注（不改写）。归 PP owner。

## 既有票

none

## 完成记录（2026-10-21，Claude Code 作者子代理，分支 claude-dcg150）

决定落在 [ADR-0234](../../../docs/adr/0234-an-older-price-card-version-leaves-new-selections-only-through-an-explicit-closure-for-a-named-successor.md)，形照 ADR-0233（票 50）。票面「要裁的」两问代 PP owner 裁在 ADR 决定一、决定六。

### 做了什么

- **两种收尾，各一口**（决定一至四）。截止：后继晚于旧版起点，旧版此刻的选用截止在后继此刻的选用起点（截止点服务端取）。退役：后继不晚于旧版起点（同起点重发，或连起点一起改正），旧版整段让位、自退役登记起不再参与新的选择（退役时刻取登记时刻）。两种都要同一定价方案、同一计价范围、方向与目的，后继接得住旧版此后的那一段；先锁旧版行再读两版此刻的样子判，写截止那一条语句自己再锁一次版本行。
- **领域**：`PriceCardStanding`（`CutForSuccessor`、`RetireForCoveringSuccessor`、`Selection`），`PriceCardCut`、`PriceCardRetirement`、`PriceCardClosureReference`，选用区间的读法只在 `SelectablePriceCardPeriod` 一处。
- **持久化**：迁移 `parcel_pricing/0013`——截止册（只增，按登记顺序取最新一条）与退役册（一版一行），后继都以外键指回同一方案的版本行；版本行一字不动。
- **读法**（决定五）：`ResolveInForce`（结算请求、受理前估价）与 `LoadApplicable`（试算）同一口径折进收尾；`FindByReference`（重放）不折；`FindPricedScopesBinding` 不算已退役的版本；管理台价卡目录 `/pricing-price-cards` 多回显 `selectionEndsAt`、`retiredAt`、`successorVersion`。
- **续办**（决定四）：收尾同笔交价卡种类的「参数已登记」，带旧版的计价范围与方向；信封标识另取 `price-card-closure` 指纹前缀，带收尾种类、后继与依据，与两版各自登记那一封分开。载荷不变，SA 一行没改、没有新的登记种类。
- **入口**：`parcel-api` 的 `POST /pricing-price-card-successor-cuts`、`POST /pricing-price-card-retirements`（操作者渠道的登记册配置写，新特性文件 `cmd/parcel-api/endpoints_pricing_price_card_closure.go`，未接线占位登在同名测试文件）；受控登记口 `parcel-pricing-register -kind price-card-cut|price-card-retirement`。没改 `endpoints.go`、`main.go`、`assemble*.go`、`routes*.go`。
- **文档**：ADR-0234；PP CONTEXT 增一条规则；UC-PP-001 增 `AT-PP-007` 至 `012`；AT 盘点加六行（总览计数未改）；票 14 那条跨方案派发用例的注释改指 ADR-0234。

### 代决（用户「你作为业务和系统专家自决」；逐条越权风险点见 ADR-0234 文末，此处不复述）

1. **截止口归登记册配置写**，不归运营决定（ADR-0151 判据：提交方是管理价卡登记册的操作者，授予按登记册；收尾不指名任何一份请求）。
2. **价卡要「同起点退役」那一半**，且放宽为「后继不晚于旧版起点」：补的是卡上从起点就在的缺陷，只做截止会让基准时点在旧版起点到截止点之间的请求永远停着；选卡只问每一刻由哪一版答，后继盖住旧版整段即可（与 ADR-0233 只收同起点不同，越权风险点 2）。
3. **收尾自己交「参数已登记」**：收尾成立的前提是后继已在册，后继登记那一封必然先到，它续办出的那一代只能停在适用冲突；能把它续办到后继上的是收尾这一封。中间那一代如实留着。
4. **已在旧版下形成的评价不标注、不改写**：评价在当时的册上是对的答案；要不要按后继重算已形成的金额是结算调整那一侧的事；用的那一版后来有没有被收尾，按版本清单查价卡目录即得。
5. **后继只能是同一定价方案的另一版**：跨方案让位与无后继退役要另裁与试算多候选的关系，归票 168。
6. 截止的登记时刻取服务端时钟、不进重放比对，依据由登记方给（价卡这一侧没有外部来源的区间更正）。

### 用例

- 领域 `internal/parcelpricing/domain/price_card_closure_test.go`：`TestACutHandsThePredecessorsRestToTheSuccessorAtItsStart`、`TestTheSelectablePeriodFoldsTheLatestCutAndTheRetirement`（007）；`TestARetirementLeavesOnlyTheCoveringSuccessor`（008，同起点与起点改早两格）；`TestReplayingAClosureRecognisesTheRegisteredOne`（009）；`TestAPriceCardClosureThatDoesNotHoldIsRefused`（010，九格）。
- 真库 `internal/parcelpricing/adapters/postgres/price_card_closure_test.go`：
  - 007：`TestASuccessorCutEndsTheApplicabilityConflictInTheStore`。先复现两版适用冲突；截止后截止点前一秒选 v1、截止点起选 v2，在用解析与适用装载同答；版本行仍无上界，重放点读原样。
  - 008：`TestARetirementLeavesOnlyTheCoveringSuccessorInTheStore`。退役后任何时点只选后继，绑序列的卡不再算已退役的那一版；重放回显原时刻，换依据答已凭别的退役。
  - 009：`TestReplayingASuccessorCutWritesNothingMore`。
  - 010：`TestAPriceCardClosureThatDoesNotHoldWritesNothing`（旧版或后继不在册、他租户、换了范围、同起点用截止），一行不写、一封不交。
  - 011：`TestConcurrentCutsOnOnePredecessorQueueOnItsRow`，后到者停在锁上、读到已截止答越界。
  - 012：`TestAClosureIsHandedOffApartFromTheCardRegistrations`，登记一封、截止与退役各一封，载荷同形。
  - 目录：`TestThePriceCardCatalogueEchoesTheClosures`。`transaction_guard_test.go` 补锁与两个写口的无事务负向证据。
- 用例层 `internal/parcelpricing/application/close_price_card_version_test.go`：交出的信封带旧版那一版、范围、方向与收尾身份，先锁的是旧版；不成立不交、内部方向不交、交不出整笔失败、缺交接口构造即拒（012）。
- 在线口 `internal/parcelpricing/adapters/http/register_price_card_closure_test.go`：形状门（自报租户、截止点、退役时刻、尾随内容答畸形）、没有授予 403 且不读载荷、答复回显（010）；目录在线口 `TestPriceCardsEndpointEchoesTheClosures`。
- 受控登记口 `cmd/parcel-pricing-register/price_card_closure_test.go`：两种文档的路由、翻译与退出码。
- **端到端**（票面要求）`cmd/parcel-dispatch/price_card_closure_resumption_test.go::TestARequestStalledOnAMissingWeightBandResumesOnceTheOldCardIsClosed`，真库、生产路由表，两格（截止在后继起点、同起点重发退役）：v1 缺 1.2 公斤那一段 → 第一代停在价表没有这一段 → 登 v2 → 凭 v2 的登记续办出第二代、停在适用冲突 → 收尾 v1 → 收尾那一封续办出第三代，计价选到 v2，结算形成 `BUYSC-<第三代>`；前两代与第一代那一份评价原样，重放同一收尾不出第四代。

### 验证

- 钉 `970441da`（此后只改票面）：`go build ./...`、`go vet ./...` 干净，变更的 Go 文件 `gofmt -l` 干净；每一笔提交导出到 scratchpad 单独 `go build ./...` 与相关包 `go vet` 过。
- `scripts/test-full.sh` 跑了以下各包，全绿：
  - 改动的 PP 各包及其反向依赖：`./internal/parcelpricing/...` 全部子包，`cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-pricing-feed`、`cmd/parcel-pricing-register`、`cmd/parcel-pricing-template`，NR、PS、SA 各自的 `adapters/parcelpricing`，SA `adapters/inbox`，`scripts/demo-seeds/seedgen`；
  - `./internal/architecture/...`、`./migrations/...`、`./tools/...`。
- 变异检验：
  - 把收尾用例里先锁旧版那一步换成恒得，`TestConcurrentCutsOnOnePredecessorQueueOnItsRow` 转红（后到者答 REGISTERED），换回后绿。
  - 收尾那一封沿用旧版登记那一封的信封标识，端到端用例两格都转红（被入队一步吞掉，第三代不出），换回后绿。
- 演示种子：在 55432 上建一次性库 `dcg150_seed`，`seed.sh` 灌一遍退 0（迁移 0013 施加、两张收尾册零行），用完 `DROP DATABASE … WITH (FORCE)`。种子数据与步骤未改。
- 没跑全量，没重生成 ADR 索引、AT 计数与机制清点（`adr-index -check`、`at-coverage-counts -check` 如预期报待重生成）。

### 没做与去向

- 让位给另一定价方案的退役、不指名后继的退役 → 票 [168](./168-price-card-has-no-retirement-without-a-same-plan-successor.md)。
- 管理台的两张收尾表单与价卡目录渲染收尾三格（本票不碰 `apps/admin-web`）→ 票 [169](./169-admin-web-price-card-closure-forms.md)。
- network-routing 线路版本按「方案/版本」指名成本依据，收尾不动它，择优照旧按被收尾的那一版试算（ADR-0234 越权风险点 6，运营约束：另发一版线路改指后继）；本票不改 NR，号段已用完，没另立票，留给协调方定要不要立。
- 收尾没有审批一步（越权风险点 1）、续办多推一代（越权风险点 4）留 owner 复核。
- 迁移号用了 `parcel_pricing/0013`（main 未动，`git rebase main` 无事可做）；没有新的 access_identity 决定种类，SA 没改。

