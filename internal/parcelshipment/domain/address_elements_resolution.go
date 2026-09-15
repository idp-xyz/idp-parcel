package domain

import (
	"errors"
	"fmt"
)

var ErrInvalidAddressElementsResolution = errors.New("parcel shipment: invalid address elements resolution")

// AddressElementsOutcome 是「地址要素」读口每一段的封闭五格（pp-seams/03 裁决 2 / 5 与裁决 7 追裁）。形照
// DeclaredMeasurementOutcome：DeliveryPlaceOutcome 那几格答的是引用（总能派生），本口答的是内容，内容可以如实缺席，
// 「要素缺席」得自己占一格——它不是「无」（对象不是委托成员）也不是「未定」（两条修订分叉）。
//
// **今天这一格同时覆盖「条目未落库」**：提交版本今天不携带地址要素（寄收件 name/value 条目只进 PayloadDigest，不进
// 快照），所以对今天全部快照口都答 NOT_PROVIDED；「租户没报」与「本上下文没留」在代码上分不开，不预拟分不开的格。
// 内容落库归 pp-seams/05，那票落地后基线格开始带值、本格只剩「租户没报」，格名不变。
//
// 没有`不知道`格，理由同 DeliveryPlaceOutcome：走到答案要翻的册子全是本上下文自己的，答不出就是读面坏了，上抛 error。
type AddressElementsOutcome uint8

const (
	AddressElementsOutcomeInvalid AddressElementsOutcome = iota
	// AddressElementsAnchoredOnBaseline：该范围上尚无修订版本，答接受基线那一版上的要素（至少一格在场、另一格可缺如实），
	// 带接受基线锚。
	AddressElementsAnchoredOnBaseline
	// AddressElementsAnchoredOnAdoptedVersion：该范围上的当前采用判断为`已采用`，答那一版的锚——**只交锚不交要素**：
	// 客户原始资料版本今天只留痕不留内容（CustomerSourceDataVersion 是请求指纹 + 留痕清单），本上下文说不出那一版报了
	// 什么；交基线值等于把一份已被客户更正的地址当现行地址送出去。消费方拿到这一格该停在「输入不可得」。
	AddressElementsAnchoredOnAdoptedVersion
	// AddressElementsUndetermined：当前采用判断为`待复核`，本上下文此刻说不出该按哪一版，不给锚也不给值。
	AddressElementsUndetermined
	// AddressElementsNotProvided：对象是已接受委托的成员，锚在基线上，但基线那一版上两个要素都缺席——见类型头注。
	AddressElementsNotProvided
	// NoAddressElements：对象不属任何已接受委托的成员集合（含集运单元与不可见对象），按统一不可见结果答。
	NoAddressElements
)

func (outcome AddressElementsOutcome) String() string {
	switch outcome {
	case AddressElementsAnchoredOnBaseline:
		return "ANCHORED_ON_BASELINE"
	case AddressElementsAnchoredOnAdoptedVersion:
		return "ANCHORED_ON_ADOPTED_VERSION"
	case AddressElementsUndetermined:
		return "UNDETERMINED"
	case AddressElementsNotProvided:
		return "NOT_PROVIDED"
	case NoAddressElements:
		return "NO_ADDRESS_ELEMENTS"
	default:
		return ""
	}
}

// AddressElementsResolution 是读口一段（寄件或收件资料范围）的答复：五格之一；基线格附要素与基线锚，已采用版本格只附锚，
// 其余三格两样都不附。
type AddressElementsResolution struct {
	outcome  AddressElementsOutcome
	elements AddressElements
	anchor   SourceDataVersionAnchor
}

// AddressElementsOnBaseline 把接受基线上的一组要素包成基线锚那一格。两个要素都缺的不是基线格，是「要素缺席」——构造器拒，
// 调用方挑不了格。
func AddressElementsOnBaseline(elements AddressElements) (AddressElementsResolution, error) {
	if elements.Empty() {
		return AddressElementsResolution{}, ErrInvalidAddressElementsResolution
	}
	return AddressElementsResolution{
		outcome:  AddressElementsAnchoredOnBaseline,
		elements: elements,
		anchor:   NewAcceptanceBaselineAnchor(),
	}, nil
}

// AddressElementsOnAdoptedVersion 是已采用版本锚那一格：只收指着某一版的锚，基线锚在这里不合法——那是另一格。
func AddressElementsOnAdoptedVersion(anchor SourceDataVersionAnchor) (AddressElementsResolution, error) {
	if _, adopted := anchor.AdoptedVersion(); !adopted {
		return AddressElementsResolution{}, ErrInvalidAddressElementsResolution
	}
	return AddressElementsResolution{outcome: AddressElementsAnchoredOnAdoptedVersion, anchor: anchor}, nil
}

// AddressElementsUndeterminedResolution 是「未定」那一格。
func AddressElementsUndeterminedResolution() AddressElementsResolution {
	return AddressElementsResolution{outcome: AddressElementsUndetermined}
}

// AddressElementsNotProvidedResolution 是「要素缺席」那一格。
func AddressElementsNotProvidedResolution() AddressElementsResolution {
	return AddressElementsResolution{outcome: AddressElementsNotProvided}
}

// NoAddressElementsResolution 是「无」那一格。
func NoAddressElementsResolution() AddressElementsResolution {
	return AddressElementsResolution{outcome: NoAddressElements}
}

