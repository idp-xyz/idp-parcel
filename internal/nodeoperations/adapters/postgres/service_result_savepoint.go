package postgres

import (
	"context"
	"errors"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// serviceResultSavepoint 是装饰器在调用方事务里开的 savepoint 名。同一事务里先后几次读各自开、各自收，名字重用无妨。
const serviceResultSavepoint = "node_operations_parcel_service_results"

// SavepointedServiceResultView 把服务结果读口隔在一个 savepoint 里（ADR-0186）。
//
// 跨上下文读口须经 savepoint 隔离：PS 的读口在收寄那一笔事务里发 SQL，一条 SQL 级错误（表被挪走、权限、语句超时）
// 会把整笔事务打进中止态，收寄的写跟着作废，提交时只拿到 rollback——编排已经决定「服务结果读不动照常落册」，没有这层
// 隔离那句话在库里不成立。内层失败就退回 savepoint、把错误原样交回，编排据此记读不动；成功就 RELEASE。
//
// ctx 不在事务里时直通：没有可保护的写，事务块外也开不了 savepoint。身份核对读口另由 SavepointedParcelIdentityView 包：
// 它读不动也要落册（ADR-0199 决定五）。
type SavepointedServiceResultView struct {
	db    *bentopg.DB
	inner ports.ParcelServiceResultView
}

var _ ports.ParcelServiceResultView = (*SavepointedServiceResultView)(nil)

func NewSavepointedServiceResultView(db *bentopg.DB, inner ports.ParcelServiceResultView) (*SavepointedServiceResultView, error) {
	if db == nil {
		return nil, errors.New("node operations postgres: db is nil")
	}
	if inner == nil {
		return nil, errors.New("node operations postgres: service result view is nil")
	}
	return &SavepointedServiceResultView{db: db, inner: inner}, nil
}

func (view *SavepointedServiceResultView) FindParcelServiceResults(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.ParcelAssociationReference,
) (ports.ParcelServiceResults, error) {
	executor, err := view.db.RequireExecutor(ctx)
	if errors.Is(err, bentopg.ErrTransactionRequired) {
		return view.inner.FindParcelServiceResults(ctx, tenant, parcel)
	}
	if err != nil {
		return ports.ParcelServiceResults{}, fmt.Errorf("service result savepoint: %w", err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT "+serviceResultSavepoint); err != nil {
		return ports.ParcelServiceResults{}, fmt.Errorf("service result savepoint: %w", err)
	}
	results, readErr := view.inner.FindParcelServiceResults(ctx, tenant, parcel)
	if readErr != nil {
		if _, err := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT "+serviceResultSavepoint); err != nil {
			// 退不回 savepoint，事务就还在中止态，随后的写会自己失败、整笔回滚；两条错误都交回，免得只剩一半原因。
			return ports.ParcelServiceResults{}, errors.Join(readErr, fmt.Errorf("service result savepoint rollback: %w", err))
		}
		return ports.ParcelServiceResults{}, readErr
	}
	if _, err := executor.Exec(ctx, "RELEASE SAVEPOINT "+serviceResultSavepoint); err != nil {
		return ports.ParcelServiceResults{}, fmt.Errorf("service result savepoint release: %w", err)
	}
	return results, nil
}
