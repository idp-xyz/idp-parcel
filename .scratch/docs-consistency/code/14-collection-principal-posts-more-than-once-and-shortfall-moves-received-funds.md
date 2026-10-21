# 14 CR 同一笔本金可以多次「自外部来源入账」，`短款`会挪走已到账的本金

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code14。R1（上线前必修）：碰钱且破坏数据完整性。CR 受托保管的是客户的代收本金：一包裹的同一笔钱经收件人付款、渠道代收报告、渠道回款通知三层各自全额入账，`应付客户`会超过真实到账；短款又从已到账的`待清分`里挪走差额，客户少拿钱。今天记账只走受控 CLI（`cmd/parcel-collection-register`），但守恒不变量一破就无法事后分辨哪一笔是重的，没有可靠绕法。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无——2026-10-21 文档自洽 09 已裁（[ADR-0271](../../../docs/adr/0271-collection-principal-enters-the-ledger-only-on-the-channel-collection-report-and-shortfall-comes-only-from-in-transit.md)），裁向见文末「裁向（文档自洽 09，2026-10-21）」。此前：docs-consistency/09（SA+CR 修订）先裁 [audit/sa-cr.md](../audit/sa-cr.md) H3、H5：四层事实里哪一层负责入账、其余层只做移位或旁证；真实到账遇已由渠道层入账的本金走入账还是走移位；`短款`只记在差异事项上、不占资金位置，还是只允许从「已报告未到账」的`渠道在途`落入。
出处：[audit/sa-cr.md](../audit/sa-cr.md) H3、H5（两条在审计里归 B，建议修法写明「裁定之后另立代码票」；同一上下文、同一根因——记账只核形状不核依据的入账次数与资金来源——合成本票。审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，CR 代码在此期间未改）。

## 现象

1. **重复入账**（H3）：入账只按记账标识判重放；凭同一条代收事实换一个记账标识可以再入一次。三层事实的入账位置都是`渠道在途`，各层各自全额入账，同一笔钱记两三遍。运营企业真实到账既能入账到`待清分`，又能再凭它把`渠道在途`移到`待清分`，那一笔移位的金额不受事实金额约束。
2. **短款挪钱**（H5）：差额那部分钱从没进账，短款记账却不限来源位置；测试里指令 12000、到账 10000，全额入`待清分`后再移 2000 到`短款`，只剩 8000 能进`应付客户`。Lifecycles 也没有任何一条从`短款`出去。

## 证据

- 文档：
  - CR CONTEXT Rules：「资金只能自外部来源入账一次，且入账必须以一笔已接受的代收事实为依据」；「同一笔代收事实不得重复入账」。
  - Language「分配守恒」：「全部非外部资金位置的余额之和恒等于自外部来源入账的金额之和」。
  - Language「资金位置」：「`短款`（实收少于指令的差额已确认落在此处）」；Lifecycles：「`待清分` 或 `应付客户` → `短款`：差异事项已登记且短款结论已确认」。（这两句与守恒相抵，是 H5 要裁的那一处。）
- 代码：
  - `internal/collectionremittance/application/post_subledger.go` 的 `PostSubledger` 按记账标识判重放；`factPostingShapeHolds` 只核去向等于事实的 `IntakePosition()`、金额等于事实金额（外部来源那一支），或只核层次与起讫位置（渠道在途 → 待清分那一支，不核金额上限）；不查这条事实是否已入过账。
  - 同文件 `discrepancyPostingShapeHolds`：去向是事项落点即可，不限来源位置。
  - `migrations/collection_remittance/0001_collection_subledger_and_remittance.sql`：`subledger_posting` 只有主键 `(tenant_id, posting_ref)`，没有按（依据种类，依据引用）的入账唯一约束。
  - `application/handler_test.go` 的 `TestADiscrepancySettlesOnlyThroughItsOwnPosting` 一带即上面那组短款数字。

## 三类

机制：一条事实至多入账一次、移位金额以事实为限、短款的来源位置受限。产品策略：入账层与移位层的划分（待 09 裁）。租户取值：无。

## 规模

M：唯一约束（迁移另取号）与编排判重；移位金额核对；短款记账核来源位置；改写那组短款测试；存量只有演示租户，`--reset` 重灌即可。

## 最小的诚实修法（按 09 的裁定二选一落细节，下列三条不随裁向变）

1. 外部来源 → 任一位置的入账，按（依据种类，依据引用）唯一：第二次换标识入账答`已入账`并回指第一笔，不记账。库上加同一唯一约束，编排判重与约束同一口径。
2. 凭事实的移位，累计金额不得超过该事实金额。
3. 短款记账的来源位置按 09 的裁定限死；不满足的答依据不符，不记账。

## 要裁的

全部在 Blocked by 那一裁里；裁定落在 09 新立的 ADR，本票只照它实现。

