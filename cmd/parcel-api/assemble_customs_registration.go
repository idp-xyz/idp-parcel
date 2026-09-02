package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// 关务四类配置登记的生产装配（ADR-0085，票 admin-write-faces/02 切片 02b）。四类分属
// 两个登记用例 handler（案件配置面与口岸路径面），传输层却按类各要一个 Registrar：
// 四个包装类型因此不合并，判据同价卡那两格——合并就得把四组 Handle 挤进一个类型再按
// 命令分派，装配测试会盖不住「某一格接错了编排」。
//
// 事务边界与 transactionalPriceCardRegistration 同源：登记册写口按框架合同无事务即拒，
// 一次调用一笔事务，登记与它的冲突判定读回因此看同一份快照；用例交回业务答案（含重放
// 与治理答案）时提交，返回错误时整笔回滚，端点按 ADR-0022 答「没形成答案」。

// caseConfigurationInTransaction 是案件配置面三格共用的事务壳（解释规则与门禁目录在
// 本文件，就绪与授权那几格属命令面不在本端点族内）。
func caseConfigurationInTransaction(
	ctx context.Context,
	transactor bentoapp.Transactor,
	register func(context.Context) (customsapp.CaseConfigurationOutcome, error),
) (customsapp.CaseConfigurationOutcome, error) {
	var outcome customsapp.CaseConfigurationOutcome
	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		registered, registerErr := register(txCtx)
		if registerErr != nil {
			return registerErr
		}
		outcome = registered
		return nil
	})
	if err != nil {
		return customsapp.CaseConfigurationOutcomeInvalid, err
	}
	return outcome, nil
}

type transactionalInterpretationRuleRegistration struct {
	transactor bentoapp.Transactor
	inner      *customsapp.RegisterCaseConfigurationHandler
}

var _ customshttp.InterpretationRuleRegistrar = transactionalInterpretationRuleRegistration{}

func (registration transactionalInterpretationRuleRegistration) Handle(
	ctx context.Context,
	command customsapp.RegisterInterpretationRuleCommand,
) (customsapp.CaseConfigurationOutcome, error) {
	return caseConfigurationInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (customsapp.CaseConfigurationOutcome, error) {
			return registration.inner.RegisterInterpretationRule(txCtx, command)
		})
}

type transactionalGateCatalogRegistration struct {
	transactor bentoapp.Transactor
	inner      *customsapp.RegisterCaseConfigurationHandler
}

var _ customshttp.GateCatalogRegistrar = transactionalGateCatalogRegistration{}

func (registration transactionalGateCatalogRegistration) Handle(
	ctx context.Context,
	command customsapp.RegisterGateCatalogCommand,
) (customsapp.CaseConfigurationOutcome, error) {
	return caseConfigurationInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (customsapp.CaseConfigurationOutcome, error) {
			return registration.inner.RegisterGateCatalog(txCtx, command)
		})
}

type transactionalCandidatePortRegistration struct {
	transactor bentoapp.Transactor
	inner      *customsapp.RegisterPortsPathsHandler
}

var _ customshttp.CandidatePortRegistrar = transactionalCandidatePortRegistration{}

func (registration transactionalCandidatePortRegistration) Handle(
	ctx context.Context,
	command customsapp.RegisterCandidatePortCommand,
) (customsapp.CaseConfigurationOutcome, error) {
	return caseConfigurationInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (customsapp.CaseConfigurationOutcome, error) {
			return registration.inner.RegisterCandidatePort(txCtx, command)
		})
}

type transactionalDeclarationPathRegistration struct {
	transactor bentoapp.Transactor
	inner      *customsapp.RegisterPortsPathsHandler
}

var _ customshttp.DeclarationPathRegistrar = transactionalDeclarationPathRegistration{}

