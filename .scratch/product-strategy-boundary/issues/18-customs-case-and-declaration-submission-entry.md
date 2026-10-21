# 18 关务立案与提交申报：生产入口与触发面

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-psb18（2026-10-21 Claude Code 作者子代理收口，完成记录见文末）。此前 in-progress——2026-10-20 分诊并认领，分支 claude-psb18。此前 ready-for-agent——2026-10-20 Claude Code 会话代 CC owner 分诊（用户原话「你作为业务和系统专家自决……解决所有的问题」），「要裁的」三问裁定见文末「分诊裁定」。更早的 needs-triage——2026-09-24 通道 2 立票（用户令通道 2 独立承接）；分诊时先定「要裁的」再转 ready-for-agent
Blocked by: 无；触发面与 [routing-first-cut/12](../../routing-first-cut/issues/12-cc-customs-applicability-judgment-for-route-candidates.md)（候选的关务适用性）、[票 10](./10-cc-declaration-channel-and-public-regulatory-reference-configuration.md) 第 1 项（申报发送连接器）相邻，分诊时划清
地盘：`internal/customscompliance`（入口与触发）、`cmd/parcel-api` 或 `cmd/parcel-dispatch` 的装配。
出处：[票 05](./05-demo-journey-criterion-evidence.md) 格 9——开发主线重定级表 PN-05 第一项记「未见缺口」，动线取证补出这一格。

## 现象

`customscompliance/application` 的 `NewEstablishCaseHandler`、`NewSubmitDeclarationHandler` 在 `cmd/` 零引用；接受决定的扇出只到 VE 与初始路由。生产进程里没有任何端点或消费门会建案或提交，外部结果口因此答「归属不上」（票 05 格 11「08 重走」）。

## 做什么

- **机制半边**：两份编排进生产装配，各有入口。
- **产品策略半边**：触发面的内置策略——谁、在什么业务时点、为哪些包裹建案；提交在什么条件下发起。

## 要裁的

1. 立案：只给运营命令面，还是另有内置触发（例如消费接受决定并按关务适用性判断派生）？若是后者，与 routing-first-cut/12 的判断口怎么分工。
2. 提交：[首发范围](../../../docs/product/PILOT-SCOPE.md)要求授权确认后提交、不启用生产自动申报——提交是否只给命令面，授权确认从哪一本册读。
3. 入口形态：隔离形态下照 operator-channel/08 放行，生产形态等操作者渠道（ADR-0149 前线操作者族）。

## 完成判据

- 生产进程至少有一条路能建案并形成提交版本（隔离形态实测只记 `S`），票 05 格 9 越过；触发面有内置策略或显式的命令面，没有默认的自动申报。

## 分诊裁定

2026-10-20，Claude Code 会话代 CC owner 裁（用户原话「你作为业务和系统专家自决……解决所有的问题」）。读过：PILOT-SCOPE 关务各条、CC `CONTEXT.md`、UC-CC-001 / 004 / 005、ADR-0091 / 0146 / 0149 / 0150 / 0151 / 0204 / 0212 / 0222，operator-channel/08，routing-first-cut/12，本目录 05、07、10。决定与逐条越权风险点落在 ADR-0227，这里只记三问的答案。

1. **立案**：本票只给显式的运营命令面 `POST /customs-cases`，挂操作者渠道「运营决定」能力面的新一种决定（建案）。包裹由请求指名，客户归属与来源资料引用由服务端按信封租户在 parcel-shipment 查当前已接受委托取出，不从请求体收；查不到已接受委托答未受理（UC-CC-001「只有提交版本而尚未接受的范围不得建立正式关务案件」）。要不要建案仍只由建案要求规则册答，未登记答未决、指名未配置。**内置触发不在本票**，另立 [35](./35-built-in-customs-case-trigger-from-acceptance.md)：接受决定不带监管程序与法定义务范围，routing-first-cut/12 的关务适用性判断按路由候选作答、只答可用 / 不可用 / 状态未知，也不产出程序与义务；派生立案要一套「已采用路由的关务段 → 辖区、方向、程序、义务」的判断方法，那是新的产品策略。分工：rfc/12 的判断口是路由判断的输入，不触发建案，也不被建案当依据复用；35 若做，读接受决定与已采用路由，仍经建案要求规则册判。立案不等于申报，内置立案与「不启用生产自动申报」不冲突——不在本票是因为它不便宜、边界没定，不是因为它被禁止。
2. **提交**：只给命令面，生产进程没有任何自动提交路径。批准与实际提交分两口、两种运营决定（CONTEXT「请求、批准、拒绝、撤销、申报渠道账号使用和实际提交权限必须分别表达」）：授权确认 `POST /customs-submission-authorizations`（UC-CC-004 首发人工确认：确认人取信封里的操作者，单元当前就绪须有效，就绪没登答未决、指名未配置）；提交执行 `POST /customs-declaration-submissions`（UC-CC-005：就绪与授权须同时有效，单元上没有授权答未决 `AUTHORITY_UNCONFIGURED`）。授权读 access_identity 操作者册的运营决定授予（决定种类加准入范围），三种决定种类追加进 `accessidentity` 与迁移 `access_identity/0018`；谁被授予哪一种是租户取值，没有授予答 `OPERATOR_NOT_GRANTED`。申报就绪仍由既有的就绪判断册承担，与授权分开。提交形成不可覆盖的提交版本与首次提交尝试；没有发送连接器，尝试记「已登记待发送」（UC-CC-005 结果契约「提交已登记待发送」），不带发送时间，不收调用方给的尝试结果与发送时间——迁移 `customs_compliance/0027` 放开这一格。发送与「尝试已发起」那一步归 [票 10](./10-cc-declaration-channel-and-public-regulatory-reference-configuration.md) 第 1 项。
3. **入口形态**：照 ADR-0150 决定三与最近先例（ADR-0204、ADR-0212），三口直接挂操作者渠道的「运营决定」能力面，合成租户 `SYN-TENANT-01` 与真实租户走同一条命令面，不加隔离放行（写开关换不了运营决定口）。ADR-0149 的前线操作者族已由 operator-channel/15 落地，票面「生产形态等操作者渠道」那一半不再等。

