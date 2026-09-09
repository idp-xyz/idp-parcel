# `internal/platform/pgtest` 改模板库：先量四段各占多少，再让每用例 `CREATE DATABASE … TEMPLATE …`

Category: enhancement
Status: resolved——2026-09-09 17:1x 通道 5 完成（12:0x 认领），分支 `mcp5-pgtest01`，基 `74ef0da8`，main SHA 由推送方补；完成记录见文末 Comments。立票经过：2026-09-09 通道 1 代裁立票（用户授权自决）：MCP-5 09-08 提议、parallel-sessions「验证」节已把解法与不许走的路写死（模板库；不关 `fsync`、
不用事务回滚包裹），本票是那一段的票。**先量再改**，量不出「时间几乎全在夹具」就停下报数、不改
Blocked by: 无

## 缺口（parallel-sessions「验证」节的取证，锚 `6ae24ae2`）

`pgtest` 给**每个用例**一个物理库：`CREATE DATABASE` → 跑真实迁移计划 → `DROP DATABASE … WITH (FORCE)`。`internal/parcelshipment/adapters/postgres` 143 个用例总 45.2 秒、
中位 0.31 秒、最慢 0.42 秒，均匀到看不出哪个用例重——时间几乎全在建库 + 全套迁移 + 删库，不在用例正文。全仓含 DSN `-p 1` 一次 9–10 分钟，
推送方每票付一次；作者与评审各自再付。

## 取证

### 改前——逐用例迁移（量于 `788e226c`，基 `74ef0da8`）

量法：`pgtest` 内打点。设 `IDP_PARCEL_PGTEST_TIMING` 指向一份 TSV，每次 `Pool(t)` 在清理函数末尾追加一行「建库 / 迁移 / 用例正文 / 删库 / 合计」
（列定义见 `internal/platform/pgtest/timing.go` 的 `TimingVariable` 注释）；未设变量零行为变化。命令：带 DSN 与该变量
`go test -count=1 ./internal/parcelshipment/adapters/postgres/`，143 行 TSV，跑两遍。

环境：本机 55432 单实例 `postgres:16.14`（容器 `idp-parcel-postgres-gate`）。量前 `docker stats` CPU 2.5%，`pg_stat_activity` 客户端后端 1
（只有取数的 psql 自己），无遗留 `parcel_*` 库。量数窗口 14:16–14:18 先广播给通道 2/3/4/6 请他们别跑带 DSN 的包；两遍数字相差不到 1%，互证窗口内无干扰。

| 段 | 中位 ms（跑 1 / 跑 2） | 合计 ms（跑 1 / 跑 2） | 占合计（跑 1） |
|---|---|---|---|
| 建库 | 5.7 / 5.7 | 840 / 849 | 1.9% |
| 迁移 | 283.2 / 285.8 | 41,034 / 41,298 | 93.6% |
| 用例正文 | 7.0 / 6.9 | 1,029 / 1,034 | 2.3% |
| 删库 | 4.6 / 4.5 | 949 / 850 | 2.2% |
| 合计 | 301.3 / 304.6 | 43,852 / 44,030 | 100% |

`go test` 自报 43.9 s / 44.1 s。迁移 + 建删库合计占 97.7%，远过一半——停下条件不触发，做法第 2、3 步继续。最慢的用例正文 18.6 ms
（`TestListWaitingOnCustomerSupplementFiltersOrdersAndTranscribes`），没有任何一个用例的正文超过其合计的 7%：「时间几乎全在夹具」由数字坐实，
且几乎全在**迁移**那一段，建库与删库合起来只占 4%。

### 改后——模板库克隆（量于 `fc2b473b`，基 `74ef0da8`）

量法与改前同一把打点、同一条命令、同一批 143 个用例，跑两遍。模板库模式下「迁移」一段记的是本用例为模板库等的时间：只有触发建模板的那一个用例真付
（建模板库 + 全套迁移 + 回收上一进程遗留的模板），其余用例为 0；「建库」一段记的是 `CREATE DATABASE … TEMPLATE` 克隆。

环境：同一实例 `postgres:16.14`（容器 `idp-parcel-postgres-gate`）。量前 `docker stats` CPU 2.57%，`pg_stat_activity` 客户端后端 1（只有取数的 psql 自己）。
量前实例上有一个 `parcel_tpl_6648_…`：前一任会话 16:0x 中断时留下、主人进程已不在（宿主机查无 pid 6648），由本次第一个建模板的进程回收（见「模板库残留」）。
量数窗口 17:02–17:05 先广播全通道；窗口前后 `pg_stat_activity` 客户端后端都只有 1，无外部干扰。

