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

// PreAdviceValidityWindow 是时窗册的一行。册只增：同租户任一时刻至多一行生效，生效区间重叠在登记时拒。登记时没有终点的那一行，
// 日后由一份显式的收口（PreAdviceWindowClosure）结束，收口是追加的另一份事实，不改写这一行登记的内容；读它在某一刻生效不生效、
// 与别的行重叠不重叠，都把收口时刻当作它的终点。不取「登新行即隐式截断前一行」：那等于改历史，一次笔误就会悄悄截断在用的时窗
// （票 ps-external-mark-relations/14 裁定）。
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
	// closure 零值即没收过口。
	closure PreAdviceWindowClosure
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

// EffectiveUntil 报告登记时给的终点及其是否存在。收口不改它：收口之后的终点读 Closure。
func (window PreAdviceValidityWindow) EffectiveUntil() (time.Time, bool) {
	return window.effectiveUntil, !window.effectiveUntil.IsZero()
}

// Closure 交回这一行的收口及其是否存在。
func (window PreAdviceValidityWindow) Closure() (PreAdviceWindowClosure, bool) {
	return window.closure, window.closure.valid()
}

// effectiveEnd 是这一行实际的终点：收过口就是收口时刻，否则是登记时给的终点；零值即没有终点。收口只落给登记时没有终点的行
// （Close），两者不会同时在场。
func (window PreAdviceValidityWindow) effectiveEnd() time.Time {
	if window.closure.valid() {
		return window.closure.closedAt
	}
	return window.effectiveUntil
}

// Close 把一份收口挂到这一行上，交回收口之后的这一行；收口指的必须就是这一行（同租户、同起点），否则是调用方读错了行，答形不成。
// 这一行登记时带终点、或已收过口，答不开放——重复收口是不是原样，由编排拿 Closure 比内容，这里不替它判。
func (window PreAdviceValidityWindow) Close(closure PreAdviceWindowClosure) (PreAdviceValidityWindow, error) {
	if !closure.valid() || closure.tenantID != window.tenantID || !closure.windowFrom.Equal(window.effectiveFrom) {
		return PreAdviceValidityWindow{}, ErrInvalidPreAdviceWindowClosure
	}
	if !window.Registered() || !window.effectiveUntil.IsZero() || window.closure.valid() {
		return PreAdviceValidityWindow{}, ErrPreAdviceWindowNotOpen
	}
	window.closure = closure
	return window, nil
}

// Registered 报告这是一行在册的时窗；零值答否。
func (window PreAdviceValidityWindow) Registered() bool {
	return window.tenantID.valid() && !window.effectiveFrom.IsZero()
}

// EffectiveAt 报告这一行在时刻 at 生效：含起点、不含终点；收过口的，终点是收口时刻。
func (window PreAdviceValidityWindow) EffectiveAt(at time.Time) bool {
	if !window.Registered() || at.Before(window.effectiveFrom) {
		return false
	}
	end := window.effectiveEnd()
	return end.IsZero() || at.Before(end)
}

// Admits 报告一次业务时间为 receivedAt 的送站，落不落在预计送达时刻为 expectedArrivalAt 的那份预报的有效时窗里：两端都算在内。
func (window PreAdviceValidityWindow) Admits(expectedArrivalAt, receivedAt time.Time) bool {
	return !receivedAt.Before(expectedArrivalAt.Add(-window.lead)) && !receivedAt.After(expectedArrivalAt.Add(window.grace))
}

// Overlaps 报告两行的生效区间有没有交集（半开区间，首尾相接不算；收过口的按收口时刻截止）。不看租户：同租户才比，那是调用方的事。
func (window PreAdviceValidityWindow) Overlaps(other PreAdviceValidityWindow) bool {
	end, otherEnd := window.effectiveEnd(), other.effectiveEnd()
	startsBeforeOtherEnds := otherEnd.IsZero() || window.effectiveFrom.Before(otherEnd)
	otherStartsBeforeThisEnds := end.IsZero() || other.effectiveFrom.Before(end)
	return startsBeforeOtherEnds && otherStartsBeforeThisEnds
}

