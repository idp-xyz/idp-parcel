package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// 本文件是价卡草稿册的一行（ADR-0101 决定三；票 price-card-import/03）：一次导入立下的一份草稿，键为
// 租户 × 方案标识 × 方案版本，一版一行。它不是价卡版本——不进版本清单，任何评价读不到它；发布把已批准的
// 草稿交给既有的价卡登记（ADR-0101 决定五）。
//
// 状态四格对齐 CONTEXT「定价方案与价表版本」生命周期（spec 自决第 1 格）。前两格只由录入进入，由读口的读法
// 决定、不由调用方声明：读通了是`已校验`，带过了构造门的方案与方向授权引用——登记要的除批准者外一样不缺；
// 没读通是`草稿`，只记逐格问题。后两格只从前一格推进而来，推进动作不在本文件；这里立它们的痕迹与读回门。
//
// 草稿与已发布版本是两条持久化路径，所以有自己的一扇门（判据见 plan_snapshot.go 头注）；方案部分复用登记
// 文档那组折装函数，不另立第二个口径。

var (
	// ErrInvalidPriceCardDraft 表示一次录入立不成草稿，或拿一份不是同一版新录入的草稿去问落点。
	ErrInvalidPriceCardDraft = errors.New("parcel pricing: invalid price card draft")
	// ErrInvalidRehydratedPriceCardDraft 表示库里那一行不可能是录入或推进出来的：处置是去查那一行或写它的适配器，
	// 与 ErrCanonicalizationVersionUnsupported 分格——后者是行好好的、本构建算不了它的摘要。
	ErrInvalidRehydratedPriceCardDraft = errors.New("parcel pricing: invalid rehydrated price card draft")
)

// PriceCardDraftStatus 是草稿的状态四格。
type PriceCardDraftStatus uint8

const (
	PriceCardDraftStatusInvalid PriceCardDraftStatus = iota
	// PriceCardDraftStatusDraft：解析了但没过构造门，只记逐格问题，不带方案快照。
	PriceCardDraftStatusDraft
	// PriceCardDraftStatusValidated：过了构造门，方案快照与规范化摘要已算出。
	PriceCardDraftStatusValidated
	// PriceCardDraftStatusApproved：业务责任方已批准它进入受控发布。
	PriceCardDraftStatusApproved
	// PriceCardDraftStatusPublished：已交登记、进了版本清单。
	PriceCardDraftStatusPublished
)

func (status PriceCardDraftStatus) String() string {
	switch status {
	case PriceCardDraftStatusDraft:
		return "DRAFT"
	case PriceCardDraftStatusValidated:
		return "VALIDATED"
	case PriceCardDraftStatusApproved:
		return "APPROVED"
	case PriceCardDraftStatusPublished:
		return "PUBLISHED"
	default:
		return ""
	}
}

// ParsePriceCardDraftStatus 按名字认一格状态：库上的列与查阅读口的筛选写的都是名字。
func ParsePriceCardDraftStatus(name string) (PriceCardDraftStatus, bool) {
	for status := PriceCardDraftStatusDraft; status <= PriceCardDraftStatusPublished; status++ {
		if status.String() == name {
			return status, true
		}
	}
	return PriceCardDraftStatusInvalid, false
}

func (status PriceCardDraftStatus) valid() bool {
	return status >= PriceCardDraftStatusDraft && status <= PriceCardDraftStatusPublished
}

func (status PriceCardDraftStatus) carriesContent() bool {
	return status >= PriceCardDraftStatusValidated && status <= PriceCardDraftStatusPublished
}

// PriceCardDraftProblem 是`草稿`上的一条逐格问题。坐标与码照模板读口交出的原样记：码的封闭集合与各码的情形归
// 导入模板规范（docs/design/pp-price-card-import-template-and-validation-spec.md 第六节），领域只守形状。
type PriceCardDraftProblem struct {
	sheet   string
	row     int
	column  string
	code    string
	message string
}

