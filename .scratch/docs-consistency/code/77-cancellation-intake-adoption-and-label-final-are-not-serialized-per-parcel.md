# 77 PS 取消提交、收寄采用与面单终局形成三处之间不按包裹串行

Category: bug
Status: resolved——2026-10-26 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-code77（见文末「完成记录」）。此前 ready-for-agent——R2（上线后尽快）：两份互斥的终局可能同时落下，且之后没有任何信封会重新比对。read committed 下，取消的提交边界与收寄采用、面单终局形成各在自己的事务里读对方的表；两笔并发时都看不见对方尚未提交的写入，取消与责任起点（或非取消终局）会同时成立。今天取消授权停在未配置，取消形成不了，这一格还到不了；租户一登记取消授权即生效。2026-10-21 协调方据 code/21、35 隔离评审 N4 立，分级 R2。
Blocked by: 无
出处：code/21、code/35 隔离评审 N4（协调方转达）；[ADR-0254](../../../docs/adr/0254-cancellation-and-boundary-fact-order-is-judged-by-their-business-times-and-a-late-earlier-boundary-fact-is-a-fact-conflict.md) 决定二「谁先越过提交边界只决定走哪一格」——前提是「先越过」有一个确定的先后。

## 现象

ADR-0254 的三格建立在「谁先越过提交边界」可以判定之上：取消到提交边界时边界事实已在册 → 不形成取消；取消先提交 → 后到的边界事实按业务时间比。两边各在自己的事务里读对方：

- 取消编排（`CancelParcelHandler.Handle`）在提交边界上读当前有效终局、采用链与首次有效收寄，随后写取消记录；
- 收寄采用（`AdoptNetworkIntakeHandler.Handle`）读已成立的取消，随后写采用记录；
- 面单终局判断（`JudgeLabelServiceFinalHandler.Handle`）读已成立的取消，随后经采用路径写终局。

三处之间没有按包裹的锁，也没有把对方的写入挡在同一索引上的唯一约束（取消表与采用表、终局表是三张表）。两笔并发、都在对方提交之前读，各自看到「对方不在册」，就会同时提交取消与责任起点，或取消与非取消终局。此后两边都已越过提交边界，消费门都已入账，不会再有信封重新比对——ADR-0254 的冲突那一格从未被触发，冲突册里也没有这一行。

## 证据

- 代码：`internal/parcelshipment/application/cancel_parcel.go`、`adopt_network_intake.go`、`judge_label_service_final.go` 三处读对方、写自己；`cmd/parcel-api/assemble_cancellation.go` 与 `cmd/parcel-dispatch/assemble.go` 的事务壳都不取包裹级锁。
- 先例：code/70 的作用域级咨询锁（`ports.PreAcceptanceControlScopeLock`）、code/39 的来源身份锁、code/14 的分户账锁。

## 三类

机制。无产品策略、无租户取值。

## 规模

M：一个按（租户，包裹）取 `pg_advisory_xact_lock` 的端口与适配器（锁空间如 `parcel_shipment.parcel.cancellation_boundary`，走 `RequireExecutor`）；三处编排在各自的提交边界读之前取锁，锁与读写在同一笔环境事务里；真库并发用例三对各一遍（屏障停在先手读完之后，后手要么已提交、要么经 `pg_locks` 看到等在锁上）。无迁移。

## 最小的诚实修法

1. 立端口（例如 `ports.ParcelBoundaryLock`）与 PostgreSQL 适配器，照 code/70 的形。
2. 取消编排在读终局之前取锁；收寄采用在读取消之前取锁；面单终局判断在读取消之前取锁（形成终局走的采用路径在同一笔事务里，锁一直持到提交）。
3. 取锁失败按依赖未决，各编排各用自己的原因格。
4. 补并发用例：取消对收寄采用、取消对面单终局，各证后手看得见先手的写入，落到 ADR-0254 的某一格，而不是两份终局同时成立。

## 要裁的

- 网络服务的交付终局（`AdoptOnEffectiveDelivery` 那一路）是否也要进同一把锁：交付以责任起点为前提，责任起点已由采用那一路串行，今天看不出需要；落地时核一遍。

## 完成记录（2026-10-26，分支 claude-code77，rebase 于 origin/main `610368f1e`，下文 SHA 均为 rebase 后的）

### 做了什么