## 裁向（文档自洽 09，2026-10-21；评审轮改为按指令判）

[ADR-0271](../../../docs/adr/0271-collection-principal-enters-the-ledger-only-on-the-channel-collection-report-and-shortfall-comes-only-from-in-transit.md) 裁定，本票照它实现；「最小的诚实修法」里「第二次换标识入账答`已入账`并回指第一笔」那半句按本节「判重」一项改为答内容冲突：

- **入账**：只有渠道代收报告（`CHANNEL_COLLECTION_REPORT`）能自外部来源入账，去向`渠道在途`、金额等于事实金额。`CollectionFact.IntakePosition()` 只对这一来源层级有位置；收件人付款与渠道回款通知凭它记任何一笔都答`NOT_ACCEPTED`。
- **移位**：运营企业真实到账（`OPERATOR_BANK_CREDIT`）只收`渠道在途` → `待清分`，金额等于事实金额；不再能自外部来源入账。余额守卫今天按分户账键判（`SubledgerBalance.Admit` 只看这本账的`渠道在途`余额），同账别的指令的在途款也算得进来；要另加按指令的上限：移位上限 = 该指令已入账报告 − 该指令自`渠道在途`移出的金额（到账移位与短款落账，减去冲正回`渠道在途`的），不够答`UNDERFUNDED`。
- **判重**：凭代收事实的记账按（依据种类、依据引用）唯一。同一记账标识重放仍答`EXISTING`；换一个记账标识再凭同一条事实记账答`CONTENT_CONFLICT`，不记账。编排判重先于余额守卫，与库上的唯一约束（迁移另取号）同一口径。
- **差额来源**：凭短款事项的记账只收`渠道在途` → `短款`，凭溢款事项的落账只收`待清分` → `溢款`（`溢款` → `应付客户` 的归属确认照旧），两者金额都等于差异事项金额；不满足答`NOT_ACCEPTED`。差异事项的金额按指令核：短款 = 该指令已入账报告 − 该指令自`渠道在途`移出的金额 − 经 settlement-accounting 认可的协议抵扣；溢款 = 该指令移入`待清分`的到账 − 指令金额 − 该指令已落入`溢款`的金额。该指令有停在余额不足的到账时不受理短款事项的落账。协议抵扣的记账形未裁，本票不做，认可的抵扣额留在`渠道在途`，见 [code/47](./47-agreed-cod-offset-has-no-posting-form.md)。领域构造门与迁移 CHECK 各补一份。差异事项登记不因没有可落的余额而拒收：指令金额与报告之差只登事项、不记账。
- **测试**：`application/handler_test.go` 里凭真实到账自外部来源入账的各用例（短款、冲正等）改成渠道报告先入`渠道在途`、到账再移位；短款用例改成渠道报告入`渠道在途`、到账移位、差额自`渠道在途`落入`短款`；补收件人付款、渠道代收报告、渠道回款通知各自入账，同一事实换标识，到账多于该指令报告，同账两条指令一单的到账借不到另一单的在途款，溢款自`渠道在途`各一例。演示种子已是这一形（一条指令），不改。
- **注释**：`IntakePosition`、`factPostingShapeHolds`、`discrepancyPostingShapeHolds` 与 `DiscrepancyItem.SettlementPosition` 的注释随之改；[code/45](./45-sa-cr-comments-carry-counts-that-no-longer-hold.md) 里 `collectionremittance` 包的那几条注释可一并改。

建议分级不变（R1）。

## 完成记录（2026-10-21，分支 claude-code14，基于 main `22d66b9c8`）

### 做了什么

- 领域（`internal/collectionremittance/domain`）：
  - `CollectionFact.IntakePosition()` 只对渠道代收报告交`渠道在途`，其余三层交 `FundPositionUnknown`；来源层级与 `IntakePosition` 的注释随之改。
  - `DiscrepancyItem.SettlementSource()` 新增：短款自`渠道在途`、溢款自`待清分`；`SettlementPosition` 注释随之改。
  - `RecordSubledgerPosting` 补构造门：入账只入`渠道在途`；凭代收事实只有入账与`渠道在途` → `待清分`两种形；`短款`只从`渠道在途`、`溢款`只从`待清分`落入。
  - 新增 `InstructionPrincipal`（`DeriveInstructionPrincipal`）：三式只在这里定义——`TransferCeiling`（报告 − 自在途移出，冲正回在途的算回）、`ShortfallCeiling`（同上，抵扣无记账形只得上限）、`SurplusDue`（移入待清分的到账 − 指令金额 − 已落溢款）；`PostingOf` 答事实已支撑哪一笔，`HasUnmovedArrival` 答有无未移位到账，`Admit` 判一笔记账过不过按指令的上限。
