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

## 完成记录（2026-10-21，分支 claude-code72，基于 main `32dc1d98d`；与 code/73 同一分支）

### 做了什么

- `d2583d3a7` `FormAcceptanceDecisionHandler.pauseForExternalResume`：等待态是`等待受控补充`、暂停 `Save` 成功之后，按 `OccupationFormed()` 用本轮已读回的控制结果按原关联释放本版本；放不掉照 `ControlReleasePending` 交补偿引用（`CompensationReference`），暂停不撤销、原因照旧交回、消费门照旧入账。`等待人工复核`与`等待授权处置`不放。释放经 code/73 抽出的 `occupationRelease`（`internal/parcelshipment/application/release_formed_occupation.go`），不另写一份。
  - 编排用例（`customer_supplement_pause_release_test.go`）：暂停即放、只放停住那一版；放不掉暂停照旧落库、补偿引用与同一版被拒绝时派生的逐字一致；明确无控制不放；暂停没落库（保存失败、版本冲突）不放；人工复核与授权处置两格不放。
  - 派发真库用例 `cmd/parcel-dispatch/customer_supplement_pause_release_test.go::TestACustomerSupplementPauseReleasesThePausedVersionsFreeze`：控制与释放走生产 PS→SA 适配器、作用域源、SA 施加与释放编排、两本账，余额只够冻一笔。暂停之后首版冻结已释放；补件后新版本照常冻下一笔并接受；链开头对首版那一笔再请求一次，到达提供方、答原释放，释放时刻不变。
  - code/20 的真库用例 `TestASupersededVersionsFreezeIsReleasedOnceBeforeTheNewVersionIsControlled` 改为暂停那一次释放注入失败：暂停照旧入账、首版那一笔留着，补件后链开头那一步作兜底，其余断言不变。
- `159841a2f` PS CONTEXT 生命周期「判断遇到客户可补充缺口 → 等待受控补充」补规则句（引 ADR-0132 决定一，规则句不挂票号）；UC-PS-001「给开发的交接」补 `AT-PS-035` 停等受控补充那一格的已补行。
- `76fb8c6a3` AT 盘点 `AT-PS-035` 行补点名，行仍在①，总览计数未动。

### 代决（用户授权「你作为业务和系统专家自决……解决所有的问题」）

1. **只有`等待受控补充`随暂停释放。** `等待人工复核`那一版复核完成后仍由任务按规则判、可以接受，那时占用合法；`等待授权处置`由处置命令自己放（ADR-0132 决定一），Decide 再放就是两条路认领同一笔。
2. **释放在暂停落库之后、交回原因之前。** 暂停没落库时整轮回滚重投，先放就是在一份不知停没停住的版本上宣告它不会再被判；保存失败、版本冲突两格都不放。
3. **只放停住那一版，不顺带放被取代各版。** Decide 只经接受链调用，同一轮里链开头已放过被取代各版。
4. **补偿引用交在 `FormAcceptanceDecisionResult.CompensationReference`，链结果不透出**——与自动拒绝那一支今天的形状相同（链本来就丢掉自动拒绝的补偿引用）。兜底有二：补件之后链开头再请求一次；客户撤回时撤回逐版释放（code/73）。
5. **无须新 ADR**：协调方已裁，照 ADR-0132 决定一的理由扩到 Decide 停等补充这一路，CONTEXT 补一句引它。
6. **UC-PS-002 不改**：它是接受后的资料修订，没有指向本票的缺口注，也不承接受控补充；受控补充那一格在 UC-PS-001，已补行加在那里。

### 越权风险点（待 PS owner 复核）

1. **暂停即放是把 ADR-0132 的理由扩到 Decide 停等补充这一路，待 PS owner 复核。** owner 若认为占用应保留到新版本形成，改的是 CONTEXT 那一句与 `pauseForExternalResume` 那一支。
2. **同一版本被再驱一拍时会在已释放的占用上判。** `Decide` 不拒绝对停在`等待受控补充`的版本重判；若另一扇门拿同一版本再驱链、且判断翻成全通过，接受会形成在一笔已释放的冻结上。今天走不到：判断账以（成员 + 版本 + 时点）为键、同键不覆盖，同版本重判仍读到`证据不足`；ADR-0106 决定三只让新版本的信封续办。授权处置选`交客户补充`之后本就是同一处境（既有）。SA 对已释放冻结的重复施加按请求身份答原冻结，PS 侧怎么译这一格本票未核。
3. **SA 释放的库错误会拖住暂停提交。** 释放与暂停同在消费门那一笔事务里，SA 账本没有保存点；释放若在库里报错（含 code/70 的锁等超时），事务已中止，提交失败、整轮回滚重投，暂停跟着回滚。本票之前暂停的提交不依赖释放。替身注入的失败不到库，用例证不到这一格。去向 [code/71](./71-release-and-control-ports-answer-a-closed-set-instead-of-a-bare-error.md)（释放端口答封闭集）一并看。
4. 每份停等补充、占过的委托多一次释放请求（含一次作用域重解与账本读、一次作用域锁），补件后链开头再请求一次答原释放。
5. 存量：首发前只有演示租户；既存停在等待受控补充、仍占着的委托，在补件后的链轮次或撤回时收口，或 `--reset` 重灌。

### AT

- `AT-PS-035`：停等受控补充那一格补编排两条、派发真库一条，行仍在①，总览计数未动。
- 未重生成 `docs/product/MECHANISM-INVENTORY.md`、ADR 索引与 AT 计数。

### 验证（代码同 `d2583d3a7`）

- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/parcelshipment/... ./internal/settlementaccounting/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./tests/bentocontract/... ./internal/visibilityexception/adapters/inbox/ ./internal/architecture/... ./migrations/...`：退 0，38 个包 ok、3 个无测试文件、0 FAIL。PS `application` 的反向依赖（`go list -test` 算）都在这一组里，组外只多出 VE `adapters/inbox` 的测试，已一并跑。
- 新用例与改动用例 `-count=3`（PS `application` 释放相关用例、`cmd/parcel-dispatch` 受控补充与授权处置相关真库用例）：ok。
- 变异（逐个施加、各跑一遍）：去掉暂停那一处释放，编排两条、派发真库三条（本票两条与 code/20 那一条）变红；终局各路只放命令指名那一版，编排六条、派发真库一条变红；自动拒绝不放其余各版，编排一条变红。恢复后绿。
- `go build ./...`、`go vet ./...`、`gofmt -l` 干净。
- 演示种子：本票不碰种子与登记 CLI 的任何输入或路径（种子只灌主数据登记，不走接受链、撤回、拒绝与处置），未灌。无迁移。
