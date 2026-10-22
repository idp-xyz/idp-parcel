# 72 停在等待受控补充的那一轮也释放本版本已成立的占用

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code72（见文末「完成记录」）。此前 ready-for-agent——R2（上线后尽快）：Decide 判出资料不足、停在`等待受控补充`时，本版本已成立的冻结或额度占用一直占着，直到客户补件（code/20 在取代时放）或撤回。客户不补件也不撤回时，那一笔就一直占下去。协调方已裁定照下文做（见「要裁的」）。2026-10-21 协调方据 code/20 隔离评审与越权风险点 2 立。
Blocked by: 无
出处：code/20 完成记录越权风险点 2；code/20 隔离评审。

## 现象

`FormAcceptanceDecisionHandler.pauseForExternalResume` 把等待态写进聚合之后交回 `CustomerSupplementPending`，不发释放。只有授权处置选`交客户补充`那一路会在处置时释放（[ADR-0132](../../../docs/adr/0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md) 决定一）；Decide 自己停进`等待受控补充`的那一路不放。

## 证据

- ADR-0132 决定一：选`交客户补充`那一刻，这一份提交版本的接受已确定不成立——「它不会再被判，占着客户的资金或额度等一份不会到来的接受没有依据」。
- [ADR-0106](../../../docs/adr/0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md) 决定三：`等待受控补充`只由新提交版本的信封续办，停住的那一版不会再被判。
- PS CONTEXT 词条「控制项结果」：接受确定未成立时必须按原业务关联请求释放。

## 三类

机制（把 ADR-0132 的理由扩到它原本没覆盖的一条路）。无租户取值。

## 规模

S：`pauseForExternalResume` 在等待态是`等待受控补充`、且暂停已落库之后，按 `OccupationFormed()` 释放本版本；补用例。

## 最小的诚实修法

1. 暂停 `Save` 成功之后、交回 `CustomerSupplementPending` 之前，本版本 `OccupationFormed()` 为真即按原关联释放。
2. 释放失败照 `ControlReleasePending` 留补偿续办引用。暂停不撤销：等待态照旧随本份投递入账。
3. 客户补件之后，接受链的 `releaseSuperseded`（code/20）对同一笔再请求一次，提供方按幂等答原释放。
4. PS CONTEXT 生命周期「判断遇到客户可补充缺口 → 等待受控补充」补一句：停住的那一版已成立的占用随暂停释放，并引 ADR-0132 决定一。
5. 补编排与真库用例：资料不足 → 已冻结 → 停等补充 → 冻结已释放；补件后新版本照常冻结。

## 要裁的

已裁（协调方 2026-10-21）：照上面做，不另写 ADR。理由是 ADR-0132 决定一「它不会再被判，占着……没有依据」，加上 ADR-0106 决定三——那一版同样不会再被判。落地时完成记录的越权风险点写「暂停即放是把 ADR-0132 的理由扩到 Decide 停等补充这一路，待 PS owner 复核」。

## 完成记录（2026-10-21，分支 claude-code72，基于 main `32dc1d98d`，评审后 rebase 于 main `e0d0909cf`，下文 SHA 均为 rebase 后的；与 code/73 同一分支）

### 做了什么

- `050f920de` `FormAcceptanceDecisionHandler.pauseForExternalResume`：等待态是`等待受控补充`、暂停 `Save` 成功之后，按 `OccupationFormed()` 用本轮已读回的控制结果按原关联释放本版本；放不掉照 `ControlReleasePending` 交补偿引用（`CompensationReference`），暂停不撤销、原因照旧交回、消费门照旧入账。`等待人工复核`与`等待授权处置`不放。释放经 code/73 抽出的 `occupationRelease`（`internal/parcelshipment/application/release_formed_occupation.go`），不另写一份。
  - 编排用例（`customer_supplement_pause_release_test.go`）：暂停即放、只放停住那一版；放不掉暂停照旧落库、补偿引用与同一版被拒绝时派生的逐字一致；明确无控制不放；暂停没落库（保存失败、版本冲突）不放；人工复核与授权处置两格不放。
  - 派发真库用例 `cmd/parcel-dispatch/customer_supplement_pause_release_test.go::TestACustomerSupplementPauseReleasesThePausedVersionsFreeze`：控制与释放走生产 PS→SA 适配器、作用域源、SA 施加与释放编排、两本账，余额只够冻一笔。暂停之后首版冻结已释放；补件后新版本照常冻下一笔并接受；链开头对首版那一笔再请求一次，到达提供方、答原释放，释放时刻不变。
  - code/20 的真库用例 `TestASupersededVersionsFreezeIsReleasedOnceBeforeTheNewVersionIsControlled` 改为暂停那一次释放注入失败：暂停照旧入账、首版那一笔留着，补件后链开头那一步作兜底，其余断言不变。
