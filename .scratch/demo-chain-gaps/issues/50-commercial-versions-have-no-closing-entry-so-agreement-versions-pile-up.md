# 50 商业版本没有收尾写口：同一供应商协议的旧版无法替代或退役，新旧两版并存在册

Category: enhancement
Status: resolved——2026-10-21 进 main（全量钉 `1ec4059c`：`scripts/test-full.sh` 158 ok / 0 FAIL，见文末「进 main 记录」）；完整替代写口转票 [148](./148-commercial-version-supersession-write-face.md)，管理台收尾表单与目录余项转票 [149](./149-admin-web-commercial-version-closure-forms.md)，在途委托在收尾后成本不形成由票 [88](./88-actual-fulfillment-not-formed-when-adoption-or-agreement-arrives-late.md) 承接（88 落地前不得截止或退役仍被在途委托指名的供应商协议版本，见 ADR-0233）。此前 in-progress——完工待评审与进 main，分支 claude-dcg50。此前 in-progress——2026-10-21 认领；ready-for-agent——2026-10-20 上线就绪评估归 R1（上线前必修），裁定与批次见 [triage-2026-10-19](../triage-2026-10-19.md)「上线就绪评估（2026-10-20）」。此前 needs-triage——2026-10-19 Claude Code 作者子代理立（票 [29](./29-supplier-agreements-declare-charge-occurrence-conditions.md) 完成记录「偏离与待裁」，用户「你作为业务和系统专家自决……解决所有的问题」授权）。是否认领、归哪一位 owner 待分诊。
Blocked by: 无
地盘（建议）：PC `application` 发布用例或新用例、`registrationjson` 批文、`postgres` 版本册；可能需要新 ADR。
出处：[ADR-0209](../../../docs/adr/0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md) 越权风险点 5；PC CONTEXT「供应商商业协议版本」生命周期「协议版本到期、终止或被替代后……」。

## 现象

演示租户的干线协议以 `v2` 重发（声明收费发生条件）之后，`v1` 仍是`已生效`、区间与 `v2` 相同。领域上有 `CommercialVersion.SupersededBy` 与 `Retire`，但发布批、在线发布口与任何登记 CLI 都没有调它们的写口——生产上没有一个商业版本能被收尾（2026-10-19 在 `claude-dcg29` 上 `grep -rn '\.Retire(\|\.SupersededBy('` 只命中 PC 参与方关系与 VE 处置请求，没有商业版本）。

## 影响

- 运输履约按 `v1` 登记发生项答`未配置`（v1 没声明条件），登记方要自己知道该指名 `v2`。
- 真实租户换版时，旧版会一直`已生效`；按版本点读的消费方（委托提交、发生项登记）照旧接受旧版，换版不收口。
- 按范围解析的册（合同、政策）若出现同样的新旧并存，解析答`适用冲突`（票 30 的改判就是为了躲这一格）。

## 三类

机制（版本收尾写口）与产品策略（替代是否必须同对象、是否自动以新版生效时点收尾旧版）。哪一版何时收尾是租户取值。

## 完成判据（草拟，待分诊）

- 有一个受控入口能把一个商业版本以指名后继替代（`SUPERSEDED`）或按决定引用退役（`RETIRED`），写入版本册并留历史。
- 运输履约核对口对已替代 / 已退役的版本答未生效 / 已终止（适配器已有这两格）。
- 演示租户的干线 `v1` 由 `v2` 替代。

## 完成记录（2026-10-21，Claude Code 作者子代理，分支 claude-dcg50）

范围先照上线就绪评估只做截止；开工后协调方转来代决复核（main 本地提交 `d6c7902f` 的 `.scratch/owner-review-20261020/report.md`）M16、M17 两条并进本票：点读口折入收尾、同起点重发按决定退役，退役所需的 ADR 提前到本票，号 [ADR-0233](../../../docs/adr/0233-an-older-commercial-version-is-closed-for-a-named-successor-by-a-cut-or-a-same-start-retirement.md)。

