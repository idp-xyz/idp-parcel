package commercialhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

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
	ListOpenDrafts(ctx context.Context, tenant domain.TenantID, limit int) ([]ports.OpenPublicationDraft, error)
}

// 编译期锁缝：读口形状与端口保持一致。
var _ PublicationDraftReader = ports.PublicationDraftRead(nil)

const (
	outcomePublicationDraftsListed = "PUBLICATION_DRAFTS_LISTED"
	outcomePublicationDraftRead    = "PUBLICATION_DRAFT_READ"
	outcomePublicationDraftAbsent  = "DRAFT_NOT_FOUND"
	// 载体在册、本构建折不回它的正文：单份读回交这一格加库上剩下的事实（没有 content），不答 5xx。
	outcomePublicationDraftNotReadable = "DRAFT_NOT_READABLE"

	// 路由模式里的路径参数名；装配点的 Pattern 与这里必须同字，否则 PathValue 恒空、端点恒答 400。
	draftKindPathValue     = "kind"
	draftObjectIDPathValue = "objectId"
	draftVersionPathValue  = "version"
)

// NewQueryPublicationDraftsEndpoint 交回待批准发布载体列表的 HTTP 入口（GET /commercial-open-publication-drafts，ADR-0314 决定一）。
//
// 它是管理台「待批准」列表的供数面：交出租户名下还没发布的载体（`待批准`与`已批准`），不带正文，每份带内容摘要、录入者与时刻、
// 状态与批准痕迹，批准者据此选一份去翻看。挂操作者查阅读——读的是租户自己的在途工作，不是写面；批准与发布仍各走各的登记写口。
// 空列表答 200 加空数组（空是内容，不是缺席）。本构建折不回正文的载体照样在列表里，`readable` 为假并带成因 `unreadableCause`，
// 其余字段取自库上的列（含列上的规范化版本与摘要）：批准者看得见这一份在、动不了它，不会因为一行读不回而整页答不出。
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
		for _, open := range drafts {
			if draft, readable := open.Draft(); readable {
				bodies = append(bodies, publicationDraftBodyOf(draft, false))
			} else if unreadable, ok := open.Unreadable(); ok {
				bodies = append(bodies, unreadablePublicationDraftBodyOf(unreadable))
			}
		}
		writeJSON(response, http.StatusOK, publicationDraftListResponse{Outcome: outcomePublicationDraftsListed, Drafts: bodies})
	})
}

// NewQueryPublicationDraftEndpoint 交回单份载体读回的 HTTP 入口
// （GET /commercial-publication-draft-readbacks/{kind}/{objectId}/{version}，ADR-0314 决定一）。
//
// 它交出载体的全部：壳、状态与痕迹，加规范化文档——正是内容摘要盖住的那份内容，批准者看到的就是他批准的东西。载体不在答
// 200 加 `DRAFT_NOT_FOUND`（ADR-0022：状态码只说答案有没有形成，不在册是业务答案）；跨租户与不在册同形，不泄露存在性。
// 载体在册而本构建折不回它的正文，答 200 加 `DRAFT_NOT_READABLE`，`draft` 只带库上的事实、不带 `content`（没有正文可给，
// 也不拿一份折不回的文档冒充摘要盖住的内容）。
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
		var unreadable *domain.UnreadablePublicationDraft
		if errors.As(err, &unreadable) {
			body := unreadablePublicationDraftBodyOf(unreadable)
			writeJSON(response, http.StatusOK, publicationDraftReadResponse{Outcome: outcomePublicationDraftNotReadable, Draft: &body})
			return
		}
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
// `readable` 为假的是本构建折不回正文的载体：其余字段取自库上的列，`canonicalization` 与 `contentDigest` 是列上的原值（可以是
// 本构建不认得的规范化版本），`unreadableCause` 说为什么读不回；这样的载体没有 `content`，批准与发布都会答 `DRAFT_NOT_READABLE`。
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
	ShellDigest       string                      `json:"shellDigest"`
	Submitter         string                      `json:"submitter"`
	SubmittedAt       string                      `json:"submittedAt"`
	Status            string                      `json:"status"`
	Approver          string                      `json:"approver,omitempty"`
	ApprovedAt        string                      `json:"approvedAt,omitempty"`
	PublishedAt       string                      `json:"publishedAt,omitempty"`
	Readable          bool                        `json:"readable"`
	UnreadableCause   string                      `json:"unreadableCause,omitempty"`
	Content           json.RawMessage             `json:"content,omitempty"`
}

type publicationDraftReference struct {
	Kind     string `json:"kind"`
	ObjectID string `json:"objectId"`
}

func publicationDraftBodyOf(draft domain.PublicationDraft, withContent bool) publicationDraftBody {
	canonical := draft.Canonical()
	body := publicationDraftFactsBody(draft, canonical.Canonicalization(), canonical.Digest().String())
	body.Readable = true
	if withContent {
		body.Content = json.RawMessage(canonical.Document())
	}
	return body
}

func unreadablePublicationDraftBodyOf(unreadable *domain.UnreadablePublicationDraft) publicationDraftBody {
	body := publicationDraftFactsBody(unreadable, unreadable.Canonicalization(), unreadable.ContentDigest().String())
	body.UnreadableCause = unreadable.Cause().Error()
	return body
}

// publicationDraftFacts 是读得回的载体与读不回的行共有的那一面：壳、状态与各痕迹。两边的字段在响应里不分叉，靠的是
// 两者走同一个映射而不是各抄一遍。
type publicationDraftFacts interface {
	Kind() domain.CommercialObjectKind
	ObjectID() domain.CommercialObjectID
	Version() domain.CommercialVersionLabel
	Scope() domain.CommercialScopeReference
	Effective() domain.EffectiveInterval
	DeclaredReferences() []domain.DeclaredReference
	ShellDigest() string
	Submitter() domain.OperatorSubjectReference
	SubmittedAt() time.Time
	Status() domain.PublicationDraftStatus
	Approver() (domain.OperatorSubjectReference, bool)
	ApprovedAt() (time.Time, bool)
	PublishedAt() (time.Time, bool)
}

func publicationDraftFactsBody(draft publicationDraftFacts, canonicalization, contentDigest string) publicationDraftBody {
	references := draft.DeclaredReferences()
	body := publicationDraftBody{
		Kind:              draft.Kind().String(),
		ObjectID:          draft.ObjectID().String(),
		Version:           draft.Version().String(),
		Scope:             draft.Scope().String(),
		EffectiveStartsAt: rfc3339(draft.Effective().StartsAt()),
		References:        make([]publicationDraftReference, 0, len(references)),
		Canonicalization:  canonicalization,
		ContentDigest:     contentDigest,
		ShellDigest:       draft.ShellDigest(),
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
	return body
}
