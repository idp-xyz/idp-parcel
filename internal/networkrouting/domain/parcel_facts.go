package domain

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidParcelFactsPoint 说候选出价所处的判断点立不起来：种类认不出，或首次收寄复核没有收寄的业务时间。
var ErrInvalidParcelFactsPoint = errors.New("network routing: invalid parcel facts point")

// ParcelFactsPointKind 是候选出价所处的判断点。包裹事实取哪一种来源由判断点定，不是租户取值：预路由使用客户声明快照，
// 首次收寄复核使用当时已经取得的物理事实，到站复核使用当前有效物理实测（CONTEXT「预路由使用客户声明快照」那一条）。
type ParcelFactsPointKind uint8

const (
	ParcelFactsPointKindInvalid ParcelFactsPointKind = iota
	// PreRouteFactsPoint 是初始路由判断（含其并列的授权裁决）：接受时手上只有客户声明。
	PreRouteFactsPoint
	// FirstIntakeFactsPoint 是有效网络收寄（节点收寄、场外揽收）触发的复核：只认收寄那一刻之前已经取得的物理事实。
	FirstIntakeFactsPoint
	// ArrivalFactsPoint 是权威运输交接把包裹交进接收节点触发的复核：认判断此刻仍有效的物理实测。
	ArrivalFactsPoint
	// DecisionFactsPoint 是授权角色据复核建议形成改路决定：决定由操作者此刻发起、不是复核触发，新计划从决定时刻生效，
	// 与到站复核同取判断此刻仍有效的物理实测。
	DecisionFactsPoint
)

// ParcelFactsPoint 是判断点加上它要的业务时点。零值不合法：每个取成本的调用点都得自己指名判断点，不落进默认。
type ParcelFactsPoint struct {
	kind ParcelFactsPointKind
	at   time.Time
}

func NewPreRouteFactsPoint() ParcelFactsPoint { return ParcelFactsPoint{kind: PreRouteFactsPoint} }

func NewArrivalFactsPoint() ParcelFactsPoint { return ParcelFactsPoint{kind: ArrivalFactsPoint} }

func NewDecisionFactsPoint() ParcelFactsPoint { return ParcelFactsPoint{kind: DecisionFactsPoint} }

// NewFirstIntakeFactsPoint 以收寄的业务时间立首次收寄复核的判断点。时间必填：「当时已经取得」没有时点就无从截。
func NewFirstIntakeFactsPoint(intakeAt time.Time) (ParcelFactsPoint, error) {
	if intakeAt.IsZero() {
		return ParcelFactsPoint{}, fmt.Errorf("%w: first intake needs the business time of the intake", ErrInvalidParcelFactsPoint)
	}
	return ParcelFactsPoint{kind: FirstIntakeFactsPoint, at: intakeAt.UTC()}, nil
}

func (point ParcelFactsPoint) Kind() ParcelFactsPointKind { return point.kind }

func (point ParcelFactsPoint) Valid() bool {
	switch point.kind {
	case PreRouteFactsPoint, ArrivalFactsPoint, DecisionFactsPoint:
		return true
	case FirstIntakeFactsPoint:
		return !point.at.IsZero()
	default:
		return false
	}
}

// WantsPhysicalFacts 答这个判断点是否以物理事实为先：预路由之外的判断点都是。
func (point ParcelFactsPoint) WantsPhysicalFacts() bool {
	return point.Valid() && point.kind != PreRouteFactsPoint
}

// PhysicalFactsAsOf 答物理事实截至哪一刻：首次收寄复核截到收寄的业务时间，其余取传入的判断此刻。预路由不读物理事实，
// 答不出时点。
func (point ParcelFactsPoint) PhysicalFactsAsOf(now time.Time) (time.Time, bool) {
	switch point.kind {
	case FirstIntakeFactsPoint:
		return point.at, true
	case ArrivalFactsPoint, DecisionFactsPoint:
		return now.UTC(), true
	default:
		return time.Time{}, false
	}
}

