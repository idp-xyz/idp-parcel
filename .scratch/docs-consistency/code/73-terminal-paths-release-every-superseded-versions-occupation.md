# 73 撤回、拒绝这类终局路径也放掉被取代各版的占用

Category: bug
Status: resolved——2026-10-21 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-code72（见文末「完成记录」）。此前 ready-for-agent——R2（上线后尽快）：撤回、主动拒绝、授权处置拒绝、自动拒绝都只放当前提交版本那一笔。被取代版本那一笔若还没放（取代之后接受链还没有走成一轮，或那一轮回滚了），委托到了终局之后就再没有轮次去放它，成了孤儿。2026-10-21 协调方据 code/20 隔离评审立。
Blocked by: 无
出处：code/20 隔离评审；code/20 完成记录。

## 现象

code/20 把被取代版本的释放放在接受链每一轮的开头（`AdvanceAcceptanceChainHandler.releaseSuperseded`）。委托若在新版本那一轮链提交之前就走到终局，被取代版本那一笔就没有人放了。终局路径的释放只读本版判断：
- `WithdrawShipmentRequestHandler.releaseOccupation`；
- `RejectShipmentRequestHandler` 的释放；
- `DisposeShipmentRequestHandler` 的释放；
- `FormAcceptanceDecisionHandler.releaseIfRejected`。

## 证据

- [UC-PS-005](../../../docs/application/parcel-shipment/UC-PS-005-WITHDRAW-SUBMITTED-SHIPMENT-REQUEST.md) 步骤 6：按原业务关联幂等释放已成立各项的占用。这一句说的是这份委托已成立的占用，不限于当前版本。
- PS CONTEXT 生命周期「已提交期间形成新提交版本」：被取代版本已成立各项的占用按原业务关联释放（code/20 补）。

## 三类

机制。无产品策略、无租户取值。

## 规模

S–M：把 `releaseSuperseded` 的逐版逻辑抽成可复用的一处，终局各路在释放本版之后再遍历 `PriorSubmissionVersions()`；补偿引用按版本各派一条。补用例。

## 最小的诚实修法

1. 逐版释放抽成一个包内函数，入参是判断读口、释放端口、委托与命令范围。接受链与终局各路共用这一处，一决策一处定义。
2. 终局各路在本版释放之后遍历被取代各版，按 `OccupationFormed()` 释放。失败照 `ControlReleasePending` 留补偿，不回滚终局决定（UC-PS-005「撤回提交成功后不等待财务释放才生效」）。
3. 用例：补件之后、新版本那一轮链提交之前撤回，两版的占用都释放；拒绝一路同此。

## 要裁的

无。

## 完成记录（2026-10-21，分支 claude-code72，基于 main `32dc1d98d`，评审后 rebase 于 main `e0d0909cf`，下文 SHA 均为 rebase 后的；与 code/72 同一分支）

### 做了什么

- `7131d4c56` 逐版释放抽成包内 `occupationRelease`（`internal/parcelshipment/application/release_formed_occupation.go`）：入参是判断读口、释放端口、来源身份与委托；`releaseFormed` 放一版已读回的控制结果，`releaseRecorded` 逐版读回再放，`releaseEvery` 放命令指名那一版与其余各版，`reference` 派生续办引用。接受链开头的 `releaseSuperseded`（code/20）、自动拒绝 `releaseIfRejected`、主动拒绝、撤回、授权处置的释放都改走它；三份 `compensationReference` 删去，引用只在这一处派生，范围与原来逐字一致（原因 + 租户、客户账户、委托、版本〔、控制关联〕）。
  - 终局各路先放命令指名那一版，再放委托的当前版本（与指名不同时）与被取代各版；一版放不掉或读不回不停下，接着放其余各版，补偿引用交第一处；终局决定不回滚。
  - 编排用例（`terminal_release_superseded_test.go`）：补件后撤回两版都放；被取代那一版放不掉撤回照样成立、引用非空，本版放不掉时引用不同且被取代那一版照样去放；命令指名旧版本时当前版本也放、每版一次；被取代那一版没占用不放；重复撤回两轮各放两版、补偿引用与首次一致；主动拒绝、自动拒绝、授权处置两个去向同此。释放替身加 `errFor` 按版本注入失败。
  - 派发真库用例 `cmd/parcel-dispatch/customer_supplement_pause_release_test.go::TestAWithdrawalBeforeTheNewVersionsChainReleasesTheSupersededFreeze`（随 `050f920de` 提交，因它要靠 code/72 的暂停释放注入失败才摆得出场景）：暂停那一次释放放不掉、首版那一笔留着；补件之后、新版本那一封驱链之前撤回，撤回编排用生产来源仓储、委托仓储、判断账与 PS→SA 适配器，首版冻结由撤回放掉、不留补偿引用、新版本不占。