- 编排（`application.PostSubledger`）：重放 → 解依据与形状 → 凭代收事实按依据判重（换标识答 `CONTENT_CONFLICT`）→ 读回账面 → 按指令的上限 → 按账的余额守卫 → 落账。收件人付款、渠道回款通知与真实到账凭它们入账、旁证凭它们移位都答 `NOT_ACCEPTED`。自`渠道在途`移出的冲正沿冲正链找回指令，同样按指令判。撞上库上唯一约束（并发）照旧折成冲突。
- 端口与真库读口：`CollectionView.LoadInstructionFacts`、`SubledgerView.LoadInstructionPostings`（凭该指令的事实、差异事项、清分，沿冲正链收冲正；一趟递归 CTE，只扫该指令的分户账）。
- 迁移 `migrations/collection_remittance/0002_principal_enters_once_and_variances_have_fixed_sources.sql`：部分唯一索引 `subledger_posting_one_per_collection_fact`（`(tenant_id, basis_ref) WHERE basis_kind = 'COLLECTION_FACT'`），CHECK `subledger_posting_intake_lands_in_transit`、`subledger_posting_collection_fact_shapes`、`subledger_posting_shortfall_from_in_transit`、`subledger_posting_surplus_from_awaiting_allocation`，索引 `collection_fact_by_instruction`。只加约束不改行。
- 受控 CLI（`cmd/parcel-collection-register`）：余额不足的提示与退出码 4 的头注补上「按指令算的上限」；装配不变（读口是既有两个适配器上的新方法）。
- 测试：`handler_test.go` 里凭真实到账自外部来源入账的各用例改成报告入在途、到账再移位；短款用例改成自在途落入、待清分 10000 不动。新增领域、编排、真库与生产装配用例：三层各记一次只入报告那一笔；换标识重记答冲突（入账与移位各一，判重先于余额守卫）；同账 X、Y 两指令 X 落短款后迟到款借不到 Y 的在途款、冲正短款后才移得动、Y 到账多于报告整笔停住；场景十七短款半边（报告 100、到账 96、短款 1 落入、5 越过上限答余额不足，3 留在途）；有未移位到账时短款停住、补报告移位后才落；溢款只从待清分落一次、自在途答未受理、第二项再落答余额不足；库上唯一约束与 CHECK 各撞一次；按指令读口沿冲正链追溯、不收同账别的指令与批次汇付。新编排用例在旧代码上跑过，红在预期处。
- 文档：UC-CR-001 指向本票的缺口注改为已补，结果表与主流程补「按依据判重、按指令判上限」，`AT-CR-006`、`007`、`008`、`016` 去掉「未实现」注。CR CONTEXT 与 ADR 正文未改。

### 代决与越权风险点

用户授权「你作为业务和系统专家自决……解决所有的问题」，下列为本票代决，待 CR owner 复核：

1. **给未定义词「停在余额不足的到账」补操作定义：该指令已登记、尚未移位的真实到账**（CR owner）。ADR-0271 没有定义这个词，系统也不记失败的尝试，读不出哪一笔「停在」余额不足；而一笔够移、只是还没过账的到账同样会让先落的短款变错（先落短款再移到账与反过来终局不同）。定义落在 CR CONTEXT「清分、汇付与差异」短款那条之后，UC-CR-001 引用它；这是补操作定义，不算改写 ADR。越权点：运营忘了过账一笔到账时，短款也落不下，要先过那一笔。若 owner 要更窄的口径，改 CONTEXT 那一句与 `InstructionPrincipal.Admit` 一处。
2. **有未移位到账时的短款、短款越过上限、溢款多于按指令算的溢款，都答 `UNDERFUNDED`**（CR owner）。处置都是等上游那一笔（补报告、移到账、冲正短款）再重跑同一命令，按 ADR-0029 归余额不足；没有新立结果格。越权点：账面那一格的余额可能是够的，「余额不足」字面上不贴切。
3. **溢款金额少于按指令算的溢款答 `NOT_ACCEPTED`**（CR owner）。多收已经全在`待清分`里，事项报少了，等也等不对；要登一项金额对的事项。越权点：分两次登溢款事项（先登一部分）的做法被拒，须一次登足当下的多收。
4. **短款只核「不越过」报告 − 移出，不核相等**（CR owner，知会 SA owner）。协议抵扣没有记账形（code/47），差额里哪部分是认可的抵扣账上读不出来；照票面只保证不越过上限。场景十七的抵扣那一格（3 的了结）待 code/47。
5. **自`渠道在途`移出的冲正也按指令判上限**（CR owner）。票面只点了到账与短款；冲正一笔冲正（把移回在途的到账再移出去）不过这一层就能借走同账别的指令的在途款，故一并收进。
6. **凭代收事实的记账形在库上也封死**（机制）。除票面点名的入账与差额来源外，另加 CHECK：凭代收事实只有入账与`渠道在途` → `待清分`两种形，领域构造门同一份。
7. **记账读回账面之前取分户账事务锁**（机制，评审轮 N1 加）。环境事务是 READ COMMITTED，按指令的上限与按账的余额守卫都是读后写；`PostSubledger` 在解依据之后经写口 `LockLedger` 取（租户，分户账四维）的 `pg_advisory_xact_lock`（命名空间 `collection_remittance.subledger_posting.ledger`），锁与读回、落账在同一笔环境事务、同一条连接上，两笔并发记账因此排先后。越权点：同一本账上的记账全部串行，账上记账并发高时会排队；绕过编排的写入不取这把锁，凭同一条事实的那一类由唯一约束兜住、编排折成内容冲突，其余仍只靠 CHECK 与读口的派生核。
8. **存量按指令为负不报错**（机制）。按旧口径落下、同账借用过别的指令在途款的账，派生时名下在途可为负，判下一笔时什么也移不出去，不把这条指令此后的每次记账折成未决。

