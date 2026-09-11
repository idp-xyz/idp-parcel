package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	visibilityapp "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

// VE 配置登记的生产装配（ADR-0085，票 admin-write-faces/02 切片 02d；异常披露规则与
// 冲突信号规则两册随票 ve-disclosure-policy-view/02 步二加入）。各类共用一个登记用例
// 对象（application.CatalogRegistration），传输层却按类各要一个 Registrar，且各 Registrar
// 的方法同名 `Handle` 而命令类型互不相同——一个类型实现不了全部，逐类包装因此不是重复
// 而是必需。
//
// 事务边界与 transactionalPriceCardRegistration 同源：一次调用一笔事务，用例交回业务
// 答案时提交，返回错误时整笔回滚，端点按 ADR-0022 答「没形成答案」。

// veCatalogInTransaction 是各格共用的事务壳，各 Handle 只差交给用例的那一句。
func veCatalogInTransaction(
	ctx context.Context,
	transactor bentoapp.Transactor,
	register func(context.Context) (visibilityapp.RegisterCatalogResult, error),
) (visibilityapp.RegisterCatalogResult, error) {
	var result visibilityapp.RegisterCatalogResult
	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		registered, registerErr := register(txCtx)
		if registerErr != nil {
			return registerErr
		}
		result = registered
		return nil
	})
	if err != nil {
		return visibilityapp.RegisterCatalogResult{}, err
	}
	return result, nil
}

type transactionalMilestoneMappingRegistration struct {
	transactor bentoapp.Transactor
	inner      *visibilityapp.CatalogRegistration
}

var _ visibilityhttp.MilestoneMappingRegistrar = transactionalMilestoneMappingRegistration{}

func (registration transactionalMilestoneMappingRegistration) Handle(
	ctx context.Context,
	command visibilityapp.RegisterMilestoneMappingCommand,
) (visibilityapp.RegisterCatalogResult, error) {
	return veCatalogInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (visibilityapp.RegisterCatalogResult, error) {
			return registration.inner.RegisterMilestoneMapping(txCtx, command)
		})
}

type transactionalTriageRulesRegistration struct {
	transactor bentoapp.Transactor
	inner      *visibilityapp.CatalogRegistration
}

var _ visibilityhttp.TriageRulesRegistrar = transactionalTriageRulesRegistration{}

func (registration transactionalTriageRulesRegistration) Handle(
	ctx context.Context,
	command visibilityapp.RegisterTriageRulesCommand,
) (visibilityapp.RegisterCatalogResult, error) {
	return veCatalogInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (visibilityapp.RegisterCatalogResult, error) {
			return registration.inner.RegisterTriageRules(txCtx, command)
		})
}

type transactionalNotificationPolicyRegistration struct {
	transactor bentoapp.Transactor
	inner      *visibilityapp.CatalogRegistration
}

var _ visibilityhttp.NotificationPolicyRegistrar = transactionalNotificationPolicyRegistration{}

func (registration transactionalNotificationPolicyRegistration) Handle(
	ctx context.Context,
	command visibilityapp.RegisterNotificationPolicyCommand,
) (visibilityapp.RegisterCatalogResult, error) {
	return veCatalogInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (visibilityapp.RegisterCatalogResult, error) {
			return registration.inner.RegisterNotificationPolicy(txCtx, command)
		})
}

type transactionalClaimEligibilityRegistration struct {
	transactor bentoapp.Transactor
	inner      *visibilityapp.CatalogRegistration
}

var _ visibilityhttp.ClaimEligibilityRegistrar = transactionalClaimEligibilityRegistration{}

func (registration transactionalClaimEligibilityRegistration) Handle(
	ctx context.Context,
	command visibilityapp.RegisterClaimEligibilityCommand,
) (visibilityapp.RegisterCatalogResult, error) {
	return veCatalogInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (visibilityapp.RegisterCatalogResult, error) {
			return registration.inner.RegisterClaimEligibility(txCtx, command)
		})
}

type transactionalClaimAuthorizationRegistration struct {
	transactor bentoapp.Transactor
	inner      *visibilityapp.CatalogRegistration
}

var _ visibilityhttp.ClaimAuthorizationRegistrar = transactionalClaimAuthorizationRegistration{}

func (registration transactionalClaimAuthorizationRegistration) Handle(
	ctx context.Context,
	command visibilityapp.RegisterClaimAuthorizationCommand,
) (visibilityapp.RegisterCatalogResult, error) {
	return veCatalogInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (visibilityapp.RegisterCatalogResult, error) {
			return registration.inner.RegisterClaimAuthorization(txCtx, command)
		})
}

