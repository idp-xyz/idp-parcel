package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidAuthorityGrant       = errors.New("party commercial: invalid authority grant")
	ErrInvalidAuthorizationRequest = errors.New("party commercial: invalid authorization request")
	ErrNotAuthorized               = errors.New("party commercial: no effective grant authorizes this request")
	// ErrAuthorityRulesNotConfigured 说的是这个范围在这个时刻一条已生效的授权规则都没有，
	// 因此本上下文还答不了「许不许」。它与 ErrNotAuthorized 的恢复动作相反：前者要租户先把
	// `PAR-COM-14` 的授权规则登记上，后者是权威已经答过的业务拒绝，再登记也不会变。
	//
	// 首发尤其要紧：没有租户就没有任何授权规则，每一次请求都落在这一格上。压成`不允许`，
	// 等于把一个尚未配置的产品说成「你无权这么做」。
	ErrAuthorityRulesNotConfigured = errors.New("party commercial: no authorization rule is configured for this scope at this time")
)

// AuthorityLevel 是商业权限等级，不是人事职级。它是版本化的业务授权：职务名称、
// 组织位置或技术账号都不会带来它，任何参与方角色也不能顶替它。
type AuthorityLevel struct{ requiredValue }

func NewAuthorityLevel(value string) (AuthorityLevel, error) {
	required, err := newRequiredValue("authority level", value)
	return AuthorityLevel{required}, err
}

type StructuredReason struct{ requiredValue }

func NewStructuredReason(value string) (StructuredReason, error) {
	required, err := newRequiredValue("structured reason", value)
	return StructuredReason{required}, err
}

type EvidenceReference struct{ requiredValue }

func NewEvidenceReference(value string) (EvidenceReference, error) {
	required, err := newRequiredValue("evidence reference", value)
	return EvidenceReference{required}, err
}

// AuthorizedAction 是一份授权允许做的事。人工复核与主动拒绝是两个动作，因为获准
// 复核并不等于获准直接拒掉这单业务。
type AuthorizedAction uint8

const (
	AuthorizedActionInvalid AuthorizedAction = iota
	ManualReviewAction
	ActiveRejectionAction
)

func (action AuthorizedAction) valid() bool {
	return action >= ManualReviewAction && action <= ActiveRejectionAction
}

func (action AuthorizedAction) String() string {
	switch action {
	case ManualReviewAction:
		return "MANUAL_REVIEW"
	case ActiveRejectionAction:
		return "ACTIVE_REJECTION"
	default:
		return ""
	}
}

// AuthorityGrant 是一条版本化授权：在哪个有效区间内，为哪个责任法人和适用范围、
// 以哪个商业权限等级，允许哪个动作。
type AuthorityGrant struct {
	version     CommercialVersion
	action      AuthorizedAction
	legalEntity LegalEntityReference
	level       AuthorityLevel
	scope       CommercialScopeReference
	effective   EffectiveInterval
}

func NewAuthorityGrant(
	version CommercialVersion,
	action AuthorizedAction,
	legalEntity LegalEntityReference,
	level AuthorityLevel,
	scope CommercialScopeReference,
	effective EffectiveInterval,
) (AuthorityGrant, error) {
	if version.kind != AuthorizationRuleObject ||
		version.status != CommercialVersionEffective ||
		!action.valid() || !legalEntity.valid() || !level.valid() ||
		!scope.valid() || !effective.valid() {
		return AuthorityGrant{}, ErrInvalidAuthorityGrant
	}
	return AuthorityGrant{
		version:     version,
		action:      action,
		legalEntity: legalEntity,
		level:       level,
		scope:       scope,
		effective:   effective,
	}, nil
}

func (grant AuthorityGrant) Version() CommercialVersion {
	return grant.version
}

func (grant AuthorityGrant) Action() AuthorizedAction {
	return grant.action
}

func (grant AuthorityGrant) Level() AuthorityLevel {
	return grant.level
}

func (grant AuthorityGrant) LegalEntity() LegalEntityReference {
	return grant.legalEntity
}

func (grant AuthorityGrant) Scope() CommercialScopeReference {
	return grant.scope
}

func (grant AuthorityGrant) Effective() EffectiveInterval {
	return grant.effective
}

func (grant AuthorityGrant) permits(request AuthorizationRequest) bool {
	return grant.action == request.action &&
		grant.legalEntity == request.legalEntity &&
		grant.level == request.level &&
		grant.scope == request.scope &&
		grant.effective.Contains(request.at)
}

