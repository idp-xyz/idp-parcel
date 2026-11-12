package commercialhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// PartyIdentityCatalogueReader 是参与方身份目录各端点消费的读口。它嵌入两册的修订历史读口
// （法人册票 admin-web-group-legal-entities/03，参与方册票 12）：这些端点在装配点上共用同一个读口参数，
// 判据在 ports.PartyIdentityCatalogueRead——读同一张表、同一租户作用域、供同一页。
type PartyIdentityCatalogueReader interface {
	LegalEntityRevisionHistoryReader
	BusinessPartyRevisionHistoryReader
	ListBusinessParties(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.BusinessPartyRow], error)
	ListGroupLegalEntities(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.GroupLegalEntityRow], error)
	ListPartyRelationships(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.PartyRelationshipRow], error)
}

// 编译期锁缝：读口形状与端口保持一致。
var _ PartyIdentityCatalogueReader = ports.PartyIdentityCatalogueRead(nil)

// 三个端点各自唯一的业务成格；空册也是这一格（ADR-0077 Decision 四）。
const (
	outcomeBusinessPartiesListed    = "BUSINESS_PARTIES_LISTED"
	outcomeGroupLegalEntitiesListed = "GROUP_LEGAL_ENTITIES_LISTED"
	outcomePartyRelationshipsListed = "PARTY_RELATIONSHIPS_LISTED"
)

// 集团与法人、业务参与方**各立入口**，判据与合同/协议两端点的文件注释同一条：这是
// 管理台上两张独立的页，一页一入口；且两册的行形状与状态代数都不同（身份状态按时点
// 导出、关系状态是登记事实），折进一个带 kind 的入口会让两种状态在同一响应形状里
// 相互冒充。
//
// 各目录端点的查询参数（after、sort、筛选维与 q）各按本册声明解码（ports.BusinessPartyPages、
// GroupLegalEntityPages、PartyRelationshipPages；ADR-0144 决定六），在 Intake 之后：渠道未配置时对一切请求同答 403
// （ADR-0055 决定三）。

// NewQueryBusinessPartiesEndpoint 交回业务参与方身份本体查阅的 HTTP 入口
// （GET /commercial-business-parties，ADR-0077）。
//
// 它与关系那一口分立，而不是折进 /commercial-party-relationships：一个参与方**既可以
// 不是法人、也可以不在任何关系里**，被停用的那种恰恰如此；把身份本体挂在关系入口下，
// 那类参与方就永远不上列，而身份生命周期的`已登记`与`已停用`两格因此在管理台无实例
// 可见（票 admin-remainder-mechanism-batch/01 的补格裁定）。
func NewQueryBusinessPartiesEndpoint(
	intake CommercialCatalogueIntake,
	reader PartyIdentityCatalogueReader,
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

		pageQuery, err := ports.BusinessPartyPages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}

		page, err := reader.ListBusinessParties(request.Context(), query.Scope.Tenant(), query.Limit, pageQuery)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		bodies := make([]businessPartyBody, 0, len(page.Rows))
		for _, row := range page.Rows {
			bodies = append(bodies, businessPartyBodyOf(row))
		}
		writeJSON(response, http.StatusOK, businessPartyListResponse{
			Outcome: outcomeBusinessPartiesListed,
			Parties: bodies,
			Page:    page.Page(query.Limit),
			Facets:  page.Facets,
		})
	})
}

// Facets 是 status 与 categories 两维的分面计数（ADR-0378 决定五、六）：各值在本次的 q 与另一维之下各有几家。
type businessPartyListResponse struct {
	Outcome string               `json:"outcome"`
	Parties []businessPartyBody  `json:"parties"`
	Page    cataloguepage.Page   `json:"page"`
	Facets  cataloguepage.Facets `json:"facets,omitempty"`
}

// businessPartyBody 逐字段透出参与方身份本体的最新修订。这里没有 partyNameKnown：
// 名称在本册行上是 NOT NULL 的，不像法人与关系那样要左连接过来才有——那两处的
// 「查无此人」是写入门失败的悬空引用，本册没有那一格可缺。
type businessPartyBody struct {
	TenantID          string `json:"tenantId"`
	PartyID           string `json:"partyId"`
	PartyName         string `json:"partyName"`
	Status            string `json:"status"`
	Revision          int    `json:"revision"`
	Basis             string `json:"basis"`
	EffectiveFrom     string `json:"effectiveFrom"`
	DeactivatedAt     string `json:"deactivatedAt,omitempty"`
	DeactivationBasis string `json:"deactivationBasis,omitempty"`
	RegisteredAt      string `json:"registeredAt"`
	// Categories 与 RelationshipCounts 是装载时点由登记的事实读出的（ADR-0378 决定三），与 status 同一口径：类别永不
	// 为空、照词表次序；段数只是行数。
	Categories         []string               `json:"categories"`
	RelationshipCounts relationshipCountsBody `json:"relationshipCounts"`
	businessPartyIdentificationBody
}

