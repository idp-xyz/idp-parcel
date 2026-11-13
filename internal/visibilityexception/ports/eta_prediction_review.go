package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// ETAPredictionReviewRow 是 ETA 预测查阅的一行：一条预测线的当前版，各格照行转写。
//
// 这是运营读面，只说「登记了什么预测」；这份预测对客户可不可见是可见性判断的事，不在这一行上答。
type ETAPredictionReviewRow struct {
	Parcel       string
	Milestone    string
	Source       string
	Version      string
	Inputs       string
	Model        string
	RangeFrom    time.Time
	RangeTo      time.Time
	Confidence   string
	PredictedAt  time.Time
	PriorVersion string
	// ManualEntry 只在这一版是人工录入的时在场；缺席即预测来源送来的。
	ManualEntry *ManualETAEntryNote
}

// ManualETAEntryNote 是人工录入 ETA 的留痕：谁录的、凭哪份原件、何时录的。
type ManualETAEntryNote struct {
	EnteredBy string
	Original  string
	EnteredAt time.Time
}

// ETAPredictionReviewRead 按租户上列各预测线的当前版。
type ETAPredictionReviewRead interface {
	ListCurrentETAPredictions(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ETAPredictionReviewRow], error)
}
