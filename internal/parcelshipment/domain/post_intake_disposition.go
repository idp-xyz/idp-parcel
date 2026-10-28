package domain

import (
	"errors"
	"fmt"
	"time"
)

// 本文件是收寄后服务处置决定（PS CONTEXT「收寄后服务处置决定」，ADR-0332）：包裹越过适用取消边界之后，有权主体针对继续、拦截、改送、退运、
// 服务终止形成的明确业务决定。它固定对象（包裹）、目的（去向）、决定来源、实际决定方、依据、生效时间和责任范围（被决定时在册的责任起点），
// 只追加：改主意是在同一包裹的决定链上追加后一份，后一份取代前一份为当前决定，前面的一字不动。
//
// 决定不等于执行，也不形成包裹终局：它只向各对象所有者提出范围化请求（OwnerRequests），各所有者独立承接或拒绝，承接与执行的事实归它们自己；
// 包裹没有「停止中」之类的总状态，委托汇总不读它，原旅程与原履约事实不因它倒退。

// ErrInvalidPostIntakeDisposition 说明一份收寄后服务处置决定立不起来：去向不在封闭集、来源与请求引用对不上、改送缺目的地（或别的去向带了目的地）、
// 决定人或依据缺席，或决定链的版本接不上。
var ErrInvalidPostIntakeDisposition = errors.New("parcel shipment: invalid post-intake disposition")

// PostIntakeDispositionDirection 是处置去向的封闭集合。
//
//   - 继续：不改变服务。它是对一次停止请求的明确答复，也是对此前拦截等决定的撤回——后一份决定取代前一份，所有者据此释放尚未消耗的请求。
//   - 拦截：在当前控制方处扣住，不再向前移动。
//   - 改送：同一收件关系内改去另一个交付去向（备用旅程）。换收件人、换客户账户、换产品属服务变更，形成关联新委托，不在这里。
//   - 退运：普通或商业退运，另起关联但独立的退运旅程。监管退运由监管处置决定经 UC-TF-001 承接，这里的退运不能冒充它。
//   - 服务终止：终止后续服务，释放尚未消耗的计划；实物去向另需一份退运等决定，终局服务结果在所有者的执行结果满足合同终局规则时才形成。
type PostIntakeDispositionDirection uint8

const (
	PostIntakeDispositionDirectionInvalid PostIntakeDispositionDirection = iota
	PostIntakeContinue
	PostIntakeIntercept
	PostIntakeRedirect
	PostIntakeReturn
	PostIntakeTerminateService
)

var postIntakeDispositionDirections = []PostIntakeDispositionDirection{
	PostIntakeContinue, PostIntakeIntercept, PostIntakeRedirect, PostIntakeReturn, PostIntakeTerminateService,
}

func (direction PostIntakeDispositionDirection) String() string {
	switch direction {
	case PostIntakeContinue:
		return "CONTINUE"
	case PostIntakeIntercept:
		return "INTERCEPT"
	case PostIntakeRedirect:
		return "REDIRECT"
	case PostIntakeReturn:
		return "RETURN"
	case PostIntakeTerminateService:
		return "TERMINATE_SERVICE"
	default:
		return ""
	}
}

// ParsePostIntakeDispositionDirection 按字面认去向，词不在封闭集内即拒，不猜。
func ParsePostIntakeDispositionDirection(raw string) (PostIntakeDispositionDirection, error) {
	for _, direction := range postIntakeDispositionDirections {
		if direction.String() == raw {
			return direction, nil
		}
	}
	return PostIntakeDispositionDirectionInvalid, fmt.Errorf("post-intake disposition direction %q is not a direction", raw)
}

// PostIntakeDispositionSource 是决定来源：回应一次客户的停止请求，或运营自己发起（例如异常案件、拒收信号之后的退运需求识别——信号本身不是决定，
// 决定仍要有权主体明确作出）。
type PostIntakeDispositionSource uint8