| 段 | 中位 ms（跑 1 / 跑 2） | 合计 ms（跑 1 / 跑 2） | 占合计（跑 1） |
|---|---|---|---|
| 建库（克隆） | 18.9 / 19.0 | 2,843 / 2,753 | 53.8% |
| 迁移（等模板） | 0.0 / 0.0 | 325 / 411 | 6.1% |
| 用例正文 | 8.4 / 8.5 | 1,218 / 1,204 | 23.1% |
| 删库 | 4.8 / 4.7 | 898 / 1,408 | 17.0% |
| 合计 | 32.5 / 32.7 | 5,284 / 5,776 | 100% |

`go test` 自报 5.316 s / 5.807 s（改前 43.9 s / 44.1 s）。

两组并列读法：

- 每用例合计中位 301.3 ms → 32.5 ms；包合计 43,852 ms → 5,284 ms，7.6–8.3 倍。
- 「迁移」从每用例付 283 ms（合计 41,034）变成整个进程付一次 325 / 411 ms，落在首个调 `Pool` 的用例 `TestDecisionIntentFollowsTheTransactionalTemplate` 上；其余各行为 0。
- 「建库」从空库 `CREATE DATABASE` 5.7 ms 变成克隆 18.9 ms——多付约 13 ms 拷文件，换掉 283 ms 迁移。跑 1 的首次克隆 125.6 ms 明显高于其后各次，跑 2 未重现（22.9 ms），不深究。
- 「用例正文」中位 7.0 → 8.4 ms，最慢仍是 `TestListWaitingOnCustomerSupplementFiltersOrdersAndTranscribes`（18.6 → 18.4 ms）：用例本身没变，差异在噪声内。
- 夹具（建 + 迁 + 删）仍占合计 77%，但绝对量从 42.8 s 降到 4.1 s。两遍相差 9%（改前 <1%）：夹具费缩到 5 秒量级后，几百毫秒的抖动（删库段 898 vs 1,408）在比例上就显眼了。

`-p 1` 能不能放**不在本票裁**，此处只交数。

## 做法

1. **量**（单独一笔，只加脚本 / 测试辅助，不改 `pgtest` 行为）：把一个用例的耗时拆成「建库 / 迁移 / 用例正文 / 删库」四段，对 PS postgres 包全部用例取
   中位数与合计，数字与 SHA 写进本票「取证」节。四段怎么量由作者定（`pgtest` 内打点、或 `-v` 时间戳差），写出来。**若迁移 + 建删库合计不到总时长的一半**，停下，
   本票转 needs-info 报数，不做下面两步。
2. **模板库**：每个测试进程（每个包的 `TestMain` 或首次 `Pool(t)`）只迁移一次到一个模板库（名字带进程 / 包标识，避免并行包相撞），之后每用例
   `CREATE DATABASE <用例库> TEMPLATE <模板库>`（文件级拷贝），用完照旧 `DROP … WITH (FORCE)`；进程退出后模板库由下一个建模板的进程回收（原写「进程退出时删」，
   收尾时按实现改口，理由见完成判据）。**隔离语义与「证的是随产品发出的那份 SQL」一字不变**
   ——模板库就是用那份迁移计划建的。改动只落在 `internal/platform/pgtest` 一个文件（或它的目录内），**`Pool(t)` 的签名不动**，任何一个调用方零改动。
3. **再量**：同一组用例重跑，四段数字并列写进「取证」节；`-p 1` 能不能放**不在本票裁**——parallel-sessions 写明「以 `ci.yml` 那种带 run 号的实测为据，不凭推」，
   本票只把两组数字交出来，放不放另立票。

## 不许走的路（parallel-sessions 已写死，此处只引不复述理由）

关 `fsync`；事务回滚包裹用例。

## 完成判据

四段耗时两组数字在票面；`pgtest` 模板库落地且 `Pool(t)` 签名不变；含 DSN 全仓 `go test -p 1 -count=1 ./...` 与改前同码（100 ok / 0 FAIL / 16 无测试 的口径）、
用时写进票面；`internal/platform/pgtest` 自己的测试盖「模板库只建一次 / 每用例库独立 / 退出进程的模板库由下一个建模板的进程回收 / 模板库不接受连接而克隆仍成」。

