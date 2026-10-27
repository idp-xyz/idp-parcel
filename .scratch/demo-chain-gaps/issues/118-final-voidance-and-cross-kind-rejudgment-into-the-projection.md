# 118 终局失效与失效后异种改判怎样进追踪投影

Category: enhancement
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R3）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-19 票 [103](./103-a-rederived-final-outcome-enters-the-projection-without-its-predecessor.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。103 票面「要裁的」把这两格留给 ADR-0138 的实现票，而那张票要等 ADR-0138 被接受才立；本票替它把 VE 这一侧的两格先记下，免得实现时只改 PS。
Blocked by: [ADR-0138](../../../docs/adr/0138-voided-carrier-pickup-voids-the-label-service-final-as-a-third-ledger-record-and-rejudges-on-remaining-basis.md)（Proposed，待 PS owner 接受）
地盘：VE `adapters/parcelshipment/derive_on_final_outcome.go`（终局译装器 `predecessorOf` 与 `kindForFinalRecord`）；ADR-0138 的第三种记录若随 `final-outcome.formed` 发出，还碰 VE inbox 的终局信封译码。
出处：VE CONTEXT「来源事实替代关系只能由源上下文给出，且只在同一源上下文、同一事实引用的版本之间成立……跨源上下文或跨事实引用的分歧仍是冲突」「终局事实有效性变化也不能被伪装为人工重开」；UC-VE-002 `AT-VE-176`。

## 现状（分支 `claude-dcg103`）

- 103 让终局译装器把重派生的前一代译成同一事实引用下的前身：按终局版本找回前一代的采用记录，取它的采用版本。
- 前一代若不是同包裹、同责任结果种类的已采用终局，译装器按来源记录不一致确定性拒收（`ErrFinalRecordInconsistent`，`AT-VE-176`）。今天 `FormParcelFinalHandler` 只在来源同种时重派生，所以那一格只能是账坏了。

## ADR-0138 落地后会变的两格

- **终局失效**：ADR-0138 加第三种记录「终局失效」回指前版、包裹回到「当前无有效终局」。它怎样进投影——第三种来源事实类型，还是一份替代前代的新版本——要裁；不接的话，失效之后被失效的那一代仍在当前有效集里，客户视图终局一维（票 97）继续展示一份 PS 已经说不成立的终局。
- **失效后按剩余依据改判出异种终局**：新一代若以 `PriorVersion` 指回被失效的那一代、而责任结果种类不同，事实引用按种类分流，两代不在同一引用下，替代关系照 CONTEXT 不成立。103 的译装器会把它当账坏拒收——ADR-0138 若让它成为正常路径，这一格必须先改，否则改判出的终局进不了投影。可选的方向：PS 让异种改判不带 `PriorVersion`（前代已由失效记录了结），或由 VE 按冲突处理。

## 三类

- 机制：上面两格的译法。
- 产品策略：无。
- 租户取值：无。

## 规模

M，随 ADR-0138 的实现票一起做或紧随其后。