- `2e516115e` 代码与用例：
  - 新端口 `ports.CancellationBoundaryLock`（`ports/cancellation_boundary.go`）与适配器 `pspostgres.CancellationBoundaryLocks`：按（租户，包裹）取 `pg_advisory_xact_lock(hashtext('parcel_shipment.parcel.cancellation_boundary'), hashtext(租户 || chr(31) || 包裹))`，走 `RequireExecutor`，锁与读写同一笔环境事务、随事务结束放。
  - 取消编排 `CancelParcelHandler`：在委托可见性核验之后、取消库的幂等读之前取锁（也就在授权与提交边界上的终局、取消、采用链、首次有效收寄几道读之前）。取收寄采用 `AdoptNetworkIntakeHandler`：边界核验做的时候（`Cancellations` 非 nil）在读已成立的取消之前取锁，责任起点读与采用写入都在锁里。面单渠道终局判断 `JudgeLabelServiceFinalHandler`：在读已成立的取消之前取锁，判出终局的格经采用路径（`FormParcelFinalHandler`）落在同一笔里。三处共用包内 `lockCancellationBoundary`；没装锁与取不到锁同答，不读边界、各按自己的原因格答未决（取消 `CANCELLATION_STORE_UNAVAILABLE`，采用与面单判断 `CANCELLATION_VIEW_UNAVAILABLE`），带续办引用。
  - 装配：`cmd/parcel-api/assemble_cancellation.go` 的取消编排、`cmd/parcel-dispatch/assemble.go` 的 `networkIntakeAdoption`（节点收寄、场外揽收两条采用链共用）与 `labelFinalJudgmentCore`（面单交易、关闭/重开决定、首次有效收寄三扇门共用）接同一把锁。未动 endpoints.go、main.go、endpoints_test.go、routes.go。
  - 真库并发用例 `internal/parcelshipment/adapters/postgres/cancellation_boundary_race_test.go`：三只编排走真取消库、采用库、终局库、冲突册与真锁，委托、授权、资格、终局规则、标识与发布意图用替身。屏障停在先手最后一道边界读之后，后手要么已提交、要么经池外单独连接看 `pg_locks` 等在咨询锁上才放先手；每一步十秒期限。
    - `TestACancellationAheadOfAnIntakeAdoptionIsSeenByTheAdoption`：取消先手，采用后手、收寄早于取消的业务时间 → 取消成立，采用停在 `CANCELLATION_ORDER_CONFLICT`，冲突册一行，无责任起点（ADR-0254 决定二第 3 格）。
    - `TestAnIntakeAdoptionAheadOfACancellationTurnsItIntoDispositionPending`：采用先手 → 取消答 `DISPOSITION_PENDING` 记下越过的那一代（第 1 格）。
    - `TestACancellationAheadOfALabelServiceJudgmentIsSeenByTheJudgment`：取消先手，面单判断带首次有效收寄后手 → `LABEL_SERVICE_NOT_FINAL`/`CANCELLATION_ORDER_CONFLICT`，冲突册一行，无终局（第 3 格）。
    - `TestALabelServiceFinalAheadOfACancellationIsReturnedAsTheStandingFinal`：面单判断先手 → 取消答 `FINAL_OUTCOME_STANDS` 指名那份终局（第 1 格）。
    - `TestAConcurrentReplayOfOneCancellationAnswersTheExistingResult`：同一取消请求并发重发 → 后手答 `EXISTING_RESULT`（钉住锁在幂等读之前，代决 2）。
    - 每条都读回落库状态，断言取消与责任起点或非取消终局不同时成立。修前四条交叉用例全红（后手径直提交，例如采用答 `COMMITMENT_FORMED` 而取消也答 `PARCEL_CANCELLED`）。
  - 锁本身（`cancellation_boundary_lock_test.go`）：`TestTheCancellationBoundaryLockRefusesToRunOutsideATransaction`（PBC-08 负面证据）；`TestTheCancellationBoundaryLockQueuesOnlyTheSameParcelOfTheSameTenant`（一笔握锁，另起事务限 `lock_timeout`：同包裹 55P03，同租户别的包裹、别的租户同号包裹立刻取到）。
  - 编排单元用例：`TestTheCancellationQueuesOnItsParcelBeforeReadingAnything`、`TestTheIntakeAdoptionQueuesOnItsParcelBeforeReadingTheCancellation`、`TestTheLabelServiceJudgmentQueuesOnItsParcelBeforeReadingTheCancellation`——锁指名命令的（租户，包裹）、取锁那一刻边界读一道都还没发生；取不到锁与没装锁都一道不读、不落记录、不交意图；采用在取消机制未接入时不取锁。既有编排与适配器测试的夹具补锁替身。
