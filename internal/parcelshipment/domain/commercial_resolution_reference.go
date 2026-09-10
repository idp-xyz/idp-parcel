package domain

import (
	"errors"
	"fmt"
)

// ErrAcceptedWithoutCommercialResolution 表示一份`已接受`委托的接受决定上没有商业解析回指。这是接受流的
// 装配缺陷不是业务答案（ADR-0062 决定三「闭包在场却未采用规则包 → error」同一判据）：折成「没有」会让
// 消费方把一次坏装配读成「所有者说这个对象没有交付条件」，任务永远停在待形成而原因不可见。
var ErrAcceptedWithoutCommercialResolution = errors.New(
	"parcel shipment: accepted shipment request carries no commercial resolution reference")

// CommercialResolutionReferenceFor 按声明包裹答「商业解析回指」——委托接受时固定在接受决定上的
// CommercialResolutionID（ADR-0133 决定一 / 二；TF 交付条件缝里对象走到合同的那一跳）。
//
// 三格：回指（`已接受`且接受决定带解析标识，第二个返回值为真）/ 没有（对象不属本委托接受基线的成员集合，
// 或委托未接受——决定前的商业依据是候选，不是固定下来的引用；第二个返回值为假、error 为空）/ error（`已接受`
// 而接受决定缺回指，见 ErrAcceptedWithoutCommercialResolution）。没有`不知道`格：册子都是本上下文自己的。
//
// 包裹 → 委托与 DeliveryPlaceReferenceFor 同一条路：只经接受基线的声明成员（AcceptanceBaseline.covers）；包裹
// 身份谱系今天领域里没有模型，谱系包裹落「没有」，是「谱系未建模」的今日形状，谱系落地那票要补这一路。
// 两问各自一个方法而不是合成一问带两个答案：ADR-0130 与 0133 都写了读口不预设按委托或按引用反查——合成
// 一个宽口就是在预设消费方同时要两样。
//
// 交回的就是 CommercialResolutionID 本身，不拆、不拼合同版本：「对象/版本」两段式只许 party-commercial 一处
// 拼（ADR-0080 决定七）；它也不是能力凭证（ADR-0062 决定三 / ADR-0003），租户由调用方给。资料修订不动它——
// 回指是接受时固定的，客户原始资料版本再多也不换合同。
func (request ShipmentRequest) CommercialResolutionReferenceFor(parcel DeclaredParcelID) (CommercialResolutionID, bool, error) {
	if request.state != ShipmentRequestAccepted || !request.baseline.covers(parcel) {
		return CommercialResolutionID{}, false, nil
	}
	decision, present := request.AcceptanceDecision()
	if !present || !decision.basis.resolutionID.valid() {
		return CommercialResolutionID{}, false, fmt.Errorf("%w: shipment request %s",
			ErrAcceptedWithoutCommercialResolution, request.shipmentRequestID)
	}
	return decision.basis.resolutionID, true, nil
}
