# sa-cc/19 评审 Standards 尾巴：适配器测试替身 `rederiveStores.ListVerificationsByFundsFact` 遍历 map 交回、不守端口口径「核对时刻升序、同刻按指纹字典序」；`duty_registers_test.go` 两处 seed 手拼 `tenant + "/" + fact + "/" + version` 不走同文件的 `fundsVersionKey`

Category: chore
Status: resolved——**2026-09-15 12:5x 通道 2**（task-38d5d3a7-b21b-46f4-9d4b-2841bd138581；分支 `mcp2-sacc30` 基 `e1ab9fb5`，代码 tip 即本笔——两份测试文件与完成记录同一提交，SHA 见交付消息；做法 2 已做，先 red 后 green；全文见「完成记录」）。此前 ready-for-agent——2026-09-15 12:4x 通道 1 立票（sa-cc/19 非作者评审 ← 通道 6 Standards 非阻断 ① ②，推送方处置「合一张 A 类零行为尾巴」）。只测试文件，零生产改动
Blocked by: 无（sa-cc/19 已进 main `49ffc96c`）

## 缺口（评审钉 `a0cb6fef`，进 main 后在 `07341b8b` 同形）

- `internal/customscompliance/adapters/settlementaccounting/receive_on_adopted_funds_fact_test.go` 替身 `rederiveStores` 的 `ListVerificationsByFundsFact` 遍历 map 交回、无序；端口 `ports.DutyVerificationStore.ListVerificationsByFundsFact` 头注写死「核对时刻升序、同一时刻按指纹字典序」，`latestVerificationPerLineage` 只在**同刻并存**时依赖这个序（严格更晚才换人）。适配器那两格今天没有同刻两版被列到的路径（重投走`已存在`不触发、v3 那格列之前就故障），所以绿；将来谁在这本替身上铺同刻两版会 flake。应用层同名替身 `dutyStoreDouble`（`reconcile_duty_payment_test.go`）已按口径 `sort.Slice`。
- `cmd/parcel-customs-register/duty_registers_test.go` `seedFundsFact` 用 `tenant+"/"+fact+"/"+version.String()` 作 `fakeDutyBook.funds` 的键、`seedFundsFactWithoutPayer` 用 `tenant + "/" + fact + "/" + fact + "/v1"`，都不走同文件为此加的 `fundsVersionKey(tenant, fact, version)`——键的拼法有两份，改一处另一处不跟。

## 做法

1. `rederiveStores.ListVerificationsByFundsFact` 交回前照 `dutyStoreDouble` 那份 `sort.Slice`（先 `VerifiedAt` 升序、同刻按 `Key.Digest` 字典序）；替身头注补一句「与端口口径同序，同刻两版靠它分先后」。
2. **可选**（作者定，做了写判断项）：在该文件加一格「同刻两版在册、v2 到达 → `latestVerificationPerLineage` 取指纹字典序小的那版承前」，把 19 判断项 ② 写成断言。
3. `seedFundsFact` / `seedFundsFactWithoutPayer` 的键改走 `fundsVersionKey`——后者的 `fact + "/v1"` 先经 `domain.NewFundsFactVersion` 再传，与前者同一路。

## 红线

- 零生产改动：`git diff --stat -- ':!*_test.go'` 为空；`internal/customscompliance/**` 非测试文件零 diff。
- 不改端口口径、不改 `latestVerificationPerLineage`；替身只是守口径，不替生产排序。
- 注释中文；不写行号、不数别处。

## 完成判据

1. `rederiveStores.ListVerificationsByFundsFact` 有排序且与 `dutyStoreDouble` 同键序；若做了做法 2，那一格 PASS。
2. `git grep -n 'tenant+"/"+fact+"/"\|tenant + "/" + fact + "/"' -- cmd/parcel-customs-register/duty_registers_test.go` 只剩 `fundsVersionKey` 自己那一行。
3. `gofmt -l` 空；`go vet ./internal/customscompliance/... ./cmd/parcel-customs-register/...` 0；不带 DSN 两处包 ok。
4. 完成记录同笔；清点零差（不增删文件）。

