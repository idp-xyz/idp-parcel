package domain

import (
	"errors"
	"time"
)

var ErrInvalidRouteHandoff = errors.New("network routing: invalid route handoff")

// AcceptanceDecisionReference 指名触发本次初始路由的那个委托接受决定。决定属
// parcel-shipment，这里只引用不重述。
type AcceptanceDecisionReference struct{ requiredValue }

func NewAcceptanceDecisionReference(value string) (AcceptanceDecisionReference, error) {
	required, err := newRequiredValue("acceptance decision reference", value)
	return AcceptanceDecisionReference{required}, err
}

// AcceptanceBaselineReference 指名路由所依据的不可覆盖接受基线版本。UC-NR-001 启动条件
// 硬句：「只有状态字符串而没有基线引用时不得继续」——路由的业务输入是基线本体，一个
// 「已接受」的状态字读不出成员、声明与承诺。
type AcceptanceBaselineReference struct{ requiredValue }

func NewAcceptanceBaselineReference(value string) (AcceptanceBaselineReference, error) {
	required, err := newRequiredValue("acceptance baseline reference", value)
	return AcceptanceBaselineReference{required}, err
}

// CommercialResolutionReference 是消费方对一次已固定商业解析的引用（ADR-0064）。
// 权威在提供方按标识持有的闭包；它是命令附加字段，不是初始路由判断维。
type CommercialResolutionReference struct{ requiredValue }

func NewCommercialResolutionReference(value string) (CommercialResolutionReference, error) {
	required, err := newRequiredValue("commercial resolution reference", value)
	return CommercialResolutionReference{required}, err
}

func (ref CommercialResolutionReference) Valid() bool {
	return ref.valid()
}

// RouteHandoffSpec 是一次委托接受交接所需的全部输入。
type RouteHandoffSpec struct {
	Correlation        RequestCorrelationID
	TenantID           TenantID
	CustomerAccountID  CustomerAccountID
	ShipmentRequestID  ShipmentRequestID
	AcceptanceDecision AcceptanceDecisionReference
	AcceptanceBaseline AcceptanceBaselineReference
	Parcels            []DeclaredParcelID
	AcceptedAt         time.Time
}

// RouteHandoff 是 parcel-shipment 已接受委托到本上下文的路由交接（UC-NR-001 输入组）。
// 它是触发凭据不是判断：路由结果按包裹各自形成，交接只回答「为谁、依据哪一版基线」。
type RouteHandoff struct {
	correlation        RequestCorrelationID
	tenantID           TenantID
	customerAccountID  CustomerAccountID
	shipmentRequestID  ShipmentRequestID
	acceptanceDecision AcceptanceDecisionReference
	acceptanceBaseline AcceptanceBaselineReference
	parcels            []DeclaredParcelID
	acceptedAt         time.Time
}

// NewRouteHandoff 全件必备：缺决定或基线引用的交接不得继续（启动条件）；成员空或重复
// 是装配错误——路由以包裹为计划单位，一个没有成员的交接没有可判断的对象。
func NewRouteHandoff(spec RouteHandoffSpec) (RouteHandoff, error) {
	if !spec.Correlation.valid() ||
		!spec.TenantID.valid() ||
		!spec.CustomerAccountID.valid() ||
		!spec.ShipmentRequestID.valid() ||
		!spec.AcceptanceDecision.valid() ||
		!spec.AcceptanceBaseline.valid() ||
		spec.AcceptedAt.IsZero() {
		return RouteHandoff{}, ErrInvalidRouteHandoff
	}
	if len(spec.Parcels) == 0 {
		return RouteHandoff{}, ErrInvalidRouteHandoff
	}
	seen := make(map[DeclaredParcelID]struct{}, len(spec.Parcels))
	parcels := make([]DeclaredParcelID, 0, len(spec.Parcels))
	for _, parcel := range spec.Parcels {
		if !parcel.valid() {
			return RouteHandoff{}, ErrInvalidRouteHandoff
		}
		if _, duplicated := seen[parcel]; duplicated {
			return RouteHandoff{}, ErrInvalidRouteHandoff
		}
		seen[parcel] = struct{}{}
		parcels = append(parcels, parcel)
	}
	return RouteHandoff{
		correlation:        spec.Correlation,
		tenantID:           spec.TenantID,
		customerAccountID:  spec.CustomerAccountID,
		shipmentRequestID:  spec.ShipmentRequestID,
		acceptanceDecision: spec.AcceptanceDecision,
		acceptanceBaseline: spec.AcceptanceBaseline,
		parcels:            parcels,
		acceptedAt:         spec.AcceptedAt.UTC(),
	}, nil
}

