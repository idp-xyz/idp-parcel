# PS `adapters/partycommercial` 的三条评审尾巴：两处计数措辞、一段历史头注、主动拒绝那只缺对称的动作守卫

Category: enhancement
Status: resolved——2026-09-10 16:0x 通道 2 交活（task-ca3d37b0，分支 `mcp2-psr08` 基 `0e8d048a`；代码两笔 `29b08395`（①② 纯注释 + 立票）/ `9decab60`（③ 守卫 + 用例），票面笔随其后；见「完成记录」；进 main 的 SHA 由推送方补记）。此前 in-progress——15:5x 立票即认领。理由：零待裁，三件都是评审已点过的——[03](./03-source-data-amendment-authorization-needs-a-pc-action-kind-and-a-decider.md) 通道 6 评审 Standards ① 与作者判断题 5（推送方在「进 main 记录」里判「归一张 PS 簿记小笔」），[wbr/04](../../wiring-baseline-remainder/issues/04-pc-manual-review-predicate-answers-who-may-not-whether-and-nobody-asks-either.md) 评审的「观察（不在本票地盘）」
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

## 完成记录（通道 2 · 2026-09-10 · 分支 `mcp2-psr08`，基 `0e8d048a`）

**第 1 件 · 计数措辞**（`29b08395`，纯注释）
- `SourceDataAmendmentRequestCoordinates` 头注：「RequesterKind 取 PC 的两格；OperatorRole 只在运营角色代录时读。用 Kind 而不用……理由与 PC 两个请求方构造器分立的理由相同」→「本适配器接客户账户 / 运营角色两种请求方，RequesterKind 说是哪一种；OperatorRole 只在运营角色代录时读。用 Kind 而不用……理由与 PC 把请求方构造器按种类分立的理由相同」——「两种」数的是本文件 `formSourceDataAmendmentRequest` 自己 switch 的 case，改 case 的人必路过；「两个构造器」那处顺手一并去数。
- `SourceDataAmendmentAuthorizationAdapter` 头注：「与撤回、拒绝两只同形」→「形状照 withdrawal_authorization.go / active_rejection.go」。
- 顺带：同文件用例头注「与撤回、拒绝两只的区别只有一处」→「与 withdrawal_authorization_test.go / active_rejection_test.go 钉的表相比，区别只有一处」（同一条理由，评审未点名）。

**第 2 件 · 历史头注**（`29b08395`，纯注释，实现一字不动）
- `UnconfiguredSourceDataAmendmentAuthorizer` 头注：原「资料修订授权口在提供方那半还没立起来之前的如实答复……提供方缺的不是一条登记（PC 授权动作封闭集今天没有「资料修订」这一格……票 ps-port-remainder/03）……提供方那半落地后在装配点换成照 WithdrawalAuthorizationAdapter 形状的适配器，本类型随之退场」→ 新「资料修订授权口的未配置替身……它不是生产装配的选择——那里接的是 SourceDataAmendmentAuthorizationAdapter（PC 裁定编排 + 真授权册 + 真委派册，票 03 落地）。它留给两类地方：装配用例要一个不碰 PC 库、答复不随询问内容变的授权口；以及没有 PC 库可接的路径」。取「未配置」不取 error 的那段理由（ADR-0063）原样保留。
- 顺带：其用例 Covers 句「提供方那半没立之前，未配置的授权适配器……」→「不接 PC 库的未配置替身……真适配器的翻译表在 source_data_amendment_authorization_test.go」。

**第 3 件 · 对称的动作守卫**（`9decab60`，加一道拒译的守卫，三值翻译表与导出签名不动）
- 读下来就是「同形守卫」：`ManualReviewAuthorizationAdapter.AuthorizeManualReview` 在 `formed` 之后、问提供方之前比 `request.Action()`，不符即 `%w: request mapping formed action %q, want MANUAL_REVIEW` 包 `ErrUntranslatableAnswer`。派单里写的「`withdrawal_authorization.go` 里守卫的形」实为复核那只——撤回那只没有也不能有（PC 今天没有撤回动作，映射借现有动作占位，它头注写明），本票不给撤回加。
- `ActiveRejectionAdapter.AuthorizeActiveRejection` 加同位置同形一道：`request.Action() != pcdomain.ActiveRejectionAction` → `ErrUntranslatableAnswer`，不问提供方；方法头注补一段理由（复核权被读成拒绝权）并写明撤回那只为何没有。
- 用例 `TestActiveRejectionOnlyTranslatesAMappingThatFormsTheRejectionAction` 两个子测：正——折出拒绝动作的映射到达提供方（读失败哨兵证）且不是 `ErrUntranslatableAnswer`；反——折出 `ManualReviewAction`（借 `reviewPCRequest(t, pcdomain.ManualReviewAction)`）→ `ErrUntranslatableAnswer`、哨兵未出现（提供方没被问）。红先于绿：反格先红——当时请求原样交到了提供方。
- 连带：`manual_review_authorization.go` 那句「这一道守卫主动拒绝那只适配器没有，这里加上是因为……」随之为假，改成「这一道守卫是本口的全部意义所在……主动拒绝那只（ActiveRejectionAdapter）有同形的一道，方向相反」。
- 生产影响：`cmd/parcel-api/assemble_review.go` 装的 `ActiveRejectionAdapter` RequestSource 为 nil，在守卫之前就停在「映射未配置」，行为不变；`cmd/**` 未碰。