UC-CC-004 的拒绝、撤销、再次申请，授权目标绑定到组成 / 资料 / 渠道，受理口组成核对案件关联，以及既有授权登记 CLI 与业务角色确认并存的处理，另立 [36](./36-submission-authorization-business-role-face-remainder.md)。

## 完成记录

2026-10-21，Claude Code 作者子代理，分支 `claude-psb18`（基 `8bf7a231`）。决定与逐条越权风险点在 [ADR-0227](../../../docs/adr/0227-customs-case-and-declaration-submission-enter-production-through-operator-decision-faces.md)；证据只记 `S`。作者不自评，评审另派非作者。

### 做了什么

- **三口三种运营决定**：`accessidentity` 追加 `CUSTOMS_CASE_ESTABLISHMENT`、`CUSTOMS_SUBMISSION_AUTHORIZATION`、`CUSTOMS_DECLARATION_SUBMISSION`，迁移 `access_identity/0018` 重建决定种类 CHECK。`cmd/parcel-api/endpoints_customs_decisions.go` 登 `POST /customs-cases`、`/customs-submission-authorizations`、`/customs-declaration-submissions`，挂 `decisionFace`；`operatorDecisionIntakes` 多一格 `customs`（CC 的运营决定认证方与 Intake，判准入范围）。
- **建案**：新端口 `AcceptedParcelSourceView` 与消费侧适配器 `customscompliance/adapters/parcelshipment`（译 PS 按包裹反查当前已接受委托的口），新编排 `EstablishCaseForParcelsHandler` 按包裹查客户归属与来源引用后交给既有建案编排；未接受答 `PARCEL_NOT_ACCEPTED` 逐件列出。既有建案编排的未决此后带原因（规则未配置 / 读不动、案件册、标识签发、源上下文）。
- **授权确认**：新编排 `ConfirmSubmissionAuthorizationHandler`：确认人取信封操作者，就绪须当前有效（没登答 `READINESS_UNCONFIGURED`，失效答 `NOT_READY`），授权依据「`CUSTOMS_SUBMISSION_AUTHORIZATION`@确认人」落既有提交授权册；已有不另形成、不顶替，失效答 `AUTHORIZATION_NO_LONGER_EFFECTIVE`。
- **提交执行与「已登记待发送」**：领域 `AttemptResult` 加 `REGISTERED_PENDING_SEND`，与发送时间配对（`AttemptSendTimeMatches`，构造、读回、受理三处同一份），受控重发不能停在这一格；迁移 `customs_compliance/0027` 放开结果集合、`sent_at` 可空并配对；提交编排受理处先配对。提交执行口的载荷没有尝试结果与发送时间两格，首次尝试一律待发送。
- **装配**：`cmd/parcel-api/assemble_customs_decisions.go` 三只编排全接真库缝，各包一笔事务，未决整笔回滚。
- **没有自动提交路径**：`cmd/parcel-dispatch/customs_no_automatic_submission_test.go` 两条用例（接受扇出后关务各册与意图为零；扫 `cmd/` 生产源码，建案 / 授权 / 提交编排只在上面那一个装配文件里构造）。
- **文档**：ADR-0227（Status 带索引注记，不手改 adr/README）；CC `CONTEXT.md`「提交尝试」词条与「提交与外部监管结果」生命周期首条；UC-CC-001 / 004 / 005 验收示例加 `AT-CC-437`–`445`；AT 盘点移入 `AT-CC-020`、`078`、`105`、`107`、`112`、`122` 到①并加 437–445 九行（不改「总览」）；票 10 加一条 Comment；立票 35、36（分诊那一笔）。
- **演示种子**：配置员加三种授予（`data/access/02`）、准入对照（`03`）三行，新步 `steps/460-customs-declaration-faces.sh` 登三份运营决定权威区间（`data/governance/42`–`44`）与演示单元 `SYN-UNIT-CN-EXPORT-DEMO-01` 的就绪（`data/customs/31`，不登授权）；运行时脚本 `declare-one-parcel.sh <包裹号>` 走三口；README 两处。