- `52f6054b6` UC-PS-001、UC-PS-005「给开发的交接」各补一格已补行（UC-PS-005 步骤 6 与 `AT-PS-073`；UC-PS-001 `AT-PS-035` 终局路径那一格）。PS CONTEXT 不改：词条「控制项结果」与生命周期「已提交期间形成新提交版本」已要求被取代版本的占用按原关联释放，落在哪一步属机制。
- `ee492a101` AT 盘点 `AT-PS-035`、`AT-PS-068`、`AT-PS-073` 行补点名，三行仍在①，总览计数未动；撤回放不掉那一条用例的 Covers 补 `AT-PS-073`。

### 代决（用户授权「你作为业务和系统专家自决……解决所有的问题」）

1. **一处定义用包内结构体加方法，不用一个长参数的自由函数。** 各路要的是同一组口与范围上的几个动作（放一版、逐版放、派引用），收进一个值里，各路只决定放哪几版、没收口时停下还是留补偿。
2. **终局放的是委托的全部版本，不只是「本版 + 被取代各版」。** 撤回与主动拒绝的命令不核对版本：命令指名旧版本时，只按「本版之后遍历 `PriorSubmissionVersions()`」会漏掉当前版本那一笔。命令指名那一版排第一，其余去重随后。
3. **一版没收口不停下，补偿引用只交第一处。** 结果只有一格补偿引用（HTTP 契约同），不扩成多格；终局之后再没有轮次，停在第一处会让后面几笔等第一处续办成功才有人问。接受链开头那一步随之也是「放完再停」：放不掉时整轮回滚，多放的随之撤回，停下的结论与续办引用不变（code/20 时是放到第一处就停）。
4. **自动拒绝与授权处置`交客户补充`也遍历其余各版**，尽管同一轮链开头（或本版那一轮链开头）已放过：多一次请求、提供方按幂等答原释放，换来各路同一条路、不依赖调用方的轮次顺序。
5. 无迁移，无 ADR，装配一行不改（照 ADR-0222，各释放口原样复用）。

### 越权风险点（待 PS owner 复核）

1. **多版放不掉时只交回一个补偿引用。** 第二处起没有引用透出；它们的续办靠下一次触达（撤回重放会逐版重放；拒绝、处置之后没有）。真要每版各一条，得扩结果与 HTTP 契约。
2. **`cmd/parcel-api` 的撤回、拒绝、处置三处释放口今天没有作用域源**（code/20 越权风险点 5、code/70 越权风险点 2，既有）：在那里被取代版本有占用时，逐版释放必然留补偿，被取代那一笔要等别的路收口。本票的真库用例在派发侧夹具里装了作用域源，证的是机制。（2026-10-26 注：由 [code/71](./71-release-and-control-ports-answer-a-closed-set-instead-of-a-bare-error.md) 回答——释放按施加时随控制结果存下的作用域认领，三处释放口不再需要作用域源。）
3. **释放的库错误曾会中止外层事务——已在本票内收住（code/72 评审 N1）。** parcel-api 撤回、主动拒绝、授权处置三处释放口与 parcel-dispatch 形成决定那一步都包了 `pspostgres.SavepointedControlRelease`，某一版的释放在库里报错时退回 savepoint，后面几版照放、终局决定照样提交。隔离靠装配兑现，编排不校验。
4. 每次终局多若干次释放请求（每个占过的版本一次，含作用域重解、账本读与作用域锁）；已释放的答原释放，不回写（code/20 止血、code/70 写口）。
5. 存量：首发前只有演示租户；既存孤儿 `--reset` 重灌。