// NewPriceCardDraftProblem 立一条问题。表名与列键可空：整份或整表层面的问题指不到那一格。
func NewPriceCardDraftProblem(sheet string, row int, column, code, message string) (PriceCardDraftProblem, error) {
	problem := PriceCardDraftProblem{sheet: sheet, row: row, column: column, code: code, message: message}
	if !problem.valid() {
		return PriceCardDraftProblem{}, fmt.Errorf("%w：逐格问题要有码与说明，行号不能为负", ErrInvalidPriceCardDraft)
	}
	return problem, nil
}

func (problem PriceCardDraftProblem) Sheet() string   { return problem.sheet }
func (problem PriceCardDraftProblem) Row() int        { return problem.row }
func (problem PriceCardDraftProblem) Column() string  { return problem.column }
func (problem PriceCardDraftProblem) Code() string    { return problem.code }
func (problem PriceCardDraftProblem) Message() string { return problem.message }

func (problem PriceCardDraftProblem) valid() bool {
	return problem.row >= 0 && trimmed(problem.code) && strings.TrimSpace(problem.message) != ""
}

// PriceCardDraftContent 是过了构造门的那一张卡：方案，加登记要的方向授权引用（party-commercial 签发，这里只引用）。
type PriceCardDraftContent struct {
	Plan                   PricingPlanVersion
	DirectionAuthorization VersionReference
}

func (content PriceCardDraftContent) valid() bool {
	return content.Plan.valid() &&
		content.DirectionAuthorization.kind == ArtifactCommercialAuthorization &&
		content.DirectionAuthorization.valid()
}

// PriceCardDraftSubmission 是一次录入交给草稿册的全部事实：租户与录入者取自操作者信封，源文件身份按上传字节
// 算出，方案身份与内容或逐格问题取自模板读口。Content 与 Problems 恰有一样。
type PriceCardDraftSubmission struct {
	Tenant      TenantID
	Plan        VersionReference
	Source      SourceFileIdentity
	Content     *PriceCardDraftContent
	Problems    []PriceCardDraftProblem
	Submitter   string
	SubmittedAt time.Time
}

// PriceCardDraft 是草稿册的一行。值类型：读回与推进都交回新值，不改接收者。
type PriceCardDraft struct {
	tenant      TenantID
	plan        VersionReference
	status      PriceCardDraftStatus
	source      SourceFileIdentity
	content     PriceCardDraftContent
	problems    []PriceCardDraftProblem
	submitter   string
	submittedAt time.Time
	approver    string
	approvedAt  time.Time
	publishedAt time.Time
}

// SubmitPriceCardDraft 由一次录入立一份草稿：带内容的进`已校验`，带问题的进`草稿`，两样都带或都不带的立不住。
func SubmitPriceCardDraft(submission PriceCardDraftSubmission) (PriceCardDraft, error) {
	draft := PriceCardDraft{
		tenant:      submission.Tenant,
		plan:        submission.Plan,
		source:      submission.Source,
		problems:    append([]PriceCardDraftProblem(nil), submission.Problems...),
		submitter:   submission.Submitter,
		submittedAt: submission.SubmittedAt.UTC(),
	}
	switch {
	case submission.Content != nil && len(submission.Problems) == 0:
		draft.status, draft.content = PriceCardDraftStatusValidated, *submission.Content
	case submission.Content == nil && len(submission.Problems) > 0:
		draft.status = PriceCardDraftStatusDraft
	default:
		return PriceCardDraft{}, fmt.Errorf("%w：一次录入带内容或带逐格问题，恰有一样", ErrInvalidPriceCardDraft)
	}
	if reason := draft.incoherence(); reason != "" {
		return PriceCardDraft{}, fmt.Errorf("%w：%s", ErrInvalidPriceCardDraft, reason)
	}
	return draft, nil
}

