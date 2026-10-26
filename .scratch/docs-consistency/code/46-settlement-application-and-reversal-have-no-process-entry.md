# 46 SA 核销与核销撤销没有任何进程入口

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-code12（见文末「完成记录」，与 [code/12](./12-funds-mapping-has-no-compensation-or-recovery-targets.md) 同批）。原 ready-for-agent——R2（上线后尽快）：资金映射已在生产装配上，核销却没有命令、端点或编排调用；首批收款一来只能停在未分配，入账余额「按核销分配净额」那一支走不到，已确认未结也不随核销离开。看得见的卡住、不答错金额；绕法是线下对账、留未分配。2026-10-21 文档自洽 09 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc09。
Blocked by: 无（与 [12](./12-funds-mapping-has-no-compensation-or-recovery-targets.md) 改同一只编排，宜同批或排在 12 之后）
出处：[audit/sa-cr.md](../audit/sa-cr.md) M12 的代码一半（文档一半由文档自洽 09 在 ADR-0236 的 Status 行注记更正）。取证于 main `899a4555`。

## 现象

UC-SA-005 主流程按指示或唯一匹配形成核销、处理撤销与退回形成核销撤销，SA CONTEXT 的入账余额按「指向该作用域客户应收的核销分配净额」重算；应用层有 `MapExternalFundsHandler.Apply` 与 `Reverse`，但 `cmd/` 下没有任何构造或调用：

- `cmd/parcel-settlement-register/translate.go` 头注：「核销（Apply）与撤销（Reverse）没有命令：核销面归 UC-SA-005 另一段的票。」那张票并不存在。命令表里只有 `funds-mapping`。
- `cmd/parcel-api` 只有 `GET /settlement-funds-applications`（`endpoints_settlement_catalogue.go`），没有写口。
- `cmd/parcel-dispatch` 没有任何信封驱动核销。

## 证据

- 文档：UC-SA-005 主流程「按明确指定范围形成核销」与「处理外部撤销、退回或更正，形成反向资金映射、核销撤销或重分配」，`AT-SA-113`、`AT-SA-116`、`AT-SA-117`；SA CONTEXT「运营余额、收付款与坏账」入账余额那一条与核销各条；[ADR-0182](../../../docs/adr/0182-receivable-account-is-a-funds-mapping-target.md) 决定二「核销与核销撤销仍没有命令」。
- [ADR-0236](../../../docs/adr/0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md) 决定四附段与决定七说映射与核销「只有受控 CLI」，与代码不符；文档自洽 09 已在它的 Status 行注记里按 ADR-0182 决定二更正，正文不改。
- 代码：`application.MapExternalFundsHandler.Apply`、`.Reverse`；`ApplySettlementCommand`、`ReverseApplicationCommand`；`unversionedApplyDigest` 已备好载荷摘要。

## 三类

机制：核销与撤销的进程入口（受控 CLI 一路即可首发；在线口与授予照 ADR-0236 的口径另裁）。产品策略：无。租户取值：核销指示与账期顺序是来源数据，不预填。

## 规模

M：`parcel-settlement-register` 加 `funds-application` 与 `funds-application-reversal` 两个命令（照 `funds-mapping` 的译法与重放口径），装配 `MapExternalFundsHandler` 的 Apply/Reverse；补竖切测试；更正 `translate.go` 那句头注。

## 最小的诚实修法

先开受控 CLI 一路：命令只译载荷、不推导分配，分配必须有映射背书（`Apply` 已守）；撤销只登记核销关系失效、原分配不删（`Reverse` 已守）。在线写口与操作者授予不在本票。

## 要裁的

无（在线口另裁，不挡本票）。

## 完成记录（2026-10-25，作者会话，分支 claude-code12，变基到 main `061156bcc`）

与 [code/12](./12-funds-mapping-has-no-compensation-or-recovery-targets.md) 同批，两票共两笔代码与文档提交；测试与变异一并记在 code/12 完成记录「测试」，这里只列本票的那几处。证据只记 `S`（隔离合成）。

### 落了什么

