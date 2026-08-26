package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// materialReceiptRegistryDouble 记到达的入参、按配置交回结果——用例的职责是挡不完整
// 登记与翻译写入口代数，写入口本体另有真库测试。
type materialReceiptRegistryDouble struct {
	outcome        ports.MaterialReceiptWriteOutcome
	err            error
	registerCalls  int
	revokeCalls    int
	lastTenant     domain.TenantID
	lastReceipt    ports.MaterialReceipt
	lastRevocation ports.MaterialReceiptRevocation
}

func (double *materialReceiptRegistryDouble) RegisterReceipt(
	_ context.Context, tenant domain.TenantID, receipt ports.MaterialReceipt,
) (ports.MaterialReceiptWriteOutcome, error) {
	double.registerCalls++
	double.lastTenant = tenant
	double.lastReceipt = receipt
	return double.outcome, double.err
}

func (double *materialReceiptRegistryDouble) RevokeReceipt(
	_ context.Context, tenant domain.TenantID, revocation ports.MaterialReceiptRevocation,
) (ports.MaterialReceiptWriteOutcome, error) {
	double.revokeCalls++
	double.lastTenant = tenant
	double.lastRevocation = revocation
	return double.outcome, double.err
}

var _ ports.MaterialReceiptRegistry = (*materialReceiptRegistryDouble)(nil)

var (
	receiptReceivedAt = time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	receiptRevokedAt  = time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
)

func validReceiptCommand(t *testing.T) application.RegisterMaterialReceiptCommand {
	t.Helper()
	return application.RegisterMaterialReceiptCommand{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-a"),
		Batch:      mustValue(t, domain.NewClaimBatchReference, "batch-1"),
		Item:       mustValue(t, domain.NewClaimItemID, "item-1"),
		Material:   mustValue(t, domain.NewMaterialRequirementReference, "MAT-PHOTO"),
		ReceivedAt: receiptReceivedAt,
		ReceivedBy: "operator-a",
	}
}

func validRevokeCommand(t *testing.T) application.RevokeMaterialReceiptCommand {
	t.Helper()
	return application.RevokeMaterialReceiptCommand{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-a"),
		Batch:      mustValue(t, domain.NewClaimBatchReference, "batch-1"),
		Item:       mustValue(t, domain.NewClaimItemID, "item-1"),
		Material:   mustValue(t, domain.NewMaterialRequirementReference, "MAT-PHOTO"),
		ReceivedAt: receiptReceivedAt,
		RevokedBy:  "supervisor-a",
		RevokedAt:  receiptRevokedAt,
	}
}

func newReceiptService(t *testing.T, registry ports.MaterialReceiptRegistry) *application.MaterialReceiptRegistration {
	t.Helper()
	service, err := application.NewMaterialReceiptRegistration(registry)
	if err != nil {
		t.Fatalf("构造收讫登记用例：%v", err)
	}
	return service
}

