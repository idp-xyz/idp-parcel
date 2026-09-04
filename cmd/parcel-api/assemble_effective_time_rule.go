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

// transactionalEffectiveTimeRuleRegistrar 把一次规则登记包进一笔事务，理由随
// transactionalCredentialRegistrar：目录的写口按框架合同无事务即拒，事务边界归装配点；编排返回
// error 时整笔回滚。
type transactionalEffectiveTimeRuleRegistrar struct {
	transactor bentoapp.Transactor
	inner      *tfapp.RegisterEffectiveTimeRuleHandler
}

var _ tfhttp.EffectiveTimeRuleRegistrar = transactionalEffectiveTimeRuleRegistrar{}

func (registrar transactionalEffectiveTimeRuleRegistrar) Register(
	ctx context.Context,
	command tfapp.RegisterEffectiveTimeRuleCommand,
) (tfapp.RegisterEffectiveTimeRuleResult, error) {
	var result tfapp.RegisterEffectiveTimeRuleResult
	err := registrar.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := registrar.inner.Register(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.RegisterEffectiveTimeRuleResult{}, err
	}
	return result, nil
}

// buildEffectiveTimeRuleRegistration 装配 `/transport-fulfillment-effective-time-rule-registrations` 背后的
// 真编排（票 label-channel/19）。缝只有两条——规则目录与时钟——全接真；没有意图交付：规则是本上下文
// 自己的实例参数，收编执行器按需来问，没有谁要在它变动时被通知。
//
// 同一只 EffectiveTimeRuleCatalogue 也是 ports.EffectiveTimeRules 的生产实现（ADR-0102 决定三的第二种
// 来源）；收编执行器的生产入口随第一家真源的拉取节拍票立，到那一步在那处装配点把它按规则口注入，
// 本函数不替它预留——与凭证登记那条的分工同形。
func buildEffectiveTimeRuleRegistration(db *bentopg.DB) (tfhttp.EffectiveTimeRuleRegistrar, error) {
	rules, err := tfpostgres.NewEffectiveTimeRuleCatalogue(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: effective time rule catalogue: %w", err)
	}
	handler := tfapp.NewRegisterEffectiveTimeRuleHandler(tfapp.RegisterEffectiveTimeRuleDeps{
		Rules: rules,
		Clock: systemClock{},
	})
	return transactionalEffectiveTimeRuleRegistrar{transactor: db.Transactor(), inner: handler}, nil
}