const (
	PostIntakeDispositionSourceInvalid PostIntakeDispositionSource = iota
	PostIntakeCustomerRequest
	PostIntakeOperatorInitiated
)

func (source PostIntakeDispositionSource) String() string {
	switch source {
	case PostIntakeCustomerRequest:
		return "CUSTOMER_REQUEST"
	case PostIntakeOperatorInitiated:
		return "OPERATOR_INITIATED"
	default:
		return ""
	}
}

// ParsePostIntakeDispositionSource 按字面认决定来源。
func ParsePostIntakeDispositionSource(raw string) (PostIntakeDispositionSource, error) {
	for _, source := range []PostIntakeDispositionSource{PostIntakeCustomerRequest, PostIntakeOperatorInitiated} {
		if source.String() == raw {
			return source, nil
		}
	}
	return PostIntakeDispositionSourceInvalid, fmt.Errorf("post-intake disposition source %q is not a source", raw)
}

// PostIntakeDispositionDeciderReference 是实际决定方的引用，取（发行方，sub）这一对：本上下文只记引用，不拥有操作者目录；有没有权决定由「运营决定」的
// 授予把守。
type PostIntakeDispositionDeciderReference struct{ requiredValue }

func NewPostIntakeDispositionDeciderReference(value string) (PostIntakeDispositionDeciderReference, error) {
	required, err := newRequiredValue("post-intake disposition decider reference", value)
	return PostIntakeDispositionDeciderReference{required}, err
}

// PostIntakeDispositionBasis 是决定依据：决定人核对了什么——产品、合同、客户授权、限制与可执行性的核对结论或证据引用。本上下文只要求它在场，不判够不够；
// 够不够由各所有者承接时各自判。
type PostIntakeDispositionBasis struct{ requiredValue }

func NewPostIntakeDispositionBasis(value string) (PostIntakeDispositionBasis, error) {
	required, err := newRequiredValue("post-intake disposition basis", value)
	return PostIntakeDispositionBasis{required}, err
}

// PostIntakeDispositionDestination 是改送的目的地引用，由收件关系内的交付去向所有者解读；本上下文只记引用。
type PostIntakeDispositionDestination struct{ requiredValue }

func NewPostIntakeDispositionDestination(value string) (PostIntakeDispositionDestination, error) {
	required, err := newRequiredValue("post-intake disposition destination", value)
	return PostIntakeDispositionDestination{required}, err
}

// DispositionOwner 是被请求的对象所有者。
type DispositionOwner uint8

const (
	DispositionOwnerInvalid DispositionOwner = iota
	DispositionOwnerNetworkRouting
	DispositionOwnerNodeOperations
	DispositionOwnerTransportFulfillment
)

func (owner DispositionOwner) String() string {
	switch owner {
	case DispositionOwnerNetworkRouting:
		return "NETWORK_ROUTING"
	case DispositionOwnerNodeOperations:
		return "NODE_OPERATIONS"
	case DispositionOwnerTransportFulfillment:
		return "TRANSPORT_FULFILLMENT"
	default:
		return ""
	}
}

// DispositionOwnerAction 是向所有者提出的请求动作。动作是请求，不是执行：所有者承接与否、怎么执行、执行结果都是它自己的事实。
type DispositionOwnerAction uint8

