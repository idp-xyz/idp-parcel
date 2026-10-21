package main

import (
	"context"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpartycommercial "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/partycommercial"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// errCommissionAgreementNotRevised 只在本壳里流转：编排答换版成立以外的格时回调交它，逼事务回滚；回滚之后认出它，交回编排那一份答复。
var errCommissionAgreementNotRevised = errors.New("parcel-api: commission agreement not revised; transaction rolled back")

// transactionalCommissionAgreementReviser 把一次换版包进一笔事务：换版锁、换版行与门铃同笔，写口无事务即拒，事务边界归装配点。编排答
// 换版成立以外的任一格都整笔回滚再交回那一份答复：其余各格本不该留下写入，未决那几格可能已写到一半（落了行而门铃没交）。
type transactionalCommissionAgreementReviser struct {
	transactor bentoapp.Transactor
	inner      *tfapp.ReviseCommissionAgreementHandler
}

var _ tfhttp.CommissionAgreementReviser = transactionalCommissionAgreementReviser{}

func (reviser transactionalCommissionAgreementReviser) Revise(
	ctx context.Context,
	command tfapp.ReviseCommissionAgreementCommand,
) (tfapp.CommissionAgreementRevisionResult, error) {
	var result tfapp.CommissionAgreementRevisionResult
	err := reviser.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		revised, reviseErr := reviser.inner.Revise(txCtx, command)
		if reviseErr != nil {
			return reviseErr
		}
		result = revised
		if revised.Outcome() != tfapp.CommissionAgreementRevised {
			return errCommissionAgreementNotRevised
		}
		return nil
	})
	if errors.Is(err, errCommissionAgreementNotRevised) {
		return result, nil
	}
	if err != nil {
		return tfapp.CommissionAgreementRevisionResult{}, err
	}
	return result, nil
}

// buildCommissionAgreementRevision 装配 `/transport-fulfillment-transport-commission-agreement-revisions` 背后的真编排（ADR-0231 决定三）。
func buildCommissionAgreementRevision(db *bentopg.DB) (transactionalCommissionAgreementReviser, error) {
	deps, err := commissionAgreementRevisionDeps(db)
	if err != nil {
		return transactionalCommissionAgreementReviser{}, err
	}
	return transactionalCommissionAgreementReviser{transactor: db.Transactor(), inner: tfapp.NewReviseCommissionAgreementHandler(deps)}, nil
}

// commissionAgreementRevisionDeps 交回换版编排的真依赖：委托册（读委托连同它的换版，也是换版锁与换版行的写口）、TF 对 PC 协议版本点读口
// 的消费侧适配器（ADR-0025）、换版门铃的 Outbox 意图与时钟。与事务壳分开交出：并发用例要在同一套真依赖外面套一层挂起。
func commissionAgreementRevisionDeps(db *bentopg.DB) (tfapp.ReviseCommissionAgreementDeps, error) {
	none := tfapp.ReviseCommissionAgreementDeps{}
	commissions, err := tfpostgres.NewTransportCommissions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: transport commissions for agreement revision: %w", err)
	}
	agreementVersions, err := pcpostgres.NewSupplierAgreementContents(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: supplier agreement versions for agreement revision: %w", err)
	}
	agreements, err := tfpartycommercial.NewSupplierAgreementSource(agreementVersions)
	if err != nil {
		return none, fmt.Errorf("parcel-api: supplier agreement check for agreement revision: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: outbox store for agreement revision: %w", err)
	}
	clock := systemClock{}
	doorbell, err := tfpostgres.NewOutboxCommissionAgreementRevisionHandoff(db, store, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-api: commission agreement revision doorbell: %w", err)
	}
	return tfapp.ReviseCommissionAgreementDeps{
		Commissions: commissions,
		Revisions:   commissions,
		Agreements:  agreements,
		Doorbell:    doorbell,
		Clock:       clock,
	}, nil
}
