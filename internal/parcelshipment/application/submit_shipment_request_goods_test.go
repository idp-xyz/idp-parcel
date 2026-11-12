package application_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func goodsLine(description, quantity string) domain.GoodsLineSpec {
	return domain.GoodsLineSpec{
		Descriptions: []domain.GoodsDescriptionSpec{{Language: "en", Text: domain.GoodsCellSpec{Value: description, Mark: domain.SourceMarkThisDeclaration}}},
		Quantity:     domain.GoodsCellSpec{Value: quantity, Mark: domain.SourceMarkThisDeclaration},
	}
}

// packedGoods 是两行货物，parcel-1 装第一行、parcel-2 装第二行。parcels 是造它时对着的成员集合。
func packedGoods(t *testing.T, parcels ...string) domain.GoodsDetails {
	t.Helper()
	members := make([]domain.DeclaredParcelID, 0, len(parcels))
	for _, parcel := range parcels {
		members = append(members, mustValue(t, domain.NewDeclaredParcelID, parcel))
	}
	packing := []domain.PackingAllocationSpec{{Parcel: members[0], Line: 1, Quantity: "5"}}
	second := members[0]
	if len(members) > 1 {
		second = members[1]
	}
	packing = append(packing, domain.PackingAllocationSpec{Parcel: second, Line: 2, Quantity: "1"})
	goods, err := domain.NewGoodsDetails(domain.GoodsDetailsSpec{
		Lines:   []domain.GoodsLineSpec{goodsLine("Men's cotton T-shirt", "5"), goodsLine("Wrist watch", "1")},
		Packing: packing,
	}, members)
	if err != nil {
		t.Fatalf("new goods details: %v", err)
	}
	return goods
}

func platformDeclarations(t *testing.T, labelled ...string) (domain.PlatformShipmentDeclaration, []domain.PlatformBoxLabelDeclaration) {
	t.Helper()
	shipment, err := domain.DeclarePlatformShipment("AMAZON", mustValue(t, domain.NewExternalIdentifierNumber, "FBA15SYNTEST"), "SYN-REF-1")
	if err != nil {
		t.Fatalf("declare platform shipment: %v", err)
	}
	labels := make([]domain.PlatformBoxLabelDeclaration, 0, len(labelled))
	for index, parcel := range labelled {
		label, err := domain.DeclarePlatformBoxLabel(mustValue(t, domain.NewDeclaredParcelID, parcel),
			mustValue(t, domain.NewExternalIdentifierNumber, "FBA15SYNTESTU00000"+string(rune('1'+index))))
		if err != nil {
			t.Fatalf("declare box label: %v", err)
		}
		labels = append(labels, label)
	}
	return shipment, labels
}

// Covers: ADR-0373 决定五、ADR-0374 决定四——建单把命令带来的货物明细与平台声明交给聚合，落库的委托带着它们（`AT-PS-188` 的编排半边）。
func TestTheStoredRequestCarriesTheDeclaredGoodsAndPlatformDeclarations(t *testing.T) {
	fixture := newFixture(t)
	command := twoParcelCommand(t, fixture, "parcel-1", "parcel-2")
	command.DeclaredGoods = packedGoods(t, "parcel-1", "parcel-2")
	command.PlatformShipment, command.PlatformBoxLabels = platformDeclarations(t, "parcel-1", "parcel-2")

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil || result.Outcome() != application.OutcomeSubmitted {
		t.Fatalf("outcome = %q, err = %v, want SUBMITTED", result.Outcome(), err)
	}
	stored, found := fixture.requests.stored(t, fixture.identity(t))
	if !found {
		t.Fatal("no shipment request was stored")
	}
	if goods := stored.CurrentSubmissionVersion().Goods(); len(goods.Lines()) != 2 || len(goods.Packing()) != 2 {
		t.Fatalf("stored goods = %d lines %d packing", len(goods.Lines()), len(goods.Packing()))
	}
	if _, declared := stored.PlatformShipment(); !declared || len(stored.PlatformBoxLabels()) != 2 {
		t.Fatal("stored request lost its platform declarations")
	}
}

// Covers: ADR-0373 决定三、ADR-0374 决定四——货物明细或箱标号对不上成员集合，与客户包裹件号对不上同格：答`输入未受理`。
func TestGoodsOrBoxLabelsThatDoNotFitTheMembersAreNotAccepted(t *testing.T) {
	cases := map[string]func(*testing.T, *application.SubmitShipmentRequestCommand){
		"packing leaves a member out": func(t *testing.T, command *application.SubmitShipmentRequestCommand) {
			command.DeclaredGoods = packedGoods(t, "parcel-1")
		},
		"box label outside the request": func(t *testing.T, command *application.SubmitShipmentRequestCommand) {
			command.PlatformShipment, command.PlatformBoxLabels = platformDeclarations(t, "parcel-1", "parcel-9")
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newFixture(t)
			command := twoParcelCommand(t, fixture, "parcel-1", "parcel-2")
			mutate(t, &command)
			result, err := fixture.handler.Handle(context.Background(), command)
			if err != nil || result.Outcome() != application.OutcomeInputNotAccepted {
				t.Fatalf("outcome = %q, err = %v, want INPUT_NOT_ACCEPTED", result.Outcome(), err)
			}
		})
	}
}

// Covers: ADR-0373 决定四——补充整份重报货物明细，新版本带它自己的那一份。
func TestASupplementCarriesItsOwnGoodsDetails(t *testing.T) {
	fixture := newSupplementFixture(t)
	command := fixture.command(t)
	command.DeclaredGoods = packedGoods(t, "parcel-1", "parcel-2")

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil || result.Outcome() != application.SupplementRecorded {
		t.Fatalf("outcome = %q, err = %v, want SUPPLEMENT_RECORDED", result.Outcome(), err)
	}
	if fixture.requests.saved == nil {
		t.Fatal("supplement was not saved")
	}
	if goods := fixture.requests.saved.CurrentSubmissionVersion().Goods(); len(goods.Lines()) != 2 {
		t.Fatalf("new version goods = %d lines, want 2", len(goods.Lines()))
	}
}