// Covers: 两命令的字段逐一到达写入口（尤其收讫时刻——它是行身份的一件，用例不许拿
// 时钟代填），写入口的每一格译成对的用例答案：Recorded/AlreadyRecorded 是登记两格，
// RevocationRecorded/RevocationAlreadyRecorded 是撤销两格，Unknown 译成
// RECEIPT_NOT_FOUND 的拒绝——治理答案，恢复动作是人工核对引用。
func TestReceiptCommandsReachTheRegistryAndOutcomesTranslate(t *testing.T) {
	registry := &materialReceiptRegistryDouble{outcome: ports.MaterialReceiptRecorded}
	service := newReceiptService(t, registry)

	command := validReceiptCommand(t)
	result, err := service.RegisterReceipt(t.Context(), command)
	if err != nil || result.Outcome() != application.MaterialReceiptRegistered {
		t.Fatalf("登记：err=%v outcome=%v", err, result.Outcome())
	}
	if registry.lastTenant != command.TenantID ||
		registry.lastReceipt.Batch != command.Batch ||
		registry.lastReceipt.Item != command.Item ||
		registry.lastReceipt.Material != command.Material ||
		!registry.lastReceipt.ReceivedAt.Equal(command.ReceivedAt) ||
		registry.lastReceipt.ReceivedBy != command.ReceivedBy {
		t.Fatalf("到达写入口的收讫 = %+v（租户 %v）", registry.lastReceipt, registry.lastTenant)
	}

	registry.outcome = ports.MaterialReceiptAlreadyRecorded
	if result, err := service.RegisterReceipt(t.Context(), command); err != nil ||
		result.Outcome() != application.MaterialReceiptReplayed {
		t.Fatalf("重放：err=%v outcome=%v，要 ALREADY_REGISTERED", err, result.Outcome())
	}

	revoke := validRevokeCommand(t)
	registry.outcome = ports.MaterialReceiptRevocationRecorded
	if result, err := service.RevokeReceipt(t.Context(), revoke); err != nil ||
		result.Outcome() != application.MaterialReceiptRevoked {
		t.Fatalf("撤销：err=%v outcome=%v", err, result.Outcome())
	}
	if registry.lastRevocation.RevokedBy != revoke.RevokedBy ||
		!registry.lastRevocation.RevokedAt.Equal(revoke.RevokedAt) ||
		!registry.lastRevocation.ReceivedAt.Equal(revoke.ReceivedAt) {
		t.Fatalf("到达写入口的撤销 = %+v", registry.lastRevocation)
	}

	registry.outcome = ports.MaterialReceiptRevocationAlreadyRecorded
	if result, err := service.RevokeReceipt(t.Context(), revoke); err != nil ||
		result.Outcome() != application.MaterialReceiptRevocationReplayed {
		t.Fatalf("撤销重放：err=%v outcome=%v，要 ALREADY_REVOKED", err, result.Outcome())
	}

	registry.outcome = ports.MaterialReceiptUnknown
	result, err = service.RevokeReceipt(t.Context(), revoke)
	if err != nil || result.Outcome() != application.MaterialReceiptRefused ||
		result.RefusalReason() != application.ReceiptNotFound {
		t.Fatalf("无从撤销：err=%v outcome=%v reason=%v", err, result.Outcome(), result.RefusalReason())
	}
}

