package settlementaccounting_test

import (
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/settlementaccounting"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件证 SA 评价请求记录 → PP「按评价请求形成评价」命令的翻译（票 sa-cc/11 做法 1「取主要范围、计算目的、合格
// 来源引用三件」+ 裁决 1 让入口收的那几样）。只译不判：一个数字都不出现，SA 的内容一格都不复制进评价——命令里的
// 来源引用是钥匙，评价上留的只有回指。

var occurredAt = time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

func mustSA[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

// synRecord 是一份 SYN- 合成的 SA 评价请求记录：BUY_SUPPLIER_COST，发生项 SYN-OCC-1@v1 业务时点 occurredAt。
func synRecord(t *testing.T) saports.EvaluationRequestRecord {
	t.Helper()
	occurrence, err := sadomain.NewTransportChargeOccurrence(
		mustSA(t, sadomain.NewChargeOccurrenceID, "SYN-OCC-1"),
		mustSA(t, sadomain.NewOccurrenceReasonReference, "SYN-REASON-BOOKING"),
		mustSA(t, sadomain.NewOccurrenceVersion, "v1"),
		occurredAt,
	)
	if err != nil {
		t.Fatalf("occurrence: %v", err)
	}
	request, err := sadomain.SubmitEvaluationRequest(sadomain.EvaluationRequestSpec{
		ID:      mustSA(t, sadomain.NewEvaluationRequestID, "EVREQ-SYN-1"),
		Scope:   mustSA(t, sadomain.NewPrimaryScopeReference, "scope-1"),
		Purpose: sadomain.BuySupplierCost,
		Sources: sadomain.EligibleSourceReferences{
			Occurrence: occurrence,
			FeeItem:    mustSA(t, sadomain.NewFeeItemReference, "SYN-FEE-1"),
			Agreement:  mustSA(t, sadomain.NewSupplierAgreementReference, "SYN-AGR-1@v1"),
		},
		RequestedAt: occurredAt.Add(time.Hour),
		RequestedBy: mustSA(t, sadomain.NewRequesterReference, "SYN-SETTLEMENT-JOB"),
	})
	if err != nil {
		t.Fatalf("submit evaluation request: %v", err)
	}
	return saports.EvaluationRequestRecord{
		Key: saports.EvaluationRequestKey{
			TenantID: mustSA(t, sadomain.NewTenantID, "tenant-1"),
			Request:  request.ID(),
		},
		Request:    request,
		RecordedAt: occurredAt.Add(2 * time.Hour),
	}
}

// Covers: 命令逐格译——租户 / 回指取键、范围按字面、BUY_SUPPLIER_COST 译成 BUY + SUPPLIER_COST 一对、计价基准时点取
// 发生项业务时间（不是 RequestedAt 也不是 RecordedAt）、三件来源引用按字面带钥匙、证据层级照装配方声明。
func TestAnEvaluationRequestRecordTranslatesIntoAFormationCommand(t *testing.T) {
	command, err := adapter.TranslateEvaluationRequest(synRecord(t), ppdomain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if command.Tenant.String() != "tenant-1" || command.Request.String() != "EVREQ-SYN-1" || command.Scope.String() != "scope-1" {
		t.Fatalf("keys = tenant %q request %q scope %q", command.Tenant, command.Request, command.Scope)
	}
	if command.Direction != ppdomain.PricingDirectionBuy || command.Purpose != ppdomain.PricingPurposeSupplierCost {
		t.Fatalf("direction / purpose = %s / %s, want BUY / SUPPLIER_COST", command.Direction, command.Purpose)
	}
	if !command.BasisAt.Equal(occurredAt) {
		t.Fatalf("basis at = %v, want the occurrence business time %v", command.BasisAt, occurredAt)
	}
	if command.Sources.Occurrence != "SYN-OCC-1" || command.Sources.OccurrenceVersion != "v1" ||
		command.Sources.FeeItem != "SYN-FEE-1" || command.Sources.SupplierAgreement != "SYN-AGR-1@v1" {
		t.Fatalf("sources = %+v", command.Sources)
	}
	if command.Evidence != ppdomain.EvidenceSynthetic {
		t.Fatalf("evidence = %q, want the declared S", command.Evidence)
	}
}

// Covers: 证据层级不给默认——装配方没声明合法的一格，翻译拒，不悄悄记成 S 或 P。
func TestTranslationRefusesAnUndeclaredEvidenceKind(t *testing.T) {
	for name, evidence := range map[string]ppdomain.EvidenceKind{"空": "", "词表外": "X"} {
		t.Run(name, func(t *testing.T) {
			if _, err := adapter.TranslateEvaluationRequest(synRecord(t), evidence); err == nil {
				t.Fatal("undeclared evidence kind was accepted")
			}
		})
	}
}

// Covers: 零值记录（读口交回了空对象）译不出，报 ErrUntranslatableAnswer 而不是 panic 或空命令。
func TestTranslationRefusesAZeroRecord(t *testing.T) {
	_, err := adapter.TranslateEvaluationRequest(saports.EvaluationRequestRecord{}, ppdomain.EvidenceSynthetic)
	if err == nil {
		t.Fatal("zero record translated")
	}
}
