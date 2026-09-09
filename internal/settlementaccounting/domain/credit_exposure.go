package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidCreditStanding  = errors.New("settlement accounting: invalid credit standing")
	ErrInvalidExposureRequest = errors.New("settlement accounting: invalid exposure request")
	ErrExposureNotFound       = errors.New("settlement accounting: credit exposure not found")
	// ErrStandingNotAuthorized 拒绝拿一份没换上授权额度的登记状况去判暴露：登记状况里的 limit 只是
	// 一列登记值（ADR-0127 决定四），据它占额度就是拿本上下文自己的数当政策额度，且形成的暴露答不出
	// 出处。它是编程错误不是业务答案——编排少调了一步 WithAuthorizedLimit。
	ErrStandingNotAuthorized = errors.New("settlement accounting: credit standing carries no authorized limit")
)

// CreditStanding 是一个账期作用域当前的信用状况快照：当前有效额度、已占用暴露与是否
// 逾期。它与运营余额分开取——SET-03 明写预付冻结不与同一客户账期范围共用余额、额度，
// 合成一个对象会让两本账在实现里合流（ADR-0047）。
//
// policy 是当前有效额度的出处：登记状况自己没有（零值），经 WithAuthorizedLimit 换上商业侧
// 授权额度时随额度一起进来。额度与出处绑在同一份快照上，暴露据它判额度时才答得出「按哪一版
// 信用政策判的」——CONTEXT 要每项信用暴露保存实际采用的政策，这一格就是它的来路。
type CreditStanding struct {
	scope        SettlementScope
	limitMinor   int64
	exposedMinor int64
	overdue      bool
	policy       CreditPolicyReference
}

func NewCreditStanding(
	scope SettlementScope,
	limitMinor, exposedMinor int64,
	overdue bool,
) (CreditStanding, error) {
	if !scope.valid() || limitMinor < 0 || exposedMinor < 0 {
		return CreditStanding{}, ErrInvalidCreditStanding
	}
	return CreditStanding{
		scope:        scope,
		limitMinor:   limitMinor,
		exposedMinor: exposedMinor,
		overdue:      overdue,
	}, nil
}

func (standing CreditStanding) Scope() SettlementScope {
	return standing.scope
}

// WithAuthorizedLimit 交回一份把当前有效额度换成商业侧授权额度的副本（ADR-0127）：额度出自
// 闭包交出的信用政策，已占用暴露与逾期仍是本上下文自己的账本与状况登记——两边各拥有自己
// 那一半，合在这一份快照上判。原值不改：状况快照是读回来的事实，不就地改写。
//
// 额度必须带着出处进来：没有政策出处的授权额度就是本上下文自己发明的额度（与 CreditBasis
// 构造门同一条理由），拒在这里而不是等落库时发现政策列为空。
func (standing CreditStanding) WithAuthorizedLimit(limitMinor int64, policy CreditPolicyReference) (CreditStanding, error) {
	if !standing.scope.valid() || limitMinor < 0 || policy.String() == "" {
		return CreditStanding{}, ErrInvalidCreditStanding
	}
	standing.limitMinor = limitMinor
	standing.policy = policy
	return standing, nil
}

// LimitMinor 是本作用域当前有效的授信额度。
func (standing CreditStanding) LimitMinor() int64 {
	return standing.limitMinor
}

// Policy 是当前有效额度出自哪一版信用政策；登记状况没有换上授权额度时为零值。
func (standing CreditStanding) Policy() CreditPolicyReference {
	return standing.policy
}

// Headroom 是本作用域还可占用的额度。
func (standing CreditStanding) Headroom() int64 {
	return standing.limitMinor - standing.exposedMinor
}

func (standing CreditStanding) Overdue() bool {
	return standing.overdue
}

// ExposureRequest 与 FreezeRequest 形状相同但刻意分立：冻结占资金、暴露占额度，两者的
// 请求进不同的账本。零金额暴露与零金额冻结同罪——一次什么也没占用、看上去却执行过的
// 控制。
type ExposureRequest struct {
	requestID   ControlRequestID
	scope       SettlementScope
	amountMinor int64
	association BusinessAssociationReference
	requestedAt time.Time
}

