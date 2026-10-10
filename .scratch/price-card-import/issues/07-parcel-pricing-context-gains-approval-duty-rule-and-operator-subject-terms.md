# 07 parcel-pricing CONTEXT 收「价卡发布审批职责规则」与价卡侧操作者主体词条

Category: enhancement
Status: needs-triage——2026-10-11 通道 1 立（用户授权自决），出自 [04](04-approval-and-publication.md)「评审 ← 通道 2」Spec 非阻断 P3
父票：[spec](../spec.md)
地盘：parcel-pricing 的 `CONTEXT.md`（按 [CONTEXT-MAP](../../../docs/domain/CONTEXT-MAP.md) 找）；纯文档。
出处：04「没做的、留给后续」里 CONTEXT 那条；AGENTS.md「改文档」（改领域语言先改对应 `CONTEXT.md`，再改引用它的 `UC-*`）。

## 现象（钉 `b07939a7`）

- 04 已在 parcel-pricing 立了「价卡发布审批职责规则」（`PriceCardApprovalDutyRule`，表 `parcel_pricing.price_card_approval_duty_rule`）与价卡侧的操作者主体（`OperatorSubject`、`OperatorGrant`），parcel-pricing `CONTEXT.md` 没有对应词条；概念权威眼下只在 ADR-0101 决定六与 spec 自决第 2 格。
- PC 那一条（商业版本的发布审批）在 PC CONTEXT 有词条；两边「形同、不共用」（spec 自决第 2 格），授予的词汇也不同（04 判断项「授予集的词汇与来源」）。

## 待分诊

- 词条收到哪一层：只收「审批职责规则」与批准者 / 录入者，还是连同操作者主体与授予一并收。
- 与 PC 词条怎么互引而不复述（AGENTS.md 红线「单一权威」）。
- 04 判断项里标「越权风险点 · 待 PP owner 复核」的几格（授予集的词汇与来源、状态不对分三格、发布的答案代数）要不要等 owner 复核后再落词条。