const (
	DispositionOwnerActionInvalid DispositionOwnerAction = iota
	// DispositionActionHoldAtControl：节点侧在当前控制方处扣住。
	DispositionActionHoldAtControl
	// DispositionActionHoldOnwardMovement：运输侧不再安排向前的移动。
	DispositionActionHoldOnwardMovement
	// DispositionActionReplanToAlternateDestination：路由侧按改送目的地重新规划。
	DispositionActionReplanToAlternateDestination
	// DispositionActionStartAlternateJourney：运输侧另起备用旅程（UC-TF-007，决定依据种类为服务处置决定）。
	DispositionActionStartAlternateJourney
	// DispositionActionPlanReturnRoute：路由侧规划退运路径。
	DispositionActionPlanReturnRoute
	// DispositionActionLoadForReturn：节点侧按退运装载并交出（UC-NO-003）。
	DispositionActionLoadForReturn
	// DispositionActionStartReturnJourney：运输侧另起退运旅程（UC-TF-007，决定依据种类为服务处置决定）。
	DispositionActionStartReturnJourney
	// DispositionActionReleaseUnconsumedPlan：路由侧释放尚未消耗的计划。
	DispositionActionReleaseUnconsumedPlan
)

func (action DispositionOwnerAction) String() string {
	switch action {
	case DispositionActionHoldAtControl:
		return "HOLD_AT_CONTROL"
	case DispositionActionHoldOnwardMovement:
		return "HOLD_ONWARD_MOVEMENT"
	case DispositionActionReplanToAlternateDestination:
		return "REPLAN_TO_ALTERNATE_DESTINATION"
	case DispositionActionStartAlternateJourney:
		return "START_ALTERNATE_JOURNEY"
	case DispositionActionPlanReturnRoute:
		return "PLAN_RETURN_ROUTE"
	case DispositionActionLoadForReturn:
		return "LOAD_FOR_RETURN"
	case DispositionActionStartReturnJourney:
		return "START_RETURN_JOURNEY"
	case DispositionActionReleaseUnconsumedPlan:
		return "RELEASE_UNCONSUMED_PLAN"
	default:
		return ""
	}
}

// DispositionOwnerRequest 是向一个所有者提出的一项范围化请求。范围就是决定的包裹与责任范围（被决定时在册的责任起点）。
type DispositionOwnerRequest struct {
	Owner  DispositionOwner
	Action DispositionOwnerAction
}

// OwnerRequests 是这个去向向各对象所有者提出的请求，封闭表：继续不请求任何所有者；结算不在其中——处置决定不是费用依据，费用只随所有者自己的
// 执行事实形成。没有一个去向直接改写所有者的对象，也没有一个去向倒退原旅程。
func (direction PostIntakeDispositionDirection) OwnerRequests() []DispositionOwnerRequest {
	switch direction {
	case PostIntakeIntercept:
		return []DispositionOwnerRequest{
			{DispositionOwnerNodeOperations, DispositionActionHoldAtControl},
			{DispositionOwnerTransportFulfillment, DispositionActionHoldOnwardMovement},
		}
	case PostIntakeRedirect:
		return []DispositionOwnerRequest{
			{DispositionOwnerNetworkRouting, DispositionActionReplanToAlternateDestination},
			{DispositionOwnerTransportFulfillment, DispositionActionStartAlternateJourney},
		}
	case PostIntakeReturn:
		return []DispositionOwnerRequest{
			{DispositionOwnerNetworkRouting, DispositionActionPlanReturnRoute},
			{DispositionOwnerNodeOperations, DispositionActionLoadForReturn},
			{DispositionOwnerTransportFulfillment, DispositionActionStartReturnJourney},
		}
	case PostIntakeTerminateService:
		return []DispositionOwnerRequest{
			{DispositionOwnerNetworkRouting, DispositionActionReleaseUnconsumedPlan},
			{DispositionOwnerNodeOperations, DispositionActionHoldAtControl},
			{DispositionOwnerTransportFulfillment, DispositionActionHoldOnwardMovement},
		}
	default:
		return nil
	}
}

// OriginalJourneyRelation 是决定与原旅程的关系。改送与退运另起关联但独立的旅程：新旅程指向原旅程，原旅程的中断、提前终止或继续由原所有者按事实
// 自行形成，不被倒退、不被覆盖；其余去向不另起旅程，原旅程不变。
type OriginalJourneyRelation uint8

const (
	OriginalJourneyRelationInvalid OriginalJourneyRelation = iota
	OriginalJourneyUnchanged
	OriginalJourneyAssociatedIndependent
)

