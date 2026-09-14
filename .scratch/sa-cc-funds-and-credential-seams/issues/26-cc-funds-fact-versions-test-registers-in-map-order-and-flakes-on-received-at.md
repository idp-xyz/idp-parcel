# CC postgres `TestFundsFactVersionsAccrueAsRowsThatPointBack` 用 map 迭代登记 v1 / v2，顺序随机、各自一笔事务，`ListFundsFactVersions` 按 `received_at` 如实列出反序时用例断言「v1 走样」——main 既有 flaky，生产代码没错

Category: bug
Status: resolved——2026-09-14 21:4x 通道 3 完工（task-e73efbe2，通道 1 派单自立自做；`/tdd`；分支 `mcp3-sacc26` 基 main `5bf6eb84`，隔离树 `%TEMP%\idp-parcel-mcp3-sacc26`）：只改 `duty_payment_reconciliation_test.go` 一份，生产零 diff；完成判据逐条见「完成记录」；待非作者评审后进 main。此前 in-progress——同刻立票即开工
Blocked by: 无

## 缺口（推送方取证钉 `09833d67`，作者复现钉 `5bf6eb84`）

- 推送方带 DSN 全仓跑 `09833d67`（sa-cc/25 + 四笔 .md 取证）得 112 ok / **1 FAIL**：`internal/customscompliance/adapters/postgres` `TestFundsFactVersionsAccrueAsRowsThatPointBack`，`duty_payment_reconciliation_test.go` 里 `t.Fatalf("v1 走样：%+v", versions[0])`——`versions[0]` 是 v2。该包与 `migrations/customs_compliance/` 在 `84e37713..09833d67` **零 diff**，所以是 main 既有。同 tip 单跑 `go test ./internal/customscompliance/adapters/postgres/ -run 'TestFundsFactVersionsAccrueAsRowsThatPointBack$' -count=40 -v` 得 **36 PASS / 4 FAIL**。
- 作者复现（基 `5bf6eb84`，带 DSN，`-v` 全 PASS/FAIL 非 SKIP）：同一条命令 `-count=40` 得 **36 PASS / 4 FAIL / 0 SKIP**。
- **成因**（代码为准）：用例以 `for name, registration := range map[string]ports.ExternalFundsFactRegistration{"v1": first, "v2": second}` 登记两版，Go 小 map 迭代起点随机；每版经 `register(t, fixture, …)` → `fixture.db.Transactor().WithinTransaction` **各自一笔事务**，`RegisterFundsFact` 的 `received_at` 取 `now()`（事务时钟，`RegisterFundsFact` 头注「两处 received_at 都取事务内库时钟——它是本上下文接收这一动作的时间」）故两版不同；`ListFundsFactVersions` `ORDER BY received_at ASC, version ASC`——v2 先登时它**如实**把 v2 排前（端口 `ExternalFundsFactRegister` 头注「ListFundsFactVersions 按接收先后列全部版本」是写明的语义，适配器没错），用例却断言 `versions[0]` 是 v1。用例自 sa-cc/13 `e833355d` 起在 main，此前全仓绿是运气。
- 顺带量到（与派单字面不同，见「判断项」1）：同一用例尾部**已经有**一段确定性的反序登记（`[]ports.ExternalFundsFactRegistration{late, earlier}` 切片，`SYN-FUNDS-02` 先 v2 后 v1），断言 `ListFundsFactVersions` 列为 [v2, v1]；它缺的是 `LoadFundsFact` 在反序下交回**最近接收**那一版的断言，且与正序格挤在一个用例里。

## 做法（`/tdd`：先让随机性消失，再把两种到达顺序各钉一格）

1. 正序格：`TestFundsFactVersionsAccrueAsRowsThatPointBack` 的登记顺序改为显式先 v1 后 v2（带名字的结构体切片，不用 map）；断言不变（v1 在前、v2 回指 v1、`LoadFundsFact` 交回 v2、身份 1 行 / 版本 2 行直读、同版本重登不顶替）。
2. 反序格：把原用例尾部那段 `SYN-FUNDS-02` 反序登记抽成独立用例 `TestFundsFactVersionsArrivingOutOfOrderListByReceiptAndLoadTheLatestReceived`，断言两行都在、`ListFundsFactVersions` 按接收先后列为 [v2, v1]（v2 回指 v1、v1 无回指、金额各自）、**`LoadFundsFact` 交回最近接收的 v1**（端口头注「最近接收的那一版」，不是版本链上最新的）。
3. 两处头注改口：正序格头注去掉「先到 v2 后到 v1 也各占一行」那半句，写明「登记顺序显式先 v1 后 v2……交给 map 迭代一类的随机源，断言会随机翻面」并指向反序格；反序格头注写明它与正序格是同一条规则的两个到达顺序、各钉一格。

