# 94 SA 同一条资金事实映射到多个作用域时，每个作用域都按全额记入入账余额

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code94（见文末「完成记录」）。原 ready-for-agent——R1（上线前必修）：碰钱且静默。同一笔收款映射到两个客户应收作用域，两边的入账余额都按全额算，可用余额凭空多出一笔，没有任何提示。预付冻结读可用余额，以入账余额为基数的比例额度读 `posted_minor`（parcel-dispatch 生产装配），两处都会按多出来的钱放行。2026-10-26 票 [code/12](./12-funds-mapping-has-no-compensation-or-recovery-targets.md)、[code/46](./46-settlement-application-and-reversal-have-no-process-entry.md) 第三轮轻评审 N-a 立（用户「你作为业务和系统专家自决」授权，协调方裁定立票），分支 claude-code12。
Blocked by: 无
出处：code/12、code/46 第三轮隔离轻评审 N-a，评审真库手算；取证于分支 claude-code12。

## 现象

- SA CONTEXT「运营余额、收付款与坏账」：「入账余额……按该作用域已采用并已映射的客户方向真实收付净额减去指向该作用域客户应收的核销分配净额重算并推进……未分配就是这个差」；「不同责任法人、结算账户和币种默认不能共用余额」。
- 代码：`CustomerReceiptPostings.CustomerDirectionNets` 的 P 在一个作用域之内按（租户、事实）去重（code/12 代决 10），但每个作用域各算各的：同一条事实只要有一条映射落在这个作用域的对账单或账户上，这个作用域就按链头全额记入。
- 评审手算的例子：
  - 收款 F2 金额 5000，映射到 acct-1 下的 stmt-1（作用域 A），又映射到账户 acct-2（作用域 B）。
  - 全部 5000 都核销到 stmt-1：A 的入账余额 0，账是平的。
  - B 的入账余额仍是 5000，B 的可用余额凭空多出 5000，没有任何提示。

## 三类

机制。无产品策略；无租户取值。

## 规模

M。

## 最小的诚实修法

二选一，见「要裁的」。推荐乙。

## 要裁的

- **甲：映射时拒绝。** 一条事实已经映射到某个客户应收作用域，再映射到别的作用域就答`未受理`。实现最小，但挡掉了合法场景：同一付款人一笔款付两个账户。
- **乙（推荐）：P 按核销分配拆分。** 一条事实只映射到一个作用域时，照旧全额记入。映射到多个作用域时，每个作用域只记指向它的那部分核销分配；还没核销的余额停在未分配，不计入任何一个作用域。有几处要一并定：
  - 已映射到多个作用域、还没有任何核销的那段时间里，每个作用域各记多少；
  - 资金退回在多个作用域之间怎么分；
  - 读面上让「未计入任何作用域的未分配」看得见。

## 完成记录（2026-10-26，作者会话，分支 claude-code94，变基到 origin/main `4bb6c2fa0`）

协调方裁「要裁的」走乙。证据只记 `S`（隔离合成）。立 [ADR-0283](../../../docs/adr/0283-a-funds-fact-mapped-into-several-customer-scopes-is-posted-by-its-applications.md)，部分停用（收窄）ADR-0181 决定二两句。不加迁移（协调方预留的 SA 0047 未用）。

### 落了什么