func (relation OriginalJourneyRelation) String() string {
	switch relation {
	case OriginalJourneyUnchanged:
		return "UNCHANGED"
	case OriginalJourneyAssociatedIndependent:
		return "ASSOCIATED_INDEPENDENT"
	default:
		return ""
	}
}

// OriginalJourneyRelation 见类型注释。
func (direction PostIntakeDispositionDirection) OriginalJourneyRelation() OriginalJourneyRelation {
	switch direction {
	case PostIntakeRedirect, PostIntakeReturn:
		return OriginalJourneyAssociatedIndependent
	case PostIntakeContinue, PostIntakeIntercept, PostIntakeTerminateService:
		return OriginalJourneyUnchanged
	default:
		return OriginalJourneyRelationInvalid
	}
}

// ServiceDispositionBasisKind 是向运输履约提出另起旅程时的决定依据种类：本上下文的处置决定一律是普通/商业处置决定，从不是监管处置决定——监管退运的
// 依据由关务协作形成、经 UC-TF-001 承接，这里的退运不能用它冒充，反过来也不能用监管依据冒充本决定。
const ServiceDispositionBasisKind = "SERVICE_DISPOSITION"

// PostIntakeDispositionShapeError 核对去向与目的地的搭配：改送必须带目的地，别的去向不带。接入面与编排共用，目的地的搭配不是接入面一家的事。
func PostIntakeDispositionShapeError(direction PostIntakeDispositionDirection, destinationPresent bool) error {
	if direction.String() == "" {
		return fmt.Errorf("%w: direction is not a direction", ErrInvalidPostIntakeDisposition)
	}
	if direction == PostIntakeRedirect && !destinationPresent {
		return fmt.Errorf("%w: a redirect names its destination", ErrInvalidPostIntakeDisposition)
	}
	if direction != PostIntakeRedirect && destinationPresent {
		return fmt.Errorf("%w: only a redirect carries a destination", ErrInvalidPostIntakeDisposition)
	}
	return nil
}

// PostIntakeDispositionSpec 是形成一份决定所需的全部输入。
//
// Version 是决定链上的版本，从 1 起；Supersedes 是它取代的前一份的版本，首份为 0，其余恰好是 Version-1——链是线性的，不分叉。RequestKey 在决定来源是
// 客户请求时指名它回应的那份请求，运营发起时必须为空。ResponsibilityStart 是决定形成时在册的责任起点版本，即决定的责任范围。
type PostIntakeDispositionSpec struct {
	Tenant              TenantID
	Parcel              DeclaredParcelID
	Version             int
	Supersedes          int
	Direction           PostIntakeDispositionDirection
	Source              PostIntakeDispositionSource
	RequestKey          SourceRequestKey
	Destination         PostIntakeDispositionDestination
	ResponsibilityStart SourceResultVersion
	Decider             PostIntakeDispositionDeciderReference
	Basis               PostIntakeDispositionBasis
	DecidedAt           time.Time
}

// PostIntakeDisposition 是一份已形成的收寄后服务处置决定。
type PostIntakeDisposition struct {
	spec PostIntakeDispositionSpec
}

