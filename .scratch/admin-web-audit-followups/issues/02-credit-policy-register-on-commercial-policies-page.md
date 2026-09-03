# 02 商业政策页补「信用政策」册——后端已供七类，前端只认六类

Category: bug
Status: resolved（2026-09-03，MCP-2，`42846ed`）
Blocked by: 01（已 resolved，`d722301`）

## 缺什么

后端 `GET /commercial-policies?kind=` 的封闭集七词（`query_commercial_policies.go`）：
`ACCEPTANCE_RULE_PACKAGE / PRE_ACCEPTANCE_CONTROL / PRICE_POLICY / SETTLEMENT_POLICY /
AS_OF_POLICY / AUTHORIZATION_RULE / CREDIT_POLICY`。`0020_credit_policy.sql` 建了正文册，
`serveCreditPolicies` 转写它。

前端 `apps/admin-web/src/pages/party/api.ts` 的 `CommercialPolicyKind` 六词，无 `CREDIT_POLICY`；
`CommercialPoliciesPage.tsx` 的 `kindColumns` 头注与页面 description 两处写着「信用政策没有独立
正文册，封闭集里如实没有它」。这句在信用册落地之前是真话，现在是**页面替后端说的一句假话**——
操作者按它会以为信用政策在这个产品里不可见。

## 行形状（取自后端 `creditPolicyBody`，以它为准）

`objectId / version / legalEntity / authorityLevel / chargeType / effectiveStartsAt /
effectiveEndsAt? / registeredAt`，额度两键**恰一在场**：`limitMinor`（金额，最小货币单位）或
`limitRatioBasisPoints`（比例，基点）。**零额度是合法声明**（「授予零信用」），后端用指针而不用
`omitempty` 就是为了让 `limitMinor: 0` 在场——前端不得把 0 显示成「未声明」。

CONTEXT 原句（`docs/domain/party-commercial/CONTEXT.md`）：「信用政策和人工费用调整授权按
责任法人、业务角色、费用类型、金额或比例形成版本」——列名从这句取：责任法人、授权层级、费用
类型、额度。领域注释：「信用按费用类型而非按客户授予」。

## 做什么

1. 把 `CommercialPoliciesPage.tsx` 里的纯行转写（`kindColumns` / `rowsOf` / `declaredList` /
   `cancellationCell`）抽到 `pages/party/policy-rows.ts`，页面只留渲染。**这是让票 01 的底座
   够得着行为的缝**，不是顺手重构：`.tsx` 在 Node 里跑不起来。
2. 红：`policy-rows.test.ts` 用后端测试 `TestPoliciesEndpointTranscribesCreditLimitAsExactlyOneKey`
   里那两行（零金额 / 比例带结束）作输入，钉额度格的三态与列集。
3. 绿：`CommercialPolicyKind` 加 `CREDIT_POLICY`、`CreditPolicyRecord`、联合分支、`kindColumns` 一格、
   `policyKindLabels` / `commercialPolicyKinds` 加一格；删两处「无独立正文册」文案。

## 不做

- 价格政策口径（`caliber`）的前端呈现——那是票 `party-commercial-context-gaps/06` 的范围，
  MCP-5 在途；本票不碰 `PricePolicyRecord`。
- 信用政策的登记签——发布口的落点问题归 `admin-write-faces/03`。

## 完成判据

- `pnpm test` 含本票用例且 `fail 0`；`tsc --noEmit` 无输出。
- 页面七个 chip；切到「信用政策」发 `GET /commercial-policies?kind=CREDIT_POLICY`。
- 全树 `apps/admin-web/src` 不再出现「无独立正文册」。

## Comments

- 2026-09-03 · MCP-2：落于 `42846ed`（父 `1a5ce70`）。红：`policy-rows.test.ts` 在 tsc 层红
  （`CREDIT_POLICY` 不在联合）；绿：`api.ts` 加类型与分支、`presentation.ts` 加词与 chip、
  `policy-rows.ts` 加列与行转写。`pnpm test` 16 例 `fail 0`，`tsc --noEmit` 无输出，
  `apps/admin-web/src` 下「无独立正文册」零命中（VE 那句「六种册子」是 VE 自己的六类，不属本票）。
  抽出的行转写模块顺带钉住了取消授权三态——那三句是页面替目录说的话，抽出来之后不能变。
