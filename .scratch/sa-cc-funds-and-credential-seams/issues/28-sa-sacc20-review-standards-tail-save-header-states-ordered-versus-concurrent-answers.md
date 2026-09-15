# sa-cc/20 评审 Standards 尾巴：`Save` 头注与 `FundsFactAlreadyAdopted` 注只写「两种都按当前链头作答」，没写出「顺序到达`未受理` / 并发到达`已采用`」这条不对称——同一业务事实两张脸，读注释的人分不出

Category: chore
Status: ready-for-agent——2026-09-15 11:2x 通道 1 立票（sa-cc/20 非作者评审 ← 通道 3 Standards 非阻断 ①，推送方处置「合一张 A 类零行为尾巴」）。零行为，只注释
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

## Comments

- 2026-09-15 11:2x · 通道 1：立票（评审尾巴，推送方处置时点名）。只写票面，未动代码。
