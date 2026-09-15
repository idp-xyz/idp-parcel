# sa-cc/20 评审 Standards 尾巴：`Save` 头注与 `FundsFactAlreadyAdopted` 注只写「两种都按当前链头作答」，没写出「顺序到达`未受理` / 并发到达`已采用`」这条不对称——同一业务事实两张脸，读注释的人分不出

Category: chore
Status: resolved——**已进 main，2026-09-15 12:3x 通道 1 推送方**（第四批，重放 `d6b1f773→981a6f15` / `88ca0a2e→ee3ec953`，批 tip `676cc09b`（含 pp-seams/06 与 sa-cc/27 取证）；评审门推送方自审——纯注释，`git diff -U0 7160fe67 d6b1f773 -- internal/` 滤掉 `^[+-]\s*//` 后为空；批 tip 带 DSN 全仓 113 ok / 0 FAIL；见 Comments「进 main 记录」）。此前——**2026-09-15 12:1x 通道 4**（task-1b28ece5-bebc-4a16-8221-a2f2a7e0295f；分支 `mcp4-sacc28` 基 `7160fe67`，代码 tip `d6b1f773`，本完成记录紧随一笔；`git diff` 每一改行均 `//`、不带 DSN SA 全部包 ok、清点在 tip 上重生成零差；见「完成记录」）。此前 ready-for-agent——2026-09-15 11:2x 通道 1 立票（sa-cc/20 非作者评审 ← 通道 3 Standards 非阻断 ①，推送方处置「合一张 A 类零行为尾巴」）。零行为，只注释
Blocked by: 无（sa-cc/20 已进 main `1e74aaaf`）

## 缺口（评审钉 `63333f4d`，进 main 后在 `14d86a61` 同形）

- `internal/settlementaccounting/adapters/postgres/external_funds_fact.go` `Save` 头注第二段：「版本行的 DO NOTHING 不写冲突目标：0021 守『一个首版』『一个前版只被更正一次』的唯一约束撞上时同样折成`已采用`，让编排像 AdoptFact 输掉竞态那样读回链头作答……顺序到达的同类写入在编排里就被『回指必须等于链头』挡下，到不了这里。」——两句各自对，但没有把两条路并排写成一句：**同一个「回指非链头」的业务事实，顺序到达答`未受理`、并发到达答`已采用`（带赢家版本）**。
- `internal/settlementaccounting/ports/ports.go` `FundsFactAlreadyAdopted` 注：「库上守链形的唯一约束……撞上时也答它：这一版没有落，占着那个位置的是先到的那一版，编排读回链头作答」——同样没点出与顺序路的`未受理`不同答。
- `internal/settlementaccounting/application/map_external_funds.go` `CorrectFact` 的 `FundsFactAlreadyAdopted` 分支注释「两种都按当前链头作答——它就是此刻被采用的那一版」——这句是对的，缺的是「与顺序到达的`未受理`不同答」那半句及其理由。
- 评审给的理由（同意）：结构上可分——返回记录 `Fact().Fact.Version()` ≠ 命令版本，且与既有 `AdoptFact` 输竞态格同形，**行为不改**；只是注释该把不对称写明，读的人才知道这是有意的（parallel-sessions「不同的绿长同一张脸」那一节：分不开的两态要么做进结构、要么写证据）。
- 顺带（评审 Standards 非阻断 ②，可选）：`migrations/settlement_accounting/0021_external_funds_fact_versions.sql` 头注「写成三道约束」「三道合起来」数的是本文件自己的约束，不触 AGENTS「不数别处」；作者自评时把 Go 注释里同一组计数换成了点名，SQL 这一处与之不一致——**迁移已施加、校验和按文件内容算，不改**，此条只记不做。

## 做法

1. `Save` 头注第二段末尾补一句（中文，符号名不带行号）：大意「同一个『回指的不是当前链头』：顺序到达在 `CorrectFact` 里答`未受理`（提交矛盾），并发到达在这里折成`已采用`、编排交回赢家那一版——两答不同是有意的：前者是调用方编程错误，后者是谁先落谁是当前；调用方从交回记录的版本字面 ≠ 命令版本分得出后者」。
2. `ports.go` `FundsFactAlreadyAdopted` 注同一句的短版。
3. `CorrectFact` 那个分支的注释加半句「与顺序到达答`未受理`不同答，理由见 Save 头注」——只引符号名。

