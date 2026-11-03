package parcelshipment_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/parcelshipment"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件证按运单号整单读口的适配器（ADR-0356 决定四；ADR-0025）：PS 每个委托状态各有落点、查无译查无此单、PS 读不动不译成查无
// 此单而原样上抛、词表外的状态不兜底；列出时件、位次、件号与分页原样转述，键（租户、账户、号、页）原样交给 PS。PS 那一侧用真
// 应用读口，只把最底下的读口换成替身——分页与状态判断是 PS 的，这里只证翻译。

type shipmentByNumberViewDouble struct {
	record psports.CustomerShipmentByDocumentNumberRecord
	found  bool
	err    error
	keys   []string
}

func (double *shipmentByNumberViewDouble) FindCustomerShipmentByDocumentNumber(
	_ context.Context,
	tenant psdomain.TenantID,
	customer psdomain.CustomerAccountID,
	number psdomain.ShipmentDocumentNumber,
) (psports.CustomerShipmentByDocumentNumberRecord, bool, error) {
	double.keys = append(double.keys, tenant.String()+"/"+customer.String()+"/"+number.String())
	return double.record, double.found, double.err
}

func shipmentLookupOver(t *testing.T, view *shipmentByNumberViewDouble) *adapter.CustomerShipmentLookup {
	t.Helper()
	reads, err := psapplication.NewReadCustomerShipmentByDocumentNumberHandler(view)
	if err != nil {
		t.Fatal(err)
	}
	lookup, err := adapter.NewCustomerShipmentLookup(reads)
	if err != nil {
		t.Fatal(err)
	}
	return lookup
}

func locate(t *testing.T, lookup *adapter.CustomerShipmentLookup, page veports.ShipmentParcelPage) (veports.LocatedCustomerShipment, error) {
	t.Helper()
	return lookup.LocateCustomerShipment(t.Context(),
		lookupValue(t, vedomain.NewTenantID, "tenant-1"),
		lookupValue(t, vedomain.NewCustomerAccountReference, "account-9"),
		lookupValue(t, vedomain.NewShipmentDocumentNumberReference, "P20261102000001"),
		page)
}

// threeEffectiveParcels 是 PS 一侧立出的三件：p-1 带件号，p-2 没有，p-3 带件号。
func threeEffectiveParcels(t *testing.T) []psdomain.EffectiveParcel {
	t.Helper()
	members := []psdomain.DeclaredParcelID{
		lookupValue(t, psdomain.NewDeclaredParcelID, "p-1"),
		lookupValue(t, psdomain.NewDeclaredParcelID, "p-2"),
		lookupValue(t, psdomain.NewDeclaredParcelID, "p-3"),
	}
	var numbers []psdomain.CustomerParcelNumberDeclaration
	for _, pair := range [][2]string{{"p-1", "CUST-1"}, {"p-3", "CUST-3"}} {
		declaration, err := psdomain.DeclareCustomerParcelNumber(lookupValue(t, psdomain.NewDeclaredParcelID, pair[0]),
			lookupValue(t, psdomain.NewExternalIdentifierNumber, pair[1]))
		if err != nil {
			t.Fatal(err)
		}
		numbers = append(numbers, declaration)
	}
	parcels, err := psdomain.CurrentEffectiveParcelsOfBaseline(members, members, numbers)
	if err != nil {
		t.Fatal(err)
	}
	return parcels
}

// Covers: ADR-0356 决定四「翻译是全函数」——已提交译待接受，已拒绝与已撤回译没有服务，查无译查无此单（不分成因），已接受译逐件列出；
// 键原样交给 PS：租户、请求方账户与号都在里面。
func TestEveryShipmentRequestStateHasItsStanding(t *testing.T) {
	for name, testCase := range map[string]struct {
		view *shipmentByNumberViewDouble
		want veports.ShipmentTrackingStanding
	}{
		"submitted": {&shipmentByNumberViewDouble{record: psports.CustomerShipmentByDocumentNumberRecord{State: psdomain.ShipmentRequestSubmitted}, found: true}, veports.ShipmentAwaitingAcceptance},
		"rejected":  {&shipmentByNumberViewDouble{record: psports.CustomerShipmentByDocumentNumberRecord{State: psdomain.ShipmentRequestRejected}, found: true}, veports.ShipmentClosedWithoutService},
		"withdrawn": {&shipmentByNumberViewDouble{record: psports.CustomerShipmentByDocumentNumberRecord{State: psdomain.ShipmentRequestWithdrawn}, found: true}, veports.ShipmentClosedWithoutService},
		"not found": {&shipmentByNumberViewDouble{}, veports.ShipmentNotFound},
		"accepted": {&shipmentByNumberViewDouble{record: psports.CustomerShipmentByDocumentNumberRecord{
			State: psdomain.ShipmentRequestAccepted, EffectiveParcels: threeEffectiveParcels(t)}, found: true}, veports.ShipmentParcelsListed},
	} {
		located, err := locate(t, shipmentLookupOver(t, testCase.view), veports.ShipmentParcelPage{Limit: 50})
		if err != nil || located.Standing != testCase.want {
			t.Errorf("%s：standing=%s err=%v, want %s", name, located.Standing, err, testCase.want)
		}
		if testCase.want != veports.ShipmentParcelsListed && (len(located.Parcels) != 0 || located.Total != 0 || located.More) {
			t.Errorf("%s 带了件：%+v", name, located)
		}
		if len(testCase.view.keys) != 1 || testCase.view.keys[0] != "tenant-1/account-9/P20261102000001" {
			t.Errorf("%s：交给 PS 的键 = %v", name, testCase.view.keys)
		}
	}
}

