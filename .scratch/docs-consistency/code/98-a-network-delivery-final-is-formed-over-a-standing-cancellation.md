# 98 网络服务的交付终局采用不读已成立的取消：取消在册时，迟到的有效交付仍形成非取消终局

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code98（见文末「完成记录」）。此前 needs-triage——建议 R2（上线后尽快）：一件包裹可能同时带着取消终局与非取消终局，委托完成汇总把它算成履约终局。顺序发生即可到达，与并发无关。今天取消授权与终局规则都停在未配置，取消形成不了、交付终局也形成不了，这一格还到不了；租户把两样都登记之后即生效。2026-10-26 code/77 落地时核「交付终局要不要进取消边界锁」所得。
Blocked by: 无
出处：[code/77](./77-cancellation-intake-adoption-and-label-final-are-not-serialized-per-parcel.md)「要裁的」一节的核查（完成记录代决 1）。

## 现象

网络服务的有效交付经 `AdoptOnEffectiveDeliveryAdapter` 交给终局采用编排 `FormParcelFinalHandler`。编排的「既有终局分派」只问终局库的当前有效终局（`Finals.FindCurrentFinal`），不问取消库里已成立的取消；取消读口只用在委托完成摘要的派生上。

一件包裹在收寄之前已经合法取消，实物却仍进了网络（收寄采用答 `SOURCE_NOT_ADOPTED`/`CANCELLED_BEFORE_INTAKE`，或停在 `CANCELLATION_ORDER_CONFLICT`，都没有责任起点）。此后 transport-fulfillment 若登记了这件包裹的有效交付，终局采用照常判规则、形成非取消终局：

- 包裹同时带着取消终局与非取消终局；
- `deriveCompletion` 先读终局库，这件包裹在汇总里算履约终局、不算取消；
- 停在冲突上的包裹（ADR-0254 决定二第 3 格「不形成责任起点也不形成非取消终局」）由交付一路绕过冲突形成终局，冲突册上那一行没人再看。

这不是并发问题：交付一路不读取消写下的任何东西，先后怎么排它都照样形成终局。code/77 的取消边界锁因此没有收它（见那张票的完成记录）。

## 证据

- 代码：`internal/parcelshipment/application/form_parcel_final.go` 的 `Handle`，既有终局分派只调 `Finals.FindCurrentFinal`；`Cancellations` 只在 `deriveCompletion` 里读。`internal/parcelshipment/adapters/transportfulfillment/adopt_on_effective_delivery.go` 按包裹反查当前已接受委托，不看取消。
- 文档：
  - UC-PS-004 应用流程「校验包裹身份、接受基线、服务类型、谱系和既有终局｜被替代/越权来源不采用」；
  - PS CONTEXT「取消或依据适用终局规则形成的包裹级失败结果可以在责任起点或正式承诺尚未形成时成为终局」——取消本身就是一份终局；
  - UC-PS-006 `AT-PS-080`「取消不被覆盖」；ADR-0254 决定二第 3 格；
  - UC-PS-003 启动条件「已经取消、已经终局或被身份谱系替代的包裹仍可产生真实节点或运输事实。该事实必须保全，但不能自动重开服务」。

## 三类

机制（既有终局分派漏读一处）。交付到达时落哪一格（不采用，还是并入冲突）可能要 PS owner 定，见「要裁的」。

## 规模

S：终局采用编排在既有终局分派处读已成立的取消；交付一路由此读了取消写下的东西，要与取消一起进 code/77 那把按包裹的取消边界锁（锁在这一道读之前取）；补用例（顺序一条、真库并发一条）。

## 最小的诚实修法

1. `FormParcelFinalHandler` 的既有终局分派同时读已成立的取消：取消在册即不采用，依据指名那份取消（如 `PARCEL_CANCELLATION/<取消标识>`），来源与物理事实保留。
2. 交付一路在这道读之前取 `ports.CancellationBoundaryLock`，与取消、收寄采用、面单渠道终局判断排同一个队；面单渠道一路已在判断前取过，同一事务重入。
3. 补用例：取消在册后到达的有效交付不形成终局；取消与交付并发时两种先后各一遍。

## 要裁的