第三条原写「进程退出模板库删掉」，2026-09-09 收尾按实现的形状改口：主人以会话级咨询锁示活、下一个建模板的进程回收、实例上稳态恒余一个模板库。理由：Go 测试
二进制在 `m.Run` 返回后直接 `os.Exit`，没有退出钩，本包又不能要求每个调用方补 `TestMain`；parallel-sessions「验证」段（权威）只要求「每进程迁移一次 + 每用例
TEMPLATE 克隆」，未要求退出即删——原句是派生物，评审（通道 3）判机制诚实、改措辞不另立票。第四条是评审 Spec ① 的加固：模板库建成即 `ALTER DATABASE …
ALLOW_CONNECTIONS false`，「模板库不被任何用例写」从约定变成结构。

## 边界

不动任何迁移文件；不动 `compose.yaml`；不动 CI 分片。

## Comments

### 2026-09-09 17:1x 通道 5 · 完成记录（分支 `mcp5-pgtest01`，基 `74ef0da8`）

**逐笔**

- `f321b9c9` 认领，票面转 in-progress。
- `788e226c` 打点：`IDP_PARCEL_PGTEST_TIMING` 四段 TSV（`timing.go`），未设变量零行为变化。
- `8a81aa28` 取证：改前四段——迁移占 93.6%，停下条件不触发。
- `fc2b473b` 模板库：`template.go` 新、`template_test.go` 新、`database.go` 改；`Pool(t)` 签名不动。
- `db66ea02` 再量：改后四段与改前并列进「取证」节。
- 本笔：完成记录、全仓同码用时、模板库残留，Status → resolved。

**判据逐项**（验于 `fc2b473b` 的代码，17:01–17:10，实例 `idp-parcel-postgres-gate` 127.0.0.1:55432，两段量数窗口都先广播、后广播「窗口关」）

| 判据 | 结果 | 证据 |
|---|---|---|
| 四段耗时两组数字在票面 | ✓ | 「取证」节改前（量于 `788e226c`）/ 改后（量于 `fc2b473b`）两表并列 |
| 模板库落地且 `Pool(t)` 签名不变 | ✓ | `func Pool(t *testing.T) *pgxpool.Pool` 未动；`git diff --stat 74ef0da8..HEAD` 只有本票面与 `internal/platform/pgtest/` 下四个文件，调用方零改动 |
| 模板库只建一次 | ✓ | `TestTemplateIsBuiltOncePerProcessAndClonesCarryTheShippedPlan` PASS：两次 `Pool` 后本进程前缀的模板库恰一个；克隆库 `applied_migration` 与 `migrate.Plan()` 逐条同 ID 同校验和 |
| 每用例库独立 | ✓ | `TestEachPoolIsAnIndependentCopyOfTheTemplate` PASS：第一个库写下的表在第二个库不可见 |
| 退出进程的模板库由后来者回收（原写「进程退出模板库删掉」，收尾改口） | ✓ | `TestTemplateOfAnExitedProcessIsReapedByTheNext` PASS：子进程建模板后退出，父进程回收器删掉它、不删主人仍在的 |
| 含 DSN 全仓 `go test -p 1 -count=1 ./...` 与改前同码 | ✓ | 101 ok / 0 FAIL / 15 无测试，用时 118 s（17:07:22–17:09:20）。改前口径 100 / 0 / 16 量于 main `56ed4111`；差的那一个是 `internal/platform/pgtest` 自己——本支给它加了测试，从「无测试」变 ok，包总数同为 116 |
| 不动迁移文件 / `compose.yaml` / CI 分片 | ✓ | 同上 `diff --stat`，五个文件之外无改动 |
| 不关 `fsync`、不用事务回滚包裹 | ✓ | `internal/platform/pgtest/` 下无 `fsync`、无 `BEGIN`/`ROLLBACK`；每用例仍是自己的物理库，用完 `DROP … WITH (FORCE)` |

`pgtest` 自测（`go test -count=1 -v ./internal/platform/pgtest/`，带 DSN）：3 PASS / 1 SKIP。SKIP 的是 `TestHelperTemplateOwnerProcess`，它只作为子进程被驱动，直接跑时按设计跳过，不是一条独立判据。

**「进程退出模板库删掉」的保留**：Go 测试二进制在 `m.Run` 返回后直接 `os.Exit`，本包又不能要求每个调用方补 `TestMain`，所以没有「退出那一刻删」的钩子；`fc2b473b` 的解法是「主人以会话级咨询锁示活、后来者回收」——模板库在**下一个建模板的进程**启动时被删。于是实例上稳态**恒有一个**孤儿模板（最后一个进程的），直到下一次任何带 DSN 的 `pgtest` 进程起来。这是机制的形状，不是漏；票面判据原写「进程退出时删」——评审（通道 3，钉 `7a906a86`）判机制诚实、改措辞不另立票，判据与做法第 2 步已照实现的形状改口（见文末「收尾」）。

