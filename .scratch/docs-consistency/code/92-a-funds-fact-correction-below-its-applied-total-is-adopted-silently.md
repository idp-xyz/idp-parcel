# 92 SA 外部更正把事实金额改到已核销合计以下：照采，不标超核、不重算

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code92（见文末「完成记录」）。原 ready-for-agent——R2（上线后尽快）：外部更正照采是对的（外部系统拥有资金事实），可采下来之后既不说出这条事实已经超核，也不重算它映射到的客户应收作用域；读面上的累计未分配为负是唯一的信号，入账余额停在更正前的金额上，直到同作用域下一次映射、核销或撤销才跟上。看得见但要人去找，不答错可用余额以外的数。2026-10-26 票 [code/12](./12-funds-mapping-has-no-compensation-or-recovery-targets.md) 第二轮评审 N3 立（协调方裁定本批不改代码、立票），分支 claude-code12。
Blocked by: 无
出处：code/12、code/46 第二轮隔离评审 N3；取证于分支 claude-code12。

## 现象

- UC-SA-005 `AT-SA-114`「外部事实金额被更正 → 保留原版本，按新有效版本重算差额和核销」；SA CONTEXT「入账余额在形成它的那次 `UC-SA-005` 事务里……重算并推进」。
- 代码：`application.MapExternalFundsHandler.CorrectFact` 从链头形成新版本、落版本行、交采用信封，不读这条事实上未撤销核销的合计，也不重算它映射到的作用域。
- 更正后的金额低于未撤销核销合计时：
  - 已采用资金事实册读面的 `UnappliedMinor`（事实金额 − 未撤销核销合计）为负，`parcel-settlement-register` 核销类答复把它说成「累计超核 X」——这两处是今天仅有的信号，都不截成 0。
  - 对同一事实再交核销一律答 `APPLICATION_IMBALANCE`，但没有一格告诉操作员该先撤哪一笔。
  - `CustomerReceiptPostings.CustomerDirectionNets` 的 P 取链头金额，可更正这一步不推进，`operational_balance` 停在更正前的数。
- 核销与更正在事实身份行上排队（`SettlementApplications.LockFundsFactApplications` 头注），这只保证核销读到更正后的金额，不保住「已核销合计不超过事实金额」。

## 三类

机制。无产品策略；无租户取值。

## 规模

S–M。

## 最小的诚实修法

- `CorrectFact` 先对事实身份行 `FOR UPDATE`，与核销同一把锁、同排在资金侧锁序全序首位（`ports.FundsFactApplicationLock` 头注）；插版本行时外键那把 KEY SHARE 来在读之后，晚了。
- 取锁后求未撤销核销合计，然后照采。
- 新金额低于合计时，答复带出超核金额；HTTP 照 ADR-0022 放在响应体，不改状态码。
- 同一事务里重算该事实映射到的对账单与账户作用域（走 `recomputeScope`，锁序取全序的子序列）。
- 不自动撤销任何核销：撤哪一笔是人的判断。

## 要裁的

- 超核那一格叫什么：是采用编排的一个结果格，还是采用结果上的一个字段（采用本身成立）。
- 要不要一本待处理册，让超核的事实列出来等人撤销，而不是只靠读面上的负数。

## 完成记录（2026-10-26，作者会话，分支 claude-code92，变基到 origin/main `b6f0c10ba`）

### 做了什么

- `application.MapExternalFundsHandler.CorrectFact`：命令自己的形判完之后第一步对事实身份行取锁（`ports.FundsFactApplicationLock`，与核销、撤销、映射同一把，全序首位），取到再查新版本、查链头、数未撤销核销的合计；事实不在册答`未受理`，锁读不动答未决（资金事实库不可用）。形成新版本之后，按键序取齐事实落到的全部客户应收作用域的咨询锁，再落版本行，落下后在同一事务里 `recomputeScopes`，然后交采用信封。锁序是全序（事实 → 金额本体 → 作用域）的子序列：事实 → 作用域。没有被受理的更正（回指非链头、事实未采用）只取事实锁，什么都不落；事实没映射到任何客户应收作用域时只取事实锁。
- `FundsResult.OverAppliedMinor`：新金额低于合计时的差额，否则 0；更正的答复带出，不是新结果格（结果格仍是已采用）。重放同一次更正对仍是链头的那一版带出此刻的超核，被后来的更正取代的版本（重放首版）不带。
- HTTP：`fundsRegistrationResponse.overAppliedMinor`，`omitempty`，只在超核时在场，状态码仍是 201（ADR-0022）；采用与更正两口、集成客户端与手工两族共用同一个响应形。受控 CLI：更正的答复补「事实 X 累计超核 N」，与映射、核销答复同一句话，退出码不变。管理台：`RegistrationResponseBody.overAppliedMinor`，登记答案示出「累计超核：N……撤销哪一笔核销要人判断」，说明抽成纯函数 `over-applied-note.ts`（组件带 ui-primitives，Node 测试装不进来）。
- 头注改准：`ports.FundsFactApplicationLock`（更正走同一把锁、取在落版本行之前、重算集合含「外部更正：事实落到的全部客户应收作用域」）、`SettlementApplications.LockFundsFactApplications`（去掉「更正侧得自己先取锁」的待办口气，改成更正自己也走本方法）；`ExternalFundsFactCatalogueRow` 与册读面两处「超核唯一的信号」改成「读面上超核就是这个负数」；CLI `factBalanceOf` 头注同步。
- 文档：新立 ADR-0285；ADR-0181 与 ADR-0283 的 Status 注记与 Links 各加前向指针；SA `CONTEXT`「运营余额、收付款与坏账」补一条规则句（不挂票号）；UC-SA-005 状态行改写、并发一节补一条、`AT-SA-114` 预期结果补超核一句，去掉「给开发的交接」整节（那里只有指向本票的这一行）；AT 盘点 `AT-SA-114` 点名另一半并文末追记。
- 无迁移。无新票。