### 做了什么

- **两种收尾，各一口**（ADR-0233 决定一至四）。截止：后继晚于旧版起点，旧版截止在后继此刻的选用起点，落一条区间更正（ADR-0038、0056）。同起点退役：两版同一起点，旧版按决定引用 `已生效 → 已退役`，另册保存，退役时刻取登记时刻。两种都要同一对象、同一范围、两版已生效、后继接得住旧版此后的那一段；先锁旧版行再读整册判。
- **领域**：`CommercialRegistry.CutAtSuccessorStart`、`RetireAtSameStartSuccessor`、`RegisterRetirement`；`SupplierAgreement.NarrowedBy`、`EndedBy`；自带区间的政策正文（价格、结算、信用）选候选时随壳的退役落选、被壳上的更正收窄（`bodyExcludedAt`）。
- **用例与端口**：`CloseCommercialVersionHandler`（`Cut`、`RetireAtSameStart`），端口 `ports.VersionClosureRegistry`。
- **持久化**：迁移 `party_commercial/0040`——区间更正册的类别约束放到 1..10（客户服务规则写得进截止）；新立 `commercial_version_retirement`。整册装载在各正文之前施加退役。
- **读法**（决定五，代决复核 M16）：供应商协议版本点读交回已退役的壳；版本点读与正文点读都把最新一条更正折进判用区间；内部成本政策点读折进退役。TF 与 PS 的适配器不动。
- **入口**：`parcel-api` 的 `POST /commercial-version-successor-cuts`、`POST /commercial-version-retirements`（操作者渠道的登记册配置写，新特性文件 `cmd/parcel-api/endpoints_commercial_version_closure.go`）；受控 CLI `parcel-commercial register-version-closures`，与在线口共用 `registrationjson.VersionClosureBatchDocument`。
- **演示**：种子步 `110-commercial-version-closures.sh`，干线协议 `v1` 让位给 `v2`、`v2` 让位给 `v3`（三版同一区间）；演示动线指名的 `v3` 不受影响，动线补记一条。
- **文档**：ADR-0233；PC CONTEXT 增一条规则；UC-PC-001 增 `AT-PC-054` 至 `058`；AT 盘点①加五行。

### 代决（用户「你作为业务和系统专家自决」；逐条越权风险点见 ADR-0233 文末，此处不复述）

1. 截止与退役分两口，不由服务端按起点替登记方挑（两者对已指名旧版的消费方是未生效与已终止两种答复）。
2. 截止点不由登记方给，取后继此刻的选用起点；更正依据与更正时刻由登记方给（重放靠它们逐字相同来认）。
3. 退役时刻取登记时刻，不由登记方给；重放比对后继与决定引用、不比时刻。
4. 「后继接得住」同时看壳与政策正文的终点；截止另要求后继的政策正文从截止点开始。发布不要求正文与壳同区间，这两格在收尾时判。
5. 政策正文（与供应商协议正文）只被壳上最新一条更正的终点收窄，不拿更正的起点裁正文前段（评审 2 改定，原先是整段取交）；壳没有收尾时照旧只看正文区间，不拿壳的原区间去收窄它（今天按正文区间选中的版本不能因为这一处换判法而落选）。
6. 供应商协议正文点读对已退役的那一版一律答不支持采购，终止时刻读作退役时刻（不按业务时点）；版本点读交回已退役的壳、只交壳。
7. 内部成本政策点读口只折退役、不折区间更正：它本来不按区间判。
8. 演示把 `v1`、`v2` 都退役，只留 `v3`。

本票实施层面另有几处，ADR 里没有：

- **发布新版与收尾旧版不在同一笔**：两步之间的窗口里，同范围解析照旧答`适用冲突`。停在上面的委托靠接受判断的内部重试续办（PS 把这一格译成本方重问），收尾口不发「参数已登记」续办信封。
- **退役一份已被委托指名的供应商协议版本，会让这份委托此后的发生项登记停住**，要等票 88 的「委托协议换版」改指后继（ADR-0233 越权风险点 1）。演示没有委托指名 `v1`、`v2`。
- **整份种子不加 `--reset` 重跑，停在网络节点登记的主键上**。这与本票无关，步 110 自己以重放回答。

