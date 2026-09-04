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

// transactionalPriceCardRegistration 把价卡登记用例包进一笔事务，理由随
// transactionalClaims：登记册的写口按框架合同无事务即拒，事务边界归装配点，形状照
// 登记 CLI 的 execute——在线口与 CLI 消费同一登记用例，答案代数一致（ADR-0085）。
// 用例交回业务答案（含幂等重放与治理答案）时事务提交；返回错误时整笔回滚，端点按
// ADR-0022 答「没形成答案」。
type transactionalPriceCardRegistration struct {
	transactor bentoapp.Transactor
	inner      *pricingapp.RegisterPriceCardHandler
}

var _ pricinghttp.PriceCardRegistrar = transactionalPriceCardRegistration{}

func (registration transactionalPriceCardRegistration) Handle(
	ctx context.Context,
	command pricingapp.RegisterPriceCardCommand,
) (pricingapp.RegisterPriceCardOutcome, error) {
	var outcome pricingapp.RegisterPriceCardOutcome
	err := registration.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := registration.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		outcome = handled
		return nil
	})
	if err != nil {
		return pricingapp.RegisterPriceCardOutcomeInvalid, err
	}
	return outcome, nil
}

// transactionalReferenceSeriesRegistration 同上，为序列登记包事务。两个包装不合并：
// 两端点各有自己的命令与答案代数，合并就得把两组 Handle 挤进一个类型再按命令分派，
// 装配测试会盖不住「一格接错编排」。
type transactionalReferenceSeriesRegistration struct {
	transactor bentoapp.Transactor
	inner      *pricingapp.RegisterReferenceSeriesHandler
}

var _ pricinghttp.ReferenceSeriesRegistrar = transactionalReferenceSeriesRegistration{}

func (registration transactionalReferenceSeriesRegistration) Handle(
	ctx context.Context,
	command pricingapp.RegisterReferenceSeriesCommand,
) (pricingapp.RegisterReferenceSeriesOutcome, error) {
	var outcome pricingapp.RegisterReferenceSeriesOutcome
	err := registration.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := registration.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		outcome = handled
		return nil
	})
	if err != nil {
		return pricingapp.RegisterReferenceSeriesOutcomeInvalid, err
	}
	return outcome, nil
}

// transactionalReferenceSeriesReview 为序列版本复核包事务，判据同上两个包装。
//
// 它不并进 transactionalReferenceSeriesRegistration：登记与复核是两种命令、两套答案代数，
// 而**四眼门只在复核那一侧**（领域拒绝复核责任方等于登记责任方）。合成一个类型之后装配点
// 可以把登记那一格的编排接到复核端点上而编译仍绿，装配测试也盖不住。
type transactionalReferenceSeriesReview struct {
	transactor bentoapp.Transactor
	inner      *pricingapp.ReviewReferenceSeriesHandler
}

var _ pricinghttp.ReferenceSeriesReviewer = transactionalReferenceSeriesReview{}

func (review transactionalReferenceSeriesReview) Handle(
	ctx context.Context,
	command pricingapp.ReviewReferenceSeriesCommand,
) (pricingapp.ReviewReferenceSeriesOutcome, error) {
	var outcome pricingapp.ReviewReferenceSeriesOutcome
	err := review.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := review.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		outcome = handled
		return nil
	})
	if err != nil {
		return pricingapp.ReviewReferenceSeriesOutcomeInvalid, err
	}
	return outcome, nil
}

// buildPriceCardRegistrationOrchestration 装配 `/pricing-price-card-registrations`
// 的真编排（ADR-0085 首切片，票 admin-write-faces/01）。接真不等墙降：未配置 Intake
// 拒在编排之前，接入渠道就位前编排一次也不会被调到——墙降那笔工作换的只是 Intake。
func buildPriceCardRegistrationOrchestration(db *bentopg.DB) (transactionalPriceCardRegistration, error) {
	catalog, err := pppostgres.NewPriceCards(db)
	if err != nil {
		return transactionalPriceCardRegistration{}, fmt.Errorf("parcel-api: price card catalog: %w", err)
	}
	handler := pricingapp.NewRegisterPriceCardHandler(pricingapp.RegisterPriceCardDeps{Catalog: catalog})
	return transactionalPriceCardRegistration{transactor: db.Transactor(), inner: handler}, nil
}

// buildReferenceSeriesRegistrationOrchestration 装配 `/pricing-reference-series-registrations`
// 的真编排，判据同上。
func buildReferenceSeriesRegistrationOrchestration(db *bentopg.DB) (transactionalReferenceSeriesRegistration, error) {
	register, err := pppostgres.NewReferenceSeriesVersions(db)
	if err != nil {
		return transactionalReferenceSeriesRegistration{}, fmt.Errorf("parcel-api: reference series register: %w", err)
	}
	handler := pricingapp.NewRegisterReferenceSeriesHandler(pricingapp.RegisterReferenceSeriesDeps{Register: register})
	return transactionalReferenceSeriesRegistration{transactor: db.Transactor(), inner: handler}, nil
}

// buildReferenceSeriesReviewOrchestration 装配 `/pricing-reference-series-reviews` 的真编排
// （ADR-0099 决定二，票 pricing-reference-series-operations/04）。
//
// 版本读口与复核写口是两只适配器：读回一版登记走 ReferenceSeriesVersions（四眼门要拿到登记
// 责任方，而 domain.NewSeriesReview 以登记为入参），追加复核走 ReferenceSeriesReviews。它们
// 落在两张表上，分设不是为了对称。
func buildReferenceSeriesReviewOrchestration(db *bentopg.DB) (transactionalReferenceSeriesReview, error) {
	versions, err := pppostgres.NewReferenceSeriesVersions(db)
	if err != nil {
		return transactionalReferenceSeriesReview{}, fmt.Errorf("parcel-api: reference series version loader: %w", err)
	}
	reviews, err := pppostgres.NewReferenceSeriesReviews(db)
	if err != nil {
		return transactionalReferenceSeriesReview{}, fmt.Errorf("parcel-api: reference series review register: %w", err)
	}
	handler := pricingapp.NewReviewReferenceSeriesHandler(pricingapp.ReviewReferenceSeriesDeps{
		Versions: versions,
		Reviews:  reviews,
		Clock:    systemClock{},
	})
	return transactionalReferenceSeriesReview{transactor: db.Transactor(), inner: handler}, nil
}

// buildReferenceSeriesPreviewOrchestration 装配 `/pricing-reference-series-previews` 的真编排（票
// pricing-reference-series-operations/08）。**没有事务包装**：预览只从版本读口取对照那一版，不写
// 任何一张表；给它包一笔事务等于在装配点上宣称它会写。它拿的是与复核编排同一只
// ReferenceSeriesVersions 适配器——读回门一处，对照版本经整版重验后才进领域比对。
func buildReferenceSeriesPreviewOrchestration(db *bentopg.DB) (*pricingapp.PreviewReferenceSeriesHandler, error) {
	versions, err := pppostgres.NewReferenceSeriesVersions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: reference series version loader: %w", err)
	}
	return pricingapp.NewPreviewReferenceSeriesHandler(pricingapp.PreviewReferenceSeriesDeps{Versions: versions}), nil
}

var _ pricinghttp.ReferenceSeriesPreviewer = (*pricingapp.PreviewReferenceSeriesHandler)(nil)
