package partycommercial

import (
	"context"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// RequestedServiceProductKeys 在登记面折出的闭包键上叠委托声明的服务产品（票 psb/17）。
//
// 登记面只知道（租户，客户账户）那一行，不知道这份委托声明了哪个产品。声明在提交版本上，
// 由另一协作者按查询里的引用读出。两件事分两个协作者：登记面没折出键与读不到声明的恢复
// 动作不同，压进一个方法里调用方分不清该催人登记还是该重试。
type RequestedServiceProductKeys struct {
	keys     closureKeyFormer
	products declaredServiceProductSource
}

type closureKeyFormer interface {
	FormResolutionKey(ctx context.Context, query psports.CommercialBasisQuery) (pcdomain.ClosureResolutionKey, bool, error)
}

type declaredServiceProductSource interface {
	RequestedServiceProduct(ctx context.Context, query psports.CommercialBasisQuery) (psdomain.DeclaredServiceProduct, error)
}

func NewRequestedServiceProductKeys(
	keys closureKeyFormer,
	products declaredServiceProductSource,
) (*RequestedServiceProductKeys, error) {
	if keys == nil {
		return nil, fmt.Errorf("parcel shipment party commercial: resolution key former is nil")
	}
	if products == nil {
		return nil, fmt.Errorf("parcel shipment party commercial: requested service product source is nil")
	}
	return &RequestedServiceProductKeys{keys: keys, products: products}, nil
}

var _ ResolutionKeySource = (*RequestedServiceProductKeys)(nil)

// FormResolutionKey 先要登记面的键。没折出来（未配置或失败）原样交回，不去读声明：
// 未配置与读不到分格的责任在登记面。闭包不要服务产品依据时也不读：叠上去的键最小身份
// 不成立，读了也用不上。读不到声明是依赖故障，不是「客户没声明」——压成未声明会让同一
// 范围两个产品的委托换一个冲突答复。全空白的声明在对象标识里没有对应，响亮报出。
func (adapter *RequestedServiceProductKeys) FormResolutionKey(
	ctx context.Context,
	query psports.CommercialBasisQuery,
) (pcdomain.ClosureResolutionKey, bool, error) {
	key, formed, err := adapter.keys.FormResolutionKey(ctx, query)
	if err != nil || !formed || !closureRequestsServiceProduct(key) {
		return key, formed, err
	}
	product, err := adapter.products.RequestedServiceProduct(ctx, query)
	if err != nil {
		return pcdomain.ClosureResolutionKey{}, false, err
	}
	if !product.Declared() {
		return key, true, nil
	}
	id, err := pcdomain.NewCommercialObjectID(product.String())
	if err != nil {
		return pcdomain.ClosureResolutionKey{}, false, fmt.Errorf("translate requested service product: %w", err)
	}
	key.ServiceProduct = id
	return key, true, nil
}

func closureRequestsServiceProduct(key pcdomain.ClosureResolutionKey) bool {
	for _, kind := range key.RequiredBases {
		if kind == pcdomain.ServiceProductObject {
			return true
		}
	}
	return false
}

// shipmentRequestFinder 是读声明所需的那一格，不把整份委托仓储接口拖进构造签名。
type shipmentRequestFinder interface {
	FindBySourceIdentity(ctx context.Context, identity psdomain.SourceIdentity) (psdomain.ShipmentRequest, bool, error)
}

// ShipmentRequestedServiceProduct 按查询里的来源身份与提交版本，从委托上读出声明的服务产品。
type ShipmentRequestedServiceProduct struct {
	requests shipmentRequestFinder
}

func NewShipmentRequestedServiceProduct(requests shipmentRequestFinder) (*ShipmentRequestedServiceProduct, error) {
	if requests == nil {
		return nil, fmt.Errorf("parcel shipment party commercial: shipment requests are nil")
	}
	return &ShipmentRequestedServiceProduct{requests: requests}, nil
}

// RequestedServiceProduct 交回该版本声明的产品；未声明是零值，不是错误。委托或版本
// 读不到则上抛：那是依赖故障，当成没声明会把「库读失败」答成「客户没选产品」。
func (source *ShipmentRequestedServiceProduct) RequestedServiceProduct(
	ctx context.Context,
	query psports.CommercialBasisQuery,
) (psdomain.DeclaredServiceProduct, error) {
	request, found, err := source.requests.FindBySourceIdentity(ctx, query.Identity)
	if err != nil {
		return psdomain.DeclaredServiceProduct{}, fmt.Errorf("read requested service product: %w", err)
	}
	if !found {
		return psdomain.DeclaredServiceProduct{}, fmt.Errorf("read requested service product: shipment request not found")
	}
	version, ok := request.SubmissionVersionByID(query.SubmissionVersion)
	if !ok {
		return psdomain.DeclaredServiceProduct{}, fmt.Errorf("read requested service product: submission version not found")
	}
	return version.RequestedServiceProduct(), nil
}