1. **两条命令**：`parcel-settlement-register` 的封闭命令表 `allCommands` 加 `funds-application`（调 `Apply`）与 `funds-application-reversal`（调 `Reverse`），分派照 `funds-mapping` 那一条的形写在 `translate.go`。`buildRegistrar` 本来就装着这只 `MapExternalFundsHandler`（核销写口与已结视图交接口都接真），本票只补 code/12 的那一口；没有别的装配总表要改，形照票 sell-customer-charge/01 加命令那一笔。
2. **译法**：`registrationjson.FundsApplicationFromJSON`、`FundsApplicationReversalFromJSON`。一格对命令一格，零默认、未知字段拒（分配里的也拒）；映射清单与分配原样带过去，不从分配推映射清单、不并片、不排序、不按金额或到期日代拟分配；方向（`DEBIT`/`CREDIT`）与目标种类按封闭词表译，标识过构造门并指名哪一格。撤销载荷只有核销引用、撤销依据与时刻，没有分配格。
3. **答案与重放**：两条命令走 `fundsAnswer` 同一张表（已核销、已存在、已撤销、已撤销重放 → 0；`未受理`、分配失衡、跨币种 → 1；核销冲突 → 2；未决、行已落信封未出 → 3），答复带核销引用、本笔核销金额与这一步之后该事实的累计未分配（修订轮改准，见文末「评审记录与修订轮」）。重放口径照 `funds-mapping`：同键同内容答已存在并再交同一封（Outbox 按认领键吞重），同键异内容答冲突；撤销重放答已撤销、不二撤。撤销只写撤销两列，原分配不删（竖切用例直读 `allocations` 前后逐字相同）。
4. **确定性拒绝不进未决**：核销或撤销信封被框架校验拒收时，交接口以 `ports.ErrSettlementApplicationHandoffRejected` 交出，编排返 error 让事务壳整笔回滚，`execute` 的哨兵表加一行归用法格 1、点名原因、不叫人重跑。此前 `handOff` 把一切交接失败都折成续办引用，CLI 会退 3 并说「重跑同一命令补发」——那一格重跑永远补不上（同票 sa-cc/32 对采用信封的做法）。
5. **注释**：`translate.go` 头注删「核销（Apply）与撤销（Reverse）没有命令：核销面归 UC-SA-005 另一段的票」，改写成三条命令的现状；`main.go` 包注、退出码说明与 `fundsAnswer` 头注随事实改准。
6. **ADR**：ADR-0182 Status 行索引注记加一条「代码事实」、Links 加一行：决定二「核销与核销撤销仍没有命令。」写的是成文时的代码，本票落地起有两条命令、仍只有受控 CLI；不停用条款。

### 代决

1. **ADR 注记只写在 0182 一处**（docs/adr/README「一件事只写一处」）：这是代码事实追上了决定二的「仍没有」，不是改口径，所以不是部分停用、不立 ADR。ADR-0236 Status 行的「表述更正」（2026-10-21）写的是「以 ADR-0182 决定二为准」，读的人顺链到 0182 注记就见现状，0236 注记不改、不写第二处（修订轮按评审 N5 在 0236 注记末尾加了半句指针，现状仍只写在 0182 一处）；它说的「今天」是 2026-10-21 那一天。本票落地后 0236 决定四、决定七「核销……只有受控 CLI」两句与代码相符。
2. **（修订轮撤回，code/88 并入本票修，见文末）** 核销与撤销的事务里不重算入账余额与已确认未结，原留给新票 [code/88](./88-settlement-application-and-reversal-do-not-recompute-the-posted-balance.md)：SA CONTEXT 与 ADR-0181 决定一要在那次事务里重算，而 ADR-0181 决定六（及 ADR-0182 对它的收窄）把首切钉在映射上；把核销加进调用点是落决定一还是扩成决定六，属已接受 ADR 的口径，作者不自取。本票落地后指向对账单的核销分配在同作用域下一次映射时进重算，可用余额不受影响（两列等额移动）。UC-SA-005 交接表那一行改指 code/88。
3. **映射清单与分配都由交进来的人写全**：票面「命令只译载荷、不推导分配」；映射清单与分配里的映射重复写，是为了与 `ApplySettlementCommand` 一格对一格、载荷摘要不变形。
4. **撤销一笔不存在的核销答 `未受理`**（既有 `Reverse` 行为，CLI 退 1）。

