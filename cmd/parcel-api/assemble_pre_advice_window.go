package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
)

// transactionalPreAdviceWindowRegistrar 把每次时窗登记包进一笔事务，理由随 transactionalTransportScheduleRegistrar：登记册的写口按
// 框架合同无事务即拒，事务边界归装配点。
type transactionalPreAdviceWindowRegistrar struct {
	transactor bentoapp.Transactor
	inner      *nodeopsapp.RegisterPreAdviceWindowHandler
}

var _ nodeopshttp.PreAdviceWindowRegistrar = transactionalPreAdviceWindowRegistrar{}

func (registrar transactionalPreAdviceWindowRegistrar) Handle(
	ctx context.Context,
	command nodeopsapp.RegisterPreAdviceWindowCommand,
) (nodeopsapp.PreAdviceWindowRegistrationResult, error) {
	var result nodeopsapp.PreAdviceWindowRegistrationResult
	err := registrar.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := registrar.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return nodeopsapp.PreAdviceWindowRegistrationResult{}, err
	}
	return result, nil
}

// buildPreAdviceWindowRegistration 装配 `/node-operations-pre-advice-window-registrations` 背后的真编排（ADR-0212 决定二末项；
// ADR-0085 决定一）。缝——时窗册与时钟——全接真；没有意图交付：时窗是本上下文判交付范围时自己读的租户取值。受控 CLI
// parcel-node-operations-register 装的是同一个编排。
func buildPreAdviceWindowRegistration(db *bentopg.DB) (nodeopshttp.PreAdviceWindowRegistrar, error) {
	windows, err := nopostgres.NewPreAdviceWindows(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: pre-advice windows: %w", err)
	}
	handler, err := nodeopsapp.NewRegisterPreAdviceWindowHandler(nodeopsapp.RegisterPreAdviceWindowDeps{
		Windows: windows,
		Clock:   systemClock{},
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: pre-advice window registration: %w", err)
	}
	return transactionalPreAdviceWindowRegistrar{transactor: db.Transactor(), inner: handler}, nil
}