## 地盘

`internal/customscompliance/adapters/settlementaccounting/receive_on_adopted_funds_fact_test.go`、`cmd/parcel-customs-register/duty_registers_test.go`（只测试）。撞点：[29](29-cc-handoff-envelope-id-exceeds-framework-limit-and-rederive-path-swallows-handoff-failure.md) 若同期开工会碰 `receive_on_adopted_funds_fact.go`（生产文件）与 `assemble_test.go`，与本票两份测试文件不同文件。

## 完成记录（2026-09-15 通道 2，基 `e1ab9fb5`，只两份测试文件）

### 逐条对完成判据 1–4

1. `rederiveStores.ListVerificationsByFundsFact`（`internal/customscompliance/adapters/settlementaccounting/receive_on_adopted_funds_fact_test.go`）交回前 `sort.Slice`：`Verification.VerifiedAt()` 升序、同刻按 `Key.Digest` 字典序——比较函数与 `dutyStoreDouble.ListVerificationsByFundsFact`（`application/reconcile_duty_payment_test.go`）同一份；`rederiveStores` 头注补「与端口口径同序，同刻两版靠它分先后」一句并写明不排序的后果。做法 2 已做：新格 `TestSameInstantVersionsHandTheirLineageToTheLexicographicallySmallerDigest` PASS，red → green 见「验证」。
2. `git grep -n -e 'tenant+"/"+fact+"/"' -e 'tenant + "/" + fact + "/"' -- cmd/parcel-customs-register/duty_registers_test.go` **零命中**——比票面「只剩 `fundsVersionKey` 自己那一行」更严：`fundsVersionKey` 函数体拼的是 `tenant.String() + "/" + fact.String() + "/" + version.String()`，这两个模式本就不匹配它；文件里再没有第二份拼法（含原本那条 `"SYN-T1/SYN-FUNDS-01/SYN-FUNDS-01/v1"` 字面键，见判断项 3）。
3. `gofmt -l ./internal/ ./cmd/` 空；`go vet ./internal/customscompliance/... ./cmd/parcel-customs-register/...` 退出码 0；不带 DSN `go test -count=1 ./internal/customscompliance/... ./cmd/parcel-customs-register/...` 全 ok。
4. 完成记录与两份测试同一笔；不增删文件（工作树只 `M` 两份 `_test.go` + 本票 .md），清点零差。

**零生产改动自核**：`git diff --stat -- ':!*_test.go' ':!.scratch'` 输出为空（提交前量于工作树；提交后同命令加 `e1ab9fb5 <tip>` 同样为空）。

### 判断项