## 红线

- **不改生产代码**：`duty_payment_reconciliation.go`（`RegisterFundsFact` / `ListFundsFactVersions` / `LoadFundsFact`）一字不动；`ORDER BY received_at` 是对的。不改迁移。
- 不动其他测试。不在用例里 `Sleep` 凑时序——两笔事务先后执行 `now()` 已经不同。
- 注释中文；不写行号、不数别处。
- 真库占号：跑带 DSN 用例前排队列、广播占 55432，跑完释。

## 完成判据

1. `git grep -n 'map\[string\]ports.ExternalFundsFactRegistration' -- internal/customscompliance/adapters/postgres/` 零命中。
2. 带 DSN `go test ./internal/customscompliance/adapters/postgres/ -run 'TestFundsFactVersions' -count=40 -v` **40/40 PASS**（两个用例）；反序那一格 `-v` PASS 非 SKIP。
3. 带 DSN `go test -count=1 ./internal/customscompliance/adapters/postgres/` ok；`gofmt -l` 空；`go vet ./internal/customscompliance/...` 0。
4. `git diff --stat -- internal/customscompliance/ ':!*_test.go'` 空（生产零 diff）。
5. 完成记录同笔；清点零差（不增删文件）。

## 地盘

`internal/customscompliance/adapters/postgres/duty_payment_reconciliation_test.go`（只这一份）、本票面、sa-cc `spec.md` 子票表一行。撞点：在途分支零。

## 参照

[13](13-cc-correction-version-inbound-registration-and-rereconciliation.md) 完成记录「逐条对完成判据」2（本用例的出处：两版并存、身份 1 行版本 2 行直读、同版本重登不顶替、乱序各占一行）与裁决 1「迟到的前版按自己的版本进」；`internal/customscompliance/ports/ports.go` `ExternalFundsFactRegister` 头注「两个读口分工」（`LoadFundsFact` 交回**最近接收**的那一版 / `ListFundsFactVersions` 按接收先后列）；`internal/customscompliance/adapters/postgres/duty_payment_reconciliation.go` `RegisterFundsFact` 头注（`received_at` 取事务内库时钟）与 `LoadFundsFact` 头注（「同一事务内到达的两版 received_at 相同，再按版本字面定序只为确定性，不是版本大小的判断」）；UC-CC-009 一致性节「不按最后到达覆盖」；`docs/agents/parallel-sessions.md`「没有已知 flaky 用例时第二遍是零信息，有的话该做的是把它修掉或点名」。

## 完成记录

（通道 3 · task-e73efbe2 · 2026-09-14 21:3x–21:4x · 隔离树 `%TEMP%\idp-parcel-mcp3-sacc26`，分支 `mcp3-sacc26` 基远端 main `5bf6eb84`，一笔：本用例文件 + 本票面 + spec 一行。）

**`/tdd`**：red——基线 `5bf6eb84` 上 `-run 'TestFundsFactVersionsAccrueAsRowsThatPointBack$' -count=40 -v` 得 36 PASS / 4 FAIL / 0 SKIP（与推送方 `09833d67` 上的 36 / 4 同数，都是随机抽样，不是定数）；green——做法 1–3 落下后 `-run 'TestFundsFactVersions' -count=40 -v`：`TestFundsFactVersionsAccrueAsRowsThatPointBack` 40 PASS、`TestFundsFactVersionsArrivingOutOfOrderListByReceiptAndLoadTheLatestReceived` 40 PASS、FAIL 0、SKIP 0。反序格的新断言（`LoadFundsFact` 交回最近接收的 v1）在既有生产代码上一次即绿——它钉的是端口头注已写明的语义，不是新行为，所以没有一段属于它自己的 red；这一点如实记，不假装。

**逐条对完成判据**：