func NewExposureRequest(
	requestID ControlRequestID,
	scope SettlementScope,
	amountMinor int64,
	association BusinessAssociationReference,
	requestedAt time.Time,
) (ExposureRequest, error) {
	if !requestID.valid() || !scope.valid() || amountMinor <= 0 ||
		!association.valid() || requestedAt.IsZero() {
		return ExposureRequest{}, ErrInvalidExposureRequest
	}
	return ExposureRequest{
		requestID:   requestID,
		scope:       scope,
		amountMinor: amountMinor,
		association: association,
		requestedAt: requestedAt.UTC(),
	}, nil
}

func (request ExposureRequest) RequestID() ControlRequestID {
	return request.requestID
}

func (request ExposureRequest) AmountMinor() int64 {
	return request.amountMinor
}

type ExposureID struct{ requiredValue }

// ExposureStatus 是信用暴露可能结果的封闭集合。没有一个是接受判决：超额与逾期是本
// 上下文报告的业务限制依据，是否据此阻断委托由 `parcel-shipment` 决定（CONTEXT「余额
// 不足或逾期只向订单接受等责任上下文提供信用暴露和业务限制依据」）。
type ExposureStatus uint8

const (
	ExposureStatusInvalid ExposureStatus = iota
	ExposureRecorded
	ExposureReleased
	ExposureRestricted
)

func (status ExposureStatus) String() string {
	switch status {
	case ExposureRecorded:
		return "RECORDED"
	case ExposureReleased:
		return "RELEASED"
	case ExposureRestricted:
		return "RESTRICTED"
	default:
		return ""
	}
}

// CreditExposure 是账期方式下一次接受前控制留下的暴露记录。它不是费用也不是应收：
// 金额责任由费用对象拥有，暴露只回答「这次控制占了多少额度」，以及按哪一版信用政策的
// 额度判的（policy，CONTEXT「每项信用暴露保存实际采用的政策」的信用政策那一份）。
type CreditExposure struct {
	exposureID  ExposureID
	requestID   ControlRequestID
	scope       SettlementScope
	amountMinor int64
	association BusinessAssociationReference
	status      ExposureStatus
	exposedAt   time.Time
	releasedAt  time.Time
	reason      RestrictionReason
	policy      CreditPolicyReference
}

func (exposure CreditExposure) ExposureID() ExposureID {
	return exposure.exposureID
}

func (exposure CreditExposure) RequestID() ControlRequestID {
	return exposure.requestID
}

func (exposure CreditExposure) AmountMinor() int64 {
	return exposure.amountMinor
}

func (exposure CreditExposure) Status() ExposureStatus {
	return exposure.status
}

func (exposure CreditExposure) ExposedAt() time.Time {
	return exposure.exposedAt
}

func (exposure CreditExposure) ReleasedAt() time.Time {
	return exposure.releasedAt
}

func (exposure CreditExposure) Reason() RestrictionReason {
	return exposure.reason
}

// Policy 是这次暴露据以判额度的信用政策版本。它取自形成时那份状况快照，重放交回原暴露时
// 随原暴露走。只有政策引用列落地之前入库的存量行读回为零值——新形成的暴露一律有出处：Expose
// 不收没换上授权额度的状况，而授权额度进不了没有出处的 WithAuthorizedLimit。
func (exposure CreditExposure) Policy() CreditPolicyReference {
	return exposure.policy
}

// CreditExposureLedger 只增不删地记录信用暴露，代数与冻结账本一致（幂等重放、同身份
// 异内容冲突、显式释放）——但它是另一本账：占的是额度不是资金，两本互不借用。
type CreditExposureLedger struct {
	byRequest  map[ControlRequestID]ExposureID
	byExposure map[ExposureID]CreditExposure
	digests    map[ControlRequestID]string
	nextID     int
}

func NewCreditExposureLedger() *CreditExposureLedger {
	return &CreditExposureLedger{
		byRequest:  make(map[ControlRequestID]ExposureID),
		byExposure: make(map[ExposureID]CreditExposure),
		digests:    make(map[ControlRequestID]string),
	}
}

