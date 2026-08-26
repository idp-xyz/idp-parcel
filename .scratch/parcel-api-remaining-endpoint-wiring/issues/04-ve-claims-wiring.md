# VE 索赔受理接线

Category: enhancement
Status: resolved

阻塞条件(非票号):工作树里 MCP-3 未提交批次先落提交,见父规格「排序约束」。
(2026-08-24 认领时核:该批次已随 `463646b` 落库,阻塞已消。)

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

## Comments

2026-08-24 随 `20cdc5d` 落库。归属:MCP-4 认领并标 in-progress 后停机(足迹止于票面
标记,mtime 17:01,cmd/parcel-api 零未提交改动),MCP-9 依用户在通道 9 的指令接手完成;
票面认领标记系 MCP-4 未提交编辑,随本笔簿记一并带入。

七条缝逐条裁决:Claims、Recoveries、Identities(veidentity.NewRecoveryMatters)、
Settlement(OutboxLiabilityHandoff 经真 Outbox)与 Clock 五条接真。Eligibility 显式
未配置,但成因与立票时的预设不同——登记面本身已存在(claim_contract_scope/授权目录,
经 parcel-ve-register 可登),真适配器 ClaimEligibilityRules 也在;接不上是因为它把
租户钉在装配期(为受控登记口而设),而 parcel-api 是多租户入口、今天没有租户可钉:
钉空租户会以「声明不在场」的业务答案顶「没接」,且租户真登记后答案也不变——接错看
着像接对。缺口是这条缝带租户维的读法,属机制半边,不是等登记。按端口合同「依赖调
不通作为错误返回」如实报错,审核入口停在指名到缝的未决(装配测试对真库钉住:不是
error、不是业务否定)。Evidence 显式未配置——归集面机制未建,按端口自设 known=false
格答「归集无从查起」,不造空清单替客户立补充义务。受理入口不碰两条未配置缝(受理只
保全提交事实),/claims 客户提交面只见 ClaimReceiver。

装配测试 assemble_claims_test.go 对真库实跑 PASS:受理真实落库、重放走已有项证首笔
事务提交、资格未决指名到缝、证据缝形状直钉。全仓验证按 20cdc5d 在临时 worktree 检出:
gofmt 清、build/vet 退 0、go test -count=1 ./... 全 ok(DSN 已设,PG 包实跑非跳过)。

2026-08-26 MCP-1 追记:上段点名的资格缝缺口已随票 `ve-claims-read-seams/01` 收口
(`bfabc0d`)——查询自带租户的多租户读适配器接真,报错桩与 UNAVAILABLE 验收钉退役,
装配测试改钉「未登记停待登记 / 别的租户登册不改答 / 本租户登册后按册答」三态。原句
一律保留。证据缝仍显式未配置,承载票 `ve-claims-read-seams/02`。
