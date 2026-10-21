# 50 商业版本没有收尾写口：同一供应商协议的旧版无法替代或退役，新旧两版并存在册

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg50。此前 in-progress——2026-10-21 认领；ready-for-agent——2026-10-20 上线就绪评估归 R1（上线前必修），裁定与批次见 [triage-2026-10-19](../triage-2026-10-19.md)「上线就绪评估（2026-10-20）」。此前 needs-triage——2026-10-19 Claude Code 作者子代理立（票 [29](./29-supplier-agreements-declare-charge-occurrence-conditions.md) 完成记录「偏离与待裁」，用户「你作为业务和系统专家自决……解决所有的问题」授权）。是否认领、归哪一位 owner 待分诊。
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
5. 政策正文与壳上的更正取交、不取代；壳没有收尾时照旧只看正文区间，不拿壳的原区间去收窄它（今天按正文区间选中的版本不能因为这一处换判法而落选）。
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

