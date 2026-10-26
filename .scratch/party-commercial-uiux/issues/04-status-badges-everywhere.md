# 04 — 状态一律用徽章

**Parent:** [spec](../spec.md)

**What to build:** 参与方与商业区里同是「状态」，法人、参与方、关系三处是彩色徽章（`statusBadge`），合同、协议、产品、授权规则、策略册与客户账户是纯文字（`labelOf` 或直出），全景里供应商协议的状态也是纯文字。这些词（已发布、已生效、已退役，以及身份那套）在 `src/domain/status.tsx` 的 `domainStatusTones` 里都有色调。列头也有「状态」「身份状态」「生命周期状态」三种叫法；「生命周期状态」在 1440 宽下逐字竖排。

改法：本区各表的状态格一律走 `statusBadge`，列头统一叫「状态」。词表里没有的状态词照现有做法退成中性徽章，不自造色调。

**Blocked by:** None — can start immediately

**Category:** enhancement

**Status:** ready-for-agent

- [ ] 合同、协议、产品、渠道映射、各政策册、客户账户、参与方全景里的状态格都是徽章。
- [ ] 本区列头只用「状态」一种叫法。
- [ ] 不新增 `domainStatusTones` 之外的色调。
- [ ] 三道门退 0。