### 用例

- 领域 `internal/partycommercial/domain/version_closure_test.go`：`TestCutAtSuccessorStartSplitsTheOverlapByTime`、`TestACutNarrowsPolicyBodiesThatCarryTheirOwnInterval`（054）；`TestSameStartRetirementLeavesOnlyTheSuccessor`（055）；`TestReplayingACutRecognisesTheRegisteredOne`、`TestReplayingARetirementRecognisesTheRegisteredOne`（056）；`TestACutThatDoesNotHoldIsRefused`、`TestASameStartRetirementThatDoesNotHoldIsRefused`（057）；`TestASupplierAgreementBodyFoldsTheVersionClosure`（058）。
- 真库 `internal/partycommercial/adapters/postgres/version_closure_test.go`：
  - 054：`TestASuccessorCutEndsTheConflictInTheStore`。先复现旧版未截止时答`适用冲突`，截止后新版生效时点之前得旧版、之后得新版；已固定的解析记录读回不变，提交前重校答已失效。
  - 055、056：`TestASameStartRetirementLeavesOnlyTheSuccessorInTheStore`。版本行状态列仍是已生效，重放回显原时刻，换依据答未受理，已退役再截止答未受理。
  - 056：`TestReplayingASuccessorCutWritesNothingMore`。
  - 057：`TestASuccessorCutThatDoesNotHoldWritesNothing`，覆盖对象不同、同起点、范围不对、他租户四格。
  - 并发：`TestConcurrentCutsOnOnePredecessorQueueOnItsRow`，后到者停在锁上，读到已截止而被拒。
  - 迁移 0040：`TestACustomerServiceRuleCutLandsInTheStore`。
  - 058：`TestSupplierAgreementPointReadsFoldTheVersionClosure`。
  - 内部成本政策点读：`TestInternalCostPolicyReadFoldsTheRetirement`。
  - `transaction_guard_test.go` 补锁与退役两个写口的无事务负向证据。
- 在线口 `internal/partycommercial/adapters/http/register_version_closure_test.go`：形状门、未授权（403 `OPERATOR_NOT_GRANTED`，不读载荷、不调编排）、答复回显（057）。
- TF `internal/transportfulfillment/adapters/partycommercial/supplier_agreement_closure_db_test.go`：`TestAClosedAgreementVersionIsCheckedAsClosed`（058）。截止后按截止点之后的时点核答 `AGREEMENT_NOT_EFFECTIVE`、之前照旧在册；退役后答 `AGREEMENT_TERMINATED`；后继照旧在册。
- CLI `cmd/parcel-commercial/register_version_closures_test.go`：批文两节的翻译与拒收。

### 验证

- 钉 `5812e0c2`（此后只改票面）：`go build ./...`、`go vet ./...` 干净，变更的 Go 文件 `gofmt -l` 干净。
- `scripts/test-full.sh` 跑了以下各包，全绿：
  - 改动的 PC 各包及其反向依赖：`cmd/parcel-api`、`cmd/parcel-commercial`、`cmd/parcel-dispatch`、`cmd/parcel-frontline-import`，以及 NR、NO、PS（`adapters/partycommercial`、`adapters/postgres`）、SA、TF、VE 各自的 `adapters/partycommercial`。
  - `./internal/architecture/...`、`./migrations/...`、`./tools/...`。
- 变异检验：把用例里的先锁一步换成恒得，并发用例即红（后到者答已登记），换回后绿。
- 每一笔提交前把暂存区导出到 scratchpad 的 `dcg50-` 目录单独 `go build ./...`、`go vet`。
- 演示种子：在 55432 上建一次性库 `dcg50_seed`，`seed.sh` 灌两遍。第一遍步 110 两项 `REGISTERED`、整份退 0；第二遍两项 `ALREADY_REGISTERED`。用完 `DROP DATABASE … WITH (FORCE)`。

