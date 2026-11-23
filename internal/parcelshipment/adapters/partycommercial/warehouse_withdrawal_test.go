package partycommercial

import (
	"context"
	"testing"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

type agencyReferenceDouble struct {
	outcome   psports.AuthorizationOutcome
	reference string
	called    int
}

func (double *agencyReferenceDouble) Reference(context.Context, string, string, string, string) (string, psports.AuthorizationOutcome, error) {
	double.called++
	return double.reference, double.outcome, nil
}

func warehouseWithdrawalQuery(t *testing.T, requester, source string) psports.WithdrawalAuthorizationQuery {
	t.Helper()
	tenant, err := psdomain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := psdomain.NewCustomerAccountID("SYN-OWNER-01")
	if err != nil {
		t.Fatal(err)
	}
	origin, err := psdomain.NewSource(source)
	if err != nil {
		t.Fatal(err)
	}
	key, err := psdomain.NewSourceRequestKey("OBR-1/1")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := psdomain.NewSourceIdentity(tenant, owner, origin, key)
	if err != nil {
		t.Fatal(err)
	}
	who, err := psdomain.NewWithdrawalRequesterReference(requester)
	if err != nil {
		t.Fatal(err)
	}
	shipment, err := psdomain.NewShipmentRequestID("SHR-1")
	if err != nil {
		t.Fatal(err)
	}
	return psports.WithdrawalAuthorizationQuery{Identity: identity, ShipmentRequestID: shipment, Requester: who}
}

func TestWarehouseWithdrawalOnlyGrantsTheAgencyRepresentative(t *testing.T) {
	agency := &agencyReferenceDouble{outcome: psports.AuthorizationGranted, reference: "grant/1"}
	authorizer := &WarehouseWithdrawalAuthorizer{agency: agency}
	granted, err := authorizer.AuthorizeWithdrawal(t.Context(), warehouseWithdrawalQuery(t, psdomain.WarehouseFulfillmentRequester, psdomain.SourceWarehouseOutbound))
	if err != nil || granted.Outcome != psports.AuthorizationGranted || granted.Authority.String() != "grant/1" || agency.called != 1 {
		t.Fatalf("granted %v %s err %v called %d", granted.Outcome, granted.Authority.String(), err, agency.called)
	}
	if _, err := authorizer.AuthorizeWithdrawal(t.Context(), warehouseWithdrawalQuery(t, "CUSTOMER-CONTACT-1", psdomain.SourceWarehouseOutbound)); err == nil || agency.called != 1 {
		t.Fatal("别的请求方被当成仓配代办")
	}
	if _, err := authorizer.AuthorizeWithdrawal(t.Context(), warehouseWithdrawalQuery(t, psdomain.WarehouseFulfillmentRequester, "CUSTOMER_CHANNEL")); err == nil || agency.called != 1 {
		t.Fatal("别的来源被当成仓配出库")
	}
	agency.outcome = psports.AuthorizationRulesNotConfigured
	unconfigured, err := authorizer.AuthorizeWithdrawal(t.Context(), warehouseWithdrawalQuery(t, psdomain.WarehouseFulfillmentRequester, psdomain.SourceWarehouseOutbound))
	if err != nil || unconfigured.Outcome != psports.AuthorizationRulesNotConfigured {
		t.Fatalf("未配置 %v err %v", unconfigured.Outcome, err)
	}
}
