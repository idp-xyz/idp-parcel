package domain

import (
	"errors"
	"strings"
	"time"
)

// 预报有效时窗册（ADR-0212 决定二末项；参数登记册 PAR-NET-18）。册的列归产品，行是租户取值：一份预报只对业务时间落在〔预计送达
// 时刻减提前量，预计送达时刻加宽限量〕里的送站算数。仓内不写任何默认时长；判断时没有生效行，交付范围核对态答未配置。

// ErrInvalidPreAdviceValidityWindow 说明一行时窗立不起来。重发同样的内容不会变好。
var ErrInvalidPreAdviceValidityWindow = errors.New("node operations: invalid pre-advice validity window")

// PreAdviceValidityWindowSpec 是登一行时窗所需的全部输入。EffectiveUntil 零值即没有终点。
type PreAdviceValidityWindowSpec struct {
	TenantID       TenantID
	Lead           time.Duration
	Grace          time.Duration
	EffectiveFrom  time.Time
	EffectiveUntil time.Time
	Basis          string
	RegisteredAt   time.Time
}

// PreAdviceValidityWindow 是时窗册的一行。册只增：同租户任一时刻至多一行生效，生效区间重叠在登记时拒，没有改写或收尾的路。代价
// 是登下开放终点的那一行之后，这个租户再也登不进更晚起点的行——要换值的租户先登带终点的行。允许后继给开放前版落终点（关务节点
// 区域册那一种）会让「重叠在登记时拒」不再成立，那是另一个决定，不在本册。
//
// 零值即「没有生效行」：推导据此答未配置。
type PreAdviceValidityWindow struct {
	tenantID       TenantID
	lead           time.Duration
	grace          time.Duration
	effectiveFrom  time.Time
	effectiveUntil time.Time
	basis          string
	registeredAt   time.Time
}

// FormPreAdviceValidityWindow 立一行时窗。提前量与宽限量不得为负、以整分钟为粒度（册以分钟存，粒度是机制，不是租户取值）；两者为
// 零照收——那是租户的取值，产品不替它判合不合理。生效区间半开：含起点、不含终点。登记依据必备，去两端空白后逐字保存。
func FormPreAdviceValidityWindow(spec PreAdviceValidityWindowSpec) (PreAdviceValidityWindow, error) {
	basis := strings.TrimSpace(spec.Basis)
	if !spec.TenantID.valid() ||
		spec.Lead < 0 || spec.Lead%time.Minute != 0 ||
		spec.Grace < 0 || spec.Grace%time.Minute != 0 ||
		spec.EffectiveFrom.IsZero() ||
		basis == "" ||
		spec.RegisteredAt.IsZero() {
		return PreAdviceValidityWindow{}, ErrInvalidPreAdviceValidityWindow
	}
	from := registerInstant(spec.EffectiveFrom)
	var until time.Time
	if !spec.EffectiveUntil.IsZero() {
		until = registerInstant(spec.EffectiveUntil)
		if !until.After(from) {
			return PreAdviceValidityWindow{}, ErrInvalidPreAdviceValidityWindow
		}
	}
	return PreAdviceValidityWindow{
		tenantID:       spec.TenantID,
		lead:           spec.Lead,
		grace:          spec.Grace,
		effectiveFrom:  from,
		effectiveUntil: until,
		basis:          basis,
		registeredAt:   registerInstant(spec.RegisteredAt),
	}, nil
}

func (window PreAdviceValidityWindow) TenantID() TenantID       { return window.tenantID }
func (window PreAdviceValidityWindow) Lead() time.Duration      { return window.lead }
func (window PreAdviceValidityWindow) Grace() time.Duration     { return window.grace }
func (window PreAdviceValidityWindow) EffectiveFrom() time.Time { return window.effectiveFrom }
func (window PreAdviceValidityWindow) Basis() string            { return window.basis }
func (window PreAdviceValidityWindow) RegisteredAt() time.Time  { return window.registeredAt }

// EffectiveUntil 报告生效区间的终点及其是否存在。
func (window PreAdviceValidityWindow) EffectiveUntil() (time.Time, bool) {
	return window.effectiveUntil, !window.effectiveUntil.IsZero()
}

// Registered 报告这是一行在册的时窗；零值答否。
func (window PreAdviceValidityWindow) Registered() bool {
	return window.tenantID.valid() && !window.effectiveFrom.IsZero()
}

// EffectiveAt 报告这一行在时刻 at 生效：含起点、不含终点。
func (window PreAdviceValidityWindow) EffectiveAt(at time.Time) bool {
	if !window.Registered() || at.Before(window.effectiveFrom) {
		return false
	}
	return window.effectiveUntil.IsZero() || at.Before(window.effectiveUntil)
}

// Admits 报告一次业务时间为 receivedAt 的送站，落不落在预计送达时刻为 expectedArrivalAt 的那份预报的有效时窗里：两端都算在内。
func (window PreAdviceValidityWindow) Admits(expectedArrivalAt, receivedAt time.Time) bool {
	return !receivedAt.Before(expectedArrivalAt.Add(-window.lead)) && !receivedAt.After(expectedArrivalAt.Add(window.grace))
}

// Overlaps 报告两行的生效区间有没有交集（半开区间，首尾相接不算）。不看租户：同租户才比，那是调用方的事。
func (window PreAdviceValidityWindow) Overlaps(other PreAdviceValidityWindow) bool {
	startsBeforeOtherEnds := other.effectiveUntil.IsZero() || window.effectiveFrom.Before(other.effectiveUntil)
	otherStartsBeforeThisEnds := window.effectiveUntil.IsZero() || other.effectiveFrom.Before(window.effectiveUntil)
	return startsBeforeOtherEnds && otherStartsBeforeThisEnds
}

// SameContent 报告两行是不是同一份登记内容：租户、两个时长、生效区间与登记依据逐格相等。登记时刻不比——那是册记下的，不是登记方
// 说的，重放的那一次本来就晚。
func (window PreAdviceValidityWindow) SameContent(other PreAdviceValidityWindow) bool {
	return window.tenantID == other.tenantID &&
		window.lead == other.lead &&
		window.grace == other.grace &&
		window.effectiveFrom.Equal(other.effectiveFrom) &&
		window.effectiveUntil.Equal(other.effectiveUntil) &&
		window.basis == other.basis
}