### 过程记录（stash 事故核对，协调方要求写进来）

- 00:28 我用 `git stash push -k -- <路径>` 暂存半成品，想单独核对暂存区能否编译。stash 列表是各 worktree 共用的。票 psb/18 的会话随后在它自己的 worktree 里 `git stash pop`，弹走了这一份（`683fe655`），过后用 `git stash store` 原样放回。
- **(a) 内容完整**。我是从 `683fe655` 逐文件取回的，没有 pop。stash 涉及的 8 个文件逐字节比对全部一致：4 个从 stash 取回，另 4 个本地还在、内容相同。未跟踪的新文件从没离开本 worktree，也没有混进别人的东西。取回后核对 `stash@{0}` 仍是 `683fe655`，才把它 drop 掉，免得再被别人弹走。
- **(b) 扑空那一次没有写入**。那几分钟里我的 `git stash pop` 扑空（`No stash entries found`），什么也没写，没有重写或丢失任何东西。
- 收尾时核对：`idp-parcel-psb18` 已移除，main 上没有本票的任何文件。此后不再用 `git stash`；暂存区的单独编译改为导出到 scratchpad 下 `dcg50-` 前缀的目录里做。

### 没做与去向

- **退役已在本票，完整替代仍归 R2**：壳进`已替代`的替代写口、不指名后继的一般退役、后继晚于旧版起点时的退役、延长型的区间更正、合同委派与授权授予随版本收尾，都去 [148](./148-commercial-version-supersession-write-face.md)，需新 ADR。方向照上线就绪评估：替代必须同对象、显式指名后继，不以新版生效时点自动收尾旧版。
- 管理台的收尾表单（本票不碰 `apps/admin-web`）→ [149](./149-admin-web-commercial-version-closure-forms.md)。
- **TF/PS 碰了哪些文件**：TF 只加一个测试文件（上列），不碰派送一带；PS 没有改动。M16 说的 PS `agreementOf` 同病，病根在 PC 正文点读口，已在那里修好。
- ADR 索引、机制清点与 AT 计数没有重生成，进 main 时由协调方做。
- 迁移号用了 `party_commercial/0040`；没有新的 access_identity 决定种类。

## 评审记录（2026-10-21，隔离评审无阻断项；修订在同一分支）

必修：

1. **ADR-0233 越权风险点 1 补实**（`1d4d6e51`）。写明三件事：
   - (a) TF 自动形成实际履约发生项时（`registrationDisposition`），已终止与未生效都判作已了结，成本静默不形成；
   - (b) 截止对在途委托同样生效；
   - (c) 与 PC CONTEXT「既有委托继续引用原版本」、ADR-0204 决定一的张力。
   不加硬护栏，承接口是票 88。运营约束「票 88 落地前，不得截止或退役仍被在途委托指名的供应商协议版本」写进了 ADR 的状态行、决定一与种子 README 第 8 节（`d3708a79`）。票 88 追加「出自票 50 评审」一节（`bd0a2c17`）。TF 文件未改。
2. **「接得住」的两个洞**（`6ef1ce9c`）。
   - (a) 同起点退役另要求后继正文起点不晚于旧版正文起点，用例 `TestASameStartRetirementRefusesASuccessorBodyStartingLater`。
   - (b) 取第二条：壳上的更正对正文只按终点收窄。用例 `TestACutLeavesTheFrontOfABodyStartingBeforeItsShell`，供应商协议同口径用 `TestASupplierAgreementCutKeepsTheBodyFront`。理由：
     - 截止口只截终点（起点取壳此刻的选用起点），本来就不改起点；
     - 第一条（旧版正文起点早于壳起点就拒）在供应商协议上判不了：协议正文不进整册，领域截止时看不到它；
     - 这几类按正文选候选，截止要截的是正文的终点。
   改起点、延终点的更正今天没有生产写入方，记在 ADR 越权风险点 3。
