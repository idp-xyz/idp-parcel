package domain

import (
	"errors"
	"fmt"
)

var ErrInvalidDeclaredMeasurementResolution = errors.New("parcel shipment: invalid declared measurement resolution")

// declaredMeasurementDataGroupWord 是本上下文对「申报测量」资料组的自有原词，与 deliveryPlaceDataGroupWord 同款：
// SourceDataGroupReference 是开放引用，矩阵登记侧用什么词是实例半边；读口按这个词查该包裹上的修订版本，查不到就答
// 基线锚——那是如实答案（本上下文没见过那个范围上的修订），不是默认。
const declaredMeasurementDataGroupWord = "DECLARED_MEASUREMENT"

// DeclaredMeasurementDataGroup 交回申报测量资料组的原词引用。
func DeclaredMeasurementDataGroup() SourceDataGroupReference {
	return SourceDataGroupReference{requiredValue{value: declaredMeasurementDataGroupWord}}
}

// DeclaredMeasurementOutcome 是「申报测量」读口的封闭五格（pp-seams/02 裁决 3 / 5 / 6）。它照 DeliveryPlaceOutcome 的
// 四格再加「未申报」一格：那四格答的是引用，引用总能派生；本口答的是内容，而内容可以如实缺席（测量必填与否由真实产品
// 定，ADR-0048 允许成员无画像）——缺席不是「无」（对象不是委托成员）也不是「未定」（两条修订分叉），得自己占一格。
//
// 没有`不知道`格，理由同 DeliveryPlaceOutcome：走到答案要翻的册子全是本上下文自己的，答不出就是读面坏了，上抛 error。
type DeclaredMeasurementOutcome uint8

const (
	DeclaredMeasurementOutcomeInvalid DeclaredMeasurementOutcome = iota
	// DeclaredMeasurementAnchoredOnBaseline：该包裹的申报测量范围上尚无修订版本，答接受基线那一版提交版本上的成员画像，
	// 带接受基线锚。这是今天唯一带测量的一格。
	DeclaredMeasurementAnchoredOnBaseline
	// DeclaredMeasurementAnchoredOnAdoptedVersion：该范围上的当前采用判断为`已采用`，答那一版的锚——**只交锚不交测量**。
	// 客户原始资料版本今天只留痕不留内容（CustomerSourceDataVersion 是请求指纹 + 留痕清单，修订的载荷只进摘要），本上下文
	// 说不出那一版报了多少；交基线值等于把一份已被客户更正的申报当现行申报送出去。消费方拿到这一格该停在「输入不可得」，
	// 而不是回头用基线值。版本留内容那天，这一格补上测量，格名不变。
	DeclaredMeasurementAnchoredOnAdoptedVersion
	// DeclaredMeasurementUndetermined：当前采用判断为`待复核`，本上下文此刻说不出该按哪一版，不给锚也不给值。
	DeclaredMeasurementUndetermined
	// DeclaredMeasurementNotDeclared：对象是已接受委托的成员，但接受基线那一版上没有它的画像——客户没报测量。缺席是真话，
	// 不拿同委托别的成员的测量顶，也不填默认。
	DeclaredMeasurementNotDeclared
	// NoDeclaredMeasurement：对象不属任何已接受委托的成员集合（含集运单元与不可见对象），按统一不可见结果答，不区分
	// 不存在、他租户与未授权。
	NoDeclaredMeasurement
)

func (outcome DeclaredMeasurementOutcome) String() string {
	switch outcome {
	case DeclaredMeasurementAnchoredOnBaseline:
		return "ANCHORED_ON_BASELINE"
	case DeclaredMeasurementAnchoredOnAdoptedVersion:
		return "ANCHORED_ON_ADOPTED_VERSION"
	case DeclaredMeasurementUndetermined:
		return "UNDETERMINED"
	case DeclaredMeasurementNotDeclared:
		return "NOT_DECLARED"
	case NoDeclaredMeasurement:
		return "NO_DECLARED_MEASUREMENT"
	default:
		return ""
	}
}

// DeclaredMeasurementResolution 是读口对一次（租户，包裹身份）之问的答复：五格之一；基线格附测量与基线锚，已采用版本格
// 只附锚，其余三格两样都不附。
type DeclaredMeasurementResolution struct {
	outcome     DeclaredMeasurementOutcome
	measurement DeclaredMeasurement
	anchor      SourceDataVersionAnchor
}

// DeclaredMeasurementOnBaseline 把接受基线上的一份测量包成基线锚那一格。格由构造器定死，调用方挑不了：一份基线值
// 不可能被说成「已采用版本锚」。
func DeclaredMeasurementOnBaseline(measurement DeclaredMeasurement) (DeclaredMeasurementResolution, error) {
	if !measurement.declared() {
		return DeclaredMeasurementResolution{}, ErrInvalidDeclaredMeasurementResolution
	}
	return DeclaredMeasurementResolution{
		outcome:     DeclaredMeasurementAnchoredOnBaseline,
		measurement: measurement,
		anchor:      NewAcceptanceBaselineAnchor(),
	}, nil
}

