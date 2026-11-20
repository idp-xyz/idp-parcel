package warehousehttp

import (
	"context"
	"errors"
	"io"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

// OperatorPreAdviceIntake 是入库预报代录在本包的 Intake。三个写口共用这一只：先认证（授予与准入范围），后读请求体。
type OperatorPreAdviceIntake struct {
	authenticator PreAdviceOnBehalfAuthenticator
}

func NewOperatorPreAdviceIntake(authenticator PreAdviceOnBehalfAuthenticator) (*OperatorPreAdviceIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: inbound pre-advice on-behalf intake needs an authenticator")
	}
	return &OperatorPreAdviceIntake{authenticator: authenticator}, nil
}

func (intake *OperatorPreAdviceIntake) IntakePreAdvice(
	ctx context.Context,
	request *http.Request,
) (application.AcceptPreAdviceCommand, error) {
	identity, err := intake.authenticate(ctx, request)
	if err != nil {
		return application.AcceptPreAdviceCommand{}, err
	}
	command, err := registrationjson.PreAdviceOnBehalfCommand(requestBody(request), identity.Tenant, identity.Registrant)
	if err != nil {
		return application.AcceptPreAdviceCommand{}, asMalformed(err)
	}
	return command, nil
}

func (intake *OperatorPreAdviceIntake) IntakePreAdviceNotice(
	ctx context.Context,
	request *http.Request,
) (application.UpdatePreAdviceNoticeCommand, error) {
	identity, err := intake.authenticate(ctx, request)
	if err != nil {
		return application.UpdatePreAdviceNoticeCommand{}, err
	}
	command, err := registrationjson.PreAdviceNoticeOnBehalfCommand(requestBody(request), identity.Tenant, identity.Registrant)
	if err != nil {
		return application.UpdatePreAdviceNoticeCommand{}, asMalformed(err)
	}
	return command, nil
}

func (intake *OperatorPreAdviceIntake) IntakePreAdviceWithdrawal(
	ctx context.Context,
	request *http.Request,
) (application.WithdrawPreAdviceCommand, error) {
	identity, err := intake.authenticate(ctx, request)
	if err != nil {
		return application.WithdrawPreAdviceCommand{}, err
	}
	command, err := registrationjson.PreAdviceWithdrawalOnBehalfCommand(requestBody(request), identity.Tenant, identity.Registrant)
	if err != nil {
		return application.WithdrawPreAdviceCommand{}, asMalformed(err)
	}
	return command, nil
}

func (intake *OperatorPreAdviceIntake) authenticate(ctx context.Context, request *http.Request) (OperatorIdentity, error) {
	return intake.authenticator.AuthenticatePreAdviceOnBehalf(ctx, httpapi.BearerToken(request))
}

func requestBody(request *http.Request) io.Reader {
	if request.Body == nil {
		return http.NoBody
	}
	return request.Body
}

func asMalformed(err error) error {
	var malformed *registrationjson.MalformedDocumentError
	if errors.As(err, &malformed) {
		return &malformedRequest{reason: malformed.Reason}
	}
	return err
}

var _ PreAdviceOnBehalfIntake = (*OperatorPreAdviceIntake)(nil)
