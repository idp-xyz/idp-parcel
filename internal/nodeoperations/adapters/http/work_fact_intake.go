package nodeopshttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

const (
	headerWorkDevice    = "Parcel-Device-Id"
	headerWorkScopeKind = "Parcel-Work-Scope-Kind"
	headerWorkScopeRef  = "Parcel-Work-Scope-Reference"
)

var (
	ErrOperatorCredentialRejected    = errors.New("node operations http: operator credential rejected")
	ErrOperatorNotGranted            = errors.New("node operations http: operator holds no grant for the capability face, scope or decision this endpoint needs")
	ErrDeviceNotInUse                = errors.New("node operations http: device is not registered for this work scope or has been deactivated")
	ErrIdentityDependencyUnavailable = errors.New("node operations http: identity dependency unavailable")
)

const (
	codeOperatorCredentialRejected    = "OPERATOR_CREDENTIAL_REJECTED"
	codeOperatorNotGranted            = "OPERATOR_NOT_GRANTED"
	codeDeviceNotInUse                = "DEVICE_NOT_IN_USE"
	codeIdentityDependencyUnavailable = "IDENTITY_DEPENDENCY_UNAVAILABLE"
)

// WorkFactAuthenticator 以作业事实登记能力面铸信封，交回租户与节点。节点收寄只认种类 NODE。
type WorkFactAuthenticator interface {
	AuthenticateNodeReception(ctx context.Context, bearerToken, deviceID, scopeKind, scopeReference string) (domain.TenantID, domain.NodeReference, error)
}

// WorkFactReceptionIntake 是节点收寄口的操作者渠道 Intake（ADR-0149 决定二）。
type WorkFactReceptionIntake struct {
	authenticator WorkFactAuthenticator
}

func NewWorkFactReceptionIntake(authenticator WorkFactAuthenticator) (*WorkFactReceptionIntake, error) {
	if authenticator == nil {
		return nil, errors.New("node operations http: work fact reception intake needs an authenticator")
	}
	return &WorkFactReceptionIntake{authenticator: authenticator}, nil
}

var _ ReceptionIntake = (*WorkFactReceptionIntake)(nil)

func (intake *WorkFactReceptionIntake) IntakeReception(ctx context.Context, request *http.Request) (application.ReceiveDeliveredUnitCommand, error) {
	none := application.ReceiveDeliveredUnitCommand{}
	device := strings.TrimSpace(request.Header.Get(headerWorkDevice))
	kind := strings.TrimSpace(request.Header.Get(headerWorkScopeKind))
	reference := strings.TrimSpace(request.Header.Get(headerWorkScopeRef))
	if device == "" || kind == "" || reference == "" {
		return none, fmt.Errorf("%w: %s, %s and %s are required", ErrMalformedRequest, headerWorkDevice, headerWorkScopeKind, headerWorkScopeRef)
	}
	if kind != "NODE" {
		return none, fmt.Errorf("%w: node reception requires Parcel-Work-Scope-Kind NODE", ErrMalformedRequest)
	}
	tenant, node, err := intake.authenticator.AuthenticateNodeReception(ctx, httpapi.BearerToken(request), device, kind, reference)
	if err != nil {
		return none, err
	}
	return receptionCommand(tenant, node, request)
}
