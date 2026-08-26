package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// MaterialReceiptOutcome 是一次收讫登记或撤销的应用处理结果。
//
// 与目录登记（RegisterCatalogOutcome）不同，这里有幂等重放的两格：收讫行的身份就是
// 事实本身（五件成行），同五件重登没有内容可被顶替，答「已在场」是续办成功而不是
// 治理拒绝——把它折进`已拒绝`会让未决后的重跑被读成失败。没有`未决`格，理由同目录：
// 依赖调不通时没有一个如实的中间答案可记，那一路交回错误。
type MaterialReceiptOutcome uint8

const (
	MaterialReceiptOutcomeInvalid MaterialReceiptOutcome = iota
	MaterialReceiptRegistered
	MaterialReceiptReplayed
	MaterialReceiptRevoked
	MaterialReceiptRevocationReplayed
	MaterialReceiptRefused
)

func (outcome MaterialReceiptOutcome) String() string {
	switch outcome {
	case MaterialReceiptRegistered:
		return "REGISTERED"
	case MaterialReceiptReplayed:
		return "ALREADY_REGISTERED"
	case MaterialReceiptRevoked:
		return "REVOKED"
	case MaterialReceiptRevocationReplayed:
		return "ALREADY_REVOKED"
	case MaterialReceiptRefused:
		return "REFUSED"
	default:
		return ""
	}
}

// MaterialReceiptRefusalReason 指名这一笔差在哪一件。逐格分开的理由随目录登记：恢复
// 动作各不相同——缺行身份要补引用，缺时刻要问清何时收讫/何时撤销，缺经手声明要问清
// 谁经手，时序倒错要核对两个时刻，无从撤销要人工核对收讫引用。
type MaterialReceiptRefusalReason uint8

const (
	MaterialReceiptRefusalReasonNone MaterialReceiptRefusalReason = iota
	ReceiptScopeMissing
	ReceiptMaterialMissing
	ReceiptTimeMissing
	ReceiptResponsibleMissing
	ReceiptRevocationBeforeReceipt
	ReceiptNotFound
)

func (reason MaterialReceiptRefusalReason) String() string {
	switch reason {
	case ReceiptScopeMissing:
		return "SCOPE_MISSING"
	case ReceiptMaterialMissing:
		return "MATERIAL_MISSING"
	case ReceiptTimeMissing:
		return "TIME_MISSING"
	case ReceiptResponsibleMissing:
		return "RESPONSIBLE_MISSING"
	case ReceiptRevocationBeforeReceipt:
		return "REVOCATION_BEFORE_RECEIPT"
	case ReceiptNotFound:
		return "RECEIPT_NOT_FOUND"
	default:
		return ""
	}
}

type MaterialReceiptResult struct {
	outcome MaterialReceiptOutcome
	refusal MaterialReceiptRefusalReason
}

func (result MaterialReceiptResult) Outcome() MaterialReceiptOutcome {
	return result.outcome
}

// RefusalReason 只在被拒时有值。
func (result MaterialReceiptResult) RefusalReason() MaterialReceiptRefusalReason {
	return result.refusal
}

func receiptResult(outcome MaterialReceiptOutcome) MaterialReceiptResult {
	return MaterialReceiptResult{outcome: outcome}
}

func receiptRefused(reason MaterialReceiptRefusalReason) MaterialReceiptResult {
	return MaterialReceiptResult{outcome: MaterialReceiptRefused, refusal: reason}
}

// RegisterMaterialReceiptCommand 登记一笔收讫。ReceivedAt 取材料实际收讫的业务时刻，
// 不取本方时钟——它是行身份的一件，凑时钟会让同一次收讫两次登记长成两行。
type RegisterMaterialReceiptCommand struct {
	TenantID   domain.TenantID
	Batch      domain.ClaimBatchReference
	Item       domain.ClaimItemID
	Material   domain.MaterialRequirementReference
	ReceivedAt time.Time
	ReceivedBy string
}

// RevokeMaterialReceiptCommand 撤销一笔收讫，五件全键指名对象（ports.MaterialReceiptRevocation）。
type RevokeMaterialReceiptCommand struct {
	TenantID   domain.TenantID
	Batch      domain.ClaimBatchReference
	Item       domain.ClaimItemID
	Material   domain.MaterialRequirementReference
	ReceivedAt time.Time
	RevokedBy  string
	RevokedAt  time.Time
}

