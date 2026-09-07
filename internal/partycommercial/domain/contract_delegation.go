package domain

import (
	"errors"
	"sort"
)

var (
	ErrInvalidAuthorizationRequester = errors.New("party commercial: invalid authorization requester")
	ErrInvalidDelegator              = errors.New("party commercial: invalid delegator")
	ErrInvalidContractDelegation     = errors.New("party commercial: invalid contract delegation")
	// ErrDuplicateContractDelegation 拒绝同一合同版本内第二条同键（动作 × 范围 × 等级）的委派：两条
	// 会让「这一等级替谁决定」有两个答案。与「立不住」分格，恢复动作不同——去掉多出的那一条。
	ErrDuplicateContractDelegation = errors.New("party commercial: duplicate contract delegation")
)

// OperatorRoleReference 引用代客户提出请求的运营侧主体。本上下文不存操作者、也不存操作者
// 持哪一等级（ADR-0100 Decision 二），只携带这个引用以记下「谁提出了请求」；它永远不是
// 资料修订的实际决定方——UC-PS-002「登录操作人不能替代实际决定方」。
type OperatorRoleReference struct{ requiredValue }

func NewOperatorRoleReference(value string) (OperatorRoleReference, error) {
	required, err := newRequiredValue("operator role reference", value)
	return OperatorRoleReference{required}, err
}

// RequesterKind 是请求方的种类：客户账户自己，或代客户账户提出请求的运营角色（BD-PS-009
// 「客户可提交请求，授权运营角色可代录」）。
type RequesterKind uint8

const (
	RequesterKindInvalid RequesterKind = iota
	CustomerAccountRequester
	OperatorRoleRequester
)

func (kind RequesterKind) String() string {
	switch kind {
	case CustomerAccountRequester:
		return "CUSTOMER_ACCOUNT"
	case OperatorRoleRequester:
		return "OPERATOR_ROLE"
	default:
		return ""
	}
}

// AuthorizationRequester 是提出授权请求的主体。它不是 AuthorizationRequest 头注禁的「参与方
// 角色」——那句禁的是拿承运商代理商之类的关系角色换授权；请求方是主体引用，只用来解出实际
// 决定方，授权仍只来自版本化授权规则加合同委派（ADR-0116 Decision 三）。
//
// 两个构造器分立而不是一个带可空字段：运营角色代录时必须说清代的是哪个客户账户，可空字段
// 分不清「客户自己来的」与「忘了填所代客户」。
type AuthorizationRequester struct {
	kind     RequesterKind
	account  CustomerAccountID
	operator OperatorRoleReference
}

// RequestedByCustomerAccount 是客户账户自己提出的请求。
func RequestedByCustomerAccount(account CustomerAccountID) (AuthorizationRequester, error) {
	if !account.valid() {
		return AuthorizationRequester{}, ErrInvalidAuthorizationRequester
	}
	return AuthorizationRequester{kind: CustomerAccountRequester, account: account}, nil
}

// RequestedByOperatorRole 是运营角色代某个客户账户提出的请求。
func RequestedByOperatorRole(operator OperatorRoleReference, onBehalfOf CustomerAccountID) (AuthorizationRequester, error) {
	if !operator.valid() || !onBehalfOf.valid() {
		return AuthorizationRequester{}, ErrInvalidAuthorizationRequester
	}
	return AuthorizationRequester{kind: OperatorRoleRequester, account: onBehalfOf, operator: operator}, nil
}

func (requester AuthorizationRequester) Kind() RequesterKind {
	return requester.kind
}

// CustomerAccount 是请求所关乎的客户账户：客户自己请求时是它自己，运营角色代录时是所代的客户。
func (requester AuthorizationRequester) CustomerAccount() CustomerAccountID {
	return requester.account
}

// OperatorRole 报出代录的运营角色；客户自己请求时第二个返回值为 false。
func (requester AuthorizationRequester) OperatorRole() (OperatorRoleReference, bool) {
	return requester.operator, requester.kind == OperatorRoleRequester
}

func (requester AuthorizationRequester) valid() bool {
	switch requester.kind {
	case CustomerAccountRequester:
		return requester.account.valid()
	case OperatorRoleRequester:
		return requester.account.valid() && requester.operator.valid()
	default:
		return false
	}
}

// decider 是「决定方 = 请求方」那一格的转写：客户自己请求即客户账户，运营角色请求它自己
// 拥有的动作即运营角色。
func (requester AuthorizationRequester) decider() Decider {
	switch requester.kind {
	case CustomerAccountRequester:
		return Decider{kind: CustomerAccountDecider, reference: requester.account.String()}
	case OperatorRoleRequester:
		return Decider{kind: OperatorRoleDecider, reference: requester.operator.String()}
	default:
		return Decider{}
	}
}

// DelegatorKind 是委派方的种类：该合同的货主客户账户，或其责任法人（ADR-0116 Decision 二）。
type DelegatorKind uint8

const (
	DelegatorKindInvalid DelegatorKind = iota
	CustomerAccountDelegator
	LegalEntityDelegator
)

