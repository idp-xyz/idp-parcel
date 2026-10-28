package registrationjson_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// Covers: AT-SA-220（ADR-0336 决定二）——确认条件登记只认产品内置的依据种类，未知字段与写错的种类当场拒。
func TestTheChargeConfirmationConditionDocumentOnlyNamesABuiltInBasisKind(t *testing.T) {
	tenant, feeItem, kind, err := registrationjson.ChargeConfirmationConditionFromJSON([]byte(
		`{"tenantId":"SYN-TENANT-01","feeItem":"SYN-FEE-ITEM-FREIGHT-01","basisKind":"EFFECTIVE_DELIVERY_FINALIZED"}`))
	if err != nil || tenant.String() != "SYN-TENANT-01" || feeItem.String() != "SYN-FEE-ITEM-FREIGHT-01" ||
		kind.String() != domain.EffectiveDeliveryFinalizedBasis().String() {
		t.Fatalf("译装 = %s / %s / %s err = %v", tenant, feeItem, kind, err)
	}
	if _, _, _, err := registrationjson.ChargeConfirmationConditionFromJSON([]byte(
		`{"tenantId":"SYN-TENANT-01","feeItem":"SYN-FEE-ITEM-FREIGHT-01","basisKind":"DELIVERED"}`)); !errors.Is(err, domain.ErrUnknownConfirmationBasisKind) {
		t.Fatalf("写错的种类 err = %v", err)
	}
	if _, _, _, err := registrationjson.ChargeConfirmationConditionFromJSON([]byte(
		`{"tenantId":"SYN-TENANT-01","feeItem":"SYN-FEE-ITEM-FREIGHT-01","basisKind":"EFFECTIVE_DELIVERY_FINALIZED","met":true}`)); err == nil {
		t.Fatal("未知字段没拒")
	}
}
