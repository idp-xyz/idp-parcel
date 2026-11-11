package parcelshipment

import (
	"context"
	"fmt"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// AcceptedServiceProducts 实现 ppports.AcceptedServiceProductView：正式 SELL 评价按包裹读覆盖它的已接受委托申报的服务产品（ADR-0378
// 决定三）。只翻译不判断。
type AcceptedServiceProducts struct {
	products psports.AcceptedServiceProductView
}

// NewAcceptedServiceProducts 构造期拒 nil：漏装读口要在启动时炸。
func NewAcceptedServiceProducts(products psports.AcceptedServiceProductView) (*AcceptedServiceProducts, error) {
	if products == nil {
		return nil, fmt.Errorf("parcel pricing parcelshipment adapter: accepted service product view is nil")
	}
	return &AcceptedServiceProducts{products: products}, nil
}

var _ ppports.AcceptedServiceProductView = (*AcceptedServiceProducts)(nil)

// LoadAcceptedServiceProduct 按包裹读申报的产品并译成计价的产品引用。租户或包裹在提供方的构造门上立不起来，指不到那一侧任何一份
// 委托，答没有产品，与「这个租户下没有」同一个答案。
func (adapter *AcceptedServiceProducts) LoadAcceptedServiceProduct(
	ctx context.Context,
	tenant ppdomain.TenantID,
	parcel string,
) (ppdomain.ServiceProductReference, error) {
	psTenant, err := psdomain.NewTenantID(tenant.String())
	if err != nil {
		return ppdomain.ServiceProductReference{}, nil
	}
	psParcel, err := psdomain.NewDeclaredParcelID(parcel)
	if err != nil {
		return ppdomain.ServiceProductReference{}, nil
	}
	declared, err := adapter.products.LoadAcceptedServiceProduct(ctx, psTenant, psParcel)
	if err != nil {
		return ppdomain.ServiceProductReference{}, fmt.Errorf("load accepted service product: %w", err)
	}
	return serviceProductOf(declared), nil
}

// serviceProductOf 把申报的服务产品译成计价的产品引用。小包托运原样保全客户写的值（不去空白）；写成计价的引用立不起来（比如带首尾
// 空白）的值，任何一张卡都不可能指名它——卡上的产品引用过同一道构造门——所以按它选卡与没申报落在同一层：只看不声明产品的卡。
// 这里如实折成零值，快照里也就不记一个与选卡无关的串；那一版是否真有这个产品，由商业解析在接受时答。
func serviceProductOf(declared psdomain.DeclaredServiceProduct) ppdomain.ServiceProductReference {
	if !declared.Declared() {
		return ppdomain.ServiceProductReference{}
	}
	product, err := ppdomain.NewServiceProductReference(declared.String())
	if err != nil {
		return ppdomain.ServiceProductReference{}
	}
	return product
}
