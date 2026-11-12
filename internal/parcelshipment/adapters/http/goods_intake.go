package shipmenthttp

import (
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件把首方载荷里的货物明细与平台声明译成领域值（ADR-0373、ADR-0374）。提交口、补充口与资料修订口共用：词表只此一份。
//
// 同一份货物明细要交两处——对着客户包裹参考的那一份进摘要（摘要按客户给的内容算，现签的内部标识不是内容），对着现签或
// 回执里的成员的那一份进命令（版本按成员取画像与分配）。只译、只校验一次：先对着客户参考造好，再用 WithPackingParcels
// 把装箱分配挪到成员上，两边因此说的是同一份内容。平台声明便宜，照旧对着两套标识各译一次。

// goodsDraft 是委托级的货物明细：内件性质与货物明细行。装箱分配写在各包裹草案上（packing），与每箱的平台箱标号同处。
type goodsDraft struct {
	ContentCategory     string           `json:"contentCategory"`
	ContentCategoryNote string           `json:"contentCategoryNote"`
	Lines               []goodsLineDraft `json:"lines"`
}

type goodsLineDraft struct {
	Descriptions   []goodsDescriptionDraft `json:"descriptions"`
	Quantity       string                  `json:"quantity"`
	Unit           string                  `json:"unit"`
	UnitValue      string                  `json:"unitValue"`
	LineValue      string                  `json:"lineValue"`
	Currency       string                  `json:"currency"`
	NetWeight      string                  `json:"netWeight"`
	NetWeightUnit  string                  `json:"netWeightUnit"`
	CommodityCodes []commodityCodeDraft    `json:"commodityCodes"`
	OriginCountry  string                  `json:"originCountry"`
	SKU            string                  `json:"sku"`
	SalesOrder     *salesOrderDraft        `json:"salesOrder"`
}

type goodsDescriptionDraft struct {
	Lang string `json:"lang"`
	Text string `json:"text"`
}

type commodityCodeDraft struct {
	Side string `json:"side"`
	Code string `json:"code"`
}

type salesOrderDraft struct {
	Platform    string `json:"platform"`
	OrderNumber string `json:"orderNumber"`
	OrderLine   string `json:"orderLine"`
}

// packingDraft 是一个包裹装了哪一行多少；行号从 1 起，与 goods.lines 的次序对应。
type packingDraft struct {
	Line     int    `json:"line"`
	Quantity string `json:"quantity"`
}

type platformShipmentDraft struct {
	Platform    string `json:"platform"`
	ShipmentID  string `json:"shipmentId"`
	ReferenceID string `json:"referenceId"`
}

// declared 是客户渠道交来的一格：一律记本单申报（PS CONTEXT「申报格来源标记」），由接入口记、不由客户声明。值原样，
// 不去空白——货物明细只认形状、不规范化（ADR-0373 决定六）。
func declared(value string) domain.GoodsCellSpec {
	return domain.GoodsCellSpec{Value: value, Mark: domain.SourceMarkThisDeclaration}
}

// declaredGoods 把草案的货物明细译成领域值；members 与 draft.Parcels 按次序对应，分配挂在它们上面。
func declaredGoods(draft submissionDraft, members []domain.DeclaredParcelID) (domain.GoodsDetails, error) {
	if len(members) != len(draft.Parcels) {
		return domain.GoodsDetails{}, fmt.Errorf("customer intake: %d parcels and %d members", len(draft.Parcels), len(members))
	}
	if draft.Goods == nil {
		for index, parcel := range draft.Parcels {
			if len(parcel.Packing) > 0 {
				return domain.GoodsDetails{}, malformedField(fmt.Sprintf("parcels[%d].packing", index),
					errors.New("packing needs goods lines to point at"))
			}
		}
		return domain.GoodsDetails{}, nil
	}
	spec, err := goodsSpecOf(*draft.Goods)
	if err != nil {
		return domain.GoodsDetails{}, err
	}
	for index, parcel := range draft.Parcels {
		for _, allocation := range parcel.Packing {
			spec.Packing = append(spec.Packing, domain.PackingAllocationSpec{
				Parcel: members[index], Line: allocation.Line, Quantity: allocation.Quantity,
			})
		}
	}
	// 装箱分配错写在包裹上（parcels[i].packing），其余写在 goods 上；原因里点名了是哪个包裹参考、哪一行。
	return goodsDetailsOf(spec, members, "parcels[].packing")
}

// goodsSpecOf 把委托级的内件性质与货物明细行译成领域规格；装箱分配由调用方按各口的写法另加。
func goodsSpecOf(draft goodsDraft) (domain.GoodsDetailsSpec, error) {
	spec := domain.GoodsDetailsSpec{}
	if draft.ContentCategory != "" {
		category, ok := domain.ParseContentCategory(draft.ContentCategory)
		if !ok {
			return domain.GoodsDetailsSpec{}, malformedField("goods.contentCategory",
				fmt.Errorf("%q is not one of the content categories", draft.ContentCategory))
		}
		spec.Category, spec.CategoryMark = category, domain.SourceMarkThisDeclaration
	}
	spec.CategoryNote = declared(draft.ContentCategoryNote)
	for index, line := range draft.Lines {
		lineSpec, err := declaredGoodsLine(line)
		if err != nil {
			return domain.GoodsDetailsSpec{}, malformedField(fmt.Sprintf("goods.lines[%d]", index), err)
		}
		spec.Lines = append(spec.Lines, lineSpec)
	}
	return spec, nil
}

// goodsDetailsOf 立货物明细，装箱分配的错指到 packingField，其余指到 goods。
func goodsDetailsOf(spec domain.GoodsDetailsSpec, members []domain.DeclaredParcelID, packingField string) (domain.GoodsDetails, error) {
	goods, err := domain.NewGoodsDetails(spec, members)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidPacking) {
			return domain.GoodsDetails{}, malformedField(packingField, err)
		}
		return domain.GoodsDetails{}, malformedField("goods", err)
	}
	return goods, nil
}

