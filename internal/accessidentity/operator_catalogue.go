package accessidentity

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// ErrOperatorCatalogueTenantRequired 表示按租户列操作者时没给出租户。空租户不能列成「全部租户的操作者」。
var ErrOperatorCatalogueTenantRequired = errors.New("access identity: operator catalogue list needs a tenant")

// ErrIncompleteOperatorCatalogueRow 表示查阅行缺登记时刻：册上每一行都带登记时刻，读回来没有是读口的毛病。
var ErrIncompleteOperatorCatalogueRow = errors.New("access identity: operator catalogue row needs a recorded time")

// RecordedGrantRow 是操作者册查阅交回的一笔授予：册上的授予（连同撤销）加它的登记时刻。登记时刻不进 OperatorGrant——
// 它不是授予的内容，重放比对不看它；查阅页按它排序、显示「谁先登的」，所以只在查阅行上带。
type RecordedGrantRow struct {
	recorded   RecordedGrant
	recordedAt time.Time
}

func NewRecordedGrantRow(recorded RecordedGrant, recordedAt time.Time) (RecordedGrantRow, error) {
	if recordedAt.IsZero() {
		return RecordedGrantRow{}, ErrIncompleteOperatorCatalogueRow
	}
	return RecordedGrantRow{recorded: recorded, recordedAt: recordedAt}, nil
}

func (row RecordedGrantRow) Recorded() RecordedGrant { return row.recorded }
func (row RecordedGrantRow) RecordedAt() time.Time   { return row.recordedAt }

// OperatorCatalogueRow 是操作者册查阅交回的一个操作者：绑定、它的登记时刻，与名下每一笔授予（连同撤销与登记时刻）。
//
// 它与 OperatorStanding 答的是同一本册，分开是因为用处不同：OperatorStanding 给铸造信封判「此刻持不持有」，这里给
// 管理台显「册上怎么写、什么时候写的」（ADR-0363 决定二）。此刻生不生效不在行上存——那是一份会过期的推导，由读的人对
// 自己的时点判。
type OperatorCatalogueRow struct {
	binding    OperatorBinding
	recordedAt time.Time
	grants     []RecordedGrantRow
}

// NewOperatorCatalogueRow 的纪律同 NewOperatorStanding：混进别的主体或别的租户名下的授予即拒。
func NewOperatorCatalogueRow(binding OperatorBinding, recordedAt time.Time, grants []RecordedGrantRow) (OperatorCatalogueRow, error) {
	if recordedAt.IsZero() {
		return OperatorCatalogueRow{}, ErrIncompleteOperatorCatalogueRow
	}
	copied := make([]RecordedGrantRow, 0, len(grants))
	for _, row := range grants {
		if row.recorded.grant.subject != binding.subject || row.recorded.grant.tenantID != binding.tenantID {
			return OperatorCatalogueRow{}, ErrGrantOutsideBinding
		}
		copied = append(copied, row)
	}
	return OperatorCatalogueRow{binding: binding, recordedAt: recordedAt, grants: copied}, nil
}

func (row OperatorCatalogueRow) Binding() OperatorBinding { return row.binding }
func (row OperatorCatalogueRow) RecordedAt() time.Time    { return row.recordedAt }

func (row OperatorCatalogueRow) Grants() []RecordedGrantRow {
	return append([]RecordedGrantRow(nil), row.grants...)
}

// OperatorCatalogue 是操作者册按租户的查阅口与按授予标识的装载口（ADR-0363 决定二、三）。
//
// ListOperators 只列本租户，按 OperatorCataloguePages 解出的查询取一页（ADR-0144 决定六）：本页的操作者连同名下的授予与
// 撤销出自同一条查询，绑定与授予不取自两个时点（ADR-0363 决定二）；授予在各自操作者名下按登记时刻排。limit 是页大小，非正
// 即拒；空册答空页而不是 error。FindGrant 按（租户、授予标识）取一笔授予连同撤销，给撤销口在写入前判防锁死；不在册答
// found=false 而不返回 error，理由同 OperatorRegistry。
type OperatorCatalogue interface {
	ListOperators(
		ctx context.Context,
		tenantID string,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[OperatorCatalogueRow], error)
	FindGrant(ctx context.Context, tenantID, grantID string) (RecordedGrant, bool, error)
}