// incoherence 答这份草稿哪里立不住，空串即立得住。录入与重建共用这一道判据：录入只进得了前两格，重建四格都查。
func (draft PriceCardDraft) incoherence() string {
	switch {
	case !draft.status.valid():
		return "状态不在四格里"
	case !draft.tenant.valid():
		return "缺租户"
	case draft.plan.kind != ArtifactPricingPlan || !draft.plan.valid():
		return "键不是一条方案版本引用"
	case !draft.source.valid():
		return "源文件身份立不住"
	case !trimmed(draft.submitter) || draft.submittedAt.IsZero():
		return "缺录入者或录入时刻"
	}
	if draft.status.carriesContent() {
		switch {
		case !draft.content.valid():
			return "内容立不住：方案没过构造门，或方向授权引用不是授权工件"
		case !draft.content.Plan.reference.SameIdentity(draft.plan):
			return "内容里的方案不是键上那一版"
		case len(draft.problems) != 0:
			return "带内容的草稿还记着逐格问题"
		}
	} else {
		if len(draft.problems) == 0 {
			return "`草稿`一条逐格问题也没有"
		}
		for _, problem := range draft.problems {
			if !problem.valid() {
				return "逐格问题缺码、缺说明或行号为负"
			}
		}
	}
	return draft.traceIncoherence()
}

// traceIncoherence 按格查批准与发布的痕迹：批准者与批准时刻自`已批准`起在场且批准不早于录入，发布时刻只在
// `已发布`在场且不早于批准。
func (draft PriceCardDraft) traceIncoherence() string {
	approved := trimmed(draft.approver) && !draft.approvedAt.IsZero() && !draft.approvedAt.Before(draft.submittedAt)
	switch draft.status {
	case PriceCardDraftStatusDraft, PriceCardDraftStatusValidated:
		if draft.approver != "" || !draft.approvedAt.IsZero() || !draft.publishedAt.IsZero() {
			return "还没批准的草稿带着批准或发布痕迹"
		}
	case PriceCardDraftStatusApproved:
		if !approved {
			return "已批准的草稿缺批准者或批准时刻，或批准早于录入"
		}
		if !draft.publishedAt.IsZero() {
			return "已批准未发布的草稿带着发布时刻"
		}
	case PriceCardDraftStatusPublished:
		if !approved {
			return "已发布的草稿缺批准者或批准时刻，或批准早于录入"
		}
		if draft.publishedAt.IsZero() || draft.publishedAt.Before(draft.approvedAt) {
			return "已发布的草稿缺发布时刻，或发布早于批准"
		}
	}
	return ""
}

func (draft PriceCardDraft) Tenant() TenantID               { return draft.tenant }
func (draft PriceCardDraft) Plan() VersionReference         { return draft.plan }
func (draft PriceCardDraft) Status() PriceCardDraftStatus   { return draft.status }
func (draft PriceCardDraft) SourceFile() SourceFileIdentity { return draft.source }
func (draft PriceCardDraft) Submitter() string              { return draft.submitter }
func (draft PriceCardDraft) SubmittedAt() time.Time         { return draft.submittedAt }

// Content 交回过了构造门的那张卡；`草稿`没有内容，第二个返回值为假。
func (draft PriceCardDraft) Content() (PriceCardDraftContent, bool) {
	if !draft.status.carriesContent() {
		return PriceCardDraftContent{}, false
	}
	return draft.content, true
}

// Problems 交回逐格问题（副本）；只有`草稿`有。
func (draft PriceCardDraft) Problems() []PriceCardDraftProblem {
	return append([]PriceCardDraftProblem(nil), draft.problems...)
}

func (draft PriceCardDraft) Approver() (string, bool) {
	return draft.approver, draft.approver != ""
}

func (draft PriceCardDraft) ApprovedAt() (time.Time, bool) {
	return draft.approvedAt, !draft.approvedAt.IsZero()
}

func (draft PriceCardDraft) PublishedAt() (time.Time, bool) {
	return draft.publishedAt, !draft.publishedAt.IsZero()
}

// PriceCardDraftResubmission 是同一版再录一次时那一行的落点。
type PriceCardDraftResubmission uint8

const (
	PriceCardDraftResubmissionInvalid PriceCardDraftResubmission = iota
	// PriceCardDraftResubmissionReplay：同版同内容再录，行一字不动——不论草稿此刻在哪一格。
	PriceCardDraftResubmissionReplay
	// PriceCardDraftResubmissionRevision：`已批准`之前换了内容，替换那一行，内容、录入者与录入时刻随之更新。
	PriceCardDraftResubmissionRevision
	// PriceCardDraftResubmissionContentFixed：草稿已`已批准`或`已发布`，内容固定；换内容要另起版本号。
	PriceCardDraftResubmissionContentFixed
)

