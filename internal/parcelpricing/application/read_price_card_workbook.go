package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 已登记价卡（或带内容的草稿）按方案标识与版本读回成工作簿。已登记的优先：那是册上的权威内容。
// 没有登记再看草稿。草稿没过校验时册上只有问题清单，不编一份残表出来。
// 模板表达不了的结构由投影答一处业务结论，这里原样交回，不改写成 500，也不改去草稿里找一份更窄的。

// ReadPriceCardWorkbookOutcome 是这一口的业务结论。
type ReadPriceCardWorkbookOutcome uint8

const (
	ReadPriceCardWorkbookOutcomeInvalid ReadPriceCardWorkbookOutcome = iota
	// PriceCardWorkbookReady：交回了一份工作簿。
	PriceCardWorkbookReady
	// PriceCardWorkbookNotRepresentable：卡在册（或草稿有内容），但含导入模板表达不了的结构。
	PriceCardWorkbookNotRepresentable
	// PriceCardWorkbookNotFound：登记册与草稿册都没有这一版。
	PriceCardWorkbookNotFound
	// PriceCardWorkbookDraftWithoutContent：只有一份没过校验的草稿，册上是问题清单，没有整份内容。
	PriceCardWorkbookDraftWithoutContent
	// PriceCardWorkbookCanonicalizationUnsupported：册上有这一版，本构建按它的规范化版本重建不了。
	PriceCardWorkbookCanonicalizationUnsupported
)

func (outcome ReadPriceCardWorkbookOutcome) String() string {
	switch outcome {
	case PriceCardWorkbookReady:
		return "WORKBOOK"
	case PriceCardWorkbookNotRepresentable:
		return "NOT_REPRESENTABLE"
	case PriceCardWorkbookNotFound:
		return "NOT_FOUND"
	case PriceCardWorkbookDraftWithoutContent:
		return "DRAFT_WITHOUT_CONTENT"
	case PriceCardWorkbookCanonicalizationUnsupported:
		return "CANONICALIZATION_UNSUPPORTED"
	default:
		return ""
	}
}

// PriceCardWorkbookOrigin 说这份工作簿从登记册来还是从草稿册来。
type PriceCardWorkbookOrigin uint8

const (
	PriceCardWorkbookOriginInvalid PriceCardWorkbookOrigin = iota
	PriceCardWorkbookFromRegister
	PriceCardWorkbookFromDraft
)

func (origin PriceCardWorkbookOrigin) String() string {
	switch origin {
	case PriceCardWorkbookFromRegister:
		return "REGISTERED"
	case PriceCardWorkbookFromDraft:
		return "DRAFT"
	default:
		return ""
	}
}

// ReadPriceCardWorkbookQuery 是一次读回。租户只来自操作者信封。载体由读口选，不改投影的判定。
type ReadPriceCardWorkbookQuery struct {
	Tenant  domain.TenantID
	Plan    domain.VersionReference
	Carrier ports.PriceCardWorkbookCarrier
}

// ReadPriceCardWorkbookResult 是读回的答案。Body 只在 WORKBOOK 时有：结构化工作簿的 JSON，或电子表格字节。
type ReadPriceCardWorkbookResult struct {
	Outcome          ReadPriceCardWorkbookOutcome
	Origin           PriceCardWorkbookOrigin
	PlanID           string
	PlanVersion      string
	TemplateVersion  string
	Canonicalization string
	ContentDigest    string
	Where            string
	Conclusion       string
	Body             []byte
	MediaType        string
	FileName         string
}

// PriceCardDraftByReference 按方案版本读一份草稿。批准与发布的写口也有这一读，读回工作簿只用读的那一半。
type PriceCardDraftByReference interface {
	LoadPriceCardDraft(ctx context.Context, tenant domain.TenantID, plan domain.VersionReference) (domain.PriceCardDraft, bool, error)
}

// ReadPriceCardWorkbookHandler 编排登记册、草稿册与模板投影。它不读 HTTP，也不自己译表。
type ReadPriceCardWorkbookHandler struct {
	registered ports.RegisteredPriceCardRead
	drafts     PriceCardDraftByReference
	projector  ports.PriceCardWorkbookProjector
}