- 取消在册时到达的有效交付答哪一格：按取消在先不采用，还是当收寄冲突仍未解开时并入冲突、停着等决定（code/76 的决定面）。

## 要裁的（协调方已裁，用户授权自决）

取消在册时到达的有效交付**一律不采用**，依据指名那份取消，来源与物理事实保留；停在 `CANCELLATION_ORDER_CONFLICT` 的包裹同样不采用（ADR-0254 决定二第 3 格「不形成责任起点也不形成非取消终局」，交付一路不得绕过冲突；冲突由 code/76 将来的决定面解开，本票不做）。

## 完成记录（2026-10-26，分支 claude-code98，rebase 于 origin/main `4292299b8`，下文 SHA 均为 rebase 后的）

### 做了什么

- `48d0c944e` 代码与用例：
  - `FormParcelFinalHandler`（`application/form_parcel_final.go`）既有终局分派前读已成立的取消（`Cancellations` 读口，原先只在 `deriveCompletion` 里读）：在册即答 `SOURCE_NOT_ADOPTED`，依据 `PARCEL_CANCELLATION/<取消标识>`（形同取消编排答既有取消时用的那一串），来源（种类与版本）保全在不采用记录上，照常交下游一份不采用意图。复用既有结果格，没有新增结果格、原因码或迁移。
  - 取消判断在终局规则判断之前、也在既有终局分派（同源重派生 / 异源不采用）之前：规则放行与否、是否配置都不改它；更正出的新版本（新来源版本）到达同样不采用；同一份交付重投答 `EXISTING_RESULT` 带原依据。
  - 读之前取 `ports.CancellationBoundaryLock`（锁空间 `parcel_shipment.parcel.cancellation_boundary`，经 `RequireExecutor`），**放在幂等读（`Finals.FindByKey`）之前**，随外层事务持到提交。取不到锁、没装锁、取消读不回，答 `FINAL_UNDECIDED`/`FINAL_STORE_UNAVAILABLE` 带续办引用，什么都不读、不落、不交意图。`Cancellations` 为 nil（取消机制未接入）时不读也不取锁，沿 code/77 代决 6。
  - 读口答了「有取消」却指不出是哪一份、或交回别的包裹的取消：响亮报错（同面单渠道判断的先例），不当作没有、不编依据。
  - 装配：`cmd/parcel-dispatch/assemble.go` 的 `parcelFinalAdoptionChain` 接上同一把锁（有效交付与面单判断两条链共用这只编排；面单判断判断前已取过，同一事务里再取是重入）。未动 `cmd/parcel-api`、`assemble_submission.go`、`production_ownership.go`、首方五口的 `endpoints_*.go`、`CancelParcelHandler` 的行为。
  - 没有改 `adopt_on_effective_delivery.go`：锁与读都在它转交的编排里，适配器只翻译、不判断；其测试夹具补了锁替身与两格新用例（见下）。
  - 端口头注（`ports/cancellation_boundary.go`）补了锁序一段并把「三处」改成几处；`cancel_parcel.go` 里 `lockCancellationBoundary` 的一行注释同步（仅注释，未碰逻辑，code/101 动该文件时可能在这一行相邻处冲突，取两边即可）。
- `db2c8795e` 文档：PS CONTEXT 补规则句、取消提交的排队一条扩到网络服务履约结果的终局采用；UC-PS-004 流程第 3 步、`SOURCE_NOT_ADOPTED` 一行、终局形成规则、一致性与并发各补一条，交接表记一行（冲突解开后此前被不采用的履约结果不会自动重判，归 code/76）；UC-PS-006 并发一条同步；`.scratch/at-coverage-inventory.md` 给 `AT-PS-053`、`AT-PS-061`、`AT-PS-080`、`AT-PS-086`、`AT-PS-107` 补点名并追记（不立新 AT 号）。

### 用例（先红后绿）

