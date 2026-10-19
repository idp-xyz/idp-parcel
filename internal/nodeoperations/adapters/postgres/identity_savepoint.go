package postgres

import (
	"context"
	"errors"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// identitySavepoint 是装饰器在调用方事务里开的 savepoint 名。与服务结果那一只各用各的名字：两只读口在同一笔事务里先后开、各自收，
// 名字不同，一只退回时不会退到另一只的点上。
const identitySavepoint = "node_operations_parcel_identity"

// SavepointedParcelIdentityView 把身份核对读口隔在一个 savepoint 里（ADR-0199 决定五），做法同 SavepointedServiceResultView。
//
// 身份核对读不动要落未核对的待识别：PS 的读口在收寄那一笔事务里发 SQL，一条 SQL 级错误（表被挪走、权限、语句超时）会把整笔事务
// 打进中止态，未核对那一行跟着写不进去，提交时只拿到 rollback——编排已经决定「读不动照常落册」，没有这层隔离那句话在库里不成立。
// 内层失败就退回 savepoint、把错误原样交回，编排据此记未核对；成功就 RELEASE。ctx 不在事务里时直通：没有可保护的写，事务块外也
// 开不了 savepoint。
type SavepointedParcelIdentityView struct {
	db    *bentopg.DB
	inner ports.ParcelIdentityView
}

var _ ports.ParcelIdentityView = (*SavepointedParcelIdentityView)(nil)

func NewSavepointedParcelIdentityView(db *bentopg.DB, inner ports.ParcelIdentityView) (*SavepointedParcelIdentityView, error) {
	if db == nil {
		return nil, errors.New("node operations postgres: db is nil")
	}
	if inner == nil {
		return nil, errors.New("node operations postgres: parcel identity view is nil")
	}
	return &SavepointedParcelIdentityView{db: db, inner: inner}, nil
}

func (view *SavepointedParcelIdentityView) ResolveParcelIdentity(
	ctx context.Context,
	tenant domain.TenantID,
	observation ports.ExternalMarkObservation,
) (ports.ParcelIdentityAnswer, error) {
	executor, err := view.db.RequireExecutor(ctx)
	if errors.Is(err, bentopg.ErrTransactionRequired) {
		return view.inner.ResolveParcelIdentity(ctx, tenant, observation)
	}
	if err != nil {
		return ports.ParcelIdentityAnswer{}, fmt.Errorf("parcel identity savepoint: %w", err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT "+identitySavepoint); err != nil {
		return ports.ParcelIdentityAnswer{}, fmt.Errorf("parcel identity savepoint: %w", err)
	}
	answer, readErr := view.inner.ResolveParcelIdentity(ctx, tenant, observation)
	if readErr != nil {
		if _, err := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT "+identitySavepoint); err != nil {
			// 退不回 savepoint，事务就还在中止态，随后的写会自己失败、整笔回滚；两条错误都交回，免得只剩一半原因。
			return ports.ParcelIdentityAnswer{}, errors.Join(readErr, fmt.Errorf("parcel identity savepoint rollback: %w", err))
		}
		return ports.ParcelIdentityAnswer{}, readErr
	}
	if _, err := executor.Exec(ctx, "RELEASE SAVEPOINT "+identitySavepoint); err != nil {
		return ports.ParcelIdentityAnswer{}, fmt.Errorf("parcel identity savepoint release: %w", err)
	}
	return answer, nil
}
