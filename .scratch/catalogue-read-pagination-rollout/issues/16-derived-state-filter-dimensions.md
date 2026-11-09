# 16 状态由读面派生的几册补状态筛选维

Category: enhancement
Status: resolved——2026-11-09 进 main；七个状态维已放，五处导不出的写在判断项并有 ADR-0372 草案（Proposed，待 owner 复核）
Blocked by: 04、05、06、08、09、12（这几张票进 main 之后，在它们落成的读面上加）
地盘：下表各册所在上下文的 `ports` 声明、postgres 读面、端点答复体与用例；需要时各上下文一条索引迁移。
出处：[ADR-0144](../../../docs/adr/0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md) 决定四；spec「可排维、筛选维从小放」。

## 为什么

几张迁移子票没给下表各册放状态筛选维，理由都是「状态不是登记表上的一列，是读口或页面现算的」。ADR-0144 决定四要求有状态的册至少放出状态一维，
筛选维名取答复 JSON 的字段名；答复里没有那一格，就无从筛。翻页之后管理台若仍在页内按状态筛，筛的只是当前一页，比迁移前在截断的一批里筛更难察觉。

| 册 | 来自 | 状态怎么来的 | 出处 |
|---|---|---|---|
| 就绪判断、提交授权（`/customs-case-registers` 按 `registry` 分派的两册） | 09 | 「失效与否」，读面里判 | 09 的非作者评审（钉 `2f952629a`） |
| 订舱请求（`/transport-fulfillment-records` 按 `registry` 分派的那一册） | 05 | 读口在 Go 里派生 | 05 的作者交回 |
| 派送预报、交接单、身份复核单元（`/node-operations-records` 按册名分派的几册） | 06 | 预报撤回、交接单关闭与范围、复核单元冲突与处置，读面里判 | 06 的非作者评审（钉 `bb8e4f322`） |
| 价卡（`/pricing-price-cards`） | 04 | 管理台页面现算 | 04 的作者交回 |
| 供应商账单接收、客户对账单（`/settlement-statements` 按 `registry` 分派的两册） | 08 | 接收进度读面派生；对账单作废没放成筛选维 | 08 的非作者评审（钉 `5191ce705`） |
| 授权区间、暂停（`/governance-registers` 按 `register` 分派的两册） | 12 | 区间开闭、暂停是否已恢复，读面里判 | 12 的非作者评审（钉 `9700d7ffc`） |

## 做什么

逐册判断状态能否在 SQL 里由本册自己的行导出（`CASE` 表达式或 `EXISTS`）：

- 能：读面导出这一列，答复体加这一格（已有则复用），声明放出状态维（词表取领域里那份封闭词表），落列 `Filters` 落到那个表达式；排序或筛选要走索引时补表达式索引。
- 不能（要跨上下文取数、要按现行规则版本现算等）：写进本票判断项，回 ADR-0144 改决定四的适用范围，不在代码里默默豁免。

## 不做

- 不改状态的判定规则本身，不改写面；管理台接这一维归 14 里对应上下文那一笔。

## 完成判据

- 每册：真库用例证状态维筛出的行与答复里的状态格一致、`total` 随之变；端点用例证词表外取值答 400 带理由散文。
- 导不出的册：票面写明理由，并有一篇待接受的 ADR 草案。

## 判断项

1. **能导出与不能导出的界。** 逐册看状态从哪来：是本行某一列（含它在不在），或以本行为键的留痕册上有没有那一行（撤销、撤回、关闭、退役、恢复），
   就在 SQL 里以 `(<条件>)::text` 或 `(EXISTS (…))::text` 导出，筛与答复里的状态格、与领域读的是同一格事实；`EXISTS` 一律按留痕册主键探、带租户。
   状态要由领域规则在多行之上求结论、要按读取时点现算、或册上根本没有领域状态的，不导，写进下面一条并起草 ADR-0372。界划在「有没有那一行」与
   「那几行合起来怎么判」之间，这一刀本身列在 ADR-0372 越权风险点「「留痕册上有没有那一行」算不算本册」，评审若认为 `EXISTS` 也算第二处定义，已补的四维要回退。
