package application_test

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件钉资料修订编排在委托级资料组「货物明细」上的走法（ADR-0373 决定四，票 declared-goods-lines/06，AT-PS-185）：允许
// 声明那一格允许才形成版本，没声明答等复核；装箱分配以接受基线为界，在问矩阵之前就答。

func goodsDataScope(t *testing.T) domain.SourceDataScope {
	t.Helper()
	scope, err := domain.NewShipmentScopedSourceData(mustValue(t, domain.NewShipmentRequestID, "request-1"), domain.GoodsDetailsDataGroup())
	if err != nil {
		t.Fatalf("货物明细范围：%v", err)
	}
	return scope
}

// goodsContent 造一份一行货的货物明细内容，parcels 是装箱分配指的包裹，每箱装 1。
func goodsContent(t *testing.T, parcels ...string) domain.SourceDataVersionContent {
	t.Helper()
	cell := func(value string) domain.GoodsCellSpec {
		return domain.GoodsCellSpec{Value: value, Mark: domain.SourceMarkThisDeclaration}
	}
	spec := domain.GoodsDetailsSpec{Lines: []domain.GoodsLineSpec{{
		Descriptions: []domain.GoodsDescriptionSpec{{Language: "en", Text: cell("T-shirt")}},
		Quantity:     cell(strconv.Itoa(len(parcels))),
	}}}
	members := make([]domain.DeclaredParcelID, 0, len(parcels))
	for _, raw := range parcels {
		parcel := mustValue(t, domain.NewDeclaredParcelID, raw)
		members = append(members, parcel)
		spec.Packing = append(spec.Packing, domain.PackingAllocationSpec{Parcel: parcel, Line: 1, Quantity: "1"})
	}
	if len(parcels) == 0 {
		spec.Lines[0].Quantity = cell("2")
	}
	goods, err := domain.NewGoodsDetails(spec, members)
	if err != nil {
		t.Fatalf("货物明细：%v", err)
	}
	content, err := domain.NewGoodsDetailsContent(goods)
	if err != nil {
		t.Fatalf("货物明细内容：%v", err)
	}
	return content
}

// Covers: AT-PS-185「允许声明那一格允许时形成新版本」——整组货物明细原样进版本，保存后在用的货物明细就是这一版的。
func TestAnAllowedGoodsAmendmentFormsAVersionAndBecomesTheGoodsInEffect(t *testing.T) {
	fixture := newAmendmentFixture(t)
	command := fixture.command(t)
	command.Scope = goodsDataScope(t)
	command.Content = goodsContent(t, "parcel-1", "parcel-2")

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.AmendmentRecorded {
		t.Fatalf("outcome = %v, want RECORDED", result.Outcome())
	}
	if fixture.requests.saved == nil {
		t.Fatal("版本形成了却没保存")
	}
	goods, standing := fixture.requests.saved.GoodsDetailsInEffect()
	if standing != domain.GoodsDetailsOnAdoptedVersion || len(goods.Packing()) != 2 {
		t.Fatalf("在用的货物明细：standing = %v、%d 条分配", standing, len(goods.Packing()))
	}
}

// Covers: AT-PS-185「那一格没声明答等复核」——资料组是开放引用，`party-commercial` 没登这一格就是还没人说这处能不能改，
// 不形成版本。
func TestAGoodsAmendmentWithoutADeclaredAllowanceAwaitsReview(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.rules.allowance = ports.SourceDataAmendmentNotDeclared
	command := fixture.command(t)
	command.Scope = goodsDataScope(t)
	command.Content = goodsContent(t, "parcel-1", "parcel-2")

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.AmendmentAwaitingReview {
		t.Fatalf("outcome = %v, want AWAITING_REVIEW", result.Outcome())
	}
	if _, formed := result.Version(); formed || fixture.requests.saved != nil {
		t.Fatal("没声明的那一格形成或保存了版本")
	}
}

// Covers: ADR-0373 决定四「增删行或改分配不是增删委托成员」——装箱分配指到接受基线外的包裹，与范围越过基线同答不允许，且排在
// 矩阵之前（矩阵一次读不回也不该把它变成未决）；漏了成员是调用方的错，答内容不配。
func TestAGoodsAmendmentsPackingIsCheckedAgainstTheBaselineBeforeTheMatrix(t *testing.T) {
	outside := newAmendmentFixture(t)
	outside.rules.err = errors.New("rule declaration unreachable")
	command := outside.command(t)
	command.Scope = goodsDataScope(t)
	command.Content = goodsContent(t, "parcel-1", "parcel-2", "parcel-9")
	result, err := outside.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.AmendmentDisallowed {
		t.Fatalf("指到基线外：outcome = %v, want DISALLOWED", result.Outcome())
	}

	partial := newAmendmentFixture(t)
	command = partial.command(t)
	command.Scope = goodsDataScope(t)
	command.Content = goodsContent(t, "parcel-1")
	if _, err := partial.handler.Handle(context.Background(), command); !errors.Is(err, application.ErrAmendmentContentUnfit) ||
		!errors.Is(err, domain.ErrInvalidPacking) {
		t.Fatalf("漏了成员：err = %v, want ErrAmendmentContentUnfit 且 ErrInvalidPacking", err)
	}
}

// Covers: 补充与更正交来完整的新货物明细——不带货物明细的更正在授权之前就答内容不配：整组替换下它读出来就是清空。
func TestAGoodsCorrectionWithoutGoodsIsRefusedBeforeAuthorization(t *testing.T) {
	fixture := newAmendmentFixture(t)
	command := fixture.command(t)
	command.Scope = goodsDataScope(t)
	if _, err := fixture.handler.Handle(context.Background(), command); !errors.Is(err, application.ErrAmendmentContentUnfit) {
		t.Fatalf("err = %v, want ErrAmendmentContentUnfit", err)
	}
	if fixture.authorizer.calls != 0 {
		t.Fatalf("问了 %d 次授权", fixture.authorizer.calls)
	}
}
