package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// DeliveryScopeEvidence 实现 ports.DeliveryScopeEvidence：收寄与重核的身份判断推导可验证交付范围时，按标识读预报与交接单、按实物读
// 最后归入的交接单与它是不是交接单成员、按原收寄业务时间读时窗。三本册各自的读口就是这几问，这里只把它们并成一个口；写口一个也不露出来。读走各册的
// ReadExecutor：在收寄那一笔事务里调时读得到同一事务里刚写的行。
type DeliveryScopeEvidence struct {
	preAdvices *DeliveryPreAdvices
	notes      *DeliveryHandoverNotes
	windows    *PreAdviceWindows
}

var _ ports.DeliveryScopeEvidence = DeliveryScopeEvidence{}

func NewDeliveryScopeEvidence(db *bentopg.DB) (DeliveryScopeEvidence, error) {
	preAdvices, err := NewDeliveryPreAdvices(db)
	if err != nil {
		return DeliveryScopeEvidence{}, fmt.Errorf("delivery scope evidence: %w", err)
	}
	notes, err := NewDeliveryHandoverNotes(db)
	if err != nil {
		return DeliveryScopeEvidence{}, fmt.Errorf("delivery scope evidence: %w", err)
	}
	windows, err := NewPreAdviceWindows(db)
	if err != nil {
		return DeliveryScopeEvidence{}, fmt.Errorf("delivery scope evidence: %w", err)
	}
	return DeliveryScopeEvidence{preAdvices: preAdvices, notes: notes, windows: windows}, nil
}

func (evidence DeliveryScopeEvidence) FindDeliveryPreAdvice(
	ctx context.Context, tenant domain.TenantID, id domain.DeliveryPreAdviceID,
) (domain.DeliveryPreAdviceState, bool, error) {
	return evidence.preAdvices.FindDeliveryPreAdvice(ctx, tenant, id)
}

func (evidence DeliveryScopeEvidence) FindDeliveryHandoverNote(
	ctx context.Context, tenant domain.TenantID, id domain.DeliveryHandoverNoteID,
) (domain.DeliveryHandoverNoteState, bool, error) {
	return evidence.notes.FindDeliveryHandoverNote(ctx, tenant, id)
}

func (evidence DeliveryScopeEvidence) FindLastAssignedHandoverNote(
	ctx context.Context, tenant domain.TenantID, unit domain.HandlingUnitID,
) (domain.DeliveryHandoverNoteID, bool, error) {
	return evidence.notes.FindLastAssignedHandoverNote(ctx, tenant, unit)
}

func (evidence DeliveryScopeEvidence) IsDeliveryHandoverMember(
	ctx context.Context, tenant domain.TenantID, note domain.DeliveryHandoverNoteID, unit domain.HandlingUnitID,
) (bool, error) {
	return evidence.notes.IsDeliveryHandoverMember(ctx, tenant, note, unit)
}

func (evidence DeliveryScopeEvidence) FindPreAdviceWindowAt(
	ctx context.Context, tenant domain.TenantID, at time.Time,
) (domain.PreAdviceValidityWindow, bool, error) {
	return evidence.windows.FindPreAdviceWindowAt(ctx, tenant, at)
}