// ParcelFactsBasis 说一次候选出价的包裹事实出自哪里。它随逐段成本出处与复核记录留痕，让审计看得出价是按声明还是按
// 物理事实出的——取数侧没有退路时静默落到声明，复核与预路由就读不出区别了。
type ParcelFactsBasis uint8

const (
	ParcelFactsBasisInvalid ParcelFactsBasis = iota
	// ParcelFactsDeclaration 是预路由取客户声明，本该如此。
	ParcelFactsDeclaration
	// ParcelFactsPhysical 是用到的每一项（实重、外廓）都出自物理事实。
	ParcelFactsPhysical
	// ParcelFactsPhysicalWithDeclaration 是物理事实只取得了一部分：有的项（常见是只称了重、没量外廓）取物理事实，其余取
	// 客户声明。客户声明只补物理事实没有的项，不顶替任何一项已有的实测。
	ParcelFactsPhysicalWithDeclaration
	// ParcelFactsDeclarationFallback 是该取物理事实而一项都没取得，退回客户声明出价。排序照常，出处如实标明。
	ParcelFactsDeclarationFallback
	// ParcelFactsPhysicalUnresolved 是物理事实在、却定不下用哪一条（多于一条而没有已登记的取用规则）。客户声明不得顶替
	// 实测，所以不退回声明：各段待判断，候选缺成本依据。它没有出价，只落在复核记录上，不会出现在成本出处里。
	ParcelFactsPhysicalUnresolved
	// ParcelFactsNone 是该取的事实一项也没有，声明也没有：各段待判断。
	ParcelFactsNone
)

func (basis ParcelFactsBasis) String() string {
	switch basis {
	case ParcelFactsDeclaration:
		return "DECLARATION"
	case ParcelFactsPhysical:
		return "PHYSICAL"
	case ParcelFactsPhysicalWithDeclaration:
		return "PHYSICAL_WITH_DECLARATION"
	case ParcelFactsDeclarationFallback:
		return "DECLARATION_FALLBACK"
	case ParcelFactsPhysicalUnresolved:
		return "PHYSICAL_UNRESOLVED"
	case ParcelFactsNone:
		return "NONE"
	default:
		return ""
	}
}

func (basis ParcelFactsBasis) Valid() bool { return basis.String() != "" }

// PricesACandidate 答这一格依据下是否可能有出价：成本出处只记出了价的段，所以只有这几格能进出处。
func (basis ParcelFactsBasis) PricesACandidate() bool {
	switch basis {
	case ParcelFactsDeclaration, ParcelFactsPhysical, ParcelFactsPhysicalWithDeclaration, ParcelFactsDeclarationFallback:
		return true
	default:
		return false
	}
}

// ParseParcelFactsBasis 把库里的文本读回依据。认不出即错误，不兜底成某一格。
func ParseParcelFactsBasis(raw string) (ParcelFactsBasis, error) {
	for basis := ParcelFactsDeclaration; basis <= ParcelFactsNone; basis++ {
		if basis.String() == raw {
			return basis, nil
		}
	}
	return ParcelFactsBasisInvalid, fmt.Errorf("network routing: unknown parcel facts basis %q", raw)
}

// ParcelFactsPoint 由触发所携控制依据定这次复核的判断点：节点收寄与场外揽收是有效网络收寄的两种来源，同属首次收寄复核，
// 取收寄的业务时间；把包裹交进接收节点的权威运输交接是到站复核。场外揽收没有节点能称重，当时没有物理事实是常态，由取数侧
// 如实退回声明，不在这里另立一格。
func (trigger ReassessmentTrigger) ParcelFactsPoint() ParcelFactsPoint {
	switch trigger.control {
	case NodeIntakeControl, OffsitePickupControl:
		point, err := NewFirstIntakeFactsPoint(trigger.occurredAt)
		if err != nil {
			// 构造期已校验业务时间非零，到不了这里；到了就让下游认不出判断点而响亮报错，不静默落到某一格。
			return ParcelFactsPoint{}
		}
		return point
	case TransportHandoverControl:
		return NewArrivalFactsPoint()
	default:
		return ParcelFactsPoint{}
	}
}
