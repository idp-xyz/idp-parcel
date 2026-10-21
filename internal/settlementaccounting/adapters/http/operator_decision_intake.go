package settlementhttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// OperatorDecision 是本包各口对应的运营决定种类，字面与操作者册的决定种类相同。
type OperatorDecision string

// OperatorDecisionExternalFundsFactAdoption 是外部资金事实采用（ADR-0236 决定一）：首版与更正同一种——更正是采用的一种，随首版的
// 认证与授予（ADR-0151 决定四的同一条判据）。
const OperatorDecisionExternalFundsFactAdoption OperatorDecision = "EXTERNAL_FUNDS_FACT_ADOPTION"

// OperatorIdentity 是一次运营决定出示认证出的身份：租户与做出决定的操作者（「发行方#sub」）。
type OperatorIdentity struct {
	Tenant   domain.TenantID
	Operator string
}

// OperatorDecisionAuthenticator 认证一次运营决定出示，交回出示者的身份。失败只交回本包的接入格，端点体按它们作答；从共享接入身份
// 能力译过来的那一层在 adapters/accessidentity。
type OperatorDecisionAuthenticator interface {
	AuthenticateOperatorDecision(ctx context.Context, bearerToken string, decision OperatorDecision) (OperatorIdentity, error)
}

// OperatorDecisionIntake 是操作者渠道「运营决定」能力面在外部资金事实采用与更正上的 Intake（ADR-0236 决定二）：租户的财务人员
// 照银行回单或支付系统的记录在管理台手工采用。与集成客户端族（IntegrationClientIntake）并存，编排、载荷形状与答案代数是同
// 一份——载荷仍由 registrationjson 那一份译装，载荷里自报租户即拒；两族只差谁在提交、凭什么授予。
//
// 提交操作者记成命令上的操作证据（`OPERATOR/<发行方>#<sub>`），只进审计。先认证、后读载荷：令牌不过的调用方看不到载荷校验的结果。
type OperatorDecisionIntake struct {
	authenticator OperatorDecisionAuthenticator
}

var (
	_ ExternalFundsFactRegistrationIntake           = (*OperatorDecisionIntake)(nil)
	_ ExternalFundsFactCorrectionRegistrationIntake = (*OperatorDecisionIntake)(nil)
)

func NewOperatorDecisionIntake(authenticator OperatorDecisionAuthenticator) (*OperatorDecisionIntake, error) {
	if authenticator == nil {
		return nil, errors.New("settlement http: operator decision intake needs an authenticator")
	}
	return &OperatorDecisionIntake{authenticator: authenticator}, nil
}

func (intake *OperatorDecisionIntake) IntakeExternalFundsFactRegistration(
	ctx context.Context,
	request *http.Request,
) (application.AdoptFundsFactCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionExternalFundsFactAdoption)
	if err != nil {
		return application.AdoptFundsFactCommand{}, err
	}
	payload, err := readSinglePayload(request.Body)
	if err != nil {
		return application.AdoptFundsFactCommand{}, err
	}
	command, err := registrationjson.ExternalFundsFactFromJSONForTenant(payload, operator.Tenant)
	if err != nil {
		return application.AdoptFundsFactCommand{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	command.OperationEvidence = operatorEvidence(operator)
	return command, nil
}

func (intake *OperatorDecisionIntake) IntakeExternalFundsFactCorrectionRegistration(
	ctx context.Context,
	request *http.Request,
) (application.CorrectFundsFactCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionExternalFundsFactAdoption)
	if err != nil {
		return application.CorrectFundsFactCommand{}, err
	}
	payload, err := readSinglePayload(request.Body)
	if err != nil {
		return application.CorrectFundsFactCommand{}, err
	}
	command, err := registrationjson.ExternalFundsFactCorrectionFromJSONForTenant(payload, operator.Tenant)
	if err != nil {
		return application.CorrectFundsFactCommand{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	command.OperationEvidence = operatorEvidence(operator)
	return command, nil
}

// operatorEvidence 带上渠道前缀：操作证据说的是经哪个渠道、由谁提交（与 PS 操作证据同一写法），单写「发行方#sub」读的人分不出
// 是哪一族交的。
func operatorEvidence(operator OperatorIdentity) string { return "OPERATOR/" + operator.Operator }
