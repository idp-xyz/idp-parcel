package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件是送站预报册与送站交接单册的运营查阅读端口（票 ps-external-mark-relations/13；UC-NO-006「权限、隔离与审计」）：管理台按节点与
// 业务时刻列预报与交接单、按标识读一份。读口不与两本册的写口共接口，理由同 ReviewCatalogueRead：扩写侧接口会拆全部写侧替身。
//
// 交回的是经领域还原门读回的证据状态（domain.DeliveryPreAdviceState、domain.DeliveryHandoverNoteState），不是照行转写：范围成立与否、
// 成立时刻都由领域按各账户的结论派生（DeliveryHandoverNoteState.ScopeEstablishedAt），读面另算一遍就是第二个口径；一行坏记录也要让整页
// 答不出，而不是悄悄显示成「待确认」。
//
// 租户在方法签名上，别的租户的同名对象与不存在同答（found=false；列表里不出现）：指名别的客户账户或别的租户的对象，与查无此对象同答
// （UC-NO-006「权限、隔离与审计」）。Limit 必须为正，每页多大由接入面按渠道契约裁决。

// DeliveryScopeCatalogueFilter 是两本册列表的筛选：节点逐字比，零值即不按节点筛；From、Until 是业务时刻（预报的预计送达、交接单的开立）
// 的左闭右开区间，各自零值即那一端不设界。「一日」从哪一刻起算随看的人所在的时区，所以读口只收瞬间，不收日期。
type DeliveryScopeCatalogueFilter struct {
	Node  domain.NodeReference
	From  time.Time
	Until time.Time
}

// DeliveryHandoverNoteCatalogueRow 是交接单列表的一行：交接单此刻的证据状态（开立、关闭、各账户的结论），与名下的成员数。成员逐件不在
// 列表行上，详情才列。
type DeliveryHandoverNoteCatalogueRow struct {
	State       domain.DeliveryHandoverNoteState
	MemberCount int64
}

// DeliveryHandoverNoteDetail 是一张交接单的详情：证据状态，与名下的成员（收寄带来的与归入的），按落册先后。
type DeliveryHandoverNoteDetail struct {
	State   domain.DeliveryHandoverNoteState
	Members []domain.DeliveryHandoverMember
}

// DeliveryScopeCatalogueRead 是两本册的运营查阅读端口。列表新近在前（业务时刻倒序，同刻按标识）。
type DeliveryScopeCatalogueRead interface {
	ListDeliveryPreAdvices(
		ctx context.Context,
		tenant domain.TenantID,
		filter DeliveryScopeCatalogueFilter,
		limit int,
	) ([]domain.DeliveryPreAdviceState, error)
	FindDeliveryPreAdvice(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.DeliveryPreAdviceID,
	) (domain.DeliveryPreAdviceState, bool, error)
	ListDeliveryHandoverNotes(
		ctx context.Context,
		tenant domain.TenantID,
		filter DeliveryScopeCatalogueFilter,
		limit int,
	) ([]DeliveryHandoverNoteCatalogueRow, error)
	FindDeliveryHandoverNoteDetail(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.DeliveryHandoverNoteID,
	) (DeliveryHandoverNoteDetail, bool, error)
}