func (handoff RouteHandoff) Correlation() RequestCorrelationID {
	return handoff.correlation
}

func (handoff RouteHandoff) AcceptanceDecision() AcceptanceDecisionReference {
	return handoff.acceptanceDecision
}

func (handoff RouteHandoff) AcceptanceBaseline() AcceptanceBaselineReference {
	return handoff.acceptanceBaseline
}

func (handoff RouteHandoff) Parcels() []DeclaredParcelID {
	return append([]DeclaredParcelID(nil), handoff.parcels...)
}

func (handoff RouteHandoff) AcceptedAt() time.Time {
	return handoff.acceptedAt
}

// InitialRouteJudgmentKey 是一次包裹级初始路由判断的完整范围。幂等硬句钉在维度选择上：
// 「同一接受基线、同一包裹和同一初始路由目的只能形成一个当前有效初始路由结果」——基线
// 与目的都在键内，判断时点刻意不在：重复交接按同一键找回已有结果，而不是每个时点各造
// 一份并行计划。
type InitialRouteJudgmentKey struct {
	TenantID           TenantID
	CustomerAccountID  CustomerAccountID
	ShipmentRequestID  ShipmentRequestID
	AcceptanceBaseline AcceptanceBaselineReference
	DeclaredParcelID   DeclaredParcelID
	ServicePurpose     ServicePurpose
}

// MinimumIdentityEstablished 与可达性判断键同一条纪律：身份不成立时不读任何权威。
func (key InitialRouteJudgmentKey) MinimumIdentityEstablished() bool {
	return key.TenantID.valid() &&
		key.CustomerAccountID.valid() &&
		key.ShipmentRequestID.valid() &&
		key.AcceptanceBaseline.valid() &&
		key.DeclaredParcelID.valid() &&
		key.ServicePurpose.valid()
}

// SameJudgmentScope 是幂等与冲突共用的界线：同键重复交接返回已有结果，异键共用一个
// 交接关联则是冲突。
func (key InitialRouteJudgmentKey) SameJudgmentScope(other InitialRouteJudgmentKey) bool {
	return key == other
}

// JudgmentKeys 按交接逐包裹派生判断键（UC-NR-001 步骤 4「按接受基线逐包裹建立独立判断
// 范围」）。目的是装配参数：属服务产品的话语，未配置时停下而不是替产品挑一个。
func (handoff RouteHandoff) JudgmentKeys(purpose ServicePurpose) ([]InitialRouteJudgmentKey, error) {
	if !purpose.valid() {
		return nil, ErrInvalidRouteHandoff
	}
	keys := make([]InitialRouteJudgmentKey, 0, len(handoff.parcels))
	for _, parcel := range handoff.parcels {
		keys = append(keys, InitialRouteJudgmentKey{
			TenantID:           handoff.tenantID,
			CustomerAccountID:  handoff.customerAccountID,
			ShipmentRequestID:  handoff.shipmentRequestID,
			AcceptanceBaseline: handoff.acceptanceBaseline,
			DeclaredParcelID:   parcel,
			ServicePurpose:     purpose,
		})
	}
	return keys, nil
}
