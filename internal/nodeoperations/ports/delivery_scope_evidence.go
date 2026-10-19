package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// DeliveryScopeEvidence 是身份判断推导可验证交付范围时要读的证据状态（ADR-0212 决定四至六）：依据指名的预报或交接单此刻的状态、
// 一件实物最后归入的交接单，与原收寄业务时间生效的那一行预报有效时窗。只读，不经写口：收寄与重核不改这几本册。读法与各本册的同名
// 读口逐格相同，生产上就是那几本册；单立一个端口，是为了编排的替身只写这几问，不必连带各本册的写口。读不动作为错误返回——那是本方
// 册读不动，身份判断据此不形成答案，不当成没有依据。
type DeliveryScopeEvidence interface {
	FindDeliveryPreAdvice(ctx context.Context, tenant domain.TenantID, id domain.DeliveryPreAdviceID) (domain.DeliveryPreAdviceState, bool, error)
	FindDeliveryHandoverNote(ctx context.Context, tenant domain.TenantID, id domain.DeliveryHandoverNoteID) (domain.DeliveryHandoverNoteState, bool, error)
	FindLastAssignedHandoverNote(ctx context.Context, tenant domain.TenantID, unit domain.HandlingUnitID) (domain.DeliveryHandoverNoteID, bool, error)
	FindPreAdviceWindowAt(ctx context.Context, tenant domain.TenantID, at time.Time) (domain.PreAdviceValidityWindow, bool, error)
}
