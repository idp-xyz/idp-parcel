package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// WaveAssignment 是一个波次此刻派给了谁。Assignee 为空是没派，谁都可以拣。
type WaveAssignment struct {
	Warehouse domain.WarehouseCode
	Status    string
	Assignee  string
}

// WaveAssignmentStore 读写波次的派工。写入在没有环境事务时拒绝。
type WaveAssignmentStore interface {
	// LoadWaveAssignment 先锁住波次那一行再读：两个人同时领同一个波次时排在这一行上，后到的看得到先到的。
	// 只在事务里用。
	LoadWaveAssignment(ctx context.Context, tenant domain.TenantID, wave domain.WaveID) (WaveAssignment, bool, error)
	// WaveAssignee 不加锁地读这个波次派给了谁；没有这个波次或没派答空。
	WaveAssignee(ctx context.Context, tenant domain.TenantID, wave domain.WaveID) (string, error)
	// SaveWaveAssignee 把波次派给 assignee，by 是派的人；assignee 为空是取消指派。
	SaveWaveAssignee(ctx context.Context, tenant domain.TenantID, wave domain.WaveID, assignee, by string, at time.Time) error
	// WaveZoneAssignee 不加锁地读这个波次的这个库区单独派给了谁；没单独派答空。
	WaveZoneAssignee(ctx context.Context, tenant domain.TenantID, wave domain.WaveID, zone domain.ZoneCode) (string, error)
	// SaveWaveZoneAssignee 把波次里一个库区的格派给 assignee；assignee 为空是取消这个库区的指派，它的格回到照整个波次的指派走。
	SaveWaveZoneAssignee(ctx context.Context, tenant domain.TenantID, wave domain.WaveID, zone domain.ZoneCode, assignee, by string, at time.Time) error
}
