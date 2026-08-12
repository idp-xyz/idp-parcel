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

// AuthorityViewRevision 用于证明某个范围解析当时所依据的商业视图是否仍是同一个。它
// 不替代不可变的对象版本：新修订的意思是「需要重新检查是否仍相容」，而不是「采用版本
// 变了」。
type AuthorityViewRevision struct{ requiredValue }

// ContinuationReference 让调用方把停下的决定重新接上。`已失效`与`解析未决`都必须保持
// 可续办，因为两者都不是调用方可以据以行动的业务拒绝。
type ContinuationReference struct{ requiredValue }

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
	if key.Purpose == PricingPurpose {
		return key.PriceDirection.valid()
	}
	return !key.PriceDirection.valid()
}

func (key ResolutionKey) fingerprint() string {
	return strings.Join([]string{
		key.TenantID.String(),
		key.CustomerAccountID.String(),
		key.LegalEntityCandidate.String(),
		key.Scope.String(),
		key.RequiredBasis.String(),
		key.Purpose.String(),
		key.PriceDirection.String(),
		key.Anchor.PolicyVersion().String(),
		key.Anchor.At().Format(time.RFC3339Nano),
	}, "\x00")
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
	outcome        ResolutionOutcome
	resolutionID   ResolutionID
	key            ResolutionKey
	anchor         SelectionAnchor
	adopted        CommercialVersion
	hasAdopted     bool
	pricePolicy    CommercialPricePolicy
	hasPricePolicy bool
	viewRevision   AuthorityViewRevision
	reason         ResolutionReason
	continuation   ContinuationReference
	candidateCount int
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

	candidates := registry.applicable(key)
	result := Resolution{
		key:            key,
		anchor:         key.Anchor,
		viewRevision:   registry.ViewRevision(key.Scope),
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
		viewRevision: registry.ViewRevision(key.Scope),
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

// ValidateBeforeDecision 在调用方提交决定之前重跑第一阶段。它按原查询重解，而不是只
// 检查已采用对象自身：同范围新增一个竞争候选时，那个对象一个字节都没变，解析却已经
// 不再唯一，只有重解看得见。
//
// 原本就不是唯一解析的结果原样返回——不存在「采用依据是否仍有效」这个问题。
func ValidateBeforeDecision(
	registry *CommercialRegistry,
	prior Resolution,
	standingOf PricingPlanStandingLookup,
) Resolution {
	if prior.outcome != UniquelyResolved {
		return prior
	}
	// 权威读不到时，原结果既不能被确认也不能被断言失效，因此保持`解析未决`且可续办。
	if registry == nil {
		stalled := prior
		stalled.outcome = ResolutionPending
		stalled.adopted = CommercialVersion{}
		stalled.hasAdopted = false
		stalled.pricePolicy = CommercialPricePolicy{}
		stalled.hasPricePolicy = false
		stalled.reason = AuthorityUnreadable
		stalled.continuation = continuationFor(prior.key.fingerprint(), prior.resolutionID, AuthorityUnreadable)
		return stalled
	}

	current := ResolveCommercialBasis(registry, prior.key, standingOf)
	if current.outcome == UniquelyResolved && current.resolutionID == prior.resolutionID {
		return prior
	}

	stale := Resolution{
		outcome:        ResolutionStale,
		resolutionID:   prior.resolutionID,
		key:            prior.key,
		anchor:         prior.anchor,
		viewRevision:   current.viewRevision,
		candidateCount: current.candidateCount,
		reason:         CurrentResolutionChanged,
		continuation:   continuationFor(prior.key.fingerprint(), prior.resolutionID, CurrentResolutionChanged),
	}
	return stale
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
// 且在锚点时刻生效。已收尾的版本在这里就落选，而不是留到后面再过滤。
func (registry *CommercialRegistry) applicable(key ResolutionKey) []CommercialVersion {
	matches := make([]CommercialVersion, 0, 2)
	for _, version := range registry.versions {
		if version.kind != key.RequiredBasis || version.scope != key.Scope {
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