type transactionalDisclosurePolicyRegistration struct {
	transactor bentoapp.Transactor
	inner      *visibilityapp.CatalogRegistration
}

var _ visibilityhttp.DisclosurePolicyRegistrar = transactionalDisclosurePolicyRegistration{}

func (registration transactionalDisclosurePolicyRegistration) Handle(
	ctx context.Context,
	command visibilityapp.RegisterDisclosurePolicyCommand,
) (visibilityapp.RegisterCatalogResult, error) {
	return veCatalogInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (visibilityapp.RegisterCatalogResult, error) {
			return registration.inner.RegisterDisclosurePolicy(txCtx, command)
		})
}

type transactionalExceptionDisclosureRulesRegistration struct {
	transactor bentoapp.Transactor
	inner      *visibilityapp.CatalogRegistration
}

var _ visibilityhttp.ExceptionDisclosureRulesRegistrar = transactionalExceptionDisclosureRulesRegistration{}

func (registration transactionalExceptionDisclosureRulesRegistration) Handle(
	ctx context.Context,
	command visibilityapp.RegisterExceptionDisclosureRulesCommand,
) (visibilityapp.RegisterCatalogResult, error) {
	return veCatalogInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (visibilityapp.RegisterCatalogResult, error) {
			return registration.inner.RegisterExceptionDisclosureRules(txCtx, command)
		})
}

type transactionalConflictSignalRuleRegistration struct {
	transactor bentoapp.Transactor
	inner      *visibilityapp.CatalogRegistration
}

var _ visibilityhttp.ConflictSignalRuleRegistrar = transactionalConflictSignalRuleRegistration{}

func (registration transactionalConflictSignalRuleRegistration) Handle(
	ctx context.Context,
	command visibilityapp.RegisterConflictSignalRuleCommand,
) (visibilityapp.RegisterCatalogResult, error) {
	return veCatalogInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (visibilityapp.RegisterCatalogResult, error) {
			return registration.inner.RegisterConflictSignalRule(txCtx, command)
		})
}

// veRegistrationOrchestration 收拢各格，判据同 customsRegistrationOrchestration。
type veRegistrationOrchestration struct {
	milestoneMapping         transactionalMilestoneMappingRegistration
	triageRules              transactionalTriageRulesRegistration
	notificationPolicy       transactionalNotificationPolicyRegistration
	claimEligibility         transactionalClaimEligibilityRegistration
	claimAuthorization       transactionalClaimAuthorizationRegistration
	disclosurePolicy         transactionalDisclosurePolicyRegistration
	exceptionDisclosureRules transactionalExceptionDisclosureRulesRegistration
	conflictSignalRule       transactionalConflictSignalRuleRegistration
}

// buildVERegistrationOrchestration 装配全部 `/visibility-catalogue-*-registrations`
// 的真编排。接真不等墙降，判据同价卡首切片；形照 `parcel-ve-register` 的同一份装配。
func buildVERegistrationOrchestration(db *bentopg.DB) (veRegistrationOrchestration, error) {
	none := veRegistrationOrchestration{}

	registrar, err := vepostgres.NewCatalogRegistrar(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: visibility catalog registrar: %w", err)
	}
	catalogs, err := visibilityapp.NewCatalogRegistration(registrar)
	if err != nil {
		return none, fmt.Errorf("parcel-api: visibility catalog registration: %w", err)
	}

	transactor := db.Transactor()
	return veRegistrationOrchestration{
		milestoneMapping:   transactionalMilestoneMappingRegistration{transactor: transactor, inner: catalogs},
		triageRules:        transactionalTriageRulesRegistration{transactor: transactor, inner: catalogs},
		notificationPolicy: transactionalNotificationPolicyRegistration{transactor: transactor, inner: catalogs},
		claimEligibility:   transactionalClaimEligibilityRegistration{transactor: transactor, inner: catalogs},
		claimAuthorization: transactionalClaimAuthorizationRegistration{transactor: transactor, inner: catalogs},
		disclosurePolicy:   transactionalDisclosurePolicyRegistration{transactor: transactor, inner: catalogs},
		exceptionDisclosureRules: transactionalExceptionDisclosureRulesRegistration{
			transactor: transactor, inner: catalogs,
		},
		conflictSignalRule: transactionalConflictSignalRuleRegistration{transactor: transactor, inner: catalogs},
	}, nil
}
