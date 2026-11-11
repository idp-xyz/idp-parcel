package postgres

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件是货物明细与平台声明在委托快照里的落法（ADR-0373 决定五、ADR-0374 决定四）。两样都是可缺段：没报即 omitempty 整段
// 不写，早于它们的快照读回即没报，零迁移。读回一律经领域的还原构造器（RehydrateGoodsDetails、RehydratePlatformShipment、
// RehydratePlatformBoxLabel）：包外造不出半截的一份，又不再过接入的门——门日后收紧，已存的委托照样读得回来。

type goodsCellDocument struct {
	Value string `json:"value"`
	Mark  string `json:"mark"`
}

type goodsDescriptionDocument struct {
	Language string            `json:"language"`
	Text     goodsCellDocument `json:"text"`
}

type commodityCodeDocument struct {
	Side string            `json:"side"`
	Code goodsCellDocument `json:"code"`
}

type salesOrderDocument struct {
	Platform    *goodsCellDocument `json:"platform,omitempty"`
	OrderNumber *goodsCellDocument `json:"orderNumber,omitempty"`
	OrderLine   *goodsCellDocument `json:"orderLine,omitempty"`
}

type goodsLineDocument struct {
	Descriptions   []goodsDescriptionDocument `json:"descriptions,omitempty"`
	Quantity       *goodsCellDocument         `json:"quantity,omitempty"`
	Unit           *goodsCellDocument         `json:"unit,omitempty"`
	UnitValue      *goodsCellDocument         `json:"unitValue,omitempty"`
	LineValue      *goodsCellDocument         `json:"lineValue,omitempty"`
	Currency       *goodsCellDocument         `json:"currency,omitempty"`
	NetWeight      *goodsCellDocument         `json:"netWeight,omitempty"`
	NetWeightUnit  *goodsCellDocument         `json:"netWeightUnit,omitempty"`
	CommodityCodes []commodityCodeDocument    `json:"commodityCodes,omitempty"`
	OriginCountry  *goodsCellDocument         `json:"originCountry,omitempty"`
	OwnerSKU       *goodsCellDocument         `json:"ownerSku,omitempty"`
	SalesOrder     *salesOrderDocument        `json:"salesOrder,omitempty"`
}

type packingDocument struct {
	Parcel   string `json:"parcel"`
	Line     int    `json:"line"`
	Quantity string `json:"quantity"`
}

type goodsDocument struct {
	Category     string              `json:"category,omitempty"`
	CategoryMark string              `json:"categoryMark,omitempty"`
	CategoryNote *goodsCellDocument  `json:"categoryNote,omitempty"`
	Lines        []goodsLineDocument `json:"lines,omitempty"`
	Packing      []packingDocument   `json:"packing,omitempty"`
}

type platformShipmentDocument struct {
	Platform  string `json:"platform"`
	Shipment  string `json:"shipment"`
	Reference string `json:"reference,omitempty"`
}

type platformBoxLabelDocument struct {
	Parcel string `json:"parcel"`
	Number string `json:"number"`
}

func goodsCellDocumentOf(cell domain.GoodsCell, present bool) *goodsCellDocument {
	if !present {
		return nil
	}
	return &goodsCellDocument{Value: cell.Value(), Mark: cell.Mark().String()}
}

func goodsDocumentOf(goods domain.GoodsDetails) *goodsDocument {
	if !goods.Declared() {
		return nil
	}
	document := &goodsDocument{}
	if category, mark, present := goods.Category(); present {
		document.Category = category.String()
		document.CategoryMark = mark.String()
	}
	document.CategoryNote = goodsCellDocumentOf(goods.CategoryNote())
	for _, line := range goods.Lines() {
		document.Lines = append(document.Lines, goodsLineDocumentOf(line))
	}
	for _, allocation := range goods.Packing() {
		document.Packing = append(document.Packing, packingDocument{
			Parcel: allocation.Parcel().String(), Line: allocation.Line(), Quantity: allocation.Quantity(),
		})
	}
	return document
}

func goodsLineDocumentOf(line domain.GoodsLine) goodsLineDocument {
	document := goodsLineDocument{
		Quantity:      goodsCellDocumentOf(line.Quantity()),
		Unit:          goodsCellDocumentOf(line.Unit()),
		UnitValue:     goodsCellDocumentOf(line.UnitValue()),
		LineValue:     goodsCellDocumentOf(line.LineValue()),
		Currency:      goodsCellDocumentOf(line.Currency()),
		NetWeight:     goodsCellDocumentOf(line.NetWeight()),
		NetWeightUnit: goodsCellDocumentOf(line.NetWeightUnit()),
		OriginCountry: goodsCellDocumentOf(line.OriginCountry()),
		OwnerSKU:      goodsCellDocumentOf(line.OwnerSKU()),
	}
	for _, description := range line.Descriptions() {
		document.Descriptions = append(document.Descriptions, goodsDescriptionDocument{
			Language: description.Language(),
			Text:     goodsCellDocument{Value: description.Text().Value(), Mark: description.Text().Mark().String()},
		})
	}
	for _, code := range line.CommodityCodes() {
		document.CommodityCodes = append(document.CommodityCodes, commodityCodeDocument{
			Side: code.Side().String(),
			Code: goodsCellDocument{Value: code.Code().Value(), Mark: code.Code().Mark().String()},
		})
	}
	if order, present := line.SalesOrder(); present {
		document.SalesOrder = &salesOrderDocument{
			Platform:    goodsCellDocumentOf(order.Platform()),
			OrderNumber: goodsCellDocumentOf(order.OrderNumber()),
			OrderLine:   goodsCellDocumentOf(order.OrderLine()),
		}
	}
	return document
}

