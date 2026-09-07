package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// transactionalMasterDocumentRegistrar 把总单首登与形成新版本各包进一笔事务，理由随
// transactionalCredentialRegistrar：登记册的写口按框架合同无事务即拒，事务边界归装配点；主表一版与子表
// 关联同笔落库，编排返回 error 时整笔回滚——不会留下一版没有关联或一组没有主行的关联。
type transactionalMasterDocumentRegistrar struct {
	transactor bentoapp.Transactor
	inner      *tfapp.RegisterMasterDocumentHandler
}

var _ tfhttp.MasterDocumentRegistrar = transactionalMasterDocumentRegistrar{}

func (registrar transactionalMasterDocumentRegistrar) Register(
	ctx context.Context,
	command tfapp.RegisterMasterDocumentCommand,
) (tfapp.RegisterMasterDocumentResult, error) {
	var result tfapp.RegisterMasterDocumentResult
	err := registrar.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := registrar.inner.Register(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.RegisterMasterDocumentResult{}, err
	}
	return result, nil
}

func (registrar transactionalMasterDocumentRegistrar) Revise(
	ctx context.Context,
	command tfapp.ReviseMasterDocumentCommand,
) (tfapp.RegisterMasterDocumentResult, error) {
	var result tfapp.RegisterMasterDocumentResult
	err := registrar.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := registrar.inner.Revise(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.RegisterMasterDocumentResult{}, err
	}
	return result, nil
}

// buildMasterDocumentRegistration 装配 `/transport-fulfillment-carrier-master-document-registrations` 与
// `/transport-fulfillment-carrier-master-document-revisions` 背后的真编排（ADR-0113 决定五）。缝只有两条——总单
// 登记册与时钟——全接真；没有意图交付：总单是本上下文自己的凭证事实，parcel-pricing 的主单级评价按需来引
// （ADR-0111），没有谁要在它变动时被通知。
func buildMasterDocumentRegistration(db *bentopg.DB) (tfhttp.MasterDocumentRegistrar, error) {
	documents, err := tfpostgres.NewMasterDocuments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: master documents: %w", err)
	}
	handler := tfapp.NewRegisterMasterDocumentHandler(tfapp.RegisterMasterDocumentDeps{
		Documents: documents,
		Clock:     systemClock{},
	})
	return transactionalMasterDocumentRegistrar{transactor: db.Transactor(), inner: handler}, nil
}