### 越权风险点（待 SA owner 复核）

1. **在线写口与操作者授予不在本票**（票面与 ADR-0236 决定七）：核销与撤销只有受控 CLI，管理台收付款核销页仍只读。
2. **（修订轮已修，见文末）入账余额在核销后停在旧值，直到同作用域下一次映射**；以入账余额为基数的比例额度这期间读旧值。另有一格本票让它走得到：分给对账单的分配超出该作用域已确认应收时，此后同作用域的映射重算求出负的已确认未结、整笔答未决；续办是用本票的撤销命令撤掉那一笔。两格都在 code/88。
3. **撤销依据只是引用**：`Reverse` 不核依据指的资金退回或撤销事实是否已采用；登错依据产品不拦。
4. **演示种子没有核销步**：演示租户今天的种子只有一笔映射到账户的收款，本票不加核销种子；演示要走核销须手跑这两条命令。
5. **潜伏卡死：确认后的贷记调整让作用域账不平之后，撤销也撤不动**（第二轮评审实测）。
   - 情形：两笔各 3000 核满对账单 C 的已确认应收 6000，之后来一笔确认后的贷记调整 4000，已确认应收变 2000。撤哪一笔 W 都还是 3000 > 2000，重算拒绝、整笔回滚，该作用域的撤销、映射、核销全堵。
   - 今天不可达：费用调整没有入口，`buildSettlementOrchestrations` 只为启动即查而构造调整编排。
   - 去向：等费用调整开入口时处理。入口归 [product-strategy-boundary/22](../../product-strategy-boundary/issues/22-settlement-orchestrations-assembly-and-triggers.md)「结算其余编排的装配、入口与触发面」；协调方转述时写的是 /12，那张是金额文法与分摊、已 resolved，不承接入口。思路是放行不让账变得更坏的撤销，或允许一次撤多笔。

### 新票

- [code/88](./88-settlement-application-and-reversal-do-not-recompute-the-posted-balance.md)（修订轮并入本票修，resolved，号不回收）：核销与核销撤销的事务里不重算入账余额与已确认未结；顺带记映射重算的两个确定性失败（`ErrNegativeConfirmedUnsettled`、`ErrNotReceivableAccount`）今天在 CLI 落到未决 3，以及同作用域重算没有作用域锁。号段照协调方提醒从 code/88 起取，取号前 ls 过 main 与各 worktree。

### 测试

见 code/12 完成记录「测试」。本票专属的：`cmd/parcel-settlement-register/funds_application_vertical_test.go` 两条真库竖切（核销、重放、冲突、失衡、撤销、撤销重放、撤不存在的、追偿收款；信封被拒整笔回滚退 1、重放同样退 1）；`translate_test.go` 的命令表与载荷错送用例；`registrationjson/funds_application_test.go` 的保真与拒绝面；`settlement_application_handoff_test.go` 与应用层的信封被拒两格。

## 评审记录与修订轮（2026-10-26，协调方隔离评审：不可合入，一条阻断；作者修订轮，分支 claude-code12）

阻断 B1 与 N1、N3、N4、N7、协调方另裁各条记在 [code/12](./12-funds-mapping-has-no-compensation-or-recovery-targets.md)「评审记录与修订轮」，锁序全序论证也在那里。本票这边：

