package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件是送站交接单命令口要的另两个端口（ADR-0212 决定三、六；票 ps-external-mark-relations/12）：归入要问收寄册的那一问，与「交付
// 范围成立」的自投门铃。

// HandoverAssignmentReceptions 是归入要问收寄册的那一问：这件作业实物在交接单开立的节点上各条判断链此刻的当前有效版本。问法与锁同
// ReceptionStore.FindCurrentJudgmentsAtNode——取的是同一把实物锁，归入与这件实物的收寄、重核串在一起：判完「当前有效版本属于重核
// 对象」到落下成员之间，不会有一版重核插进来把它关联掉。生产上就是收寄库。
type HandoverAssignmentReceptions interface {
	FindCurrentJudgmentsAtNode(
		ctx context.Context,
		tenant domain.TenantID,
		unit domain.HandlingUnitID,
		node domain.NodeReference,
	) ([]ReceptionRecord, error)
}

// DeliveryScopeEstablishedIntent 是「交付范围成立」的一封自投门铃（ADR-0212 决定六）。Version 零值即交接单最后一个绑定得到确认的
// 那一封（租户，交接单），重核名下属于重核对象的各件；非零即实物归入一张范围已成立的交接单的那一封，只重核那一版。Note 两种都带：
// 同一张交接单的门铃排一条队。
type DeliveryScopeEstablishedIntent struct {
	Tenant     domain.TenantID
	Note       domain.DeliveryHandoverNoteID
	Version    domain.IntakeResultVersion
	OccurredAt time.Time
}

// DeliveryScopeEstablishedDoorbell 把门铃写入 Outbox，与触发它的结论或归入同一笔事务。信封身份由触发认领，重放不出第二封。门铃不是
// 身份答案：接住它的一方照旧经身份核对读口问 PS，范围按存下的依据与此刻的证据重算。
type DeliveryScopeEstablishedDoorbell interface {
	RingDeliveryScopeEstablished(ctx context.Context, intent DeliveryScopeEstablishedIntent) error
}