**验证**（工作树与 HEAD `9decab60` 逐字相同、`git status --untracked-files=all` 零行，故等价于在提交状态上跑）：`gofmt -l` 空；`go build ./...` / `go vet ./...` 全仓 0；`go test -count=1 -v ./internal/parcelshipment/adapters/partycommercial/ ./internal/architecture/...` 两包 ok，**PASS 322 / SKIP 0 / FAIL 0**（无 DSN——第 3 件不动导出签名，反向依赖不必带 DSN）。清点未动（不动文件面）。

**给评审的判断题**：(1) 守卫拒译用 `ErrUntranslatableAnswer`——与复核那只同一个词汇，wbr/04 评审已对该哨兵「所指略错位」判「接受，不必改」，本票照抄不另立哨兵；(2) 第 1 件把「两个构造器」也一并去数，评审没点名，属同一条纪律的顺手。

## Comments

- 2026-09-10 15:5x · 通道 2：立票即认领（派单 task-ca3d37b0，时限 40 分）。
- 2026-09-10 16:0x · 通道 2：三件落地，分支 `mcp2-psr08` 代码 tip `9decab60`，详见「完成记录」。第 3 件有 .go 行为改动（一道守卫），按派单等非作者评审；第 1、2 件纯注释。
- **评审 ← 通道 4 · 钉 `f2c96f02` · 16:2x**（task-a7df43ca，非作者；隔离检出只读，跑过 gofmt / vet / 适配器包 `-count=1` ok；两轴串行互不引用；由推送方代落）。
  - **Spec**：**阻断 0 · 非阻断 1 · 判断题 2**。(a) 守卫位置与答法属实：`!formed` 之后、`NewTenantID` / `adjudicate.Handle` 之前比 `request.Action() != pcdomain.ActiveRejectionAction`，答 `%w: … want ACTIVE_REJECTION` 包 `ErrUntranslatableAnswer`，与 `manual_review_authorization.go` 的 formed → Action 守卫 → 提供方同序同形；三值翻译表一字未动。(b) 行为不变属实：`cmd/parcel-api/assemble_review.go` 传 nil RequestSource，适配器首句即 return error，守卫不可达。(c) 反格真证「不问提供方」：`grantStoreDouble{err: providerAsked}` 让 `LoadEffectiveGrants` 报哨兵，正格 `errors.Is(err, providerAsked)`、反格 `Untranslatable && !errors.Is(err, providerAsked)`。(d) 撤回那只不加成立：其头注原句「PC 的 AuthorizedAction 今天也没有撤回动作……只转交 RequestSource 声明的请求，不代它挑动作」，无目标动作可比。**非阻断**：`active_rejection.go` 新头注写撤回那只「映射借现有动作占位」——是推论不是引文，与「理由钉在它的头注上」那半句字面对不上；改一句或保持归作者。判断题 (1) 以 `ErrUntranslatableAnswer` 表请求侧映射问题、与复核那只同词汇——wbr/04 评审已判接受，认可；(2)「两个构造器」顺手去数，同一纪律，认可。
  - **Standards**：**阻断 0 · 非阻断 1 · 判断题 1**。注释全中文；新句跨文件引用全用文件名 / 符号名，无行号；计数自查：「两种请求方」数的是同文件 switch 自己的 case、「两类地方」同句内列举，都不是数别处。**非阻断**：`unconfigured_source_data_amendment_test.go` Covers 句保留「票 ps-port-remainder/04 **第 2 条**」序位——本笔改的同一行未动的前半，非本票引入，可随下一批序位清剪一并改，不挡。判断题：`active_rejection.go` 头注引「PC CONTEXT」不带词条名，与复核那只同形照抄，认可。**结论：两轴无阻断，可重放。**
- **进 main 记录 · 通道 1 · 2026-09-10 16:3x**：分支 `mcp2-psr08` 三笔在 `d3b82200` 上 cherry-pick 全干净——`29b08395→d731b6c2` / `9decab60→0806a7e6` / `f2c96f02→c5b3edc1`；适配器包与票面与分支逐字相同。验证钉 `c5b3edc1`（隔离 detached 树 `%TEMP%\idp-replay-psr08`）：`gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；占 55432 广播后含 DSN `go test -p 1 -count=1 ./...` 退 0，**104 ok / 0 FAIL / 15 无测试 / 0 cached**（120 s）；释号。本笔（本条 + tasks.md）在 `c5b3edc1` 之上，纯 .md；`ls-remote` 核 `d3b82200` 未动后 ff 并 `push <sha>:main`。分支指针改 `merged/mcp2-psr08`、远端删；树由作者比内容后拆。评审两条非阻断（撤回头注那半句、Covers 句「第 2 条」序位）随下一批 PS 序位清剪带走，不另立票。
