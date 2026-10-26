package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
)

// transactionalReferenceCatalogueReview 为目录版本复核包事务，判据同序列复核那一只包装：复核册的写口无事务即拒，读登记、
// 读规则、追加复核与交「参数已登记」在同一笔里，信封与复核行同生共死；编排返回错误时整笔回滚，端点答「没形成答案」。
// 不与目录登记那一只合并：两种命令、两套答案代数，合成一个类型之后登记编排接到复核端点上编译仍绿。
type transactionalReferenceCatalogueReview struct {
	transactor bentoapp.Transactor
	inner      *pricingapp.ReviewReferenceCatalogueHandler
}

var _ pricinghttp.ReferenceCatalogueReviewer = transactionalReferenceCatalogueReview{}

func (review transactionalReferenceCatalogueReview) Handle(
	ctx context.Context,
	command pricingapp.ReviewReferenceCatalogueCommand,
) (pricingapp.ReviewReferenceCatalogueOutcome, error) {
	var outcome pricingapp.ReviewReferenceCatalogueOutcome
	err := review.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := review.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		outcome = handled
		return nil
	})
	if err != nil {
		return pricingapp.ReviewReferenceCatalogueOutcomeInvalid, err
	}
	return outcome, nil
}

// buildReferenceCatalogueReviewOrchestration 装配 `/pricing-reference-catalogue-reviews` 的真编排：版本读口、复核写口、
// 参考复核职责规则读口（与序列复核口、受控登记口同一本规则册），通过时按绑了这本目录的卡交「参数已登记」。
func buildReferenceCatalogueReviewOrchestration(db *bentopg.DB) (transactionalReferenceCatalogueReview, error) {
	versions, err := pppostgres.NewReferenceCatalogueVersions(db)
	if err != nil {
		return transactionalReferenceCatalogueReview{}, fmt.Errorf("parcel-api: reference catalogue version loader: %w", err)
	}
	reviews, err := pppostgres.NewReferenceCatalogueReviews(db)
	if err != nil {
		return transactionalReferenceCatalogueReview{}, fmt.Errorf("parcel-api: reference catalogue review register: %w", err)
	}
	dutyRules, err := pppostgres.NewReferenceReviewDutyRules(db)
	if err != nil {
		return transactionalReferenceCatalogueReview{}, fmt.Errorf("parcel-api: reference review duty rule view: %w", err)
	}
	registered, err := pricingParameterRegisteredHandoff(db)
	if err != nil {
		return transactionalReferenceCatalogueReview{}, err
	}
	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		return transactionalReferenceCatalogueReview{}, fmt.Errorf("parcel-api: price card catalog: %w", err)
	}
	handler := pricingapp.NewReviewReferenceCatalogueHandler(pricingapp.ReviewReferenceCatalogueDeps{
		Versions:     versions,
		Reviews:      reviews,
		DutyRules:    dutyRules,
		Clock:        systemClock{},
		Registered:   registered,
		PricedScopes: cards,
	})
	return transactionalReferenceCatalogueReview{transactor: db.Transactor(), inner: handler}, nil
}