func (registration transactionalDeclarationPathRegistration) Handle(
	ctx context.Context,
	command customsapp.RegisterDeclarationPathCommand,
) (customsapp.CaseConfigurationOutcome, error) {
	return caseConfigurationInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (customsapp.CaseConfigurationOutcome, error) {
			return registration.inner.RegisterDeclarationPath(txCtx, command)
		})
}

// customsRegistrationOrchestration 收拢四格，供装配点一次取回。分四个字段而不是一个
// handler：端点表按类各接一格，收成一格就得在装配行上现取字段，那正是要避免的「谁接
// 谁在装配点看不出来」。
type customsRegistrationOrchestration struct {
	interpretationRule transactionalInterpretationRuleRegistration
	gateCatalog        transactionalGateCatalogRegistration
	candidatePort      transactionalCandidatePortRegistration
	declarationPath    transactionalDeclarationPathRegistration
}

// buildCustomsRegistrationOrchestration 装配四个 `/customs-*-registrations` 的真编排。
// 接真不等墙降，判据同价卡首切片。
//
// 案件配置面的十只适配器全建而不只建解释规则与门禁那四只：`RegisterCaseConfigurationDeps`
// 的读口是冲突判定的一半（缺读口时「已在册」说不出是重放还是改内容），而半装的
// handler 会让本端点族之外的方法在被调时空指针崩溃——它今天走不到只是因为没接端点，
// 那不是一条守得住的边界。形照 `parcel-customs-register` 的同一份装配。
func buildCustomsRegistrationOrchestration(db *bentopg.DB) (customsRegistrationOrchestration, error) {
	none := customsRegistrationOrchestration{}

	readiness, err := ccpostgres.NewReadinessRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs readiness registry: %w", err)
	}
	readinessView, err := ccpostgres.NewReadinessView(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs readiness view: %w", err)
	}
	authorities, err := ccpostgres.NewSubmissionAuthorityRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs submission authority registry: %w", err)
	}
	authorityView, err := ccpostgres.NewSubmissionAuthorityView(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs submission authority view: %w", err)
	}
	rules, err := ccpostgres.NewInterpretationRuleRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs interpretation rule registry: %w", err)
	}
	ruleView, err := ccpostgres.NewInterpretationRuleView(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs interpretation rule view: %w", err)
	}
	obligations, err := ccpostgres.NewObligationInventoryRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs obligation registry: %w", err)
	}
	obligationView, err := ccpostgres.NewObligationInventoryView(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs obligation view: %w", err)
	}
	gates, err := ccpostgres.NewGateConditionRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs gate condition registry: %w", err)
	}
	gateView, err := ccpostgres.NewGateConditionView(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs gate condition view: %w", err)
	}
	portsPaths, err := ccpostgres.NewPortsPathsRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs ports paths registry: %w", err)
	}
	portsPathsView, err := ccpostgres.NewPortsPathsPointView(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs ports paths view: %w", err)
	}

	configurations := customsapp.NewRegisterCaseConfigurationHandler(customsapp.RegisterCaseConfigurationDeps{
		Readiness:      readiness,
		ReadinessView:  readinessView,
		Authorities:    authorities,
		AuthorityView:  authorityView,
		Rules:          rules,
		RuleView:       ruleView,
		Obligations:    obligations,
		ObligationView: obligationView,
		Gates:          gates,
		GateView:       gateView,
	})
	portsPathsHandler := customsapp.NewRegisterPortsPathsHandler(
		customsapp.RegisterPortsPathsDeps{Registry: portsPaths, View: portsPathsView})

	transactor := db.Transactor()
	return customsRegistrationOrchestration{
		interpretationRule: transactionalInterpretationRuleRegistration{transactor: transactor, inner: configurations},
		gateCatalog:        transactionalGateCatalogRegistration{transactor: transactor, inner: configurations},
		candidatePort:      transactionalCandidatePortRegistration{transactor: transactor, inner: portsPathsHandler},
		declarationPath:    transactionalDeclarationPathRegistration{transactor: transactor, inner: portsPathsHandler},
	}, nil
}
