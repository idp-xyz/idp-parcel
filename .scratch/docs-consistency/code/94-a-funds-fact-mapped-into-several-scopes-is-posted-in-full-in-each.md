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

1. **P 的记法**（修订轮改为统一公式，见文末「评审记录与修订轮」N4）：`CustomerReceiptPostings.CustomerDirectionNets` 的 P 逐条事实记一次，每个作用域取这条事实在它身上可能的最小记入——收款记 d（指向本作用域的未撤销分配，分配沿的映射落在本作用域上，对账单分配与账户分配都算），只落到一个客户应收作用域时再加 r（事实金额减全部未撤销核销，不低于 0）；退回扣 d + r，不论落到几个作用域；都不超过事实金额。只有客户映射、只落到一个作用域时即全额。W 与已确认未结不变。
2. **作用域口径收成一处**：`mappingReceivableScopesSQL`（对账单映射经对账单落到同币种的应收账户，账户映射落到应收账户本身）供 P、新读口 `ports.CustomerReceiptBooks.FactCustomerScopes`、资金事实册的作用域计数共用。
3. **映射**：`MapExternalFundsHandler.Map` 映到对账单或账户时先锁事实身份行（`FactLocks.LockFundsFactApplications`，与核销、撤销同一把），再读事实、读这条映射之前事实已落到的作用域、插映射行；`advanceCustomerReceipt` 重算这一笔改了 P 的作用域（修订轮收窄：目标；一条收款由一个作用域变两个时连原作用域，原作用域入账余额同事务下降），作用域咨询锁按（法人、账户、币种）键序取（`sortScopes`）；信用状况只在账户级映射的那个作用域上重算。映到应付、贷项、金额本体的映射不取任何一把锁。
4. **核销与撤销**（修订轮收窄）：`applicationScopes` 取对账单分配与账户分配所指的作用域；事实只落到一个作用域时并上那一个；事实是退回时并上全部；键序排好。`admitApplicationScopes` 按序锁、只在有对账单分配的作用域上判已确认未结会不会为负；撤销取同一组作用域。`recomputeScope` 的账不平带上作用域的法人、账户、币种。
5. **读面**：`ports.ExternalFundsFactCatalogueRow` 加 `CustomerScopeCount`、`UnscopedUnappliedMinor`（落在几个作用域上时等于为正的累计未分配，否则 0）；`GET /settlement-funds-applications` 透出 `customerScopes`、`unscopedUnappliedAmount`；管理台收付款核销页资金事实册加一栏「入账余额记入」（`scopePostingText`）；`FundsResult.FactScopePosting` 交回作用域个数与事实种类，`parcel-settlement-register` 的映射、核销、撤销答复在多作用域时说「累计未分配 Y（落在 N 个客户应收作用域上，按核销记入：未分配不计入任何作用域）」，退回说「每个作用域都先扣」；只落到一个作用域时答复不变。
6. **文档**：ADR-0283；ADR-0181 Status 行索引注记与 Links 各加一条前向指针（正文不动）；SA CONTEXT「运营余额、收付款与坏账」加一条规则句，「未分配就是这个差」限在只落到一个作用域的事实上；AT 盘点 `AT-SA-106`、`AT-SA-112` 两行补用例名（不新立 AT 号）。未动 `cmd/parcel-api` 的 endpoints.go、main.go、endpoints_test.go、routes.go，未动装配（新读口由既有适配器实现）。

### 代决

1. **作用域个数的口径**：只数客户应收作用域（对账单、应收账户两格收成的），映到应付、贷项、金额本体的不计；同一作用域下几个目标算一个。
2. **币种与事实不同的作用域也算一个**：它从这条事实上记不到钱，但这条映射说明钱该归谁还没分清，不让另一个作用域因此按全额放行。
3. **退回**：分配那部分照协调方「同一规则」——指向各作用域的未撤销分配从该作用域离开，撤销那一截回到没核销的部分。没核销的那一截取保守方向：不论落到几个作用域，在每个作用域都先扣（修订轮统一公式同此）。依据 SA CONTEXT「退回与撤销按反向映射离开」与协调方「钱没分到谁头上之前不放行」：退回是钱离开，各记 0 等于在每个作用域都当钱没走，可用余额偏大，正是本票要堵的那一类。各作用域扣的合起来大于退回金额，是每边的下界。
4. **重算范围**（修订轮按协调方裁定改）：只重算这一笔改了 P 或 W 的作用域，集合见文末「最终重算集合」。一个作用域账已不平时，只有碰到它的那一笔答账不平，答复带作用域身份。
5. **映射锁事实**：只在映到对账单或账户时取，取在读事实与插映射行之前，且不论事实种类（失败付款映不进去，多锁一下无害）；映射答复里的累计未分配因此读的是锁后的链头。
6. **读面形态**：两格总在场，不靠缺席表达 0 个与 1 个作用域；`UnscopedUnappliedMinor` 不并进 `UnappliedMinor`——「未分配」与「未分配且哪个作用域都没记它」续办不同。超核时这一格是 0，超核由 `UnappliedMinor` 为负说出。不新建管理台页、不设待处理册。
7. **CLI 答复只在多作用域时开口**：单作用域答复保持原样，既有用例与操作员读法不变。
8. **存量不回填**：演示种子只把一笔收款映到一个账户（一次性库实测入账余额 20000，与按全额同值）；已落在几个作用域上的事实在下一次映射、核销或撤销时按新口径重算。
9. **判定构成部分停用**：乙把 ADR-0181 决定二 P 的口径与「未分配是 P − W 的差」对多作用域事实收窄，照 docs/adr/README.md 属部分停用，立 ADR-0283。决定一照用、不停用：一个变几个的那一笔映射就是原作用域那次变化的所在；ADR-0182 决定一同理照用。

