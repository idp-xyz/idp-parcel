package domain

import (
	"errors"
	"fmt"
)

// ErrUnanchoredSourceDataAdoption 表示某个资料范围上的当前采用判断长出了本包不认识的取值，没有对应的锚或答格。
// CONTEXT 的封闭集合还有三项没有产生规则，它们落地那天要在各读口各配一格，而不是被默认成基线锚。
var ErrUnanchoredSourceDataAdoption = errors.New("parcel shipment: source data adoption has no anchor")

// sourceDataAnchorResolution 是「按资料范围解析查询时刻的资料版本锚」这一共用内部步骤的答复：
// 锚（基线锚或已采用版本锚）在场，或 `待复核` 时 undetermined 为真、锚为零值。两格互斥，由 resolveSourceDataAnchor
// 一处产出。
type sourceDataAnchorResolution struct {
	anchor       SourceDataVersionAnchor
	undetermined bool
}

// resolveSourceDataAnchor 是按（租户，包裹身份）答问、答案钉在资料版本锚上的读口（DeliveryPlaceReferenceFor、
// DeclaredMeasurementFor，以及后继照同一锚法开的口）走到锚的那一段共用路（pp-seams/02 裁决 3 与 03 裁决 2 都写
// 「作者可抽一个共用内部步骤……对外仍一口一问」）：范围上没有版本答接受基线锚，当前采用判断为`已采用`答那一版
// 的锚，`待复核`答未定。它只解析锚，不知道范围里装的是什么、也不解释任何一格；各读口各带各的范围来问，再把三态
// 译成自己的封闭答格。
//
// 派生规则只有一处：当前采用判断由 CurrentSourceDataAdoption 给出，这里只译不重算。
func (request ShipmentRequest) resolveSourceDataAnchor(scope SourceDataScope) (sourceDataAnchorResolution, error) {
	judgment, derived := request.CurrentSourceDataAdoption(scope)
	if !derived {
		return sourceDataAnchorResolution{anchor: NewAcceptanceBaselineAnchor()}, nil
	}
	switch judgment.Outcome() {
	case SourceDataAdopted:
		adopted, _ := judgment.AdoptedVersion()
		anchor, err := NewSourceDataVersionAnchor(adopted)
		if err != nil {
			return sourceDataAnchorResolution{}, err
		}
		return sourceDataAnchorResolution{anchor: anchor}, nil
	case SourceDataAwaitingReview:
		return sourceDataAnchorResolution{undetermined: true}, nil
	default:
		return sourceDataAnchorResolution{}, fmt.Errorf("%w: 采用判断 %q", ErrUnanchoredSourceDataAdoption, judgment.Outcome())
	}
}
