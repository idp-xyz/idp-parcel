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
// 再两个是口岸目录与申报路径目录（票 admin-remainder-mechanism-batch/03），版本代数
// 同解释规则——起点入键、终点在换版时落定；末三个是监管凭证、税费付款协作事项、税费
// 付款核对（票 sa-cc/07）——凭证是不可变版本（同身份换任何一件都是冲突），协作事项按
// （范围，税费引用）一格一行，核对按三维加内容指纹逐版追加。外部资金事实引用**没有**
// 命令：它进本上下文只经 settlement-accounting 的采用信封（ADR-0137 决定四），CC 另开
// 人工补录口就是第二个铸造点。
//
// 用法文本与未知命令的错误文本都从 allCommands 生成，不各抄一遍（parcel-network-register
// supportedKinds 的教训）。
const (
	commandReadinessRegister       = "readiness-register"
	commandReadinessRevoke         = "readiness-revoke"
	commandAuthorityGrant          = "authority-grant"
	commandAuthorityRevoke         = "authority-revoke"
	commandInterpretationRule      = "interpretation-rule"
	commandObligationCatalog       = "obligation-catalog"
	commandObligationItem          = "obligation-item"
	commandGateCatalog             = "gate-catalog"
	commandGateFinding             = "gate-finding"
	commandCaseRequirement         = "case-requirement"
	commandCandidatePort           = "candidate-port"
	commandDeclarationPath         = "declaration-path"
	commandRegulatoryCredential    = "regulatory-credential"
	commandDutyCollaboration       = "duty-collaboration"
	commandDutyPaymentVerification = "duty-payment-verification"
)

var allCommands = []string{
	commandReadinessRegister, commandReadinessRevoke,
	commandAuthorityGrant, commandAuthorityRevoke,
	commandInterpretationRule,
	commandObligationCatalog, commandObligationItem,
	commandGateCatalog, commandGateFinding,
	commandCaseRequirement,
	commandCandidatePort, commandDeclarationPath,
	commandRegulatoryCredential, commandDutyCollaboration, commandDutyPaymentVerification,
}

// answer 是一个登记用例交回的封闭结果在本入口的译法。两族用例各有自己的结果代数
// （案件配置族的格、税费付款协作与核对族的格），各自成一型实现本接口——退出码
// 把它们按恢复动作归进同一张四格表，但格与格之间不互译：一族的「已存在」不借另一族
// 的常量，一族多出来的格（待关联、前置未齐）也不折进另一族已有的格里。
type answer interface {
	exit(command string) (string, int)
}

type dispatchFunc func(
	ctx context.Context,
	registrar registrar,
) (answer, error)

// configurationCall 是案件配置族用例的一次调用（案件配置、建案要求规则、口岸路径、
// 凭证等各 handler 都交回同一套案件配置族的格），由 configured 包成 dispatchFunc。
type configurationCall func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error)

func configured(call configurationCall) dispatchFunc {
	return func(ctx context.Context, registrar registrar) (answer, error) {
		outcome, err := call(ctx, registrar)
		return configurationResult{outcome: outcome}, err
	}
}

// dutyReconciliationCall 是税费付款协作与核对族用例的一次调用，由 reconciled 包成
// dispatchFunc。
type dutyReconciliationCall func(ctx context.Context, registrar registrar) (application.DutyReconciliationResult, error)

func reconciled(call dutyReconciliationCall) dispatchFunc {
	return func(ctx context.Context, registrar registrar) (answer, error) {
		result, err := call(ctx, registrar)
		return dutyReconciliationResult{result: result}, err
	}
}

// commandFor 按命令译装输入，交回一个在事务内执行的调用。译装失败当场拒，不进事务
// ——用法错误与「登记与否未知」是两个退出码，让它进了事务就分不开了。
func commandFor(command string, raw []byte) (dispatchFunc, error) {
	switch command {
	case commandReadinessRegister:
		translated, err := registrationjson.ReadinessRegisterFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return configured(func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.configurations.RegisterReadiness(ctx, translated)
		}), nil
	case commandReadinessRevoke:
		translated, err := registrationjson.ReadinessRevokeFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return configured(func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.configurations.RevokeReadiness(ctx, translated)
		}), nil
	case commandAuthorityGrant:
		translated, err := registrationjson.AuthorityGrantFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return configured(func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.configurations.GrantSubmissionAuthority(ctx, translated)
		}), nil
	case commandAuthorityRevoke:
		translated, err := registrationjson.AuthorityRevokeFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return configured(func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.configurations.RevokeSubmissionAuthority(ctx, translated)
		}), nil
	case commandInterpretationRule:
		translated, err := registrationjson.InterpretationRuleFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return configured(func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.configurations.RegisterInterpretationRule(ctx, translated)
		}), nil
	case commandObligationCatalog:
		translated, err := registrationjson.ObligationCatalogFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return configured(func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.configurations.RegisterObligationCatalog(ctx, translated)
		}), nil
	case commandObligationItem:
		translated, err := registrationjson.ObligationItemFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return configured(func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.configurations.RegisterObligationItem(ctx, translated)
		}), nil
	case commandGateCatalog:
		translated, err := registrationjson.GateCatalogFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return configured(func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.configurations.RegisterGateCatalog(ctx, translated)
		}), nil
	case commandGateFinding:
		translated, err := registrationjson.GateFindingFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return configured(func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.configurations.RegisterGateFinding(ctx, translated)
		}), nil
	case commandCaseRequirement:
		translated, err := registrationjson.CaseRequirementFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return configured(func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.requirements.Handle(ctx, translated)
		}), nil
	case commandCandidatePort:
		translated, err := registrationjson.CandidatePortFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return configured(func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.portsPaths.RegisterCandidatePort(ctx, translated)
		}), nil
	case commandDeclarationPath:
		translated, err := registrationjson.DeclarationPathFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return configured(func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.portsPaths.RegisterDeclarationPath(ctx, translated)
		}), nil
	case commandRegulatoryCredential:
		translated, err := registrationjson.RegulatoryCredentialFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return configured(func(ctx context.Context, registrar registrar) (application.CaseConfigurationOutcome, error) {
			return registrar.credentials.Handle(ctx, translated)
		}), nil
	case commandDutyCollaboration:
		translated, err := registrationjson.DutyCollaborationFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return reconciled(func(ctx context.Context, registrar registrar) (application.DutyReconciliationResult, error) {
			return registrar.dutyReconciliation.FormCollaboration(ctx, translated)
		}), nil
	case commandDutyPaymentVerification:
		translated, err := registrationjson.DutyPaymentVerificationFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return reconciled(func(ctx context.Context, registrar registrar) (application.DutyReconciliationResult, error) {
			return registrar.dutyReconciliation.VerifyPayment(ctx, translated)
		}), nil
	default:
		return nil, fmt.Errorf("未知登记命令 %q（支持 %s）", command, strings.Join(allCommands, " / "))
	}
}
