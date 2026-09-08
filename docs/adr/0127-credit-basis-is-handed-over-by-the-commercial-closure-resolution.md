# ADR-0127：信用依据随商业闭包解析交出——闭包解析加 `resolveCreditPolicyBasis` 一步（镜像 `resolveSettlementPolicyBasis`），`ResolveCreditPolicy` 作（法人 × 权限等级 × 费用类型 × 时点）四维选择器，`CreditBasis` 随 `Resolution` / 闭包交出并入快照；键上新增 `CreditSelector`（等级 × 费用类型）；`settlement-accounting` 账期分支的授信额度从闭包交出的信用依据取，不从登记状况事实取

Status: Accepted（2026-09-08，MCP-1 代裁**甲**——owner 授权自决口径，裁决原文在票 [wiring-baseline-remainder/03](../../.scratch/wiring-baseline-remainder/issues/03-pc-credit-basis-is-never-asked-for-the-pc-to-sa-seam-does-not-exist.md) Comments「11:0x · MCP-1 代裁」一条；本记录由 MCP-3 落文并实施。落文时读过：该票全文（含 MCP-6 02:3x 取证与两条路）、ADR-0044 / ADR-0080 / ADR-0115 / ADR-0025 / ADR-0028、`party-commercial` `CONTEXT.md` 信用政策那句、`settlement-accounting` `CONTEXT.md` 授信额度与接受前控制那几句、`internal/partycommercial/domain` 的 `commercial_resolution.go` / `reference_closure.go` / `credit_policy.go` / `credit_limit.go` / `commercial_registry.go` / `resolution_rehydration.go`、`adapters/postgres` 的 `commercial_publication.go` / `commercial_resolution.go` / `credit_policy.go`、`migrations/party_commercial/0020_credit_policy.sql`、`internal/settlementaccounting` 的 `application/apply_pre_acceptance_control.go` / `domain/credit_exposure.go` / `adapters/postgres/operational_position.go` / `adapters/partycommercial/pre_acceptance_control_policy.go`、`internal/parcelshipment/adapters/partycommercial/commercial_resolution_keys.go`（只读，PS 地盘）。未重读 UC-SA-002 全文与 PS 接受编排；本记录因此只裁**信用政策在哪一层选、选出的依据怎么交出、SA 账期分支从哪里取额度**三件，不改 PS 任何一格、不动 `0020`）
Date: 2026-09-08

## Context

`party-commercial` `CONTEXT.md` 写「信用政策……按责任法人、业务角色、费用类型、金额或比例形成版本。政策只提供业务判断依据，不直接修改结算余额或形成调整金额」；`settlement-accounting` `CONTEXT.md` 要求运营结算余额区分「当前有效授信额度」，每项信用暴露保存实际采用的政策与范围，且「余额不足或逾期只向订单接受等责任上下文提供信用暴露和业务限制依据」。

代码在 `2c0008c3` 上是这样的（本记录落文时复核）：

- 领域层早有 `CreditPolicy` / `CreditPolicyQuery` / `ResolveCreditPolicy` / `CreditBasis`：对一组信用政策正文按 `covers`（法人、等级、费用类型、时点）选唯一 / `适用冲突` / `无适用依据`，`CreditBasis` 的注释把消费方点了名——「是本上下文交给 settlement-accounting 的东西」。**`ResolveCreditPolicy` 在全仓非测试代码里零调用**：唯一提到它的地方是 `ports.CreditPolicyContentView` 的注释与 `internal/architecture/production_wiring_baseline.txt` 的条目行。
- 正文表 `0020` 已落地（票 party-commercial-context-gaps/03），主键是版本四元组，**一版一行**；写口 `SaveCreditPolicy` 随发布同笔，点读口 `LoadCreditPolicy` 在 PC 之外零非测试调用。`0020` 头注与 `ports.CreditPolicyContentView` 注释都写着「本表不进整册装载：今天没有任何解析在信用政策之间选」。
- 闭包解析把 `CreditPolicyObject` 当版本壳按范围选（`registry.applicable`），交出的只是一份 `CommercialVersion`——与 ADR-0044 落地前结算政策、ADR-0034 落地前价格规则的形状相同。
- SA 的 `exposeCredit` 读 `CreditStandingView.LoadCreditStanding`，额度来自 `settlement_accounting.credit_standing.limit_minor`——一条**登记进来的状况事实**，`RecordStanding` 在生产路径上零调用；`BalancePosting` 的注释却写着「授信额度来自商业侧信用政策」。于是账期分支形成信用暴露时，没有一份出自政策版本的额度依据可比，`AT-SA-171` 的「B 只形成适用信用暴露 / 限制结果……分别保存政策和范围」在 SA 侧无从落。

票 03 取证出「要先裁的一格」：既然一版一行，`ResolveCreditPolicy` 的四维选择在哪一层还有多候选？答案是**同一范围内多个信用政策对象各自的生效版本**——与结算政策完全同形。两条路：