type relationshipCountsBody struct {
	AsHolder       relationshipTallyBody `json:"asHolder"`
	AsCounterparty relationshipTallyBody `json:"asCounterparty"`
}

type relationshipTallyBody struct {
	Live      int `json:"live"`
	Candidate int `json:"candidate"`
}

func businessPartyBodyOf(row ports.BusinessPartyRow) businessPartyBody {
	body := businessPartyBody{
		TenantID:      row.TenantID,
		PartyID:       row.PartyID,
		PartyName:     row.PartyName,
		Status:        row.Status,
		Revision:      row.Revision,
		Basis:         row.Basis,
		EffectiveFrom: rfc3339(row.EffectiveFrom),
		RegisteredAt:  rfc3339(row.RegisteredAt),
		// 读面保证类别不为空；这里仍给一个非 nil 的切片，答复里这一格总是数组，不会编成 null。
		Categories: append([]string{}, row.Categories...),
		RelationshipCounts: relationshipCountsBody{
			AsHolder: relationshipTallyBody{
				Live: row.RelationshipCounts.AsHolder.Live, Candidate: row.RelationshipCounts.AsHolder.Candidate,
			},
			AsCounterparty: relationshipTallyBody{
				Live: row.RelationshipCounts.AsCounterparty.Live, Candidate: row.RelationshipCounts.AsCounterparty.Candidate,
			},
		},
	}
	if row.HasDeactivation {
		body.DeactivatedAt = rfc3339(row.DeactivatedAt)
		body.DeactivationBasis = row.DeactivationBasis
	}
	body.businessPartyIdentificationBody = businessPartyIdentificationBodyOf(row.BusinessPartyIdentificationRow)
	return body
}

// businessPartyIdentificationBody 是往来单位一笔修订的识别信息（ADR-0367），目录行与修订历史行共用。identityLayerRegistered
// 是显式布尔，判据同法人的身份层：为假即这笔修订没带号，页面不拿空数组去推；外部编码零项时整格缺席。
type businessPartyIdentificationBody struct {
	identityLayerBody
	ExternalReferences []externalReferenceBody `json:"externalReferences,omitempty"`
}

type externalReferenceBody struct {
	System  string `json:"system"`
	Purpose string `json:"purpose"`
	Code    string `json:"code"`
}

func businessPartyIdentificationBodyOf(row ports.BusinessPartyIdentificationRow) businessPartyIdentificationBody {
	body := businessPartyIdentificationBody{
		identityLayerBody: identityLayerBodyOf(
			row.HasIdentityLayer, row.RegistrationCountry, row.LifetimeNumbers,
			row.HasIdentityCorrection, row.IdentityCorrectionBasis,
		),
	}
	for _, reference := range row.ExternalReferences {
		body.ExternalReferences = append(body.ExternalReferences, externalReferenceBody{
			System: reference.System, Purpose: reference.Purpose, Code: reference.Code,
		})
	}
	return body
}

// NewQueryGroupLegalEntitiesEndpoint 交回集团与法人目录查阅的 HTTP 入口
// （GET /commercial-group-legal-entities，ADR-0077）。
func NewQueryGroupLegalEntitiesEndpoint(
	intake CommercialCatalogueIntake,
	reader PartyIdentityCatalogueReader,
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

		pageQuery, err := ports.GroupLegalEntityPages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}

		page, err := reader.ListGroupLegalEntities(request.Context(), query.Scope.Tenant(), query.Limit, pageQuery)
		if err != nil {
			// 读不回是答案未形成，不是「空目录」——伪装成后者会让一次该重试的故障
			// 变成一份看起来如实的空册。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		bodies := make([]groupLegalEntityBody, 0, len(page.Rows))
		for _, row := range page.Rows {
			bodies = append(bodies, groupLegalEntityBodyOf(row))
		}
		writeJSON(response, http.StatusOK, groupLegalEntityListResponse{
			Outcome:  outcomeGroupLegalEntitiesListed,
			Entities: bodies,
			Page:     page.Page(query.Limit),
		})
	})
}

