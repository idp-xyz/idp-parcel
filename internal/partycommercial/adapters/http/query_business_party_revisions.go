package commercialhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// BusinessPartyRevisionHistoryReader 是业务参与方修订历史端点消费的读口（票 admin-web-group-
// legal-entities/12）。它单独成接口而不并进 PartyIdentityCatalogueReader 的方法表，判据同
// LegalEntityRevisionHistoryReader：端点只声明自己用到的那一个方法，装配点交进来的仍是同一只
// 商业目录适配器（PartyIdentityCatalogueReader 嵌入了它）。
type BusinessPartyRevisionHistoryReader interface {
	ListBusinessPartyRevisions(
		ctx context.Context,
		tenant domain.TenantID,
		party domain.PartyID,
	) ([]ports.BusinessPartyRevisionRow, error)
}

// 编译期锁缝：读口形状与端口保持一致。
var _ BusinessPartyRevisionHistoryReader = ports.BusinessPartyRevisionHistoryRead(nil)

// 本端点唯一的业务成格；不在册的参与方也是这一格（空数组），理由在构造函数注释。
const outcomeBusinessPartyRevisionsListed = "BUSINESS_PARTY_REVISIONS_LISTED"

// partyIDPathValue 是路由模式里的路径参数名；装配点的 Pattern 与这里必须同字，
// 否则 PathValue 恒空、端点恒答 400——cmd/parcel-api 的隔离读用例经真路由钉住这一格。
const partyIDPathValue = "partyId"

// NewQueryBusinessPartyRevisionsEndpoint 交回业务参与方修订历史查阅的 HTTP 入口
// （GET /commercial-business-parties/{partyId}/revisions）。
//
// 它是业务参与方页详情抽屉「修订历史」区的供数面，把 NewQueryLegalEntityRevisionsEndpoint 的形状搬到
// 参与方册：目录行是每个参与方的最新修订（ADR-0077），身份登记按修订版本化只增不覆盖（CONTEXT Lifecycles
// 下「参与方身份（业务参与方、责任法人、货主客户账户）」），这一口把一个参与方的整条修订链交出来，让
// 名称从哪份换到哪份、依据换过几次、停用是哪一笔在页面上看得见。不做 diff，两笔之间改了什么由前端并排显，
// 这里只交事实。
//
// 参与方标识从路径取而不从查询串取、**不在册答 200 + 空数组不答 404**、Intake 与其余目录查阅口同一个
// ——三条裁决与理由与法人那一口一字不改（票 03 按 ADR-0022 裁，判据同 writePartyRegistryAnswer 对`未找到`
// 走 200；跨租户与不在册在列表形状上结构同形，不泄露存在性），此处不复述。
func NewQueryBusinessPartyRevisionsEndpoint(
	intake CommercialCatalogueIntake,
	reader BusinessPartyRevisionHistoryReader,
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

		// 路径参数在 Intake 之后读：未配置要拒在一切请求内容之前（ADR-0055），路径也是请求内容。
		party, err := domain.NewPartyID(request.PathValue(partyIDPathValue))
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}

		rows, err := reader.ListBusinessPartyRevisions(request.Context(), query.Scope.Tenant(), party)
		if err != nil {
			// 读不回是答案未形成，不是「没有历史」——伪装成后者会让一次该重试的故障变成一段
			// 看起来如实的空历史。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		bodies := make([]businessPartyRevisionBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, businessPartyRevisionBodyOf(row))
		}
		writeJSON(response, http.StatusOK, businessPartyRevisionListResponse{
			Outcome:   outcomeBusinessPartyRevisionsListed,
			PartyID:   party.String(),
			Revisions: bodies,
		})
	})
}

// businessPartyRevisionListResponse 顶层回显参与方标识：抽屉切换行时上一问的答案可能后到，
// 页面据此核对答的是不是它此刻问的那个参与方，不拿数组首笔的 partyId 去推（空数组没有首笔）。
type businessPartyRevisionListResponse struct {
	Outcome   string                      `json:"outcome"`
	PartyID   string                      `json:"partyId"`
	Revisions []businessPartyRevisionBody `json:"revisions"`
}

// businessPartyRevisionBody 逐字段透出一笔修订。带 partyName 不带 status：名称登在本册自己的行上、
// 随修订走，是这一笔的内容；每一笔各有自己的生效与停用时点，给历史上的每一笔算「此刻的状态」会让被顶替
// 的旧笔各自显出一格状态（理由在 ports.BusinessPartyRevisionRow）。停用两件只在已停用那一笔在场，判据
// 同 businessPartyBody：显式布尔，不拿空串去推。
type businessPartyRevisionBody struct {
	TenantID          string `json:"tenantId"`
	PartyID           string `json:"partyId"`
	PartyName         string `json:"partyName"`
	Revision          int    `json:"revision"`
	Basis             string `json:"basis"`
	EffectiveFrom     string `json:"effectiveFrom"`
	DeactivatedAt     string `json:"deactivatedAt,omitempty"`
	DeactivationBasis string `json:"deactivationBasis,omitempty"`
	RegisteredAt      string `json:"registeredAt"`
}

func businessPartyRevisionBodyOf(row ports.BusinessPartyRevisionRow) businessPartyRevisionBody {
	body := businessPartyRevisionBody{
		TenantID:      row.TenantID,
		PartyID:       row.PartyID,
		PartyName:     row.PartyName,
		Revision:      row.Revision,
		Basis:         row.Basis,
		EffectiveFrom: rfc3339(row.EffectiveFrom),
		RegisteredAt:  rfc3339(row.RegisteredAt),
	}
	if row.HasDeactivation {
		body.DeactivatedAt = rfc3339(row.DeactivatedAt)
		body.DeactivationBasis = row.DeactivationBasis
	}
	return body
}
