package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 派送重问间隔的存取口（ADR-0230 决定五）。写的是登记册配置写那一口，读的是到期重问。

// DeliveryDispatchReaskCadenceRecord 是登过的一版。
type DeliveryDispatchReaskCadenceRecord struct {
	Cadence    domain.DeliveryDispatchReaskCadence
	RecordedAt time.Time
}

// DeliveryDispatchReaskCadenceRegistry 按版本找回、首登一版。Save 撞键答 false 不报错（ADR-0031）：同版本是重放还是冲突，由编排
// 读回在册那一版比正文分。写口要环境事务。
type DeliveryDispatchReaskCadenceRegistry interface {
	FindByVersion(
		ctx context.Context,
		tenant domain.TenantID,
		version domain.DeliveryDispatchReaskCadenceVersion,
	) (DeliveryDispatchReaskCadenceRecord, bool, error)
	Save(ctx context.Context, record DeliveryDispatchReaskCadenceRecord) (bool, error)
}

// DeliveryDispatchReaskCadenceReader 答一个租户此刻生效的那一版：最近登记的那一版。没登记过答 false——那是`未配置`，不是错误。
type DeliveryDispatchReaskCadenceReader interface {
	CurrentCadence(ctx context.Context, tenant domain.TenantID) (DeliveryDispatchReaskCadenceRecord, bool, error)
}