func (kind DelegatorKind) String() string {
	switch kind {
	case CustomerAccountDelegator:
		return "CUSTOMER_ACCOUNT"
	case LegalEntityDelegator:
		return "LEGAL_ENTITY"
	default:
		return ""
	}
}

// Delegator 是合同委派的委派方——把某一决定权交出去的那一方，恰一：客户账户或责任法人。
// 两个构造器分立而不用可空字段：可空字段分不清「法人委派」与「忘了填客户账户」。
type Delegator struct {
	kind        DelegatorKind
	account     CustomerAccountID
	legalEntity LegalEntityReference
}

func DelegatedByCustomerAccount(account CustomerAccountID) (Delegator, error) {
	if !account.valid() {
		return Delegator{}, ErrInvalidDelegator
	}
	return Delegator{kind: CustomerAccountDelegator, account: account}, nil
}

func DelegatedByLegalEntity(entity LegalEntityReference) (Delegator, error) {
	if !entity.valid() {
		return Delegator{}, ErrInvalidDelegator
	}
	return Delegator{kind: LegalEntityDelegator, legalEntity: entity}, nil
}

func (delegator Delegator) Kind() DelegatorKind {
	return delegator.kind
}

// Reference 是委派方的引用串，按种类取自客户账户或责任法人——持久化面与批文只认一个串加一个种类。
func (delegator Delegator) Reference() string {
	switch delegator.kind {
	case CustomerAccountDelegator:
		return delegator.account.String()
	case LegalEntityDelegator:
		return delegator.legalEntity.String()
	default:
		return ""
	}
}

func (delegator Delegator) CustomerAccount() (CustomerAccountID, bool) {
	return delegator.account, delegator.kind == CustomerAccountDelegator
}

func (delegator Delegator) LegalEntity() (LegalEntityReference, bool) {
	return delegator.legalEntity, delegator.kind == LegalEntityDelegator
}

func (delegator Delegator) valid() bool {
	switch delegator.kind {
	case CustomerAccountDelegator:
		return delegator.account.valid()
	case LegalEntityDelegator:
		return delegator.legalEntity.valid()
	default:
		return false
	}
}

func (delegator Delegator) decider() Decider {
	switch delegator.kind {
	case CustomerAccountDelegator:
		return Decider{kind: CustomerAccountDecider, reference: delegator.account.String()}
	case LegalEntityDelegator:
		return Decider{kind: LegalEntityDecider, reference: delegator.legalEntity.String()}
	default:
		return Decider{}
	}
}

// DeciderKind 是实际决定方的种类。比委派方多一格「运营角色」：人工复核与主动拒绝是运营角色
// 凭授权规则自己作的决定，那两格的决定方就是提出请求的运营角色；资料修订的决定方只会是客户
// 账户或责任法人。
type DeciderKind uint8

const (
	DeciderKindInvalid DeciderKind = iota
	CustomerAccountDecider
	LegalEntityDecider
	OperatorRoleDecider
)

func (kind DeciderKind) String() string {
	switch kind {
	case CustomerAccountDecider:
		return "CUSTOMER_ACCOUNT"
	case LegalEntityDecider:
		return "LEGAL_ENTITY"
	case OperatorRoleDecider:
		return "OPERATOR_ROLE"
	default:
		return ""
	}
}

// Decider 是一次裁定的实际决定方（UC-PS-002 步骤 4，ADR-0116 Decision 三）。它只由本包在
// Authorize 里解出，没有公开构造器：实际决定方是裁定的产物，不是调用方能声明的输入——
// PS 端口头注「不由调用方声明」在这里是结构性的。
type Decider struct {
	kind      DeciderKind
	reference string
}

func (decider Decider) Kind() DeciderKind {
	return decider.kind
}

// Reference 是决定方的引用串：客户账户标识、责任法人引用或运营角色引用，按 Kind 读。
func (decider Decider) Reference() string {
	return decider.reference
}

func (decider Decider) valid() bool {
	return decider.kind != DeciderKindInvalid && decider.reference != ""
}

// ContractDelegation 是一条合同委派：某个客户合同版本声明，委派方把某一授权动作在某一商业范围
// 内的实际决定权，在有效期间内委派给持某一商业权限等级的运营角色（CONTEXT「合同委派」词条，
// ADR-0116 Decision 二）。
//
// 受托方是权限等级而不是操作者主体：本上下文只说「这一范围的资料修订委派给持这一等级的运营
// 角色」，某个操作者持哪一等级是 accessidentity 授予那一格的事（ADR-0100 Decision 二）。
// 不按资料组细分：委派答「谁能替谁提」，允许矩阵（票 pc-gaps/10）答「这一格能不能改」。
type ContractDelegation struct {
	contract  CommercialVersion
	delegator Delegator
	action    AuthorizedAction
	scope     CommercialScopeReference
	level     AuthorityLevel
	effective EffectiveInterval
}

