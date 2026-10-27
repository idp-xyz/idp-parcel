package identityhttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// OperatorGrantsPath 是「本会话的操作者此刻持有哪几格授予」自查的路径。parcel-api 端点表挂这一行。
const OperatorGrantsPath = "/operator-grants"

// outcomeOperatorGrantsResolved 是本端点唯一的业务成格。
const outcomeOperatorGrantsResolved = "OPERATOR_GRANTS_RESOLVED"

// OperatorGrantsHeld 是认证件交回的：信封上铸造那一刻生效的能力面与运营决定种类。
type OperatorGrantsHeld struct {
	Faces     []accessidentity.CapabilityFace
	Decisions []accessidentity.DecisionKind
}

// OperatorGrantsAuthenticator 认证一次授予自查出示，交回信封上的授予。
type OperatorGrantsAuthenticator interface {
	AuthenticateGrantsRead(ctx context.Context, bearerToken string) (OperatorGrantsHeld, error)
}

// grantsAuthenticator 以主数据与运营查阅读铸信封，不带准入要求：自查不形成作业事实。
type grantsAuthenticator struct {
	minter operatorMinter
}

// NewOperatorGrantsAuthenticator 的 minter 不许缺。
func NewOperatorGrantsAuthenticator(minter operatorMinter) (OperatorGrantsAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("access identity http: operator grants authenticator needs a minter")
	}
	return grantsAuthenticator{minter: minter}, nil
}

// AuthenticateGrantsRead 不把请求里的租户交进铸造：租户只取自操作者册上的绑定，授予集也只取自信封。
func (authenticator grantsAuthenticator) AuthenticateGrantsRead(ctx context.Context, bearerToken string) (OperatorGrantsHeld, error) {
	envelope, err := authenticator.minter.MintOperator(ctx, accessidentity.NewOperatorCredential(bearerToken),
		accessidentity.OperatorRequest{Face: accessidentity.CapabilityMasterDataAndOperationsRead})
	if err != nil {
		return OperatorGrantsHeld{}, err
	}
	if !envelope.Minted() {
		return OperatorGrantsHeld{}, errors.New("access identity http: minted envelope is not minted")
	}
	return OperatorGrantsHeld{Faces: envelope.Grants(), Decisions: envelope.Decisions()}, nil
}

// OperatorGrantsHandler 把已装好的处理器交回端点表；没装时答未配置，同 OperatorBindingHandler。
func OperatorGrantsHandler(wired http.Handler) http.Handler {
	if wired == nil {
		return UnconfiguredOperatorGrants()
	}
	return wired
}

// UnconfiguredOperatorGrants 在这一口还没接上操作者铸造器时答未配置。
func UnconfiguredOperatorGrants() http.Handler {
	return unconfiguredOperatorRead()
}

// NewQueryOperatorGrantsEndpoint 交回授予自查的 HTTP 入口：答这个会话的操作者此刻持有哪几格能力面授予与哪几种运营决定授予
// （ADR-0315）。管理台据此不摆这个操作者做不了的写入口；摆不摆只是呈现，每个写口照旧在服务端判授予，拒绝仍是服务端的答案。
//
// 只答自己：授予集取自令牌所证明的那个主体在操作者册上的行，没有按别的主体查询的参数，也不回绑定租户（那是
// /operator-binding 的事）。用主数据与运营查阅读这一面守口，认证件与操作者绑定同款——没有这一面授予的人与不在册、不绑
// 这个租户的人同答 OPERATOR_NOT_GRANTED（ADR-0100 决定四），不借自查口把「我持有什么」说给一个只证明了自己持有令牌的人。
// 管理台那一侧把这类拒绝当作「不知道」，不据此隐藏任何入口。
func NewQueryOperatorGrantsEndpoint(authenticator OperatorGrantsAuthenticator) (http.Handler, error) {
	if authenticator == nil {
		return nil, errors.New("access identity http: operator grants endpoint needs an authenticator")
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		held, err := authenticator.AuthenticateGrantsRead(request.Context(), httpapi.BearerToken(request))
		if err != nil {
			writeCatalogueProblem(response, err)
			return
		}
		body := operatorGrantsResponse{
			Outcome:   outcomeOperatorGrantsResolved,
			Grants:    make([]string, 0, len(held.Faces)),
			Decisions: make([]string, 0, len(held.Decisions)),
		}
		for _, face := range held.Faces {
			body.Grants = append(body.Grants, string(face))
		}
		for _, kind := range held.Decisions {
			body.Decisions = append(body.Decisions, string(kind))
		}
		writeJSON(response, http.StatusOK, body)
	}), nil
}

type operatorGrantsResponse struct {
	Outcome   string   `json:"outcome"`
	Grants    []string `json:"grants"`
	Decisions []string `json:"decisions"`
}
