package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: ADR-0384 决定一——收货差异确认过得了客户能力 CHECK，不因此持有仓配查阅或委托提交。
func TestReceiptDiscrepancyConclusionGrantPassesTheCapabilityCheck(t *testing.T) {
	register := newCustomerRegister(t)
	subject := customerSubjectOf(t, "https://auth.syn.example/customers", "SYN-CUSTOMER-CLIENT-RDC-01")
	expectCustomerOutcome(t, "绑定", register.bind(t, customerBindingOf(t, subject, defaultCustomerBinding("SYN-TENANT-01"))),
		accessidentity.CustomerClientRegistrationRecorded)

	startsAt := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	interval, err := accessidentity.NewEffectiveInterval(startsAt, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := accessidentity.NewCustomerClientGrant("SYN-TENANT-01", "SYN-CUSTOMER-GRANT-RECEIPT-DISCREPANCY-CONCLUSION", subject,
		accessidentity.CustomerCapabilityReceiptDiscrepancyConclusion, interval, "SYN-CUSTOMER-GRANT-BASIS-RDC")
	if err != nil {
		t.Fatal(err)
	}
	expectCustomerOutcome(t, "收货差异确认", register.grant(t, grant), accessidentity.CustomerClientRegistrationRecorded)

	standing := register.standing(t, subject)
	if !standing.HoldsAt(accessidentity.CustomerCapabilityReceiptDiscrepancyConclusion, startsAt) {
		t.Fatal("收货差异确认授予没按字面量读回")
	}
	if standing.HoldsAt(accessidentity.CustomerCapabilityWarehouseRead, startsAt) ||
		standing.HoldsAt(accessidentity.CustomerCapabilityShipmentRequestSubmission, startsAt) {
		t.Fatal("这个主体不该同时持有仓配查阅或委托提交")
	}
}

// Covers: ADR-0384 决定二——收货差异结论代录过得了决定种类 CHECK，不因此持有入库预报完结。
func TestReceiptDiscrepancyConclusionOnBehalfGrantPassesTheDecisionKindCheck(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-DISCREPANCY-CONCLUSION")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-RECEIPT-DISCREPANCY-CONCLUSION", subject,
		accessidentity.DecisionReceiptDiscrepancyRecordOnBehalf, interval, "SYN-GRANT-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	expectOutcome(t, "receipt discrepancy conclusion on behalf grant", register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	at := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	standing := register.standing(t, subject)
	if !standing.HoldsDecisionAt(accessidentity.DecisionReceiptDiscrepancyRecordOnBehalf, at) ||
		standing.HoldsDecisionAt(accessidentity.DecisionInboundPreAdviceClose, at) {
		t.Fatalf("收货差异结论代录授予没有单独留下：grants = %+v", standing.Grants())
	}
}
