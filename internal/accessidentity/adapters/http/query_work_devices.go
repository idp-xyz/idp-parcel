package identityhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// WorkDeviceCataloguePath 是作业设备册查阅的路径。parcel-api 端点表挂这一行。
const WorkDeviceCataloguePath = "/work-devices"

// outcomeWorkDevicesListed 是本端点唯一的业务成格。空册也是这一格：空列表不是未配置。
const outcomeWorkDevicesListed = "WORK_DEVICES_LISTED"

const (
	codeMethodNotAllowed              = "METHOD_NOT_ALLOWED"
	codeAccessChannelNotConfigured    = "ACCESS_CHANNEL_NOT_CONFIGURED"
	codeOperatorCredentialRejected    = "OPERATOR_CREDENTIAL_REJECTED"
	codeOperatorNotGranted            = "OPERATOR_NOT_GRANTED"
	codeIdentityDependencyUnavailable = "IDENTITY_DEPENDENCY_UNAVAILABLE"
	codeNoAnswerFormed                = "NO_ANSWER_FORMED"
	codeIntakeFailed                  = "INTAKE_FAILED"
)

// operatorMinter 是查阅认证对铸造的最小依赖。OperatorMinter 满足它。
type operatorMinter interface {
	MintOperator(
		ctx context.Context,
		credential accessidentity.OperatorCredential,
		request accessidentity.OperatorRequest,
	) (accessidentity.OperatorEnvelope, error)
}

// OperatorCatalogueAuthenticator 认证一次主数据与运营查阅读出示，交回信封上的租户。
type OperatorCatalogueAuthenticator interface {
	AuthenticateCatalogueRead(ctx context.Context, bearerToken string) (string, error)
}

// catalogueAuthenticator 以主数据与运营查阅读铸信封，不带准入要求：查阅不形成作业事实。
type catalogueAuthenticator struct {
	minter operatorMinter
}

// NewOperatorCatalogueAuthenticator 的 minter 不许缺。
func NewOperatorCatalogueAuthenticator(minter operatorMinter) (OperatorCatalogueAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("access identity http: work device catalogue authenticator needs a minter")
	}
	return catalogueAuthenticator{minter: minter}, nil
}

// AuthenticateCatalogueRead 不把请求里的租户交进铸造：租户只取自信封上的绑定。
func (authenticator catalogueAuthenticator) AuthenticateCatalogueRead(ctx context.Context, bearerToken string) (string, error) {
	envelope, err := authenticator.minter.MintOperator(ctx, accessidentity.NewOperatorCredential(bearerToken),
		accessidentity.OperatorRequest{Face: accessidentity.CapabilityMasterDataAndOperationsRead})
	if err != nil {
		return "", err
	}
	if !envelope.Minted() || envelope.TenantID() == "" {
		return "", errors.New("access identity http: minted envelope has no tenant")
	}
	return envelope.TenantID(), nil
}

// WorkDeviceCatalogueHandler 把已装好的查阅处理器交回端点表。没装时答未配置：装配测试不建铸造器，
// 仍要这一行在表上，答复与发行方未设同一格。生产装配传 NewQueryWorkDevicesEndpoint 的结果。
func WorkDeviceCatalogueHandler(wired http.Handler) http.Handler {
	if wired == nil {
		return UnconfiguredWorkDeviceCatalogue()
	}
	return wired
}

// UnconfiguredWorkDeviceCatalogue 在这一口还没接上操作者铸造器时答未配置。
func UnconfiguredWorkDeviceCatalogue() http.Handler {
	return unconfiguredOperatorRead()
}

// unconfiguredOperatorRead 是本包各查阅口没接铸造器时的同一个答复：只收 GET，答未配置，与发行方未设同一格。
func unconfiguredOperatorRead() http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
	})
}

// NewQueryWorkDevicesEndpoint 交回作业设备册查阅的 HTTP 入口。只接受 GET。租户只来自认证结果；查询参数在认证之后按
// accessidentity.DeviceCataloguePages 解码，页大小是 operatorCatalogueReadLimit。
func NewQueryWorkDevicesEndpoint(
	authenticator OperatorCatalogueAuthenticator,
	catalogue accessidentity.DeviceCatalogue,
) (http.Handler, error) {
	if authenticator == nil || catalogue == nil {
		return nil, errors.New("access identity http: work device catalogue endpoint needs an authenticator and a catalogue")
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		tenant, err := authenticator.AuthenticateCatalogueRead(request.Context(), httpapi.BearerToken(request))
		if err != nil {
			writeCatalogueProblem(response, err)
			return
		}
		query, err := accessidentity.DeviceCataloguePages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		result, err := catalogue.ListDevices(request.Context(), tenant, operatorCatalogueReadLimit, query)
		if err != nil {
			// 读不回是答案未形成，不是空册。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		bodies := make([]workDeviceBody, 0, len(result.Rows))
		for _, row := range result.Rows {
			bodies = append(bodies, workDeviceBodyOf(row))
		}
		writeJSON(response, http.StatusOK, workDeviceListResponse{
			Outcome: outcomeWorkDevicesListed,
			Devices: bodies,
			Page:    result.Page(operatorCatalogueReadLimit),
		})
	}), nil
}

func writeCatalogueProblem(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, accessidentity.ErrAccessChannelNotConfigured):
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
	case errors.Is(err, accessidentity.ErrCredentialRejected):
		writeProblem(response, http.StatusUnauthorized, codeOperatorCredentialRejected)
	case errors.Is(err, accessidentity.ErrOperatorNotGranted):
		writeProblem(response, http.StatusForbidden, codeOperatorNotGranted)
	case errors.Is(err, accessidentity.ErrCredentialVerifierUnavailable),
		errors.Is(err, accessidentity.ErrOperatorRegistryUnavailable):
		writeProblem(response, http.StatusServiceUnavailable, codeIdentityDependencyUnavailable)
	default:
		writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
	}
}

type workDeviceListResponse struct {
	Outcome string             `json:"outcome"`
	Devices []workDeviceBody   `json:"devices"`
	Page    cataloguepage.Page `json:"page"`
}

// workDeviceBody 只有查阅行上的那几格。deactivatedAt 缺席即这一行尚未停用。
type workDeviceBody struct {
	DeviceID      string `json:"deviceId"`
	WorkScopeKind string `json:"workScopeKind"`
	WorkScopeRef  string `json:"workScopeRef"`
	Status        string `json:"status"`
	DeactivatedAt string `json:"deactivatedAt,omitempty"`
}

func workDeviceBodyOf(row accessidentity.DeviceCatalogueRow) workDeviceBody {
	body := workDeviceBody{
		DeviceID:      row.DeviceID(),
		WorkScopeKind: row.WorkScope().Kind().String(),
		WorkScopeRef:  row.WorkScope().Reference(),
		Status:        row.Status().String(),
	}
	if at, deactivated := row.DeactivatedAt(); deactivated {
		body.DeactivatedAt = at.UTC().Format(time.RFC3339Nano)
	}
	return body
}

type problemResponse struct {
	Error problemDetail `json:"error"`
}

// problemDetail 的 detail 只随「改请求才会好」的 4xx 在场，是给操作者看的理由散文，
// 调用方不据此分支；为空时那一格缺席，不写空串。
type problemDetail struct {
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

func writeProblem(response http.ResponseWriter, status int, code string) {
	writeJSON(response, status, problemResponse{Error: problemDetail{Code: code}})
}

func writeProblemWithDetail(response http.ResponseWriter, status int, code string, detail string) {
	writeJSON(response, status, problemResponse{Error: problemDetail{Code: code, Detail: detail}})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
