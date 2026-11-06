package commercialhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// RegistrationNumberTypeCatalogueReader 是注册号类型目录端点消费的读口。
type RegistrationNumberTypeCatalogueReader interface {
	ListRegistrationNumberTypes(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.RegistrationNumberTypeRow], error)
}

// 编译期锁缝：读口形状与端口保持一致。
var _ RegistrationNumberTypeCatalogueReader = ports.RegistrationNumberTypeCatalogueRead(nil)

// 本端点唯一的业务成格；空册也是这一格（ADR-0077 Decision 四）。
const outcomeRegistrationNumberTypesListed = "REGISTRATION_NUMBER_TYPES_LISTED"

// NewQueryRegistrationNumberTypesEndpoint 交回注册号类型目录查阅的 HTTP 入口
// （GET /commercial-registration-number-types，ADR-0077）。查阅不触发判断——判号走
// ports.RegistrationNumberTypeLookup，不走本口。查询参数按 ports.RegistrationNumberTypePages 解码（ADR-0144 决定六），
// 在 Intake 之后：渠道未配置时对一切请求同答 403（ADR-0055 决定三）。
func NewQueryRegistrationNumberTypesEndpoint(
	intake CommercialCatalogueIntake,
	reader RegistrationNumberTypeCatalogueReader,
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

		pageQuery, err := ports.RegistrationNumberTypePages.Decode(request.URL.Query())
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}

		page, err := reader.ListRegistrationNumberTypes(request.Context(), query.Scope.Tenant(), query.Limit, pageQuery)
		if err != nil {
			// 读不回是答案未形成，不是「空目录」。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		bodies := make([]registrationNumberTypeBody, 0, len(page.Rows))
		for _, row := range page.Rows {
			bodies = append(bodies, registrationNumberTypeBodyOf(row))
		}
		writeJSON(response, http.StatusOK, registrationNumberTypeListResponse{
			Outcome:                 outcomeRegistrationNumberTypesListed,
			RegistrationNumberTypes: bodies,
			Page:                    page.Page(query.Limit),
		})
	})
}

type registrationNumberTypeListResponse struct {
	Outcome                 string                       `json:"outcome"`
	RegistrationNumberTypes []registrationNumberTypeBody `json:"registrationNumberTypes"`
	Page                    cataloguepage.Page           `json:"page"`
}

// registrationNumberTypeBody 逐字段透出类型的最新修订。停用两件只在已停用时出现：缺席即没有停用，
// 不拿空串兼作「没停用」（判据同法人目录行）。
type registrationNumberTypeBody struct {
	TenantID          string `json:"tenantId"`
	CountryCode       string `json:"countryCode"`
	TypeCode          string `json:"typeCode"`
	Revision          int    `json:"revision"`
	TypeName          string `json:"typeName"`
	Layer             string `json:"layer"`
	FormatPattern     string `json:"formatPattern"`
	Basis             string `json:"basis"`
	Status            string `json:"status"`
	EffectiveFrom     string `json:"effectiveFrom"`
	DeactivatedAt     string `json:"deactivatedAt,omitempty"`
	DeactivationBasis string `json:"deactivationBasis,omitempty"`
	RegisteredAt      string `json:"registeredAt"`
}

func registrationNumberTypeBodyOf(row ports.RegistrationNumberTypeRow) registrationNumberTypeBody {
	body := registrationNumberTypeBody{
		TenantID:      row.TenantID,
		CountryCode:   row.CountryCode,
		TypeCode:      row.TypeCode,
		Revision:      row.Revision,
		TypeName:      row.TypeName,
		Layer:         row.Layer,
		FormatPattern: row.FormatPattern,
		Basis:         row.Basis,
		Status:        row.Status,
		EffectiveFrom: rfc3339(row.EffectiveFrom),
		RegisteredAt:  rfc3339(row.RegisteredAt),
	}
	if row.HasDeactivation {
		body.DeactivatedAt = rfc3339(row.DeactivatedAt)
		body.DeactivationBasis = row.DeactivationBasis
	}
	return body
}
