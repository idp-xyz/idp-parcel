package application

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 监管凭证登记册的登记用例（票 mechanism-executor-triage/07 CC-a）。它没有编号 UC：
// RegulatoryCredential 是「监管凭证的不可变版本」，与口岸目录、建案要求规则、案件配置
// 五本同族，是登记面不是判断——第一个读它的判断是 UC-CC-003 步 7（JudgeCredentialApplicability）。
//
// 冲突判定同其余登记册：写口一律 ON CONFLICT DO NOTHING，同键已在册只答`已登记`；读回
// 在册版本按 CCC-1 形状比，同则`已存在`（重放），异则`内容冲突`（拒绝，绝不顶替）。凭证是不可变
// 版本，「同身份换期限/换持有人/换额度」在这里全是冲突——那是另一张凭证，走另一个身份登记。
// 事务由进程级入口给出。
//
// 领域构造把门（身份/机构/持有人/程序必备、期限有序、额度非负），本层不复述那几格，只把
// 构造拒绝翻成`未受理`。额度为零在领域约定为「来源未提供」并原样落册——不猜测补齐是
// 与监管决定数量维度同一条纪律，读口 Uses 把这层约定翻成显式的第二个返回值。

// RegisterCredentialCommand 携带一次凭证版本登记：RegulatoryCredential 的全部登记内容加
// 租户。Uses 为零即来源未提供次数额度。
type RegisterCredentialCommand struct {
	TenantID  domain.TenantID
	ID        domain.CredentialID
	Issuer    domain.RegulatoryAuthorityReference
	Holder    domain.CredentialHolderReference
	Procedure domain.CustomsProcedureReference
	ValidFrom time.Time
	ValidTo   time.Time
	Uses      int
}

// RegisterCredentialDeps 写口与读口两半，理由同其余登记册：冲突判定靠读回。
type RegisterCredentialDeps struct {
	Registry ports.CredentialRegistry
	View     ports.CredentialView
}

type RegisterCredentialHandler struct {
	deps RegisterCredentialDeps
}

func NewRegisterCredentialHandler(deps RegisterCredentialDeps) *RegisterCredentialHandler {
	return &RegisterCredentialHandler{deps: deps}
}

func (handler *RegisterCredentialHandler) Handle(
	ctx context.Context,
	command RegisterCredentialCommand,
) (CaseConfigurationOutcome, error) {
	if blankTenant(command.TenantID) {
		return ConfigurationNotAccepted, nil
	}
	credential, err := domain.RegisterCredential(
		command.ID, command.Issuer, command.Holder, command.Procedure,
		command.ValidFrom, command.ValidTo, command.Uses)
	if err != nil {
		return ConfigurationNotAccepted, nil
	}

	saved, err := handler.deps.Registry.RegisterCredential(ctx, command.TenantID, credential)
	if err != nil {
		return ConfigurationUndecided, nil
	}
	if saved == ports.CaseConfigurationRegistered {
		return ConfigurationRegistered, nil
	}

	existing, found, err := handler.deps.View.LoadCredential(ctx, command.TenantID, credential.ID())
	if err != nil || !found {
		return ConfigurationUndecided, nil
	}
	same, err := sameCredentialContent(existing, credential)
	if err != nil {
		return CaseConfigurationOutcomeInvalid, err
	}
	if !same {
		return ConfigurationContentConflict, nil
	}
	return ConfigurationExisting, nil
}

// sameCredentialContent 按 CCC-1 形状比两版凭证。册上不存摘要，两侧都从凭证本体现算，所以
// 没有无版本的旧摘要要认。形状把时刻写成 UTC：读回的时刻带库侧位置信息，按结构体相等比会把
// 同一时刻判成两个。
func sameCredentialContent(existing, requested domain.RegulatoryCredential) (bool, error) {
	_, existingDigest, err := domain.CanonicalizeCredentialRegistrationPayload(existing)
	if err != nil {
		return false, err
	}
	_, requestedDigest, err := domain.CanonicalizeCredentialRegistrationPayload(requested)
	if err != nil {
		return false, err
	}
	return existingDigest == requestedDigest, nil
}
