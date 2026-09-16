package commercialhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// LegalEntityRevisionHistoryReader 是责任法人修订历史端点消费的读口（票 admin-web-group-
// legal-entities/03）。它单独成接口而不并进 PartyIdentityCatalogueReader 的方法表：端点只声明
// 自己用到的那一个方法，装配点交进来的仍是同一只商业目录适配器（PartyIdentityCatalogueReader
// 嵌入了它）。
type LegalEntityRevisionHistoryReader interface {
	ListLegalEntityRevisions(
		ctx context.Context,
		tenant domain.TenantID,
		entity domain.LegalEntityReference,
	) ([]ports.LegalEntityRevisionRow, error)
}

// 编译期锁缝：读口形状与端口保持一致。
var _ LegalEntityRevisionHistoryReader = ports.LegalEntityRevisionHistoryRead(nil)

// 本端点唯一的业务成格；不在册的法人也是这一格（空数组），理由在构造函数注释。
const outcomeLegalEntityRevisionsListed = "LEGAL_ENTITY_REVISIONS_LISTED"

// legalEntityIDPathValue 是路由模式里的路径参数名；装配点的 Pattern 与这里必须同字，
// 否则 PathValue 恒空、端点恒答 400——cmd/parcel-api 的隔离读用例经真路由钉住这一格。
const legalEntityIDPathValue = "legalEntityId"

// NewQueryLegalEntityRevisionsEndpoint 交回责任法人修订历史查阅的 HTTP 入口
// （GET /commercial-group-legal-entities/{legalEntityId}/revisions）。
//
// 它是集团与法人页详情抽屉「修订历史」区的供数面：目录行是每个法人的最新修订（ADR-0077），
// 身份登记按修订版本化只增不覆盖（CONTEXT「身份生命周期」），这一口把一个法人的整条修订链
// 交出来，让 r1→r2 改了什么、依据从哪份换到哪份、停用是哪一笔在页面上看得见。不做 diff，
// 两笔之间改了什么由前端并排显，这里只交事实。
//
// 法人标识从路径取而不从查询串取：它定位的是资源本身（这个法人的历史），不是在一份列表上
// 挑格；空标识是请求构造不出查询（400 MALFORMED_REQUEST，重发不会好），不是空历史。
//
// **法人不在册答 200 + 空数组，不答 404。** 票 03 按 ADR-0022 裁：404 说的是「这个产品没有
// 这条能力」，而这里能力在、册也在，只是册上没有这一个身份——查阅方要去查的是册面不是路由，
// 判据同 writePartyRegistryAnswer 对`未找到`走 200 的处置。它也不违 ADR-0022「`!found` 不得
// 区分不存在与他租」那一句：读口是列表形状、没有 found 一格，别家租户的法人在本租户作用域下
// 与不在册同为零行，两态在这里结构上同形，不必分、也分不出。
//
// Intake 与其余目录查阅口同一个（CommercialCatalogueIntake）：作用域形状同为租户一维，隔离读
// 准入（ADR-0078）启用时随 commercialCatalogue 那一格一起换值，不另加开关；Limit 不用——
// 一个法人的修订链是要全部交出的证据面。
func NewQueryLegalEntityRevisionsEndpoint(
	intake CommercialCatalogueIntake,
	reader LegalEntityRevisionHistoryReader,
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
		entity, err := domain.NewLegalEntityReference(request.PathValue(legalEntityIDPathValue))
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}

		rows, err := reader.ListLegalEntityRevisions(request.Context(), query.Scope.Tenant(), entity)
		if err != nil {
			// 读不回是答案未形成，不是「没有历史」——伪装成后者会让一次该重试的故障变成一段
			// 看起来如实的空历史。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		bodies := make([]legalEntityRevisionBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, legalEntityRevisionBodyOf(row))
		}
		writeJSON(response, http.StatusOK, legalEntityRevisionListResponse{
			Outcome:       outcomeLegalEntityRevisionsListed,
			LegalEntityID: entity.String(),
			Revisions:     bodies,
		})
	})
}

// legalEntityRevisionListResponse 顶层回显法人标识：抽屉切换行时上一问的答案可能后到，
// 页面据此核对答的是不是它此刻问的那个法人，不拿数组首笔的 legalEntityId 去推（空数组没有首笔）。
type legalEntityRevisionListResponse struct {
	Outcome       string                    `json:"outcome"`
	LegalEntityID string                    `json:"legalEntityId"`
	Revisions     []legalEntityRevisionBody `json:"revisions"`
}

// legalEntityRevisionBody 逐字段透出一笔修订。没有 status 与 partyName——每一笔各有自己的生效
// 与停用时点，给历史上的每一笔算「此刻的状态」会让被顶替的旧笔各自显出一格状态；名称在参与方册
// 上不随法人修订走（理由在 ports.LegalEntityRevisionRow）。停用两件只在已停用那一笔在场，判据
// 同 groupLegalEntityBody：显式布尔，不拿空串去推。
type legalEntityRevisionBody struct {
	TenantID          string `json:"tenantId"`
	LegalEntityID     string `json:"legalEntityId"`
	PartyID           string `json:"partyId"`
	Revision          int    `json:"revision"`
	Basis             string `json:"basis"`
	EffectiveFrom     string `json:"effectiveFrom"`
	DeactivatedAt     string `json:"deactivatedAt,omitempty"`
	DeactivationBasis string `json:"deactivationBasis,omitempty"`
	RegisteredAt      string `json:"registeredAt"`
}

func legalEntityRevisionBodyOf(row ports.LegalEntityRevisionRow) legalEntityRevisionBody {
	body := legalEntityRevisionBody{
		TenantID:      row.TenantID,
		LegalEntityID: row.LegalEntityID,
		PartyID:       row.PartyID,
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
