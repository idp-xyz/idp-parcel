package domain

import "fmt"

// 本文件是货物明细按资料版本锚的读法（ADR-0373 决定九，票 declared-goods-lines/07）：关务形成正式申报资料时按（资料版本锚，
// 委托，行号）逐行溯源。客户原始资料版本不可覆盖，所以锚一旦记下，按它读回的永远是同一份行——行号是版本内的序号，换了版本
// 就是另一组行，引用本身因此稳定。

// GoodsDetailsResolution 是货物明细读口的答复：锚在场时附那一版的货物明细（可以如实缺席：显式清空、或首次提交就没报）；两条
// 修订分叉时 undetermined 为真，不给锚也不给货物明细。两格都附接受基线的成员：没报装箱分配时，单件委托的那一件装的就是
// 全部行，多件委托的整票清单却说不出哪件装了什么（ADR-0373 决定三）——不交成员，读的人分不开这两种情形。
type GoodsDetailsResolution struct {
	anchor       SourceDataVersionAnchor
	goods        GoodsDetails
	members      []DeclaredParcelID
	undetermined bool
}

// GoodsDetailsAnchoredAt 是「按这一版」那一格。基线锚与版本锚都收：货物明细的读口答的是「这一版上有什么」，基线那一版与
// 修订的那一版同样是一版——这与 DeclaredMeasurementOnAdoptedVersion 只收版本锚不同，那一口的基线格另有自己的构造器。锚
// 立不起来拒；货物明细是那一版的内容、可以缺席。
func GoodsDetailsAnchoredAt(anchor SourceDataVersionAnchor, goods GoodsDetails, members []DeclaredParcelID) (GoodsDetailsResolution, error) {
	if !anchor.valid() {
		return GoodsDetailsResolution{}, ErrInvalidSourceDataVersionAnchor
	}
	return GoodsDetailsResolution{anchor: anchor, goods: goods, members: append([]DeclaredParcelID(nil), members...)}, nil
}

// GoodsDetailsUndeterminedResolution 是「待复核」那一格。
func GoodsDetailsUndeterminedResolution(members []DeclaredParcelID) GoodsDetailsResolution {
	return GoodsDetailsResolution{undetermined: true, members: append([]DeclaredParcelID(nil), members...)}
}

// Anchor 的第二个返回值为假即待复核，此刻说不出该按哪一版。
func (resolution GoodsDetailsResolution) Anchor() (SourceDataVersionAnchor, bool) {
	return resolution.anchor, resolution.anchor.valid()
}

func (resolution GoodsDetailsResolution) Goods() GoodsDetails {
	return resolution.goods
}

// Members 是接受基线的成员，按基线的次序；锚换了它也不变——资料修订不增删成员。
func (resolution GoodsDetailsResolution) Members() []DeclaredParcelID {
	return append([]DeclaredParcelID(nil), resolution.members...)
}

func (resolution GoodsDetailsResolution) Undetermined() bool {
	return resolution.undetermined
}

// CurrentGoodsDetails 答这份委托货物明细范围上此刻的锚与那一版的货物明细。锚由共用步骤 resolveSourceDataAnchor 解析，
// 采用判断与 GoodsDetailsInEffect 同出一段。还没接受的委托答 false：资料版本锚在接受之后才有意义。
func (request ShipmentRequest) CurrentGoodsDetails() (GoodsDetailsResolution, bool, error) {
	if request.state != ShipmentRequestAccepted {
		return GoodsDetailsResolution{}, false, nil
	}
	scope, err := NewShipmentScopedSourceData(request.shipmentRequestID, GoodsDetailsDataGroup())
	if err != nil {
		return GoodsDetailsResolution{}, false, fmt.Errorf("current goods details: %w", err)
	}
	resolved, err := request.resolveSourceDataAnchor(scope)
	if err != nil {
		return GoodsDetailsResolution{}, false, fmt.Errorf("current goods details: %w", err)
	}
	if resolved.undetermined {
		return GoodsDetailsUndeterminedResolution(request.baseline.DeclaredParcelIDs()), true, nil
	}
	resolution, found := request.GoodsDetailsResolutionAt(resolved.anchor)
	if !found {
		return GoodsDetailsResolution{}, false, fmt.Errorf("current goods details: 采用判断指的那一版不在本委托的货物明细范围上")
	}
	return resolution, true, nil
}

// GoodsDetailsResolutionAt 是 GoodsDetailsAt 带上锚与基线成员的形，读口交给消费方的就是它。
func (request ShipmentRequest) GoodsDetailsResolutionAt(anchor SourceDataVersionAnchor) (GoodsDetailsResolution, bool) {
	goods, found := request.GoodsDetailsAt(anchor)
	if !found {
		return GoodsDetailsResolution{}, false
	}
	resolution, err := GoodsDetailsAnchoredAt(anchor, goods, request.baseline.DeclaredParcelIDs())
	return resolution, err == nil
}

// GoodsDetailsAt 按一枚资料版本锚答货物明细：基线锚答接受基线那一版提交版本上的货物明细；版本锚答那一版资料修订自己的内容，
// 那一版得在本委托的货物明细范围上——别的资料组的版本不带货物明细，按它答「没报」是说错话，答不可见。还没接受的委托一律
// 不可见。第二个返回值为真时货物明细仍可缺席：那一版就是这么说的。
//
// 版本锚只认客户原始资料版本的标识：提交版本的标识不是版本锚，接受基线那一版只有基线锚一种写法。采用判断允许更正以那一版
// 提交版本为基准（adoptionAmong 的链头），那是基准的写法，不是锚的写法——锚给两种拼法，同一版就会被记成两个引用。
//
// 锚不随采用判断漂移：一枚已被后来修订顶掉的版本锚照样答它那一版，关务当初按哪一版形成的就读得回哪一版。
func (request ShipmentRequest) GoodsDetailsAt(anchor SourceDataVersionAnchor) (GoodsDetails, bool) {
	if request.state != ShipmentRequestAccepted || !anchor.valid() {
		return GoodsDetails{}, false
	}
	if anchor.OnAcceptanceBaseline() {
		version, found := request.submissionVersionByID(request.baseline.SubmissionVersionID())
		if !found {
			return GoodsDetails{}, false
		}
		return version.goods, true
	}
	versionID, _ := anchor.AdoptedVersion()
	version, found := request.sourceDataVersionByID(versionID)
	if !found || version.scope.DataGroup() != GoodsDetailsDataGroup() {
		return GoodsDetails{}, false
	}
	goods, _ := version.content.Goods()
	return goods, true
}