- `e74c99524` PS CONTEXT 取消一节补规则句「同一包裹的取消提交、收寄采用与面单渠道服务的终局判断依次越过提交边界」；UC-PS-003、UC-PS-004、UC-PS-006 一致性一节各补一行引它。
- `6db6eb799` AT 盘点 `AT-PS-079`、`AT-PS-086`、`AT-PS-107`、`AT-PS-108` 四行补并发用例点名（并发半边）；立 [code/98](./98-a-network-delivery-final-is-formed-over-a-standing-cancellation.md)。

### 代决（用户授权「你作为业务和系统专家自决」，协调方派单）

1. **网络服务的交付终局（`AdoptOnEffectiveDelivery` 那一路）不进这把锁。** 核过：交付一路经 `FormParcelFinalHandler` 只读终局库（`FindCurrentFinal`），不读取消写下的任何东西，排不排队它都照样形成终局，锁改不了它的答案。取消一侧：交付以责任起点为前提，责任起点在取消的提交边界上要么已在册（取消转待处置、不形成取消终局），要么经采用那一路的锁排在取消之后（采用读到取消、不形成责任起点）。剩下「取消在册、没有责任起点、却到了一份有效交付」这一格是顺序缺口——终局采用不读已成立的取消，与先后无关，加锁也收不住，立 code/98（needs-triage，建议 R2）；那张票修法里写明交付一路一旦读取消，就要在那道读之前取同一把锁。
2. **取消编排的锁提到幂等读之前，不只在终局读之前。** 只在终局读之前取锁时，同一请求并发重发的后手越过幂等读时先手还没落记录，进锁后读到先手已成立的取消，答成 `FINAL_OUTCOME_STANDS`，违反 UC-PS-006「重复请求返回原逐包裹结果」（变异 M6 实测）。代价：授权判断（读 PC 声明）也在锁里；授权未配置的今天，每一次取消请求都取一次锁再答 `AUTHORITY_UNCONFIGURED`。采用与面单判断不这样提：采用同键重投落在同一主键上，`commit` 读回赢家答已有结果；面单判断的幂等在采用路径的终局键上。
3. **取锁失败不新增原因码，归锁守的那一道读的那一格**（ADR-0029，恢复动作同为重试；code/70 代决 4 的先例）：取消 `CANCELLATION_STORE_UNAVAILABLE`（锁守的第一道读是取消库），采用与面单判断 `CANCELLATION_VIEW_UNAVAILABLE`。三只编排的封闭集、UC 结果表、HTTP 与消费门译法都不动。
4. **没装锁按取不到锁办（依赖未决），不响亮报错、不 panic。** 同冲突册未接入按写不进办的先例（code/35 代决 3、code/71 代决 6）。装配漏接由装配真库用例兜住：拆掉 parcel-api 那一处，`TestTheWiredCancellationAnswersHonestlyAgainstARealDatabase` 与 `TestTheWiredCancellationStopsAtTheBoundaryFactsAgainstARealDatabase` 红；拆掉 parcel-dispatch 两处，`TestALateEarlierNodeIntakeIsConsumedAsARecordedConflict` 与 `TestALateEarlierCarrierPickupIsConsumedAsARecordedConflict` 红。
5. **三处一把锁、一个锁空间。** 任意两处都要互斥；按对分锁，一笔事务就要取两把，取的先后又成了一处要守的全序。
6. **采用只在边界核验做的时候取锁**（`Cancellations` 非 nil）。nil 取消读口按既有约定是「取消机制未接入、边界核验整段不做」，没有取消可争，排队没有对象。
7. **无迁移**（咨询锁不改表）；**无 ADR**：机制。ADR-0254 决定二的三格一格未动，CONTEXT 新句只让「谁先越过提交边界」在并发下有确定的先后，不改任何一格的结果；未改、未接、未扩、未按口解、未收窄任何已接受 ADR 的口径。
8. **不立新 AT 号。** 并发是既有四个 AT 在并发下的同一结果，四行补点名、注「并发半边」。

### 锁序（全序与不成环的论证）

