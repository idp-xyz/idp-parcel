package domain

import (
	"errors"
	"time"
)

// ErrInvalidCustomerParcelNumber 说明委托上的（声明包裹，客户包裹件号）配对立不起来：指着成员集合外的包裹，
// 或一件包裹给了两个号。它与 ErrInvalidDeclaredMeasurement 同格——客户输入的问题，编排答`输入未受理`。
var ErrInvalidCustomerParcelNumber = errors.New("parcel shipment: invalid customer parcel number")

// ExternalIdentifierNumber 是外部参与方分配的号码本身。逐字保存、逐字比对，不做大小写、空白或校验位折算：
// 条码规则属租户取值（UC-NO-002 明禁写默认条码规则；ADR-0184 决定五）。
type ExternalIdentifierNumber struct{ requiredValue }

func NewExternalIdentifierNumber(value string) (ExternalIdentifierNumber, error) {
	required, err := newRequiredValue("external identifier number", value)
	return ExternalIdentifierNumber{required}, err
}

// ExternalIdentifierKind 是标识类型的封闭集合。类型决定被标识对象的种类与分配方的种类（ADR-0184 决定二）：
// 「号码挂错对象」在形状上立不起来，CONTEXT「外部标识必须关联其真实标识对象」因此不靠调用方自觉。
//
// 今天只有有来源的类型进来。客户参考号（委托）与渠道订单号或主号（面单交易）等各自的来源与消费方。
type ExternalIdentifierKind uint8

const (
	ExternalIdentifierKindInvalid ExternalIdentifierKind = iota
	// CustomerParcelNumber 是客户在委托里为声明包裹给出的包裹件号：分配方是该委托的客户账户，标识包裹。
	CustomerParcelNumber
)

func (kind ExternalIdentifierKind) String() string {
	switch kind {
	case CustomerParcelNumber:
		return "CUSTOMER_PARCEL_NUMBER"
	default:
		return ""
	}
}

// ExternalIdentifierAllocator 是号码的分配方。它的种类由标识类型定：客户包裹件号的分配方是客户账户。
type ExternalIdentifierAllocator struct{ requiredValue }

// ExternalIdentifierScope 是号码在其中算数的分配方编号空间。首切与分配方同值，仍单列一格：委托内箱号、渠道
// 产品内号段这类更窄的范围日后是加列，不是改键（ADR-0184 决定六）。
type ExternalIdentifierScope struct{ requiredValue }

// ExternalIdentifierBasis 是关联的形成依据——客户包裹件号的那一条是接受决定。
type ExternalIdentifierBasis struct{ requiredValue }

// CustomerParcelNumberDeclaration 是委托上的一组（声明包裹，客户包裹件号）。它挂在委托上、首次提交时固定：
// 号码标识的是成员，新提交版本的成员集合又必须与当前版本相等，所以它不随版本重报（ADR-0184 决定四）。
type CustomerParcelNumberDeclaration struct {
	parcel DeclaredParcelID
	number ExternalIdentifierNumber
}

func DeclareCustomerParcelNumber(parcel DeclaredParcelID, number ExternalIdentifierNumber) (CustomerParcelNumberDeclaration, error) {
	if !parcel.valid() || !number.valid() {
		return CustomerParcelNumberDeclaration{}, ErrInvalidCustomerParcelNumber
	}
	return CustomerParcelNumberDeclaration{parcel: parcel, number: number}, nil
}

func (declaration CustomerParcelNumberDeclaration) Parcel() DeclaredParcelID {
	return declaration.parcel
}

func (declaration CustomerParcelNumberDeclaration) Number() ExternalIdentifierNumber {
	return declaration.number
}

// customerParcelNumbersFor 校验配对贴合成员集合并交回拷贝：每件至多一个号，指着成员集合外即拒。两件同号
// 不拒——节点会答身份冲突；在提交时就拒是一条没有 CONTEXT 依据的新门（ADR-0184 越权风险点 4）。
func customerParcelNumbersFor(
	declarations []CustomerParcelNumberDeclaration,
	members []DeclaredParcelID,
) ([]CustomerParcelNumberDeclaration, error) {
	if len(declarations) == 0 {
		return nil, nil
	}
	declared := make(map[DeclaredParcelID]struct{}, len(members))
	for _, member := range members {
		declared[member] = struct{}{}
	}
	seen := make(map[DeclaredParcelID]struct{}, len(declarations))
	copied := make([]CustomerParcelNumberDeclaration, 0, len(declarations))
	for _, declaration := range declarations {
		if !declaration.parcel.valid() || !declaration.number.valid() {
			return nil, ErrInvalidCustomerParcelNumber
		}
		if _, member := declared[declaration.parcel]; !member {
			return nil, ErrInvalidCustomerParcelNumber
		}
		if _, twice := seen[declaration.parcel]; twice {
			return nil, ErrInvalidCustomerParcelNumber
		}
		seen[declaration.parcel] = struct{}{}
		copied = append(copied, declaration)
	}
	return copied, nil
}