// NewQueryPartyRelationshipsEndpoint 交回业务参与方（关系目录）查阅的 HTTP 入口
// （GET /commercial-party-relationships，ADR-0077）。
func NewQueryPartyRelationshipsEndpoint(
	intake CommercialCatalogueIntake,
	reader PartyIdentityCatalogueReader,
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

		pageQuery, err := ports.PartyRelationshipPages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}

		page, err := reader.ListPartyRelationships(request.Context(), query.Scope.Tenant(), query.Limit, pageQuery)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		bodies := make([]partyRelationshipBody, 0, len(page.Rows))
		for _, row := range page.Rows {
			bodies = append(bodies, partyRelationshipBodyOf(row))
		}
		writeJSON(response, http.StatusOK, partyRelationshipListResponse{
			Outcome:       outcomePartyRelationshipsListed,
			Relationships: bodies,
			Page:          page.Page(query.Limit),
			Facets:        page.Facets,
		})
	})
}

type groupLegalEntityListResponse struct {
	Outcome  string                 `json:"outcome"`
	Entities []groupLegalEntityBody `json:"entities"`
	Page     cataloguepage.Page     `json:"page"`
}

// groupLegalEntityBody 逐字段透出法人最新修订与其参与方名称。
//
// kind 是封闭词转写：本册今天只有责任法人一格（经营组织没有登记面，如实不上列，
// 不预开空格）。partyName 只在参与方册查得到时在场——法人钉着的参与方查无此人是
// 写入门失败才会出现的悬空，页面按缺席如实显示，不补占位文本。deactivation 两件
// 只在已停用时在场，判据与合同页 contentRegistered 同款：显式布尔，不拿空串去推。
type groupLegalEntityBody struct {
	TenantID          string `json:"tenantId"`
	LegalEntityID     string `json:"legalEntityId"`
	Kind              string `json:"kind"`
	PartyID           string `json:"partyId"`
	PartyName         string `json:"partyName,omitempty"`
	PartyNameKnown    bool   `json:"partyNameKnown"`
	Status            string `json:"status"`
	Revision          int    `json:"revision"`
	Basis             string `json:"basis"`
	EffectiveFrom     string `json:"effectiveFrom"`
	DeactivatedAt     string `json:"deactivatedAt,omitempty"`
	DeactivationBasis string `json:"deactivationBasis,omitempty"`
	RegisteredAt      string `json:"registeredAt"`
	identityLayerBody
}

// identityLayerBody 是法人身份层在答复里的四格（ADR-0145 决定一、二），目录行与修订历史行共用。判据同停用两件：
// identityLayerRegistered 是显式布尔，为假即这笔修订登记于本格落地之前、国家与号都没有，页面据它如实说「本修订
// 登记时尚无此格」，不拿空数组去推；更正依据只在这笔修订是身份更正时在场。
type identityLayerBody struct {
	IdentityLayerRegistered     bool                             `json:"identityLayerRegistered"`
	RegistrationCountry         string                           `json:"registrationCountry,omitempty"`
	LifetimeRegistrationNumbers []lifetimeRegistrationNumberBody `json:"lifetimeRegistrationNumbers,omitempty"`
	IdentityCorrectionBasis     string                           `json:"identityCorrectionBasis,omitempty"`
}

type lifetimeRegistrationNumberBody struct {
	TypeCode string `json:"typeCode"`
	Number   string `json:"number"`
}

func identityLayerBodyOf(
	hasIdentity bool,
	country string,
	numbers []ports.LifetimeRegistrationNumberRow,
	hasCorrection bool,
	correction string,
) identityLayerBody {
	body := identityLayerBody{IdentityLayerRegistered: hasIdentity}
	if hasIdentity {
		body.RegistrationCountry = country
		body.LifetimeRegistrationNumbers = make([]lifetimeRegistrationNumberBody, len(numbers))
		for index, number := range numbers {
			body.LifetimeRegistrationNumbers[index] = lifetimeRegistrationNumberBody{
				TypeCode: number.TypeCode,
				Number:   number.Number,
			}
		}
	}
	if hasCorrection {
		body.IdentityCorrectionBasis = correction
	}
	return body
}