- 编排单元（`application/form_parcel_final_test.go`）：`TestADeliveryOverAStandingCancellationIsNotAdopted`（指名取消、来源保全、不留当前有效终局、规则未配置也答不采用、重投答已有结果带原依据、更正版本仍不采用、下游一份不采用意图、完成摘要以取消终局计入汇总）；`TestADeliveryWithoutACancellationStillFormsTheFinalAfterReadingTheCancellationView`（不回归）；`TestTheFinalAdoptionQueuesOnItsParcelBeforeReadingTheCancellation`（锁指名命令的（租户，包裹）、取锁那一刻取消与终局库都还没读、未受理的探测不取锁、取不到锁与没装锁一道不读不落不交、取消机制未接入不读不锁）；`TestAnUnreadableOrBrokenCancellationViewNeverLetsTheDeliveryThrough`（读不回为未决、指不出是哪一份与别的包裹的取消为报错）；`TestACancelledSiblingCountsTowardCompleteness` 改成取消的是另一个成员（原先取消视图对任何包裹都答同一份，现在会被不采用）。
- 适配器（`adapters/transportfulfillment/adopt_on_effective_delivery_test.go`）：`TestFinalOutcomesMapToConsumptionSlots` 补两格——取消在册的包裹交付信封入账且包裹上没有当前有效终局、只有一份依据指名取消的不采用记录；取不到边界锁回滚（`ErrFinalUndecided`/`FINAL_STORE_UNAVAILABLE`）。
- 真库（新文件 `adapters/postgres/delivery_cancellation_race_test.go`，夹具沿 `cancellation_boundary_race_test.go` 的 `boundaryRig`，池外连接轮询 `pg_locks`、每步十秒期限）：取消先手/交付后手（`TestACancellationAheadOfAnEffectiveDeliveryIsSeenByTheFinalAdoption`）、交付先手/取消后手（`TestAnEffectiveDeliveryFinalAheadOfACancellationIsReturnedAsTheStandingFinal`，取消答 `FINAL_OUTCOME_STANDS` 指名那份终局）、同一份交付并发重投（`TestAConcurrentReplayOfOneEffectiveDeliveryAnswersTheExistingResult`，钉住锁在幂等读之前）、停在冲突上的包裹交付不绕过冲突（`TestAnEffectiveDeliveryDoesNotGetPastACancellationOrderConflict`，顺序用例，冲突册一行与取消原样不动）、没有取消照常形成终局并在随后的取消上答既有终局（`TestAnEffectiveDeliveryOnAParcelWithoutACancellationFormsTheFinalAsBefore`）。修前三条交叉/冲突用例红（交付径直形成终局，或取消径直成立）。
- 生产装配（新文件 `cmd/parcel-dispatch/cancelled_parcel_delivery_final_test.go`，经生产 `wireDispatcher` 与 SYN-V0 夹具）：`TestAnEffectiveDeliveryOverAStandingCancellationIsConsumedAsARefusal`、`TestAnEffectiveDeliveryDoesNotGetPastAStandingCancellationOrderConflict`（先让节点收寄停在冲突上，再投交付信封）：信封无失败码、inbox 一行、终局库恰一行不采用且依据指名那份取消、无已采用终局；冲突与取消不动。
- 既有夹具跟着补锁：`cancellation_boundary_race_test.go` 里面单判断的采用编排、`delivery_outcome_test.go`、`adopt_on_effective_delivery_test.go`（它们都接了 `Cancellations`，没装锁会停在未决）。

### 代决（用户授权「你作为业务和系统专家自决」，协调方派单；除派单已裁两条外）

