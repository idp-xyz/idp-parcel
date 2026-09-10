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
// **今天三条缝一条都没接。** 本包之外没有任何类型实现这三个接口（测试替身除外）：parcel-shipment 那一侧读哪个读面、
// network-routing 那一侧对没有计划段的对象怎么答、party-commercial 那一侧条件引用指哪一版，各在票
// tf-segment-lifecycle-closure/12–14 里与所有者对齐后才落适配器。生产装配把这三格留空，`TriggerDeliveryDispatchHandler`
// 于是答 DELIVERY_PLACE_SOURCE_NOT_WIRED 停在第一条缝上——那是 ADR-0114 决定三要的诚实停点，不是缺陷，也不许用替身或
// 默认值补齐。哪一票先接上线，执行器就往下走一格。

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

// DeliveryWindowSource 取计划履约段的时间窗口（network-routing）。窗口是计划，任务照抄它作工作范围，
// 不据它推任何实际事实（ADR-0004）。
//
// 三步法的 expand 段（票 tf-segment-lifecycle-closure/13 做法第 2 步）：按对象的旧法与按计划履约段引用的新法并存，
// 直到执行器迁到新法、contract 段删旧并把新法改回 LoadDeliveryWindow 这个名字。
type DeliveryWindowSource interface {
	LoadDeliveryWindow(
		ctx context.Context,
		tenant domain.TenantID,
		object domain.CarriedObjectReference,
	) (from, to time.Time, resolution RequirementResolution, err error)
	// LoadDeliveryWindowByPlannedSegment 按参与关系上登记方关联的计划履约段引用取（ADR-0131 决定一：NR 计划里没有服务
	// 动作，按对象问要 NR 推「哪一段是派送段」，ADR-0114 决定一禁止那种推法）。present=false 即对象没有计划段，由适配器
	// 答 MISSING、不出本上下文（ADR-0131 决定三）；引用所钉那一版的适用性不折进答案（决定二）。
	LoadDeliveryWindowByPlannedSegment(
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