// Covers: 缺件逐格拒且写入口不被碰——一个默认值都不补：缺收讫时刻不拿时钟凑
// （TIME_MISSING），缺经手声明不拿进程属主顶（RESPONSIBLE_MISSING，那是通道身份、
// 另一轨），撤销早于收讫在用例就拒（库上 CHECK 是第二道网，撞上会把整笔事务打进
// 中止态）。
func TestReceiptRefusalsNameTheMissingPieceWithoutTouchingTheRegistry(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(*application.RegisterMaterialReceiptCommand, *application.RevokeMaterialReceiptCommand)
		want   application.MaterialReceiptRefusalReason
	}{
		{"缺租户", func(register *application.RegisterMaterialReceiptCommand, revoke *application.RevokeMaterialReceiptCommand) {
			register.TenantID = domain.TenantID{}
			revoke.TenantID = domain.TenantID{}
		}, application.ReceiptScopeMissing},
		{"缺批次", func(register *application.RegisterMaterialReceiptCommand, revoke *application.RevokeMaterialReceiptCommand) {
			register.Batch = domain.ClaimBatchReference{}
			revoke.Batch = domain.ClaimBatchReference{}
		}, application.ReceiptScopeMissing},
		{"缺索赔项", func(register *application.RegisterMaterialReceiptCommand, revoke *application.RevokeMaterialReceiptCommand) {
			register.Item = domain.ClaimItemID{}
			revoke.Item = domain.ClaimItemID{}
		}, application.ReceiptScopeMissing},
		{"缺材料引用", func(register *application.RegisterMaterialReceiptCommand, revoke *application.RevokeMaterialReceiptCommand) {
			register.Material = domain.MaterialRequirementReference{}
			revoke.Material = domain.MaterialRequirementReference{}
		}, application.ReceiptMaterialMissing},
		{"缺收讫时刻", func(register *application.RegisterMaterialReceiptCommand, revoke *application.RevokeMaterialReceiptCommand) {
			register.ReceivedAt = time.Time{}
			revoke.ReceivedAt = time.Time{}
		}, application.ReceiptTimeMissing},
		{"缺经手声明", func(register *application.RegisterMaterialReceiptCommand, revoke *application.RevokeMaterialReceiptCommand) {
			register.ReceivedBy = "  "
			revoke.RevokedBy = "  "
		}, application.ReceiptResponsibleMissing},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			registry := &materialReceiptRegistryDouble{outcome: ports.MaterialReceiptRecorded}
			service := newReceiptService(t, registry)
			register := validReceiptCommand(t)
			revoke := validRevokeCommand(t)
			testCase.mutate(&register, &revoke)

			result, err := service.RegisterReceipt(t.Context(), register)
			if err != nil || result.Outcome() != application.MaterialReceiptRefused ||
				result.RefusalReason() != testCase.want {
				t.Fatalf("登记：err=%v outcome=%v reason=%v，要 %v",
					err, result.Outcome(), result.RefusalReason(), testCase.want)
			}
			result, err = service.RevokeReceipt(t.Context(), revoke)
			if err != nil || result.Outcome() != application.MaterialReceiptRefused ||
				result.RefusalReason() != testCase.want {
				t.Fatalf("撤销：err=%v outcome=%v reason=%v，要 %v",
					err, result.Outcome(), result.RefusalReason(), testCase.want)
			}
			if registry.registerCalls != 0 || registry.revokeCalls != 0 {
				t.Fatalf("缺件拒绝碰了写入口：register=%d revoke=%d",
					registry.registerCalls, registry.revokeCalls)
			}
		})
	}

	t.Run("撤销独有两格", func(t *testing.T) {
		registry := &materialReceiptRegistryDouble{outcome: ports.MaterialReceiptRevocationRecorded}
		service := newReceiptService(t, registry)

		missingRevokedAt := validRevokeCommand(t)
		missingRevokedAt.RevokedAt = time.Time{}
		if result, err := service.RevokeReceipt(t.Context(), missingRevokedAt); err != nil ||
			result.RefusalReason() != application.ReceiptTimeMissing {
			t.Fatalf("缺撤销时刻：err=%v reason=%v", err, result.RefusalReason())
		}

		beforeReceipt := validRevokeCommand(t)
		beforeReceipt.RevokedAt = beforeReceipt.ReceivedAt.Add(-time.Hour)
		if result, err := service.RevokeReceipt(t.Context(), beforeReceipt); err != nil ||
			result.RefusalReason() != application.ReceiptRevocationBeforeReceipt {
			t.Fatalf("撤销早于收讫：err=%v reason=%v", err, result.RefusalReason())
		}
		if registry.revokeCalls != 0 {
			t.Fatalf("时序拒绝碰了写入口：%d", registry.revokeCalls)
		}
	})
}

// Covers: 依赖故障不折成任何一格——登记与否未知，交回错误让入口翻成未决续办。
func TestReceiptRegistryFailureSurfacesAsError(t *testing.T) {
	registry := &materialReceiptRegistryDouble{err: errors.New("registry down")}
	service := newReceiptService(t, registry)

	if _, err := service.RegisterReceipt(t.Context(), validReceiptCommand(t)); err == nil {
		t.Fatal("登记的依赖故障没有交回错误")
	}
	if _, err := service.RevokeReceipt(t.Context(), validRevokeCommand(t)); err == nil {
		t.Fatal("撤销的依赖故障没有交回错误")
	}
}
