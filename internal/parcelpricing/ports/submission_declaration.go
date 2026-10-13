package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// SubmissionDeclaration 是一份委托的一个提交版本上、在本上下文词汇里的客户申报（ADR-0179 决定四）。Route 为 nil 即寄收两段邮编没报
// 全：邮编随委托级的寄收件资料申报，各件共用这一条路线。要不要路线由卡的目录绑定定，这里不判。
type SubmissionDeclaration struct {
	Parcels []DeclaredParcel
	Route   *domain.PostalRoute
}

// DeclaredParcel 是一件声明包裹的申报测量。Parcel 是 parcel-shipment 的声明包裹标识原样，本上下文不解释、不铸造。Weight 为 nil 即
// 这件没报测量，或报的单位不在计价单位表内——两者都不估，不拿同委托别件的测量顶；Dimensions 为 nil 即没报外廓。
type DeclaredParcel struct {
	Parcel     string
	Weight     *domain.Weight
	Dimensions *domain.Dimensions
}

// SubmissionDeclarationView 是受理前试算读小包托运申报的消费侧口：按（租户、委托、提交版本）逐件交回，与声明成员同序。found=false
// 即这份委托的这一版对本租户不可见，不区分是哪一种不可见；error 只表示读面坏了。单位与邮编的翻译在实现它的适配器里，入口只拿计价
// 词汇。两个引用是 parcel-shipment 的标识原样。
type SubmissionDeclarationView interface {
	LoadSubmissionDeclaration(
		ctx context.Context,
		tenant domain.TenantID,
		shipmentRequest string,
		submissionVersion string,
	) (SubmissionDeclaration, bool, error)
}
