package domain

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var (
	ErrInvalidDeliveryPlaceReference  = errors.New("parcel shipment: invalid delivery place reference")
	ErrInvalidSourceDataVersionAnchor = errors.New("parcel shipment: invalid source data version anchor")
)

// deliveryPlaceReferenceShapeVersion 标识规范串的形状（ADR-0014 先例 `PSC-1:`）：TF 只当不透明串
// 存进派送任务，日后寄收件范围拿到模型（PSC-2）、范围段从资料组引用换成槽位引用时，换号不改旧串，
// 旧串仍按 DPR-1 解读。
const deliveryPlaceReferenceShapeVersion = "DPR-1"

// deliveryPlaceDataGroupWord 是本上下文对「收件」资料组的自有原词（票 ps-port-remainder/06 要落的第 4 条）。
// `SourceDataGroupReference` 是开放引用（ADR-0120 决定七），矩阵登记侧（`PAR-COM-13`）用什么词是实例
// 半边；引用里要能写出这一段，机制半边就得有一个词。两侧对不上时读口按这个词查，查不到修订版本就答
// 基线锚——那是如实答案（本上下文没见过那个范围上的修订），不是默认。
const deliveryPlaceDataGroupWord = "DELIVERY_PLACE"

// DeliveryPlaceDataGroup 交回收件资料组的原词引用。
func DeliveryPlaceDataGroup() SourceDataGroupReference {
	return SourceDataGroupReference{requiredValue{value: deliveryPlaceDataGroupWord}}
}

// SourceDataVersionAnchor 是收件地点引用钉住的资料版本锚，两态与 SourceDataBasis 同形：接受基线（该范围
// 上尚无修订版本）或当前采用的那份客户原始资料版本（ADR-0130 决定一）。
//
// 另立一型而不复用 SourceDataBasis：那个类型说的是「本次修订依据什么」，构造器叫
// NewSupplementOnAcceptanceBaseline / NewAmendmentOfVersion——一个锚借它的名字读出来就成了一次修订。
// 同形不同义的两样东西合用一型，调用点把「修订所依据的前版」当「引用所钉的当前版」传过去编译器不响，
// 本包 DeclaredAsOf 与 EchoedAsOfPolicy 分型守的是同一条。
//
// 零值不是锚：它会让「钉在基线上」与「没钉」变成同一个值。
type SourceDataVersionAnchor struct {
	version    SourceDataVersionID
	onBaseline bool
}

// NewAcceptanceBaselineAnchor 是「该范围上没有任何修订版本」时的锚：收件资料只在接受基线快照里。
func NewAcceptanceBaselineAnchor() SourceDataVersionAnchor {
	return SourceDataVersionAnchor{onBaseline: true}
}

// NewSourceDataVersionAnchor 钉住当前采用的那一版客户原始资料版本。
func NewSourceDataVersionAnchor(version SourceDataVersionID) (SourceDataVersionAnchor, error) {
	if !version.valid() {
		return SourceDataVersionAnchor{}, ErrInvalidSourceDataVersionAnchor
	}
	return SourceDataVersionAnchor{version: version}, nil
}

func (anchor SourceDataVersionAnchor) OnAcceptanceBaseline() bool {
	return anchor.onBaseline
}

// AdoptedVersion 的第二个返回值为假时表示锚在接受基线上，而不是「读不到版本」。
func (anchor SourceDataVersionAnchor) AdoptedVersion() (SourceDataVersionID, bool) {
	return anchor.version, anchor.version.valid()
}

func (anchor SourceDataVersionAnchor) valid() bool {
	return anchor.version.valid() != anchor.onBaseline
}

// DeliveryPlaceReference 是本上下文为一份已接受委托的收件资料范围对外交出的复合引用（PS CONTEXT
// 「收件地点引用」；ADR-0130 决定一），四段：租户、委托、收件资料范围、资料版本锚。委托由范围携带
// （SourceDataScope 委托必填），构造时不另收一份——收两份就得校它们相等，而相等之外没有第二种合法值。
//
// 它不另铸地点身份、不是目的地节点、不含一字地址内容：收件地址的版本就是客户原始资料版本，引用只
// 指向它，旧锚永远解析到那一版内容。
type DeliveryPlaceReference struct {
	tenant TenantID
	scope  SourceDataScope
	anchor SourceDataVersionAnchor
}