func (resolution AddressElementsResolution) Outcome() AddressElementsOutcome {
	return resolution.outcome
}

// Elements 只在基线格在场；其余四格第二个返回值为假，不是「要素读不到」。在场的那一组里仍可能有一格缺席，逐格问它自己。
func (resolution AddressElementsResolution) Elements() (AddressElements, bool) {
	return resolution.elements, resolution.outcome == AddressElementsAnchoredOnBaseline
}

// Anchor 在基线格与已采用版本格在场；其余三格第二个返回值为假。
func (resolution AddressElementsResolution) Anchor() (SourceDataVersionAnchor, bool) {
	return resolution.anchor, resolution.anchor.valid()
}

// ShipmentAddressElements 是读口对一次（租户，包裹身份）之问的答复：起点与目的两段各自成格（03 裁决 2「一段有一段无是
// 常态」）。起点是寄件资料范围上的要素——PS 只答自己有的（寄件人邮编）；起点为节点邮编时的读口归 NR / NO（裁决 3）。
type ShipmentAddressElements struct {
	origin      AddressElementsResolution
	destination AddressElementsResolution
}

// NoShipmentAddressElements 是两段皆「无」的答复：对象不属任何已接受委托的成员集合。
func NoShipmentAddressElements() ShipmentAddressElements {
	return ShipmentAddressElements{origin: NoAddressElementsResolution(), destination: NoAddressElementsResolution()}
}

// Origin 是寄件资料范围（SenderPlaceDataGroup）那一段。
func (answer ShipmentAddressElements) Origin() AddressElementsResolution {
	return answer.origin
}

// Destination 是收件资料范围（DeliveryPlaceDataGroup）那一段。
func (answer ShipmentAddressElements) Destination() AddressElementsResolution {
	return answer.destination
}

// AddressElementsFor 按包裹身份答寄件与收件两段地址要素（pp-seams/03 裁决 2 / 3 / 4）。委托与接受基线是本上下文内部走到
// 答案的路，不是键：先经接受基线成员集合确认这件包裹属于本委托，再对寄件、收件两个委托级范围各解析一次资料版本锚（与
// DeliveryPlaceReferenceFor / DeclaredMeasurementFor 同一个共用内部步骤），锚在基线上才去接受基线所指那一版提交版本上取要素。
//
// 两段范围都是委托级：CONTEXT 硬句「同一委托中的包裹必须共享……寄收件关系」，寄收件资料按委托级范围形成，与收件地点引用
// 同一取法。不新造「寄件地点引用」：寄件那一段只交值不交地点（裁决 2）。包裹 → 委托只按声明成员走，谱系包裹落「无」。
func (request ShipmentRequest) AddressElementsFor(parcel DeclaredParcelID) (ShipmentAddressElements, error) {
	if request.state != ShipmentRequestAccepted || !request.baseline.covers(parcel) {
		return NoShipmentAddressElements(), nil
	}
	baselineVersion, found := request.submissionVersionByID(request.baseline.SubmissionVersionID())
	if !found {
		return ShipmentAddressElements{}, fmt.Errorf("%w: 接受基线指向的提交版本 %q 不在本委托上",
			ErrInvalidAddressElementsResolution, request.baseline.SubmissionVersionID())
	}
	origin, err := request.addressElementsInGroup(baselineVersion, SenderPlaceDataGroup())
	if err != nil {
		return ShipmentAddressElements{}, err
	}
	destination, err := request.addressElementsInGroup(baselineVersion, DeliveryPlaceDataGroup())
	if err != nil {
		return ShipmentAddressElements{}, err
	}
	return ShipmentAddressElements{origin: origin, destination: destination}, nil
}

// addressElementsInGroup 答一个资料范围那一段：先锚后内容——基线上没要素而后来的补充被采用，答的是已采用版本格（客户补报了，
// 内容在那一版上），不是「要素缺席」。
func (request ShipmentRequest) addressElementsInGroup(
	baselineVersion SubmissionVersion,
	group SourceDataGroupReference,
) (AddressElementsResolution, error) {
	scope, err := NewShipmentScopedSourceData(request.shipmentRequestID, group)
	if err != nil {
		return AddressElementsResolution{}, fmt.Errorf("address elements: %w", err)
	}
	resolved, err := request.resolveSourceDataAnchor(scope)
	if err != nil {
		return AddressElementsResolution{}, fmt.Errorf("address elements: %w", err)
	}
	if resolved.undetermined {
		return AddressElementsUndeterminedResolution(), nil
	}
	if _, adopted := resolved.anchor.AdoptedVersion(); adopted {
		return AddressElementsOnAdoptedVersion(resolved.anchor)
	}
	elements := baselineVersion.addressElements(group)
	if elements.Empty() {
		return AddressElementsNotProvidedResolution(), nil
	}
	return AddressElementsOnBaseline(elements)
}

// addressElements 交回本提交版本上某个资料范围的地址要素。**今天恒为缺席**：提交版本不携带寄收件条目（它们只进
// PayloadDigest），本方法是 pp-seams/05「提交版本留内容」落地时接内容的那一处——那票让版本带上按封闭要素名挑出的子段后，
// 这里改成从子段读，读口与答格不动。
func (version SubmissionVersion) addressElements(SourceDataGroupReference) AddressElements {
	return AddressElements{}
}
