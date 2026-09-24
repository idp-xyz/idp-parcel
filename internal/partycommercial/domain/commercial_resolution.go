package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

var ErrInvalidSelectionAnchor = errors.New("party commercial: invalid commercial selection anchor")

type TenantID struct{ requiredValue }

func NewTenantID(value string) (TenantID, error) {
	required, err := newRequiredValue("tenant ID", value)
	return TenantID{required}, err
}

type CustomerAccountID struct{ requiredValue }

func NewCustomerAccountID(value string) (CustomerAccountID, error) {
	required, err := newRequiredValue("customer account ID", value)
	return CustomerAccountID{required}, err
}

type LegalEntityReference struct{ requiredValue }

func NewLegalEntityReference(value string) (LegalEntityReference, error) {
	required, err := newRequiredValue("legal entity reference", value)
	return LegalEntityReference{required}, err
}

type AnchorPolicyVersion struct{ requiredValue }

func NewAnchorPolicyVersion(value string) (AnchorPolicyVersion, error) {
	required, err := newRequiredValue("anchor policy version", value)
	return AnchorPolicyVersion{required}, err
}

type ResolutionID struct{ requiredValue }

// NewResolutionID 从消费方记录的字符串重建解析标识（ADR-0027：按标识回指的入口）。
// 它不签发新解析——身份仍由 resolutionIdentity 派生；这里只收非空引用。
func NewResolutionID(value string) (ResolutionID, error) {
	required, err := newRequiredValue("resolution ID", value)
	return ResolutionID{required}, err
}

// AuthorityViewRevision 用于证明某个范围解析当时所依据的商业视图是否仍是同一个。它
// 不替代不可变的对象版本：新修订的意思是「需要重新检查是否仍相容」，而不是「采用版本
// 变了」。
type AuthorityViewRevision struct{ requiredValue }

func NewAuthorityViewRevision(value string) (AuthorityViewRevision, error) {
	required, err := newRequiredValue("authority view revision", value)
	return AuthorityViewRevision{required}, err
}

// ContinuationReference 让调用方把停下的决定重新接上。`已失效`与`解析未决`都必须保持
// 可续办，因为两者都不是调用方可以据以行动的业务拒绝。
type ContinuationReference struct{ requiredValue }

func NewContinuationReference(value string) (ContinuationReference, error) {
	required, err := newRequiredValue("continuation reference", value)
	return ContinuationReference{required}, err
}

// SelectionAnchor 是第一阶段据以选择的业务时点。没有产生它的版本化策略就构造不出来
// ——这正是拦住来源发生时间、客户请求时间、系统当前时间或待选规则包悄悄变成锚点的
// 办法。
type SelectionAnchor struct {
	at            time.Time
	policyVersion AnchorPolicyVersion
}

func NewSelectionAnchor(at time.Time, policyVersion AnchorPolicyVersion) (SelectionAnchor, error) {
	if at.IsZero() || !policyVersion.valid() {
		return SelectionAnchor{}, ErrInvalidSelectionAnchor
	}
	return SelectionAnchor{at: at.UTC(), policyVersion: policyVersion}, nil
}

func (anchor SelectionAnchor) At() time.Time {
	return anchor.at
}

func (anchor SelectionAnchor) PolicyVersion() AnchorPolicyVersion {
	return anchor.policyVersion
}

func (anchor SelectionAnchor) valid() bool {
	return !anchor.at.IsZero() && anchor.policyVersion.valid()
}

// ResolutionPurpose 是调用方要这份依据做什么。它属于解析键的一部分，因为同一范围对
// 不同问题给出不同答案：接受控制依据与计价依据不可互换。
type ResolutionPurpose uint8

const (
	ResolutionPurposeInvalid ResolutionPurpose = iota
	AcceptanceControlPurpose
	PricingPurpose
)

func (purpose ResolutionPurpose) valid() bool {
	return purpose >= AcceptanceControlPurpose && purpose <= PricingPurpose
}

func (purpose ResolutionPurpose) String() string {
	switch purpose {
	case AcceptanceControlPurpose:
		return "ACCEPTANCE_CONTROL"
	case PricingPurpose:
		return "PRICING"
	default:
		return ""
	}
}

// PriceDirection 区分对外销售、对外采购与运营企业自有法人之间的结算。三者绝不共用
// 解析结果与缓存，因此 `SELL` 请求不可能被 `BUY` 结果回答。
type PriceDirection uint8

