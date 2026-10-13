package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// SubmissionVersionDeclarationView 按（租户、委托、提交版本）逐件答申报测量与寄、收两段邮编（ADR-0179 决定四）。消费方是
// `parcel-pricing` 受理前试算的消费侧适配器：试算按委托当前提交版本逐件造输入，受理前没有接受基线可走。
//
// 另开一口而不拓宽 DeclaredMeasurementView / AddressElementsView：那两口按（租户，包裹身份）问、只经接受基线的声明成员走，受理前
// 答「无」；它们服务接受之后的正式评价，拓宽它们会让同一问在接受前后换成两种答法。本口按提交版本问，答的是那一版的原样申报，
// 不解析资料版本锚、不算分区、不译单位——单位对到计价单位表与邮编 → 分区都归计价那一侧。
//
// 同 DeclaredMeasurementView 一样不过授权查询作用域：消费方是进程内另一个限界上下文，不是客户端。
type SubmissionVersionDeclarationView interface {
	// LoadSubmissionVersionDeclaration 答那一版上的申报。found=false 是统一不可见结果：这个租户下没有这份委托、委托上没有这一版、
	// 或委托已拒绝 / 已撤回——不区分哪一种。error 只表示读面坏了。
	LoadSubmissionVersionDeclaration(
		ctx context.Context,
		tenant domain.TenantID,
		shipmentRequestID domain.ShipmentRequestID,
		submissionVersion domain.SubmissionVersionID,
	) (domain.SubmissionVersionDeclaration, bool, error)
}