func NewDeliveryPlaceReference(
	tenant TenantID,
	scope SourceDataScope,
	anchor SourceDataVersionAnchor,
) (DeliveryPlaceReference, error) {
	reference := DeliveryPlaceReference{tenant: tenant, scope: scope, anchor: anchor}
	if !reference.valid() {
		return DeliveryPlaceReference{}, ErrInvalidDeliveryPlaceReference
	}
	return reference, nil
}

func (reference DeliveryPlaceReference) TenantID() TenantID {
	return reference.tenant
}

func (reference DeliveryPlaceReference) ShipmentRequestID() ShipmentRequestID {
	return reference.scope.shipmentRequestID
}

// Scope 是引用指向的收件资料范围，可直接送进 CurrentSourceDataAdoption——「按锚解析回地址内容」
// 的第二个消费方（ADR-0130 Consequences）拿它就能走到那一版。
func (reference DeliveryPlaceReference) Scope() SourceDataScope {
	return reference.scope
}

func (reference DeliveryPlaceReference) Anchor() SourceDataVersionAnchor {
	return reference.anchor
}

// String 交出规范串：`DPR-1:<租户>/<委托>/<范围>/<锚>`。范围段是资料组，指名了包裹则 `<资料组>;<包裹>`；
// 锚段是字面 `baseline` 或 `version;<版本>`。每一段的内容按 URL 路径段转义（`/` `;` 与非保留字符一律
// 进 `%XX`，大写十六进制），分隔符因此在段内不可能出现，段数与子段数就是结构本身，不必另带长度或引号。
// 四段之外不多一字；零值引用交回空串，与本包其余值对象同款。
func (reference DeliveryPlaceReference) String() string {
	if !reference.valid() {
		return ""
	}
	scope := url.PathEscape(reference.scope.dataGroup.String())
	if parcel, named := reference.scope.DeclaredParcelID(); named {
		scope += deliveryPlaceSubSegmentSeparator + url.PathEscape(parcel.String())
	}
	anchor := deliveryPlaceBaselineAnchorWord
	if version, adopted := reference.anchor.AdoptedVersion(); adopted {
		anchor = deliveryPlaceVersionAnchorWord + deliveryPlaceSubSegmentSeparator + url.PathEscape(version.String())
	}
	return deliveryPlaceReferenceShapeVersion + ":" + strings.Join([]string{
		url.PathEscape(reference.tenant.String()),
		url.PathEscape(reference.scope.shipmentRequestID.String()),
		scope,
		anchor,
	}, deliveryPlaceSegmentSeparator)
}

const (
	deliveryPlaceSegmentSeparator    = "/"
	deliveryPlaceSubSegmentSeparator = ";"
	deliveryPlaceBaselineAnchorWord  = "baseline"
	deliveryPlaceVersionAnchorWord   = "version"
)

// ParseDeliveryPlaceReference 是规范串的重建门（ADR-0028）：只收本类型自己渲染得出的那一种写法。
//
// 解出四段、逐段过领域构造函数之后，再把重建结果渲染一遍与入串逐字比——不等就拒，不做规范化。
// 一个串换个转义大小写、多转义一个字母，语义相同而字节不同；收下它，TF 拿两个引用比「地址内容变了
// 没有」就会把同一个引用比成两个。ErrInvalidDeliveryPlaceReference 一格覆盖全部拒绝理由，错误文本里
// 说明是哪一段。
func ParseDeliveryPlaceReference(canonical string) (DeliveryPlaceReference, error) {
	body, prefixed := strings.CutPrefix(canonical, deliveryPlaceReferenceShapeVersion+":")
	if !prefixed {
		return DeliveryPlaceReference{}, fmt.Errorf("%w: 缺形状版本前缀 %s:", ErrInvalidDeliveryPlaceReference, deliveryPlaceReferenceShapeVersion)
	}
	segments := strings.Split(body, deliveryPlaceSegmentSeparator)
	if len(segments) != 4 {
		return DeliveryPlaceReference{}, fmt.Errorf("%w: 要四段，得到 %d 段", ErrInvalidDeliveryPlaceReference, len(segments))
	}

	tenantValue, err := url.PathUnescape(segments[0])
	if err != nil {
		return DeliveryPlaceReference{}, fmt.Errorf("%w: 租户段：%v", ErrInvalidDeliveryPlaceReference, err)
	}
	tenant, err := NewTenantID(tenantValue)
	if err != nil {
		return DeliveryPlaceReference{}, fmt.Errorf("%w: 租户段：%v", ErrInvalidDeliveryPlaceReference, err)
	}
	requestValue, err := url.PathUnescape(segments[1])
	if err != nil {
		return DeliveryPlaceReference{}, fmt.Errorf("%w: 委托段：%v", ErrInvalidDeliveryPlaceReference, err)
	}
	requestID, err := NewShipmentRequestID(requestValue)
	if err != nil {
		return DeliveryPlaceReference{}, fmt.Errorf("%w: 委托段：%v", ErrInvalidDeliveryPlaceReference, err)
	}
	scope, err := parseDeliveryPlaceScopeSegment(requestID, segments[2])
	if err != nil {
		return DeliveryPlaceReference{}, err
	}
	anchor, err := parseDeliveryPlaceAnchorSegment(segments[3])
	if err != nil {
		return DeliveryPlaceReference{}, err
	}

	reference, err := NewDeliveryPlaceReference(tenant, scope, anchor)
	if err != nil {
		return DeliveryPlaceReference{}, err
	}
	if reference.String() != canonical {
		return DeliveryPlaceReference{}, fmt.Errorf("%w: 非规范串，重建后渲染为 %q", ErrInvalidDeliveryPlaceReference, reference.String())
	}
	return reference, nil
}