const (
	PriceDirectionInvalid PriceDirection = iota
	BuyDirection
	SellDirection
	InternalDirection
)

func (direction PriceDirection) valid() bool {
	return direction >= BuyDirection && direction <= InternalDirection
}

func (direction PriceDirection) String() string {
	switch direction {
	case BuyDirection:
		return "BUY"
	case SellDirection:
		return "SELL"
	case InternalDirection:
		return "INTERNAL"
	default:
		return ""
	}
}

// ResolutionKey 是选出一种必需依据所依据的完整维度集合。少任何一维，都会让一个客户的
// 解析回答另一个客户，或让一个范围借用为另一范围解析出的依据。
type ResolutionKey struct {
	TenantID             TenantID
	CustomerAccountID    CustomerAccountID
	LegalEntityCandidate LegalEntityReference
	Scope                CommercialScopeReference
	RequiredBasis        CommercialObjectKind
	Purpose              ResolutionPurpose
	// PriceDirection 只对计价目的有意义，其他目的必须缺席：否则两个仅在对该目的
	// 毫无意义的维度上不同的键，会解析出不同的身份。
	PriceDirection PriceDirection
	// Settlement 只在请求结算政策依据时有意义，纪律与 PriceDirection 同（ADR-0044）：
	// 结算必填、其余必缺，部分给出即输入未受理。
	Settlement SettlementSelector
	// Credit 只在请求信用政策依据时有意义，纪律同上（ADR-0127）。
	Credit CreditSelector
	// ServiceProduct 是委托声明请求的服务产品——对象身份，不是版本：选哪一版仍按锚点解（ADR-0080）。在场即收窄
	// 候选：服务产品按身份，其余对象看正文有没有指名另一个服务产品（admitsDeclaredServiceProduct）。它不设「只对
	// 某一种依据在场」的纪律——闭包把它带给每一项成员，起不起作用由候选自己的正文决定。
	ServiceProduct CommercialObjectID
	Anchor         SelectionAnchor
}

func (key ResolutionKey) minimumIdentityEstablished() bool {
	if !key.TenantID.valid() ||
		!key.CustomerAccountID.valid() ||
		!key.LegalEntityCandidate.valid() ||
		!key.Scope.valid() ||
		!key.RequiredBasis.valid() ||
		!key.Purpose.valid() {
		return false
	}
	if key.RequiredBasis == SettlementPolicyObject {
		if !key.Settlement.declared() {
			return false
		}
	} else if !key.Settlement.empty() {
		return false
	}
	if key.RequiredBasis == CreditPolicyObject {
		if !key.Credit.declared() {
			return false
		}
	} else if !key.Credit.empty() {
		return false
	}
	if key.Purpose == PricingPurpose {
		return key.PriceDirection.valid()
	}
	return !key.PriceDirection.valid()
}

func (key ResolutionKey) fingerprint() string {
	parts := []string{
		key.TenantID.String(),
		key.CustomerAccountID.String(),
		key.LegalEntityCandidate.String(),
		key.Scope.String(),
		key.RequiredBasis.String(),
		key.Purpose.String(),
		key.PriceDirection.String(),
		key.Settlement.fingerprint(),
		key.Credit.fingerprint(),
		key.Anchor.PolicyVersion().String(),
		key.Anchor.At().Format(time.RFC3339Nano),
	}
	// 声明的服务产品只在在场时追加：不声明的键指纹与这一维出现之前逐字节相同，已固定的解析标识与续办引用
	// 因此不因本维的出现而改口。
	if key.ServiceProduct.valid() {
		parts = append(parts, "service-product="+key.ServiceProduct.String())
	}
	return strings.Join(parts, "\x00")
}

// ResolutionOutcome 是第一阶段可能给出的封闭答案集合。它们刻意分开：`无适用依据`是
// 权威说了话，`解析未决`是权威根本没读到；合并两者会让一次依赖失败被读成「这个客户
// 没有合同」。
type ResolutionOutcome uint8

const (
	ResolutionOutcomeInvalid ResolutionOutcome = iota
	UniquelyResolved
	NoApplicableBasis
	ApplicabilityConflict
	ResolutionPending
	InputNotAccepted
	ResolutionStale
	// BasisNotResolved 说的是调用方回指的解析标识不指向一份它可用的原解析。它刻意不区分
	// 「从未签发」与「属于另一个客户账户」——两者的恢复动作同为回第一阶段重解，分开就等于
	// 回答了调用方无权知道的「这份解析存不存在」（ADR-0029）。
	BasisNotResolved
)