func NewReadPriceCardWorkbookHandler(
	registered ports.RegisteredPriceCardRead,
	drafts PriceCardDraftByReference,
	projector ports.PriceCardWorkbookProjector,
) (*ReadPriceCardWorkbookHandler, error) {
	if registered == nil || drafts == nil || projector == nil {
		return nil, fmt.Errorf("parcel pricing: price card workbook read needs the register, the draft book and the projector")
	}
	return &ReadPriceCardWorkbookHandler{registered: registered, drafts: drafts, projector: projector}, nil
}

func (handler *ReadPriceCardWorkbookHandler) Handle(
	ctx context.Context,
	query ReadPriceCardWorkbookQuery,
) (ReadPriceCardWorkbookResult, error) {
	result := ReadPriceCardWorkbookResult{PlanID: query.Plan.ID(), PlanVersion: query.Plan.Version()}
	if query.Tenant.String() == "" || query.Plan.Kind() != domain.ArtifactPricingPlan || query.Plan.ID() == "" || query.Plan.Version() == "" {
		return result, fmt.Errorf("parcel pricing: price card workbook read needs a tenant and a pricing-plan reference")
	}
	switch query.Carrier {
	case ports.PriceCardWorkbookStructured, ports.PriceCardWorkbookSpreadsheet:
	default:
		return result, fmt.Errorf("parcel pricing: price card workbook read was given carrier %s", query.Carrier)
	}

	subject, found, err := handler.registered.ReadRegisteredPriceCard(ctx, query.Tenant, query.Plan)
	switch {
	case errors.Is(err, domain.ErrCanonicalizationVersionUnsupported):
		result.Outcome = PriceCardWorkbookCanonicalizationUnsupported
		result.Origin = PriceCardWorkbookFromRegister
		result.Conclusion = "这一版已登记，但它的规范化版本本构建重建不了"
		return result, nil
	case err != nil:
		return result, err
	case found:
		return handler.project(query, subject, PriceCardWorkbookFromRegister)
	}

	draft, found, err := handler.drafts.LoadPriceCardDraft(ctx, query.Tenant, query.Plan)
	if err != nil {
		return result, err
	}
	if !found {
		result.Outcome = PriceCardWorkbookNotFound
		return result, nil
	}
	content, hasContent := draft.Content()
	if !hasContent {
		result.Outcome = PriceCardWorkbookDraftWithoutContent
		result.Origin = PriceCardWorkbookFromDraft
		result.Where, result.Conclusion = draftProblem(draft)
		return result, nil
	}
	return handler.project(query, ports.PriceCardWorkbookSubject{
		Plan: content.Plan, DirectionAuthorization: content.DirectionAuthorization,
	}, PriceCardWorkbookFromDraft)
}

func (handler *ReadPriceCardWorkbookHandler) project(
	query ReadPriceCardWorkbookQuery,
	subject ports.PriceCardWorkbookSubject,
	origin PriceCardWorkbookOrigin,
) (ReadPriceCardWorkbookResult, error) {
	result := ReadPriceCardWorkbookResult{
		Origin:           origin,
		PlanID:           subject.Plan.Reference().ID(),
		PlanVersion:      subject.Plan.Reference().Version(),
		Canonicalization: subject.Plan.CanonicalizationVersion(),
		ContentDigest:    subject.Plan.ContentDigest(),
	}
	projection, err := handler.projector.ProjectPriceCardWorkbook(subject.Plan, subject.DirectionAuthorization, query.Carrier)
	if err != nil {
		return result, err
	}
	if projection.Gap.Where != "" {
		result.Outcome = PriceCardWorkbookNotRepresentable
		result.Where = projection.Gap.Where
		result.Conclusion = projection.Gap.Conclusion
		return result, nil
	}
	result.Outcome = PriceCardWorkbookReady
	result.TemplateVersion = projection.Document.TemplateVersion
	result.Body = projection.Document.Body
	result.MediaType = projection.Document.MediaType
	result.FileName = projection.Document.FileName
	return result, nil
}

// draftProblem 点出问题清单的第一处。整份没有可编辑的内容，不把问题逐条重抄成另一份清单。
func draftProblem(draft domain.PriceCardDraft) (string, string) {
	problems := draft.Problems()
	if len(problems) == 0 {
		return "草稿", "这一版没过校验，草稿没有整份价卡"
	}
	first := problems[0]
	where := first.Sheet()
	if column := first.Column(); column != "" {
		if where != "" {
			where += "."
		}
		where += column
	}
	if where == "" {
		where = "草稿"
	}
	return where, "这一版没过校验，草稿只记下了问题清单（先是" + first.Message() + "），没有可交回的整份价卡"
}
