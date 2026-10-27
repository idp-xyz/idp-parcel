package commercialhttp

import (
	"context"
	"encoding/json"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// PublicationDraftReader 是待批准发布载体读回端点消费的读口（ADR-0314 决定一）。它单独成接口，端点只声明自己用到的那两个方法；
// 装配点交进来的仍是同一只载体册适配器。
type PublicationDraftReader interface {
	LoadDraft(
		ctx context.Context,
		tenant domain.TenantID,
		kind domain.CommercialObjectKind,
		objectID domain.CommercialObjectID,
		version domain.CommercialVersionLabel,
	) (domain.PublicationDraft, bool, error)
	ListOpenDrafts(ctx context.Context, tenant domain.TenantID, limit int) ([]domain.PublicationDraft, error)
}

// 编译期锁缝：读口形状与端口保持一致。
var _ PublicationDraftReader = ports.PublicationDraftRead(nil)

const (
	outcomePublicationDraftsListed = "PUBLICATION_DRAFTS_LISTED"
	outcomePublicationDraftRead    = "PUBLICATION_DRAFT_READ"
	outcomePublicationDraftAbsent  = "DRAFT_NOT_FOUND"

	// 路由模式里的路径参数名；装配点的 Pattern 与这里必须同字，否则 PathValue 恒空、端点恒答 400。
	draftKindPathValue     = "kind"
	draftObjectIDPathValue = "objectId"
	draftVersionPathValue  = "version"
)

// NewQueryPublicationDraftsEndpoint 交回待批准发布载体列表的 HTTP 入口（GET /commercial-open-publication-drafts，ADR-0314 决定一）。
//
// 它是管理台「待批准」列表的供数面：交出租户名下还没发布的载体（`待批准`与`已批准`），不带正文，每份带内容摘要、录入者与时刻、
// 状态与批准痕迹，批准者据此选一份去翻看。挂操作者查阅读——读的是租户自己的在途工作，不是写面；批准与发布仍各走各的登记写口。
// 空列表答 200 加空数组（空是内容，不是缺席）。
func NewQueryPublicationDraftsEndpoint(intake CommercialCatalogueIntake, reader PublicationDraftReader) http.Handler {
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
		drafts, err := reader.ListOpenDrafts(request.Context(), query.Scope.Tenant(), query.Limit)
		if err != nil {
			// 读不回是答案未形成，不是「没有待批准的」：伪装成空列表会让批准者以为没有活要干。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		bodies := make([]publicationDraftBody, 0, len(drafts))
		for _, draft := range drafts {
			bodies = append(bodies, publicationDraftBodyOf(draft, false))
		}
		writeJSON(response, http.StatusOK, publicationDraftListResponse{Outcome: outcomePublicationDraftsListed, Drafts: bodies})
	})
}

// NewQueryPublicationDraftEndpoint 交回单份载体读回的 HTTP 入口
// （GET /commercial-publication-draft-readbacks/{kind}/{objectId}/{version}，ADR-0314 决定一）。
//
// 它交出载体的全部：壳、状态与痕迹，加规范化文档——正是内容摘要盖住的那份内容，批准者看到的就是他批准的东西。载体不在答
// 200 加 `DRAFT_NOT_FOUND`（ADR-0022：状态码只说答案有没有形成，不在册是业务答案）；跨租户与不在册同形，不泄露存在性。
func NewQueryPublicationDraftEndpoint(intake CommercialCatalogueIntake, reader PublicationDraftReader) http.Handler {
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
		kind, known := domain.CommercialObjectKindNamed(request.PathValue(draftKindPathValue))
		objectID, objectErr := domain.NewCommercialObjectID(request.PathValue(draftObjectIDPathValue))
		version, versionErr := domain.NewCommercialVersionLabel(request.PathValue(draftVersionPathValue))
		if !known || objectErr != nil || versionErr != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		draft, found, err := reader.LoadDraft(request.Context(), query.Scope.Tenant(), kind, objectID, version)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if !found {
			writeJSON(response, http.StatusOK, publicationDraftReadResponse{Outcome: outcomePublicationDraftAbsent})
			return
		}
		body := publicationDraftBodyOf(draft, true)
		writeJSON(response, http.StatusOK, publicationDraftReadResponse{Outcome: outcomePublicationDraftRead, Draft: &body})
	})
}

type publicationDraftListResponse struct {
	Outcome string                 `json:"outcome"`
	Drafts  []publicationDraftBody `json:"drafts"`
}

type publicationDraftReadResponse struct {
	Outcome string                `json:"outcome"`
	Draft   *publicationDraftBody `json:"draft,omitempty"`
}

// publicationDraftBody 逐字段透出一份载体。录入者与批准者是操作者主体引用原串（信封里的 issuer#subject），租户内的操作者
// 本来就互相可见；批准与发布的痕迹只在已到那一格的载体上在场，判据同库上 CHECK（批准者与时刻成对、发布时刻只在已发布）。
// content 只在单份读回上带（列表不带正文，免得列表随载体数膨胀），是规范化文档的原字节——JSON，键名镜像批文。
type publicationDraftBody struct {
	Kind              string                      `json:"kind"`
	ObjectID          string                      `json:"objectId"`
	Version           string                      `json:"version"`
	Scope             string                      `json:"scope"`
	EffectiveStartsAt string                      `json:"effectiveStartsAt"`
	EffectiveEndsAt   string                      `json:"effectiveEndsAt,omitempty"`
	References        []publicationDraftReference `json:"references"`
	Canonicalization  string                      `json:"canonicalization"`
	ContentDigest     string                      `json:"contentDigest"`
	Submitter         string                      `json:"submitter"`
	SubmittedAt       string                      `json:"submittedAt"`
	Status            string                      `json:"status"`
	Approver          string                      `json:"approver,omitempty"`
	ApprovedAt        string                      `json:"approvedAt,omitempty"`
	PublishedAt       string                      `json:"publishedAt,omitempty"`
	Content           json.RawMessage             `json:"content,omitempty"`
}

type publicationDraftReference struct {
	Kind     string `json:"kind"`
	ObjectID string `json:"objectId"`
}

func publicationDraftBodyOf(draft domain.PublicationDraft, withContent bool) publicationDraftBody {
	canonical := draft.Canonical()
	references := draft.DeclaredReferences()
	body := publicationDraftBody{
		Kind:              draft.Kind().String(),
		ObjectID:          draft.ObjectID().String(),
		Version:           draft.Version().String(),
		Scope:             draft.Scope().String(),
		EffectiveStartsAt: rfc3339(draft.Effective().StartsAt()),
		References:        make([]publicationDraftReference, 0, len(references)),
		Canonicalization:  canonical.Canonicalization(),
		ContentDigest:     canonical.Digest().String(),
		Submitter:         draft.Submitter().String(),
		SubmittedAt:       rfc3339(draft.SubmittedAt()),
		Status:            draft.Status().String(),
	}
	if end, bounded := draft.Effective().EndsAt(); bounded {
		body.EffectiveEndsAt = rfc3339(end)
	}
	for _, reference := range references {
		body.References = append(body.References, publicationDraftReference{
			Kind: reference.Kind().String(), ObjectID: reference.ObjectID().String(),
		})
	}
	if approver, approved := draft.Approver(); approved {
		body.Approver = approver.String()
	}
	if at, approved := draft.ApprovedAt(); approved {
		body.ApprovedAt = rfc3339(at)
	}
	if at, published := draft.PublishedAt(); published {
		body.PublishedAt = rfc3339(at)
	}
	if withContent {
		body.Content = json.RawMessage(canonical.Document())
	}
	return body
}
