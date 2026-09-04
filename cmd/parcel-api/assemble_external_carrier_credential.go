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

// transactionalCredentialRegistrar 把凭证首登与改变适用关系各包进一笔事务，理由随
// transactionalDelivery：登记册的写口按框架合同无事务即拒，事务边界归装配点；编排返回 error 时
// 整笔回滚。
type transactionalCredentialRegistrar struct {
	transactor bentoapp.Transactor
	inner      *tfapp.RegisterExternalCarrierCredentialHandler
}

var _ tfhttp.CredentialRegistrar = transactionalCredentialRegistrar{}

func (registrar transactionalCredentialRegistrar) Register(
	ctx context.Context,
	command tfapp.RegisterExternalCarrierCredentialCommand,
) (tfapp.RegisterExternalCarrierCredentialResult, error) {
	var result tfapp.RegisterExternalCarrierCredentialResult
	err := registrar.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := registrar.inner.Register(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.RegisterExternalCarrierCredentialResult{}, err
	}
	return result, nil
}

func (registrar transactionalCredentialRegistrar) ChangeApplicability(
	ctx context.Context,
	command tfapp.ChangeCredentialApplicabilityCommand,
) (tfapp.RegisterExternalCarrierCredentialResult, error) {
	var result tfapp.RegisterExternalCarrierCredentialResult
	err := registrar.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := registrar.inner.ChangeApplicability(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.RegisterExternalCarrierCredentialResult{}, err
	}
	return result, nil
}

// buildExternalCarrierCredentialRegistration 装配 `/transport-fulfillment-external-carrier-credential-registrations`
// 与 `/transport-fulfillment-external-carrier-credential-applicability-changes` 背后的真编排（票
// label-channel/18）。缝只有两条——凭证登记册与时钟——全接真；没有意图交付：凭证是本上下文自己的
// 实例参数，收编执行器按需来问，没有谁要在它变动时被通知。
//
// 同一只 ExternalCarrierCredentials 也是 ports.ExternalCarrierCredentialResolver 的生产实现；收编执行器
// （AdoptTrackingMaterialHandler）的生产入口随第一家真源的拉取节拍票立（label-channel/16 完成记录第 1 格），
// 到那一步在那处装配点把它按解析口注入，本函数不替它预留。
func buildExternalCarrierCredentialRegistration(db *bentopg.DB) (tfhttp.CredentialRegistrar, error) {
	credentials, err := tfpostgres.NewExternalCarrierCredentials(db, systemClock{})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: external carrier credentials: %w", err)
	}
	handler := tfapp.NewRegisterExternalCarrierCredentialHandler(tfapp.RegisterExternalCarrierCredentialDeps{
		Credentials: credentials,
		Clock:       systemClock{},
	})
	return transactionalCredentialRegistrar{transactor: db.Transactor(), inner: handler}, nil
}