2. **导不出的五处（取证钉 `f117de0aa`，这几处派生代码本票未改）。**
   - 订舱请求（05）：状态词由名下各份承运答复经 `domain.CurrentCarrierAcceptance` 选出当前那一份、再比接受量与申请量派生；那个函数头注写明「当前那一份」
     只在它一处定义、查阅口也经它；答复史凑不成时它答错误而不是挑一个词。SQL 里照写是第二处定义，坏行上还会悄悄给出一个词。
   - 送站交接单的范围成立（06）：`DeliveryHandoverNoteState` 的 `ScopeEstablished` 按声称账户的绑定结论求（ADR-0212 决定三）。交接单的生命周期
     「开放 → 已关闭」已作 `closed` 放出，范围成立不是生命周期。
   - 身份复核单元（06）：一件实物此刻冲突与否、处置过没有，是领域对它各链当前有效版本合起来的身份判断（`domain.JudgeIdentityBasis`，处置编排的
     `headsInConflict` 经它判）；答复里没有实物一级的状态格。
   - 价卡的页面五态（04）：管理台 `priceCardStatus(row, now)` 按此刻现算；CONTEXT 写明截止不是状态迁移。领域状态「已发布 → 已退役」已作 `retired` 放出。
   - 供应商账单接收进度（08）：管理台 `billProgress` 从读口的几个和派生，`supplier-bills.ts` 文件头与它上方的词表块注都自称呈现词、不是领域状态；「审核人未配置」一格还读此刻审核授权册。
   五处的理由各写进声明头注并指到 ADR-0372 草案（Proposed，拟收窄 ADR-0144 决定四），没有在代码里默默豁免。
3. **词表取 `true` / `false`。** 补上的几维在领域里都是一个布尔（`Effective()`、`Withdrawn()`、`Closed()`、退役与否、作废与否、终点缺席、有无恢复决定），
   照 customs-compliance 提交版本册 `isCurrent` 的先例，词即答复体里那个布尔的两种写法。要领域词得先立枚举，列在 ADR-0372 越权风险点「布尔词表取 `true` / `false`」。
4. **答复体加了六格、复用两格。** 新加且恒在场：就绪判断与提交授权的 `effective`、价卡的 `retired`、对账单的 `voided`、权威区间的 `open`、暂停的
   `resumed`（读面随行选出 `EXISTS`，扫进新加的 `ports.SuspensionRegistryRow.Resumed`，与筛选维落列共用常量 `suspensionResumed`）。复用：预报的
   `withdrawn`、交接单的 `closed`（本就恒在场）。管理台的类型还没有这几格，多出的字段不影响现有页面；接这一维归 14。
5. **多放了一册：待对账队列。** 它与对账单册共用 `customerStatementSpec` 与 `customerStatementPageColumns`（同列同解码），一并放出 `voided`；
   作废之前已开的争议照旧让已作废的单留在队列里，筛它有用。队列张数仍是整个积压、不随这一维变（读面在有筛选时本就另数一次），真库用例证了。
6. **两维说登记、不说此刻。** `open` 是区间还没有终点，不是此刻在效（终点可在将来）；`resumed` 是名下有恢复决定（与 `GovernIncidentHandler.Resume`
   的「已恢复」同一个读法），不是此刻已不拦（恢复另有生效时点，此刻拦不拦仍由 `Suspensions.FindUnresumedSuspension` 按时点现算）。两处声明头注都写明，
   真库用例里那条恢复的生效时点特意落在一年后。
7. **不建索引，没有新迁移。** 本行列上的几维是本租户（治理册无租户）行上的再滤一遍，不改次序；`EXISTS` 那几维探的是留痕册主键
   （`delivery_pre_advice_withdrawal_pkey`、`delivery_handover_closure_pkey`、`price_card_version_retirement_pkey`、`resumption_decision_pkey`）。
   缺省序的索引各子票已建，不动。
8. **ADR 索引与机制清点没重生成**（照派单）：`go run ./tools/adr-index -check` 在本分支上因新草案未入索引退非零，由推送方进 main 时重生成。ADR-0144
   的前向指针等 ADR-0372 被接受时再加。

## 完成记录（2026-11-09，作者会话，分支 `worktree-agent-a898ab22097add99e`，基 `f117de0aa`）

**落点**

| 笔 | 上下文 | 做了什么 |
|---|---|---|
| `4ddf1c16e` | customs-compliance | `ReadinessJudgmentPages`、`SubmissionAuthorityPages` 放出 `effective`；落列 `revocationAbsent`；两种答复体加 `effective`；真库与端点用例 |
| `93c51caa1` | node-operations | `DeliveryPreAdvicePages` 放出 `withdrawn`、`DeliveryHandoverNotePages` 放出 `closed`（`EXISTS` 留痕册）；`IdentityReviewUnitPages` 与交接单头注写不放的理由；真库与端点用例 |
| `40b84fba5` | parcel-pricing | `PriceCardPages` 放出 `retired`（`EXISTS` 退役册）；答复体加 `retired`；头注写五态不放的理由；真库与端点用例 |
| `58f3b60a8` | settlement-accounting | `customerStatementSpec` 两册放出 `voided`；答复体加 `voided`；`SupplierBillReceptionPages` 头注写不放的理由；真库与端点用例 |
| `92144084c` | pilot-governance | `AuthorityIntervalRegistryPages` 放出 `open`、`SuspensionRegistryPages` 放出 `resumed`；行类型加 `Resumed`；两种答复体各加一格；真库与端点用例 |
| `3f738f6d8` | transport-fulfillment | 只改 `BookingRequestPages` 头注：不放的理由改指 ADR-0372 草案 |
| `0da0c8c6b` | docs/adr | ADR-0372 草案（Proposed） |
| 本笔 | 票面 | 判断项与本记录 |