func (outcome ResolutionOutcome) String() string {
	switch outcome {
	case UniquelyResolved:
		return "UNIQUELY_RESOLVED"
	case NoApplicableBasis:
		return "NO_APPLICABLE_BASIS"
	case ApplicabilityConflict:
		return "APPLICABILITY_CONFLICT"
	case ResolutionPending:
		return "RESOLUTION_PENDING"
	case InputNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case ResolutionStale:
		return "STALE"
	case BasisNotResolved:
		return "BASIS_NOT_RESOLVED"
	default:
		return ""
	}
}

// ResolutionReason 是未能给出可用依据时的稳定原因。它是封闭集合而非自由文本，这样
// `解析未决`与`已失效`才能按原因维度统计而不是按消息文本检索；取值与产生它的规则同时
// 出现。
//
// 唯一解析没有原因，所以零值表示「缺席」而不是「未知」。
type ResolutionReason uint8

const (
	ResolutionReasonNone ResolutionReason = iota
	AnchorPolicyNotConfigured
	AuthorityUnreadable
	CurrentResolutionChanged
	// NamedReferenceNotConfirmed 说的是某个采用版本在正文里指名的对外引用，本次解析
	// 确认不了它就是被采用的那一个。它是未决而不是`无适用依据`：权威并没有说这个范围
	// 没有对象，是本次解析没能核实指名的那一个。
	NamedReferenceNotConfirmed
	// BoundPlanNotConfirmed / BoundPlanWithdrawn 与 ErrPricingPlan* 一一对应：同一份
	// 政策绑的方案「没问到」与「已退役」恢复动作不同，原因必须分开（ADR-0032 / ADR-0034）。
	// 前缀 Bound 避免与 PricingPlanStanding 的同名常量撞车。
	BoundPlanNotConfirmed
	BoundPlanWithdrawn
)

func (reason ResolutionReason) String() string {
	switch reason {
	case AnchorPolicyNotConfigured:
		return "ANCHOR_POLICY_NOT_CONFIGURED"
	case AuthorityUnreadable:
		return "AUTHORITY_UNREADABLE"
	case CurrentResolutionChanged:
		return "CURRENT_RESOLUTION_CHANGED"
	case NamedReferenceNotConfirmed:
		return "NAMED_REFERENCE_NOT_CONFIRMED"
	case BoundPlanNotConfirmed:
		return "BOUND_PLAN_NOT_CONFIRMED"
	case BoundPlanWithdrawn:
		return "BOUND_PLAN_WITHDRAWN"
	default:
		return ""
	}
}

type Resolution struct {
	outcome             ResolutionOutcome
	resolutionID        ResolutionID
	key                 ResolutionKey
	anchor              SelectionAnchor
	adopted             CommercialVersion
	hasAdopted          bool
	pricePolicy         CommercialPricePolicy
	hasPricePolicy      bool
	settlementPolicy    SettlementPolicy
	hasSettlementPolicy bool
	creditBasis         CreditBasis
	hasCreditBasis      bool
	viewRevision        AuthorityViewRevision
	reason              ResolutionReason
	continuation        ContinuationReference
	candidateCount      int
}

func (resolution Resolution) Outcome() ResolutionOutcome {
	return resolution.outcome
}

func (resolution Resolution) ResolutionID() ResolutionID {
	return resolution.resolutionID
}

func (resolution Resolution) Anchor() SelectionAnchor {
	return resolution.anchor
}

func (resolution Resolution) AdoptedVersion() (CommercialVersion, bool) {
	return resolution.adopted, resolution.hasAdopted
}

// AdoptedPricePolicy 在计价目的下采用价格政策时交回完整绑定；非计价或未采用时缺席。
func (resolution Resolution) AdoptedPricePolicy() (CommercialPricePolicy, bool) {
	return resolution.pricePolicy, resolution.hasPricePolicy
}

// AdoptedSettlementPolicy 在采用结算政策依据时交回完整政策——预付/账期方式与六维适用
// 范围因此可观察（ADR-0044）；其他依据缺席。
func (resolution Resolution) AdoptedSettlementPolicy() (SettlementPolicy, bool) {
	return resolution.settlementPolicy, resolution.hasSettlementPolicy
}