const kindResponsibleLegalEntity = "RESPONSIBLE_LEGAL_ENTITY"

func groupLegalEntityBodyOf(row ports.GroupLegalEntityRow) groupLegalEntityBody {
	body := groupLegalEntityBody{
		TenantID:       row.TenantID,
		LegalEntityID:  row.LegalEntityID,
		Kind:           kindResponsibleLegalEntity,
		PartyID:        row.PartyID,
		PartyNameKnown: row.HasPartyName,
		Status:         row.Status,
		Revision:       row.Revision,
		Basis:          row.Basis,
		EffectiveFrom:  rfc3339(row.EffectiveFrom),
		RegisteredAt:   rfc3339(row.RegisteredAt),
		identityLayerBody: identityLayerBodyOf(
			row.HasIdentityLayer, row.RegistrationCountry, row.LifetimeNumbers,
			row.HasIdentityCorrection, row.IdentityCorrectionBasis,
		),
	}
	if row.HasPartyName {
		body.PartyName = row.PartyName
	}
	if row.HasDeactivation {
		body.DeactivatedAt = rfc3339(row.DeactivatedAt)
		body.DeactivationBasis = row.DeactivationBasis
	}
	return body
}

// Facets 是 status 一维的分面计数（ADR-0378 决定五、六）：各状态在本次的 q 与其余各维之下各有几段。
type partyRelationshipListResponse struct {
	Outcome       string                  `json:"outcome"`
	Relationships []partyRelationshipBody `json:"relationships"`
	Page          cataloguepage.Page      `json:"page"`
	Facets        cataloguepage.Facets    `json:"facets,omitempty"`
}

// partyRelationshipBody 逐字段透出关系最新修订。方向由持有方→相对方的字段次序表达
// （CONTEXT 规则句「参与方关系必须保存双方、角色、方向、适用范围、依据和有效区间」；方向不另设标志位）。终止三件
// （endedAt/endBasis/successor）只在相应终止格在场，页面按状态分读。
type partyRelationshipBody struct {
	TenantID              string `json:"tenantId"`
	RelationshipID        string `json:"relationshipId"`
	Revision              int    `json:"revision"`
	HolderID              string `json:"holderId"`
	HolderName            string `json:"holderName,omitempty"`
	HolderNameKnown       bool   `json:"holderNameKnown"`
	CounterpartyID        string `json:"counterpartyId"`
	CounterpartyName      string `json:"counterpartyName,omitempty"`
	CounterpartyNameKnown bool   `json:"counterpartyNameKnown"`
	Role                  string `json:"role"`
	Scope                 string `json:"scope"`
	Basis                 string `json:"basis"`
	Status                string `json:"status"`
	EffectiveStartsAt     string `json:"effectiveStartsAt"`
	EffectiveEndsAt       string `json:"effectiveEndsAt,omitempty"`
	EndedAt               string `json:"endedAt,omitempty"`
	EndBasis              string `json:"endBasis,omitempty"`
	SuccessorID           string `json:"successorId,omitempty"`
	RegisteredAt          string `json:"registeredAt"`
}

func partyRelationshipBodyOf(row ports.PartyRelationshipRow) partyRelationshipBody {
	body := partyRelationshipBody{
		TenantID:              row.TenantID,
		RelationshipID:        row.RelationshipID,
		Revision:              row.Revision,
		HolderID:              row.HolderID,
		HolderNameKnown:       row.HasHolderName,
		CounterpartyID:        row.CounterpartyID,
		CounterpartyNameKnown: row.HasCounterpartyName,
		Role:                  row.Role,
		Scope:                 row.Scope,
		Basis:                 row.Basis,
		Status:                row.Status,
		EffectiveStartsAt:     rfc3339(row.EffectiveStartsAt),
		RegisteredAt:          rfc3339(row.RegisteredAt),
	}
	if row.HasHolderName {
		body.HolderName = row.HolderName
	}
	if row.HasCounterpartyName {
		body.CounterpartyName = row.CounterpartyName
	}
	if row.HasEffectiveEnd {
		body.EffectiveEndsAt = rfc3339(row.EffectiveEndsAt)
	}
	if row.HasEnd {
		body.EndedAt = rfc3339(row.EndedAt)
	}
	if row.EndBasis != "" {
		body.EndBasis = row.EndBasis
	}
	if row.SuccessorID != "" {
		body.SuccessorID = row.SuccessorID
	}
	return body
}