// MaterialReceiptRegistration 是材料归集面的受控登记用例（.scratch/ve-claims-read-seams/02）。
//
// 它站在写入口之前，只作一件事：**把不完整的登记挡在库外**，一个默认值都不补——缺
// 收讫时刻不拿本方时钟凑，缺经手声明不拿进程属主顶（那是通道身份，另一轨）。撤销的
// 时序判据（不得早于收讫）也在这里拒：库上的 CHECK 是第二道网，但撞上它会把整笔
// 事务打进中止态，而登记与留痕同事务（ADR-0031 不捕 23505 的同一条理由）。
//
// 事务边界不归本用例：登记与通道留痕必须同一提交，事务由进程级入口开启。
type MaterialReceiptRegistration struct {
	registry ports.MaterialReceiptRegistry
}

func NewMaterialReceiptRegistration(registry ports.MaterialReceiptRegistry) (*MaterialReceiptRegistration, error) {
	if registry == nil {
		return nil, fmt.Errorf("visibility exception application: material receipt registry is required")
	}
	return &MaterialReceiptRegistration{registry: registry}, nil
}

func (service *MaterialReceiptRegistration) RegisterReceipt(
	ctx context.Context,
	command RegisterMaterialReceiptCommand,
) (MaterialReceiptResult, error) {
	switch {
	case !present(command.TenantID.String()) ||
		!present(command.Batch.String()) ||
		!present(command.Item.String()):
		return receiptRefused(ReceiptScopeMissing), nil
	case !present(command.Material.String()):
		return receiptRefused(ReceiptMaterialMissing), nil
	case command.ReceivedAt.IsZero():
		return receiptRefused(ReceiptTimeMissing), nil
	case !present(command.ReceivedBy):
		return receiptRefused(ReceiptResponsibleMissing), nil
	}

	outcome, err := service.registry.RegisterReceipt(ctx, command.TenantID, ports.MaterialReceipt{
		Batch:      command.Batch,
		Item:       command.Item,
		Material:   command.Material,
		ReceivedAt: command.ReceivedAt,
		ReceivedBy: command.ReceivedBy,
	})
	if err != nil {
		return MaterialReceiptResult{}, fmt.Errorf("register material receipt: %w", err)
	}
	switch outcome {
	case ports.MaterialReceiptRecorded:
		return receiptResult(MaterialReceiptRegistered), nil
	case ports.MaterialReceiptAlreadyRecorded:
		return receiptResult(MaterialReceiptReplayed), nil
	default:
		return MaterialReceiptResult{}, fmt.Errorf(
			"visibility exception application: 未知收讫登记结果 %d", outcome)
	}
}

func (service *MaterialReceiptRegistration) RevokeReceipt(
	ctx context.Context,
	command RevokeMaterialReceiptCommand,
) (MaterialReceiptResult, error) {
	switch {
	case !present(command.TenantID.String()) ||
		!present(command.Batch.String()) ||
		!present(command.Item.String()):
		return receiptRefused(ReceiptScopeMissing), nil
	case !present(command.Material.String()):
		return receiptRefused(ReceiptMaterialMissing), nil
	case command.ReceivedAt.IsZero(), command.RevokedAt.IsZero():
		return receiptRefused(ReceiptTimeMissing), nil
	case !present(command.RevokedBy):
		return receiptRefused(ReceiptResponsibleMissing), nil
	case command.RevokedAt.Before(command.ReceivedAt):
		return receiptRefused(ReceiptRevocationBeforeReceipt), nil
	}

	outcome, err := service.registry.RevokeReceipt(ctx, command.TenantID, ports.MaterialReceiptRevocation{
		Batch:      command.Batch,
		Item:       command.Item,
		Material:   command.Material,
		ReceivedAt: command.ReceivedAt,
		RevokedBy:  command.RevokedBy,
		RevokedAt:  command.RevokedAt,
	})
	if err != nil {
		return MaterialReceiptResult{}, fmt.Errorf("revoke material receipt: %w", err)
	}
	switch outcome {
	case ports.MaterialReceiptRevocationRecorded:
		return receiptResult(MaterialReceiptRevoked), nil
	case ports.MaterialReceiptRevocationAlreadyRecorded:
		return receiptResult(MaterialReceiptRevocationReplayed), nil
	case ports.MaterialReceiptUnknown:
		return receiptRefused(ReceiptNotFound), nil
	default:
		return MaterialReceiptResult{}, fmt.Errorf(
			"visibility exception application: 未知收讫撤销结果 %d", outcome)
	}
}