- 本票的包裹边界锁记作 P。持 P 的事务只有三种：parcel-api 取消（一次请求一件包裹、一笔事务一次 `Handle`）、parcel-dispatch 收寄采用（节点收寄、场外揽收各一封信一件包裹）、parcel-dispatch 面单判断（三扇门各一封信一件包裹）。扇出各路各开各的事务（`dispatch.FanOut` 逐路调，消费门各自 `WithinTransaction`，派发器不开外层事务），所以**一笔事务至多持一个 P 键**。
- 这三种事务里的取锁次序：消费门认领 inbox 行（仅 dispatch）→ P → 本笔插入（取消、采用、冲突册、终局各表的行与唯一键等待）→ outbox 插入。P 之前只有 inbox 认领与读已提交下的普通读（不取行锁）；P 之后不更新委托行、不取 code/70 作用域锁 S、不取 code/39 来源身份锁、不写 code/71 释放未受理册。
- 反过来，持 S 的事务（接受链的施加与被取代版本释放，parcel-api 撤回、主动拒绝、授权处置的释放与 code/71 释放未受理册的 savepoint 写入）与持来源身份锁的事务（parcel-api 提交守卫）都不调这三只编排，不取 P。P 与 S、来源身份锁、委托行锁之间没有任何一笔事务同时持有两边，图上没有边，P 可放在全序的任意位置；写作 inbox 认领 → P → 本笔行写入 → outbox。
- 持 P 之间不成环：T1 持 P(a) 等的只可能是 P(a) 本身或一处唯一键插入等待。后者的另一方若也是三只编排之一，它插入那一行之前必已持 P(a)，与 T1 持 P(a) 矛盾；若是不取 P 的写入方（交付一路往终局库插行），它从不等 P，先提交或回滚，T1 随后继续。重投的同一封信在消费门 inbox 认领处就排住，那时还没取 P。散列撞键只让两件不相干的包裹多等，一笔事务仍只持一个键，不成环。
- 不在本票、只记观察：parcel-api 撤回、主动拒绝、授权处置先存委托（委托行锁）再释放（S），接受链先施加控制（S）再存决定（委托行锁），次序相反。code/70 越权风险点 2 记过这一格、说接上 parcel-api 作用域时另立票；code/71 越权风险点 8 说三处释放口今天已放得掉。它今天是否已可达本票没有实测，交协调方判。

### 越权风险点（待 PS owner 复核）

1. **交付终局一路不排队**（代决 1）。取消在册而交付仍形成非取消终局那一格留在 code/98；那张票的修法若让交付一路读取消，须在那道读之前取同一把锁，否则并发窗口重新打开。
2. **锁等失败与读口读不回同一格**（代决 3），运维从原因码分不出是锁等超时、连接断了还是库读不回。
3. **锁持到外层事务提交。** 取消那一笔里授权读也在锁内（代决 2）；采用那一笔里资格判断与发布意图入队在锁内；面单判断那一笔里交易册、继续尝试登记册、有效期规则与终局采用在锁内。都只挡同一件包裹，别的包裹只在散列撞键时多等。
4. **面单渠道服务的首次有效收寄登记不在锁里。** 取消读 TF 登记册时可能恰好错过一份刚提交的收寄登记，取消照常成立；随后那封收寄信触发的面单判断在锁里读到取消，按业务时间落第 2 或第 3 格。这与 ADR-0254「谁先越过提交边界」一致——PS 一侧越过边界的是面单判断，不是 TF 的登记——但客户可能看到「已取消」，而 TF 那边的收寄登记早了几毫秒。
5. **没装锁不响亮**（代决 4）：装配漏接时取消、采用、面单判断永远停在未决，只有装配真库用例拦得住。
6. **同一包裹的两路采用或两路面单判断也随之排队。** 两份来源抢同一件包裹的责任起点时，后到的那一路不再撞链索引答未决、重试后才收敛，而是读到先到的责任起点直接按「先合法形成者保留」答不采用（或接更正链）；结果与重试收敛后相同，少一轮重投。
7. 今天走不到：取消授权停在未配置，取消形成不了；锁照样每次都取（代决 2），行为上不可见。

### 迁移

无（咨询锁不改表）。开工时核 `migrations/parcel_shipment/` 最大号 0030，未取号。

### AT

- 未立新号。`AT-PS-079`、`AT-PS-086`、`AT-PS-107`、`AT-PS-108` 四行补点名（并发半边）；`go run ./tools/at-coverage-counts -check` 退 0（行数不变，总览不需重算）。
- 未重生成 ADR 索引（未动 ADR）与 `docs/product/MECHANISM-INVENTORY.md`（多一个端口与适配器，由推送方生成）。