// DeclaredMeasurementOnAdoptedVersion 是已采用版本锚那一格：只收指着某一版的锚，基线锚在这里不合法——那是另一格。
func DeclaredMeasurementOnAdoptedVersion(anchor SourceDataVersionAnchor) (DeclaredMeasurementResolution, error) {
	if _, adopted := anchor.AdoptedVersion(); !adopted {
		return DeclaredMeasurementResolution{}, ErrInvalidDeclaredMeasurementResolution
	}
	return DeclaredMeasurementResolution{outcome: DeclaredMeasurementAnchoredOnAdoptedVersion, anchor: anchor}, nil
}

// DeclaredMeasurementUndeterminedResolution 是「未定」那一格。
func DeclaredMeasurementUndeterminedResolution() DeclaredMeasurementResolution {
	return DeclaredMeasurementResolution{outcome: DeclaredMeasurementUndetermined}
}

// DeclaredMeasurementNotDeclaredResolution 是「未申报」那一格。
func DeclaredMeasurementNotDeclaredResolution() DeclaredMeasurementResolution {
	return DeclaredMeasurementResolution{outcome: DeclaredMeasurementNotDeclared}
}

// NoDeclaredMeasurementResolution 是「无」那一格。
func NoDeclaredMeasurementResolution() DeclaredMeasurementResolution {
	return DeclaredMeasurementResolution{outcome: NoDeclaredMeasurement}
}

func (resolution DeclaredMeasurementResolution) Outcome() DeclaredMeasurementOutcome {
	return resolution.outcome
}

// Measurement 只在基线格在场；其余四格第二个返回值为假，不是「测量读不到」。
func (resolution DeclaredMeasurementResolution) Measurement() (DeclaredMeasurement, bool) {
	return resolution.measurement, resolution.measurement.declared()
}

// Anchor 在基线格与已采用版本格在场；其余三格第二个返回值为假。
func (resolution DeclaredMeasurementResolution) Anchor() (SourceDataVersionAnchor, bool) {
	return resolution.anchor, resolution.anchor.valid()
}

// DeclaredMeasurementFor 按包裹身份答申报测量（pp-seams/02 裁决 3 / 4 / 5）。委托与接受基线是本上下文内部走到答案
// 的路，不是键：先经接受基线成员集合确认这件包裹属于本委托，再取该包裹申报测量范围上查询时刻的资料版本锚，锚在
// 基线上才去接受基线所指那一版提交版本上取画像。
//
// 范围按包裹指名：测量是逐成员申报的（ADR-0048 画像按成员），作用于整份委托的「测量」修订今天没有产生规则，词表
// 落地那天再多问一条范围。包裹 → 委托只按声明成员走，谱系包裹落「无」，与 DeliveryPlaceReferenceFor 同一今日形状。
//
// 先看锚再看画像：基线上没画像而后来的补充被采用，答的是已采用版本格（客户补报了，内容在那一版上），不是「未申报」。
func (request ShipmentRequest) DeclaredMeasurementFor(parcel DeclaredParcelID) (DeclaredMeasurementResolution, error) {
	if request.state != ShipmentRequestAccepted || !request.baseline.covers(parcel) {
		return NoDeclaredMeasurementResolution(), nil
	}
	scope, err := NewParcelScopedSourceData(request.shipmentRequestID, parcel, DeclaredMeasurementDataGroup())
	if err != nil {
		return DeclaredMeasurementResolution{}, fmt.Errorf("declared measurement: %w", err)
	}
	resolved, err := request.resolveSourceDataAnchor(scope)
	if err != nil {
		return DeclaredMeasurementResolution{}, fmt.Errorf("declared measurement: %w", err)
	}
	if resolved.undetermined {
		return DeclaredMeasurementUndeterminedResolution(), nil
	}
	if _, adopted := resolved.anchor.AdoptedVersion(); adopted {
		return DeclaredMeasurementOnAdoptedVersion(resolved.anchor)
	}

	baselineVersion, found := request.submissionVersionByID(request.baseline.SubmissionVersionID())
	if !found {
		return DeclaredMeasurementResolution{}, fmt.Errorf("%w: 接受基线指向的提交版本 %q 不在本委托上",
			ErrInvalidDeclaredMeasurementResolution, request.baseline.SubmissionVersionID())
	}
	profile, declared := baselineVersion.ProfileFor(parcel)
	if !declared {
		return DeclaredMeasurementNotDeclaredResolution(), nil
	}
	return DeclaredMeasurementOnBaseline(profile.Measurement())
}
