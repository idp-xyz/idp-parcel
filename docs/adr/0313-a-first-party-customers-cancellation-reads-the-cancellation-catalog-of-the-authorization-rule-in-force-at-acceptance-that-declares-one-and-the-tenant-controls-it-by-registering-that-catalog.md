# ADR-0313：首方直客的取消授权读「接受那一刻在效、且登记了取消授权目录」的那一版授权规则的目录，而不走接受时闭包；请求方格由客户账户本人折出；租户的控制点是登不登这份目录

Status: Accepted（2026-10-26，Claude Code 会话 `tops-20` 代 owner 决，用户授权代决；票 first-party-api/11。裁决能力边界：读过 PS 的 `ServiceStageRulesAdapter`、`DeclaredStageContent`、`ResolvedAdoptedStageOwner`、`CancelParcelHandler`，PC 的 `CancellationAuthorityContent`、`CommercialRegistry.applicable`、`StageContentDeclarations.LoadCancellationAuthority`，演示发布批里全部四版授权规则与两个解析键；越权风险点见文末，待 PS owner 与 PC owner 复核。）<!-- 索引注记 -->｜**入口**：首方取消口的授权缝（`CancellationAuthorityView`）此前停在 `AUTHORITY_UNCONFIGURED`，因为没有「采用的授权规则版本」的来源；本记录定这个来源怎么取。**取法**：账户的商业范围取自解析键登记行，时点取委托接受决定的时刻，在该范围、该时点已生效的授权规则里，取登记了取消授权目录的那一版；零版是未配置，恰一版读它的目录，多于一版是登记册冲突、不择一。**请求方格**：请求方引用正是委托所属客户账户本人才折成「客户」格。**租户的控制点**：登不登取消授权目录、目录里有没有客户这一行。
Date: 2026-10-26

## Context

- 取消授权目录归授权规则版本（ADR-0058），「采用哪一版」本应由接受时固定的闭包回指（`ResolvedAdoptedStageOwner`，ADR-0062），只要解析键把授权规则列进必需依据。
- **闭包这条路在授权规则上走不通。** 闭包按（租户、范围、类别、锚点）选，一类对象恰有一版才算唯一（`CommercialRegistry.applicable`，多于一版答适用冲突）。授权规则在同一范围里天然不止一版：价格 / 成本方向授权（定价引用它们，且命名引用要求同范围——把它们挪出范围，发布批里的 `SYN-AUTH-PRICE-DIR-01` 会因「命名引用未发布」停在待定）、资料修订的授予载体（ADR-0312 把授予单独占一版）、取消授权目录各是一版。演示租户 `SYN-TENANT-01` 的 `SYN-SCOPE-01` 里接受时点就有四版授权规则，只有 `SYN-AUTH-CANCEL-01` 登了取消目录；把授权规则列进必需依据，每次接受都会答适用冲突。真实租户上线后定价方向授权与取消目录同处一个范围是常态，不是演示的特例。
- 取消授权不是 PC 裁定（没有授权册的授予、没有法人与等级坐标）：目录只按请求方格取（客户 / 运营），命中带规则引用。所以它不适用资料修订的坐标映射（ADR-0311）——那里要的法人、等级、范围三格，取消用不上。

## 候选与反方

- **把授权规则列进解析键必需依据，走闭包。** 否决：见 Context，同范围多版授权规则使接受答适用冲突；要绕开只能为每种授权规则各开一个范围，而定价的命名引用要求同范围，绕不开。
- **改 PC 的闭包选法，让授权规则按用途分开选。** 方向对，但动 PC 的解析域与全部既有闭包，不是取消口这一票的体量。留作重启条件。
- **运行时取「当下」在效的那一版。** 否决：取消授权跟着接受那一刻生效的规则走，与终局规则、收寄资格同一条纪律；按当下读，租户事后换版会改变已接受委托的授权。
- **按接受时点、在范围内按目录的在场收窄（采纳）。** 目录是授权规则版本正文里的一族声明，「登记了目录」是租户登记的事实，不需要产品再发明一个选择键。

## Decision

**一、取消授权目录取自接受那一刻在效的、登记了目录的授权规则。**（机制）

- 商业范围读解析键登记行（与资料修订同一行）；时点取委托接受决定的时刻；在该租户、范围、时点已生效的授权规则里（PC `CommercialRegistry.EffectiveVersionsAt`，选用区间折区间更正，与解析选用同一口径），逐版读取消授权目录，留下登记了目录的。
- 零版 → `found=false`，授权停在 `目录未配置`（`AUTHORITY_UNCONFIGURED`）；恰一版 → 读它；多于一版 → `ErrCancellationAuthorityConflict`，答未决 `AUTHORITY_UNAVAILABLE`，不择一，也不折成未配置。
- 账户没登解析键、委托不在、委托尚未接受 → 同样 `found=false`。

