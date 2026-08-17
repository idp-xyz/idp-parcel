# ADR-0051: 资格审核答复含等待补充；一次性只约束终局格

Status: Accepted  
Date: 2026-08-17

## Context

`visibility-exception` 的 `EligibilityScreen` 是封闭二值（`ClaimEligible` / `ClaimIneligible`），`ClaimItem.ScreenEligibility` 对任何已写下的资格结果一律 `ErrClaimAlreadyScreened`。适配器注释把这条选择钉成硬的：因为「证不了」而答不通过，该索赔被永久拒掉且再无第二次机会。

CONTEXT 生命周期不是二值。资格审核的出口是三岔：「等待补充、不予受理或进入责任审核」。资料不足时索赔项进入限期补充，并固定缺少材料、补充范围、通知依据和当前截止时间；收到材料后重新判断最低材料要求；获批延期形成新补充期限版本、原期限保留；补充期限届满只触发资格复核，规则未定或延期待确认时保持待决定，不能默认拒赔。

二值一次性与这三岔直接冲突。把「资料还不足」写成 `ClaimIneligible` 就是默认拒赔且不可逆；把其余未核维写成 `ClaimEligible` 就是把未审完的索赔永久标成已过审。`parcel-shipment` 的 `IntakeEligibilityNotEstablished` 能续办，是因为它有第三格；同形搬到这里会变成不可逆拒赔。

合同不承担该索赔类型、超过首次索赔期限（`AT-VE-124`）仍是永久不予受理——那两格不随材料补充而变，换范围按 CONTEXT 是另一个索赔项，也不得借补充绕过首次期限。第三态是新态，不是把不通过改成可翻案。

本记录不改 CONTEXT.md。语言已经在那里；改的是领域类型去对齐它。

## Decision

**一、`EligibilityScreen` 增加 `ClaimAwaitingSupplement`（等待补充）。** 三态封闭。资格审核可以落到通过、不予受理或等待补充。

**二、`ErrClaimAlreadyScreened` 只挡住终局格。** 终局 = `ClaimEligible` 或 `ClaimIneligible`。处于等待补充时允许：材料到达后重新判断（仍待补、或落到终局）；获批延期写入新的补充期限版本。已撤回仍拒。

**三、等待补充必带四件落点。** 缺少材料、补充范围、通知依据、当前截止时间缺一不可。没有这四件的「等待补充」与「资格尚未审核」分不开。

**四、补充期限版本化。** 获批延期形成新版本，原期限保留在历史上，不覆盖。当前截止时间永远是最新一版。

**五、两格永久不予受理，不得经补充翻案：** 合同责任范围不承担该索赔类型；超过首次索赔期限（`AT-VE-124`）。届满处置分格：规则明确逾期未补 → 有依据的不予受理（`AT-VE-119`）；规则未定或延期待确认 → 保持待决定并升级，不默认拒赔（`AT-VE-120`）。保持待决定不是 `ClaimIneligible`。

**六、目录与编排的职责拆分不在本记录落地。** 本记录只定答复代数与索赔项不变量。目录继续可以只答「合同是否覆盖该类型」那一维永久格；材料与重复关系由后续编排切片核。

## Consequences

- `ClaimItem`、快照、重建门、`claim_item` 的 CHECK 与补充期限历史表必须同笔认识第三态和四件落点。
- 旧适配器注释里「二值一次性是硬选择」作废，按本记录重写：永久格仍一次性，等待补充可重入。
- 编排在目录能交回等待补充之前，不会走到第三态；领域先能表达，避免切片 (b) 把结果塞进二值。
- CONTEXT.md 一字不改。

## Alternatives considered

- **保持二值，资料不足用 `found=false`。** 否决：那一格已被端口定义为「资格目录未配置」。混用会让「规则说等材料」读成「租户还没登记规则」。
- **资料不足答 `ClaimIneligible`，靠业务约定事后翻案。** 否决：`ErrClaimAlreadyScreened` 挡住重审，翻案在类型上不存在，且即默认拒赔。
- **改 CONTEXT 把生命周期收成二值。** 否决：三岔、四件落点、届满不默认拒赔都是已确认规则；代码对齐文档，不是文档迁就代码。
- **让所有 `ClaimIneligible` 可重审。** 否决：合同不覆盖与超首次期限是永久格，翻案等于借补充绕过 `AT-VE-124`。

## Links

- [全程追踪与异常上下文](../domain/visibility-exception/CONTEXT.md)：客户索赔项生命周期与资料不足 / 届满硬句
- [UC-VE-007](../application/visibility-exception/UC-VE-007-MANAGE-EVIDENCE-CLAIMS-AND-RECOVERY.md)：`AT-VE-114` / `115` / `116` / `119` / `120` / `124`
- `.scratch/ve-claim-eligibility-dimensions`：缺维票与本记录所裁的第三条路
