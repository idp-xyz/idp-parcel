package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// shipmentByNumberDouble 是按运单号读口的替身：record 与 found 原样交回，err 非空即读不动；asked 记被问的键。
type shipmentByNumberDouble struct {
	record ports.CustomerShipmentByDocumentNumberRecord
	found  bool
	err    error
	asked  []string
}

func (double *shipmentByNumberDouble) FindCustomerShipmentByDocumentNumber(
	_ context.Context,
	tenant domain.TenantID,
	customer domain.CustomerAccountID,
	number domain.ShipmentDocumentNumber,
) (ports.CustomerShipmentByDocumentNumberRecord, bool, error) {
	double.asked = append(double.asked, tenant.String()+"/"+customer.String()+"/"+number.String())
	return double.record, double.found, double.err
}

// effectiveParcels 造 count 件当前有效包裹，位次 1..count，第奇数件带件号。
func effectiveParcels(t *testing.T, count int) []domain.EffectiveParcel {
	t.Helper()
	members := make([]domain.DeclaredParcelID, count)
	var numbers []domain.CustomerParcelNumberDeclaration
	for index := range members {
		members[index] = mustValue(t, domain.NewDeclaredParcelID, fmt.Sprintf("parcel-%03d", index+1))
		if index%2 == 0 {
			declaration, err := domain.DeclareCustomerParcelNumber(members[index],
				mustValue(t, domain.NewExternalIdentifierNumber, fmt.Sprintf("CUST-%03d", index+1)))
			if err != nil {
				t.Fatal(err)
			}
			numbers = append(numbers, declaration)
		}
	}
	parcels, err := domain.CurrentEffectiveParcelsOfBaseline(members, members, numbers)
	if err != nil {
		t.Fatalf("立当前有效包裹：%v", err)
	}
	return parcels
}

func byNumberQuery(t *testing.T, after, limit int) application.ReadCustomerShipmentByDocumentNumberQuery {
	t.Helper()
	return application.ReadCustomerShipmentByDocumentNumberQuery{
		Tenant:        mustValue(t, domain.NewTenantID, "tenant-1"),
		Customer:      mustValue(t, domain.NewCustomerAccountID, "customer-1"),
		Number:        mustValue(t, domain.NewShipmentDocumentNumber, "P20261102000001"),
		AfterPosition: after,
		Limit:         limit,
	}
}

