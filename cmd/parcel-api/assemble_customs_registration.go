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

// 关务配置登记写面的生产装配（ADR-0085，票 admin-write-faces/02 切片 02b；凭证与税费
// 付款协作 / 核对三册随票 sa-cc/07 步二进来）。各类分属不同的登记用例 handler，传输层
// 却按类各要一个 Registrar：包装类型因此不合并，判据同价卡那两格——合并就得把各组
// 方法挤进一个类型再按命令分派，装配测试会盖不住「某一格接错了编排」。
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

type transactionalCaseRequirementRegistration struct {
	transactor bentoapp.Transactor
	inner      *customsapp.RegisterCaseRequirementRuleHandler
}

var _ customshttp.CaseRequirementRegistrar = transactionalCaseRequirementRegistration{}

func (registration transactionalCaseRequirementRegistration) Handle(
	ctx context.Context,
	command customsapp.RegisterCaseRequirementRuleCommand,
) (customsapp.CaseConfigurationOutcome, error) {
	return caseConfigurationInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (customsapp.CaseConfigurationOutcome, error) {
			return registration.inner.Handle(txCtx, command)
		})
}

// 凭证册与税费付款协作 / 核对两册的事务壳（票 sa-cc/07 步二）。凭证册交回配置族的
// CaseConfigurationOutcome，壳照上面几格；协作与核对交回 DutyReconciliationResult，壳另立
// 一份 dutyReconciliationInTransaction——两族答案类型不同，硬套同一个泛型壳就得在装配层
// 引入类型参数，而这一层要的是「谁接谁一眼看得出」。
type transactionalRegulatoryCredentialRegistration struct {
	transactor bentoapp.Transactor
	inner      *customsapp.RegisterCredentialHandler
}

var _ customshttp.RegulatoryCredentialRegistrar = transactionalRegulatoryCredentialRegistration{}

func (registration transactionalRegulatoryCredentialRegistration) Handle(
	ctx context.Context,
	command customsapp.RegisterCredentialCommand,
) (customsapp.CaseConfigurationOutcome, error) {
	return caseConfigurationInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (customsapp.CaseConfigurationOutcome, error) {
			return registration.inner.Handle(txCtx, command)
		})
}

// dutyReconciliationInTransaction 是协作与核对两格共用的事务壳，边界判据同
// caseConfigurationInTransaction：用例交回业务答案（含重放、前置未齐与业务未决）时提交，
// 返回错误时整笔回滚。一次调用一笔事务，核对读两道前置与落核对因此看同一份快照。
func dutyReconciliationInTransaction(
	ctx context.Context,
	transactor bentoapp.Transactor,
	register func(context.Context) (customsapp.DutyReconciliationResult, error),
) (customsapp.DutyReconciliationResult, error) {
	var result customsapp.DutyReconciliationResult
	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		registered, registerErr := register(txCtx)
		if registerErr != nil {
			return registerErr
		}
		result = registered
		return nil
	})
	if err != nil {
		return customsapp.DutyReconciliationResult{}, err
	}
	return result, nil
}

// 协作与核对两个包装类型各自只暴露端点要的那一个用例方法：内层是同一个
// DutyPaymentReconciliationHandler，它本身就同时满足两个 Registrar 契约，但直接把它接进
// 端点表就绕过了事务壳——登记写口无环境事务即拒，形照 parcel-customs-register 的 execute。
type transactionalDutyCollaborationRegistration struct {
	transactor bentoapp.Transactor
	inner      *customsapp.DutyPaymentReconciliationHandler
}

var _ customshttp.DutyCollaborationRegistrar = transactionalDutyCollaborationRegistration{}

func (registration transactionalDutyCollaborationRegistration) FormCollaboration(
	ctx context.Context,
	command customsapp.FormDutyCollaborationCommand,
) (customsapp.DutyReconciliationResult, error) {
	return dutyReconciliationInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (customsapp.DutyReconciliationResult, error) {
			return registration.inner.FormCollaboration(txCtx, command)
		})
}

type transactionalDutyPaymentVerificationRegistration struct {
	transactor bentoapp.Transactor
	inner      *customsapp.DutyPaymentReconciliationHandler
}

var _ customshttp.DutyPaymentVerificationRegistrar = transactionalDutyPaymentVerificationRegistration{}

func (registration transactionalDutyPaymentVerificationRegistration) VerifyPayment(
	ctx context.Context,
	command customsapp.VerifyDutyPaymentCommand,
) (customsapp.DutyReconciliationResult, error) {
	return dutyReconciliationInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (customsapp.DutyReconciliationResult, error) {
			return registration.inner.VerifyPayment(txCtx, command)
		})
}