### AT

- `AT-PS-035`：终局路径那一格补编排三条；`AT-PS-068`、`AT-PS-073` 各补编排一条。行都在①，总览计数未动。派发真库那一条（撤回放掉被取代那一版）没有恰好对应的 AT 行，未点名。
- 未重生成 `docs/product/MECHANISM-INVENTORY.md`、ADR 索引与 AT 计数。

### 验证（代码同 `050f920de`）

- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/parcelshipment/... ./internal/settlementaccounting/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./tests/bentocontract/... ./internal/visibilityexception/adapters/inbox/ ./internal/architecture/... ./migrations/...`：退 0，38 个包 ok、3 个无测试文件、0 FAIL。PS `application` 的反向依赖（`go list -test` 算）都在这一组里，组外只多出 VE `adapters/inbox` 的测试，已一并跑。
- 新用例与改动用例 `-count=3`（PS `application` 释放相关用例、`cmd/parcel-dispatch` 受控补充与授权处置相关真库用例）：ok。
- 变异（逐个施加、各跑一遍）：去掉暂停那一处释放，编排两条、派发真库三条（本票两条与 code/20 那一条）变红；终局各路只放命令指名那一版，编排六条、派发真库一条变红；自动拒绝不放其余各版，编排一条变红。恢复后绿。
- `go build ./...`、`go vet ./...`、`gofmt -l` 干净。
- 演示种子：本票不碰种子与登记 CLI 的任何输入或路径（种子只灌主数据登记，不走接受链、撤回、拒绝与处置），未灌。无迁移。

## 评审记录

### 第 1 轮（隔离评审，协调方转达；code/72、73 同一轮）

没有阻断。逐条去向：

| 条目 | 去向 |
|---|---|
| N1 暂停那次释放与暂停同在一笔事务里，没有保存点：评审在真库上让释放执行 `SELECT 1/0`，`Consume` 报 25P02、委托回到`已提交`、投递没入账；`等待受控补充`入账从此要看 SA 账本库是否健康，是本票引入的活性退化 | 本票内修：`cb2e5dfb2` 新增 `pspostgres.SavepointedControlRelease`（`internal/parcelshipment/adapters/postgres/control_release_savepoint.go`），内层答错误即退回 savepoint、原样交回，成功 RELEASE，事务外直通，做法同 NR 的 `SavepointedRouteCandidateCosts`。装配改最小一行包上它：`cmd/parcel-dispatch/assemble.go` 形成决定那一步的释放口（接受链开头放被取代各版、暂停、自动拒绝都经它），`cmd/parcel-api` 的 `assemble_withdrawal.go`、`assemble_review.go`（主动拒绝）、`assemble_disposition.go` 各一行；endpoints、main、routes 未动。真库用例：裸接中毒释放口时暂停随 25P02 丢失（前提）；包上之后暂停照样落库、本份投递入账、直驱形成决定那一步交回补偿引用；撤回照样成立、库里为已撤回、交回补偿引用。适配器真库用例两条（失败、失败、成功各自退回后随后的写照常提交；事务外直通与少接一件）；`5e016913b` 补 PBC-08 门禁要的无事务负向证据。在 [code/81](./81-savepoint-decorators-are-three-copies-of-one-helper.md) 票面登记为又一份副本。原越权风险点 3「并入 code/71」撤回：端口改成封闭集撤不掉已中止的事务 |
| N2 越权风险点 2 的理由不符事实（判断账主键含 `resolution_id`，可达性每轮重问 NR，不是「同键不覆盖」） | 已改写：保护是两层——没有驱动方会再驱停在受控补充的版本；真被再驱时控制那一步重施加拿回已释放的冻结、`controlItemFor` 答 `ErrUntranslatableAnswer`、折成 `FINANCIAL_CONTROL_UNAVAILABLE`，链在 `Decide` 之前停。补残余：这样的再驱停在内部重试、一直重投。第二层由派发真库用例 `TestARedrivenPausedVersionStopsAtTheReleasedFreeze` 钉住 |
| N3 两处说法 | `9c1e6d1ad`：`FormAcceptanceDecisionResult.CompensationReference` 注释补上停进`等待受控补充`那一格；暂停那一路的补偿引用被链丢掉、没有持久记录，`pauseForExternalResume` 注释、PS CONTEXT 新句、UC-PS-001 交接行都改为照实写它的两条兜底（补件后链开头再请求一次、撤回逐版释放），不再写「补偿续办」 |
| N4「补偿引用交第一处」没有用例钉住 | `82bafeb10`：撤回、主动拒绝、自动拒绝、授权处置各一条「两版都放不掉，交回的引用等于只有指名那一版放不掉时那一个」；`occupationReleaseGap.or` 改成后者胜出时四条全红 |
| 信息：授权处置选`交客户补充`后再驱同一版本，`Decide` 会再放一次（幂等），「两条路认领同一笔」不严格 | 注释与代决 1 已改为按 ADR-0132 决定一的释放时刻说理；记为 code/72 越权风险点 6 |
| 信息：链先取作用域锁再写委托行、撤回先写委托行再取作用域锁，73 略有放大 | 记为 code/72 越权风险点 7 |
| 信息：ADR-0132 越权风险点 4 现在同时管两处 | 记为 code/72 越权风险点 8 |

AT 盘点：`AT-PS-035` 行再补三条路两版都放不掉的编排用例，`AT-PS-073` 行再补撤回两版都放不掉的编排用例与 `TestAPoisonedWithdrawalReleaseStillCommitsTheWithdrawal`；行仍在①，总览计数未动。

验证（rebase 于 main `e0d0909cf` 之后，代码同 `5e016913b`）：
- `git rebase e0d0909cf` 退 0，无冲突。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/parcelshipment/... ./internal/settlementaccounting/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./tests/bentocontract/... ./cmd/parcel-commercial/ ./cmd/parcel-frontline-import/ ./cmd/parcel-settlement-register/ ./internal/visibilityexception/adapters/inbox/ ./internal/visibilityexception/adapters/parcelshipment/ ./internal/architecture/... ./migrations/...`：退 0，42 个包 ok、3 个无测试文件、0 FAIL。这一轮多动了 PS `adapters/postgres`，它与 PS `application` 的反向依赖（`go list -test` 算）都在这一组里。
- 新用例与改动用例 `-count=3`（PS `application` 释放相关、PS `adapters/postgres` 的 savepoint 用例、`cmd/parcel-dispatch` 中毒释放、再驱与受控补充释放各条）：ok。
- 变异：装饰器改成直通，两条中毒真库用例变红；`or` 改成后者胜出，N4 四条变红。恢复后绿。
- `go build ./...`、`go vet ./...`、`gofmt -l` 干净。

## 进 main 记录（2026-10-21，协调方）

- 分支 `claude-code72`（code/72 与 code/73 同批）已由作者 rebase 到 main `e0d0909cf`，快进到 tip `8ae0a780a`；隔离评审无阻断，N1–N4 已收。N1（释放与暂停同笔、无保存点）由协调方裁定在本票内修，放行 `cmd/parcel-dispatch/assemble.go` 与 `cmd/parcel-api` 三个 assemble 文件各改一行包上 `SavepointedControlRelease`；协调方核过装饰器与装配 diff，未再起第二轮评审。
- 协调方单独提交重生成的机制清点；ADR 索引与 AT 计数无变化；检查均退 0。
- 全量 `scripts/test-full.sh` 退 0，158 个包 ok；`go build ./...`、`go vet ./...` 干净。无迁移，未灌种子。
- 与 code/72 同一分支同批进 main。