// AuthorizationRequest 请求执行一个动作。它有意不携带参与方角色：承运商代理商或
// 渠道服务方即便持有某种关系角色，在这里也一无所得——授权只来自版本化的授权规则。
type AuthorizationRequest struct {
	action      AuthorizedAction
	legalEntity LegalEntityReference
	level       AuthorityLevel
	scope       CommercialScopeReference
	reason      StructuredReason
	evidence    EvidenceReference
	at          time.Time
}

// NewAuthorizationRequest 要求每个动作都带结构化原因和证据。缺了它们的主动拒绝
// 事后无从辩护，而自由文本理由会让拒绝无法按原因统计。
func NewAuthorizationRequest(
	action AuthorizedAction,
	legalEntity LegalEntityReference,
	level AuthorityLevel,
	scope CommercialScopeReference,
	reason StructuredReason,
	evidence EvidenceReference,
	at time.Time,
) (AuthorizationRequest, error) {
	if !action.valid() || !legalEntity.valid() || !level.valid() || !scope.valid() ||
		!reason.valid() || !evidence.valid() || at.IsZero() {
		return AuthorizationRequest{}, ErrInvalidAuthorizationRequest
	}
	return AuthorizationRequest{
		action:      action,
		legalEntity: legalEntity,
		level:       level,
		scope:       scope,
		reason:      reason,
		evidence:    evidence,
		at:          at.UTC(),
	}, nil
}

func (request AuthorizationRequest) Action() AuthorizedAction {
	return request.action
}

func (request AuthorizationRequest) LegalEntity() LegalEntityReference {
	return request.legalEntity
}

func (request AuthorizationRequest) Level() AuthorityLevel {
	return request.level
}

func (request AuthorizationRequest) Scope() CommercialScopeReference {
	return request.scope
}

func (request AuthorizationRequest) Reason() StructuredReason {
	return request.reason
}

func (request AuthorizationRequest) Evidence() EvidenceReference {
	return request.evidence
}

func (request AuthorizationRequest) At() time.Time {
	return request.at
}

// Authorization 记录某条具体授权允许了某个具体请求，连同请求方给出的原因和证据。
type Authorization struct {
	action   AuthorizedAction
	grant    AuthorityGrant
	reason   StructuredReason
	evidence EvidenceReference
	at       time.Time
}

func (authorization Authorization) Action() AuthorizedAction {
	return authorization.action
}

func (authorization Authorization) GrantVersion() CommercialVersion {
	return authorization.grant.version
}

func (authorization Authorization) Reason() StructuredReason {
	return authorization.reason
}

func (authorization Authorization) Evidence() EvidenceReference {
	return authorization.evidence
}

func (authorization Authorization) At() time.Time {
	return authorization.at
}

// Authorize 寻找一条允许该请求的已生效授权。没有授权就是拒绝，绝不是放行：
// 授权要么显式授予，要么就是没有。
//
// 没通过分成两格，按调用方的恢复动作而不是本上下文观察到的原因（ADR-0029）：这个范围在
// 这个时刻一条已生效规则都没有时交回`未配置`，等租户把规则登记上；有规则而本请求不在其内
// 才是业务拒绝。
//
// 判据取**范围加时刻**，不取整份请求。责任法人、权限等级或动作任一不符，都说明权威已经就
// 这个范围表过态、只是没把这一项放进去——那是它给出的答案，不是它还没被问过。反过来，规则
// 全部过期与从未登记同落`未配置`：在请求那个时刻，这个范围都没有一条管得着的规则，两者要做
// 的事同为「让一条现行规则存在」。
func Authorize(grants []AuthorityGrant, request AuthorizationRequest) (Authorization, error) {
	configured := false
	for _, grant := range grants {
		if grant.scope == request.scope && grant.effective.Contains(request.at) {
			configured = true
		}
		if grant.permits(request) {
			return Authorization{
				action:   request.action,
				grant:    grant,
				reason:   request.reason,
				evidence: request.evidence,
				at:       request.at,
			}, nil
		}
	}
	if !configured {
		return Authorization{}, ErrAuthorityRulesNotConfigured
	}
	return Authorization{}, ErrNotAuthorized
}

// ManualReviewRequirementFor 回答某个范围是否要求人工复核。沉默意味着不要求：
// 本上下文规定人工复核不是默认步骤，因此没有声明绝不能被读成需要复核。
func ManualReviewRequirementFor(
	grants []AuthorityGrant,
	scope CommercialScopeReference,
	at time.Time,
) bool {
	for _, grant := range grants {
		if grant.action == ManualReviewAction &&
			grant.scope == scope &&
			grant.effective.Contains(at) {
			return true
		}
	}
	return false
}
