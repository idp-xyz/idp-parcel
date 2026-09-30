package registrationjson

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

type amountGrammarDocument struct {
	TenantID         string `json:"tenantId"`
	SubjectKind      string `json:"subjectKind"`
	SubjectRef       string `json:"subjectRef"`
	LimitMinor       int64  `json:"limitMinor"`
	RatioBasisPoints int64  `json:"ratioBasisPoints"`
	DeductibleMinor  int64  `json:"deductibleMinor"`
}

// AmountGrammarFromJSON 译装限额、比例、免赔三项取值。不把主张收成金额。
func AmountGrammarFromJSON(raw []byte) (domain.TenantID, domain.AmountGrammarRegistration, error) {
	var document amountGrammarDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.AmountGrammarRegistration{}, fmt.Errorf("金额文法登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.AmountGrammarRegistration{}, fmt.Errorf("tenantId：%w", err)
	}
	subject, err := domain.AmountGrammarSubjectFromName(document.SubjectKind)
	if err != nil {
		return domain.TenantID{}, domain.AmountGrammarRegistration{}, fmt.Errorf("subjectKind：%w", err)
	}
	grammar, err := domain.NewAmountGrammar(document.LimitMinor, document.RatioBasisPoints, document.DeductibleMinor)
	if err != nil {
		return domain.TenantID{}, domain.AmountGrammarRegistration{}, err
	}
	registration, err := domain.NewAmountGrammarRegistration(subject, document.SubjectRef, grammar)
	if err != nil {
		return domain.TenantID{}, domain.AmountGrammarRegistration{}, err
	}
	return tenant, registration, nil
}