// Covers: ADR-0356 决定三的翻译半边——列出时每件的包裹引用、位次与件号原样转述，没声明件号的件不带；页与 More、Total 照 PS 的分页。
// 谱系落地之前来源关系恒空。
func TestListedParcelsAreTranslatedPageByPage(t *testing.T) {
	lookup := shipmentLookupOver(t, &shipmentByNumberViewDouble{record: psports.CustomerShipmentByDocumentNumberRecord{
		State: psdomain.ShipmentRequestAccepted, EffectiveParcels: threeEffectiveParcels(t)}, found: true})

	first, err := locate(t, lookup, veports.ShipmentParcelPage{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	second, err := locate(t, lookup, veports.ShipmentParcelPage{AfterPosition: first.Parcels[len(first.Parcels)-1].Position, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	describe := func(page veports.LocatedCustomerShipment) string {
		text := fmt.Sprintf("total=%d more=%v", page.Total, page.More)
		for _, parcel := range page.Parcels {
			text += fmt.Sprintf(" %d:%s", parcel.Position, parcel.Parcel)
			if parcel.HasCustomerParcelReference {
				text += "=" + parcel.CustomerParcelReference.String()
			}
			if len(parcel.Replaces) != 0 {
				text += fmt.Sprintf("<%v", parcel.Replaces)
			}
		}
		return text
	}
	if got := describe(first); got != "total=3 more=true 1:p-1=CUST-1 2:p-2" {
		t.Errorf("第一页 = %s", got)
	}
	if got := describe(second); got != "total=3 more=false 3:p-3=CUST-3" {
		t.Errorf("第二页 = %s", got)
	}
}

// Covers: `AT-VE-236` 的适配器半边——PS 读不动原样上抛，不译成查无此单；PS 的数据立不起当前有效包裹（已接受却没有件）同样是错，
// 不是任何一种整单答复。
func TestAnUnreadableShipmentIsAnErrorNotShipmentNotFound(t *testing.T) {
	down := errors.New("parcel shipment store down")
	located, err := locate(t, shipmentLookupOver(t, &shipmentByNumberViewDouble{err: down}), veports.ShipmentParcelPage{Limit: 50})
	if !errors.Is(err, down) || located.Standing != veports.ShipmentTrackingStandingInvalid {
		t.Fatalf("PS 读不动：standing=%s err=%v, want 原错误、无整单答复", located.Standing, err)
	}

	broken := &shipmentByNumberViewDouble{record: psports.CustomerShipmentByDocumentNumberRecord{State: psdomain.ShipmentRequestAccepted}, found: true}
	if located, err := locate(t, shipmentLookupOver(t, broken), veports.ShipmentParcelPage{Limit: 50}); err == nil || located.Standing != veports.ShipmentTrackingStandingInvalid {
		t.Fatalf("已接受却没有件：standing=%s err=%v", located.Standing, err)
	}
}

// Covers: ADR-0356 决定四「不留兜底」——PS 交来词表外的状态，包住本上下文的具名哨兵上抛，不落四种里的任何一种。
func TestAStateOutsideTheVocabularyIsUntranslatable(t *testing.T) {
	view := &shipmentByNumberViewDouble{record: psports.CustomerShipmentByDocumentNumberRecord{State: psdomain.ShipmentRequestState(99)}, found: true}
	located, err := locate(t, shipmentLookupOver(t, view), veports.ShipmentParcelPage{Limit: 50})
	if !errors.Is(err, veports.ErrCustomerShipmentUntranslatable) || located.Standing != veports.ShipmentTrackingStandingInvalid {
		t.Fatalf("词表外的状态：standing=%s err=%v", located.Standing, err)
	}
}
