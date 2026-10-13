package main

// 本文件把准入范围对照册的两种登记批译成领域类型（票 operator-channel/19，ADR-0180 决定一、二），纪律同 translate.go：
// 未知字段、缺件、集合外的能力或事实类型与倒置区间在触库之前拒收，一格不代填。

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

type admissionScopeMappingBatchDocument struct {
	TenantID string                              `json:"tenantId"`
	Mappings []admissionScopeMappingItemDocument `json:"mappings"`
}

// admissionScopeMappingItemDocument 的能力与事实类型照认证方的词写（ADR-0180 决定四），不带 SYN- 前缀；带 SYN- 的是对象
// 范围——那是治理登记册里的实例标识。
type admissionScopeMappingItemDocument struct {
	MappingID         string     `json:"mappingId"`
	Capability        string     `json:"capability"`
	FactKind          string     `json:"factKind"`
	ObjectScope       string     `json:"objectScope"`
	EffectiveStartsAt time.Time  `json:"effectiveStartsAt"`
	EffectiveEndsAt   *time.Time `json:"effectiveEndsAt,omitempty"`
	Basis             string     `json:"basis"`
}

type admissionScopeMappingEndBatchDocument struct {
	TenantID string                                 `json:"tenantId"`
	Ends     []admissionScopeMappingEndItemDocument `json:"ends"`
}

type admissionScopeMappingEndItemDocument struct {
	MappingID string    `json:"mappingId"`
	EndedAt   time.Time `json:"endedAt"`
	Basis     string    `json:"basis"`
}

func admissionScopeMappingBatchFromJSON(raw []byte) ([]batchItem, error) {
	var document admissionScopeMappingBatchDocument
	if err := decodeStrict(raw, &document, "准入范围对照登记批"); err != nil {
		return nil, err
	}
	if err := requireBatch(document.TenantID, len(document.Mappings), "准入范围对照登记批"); err != nil {
		return nil, err
	}
	items := make([]batchItem, 0, len(document.Mappings))
	for _, entry := range document.Mappings {
		mapping, err := admissionScopeMappingFrom(document.TenantID, entry)
		if err != nil {
			return nil, fmt.Errorf("mappings/%s：%w", entry.MappingID, err)
		}
		requirement := mapping.Requirement()
		items = append(items, batchItem{
			label: fmt.Sprintf("对照 %s（%s · %s → %s）", mapping.MappingID(), requirement.Capability, requirement.FactKind, mapping.ObjectScope()),
			apply: func(ctx context.Context, books registers) (outcome, error) {
				answer, err := books.mappings.RegisterAdmissionScopeMapping(ctx, mapping)
				return mappingOutcome(answer), err
			},
		})
	}
	return items, nil
}

func admissionScopeMappingFrom(tenantID string, entry admissionScopeMappingItemDocument) (accessidentity.AdmissionScopeMapping, error) {
	interval, err := intervalFrom(entry.EffectiveStartsAt, entry.EffectiveEndsAt)
	if err != nil {
		return accessidentity.AdmissionScopeMapping{}, err
	}
	requirement := accessidentity.AdmissionRequirement{Capability: entry.Capability, FactKind: entry.FactKind}
	mapping, err := accessidentity.NewAdmissionScopeMapping(tenantID, entry.MappingID, requirement, entry.ObjectScope, interval, entry.Basis)
	if err != nil {
		return accessidentity.AdmissionScopeMapping{}, fmt.Errorf("capability %q、factKind %q：%w", entry.Capability, entry.FactKind, err)
	}
	return mapping, nil
}

func admissionScopeMappingEndBatchFromJSON(raw []byte) ([]batchItem, error) {
	var document admissionScopeMappingEndBatchDocument
	if err := decodeStrict(raw, &document, "准入范围对照结束批"); err != nil {
		return nil, err
	}
	if err := requireBatch(document.TenantID, len(document.Ends), "准入范围对照结束批"); err != nil {
		return nil, err
	}
	items := make([]batchItem, 0, len(document.Ends))
	for _, entry := range document.Ends {
		if entry.EndedAt.IsZero() {
			return nil, fmt.Errorf("ends/%s：endedAt 缺席：结束时刻不代填", entry.MappingID)
		}
		end, err := accessidentity.NewAdmissionScopeMappingEnd(document.TenantID, entry.MappingID, entry.EndedAt, entry.Basis)
		if err != nil {
			return nil, fmt.Errorf("ends/%s：%w", entry.MappingID, err)
		}
		items = append(items, batchItem{
			label: fmt.Sprintf("结束对照 %s", end.MappingID()),
			apply: func(ctx context.Context, books registers) (outcome, error) {
				answer, err := books.mappings.RegisterAdmissionScopeMappingEnd(ctx, end)
				return mappingOutcome(answer), err
			},
		})
	}
	return items, nil
}

// mappingOutcome 只认两格为落定，判据同 operatorOutcome：同键重叠与未登记都要人看。
func mappingOutcome(answer accessidentity.AdmissionScopeMappingOutcome) outcome {
	switch answer {
	case accessidentity.AdmissionScopeMappingRecorded, accessidentity.AdmissionScopeMappingAlreadyRegistered:
		return outcome{name: answer.String(), landed: true}
	}
	return outcome{name: answer.String()}
}