// Expose 为一次账期控制请求占用额度。逾期先于额度判：账户已逾期时这个作用域整体不该
// 再扩大暴露，答案与本笔金额无关；额度不足与余额不足同理是业务答案不是错误。`业务限制`
// 的记录不入账本——那次控制没有占用额度，也就没有可释放的东西（与冻结账本同一条纪律）。
//
// 状况必须已换上授权额度（带政策出处）才能来判：CONTEXT 要每项信用暴露保存实际采用的政策，
// 这条不变量守在这里而不是留给编排——守在编排里，少调一步的那条路会形成一份没有出处的暴露，
// 与存量行读起来一样。与作用域错配同列为前置错误，先于重放判：重放交回的是原暴露，但拿一份
// 不合格的状况来问本身已经是编程错误。
func (ledger *CreditExposureLedger) Expose(request ExposureRequest, standing CreditStanding) (CreditExposure, error) {
	if request.scope != standing.scope {
		return CreditExposure{}, ErrSettlementScopeMismatch
	}
	if standing.policy.String() == "" {
		return CreditExposure{}, ErrStandingNotAuthorized
	}

	digest := exposureDigest(request)
	if existingID, exists := ledger.byRequest[request.requestID]; exists {
		if ledger.digests[request.requestID] != digest {
			return CreditExposure{}, ErrControlRequestConflict
		}
		return ledger.byExposure[existingID], nil
	}

	if standing.overdue {
		return ledger.restricted(request, standing, "ACCOUNT_OVERDUE")
	}
	if request.amountMinor > standing.Headroom() {
		return ledger.restricted(request, standing, "AVAILABLE_CREDIT_INSUFFICIENT")
	}

	ledger.nextID++
	exposure := CreditExposure{
		exposureID:  ExposureID{requiredValue{value: fmt.Sprintf("EXP-%04d", ledger.nextID)}},
		requestID:   request.requestID,
		scope:       request.scope,
		amountMinor: request.amountMinor,
		association: request.association,
		status:      ExposureRecorded,
		exposedAt:   request.requestedAt,
		policy:      standing.policy,
	}
	ledger.byRequest[request.requestID] = exposure.exposureID
	ledger.byExposure[exposure.exposureID] = exposure
	ledger.digests[request.requestID] = digest
	return exposure, nil
}

// restricted 形成不入账本的`业务限制`结果。它同样带政策出处：受限也要答得出「按哪一版额度判的」，
// 否则调用方拿到一份限制却不知道是按哪份政策限的。
func (ledger *CreditExposureLedger) restricted(
	request ExposureRequest,
	standing CreditStanding,
	reasonValue string,
) (CreditExposure, error) {
	reason, err := NewRestrictionReason(reasonValue)
	if err != nil {
		return CreditExposure{}, err
	}
	return CreditExposure{
		requestID:   request.requestID,
		scope:       request.scope,
		amountMinor: request.amountMinor,
		association: request.association,
		status:      ExposureRestricted,
		reason:      reason,
		policy:      standing.policy,
	}, nil
}

// Release 把一笔已记录暴露放开。重复释放返回与首次相同的答案，包括原释放时间。
func (ledger *CreditExposureLedger) Release(exposureID ExposureID, releasedAt time.Time) (CreditExposure, error) {
	exposure, found := ledger.byExposure[exposureID]
	if !found {
		return CreditExposure{}, ErrExposureNotFound
	}
	if exposure.status == ExposureReleased {
		return exposure, nil
	}
	if releasedAt.IsZero() || releasedAt.Before(exposure.exposedAt) {
		return CreditExposure{}, ErrInvalidExposureRequest
	}
	exposure.status = ExposureReleased
	exposure.releasedAt = releasedAt.UTC()
	ledger.byExposure[exposureID] = exposure
	return exposure, nil
}

// ExposedMinor 是本册当前占用的额度总额，也就是信用状况里那一项`已占用暴露`。
//
// 理由与 FreezeLedger.HeldMinor 同一条，但实现分立在两本账上：它们互不借用
// （ADR-0047），抽一个共用求和出来正是 SET-03 要拦的合流。
func (ledger *CreditExposureLedger) ExposedMinor() int64 {
	total := int64(0)
	for _, exposure := range ledger.byExposure {
		if exposure.status == ExposureRecorded {
			total += exposure.amountMinor
		}
	}
	return total
}

// FindByRequest 按原控制请求身份找回暴露，释放按原业务关联认领时用（与冻结账本同款）。
func (ledger *CreditExposureLedger) FindByRequest(requestID ControlRequestID) (CreditExposure, bool) {
	exposureID, found := ledger.byRequest[requestID]
	if !found {
		return CreditExposure{}, false
	}
	exposure, found := ledger.byExposure[exposureID]
	return exposure, found
}

func exposureDigest(request ExposureRequest) string {
	return strings.Join([]string{
		request.scope.legalEntity.String(),
		request.scope.account.String(),
		request.scope.currency.String(),
		fmt.Sprint(request.amountMinor),
		request.association.String(),
	}, "\x00")
}
