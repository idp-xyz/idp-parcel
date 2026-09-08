package domain

import (
	"errors"
	"fmt"
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
	// ErrAuthorizationRequesterRequired 拦的是一次没带请求方的资料修订请求。资料修订的实际决定方
	// 只能从请求方解出（客户自己 / 代录时经委派解出委派方），不带请求方就答不了 UC-PS-002 要求的
	// 「实际决定方」——既有两格动作没有这条要求，所以只有资料修订在构造时就拒。
	ErrAuthorizationRequesterRequired = errors.New("party commercial: a source data amendment request must name its requester")
	// ErrDelegationAbsent 是 ErrNotAuthorized 的一个具名分格：授权规则在场、运营角色代客户请求，而该客户
	// 的合同没有把这一决定委派给它持的等级。它 Is ErrNotAuthorized——恢复动作同在业务侧（客户在合同里
	// 委派），不是租户去登记规则（ADR-0116 Decision 三，按 ADR-0029 的恢复动作分格）。
	ErrDelegationAbsent = fmt.Errorf("%w: no effective contract delegation hands this decision to the requesting operator role", ErrNotAuthorized)
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
// 复核并不等于获准直接拒掉这单业务；接受后客户原始资料的修订是第三个，同一条理由
// 逐字成立——三者互不蕴含、不得互相顶替（CONTEXT Rules，ADR-0116 Decision 一）。
// 撤回不在集内：一票一格，它归 `PAR-COM-14`「客户及其授权代表」那条线。
type AuthorizedAction uint8

const (
	AuthorizedActionInvalid AuthorizedAction = iota
	ManualReviewAction
	ActiveRejectionAction
	SourceDataAmendmentAction
)

func (action AuthorizedAction) valid() bool {
	return action >= ManualReviewAction && action <= SourceDataAmendmentAction
}

// decidedByCustomer 回答这个动作的决定权归不归客户。资料修订改的是客户已接受委托的原始资料，
// 决定权在客户：运营角色代录时必须经合同委派解出实际决定方，客户也只能委派自己拥有的决定。
// 人工复核与主动拒绝是运营侧凭授权规则自己作的决定，委派不参与。
func (action AuthorizedAction) decidedByCustomer() bool {
	return action == SourceDataAmendmentAction
}

func (action AuthorizedAction) String() string {
	switch action {
	case ManualReviewAction:
		return "MANUAL_REVIEW"
	case ActiveRejectionAction:
		return "ACTIVE_REJECTION"
	case SourceDataAmendmentAction:
		return "SOURCE_DATA_AMENDMENT"
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
//
// 请求方不是参与方角色：它是提出请求的**主体引用**（客户账户自己，或代某客户账户的运营
// 角色），只用来解出实际决定方，不带来任何授权（ADR-0116 Decision 三）。
type AuthorizationRequest struct {
	action      AuthorizedAction
	legalEntity LegalEntityReference
	level       AuthorityLevel
	scope       CommercialScopeReference
	reason      StructuredReason
	evidence    EvidenceReference
	at          time.Time
	requester   AuthorizationRequester
}

// NewAuthorizationRequest 要求每个动作都带结构化原因和证据。缺了它们的主动拒绝
// 事后无从辩护，而自由文本理由会让拒绝无法按原因统计。
//
// 它不带请求方，是三步法里保留的旧构造器：parcel-shipment 两只既有适配器的调用点迁到
// NewAuthorizationRequestBy 之后删。经它裁出的授权没有实际决定方；决定权归客户的动作
// 在这里就拒——没有请求方就解不出决定方，而那正是 UC-PS-002 要的那一格。
func NewAuthorizationRequest(
	action AuthorizedAction,
	legalEntity LegalEntityReference,
	level AuthorityLevel,
	scope CommercialScopeReference,
	reason StructuredReason,
	evidence EvidenceReference,
	at time.Time,
) (AuthorizationRequest, error) {
	if action.valid() && action.decidedByCustomer() {
		return AuthorizationRequest{}, ErrAuthorizationRequesterRequired
	}
	return newAuthorizationRequest(AuthorizationRequester{}, action, legalEntity, level, scope, reason, evidence, at)
}

// NewAuthorizationRequestBy 是带请求方的构造器：请求方由谁提出、代谁提出，与原因、证据
// 一样是请求自带的事实，不由裁定方补。
func NewAuthorizationRequestBy(
	requester AuthorizationRequester,
	action AuthorizedAction,
	legalEntity LegalEntityReference,
	level AuthorityLevel,
	scope CommercialScopeReference,
	reason StructuredReason,
	evidence EvidenceReference,
	at time.Time,
) (AuthorizationRequest, error) {
	if !requester.valid() {
		return AuthorizationRequest{}, ErrInvalidAuthorizationRequest
	}
	return newAuthorizationRequest(requester, action, legalEntity, level, scope, reason, evidence, at)
}

func newAuthorizationRequest(
	requester AuthorizationRequester,
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
		requester:   requester,
	}, nil
}

// Requester 报出提出请求的主体。第二个返回值为 false 即请求经旧构造器形成、没带请求方。
func (request AuthorizationRequest) Requester() (AuthorizationRequester, bool) {
	return request.requester, request.requester.valid()
}

// ResolvesDeciderByDelegation 回答这次请求的实际决定方要不要经合同委派解出：只有运营角色代客户
// 请求决定权归客户的动作才要。编排据此决定装不装载委派——不要的那几格多问一次库没有意义，
// 而要的那一格没有委派读口就答不出来。
func (request AuthorizationRequest) ResolvesDeciderByDelegation() bool {
	return request.requester.kind == OperatorRoleRequester && request.action.decidedByCustomer()
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

// Authorization 记录某条具体授权允许了某个具体请求，连同请求方给出的原因和证据，以及
// 解出的实际决定方（ADR-0116 Decision 三）。
type Authorization struct {
	action   AuthorizedAction
	grant    AuthorityGrant
	reason   StructuredReason
	evidence EvidenceReference
	at       time.Time
	decider  Decider
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

// Decider 报出这次裁定的实际决定方。第二个返回值为 false 即请求经旧构造器形成、没带请求方，
// 于是解不出决定方——那只可能发生在既有两格动作上，消费方不读它就与从前无异。
func (authorization Authorization) Decider() (Decider, bool) {
	return authorization.decider, authorization.decider.valid()
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
//
// 授权命中之后再解实际决定方（ADR-0116 Decision 三）。委派只回答「谁替谁」，顶替不了「许不许」：
// 规则缺席时不看委派照旧`未配置`；规则在场而运营角色代录的决定权归客户的动作没有有效委派，
// 是 ErrDelegationAbsent——它 Is ErrNotAuthorized，因为缺的是客户把决定权交出来，不是租户的规则。
//
// 「谁有权复核」同样走这里，带 ManualReviewAction，不另立一个按范围加时刻的谓词：那种谓词没有
// 主体、没有等级、没有三值，答不了「谁有权」，也不该答「要不要」——后者的单一权威是接单规则
// 正文的 ManualReviewDirective（ADR-0042），两问并格正是它要拦的事（UC-PC-003 第四项裁决）。
func Authorize(
	grants []AuthorityGrant,
	delegations []ContractDelegation,
	request AuthorizationRequest,
) (Authorization, error) {
	configured := false
	for _, grant := range grants {
		if grant.scope == request.scope && grant.effective.Contains(request.at) {
			configured = true
		}
		if grant.permits(request) {
			decider, err := resolveDecider(delegations, request)
			if err != nil {
				return Authorization{}, err
			}
			return Authorization{
				action:   request.action,
				grant:    grant,
				reason:   request.reason,
				evidence: request.evidence,
				at:       request.at,
				decider:  decider,
			}, nil
		}
	}
	if !configured {
		return Authorization{}, ErrAuthorityRulesNotConfigured
	}
	return Authorization{}, ErrNotAuthorized
}

// resolveDecider 按请求方解实际决定方：客户账户自己请求，决定方就是它；运营角色请求它自己
// 拥有的动作（人工复核、主动拒绝），决定方就是它；运营角色代客户请求决定权归客户的动作，
// 决定方是把这一决定委派给它所持等级的委派方——没有委派，登录操作人不能顶替。
// 没带请求方（旧构造器）解不出决定方，交回零值，由 Authorization.Decider 报为「未指名」。
func resolveDecider(delegations []ContractDelegation, request AuthorizationRequest) (Decider, error) {
	if !request.requester.valid() {
		return Decider{}, nil
	}
	if request.requester.kind == CustomerAccountRequester || !request.action.decidedByCustomer() {
		return request.requester.decider(), nil
	}
	for _, delegation := range delegations {
		if delegation.resolves(request) {
			return delegation.delegator.decider(), nil
		}
	}
	return Decider{}, ErrDelegationAbsent
}