### 核对 ADR-0181 原文（要不要立记录）

决定一原文：「`UC-SA-005` 在采用并映射、核销、核销撤销各自的事务里，按下面的求法重算该作用域的入账余额与已确认未结，并推进 `operational_balance`。」只列这几个点，外部更正不在其中；ADR-0283 决定三又按这几个点逐一定了重算哪几个作用域，其「只重算这一笔改了 P 或 W 的作用域」与更正取全部作用域也不同。所以属「扩成」，用预留号立 ADR-0285，两份旧记录只动 Status 注记与 Links。

### 代决

1. **超核是更正结果上的一个字段**（协调方裁）：结果格仍是已采用，字段名 `OverAppliedMinor` / `overAppliedMinor`，只在大于 0 时在场；不拒收，不自动撤销核销。
2. **不建待处理册**（协调方裁）：超核靠读面上的负数、CLI 与 HTTP 答复发现，写进 ADR-0285 越权风险点 1。
3. **重算集合取全部 `FactCustomerScopes`，不逐个判 P 动没动**（协调方裁，作者核过）：多作用域收款只在金额越过某个作用域的 d 时才真动，逐个判要把求 P 的 SQL 搬到应用层；我试想过用「新旧金额都不低于已核销合计就跳过」的捷径，反例是核销里有借贷混合分配、单个作用域的 d 可以大于合计，会漏，所以不取。
4. **事实锁取在 `FindVersion` 之前**：重放路径也要数合计才能带出超核；代价是重放也排队，更正少见，可以接受。
5. **作用域锁在落版本行之前取齐**（而不是留给 `recomputeScope` 自己取）：`recomputeScopes` 的契约是调用方已锁住；端口头注写明隐式行锁都落在作用域锁之后，更正落版本行与采用信封入队也守这条。有应用层用例钉「第一次推进之前两把作用域锁都已取」。
6. **重放带出此刻的超核，只对仍是链头的那一版**：响应丢了重交，信号与第一次一致；这是字段语义的一部分，不是额外功能。
7. **信用状况不重算**：它读已确认费用的到期画像，不读资金事实金额，更正不改它。
8. **管理台示出超核**：派单点名 CLI/HTTP，管理台手工登记口走同一个响应体，不示出就是同一处静默换了个面；只动共享组件 `RegistrationPanel.tsx`（字段缺席时对其余登记面没有任何变化）与新增的纯函数，没碰 `pages/party/**`、`pages/operations/*`、`pages/network/api.ts`。
9. **账不平拦更正的 HTTP 面不新立专名码**：见越权风险点 2。
10. 测试替身 `factLockDouble` 原先把事实库的读错误吞成「事实不在册」，与真适配器（读错即报错）不一致，更正走到它之后这个差异会让既有的「库不可达 → 未决」用例变红；已改成传播错误。

### 越权风险点（同 ADR-0285 文末，待 SA owner 复核）

1. **超核没有待处理册，只能靠读面发现**（协调方裁、作者记）：更正答复里的数是那一刻的快照，之后核销撤销会改变超核而答复不更新，读面上的负数才是此刻的。
2. **更正被一个已不平的作用域拦下**：更正自己造不出不平（不动 W 与已确认应收），但事实落到的作用域若已不平，重算失败、更正整笔回滚（版本行不落、信封不出）。CLI 答「账不平」退冲突格并带作用域身份；HTTP 两口答 500 `NO_ANSWER_FORMED`，没有专名码，外部系统会按 5xx 重投直到人核对了账。这是 HTTP 面上第一次可达的「账不平」（映射与核销没有在线口），是否要专名码、管理台说明怎么写，留给 owner。
3. **不逐个判 P 动没动**：多重算的同值作用域会扩大风险点 2 的拦截面。
4. **存量停在旧值**：本票之前更正过的事实，作用域要等下一次碰到才跟上（同 ADR-0283 风险点 5）。
5. **作用域咨询锁键是哈希**：更正取几个作用域时碰撞破序的最坏情形同 ADR-0283 风险点 7。
6. **管理台共享组件**多了一个可选字段的呈现；`pages/party/**` 的表单也经 `RegistrationAnswerNote` 渲染，字段缺席时不出现任何新东西，但这些页面不在本票的测试范围里。

