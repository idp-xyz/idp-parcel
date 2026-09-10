# PS `adapters/partycommercial` 的三条评审尾巴：两处计数措辞、一段历史头注、主动拒绝那只缺对称的动作守卫

Category: enhancement
Status: in-progress——2026-09-10 15:5x 通道 2 立票即认领（task-ca3d37b0，分支 `mcp2-psr08` 基 `0e8d048a`）。理由：零待裁，三件都是评审已点过的——[03](./03-source-data-amendment-authorization-needs-a-pc-action-kind-and-a-decider.md) 通道 6 评审 Standards ① 与作者判断题 5（推送方在「进 main 记录」里判「归一张 PS 簿记小笔」），[wbr/04](../../wiring-baseline-remainder/issues/04-pc-manual-review-predicate-answers-who-may-not-whether-and-nobody-asks-either.md) 评审的「观察（不在本票地盘）」
Blocked by: 无

## 三件

1. **计数措辞**（03 评审 Standards ①）：`source_data_amendment_authorization.go` 里 `SourceDataAmendmentRequestCoordinates` 头注「RequesterKind 取 PC 的两格」、适配器头注「与撤回、拒绝两只同形」——数的是 PC 封闭集与本包别的文件，被数一侧增减时这句无声变旧（AGENTS.md「计数与行号同构」）。改成不数别处的写法：前者写「本适配器接客户账户 / 运营角色两种请求方」（数的是本文件自己 switch 的两个 case，改 case 的人必路过），后者指名先例文件不数只数。
2. **历史头注**（03 判断题 5）：`unconfigured_source_data_amendment.go` 头注写的「提供方那半还没立起来之前」「提供方那半落地后在装配点换成……本类型随之退场」自 03 进 main 起已是历史——真适配器在同包、生产装配已换，而它按派单保留。改准为「装配用例与无 PC 库路径的未配置替身」，实现一字不动。
3. **对称的动作守卫**（wbr/04 评审观察）：`ManualReviewAuthorizationAdapter` 有「映射折出的动作不是本口的动作 → `ErrUntranslatableAnswer`、不问提供方」这一道守卫，`ActiveRejectionAdapter` 没有——「一份映射若折出别的动作」的论证对拒绝那只同样成立（折出 `ManualReviewAction` 会把复核权读成拒绝权）。给 `active_rejection.go` 加同形守卫 + 一正一反用例。撤回那只**不加**：PC 今天没有撤回动作，它的 RequestSource 借现有动作占位（`withdrawal_authorization.go` 头注），守卫无可比对——那属 `PAR-COM-14` 落地时的 PC 侧工作。若读下来发现不是「同形守卫」那么简单（要改端口或裁答法），记「不做，理由」。

## 不碰

`cmd/**`；`internal/partycommercial/**`；psr/06 / 07 两件 ports 头注的序位引用（通道 5 痕迹清剪票 task-14712757 在改）；`withdrawal_authorization.go` 正文（理由见第 3 件）；任何行为——第 1、2 件纯注释，第 3 件只加一道拒译的守卫，三值翻译表不动。

## 完成判据

1. 三件逐件有「原句 → 新句」或「守卫 + 用例名」或「不做，理由」。
2. gofmt 空；`go build ./...` / `go vet ./...` 0；`go test -count=1 ./internal/parcelshipment/adapters/partycommercial/ ./internal/architecture/...`（无 DSN；第 3 件不动导出签名，反向依赖不必带 DSN）。
3. 清点不变（不动文件面）。

## Comments

- 2026-09-10 15:5x · 通道 2：立票即认领（派单 task-ca3d37b0，时限 40 分）。
