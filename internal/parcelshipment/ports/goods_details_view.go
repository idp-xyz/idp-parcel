package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// GoodsDetailsView 按（租户，委托）答货物明细，供关务形成正式申报资料时逐行溯源（ADR-0373 决定九；CONTEXT-MAP
// 「parcel-shipment → customs-compliance」）。消费方是 customs-compliance 的消费侧适配器，进程内另一个限界上下文，不是客户端，
// 所以同 DeclaredMeasurementView 一样不过授权查询作用域。
//
// 两问分开：LoadCurrentGoodsDetails 答此刻的锚与那一版，关务据此记下按哪一版形成；LoadGoodsDetailsAt 按记下的锚读回同一份行。
// 并项、补算、折算与选哪种语言的品名都是关务的形成规则，本口只交客户原始资料：内件性质、行（逐格带来源标记）与装箱分配。
type GoodsDetailsView interface {
	// LoadCurrentGoodsDetails 答货物明细范围上此刻的锚与那一版的货物明细。found=false 是统一不可见结果：这个租户下没有这份
	// 委托，或委托还没接受——不区分哪一种。两条修订分叉时 found=true 而 Undetermined()。error 只表示读面坏了。
	LoadCurrentGoodsDetails(
		ctx context.Context,
		tenant domain.TenantID,
		shipmentRequestID domain.ShipmentRequestID,
	) (domain.GoodsDetailsResolution, bool, error)

	// LoadGoodsDetailsAt 按资料版本锚答那一版的货物明细，带锚与接受基线的成员。found=false 同上，另含锚指的版本不在这份委托
	// 的货物明细范围上。
	LoadGoodsDetailsAt(
		ctx context.Context,
		tenant domain.TenantID,
		shipmentRequestID domain.ShipmentRequestID,
		anchor domain.SourceDataVersionAnchor,
	) (domain.GoodsDetailsResolution, bool, error)
}