**模板库残留（做法第 4 步）**：全仓跑完 `psql` 查 `pg_database`——`parcel_test_*` 零个；`parcel_tpl_*` 一个（`parcel_tpl_36996_eb24c4d9d592`，17 MB），即最后一个建模板的包进程留下的那一个，原因见上；前面每个包的模板都被下一个包的进程回收了（`-p 1` 串行，同一时刻最多一个活模板）。窗口前实例上的 `parcel_tpl_6648_…`（前一任 16:0x 中断遗留、主人进程已不在）已在本次第一个建模板的进程里被回收——回收器在一个真孤儿上也验过一次。

**`-p 1` 只记数不裁**：全仓 118 s 墙钟，其中 `ok` 各包自报用时合计 84 s（其余是串行编译链接）；带真库的包最慢 8.4 s（`customscompliance/adapters/postgres`），PS `adapters/postgres` 5.6 s（parallel-sessions「验证」节记的改前实测 51 s）。放不放 `-p 1` 以 `ci.yml` 带 run 号的实测为据，另立票。

**尾巴**：无。

## 进 main 记录（2026-09-09 18:1x，通道 1 推送）

分支 `mcp5-pgtest01` 六笔在隔离树重放到 `135af96b` 之后（链上前有 wbr/02 三笔，零交集），零冲突、内容与分支逐文件零差：`f321b9c9→7a95b701` /
`788e226c→5b1e8d44` / `8a81aa28→8c9d8ed1` / `fc2b473b→bb930f89` / `db66ea02→64e95b6b` / `7a906a86→63fe4445`。清点在链 tip 重生成 `f9bcaa6e`
（platform 生产 15→17 / 测试 15→16；合计 875 / 831）。推送方在 `f9bcaa6e` 干净检出含 DSN `go test -p 1 -count=1 ./...` 一次：**102 ok / 0 FAIL /
15 无测试，120 s**（上一轮 `62e19b1b` 是 101 / 0 / 16、568 s；PS `adapters/postgres` 44.8 s → 5.6 s）；探针含 DSN PASS / 无 DSN SKIP。
**远端 `main = f9bcaa6e`**。分支指针改名 `merged/mcp5-pgtest01`。评审两条非阻断尾巴（下）**随票记，另派小票**：① `ALTER DATABASE … ALLOW_CONNECTIONS false`
加固；② 票面判据「进程退出模板库删掉」改口为实现的形状（后来者回收、稳态恒余一个）——评审判机制诚实、改措辞不另立票，推送方照此派给作者一笔收尾。

## Comments

**评审 ← 通道 3 · 钉 `7a906a86` · 17:48**（基 `74ef0da8` = merge-base，隔离树 `%TEMP%\idp-review-pgtest01`；原文在通道 1 台账 `task-a4fb6bd8`）

- **Standards**：阻断 0。非阻断（判断题）① `DROP DATABASE IF EXISTS … WITH (FORCE)` 三处拼接（`database.go` `dropDatabase`、`template.go` `buildTemplate`
  迁移失败清理、`reapOrphanTemplates`），可收成一个无 `t` 的 drop 函数供三处调。无发现：gofmt 空；build / vet 0；`diff --stat` 只有票面 + pgtest 四文件，
  无迁移 / compose.yaml / CI 改动；`func Pool(t *testing.T) *pgxpool.Pool` 签名不动；注释全中文，跨文件引用皆符号 / 文件名无行号；「不许走的路」：
  无 fsync、无事务回滚包裹，用例仍是独立物理库。
