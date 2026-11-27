package accessidentity_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

func TestADataVisibilityGrantCannotBeBuiltWithoutItsScope(t *testing.T) {
	subject := operatorSubject(t)
	interval, err := accessidentity.NewEffectiveInterval(mintAt.Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := accessidentity.NewOperatorGrant(operatorTenant, "SYN-GRANT-VIS", subject, accessidentity.CapabilityDataVisibility, interval, "SYN-BASIS"); err == nil {
		t.Fatal("不带范围的数据可见范围不该经普通构造登进去")
	}
	if _, err := accessidentity.NewDataVisibility(accessidentity.DataVisibilityCustomerAccount, " "); err == nil {
		t.Fatal("空白引用不是全部")
	}
	if _, err := accessidentity.NewDataVisibility(accessidentity.DataVisibilityKind("SITE"), "IDP-WH-PVG"); err == nil {
		t.Fatal("封闭集之外的种类不该立得起来")
	}
}

func TestTheEnvelopeCarriesExplicitVisibilityAndDoesNotTreatItAsAFace(t *testing.T) {
	subject := operatorSubject(t)
	binding, err := accessidentity.NewOperatorBinding(subject, operatorTenant, "SYN-BASIS-BINDING")
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(mintAt.Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	read, err := accessidentity.NewOperatorGrant(operatorTenant, "SYN-GRANT-READ", subject, accessidentity.CapabilityMasterDataAndOperationsRead, interval, "SYN-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	allCustomers, err := accessidentity.NewDataVisibility(accessidentity.DataVisibilityCustomerAccount, accessidentity.DataVisibilityAll)
	if err != nil {
		t.Fatal(err)
	}
	namedCustomer, err := accessidentity.NewDataVisibility(accessidentity.DataVisibilityCustomerAccount, "IDP-ACCOUNT-QINGHE")
	if err != nil {
		t.Fatal(err)
	}
	warehouse, err := accessidentity.NewDataVisibility(accessidentity.DataVisibilityWarehouse, "IDP-WH-PVG")
	if err != nil {
		t.Fatal(err)
	}
	grants := make([]accessidentity.RecordedGrant, 0, 4)
	for index, built := range []accessidentity.OperatorGrant{
		read,
		mustVisibilityGrant(t, subject, interval, "SYN-GRANT-ALL-CUSTOMERS", allCustomers),
		mustVisibilityGrant(t, subject, interval, "SYN-GRANT-ONE-CUSTOMER", namedCustomer),
		mustVisibilityGrant(t, subject, interval, "SYN-GRANT-ONE-WAREHOUSE", warehouse),
	} {
		recorded, err := accessidentity.NewRecordedGrant(built, nil)
		if err != nil {
			t.Fatal(index, err)
		}
		grants = append(grants, recorded)
	}
	standing, err := accessidentity.NewOperatorStanding(binding, grants)
	if err != nil {
		t.Fatal(err)
	}
	minter := newMinter(t, verifierFake{subject: subject}, registryFake{standing: standing, found: true})
	envelope, err := mint(minter, operatorTenant, accessidentity.CapabilityMasterDataAndOperationsRead)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Holds(accessidentity.CapabilityDataVisibility) {
		t.Fatal("数据可见范围不是端点门，不该出现在持有集里")
	}
	all, ids := envelope.CustomerVisibility()
	if !all || ids != nil {
		t.Fatalf("客户可见范围 = all %v ids %v，同一类里有全部时按全部", all, ids)
	}
	allWarehouses, codes := envelope.WarehouseVisibility()
	if allWarehouses || len(codes) != 1 || codes[0] != "IDP-WH-PVG" {
		t.Fatalf("仓可见范围 = all %v codes %v", allWarehouses, codes)
	}
	codes[0] = "TAMPERED"
	_, again := envelope.WarehouseVisibility()
	if again[0] != "IDP-WH-PVG" {
		t.Fatal("读出的仓名单被改写回了信封")
	}
}

func mustVisibilityGrant(
	t *testing.T,
	subject accessidentity.OperatorSubject,
	interval accessidentity.EffectiveInterval,
	grantID string,
	visibility accessidentity.DataVisibility,
) accessidentity.OperatorGrant {
	t.Helper()
	grant, err := accessidentity.NewDataVisibilityGrant(operatorTenant, grantID, subject, visibility, interval, "SYN-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	return grant
}