func parseDeliveryPlaceScopeSegment(requestID ShipmentRequestID, segment string) (SourceDataScope, error) {
	parts := strings.Split(segment, deliveryPlaceSubSegmentSeparator)
	if len(parts) > 2 {
		return SourceDataScope{}, fmt.Errorf("%w: 范围段最多两个子段，得到 %d 个", ErrInvalidDeliveryPlaceReference, len(parts))
	}
	groupValue, err := url.PathUnescape(parts[0])
	if err != nil {
		return SourceDataScope{}, fmt.Errorf("%w: 范围段：%v", ErrInvalidDeliveryPlaceReference, err)
	}
	group, err := NewSourceDataGroupReference(groupValue)
	if err != nil {
		return SourceDataScope{}, fmt.Errorf("%w: 范围段：%v", ErrInvalidDeliveryPlaceReference, err)
	}
	if len(parts) == 1 {
		scope, err := NewShipmentScopedSourceData(requestID, group)
		if err != nil {
			return SourceDataScope{}, fmt.Errorf("%w: 范围段：%v", ErrInvalidDeliveryPlaceReference, err)
		}
		return scope, nil
	}
	parcelValue, err := url.PathUnescape(parts[1])
	if err != nil {
		return SourceDataScope{}, fmt.Errorf("%w: 范围段包裹：%v", ErrInvalidDeliveryPlaceReference, err)
	}
	parcel, err := NewDeclaredParcelID(parcelValue)
	if err != nil {
		return SourceDataScope{}, fmt.Errorf("%w: 范围段包裹：%v", ErrInvalidDeliveryPlaceReference, err)
	}
	scope, err := NewParcelScopedSourceData(requestID, parcel, group)
	if err != nil {
		return SourceDataScope{}, fmt.Errorf("%w: 范围段：%v", ErrInvalidDeliveryPlaceReference, err)
	}
	return scope, nil
}

func parseDeliveryPlaceAnchorSegment(segment string) (SourceDataVersionAnchor, error) {
	if segment == deliveryPlaceBaselineAnchorWord {
		return NewAcceptanceBaselineAnchor(), nil
	}
	versionValue, prefixed := strings.CutPrefix(segment, deliveryPlaceVersionAnchorWord+deliveryPlaceSubSegmentSeparator)
	if !prefixed {
		return SourceDataVersionAnchor{}, fmt.Errorf("%w: 锚段既不是 %s 也不是 %s;<版本>", ErrInvalidDeliveryPlaceReference, deliveryPlaceBaselineAnchorWord, deliveryPlaceVersionAnchorWord)
	}
	unescaped, err := url.PathUnescape(versionValue)
	if err != nil {
		return SourceDataVersionAnchor{}, fmt.Errorf("%w: 锚段：%v", ErrInvalidDeliveryPlaceReference, err)
	}
	version, err := NewSourceDataVersionID(unescaped)
	if err != nil {
		return SourceDataVersionAnchor{}, fmt.Errorf("%w: 锚段：%v", ErrInvalidDeliveryPlaceReference, err)
	}
	anchor, err := NewSourceDataVersionAnchor(version)
	if err != nil {
		return SourceDataVersionAnchor{}, fmt.Errorf("%w: 锚段：%v", ErrInvalidDeliveryPlaceReference, err)
	}
	return anchor, nil
}

func (reference DeliveryPlaceReference) valid() bool {
	return reference.tenant.valid() && reference.scope.valid() && reference.anchor.valid()
}