### 越权风险点（待 SA owner 复核）

1. **退回没核销那一截在每个作用域都先扣**（协调方的字面是「同一规则」，这一截的方向是作者代决）：作用域可用余额偏小、可能挡冻结。要改成各记 0，只改 `CustomerDirectionNets` 里退回那一项，读面与 CLI 说法随之改。
2. **币种不同的作用域也算一个**（作者代决）。
3. **唯一客户作用域的推定**（修订轮由「兼映到非客户目标仍按全额」改：那一缺口按协调方裁定并入本票修了）：只落到一个客户作用域、又兼映到应付、贷项或金额本体的收款，核过去之前没核销的部分按推定记在客户作用域上，那段时间不是严格下界。要严格下界，n 为 1 的推定再加「事实没有非客户映射」即可。
4. **外部更正不重算**：单作用域全额、多作用域退回没核销那一截都随链头变而更正这一步不推进，归 [code/92](./92-a-funds-fact-correction-below-its-applied-total-is-adopted-silently.md)。
5. **存量停在旧值**（修订轮改准）：本票之前已落在几个作用域上、或已核到非客户目标的事实，它的作用域要等某一笔改了那个作用域的 P 或 W 才按新口径重算；重算集合收窄之后不再「碰这条事实一下就修好它的全部作用域」。首发前只有演示租户（种子为单作用域、只有客户映射，值不变），可以接受。
6. ~~重算范围变宽~~：修订轮按 N3 收窄重算集合后消失。
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
- **新用例**：postgres 包 `customer_receipt_scope_spread_test.go`（手算例；单作用域全额、单→多过渡两作用域同事务重算、尚无核销各记 0、对账单分配与账户分配按分配、撤销回到未分配、资金事实册两格；多作用域退回）；application 包 `TestMappingIntoSeveralScopesLocksTheFactThenEveryScopeInKeyOrder`（替身倒序交回作用域、目标不在键序之首；修订轮由 `TestMappingLocksTheFactThenOnlyTheScopesItMovesInKeyOrder` 取代）；HTTP 包 `TestExternalFundsFactQueryShowsTheScopeSpreadAndTheUnscopedUnapplied`；CLI 包 `funds_mapping_scope_spread_vertical_test.go`（手算例走受控登记口与答复文字；两笔映射同一事实到不同作用域并发；映射与核销同一事实并发；两条事实交叉映到同两个作用域、先占住 B 的作用域锁逼出交错）。单作用域不变另由既有 `TestMappingACustomerReceiptAdvancesThePostedBalance`、`TestAReceiptMappedToSeveralTargetsInOneScopeIsPostedOnce` 守着。
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

无（上一轮记的「兼映到应付、贷项或金额本体仍按全额」修订轮按协调方裁定并入本票修了，见文末 N4）。

## 评审记录与修订轮（2026-10-26，协调方隔离评审：无阻断；作者修订轮，分支 claude-code94）

修订轮开工前变基到 origin/main `803b97e5f`，收尾再变基到 `1ed4f7031`，两次都无冲突；其间 main 改了 SA 的评价请求一支并占用 SA 迁移 0047，不碰资金映射与 `CustomerReceiptPostings`，本票不用迁移。修订轮在同一分支上加新提交，不改写旧提交。

