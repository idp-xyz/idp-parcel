# 103 PS 终局重派生进追踪投影时不带前身，新旧两代终局同时算当前有效

Category: bug
Status: needs-triage——2026-10-19 Claude Code 子代理立（票 [97](./97-demo-disclosure-policy-keeps-the-final-dimension-pending.md) 实现时读代码发现；没有实走，证据只是代码阅读，钉 `5f0aa66d`）。
Blocked by: 无
地盘：`internal/visibilityexception/adapters/parcelshipment/derive_on_final_outcome.go`（终局译装器）；要裁的那一格可能还碰 PS 终局采用账的读口。
出处：VE CONTEXT「被替代条目留档不参与派生」与「终局事实有效性变化也不能被伪装为人工重开」；PS 终局同源重派生（`FormParcelFinalHandler.rederive`，ADR-0117 纪律半「按来源声明的更正关系回指前版成链」）；[ADR-0138](../../../docs/adr/0138-voided-carrier-pickup-voids-the-label-service-final-as-a-third-ledger-record-and-rejudges-on-remaining-basis.md)（Proposed，终局失效）。

## 现象（读代码）

- PS 同源重派生一份终局时，采用账多一行新记录：采用键的版本维换成新责任结果的版本，`ParcelFinalOutcome.PriorVersion()` 指回前一代终局。新旧两行都经 `parcel-shipment.final-outcome.formed` 发出。
- VE 的终局译装器 `finalProjectionCommand` 把记录译成已接受源事实。事实引用是 `final-outcome/<包裹>/<责任结果种类>`，版本取采用键的版本维，`Supersedes` 不填。TF 与 CC 的译装器按源侧给的前身填 `Supersedes`（例如有效交付那一只），终局这一只没有。
- 于是同种类重派生之后，投影的当前有效集里同一事实引用有两代都是已采用终局：`CurrentlyEffective` 只按源上下文给出的替代关系滤，源侧这里给了关系，翻译时丢了。

## 影响

- 映射目录若给 `final-outcome-adopted` 登一行里程碑，时间线上同一终局出现两次。演示映射今天没登这一类，所以演示上看不见。
- 票 97 的客户视图终局一维只问「投影里有没有已采用的终局」，两代都在时答案仍对。ADR-0138 若被接受、终局失效落成第三种记录并发给 VE，失效之后被失效的那一代仍在当前有效集里，终局一维会继续展示一份 PS 已经说不成立的终局。

## 三类

- 机制：终局译装器把 PS 给出的前一代关系译成 `Supersedes`。
- 产品策略：无。
- 租户取值：无。

## 要裁的

- 前身怎么译：PS 给的前身是终局版本（`FinalOutcomeVersionID`），VE 的事实版本是采用键的责任结果版本，两套号不同。译装器按前一代终局版本反查它的采用记录、取那条记录的键版本，还是 PS 在记录上另给一格前一代的键版本——前者只在 VE 一侧动，后者动 PS 读口。
- 重派生换了责任结果种类时，事实引用跟着换，两代不在同一引用下，替代关系照 VE 硬句不成立。这一格是否另按冲突处理，要一并裁。
- ADR-0138 落地时，终局失效怎样进投影（第三种事实类型，还是一份不采用的新版本替代前代），属那张实现票；本票只把重派生这一半接上。

## 规模

S–M（译装器一处加反查；若改 PS 读口则 M）。