- `52f6054b6` PS CONTEXT 生命周期「判断遇到客户可补充缺口 → 等待受控补充」补规则句（引 ADR-0132 决定一，规则句不挂票号）；UC-PS-001「给开发的交接」补 `AT-PS-035` 停等受控补充那一格的已补行。
- `ee492a101` AT 盘点 `AT-PS-035` 行补点名，行仍在①，总览计数未动。

### 代决（用户授权「你作为业务和系统专家自决……解决所有的问题」）

1. **只有`等待受控补充`随暂停释放。** `等待人工复核`那一版复核完成后仍由任务按规则判、可以接受，那时占用合法；`等待授权处置`的释放时刻由 ADR-0132 决定一定在处置那一刻，停下时处置角色还没选去向。
2. **释放在暂停落库之后、交回原因之前。** 暂停没落库时整轮回滚重投，先放就是在一份不知停没停住的版本上宣告它不会再被判；保存失败、版本冲突两格都不放。
3. **只放停住那一版，不顺带放被取代各版。** Decide 只经接受链调用，同一轮里链开头已放过被取代各版。
4. **补偿引用交在 `FormAcceptanceDecisionResult.CompensationReference`，链结果不透出、也没有持久记录**——与自动拒绝那一支今天的形状相同。这一路没有东西按引用重放，兜底是另两条路：补件之后链开头再请求一次；客户撤回时撤回逐版释放（code/73）。
5. **无须新 ADR**：协调方已裁，照 ADR-0132 决定一的理由扩到 Decide 停等补充这一路，CONTEXT 补一句引它。
6. **UC-PS-002 不改**：它是接受后的资料修订，没有指向本票的缺口注，也不承接受控补充；受控补充那一格在 UC-PS-001，已补行加在那里。

### 越权风险点（待 PS owner 复核）

1. **暂停即放是把 ADR-0132 的理由扩到 Decide 停等补充这一路，待 PS owner 复核。** owner 若认为占用应保留到新版本形成，改的是 CONTEXT 那一句与 `pauseForExternalResume` 那一支。
2. **同一版本被再驱一拍。** `Decide` 不拒绝对停在`等待受控补充`的版本重判，判断账也挡不住重判（主键含 `resolution_id`，可达性每轮都重问 NR）。真正的保护有两层（评审 N2 改写）：
   - 没有驱动方会再驱一份停在受控补充的版本：inbox 按投递去重；登记续办只列`等待运营登记`；复核完成信封只驱复核那一格。
   - 真被再驱时，控制那一步按同一请求身份重施加，SA 交回已释放的冻结，`controlItemFor` 答 `ErrUntranslatableAnswer`，折成 `FINANCIAL_CONTROL_UNAVAILABLE`，链在 `Decide` 之前就停，不会在已释放的冻结上形成接受。派发真库用例 `TestARedrivenPausedVersionStopsAtTheReleasedFreeze` 钉住这一层。
   - 残余：这样的再驱停在内部重试，消费门会对那一封一直回滚重投到失败预算耗尽。
3. **SA 释放的库错误曾会拖住暂停提交——已在本票内收住（评审 N1）。** 释放与暂停同在消费门那一笔事务里，SA 账本没有保存点；释放编排把库错折成`未形成`正常交回，事务却已中止，提交答 25P02，暂停回滚。现由 `pspostgres.SavepointedControlRelease` 隔开（见评审记录），收不住的只剩「退不回 savepoint」那一格（连接断了之类），那时整笔照旧回滚重投。隔离靠装配兑现：编排不校验释放口是否包了这一层，日后新接一条释放装配而忘了包，这一格会回来。
4. 每份停等补充、占过的委托多一次释放请求（含一次作用域重解与账本读、一次作用域锁），补件后链开头再请求一次答原释放。
5. 存量：首发前只有演示租户；既存停在等待受控补充、仍占着的委托，在补件后的链轮次或撤回时收口，或 `--reset` 重灌。
6. **授权处置选`交客户补充`之后再驱同一版本，`Decide` 会停进`等待受控补充`、再放一次**（提供方按幂等答原释放）。注释原写「两条路认领同一笔」不严格，已改为按 ADR-0132 决定一的释放时刻说理。
7. **既有的锁序隐患**：接受链先取作用域锁再写委托行，撤回先写委托行再取作用域锁（code/70 越权风险点 2 那一格）；本票让暂停与终局路径多几次释放，略有放大，今天 parcel-api 释放口没有作用域源仍走不到。
8. **ADR-0132 越权风险点 4（`交客户补充`即释放）现在同时管两处**：处置那一刻与 Decide 停等补充那一刻；owner 改它时两处一起改。

### AT

- `AT-PS-035`：停等受控补充那一格补编排两条、派发真库一条，行仍在①，总览计数未动。
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