- **N1（照做）**：ADR-0181 决定二「已核销分配同时等额离开已确认未结。可用余额在核销前后不变，同一笔钱不会既留在入账里又从应收里拿掉。」在多作用域事实上不成立（核 3000 到 stmt-1，A 的 P 与 W 同涨 3000、已确认未结降 3000，可用余额升 3000），只落到一个作用域的收款核到非客户目标上时也不成立（可用余额降）。ADR-0283「对已接受记录的作用」逐字引这一句列为收窄，0181 Status 注记与 Links 改为三句；SA CONTEXT 那句限在「只落到一个作用域的事实核到它自己的客户应收上」，规则句补「核销把钱分进所指的作用域，那里的可用余额随之上升」；ADR-0283 Consequences 补「落到几个作用域时，冻结额度在核销时才放开」。
- **N2（照做）**：postgres 包 `TestAScopeInAnotherCurrencyStillCountsAsAScope`——CNY 收款映 stmt-1 再映 USD 应收账户，acct-1 归 0、USD 账户 0，资金事实册 `CustomerScopeCount=2`、未计入任何作用域 5000。两处变异（P 只数同币种、资金事实册只数同币种）各被它杀。
- **N3（两条都做）**：
  1. `recomputeScope` 把 `ErrNegativeConfirmedUnsettled` 包上「作用域 法人 X、账户 Y、币种 Z」，`parcel-settlement-register` 答复照出（execute 那一格本就打 `%v`）。
  2. 重算集合收窄，与 N4 合起来定为下面「最终重算集合」；写进代决 4、`ports.FundsFactApplicationLock` 头注、`advanceCustomerReceipt`、`applicationScopes` 头注与 ADR-0283 决定三。CLI 包 `TestAnUnbalancedScopeOnlyRefusesTheStepsThatTouchIt` 钉住评审的回退场景：A 的已确认应收被价格纠错贷记 2000 压到已核销之下后，F 的 1000 核到 B 的账户照常落下（B 入账 1000）；再映一笔收款到 A 的对账单，答账不平退 2，答复点名 A 的法人、账户、币种。越权风险点 5 改准、6 消失。
- **N4（并入本票）**：统一公式 收款 P_s = d_s + [n=1]·r、退回 P_s = −(d_s + r)，r = max(金额 − 全部未撤销核销, 0)。作者另加一层「一个作用域从一条事实上记到或扣掉的不超过事实金额」（代决 10）。ADR-0283 决定二改写，写入「每个作用域取这条事实在它身上可能的最小记入（下界）」与唯一推定。postgres 包 `TestAReceiptAppliedToANonCustomerTargetLeavesItsOnlyCustomerScope`：5000 映 stmt-1 与 payable-x，A 5000；核 2000 到应付 A 3000；再核 3000 A 0；撤销后者 A 3000；资金事实册 1 个作用域、未计入任何作用域 0。
- **信息项（照做）**：ADR-0283 决定二「全部核销后各作用域加起来就是全额」加「核到客户应收作用域上」限定；ADR 越权风险点补哈希碰撞；CLI `mappingSubjectOf` 与 `applicationSubjectOf` 收成一个 `factBalanceOf`，超核两类答复都说「累计超核 X」（CLI 包 `TestAMappingOfAnOverAppliedFactSaysOverApplied`）；CLI 与管理台多作用域未分配补续办（收款「核到哪个作用域那里才记」，退回「核到钱离开的那个账户上就只扣那一边」）；管理台单作用域那格改为「记入 1 个作用域（核到别处的除外）」；注释里「照旧」「全额」这类前后对比改成现行规则句。
- **可选（按锁键本身排序取作用域锁）**：未做，进越权风险点 8 与 ADR-0283 越权风险点 7。理由：碰撞概率按 32 位哈希在一个租户的作用域之间可以忽略，碰上的后果是库的死锁检测拒掉一笔、答未决重试，不错账；按锁键排序要把取锁搬进适配器、一条语句取几把，而那条次序只有造出哈希碰撞才测得到，测不到的次序不如不立。

### 修订轮新增代决

10. **一个作用域从一条事实上记到或扣掉的不超过事实金额**：超核（外部更正把金额改到已核销合计以下）时 d 可能大于金额，不封顶会让作用域记得比钱还多；封顶之后只有客户映射、只落到一个作用域的事实在超核时也与改前的全额同值。
11. **映射在一变二时连原作用域只对收款**：退回的 P 与 n 无关，退回多落一个作用域时原作用域的 P 不变，不碰。
12. **核销与撤销所指的作用域按分配目标收**：对账单分配用 `CustomerStatementScope`，账户分配用 `ReceivableAccountScope`，与它沿的那条映射收成作用域同一组条件（目标与映射一致由领域守着）。

### 最终重算集合

