# 37 PS 收寄后服务处置决定没有模型：越过取消边界只记「待处置」，没有决定、没有下游请求

Category: enhancement
Status: in-progress——2026-10-28 作者完工，待隔离评审与进 main，分支 claude-code37（代码最后一笔 __SHA__）。此前：in-progress——2026-10-27 会话 tops-20（idp-parcel-91）接手 tops-26 断在中途的「上线前必须」一组（冻结清单「六」），分支 claude-code37（续 tops-26 已有 worktree 半成品）。此前 needs-triage——建议 R3（不急）：越过取消边界的包裹今天答 `DISPOSITION_PENDING` 并停在那里，退运、拦截、改送、服务终止都没有决定面；首发若不启用收寄后处置，运营可走线下。2026-10-21 文档自洽 06 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc06。分级由协调方定稿。
Blocked by: 无
出处：[audit/ps-code.md](../audit/ps-code.md) 第 14 条顺带点出「UC-PS-006 的收寄后处置决定」未实现，审计未单列；文档自洽 06 修订 UC-PS-006 时确认，见其「给开发的交接」。

## 现象

PS CONTEXT 与 UC-PS-006 写了收寄后服务处置决定的语言、规则与生命周期，代码只有取消编排在越过边界时记下的一格`待处置`。

## 证据

- `internal/parcelshipment/application/cancel_parcel.go`：`DecideParcelCancellation` 答 `ErrIntakeBoundaryCrossed` 时记 `ports.RecordDispositionPending`、结果 `DISPOSITION_PENDING`，头注写「处置判断由后续独立请求形成」；`internal/parcelshipment` 里没有这个后续请求的编排、领域对象或端点。TF 的另起旅程以 `tfdomain.ServiceDispositionDecision` 作依据种类，引用的正是这份 PS 一侧还不存在的决定。
- 文档：PS CONTEXT 词条「收寄后服务处置决定」、Rules「收寄后服务处置必须按包裹形成明确决定」一条、生命周期「收寄后服务处置决定形成 → 各对象所有者分别承接」；UC-PS-006 `AT-PS-083`、`AT-PS-084`、`AT-PS-088`。

## 三类

机制：决定的形状、只追加、向路由、节点、运输、关务、结算提出范围化请求。产品策略：处置去向的封闭集合。租户取值：谁有权、哪些去向对哪个产品可用，属 `PAR-COM-05/06/17` 与 `PAR-NET-11/13/14/15`，未登记答未配置。

## 规模

L：领域对象与只追加登记、授权端口、命令口与操作者或客户面、向各对象所有者的请求信封、各下游的承接结果读回。可拆：先立决定本体与授权，下游请求逐个接。

## 最小的诚实修法

照 UC-PS-006「本用例包含」与 CONTEXT 词条立收寄后服务处置决定：固定对象、目的、决定来源、实际决定方、授权、原因、生效时间和责任范围；任何必要依据缺失时保持未决；决定只向对象所有者提出范围化请求，不直接执行；监管退运转 `UC-TF-001`，普通退运转 `UC-TF-007`。

## 要裁的

- 处置去向的封闭集合（继续、拦截、改送、退运、服务终止及其他）由本票落地时按 CONTEXT 词条提出，交 PS owner 认可。


## 完成记录（2026-10-28，作者）

起点：`2afdd0b1e`（rebase 到 origin/main 之后；tops-26 的半成品按「WIP 接手」一笔提交后随之 rebase）。

**做了什么**
- 接手并核过半成品，与票面「最小的诚实修法」一致，沿用：领域 `PostIntakeDisposition`（只追加决定链、去向封闭五个、所有者请求封闭表）、端口、postgres（`PostIntakeDispositions`、`PostIntakeDispositionRequests`、Outbox 信封交接）、迁移 `access_identity/0028`（决定种类加 `POST_INTAKE_SERVICE_DISPOSITION`）与 `parcel_shipment/0033`（待处置记录保全请求四件、等回应部分索引、决定表）、应用编排 `DecidePostIntakeDispositionHandler`、取消编排越界时保全请求四件。迁移号与 ADR 号已核：0028、0033 在 origin/main 与各 claude-* 分支未被占。
- 半成品引的 ADR 号 0336 已被 `claude-psb22` 占用，改为取 **ADR-0332**（0332、0333 核过空；`claude-code27` 占 0334），代码注释与迁移头注同步。
- 补：HTTP `POST /post-intake-disposition-decisions`（`OperatorDecisionIntake` 加一种）与 `GET /post-intake-disposition-requests`（`OperatorOperationsReadIntake` 加一口）、`cmd/parcel-api` 装配与端点登记（`assemble_post_intake_disposition.go`、`endpoints_post_intake_disposition.go`）、ADR-0332、UC-PS-006（状态、`DISPOSITION_PENDING` 行、交接行、`AT-PS-174` 至 `AT-PS-176`）、AT 盘点 ① 三行与追记、分区主体登记一行。
- 编排把 `Adoptions` 缩成窄口 `ports.ResponsibilityStartView`。

**代决与越权风险点**：见 ADR-0332 文末四点（去向集合与请求表粒度、授予单立一种、`终局成立`与`前一份已变`等确定答案的取舍、信封不带决定人与依据正文）。待 PS owner 复核，知会 NO、TF、NR owner，访问身份 owner 复核授予单立。

**判据对用例**：`AT-PS-174`（决定链、信封同笔、并发、重发、更正、事务外拒跑）、`AT-PS-175`（授权与前提）、`AT-PS-176`（去向与请求表，领域）已有测试；`AT-PS-083`、`AT-PS-084`、`AT-PS-088` 仍在④——决定本体与信封已落，各所有者对信封的承接、承接拒绝的保留与监管退运转 `UC-TF-001` 未落。

**自验**（含 PG，DSN 55432）：`go build ./...`；`go vet` 受影响包；`gofmt -l` 无输出；`go test -count=1 -p 1` 通过 `./internal/parcelshipment/...`、`./internal/accessidentity/...`、`./cmd/parcel-api/...`、`./internal/architecture/...`、`./migrations/...`。`tools/adr-index -check` 与 `tools/at-coverage-counts -check` 只报生成物过期（协调方进 main 时重生成）。未改种子，未灌演示库。

**留给协调方**
1. 重生成 ADR 索引与 AT 盘点计数。
2. 下游承接（不挡上线，但在此之前信封入队后无人消费，决定本身可审）：TF 消费信封另起备用/退运旅程并以 `POST_INTAKE_DISPOSITION/<包裹>/<版本>` 作服务处置决定依据（UC-TF-007）、NO 扣住与按退运装载、NR 重新规划与释放、承接结果读回（`AT-PS-088`）、监管退运转 UC-TF-001（`AT-PS-084`）。
3. 管理台页与演示租户的授予种子未随票；授予是租户取值，演示租户若要演示需另灌 `POST_INTAKE_SERVICE_DISPOSITION` 授予。
4. `cmd/parcel-dispatch` 未改。
