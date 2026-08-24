# VE 索赔受理接线

Category: enhancement
Status: ready-for-agent

阻塞条件(非票号):工作树里 MCP-3 未提交批次先落提交,见父规格「排序约束」。

## 要做什么

把 `POST /claims` 背后的 `unwiredClaims` 换成真编排:新建 `cmd/parcel-api/assemble_claims.go`,构造 `visibilityapp.NewHandleClaimHandler`。

## 依赖缝逐条

`HandleClaimDeps` 的七条,这是五票里缝最多的一笔:

- `Claims`(ports.ClaimStore)— VE postgres 索赔库(claim_recovery 适配器)已存在,接真;
- `Recoveries`(ports.RecoveryStore)与 `Identities`(ports.RecoveryIdentityFactory)— 追偿库与标识签发,接既有实现;
- `Settlement`(ports.LiabilityHandoff)— 接 VE 的 Outbox handoff;
- `Eligibility`(ports.EligibilityRuleView)— 资格规则视图。VE 规则/政策登记册属 syn-wall-door-audit 票 09 的缺口范围:登记册就位前接显式未配置,编排如实答资格未配置留续办,**不得**默认「一律有资格」或「一律无资格」;
- `Evidence`(ports.ClaimEvidenceView)— 证据视图,同上逐一核对现状后裁决接真或显式未配置;
- `Clock` — 生产时钟。

## 验收

同票 01;另加:未配置缝的答案要在装配测试里钉住(资格未配置那格必须是携带续办的未决,不是 5xx 也不是业务否定)。
