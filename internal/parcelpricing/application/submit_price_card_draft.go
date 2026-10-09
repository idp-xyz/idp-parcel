package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 价卡录入（ADR-0101 决定三；票 price-card-import/03）：把一份上传文件读成草稿、落进草稿册。读法与预览走同一段
// readPriceCardImport——同一份字节过两口逐字节同摘要；读法决定草稿进哪一格，同版再录的落点由领域判、册上落定。
// 身份（租户、录入者）由 Intake 从操作者信封交进命令，编排不从任何载荷里读身份。

// SubmitPriceCardDraftOutcome 是录入的答复：草稿册四格落点逐名翻译，外加没落行的`未受理`。
type SubmitPriceCardDraftOutcome uint8

const (
	SubmitPriceCardDraftOutcomeInvalid SubmitPriceCardDraftOutcome = iota
	// PriceCardDraftSubmitted：这一版第一次录入，落了新的一行。
	PriceCardDraftSubmitted
	// PriceCardDraftReplayed：同版同内容再录，行一字不动。
	PriceCardDraftReplayed
	// PriceCardDraftRevised：`已批准`之前换了内容，那一行被替换。
	PriceCardDraftRevised
	// PriceCardDraftContentFixed：那一版已批准或已发布，内容固定；换内容要另起版本号。
	PriceCardDraftContentFixed
	// PriceCardDraftNotAccepted：命令立不住，或连方案身份都读不出来——没有行可落。
	PriceCardDraftNotAccepted
)

func (outcome SubmitPriceCardDraftOutcome) String() string {
	switch outcome {
	case PriceCardDraftSubmitted:
		return "DRAFT_SUBMITTED"
	case PriceCardDraftReplayed:
		return "DRAFT_REPLAYED"
	case PriceCardDraftRevised:
		return "DRAFT_REVISED"
	case PriceCardDraftContentFixed:
		return "CONTENT_FIXED"
	case PriceCardDraftNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// SubmitPriceCardDraftCommand 是一次录入：租户与录入者取自操作者信封，文件名与字节取自上传。
type SubmitPriceCardDraftCommand struct {
	Tenant    domain.TenantID
	Submitter string
	FileName  string
	Raw       []byte
}

// SubmitPriceCardDraftResult 是录入的结果。Reading 是同一份字节过预览会得到的那份读法；Draft 是本次录入立出的
// 草稿，`未受理`时没有。`内容已固定`时交回的是本次拟录的那份而不是册上的——调用方要的是「我交的这份读成什么」，
// 册上那份由查阅读口另答。
type SubmitPriceCardDraftResult struct {
	Outcome  SubmitPriceCardDraftOutcome
	Reading  PriceCardImportPreview
	Draft    domain.PriceCardDraft
	HasDraft bool
}

// SubmitPriceCardDraftHandler 依赖模板读口（与预览同一个）、草稿册与时钟。
type SubmitPriceCardDraftHandler struct {
	reader ports.PriceCardTemplateReader
	drafts ports.PriceCardDraftRegister
	clock  ports.Clock
}

func NewSubmitPriceCardDraftHandler(
	reader ports.PriceCardTemplateReader,
	drafts ports.PriceCardDraftRegister,
	clock ports.Clock,
) (*SubmitPriceCardDraftHandler, error) {
	if reader == nil || drafts == nil || clock == nil {
		return nil, errors.New("parcel pricing: price card draft submission needs a template reader, a draft register and a clock")
	}
	return &SubmitPriceCardDraftHandler{reader: reader, drafts: drafts, clock: clock}, nil
}

// Handle 读文件、立草稿、落册。缺录入者与缺租户同属命令立不住，不读文件。草稿立不起来、或草稿册读写失败，都不是
// 业务答案而是上抛的错：前者是读口交出了领域不认的读法，后者是依赖故障，两者都不该折成某一格落点。
func (handler *SubmitPriceCardDraftHandler) Handle(ctx context.Context, command SubmitPriceCardDraftCommand) (SubmitPriceCardDraftResult, error) {
	if strings.TrimSpace(command.Submitter) == "" {
		return SubmitPriceCardDraftResult{
			Outcome: PriceCardDraftNotAccepted,
			Reading: PriceCardImportPreview{Outcome: PriceCardImportNotAccepted},
		}, nil
	}
	reading := readPriceCardImport(handler.reader, command.Tenant, command.FileName, command.Raw)
	if reading.Outcome == PriceCardImportNotAccepted {
		return SubmitPriceCardDraftResult{Outcome: PriceCardDraftNotAccepted, Reading: reading}, nil
	}
	submission, err := draftSubmissionOf(reading, command, handler.clock.Now())
	if err != nil {
		return SubmitPriceCardDraftResult{}, fmt.Errorf("submit price card draft: %w", err)
	}
	draft, err := domain.SubmitPriceCardDraft(submission)
	if err != nil {
		return SubmitPriceCardDraftResult{}, fmt.Errorf("submit price card draft: %w", err)
	}
	saved, err := handler.drafts.SubmitDraft(ctx, draft)
	if err != nil {
		return SubmitPriceCardDraftResult{}, fmt.Errorf("submit price card draft: %w", err)
	}
	result := SubmitPriceCardDraftResult{Reading: reading, Draft: draft, HasDraft: true}
	switch saved {
	case ports.PriceCardDraftSaved:
		result.Outcome = PriceCardDraftSubmitted
	case ports.PriceCardDraftReplayed:
		result.Outcome = PriceCardDraftReplayed
	case ports.PriceCardDraftRevised:
		result.Outcome = PriceCardDraftRevised
	case ports.PriceCardDraftContentFixed:
		result.Outcome = PriceCardDraftContentFixed
	default:
		return SubmitPriceCardDraftResult{}, fmt.Errorf("submit price card draft: unexpected register outcome %q", saved)
	}
	return result, nil
}

// draftSubmissionOf 把读法折成交给草稿册的录入：读通了带内容，否则带读口交出的逐格问题。
func draftSubmissionOf(reading PriceCardImportPreview, command SubmitPriceCardDraftCommand, at time.Time) (domain.PriceCardDraftSubmission, error) {
	submission := domain.PriceCardDraftSubmission{
		Tenant:      command.Tenant,
		Plan:        reading.Plan,
		Source:      reading.SourceFile,
		Submitter:   command.Submitter,
		SubmittedAt: at,
	}
	if reading.Content != nil {
		submission.Content = &domain.PriceCardDraftContent{
			Plan:                   reading.Content.Plan,
			DirectionAuthorization: reading.Content.DirectionAuthorization,
		}
		return submission, nil
	}
	for _, problem := range reading.Problems {
		recorded, err := domain.NewPriceCardDraftProblem(problem.Sheet, problem.Row, problem.Column, string(problem.Code), problem.Message)
		if err != nil {
			return domain.PriceCardDraftSubmission{}, err
		}
		submission.Problems = append(submission.Problems, recorded)
	}
	return submission, nil
}
