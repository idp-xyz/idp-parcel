package partycommercial_test

import (
	"context"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

type keyStoreDouble struct {
	row   adapter.ResolutionKeyRow
	found bool
}

func (store keyStoreDouble) RegisterResolutionKey(context.Context, adapter.ResolutionKeyRow) (adapter.ResolutionKeySaveOutcome, error) {
	return adapter.ResolutionKeySaveOutcomeInvalid, nil
}

func (store keyStoreDouble) FindResolutionKey(_ context.Context, tenant, customer string) (adapter.ResolutionKeyRow, bool, error) {
	if !store.found || tenant != store.row.TenantID || customer != store.row.CustomerAccountID {
		return adapter.ResolutionKeyRow{}, false, nil
	}
	return store.row, true, nil
}

// Covers: AT-PS-142——直客修订的商业坐标映射（ADR-0311）：法人与范围取自解析键登记行，等级是产品常量 CUSTOMER_SELF_SERVICE，
// 原因原样转写，证据指向被修订的委托与资料范围，时点取时钟，请求方种类是客户账户；没登记解析键即「折不出来」，不拿别的顶。
func TestCustomerSelfAmendmentCoordinatesComeFromTheResolutionKeyAndTheProductConstant(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	row := adapter.ResolutionKeyRow{TenantID: "SYN-TENANT-01", CustomerAccountID: "SYN-ACCOUNT-01", Scope: "SYN-SCOPE-01", LegalEntity: "SYN-LE-01"}
	identity, err := psdomain.NewSourceIdentity(
		mustID(t, psdomain.NewTenantID, "SYN-TENANT-01"), mustID(t, psdomain.NewCustomerAccountID, "SYN-ACCOUNT-01"),
		mustID(t, psdomain.NewSource, "SYN-SOURCE-A"), mustID(t, psdomain.NewSourceRequestKey, "SYN-KEY-1"))
	if err != nil {
		t.Fatal(err)
	}
	scope, err := psdomain.NewShipmentScopedSourceData(mustID(t, psdomain.NewShipmentRequestID, "SHR-1"), psdomain.DeliveryPlaceDataGroup())
	if err != nil {
		t.Fatal(err)
	}
	query := psports.SourceDataAmendmentAuthorizationQuery{
		Identity: identity, Scope: scope,
		Requester: mustID(t, psdomain.NewRequesterReference, "CUSTOMER_ACCOUNT:SYN-ACCOUNT-01"),
		Reason:    mustID(t, psdomain.NewAmendmentReasonReference, "SYN-REASON-1"),
	}
	source, err := adapter.NewCustomerSelfAmendmentRequestSource(keyStoreDouble{row: row, found: true}, fixedClock{at: now})
	if err != nil {
		t.Fatal(err)
	}
	got, formed, err := source.FormRequestCoordinates(t.Context(), query)
	if err != nil || !formed {
		t.Fatalf("formed=%v err=%v", formed, err)
	}
	if got.RequesterKind != pcdomain.CustomerAccountRequester || got.LegalEntity.String() != "SYN-LE-01" ||
		got.Scope.String() != "SYN-SCOPE-01" || got.Level.String() != "CUSTOMER_SELF_SERVICE" ||
		got.Reason.String() != "SYN-REASON-1" || !got.At.Equal(now) ||
		got.Evidence.String() != "first-party-amendment:SYN-TENANT-01/SYN-ACCOUNT-01/SYN-SOURCE-A/SYN-KEY-1#DELIVERY_PLACE" {
		t.Fatalf("坐标 = %+v", got)
	}

	absent, err := adapter.NewCustomerSelfAmendmentRequestSource(keyStoreDouble{}, fixedClock{at: now})
	if err != nil {
		t.Fatal(err)
	}
	if _, formed, err := absent.FormRequestCoordinates(t.Context(), query); err != nil || formed {
		t.Fatalf("没登记解析键：formed=%v err=%v, want 折不出来", formed, err)
	}
}

func mustID[T any](t *testing.T, build func(string) (T, error), value string) T {
	t.Helper()
	built, err := build(value)
	if err != nil {
		t.Fatal(err)
	}
	return built
}