- **N2：code/88 并入本票修**（协调方裁定，号不回收，先例 code/86）。ADR-0181 决定一原文已把核销、核销撤销列为重算点；决定六只限首切，接受记录写明核销「另拆」；ADR-0182 只为 `ACCOUNT` 收窄首切——所以把 `Apply`、`Reverse` 挂进重算是落决定一，不需新 ADR，原代决 2 撤回。
  - 落法照 `advanceCustomerReceipt` 的路子：`statementScopes` 按分配收出客户应收作用域、按作用域合计、排序。`Apply` 落行前逐个锁作用域、按账判这一笔落下后已确认未结会不会为负，会就答 `SOURCE_NOT_ACCEPTED`、什么都不落（`admitStatementScopes`）。落行后在同一事务里经 `recomputeScope` 推进。`Reverse` 改写撤销两列后同样推进。`Map` 也改走 `recomputeScope`。作用域锁是新的事务级咨询锁（`CustomerReceiptPostings.LockOperationalBalanceScope`，锁空间 `operational_balance.scope`，与接受前控制分开）。
  - **为何必须并入**：`CreditRatioBases.LoadCreditRatioBase` 的 `PostedBalanceBase` 读 `operational_balance.posted_minor`，在 `cmd/parcel-dispatch/assemble.go` 的生产装配里；核销后入账余额停在旧值，以入账余额为基数的比例额度就按核销前的数答大。
  - 映射重算的两个确定性失败在 CLI 不再落未决：`ErrNotReceivableAccount`（目标不是应收账户）归用法格 1；`ErrNegativeConfirmedUnsettled`（作用域账已不平，多半是存量的超额对账单核销或确认后的贷记调整）归冲突格 2——要人核对该作用域的核销与调整、撤掉多出的那一笔，不是改这一份请求，也不是重试。
- **N5**：ADR-0236「表述更正（2026-10-21）」末尾加半句指针（code/46 落地后两句与代码相符，现状见 ADR-0182 注记）；ADR-0182 注记删去「仍只有这一路受控 CLI、没有在线口」，改指 ADR-0236 决定七。
- **N6：并发同键重放答成失衡**（评审真库 20 次 17 次复现）。同键改在拿到事实锁之后才查：后到的那一份在锁上等，先到的提交后它查得到那一行，答 `EXISTING_APPLICATION` 退 0。`Reverse` 同理，锁事实之后再读一次核销，并发两份撤销后到的那份答已撤销。补 `TestConcurrentReplaysOfOneApplicationAnswerExistingNotImbalance`。
- 原「落了什么 3」答复一句、「越权风险点 2」随修订改写（见上两段）。

### 修订轮测试（含 PG，`TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh`，变基后的 tip）