### 验证（代码同 `2e516115e`）

- `go build ./...`、`go vet ./...` 退 0，`gofmt -l cmd internal migrations tests tools` 无输出。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh` 范围：改动包（PS `ports`、`application`、`adapters/postgres`）的反向依赖（`go list -test` 算，36 个，含 parcel-commercial、parcel-frontline-import、parcel-settlement-register、tests/bentocontract、CC/NR/NO/PP/SA/VE 各自的 PS 适配器）加 `./internal/parcelshipment/...`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./internal/architecture/...`：在 main `58203916a` 上退 0（36 个包 ok、2 个无测试文件）；rebase 到 `803b97e5f` 后 35 个包 ok，`cmd/parcel-dispatch` 的 `TestATransportPreparationUndecidedRedeliveryDoesNotReprojectVisibility` 在包并发下红一次（TF 运输准备与可见性投影，与本票无关；demo-chain-gaps/57 记过同一条是并发负载下的偶发），单独 `-count=5` 与整包重跑都绿；再 rebase 到 `f97c265cc` 后退 0（36 个包 ok、2 个无测试文件，反向依赖重算未变）；末次 rebase 到的 `610368f1e` 只比它多一笔票面 `.md`，未重跑。含 PBC-08 负面证据门禁。
- 并发与新用例 `-count=5`（PS `adapters/postgres` 五条真库并发、两条锁本身，`application` 三条编排单元）与装配真库用例（`TestTheWiredCancellation*`、`TestALateEarlier*`）`-count=5`：三个基上各跑一遍，都 ok。
- 变异（rebase 前在 main `58203916a` 上逐个施加，被变异的代码与 `2e516115e` 相同；跑 PS `adapters/postgres` 与 `application` 的新用例，装配那一组跑两个命令包，跑完从暂存副本还原并比对一致）：
  - M1 去掉采用那一处锁：采用先手、取消先手两条真库用例与采用单元用例红；
  - M2 去掉面单判断那一处锁：两条面单真库用例与单元用例红；
  - M3 去掉取消那一处锁：四条交叉真库用例、重发用例与取消单元用例红；
  - M4a 取消的锁挪到全部边界读之后：同 M3；M4b 采用的锁挪到读取消之后：取消先手对采用那一条与单元用例红；M4c 面单判断的锁挪到全部读之后：两条面单真库用例与单元用例红；
  - M5 锁键不含包裹、M5b 锁键不含租户：`TestTheCancellationBoundaryLockQueuesOnlyTheSameParcelOfTheSameTenant` 红；
  - M6 取消的锁放在幂等读之后、终局读之前：重发用例与取消单元用例红；
  - M7 两个命令的装配不接锁：见代决 4 那四条装配真库用例红。
- 泄漏：在自起的一次性 PostgreSQL 16.14 上跑五条真库并发用例与锁用例 `-count=5`（rebase 前后各一遍），跑完库上无 `parcel_test_*` 残留、无客户端会话、无咨询锁；本票所有真库跑都在 `scripts/test-full.sh` 或自起的一次性实例上，未碰 55432。

### 未决

- 网络服务交付终局采用不读已成立的取消：[code/98](./98-a-network-delivery-final-is-formed-over-a-standing-cancellation.md)。

## 进 main 记录（2026-10-26，协调方）

- 分支 `claude-code77` 由协调方 rebase 到 origin/main `51aca18cb`（期间 main 进了另一会话 TF 的 demo-chain-gaps/177，与本票不相交，无冲突）后快进，tip `bab5f6615`。无迁移、无 ADR。
- 隔离评审（Sonnet）可合入，0 阻断 0 N。评审核实取锁位置在三处编排各自全部边界读之前、生产装配一定接上 `Cancellations`、锁序无反例、并发用例修前全红且有期限；分隔符变异只造成无害的多排队。
- 评审另确认一处票外问题：撤回、主动拒绝、授权处置先锁委托行再取控制作用域锁，与接受链次序相反，同一委托并发可死锁——协调方立 code/99（R2，needs-triage），本票不改。
- 协调方单独提交重生成的机制清点；ADR 索引与 AT 计数无变化；检查均退 0。
- 全量 `scripts/test-full.sh` 退 0、158 个包 ok；`go build ./...`、`go vet ./...` 干净。
- 去向：code/98（网络服务交付终局不读已成立的取消，R2）、code/99（取锁次序相反可死锁，R2）。