### 代决与越权风险点（详见 ADR-0227 文末）

代决：分诊三问照上文「分诊裁定」；批准与实际提交分两种决定、不强制职责分离；首次尝试记「已登记待发送」而不沿用待确认；建案按已接受包裹寻址；不加隔离放行。越权风险点九条：运维登记授权一路仍被提交执行认（1）；决定种类与授予粒度、party-commercial 的角色委派不问（2）；授权只绑单元（3）；受理口不核组成在案件里（4）；提交执行人不留痕（5）；授权依据不带授予号（6）；待发送与连接器的分工（7）；建案四维与角色照请求收（8）；未决整笔回滚与编排注释的出入（9）。1、3–6 进票 36，7 进票 10，8 进票 35。

### 用例（真库）

- `cmd/parcel-api/assemble_customs_decisions_test.go`：`TestCustomsCaseAuthorizationAndSubmissionRunThroughTheProductionFaces`（经生产端点建案 → 授权前提交答 `AUTHORITY_UNCONFIGURED` 不留行 → 确认授权 → 提交成版、尝试 `REGISTERED_PENDING_SEND` 且 `sent_at` 为空 → 三口重放都答已有、尝试与意图不翻倍）、`TestEachCustomsFaceOpensOnlyForItsOwnDecisionKind`（三人各持一种，互用答 `OPERATOR_NOT_GRANTED` 不写）、`TestUnconfiguredCustomsRegistersAnswerNotConfiguredThroughTheProductionFaces`（`PARCEL_NOT_ACCEPTED`、`CASE_REQUIREMENT_UNCONFIGURED`、`READINESS_UNCONFIGURED`）。操作者册用真库 `aipg.OperatorRegistry`（过 0018 的 CHECK）。
- `cmd/parcel-dispatch/customs_no_automatic_submission_test.go`：见上。
- 领域、持久化（0027 两条 CHECK）、编排、Intake、认证方、消费侧适配器、操作者册各一组，Covers 点名 AT-CC-437–445。

### 验证

- `scripts/test-full.sh`：`./internal/accessidentity/...`、`./internal/customscompliance/...`，以及两者的全部反向依赖（含 `cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-customs-register`、`cmd/parcel-access-register` 与各上下文的 `adapters/accessidentity`、`adapters/customscompliance`），`./internal/architecture/...`、`./migrations/...`：全部 ok。`go build ./...`、`go vet ./...` 干净，`gofmt -l` 空；逐笔提交单独 `go build ./... && go vet ./...` 都过。
- 演示动线（实测，只记 `S`）：55432 上一次性库 `CREATE DATABASE` 后 `seed.sh` 退 0（步 460 四行 `REGISTERED`）；起 `parcel-api`（操作者族 Dex 5556、首方族私起的一只集成发行方、治理自授权、隔离读写两开关）与 `parcel-dispatch`；`submit-one-shipment.sh` 答已提交，dispatch 下一拍成已接受；`declare-one-parcel.sh DPCL-7QRGRUU4N6LUP72ZUTLHKGEN6Q` 打出「案件 CASE-…；提交版本 DECLV-…；首次尝试 REGISTERED_PENDING_SEND」，库里案件关联的客户归属是 `SYN-ACCOUNT-01`、来源引用是「委托/当前提交版本」，提交版本的授权依据是「`CUSTOMS_SUBMISSION_AUTHORIZATION`@配置员」，尝试 `sent_at` 为空，案件建立与提交版本形成两封信 `PUBLISHED`（VE 投影消费）；重跑三口各答已有；查阅员令牌三口都答 403 `OPERATOR_NOT_GRANTED`；未登就绪的单元确认答 `READINESS_UNCONFIGURED`，就绪已撤销的单元 03 提交答 `NOT_READY`。走完停进程、删私起的发行方、`DROP DATABASE … WITH (FORCE)`。

### 票 05 格 9

越过：生产进程经 `/customs-cases` 建案、经 `/customs-submission-authorizations` 与 `/customs-declaration-submissions` 形成提交版本与首次尝试（演示租户实测只记 `S`）；触发面是显式命令面，没有默认的自动申报。格 11 的外部结果口此后有提交可归属，重走归票 05。

### 过程记录

收口时为核对 adr-index 在本 worktree 跑过一次 `git stash` / `git stash pop`：stash 列表各 worktree 共用，本树没有可存的改动，`pop` 弹出的是另一会话（`claude-dcg50`）放在栈顶的 stash。当即按原提交 `683fe655` 与原说明 `git stash store` 放回栈顶，核对本树多出的改动与那份 stash 逐字节相同后撤掉，本分支的提交不含它。那份 stash 的提交与内容未变，缺席约数分钟。