### AT

`.scratch/at-coverage-inventory.md`：`AT-CR-006`、`007`、`008`、`016`、`017`（短款半边，抵扣半边归 code/47）自④、`AT-CR-015` 自②移入①；总览计数未动，随进 main 重算。

### 验证

- `go build ./...`、`go vet`（CR 各包、`cmd/parcel-collection-register`、`cmd/parcel-api`）、`gofmt -l`：干净。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/collectionremittance/... ./cmd/parcel-api/... ./cmd/parcel-collection-register/... ./internal/architecture/... ./migrations/... ./internal/platform/migrate/...`：ok（导入 CR 的 cmd 包按 `go list -deps -test` 算得 `cmd/parcel-api`、`cmd/parcel-collection-register`）。
- 新用例 `-count=3` 同一脚本复跑：ok。
- 一次性库 `code14seed` 灌演示种子（含迁移 0002）：CR 一节全部 `REGISTERED`/`HANDED_OVER`，种子灌入完成；另在事务里先撤掉 0002 的索引与约束、对已落的存量记账（报告 150000 入在途、到账 140000 移位、短款 10000 自在途落入、清分 140000）重施 0002 全文，成功后回滚；用完 `DROP DATABASE code14seed WITH (FORCE)`。

### 未决

- 协议抵扣的记账形：code/47。真实到账凭采用引用登记：code/54。更正回指与差异事项结论格：code/15。
- code/45 里 `collectionremittance` 包那几条注释没有一并改（例如 `rebuildPosting` 头注的依据门数）。

## 评审记录

### 第 1 轮（隔离评审，2026-10-21）

无阻断；三式、四组金额、迁移与旧实现上的红测都核过。协调方裁定与建议，均在分支 claude-code14 上修完：

- **N1（本票就修）**：并发两路记账可各自判够而透支，「单人操作」只是操作纪律。`SubledgerRegistry` 加写口 `LockLedger`（`RequireExecutor` 上 `pg_advisory_xact_lock`，键为租户、客户、责任法人、币种、渠道），`PostSubledger` 在 `resolveBasisForPosting` 之后、`loadInstructionPrincipal` 之前调它，锁与写入同一笔环境事务、不另取连接。真库屏障用例 `cmd/parcel-collection-register/concurrency_test.go`：`TestConcurrentArrivalsOfOneInstructionPostOnlyOne`（屏障连接占锁，看见两路都等在咨询锁上才放；一路落账、一路 `UNDERFUNDED`，渠道在途不透支；去掉取锁那一行时红）、`TestAFactPostedConcurrentlyOutsideTheLockAnswersConflict`（旁路写入先插未提交行，编排落账等在唯一约束上，提交后答 `CONTENT_CONFLICT`）。代决 7 改写为已加锁。
- **N2**：CR CONTEXT「清分、汇付与差异」短款那条后补「停在余额不足的到账，指该指令已登记、尚未移位的真实到账」；UC-CR-001 结果表改为引用它。代决 1 改记为给未定义词补操作定义。
- **N3**：`350-collection.sh` 头注按实测改写：一次性库灌完后原样重放本节每行答已在册（退出码 0）；换标识重跑入账、到账移位答内容冲突（2），清分、短款答余额不足（4）。「第四条依据门」改写为规则原文。
- **N4**：`ShortfallCeiling` 注释按内容指名越权风险点；场景十七用例头注去掉「本票」；ports 包头注断行理顺。
- **N5**：编排层 `TestASurplusLandsOnlyFromReceivedFundsAndOnlyOnce` 补溢款事项报少答 `NOT_ACCEPTED`；UC-CR-001 结果表「未受理」一行补这半句。

AT：`AT-CR-008` 另多一处并发半边点名（`concurrency_test.go::TestAFactPostedConcurrentlyOutsideTheLockAnswersConflict`），盘点①该行随之加注。