- **甲**：照结算政策的形，在闭包解析里加一步、`ResolveCreditPolicy` 作四维选择器、`CreditBasis` 随闭包交出。
- **乙**：闭包只认唯一版本壳、`ResolveCreditPolicy` 退化成对已选版本正文的 `covers` 校验（对不上答`无适用依据`）。

两条都是解析语义的改口（`ports.go` 自注「让它进闭包是解析语义的改动，不是登记正文的连带」），都要 ADR。派单纪律写明实施票里不顺手定，MCP-6 停下报 MCP-1，MCP-1 裁甲。

## Decision

**一、信用政策在闭包解析里选，镜像结算政策；`ResolveCreditPolicy` 是选择器不是校验器。** `ResolveCommercialBasis` 对 `RequiredBasis == CreditPolicyObject` 走新增的 `resolveCreditPolicyBasis`：候选先按政策版本的租户与范围收窄（与 `resolveSettlementPolicyBasis` 同一收窄），再由 `ResolveCreditPolicy` 按（法人、等级、费用类型、时点）选唯一 / `适用冲突` / `无适用依据`。光有已登记的信用政策**版本**没有正文 = 该范围没有可用信用依据，与 ADR-0044「光有版本没有政策」同判。理由（MCP-1 裁决原文）：价格规则与结算政策两个政策类对象已在同一闭包里用类别专属选择器解析（ADR-0034 / ADR-0044），信用政策是第三个；乙会让同一闭包里两套解析语义并存、且逼租户把（法人 × 等级）拆成不同范围——正是 `ports.go` 自注那句要拦的。

**二、键上新增 `CreditSelector`（商业权限等级、费用类型），纪律与 `SettlementSelector` / `PriceDirection` 相同：请求信用依据必填，其余请求必缺，部分给出即`输入未受理`。** 法人取键上 `LegalEntityCandidate`，时点取锚点——键上已有的维度不重复携带，重复携带就允许两者不一致。闭包键与单依据键同一形；闭包解析为 `CreditPolicyObject` 形成单依据键时把选择器带过去（`creditBasisKey`），其余成员的单依据键仍一律不携带。信用依据没有结算政策那样的前提（合同维由闭包解出），所以不进 `resolutionOrder` 的延后段，按声明次序解。选择器进两种键的指纹——**解析身份因此换代**：`RES-` / `CLO-` / `CONT-` 全部按新指纹派生，此前固定的闭包在提交前重解时会判`已失效`。今天无租户、无生产固定解析，这是键形变化的诚实答案而不是要迁移的东西。

**三、`CreditBasis` 随结果交出，并进闭包快照。** `Resolution.AdoptedCreditBasis` 与闭包 `AdoptedBasis.CreditBasis` 交回出自哪一版政策、授权多少额度（金额或比例，`CreditLimit` 两格封闭不变）、`applicable`；信用政策参与 `ViewRevision` 派生，只改正文不动版本时解析身份仍变（ADR-0044 第四条同款）。闭包快照按 ADR-0028 **整份留存额度**，不回登记册按版本重读——登记册那份正文改一次，一次已固定的解析就会改口说自己当初授权的是别的额度。重建门校验快照里的额度必须挂在 `CreditPolicyObject` 那一格、额度已声明。`0020` 自本记录起进 `LoadForScope` 的整册装载（LEFT JOIN，一版一行不放大结果集），头注与 `ports.CreditPolicyContentView` 上「本表不进整册装载」那句作废、改写；`LoadCreditPolicy` 点读口保留，它答的是「已选出版本的正文」，不再是消费方取信用依据的路。

**四、SA 账期分支的授信额度从闭包交出的信用依据取。** `settlement-accounting` 新增端口 `CreditBasisView.LoadCreditBasis(tenant, scope, resolution)`，三格照 `PreAcceptanceControlPolicyView`（ADR-0054）：found=true 带 `CreditBasis`（政策版本引用 + 额度）；found=false = 闭包没采用信用政策——租户登记的解析键没要求这一项，恢复动作是补解析键与正文，不是重试；error = 坏回指 / 读不回 / 闭包不是唯一已解析。消费侧适配器 `settlementaccounting/adapters/partycommercial.CreditBasis` 与既有的 `PreAcceptanceControlPolicy` 并列，凭同一份回指取同一份闭包，只翻译不判断（ADR-0025）。`ApplyPreAcceptanceControlHandler.exposeCredit` 在读信用状况**之前**先索取信用依据：额度取政策授权的金额（`CreditStanding.WithAuthorizedLimit`），已占用暴露与逾期仍取本上下文自己的账本与状况登记；结果带 `CreditPolicy()` 引用，`AT-SA-171`「分别保存政策和范围」的政策半边由此成立。**比例额度的基数今天未裁**（`CreditLimit` 注释：「比例相对于什么基数由消费方的业务判断给出」），遇到比例额度停在新增的`待判断`格 `CREDIT_RATIO_BASE_UNDECIDED`，不折成金额、不默认——那是 `BD-*` 一类，等它自己的裁决。`credit_standing.limit_minor` 自此只是登记状况的一列，不再是额度依据的来源。

