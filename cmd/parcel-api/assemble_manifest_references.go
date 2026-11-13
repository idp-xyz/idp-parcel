package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// transactionalManifests 把外部监管舱单引用的首登与版本推进各包进一笔事务，理由随 transactionalResults：引用册的写口
// 按框架合同无事务即拒，事务边界归装配点。编排交回业务答案（含待关联、未受理、不在册、各种冲突）时事务提交；编排返回
// 错误时整笔回滚。意图与引用在同一笔里入队：入队的 SQL 失败时这笔事务已经坏了，提交不成、整笔回滚、答 5xx，通道
// 重发时引用与意图一起重来——所以这条装配上不会出现「引用已入册而意图没交」，编排的续办引用在这里实际用不上。
type transactionalManifests struct {
	transactor bentoapp.Transactor
	inner      *customsapp.ReceiveManifestHandler
}

var _ customshttp.ManifestHandler = transactionalManifests{}

func (manifests transactionalManifests) Receive(
	ctx context.Context,
	command customsapp.ReceiveManifestCommand,
) (customsapp.ManifestResult, error) {
	var result customsapp.ManifestResult
	err := manifests.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := manifests.inner.Receive(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return customsapp.ManifestResult{}, err
	}
	return result, nil
}

func (manifests transactionalManifests) Revise(
	ctx context.Context,
	command customsapp.ReviseManifestCommand,
) (customsapp.ManifestResult, error) {
	var result customsapp.ManifestResult
	err := manifests.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := manifests.inner.Revise(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return customsapp.ManifestResult{}, err
	}
	return result, nil
}

// buildManifestReferencesOrchestration 装配外部监管舱单引用两口的真编排（UC-CC-012）。
//
// 四条缝全接真：候选申报单元读口、引用册、Outbox 意图交付与生产时钟。实例半边的留白不在缝上：哪家承运商的哪只客户端
// 持有这一类事实的授予、绑定哪个来源身份，是租户在集成客户端册上登的行；没登时认证那一步就答未授予，到不了这里。
func buildManifestReferencesOrchestration(db *bentopg.DB) (customshttp.ManifestHandler, error) {
	candidates, err := ccpostgres.NewManifestCandidateView(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: manifest candidate view: %w", err)
	}
	manifests, err := ccpostgres.NewManifests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: manifest store: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	downstream, err := ccpostgres.NewOutboxManifestHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: manifest handoff: %w", err)
	}
	handler := customsapp.NewReceiveManifestHandler(customsapp.ReceiveManifestDeps{
		Candidates: candidates,
		Store:      manifests,
		Downstream: downstream,
		Clock:      clock,
	})
	return transactionalManifests{transactor: db.Transactor(), inner: handler}, nil
}
