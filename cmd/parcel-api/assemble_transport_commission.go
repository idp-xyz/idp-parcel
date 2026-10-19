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

// transactionalTransportCommissionSubmitter 把一次委托提交包进一笔事务，理由随 transactionalDelivery：委托册的写口与
// Outbox 入队按框架合同无事务即拒，事务边界归装配点；委托与「委托已提交」意图同笔落地，编排返回 error 时整笔回滚。
// 协议核对读的是 party-commercial 的已提交数据，落在同一笔里不改变它读到什么。
//
// 编排答`未决`时整笔回滚而不提交，再交回那一份答复：读口（协议点读、委托册）在事务里报了 SQL 错误，PostgreSQL 就把
// 整笔事务置为 aborted，照常提交只换回提交错误，线上落成 500 `NO_ANSWER_FORMED`，与 ADR-0202 决定三写的「PC 读不动
// 答未决」对不上；`未决`各格也没有任何东西要落库。先例是 errActionAdmissionUnreadable。
type transactionalTransportCommissionSubmitter struct {
	transactor bentoapp.Transactor
	inner      *tfapp.CommissionTransportHandler
}

// errTransportCommissionUndecided 只在本壳里流转：编排答`未决`时回调交它，逼事务回滚；回滚之后认出它，交回编排那一份
// 答复。
var errTransportCommissionUndecided = errors.New("parcel-api: transport commission undecided; transaction rolled back")

var _ tfhttp.TransportCommissionSubmitter = transactionalTransportCommissionSubmitter{}

func (submitter transactionalTransportCommissionSubmitter) SubmitCommission(
	ctx context.Context,
	command tfapp.SubmitCommissionCommand,
) (tfapp.CommissionResult, error) {
	var result tfapp.CommissionResult
	err := submitter.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := submitter.inner.SubmitCommission(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		if handled.Outcome() == tfapp.CommissionUndecided {
			return errTransportCommissionUndecided
		}
		return nil
	})
	if errors.Is(err, errTransportCommissionUndecided) {
		return result, nil
	}
	if err != nil {
		return tfapp.CommissionResult{}, err
	}
	return result, nil
}

// buildTransportCommissionSubmission 装配 `/transport-fulfillment-transport-commission-submissions` 背后的真编排（票
// demo-chain-gaps/23，ADR-0202）。缝全接真：委托册、供应商协议核对（PC 的版本点读口经 TF 消费侧适配器，ADR-0025）、
// Outbox 意图交付与时钟。订舱与承运应答的册子留给票 24 接：本口只提交委托，编排的那几条缝在这里不装。
//
// 「委托已提交」信封照 ADR-0066 不登派发路由，派发对它如实答 `dispatch.no_subscriber` 直到失败预算耗尽——那是记录里认下
// 的代价，不是装配缺口（ADR-0202 决定六）。
func buildTransportCommissionSubmission(db *bentopg.DB) (tfhttp.TransportCommissionSubmitter, error) {
	commissions, err := tfpostgres.NewTransportCommissions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: transport commissions: %w", err)
	}
	contents, err := pcpostgres.NewSupplierAgreementContents(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: supplier agreement contents: %w", err)
	}
	agreements, err := tfpartycommercial.NewSupplierAgreementSource(contents)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: supplier agreement source: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	downstream, err := tfpostgres.NewOutboxTransportCommissionHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: transport commission handoff: %w", err)
	}
	handler := tfapp.NewCommissionTransportHandler(tfapp.CommissionTransportDeps{
		Commissions: commissions,
		Agreements:  agreements,
		Downstream:  downstream,
		Clock:       clock,
	})
	return transactionalTransportCommissionSubmitter{transactor: db.Transactor(), inner: handler}, nil
}