// ExternalIdentifierRelation 是一条外部标识关联（PS CONTEXT「外部标识关联」）。它是产物：只由形成它的
// 来源事实交出，没有公开构造器；登记册只插不改，适用状态的变化另起一行挂在它上面。
//
// 被标识对象今天只有包裹一种——有来源的两类号码都标识包裹。委托与面单交易两种对象随各自的类型进来。
type ExternalIdentifierRelation struct {
	tenant    TenantID
	kind      ExternalIdentifierKind
	allocator ExternalIdentifierAllocator
	scope     ExternalIdentifierScope
	number    ExternalIdentifierNumber
	parcel    DeclaredParcelID
	basis     ExternalIdentifierBasis
	formedAt  time.Time
}

func (relation ExternalIdentifierRelation) Tenant() TenantID {
	return relation.tenant
}

func (relation ExternalIdentifierRelation) Kind() ExternalIdentifierKind {
	return relation.kind
}

func (relation ExternalIdentifierRelation) Allocator() ExternalIdentifierAllocator {
	return relation.allocator
}

func (relation ExternalIdentifierRelation) Scope() ExternalIdentifierScope {
	return relation.scope
}

func (relation ExternalIdentifierRelation) Number() ExternalIdentifierNumber {
	return relation.number
}

func (relation ExternalIdentifierRelation) Parcel() DeclaredParcelID {
	return relation.parcel
}

func (relation ExternalIdentifierRelation) Basis() ExternalIdentifierBasis {
	return relation.basis
}

func (relation ExternalIdentifierRelation) FormedAt() time.Time {
	return relation.formedAt
}

// CustomerParcelNumbers 交回委托上的客户包裹件号配对（拷贝）。
func (request ShipmentRequest) CustomerParcelNumbers() []CustomerParcelNumberDeclaration {
	return append([]CustomerParcelNumberDeclaration(nil), request.customerParcelNumbers...)
}

// ExternalIdentifierRelationsFormedByAcceptance 交回这份委托随接受形成的外部标识关联：已接受时，接受基线里
// 每件给了号的包裹一条，分配方与适用范围都是来源身份上的客户账户，依据是接受决定，形成时刻是基线固定时刻；
// 未接受（已提交、已拒绝、已撤回）交空——接受前的声明包裹不是正式包裹（PS CONTEXT Rules）。
//
// identity 是这份委托的来源身份，客户账户取自它：委托聚合本身不带租户与客户账户，那两维在来源身份上。
func (request ShipmentRequest) ExternalIdentifierRelationsFormedByAcceptance(identity SourceIdentity) []ExternalIdentifierRelation {
	if request.state != ShipmentRequestAccepted || !identity.valid() {
		return nil
	}
	baseline, accepted := request.AcceptanceBaseline()
	decision, decided := request.AcceptanceDecision()
	if !accepted || !decided {
		return nil
	}
	customer := identity.CustomerAccountID().String()
	relations := make([]ExternalIdentifierRelation, 0, len(request.customerParcelNumbers))
	for _, declaration := range request.customerParcelNumbers {
		if !baseline.covers(declaration.parcel) {
			continue
		}
		relations = append(relations, ExternalIdentifierRelation{
			tenant:    identity.TenantID(),
			kind:      CustomerParcelNumber,
			allocator: ExternalIdentifierAllocator{requiredValue{value: customer}},
			scope:     ExternalIdentifierScope{requiredValue{value: customer}},
			number:    declaration.number,
			parcel:    declaration.parcel,
			basis:     ExternalIdentifierBasis{requiredValue{value: decision.DecisionID().String()}},
			formedAt:  baseline.FixedAt(),
		})
	}
	return relations
}
