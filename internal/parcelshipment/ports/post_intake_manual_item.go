package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件是待人工处理项要记、要读与要登记的口：收寄后服务处置决定形成时，请求表里系统不执行的几项与决定同笔落成待人工处理项；运营人员逐项登记处理结果，
// 登记即这一项的承接结果，PS 可读回。

// PostIntakeManualItem 是一项待人工处理项，连同它所属决定的去向与目的地——运营人员要办的事靠这两样才讲得清。
type PostIntakeManualItem struct {
	Key         domain.ManualItemKey
	Direction   domain.PostIntakeDispositionDirection
	Destination domain.PostIntakeDispositionDestination
	DecidedAt   time.Time
	// CurrentVersion 是这件包裹决定链此刻的版本：大于项所属的决定版本，说明这份决定已被后来的决定取代，运营人员办之前应先看链尾说了什么。
	CurrentVersion int
	Result         domain.ManualItemResult
	HasResult      bool
}

// PendingPostIntakeManualItems 是一个租户还没有登记结果的项：最早的在前，至多 limit 行；Count 是全部行数，大于 len(Items) 即清单被页大小截断。
type PendingPostIntakeManualItems struct {
	Items []PostIntakeManualItem
	Count int
}

// PostIntakeManualRegistrationOutcome 是登记结果的封闭答案。
type PostIntakeManualRegistrationOutcome uint8

const (
	PostIntakeManualRegistrationOutcomeInvalid PostIntakeManualRegistrationOutcome = iota
	PostIntakeManualRegistered
	PostIntakeManualAlreadyRegistered
)

// PostIntakeManualItems 是待人工处理项的留痕。租户条件进每条语句，否定结果不区分「没有」与「属于另一个租户」。
type PostIntakeManualItems interface {
	// RecordPostIntakeManualItems 把一份刚形成的决定里需要人工处理的请求项落成待人工处理项，要环境事务：与决定留痕同笔。同一份决定重放不重复落。
	RecordPostIntakeManualItems(ctx context.Context, disposition domain.PostIntakeDisposition) error
	FindPostIntakeManualItem(ctx context.Context, key domain.ManualItemKey) (PostIntakeManualItem, bool, error)
	// RegisterPostIntakeManualItemResult 登记一项的处理结果，每项至多一份：已有结果时不覆盖，答 `已登记过` 并读回先前那一份。键对应的项不在册时答 false。
	RegisterPostIntakeManualItemResult(
		ctx context.Context,
		key domain.ManualItemKey,
		result domain.ManualItemResult,
	) (PostIntakeManualRegistrationOutcome, PostIntakeManualItem, bool, error)
	ListPendingPostIntakeManualItems(ctx context.Context, tenant domain.TenantID, limit int) (PendingPostIntakeManualItems, error)
	// ListPostIntakeManualItemsOfParcel 读一件包裹名下的全部项（含已登记的），决定版本小的在前：PS 读回各项承接结果走这里。
	ListPostIntakeManualItemsOfParcel(ctx context.Context, tenant domain.TenantID, parcel domain.DeclaredParcelID) ([]PostIntakeManualItem, error)
}