// AdoptedCreditBasis 在采用信用政策依据时交回出自哪一版政策、授权多少额度（ADR-0127）；
// 其他依据缺席。缺席与「授予零额度」由 CreditBasis.Applicable 分开，这里不替它折叠。
func (resolution Resolution) AdoptedCreditBasis() (CreditBasis, bool) {
	return resolution.creditBasis, resolution.hasCreditBasis
}

// CandidateCount 报告权威侧持有多少个适用版本。输入未受理时恒为零：去数候选本身就
// 已经泄露了调用方无权命名的范围里有没有对象。
func (resolution Resolution) CandidateCount() int {
	return resolution.candidateCount
}

func (resolution Resolution) ViewRevision() (AuthorityViewRevision, bool) {
	if !resolution.viewRevision.valid() {
		return AuthorityViewRevision{}, false
	}
	return resolution.viewRevision, true
}

// Reason 指名解析为何未能给出可用依据。它与 ContinuationReference 配对：原因说明要修
// 什么，引用说明续办哪一次尝试。
func (resolution Resolution) Reason() ResolutionReason {
	return resolution.reason
}

func (resolution Resolution) ContinuationReference() ContinuationReference {
	return resolution.continuation
}

// ResolveCommercialBasis 执行第一阶段：为一种必需依据选出唯一适用版本。它不形成接受、
// 价格、财务控制或任何下游 `asOf`；第二阶段是调用方的事，由本次解析采用的接单规则包
// 驱动。
//
// standingOf 只在计价目的下的价格规则路径上被问到（ADR-0034）；其他路径忽略它。
func ResolveCommercialBasis(
	registry *CommercialRegistry,
	key ResolutionKey,
	standingOf PricingPlanStandingLookup,
) Resolution {
	if !key.minimumIdentityEstablished() {
		return Resolution{outcome: InputNotAccepted}
	}
	// 锚点策略缺失是`解析未决`而非失败：该策略属实例参数，可能只是尚未配置；这里
	// 唯一绝不能做的事，是拿一个默认时刻顶替它。
	if !key.Anchor.valid() {
		return pending(key, ResolutionID{}, AnchorPolicyNotConfigured, SelectionAnchor{})
	}
	if registry == nil {
		return pending(key, ResolutionID{}, AuthorityUnreadable, key.Anchor)
	}

	if key.RequiredBasis == PriceRuleObject && key.Purpose == PricingPurpose {
		return resolvePriceRuleBasis(registry, key, standingOf)
	}
	if key.RequiredBasis == SettlementPolicyObject {
		return resolveSettlementPolicyBasis(registry, key)
	}
	if key.RequiredBasis == CreditPolicyObject {
		return resolveCreditPolicyBasis(registry, key)
	}

	candidates := registry.applicable(key)
	result := Resolution{
		key:            key,
		anchor:         key.Anchor,
		viewRevision:   registry.ViewRevision(key.TenantID, key.Scope),
		candidateCount: len(candidates),
	}
	switch len(candidates) {
	case 0:
		result.outcome = NoApplicableBasis
	case 1:
		result.outcome = UniquelyResolved
		result.adopted = candidates[0]
		result.hasAdopted = true
		result.resolutionID = resolutionIdentity(key, result.viewRevision, candidates[0])
	default:
		result.outcome = ApplicabilityConflict
	}
	return result
}

// resolvePriceRuleBasis 经商业价格政策选用价格规则，把方向与定价方案绑定一并带回。
func resolvePriceRuleBasis(
	registry *CommercialRegistry,
	key ResolutionKey,
	standingOf PricingPlanStandingLookup,
) Resolution {
	result := Resolution{
		key:          key,
		anchor:       key.Anchor,
		viewRevision: registry.ViewRevision(key.TenantID, key.Scope),
	}
	query, err := NewPricePolicyQuery(key.PriceDirection, key.Scope, key.Anchor.At())
	if err != nil {
		return Resolution{outcome: InputNotAccepted}
	}
	policy, err := ResolveCommercialPricePolicy(registry.policies, query, standingOf)
	switch {
	case err == nil:
		result.outcome = UniquelyResolved
		result.adopted = policy.Version()
		result.hasAdopted = true
		result.pricePolicy = policy
		result.hasPricePolicy = true
		result.candidateCount = 1
		result.resolutionID = resolutionIdentity(key, result.viewRevision, policy.Version())
		return result
	case errors.Is(err, ErrNoApplicablePricePolicy):
		result.outcome = NoApplicableBasis
		return result
	case errors.Is(err, ErrPricePolicyConflict):
		result.outcome = ApplicabilityConflict
		result.candidateCount = 2
		return result
	case errors.Is(err, ErrPricingPlanWithdrawn):
		return pending(key, ResolutionID{}, BoundPlanWithdrawn, key.Anchor)
	default:
		// 含 ErrPricingPlanNotConfirmed 与 standing 零值：没问到就是未确认。
		return pending(key, ResolutionID{}, BoundPlanNotConfirmed, key.Anchor)
	}
}