// NewContractDelegation 要求拥有对象是当前可用的客户合同版本、动作是决定权归客户的那一格。
// 挂错类别的版本仍是合法的商业版本，入册与被选中都不报错，错要到运营角色代录时解不出决定方
// 才显形——那时它长得像「客户没委派」。人工复核与主动拒绝不是客户的决定，客户委派不了它们。
func NewContractDelegation(
	contract CommercialVersion,
	delegator Delegator,
	action AuthorizedAction,
	scope CommercialScopeReference,
	level AuthorityLevel,
	effective EffectiveInterval,
) (ContractDelegation, error) {
	if contract.kind != CustomerContractObject ||
		contract.status != CommercialVersionEffective ||
		!delegator.valid() ||
		!action.valid() || !action.decidedByCustomer() ||
		!scope.valid() || !level.valid() || !effective.valid() {
		return ContractDelegation{}, ErrInvalidContractDelegation
	}
	return ContractDelegation{
		contract:  contract,
		delegator: delegator,
		action:    action,
		scope:     scope,
		level:     level,
		effective: effective,
	}, nil
}

func (delegation ContractDelegation) Contract() CommercialVersion {
	return delegation.contract
}

func (delegation ContractDelegation) Delegator() Delegator {
	return delegation.delegator
}

func (delegation ContractDelegation) Action() AuthorizedAction {
	return delegation.action
}

func (delegation ContractDelegation) Scope() CommercialScopeReference {
	return delegation.scope
}

func (delegation ContractDelegation) Level() AuthorityLevel {
	return delegation.level
}

func (delegation ContractDelegation) Effective() EffectiveInterval {
	return delegation.effective
}

// resolves 回答这条委派是否把该请求的决定权交给了请求方所持的等级：动作、范围、等级、时点
// 五维之外，委派方还要对得上——客户账户委派对所代的客户账户，责任法人委派对请求上的责任法人。
func (delegation ContractDelegation) resolves(request AuthorizationRequest) bool {
	if delegation.action != request.action ||
		delegation.scope != request.scope ||
		delegation.level != request.level ||
		!delegation.effective.Contains(request.at) {
		return false
	}
	switch delegation.delegator.kind {
	case CustomerAccountDelegator:
		return delegation.delegator.account == request.requester.account
	case LegalEntityDelegator:
		return delegation.delegator.legalEntity == request.legalEntity
	default:
		return false
	}
}

// ContractDelegationDeclaration 是一条委派的声明输入：合同版本由声明所属的发布给出，不在这里重复。
type ContractDelegationDeclaration struct {
	Delegator Delegator
	Action    AuthorizedAction
	Scope     CommercialScopeReference
	Level     AuthorityLevel
	Effective EffectiveInterval
}

// ContractDelegationContent 是一个客户合同版本声明的全部委派——随合同版本发布登记的那一份正文
// （「声明只能随发布」），更正走新合同版本，不开行级改写。
type ContractDelegationContent struct {
	contract    CommercialVersion
	delegations []ContractDelegation
}

// NewContractDelegationContent 组装一个合同版本的委派声明：至少一条（一条都没有的声明什么都没说，
// 不登记）；同一（动作 × 范围 × 等级）在版本内只许一条——两条会让「这一等级替谁决定」有两个答案。
func NewContractDelegationContent(
	contract CommercialVersion,
	declarations []ContractDelegationDeclaration,
) (ContractDelegationContent, error) {
	if len(declarations) == 0 {
		return ContractDelegationContent{}, ErrInvalidContractDelegation
	}
	type delegationKey struct {
		action AuthorizedAction
		scope  CommercialScopeReference
		level  AuthorityLevel
	}
	seen := make(map[delegationKey]struct{}, len(declarations))
	delegations := make([]ContractDelegation, 0, len(declarations))
	for _, declaration := range declarations {
		delegation, err := NewContractDelegation(
			contract, declaration.Delegator, declaration.Action, declaration.Scope, declaration.Level, declaration.Effective)
		if err != nil {
			return ContractDelegationContent{}, err
		}
		key := delegationKey{action: delegation.action, scope: delegation.scope, level: delegation.level}
		if _, duplicate := seen[key]; duplicate {
			return ContractDelegationContent{}, ErrDuplicateContractDelegation
		}
		seen[key] = struct{}{}
		delegations = append(delegations, delegation)
	}
	sortContractDelegations(delegations)
	return ContractDelegationContent{contract: contract, delegations: delegations}, nil
}

func (content ContractDelegationContent) Contract() CommercialVersion {
	return content.contract
}

// Delegations 按（动作、范围、等级）的稳定顺序交回全部委派（副本）。
func (content ContractDelegationContent) Delegations() []ContractDelegation {
	return append([]ContractDelegation(nil), content.delegations...)
}

func sortContractDelegations(delegations []ContractDelegation) {
	sort.Slice(delegations, func(left, right int) bool {
		if delegations[left].action != delegations[right].action {
			return delegations[left].action < delegations[right].action
		}
		if delegations[left].scope != delegations[right].scope {
			return delegations[left].scope.String() < delegations[right].scope.String()
		}
		return delegations[left].level.String() < delegations[right].level.String()
	})
}
