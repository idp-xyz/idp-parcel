# 14 TF 其余构造门：判零仍按纳秒、三处自由文本不拒 NUL

Category: bug
Status: needs-triage——2026-10-19 会话 `idp-parcel-9b` 立（tf-face-algebra/01 实施中清点出的地盘外同病，未逐口实测）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：tf-face-algebra/01 完成记录「同形门清点」。
地盘：
- internal/transportfulfillment/domain（各口构造门与重建门）

## 现象（读码清点，未逐口实测）

- **判零同病**：tf-face-algebra/01 只改了装载分配与移动事实。场外揽收的 `FormOffsitePickup` 与其 `Correct`，以及履约段、承运商判断、揽派任务、履约尝试与交付、外部凭证、外部跟踪、主单、委托与订舱、交接、费用发生、班次运力、替代路程、受监管处置各口的时刻非零门仍按纳秒判。判据依 `AtRegistryPrecision` 头注「本上下文登记册的时刻一律落在 timestamptz 列上」的前提；落 jsonb 快照的口不在此列，要逐口核。
- **NUL 同病**：三处自由文本不经 `newRequiredValue`，只判空不判 NUL：`CarrierEvidenceSpec.Material`、`CarrierFirstEffectivePickupSpec.Material`、`RegulatoryTransportDispositionSpec.DeclineBasis`。
- **顺带**：`TransportMovementFact.Correct` 不挡「更正时刻早于发生时刻」，`RehydrateMovementFact` 却拒；目前没有生产调用方。

## 应当

同 tf-face-algebra/01 的裁定：判零按登记册精度（扩展 `AtRegistryPrecision` 头注那条规则，不另立定义），库收不下的取值在构造门答未受理。

## 完成判据

- [ ] 逐口核：时刻是否落 `timestamptz`、重建是否复用构造门；落的就改判零，不落的写明。
- [ ] 三处自由文本拒 NUL。
- [ ] `TransportMovementFact.Correct` 与重建门对「更正时刻早于发生时刻」口径一致。
- [ ] 每类至少一条真库回归；Go 门禁照常。
