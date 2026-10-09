package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/pricecardtemplate"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
)

// buildPriceCardImportPreview 装配 `/pricing-price-card-previews` 的真编排（ADR-0101 决定四；票 price-card-import/02）。
// 预览只读上传的字节、不碰库，依赖只有模板读口；录入口（票 03）要拿同一个读口，同一份字节过两口才逐字节同摘要。
func buildPriceCardImportPreview() (*pricingapp.PreviewPriceCardImportHandler, error) {
	handler, err := pricingapp.NewPreviewPriceCardImportHandler(pricecardtemplate.Reader{})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: price card import preview: %w", err)
	}
	return handler, nil
}

// transactionalPriceCardDraftSubmission 把价卡录入包进一笔事务，判据同 transactionalPriceCardRegistration：草稿册
// 写口按框架合同无事务即拒，读册上那一行与替换它要在同一笔里，事务边界归装配点。落点（含重放、内容已固定与未受理）
// 是业务答案，事务提交；返回错误时整笔回滚，端点答「没形成答案」。
type transactionalPriceCardDraftSubmission struct {
	transactor bentoapp.Transactor
	inner      *pricingapp.SubmitPriceCardDraftHandler
}

var _ pricinghttp.PriceCardDraftSubmitter = transactionalPriceCardDraftSubmission{}

func (submission transactionalPriceCardDraftSubmission) Handle(
	ctx context.Context,
	command pricingapp.SubmitPriceCardDraftCommand,
) (pricingapp.SubmitPriceCardDraftResult, error) {
	var result pricingapp.SubmitPriceCardDraftResult
	err := submission.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := submission.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return pricingapp.SubmitPriceCardDraftResult{}, err
	}
	return result, nil
}

// buildPriceCardDraftSubmission 装配 `/pricing-price-card-drafts` 的真编排（ADR-0101 决定三；票 price-card-import/03）：
// 模板读口与预览口是同一个，录入时刻取系统时钟。
func buildPriceCardDraftSubmission(db *bentopg.DB) (transactionalPriceCardDraftSubmission, error) {
	drafts, err := pppostgres.NewPriceCardDrafts(db)
	if err != nil {
		return transactionalPriceCardDraftSubmission{}, fmt.Errorf("parcel-api: price card draft register: %w", err)
	}
	handler, err := pricingapp.NewSubmitPriceCardDraftHandler(pricecardtemplate.Reader{}, drafts, systemClock{})
	if err != nil {
		return transactionalPriceCardDraftSubmission{}, fmt.Errorf("parcel-api: price card draft submission: %w", err)
	}
	return transactionalPriceCardDraftSubmission{transactor: db.Transactor(), inner: handler}, nil
}
