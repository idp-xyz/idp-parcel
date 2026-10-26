# ADR-0311：首方直客的资料修订以产品常量等级 `CUSTOMER_SELF_SERVICE` 向 PC 问授权；法人与范围读解析键登记行；证据指向被修订的委托；租户的控制点是授权册有没有这一等级的授予

Status: Accepted（2026-10-26，Claude Code 会话 `tops-20` 代 owner 决，用户授权代决；票 first-party-api/10。裁决能力边界：读过 `internal/partycommercial/domain/authority_grant.go` 全文（`Authorize`、`resolveDecider`、`AuthorizationRequest` 构造器）、PS 的 `SourceDataAmendmentAuthorizationAdapter` 与 `SourceDataAmendmentRequestCoordinates`、`CommercialResolutionKeys` 与 `ResolutionKeyRow`、种子 `resolution-key-syn-account-01.json` 与发布批里的合同委派、[ADR-0116](./0116-source-data-amendment-is-an-authorized-action-and-contract-delegation-resolves-the-actual-decider.md)、[ADR-0310](./0310-first-party-customer-command-five-faces-launch-with-direct-customer-one-to-one-and-command-prefixed-request-keys.md) 取舍 B；**未读** `AuthorityGrantStore` 的 postgres 实现细节与 PC 授予登记入口（该入口今天不存在，归票 first-party-api/13））<!-- 索引注记 -->｜**取法**：等级 = 产品常量，法人与范围 = 解析键登记行，证据 = 被修订委托的来源身份加资料范围，原因 = 载荷结构化原因，请求方 = 客户账户本人｜**租户控制点**：授权册里 `SOURCE_DATA_AMENDMENT` × `CUSTOMER_SELF_SERVICE` 的授予｜**对 ADR-0116、ADR-0310 的作用**：落实 ADR-0116 的请求方为客户账户那一支与 ADR-0310 取舍 B 的授权，不改其条款
Date: 2026-10-26

## Context

**资料修订的授权是 PC 裁定的，而裁定要一份商业坐标，直客渠道没有人给。** `AuthorizationRequest` 要法人、权限等级、商业范围、结构化原因、证据与业务时点，授予按这几项逐字相等匹配（`AuthorityGrant.permits`）。PS 侧的适配器早已接真，坐标由一个 `RequestSource` 提供，生产里它留着 nil，修订因此停在`授权不可用`。ADR-0310 取舍 B 已定直客 1:1：请求方就是客户账户本人，不涉委派。剩下的是坐标每一项从哪里来。

**其中只有等级没有现成来源。** 法人与范围在解析键登记面（`ResolutionKeyRow`）上已经按（租户、客户账户）登记，受理链用的就是这一行。原因在修订命令里。证据与时点都能由请求本身形成。等级：授予里的等级是租户登记的商业权限，客户账户不「持有」等级；现成的两个等级字段含义都不对——解析键上的 `CreditLevel` 是信用政策的等级，合同委派上的等级是「委派给运营角色」的受托等级，直客不委派。

## Decision

**一、等级是产品常量 `CUSTOMER_SELF_SERVICE`，只在 `adapters/partycommercial` 的 `CustomerSelfServiceAuthorityLevel` 一处定义。** 它的含义是「客户本人经首方渠道自助修订自己委托的资料」这一种请求方身份，不是某个人的职级。租户是否放行，就是授权册里有没有一条 `SOURCE_DATA_AMENDMENT` × `CUSTOMER_SELF_SERVICE` 的授予：没有这条授予的租户，PC 答`授权规则未配置`（范围上有别的授予时答`不允许`），产品不替租户放行；租户要关停，退役这条授予即可。

**二、法人与范围读解析键登记行。** 经 `ResolutionKeyStore.FindResolutionKey(租户, 客户账户)`；没登记交回「折不出来」，授权停在`授权不可用`，不拿别处顶，也不冒充`授权规则未配置`——后者是 PC 对自己登记册的答案，前者是消费侧自己的缺口。授予登记时法人与范围要与这一行逐字相等。

**三、原因与请求方。** 原因取载荷里的结构化原因引用（`reason`）原样转写；请求方种类是客户账户，账户取自信封（册上绑定），不取载荷。时点取注入时钟，不取载荷。