1. **P 的两种记法**：`CustomerReceiptPostings.CustomerDirectionNets` 的 P 逐条事实记一次。只落到本作用域一个客户应收作用域的取链头全额（不变）；落到几个作用域的只记指向本作用域、尚未撤销的核销分配净额，收款没核销的部分不计入任何作用域，退回没核销的部分（事实金额减它全部未撤销核销合计，不低于 0）在每个作用域都先扣。一片分配指向哪个作用域，看它沿的那条映射落在哪个作用域上——对账单分配与账户分配都算。W 与已确认未结不变。
2. **作用域口径收成一处**：`mappingReceivableScopesSQL`（对账单映射经对账单落到同币种的应收账户，账户映射落到应收账户本身）供 P、新读口 `ports.CustomerReceiptBooks.FactCustomerScopes`、资金事实册的作用域计数共用。
3. **映射**：`MapExternalFundsHandler.Map` 映到对账单或账户时先锁事实身份行（`FactLocks.LockFundsFactApplications`，与核销、撤销同一把），再读事实、插映射行；`advanceCustomerReceipt` 重算这条事实落到的全部作用域（单作用域变多作用域那一笔里原作用域从全额改为按核销，入账余额同事务下降），作用域咨询锁按（法人、账户、币种）键序取（`sortScopes`）；信用状况仍只在账户级映射的那个作用域上重算。映到应付、贷项、金额本体的映射不取任何一把锁，照旧。
4. **核销与撤销**：`applicationScopes` 取这条事实落到的全部作用域并上对账单分配收成的作用域，键序排好；`admitApplicationScopes` 按序锁、只在有对账单分配的作用域上判已确认未结会不会为负（与原先同一格）；撤销取同一组作用域。
5. **读面**：`ports.ExternalFundsFactCatalogueRow` 加 `CustomerScopeCount`、`UnscopedUnappliedMinor`（落在几个作用域上时等于为正的累计未分配，否则 0）；`GET /settlement-funds-applications` 透出 `customerScopes`、`unscopedUnappliedAmount`；管理台收付款核销页资金事实册加一栏「入账余额记入」（`scopePostingText`）；`FundsResult.FactScopePosting` 交回作用域个数与事实种类，`parcel-settlement-register` 的映射、核销、撤销答复在多作用域时说「累计未分配 Y（落在 N 个客户应收作用域上，按核销记入：未分配不计入任何作用域）」，退回说「每个作用域都先扣」；只落到一个作用域时答复不变。
6. **文档**：ADR-0283；ADR-0181 Status 行索引注记与 Links 各加一条前向指针（正文不动）；SA CONTEXT「运营余额、收付款与坏账」加一条规则句，「未分配就是这个差」限在只落到一个作用域的事实上；AT 盘点 `AT-SA-106`、`AT-SA-112` 两行补用例名（不新立 AT 号）。未动 `cmd/parcel-api` 的 endpoints.go、main.go、endpoints_test.go、routes.go，未动装配（新读口由既有适配器实现）。

### 代决

1. **作用域个数的口径**：只数客户应收作用域（对账单、应收账户两格收成的），映到应付、贷项、金额本体的不计；同一作用域下几个目标算一个。
2. **币种与事实不同的作用域也算一个**：它从这条事实上记不到钱，但这条映射说明钱该归谁还没分清，不让另一个作用域因此按全额放行。
3. **退回**：分配那部分照协调方「同一规则」——指向各作用域的未撤销分配从该作用域离开，撤销那一截回到没核销的部分。没核销的那一截取保守方向：在每个作用域都先扣。依据 SA CONTEXT「退回与撤销按反向映射离开」与协调方「钱没分到谁头上之前不放行」：退回是钱离开，各记 0 等于在每个作用域都当钱没走，可用余额偏大，正是本票要堵的那一类。各作用域扣的合起来大于退回金额，是每边的下界。
4. **重算范围**：映射、核销、撤销都重算这条事实落到的全部作用域（核销与撤销另并对账单分配收成的作用域，不靠「分配沿映射走」这一点）。单作用域事实与原先同一组作用域；多作用域事实上一个作用域账已不平时，对它在别的作用域上的核销也一起答账不平（见越权风险点「重算范围变宽」）。
5. **映射锁事实**：只在映到对账单或账户时取，取在读事实与插映射行之前，且不论事实种类（失败付款映不进去，多锁一下无害）；映射答复里的累计未分配因此读的是锁后的链头。
6. **读面形态**：两格总在场，不靠缺席表达 0 个与 1 个作用域；`UnscopedUnappliedMinor` 不并进 `UnappliedMinor`——「未分配」与「未分配且哪个作用域都没记它」续办不同。超核时这一格是 0，超核由 `UnappliedMinor` 为负说出。不新建管理台页、不设待处理册。
7. **CLI 答复只在多作用域时开口**：单作用域答复保持原样，既有用例与操作员读法不变。
8. **存量不回填**：演示种子只把一笔收款映到一个账户（一次性库实测入账余额 20000，与按全额同值）；已落在几个作用域上的事实在下一次映射、核销或撤销时按新口径重算。
9. **判定构成部分停用**：乙把 ADR-0181 决定二 P 的口径与「未分配是 P − W 的差」对多作用域事实收窄，照 docs/adr/README.md 属部分停用，立 ADR-0283。决定一照用、不停用：一个变几个的那一笔映射就是原作用域那次变化的所在；ADR-0182 决定一同理照用。