// customsRegistrationOrchestration 收拢各格，供装配点一次取回。分字段而不是一个
// handler：端点表按类各接一格，收成一格就得在装配行上现取字段，那正是要避免的「谁接
// 谁在装配点看不出来」。
type customsRegistrationOrchestration struct {
	interpretationRule      transactionalInterpretationRuleRegistration
	gateCatalog             transactionalGateCatalogRegistration
	candidatePort           transactionalCandidatePortRegistration
	declarationPath         transactionalDeclarationPathRegistration
	caseRequirement         transactionalCaseRequirementRegistration
	regulatoryCredential    transactionalRegulatoryCredentialRegistration
	dutyCollaboration       transactionalDutyCollaborationRegistration
	dutyPaymentVerification transactionalDutyPaymentVerificationRegistration
}

// buildCustomsRegistrationOrchestration 装配各 `/customs-*-registrations` 的真编排。
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
	requirements, err := ccpostgres.NewCaseRequirementRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs case requirement registry: %w", err)
	}
	requirementView, err := ccpostgres.NewCaseRequirementView(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs case requirement view: %w", err)
	}
	credentialRegistry, err := ccpostgres.NewCredentialRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs credential registry: %w", err)
	}
	credentialView, err := ccpostgres.NewCredentialView(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs credential view: %w", err)
	}
	// 协作事项、资金事实引用、付款核对三口在同一个适配器上（同一迁移的三张表）；编排
	// 的三个依赖都指它，资金事实那一口只被核对读前置——它的写入来自 SA 采用信封的消费者
	// （ADR-0137 Decision 四），本进程没有登它的端点。
	dutyReconciliation, err := ccpostgres.NewDutyPaymentReconciliation(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs duty payment reconciliation store: %w", err)
	}
	// 核对形成那一格同事务向 settlement-accounting 交信封（票 sa-cc/05）：交接口拿的是与
	// 登记册同一只 db，事务壳给的环境事务因此同时罩住核对行与 Outbox 意图——两者同生共死，
	// 形照 parcel-customs-register 的 buildRegistrar。
	outboxStore, err := outbox.NewStore(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs outbox store: %w", err)
	}
	verificationHandoff, err := ccpostgres.NewOutboxDutyPaymentVerificationHandoff(db, outboxStore, systemClock{})
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs duty payment verification handoff: %w", err)
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
		// 「税费付款」规则行的写读两半与目录 / 认定同一对适配器（票 sa-cc/06）；登记面（端点 / CLI）今天
		// 没有收这一格的口，接上只为让组合根不留 nil、后继登记面来接时一处可核。
		DutyRules:    gates,
		DutyRuleView: gateView,
	})
	portsPathsHandler := customsapp.NewRegisterPortsPathsHandler(
		customsapp.RegisterPortsPathsDeps{Registry: portsPaths, View: portsPathsView})
	requirementHandler := customsapp.NewRegisterCaseRequirementRuleHandler(
		customsapp.RegisterCaseRequirementRuleDeps{Rules: requirements, View: requirementView})
	credentialHandler := customsapp.NewRegisterCredentialHandler(
		customsapp.RegisterCredentialDeps{Registry: credentialRegistry, View: credentialView})
	// 构造门在构造期拒 nil 依赖（票 sa-cc/14 的纪律）：装配疏漏在进程启动那一刻炸出来，
	// 不等第一份协作事项到达。
	dutyReconciliationHandler, err := customsapp.NewDutyPaymentReconciliationHandler(
		customsapp.DutyPaymentReconciliationDeps{
			Collaborations: dutyReconciliation,
			Funds:          dutyReconciliation,
			Verifications:  dutyReconciliation,
			PayerRules:     dutyReconciliation,
			Handoff:        verificationHandoff,
			Clock:          systemClock{},
		})
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs duty payment reconciliation orchestration: %w", err)
	}

	transactor := db.Transactor()
	return customsRegistrationOrchestration{
		interpretationRule:      transactionalInterpretationRuleRegistration{transactor: transactor, inner: configurations},
		gateCatalog:             transactionalGateCatalogRegistration{transactor: transactor, inner: configurations},
		candidatePort:           transactionalCandidatePortRegistration{transactor: transactor, inner: portsPathsHandler},
		declarationPath:         transactionalDeclarationPathRegistration{transactor: transactor, inner: portsPathsHandler},
		caseRequirement:         transactionalCaseRequirementRegistration{transactor: transactor, inner: requirementHandler},
		regulatoryCredential:    transactionalRegulatoryCredentialRegistration{transactor: transactor, inner: credentialHandler},
		dutyCollaboration:       transactionalDutyCollaborationRegistration{transactor: transactor, inner: dutyReconciliationHandler},
		dutyPaymentVerification: transactionalDutyPaymentVerificationRegistration{transactor: transactor, inner: dutyReconciliationHandler},
	}, nil
}