**四、证据指向被修订的委托，不指向修订请求本身。** 形如 `first-party-amendment:<租户>/<账户>/<来源>/<原提交请求键>#<资料组>[/<包裹>]`。授权询问（`SourceDataAmendmentAuthorizationQuery`）带的是原提交的来源身份与资料范围，不带修订请求自己的来源身份，而加字段要改 PS 应用层，不在本票。修订请求自己的留痕仍在版本上（版本的请求指纹），证据加版本两样合起来可追溯。PC 侧**不按证据区分同一委托同一资料组上的两次修订**——两次询问给它的是同一份证据，区分两次修订靠 PS 的资料版本。

**五、授予的登记入口不属本记录。** 授权册今天没有生产写路径（`SaveGrant` 只有测试调用），登记入口与演示租户的参考授予归票 first-party-api/13；本票的用例经 `SaveGrant` 造授予。13 要登的取值：动作 `SOURCE_DATA_AMENDMENT`，等级 `CUSTOMER_SELF_SERVICE`，法人与范围对着演示账户的解析键行（`SYN-LE-01`、`SYN-SCOPE-01`）。

## 候选与反方

- **借解析键上的 `CreditLevel`。** 否决：两个政策各有各的等级含义，租户把信用等级改了，修订授权就静默跟着变，且没有解析键信用二维的租户（不要信用依据）读不出等级。
- **借合同委派的受托等级。** 否决：委派说的是客户把决定权交给运营角色，直客不委派；借来就把「客户有没有委派」与「客户本人能不能自助」绑成一件。
- **等级登记在解析键行上（新增一维）。** 备选，未采：它让每个租户、每个账户再登一格，而首发要的只是「租户放不放行」一个开关，授权册已是开关；日后若出现同租户内按账户区分自助等级的需求再开新记录。
- **读客户的合同委派反推等级。** 否决：同上，且委派缺席与自助是两件事。

## 对已接受记录的作用

落实 ADR-0116 决定三里「请求方是客户账户 → 决定方 = 请求方」那一支在首方直客渠道上的取法，与 ADR-0310 取舍 B 的授权。不改任何条款；两者正文不动，只在 Links 加前向指针。

## Consequences

- 首个租户上线实施清单要含两件登记：解析键行（受理链本来就要）与 `CUSTOMER_SELF_SERVICE` 授予；漏了后者的表现是修订答 200 `UNDECIDED`，原因 `SOURCE_DATA_AMENDMENT_AUTHORITY_RULES_NOT_CONFIGURED`，不是出错。登记之后客户**原样重放**同一修订请求即接着办（code/44）。
- 常量一旦有授予按它登出，就不能改名：改名等于把已登的授予全部变成「别的等级」。
- 允许矩阵（ADR-0120）仍是另一件租户取值，授权过了之后没登记的格停在`待复核`。

## 越权风险点

1. **等级常量是产品词。** 租户的授予按它逐字相等匹配，日后改名需迁移已登授予。今天生产里没有任何授予，现在定最便宜。
2. **一个等级放行整个租户的直客修订。** 授予按（法人、范围）匹配，没有按账户细分；同范围的所有直客账户共享这一条授予。若首个租户要按账户放行，须另立记录。
3. **证据不含修订请求自己的标识。** 授权审计面只看得到「谁在修订哪份委托的哪块资料」，看不到是哪一次请求；要精确到请求须改授权询问（PS 应用层）。
4. **时点取修订到达时的时钟。** 授权册在两次重放之间变了，同一请求会读出不同授权答复；这是 code/44 重放接着办的设计后果，契约已写明。
5. **没读的部分。** PC 授予登记的持久化与发布通道实现、`UC-PC-003` 全文；与它们有出入处以原文为准。

## Links

- [ADR-0116](./0116-source-data-amendment-is-an-authorized-action-and-contract-delegation-resolves-the-actual-decider.md)：资料修订是授权动作、请求方与决定方的分立
- [ADR-0310](./0310-first-party-customer-command-five-faces-launch-with-direct-customer-one-to-one-and-command-prefixed-request-keys.md)：取舍 B 直客 1:1
- [ADR-0120](./0120-source-data-amendment-allowance-is-a-third-stage-content-family-on-the-rule-package-version.md)：允许矩阵，授权之后的另一件租户取值
- [ADR-0261](./0261-authority-grants-and-contract-delegations-follow-the-closure-of-their-version-and-several-hits-are-an-applicability-conflict.md)：授予的收尾与适用冲突
- 票 [first-party-api/10](../../.scratch/first-party-api/issues/10-customer-source-data-amendment-face.md)；授予登记入口与演示种子：票 first-party-api/13