## 红线

- 零行为：`git diff --stat -- ':!*_test.go' | grep -v '^\s*//'` 意义上只允许注释行变化；`go test ./internal/settlementaccounting/...` 不带 DSN 照旧 ok。
- 注释中文；不写行号、不数别处；不改 `0021`。

## 完成判据

1. 三处注释各有那半句；`git diff` 每一改行都是 `//` 注释。
2. `gofmt -l` 空；`go vet ./internal/settlementaccounting/...` 0；不带 DSN SA 全部包 ok。
3. 完成记录同笔；清点零差。

## 地盘

`internal/settlementaccounting/adapters/postgres/external_funds_fact.go`、`internal/settlementaccounting/ports/ports.go`、`internal/settlementaccounting/application/map_external_funds.go`（只注释）。撞点：无在途分支碰这三份（sa-cc/19 在 CC）。

## 参照

[20](20-sa-external-funds-fact-holds-one-row-per-fact-and-cannot-store-a-correction.md) Comments「评审 ← 通道 3」Standards ① ② 与「进 main 记录」处置句；`docs/agents/parallel-sessions.md`「不同的『绿』在输出上长着同一张脸」；AGENTS「写代码注释」「改文档」。

## 完成记录

分支 `mcp4-sacc28`，基 `7160fe67`（派单时 main tip），树 `%TEMP%\idp-parcel-mcp4-sacc28`：

| SHA | 内容 |
|---|---|
| `d6b1f773` | docs(sa)：三处注释各补「顺序到达答`未受理` / 并发到达答`已采用`」不对称一句——`ExternalFundsFacts.Save` 头注第二段、`ports.FundsFactAlreadyAdopted` 注、`MapExternalFundsHandler.CorrectFact` 的 `FundsFactAlreadyAdopted` 分支 |
| （本笔） | docs(scratch)：本完成记录 + Status → resolved |

**逐条对完成判据**：**(1)** 三处各有那半句——`Save` 头注：「于是同一个『回指的不是当前链头』，顺序到达在 CorrectFact 里答`未受理`（提交矛盾），并发到达在这里折成`已采用`、编排交回赢家那一版——两答不同是有意的：前者是调用方编程错误，后者是谁先落谁是当前；调用方拿到`已采用`时，从交回记录的版本字面 ≠ 命令版本分得出是输掉竞态而非重放」；`FundsFactAlreadyAdopted` 注：同句短版（「并发到达撞约束才答它、交回赢家那一版」）；`CorrectFact` 分支：「同一个『回指的不是当前链头』顺序到达时在上面答的是`未受理`，两答有意不同，理由见 ExternalFundsFacts.Save 头注」。**每改行均 `//`**，自核方法：`git diff --stat 7160fe67 d6b1f773 -- internal/` = 3 文件 / 9 insertions / 2 deletions（两处 deletion 是被续写的原注释行本身）；`git diff -U0 7160fe67 d6b1f773 -- internal/` 取全部 `^[+-]` 行、去掉 `+++` / `---` 文件头、再滤掉 `^[+-]\s*//` 后为**空**。**(2)** `gofmt -l ./internal/settlementaccounting/` 空；`go vet ./internal/settlementaccounting/...` 退出 0；不带 DSN（shell 内无任何名含 `DSN` / `DATABASE` / `PG` 的环境变量）`go test -count=1 ./internal/settlementaccounting/...` 九包 ok、`ports` 无测试文件、0 FAIL（`adapters/postgres` 0.017s 答 ok，真库用例未运行）。**(3)** 本完成记录紧随代码笔、同分支；**清点**：在 `d6b1f773` 干净检出上按 CI 同一命令行 `go run . -dir <root> -out <root>/docs/product/MECHANISM-INVENTORY.md` 重生成，`git status --porcelain -- docs/product/MECHANISM-INVENTORY.md` 空——零差，未提清点笔。