**五、三步法的 expand 段留一格，写明 contract 何时收。** `ApplyPreAcceptanceControlDeps.CreditBasis` 为 nil 时 `exposeCredit` 沿旧路（额度取登记状况），结果不带政策引用。留这一格不是容忍装配疏漏，是因为 `internal/parcelshipment/adapters/settlementaccounting` 的测试夹具直接构造这份 `Deps`（PS 地盘，本票不碰），mandatory 化会让另一个上下文的测试红。生产装配（`cmd/parcel-dispatch`）本记录同笔接上真适配器；contract 段——依赖 mandatory、nil 在构造期拒——随 PS 那份夹具补上 `CreditBasisView` 替身的那笔一起落，票面记为 SA 后续项。

## Consequences

- 基线 `production_wiring_baseline.txt` PC 段 `ResolveCreditPolicy` 条目按成因**第二种**（真接上：`resolveCreditPolicyBasis` 是它的生产调用方）剪掉；`production_type_reachability_baseline.txt` 的 `CreditBasis` 条目「随它出名单」一并剪掉（它经 `AdoptedBasis.CreditBasis` / `Resolution.AdoptedCreditBasis` 被 SA 适配器拿到）。
- 解析身份换代（见决定二），无租户故无迁移；闭包夹具凡请求信用依据须给 `CreditSelector` 并登记政策正文。
- **PS 登记面欠一格**：`commercial_resolution_keys.go` 的 `ResolutionKeyRegistration.validate` 今天放行 `CreditPolicyObject` 而不承载信用二维，含它的登记行 `FormResolutionKey` 会形成一个立不起来的键（`输入未受理`）。处置二选一——像 `PriceRuleObject` 那样在登记面拒，或补两维（PS 迁移）——归 PS 地盘另立票，本记录不替它选。
- SA 的 `NotFormedReason` 新增 `CREDIT_BASIS_UNAVAILABLE` / `CREDIT_BASIS_NOT_CONFIGURED` / `CREDIT_RATIO_BASE_UNDECIDED` 三格；暴露账本行上**不**持久化政策引用（要 SA 迁移，随 contract 段另立），本记录只让形成时的结果带它。
- `LoadForScope` 多一个 LEFT JOIN；`ViewRevision` 对登记了信用政策的范围换值。

## Alternatives considered

- **乙：闭包只认唯一版本壳，`ResolveCreditPolicy` 退化成 `covers` 校验。** 否决：同一闭包里价格规则与结算政策按类别专属选择器解、信用政策按版本壳解，两套解析语义并存；且多法人 / 多等级的租户只能把每一格拆成不同范围，把本该由选择器答的维度逼进范围划分——`ports.go` 自注那句要拦的正是这个。
- **不加 `CreditSelector`，从合同 / 委派推等级与费用类型。** 否决：等级与费用类型是租户的实例参数，闭包里没有任何对象拥有「这笔委托按哪一等级、哪一费用类型授信」这句话；推出来就是发明实例参数。
- **SA 直接改 `CreditStandingView` 签名带回指。** 否决：那只端口的实现与替身横跨 PS 地盘的测试夹具，签名变更会让别人的树编不过；新增端口与三步法都不需要那一步。
- **SA 比对登记额度与政策额度、不等即冲突。** 否决：登记额度在生产路径上无人写入，为一个替身立一道冲突格是给一个不存在的东西编语义。

## Links

- [ADR-0044](./0044-settlement-basis-adopts-via-settlement-policy.md)：镜像先例（选择器进键、政策进登记册与 `ViewRevision`、结果带政策）
- [ADR-0080](./0080-commercial-closure-resolves-the-contract-first-and-keys-settlement-by-it.md)：闭包成员间解析顺序的先例——信用依据没有那样的前提，不进延后段
- [ADR-0115](./0115-pre-acceptance-financial-control-policy-content-is-a-row-per-control-and-no-control-stays-with-the-contract.md)：决定四写明「控制项只说在这个范围上做信用校验，不说按哪一版信用政策……由消费侧按闭包解析取得」——本记录答的正是那一句留给 SA 的问题
- [ADR-0054](./0054-pre-acceptance-control-policy-view-has-an-unconfigured-grade.md)：`CreditBasisView` 三格的出处
- [ADR-0028](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)：快照整份留存额度、重建门校验
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：消费侧适配器只翻译不判断
- [UC-SA-002](../application/settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md)：`AT-SA-171` / `AT-SA-172`
- 票 [wiring-baseline-remainder/03](../../.scratch/wiring-baseline-remainder/issues/03-pc-credit-basis-is-never-asked-for-the-pc-to-sa-seam-does-not-exist.md)
