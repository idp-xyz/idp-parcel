package identityhttp

import (
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// OperatorBindingPath 是「本会话的操作者绑在哪个租户」查阅的路径。parcel-api 端点表挂这一行。
const OperatorBindingPath = "/operator-binding"

// outcomeOperatorBindingResolved 是本端点唯一的业务成格。
const outcomeOperatorBindingResolved = "OPERATOR_BINDING_RESOLVED"

// OperatorBindingHandler 把已装好的处理器交回端点表；没装时答未配置，同 WorkDeviceCatalogueHandler。
func OperatorBindingHandler(wired http.Handler) http.Handler {
	if wired == nil {
		return UnconfiguredOperatorBinding()
	}
	return wired
}

// UnconfiguredOperatorBinding 在这一口还没接上操作者铸造器时答未配置。
func UnconfiguredOperatorBinding() http.Handler {
	return unconfiguredOperatorRead()
}

// NewQueryOperatorBindingEndpoint 交回操作者绑定查阅的 HTTP 入口：答这个会话的操作者在操作者册上绑的那一个租户
// （ADR-0100 决定二第三条：一个操作者账户绑定唯一租户）。管理台顶栏的作用域与集团与法人页的页头靠它显示当前
// 集团——`id_token` 里没有租户声明，绑定只在服务端的操作者册上（票 group-legal-entities-uiux/11）。
//
// 用主数据与运营查阅读这一面守口，认证件与作业设备册是同一个。ADR-0100 决定四要求「不在册 / 不绑这个租户 /
// 没有授予」三种情形同答，不把「它绑在哪个租户」告诉一个只证明了自己持有令牌的人；这里照样守住：没有这一面
// 授予的人与另两种人同答 OPERATOR_NOT_GRANTED。有这一面授予的人本来就能从任一册的行上读到 tenantId，
// 把绑定回给他不多泄露什么。只答标识：产品没有登记租户显示名的地方，不编名称。
func NewQueryOperatorBindingEndpoint(authenticator OperatorCatalogueAuthenticator) (http.Handler, error) {
	if authenticator == nil {
		return nil, errors.New("access identity http: operator binding endpoint needs an authenticator")
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
		writeJSON(response, http.StatusOK, operatorBindingResponse{
			Outcome:  outcomeOperatorBindingResolved,
			TenantID: tenant,
		})
	}), nil
}

type operatorBindingResponse struct {
	Outcome  string `json:"outcome"`
	TenantID string `json:"tenantId"`
}
