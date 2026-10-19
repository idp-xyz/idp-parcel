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
)

// transactionalIdentityRecheck 把人工重核包进一笔事务，事务边界归装配点，理由同 transactionalReception：身份判断里的包裹锁、追加
// 的那一版与它的收寄意图必须同生共死。编排在意图交不出去时自己交回 error（重核没有人读续办引用），这里整笔回滚、答 5xx。
type transactionalIdentityRecheck struct {
	transactor bentoapp.Transactor
	inner      *nodeopsapp.RecheckPendingIdentityHandler
}

var _ nodeopshttp.IdentityRecheckHandler = transactionalIdentityRecheck{}

func (recheck transactionalIdentityRecheck) Recheck(
	ctx context.Context,
	command nodeopsapp.RecheckPendingIdentityCommand,
) (nodeopsapp.RecheckPendingIdentityResult, error) {
	var result nodeopsapp.RecheckPendingIdentityResult
	err := recheck.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		rechecked, recheckErr := recheck.inner.Recheck(txCtx, command)
		if recheckErr != nil {
			return recheckErr
		}
		result = rechecked
		return nil
	})
	if err != nil {
		return nodeopsapp.RecheckPendingIdentityResult{}, err
	}
	return result, nil
}

// buildIdentityRecheckOrchestration 装配 `/node-operations/identity-rechecks` 的真编排（ADR-0199 决定三第二个触发）。依赖与收寄
// 编排同一组口：重核走的是收寄步骤 4 那一份身份判断，服务结果读口同样经 savepoint 隔离。
func buildIdentityRecheckOrchestration(db *bentopg.DB) (nodeopshttp.IdentityRecheckHandler, error) {
	deps, err := buildIdentityRecheckDeps(db)
	if err != nil {
		return nil, err
	}
	return identityRecheckWithin(db, deps)
}

// buildIdentityRecheckDeps 是 buildIdentityRecheckOrchestration 的依赖半边。单拆出来是为了真库用例只换一只依赖（在读别的实物关联
// 与落册之间挂起），其余照生产装配。
func buildIdentityRecheckDeps(db *bentopg.DB) (nodeopsapp.RecheckPendingIdentityDeps, error) {
	none := nodeopsapp.RecheckPendingIdentityDeps{}
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: recheck receptions: %w", err)
	}
	versions, err := noidentity.NewIntakeResultVersions()
	if err != nil {
		return none, fmt.Errorf("parcel-api: recheck intake result versions: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: recheck outbox store: %w", err)
	}
	clock := systemClock{}
	downstream, err := nopostgres.NewOutboxNodeIntakeHandoff(db, store, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-api: recheck node intake handoff: %w", err)
	}
	identity, err := buildParcelIdentityView(db)
	if err != nil {
		return none, err
	}
	serviceResults, err := buildParcelServiceResultView(db)
	if err != nil {
		return none, err
	}
	scope, err := nopostgres.NewDeliveryScopeEvidence(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: recheck delivery scope evidence: %w", err)
	}
	return nodeopsapp.RecheckPendingIdentityDeps{
		Identity:       identity,
		ServiceResults: serviceResults,
		Receptions:     receptions,
		Pending:        receptions,
		Versions:       versions,
		Downstream:     downstream,
		Scope:          scope,
		Clock:          clock,
	}, nil
}

// identityRecheckWithin 把一组重核依赖装成编排，包进 transactionalIdentityRecheck。
func identityRecheckWithin(db *bentopg.DB, deps nodeopsapp.RecheckPendingIdentityDeps) (nodeopshttp.IdentityRecheckHandler, error) {
	handler, err := nodeopsapp.NewRecheckPendingIdentityHandler(deps)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: identity recheck orchestration: %w", err)
	}
	return transactionalIdentityRecheck{transactor: db.Transactor(), inner: handler}, nil
}
