package identityhttp

import (
	"context"
	"errors"
	"net/http"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/accessidentity/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 作业设备在线登记与停用口：同挂操作者接入管理面（OPERATOR_ACCESS_ADMINISTRATION），租户只取自信封，
// 消费与受控批量命令行同一只 DeviceRegistrar。没有实体 PDA 的租户可在这里给浏览器作业工位登记设备号。
const (
	WorkDeviceRegistrationsPath = "/work-device-registrations"
	WorkDeviceDeactivationsPath = "/work-device-deactivations"
)

func WorkDeviceRegistrationsHandler(wired http.Handler) http.Handler {
	return writeHandlerOrUnconfigured(wired)
}

func WorkDeviceDeactivationsHandler(wired http.Handler) http.Handler {
	return writeHandlerOrUnconfigured(wired)
}

// NewRegisterWorkDeviceEndpoint 交回登记一台作业设备的在线口（POST /work-device-registrations）。
func NewRegisterWorkDeviceEndpoint(
	authenticator OperatorAccessAdministrationAuthenticator,
	transactor bentoapp.Transactor,
	registrar accessidentity.DeviceRegistrar,
) (http.Handler, error) {
	if authenticator == nil || transactor == nil || registrar == nil {
		return nil, errors.New("access identity http: work device registration needs an authenticator, a transactor and a registrar")
	}
	return deviceWriteEndpoint(authenticator, transactor, func(_ context.Context, admin OperatorAccessAdministrator, request *http.Request) (func(context.Context) (accessidentity.DeviceRegistrationOutcome, error), error) {
		var entry registrationjson.DeviceItem
		if err := decodeClosedDocument(request, &entry); err != nil {
			return nil, err
		}
		registration, err := registrationjson.DeviceFrom(admin.TenantID, entry)
		if err != nil {
			return nil, err
		}
		return func(txCtx context.Context) (accessidentity.DeviceRegistrationOutcome, error) {
			return registrar.RegisterDevice(txCtx, registration)
		}, nil
	}), nil
}

// NewDeactivateWorkDeviceEndpoint 交回停用一台作业设备的在线口（POST /work-device-deactivations）。
func NewDeactivateWorkDeviceEndpoint(
	authenticator OperatorAccessAdministrationAuthenticator,
	transactor bentoapp.Transactor,
	registrar accessidentity.DeviceRegistrar,
) (http.Handler, error) {
	if authenticator == nil || transactor == nil || registrar == nil {
		return nil, errors.New("access identity http: work device deactivation needs an authenticator, a transactor and a registrar")
	}
	return deviceWriteEndpoint(authenticator, transactor, func(_ context.Context, admin OperatorAccessAdministrator, request *http.Request) (func(context.Context) (accessidentity.DeviceRegistrationOutcome, error), error) {
		var entry registrationjson.DeviceDeactivationItem
		if err := decodeClosedDocument(request, &entry); err != nil {
			return nil, err
		}
		deactivation, err := registrationjson.DeviceDeactivationFrom(admin.TenantID, entry)
		if err != nil {
			return nil, err
		}
		return func(txCtx context.Context) (accessidentity.DeviceRegistrationOutcome, error) {
			return registrar.RegisterDeactivation(txCtx, deactivation)
		}, nil
	}), nil
}

func deviceWriteEndpoint(
	authenticator OperatorAccessAdministrationAuthenticator,
	transactor bentoapp.Transactor,
	translate func(context.Context, OperatorAccessAdministrator, *http.Request) (func(context.Context) (accessidentity.DeviceRegistrationOutcome, error), error),
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !allowMethod(response, request, http.MethodPost) {
			return
		}
		administrator, err := authenticator.AuthenticateAccessAdministration(request.Context(), httpapi.BearerToken(request))
		if err != nil {
			writeCatalogueProblem(response, err)
			return
		}
		register, err := translate(request.Context(), administrator, request)
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		var outcome accessidentity.DeviceRegistrationOutcome
		if err := transactor.WithinTransaction(request.Context(), func(txCtx context.Context) error {
			var registerErr error
			outcome, registerErr = register(txCtx)
			return registerErr
		}); err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		status := http.StatusOK
		if outcome == accessidentity.DeviceRegistrationRecorded {
			status = http.StatusCreated
		}
		writeJSON(response, status, registrationResponse{Outcome: outcome.String()})
	})
}
