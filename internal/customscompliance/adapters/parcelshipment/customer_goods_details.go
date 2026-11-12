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
// 决定九，票 declared-goods-lines/07）。只译不判：不并项、不补算、不挑品名语言，值原样；来源标记、内件性质与编码口径按下面的
// 对应表译，表外的取值答读不动——不认识的词照抄过去，等于替关务认了它。
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

var errUntranslatable = errors.New("customs compliance parcelshipment adapter: provider answered a value outside the translation table")

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
	if resolution.Undetermined() {
		return ccports.CustomerGoodsDetails{Parcels: parcelsOf(resolution)}, true, true, nil
	}
	anchor, anchored := resolution.Anchor()
	if !anchored {
		// 既没锚又不是待复核：提供方的坏答复。照「找到了」交回，关务就会记下一枚永远读不回的空锚。
		return ccports.CustomerGoodsDetails{}, false, false, fmt.Errorf("%w: a found answer without an anchor", errUntranslatable)
	}
	details, err := customerGoodsDetailsOf(anchorOf(anchor), resolution)
	return details, false, err == nil, err
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
	resolution, found, err := adapter.view.LoadGoodsDetailsAt(ctx, providerTenant, providerRequest, providerAnchor)
	if err != nil {
		return ccports.CustomerGoodsDetails{}, false, fmt.Errorf("customs compliance parcelshipment adapter: %w", err)
	}
	if !found {
		return ccports.CustomerGoodsDetails{}, false, nil
	}
	details, err := customerGoodsDetailsOf(anchor, resolution)
	return details, err == nil, err
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

func parcelsOf(resolution psdomain.GoodsDetailsResolution) []string {
	members := resolution.Members()
	parcels := make([]string, 0, len(members))
	for _, member := range members {
		parcels = append(parcels, member.String())
	}
	return parcels
}

func sourceOf(mark psdomain.DeclarationSourceMark) (ccports.CustomerGoodsCellSource, error) {
	switch mark {
	case psdomain.SourceMarkThisDeclaration:
		return ccports.CustomerGoodsCellFromThisDeclaration, nil
	case psdomain.SourceMarkBoxList:
		return ccports.CustomerGoodsCellFromBoxList, nil
	case psdomain.SourceMarkCommodityDeclaration:
		return ccports.CustomerGoodsCellFromCommodityDeclaration, nil
	default:
		return ccports.CustomerGoodsCellSourceUnknown, fmt.Errorf("%w: source mark %q", errUntranslatable, mark)
	}
}

func categoryOf(category psdomain.ContentCategory) (ccports.CustomerContentCategory, error) {
	switch category {
	case psdomain.ContentCategoryGift:
		return ccports.CustomerContentGift, nil
	case psdomain.ContentCategoryDocuments:
		return ccports.CustomerContentDocuments, nil
	case psdomain.ContentCategoryCommercialSample:
		return ccports.CustomerContentCommercialSample, nil
	case psdomain.ContentCategoryReturnedGoods:
		return ccports.CustomerContentReturnedGoods, nil
	case psdomain.ContentCategorySaleOfGoods:
		return ccports.CustomerContentSaleOfGoods, nil
	case psdomain.ContentCategoryOther:
		return ccports.CustomerContentOther, nil
	default:
		return ccports.CustomerContentNotDeclared, fmt.Errorf("%w: content category %q", errUntranslatable, category)
	}
}

func sideOf(side psdomain.CommodityCodeSide) (ccports.CustomerCommodityCodeSide, error) {
	switch side {
	case psdomain.CommodityCodeExportSide:
		return ccports.CustomerCommodityCodeExport, nil
	case psdomain.CommodityCodeImportSide:
		return ccports.CustomerCommodityCodeImport, nil
	default:
		return "", fmt.Errorf("%w: commodity code side %q", errUntranslatable, side)
	}
}

// cells 逐格翻译，第一处翻不动的记下、其余照译；收尾时交回那一处，免得每格都写一遍错误分支。
type cells struct{ err error }

func (translate *cells) value(cell psdomain.GoodsCell) ccports.CustomerGoodsCell {
	source, err := sourceOf(cell.Mark())
	if err != nil && translate.err == nil {
		translate.err = err
	}
	return ccports.CustomerGoodsCell{Value: cell.Value(), Source: source}
}

func (translate *cells) optional(cell psdomain.GoodsCell, present bool) *ccports.CustomerGoodsCell {
	if !present {
		return nil
	}
	value := translate.value(cell)
	return &value
}

func customerGoodsDetailsOf(anchor ccports.CustomerSourceDataAnchor, resolution psdomain.GoodsDetailsResolution) (ccports.CustomerGoodsDetails, error) {
	goods := resolution.Goods()
	details := ccports.CustomerGoodsDetails{Anchor: anchor, Declared: goods.Declared(), Parcels: parcelsOf(resolution)}
	if !goods.Declared() {
		return details, nil
	}
	translate := &cells{}
	if category, mark, present := goods.Category(); present {
		var err error
		if details.ContentCategory, err = categoryOf(category); err != nil {
			return ccports.CustomerGoodsDetails{}, err
		}
		if details.ContentCategorySource, err = sourceOf(mark); err != nil {
			return ccports.CustomerGoodsDetails{}, err
		}
	}
	details.ContentCategoryNote = translate.optional(goods.CategoryNote())
	for _, line := range goods.Lines() {
		row := ccports.CustomerGoodsLine{
			Line:          line.Number(),
			Quantity:      translate.optional(line.Quantity()),
			Unit:          translate.optional(line.Unit()),
			UnitValue:     translate.optional(line.UnitValue()),
			LineValue:     translate.optional(line.LineValue()),
			Currency:      translate.optional(line.Currency()),
			NetWeight:     translate.optional(line.NetWeight()),
			NetWeightUnit: translate.optional(line.NetWeightUnit()),
			OriginCountry: translate.optional(line.OriginCountry()),
			OwnerSKU:      translate.optional(line.OwnerSKU()),
		}
		for _, description := range line.Descriptions() {
			row.Descriptions = append(row.Descriptions, ccports.CustomerGoodsDescription{
				Language: description.Language(), Text: translate.value(description.Text()),
			})
		}
		for _, code := range line.CommodityCodes() {
			side, err := sideOf(code.Side())
			if err != nil {
				return ccports.CustomerGoodsDetails{}, err
			}
			row.CommodityCodes = append(row.CommodityCodes, ccports.CustomerCommodityCode{Side: side, Code: translate.value(code.Code())})
		}
		if order, present := line.SalesOrder(); present {
			row.SalesOrder = &ccports.CustomerSalesOrderReference{
				Platform:    translate.optional(order.Platform()),
				OrderNumber: translate.optional(order.OrderNumber()),
				OrderLine:   translate.optional(order.OrderLine()),
			}
		}
		details.Lines = append(details.Lines, row)
	}
	if translate.err != nil {
		return ccports.CustomerGoodsDetails{}, translate.err
	}
	for _, allocation := range goods.Packing() {
		details.Packing = append(details.Packing, ccports.CustomerGoodsPacking{
			Parcel: allocation.Parcel().String(), Line: allocation.Line(), Quantity: allocation.Quantity(),
		})
	}
	return details, nil
}