**二、请求方格由「请求方就是客户账户本人」折出。**（机制）

- 首方取消的取消者引用是 `CUSTOMER_ACCOUNT:<客户账户>`（`domain.NewCustomerAccountCancellationRequester`），映射 `CustomerSelfCancellationParty` 只在引用正是委托所属账户本人时折成「客户」格；别的账户、运营角色或任意写法都折不出来（`formed=false`，授权停在 `授权不可用`），不猜它是不是运营。运营代办若将来要接，另起一份映射。

**三、租户的控制点是目录。**（租户取值）

- 产品不代拟谁能在哪个阶段取消：租户登一版带取消授权目录的授权规则，目录里有客户这一行，才放行客户取消；目录里没有客户这一行是一份真的拒绝；什么都没登答未配置。「收寄前」这条边界不在目录里，是提交边界上的收寄 / 终局读口判的。
- 演示租户 `SYN-TENANT-01` 的参考配置「客户系统可在收寄前取消」就是发布批已有的 `SYN-AUTH-CANCEL-01`（目录含 `CUSTOMER` 与 `OPERATIONS` 两行），本票不改种子；解析键不需要改（范围取自既有行）。

**四、取消请求时刻由客户系统在载荷里声明。**（机制）

- 取消决定库的幂等键是（租户、原接入请求标识、包裹），内容摘要含请求时刻；取服务端时钟则每次重放都是新内容，被判成冲突。所以载荷带 `requestedAt`（RFC 3339，必填，比服务端时钟领先超过五分钟答 400），契约写明重放必须逐字节同载荷。取消请求自己的接入请求标识只用来铸信封（前缀 `parcel-cancellation:`），不参与幂等：换一个取消请求标识、同内容重放读回同一份取消。

## Consequences

- 首个租户上线时登记一版带取消授权目录的授权规则即可开放客户取消；不登，取消口对该租户如实答未配置。
- 取消不依赖接受时闭包，不改解析键、不改既有闭包；`ResolvedAdoptedStageOwner` 与 `DeclaredStageContent` 的取消分支不再有生产调用方（接单规则包那两支仍在用）。
- PC 新增一个只读的领域方法 `EffectiveVersionsAt`，无迁移。

## 越权风险点

1. **绕开接受时闭包选授权规则**（PS owner、PC owner）。本记录以「接受时点 + 目录在场」另立了一条选法；若 PC 日后改闭包使授权规则可按用途分开采用，本选法应并回闭包（重启条件）。
2. **一个范围同一时点至多一版登记了取消目录的授权规则**（PC owner）。租户换目录要先收尾旧版（截止或退役），否则两版并存，取消答未决 `AUTHORITY_UNAVAILABLE`。未决不落库，收尾后重放即接着办。
3. **PCN-1 不入库**（PS owner）。`CancelParcelCommand` 没有摘要的位置，取消的判重由取消决定库以（包裹、取消者、原因、请求时刻）的内容摘要承担；PCN-1 与它同一批输入，Intake 里只当载荷是否立得住的一道门。要让 PCN-1 真入库，须给命令加摘要字段并让取消库改用它（PS 应用层，不在本票）。
4. **请求时刻是客户声明的**（PS owner）。它记在取消决定上，不参与边界判断（边界判断读收寄与终局事实，不比时间），但审计看到的「请求时刻」是客户自称；服务端的决定时刻另在 `decidedAt`。
5. **`REQUEST_AWAITING_DECISION` 与 `REQUEST_ENDED` 答 200**（PS owner）。与首方族其它口的业务答复一致，不译 409；要译 409 在适配器一侧改。

## Links

- [ADR-0058](./0058-stage-content-owned-by-rule-objects.md)：阶段内容归规则对象，取消授权目录归授权规则版本
- [ADR-0062](./0062-adopted-stage-owner-from-accepted-resolution.md)：采用版本回指接受时闭包
- [ADR-0310](./0310-first-party-customer-command-five-faces-launch-with-direct-customer-one-to-one-and-command-prefixed-request-keys.md)：首方客户命令五口，取舍 C
- [ADR-0311](./0311-a-first-party-customers-source-data-amendment-asks-party-commercial-at-the-product-level-customer-self-service-and-the-tenant-controls-it-by-granting-that-level.md)：资料修订的授权坐标
- [ADR-0312](./0312-the-authority-grant-is-registered-through-the-publication-of-its-authorization-rule-version-as-a-declaration-channel.md)：授予载体单独占一版授权规则
- 票 `.scratch/first-party-api/issues/11-customer-parcel-cancellation-face.md`
