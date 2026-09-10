package domain

import "fmt"

// DeliveryPlaceOutcome 是「收件地点引用」读口的封闭四格（ADR-0130 决定二；PS CONTEXT Rules「收件地点引用
// 按（租户，包裹身份）答」那条）。前两格带引用，后两格不带。
//
// 没有`不知道`格：走到答案要翻的册子——接受基线的成员集合、收件范围上的资料版本、由它们派生的当前采用
// 判断——全是本上下文自己的，答不出就是读面坏了，上抛 error 而不是多一格让消费方去猜。
//
// `未定`与`没有`分格交：前者是「有收件资料但两条修订分叉等人来并」，后者是「对象不在任何已接受委托的
// 成员集合里」。TF 端口今天只有两格，怎么落是 TF 的事（ADR-0130 越权风险点 1），本上下文不替它并。
type DeliveryPlaceOutcome uint8

const (
	DeliveryPlaceOutcomeInvalid DeliveryPlaceOutcome = iota
	// DeliveryPlaceAnchoredOnBaseline：收件范围上尚无修订版本，引用带接受基线锚。
	DeliveryPlaceAnchoredOnBaseline
	// DeliveryPlaceAnchoredOnAdoptedVersion：收件范围上的当前采用判断为`已采用`，引用带那一版的锚。
	DeliveryPlaceAnchoredOnAdoptedVersion
	// DeliveryPlaceUndetermined：当前采用判断为`待复核`，本上下文此刻说不出该送哪一版，不给引用。
	DeliveryPlaceUndetermined
	// NoDeliveryPlace：对象不属任何已接受委托的成员集合（含集运单元与不可见对象），按统一不可见结果答，
	// 不区分不存在、他租户与未授权。
	NoDeliveryPlace
)

func (outcome DeliveryPlaceOutcome) String() string {
	switch outcome {
	case DeliveryPlaceAnchoredOnBaseline:
		return "ANCHORED_ON_BASELINE"
	case DeliveryPlaceAnchoredOnAdoptedVersion:
		return "ANCHORED_ON_ADOPTED_VERSION"
	case DeliveryPlaceUndetermined:
		return "UNDETERMINED"
	case NoDeliveryPlace:
		return "NO_DELIVERY_PLACE"
	default:
		return ""
	}
}

// DeliveryPlaceResolution 是读口对一次（租户，包裹身份）之问的答复：四格之一，前两格附引用。
type DeliveryPlaceResolution struct {
	outcome   DeliveryPlaceOutcome
	reference DeliveryPlaceReference
}

// DeliveryPlaceReferenced 把一份引用包成带引用的那一格。格由锚决定，不由调用方挑：一份基线锚引用
// 不可能被说成「已采用版本锚」——两格的差别就是锚，多一个可选参数只会让两处从此各说各话。
func DeliveryPlaceReferenced(reference DeliveryPlaceReference) (DeliveryPlaceResolution, error) {
	if !reference.valid() {
		return DeliveryPlaceResolution{}, ErrInvalidDeliveryPlaceReference
	}
	outcome := DeliveryPlaceAnchoredOnBaseline
	if _, adopted := reference.anchor.AdoptedVersion(); adopted {
		outcome = DeliveryPlaceAnchoredOnAdoptedVersion
	}
	return DeliveryPlaceResolution{outcome: outcome, reference: reference}, nil
}

// DeliveryPlaceUndeterminedResolution 是「收件地点未定」那一格。
func DeliveryPlaceUndeterminedResolution() DeliveryPlaceResolution {
	return DeliveryPlaceResolution{outcome: DeliveryPlaceUndetermined}
}

// NoDeliveryPlaceResolution 是「没有收件地点」那一格。
func NoDeliveryPlaceResolution() DeliveryPlaceResolution {
	return DeliveryPlaceResolution{outcome: NoDeliveryPlace}
}

func (resolution DeliveryPlaceResolution) Outcome() DeliveryPlaceOutcome {
	return resolution.outcome
}

// Reference 只在前两格在场；后两格第二个返回值为假，不是「引用读不到」。
func (resolution DeliveryPlaceResolution) Reference() (DeliveryPlaceReference, bool) {
	return resolution.reference, resolution.reference.valid()
}

// DeliveryPlaceReferenceFor 按包裹身份答收件地点引用（ADR-0130 决定二）。委托与接受基线是本上下文
// 内部走到答案的路，不是键：先经接受基线成员集合确认这件包裹属于本委托，再取本委托收件资料范围上
// 查询时刻的当前采用判断。
//
// 包裹 → 委托只按声明成员走。包裹身份谱系（拆分 / 合并后的新包裹）今天领域里没有模型，谱系包裹会
// 落「没有收件地点」——这一格是「谱系未建模」的今日形状，谱系落地那票要补这一路（同
// ps-port-remainder/05 NO 半边「从未关联 → 不在」那条的处置）。未接受的委托没有基线，其成员同样答
// 没有：责任起点在接受之后，接受前没有可派送的收件地点。
//
// 只查委托级范围。CONTEXT 硬句「同一委托中的包裹必须共享……寄收件关系」，所以收件资料按委托级
// 范围形成；按包裹指名收件范围能不能成立归 BD-PS-010 的资料组词表，本方法不替它决定，值对象那一侧
// 已能携带包裹段，词表落地那天只需在这里多问一条范围。
//
// 派生规则只有一处：当前采用判断由 CurrentSourceDataAdoption 给出，这里只把它的三态译成四格，不重算。
func (request ShipmentRequest) DeliveryPlaceReferenceFor(parcel DeclaredParcelID) (DeliveryPlaceResolution, error) {
	if request.state != ShipmentRequestAccepted || !request.baseline.covers(parcel) {
		return NoDeliveryPlaceResolution(), nil
	}
	scope, err := NewShipmentScopedSourceData(request.shipmentRequestID, DeliveryPlaceDataGroup())
	if err != nil {
		return DeliveryPlaceResolution{}, fmt.Errorf("delivery place reference: %w", err)
	}

	anchor := NewAcceptanceBaselineAnchor()
	if judgment, derived := request.CurrentSourceDataAdoption(scope); derived {
		switch judgment.Outcome() {
		case SourceDataAdopted:
			adopted, _ := judgment.AdoptedVersion()
			if anchor, err = NewSourceDataVersionAnchor(adopted); err != nil {
				return DeliveryPlaceResolution{}, fmt.Errorf("delivery place reference: %w", err)
			}
		case SourceDataAwaitingReview:
			return DeliveryPlaceUndeterminedResolution(), nil
		default:
			// 采用判断长出本方法不认识的取值时不猜一格：CONTEXT 的封闭集合还有三项没有产生规则，
			// 它们落地那天要在这里各配一格，而不是被默认成基线锚。
			return DeliveryPlaceResolution{}, fmt.Errorf("%w: 采用判断 %q 没有对应的收件地点答法", ErrInvalidDeliveryPlaceReference, judgment.Outcome())
		}
	}

	tenant := request.currentVersion.SourceSubmission().Identity().TenantID()
	reference, err := NewDeliveryPlaceReference(tenant, scope, anchor)
	if err != nil {
		return DeliveryPlaceResolution{}, fmt.Errorf("delivery place reference: %w", err)
	}
	return DeliveryPlaceReferenced(reference)
}