**各册**

| 册 | 结论 | 状态维 · 落列 | 判据 |
|---|---|---|---|
| 就绪判断 | 导出 | `effective` · `(revoked_at IS NULL)::text` | ✅ 真库 `TestReadinessAndAuthorityStatusDimensionAgreesWithTheEffectiveField`（筛出的行经重建门读回的 `Effective()` 与词一致、total 随之变、与 q 为与），表驱动 `pagedRegisters` 带上这一维；✅ 端点 `pagedCatalogues` 的词表外取值答 400 带 `detail`，`TestReadinessAndAuthorityListsTranscribeRevocationVerbatim` 证答复里 `effective` 恒在场 |
| 提交授权 | 导出 | 同上 | ✅ 同上两组用例各过一遍 |
| 订舱请求 | 导不出 | — | ◑ 票面判断项 2、ADR-0372 草案、声明头注；无代码改动 |
| 送站预报 | 导出 | `withdrawn` · `EXISTS` 撤回册 | ✅ 真库 `TestDeliveryScopeStatusDimensionsAgreeWithTheAnswerFields`（`Withdrawn()` 与词一致、total 随之变、与 q 为与、别的租户同标识的撤回不串进来）；✅ 端点 `TestDeliveryScopeListsRefuseQueriesOutsideTheDeclaration`（词表外、另一条列表的状态维答 400 带 `detail`）、`TestDeliveryScopeListsHandTheStatusDimensionToTheReader`、未配置渠道同答 403 |
| 送站交接单 | 导出 `closed`；范围成立导不出 | `closed` · `EXISTS` 关闭册 | ✅ 同上两组用例（`Closed()`）；◑ 范围成立见判断项 2 |
| 身份复核单元 | 导不出 | — | ◑ 判断项 2、ADR-0372、声明头注 |
| 价卡 | 导出 `retired`；页面五态导不出 | `retired` · `EXISTS` 退役册 | ✅ 真库 `TestPriceCardRetiredDimensionAgreesWithTheRetirementOnTheRow`（`HasRetirement` 与词一致、total 随之变、与 direction 和 q 为与、别的租户同版本的退役不串进来）；✅ 端点 `pagedCatalogues` 的词表外与空取值答 400、`TestPriceCardsEndpointEchoesTheClosures` 证 `retired` 恒在场；◑ 五态见判断项 2 |
| 客户对账单（含待对账队列） | 导出 | `voided` · `(s.voided_at IS NOT NULL)::text` | ✅ 真库 `TestCustomerStatementVoidedDimensionAgreesWithTheVoidTrace`（两册，作废时刻在场与否与词一致、total 随之变、队列张数不变）；✅ 端点 `pagedCatalogues` 两册的词表外与空取值答 400、`TestCustomerStatementQueryKeepsVoidedStatementsInPlace` 证 `voided` 恒在场 |
| 供应商账单接收 | 导不出 | — | ◑ 判断项 2、ADR-0372、声明头注 |
| 生产权威区间 | 导出 | `open` · `(to_at IS NULL)::text` | ✅ 真库 `TestGovernanceStatusDimensionsAgreeWithTheAnswerFields`；✅ 端点 `TestGovernanceRegistersRefuseQueriesOutsideTheDeclaration`（词表外答 400 带 `detail`、未配置同答 403）、`TestGovernanceRegistersHandTheStatusDimensionToTheReader`、`TestGovernanceRegistersEndpointTranscribesAuthorityIntervals` |
| 暂停决定 | 导出 | `resumed` · `EXISTS` 恢复册 | ✅ 同上（生效时点在将来的恢复也算有恢复决定；恢复册带 `resumed` 答 400）、`TestGovernanceRegistersEndpointTranscribesSuspensionsAndResumptions` |

✅ 是该票「每册」判据（真库：筛出的行与状态格一致、total 随之变；端点：词表外取值答 400 带理由散文）全过；◑ 是「导不出的册」判据：理由写进票面、有一篇待接受的 ADR 草案。
几处落列另做过反证：customs 落列取反、预报 `EXISTS` 去掉租户条件，对应用例各自变红，改回后绿。

**跑了什么**（WSL go1.26.8，钉 `0da0c8c6b`）

