package shipmenthttp

import (
	"sort"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件是委托详情里货物明细与平台货件的响应形状（ADR-0373、ADR-0374，票 declared-goods-lines/04）。形状与提交口对应——委托级
// goods 加逐件的 packing——只是每格多交一个来源标记：读的人要分得清这一格是本单申报、箱单还是商品申报资料。没报的格整键缺席。

type goodsCellViewBody struct {
	Value string `json:"value"`
	Mark  string `json:"mark"`
}

type goodsDescriptionViewBody struct {
	Lang string            `json:"lang"`
	Text goodsCellViewBody `json:"text"`
}

type commodityCodeViewBody struct {
	Side string            `json:"side"`
	Code goodsCellViewBody `json:"code"`
}

type salesOrderViewBody struct {
	Platform    *goodsCellViewBody `json:"platform,omitempty"`
	OrderNumber *goodsCellViewBody `json:"orderNumber,omitempty"`
	OrderLine   *goodsCellViewBody `json:"orderLine,omitempty"`
}

type goodsLineViewBody struct {
	Line           int                        `json:"line"`
	Descriptions   []goodsDescriptionViewBody `json:"descriptions,omitempty"`
	Quantity       *goodsCellViewBody         `json:"quantity,omitempty"`
	Unit           *goodsCellViewBody         `json:"unit,omitempty"`
	UnitValue      *goodsCellViewBody         `json:"unitValue,omitempty"`
	LineValue      *goodsCellViewBody         `json:"lineValue,omitempty"`
	Currency       *goodsCellViewBody         `json:"currency,omitempty"`
	NetWeight      *goodsCellViewBody         `json:"netWeight,omitempty"`
	NetWeightUnit  *goodsCellViewBody         `json:"netWeightUnit,omitempty"`
	CommodityCodes []commodityCodeViewBody    `json:"commodityCodes,omitempty"`
	OriginCountry  *goodsCellViewBody         `json:"originCountry,omitempty"`
	SKU            *goodsCellViewBody         `json:"sku,omitempty"`
	SalesOrder     *salesOrderViewBody        `json:"salesOrder,omitempty"`
}

type goodsViewBody struct {
	ContentCategory     string              `json:"contentCategory,omitempty"`
	ContentCategoryMark string              `json:"contentCategoryMark,omitempty"`
	ContentCategoryNote *goodsCellViewBody  `json:"contentCategoryNote,omitempty"`
	Lines               []goodsLineViewBody `json:"lines"`
}

type platformShipmentViewBody struct {
	Platform    string `json:"platform"`
	ShipmentID  string `json:"shipmentId"`
	ReferenceID string `json:"referenceId,omitempty"`
}

type packingViewBody struct {
	Line     int    `json:"line"`
	Quantity string `json:"quantity"`
}

func goodsCellViewBodyOf(cell domain.GoodsCell, present bool) *goodsCellViewBody {
	if !present {
		return nil
	}
	return &goodsCellViewBody{Value: cell.Value(), Mark: cell.Mark().String()}
}

func goodsViewBodyOf(goods domain.GoodsDetails) *goodsViewBody {
	if !goods.Declared() {
		return nil
	}
	body := &goodsViewBody{Lines: make([]goodsLineViewBody, 0, len(goods.Lines()))}
	if category, mark, present := goods.Category(); present {
		body.ContentCategory, body.ContentCategoryMark = category.String(), mark.String()
	}
	body.ContentCategoryNote = goodsCellViewBodyOf(goods.CategoryNote())
	for _, line := range goods.Lines() {
		lineBody := goodsLineViewBody{
			Line:          line.Number(),
			Quantity:      goodsCellViewBodyOf(line.Quantity()),
			Unit:          goodsCellViewBodyOf(line.Unit()),
			UnitValue:     goodsCellViewBodyOf(line.UnitValue()),
			LineValue:     goodsCellViewBodyOf(line.LineValue()),
			Currency:      goodsCellViewBodyOf(line.Currency()),
			NetWeight:     goodsCellViewBodyOf(line.NetWeight()),
			NetWeightUnit: goodsCellViewBodyOf(line.NetWeightUnit()),
			OriginCountry: goodsCellViewBodyOf(line.OriginCountry()),
			SKU:           goodsCellViewBodyOf(line.OwnerSKU()),
		}
		for _, description := range line.Descriptions() {
			lineBody.Descriptions = append(lineBody.Descriptions, goodsDescriptionViewBody{
				Lang: description.Language(),
				Text: goodsCellViewBody{Value: description.Text().Value(), Mark: description.Text().Mark().String()},
			})
		}
		for _, code := range line.CommodityCodes() {
			lineBody.CommodityCodes = append(lineBody.CommodityCodes, commodityCodeViewBody{
				Side: code.Side().String(),
				Code: goodsCellViewBody{Value: code.Code().Value(), Mark: code.Code().Mark().String()},
			})
		}
		if order, present := line.SalesOrder(); present {
			lineBody.SalesOrder = &salesOrderViewBody{
				Platform:    goodsCellViewBodyOf(order.Platform()),
				OrderNumber: goodsCellViewBodyOf(order.OrderNumber()),
				OrderLine:   goodsCellViewBodyOf(order.OrderLine()),
			}
		}
		body.Lines = append(body.Lines, lineBody)
	}
	return body
}

func platformShipmentViewBodyOf(shipment domain.PlatformShipmentDeclaration) *platformShipmentViewBody {
	if !shipment.Declared() {
		return nil
	}
	body := &platformShipmentViewBody{Platform: shipment.Platform(), ShipmentID: shipment.ShipmentNumber().String()}
	if reference, present := shipment.ReferenceNumber(); present {
		body.ReferenceID = reference.String()
	}
	return body
}

// packingViewBodiesByParcel 把装箱分配按包裹分好，每件按行号次序；整票清单（没报分配）交空表，各件整键缺席。一次分组，不按件
// 各扫一遍全部分配——50 箱的货件逐件扫就是 50 遍。
func packingViewBodiesByParcel(goods domain.GoodsDetails) map[domain.DeclaredParcelID][]packingViewBody {
	byParcel := make(map[domain.DeclaredParcelID][]packingViewBody)
	for _, allocation := range goods.Packing() {
		byParcel[allocation.Parcel()] = append(byParcel[allocation.Parcel()],
			packingViewBody{Line: allocation.Line(), Quantity: allocation.Quantity()})
	}
	for parcel := range byParcel {
		bodies := byParcel[parcel]
		sort.Slice(bodies, func(left, right int) bool { return bodies[left].Line < bodies[right].Line })
	}
	return byParcel
}