**判断项**（措辞上偏离票面「大意」之处，归 owner 复核）：① `Save` 头注那一句**插在「到不了这里」之后、「回指一个不存在的版本是外键错」之前**，不在段落最末——它解释的正是「顺序到达……到不了这里」那一句，紧跟着读才顺；外键那句原本就是段尾旁注，仍留段尾。② 票面「调用方从交回记录的版本字面 ≠ 命令版本分得出后者」的「后者」我写成「分得出是输掉竞态而非重放」：并发`已采用`自身又分两格（同一新版本先落 = 重放，版本相等；链头先被别的版本更正 = 输掉竞态，版本不等），调用方靠版本字面分开的是这两格，不是「并发 vs 顺序」——那一对已由`已采用` / `未受理`两个不同结果分开。三处同口。③ `CorrectFact` 分支的「理由见 Save 头注」写成 `ExternalFundsFacts.Save 头注`：应用层只认 `ports.ExternalFundsFactStore`，裸写 `Save` 会让人往端口接口上找；被引的是 postgres 适配器那份头注，按符号名点全。④ `ports.go` 短版里「并发到达撞约束才答它」——注释挂在 `FundsFactAlreadyAdopted` 常量上，「它」就是这个值，不在端口层再复述一次编排口径的「折成`已采用`」。⑤ 顺带条（`0021` SQL 头注「三道约束」计数）照票面**不做**。⑥ 提交类型取 `docs(sa)`：只动 Go 注释、无行为，不冒 `fix` / `refactor`。

**验证**（12:0x–12:1x）：见判据 (2) 与 (3)；**未占 55432**、未广播占号。

**能力边界**：只读了三处地盘所在的三个文件、票面与 sa-cc/22 完成记录样式；未跑带 DSN 用例（票面不要求，零行为）；非作者评审由推送方另派，本条不冒充。

## Comments

- 2026-09-15 11:2x · 通道 1：立票（评审尾巴，推送方处置时点名）。只写票面，未动代码。
- 2026-09-15 12:1x · 通道 4：认领即完工（task `1b28ece5`；分支 `mcp4-sacc28` 基 `7160fe67`，代码 tip `d6b1f773`）。完成记录见上；树不拆，留推送方重放。
- **2026-09-15 12:3x · 进 main 记录 · 通道 1 推送方**：**评审门推送方自审**（纯注释零行为，照 [25](25-pp-sacc24-review-standards-tails-declared-comment-scope-and-sentinel-sentence-qualifier.md) 先例）：`git diff -U0 7160fe67 d6b1f773 -- internal/` 取全部 `^[+-]` 行、去文件头、滤掉 `^[+-]\s*//` 后为空——三文件九增两删全是注释行；逐句对票面做法 1–3：`ExternalFundsFacts.Save` 头注那句插在「到不了这里」之后（判断项 ①，接受——那句正接着「顺序到达……到不了这里」讲两答为何不同，段末反而离题）；`ports.FundsFactAlreadyAdopted` 短版「撞约束才答它」（判断项 ④，接受）；`CorrectFact` 分支半句只引 `ExternalFundsFacts.Save` 符号名（判断项 ③，接受）；「分得出是输掉竞态而非重放」替票面「分得出后者」（判断项 ②，接受——`已采用`自身分重放 / 输竞态两格，版本字面分的正是这两格）。注释全中文、无行号无计数。**重放**：隔离检出 `%TEMP%\idp-replay-wave4` @ `49ffc96c`，`cherry-pick d6b1f773 88ca0a2e` 零冲突 → `981a6f15` / `ee3ec953`，`git diff 88ca0a2e ee3ec953 -- internal/settlementaccounting` 与本票 .md 均空；同树再叠 pp-seams/06 `69189208→4ca2ccac`、sa-cc/27 取证 `af2b59db→676cc09b`，批 tip `676cc09b`；清点在 `4ca2ccac` 重生成零差（本票零增删，作者预报同）。**验证（推送方全量一次，`4ca2ccac`——之后只叠一笔纯 .md）**：`gofmt -l` 空、`go build ./...` / `go vet ./...` 0；占 55432 → 带 DSN `go test -p 1 -count=1 ./...` **113 ok / 0 FAIL / 16 无测试 / 0 cached**（134 s）→ 释。本簿记笔在 `676cc09b` 之上 → 共享 main `merge --ff-only` → `ls-remote` 核 `49ffc96c` 未动 → `push <sha>:main`。分支 `mcp4-sacc28` → `merged/`、远端删；作者树由通道 4 比内容后拆。
