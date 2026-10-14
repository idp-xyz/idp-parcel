package registrationjson

import (
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

type fundsMappingDocument struct {
	TenantID   string    `json:"tenantId"`
	MappingRef string    `json:"mappingRef"`
	FactRef    string    `json:"factRef"`
	TargetKind string    `json:"targetKind"`
	TargetRef  string    `json:"targetRef"`
	Basis      string    `json:"basis"`
	MappedAt   time.Time `json:"mappedAt"`
}

// FundsMappingFromJSON 译装一次资金映射。目标种类是封闭四值，缺席与词表外都拒。
func FundsMappingFromJSON(raw []byte) (application.MapFundsCommand, error) {
	none := application.MapFundsCommand{}
	var document fundsMappingDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("资金映射登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, fmt.Errorf("tenantId：%w", err)
	}
	if _, err := domain.NewMappingReference(document.MappingRef); err != nil {
		return none, fmt.Errorf("mappingRef：%w", err)
	}
	if _, err := domain.NewFundsFactReference(document.FactRef); err != nil {
		return none, fmt.Errorf("factRef：%w", err)
	}
	kind, err := settlementTargetKindFrom(document.TargetKind)
	if err != nil {
		return none, err
	}
	if _, err := domain.NewSettlementTargetReference(document.TargetRef); err != nil {
		return none, fmt.Errorf("targetRef：%w", err)
	}
	if _, err := domain.NewMappingBasisReference(document.Basis); err != nil {
		return none, fmt.Errorf("basis：%w", err)
	}
	return application.MapFundsCommand{
		TenantID:   tenant,
		Mapping:    document.MappingRef,
		Fact:       document.FactRef,
		TargetKind: kind,
		Target:     document.TargetRef,
		Basis:      document.Basis,
		MappedAt:   document.MappedAt,
	}, nil
}

func settlementTargetKindFrom(raw string) (domain.SettlementTargetKind, error) {
	switch strings.TrimSpace(raw) {
	case "STATEMENT":
		return domain.TargetStatement, nil
	case "PAYABLE":
		return domain.TargetPayable, nil
	case "CREDIT_NOTE":
		return domain.TargetCreditNote, nil
	case "ACCOUNT":
		return domain.TargetAccount, nil
	default:
		return domain.SettlementTargetKindInvalid, fmt.Errorf(
			"targetKind=%q 不在封闭四值（STATEMENT / PAYABLE / CREDIT_NOTE / ACCOUNT），缺席同样拒", raw)
	}
}