- `go build ./...`、`go vet ./...` 退 0，`gofmt -l` 无输出。
- 改动包的反向依赖（`go list -test` 以 SA domain 与 `migrations` 为根，66 个包）加 `./cmd/parcel-settlement-register/...`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./internal/architecture/...`、`./migrations/...`：退 0（65 个包 ok、其余无测试文件；含 PBC-08 门禁）。
- 并发用例 `-count=5`（并发超额、并发同键、金额本体锁）：五次全过。
- 0046 改了：种子重灌一次性库 `c12seed2` 退 0，库上 CHECK 已是六值、`RECOVERY_ACKNOWLEDGEMENT` 在内，种子那一行 `ACCOUNT` 共存，用后删库。
- 管理台三道门（Node 22）：tsc、601 例、vite build 退 0。
- 收尾变基到 main `fb8ee7096` 之后在新 tip 上重跑：`go build`、`go vet` 退 0；反向依赖全量同上一套退 0（65 个包 ok）；并发用例 `-count=5` 五次全过；管理台三道门退 0（main 那边加了用例，631 例全过）。种子重灌是变基前在 `c12seed2` 上做的，main 那几笔迁移只动 `party_commercial`，没有重灌。
- 新用例：
  - 应用层：`TestApplyAndReverseTakeLocksInOneTotalOrderAndRecomputeTheirScopes`（锁序与两路推进）、`TestApplicationsOfOneFactStayWithinItsAmount`、`TestAFundsReturnedFactNeitherMapsNorAppliesToAClaimAmount`、`TestABrokenClaimTargetStandingIsADeterministicImbalance`。
  - postgres：`TestApplicationsAndReversalsRecomputeThePostedBalanceInTheirOwnTransaction`；`TestTheFactAndBalanceScopeLocksRefuseToRunOutsideATransaction`，即两把新锁的 PBC-08 负面证据。
  - CLI 竖切：顺序超额、并发超额、并发同键三条。
- 变异（每处改完都跑红、复原后跑绿；复原是把备份拷回，没有用 `git checkout`）：
  1. 去掉事实一级上限（`AdmitWithinFact` 不调）→ 应用层与 CLI 顺序、并发三条红；
  2. 锁序反（作用域先于金额本体）→ 锁序用例红；
  3. 撤销那一路不重算 → 应用层锁序用例与 postgres 重算用例红；
  4. 同键查回到锁前 → 并发同键用例 `-count=3` 三次全红；
  5. 金额本体求和去掉租户条件（N7）→ 未核销面用例红；
  6. 事实身份行去掉 `FOR UPDATE` → 并发超额用例红（十秒内没有一路在身份行上排队）；
  7. 资金退回规则去掉 → 应用层退回用例红。

## 第二轮评审与第三轮修订（2026-10-26，协调方隔离评审：B1 已消、无阻断；作者第三轮修订，分支 claude-code12）

N1、N3、N4、N5、N8 记在 code/12 同名一节。本票这边：

- **N2 作用域锁补真库用例**：照 `raceTwo` 抽出 `raceUnder`，占锁事务先拿住要证的那把锁，等两路都在锁上排队（`pg_stat_activity`）再放开。
  - `TestConcurrentStatementApplicationsStayWithinTheConfirmedReceivable`：两路核销（两条事实）各核对账单 C 已确认应收 10000 的 60%，一路已核销、一路`未受理`，已确认未结停在 4000。
  - `TestAMappingQueuedBehindTheScopeLockPostsBothReceipts`：一路映射排在作用域锁后面，占锁那笔事务以更晚的时刻推进另一笔映射并提交；排队那一笔拿锁后用锁后时刻推进，终值含两笔（N1 也靠它守）。
- **N6 落行竞态**：`Apply` 落行答「已有」那一支照锁后那段用 `CompareStoredDigest` 分出冲突与已存在，不报累计未分配（赢家可能在另一条事实上）。原头注「只可能是不经事实锁的写入方」不成立，已改：同一核销引用、不同事实的两份并发交，各锁各的事实，都走得到这里。补 `TestALostSaveRaceComparesTheWinnerLikeTheLockedCheck`，用替身在锁后查键与落行之间落下赢家，让这一支确定走到。
- **N7 并发撤销**：补 `TestConcurrentReversalsOfOneApplicationAnswerAlreadyReversed`，后到的那份在事实锁后再读到撤销版，答已撤销、退 0。
- **N5 的入口用例**：`TestAClaimTargetAppliedBeyondItsAmountExitsAsConflict`，在库里垫一笔越额核销，再交一笔核销退 2、整笔回滚。
- **记录**：越权风险点补潜伏卡死一条；`ports.FundsFactApplicationLock` 头注补一句——核销与撤销路上随写入隐式取的行锁都在作用域锁之后，映射在取作用域锁之前插映射行接不成环。

### 第三轮测试（含 PG，`TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh`，变基后的 tip）

- `go build ./...`、`go vet ./...` 退 0，`gofmt -l` 无输出。
- 反向依赖（`go list -test` 以 SA domain 与 `migrations` 为根，66 个包）加 `./cmd/parcel-settlement-register/...`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./internal/architecture/...`、`./migrations/...`：退 0（65 个包 ok）。
- 并发用例 `-count=5`（作用域两条、并发撤销、并发超额、并发同键、金额本体锁）：五次全过。
- 种子灌一次性库 `c12seed3` 退 0（映射重算这一路改了，冒烟一次）：演示账户入账余额 20000，与那一笔 200.00 的合成收款相符；用后删库。迁移 0046 本轮没改。
- 管理台三道门（Node 22）：tsc、640 例、vite build 退 0。
- 变异（改完都跑红、复原后跑绿；复原是把备份拷回）：
  1. N1 锁前取时（留着未推进报错）→ 排队映射那条红，退 3 并报 `posting did not advance`；锁前取时且吞掉未推进（即评审实测的原缺陷）→ 同一条红，入账余额停在 4000；
  2. N2 作用域锁换成不取锁的 SELECT → 作用域两条并发用例红（十秒内没有一路在锁上排队）；
  3. N4 去重去掉（`UNION ALL`）→ 一对多映射用例红，入账余额 20000；
  4. N7 撤销去掉锁后再读 → 并发撤销用例 `-count=3` 三次全红，后到的那路答`未受理`；
  5. N6 落行竞态不比内容 → 应用层用例红；N5 把账不平改回分配失衡 → 应用层与 CLI 两条红。