func (resubmission PriceCardDraftResubmission) String() string {
	switch resubmission {
	case PriceCardDraftResubmissionReplay:
		return "REPLAY"
	case PriceCardDraftResubmissionRevision:
		return "REVISION"
	case PriceCardDraftResubmissionContentFixed:
		return "CONTENT_FIXED"
	default:
		return ""
	}
}

// ResubmissionOf 答同一版再录一次时这一行怎么落（ADR-0126 决定三，价卡照搬）。接收者是册上那一行，incoming 是
// 这次录入立出的草稿：它必须是一份新录入（`草稿`或`已校验`）且与接收者同键——键不同的两份根本不会撞在一起，
// 交进来就是调用方的错。
func (draft PriceCardDraft) ResubmissionOf(incoming PriceCardDraft) (PriceCardDraftResubmission, error) {
	if reason := draft.incoherence(); reason != "" {
		return PriceCardDraftResubmissionInvalid, fmt.Errorf("%w：册上那一份立不住：%s", ErrInvalidPriceCardDraft, reason)
	}
	if reason := incoming.incoherence(); reason != "" {
		return PriceCardDraftResubmissionInvalid, fmt.Errorf("%w：这次录入立不住：%s", ErrInvalidPriceCardDraft, reason)
	}
	if incoming.status.carriesContent() && incoming.status != PriceCardDraftStatusValidated {
		return PriceCardDraftResubmissionInvalid, fmt.Errorf("%w：交来的不是一份新录入，而是 %s", ErrInvalidPriceCardDraft, incoming.status)
	}
	if incoming.tenant != draft.tenant || !incoming.plan.SameIdentity(draft.plan) {
		return PriceCardDraftResubmissionInvalid, fmt.Errorf("%w：交来的不是同一版", ErrInvalidPriceCardDraft)
	}
	switch {
	case draft.sameContentAs(incoming):
		return PriceCardDraftResubmissionReplay, nil
	case draft.status.carriesContent() && draft.status != PriceCardDraftStatusValidated:
		return PriceCardDraftResubmissionContentFixed, nil
	default:
		return PriceCardDraftResubmissionRevision, nil
	}
}

// sameContentAs 答两份草稿装的是不是同一份内容：源文件身份相同，且同为`草稿`时逐格问题逐条相同，同带内容时规范化
// 版本、内容摘要与方向授权引用都相同。录入者与时刻不是内容。源文件名算内容：它随登记进版本册，换了名就是另一条
// 证据索引。
func (draft PriceCardDraft) sameContentAs(other PriceCardDraft) bool {
	if draft.source != other.source || draft.status.carriesContent() != other.status.carriesContent() {
		return false
	}
	if draft.status.carriesContent() {
		mine, theirs := draft.content, other.content
		return mine.Plan.canonicalization == theirs.Plan.canonicalization &&
			mine.Plan.contentDigest == theirs.Plan.contentDigest &&
			mine.DirectionAuthorization.SameIdentity(theirs.DirectionAuthorization)
	}
	if len(draft.problems) != len(other.problems) {
		return false
	}
	for index := range draft.problems {
		if draft.problems[index] != other.problems[index] {
			return false
		}
	}
	return true
}