1. **取消在册 / 停在冲突上都答 `SOURCE_NOT_ADOPTED`、依据 `PARCEL_CANCELLATION/<id>`，不新造结果格**（派单已裁）。冲突那一格不另读冲突册：读到取消就够了，冲突册只是同一份取消上多一行待决事项。
2. **取消判断放在终局规则判断与既有终局分派之前**：「一律不采用」落到最硬的一格——规则未配置时不停在 `FINAL_RULE_UNCONFIGURED` 等一个永远不会开的终局；更正版本不会因为「同源」走重派生而绕过取消。代价：库里若已有取消与非取消终局并存的包裹（真库锁下走不到、取消授权生产也未配置），此后到达的更正会被不采用而不是重派生；这一次序由编排单元用例 `a standing cancellation is judged before the final that already stands` 用替身钉住。
3. **锁放在编排里，不放在适配器里**：读与锁同处一处，与 code/77 三只编排同形；面单判断那一路经同一只编排落终局，重入无害，其余将来的调用方也不会漏。锁在**幂等读之前**：同一份交付并发重投时后手要等先手落记录再读，读到已有结果；放在幂等读之后，后手会把先手刚形成的终局当同源新版本去重派生同一版本（变异 M5 实测红）。
4. **取不到锁、取消读不回都归 `FINAL_STORE_UNAVAILABLE`，不新增原因码**（ADR-0029，恢复动作同为重试；code/77 代决 3 的先例）。终局编排的原因集里没有「取消视图」专格，既有终局分派本来就读终局库，取消库是同一个 `parcel_shipment` 库里的另一张表；这一格的注释写明含三种读不回。
5. **`Cancellations` 为 nil 时整段不做**（不读、不锁），沿 code/77 代决 6；`Cancellations` 接入而 `BoundaryLock` 为 nil 按取不到锁办，不 panic。
6. **读口坏了响亮报错**：答有取消却指不出是哪一份、或交回别的包裹的取消，不当作没有（会让交付盖过真取消），也不编依据。
7. **不立 ADR-0288**。核过：ADR-0254 决定二第 3 格「不形成责任起点也不形成非取消终局」本就是这一格的口径，本票只是让交付一路也守它；没有改、接、扩、按口解、收窄任何已接受 ADR 的条款。该 ADR 的三格枚举的是收寄采用与承运商首次有效收寄两种边界事实，有效交付不在其列——交付不是边界事实，是边界之后的履约结果，包裹上取消成立就不该再长出非取消终局，这是 CONTEXT「取消或依据适用终局规则形成的……」与「终局形成后不得回退为取消」的对偶，不是新口径。无其它记录裁过交付与取消的先后。
8. **不立新 AT 号**：场景已被 `AT-PS-080`（取消不被覆盖）、`AT-PS-107`（冲突停着）、`AT-PS-086`、`AT-PS-061`、`AT-PS-053` 覆盖，补点名；避免与并行作者在 AT-PS-125 起撞号。
9. **无迁移**（咨询锁与既有终局库不改表）。

### 越权风险点（待 PS owner 复核）

1. **不比业务时间**（派单已裁）：取消在册时到达的交付，不论交付发生在取消的业务时间之前还是之后都不采用。发生早于取消业务时间的交付（取消形成时包裹其实已送达）按 ADR-0254 的比较口径更像第 3 格的事实冲突；但交付以责任起点为前提，责任起点在取消在册时只会是「没有」或「冲突」，这种交付先要有一份被冲突拦下的收寄。若 PS owner 想让它们并入冲突册（停着、等决定），需另立票改成「记冲突、答未决」。
2. **不采用记录按（包裹，结果种类，来源版本）永久保全**。冲突被 code/76 的决定解开（取消不成立）之后，此前被不采用的交付**不会自动重判**；同一版本重投只答已有结果、原不采用记录原样返回，所以决定面解开冲突时必须能作废或重判依据该取消的不采用记录（评审 N1 后写进 code/76 的最小修法与 UC-PS-004 交接表）。
3. **`FINAL_STORE_UNAVAILABLE` 一格含终局库、取消库、锁三种读不回**，运维从原因码分不出是哪一种（同 code/77 越权风险点 2）。
4. **有效交付采用从幂等读起就取锁**（含重放），锁持到外层事务提交：同一包裹的交付、面单判断、收寄采用、取消排同一个队；锁内包含终局库读写、规则判断（读 PC 声明）、发布意图入队与完成摘要派生。只挡同一件包裹，别的包裹仅在散列撞键时多等一会儿。
5. **没装锁不响亮**：装配漏接时 parcel-dispatch 的每份有效交付都停在未决（回滚重投），只有装配真库用例（上面两条）拦得住；变异 M7 实测红。
6. **锁序**（全序论证在 code/77 完成记录，本票已写进端口头注）：消费门认领 inbox 行 → 边界锁 → 本笔行写入 → outbox 插入。交付一路锁之前只有 inbox 认领、委托反查与 `FindBySourceIdentity`（普通读，无 `FOR UPDATE`），锁之后只有终局库插入、发布意图入队与读；不更新委托行、不取作用域锁与来源身份锁；一笔事务至多持一个本锁键。未发现反例。
7. **给 code/101 的话**：委托生命周期闸门若也加在 `form_parcel_final.go`，它的读要放在 `lockCancellationBoundary` 之后（闸门读的是会被取消/撤回改写的状态），并保持「取不到锁什么都不读」。
8. 今天走不到：取消授权停在未配置，取消形成不了；锁照样每次都取。