### 测试（含 PG，`TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh`）

- 先写失败用例看红再实现：application 包五条、postgres 包四条红（超核金额为 0、改高后入账停在旧数、事实锁没握到、核销先到时更正数不进它）。
- 新用例：application 包 `TestACorrectionBelowTheAppliedTotalIsAdoptedAndSaysHowMuchIsOverApplied`、`TestReplayingACorrectionBelowTheAppliedTotalStillSaysHowMuchIsOverApplied`、`TestACorrectionLocksTheFactFirstThenRecomputesEveryScopeTheFactReaches`、`TestACorrectionOfAFactMappedToNoCustomerScopeRecomputesNothing`、`TestACorrectionThatIsNotAcceptedLocksNoScopeAndRecomputesNothing`、`TestACorrectionStopsOnTheFactLockAccordingToWhyItCouldNotTakeIt`；postgres 包 `TestACorrectionBelowTheAppliedTotalIsAdoptedAndSaysSoAndRepostsTheScope`（评审手算例：收款 5000 映 stmt-1 核满 5000，更正到 3000，超核 2000，入账 -2000）、`TestACorrectionRepostsEveryScopeTheFactReaches`（收款改高、退回改高、多作用域收款改低）、`TestACorrectionHoldingTheFactLockMakesALaterApplicationSeeTheCorrectedAmount` 与 `TestAnApplicationHoldingTheFactLockMakesALaterCorrectionCountItInTheOverApply`（更正与核销并发的两种先后，先手停在握着事实锁之后，后手必须排在锁上，池外连接按 `pg_stat_activity` 的锁等待轮询，每步十秒期限）、`TestACorrectionOutsideATransactionIsRefusedAndWritesNothing`（PBC-08 事务外拒跑）；HTTP 包 `TestACorrectionBelowTheAppliedTotalCarriesTheOverAppliedAmountInTheBodyAndKeepsTheStatus`；CLI 包 `TestACorrectionBelowTheAppliedTotalSaysOverAppliedThroughTheCommand`、`TestAConcurrentCorrectionAndApplicationOfOneFactEndInAConsistentState`（占住事实行，两路排队后放开，先后不定，断言终态自洽：超核 = 终态已核销合计 − 事实金额，入账 = 事实金额 − 已核销合计）；管理台 `over-applied-note.test.ts`。
- 既有用例因更正多了事实锁而调整：HTTP 包替身 `fundsStoreStub` 增 `LockFundsFactApplications`（采用路径取锁数断言为 0）。
- 范围：改动包 + 反向依赖（`go list -test` 算出）+ `./cmd/parcel-api/` + `./cmd/parcel-dispatch/` + `./cmd/parcel-settlement-register/` + `./internal/architecture/...` + `./migrations/...`：退 0。新用例 `-count=3`、并发用例 `-count=5`：退 0。`go build ./...`、`go vet ./...` 退 0，`gofmt -l` 无输出；`adr-index` 在工作副本上生成一次无错（生成物已还原，`-check` 只因 README 待重生成而退 1）；`at-coverage-counts -check` 退 0。无迁移，种子未灌。
- 管理台三道门（Node 22，软链主树 `node_modules`、用后删，`dist` 与 `tsbuildinfo` 一并删）：`tsc -b --noEmit`、`scripts/run-tests.mjs`（684 过）、`vite build` 退 0。
- 变异（改完跑红、拷回备份复原并 `cmp`；没有用 `git checkout`），全部被杀：
  1. 更正不取事实锁（合计取 0）→ application 六条、postgres 四条、CLI 两条、HTTP 两条红；
  2. 更正落版本行后不重算 → postgres 四条、application 一条、CLI 两条红；
  3. 重算范围漏作用域（只重算键序第一个）→ postgres 多作用域一条、application 一条红；
  4. 超核字段恒为 0 → application 两条、postgres 四条、HTTP 一条、CLI 两条红；
  5. 只在超核时重算（升不重算）→ postgres 改高、application 一条、CLI 一条红；
  6. 作用域不排键序 → application 一条红；
  7. 作用域锁不在落版本行之前取齐 → application 一条红（加了「第一次推进之前两把锁已取」的断言之后）；
  8. HTTP 响应体不带 `overAppliedMinor`、CLI 更正的答复不用 `correctionSubjectOf` → HTTP 一条、CLI 两条红。
