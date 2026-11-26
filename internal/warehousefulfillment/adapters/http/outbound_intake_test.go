package warehousehttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// ownerOutboundAuthenticator 认得任何凭据：只为把货主口读到的载荷走完到受理命令。
type ownerOutboundAuthenticator struct {
	identity warehousehttp.CustomerCommandIdentity
}

func (authenticator ownerOutboundAuthenticator) answer(accessRequestID func() (string, error)) (warehousehttp.CustomerCommandIdentity, error) {
	if _, err := accessRequestID(); err != nil {
		return warehousehttp.CustomerCommandIdentity{}, err
	}
	return authenticator.identity, nil
}

func (authenticator ownerOutboundAuthenticator) AuthenticateOutboundRequestSubmission(_ context.Context, _ warehousehttp.PresentedCustomerCredential, accessRequestID func() (string, error)) (warehousehttp.CustomerCommandIdentity, error) {
	return authenticator.answer(accessRequestID)
}

func (authenticator ownerOutboundAuthenticator) AuthenticateOutboundRequestSplit(_ context.Context, _ warehousehttp.PresentedCustomerCredential, accessRequestID func() (string, error)) (warehousehttp.CustomerCommandIdentity, error) {
	return authenticator.answer(accessRequestID)
}

func (authenticator ownerOutboundAuthenticator) AuthenticateOutboundRequestSupplement(_ context.Context, _ warehousehttp.PresentedCustomerCredential, accessRequestID func() (string, error)) (warehousehttp.CustomerCommandIdentity, error) {
	return authenticator.answer(accessRequestID)
}

func (authenticator ownerOutboundAuthenticator) AuthenticateOutboundCancellation(_ context.Context, _ warehousehttp.PresentedCustomerCredential, accessRequestID func() (string, error)) (warehousehttp.CustomerCommandIdentity, error) {
	return authenticator.answer(accessRequestID)
}

// Covers: 货主提交出库请求时写的部分发货规则要走到受理命令里。读载荷时认了这一格、组命令时丢掉，
// 货主对这一张的交代就无声地没了，这张单会按货主的默认办。没写的仍是没写明。
func TestTheOwnerOutboundIntakeCarriesThePartialShipmentIntoTheCommand(t *testing.T) {
	tenant, _ := domain.NewTenantID("SYN-TENANT-01")
	owner, _ := domain.NewOwnerAccountID("SYN-OWNER-01")
	key, _ := domain.NewRequestKey("SYN-KEY-01")
	intake, err := warehousehttp.NewCustomerOutboundIntake(ownerOutboundAuthenticator{
		identity: warehousehttp.CustomerCommandIdentity{Tenant: tenant, Owner: owner, RequestKey: key},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"accessRequestId":"SYN-ACCESS-01","warehouse":"SYN-WH-01","dispatch":"PICKUP","recipient":"合成收件人","address":"合成地址",` +
		`"lines":[{"goodsCode":"SYN-GOODS-01","unit":"件","stockState":"GOOD","quantity":2}]`
	for name, testCase := range map[string]struct {
		tail string
		want domain.PartialShipment
	}{
		"写了整单发": {`,"partialShipment":"SHIP_COMPLETE"}`, domain.PartialShipmentShipComplete},
		"写了拆单发": {`,"partialShipment":"SPLIT_SHIP"}`, domain.PartialShipmentSplitShip},
		"没写":    {`}`, domain.PartialShipmentUnset},
	} {
		request := httptest.NewRequest(http.MethodPost, "/v1/warehouse/outbound-requests", strings.NewReader(body+testCase.tail))
		command, err := intake.IntakeOutboundRequest(request.Context(), request)
		if err != nil {
			t.Fatalf("%s：%v", name, err)
		}
		if command.PartialShipment != testCase.want || command.Owner != owner || command.Key != key || command.Origin != domain.OutboundOriginCustomer {
			t.Errorf("%s：命令里的规则 %q，货主 %s，键 %s", name, command.PartialShipment, command.Owner, command.Key)
		}
	}
}