| 一笔 | 重算的客户应收作用域 |
|---|---|
| 映到对账单或账户 | 目标所在的作用域；一条收款由这一笔从落到一个作用域变成两个时，连原来那个 |
| 映到应付、贷项或金额本体 | 无（不改任何 P，不取锁） |
| 核销、撤销 | 对账单分配与账户分配所指的作用域；事实只落到一个客户应收作用域时那一个；事实是退回时它落到的全部作用域 |

落到几个作用域的收款核到非客户目标不改任何作用域的 P，不碰。一个作用域的账已不平时，只有碰到它的那一笔答账不平，答复带它的法人、账户、币种。

### 锁序论证（修订后）

全序不变：**事实身份行（`FOR UPDATE`）→ 金额本体行（`FOR UPDATE`，按格、引用）→ 运营余额作用域咨询锁（按法人、账户、币种键序）**。重算集合变小只是每条路径取的作用域少了，取的仍是全序的前缀或子序列：

- `Map`（映到对账单或账户）：事实 → 读事实已落到的作用域 → 插映射行 → 目标（一变二的收款另加原作用域，两把按键序）。插映射行时的等待论证同前。
- `Map`（映到别的目标）：不取锁。
- `Apply`：事实 → 金额本体 → 上表那几个作用域（键序）。
- `Reverse`：事实 → 上表那几个作用域（键序）。
- `CorrectFact`：只有插版本行时经外键取的 KEY SHARE。

同类之内按同一键序、没有一条在握后一类时回头取前一类，所以不互等成环。哈希碰撞那一格见越权风险点 8。

### 修订轮测试（含 PG，`TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh`；范围、`-count=5`、种子在收尾变基后的 tip 上重跑）

- 新用例：postgres 包 `TestAScopeInAnotherCurrencyStillCountsAsAScope`（N2）、`TestAReceiptAppliedToANonCustomerTargetLeavesItsOnlyCustomerScope`（N4）；application 包 `TestMappingLocksTheFactThenOnlyTheScopesItMovesInKeyOrder`（取代上一轮的全作用域锁序用例：一变二锁事实、原作用域、目标，键序与谁是目标无关；二变三只锁事实与目标）；CLI 包 `TestAnUnbalancedScopeOnlyRefusesTheStepsThatTouchIt`（N3）、`TestAMappingOfAnOverAppliedFactSaysOverApplied`（超核说法）。
- 范围：改动包的反向依赖加 `./cmd/parcel-settlement-register/...`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./internal/architecture/...`、`./migrations/...`：退 0。新用例与并发用例 `-count=5`：退 0。`go build ./...`、`go vet ./...` 退 0，`gofmt -l` 无输出；`at-coverage-counts -check` 退 0；`adr-index` 在工作副本上生成一次后 `-check` 退 0，生成物已还原。
- 变异（改完跑红、拷回备份复原；没有用 `git checkout`），全部被杀：
  1. 多作用域收款仍记没核销的部分（`[n=1]` 改 `[n>=1]`）→ postgres 三条、CLI 四条红；
  2. 多作用域退回没核销的部分不先扣 → postgres 退回用例红；
  3. `[n=1]·r` 项去掉 → postgres 八条红（单作用域既有用例一并红）；
  4. 单作用域回到链头全额 → `TestAReceiptAppliedToANonCustomerTargetLeavesItsOnlyCustomerScope` 红；
  5. 一变二时只重算新作用域 → postgres、CLI、application 共七条红；
  6. 作用域锁不排序 → application 两条红；先锁映射目标 → application 一条红、CLI 交叉映射撞死锁红；
  7. 映射不锁事实 → CLI 两条并发用例红（十秒内没在锁上排队）、application 一条红；
  8. 核销不碰账户分配所指的作用域 → postgres、CLI 各一条红；
  9. 核销与撤销重算事实落到的全部作用域（上一轮的范围）→ `TestAnUnbalancedScopeOnlyRefusesTheStepsThatTouchIt` 红；
  10. 账不平不带作用域身份 → 同一条红；
  11. 单作用域收款核到非客户目标不重算那个作用域 → N4 用例红；
  12. P 只数同币种作用域、资金事实册只数同币种作用域 → N2 用例各红；
  13. 资金事实册不填未计入任何作用域的未分配 → postgres 三条红；
  14. 映射答复超核说成负的未分配 → `TestAMappingOfAnOverAppliedFactSaysOverApplied` 红。
- 种子：灌一次性库 `c94seed` 退 0，`SYN-SETTLEMENT-ACCOUNT-01` 入账余额 20000（不变），用后删库。
- 管理台三道门（Node 22，软链主树 `node_modules`、用后删）：`tsc -b --noEmit`、`scripts/run-tests.mjs`（672 过）、`vite build` 退 0。