// resolveSettlementPolicyBasis 经结算政策选用结算依据，把方式与适用范围一并带回
// （ADR-0044，镜像 resolvePriceRuleBasis）。候选先按政策版本的租户与范围收窄，再由
// ResolveSettlementPolicy 按键上选择器的精确六维裁决——不同费用范围的预付与账期因此
// 互不冲突（AT-PC-031），同一精确范围多候选仍是适用冲突（AT-PC-032）。
func resolveSettlementPolicyBasis(registry *CommercialRegistry, key ResolutionKey) Resolution {
	result := Resolution{
		key:          key,
		anchor:       key.Anchor,
		viewRevision: registry.ViewRevision(key.TenantID, key.Scope),
	}
	query, err := NewSettlementQuery(
		key.LegalEntityCandidate,
		key.Settlement.Counterparty,
		key.Settlement.Contract,
		key.Settlement.ChargeScope,
		key.Settlement.Currency,
		key.Anchor.At(),
	)
	if err != nil {
		return Resolution{outcome: InputNotAccepted}
	}
	inScope := make([]SettlementPolicy, 0, len(registry.settlementPolicies))
	for _, policy := range registry.settlementPolicies {
		if policy.version.tenant != key.TenantID || policy.version.scope != key.Scope {
			continue
		}
		inScope = append(inScope, policy)
	}
	policy, err := ResolveSettlementPolicy(inScope, query)
	switch {
	case err == nil:
		result.outcome = UniquelyResolved
		result.adopted = policy.Version()
		result.hasAdopted = true
		result.settlementPolicy = policy
		result.hasSettlementPolicy = true
		result.candidateCount = 1
		result.resolutionID = resolutionIdentity(key, result.viewRevision, policy.Version())
		return result
	case errors.Is(err, ErrSettlementMethodConflict):
		result.outcome = ApplicabilityConflict
		result.candidateCount = 2
		return result
	default:
		// ResolveSettlementPolicy 只有冲突与零候选两种失败；这里就是零候选。光有已登记
		// 版本没有政策也落在这一格：通用版本解析产不出方式与范围。
		result.outcome = NoApplicableBasis
		return result
	}
}

// resolveCreditPolicyBasis 经信用政策选用信用依据，把额度与出处一并带回（ADR-0127，镜像
// resolveSettlementPolicyBasis）。候选先按政策版本的租户与范围收窄，再由 ResolveCreditPolicy
// 按（法人、等级、费用类型、时点）裁决：不同费用类型互不冲突，同一格多候选是适用冲突。
// 信用政策没有「按哪一版合同选」那样的前提，所以这里不像结算政策那样等别的成员先解出来。
func resolveCreditPolicyBasis(registry *CommercialRegistry, key ResolutionKey) Resolution {
	result := Resolution{
		key:          key,
		anchor:       key.Anchor,
		viewRevision: registry.ViewRevision(key.TenantID, key.Scope),
	}
	query, err := NewCreditPolicyQuery(
		key.LegalEntityCandidate,
		key.Credit.Level,
		key.Credit.ChargeType,
		key.Anchor.At(),
	)
	if err != nil {
		return Resolution{outcome: InputNotAccepted}
	}
	inScope := make([]CreditPolicy, 0, len(registry.creditPolicies))
	for _, policy := range registry.creditPolicies {
		if policy.version.tenant != key.TenantID || policy.version.scope != key.Scope {
			continue
		}
		inScope = append(inScope, policy)
	}
	basis, err := ResolveCreditPolicy(inScope, query)
	switch {
	case err == nil:
		result.outcome = UniquelyResolved
		result.adopted = basis.PolicyVersion()
		result.hasAdopted = true
		result.creditBasis = basis
		result.hasCreditBasis = true
		result.candidateCount = 1
		result.resolutionID = resolutionIdentity(key, result.viewRevision, basis.PolicyVersion())
		return result
	case errors.Is(err, ErrCreditPolicyConflict):
		result.outcome = ApplicabilityConflict
		result.candidateCount = 2
		return result
	default:
		// ResolveCreditPolicy 只有冲突与零候选两种失败；这里就是零候选。光有已登记版本没有
		// 正文也落在这一格：通用版本解析产不出额度，而缺政策既不是无限信用也不是零额度。
		result.outcome = NoApplicableBasis
		return result
	}
}

