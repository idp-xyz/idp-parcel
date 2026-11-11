package postgres_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func cell(value string, mark domain.DeclarationSourceMark) domain.GoodsCellSpec {
	return domain.GoodsCellSpec{Value: value, Mark: mark}
}

// roundTripGoods 把货物明细的每一格都报上，来源标记三格都用到，供往返逐格比对。
func roundTripGoods(t *testing.T) domain.GoodsDetails {
	t.Helper()
	this, box, commodity := domain.SourceMarkThisDeclaration, domain.SourceMarkBoxList, domain.SourceMarkCommodityDeclaration
	parcel1 := mustBuild(t, domain.NewDeclaredParcelID, "parcel-1")
	parcel2 := mustBuild(t, domain.NewDeclaredParcelID, "parcel-2")
	goods, err := domain.NewGoodsDetails(domain.GoodsDetailsSpec{
		Category:     domain.ContentCategoryOther,
		CategoryMark: this,
		CategoryNote: cell("spare parts for repair", this),
		Lines: []domain.GoodsLineSpec{
			{
				Descriptions: []domain.GoodsDescriptionSpec{
					{Language: "en", Text: cell("Men's cotton T-shirt", box)},
					{Language: "zh", Text: cell("男式棉质T恤", box)},
				},
				Quantity: cell("5", box), Unit: cell("PCS", box),
				UnitValue: cell("3.20", commodity), LineValue: cell("12.00", this), Currency: cell("USD", commodity),
				NetWeight: cell("0.9", this), NetWeightUnit: cell("KG", this),
				CommodityCodes: []domain.CommodityCodeSpec{
					{Side: domain.CommodityCodeExportSide, Code: cell("6109100000", commodity)},
					{Side: domain.CommodityCodeImportSide, Code: cell("6109.10.0012", this)},
				},
				OriginCountry: cell("CN", commodity), OwnerSKU: cell("SKU-TEE-1", this),
				SalesOrder: domain.SalesOrderReferenceSpec{Platform: cell("SHOPIFY", this), OrderNumber: cell("1001", this), OrderLine: cell("1", this)},
			},
			{Descriptions: []domain.GoodsDescriptionSpec{{Language: "en", Text: cell("Wrist watch", this)}}, Quantity: cell("1", this)},
		},
		Packing: []domain.PackingAllocationSpec{
			{Parcel: parcel1, Line: 1, Quantity: "3"}, {Parcel: parcel2, Line: 1, Quantity: "2"}, {Parcel: parcel2, Line: 2, Quantity: "1"},
		},
	}, []domain.DeclaredParcelID{parcel1, parcel2})
	if err != nil {
		t.Fatalf("货物明细：%v", err)
	}
	return goods
}

// Covers: ADR-0373 决定五、ADR-0374 决定四——货物明细随提交版本、平台声明随委托进快照并原样读回；没报的读回仍是没报，零迁移。
func TestGoodsDetailsAndPlatformDeclarationsRoundTrip(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()

	spec := submittedShipmentRequestSpec(t, "req-key-1", "request-1")
	spec.Goods = roundTripGoods(t)
	shipment, err := domain.DeclarePlatformShipment("AMAZON", mustBuild(t, domain.NewExternalIdentifierNumber, "FBA15SYNTEST"), "SYN-REF-1")
	if err != nil {
		t.Fatalf("平台货件：%v", err)
	}
	spec.PlatformShipment = shipment
	for index, parcel := range []string{"parcel-1", "parcel-2"} {
		label, err := domain.DeclarePlatformBoxLabel(mustBuild(t, domain.NewDeclaredParcelID, parcel),
			mustBuild(t, domain.NewExternalIdentifierNumber, []string{"FBA15SYNTESTU000001", "FBA15SYNTESTU000002"}[index]))
		if err != nil {
			t.Fatalf("箱标号：%v", err)
		}
		spec.PlatformBoxLabels = append(spec.PlatformBoxLabels, label)
	}
	declared, err := domain.SubmitShipmentRequest(spec)
	if err != nil {
		t.Fatalf("建单：%v", err)
	}
	mustInsert(t, transactor, ctx, repository, declared)
	mustInsert(t, transactor, ctx, repository, submittedShipmentRequest(t, "req-key-2", "request-2"))

	found := mustFind(t, repository, ctx, "req-key-1")
	// 逐格比对用规范化摘要：两份货物明细摘要相同，就是每一格、每个标记、每条分配都相同——比手写逐格断言不漏格。
	if digestWithGoods(t, found.CurrentSubmissionVersion().Goods(), found) != digestWithGoods(t, declared.CurrentSubmissionVersion().Goods(), declared) {
		t.Fatal("读回的货物明细或平台声明与写入的不同")
	}
	if category, _, present := found.CurrentSubmissionVersion().Goods().Category(); !present || category != domain.ContentCategoryOther {
		t.Fatalf("内件性质读回成 %v present=%v", category, present)
	}

	plain := mustFind(t, repository, ctx, "req-key-2")
	if plain.CurrentSubmissionVersion().Goods().Declared() {
		t.Fatal("没报货物明细的版本读回成了报了")
	}
	if _, declared := plain.PlatformShipment(); declared || len(plain.PlatformBoxLabels()) != 0 {
		t.Fatal("没报平台声明的委托读回成了报了")
	}
}

// digestWithGoods 把一份委托的成员、货物明细与平台声明交给领域规范化，得出可比的摘要。
func digestWithGoods(t *testing.T, goods domain.GoodsDetails, request domain.ShipmentRequest) string {
	t.Helper()
	shipment, _ := request.PlatformShipment()
	digest, err := domain.CanonicalizeSubmissionPayload(domain.SubmissionPayloadSpec{
		RequestReference:  request.ShipmentRequestID(),
		DeclaredParcelIDs: request.CurrentSubmissionVersion().DeclaredParcelIDs(),
		Goods:             goods,
		PlatformShipment:  shipment,
		PlatformBoxLabels: request.PlatformBoxLabels(),
	})
	if err != nil {
		t.Fatalf("规范化：%v", err)
	}
	return digest.String()
}
