package registrationjson

import (
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 核销与核销撤销的载荷。译法照资金映射：一格对命令一格，零默认、未知字段拒、封闭词表与构造门在这里就核并指名哪一格。
// 核销是显式判断不是自动推导（UC-SA-005）：映射清单与分配都由交进来的人写全，这里不从分配推映射清单、不并片、不排序，
// 也不按金额或到期日代拟分配——分配有没有映射背书、守不守恒、超没超出目标的未核销，都是领域与用例的门。

type fundsAllocationDocument struct {
	MappingRef  string `json:"mappingRef"`
	TargetKind  string `json:"targetKind"`
	TargetRef   string `json:"targetRef"`
	Direction   string `json:"direction"`
	AmountMinor int64  `json:"amountMinor"`
}

type fundsApplicationDocument struct {
	TenantID       string                    `json:"tenantId"`
	ApplicationRef string                    `json:"applicationRef"`
	FactRef        string                    `json:"factRef"`
	TargetCurrency string                    `json:"targetCurrency"`
	MappingRefs    []string                  `json:"mappingRefs"`
	Allocations    []fundsAllocationDocument `json:"allocations"`
	Basis          string                    `json:"basis"`
	AppliedAt      time.Time                 `json:"appliedAt"`
}

// FundsApplicationFromJSON 译装一次核销。
func FundsApplicationFromJSON(raw []byte) (application.ApplySettlementCommand, error) {
	none := application.ApplySettlementCommand{}
	var document fundsApplicationDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("核销登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, fmt.Errorf("tenantId：%w", err)
	}
	if _, err := domain.NewApplicationReference(document.ApplicationRef); err != nil {
		return none, fmt.Errorf("applicationRef：%w", err)
	}
	if _, err := domain.NewFundsFactReference(document.FactRef); err != nil {
		return none, fmt.Errorf("factRef：%w", err)
	}
	if _, err := domain.NewCurrencyCode(document.TargetCurrency); err != nil {
		return none, fmt.Errorf("targetCurrency：%w", err)
	}
	for index, mappingRef := range document.MappingRefs {
		if _, err := domain.NewMappingReference(mappingRef); err != nil {
			return none, fmt.Errorf("mappingRefs[%d]：%w", index, err)
		}
	}
	allocations := make([]application.AllocationDirective, 0, len(document.Allocations))
	for index, allocation := range document.Allocations {
		directive, err := allocationDirectiveFrom(allocation)
		if err != nil {
			return none, fmt.Errorf("allocations[%d].%w", index, err)
		}
		allocations = append(allocations, directive)
	}
	if _, err := domain.NewApplicationBasisReference(document.Basis); err != nil {
		return none, fmt.Errorf("basis：%w", err)
	}
	return application.ApplySettlementCommand{
		TenantID:       tenant,
		Application:    document.ApplicationRef,
		Fact:           document.FactRef,
		TargetCurrency: document.TargetCurrency,
		MappingRefs:    document.MappingRefs,
		Allocations:    allocations,
		Basis:          document.Basis,
		AppliedAt:      document.AppliedAt,
	}, nil
}

func allocationDirectiveFrom(document fundsAllocationDocument) (application.AllocationDirective, error) {
	none := application.AllocationDirective{}
	if _, err := domain.NewMappingReference(document.MappingRef); err != nil {
		return none, fmt.Errorf("mappingRef：%w", err)
	}
	kind, err := settlementTargetKindFrom(document.TargetKind)
	if err != nil {
		return none, err
	}
	if _, err := domain.NewSettlementTargetReference(document.TargetRef); err != nil {
		return none, fmt.Errorf("targetRef：%w", err)
	}
	direction, err := allocationDirectionFrom(document.Direction)
	if err != nil {
		return none, err
	}
	return application.AllocationDirective{
		Mapping:     document.MappingRef,
		TargetKind:  kind,
		Target:      document.TargetRef,
		Direction:   direction,
		AmountMinor: document.AmountMinor,
	}, nil
}

// allocationDirectionFrom 把封闭词表译回领域常量，词表与 `domain.AllocationDirection.String()` 同字。缺席不放行：分配没有
// 「方向未定」这一格，借贷不分就是没写清这笔钱是付掉还是加回。
func allocationDirectionFrom(raw string) (domain.AllocationDirection, error) {
	switch strings.TrimSpace(raw) {
	case "DEBIT":
		return domain.AllocationDebit, nil
	case "CREDIT":
		return domain.AllocationCredit, nil
	default:
		return domain.AllocationDirectionInvalid, fmt.Errorf(
			"direction=%q 不在封闭二值（DEBIT / CREDIT），缺席同样拒", raw)
	}
}

type fundsApplicationReversalDocument struct {
	TenantID       string    `json:"tenantId"`
	ApplicationRef string    `json:"applicationRef"`
	Basis          string    `json:"basis"`
	ReversedAt     time.Time `json:"reversedAt"`
}

// FundsApplicationReversalFromJSON 译装一次核销撤销：只点名哪一笔核销、凭什么撤、何时撤。撤销只登记核销关系失效，原分配
// 不删（AT-SA-112、AT-SA-113），载荷上没有分配这一格。
func FundsApplicationReversalFromJSON(raw []byte) (application.ReverseApplicationCommand, error) {
	none := application.ReverseApplicationCommand{}
	var document fundsApplicationReversalDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("核销撤销登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, fmt.Errorf("tenantId：%w", err)
	}
	if _, err := domain.NewApplicationReference(document.ApplicationRef); err != nil {
		return none, fmt.Errorf("applicationRef：%w", err)
	}
	if _, err := domain.NewApplicationBasisReference(document.Basis); err != nil {
		return none, fmt.Errorf("basis：%w", err)
	}
	return application.ReverseApplicationCommand{
		TenantID:    tenant,
		Application: document.ApplicationRef,
		Basis:       document.Basis,
		ReversedAt:  document.ReversedAt,
	}, nil
}
