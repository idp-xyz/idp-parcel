package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: ADR-0384 决定一——商品维护过得了客户能力 CHECK，不因此持有仓配查阅或委托提交。
func TestGoodsMaintenanceGrantPassesTheCapabilityCheck(t *testing.T) {
	register := newCustomerRegister(t)
	subject := customerSubjectOf(t, "https://auth.syn.example/customers", "SYN-CUSTOMER-CLIENT-GOODS-01")
	expectCustomerOutcome(t, "绑定", register.bind(t, customerBindingOf(t, subject, defaultCustomerBinding("SYN-TENANT-01"))),
		accessidentity.CustomerClientRegistrationRecorded)

	startsAt := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	interval, err := accessidentity.NewEffectiveInterval(startsAt, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := accessidentity.NewCustomerClientGrant("SYN-TENANT-01", "SYN-CUSTOMER-GRANT-GOODS-MAINTENANCE", subject,
		accessidentity.CustomerCapabilityGoodsMaintenance, interval, "SYN-CUSTOMER-GRANT-BASIS-GOODS")
	if err != nil {
		t.Fatal(err)
	}
	expectCustomerOutcome(t, "商品维护", register.grant(t, grant), accessidentity.CustomerClientRegistrationRecorded)

	standing := register.standing(t, subject)
	if !standing.HoldsAt(accessidentity.CustomerCapabilityGoodsMaintenance, startsAt) {
		t.Fatal("商品维护授予没按字面量读回")
	}
	if standing.HoldsAt(accessidentity.CustomerCapabilityWarehouseRead, startsAt) ||
		standing.HoldsAt(accessidentity.CustomerCapabilityShipmentRequestSubmission, startsAt) {
		t.Fatal("这个主体不该同时持有仓配查阅或委托提交")
	}
}
