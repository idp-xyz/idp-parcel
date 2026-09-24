# ADR-0100：管理台运营操作者身份是产品自有的接入渠道族，属机制半边——信任锚、校验方式与操作者—租户授权模型由产品定义，不等 `PAR-INT-01`；登记册配置写面的真 Intake 据此现在就立，ADR-0055 Decision 五两项未决对该写面不适用

Status: Accepted（2026-09-03，用户经 IDP 队列通道 4 先质疑「作为一个独立的软件产品……不可能都通过 cli」，再授权本会话「作为业务和系统专家自决，先按建议出 ADR」，据此裁决。裁决能力边界：读过 [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)、[ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)、[ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)、[ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 全文，`internal/accessidentity` 全部生产文件，[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)的 `PAR-INT-01` 与 `PAR-GOV-03..07` 行，`cmd/parcel-api` 的 `assembleBusinessEndpoints` 装配表，parcel-pricing 的登记端点、登记 CLI 与管理台价卡页；未重读 ADR-0003/0078 全文与各上下文 `CONTEXT.md`，ADR-0019/0020/0021 谱系只核了「跨租户运维后台不属于产品」那一句在现行记录里仍成立——本记录因此只裁**操作者渠道的归属与准入结构**，不裁任何登记册的领域不变量，也不裁客户业务面的准入）；**部分停用**：决定四末段与决定五第一条对「作业事实登记、外部结果接收」的适用已由 [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 停用（适用场景：作业事实登记面与外部结果、外部资金事实接收面），其余各条不变
Date: 2026-09-03

## Context

管理台今天仍是纯查阅面外加一排必答 `403` 的登记签。登记册配置的在线写面按 ADR-0085 已进端点表并接上真编排（价卡登记那一格连真库装配测试都是 PASS），但每一行都挂着字面量 `UnconfiguredIntake{}`；今天唯一能把一张价卡登进库的路径是工程师在数据库网络内跑 `parcel-pricing-register`。ADR-0085 的 Alternatives 已明文否决「写面长期只走 CLI」，理由是「把一项产品能力错登记为一项运维动作」——所以现状不是终态，是两阶段接线的第一阶段。

第二阶段（换真 Intake）被三处文本钉在同一个等待项上：

1. ADR-0072 Decision 二：「登记册表结构与凭据形态在 `PAR-INT-01` 最低证据……到位前不立」。
2. ADR-0085 Decision 二的同日补记：承认「管理台写面与客户业务面分属两族证据」，操作者族「OIDC 认证结果是候选形态」，但仍写「两族各自要过 `PAR-INT-01` 的真证据门」，并要求其落地「需自己的 ADR（信任锚、校验方式、操作者—租户授权模型）」。
3. ADR-0055 Decision 五：载荷规范化摘要与准入范围装配 `PAR-GOV-03..07`「仍然拦着真渠道 Intake」；ADR-0085 Decision 二把「本记录新增的登记端点也在被拦之列」。

本记录就是补记点名的那份 ADR。它要回答的不是「要不要在线面」——那已裁——而是**操作者这一族的证据到底该由谁出**。

**`PAR-INT-01` 的定义容不下操作者。** 参数登记册原文：参数名「客户生产委托接入渠道」，最低证据是「API、标准文件或门户的现行流程、委托与资料修订业务标识、重试和冲突边界」。这一列里的每一件都是**客户**渠道的属性：客户用什么渠道投委托、委托怎么标识、重发怎么判重。运营配置员登录管理台登一张价卡，不是客户提交委托；他的身份、他能动哪个租户的哪些登记册，与客户走 API 还是走文件毫无关系。把两族挂在同一个等待项上，后果是产品自己的后台要等客户的 API 契约——一个范畴错误，代价已经在首租户实施周里兑现：运营配置员的岗位描述写着「负责录入价卡、网络、关务与商业配置」，而他能做的只有让工程师跑 CLI。

**操作者族的三件都是产品决定。** 补记点名要裁的是信任锚、校验方式、操作者—租户授权模型。逐件看它取决于什么：

- 信任锚：产品自有的 OIDC 发行方（频道 2 已通报管理台拟接 `gk.idp.xyz`，Ory Hydra 形，authorization_code + PKCE）。谁签发操作者令牌是产品的部署形态参数，不是租户参数。
- 校验方式：OIDC 标准令牌校验——签名、发行方、受众、有效期。标准定的，不取决于任何租户。
- 操作者—租户授权模型：一个操作者账户绑定唯一租户（[ADR-0003](./0003-group-tenant-legal-entity-customer-account.md) 的第一级边界），授予按能力面显式登记、可撤销、带区间；跨租户账户不存在（[ADR-0021](./0021-frontline-operations-client-is-part-of-the-product.md) 承 ADR-0019/0020 的结论：开发方的跨租户运维后台不属于产品）。模型是产品的；**某个租户的操作者是谁、授予了什么**才是实例半边。

[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)把机制半边定义为「产品必须具备的能力——规则、判断、版本化、门禁与记录形态」。三件都落在这个定义里。ADR-0072 Decision 二「形状取决于渠道类型、那是一份实例证据」的论证对客户渠道成立（API、标准文件、门户三形各异，替它拟表就是替租户拟样子），对操作者渠道不成立：操作者渠道只有一形，且这一形是产品自己选的。

**同一登记用例、两口不同待遇，暴露了真正的缺口在哪。** ADR-0085 Decision 一原句：「CLI 与端点消费同一登记用例，是同一能力的受控批量口与在线口，答案代数一致」。受控 CLI 今天在没有任何 `PAR-GOV` 门、没有任何渠道证据的情况下写同一张登记册（`scripts/demo-seeds/seed.sh` 逐个调用），在线口却被 ADR-0055 Decision 五拦着。两口之间只差一件事：**谁在认证操作者**。所以拦着在线口的真实缺口是操作者认证，不是生产准入。

**ADR-0055 Decision 五两项对登记册配置写面各自不适用，理由不同：**

- 载荷规范化摘要——它是客户提交载荷的判重与审计摘要，进出边界由 PS CONTEXT 定，ADR-0072 Decision 三已把它与渠道解耦，ADR-0091 记 `CanonicalizeSubmissionPayload` 已有生产调用点。登记册配置写面的载荷是**登记快照**，自带规范化版本号与版本内容摘要（[ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)），登记册的幂等与冲突代数（`RECORDED` / `ALREADY_REGISTERED` / `CONTENT_CONFLICT` / `CANONICALIZATION_DIFFERS`）已由它定义。没有第二份摘要要补。
- 准入范围装配 `PAR-GOV-03..07`——登记册原文管的是「限量生产准入与生产权威规则」「各阶段候选版本、证据与 `Go/No-Go` 授权」「紧急暂停」「恢复准入证据」「发布版本回退、停止新准入与对象级接管」，对象是生产中的委托、包裹与权威区间。登记一个价卡版本不形成任何生产事实：它只让一个版本进入版本清单（CONTEXT「已批准 → 已发布」那一步），这个版本会不会被采用，另由商业价格政策的采用（[ADR-0034](./0034-pricing-closure-adopts-via-price-policy.md)）与委托的准入决定。配置登记的门禁是登记册自己的治理件——发布批准责任方、方向授权引用、版本不可覆盖——它们今天就在 `PriceCardRegistration` 的构造门上。

**`internal/accessidentity` 现有的形状是客户渠道的形状。** `SourceEnvelope` 四要素是租户、客户账户、来源、来源请求键；`NewChannelRegistration` 要求客户账户与 `RequestKeyDerivation` 非空。操作者不是客户账户，登记册写面的判重也不靠来源请求键。把操作者塞进这个形状就得伪造两个字段——而该包自己把 `SubmissionEnvelope` 与 `WithdrawalEnvelope` 做成两个类型的理由（「传错与传对在调用点长得一样」）在此一字不差地成立。

## Decision

**一、运营操作者身份是产品自有的接入渠道族，属机制半边；`PAR-INT-01` 的范围回到其登记册定义。** `PAR-INT-01` 只管客户生产委托接入渠道，不再是管理台写面的等待项。ADR-0085 Decision 二补记中「两族各自要过 `PAR-INT-01` 的真证据门，不得互相顶替」一句据此**部分停用**：前半停用，后半（不得互相顶替）保留并由本记录 Decision 三、五加强。**适用场景**：只限管理台操作者族；客户业务面那一族照旧过 `PAR-INT-01`。ADR-0072 Decision 二「登记册表结构与凭据形态在 `PAR-INT-01` 最低证据到位前不立」**部分停用**：适用场景收窄为**客户接入渠道**的登记行与凭据；操作者渠道的登记册结构与凭据形态由本记录 Decision 二定。ADR-0072 Decision 一（能力归 `internal/accessidentity`、业务上下文只消费已铸造信封）不变，本记录正是在它划定的落点上立第一份生产实现。

**二、操作者渠道的三件由产品定义。**

- **信任锚**是产品自有的 OIDC 发行方。发行方地址与 JWKS 端点是 `parcel-api` 的部署形态参数，未设即操作者渠道整族未配置、缺省朝拦。管理台 SPA 以 authorization_code + PKCE 取令牌；**`parcel-api` 自己校验令牌**（签名、`iss`、`aud`、有效期），不采信 SPA 的登录态、不采信任何自报头部——ADR-0085 补记的红线原句「不把登录态铸成后端采信的任何证据」由此从「SPA 别做」变成「后端不认」。
- **校验方式**是 `CredentialVerifier` 在操作者族上的生产实现：拿到的凭据本体（令牌）只在校验那一刻存在，不落库；库里只登操作者主体的绑定。凭据本体不进仓库与数据库是 AGENTS 「敏感实例外置」红线的落法。
- **操作者—租户授权模型**是 `internal/accessidentity` 的**操作者册**：操作者主体（发行方 + `sub`）绑定唯一租户；授予按能力面（登记册配置写、主数据与运营查阅读、治理登记——治理面的授予模型按 ADR-0085 Decision 四单独裁，本记录只预留这一格）显式登记、可撤销、带生效区间；不存在跨租户主体。册的**结构**由本记录定死——这些列由产品的授权模型决定，不取决于任何租户；册的**行**（某租户的操作者是谁、授予了什么）属实例半边，由参数登记册增一行登记其证据状态，`PAR-INT-01` 行不改。

**三、操作者信封是自己的类型。** `internal/accessidentity` 增 `OperatorEnvelope`（租户、操作者主体、授予集、来源固定为管理台），不复用 `SourceEnvelope`，不给客户账户与来源请求键填占位。两个类型让「拿操作者信封去铸客户委托」在编译期走不通——形状与理由同该包 `SubmissionEnvelope` / `WithdrawalEnvelope` 的分型。与 `SourceEnvelope` 同款保持：字段不导出、包外无构造函数，拿到一个 `OperatorEnvelope` 就等于它经过了铸造。

**四、登记册配置写面与主数据查阅面的真 Intake 就是操作者渠道 Intake，在装配点逐端点换。** 覆盖面是 ADR-0085 Decision 一那一族登记端点与 [ADR-0077](./0077-master-data-catalogue-read-follows-the-operations-read-pattern.md) 那一族目录查阅端点。答复代数按恢复动作分格（[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)），三格不得合并：

- 发行方未配置 → `403` `ACCESS_CHANNEL_NOT_CONFIGURED`（恢复动作：配部署参数的人，ADR-0055 原格不变）；
- 令牌缺失、过期或校验不过 → `401`，新格（恢复动作：持令牌的人去登录或换令牌）。ADR-0055 Decision 三否决 `401` 的前件是「此刻不存在任何凭证方案」，在操作者族上这个前件自本记录起不成立，否决随前件解除，仅限此族；
- 令牌有效但操作者不在册、不绑这个租户或无此能力面的授予 → `403`，新格（恢复动作：管授予的人去登记授予）。它与探针同答约束的关系照 ADR-0055 Decision 四：不披露「哪一半不对」。

**ADR-0055 Decision 五两项未决对登记册配置写面不适用**，理由见 Context，不复述；ADR-0085 Decision 二「本记录新增的登记端点也在被拦之列」一句据此**部分停用**，**适用场景**：登记册配置写面。客户业务命令面（委托提交与撤回、作业事实登记、外部结果接收）照旧被两项未决拦着，本记录一个字不改它们。

**五、不做的，逐条写明以免被读宽。**

- 不给客户业务命令面开操作者渠道。客户委托来自客户渠道；「操作者代客提交」是另一个用例，需要它自己的授权与来源标记，不在本记录。
- 不做绕过 OIDC 的「开发用」本地账号；隔离形态（ADR-0091）继续靠 `SYN-` 写开关服务演示，它与操作者渠道是装配点上的两行，互不替代、不合并；ADR-0091 Decision 六「隔离形态不因真渠道出现而自动退场」照旧。
- 不做跨租户运维后台账户；不替租户拟其内部 IdP——租户要用自己的身份源时，在产品发行方做联邦，`parcel-api` 的信任锚与校验方式不变。
- 不裁 `PAR-GOV-03..07` 对客户业务面还拦不拦：那是另一个问题，超出本记录能力边界。

**六、受控 CLI 保留，定位回到 ADR-0085 Decision 一原句「受控批量口」。** 它今天是唯一能用的口是事故不是设计；seed 路径不变。

## Consequences

- `internal/accessidentity` 得到第一份生产实现：操作者册（含 `migrations/access_identity/` 首个模块）、OIDC 校验器、`OperatorEnvelope` 与其铸造。`doc.go` 中「本轮既没有登记册的表，也没有凭据形态」一段随实施改写为「客户渠道那一半仍等 `PAR-INT-01`；操作者那一半已按 ADR-0100 立」。
- `cmd/parcel-api`：部署形态多发行方与 JWKS 参数，未设即操作者渠道整族 `403` 未配置；登记册配置写面与目录查阅面在装配点逐端点换操作者 Intake，每换一口该口的「未配置即拒」测试改写为三格测试，未换的口答复不变（逐口纪律沿 ADR-0091 Consequences）。隔离读放行表与 `SYN-` 写开关零改动。
- 管理台外壳加登录门（authorization_code + PKCE），请求带 Bearer；各页「接入渠道未配置」的单一文案分成三态呈现，登记签在授予齐备的租户上第一次能真的登进去。
- 参数登记册增一行「运营操作者账户与授予」（实例半边：某租户的操作者主体、绑定与授予），`PAR-INT-01` 行原文不动。
- ADR-0072 与 ADR-0085 的 `Status` 行与 Links 节按 [adr/README](./README.md) 的部分停用机制加前向指针，被停条款与适用场景如 Decision 一、四所述；两记录其余各条不变。
- 首租户实施节奏里「accessidentity S1–S3 按 C0 答复推进」不再以客户答复为前件——那是客户渠道那一半的前件，操作者那一半现在就能推进。
- 客户业务面的等待项一条不变：`PAR-INT-01`、`PAR-GOV-03..07`、ADR-0055 Decision 五对它们照旧有效。

## Alternatives considered

- **维持「两族各自过 `PAR-INT-01` 门」，等客户渠道证据齐了一起换。** 否决：把产品自己的身份体系挂在客户的 API 渠道证据上是范畴错误；代价是首租户实施周里运营配置员仍只能让工程师跑 CLI，与 ADR-0085 否决「写面长期只走 CLI」的理由正面冲突——那条否决说的正是「把产品能力错登记为运维动作」，而等待本身就是在这么登记。
- **复用 `SourceEnvelope` 四要素，客户账户填占位、推导口给常量。** 否决：伪造两个字段让「信封一定经过铸造、四要素无一自报」的结构保证名存实亡，且与该包自己分型 `SubmissionEnvelope` / `WithdrawalEnvelope` 的理由相悖。
- **管理台 SPA 自己判登录态，后端信一个头部。** 否决：ADR-0055 Alternatives 原句「事后没有任何东西能把这些信封与真实认证结果区分开」在此原样成立；ADR-0085 补记的红线就是为它写的。
- **在 ADR-0091 的隔离写开关上加一个「真租户」取值。** 否决：`SYN-` 前缀门禁把真实租户结构上挡在开关之外是 ADR-0091 Decision 六的设计，不是待放宽的限制；一条生产装配里不存在的路径不该长成生产准入。
- **操作者渠道也等一份「运营方现行 SSO 流程」证据，与客户渠道同等对待。** 否决：产品是发行方，租户 SSO 是发行方侧的联邦配置——那是实例半边，但登记在发行方那边，不改变 `parcel-api` 的信任锚、校验方式与授权模型任何一件。
- **把 ADR-0055 Decision 五两项也裁成不拦客户业务面。** 不裁：本记录能力边界只到登记册配置写面与目录查阅面；客户业务面的载荷摘要与准入范围装配是另一个问题，留给读过 PS CONTEXT 与 `UC-PS-001` 的裁决。

## Links

- [ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)：在线写面属产品能力、两阶段接线的裁决；其 Decision 二补记点名本记录，本记录停用其中「两族各自要过 `PAR-INT-01` 的真证据门」与「本记录新增的登记端点也在被拦之列」两句（适用场景各见 Decision 一、四）
- [ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)：能力归 `internal/accessidentity` 的出处；本记录把其 Decision 二的适用场景收窄为客户接入渠道
- [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)：未配置格、替换缝与 Decision 五两项未决；本记录裁两项对登记册配置写面不适用，对客户业务面不动；Decision 三否决 `401` 的前件在操作者族上解除
- [ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md)：隔离形态写路径与逐口放行纪律；本记录不碰其任何一条，操作者渠道与 `SYN-` 开关是装配点上的两行
- [ADR-0077](./0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)：目录查阅面族的出处，本记录 Decision 四的覆盖面之一
- [ADR-0003](./0003-group-tenant-legal-entity-customer-account.md)：操作者绑定唯一租户所依的第一级边界
- [ADR-0021](./0021-frontline-operations-client-is-part-of-the-product.md)：该谱系现行记录，承 ADR-0019/0020「开发方的跨租户运维后台不属于产品」——本记录不设跨租户主体的依据
- [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：登记快照自带的规范化版本号与内容摘要——载荷规范化摘要那项未决对登记册写面不适用的依据
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：三格按恢复动作分格的判据
- [ADR-0034](./0034-pricing-closure-adopts-via-price-policy.md)：版本进清单不等于被采用——`PAR-GOV-03..07` 不拦配置登记的依据之一
- [参数登记册](../product/PILOT-PARAMETER-REGISTER.md)：`PAR-INT-01` 与 `PAR-GOV-03..07` 的原文出处；本记录不改其状态，只收窄前者的适用面并预告新增一行
- [开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)：机制半边定义的唯一权威
- `internal/accessidentity`：`SourceEnvelope`、`ChannelRegistration`、`CredentialVerifier`、`Minter` 的现状形状，本记录 Decision 二、三在其上增操作者那一半
- [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)：部分停用本记录决定四末段与决定五第一条中「作业事实登记、外部结果接收」那一半——一线作业事实归操作者渠道族的「作业事实登记」能力面，外部结果与资金事实归集成客户端族
- 来源：IDP 队列通道 4 的质疑与授权（2026-09-03）；`.scratch/tenant-implementation-01/implementation-week-decisions-and-customer-pack.md` 客户函中运营配置员的岗位描述
