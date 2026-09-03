package commercialhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// CustomerAccountCatalogueReader 是货主客户账户目录端点消费的读口（票 admin-write-faces/04）。
//
// 独立于 PartyIdentityCatalogueReader 而不是往那里加第四个方法：那个读口是集团与法人、业务
// 参与方两页的供数面，本册按 CONTEXT 落在客户与合同页——一页一入口，读口跟着页走；且加
// 方法会同笔拆掉所有替身与占位（理由写在 ports.CustomerAccountCatalogueRead 上，此处不复述）。
type CustomerAccountCatalogueReader interface {
	ListCustomerAccounts(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.CustomerAccountRow, error)
}

// 编译期锁缝：读口形状与端口保持一致。
var _ CustomerAccountCatalogueReader = ports.CustomerAccountCatalogueRead(nil)

// 本端点唯一的业务成格；空册也是这一格（ADR-0077 Decision 四）。
const outcomeCustomerAccountsListed = "CUSTOMER_ACCOUNTS_LISTED"

// NewQueryCustomerAccountsEndpoint 交回货主客户账户目录查阅的 HTTP 入口
// （GET /commercial-customer-accounts，ADR-0077）。
//
// 它与 /commercial-customer-contracts 分立而不折进去：合同上列的是商业版本壳（草稿→发布→
// 退役），账户上列的是参与方身份的登记修订（登记→生效→停用），两套状态代数在同一响应形状里
// 会相互冒充——判据与身份/关系两口分立那条同一句。
func NewQueryCustomerAccountsEndpoint(
	intake CommercialCatalogueIntake,
	reader CustomerAccountCatalogueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}

		rows, err := reader.ListCustomerAccounts(request.Context(), query.Scope.Tenant(), query.Limit)
		if err != nil {
			// 读不回是答案未形成，不是「空目录」——伪装成后者会让一次该重试的故障变成一份
			// 看起来如实的空册。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		bodies := make([]customerAccountBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, customerAccountBodyOf(row))
		}
		writeJSON(response, http.StatusOK, customerAccountListResponse{
			Outcome:  outcomeCustomerAccountsListed,
			Accounts: bodies,
		})
	})
}

type customerAccountListResponse struct {
	Outcome  string                `json:"outcome"`
	Accounts []customerAccountBody `json:"accounts"`
}

// customerAccountBody 逐字段透出账户最新修订与其客户参与方名称。
//
// customerPartyName 只在参与方册查得到时在场——账户钉着的参与方查无此人是写入门失败才会
// 出现的悬空，页面按缺席如实显示，不补占位文本。停用两件只在已停用时在场。两处都是显式
// 布尔或缺席字段，不拿空串去推（判据同法人册那一口）。
type customerAccountBody struct {
	TenantID               string `json:"tenantId"`
	AccountID              string `json:"accountId"`
	CustomerPartyID        string `json:"customerPartyId"`
	CustomerPartyName      string `json:"customerPartyName,omitempty"`
	CustomerPartyNameKnown bool   `json:"customerPartyNameKnown"`
	Status                 string `json:"status"`
	Revision               int    `json:"revision"`
	Basis                  string `json:"basis"`
	EffectiveFrom          string `json:"effectiveFrom"`
	DeactivatedAt          string `json:"deactivatedAt,omitempty"`
	DeactivationBasis      string `json:"deactivationBasis,omitempty"`
	RegisteredAt           string `json:"registeredAt"`
}

func customerAccountBodyOf(row ports.CustomerAccountRow) customerAccountBody {
	body := customerAccountBody{
		TenantID:               row.TenantID,
		AccountID:              row.AccountID,
		CustomerPartyID:        row.CustomerPartyID,
		CustomerPartyNameKnown: row.HasPartyName,
		Status:                 row.Status,
		Revision:               row.Revision,
		Basis:                  row.Basis,
		EffectiveFrom:          rfc3339(row.EffectiveFrom),
		RegisteredAt:           rfc3339(row.RegisteredAt),
	}
	if row.HasPartyName {
		body.CustomerPartyName = row.CustomerPartyName
	}
	if row.HasDeactivation {
		body.DeactivatedAt = rfc3339(row.DeactivatedAt)
		body.DeactivationBasis = row.DeactivationBasis
	}
	return body
}