- **Spec**：阻断 0。非阻断 ① 「模板库不被用例写」今天只靠约定：模板 `datallowconn` 仍为 true，任何拿到 AdminDSN 的用例 `withDatabase(adminDSN, templateName)`
  就能连上写脏，也是「被其他用户访问」克隆失败的唯一来源——建议 `migrateTemplate` 断开后 `ALTER DATABASE … ALLOW_CONNECTIONS false`（template0 同法；
  `CREATE DATABASE … TEMPLATE` 与 `DROP` 都不需连接），把不变式变成结构性的。② 判据「进程退出模板库删掉」→ 实现为「主人会话锁示活、下一个建模板的进程回收」，
  稳态恒余一个 17 MB 孤儿；parallel-sessions「验证」段（权威）只写「每进程迁移一次 + 每用例 TEMPLATE 克隆」，未要求退出即删，票面字面是派生物——
  判机制诚实（Go 测试二进制无退出钩），建议改票面判据措辞而不另立票。「活」的判据：进程 crash / kill → OS 关 socket → 后端退出 → 锁释放（实测子进程退出后
  < 10 s 被回收）；只有宿主机整机断电才等 TCP keepalive（pgx 默认 5 min）。两个活进程互删不可能：先锁后建，试锁只在主人会话断后才拿得到；fnv32 撞键只会让孤儿
  暂时显得有主（安全方向）。无发现：隔离（每用例 `cloneDatabase` 物理拷贝、用完 `dropDatabase` WITH (FORCE)；`migrateTemplate` 迁完即断连，主人连接连的是
  postgres 库不是模板库）；互斥（进程内 `templateMu`；名 `parcel_tpl_<pid>_<12hex>` 不撞；`-p>1` 各进程各模板）；模板 = `migrate.Run` 真实计划、每进程新建
  不复用、测试逐条比 `applied_migration` 与 `migrate.Plan()` 的 ID + 校验和；`recordTiming` 未设变量即返回；三测各钉判据，子进程 `os.Args[0]` 重执行在 Linux CI
  是标准写法。实测（带 DSN，55432 零客户端）：3 PASS / 1 SKIP 0.888 s；跑前孤儿 `parcel_tpl_36996_…` 被回收，跑后余本进程一个，`parcel_test_*` 零，advisory 锁零。
- **结论**：可推；Spec ① 一行加固建议作者随手补或推送时记为尾巴。推送方处置：① ② 合成一张收尾小票派回作者通道（见进 main 记录）。

### 2026-09-09 18:0x 通道 5 · 收尾（评审 Spec ① ② + Standards ①；分支 `mcp5-pgtest01-tail`，基 `506bbfd2`；本笔一次提）

- **Spec ① 加固**：`template.go` `buildTemplate` 在 `migrateTemplate` 迁完断开之后加一步 `forbidConnections`——`ALTER DATABASE <tpl> ALLOW_CONNECTIONS false`
  （template0 同法）。此后拿着 `AdminDSN` 的用例改个库名也连不上模板库，「模板库不被任何用例写」从约定变成结构；「被其他用户访问」这一克隆失败的唯一来源随之
  关死。关许可失败与迁移失败同路：删掉半成品、关主人连接、返回错误。核过 `reapOrphanTemplates`：只在管理连接上查 `pg_database`、试锁、`DROP`，没有连模板库
  的步骤；`CREATE DATABASE … TEMPLATE` 与 `DROP … WITH (FORCE)` 都不需要连接源库，克隆与回收不受影响。新测试 `TestTemplateRefusesConnectionsWhileClonesStillSucceed`：
  连模板库被 PostgreSQL 以 SQLSTATE 55000 拒（钉状态码不钉文案），随后再 `Pool` 仍克隆出另一个库。
- **Standards ①**：三处 `DROP DATABASE IF EXISTS … WITH (FORCE)` 拼接收成 `database.go` 的 `dropDatabaseOn(ctx, conn, name)`——无 `t`、不走 `runAsAdmin`
  （回收器调用它时已在那把锁里面），`dropDatabase` / `buildTemplate` 的 `discard` / `reapOrphanTemplates` 三处改调它；语句只拼在一处。
- **Spec ② 措辞**：完成判据第三条与做法第 2 步由「进程退出时删模板库」改口为实现的形状，理由一句写在完成判据下；完成记录里「若评审认为必须是退出那一刻，另立票」
  改为评审判机制诚实、改措辞不另立票，已改。判据表加第四条「模板库不接受连接而克隆仍成」。
- **验证**（带 DSN，实例 127.0.0.1:55432）：`gofmt -l` 空；`go build ./...` / `go vet ./...` 全仓退 0；`go test -count=1 -v ./internal/platform/pgtest/`
  4 PASS / 1 SKIP（helper 按设计）1.12 s；`go test -count=1 ./internal/parcelshipment/adapters/postgres/` ok 5.2 s——克隆仍成。跑后 `pg_database` 里本进程的
  `parcel_tpl_46244_…` `datallowconn = false`；上一进程（pgtest 自测）留下的 `allowconn=false` 模板已被它回收——回收 `allowconn=false` 的孤儿也实测过。
  同一时刻实例上另有一个 `datallowconn = true` 的 `parcel_tpl_44616_…` 与一个活的 `parcel_test_*`：别的通道正用 main 上的旧码跑带 DSN 的包，不是本树的产物。
  未跑全仓（推送方跑）。只动 `internal/platform/pgtest/` 三个文件与本票面。
