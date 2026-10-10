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

// transactionalPriceCardDraftApproval 把批准包进一笔事务（票 price-card-import/04）：草稿推进口无事务即拒，读那一行、判、
// 写回在同一笔里。答案（含`未配置`与各格拒绝）提交，返回错误整笔回滚。
type transactionalPriceCardDraftApproval struct {
	transactor bentoapp.Transactor
	inner      *pricingapp.ApprovePriceCardDraftHandler
}

var _ pricinghttp.PriceCardDraftApprover = transactionalPriceCardDraftApproval{}

func (approval transactionalPriceCardDraftApproval) Handle(
	ctx context.Context,
	command pricingapp.ApprovePriceCardDraftCommand,
) (pricingapp.ApprovePriceCardDraftResult, error) {
	var result pricingapp.ApprovePriceCardDraftResult
	err := approval.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := approval.inner.Handle(txCtx, command)
		result = handled
		return handleErr
	})
	if err != nil {
		return pricingapp.ApprovePriceCardDraftResult{}, err
	}
	return result, nil
}

// transactionalPriceCardDraftPublication 把发布包进一笔事务：登记写入与草稿推进同笔（ADR-0101 决定五）。登记落定而草稿
// 跟不上时编排返回错误，整笔回滚——不留「版本已入册、草稿还停在已批准」的半截。
type transactionalPriceCardDraftPublication struct {
	transactor bentoapp.Transactor
	inner      *pricingapp.PublishPriceCardDraftHandler
}

var _ pricinghttp.PriceCardDraftPublisher = transactionalPriceCardDraftPublication{}

func (publication transactionalPriceCardDraftPublication) Handle(
	ctx context.Context,
	command pricingapp.PublishPriceCardDraftCommand,
) (pricingapp.PublishPriceCardDraftResult, error) {
	var result pricingapp.PublishPriceCardDraftResult
	err := publication.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := publication.inner.Handle(txCtx, command)
		result = handled
		return handleErr
	})
	if err != nil {
		return pricingapp.PublishPriceCardDraftResult{}, err
	}
	return result, nil
}

// buildPriceCardDraftApproval 装配 `/pricing-price-card-draft-approvals` 的真编排：真库草稿册、真库审批职责规则册（行属
// 实例半边，今天没有租户登过，批准一律答`未配置`）与系统时钟。
func buildPriceCardDraftApproval(db *bentopg.DB) (transactionalPriceCardDraftApproval, error) {
	drafts, err := pppostgres.NewPriceCardDrafts(db)
	if err != nil {
		return transactionalPriceCardDraftApproval{}, fmt.Errorf("parcel-api: price card draft register: %w", err)
	}
	rules, err := pppostgres.NewPriceCardApprovalDutyRules(db)
	if err != nil {
		return transactionalPriceCardDraftApproval{}, fmt.Errorf("parcel-api: price card approval duty rules: %w", err)
	}
	handler, err := pricingapp.NewApprovePriceCardDraftHandler(drafts, rules, systemClock{})
	if err != nil {
		return transactionalPriceCardDraftApproval{}, fmt.Errorf("parcel-api: price card draft approval: %w", err)
	}
	return transactionalPriceCardDraftApproval{transactor: db.Transactor(), inner: handler}, nil
}

// buildPriceCardDraftPublication 装配 `/pricing-price-card-draft-publications` 的真编排：发布转交的是与登记口同一种装法的
// 价卡登记用例（真库价卡册），不另立第二条登记路径。
func buildPriceCardDraftPublication(db *bentopg.DB) (transactionalPriceCardDraftPublication, error) {
	drafts, err := pppostgres.NewPriceCardDrafts(db)
	if err != nil {
		return transactionalPriceCardDraftPublication{}, fmt.Errorf("parcel-api: price card draft register: %w", err)
	}
	catalog, err := pppostgres.NewPriceCards(db)
	if err != nil {
		return transactionalPriceCardDraftPublication{}, fmt.Errorf("parcel-api: price card catalog: %w", err)
	}
	registrar := pricingapp.NewRegisterPriceCardHandler(pricingapp.RegisterPriceCardDeps{Catalog: catalog})
	handler, err := pricingapp.NewPublishPriceCardDraftHandler(drafts, registrar, systemClock{})
	if err != nil {
		return transactionalPriceCardDraftPublication{}, fmt.Errorf("parcel-api: price card draft publication: %w", err)
	}
	return transactionalPriceCardDraftPublication{transactor: db.Transactor(), inner: handler}, nil
}