3. **管理台目录折入收尾**（`289a09d5`）。服务产品、客户合同、授权规则、供应商协议四张的状态格认退役；供应商协议另回显 `selectionEndsAt`、`successorVersion`、`retiredAt`，原区间不覆盖。真库用例两处（合同目录、供应商协议目录），在线口一条。其余目录的余项记在票 149（`bd0a2c17`）。
4. **PC CONTEXT 那一条收窄**（`1d4d6e51`）。改为「按范围解析与按版本点读、用于新决定的口（供应商协议、内部成本政策）同一口径；为既有引用点读的口（按回指取已固定的解析、按已固定的快照取正文）不折收尾」。ADR 决定五同改。
5. **种子 README 第 8 节**（`d3708a79`）。干线「采用 v1」改为指 `v3`、标明断言期那段只作历史读，并补 2026-10-21 一段。

顺手修：

6. **「接得住」按此刻还能被选中的那一段比**（`6ef1ce9c`）。新增 `bodySelectionOf`（正文被最新更正终点收窄后的那一段），`requireTakeOver`、截止的正文起点门与 `bodyExcludedAt` 共用这一条判法。用例 `TestATakeOverIsJudgedOnWhatTheResolutionStillSelects`。
7. **次序敏感**：取写进 ADR（越权风险点 9：按从旧到新的次序收尾），不改判法。理由：倒序时被拒是安全的一边，不留错误状态；按册上同对象整体覆盖来判要对区间求并，归票 148。
8. **PS 与 TF 对同一次退役分格不同**：取记进 ADR 越权风险点 2，不统一分格。理由：两侧都不放行，只是分格不同；PS 那一侧的 `AgreementFor` 今天没有生产实现，统一要改 PS 适配器或点读口的终止时刻读法，等 owner 定按决定时间还是按业务时点。

另：AT 盘点 054、057、058 三行补评审新增的领域用例（`8a24df3d`）。

验证（钉 `8a24df3d`）：
- `go build ./...`、`go vet ./...`、变更文件 `gofmt -l` 干净。
- `scripts/test-full.sh` 跑了以下各包：
  - `./internal/partycommercial/...`；
  - TF、PS、NR、SA 各自的 `adapters/partycommercial`；
  - `./cmd/parcel-api/...`、`./cmd/parcel-commercial/...`、`./cmd/parcel-dispatch/...`；
  - `./internal/architecture/...`、`./migrations/...`。
  全部通过。
- 种子数据与步骤未改（只改 README），未重灌。

## 进 main 记录

2026-10-21 协调方合入。分支基点 `61d1470a`，其后 main 多了票 12、42、product-strategy-boundary/18、代决复核文档与 `ci.yml`；与本分支只在 `.scratch/at-coverage-inventory.md` 与 `scripts/demo-seeds/README.md` 重叠，cherry-pick 19 笔按原序到 `cb087e5a` 无冲突，其余各文件与分支逐字相同；SHA 随之换新（分支 `afdba221`…`447c1bd8` 作封存出处，main 上为 `5fe6bf6c`…`d28452eb`）。本票与 psb/18 都改了演示种子（步 110 与步 460），叠合后在 55432 一次性库上 `seed.sh` 全程退 0，用完 `DROP … WITH (FORCE)`。其上按 parallel-sessions.md 重生成 ADR 索引、重算 AT 盘点计数、重生成机制清点（`1ec4059c`），各自单独成笔；`adr-index -check` 与 `at-coverage-counts -check` 退 0。在干净的隔离树上对 `1ec4059c` 跑 `go build ./...`、`go vet ./...`、`gofmt -l` 干净，`scripts/test-full.sh` 全量 158 ok、0 FAIL（`-count=1`，无缓存）；推 `1ec4059c:main`。迁移落号 `party_commercial/0040`（连号）。
