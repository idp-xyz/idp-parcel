package application

import (
	"context"
	"strings"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 第六本册子（case_requirement_rule）的登记用例。它挡的是 establish_customs_case 那堵
// `EstablishCaseUndecided` 墙——建案链的第一步，不在 W13 的两堵之内，因此不并入
// RegisterCaseConfigurationHandler 那五本（那票按「五类配置」收窄执行完毕，本用例
// 随 cc-case-requirement-rule-registry 01 另立）。
//
// 冲突判定同五本：写口一律 ON CONFLICT DO NOTHING，同键已在册只答`已登记`；读回既有
// 登记逐字段比，同则`已存在`（重放），异则`内容冲突`（拒绝，绝不顶替）。事务由进程级
// 入口给出。
//
// 受理门里依据对「要求」与「不要求」都必填：说不出依据的「不要求建案」与「规则没
// 登记」分不开，而这两者的续办动作完全不同（前者照常推进，后者等实例参数）。

// RegisterCaseRequirementRuleCommand 携带一次建案要求规则登记。键是监管范围四维
// （租户、辖区、方向、程序），内容是判断两件（要不要建案、依据）。
type RegisterCaseRequirementRuleCommand struct {
	TenantID     domain.TenantID
	Jurisdiction domain.RegulatoryJurisdictionReference
	Direction    domain.ManifestDirection
	Procedure    domain.CustomsProcedureReference
	Required     bool
	Basis        string
}

// RegisterCaseRequirementRuleDeps 写口与读口两半，理由同五本：冲突判定靠读回。
type RegisterCaseRequirementRuleDeps struct {
	Rules ports.CaseRequirementRegistry
	View  ports.CaseRequirementView
}

type RegisterCaseRequirementRuleHandler struct {
	deps RegisterCaseRequirementRuleDeps
}

func NewRegisterCaseRequirementRuleHandler(
	deps RegisterCaseRequirementRuleDeps,
) *RegisterCaseRequirementRuleHandler {
	return &RegisterCaseRequirementRuleHandler{deps: deps}
}

func (handler *RegisterCaseRequirementRuleHandler) Handle(
	ctx context.Context,
	command RegisterCaseRequirementRuleCommand,
) (CaseConfigurationOutcome, error) {
	if blankTenant(command.TenantID) ||
		command.Jurisdiction.String() == "" ||
		command.Direction.String() == "" ||
		command.Procedure.String() == "" ||
		strings.TrimSpace(command.Basis) == "" {
		return ConfigurationNotAccepted, nil
	}
	judgment := ports.CaseRequirementJudgment{Required: command.Required, Basis: command.Basis}

	saved, err := handler.deps.Rules.RegisterCaseRequirementRule(
		ctx, command.TenantID, command.Jurisdiction, command.Direction, command.Procedure, judgment)
	if err != nil {
		return ConfigurationUndecided, nil
	}
	if saved == ports.CaseConfigurationRegistered {
		return ConfigurationRegistered, nil
	}

	existing, found, err := handler.deps.View.JudgeCaseRequirement(
		ctx, command.TenantID, command.Jurisdiction, command.Direction, command.Procedure)
	if err != nil || !found {
		return ConfigurationUndecided, nil
	}
	if existing != judgment {
		return ConfigurationContentConflict, nil
	}
	return ConfigurationExisting, nil
}
