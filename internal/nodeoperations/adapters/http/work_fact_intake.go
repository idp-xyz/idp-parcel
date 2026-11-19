package nodeopshttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
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
	tenant, node, err := intake.authenticateNode(ctx, request, "node reception requires "+httpapi.HeaderWorkScopeKind+" NODE")
	if err != nil {
		return none, err
	}
	return receptionCommand(tenant, node, request)
}

// authenticateNode 读作业事实面的三个头并认证出租户与节点。各口说的是各自的事，种类不对时的错误文本由调用方给。
func (intake *WorkFactReceptionIntake) authenticateNode(ctx context.Context, request *http.Request, notNode string) (domain.TenantID, domain.NodeReference, error) {
	device, kind, reference, err := nodeWorkScope(request, notNode)
	if err != nil {
		return domain.TenantID{}, domain.NodeReference{}, err
	}
	return intake.authenticator.AuthenticateNodeReception(ctx, httpapi.BearerToken(request), device, kind, reference)
}

// nodeWorkScope 读出作业事实面的三个头：设备、作业范围种类与作业范围引用。三个头缺一个，或种类不是 NODE，都拒成载荷不成形；
// 各口（收寄、测量、交接单、执行事实）的认证器不同，这一步是它们共有的前置，种类不对时的错误文本由调用方给（notNode），
// 免得合并之后各口的服务端日志说成同一句。头名与读法（去空白、缺一即不齐）在 httpapi.ReadWorkFactHeaders，这里只认本上下文的 NODE 种类。
func nodeWorkScope(request *http.Request, notNode string) (device, kind, reference string, err error) {
	device, kind, reference, complete := httpapi.ReadWorkFactHeaders(request)
	if !complete {
		return "", "", "", fmt.Errorf("%w: %s, %s and %s are required", ErrMalformedRequest,
			httpapi.HeaderWorkDevice, httpapi.HeaderWorkScopeKind, httpapi.HeaderWorkScopeReference)
	}
	if kind != "NODE" {
		return "", "", "", fmt.Errorf("%w: %s", ErrMalformedRequest, notNode)
	}
	return device, kind, reference, nil
}
