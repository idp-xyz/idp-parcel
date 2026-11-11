package parcelshipment

import (
	"context"
	"errors"
	"fmt"

	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// CustomerGoodsDetails 实现 ccports.CustomerGoodsDetailsView：问提供方的货物明细读口，把货物明细逐格译成关务的记录（ADR-0373
// 决定九，票 declared-goods-lines/07）。只译不判：不并项、不补算、不挑品名语言，值原样，来源标记照对应表译。
type CustomerGoodsDetails struct {
	view psports.GoodsDetailsView
}

func NewCustomerGoodsDetails(view psports.GoodsDetailsView) (*CustomerGoodsDetails, error) {
	if view == nil {
		return nil, errors.New("customs compliance parcelshipment adapter: goods details view is nil")
	}
	return &CustomerGoodsDetails{view: view}, nil
}

var _ ccports.CustomerGoodsDetailsView = (*CustomerGoodsDetails)(nil)

func (adapter *CustomerGoodsDetails) FindCurrentCustomerGoodsDetails(
	ctx context.Context,
	tenant ccdomain.TenantID,
	shipmentRequest string,
) (ccports.CustomerGoodsDetails, bool, bool, error) {
	providerTenant, providerRequest, ok, err := providerKeys(tenant, shipmentRequest)
	if err != nil || !ok {
		return ccports.CustomerGoodsDetails{}, false, false, err
	}
	resolution, found, err := adapter.view.LoadCurrentGoodsDetails(ctx, providerTenant, providerRequest)
	if err != nil {
		return ccports.CustomerGoodsDetails{}, false, false, fmt.Errorf("customs compliance parcelshipment adapter: %w", err)
	}
	if !found {
		return ccports.CustomerGoodsDetails{}, false, false, nil
	}
	anchor, anchored := resolution.Anchor()
	if !anchored {
		return ccports.CustomerGoodsDetails{}, resolution.Undetermined(), true, nil
	}
	return customerGoodsDetailsOf(anchorOf(anchor), resolution.Goods()), false, true, nil
}

func (adapter *CustomerGoodsDetails) FindCustomerGoodsDetails(
	ctx context.Context,
	tenant ccdomain.TenantID,
	shipmentRequest string,
	anchor ccports.CustomerSourceDataAnchor,
) (ccports.CustomerGoodsDetails, bool, error) {
	providerTenant, providerRequest, ok, err := providerKeys(tenant, shipmentRequest)
	if err != nil || !ok {
		return ccports.CustomerGoodsDetails{}, false, err
	}
	providerAnchor, ok := providerAnchorOf(anchor)
	if !ok {
		return ccports.CustomerGoodsDetails{}, false, nil
	}
	goods, found, err := adapter.view.LoadGoodsDetailsAt(ctx, providerTenant, providerRequest, providerAnchor)
	if err != nil {
		return ccports.CustomerGoodsDetails{}, false, fmt.Errorf("customs compliance parcelshipment adapter: %w", err)
	}
	if !found {
		return ccports.CustomerGoodsDetails{}, false, nil
	}
	return customerGoodsDetailsOf(anchor, goods), true, nil
}

// providerKeys 把关务手里的键译成提供方的词。委托号在提供方词汇里构造不出来（空白）答不可见——那份委托本就不可能存在；
// 租户构造不出来是调用方的错。
func providerKeys(tenant ccdomain.TenantID, shipmentRequest string) (psdomain.TenantID, psdomain.ShipmentRequestID, bool, error) {
	providerTenant, err := psdomain.NewTenantID(tenant.String())
	if err != nil {
		return psdomain.TenantID{}, psdomain.ShipmentRequestID{}, false, fmt.Errorf("customs compliance parcelshipment adapter: tenant: %w", err)
	}
	providerRequest, err := psdomain.NewShipmentRequestID(shipmentRequest)
	if err != nil {
		return psdomain.TenantID{}, psdomain.ShipmentRequestID{}, false, nil
	}
	return providerTenant, providerRequest, true, nil
}

// providerAnchorOf 把关务记下的锚译回提供方的锚；两格都没有或两格都有（既说在基线上又指名一版）立不起来，答不可见。
func providerAnchorOf(anchor ccports.CustomerSourceDataAnchor) (psdomain.SourceDataVersionAnchor, bool) {
	if anchor.OnAcceptanceBaseline {
		if anchor.SourceDataVersion != "" {
			return psdomain.SourceDataVersionAnchor{}, false
		}
		return psdomain.NewAcceptanceBaselineAnchor(), true
	}
	version, err := psdomain.NewSourceDataVersionID(anchor.SourceDataVersion)
	if err != nil {
		return psdomain.SourceDataVersionAnchor{}, false
	}
	providerAnchor, err := psdomain.NewSourceDataVersionAnchor(version)
	if err != nil {
		return psdomain.SourceDataVersionAnchor{}, false
	}
	return providerAnchor, true
}

func anchorOf(anchor psdomain.SourceDataVersionAnchor) ccports.CustomerSourceDataAnchor {
	if version, adopted := anchor.AdoptedVersion(); adopted {
		return ccports.CustomerSourceDataAnchor{SourceDataVersion: version.String()}
	}
	return ccports.CustomerSourceDataAnchor{OnAcceptanceBaseline: true}
}

func sourceOf(mark psdomain.DeclarationSourceMark) ccports.CustomerGoodsCellSource {
	switch mark {
	case psdomain.SourceMarkThisDeclaration:
		return ccports.CustomerGoodsCellFromThisDeclaration
	case psdomain.SourceMarkBoxList:
		return ccports.CustomerGoodsCellFromBoxList
	case psdomain.SourceMarkCommodityDeclaration:
		return ccports.CustomerGoodsCellFromCommodityDeclaration
	default:
		return ccports.CustomerGoodsCellSourceUnknown
	}
}

func cellOf(cell psdomain.GoodsCell, present bool) *ccports.CustomerGoodsCell {
	if !present {
		return nil
	}
	return &ccports.CustomerGoodsCell{Value: cell.Value(), Source: sourceOf(cell.Mark())}
}

func customerGoodsDetailsOf(anchor ccports.CustomerSourceDataAnchor, goods psdomain.GoodsDetails) ccports.CustomerGoodsDetails {
	details := ccports.CustomerGoodsDetails{Anchor: anchor, Declared: goods.Declared()}
	if !goods.Declared() {
		return details
	}
	if category, mark, present := goods.Category(); present {
		details.ContentCategory, details.ContentCategorySource = category.String(), sourceOf(mark)
	}
	details.ContentCategoryNote = cellOf(goods.CategoryNote())
	for _, line := range goods.Lines() {
		row := ccports.CustomerGoodsLine{
			Line:          line.Number(),
			Quantity:      cellOf(line.Quantity()),
			Unit:          cellOf(line.Unit()),
			UnitValue:     cellOf(line.UnitValue()),
			LineValue:     cellOf(line.LineValue()),
			Currency:      cellOf(line.Currency()),
			NetWeight:     cellOf(line.NetWeight()),
			NetWeightUnit: cellOf(line.NetWeightUnit()),
			OriginCountry: cellOf(line.OriginCountry()),
			OwnerSKU:      cellOf(line.OwnerSKU()),
		}
		for _, description := range line.Descriptions() {
			row.Descriptions = append(row.Descriptions, ccports.CustomerGoodsDescription{
				Language: description.Language(), Text: *cellOf(description.Text(), true),
			})
		}
		for _, code := range line.CommodityCodes() {
			row.CommodityCodes = append(row.CommodityCodes, ccports.CustomerCommodityCode{
				Side: code.Side().String(), Code: *cellOf(code.Code(), true),
			})
		}
		if order, present := line.SalesOrder(); present {
			row.SalesOrder = &ccports.CustomerSalesOrderReference{
				Platform: cellOf(order.Platform()), OrderNumber: cellOf(order.OrderNumber()), OrderLine: cellOf(order.OrderLine()),
			}
		}
		details.Lines = append(details.Lines, row)
	}
	for _, allocation := range goods.Packing() {
		details.Packing = append(details.Packing, ccports.CustomerGoodsPacking{
			Parcel: allocation.Parcel().String(), Line: allocation.Line(), Quantity: allocation.Quantity(),
		})
	}
	return details
}
