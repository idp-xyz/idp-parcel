package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 派送要求的三条消费侧端口（ADR-0114 决定三；CONTEXT「派送要求」）。形成一项末端派送任务的七件里，地点、时间窗、
// 条件三件不在本上下文：收件地点归 parcel-shipment、计划履约段时间窗口归 network-routing、交付条件归
// party-commercial。三条各一个端口而不是一个，因为它们各有所有者、各自成票（tf-segment-lifecycle-closure/12–14）、
// 各自可以先后接线；触发执行器对每一条未接线的缝各答一格，不把三条并成一句「要求未就位」。
//
// **拉的是引用与时间窗，不拉地址本体。** 地点端口交回的是收件地点引用（parcel-shipment 拥有的身份），落进任务的
// `Place` 也是引用；地址本体留在所有者那里，需要它的一线作业端按引用向所有者取（ADR-0075 要防的那一半在这里守住）。
//
// 适配器归 `internal/transportfulfillment/adapters/<所有者>`（ADR-0025 消费侧），本包只立形状。
//
// **三条缝都已各自接线。** 地点：`adapters/parcelshipment.DeliveryPlaceSource`（票 tf-segment-lifecycle-closure/12，按 ADR-0130
// 的答复落）；时间窗：`adapters/networkrouting.DeliveryWindows`（票 13，按 ADR-0131 三问的答复落）；条件：
// `adapters/partycommercial.DeliveryConditionSource`（票 14，按 ADR-0133 的答复落，跨 parcel-shipment 与 party-commercial 两个
// 提供方）。生产装配在 `cmd/parcel-api` 的 buildDeliveryDispatchTrigger 三格全填。`TriggerDeliveryDispatchHandler` 对留空的格
// 仍答对应的 *_SOURCE_NOT_WIRED——那一格如今只在装配漏接时出现，是 ADR-0114 决定三要的诚实停点，不许用替身或默认值补齐；
// 所有者答「没有」照 RequirementMissing 落 REQUIREMENT_MISSING，任务保持待形成。

// RequirementResolution 是一条派送要求端口的答法：所有者给了、所有者说没有。「没有」是业务答案不是错误——
// 对象没有计划段就没有时间窗，合同没登交付条件就没有条件；任务保持待形成，不填默认。
type RequirementResolution uint8

const (
	RequirementResolutionInvalid RequirementResolution = iota
	RequirementResolved
	RequirementMissing
)

func (resolution RequirementResolution) String() string {
	switch resolution {
	case RequirementResolved:
		return "RESOLVED"
	case RequirementMissing:
		return "MISSING"
	default:
		return ""
	}
}

// DeliveryPlaceSource 按载运对象取收件地点引用（parcel-shipment）。
type DeliveryPlaceSource interface {
	LoadDeliveryPlace(
		ctx context.Context,
		tenant domain.TenantID,
		object domain.CarriedObjectReference,
	) (place string, resolution RequirementResolution, err error)
}

// DeliveryWindowSource 按计划履约段引用取那一段的计划时间窗口（network-routing；ADR-0131 决定一）。窗口是计划，任务
// 照抄它作工作范围，不据它推任何实际事实（ADR-0004）。
//
// 按参与关系上登记方关联的引用问、不按对象问：NR 计划里没有服务动作，按对象问要 NR 推「哪一段是派送段」，ADR-0114
// 决定一禁止那种推法；对象在执行哪条段本来就是登记方声明的事实。对象入参因此不保留——NR 侧不需要它，留着只会引诱
// 适配器拿它去推。present=false 即对象没有计划段（待路由产品此刻还没有），也交给适配器：由它答 MISSING、不出本上下文，
// NR 不给兜底窗口（决定三）；引用所钉那一版此刻的适用性不折进答案（决定二）。引用是 NR 一处定义的不透明拼写，本
// 上下文只搬运不解读，拆与核都在适配器那一侧。
type DeliveryWindowSource interface {
	LoadDeliveryWindow(
		ctx context.Context,
		tenant domain.TenantID,
		planned domain.PlannedSegmentReference,
		present bool,
	) (from, to time.Time, resolution RequirementResolution, err error)
}

// DeliveryConditionSource 按载运对象取交付条件引用（party-commercial：交付方式、收件范围、合同责任的版本引用）。
type DeliveryConditionSource interface {
	LoadDeliveryConditions(
		ctx context.Context,
		tenant domain.TenantID,
		object domain.CarriedObjectReference,
	) (conditions string, resolution RequirementResolution, err error)
}
