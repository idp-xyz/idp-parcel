# 37 PS 收寄后服务处置决定没有模型：越过取消边界只记「待处置」，没有决定、没有下游请求

Category: enhancement
Status: needs-triage——建议 R3（不急）：越过取消边界的包裹今天答 `DISPOSITION_PENDING` 并停在那里，退运、拦截、改送、服务终止都没有决定面；首发若不启用收寄后处置，运营可走线下。2026-10-21 文档自洽 06 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc06。分级由协调方定稿。
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