- 全仓 `go build ./...`、`go vet ./...` 退 0；分支上改过的 `.go` 文件 `gofmt -l` 为空。
- 带 DSN（`IDP_PARCEL_POSTGRES_DSN`）`go test -count=1 -p 1 -v`，93 个包：改过的 16 个包，加 `go list` 的 `Deps` 与测试导入反查出的全部反向依赖（含
  `cmd/*` 十二个），加 `internal/architecture/...`。86 个 ok、7 个无测试文件，9672 PASS、0 SKIP、0 FAIL（`--- PASS` / `--- SKIP` 计数，子测试计入）。
  其中：customs-compliance `adapters/postgres` 458、`adapters/http` 384；node-operations `adapters/postgres` 236、`adapters/http` 407；parcel-pricing
  `adapters/postgres` 114、`adapters/http` 188；settlement-accounting `adapters/postgres` 401、`adapters/http` 344；pilot-governance `adapters/postgres` 75、
  `adapters/http` 28；`cmd/parcel-api` 656、`cmd/parcel-dispatch` 507；`internal/architecture` 174。
- 本票没有迁移，迁移字节判据不适用。

**未验**

- 没在租户级数据量上跑 `EXPLAIN`：`EXISTS` 几维是否按留痕册主键探、没有实测。
- 管理台未改、未在浏览器里点；接这几维归 14。
- `go run ./tools/adr-index -check` 未过（新草案未入索引，见判断项 8）；全量 `scripts/test-full.sh` 由推送方在重放后的 tip 上跑。

## 评审记录

- 2026-11-09 非作者两轴评审（推送方 Claude Code 会话另起的隔离子代理，只读，钉作者分支尖 `110a17de5`，基 `f117de0aa`）：跑过全仓 `go build`、`go vet`，改过的十六个包带 DSN `go test -count=1 -p 1`（11 ok、5 无测试文件），新增五个真库用例 `-v` 单跑 41 PASS。
  - Spec 轴：无阻断。核过各落列表达式与领域读法一致（撤销两列配对约束、四本留痕册的 `EXISTS` 按主键探且带租户条件、退役取登记时刻、治理区间只追加），维名都等于答复体字段名，五处导不出的判断都站得住，待对账队列加 `voided` 后张数与成员口径不变，没有改写面与领域包，ADR-0144 未动、ADR-0372 正确停在 Proposed。非阻断四条：新维管理台没人接（见下「进 main 记录」）；六处 Go 注释写死「ADR-0372 草案」；ADR-0372 引文出处、指针位置与 ADR-0144 无 Links 节三处小问题；待对账队列两处注释漏写筛选维。
  - Standards 轴：无阻断。非阻断一条：`true` / `false` 词表散在五处，与 Postgres `boolean::text` 隐式耦合。
  - 修订：推送方在重放线上修于 `01783b9be`（词表收成 `cataloguepage.BooleanVocabulary`，注释只写编号，两处队列注释补全，ADR-0372 与本票判断项的引文出处改正、「拟收窄」挪到索引注记之前、写明 ADR-0144 要新建 Links 节）。同一评审者复评：阻断 0，非阻断一条（删词表变量后留下两段悬空注释），修于 `a30f83205`。

## 进 main 记录（2026-11-09，推送方 Claude Code 会话）

- 重放：在隔离 worktree 里把作者分支八笔 cherry-pick 到 `origin/main`，无冲突（作者分支基点以来 main 上没有与它重叠的文件）；评审期间 main 只前进了管理台几笔，随后 rebase 到 `491cfb776`。作者分支 `worktree-agent-a898ab22097add99e` 的 `4ddf1c16e`、`93c51caa1`、`40b84fba5`、`58f3b60a8`、`92144084c`、`3f738f6d8`、`0da0c8c6b`、`110a17de5` 在 main 上依次为 `1935e1707`、`482003377`、`b49ab6a79`、`269625e4c`、`ddfd039d0`、`a50d4b39c`、`142505fe7`、`ab2157618`；评审修订 `01783b9be`、`a30f83205`。
- 生成物：ADR 索引重生成两笔（`0b922aea5`、`e22455557`），`adr-index -check`、`at-coverage-counts -check` 退 0，机制清点重生成无变化。
- 全量真库：`scripts/test-full.sh` 在重放后的 `214c43281` 与 rebase 后的 `e22455557` 上各跑一次，都退 0、175 个包 ok。`a30f83205` 只删两段注释，未重跑全量，跑了 `go build`、`go vet` 与两包测试。
- 种子：`scripts/demo-seeds/seed.sh` 在 `214c43281` 上灌进一次性库，退 0，用后删库。
- 管理台接这七维：归票 14，同日随后一笔接上（见票 14）。
- ADR-0372 仍为 Proposed，待 TF、NO、PP、SA owner 复核；接受时照它的 Consequences 给 ADR-0144 挂前向指针、新建 Links 节。