// SameContent 报告两行是不是同一份登记内容：租户、两个时长、登记时给的生效区间与登记依据逐格相等。登记时刻不比——那是册记下的，
// 不是登记方说的，重放的那一次本来就晚。收口也不比：它是日后追加的另一份事实，不是这一次登记说的，收口之后原样重登仍是重放。
func (window PreAdviceValidityWindow) SameContent(other PreAdviceValidityWindow) bool {
	return window.tenantID == other.tenantID &&
		window.lead == other.lead &&
		window.grace == other.grace &&
		window.effectiveFrom.Equal(other.effectiveFrom) &&
		window.effectiveUntil.Equal(other.effectiveUntil) &&
		window.basis == other.basis
}

// ErrInvalidPreAdviceWindowClosure 说明一份收口立不起来：缺依据、收口时刻不晚于那一行的起点一类，或交给 Close 的收口指的不是这一行。
// 重发同样的内容不会变好。
var ErrInvalidPreAdviceWindowClosure = errors.New("node operations: invalid pre-advice window closure")

// ErrPreAdviceWindowNotOpen 说明要收口的那一行此刻已有终点：登记时带了终点，或已收过口。
var ErrPreAdviceWindowNotOpen = errors.New("node operations: pre-advice window is not open")

// PreAdviceWindowClosureSpec 是收口一行时窗所需的全部输入。WindowFrom 是要收口那一行的起点：同租户起点唯一，它就是那一行在册上的
// 身份——收口指名那一行，不是「此刻开着的那一行」，一份迟到或指错的收口因此落不到别的行上。
type PreAdviceWindowClosureSpec struct {
	TenantID     TenantID
	WindowFrom   time.Time
	ClosedAt     time.Time
	Basis        string
	RegisteredAt time.Time
}

// PreAdviceWindowClosure 是时窗册上追加给一行开放终点时窗的收口（票 ps-external-mark-relations/14 裁定）：自收口时刻起，那一行不再
// 生效，起点不早于收口时刻的新行照常登记。一行至多收口一次。零值即没有收口。
type PreAdviceWindowClosure struct {
	tenantID     TenantID
	windowFrom   time.Time
	closedAt     time.Time
	basis        string
	registeredAt time.Time
}

// FormPreAdviceWindowClosure 立一份收口。收口时刻必须晚于那一行的起点：恰在起点会让那一行的生效区间变空，早于起点更不成话；比较
// 在折成册上的精度之后做，免得差几纳秒的两个时刻落册后相等。登记依据必备，去两端空白后逐字保存。那一行在不在册、开不开放不在
// 这里判，归 Close 与册。
func FormPreAdviceWindowClosure(spec PreAdviceWindowClosureSpec) (PreAdviceWindowClosure, error) {
	basis := strings.TrimSpace(spec.Basis)
	if !spec.TenantID.valid() || spec.WindowFrom.IsZero() || spec.ClosedAt.IsZero() || basis == "" || spec.RegisteredAt.IsZero() {
		return PreAdviceWindowClosure{}, ErrInvalidPreAdviceWindowClosure
	}
	windowFrom, closedAt := registerInstant(spec.WindowFrom), registerInstant(spec.ClosedAt)
	if !closedAt.After(windowFrom) {
		return PreAdviceWindowClosure{}, ErrInvalidPreAdviceWindowClosure
	}
	return PreAdviceWindowClosure{
		tenantID:     spec.TenantID,
		windowFrom:   windowFrom,
		closedAt:     closedAt,
		basis:        basis,
		registeredAt: registerInstant(spec.RegisteredAt),
	}, nil
}

func (closure PreAdviceWindowClosure) TenantID() TenantID      { return closure.tenantID }
func (closure PreAdviceWindowClosure) WindowFrom() time.Time   { return closure.windowFrom }
func (closure PreAdviceWindowClosure) ClosedAt() time.Time     { return closure.closedAt }
func (closure PreAdviceWindowClosure) Basis() string           { return closure.basis }
func (closure PreAdviceWindowClosure) RegisteredAt() time.Time { return closure.registeredAt }

func (closure PreAdviceWindowClosure) valid() bool {
	return closure.tenantID.valid() && !closure.windowFrom.IsZero() && !closure.closedAt.IsZero()
}

// SameContent 报告两份收口是不是同一份内容：租户、那一行的起点、收口时刻与依据逐格相等。登记时刻不比，理由同时窗行。
func (closure PreAdviceWindowClosure) SameContent(other PreAdviceWindowClosure) bool {
	return closure.tenantID == other.tenantID &&
		closure.windowFrom.Equal(other.windowFrom) &&
		closure.closedAt.Equal(other.closedAt) &&
		closure.basis == other.basis
}