type priceCardDraftProblemSnapshot struct {
	Sheet   string `json:"sheet,omitempty"`
	Row     int    `json:"row"`
	Column  string `json:"column,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type priceCardDraftDocument struct {
	Plan                   *pricingPlanSnapshot            `json:"plan,omitempty"`
	DirectionAuthorization *versionReferenceSnapshot       `json:"directionAuthorization,omitempty"`
	Problems               []priceCardDraftProblemSnapshot `json:"problems,omitempty"`
}

// MarshalPriceCardDraftContent 把草稿的内容折成文档：带内容的是方案快照与方向授权引用，`草稿`是逐格问题。键、状态、
// 源文件身份、录入者与各格痕迹不进文档——它们在库上各有一列，推进状态只改那几列、不重写文档。
func MarshalPriceCardDraftContent(draft PriceCardDraft) ([]byte, error) {
	if reason := draft.incoherence(); reason != "" {
		return nil, fmt.Errorf("%w：%s", ErrInvalidPriceCardDraft, reason)
	}
	var document priceCardDraftDocument
	if content, has := draft.Content(); has {
		plan := pricingPlanDocumentOf(content.Plan)
		authorization := versionReferenceOf(content.DirectionAuthorization)
		document.Plan, document.DirectionAuthorization = &plan, &authorization
	}
	for _, problem := range draft.problems {
		document.Problems = append(document.Problems, priceCardDraftProblemSnapshot{
			Sheet: problem.sheet, Row: problem.row, Column: problem.column, Code: problem.code, Message: problem.message,
		})
	}
	return json.Marshal(document)
}

// RehydratePriceCardDraftSpec 是一行草稿在库里的样子：键、状态、源文件身份、内容文档、录入者与各格痕迹。
type RehydratePriceCardDraftSpec struct {
	Tenant      TenantID
	Plan        VersionReference
	Status      PriceCardDraftStatus
	Source      SourceFileIdentity
	Document    []byte
	Submitter   string
	SubmittedAt time.Time
	Approver    string
	ApprovedAt  time.Time
	PublishedAt time.Time
}

// RehydratePriceCardDraft 从库里读到的一行重建草稿：字段一律当数据收下，只校不变量（ADR-0028）。方案部分经登记文档
// 同一组折装函数读回、整图重验（含按规范化版本重算内容摘要自校）。规范化版本不是本构建的那一个时如实拒，那是
// 「结构上算不出来」不是行坏了——操作者按当前模板重新录入即可，草稿不是权威记录。
func RehydratePriceCardDraft(spec RehydratePriceCardDraftSpec) (PriceCardDraft, error) {
	var document priceCardDraftDocument
	if err := json.Unmarshal(spec.Document, &document); err != nil {
		return PriceCardDraft{}, rehydratedDraftRefusal(fmt.Sprintf("内容文档解不开：%v", err))
	}
	if hasContent := document.Plan != nil || document.DirectionAuthorization != nil; hasContent != spec.Status.carriesContent() {
		return PriceCardDraft{}, rehydratedDraftRefusal("内容与状态对不上：`草稿`不带方案，已校验起必带")
	}
	draft := PriceCardDraft{
		tenant:      spec.Tenant,
		plan:        spec.Plan,
		status:      spec.Status,
		source:      spec.Source,
		submitter:   spec.Submitter,
		submittedAt: spec.SubmittedAt.UTC(),
		approver:    spec.Approver,
		approvedAt:  spec.ApprovedAt.UTC(),
		publishedAt: spec.PublishedAt.UTC(),
	}
	if document.Plan != nil {
		if document.Plan.Canonicalization != canonicalizationVersion {
			return PriceCardDraft{}, fmt.Errorf("%w: draft records %q, this build canonicalizes %q",
				ErrCanonicalizationVersionUnsupported, document.Plan.Canonicalization, canonicalizationVersion)
		}
		draft.content.Plan = pricingPlanFrom(*document.Plan)
	}
	if document.DirectionAuthorization != nil {
		draft.content.DirectionAuthorization = versionReferenceFrom(*document.DirectionAuthorization)
	}
	for _, problem := range document.Problems {
		draft.problems = append(draft.problems, PriceCardDraftProblem{
			sheet: problem.Sheet, row: problem.Row, column: problem.Column, code: problem.Code, message: problem.Message,
		})
	}
	if reason := draft.incoherence(); reason != "" {
		return PriceCardDraft{}, rehydratedDraftRefusal(reason)
	}
	return draft, nil
}

func rehydratedDraftRefusal(reason string) error {
	return fmt.Errorf("%w：%s", ErrInvalidRehydratedPriceCardDraft, reason)
}
