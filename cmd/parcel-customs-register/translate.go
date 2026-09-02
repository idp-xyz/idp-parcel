package main

import (
	"context"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/customscompliance/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// 本文件只把命令名分派到登记用例；「登记输入 JSON → 应用命令」的译装在
// `internal/customscompliance/adapters/registrationjson`。分家的理由不是文件太长：
// 在线登记端点（ADR-0085）收同源载荷，而 `internal` 导不进 `cmd`——译装留在这里，
// 那一侧就只能拄第二份，两份的严格性此后各自漂移。命令名与退出码是本入口自己的
// 表面，因此留在这里。

// 封闭命令表：前九个对齐案件配置登记用例的九个方法——就绪与授权各带撤销半边（撤销
// 是状态推进不是删除）、解释规则按（辖区，法定生效起点）登记版本（换版即登记更晚
// 起点的新版，前版终点随之落定）、义务与门禁各分目录与明细；随后是第六本册子
// （case-requirement，建案要求规则），随 cc-case-requirement-rule-registry 01 并入本口；
// 末两个是口岸目录与申报路径目录（票 admin-remainder-mechanism-batch/03），版本代数
// 同解释规则——起点入键、终点在换版时落定。
const (
	commandReadinessRegister  = "readiness-register"
	commandReadinessRevoke    = "readiness-revoke"
	commandAuthorityGrant     = "authority-grant"
	commandAuthorityRevoke    = "authority-revoke"
	commandInterpretationRule = "interpretation-rule"
	commandObligationCatalog  = "obligation-catalog"
	commandObligationItem     = "obligation-item"
	commandGateCatalog        = "gate-catalog"
	commandGateFinding        = "gate-finding"
	commandCaseRequirement    = "case-requirement"
	commandCandidatePort      = "candidate-port"
	commandDeclarationPath    = "declaration-path"
)

var allCommands = []string{
	commandReadinessRegister, commandReadinessRevoke,
	commandAuthorityGrant, commandAuthorityRevoke,
	commandInterpretationRule,
	commandObligationCatalog, commandObligationItem,
	commandGateCatalog, commandGateFinding,
	commandCaseRequirement,
	commandCandidatePort, commandDeclarationPath,
}

type dispatchFunc func(
	ctx context.Context,
	registrar registrar,
) (application.CaseConfigurationOutcome, error)

// commandFor 按命令译装输入，交回一个在事务内执行的调用。译装失败当场拒，不进事务
// ——用法错误与「登记与否未知」是两个退出码，让它进了事务就分不开了。
func commandFor(command string, raw []byte) (dispatchFunc, error) {
	switch command {
	case commandReadinessRegister:
		translated, err := registrationjson.ReadinessRegisterFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.configurations.RegisterReadiness(ctx, translated)
		}, nil
	case commandReadinessRevoke:
		translated, err := registrationjson.ReadinessRevokeFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.configurations.RevokeReadiness(ctx, translated)
		}, nil
	case commandAuthorityGrant:
		translated, err := registrationjson.AuthorityGrantFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.configurations.GrantSubmissionAuthority(ctx, translated)
		}, nil
	case commandAuthorityRevoke:
		translated, err := registrationjson.AuthorityRevokeFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.configurations.RevokeSubmissionAuthority(ctx, translated)
		}, nil
	case commandInterpretationRule:
		translated, err := registrationjson.InterpretationRuleFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.configurations.RegisterInterpretationRule(ctx, translated)
		}, nil
	case commandObligationCatalog:
		translated, err := registrationjson.ObligationCatalogFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.configurations.RegisterObligationCatalog(ctx, translated)
		}, nil
	case commandObligationItem:
		translated, err := registrationjson.ObligationItemFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.configurations.RegisterObligationItem(ctx, translated)
		}, nil
	case commandGateCatalog:
		translated, err := registrationjson.GateCatalogFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.configurations.RegisterGateCatalog(ctx, translated)
		}, nil
	case commandGateFinding:
		translated, err := registrationjson.GateFindingFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.configurations.RegisterGateFinding(ctx, translated)
		}, nil
	case commandCaseRequirement:
		translated, err := registrationjson.CaseRequirementFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.requirements.Handle(ctx, translated)
		}, nil
	case commandCandidatePort:
		translated, err := registrationjson.CandidatePortFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.portsPaths.RegisterCandidatePort(ctx, translated)
		}, nil
	case commandDeclarationPath:
		translated, err := registrationjson.DeclarationPathFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.portsPaths.RegisterDeclarationPath(ctx, translated)
		}, nil
	default:
		return nil, fmt.Errorf("未知登记命令 %q（支持 %s）", command, strings.Join(allCommands, " / "))
	}
}