// amendmentPackingDraft 是资料修订口上的一条装箱分配。修订作用于已接受的委托，客户手里有提交答复带回的包裹号
// （ADR-0223），所以按 parcelId 指包裹，不按客户包裹参考——参考在接受后不再是这份委托的键。
type amendmentPackingDraft struct {
	ParcelID string `json:"parcelId"`
	Line     int    `json:"line"`
	Quantity string `json:"quantity"`
}

// amendedGoods 把资料修订口交来的整组新货物明细译成领域值。成员集合在这里不可得（修订口不读委托）：对着分配自己指名的
// 包裹立，只核分配的内部形状；指没指全、指没指到基线之外，由聚合对着接受基线核。
func amendedGoods(goods goodsDraft, packing []amendmentPackingDraft) (domain.GoodsDetails, error) {
	spec, err := goodsSpecOf(goods)
	if err != nil {
		return domain.GoodsDetails{}, err
	}
	var members []domain.DeclaredParcelID
	seen := make(map[domain.DeclaredParcelID]struct{})
	for index, allocation := range packing {
		parcel, err := domain.NewDeclaredParcelID(strings.TrimSpace(allocation.ParcelID))
		if err != nil {
			return domain.GoodsDetails{}, malformedField(fmt.Sprintf("packing[%d].parcelId", index), err)
		}
		if _, known := seen[parcel]; !known {
			seen[parcel] = struct{}{}
			members = append(members, parcel)
		}
		spec.Packing = append(spec.Packing, domain.PackingAllocationSpec{Parcel: parcel, Line: allocation.Line, Quantity: allocation.Quantity})
	}
	return goodsDetailsOf(spec, members, "packing")
}

