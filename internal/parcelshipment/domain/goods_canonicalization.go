package domain

import (
	"fmt"
	"sort"
)

// 本文件把货物明细与平台货件编进提交的规范化文档（ADR-0373 决定八）。两段在 canonicalSubmissionDocument 上是可缺键：
// 没报即不编码，旧载荷逐字节同前；段内的键常在，读形状即读全集，与 PSC-1 原有各段同一纪律。

type canonicalCellDocument struct {
	Value string `json:"value"`
	Mark  string `json:"mark"`
}

type canonicalDescriptionDocument struct {
	Language string                `json:"language"`
	Text     canonicalCellDocument `json:"text"`
}

type canonicalCommodityCodeDocument struct {
	Side string                `json:"side"`
	Code canonicalCellDocument `json:"code"`
}

type canonicalSalesOrderDocument struct {
	Platform    *canonicalCellDocument `json:"platform"`
	OrderNumber *canonicalCellDocument `json:"order_number"`
	OrderLine   *canonicalCellDocument `json:"order_line"`
}

type canonicalGoodsLineDocument struct {
	Descriptions   []canonicalDescriptionDocument   `json:"descriptions"`
	Quantity       *canonicalCellDocument           `json:"quantity"`
	Unit           *canonicalCellDocument           `json:"unit"`
	UnitValue      *canonicalCellDocument           `json:"unit_value"`
	LineValue      *canonicalCellDocument           `json:"line_value"`
	Currency       *canonicalCellDocument           `json:"currency"`
	NetWeight      *canonicalCellDocument           `json:"net_weight"`
	NetWeightUnit  *canonicalCellDocument           `json:"net_weight_unit"`
	CommodityCodes []canonicalCommodityCodeDocument `json:"commodity_codes"`
	OriginCountry  *canonicalCellDocument           `json:"origin_country"`
	OwnerSKU       *canonicalCellDocument           `json:"owner_sku"`
	SalesOrder     canonicalSalesOrderDocument      `json:"sales_order"`
}

type canonicalPackingDocument struct {
	Parcel   string `json:"parcel"`
	Line     int    `json:"line"`
	Quantity string `json:"quantity"`
}

type canonicalGoodsDocument struct {
	Category     string                       `json:"category"`
	CategoryMark string                       `json:"category_mark"`
	CategoryNote *canonicalCellDocument       `json:"category_note"`
	Lines        []canonicalGoodsLineDocument `json:"lines"`
	Packing      []canonicalPackingDocument   `json:"packing"`
}

type canonicalPlatformShipmentDocument struct {
	Platform  string `json:"platform"`
	Shipment  string `json:"shipment"`
	Reference string `json:"reference"`
}

func canonicalCell(cell GoodsCell) *canonicalCellDocument {
	if !cell.present() {
		return nil
	}
	return &canonicalCellDocument{Value: cell.value, Mark: cell.mark.String()}
}

// canonicalGoodsDocumentOf 编货物明细。行保持声明顺序：行号随顺序，是内容。品名、编码与装箱分配是集合，按键排序，
// 给入顺序不是内容。装箱分配对着这份提交的成员集合再核一遍（checkPackingCoversMembers）。
func canonicalGoodsDocumentOf(goods GoodsDetails, declared map[DeclaredParcelID]struct{}) (*canonicalGoodsDocument, error) {
	if !goods.Declared() {
		return nil, nil
	}
	if err := checkPackingCoversMembers(goods.packing, declared); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSubmissionPayload, err)
	}
	document := &canonicalGoodsDocument{
		Category:     goods.category.String(),
		CategoryMark: goods.categoryMark.String(),
		CategoryNote: canonicalCell(goods.categoryNote),
		Lines:        make([]canonicalGoodsLineDocument, 0, len(goods.lines)),
		Packing:      make([]canonicalPackingDocument, 0, len(goods.packing)),
	}
	for _, line := range goods.lines {
		document.Lines = append(document.Lines, canonicalGoodsLineOf(line))
	}
	for _, allocation := range goods.packing {
		document.Packing = append(document.Packing, canonicalPackingDocument{
			Parcel: allocation.parcel.String(), Line: allocation.line, Quantity: allocation.quantity,
		})
	}
	sort.Slice(document.Packing, func(left, right int) bool {
		if document.Packing[left].Parcel != document.Packing[right].Parcel {
			return document.Packing[left].Parcel < document.Packing[right].Parcel
		}
		return document.Packing[left].Line < document.Packing[right].Line
	})
	return document, nil
}

func canonicalGoodsLineOf(line GoodsLine) canonicalGoodsLineDocument {
	document := canonicalGoodsLineDocument{
		Descriptions:   make([]canonicalDescriptionDocument, 0, len(line.descriptions)),
		Quantity:       canonicalCell(line.quantity),
		Unit:           canonicalCell(line.unit),
		UnitValue:      canonicalCell(line.unitValue),
		LineValue:      canonicalCell(line.lineValue),
		Currency:       canonicalCell(line.currency),
		NetWeight:      canonicalCell(line.netWeight),
		NetWeightUnit:  canonicalCell(line.netWeightUnit),
		CommodityCodes: make([]canonicalCommodityCodeDocument, 0, len(line.commodityCodes)),
		OriginCountry:  canonicalCell(line.originCountry),
		OwnerSKU:       canonicalCell(line.ownerSKU),
		SalesOrder: canonicalSalesOrderDocument{
			Platform:    canonicalCell(line.salesOrder.platform),
			OrderNumber: canonicalCell(line.salesOrder.orderNumber),
			OrderLine:   canonicalCell(line.salesOrder.orderLine),
		},
	}
	for _, description := range line.descriptions {
		document.Descriptions = append(document.Descriptions, canonicalDescriptionDocument{
			Language: description.language,
			Text:     canonicalCellDocument{Value: description.text.value, Mark: description.text.mark.String()},
		})
	}
	sort.Slice(document.Descriptions, func(left, right int) bool {
		return document.Descriptions[left].Language < document.Descriptions[right].Language
	})
	for _, code := range line.commodityCodes {
		document.CommodityCodes = append(document.CommodityCodes, canonicalCommodityCodeDocument{
			Side: code.side.String(),
			Code: canonicalCellDocument{Value: code.code.value, Mark: code.code.mark.String()},
		})
	}
	sort.Slice(document.CommodityCodes, func(left, right int) bool {
		return document.CommodityCodes[left].Side < document.CommodityCodes[right].Side
	})
	return document
}

func canonicalPlatformShipmentOf(shipment PlatformShipmentDeclaration) *canonicalPlatformShipmentDocument {
	if !shipment.Declared() {
		return nil
	}
	return &canonicalPlatformShipmentDocument{
		Platform:  shipment.platform.String(),
		Shipment:  shipment.shipment.String(),
		Reference: shipment.reference.String(),
	}
}