### 越权风险点（待 SA owner 复核）

1. **退回没核销那一截在每个作用域都先扣**（协调方的字面是「同一规则」，这一截的方向是作者代决）：作用域可用余额偏小、可能挡冻结。要改成各记 0，只改 `CustomerDirectionNets` 里退回那一项，读面与 CLI 说法随之改。
2. **币种不同的作用域也算一个**（作者代决）。
3. **兼映到应付、贷项或金额本体的客户收款仍按全额记入它唯一的客户应收作用域**：核到那几格的部分仍在客户作用域全额里。同类缺口，本票不改；客户收款同时映到供应商或赔付一侧本身与「默认不做应收应付净额抵销」相抵。若要堵，判据扩到「事实还有任何非客户应收映射」即按核销记，另立票。
4. **外部更正不重算**：单作用域全额、多作用域退回没核销那一截都随链头变而更正这一步不推进，归 [code/92](./92-a-funds-fact-correction-below-its-applied-total-is-adopted-silently.md)。
5. **存量停在旧值**：本票之前已落在几个作用域上的事实，那几个作用域在下一次碰到之前停在全额；首发前只有演示租户。
6. **重算范围变宽**：落到几个作用域的事实，其中一个作用域账已不平时，对它在别的作用域上的核销与撤销也一起答账不平；续办与账不平那一格相同。
7. **P 每次重算把本租户全部映射收成作用域**（CTE 物化一次），无索引；与既有扫全租户 jsonb 分配同一处境。
8. **作用域咨询锁的键是哈希**：两个作用域撞同一把锁时键序失效，最坏由库的死锁检测拒掉一笔、答未决重试（`ports.FundsFactApplicationLock` 头注已写明）。

### 锁序论证

资金侧仍只有三类锁，全仓按一个全序取：**事实身份行（`FOR UPDATE`）→ 金额本体行（`FOR UPDATE`，按格、引用）→ 运营余额作用域咨询锁（按法人、账户、币种键序）**；次序写在 `ports.FundsFactApplicationLock` 头注，`Map`、`Apply`、`Reverse` 头注回指。

- `Map`（映到对账单或账户）：事实 → 插映射行 → 这条事实落到的全部作用域（键序）。插映射行时的等待只会撞上同一映射键的另一笔映射；那一笔已过了它自己的事实锁、往后只取作用域锁，而握作用域锁的事务都已过了事实锁、不会回头等事实或映射行，所以接不成环。映到别的目标的映射不取任何锁。
- `Apply`：事实 → 金额本体（`ClaimTargetDemands` 给的序）→ 作用域（键序）。
- `Reverse`：事实 → 作用域（键序）。
- `CorrectFact`：插版本行时经外键对事实身份行取 KEY SHARE，与 `FOR UPDATE` 互斥，算作只取事实这一类、不握作用域锁。
- 接受前控制在另一个锁空间，不重算、不碰这三类。

每条路径取的都是这条全序的前缀或子序列，同类之内按同一键序，没有一条在握后一类时回头取前一类；同一事务里作用域锁再取是重入。两笔映射同一事实到不同作用域、映射与核销同一事实，都在事实身份行上排队；两条事实交叉映到同两个作用域，各握自己的事实锁、按键序取作用域，不互等成环——三种都有真库并发用例（`-count=5`）。