// pending 构造所有未决答案共用的那一种形状。让它们全部走这里，是为了不让一个结果的
// 可用性取决于它由哪条路径产生：在此之前，提交前校验产生的未决可续办，而首次解析产生
// 的未决不可续办。
func pending(
	key ResolutionKey,
	priorID ResolutionID,
	reason ResolutionReason,
	anchor SelectionAnchor,
) Resolution {
	return Resolution{
		outcome:      ResolutionPending,
		key:          key,
		anchor:       anchor,
		reason:       reason,
		continuation: continuationFor(key.fingerprint(), priorID, reason),
	}
}

// continuationFor 派生调用方续办一次停滞决定所用的引用。它由查询、原解析标识与原因
// 共同派生，因此同一输入因同一原因停滞时拿到的引用始终相同——这正是调用方能查询原次
// 尝试而不必靠猜的原因。
//
// 取查询指纹而不取查询本身，是为了让单依据与引用闭包两种解析共用这一处派生：两者的键
// 形状不同，但「同一输入同一原因得到同一引用」这条对它们是同一条规则。
func continuationFor(fingerprint string, priorID ResolutionID, reason ResolutionReason) ContinuationReference {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		reason.String(),
		fingerprint,
		priorID.String(),
	}, "\x00")))
	return ContinuationReference{requiredValue{value: "CONT-" + hex.EncodeToString(digest[:8])}}
}

// resolutionIdentity 由解析键、权威视图修订与采用版本共同派生：同一输入在同一视图下
// 答案身份相同，视图一有变化身份就不同。提交前校验因此只靠比对就能发现失效。它不是新的
// 商业版本，也不创建任何东西。
func resolutionIdentity(key ResolutionKey, view AuthorityViewRevision, adopted CommercialVersion) ResolutionID {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		key.fingerprint(),
		view.String(),
		adopted.objectID.String(),
		adopted.version.String(),
		adopted.contentDigest.String(),
	}, "\x00")))
	return ResolutionID{requiredValue{value: "RES-" + hex.EncodeToString(digest[:8])}}
}

// applicable 把登记册收窄到仍可用于新决定的版本：请求的依据类型、请求的适用范围，
// 且在锚点时刻生效；委托声明了服务产品时，还要容得下那个产品。已收尾的版本在这里就落选，
// 而不是留到后面再过滤。
func (registry *CommercialRegistry) applicable(key ResolutionKey) []CommercialVersion {
	matches := make([]CommercialVersion, 0, 2)
	for _, version := range registry.versions {
		if version.tenant != key.TenantID || version.kind != key.RequiredBasis || version.scope != key.Scope {
			continue
		}
		if key.ServiceProduct.valid() && !version.admitsDeclaredServiceProduct(key.ServiceProduct) {
			continue
		}
		// 选用区间问登记册：有效性更正不改版本值对象上的原区间（ADR-0038）。
		if version.status != CommercialVersionEffective {
			continue
		}
		if !registry.selectionInterval(version).Contains(key.Anchor.At()) {
			continue
		}
		matches = append(matches, version)
	}
	return matches
}

// admitsDeclaredServiceProduct 答委托声明了某个服务产品时这一版还能不能参选：服务产品本身按身份；其余对象
// 正文指名了另一个服务产品的落选，指名了声明的那个或根本没指名服务产品的照旧参选。
//
// 只收窄服务产品而不收窄指名它的对象不够：同一范围两个产品各配一份接单规则包是常规形态，那样规则包那一项仍会
// 答`适用冲突`。收窄之后 namedReferencesConfirmed 照旧事后核对，两道各守一边。
func (version CommercialVersion) admitsDeclaredServiceProduct(declared CommercialObjectID) bool {
	if version.kind == ServiceProductObject {
		return version.objectID == declared
	}
	for _, reference := range version.references {
		if reference.kind == ServiceProductObject && reference.objectID != declared {
			return false
		}
	}
	return true
}
