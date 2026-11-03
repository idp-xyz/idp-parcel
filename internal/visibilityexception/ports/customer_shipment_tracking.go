package ports

import (
	"context"
	"errors"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// CurrentCustomerViewReader 是客户追踪查询读视图的那一口：只读当前版。查询不触发派生、披露或通知（CONTEXT：「客户读取或门户
// 展示→形成查询/展示结果，不自动形成异常披露决定、主动通知、送达或客户确认」），所以查询接的是存储读面，不接派生编排；
// CustomerViewStore 满足它。
type CurrentCustomerViewReader interface {
	FindCurrent(
		ctx context.Context,
		tenant domain.TenantID,
		customer domain.CustomerAccountReference,
		parcel domain.TrackedParcelReference,
	) (domain.CustomerTrackingView, bool, error)
}

var _ CurrentCustomerViewReader = CustomerViewStore(nil)

// ShipmentTrackingStanding 是按委托的单据号查询时整单那一级的封闭四种，按客户系统的续办分（ADR-0356 决定二；ADR-0029）。它是本
// 上下文的词：parcel-shipment 的委托状态由消费侧适配器译过来，译法见 CustomerShipmentTrackingView。
type ShipmentTrackingStanding uint8

const (
	ShipmentTrackingStandingInvalid ShipmentTrackingStanding = iota
	// ShipmentParcelsListed：委托已接受（含其后派生的已取消、已完成），逐件列它的当前有效包裹。
	ShipmentParcelsListed
	// ShipmentAwaitingAcceptance：委托已提交，接受或拒绝尚未决定。不列包裹——接受之前的声明包裹不是正式包裹。
	ShipmentAwaitingAcceptance
	// ShipmentClosedWithoutService：委托已拒绝或已撤回，这个号不会有可追踪的包裹。
	ShipmentClosedWithoutService
	// ShipmentNotFound：本租户请求方账户名下没有这个号——查无、属别的账户、委托没有单据号，提供方口上就是同一个答案。
	ShipmentNotFound
)

func (standing ShipmentTrackingStanding) String() string {
	switch standing {
	case ShipmentParcelsListed:
		return "PARCELS_LISTED"
	case ShipmentAwaitingAcceptance:
		return "AWAITING_ACCEPTANCE"
	case ShipmentClosedWithoutService:
		return "CLOSED_WITHOUT_SERVICE"
	case ShipmentNotFound:
		return "SHIPMENT_NOT_FOUND"
	default:
		return ""
	}
}

// ShipmentParcelPage 是按页取当前有效包裹的请求：AfterPosition 是上一页末件的位次，0 即第一页；Limit 由接入面按渠道契约定。
type ShipmentParcelPage struct {
	AfterPosition int
	Limit         int
}

// ShipmentParcel 是逐件列出的一件当前有效包裹。
type ShipmentParcel struct {
	Parcel domain.TrackedParcelReference
	// Position 是这一件在委托当前有效包裹次序上的位次，由 parcel-shipment 给出，一经给出不变、不因前面的件被替代而前移；游标
	// 记的就是它（ADR-0356 越权风险点 4）。
	Position int
	// CustomerParcelReference 只在委托上为这一件声明过客户包裹件号时在场（HasCustomerParcelReference）。
	CustomerParcelReference    domain.CustomerParcelReference
	HasCustomerParcelReference bool
	// Replaces 只在拆分合并形成的后继上非空，列它替代的来源包裹。包裹身份谱系落地之前恒空。
	Replaces []domain.TrackedParcelReference
}

// LocatedCustomerShipment 是按运单号找到的整单一级，与列出时本页的件。
type LocatedCustomerShipment struct {
	Standing ShipmentTrackingStanding
	// Parcels、Total、More 只随 ShipmentParcelsListed 有内容：本页的件按位次升序；Total 是这份委托此刻的当前有效包裹件数；More 答
	// 本页之后是否还有件。
	Parcels []ShipmentParcel
	Total   int
	More    bool
}

// CustomerShipmentTrackingView 按（租户，请求方货主客户账户，委托的单据号）答整单那一级（ADR-0356 决定四）。委托、它的单据号、
// 所处阶段与当前有效包裹都归 parcel-shipment，本上下文只消费答复、不存副本（CONTEXT-MAP 所有权）；实现落在消费侧适配器
// （ADR-0025）。
//
// 翻译是全函数：parcel-shipment 的每个委托状态都有明确落点——已提交译待接受；已接受（含其后派生的已取消、已完成）译逐件列出；
// 已拒绝、已撤回译没有服务；查无译查无此单。草稿没有单据号，按号不可达。状态之外的取值或译不成本上下文标识的答复，包住
// ErrCustomerShipmentUntranslatable 交回；依赖读不动原样交回。两种都不是查无此单，消费方答未形成。
type CustomerShipmentTrackingView interface {
	LocateCustomerShipment(
		ctx context.Context,
		tenant domain.TenantID,
		customer domain.CustomerAccountReference,
		number domain.ShipmentDocumentNumberReference,
		page ShipmentParcelPage,
	) (LocatedCustomerShipment, error)
}

// ErrCustomerShipmentUntranslatable 说按运单号那一读的查询维或答复译不成对方或本上下文的词：引用坏了或提供方交来了词表外的状态，
// 重问还是同一个结果。住在端口里的理由同 ErrAmbiguousCustomerAccount。
var ErrCustomerShipmentUntranslatable = errors.New(
	"visibility exception: untranslatable customer shipment lookup")