// DecidePostIntakeDisposition 形成一份决定，缺件、搭配不对或链接不上都拒。
func DecidePostIntakeDisposition(spec PostIntakeDispositionSpec) (PostIntakeDisposition, error) {
	if !spec.Tenant.valid() || !spec.Parcel.valid() || !spec.ResponsibilityStart.valid() ||
		!spec.Decider.valid() || !spec.Basis.valid() || spec.DecidedAt.IsZero() {
		return PostIntakeDisposition{}, fmt.Errorf("%w: tenant, parcel, responsibility start, decider, basis and time are required", ErrInvalidPostIntakeDisposition)
	}
	if err := PostIntakeDispositionShapeError(spec.Direction, spec.Destination.valid()); err != nil {
		return PostIntakeDisposition{}, err
	}
	if spec.Version < 1 || spec.Supersedes != spec.Version-1 {
		return PostIntakeDisposition{}, fmt.Errorf("%w: version %d does not follow predecessor %d", ErrInvalidPostIntakeDisposition, spec.Version, spec.Supersedes)
	}
	switch spec.Source {
	case PostIntakeCustomerRequest:
		if !spec.RequestKey.valid() {
			return PostIntakeDisposition{}, fmt.Errorf("%w: a decision answering a customer request names the request", ErrInvalidPostIntakeDisposition)
		}
	case PostIntakeOperatorInitiated:
		if spec.RequestKey.valid() {
			return PostIntakeDisposition{}, fmt.Errorf("%w: an operator-initiated decision answers no request", ErrInvalidPostIntakeDisposition)
		}
	default:
		return PostIntakeDisposition{}, fmt.Errorf("%w: source is not a source", ErrInvalidPostIntakeDisposition)
	}
	spec.DecidedAt = spec.DecidedAt.UTC()
	return PostIntakeDisposition{spec: spec}, nil
}

func (disposition PostIntakeDisposition) Tenant() TenantID { return disposition.spec.Tenant }
func (disposition PostIntakeDisposition) Parcel() DeclaredParcelID {
	return disposition.spec.Parcel
}
func (disposition PostIntakeDisposition) Version() int    { return disposition.spec.Version }
func (disposition PostIntakeDisposition) Supersedes() int { return disposition.spec.Supersedes }
func (disposition PostIntakeDisposition) Direction() PostIntakeDispositionDirection {
	return disposition.spec.Direction
}
func (disposition PostIntakeDisposition) Source() PostIntakeDispositionSource {
	return disposition.spec.Source
}

// RequestKey 在决定来源是客户请求时是它回应的那份请求，否则为零值。
func (disposition PostIntakeDisposition) RequestKey() SourceRequestKey {
	return disposition.spec.RequestKey
}

// Destination 只在改送上有值。
func (disposition PostIntakeDisposition) Destination() PostIntakeDispositionDestination {
	return disposition.spec.Destination
}
func (disposition PostIntakeDisposition) ResponsibilityStart() SourceResultVersion {
	return disposition.spec.ResponsibilityStart
}
func (disposition PostIntakeDisposition) Decider() PostIntakeDispositionDeciderReference {
	return disposition.spec.Decider
}
func (disposition PostIntakeDisposition) Basis() PostIntakeDispositionBasis {
	return disposition.spec.Basis
}

// DecidedAt 同时是生效时间：决定一经追加即生效，没有另行约定的生效时刻。
func (disposition PostIntakeDisposition) DecidedAt() time.Time { return disposition.spec.DecidedAt }

// SameContentAs 问两份决定说的是不是同一件事：去向、回应的请求、目的地、依据和决定方都相同。同一个人带着同样的内容重发同一条命令，要读回原决定而不是
// 再追加一份；内容不同就是另一份决定。
func (disposition PostIntakeDisposition) SameContentAs(other PostIntakeDisposition) bool {
	return disposition.spec.Direction == other.spec.Direction &&
		disposition.spec.Source == other.spec.Source &&
		disposition.spec.RequestKey == other.spec.RequestKey &&
		disposition.spec.Destination == other.spec.Destination &&
		disposition.spec.Basis == other.spec.Basis &&
		disposition.spec.Decider == other.spec.Decider
}

// BasisReference 是交给运输履约等所有者的「决定依据引用」：指名这份决定链上的这一版，所有者据此把它记作自己另起旅程的依据（UC-TF-007）。
func (disposition PostIntakeDisposition) BasisReference() string {
	return fmt.Sprintf("POST_INTAKE_DISPOSITION/%s/%d", disposition.spec.Parcel.String(), disposition.spec.Version)
}