1. ✓ `git grep -n 'map\[string\]ports.ExternalFundsFactRegistration' -- internal/customscompliance/adapters/postgres/` 零命中（exit 1）。
2. ✓ 带 DSN `-run 'TestFundsFactVersions' -count=40 -v`：两格各 40/40 PASS、0 FAIL、0 SKIP；`ok … 3.180s`。
3. ✓ 带 DSN `go test -count=1 ./internal/customscompliance/adapters/postgres/` ok（9.0 s）；`gofmt -l internal/customscompliance/` 空；`go vet ./internal/customscompliance/...` 退 0。
4. ✓ `git diff --stat -- internal/customscompliance/ ':!*_test.go'` 空；`git diff --stat` 只有 `duty_payment_reconciliation_test.go`（+34 −10）。
5. ✓ 完成记录同笔；不增删 `.go` 文件，清点零差（只多本票 `.md` 与 spec 一行）。

**判断项**：

1. **派单字面与用例实况的一处出入**：派单写「头注那句『先到 v2 后到 v1 也各占一行』……被随机顺序“有时”跑到、没有确定性地钉下」。实况是：反序那一格在用例尾部**已经**用切片确定性地登了（`SYN-FUNDS-02` 先 v2 后 v1，断言列为 [v2, v1]），从 sa-cc/13 起就在；随机的是**正序格**——map 迭代让它二分之一不到的概率变成第二个反序场景，却带着正序的断言。所以做法 2 不是「新加一格」而是「把既有反序格抽出来、补上它缺的 `LoadFundsFact` 断言」；`SYN-FUNDS-02` 那条事实沿用，没有另造。评审若认为该按派单字面另写一个全新用例、保留尾部旧段，改法零风险，我不坚持。
2. 反序格新增的三条断言（v2 回指 v1 且金额 500 / v1 无回指且金额 400 / `LoadFundsFact` 交回 v1）比旧尾段只比版本号的断言严一格；与端口头注、`LoadFundsFact` 头注、sa-cc/13 裁决 1 逐句对过，无相抵。若 sa-cc/19 落地时 `LoadFundsFact`「最近接收」口径退役（19 做法 3），反序格最后一条断言随之改口——那是 19 的事，本票头注没有把它写成永久规则。
3. 未实测到同一微秒：两笔事务的 `received_at` 在 80 次登记（40 × 2 用例）里没有出现相等，反序格的 `ORDER BY version` 兜底分支一次也没被走到；不加 `Sleep`。
4. 派单「Blocked by 无」「撞点：在途分支零」核过；main 自 `5bf6eb84` 到 `3373f7cf` 只有 `.scratch/tasks.md` 一笔，本票可直接快进或重放，零冲突。

**验证**（隔离树，Windows 本机，DSN `IDP_PARCEL_POSTGRES_DSN` 指 55432；占 / 释各广播一次）：`gofmt -l` 空；`go vet ./internal/customscompliance/...` 0；带 DSN 单包 `-count=40` 两格 80/80 PASS 非 SKIP；带 DSN 单包 `-count=1` ok。**没跑**：全仓（本票只动一份测试文件，全仓由推送方重放时带 DSN 跑一次，parallel-sessions「唯一一跑」）；`-race`（本机无 cgo）。

## Comments

- 2026-09-14 21:4x · 通道 3（task-e73efbe2）：立票即开工即完工，见「完成记录」。作者自查两轴（不代替非作者评审）——**Standards**：注释全中文；新头注只引符号名与端口头注原句，无行号、无跨文件计数；夹具沿用 `SYN-FUNDS-01` / `SYN-FUNDS-02`；生产零 diff。**Spec**：判据 1–5 逐条 ✓；派单做法 1 / 3 照字面，做法 2 与字面的出入见判断项 1。能力边界：读了 `ports.go` 两读口头注、`duty_payment_reconciliation.go` 三方法与头注、本用例文件、sa-cc/13 完成记录判据 2 与裁决 1、sa-cc/19 票面（判断项 2 引其「做法」3）、`case_config_registry_test.go` 的 `register` 夹具助手（每次调用一笔 `WithinTransaction`，「各自一笔事务」以此为准）；**没读** `pgtest` 建库路径的实现——每个用例一座新库这一点以 `newViewFixture` 的既有用法为准，未另行核。
