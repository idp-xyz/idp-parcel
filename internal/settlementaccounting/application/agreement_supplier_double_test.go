package application_test

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// agreementSupplierDouble 替 party-commercial 答「这一版协议的供应商是谁」（ADR-0358 决定一）。没登记的协议答它的缺省供应商，
// 好让不关心供应商的用例不必逐个登；要证查无、读不动或换供应商的用例改 suppliers / unknown / err。
type agreementSupplierDouble struct {
	fallback  string
	suppliers map[string]string
	unknown   map[string]bool
	err       error
	asked     []string
}

func newAgreementSupplierDouble() *agreementSupplierDouble {
	return &agreementSupplierDouble{fallback: "partner-1", suppliers: map[string]string{}, unknown: map[string]bool{}}
}

func (double *agreementSupplierDouble) LoadSupplierAgreementSupplier(
	_ context.Context, _ domain.TenantID, agreement domain.SupplierAgreementReference,
) (domain.SupplierPartyReference, bool, error) {
	double.asked = append(double.asked, agreement.String())
	if double.err != nil {
		return domain.SupplierPartyReference{}, false, double.err
	}
	if double.unknown[agreement.String()] {
		return domain.SupplierPartyReference{}, false, nil
	}
	value := double.fallback
	if named, ok := double.suppliers[agreement.String()]; ok {
		value = named
	}
	supplier, err := domain.NewSupplierPartyReference(value)
	return supplier, err == nil, err
}
