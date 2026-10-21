# 39 PS 来源保全之后建单失败的提交，重发只答「已有结果」，这份提交再也不建单

Category: bug
Status: needs-triage——建议 R1（上线前必修）：静默丢单。来源保全与建单是两个事务（首个消费者切片简报「事务边界」），保全提交之后，归属判断依赖失败或建单事务失败都答 5xx；客户照 ADR-0022 原样重发，提交编排按已保全的来源只答 `EXISTING_RESULT`、不指名委托，不再走归属与建单。客户拿到一个「已有结果」，系统里却没有这份委托。第一次数据库抖动就会撞上。2026-10-21 文档自洽 06 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc06。分级由协调方定稿。
Blocked by: 无
出处：文档自洽 06 修订 UC-PS-001 结果语义契约（「没有形成答案」那一段，评审轮补「已有结果不带原归属结果」）与 [ADR-0256](../../../docs/adr/0256-technical-non-formation-is-a-reason-of-undecided-not-an-outcome-of-its-own.md) 时读码所得，不在审计报告条目里；UC-PS-001「给开发的交接」。

## 现象

保全一经提交就不随后续步骤回滚（UC-PS-001 步骤 2 要求）。后续一步失败时，那次请求留下了保全，却没有委托。重发同一请求时，提交编排认出已保全的来源，走重放那条路：只追加一次观察，按来源身份找委托，找不到就答一个不指名委托的 `EXISTING_RESULT`。

## 证据

- `cmd/parcel-api/assemble_submission.go`：`preservationBoundary` 给保全单独开事务，头注写「保全一经提交就不随后续步骤回滚」；`submissionBoundary` 是建单的第二个事务。
- `internal/parcelshipment/application/submit_shipment_request.go`：`Handle` 在 `Preserve` 之后，`DecideProductionOwnership` 出错、签发标识出错、`Insert` 出错都直接上抛；`resolvePreserved` 对同内容的重发只答 `OutcomeExistingResult`，「绝不重判归属、也不建第二份委托」，`FindBySourceIdentity` 找不到时不指名委托。
- 文档：UC-PS-001 元数据「未决结束：本次处理没有形成生产归属或接受/拒绝决定，原始提交和处理尝试可安全续办」；ADR-0022「没有形成答案」由调用方重发。

## 三类

机制。无产品策略、无租户取值。

## 规模

M：重放那条路要分得出「保全过、建成了」「保全过、归属挡下了」「保全过、什么都没形成」；最后一格续办建单（重新判归属、照常建单），不当作已有结果；补真库用例（建单事务失败后重发建成委托，且只建一份）。

## 最小的诚实修法

1. 保全记录上记下这次处理走到了哪一步（至少分「归属已判」与「委托已建」），或由重放路按来源身份同时查归属判断与委托。
2. 重放时：委托已建 → 照旧 `EXISTING_RESULT` 并指名；归属已判且挡下 → 答原归属结果（今天 `resolvePreserved` 不带归属决定，UC-PS-001 提交答复「已有结果」一行要求返回原归属结果，一并补上）；两者都没有 → 按首次提交继续判归属与建单，并发下仍由建单的唯一键保证只建一份。
3. 补用例：`Insert` 失败后重发建成委托；归属依赖失败后重发重判归属；归属挡下之后重发带回原归属结果。

撤回与资料修订编排的同类短路归 [code/44](./44-withdrawal-and-amendment-replays-short-circuit-after-an-undecided-round.md)，本票只管提交。

## 要裁的

- 归属挡下（`OWNERSHIP_UNRESOLVED`、`ADMISSION_PAUSED`）之后的重发今天也只答已有结果、不重判；UC-PS-001 写归属未决「可安全续办」，而首发试点叠加条件写暂停「不自动恢复」。两格要不要随重发重判，本票落地时提出，交 PS owner 定；本票的必修只是「什么都没形成」那一格。