func (document *goodsCellDocument) spec() (domain.GoodsCellSpec, error) {
	if document == nil {
		return domain.GoodsCellSpec{}, nil
	}
	mark, ok := domain.ParseDeclarationSourceMark(document.Mark)
	if !ok {
		return domain.GoodsCellSpec{}, fmt.Errorf("申报格来源标记 %q 不在封闭集合里", document.Mark)
	}
	return domain.GoodsCellSpec{Value: document.Value, Mark: mark}, nil
}

// goods 把快照里的货物明细读回领域值，nil 即这一版没报。读回不过接入的门（RehydrateGoodsDetails）；装箱分配是否贴合成员，
// 由聚合重建时核一次，这里不重复。
func (document *goodsDocument) goods() (domain.GoodsDetails, error) {
	if document == nil {
		return domain.GoodsDetails{}, nil
	}
	spec := domain.GoodsDetailsSpec{}
	if document.Category != "" {
		category, ok := domain.ParseContentCategory(document.Category)
		if !ok {
			return domain.GoodsDetails{}, fmt.Errorf("内件性质 %q 不在封闭集合里", document.Category)
		}
		mark, ok := domain.ParseDeclarationSourceMark(document.CategoryMark)
		if !ok {
			return domain.GoodsDetails{}, fmt.Errorf("内件性质的来源标记 %q 不在封闭集合里", document.CategoryMark)
		}
		spec.Category, spec.CategoryMark = category, mark
	}
	note, err := document.CategoryNote.spec()
	if err != nil {
		return domain.GoodsDetails{}, err
	}
	spec.CategoryNote = note
	for _, line := range document.Lines {
		lineSpec, err := line.spec()
		if err != nil {
			return domain.GoodsDetails{}, err
		}
		spec.Lines = append(spec.Lines, lineSpec)
	}
	for _, allocation := range document.Packing {
		parcel, err := domain.NewDeclaredParcelID(allocation.Parcel)
		if err != nil {
			return domain.GoodsDetails{}, err
		}
		spec.Packing = append(spec.Packing, domain.PackingAllocationSpec{Parcel: parcel, Line: allocation.Line, Quantity: allocation.Quantity})
	}
	return domain.RehydrateGoodsDetails(spec), nil
}

// cellBinding 把快照里的一格接到行输入的那一格上，读回逐格走同一段代码。
type cellBinding struct {
	from *goodsCellDocument
	to   *domain.GoodsCellSpec
}

func (document goodsLineDocument) spec() (domain.GoodsLineSpec, error) {
	var spec domain.GoodsLineSpec
	cells := []cellBinding{
		{document.Quantity, &spec.Quantity}, {document.Unit, &spec.Unit}, {document.UnitValue, &spec.UnitValue},
		{document.LineValue, &spec.LineValue}, {document.Currency, &spec.Currency}, {document.NetWeight, &spec.NetWeight},
		{document.NetWeightUnit, &spec.NetWeightUnit}, {document.OriginCountry, &spec.OriginCountry}, {document.OwnerSKU, &spec.OwnerSKU},
	}
	if document.SalesOrder != nil {
		cells = append(cells,
			cellBinding{document.SalesOrder.Platform, &spec.SalesOrder.Platform},
			cellBinding{document.SalesOrder.OrderNumber, &spec.SalesOrder.OrderNumber},
			cellBinding{document.SalesOrder.OrderLine, &spec.SalesOrder.OrderLine},
		)
	}
	for _, pair := range cells {
		value, err := pair.from.spec()
		if err != nil {
			return domain.GoodsLineSpec{}, err
		}
		*pair.to = value
	}
	for _, description := range document.Descriptions {
		text, err := (&description.Text).spec()
		if err != nil {
			return domain.GoodsLineSpec{}, err
		}
		spec.Descriptions = append(spec.Descriptions, domain.GoodsDescriptionSpec{Language: description.Language, Text: text})
	}
	for _, code := range document.CommodityCodes {
		side, ok := domain.ParseCommodityCodeSide(code.Side)
		if !ok {
			return domain.GoodsLineSpec{}, fmt.Errorf("编码口径 %q 不在封闭集合里", code.Side)
		}
		value, err := (&code.Code).spec()
		if err != nil {
			return domain.GoodsLineSpec{}, err
		}
		spec.CommodityCodes = append(spec.CommodityCodes, domain.CommodityCodeSpec{Side: side, Code: value})
	}
	return spec, nil
}

func platformShipmentDocumentOf(request domain.ShipmentRequest) *platformShipmentDocument {
	shipment, declared := request.PlatformShipment()
	if !declared {
		return nil
	}
	document := &platformShipmentDocument{Platform: shipment.Platform(), Shipment: shipment.ShipmentNumber().String()}
	if reference, present := shipment.ReferenceNumber(); present {
		document.Reference = reference.String()
	}
	return document
}

func (document *platformShipmentDocument) declaration() (domain.PlatformShipmentDeclaration, error) {
	if document == nil {
		return domain.PlatformShipmentDeclaration{}, nil
	}
	number, err := domain.RehydrateExternalIdentifierNumber(document.Shipment)
	if err != nil {
		return domain.PlatformShipmentDeclaration{}, err
	}
	return domain.RehydratePlatformShipment(document.Platform, number, document.Reference)
}

func (document platformBoxLabelDocument) declaration() (domain.PlatformBoxLabelDeclaration, error) {
	parcel, err := domain.NewDeclaredParcelID(document.Parcel)
	if err != nil {
		return domain.PlatformBoxLabelDeclaration{}, err
	}
	number, err := domain.RehydrateExternalIdentifierNumber(document.Number)
	if err != nil {
		return domain.PlatformBoxLabelDeclaration{}, err
	}
	return domain.RehydratePlatformBoxLabel(parcel, number)
}
