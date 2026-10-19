package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	noidentity "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/identity"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodeopsdomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	nodeopsports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// transactionalIdentityDisposition 把身份冲突处置包进一笔事务，事务边界归装配点，理由同 transactionalIdentityRecheck：包裹锁、追加的
// 各版与收寄意图必须同生共死。编排在意图交不出去时自己交回 error（处置口没有人读续办引用），这里整笔回滚、答 5xx。
type transactionalIdentityDisposition struct {
	transactor bentoapp.Transactor
	inner      *nodeopsapp.DisposeIdentityConflictHandler
}

var _ nodeopshttp.IdentityDispositionHandler = transactionalIdentityDisposition{}

func (disposition transactionalIdentityDisposition) Dispose(
	ctx context.Context,
	command nodeopsapp.DisposeIdentityConflictCommand,
) (nodeopsapp.DisposeIdentityConflictResult, error) {
	var result nodeopsapp.DisposeIdentityConflictResult
	err := disposition.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		disposed, disposeErr := disposition.inner.Dispose(txCtx, command)
		if disposeErr != nil {
			return disposeErr
		}
		result = disposed
		return nil
	})
	if err != nil {
		return nodeopsapp.DisposeIdentityConflictResult{}, err
	}
	return result, nil
}

// buildIdentityDispositionOrchestration 装配 `/node-operations/identity-dispositions` 的真编排（UC-NO-004）。接真不等操作者授予：
// 运营决定 Intake 拒在编排之前，谁持有这一种授予、准入范围登没登都是租户取值。
func buildIdentityDispositionOrchestration(db *bentopg.DB) (nodeopshttp.IdentityDispositionHandler, error) {
	deps, err := buildIdentityDispositionDeps(db)
	if err != nil {
		return nil, err
	}
	return identityDispositionWithin(db, deps)
}

// buildIdentityDispositionDeps 是 buildIdentityDispositionOrchestration 的依赖半边。单拆出来是为了真库用例只换一只依赖（取完包裹锁
// 之后挂起），其余照生产装配。服务结果读口与收寄、重核同一只，经 savepoint 隔离。
func buildIdentityDispositionDeps(db *bentopg.DB) (nodeopsapp.DisposeIdentityConflictDeps, error) {
	none := nodeopsapp.DisposeIdentityConflictDeps{}
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: disposition receptions: %w", err)
	}
	versions, err := noidentity.NewIntakeResultVersions()
	if err != nil {
		return none, fmt.Errorf("parcel-api: disposition intake result versions: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: disposition outbox store: %w", err)
	}
	clock := systemClock{}
	downstream, err := nopostgres.NewOutboxNodeIntakeHandoff(db, store, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-api: disposition node intake handoff: %w", err)
	}
	serviceResults, err := buildParcelServiceResultView(db)
	if err != nil {
		return none, err
	}
	return nodeopsapp.DisposeIdentityConflictDeps{
		ServiceResults: serviceResults,
		Receptions:     receptions,
		Register:       receptions,
		Versions:       versions,
		Downstream:     downstream,
		Clock:          clock,
	}, nil
}

// identityDispositionWithin 把一组处置依赖装成编排，包进 transactionalIdentityDisposition。
func identityDispositionWithin(db *bentopg.DB, deps nodeopsapp.DisposeIdentityConflictDeps) (nodeopshttp.IdentityDispositionHandler, error) {
	handler, err := nodeopsapp.NewDisposeIdentityConflictHandler(deps)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: identity disposition orchestration: %w", err)
	}
	return transactionalIdentityDisposition{transactor: db.Transactor(), inner: handler}, nil
}

// ListIdentityReviewUnits 让未接线的节点作业查阅读口也答按实物上列那一问，与它别的几问同一个答法。
func (unwiredNodeOperationsRecords) ListIdentityReviewUnits(
	context.Context,
	nodeopsdomain.TenantID,
	int,
) ([]nodeopsports.IdentityReviewUnitRow, error) {
	return nil, errOrchestrationNotWired
}