### 迁移 / ADR / 新票

- 迁移：无。ADR：未立（预留的 ADR-0288 用不上，依据见代决 7）。新票：无。

### 验证（代码同 `48d0c944e`）

- `go build ./...`、`go vet ./...` 退 0，`gofmt -l cmd internal migrations tests tools` 无输出；`go run ./tools/at-coverage-counts -check` 退 0，`go run ./tools/adr-index -check` 退 0。
- `scripts/test-full.sh` 范围：改动包（PS `application`、`adapters/postgres`、`adapters/transportfulfillment`、`ports`）经 `go list -test` 算出的反向依赖（20 个包，含 `cmd/parcel-api`、`cmd/parcel-commercial`、`cmd/parcel-dispatch`、`cmd/parcel-frontline-import`、`cmd/parcel-settlement-register`、`tests/bentocontract`、VE 的 PS 适配器与 inbox）加 `./internal/parcelshipment/...`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./internal/architecture/...`、`./migrations/...`：rebase 前后各一遍，全部 ok（含 PBC-08 门禁）。
- 新用例 `-count=3`（编排单元、有效交付适配器、装配两条），并发与真库用例 `-count=5`（`adapters/postgres` 里 `EffectiveDelivery`、同一份交付并发重投共五条），都 ok；真库都在 `scripts/test-full.sh` 自起的一次性 PostgreSQL 上，未碰 55432。
- 变异（私有目录备份被改文件，跑完拷回并 `cmp` 一致；每个跑编排单元、`adapters/postgres`、`adapters/transportfulfillment`，M7 另跑 `cmd/parcel-dispatch` 的装配用例）：
  - M1 不读取消：单元 `TestADeliveryOverAStandingCancellationIsNotAdopted`、`TestAnUnreadableOrBrokenCancellationViewNeverLetsTheDeliveryThrough`，真库取消先手对交付与冲突停着两条，适配器入账格，红；
  - M2 交付不取边界锁：单元取锁用例，真库取消先手对交付、交付先手对取消、同一份交付并发重投三条，适配器取不到锁格，红；
  - M3 取消在册却仍形成终局（找到取消但不走不采用）：单元、真库取消先手与冲突停着、适配器入账格，红；
  - M4 锁挪到读取消之后：单元取锁先于读，真库先后两条与重投，红；
  - M5 锁挪到幂等读之后、读取消之前：真库同一份交付并发重投红（单元与其余绿，由这一条独守）；
  - M6 取消读不回时放行（当作没有取消）：单元读不回用例红；
  - M7 终局链装配不接锁：`cmd/parcel-dispatch` 两条装配用例红。
- 修前（先写用例、未改行为时）：单元新用例、真库三条交叉/冲突用例红，不回归一条绿。

### 评审记录与修订轮

独立评审结论：可合入，合入前补三个小项，已在后续提交补完。
- N1：评审指出 code/76 的重判入口没列网络服务履约结果（有效交付）；已在 code/76「最小的诚实修法」补一条（决定面解开冲突后须能作废或重判依据该取消的不采用记录，只等重投不够），并同步 UC-PS-004 交接表那一行与上文越权风险点 2 的措辞。
- N2：评审变异「取消判断挪到既有终局分派之后、规则之前」全部测试仍绿，因为已有同种终局与在册取消并存在真库锁下走不到；已在 `form_parcel_final_test.go` 加替身子用例钉住先后次序，同一变异下该子用例红、其余仍绿，从备份拷回并 `cmp` 一致。
- 措辞：去掉 `Handle` 注释与失败信息、`cmd/parcel-dispatch` 重拍注释里的「不再」「照旧」类变更口吻，改成现行规则句。
