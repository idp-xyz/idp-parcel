package warehousehttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

// OperatorGoodsIntake 是商品代录在本包的 Intake。先认证（授予与准入范围），后读请求体；租户与登记责任方填认证结果。
type OperatorGoodsIntake struct {
	authenticator GoodsOnBehalfAuthenticator
}

func NewOperatorGoodsIntake(authenticator GoodsOnBehalfAuthenticator) (*OperatorGoodsIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: goods on-behalf intake needs an authenticator")
	}
	return &OperatorGoodsIntake{authenticator: authenticator}, nil
}

func (intake *OperatorGoodsIntake) IntakeGoodsOnBehalf(
	ctx context.Context,
	request *http.Request,
) (application.RegisterGoodsCommand, error) {
	identity, err := intake.authenticator.AuthenticateGoodsOnBehalf(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.RegisterGoodsCommand{}, err
	}
	body := request.Body
	if body == nil {
		body = http.NoBody
	}
	command, err := registrationjson.GoodsOnBehalfCommand(body, identity.Tenant, identity.Registrant)
	if err != nil {
		var malformed *registrationjson.MalformedDocumentError
		if errors.As(err, &malformed) {
			return application.RegisterGoodsCommand{}, &malformedRequest{reason: malformed.Reason}
		}
		return application.RegisterGoodsCommand{}, err
	}
	return command, nil
}

var _ GoodsOnBehalfIntake = (*OperatorGoodsIntake)(nil)