### 测试（含 PG，`TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh`）

- **先红**（main `47c589855` 上只加用例、`FundsResult` 只加空壳）：`TestAReceiptMappedIntoTwoScopesAndAppliedToOneIsNotPostedInTheOther` 报 acct-2 入账 5000、要 0（评审手算例）；`TestMappingAReceiptIntoASecondScopeRepostsBothByApplication` 报记入方式未交回；`TestAReturnMappedIntoTwoScopesIsDeductedInEachUntilApplied` 报核到 acct-2 之后 acct-1 仍扣着退回。
- **新用例**：postgres 包 `customer_receipt_scope_spread_test.go`（手算例；单作用域全额、单→多过渡两作用域同事务重算、尚无核销各记 0、对账单分配与账户分配按分配、撤销回到未分配、资金事实册两格；多作用域退回）；application 包 `TestMappingIntoSeveralScopesLocksTheFactThenEveryScopeInKeyOrder`（替身倒序交回作用域、目标不在键序之首）；HTTP 包 `TestExternalFundsFactQueryShowsTheScopeSpreadAndTheUnscopedUnapplied`；CLI 包 `funds_mapping_scope_spread_vertical_test.go`（手算例走受控登记口与答复文字；两笔映射同一事实到不同作用域并发；映射与核销同一事实并发；两条事实交叉映到同两个作用域、先占住 B 的作用域锁逼出交错）。单作用域不变另由既有 `TestMappingACustomerReceiptAdvancesThePostedBalance`、`TestAReceiptMappedToSeveralTargetsInOneScopeIsPostedOnce` 守着。
- 新用例与并发四条 `-count=5`：退 0。
- **范围**：改动包的反向依赖（`go list -test` 以 SA ports、application、adapters/postgres、adapters/http、`cmd/parcel-settlement-register` 为根）加 `./cmd/parcel-settlement-register/...`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./internal/architecture/...`、`./migrations/...`：退 0。`go build ./...`、`go vet ./...` 退 0，`gofmt -l` 无输出。`go run ./tools/at-coverage-counts -check` 退 0；`go run ./tools/adr-index -check` 只报 README 生成物未重生成（推送方的事），在工作副本上生成一次后 `-check` 退 0、生成物已还原。
- **变异**（改完跑红、拷回备份复原；没有用 `git checkout`）：
  1. 多作用域仍按全额（P 的判据 `scopes = 1` 改 `>= 1`）→ postgres 三条、CLI 两条红；
  2. 过渡时只重算新作用域 → postgres、CLI 共四条红；
  3. 读面漏显：不填 `UnscopedUnappliedMinor` / 作用域计数截成 1 → postgres 两条红；
  4. 作用域锁键序不定：`sortScopes` 不排序 → application 两条红；先锁映射目标 → application 一条红、CLI 交叉映射撞死锁红；
  5. 映射不锁事实 → CLI 两条并发用例红（十秒内没在锁上排队）、application 一条红；
  6. 退回没核销那一截不先扣 → postgres 退回用例红；
  7. 核销只重算对账单分配的作用域 → postgres 两条红。
- **种子**：灌一次性库 `c94seed` 退 0；`SYN-SETTLEMENT-ACCOUNT-01` 入账余额 20000（唯一一笔收款、单作用域、全额，与改前同值）；用后 `DROP DATABASE … WITH (FORCE)`。
- **变基后重跑**：变基到 origin/main `4bb6c2fa0`（其间 main 只在本票范围内动了 AT 盘点，无冲突），范围用例、新用例与并发四条 `-count=5`、`go build ./...`、`go vet ./...`、`at-coverage-counts -check` 重跑均退 0。
- **管理台三道门**（Node 22，软链主树 `node_modules`、用后删）：`tsc -b --noEmit`、`scripts/run-tests.mjs`（667 过，含新 `src/pages/settlement/presentation.test.ts`）、`vite build` 退 0。

### 新票

无（越权风险点「兼映到应付、贷项或金额本体」若要堵，另立票由协调方定）。