1. **做法 2 做了，理由**：`verifiedFixture` 的编排时钟是固定的 `dutyClock{at: fundsOccurredAt.Add(time.Hour)}`，同一谱系在 v1 上再核一版（覆盖 `PARTIAL`、差额 `SHORT`）就与首版同刻并存、只差指纹——「同刻两版」不必造时钟就有；不加这一格，做法 1 的排序没有任何用例走到，评审那句「将来会 flake」也没有落点。断言写法：谁的指纹小**当场比**（`expected` 取两版里 `Key.Digest` 字典序小者），不写死 sha256 字面；断言 (a′) 的 `Coverage()` 等于 `expected` 的、`FundsVersion()` = v2、册上恰多一行一封。**red 证据**：替身未排序时 `go test -count=20` 红 2 次、`-count=30` 红 3 次，失败文本点名指纹 `a849d408…` 那版（`COVERED`）该被承前而实际承了 `PARTIAL`；加排序后 `-count=50` 零红。翻面率低于一半是 Go 小 map 迭代序的分布使然，不是断言有偶然通过的路径——两版覆盖不同，承错必红。
2. **`seedFundsFactWithoutPayer` 的 `fact + "/v1"` 怎么走 `NewFundsFactVersion`——不再自己拼**。`seedFundsFact` 把租户经 `domain.NewTenantID`、引用经 `NewExternalFundsFactReference`、版本经 `NewFundsFactVersion(fact + "/v1")` 构造后交 `fundsVersionKey`，并把这个键**交回**；`seedFundsFactWithoutPayer` 用返回值改付款人、再交回同一个键。于是 `"/v1"` 字面与键的拼法在文件里各只出现一次。票面写的是「后者的 `fact + "/v1"` 先经 `NewFundsFactVersion` 再传」——那样 `"/v1"` 与三次领域构造会在两只 seed 里各出现一次，两份构造与两份键是同一种重复；接手方取返回键这条路，与票面「与前者同一路」的意图一致、重复更少。
3. **顺手多消了一处**：`TestExecuteDutyPaymentVerificationPayerGridsKeepTheirExitCodes` 末尾 `fixture.duties.funds["SYN-T1/SYN-FUNDS-01/SYN-FUNDS-01/v1"]` 那条字面键——票面缺口没点名它，但它正是「键的拼法有两份，改一处另一处不跟」的第三份——改为用 `seedFundsFactWithoutPayer` 交回的键 `unprovided`。在地盘内、同一缺陷、零行为变化。
4. 两只 seed 的返回值在其余调用点（`seedFundsFact(t, fixture.duties, …)` 那几处）被丢弃：Go 允许语句形式丢单返回值，vet 不报，不为它们加 `_ =`。

### 验证（接手方自己跑，基 `e1ab9fb5` + 本笔）

- `gofmt -l ./internal/ ./cmd/` 空；`go vet ./internal/customscompliance/... ./cmd/parcel-customs-register/...` 退出码 0。
- 不带 DSN `go test -count=1 ./internal/customscompliance/... ./cmd/parcel-customs-register/...` 全 ok（`registrationjson` / `ports` 无测试文件；postgres 包真库格按既有方式 skip）。
- 新格 red / green：`go test -run TestSameInstantVersionsHandTheirLineageToTheLexicographicallySmallerDigest ./internal/customscompliance/adapters/settlementaccounting/` 替身未排序时 `-count=20` 红 2、`-count=30` 红 3；排序后 `-count=50` 红 0。
- 未占 55432、未跑真库（票面不要求，两份都是替身测试）。

### 能力边界

读了：票 30 全文；`receive_on_adopted_funds_fact_test.go` 的 `rederiveStores` 全部方法、`verifiedFixture`、两格既有用例；`reconcile_duty_payment_test.go` 的 `dutyStoreDouble.ListVerificationsByFundsFact`（照抄比较函数）；`duty_registers_test.go` 的 `fakeDutyBook` 资金事实三方法、两只 seed 与 `TestExecuteDutyPaymentVerificationPayerGridsKeepTheirExitCodes`；`rederive_duty_verifications.go` 的 `latestVerificationPerLineage`（接手 19 时读过，本票未改）。**没读**：`receive_on_adopted_funds_fact.go` 生产文件（零 diff）、[29](29-cc-handoff-envelope-id-exceeds-framework-limit-and-rederive-path-swallows-handoff-failure.md) 正文、`cmd/parcel-customs-register` 其余测试。

## 参照

[19](19-cc-new-funds-fact-version-forms-a-new-verification-version.md) Comments「评审 ← 通道 6」Standards ① ②、「完成记录」判断项 ②；[28](28-sa-sacc20-review-standards-tail-save-header-states-ordered-versus-concurrent-answers.md)（同款 A 类尾巴先例）；AGENTS「写代码注释」。

## Comments

- 2026-09-15 12:4x · 通道 1：立票（评审尾巴，推送方处置时点名）。只写票面，未动代码。