func byNumberHandler(t *testing.T, double *shipmentByNumberDouble) *application.ReadCustomerShipmentByDocumentNumberHandler {
	t.Helper()
	handler, err := application.NewReadCustomerShipmentByDocumentNumberHandler(double)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func positionsOf(parcels []domain.EffectiveParcel) []int {
	positions := make([]int, len(parcels))
	for index, parcel := range parcels {
		positions[index] = parcel.Position()
	}
	return positions
}

// Covers: ADR-0356 决定三「分页」的提供方半边——已接受委托的当前有效包裹按位次逐页取完：每页至多 limit 件、严格排在上一页末件
// 之后，Total 是全集件数、每页相同，末页 More 为假；起点越过末件答空页，不答查无。读口只被问一次、键原样交过去。
func TestAnAcceptedShipmentIsReadPageByPageInPositionOrder(t *testing.T) {
	double := &shipmentByNumberDouble{
		record: ports.CustomerShipmentByDocumentNumberRecord{State: domain.ShipmentRequestAccepted, EffectiveParcels: effectiveParcels(t, 5)},
		found:  true,
	}
	handler := byNumberHandler(t, double)

	var read []int
	after := 0
	for page := 1; ; page++ {
		result, err := handler.Handle(t.Context(), byNumberQuery(t, after, 2))
		if err != nil {
			t.Fatalf("第 %d 页：%v", page, err)
		}
		if !result.Found() || result.State() != domain.ShipmentRequestAccepted || result.Total() != 5 {
			t.Fatalf("第 %d 页：found=%v state=%s total=%d", page, result.Found(), result.State(), result.Total())
		}
		parcels := result.Parcels()
		if len(parcels) == 0 || len(parcels) > 2 {
			t.Fatalf("第 %d 页 %d 件，want 1..2", page, len(parcels))
		}
		read = append(read, positionsOf(parcels)...)
		after = parcels[len(parcels)-1].Position()
		if !result.More() {
			break
		}
	}
	if fmt.Sprint(read) != "[1 2 3 4 5]" {
		t.Fatalf("逐页读到的位次 = %v", read)
	}
	if len(double.asked) != 3 || double.asked[0] != "tenant-1/customer-1/P20261102000001" {
		t.Fatalf("读口被问 = %v", double.asked)
	}

	beyond, err := handler.Handle(t.Context(), byNumberQuery(t, 5, 2))
	if err != nil || !beyond.Found() || len(beyond.Parcels()) != 0 || beyond.More() || beyond.Total() != 5 {
		t.Fatalf("越过末件：found=%v parcels=%v more=%v total=%d err=%v", beyond.Found(), positionsOf(beyond.Parcels()), beyond.More(), beyond.Total(), err)
	}
}

// Covers: ADR-0356 决定二的提供方半边——已提交、已拒绝、已撤回只答状态、不带包裹；查无答 Found 为假；读不动交回错误，不折成查无。
func TestOtherStatesAnswerTheirStateAndNothingElse(t *testing.T) {
	for _, state := range []domain.ShipmentRequestState{domain.ShipmentRequestSubmitted, domain.ShipmentRequestRejected, domain.ShipmentRequestWithdrawn} {
		result, err := byNumberHandler(t, &shipmentByNumberDouble{record: ports.CustomerShipmentByDocumentNumberRecord{State: state}, found: true}).
			Handle(t.Context(), byNumberQuery(t, 0, 50))
		if err != nil || !result.Found() || result.State() != state || len(result.Parcels()) != 0 || result.Total() != 0 || result.More() {
			t.Errorf("%s：found=%v state=%s parcels=%d total=%d err=%v", state, result.Found(), result.State(), len(result.Parcels()), result.Total(), err)
		}
	}

	missing, err := byNumberHandler(t, &shipmentByNumberDouble{}).Handle(t.Context(), byNumberQuery(t, 0, 50))
	if err != nil || missing.Found() {
		t.Fatalf("查无：found=%v err=%v", missing.Found(), err)
	}

	down := errors.New("store down")
	if _, err := byNumberHandler(t, &shipmentByNumberDouble{err: down}).Handle(t.Context(), byNumberQuery(t, 0, 50)); !errors.Is(err, down) {
		t.Fatalf("读不动 = %v, want 包住原错误", err)
	}
}

// Covers: 立不起来的输入照实报错，不答一页——页大小非正、起点为负（调用方的编程错误，不问读口）；已接受却没有件、未接受却带了件
// （读回的数据坏了）。
func TestUnformablePagesAreErrorsNotAnswers(t *testing.T) {
	double := &shipmentByNumberDouble{
		record: ports.CustomerShipmentByDocumentNumberRecord{State: domain.ShipmentRequestAccepted, EffectiveParcels: effectiveParcels(t, 1)},
		found:  true,
	}
	for name, query := range map[string]application.ReadCustomerShipmentByDocumentNumberQuery{
		"zero limit":     byNumberQuery(t, 0, 0),
		"negative after": byNumberQuery(t, -1, 50),
	} {
		if _, err := byNumberHandler(t, double).Handle(t.Context(), query); !errors.Is(err, application.ErrInvalidEffectiveParcelPage) {
			t.Errorf("%s：%v", name, err)
		}
	}
	if len(double.asked) != 0 {
		t.Fatalf("坏的页请求问了读口 %d 次", len(double.asked))
	}

	for name, record := range map[string]ports.CustomerShipmentByDocumentNumberRecord{
		"accepted without parcels": {State: domain.ShipmentRequestAccepted},
		"submitted with parcels":   {State: domain.ShipmentRequestSubmitted, EffectiveParcels: effectiveParcels(t, 1)},
	} {
		if _, err := byNumberHandler(t, &shipmentByNumberDouble{record: record, found: true}).Handle(t.Context(), byNumberQuery(t, 0, 50)); !errors.Is(err, domain.ErrInconsistentEffectiveParcels) {
			t.Errorf("%s：%v", name, err)
		}
	}
}