// goodsOnMembers 把对着客户包裹参考造好的货物明细挪到成员上：customerParcels 与 members 按草案次序一一对应。
func goodsOnMembers(goods domain.GoodsDetails, customerParcels, members []domain.DeclaredParcelID) (domain.GoodsDetails, error) {
	if len(customerParcels) != len(members) {
		return domain.GoodsDetails{}, fmt.Errorf("customer intake: %d customer parcel references and %d members", len(customerParcels), len(members))
	}
	renamed := make(map[domain.DeclaredParcelID]domain.DeclaredParcelID, len(members))
	for index, reference := range customerParcels {
		renamed[reference] = members[index]
	}
	return goods.WithPackingParcels(renamed)
}

func declaredGoodsLine(draft goodsLineDraft) (domain.GoodsLineSpec, error) {
	spec := domain.GoodsLineSpec{
		Quantity:      declared(draft.Quantity),
		Unit:          declared(draft.Unit),
		UnitValue:     declared(draft.UnitValue),
		LineValue:     declared(draft.LineValue),
		Currency:      declared(draft.Currency),
		NetWeight:     declared(draft.NetWeight),
		NetWeightUnit: declared(draft.NetWeightUnit),
		OriginCountry: declared(draft.OriginCountry),
		OwnerSKU:      declared(draft.SKU),
	}
	for _, description := range draft.Descriptions {
		spec.Descriptions = append(spec.Descriptions, domain.GoodsDescriptionSpec{Language: description.Lang, Text: declared(description.Text)})
	}
	for _, code := range draft.CommodityCodes {
		side, ok := domain.ParseCommodityCodeSide(code.Side)
		if !ok {
			return domain.GoodsLineSpec{}, fmt.Errorf("%q is not a commodity code side", code.Side)
		}
		spec.CommodityCodes = append(spec.CommodityCodes, domain.CommodityCodeSpec{Side: side, Code: declared(code.Code)})
	}
	if draft.SalesOrder != nil {
		spec.SalesOrder = domain.SalesOrderReferenceSpec{
			Platform:    declared(draft.SalesOrder.Platform),
			OrderNumber: declared(draft.SalesOrder.OrderNumber),
			OrderLine:   declared(draft.SalesOrder.OrderLine),
		}
	}
	return spec, nil
}

// declaredPlatform 把草案的平台货件段与逐箱箱标号译成声明；members 与 draft.Parcels 按次序对应。箱标号与平台货件的搭配、
// 是否重号，由规范化与聚合核（platformBoxLabelsByParcel），这里只立声明。
func declaredPlatform(
	draft submissionDraft,
	members []domain.DeclaredParcelID,
) (domain.PlatformShipmentDeclaration, []domain.PlatformBoxLabelDeclaration, error) {
	if len(members) != len(draft.Parcels) {
		return domain.PlatformShipmentDeclaration{}, nil, fmt.Errorf("customer intake: %d parcels and %d members", len(draft.Parcels), len(members))
	}
	var shipment domain.PlatformShipmentDeclaration
	if draft.PlatformShipment != nil {
		number, err := domain.NewExternalIdentifierNumber(draft.PlatformShipment.ShipmentID)
		if err != nil {
			return domain.PlatformShipmentDeclaration{}, nil, malformedField("platformShipment.shipmentId", err)
		}
		shipment, err = domain.DeclarePlatformShipment(draft.PlatformShipment.Platform, number, draft.PlatformShipment.ReferenceID)
		if err != nil {
			return domain.PlatformShipmentDeclaration{}, nil, malformedField("platformShipment", err)
		}
	}
	var labels []domain.PlatformBoxLabelDeclaration
	for index, parcel := range draft.Parcels {
		if parcel.PlatformBoxLabel == "" {
			continue
		}
		field := fmt.Sprintf("parcels[%d].platformBoxLabel", index)
		number, err := domain.NewExternalIdentifierNumber(parcel.PlatformBoxLabel)
		if err != nil {
			return domain.PlatformShipmentDeclaration{}, nil, malformedField(field, err)
		}
		label, err := domain.DeclarePlatformBoxLabel(members[index], number)
		if err != nil {
			return domain.PlatformShipmentDeclaration{}, nil, malformedField(field, err)
		}
		labels = append(labels, label)
	}
	return shipment, labels, nil
}
