package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 价卡草稿的批准与发布（ADR-0101 决定四至六；票 price-card-import/04）。批准是另一个操作者动作：批准者身份由 Intake 从
// 操作者信封交进命令，门由租户的审批职责规则说。发布 = 把已批准的草稿交给既有的价卡登记用例，答案代数一格不改；
// 受控 CLI 不经草稿，照旧直接登记（ADR-0101 决定五）。

// ApprovePriceCardDraftOutcome 是批准的答复。拒绝按恢复动作分格（ADR-0029）：`未配置`要租户登规则；`需换人批准`与
// `批准者不合格`要换人；`未校验`要先把文件改到读得通再录；`已批准`/`已发布`是草稿已过了这一格；`草稿已变`是两个操作者
// 先后动手，后到的重读再来。
type ApprovePriceCardDraftOutcome uint8

const (
	ApprovePriceCardDraftOutcomeInvalid ApprovePriceCardDraftOutcome = iota
	PriceCardDraftApproved
	PriceCardApprovalDraftNotFound
	PriceCardApprovalNotConfigured
	PriceCardApprovalNeedsAnotherApprover
	PriceCardApprovalApproverNotQualified
	PriceCardApprovalDraftNotValidated
	PriceCardApprovalDraftAlreadyApproved
	PriceCardApprovalDraftAlreadyPublished
	PriceCardApprovalDraftChanged
)

func (outcome ApprovePriceCardDraftOutcome) String() string {
	switch outcome {
	case PriceCardDraftApproved:
		return "DRAFT_APPROVED"
	case PriceCardApprovalDraftNotFound:
		return "DRAFT_NOT_FOUND"
	case PriceCardApprovalNotConfigured:
		return "NOT_CONFIGURED"
	case PriceCardApprovalNeedsAnotherApprover:
		return "NEEDS_ANOTHER_APPROVER"
	case PriceCardApprovalApproverNotQualified:
		return "APPROVER_NOT_QUALIFIED"
	case PriceCardApprovalDraftNotValidated:
		return "DRAFT_NOT_VALIDATED"
	case PriceCardApprovalDraftAlreadyApproved:
		return "DRAFT_ALREADY_APPROVED"
	case PriceCardApprovalDraftAlreadyPublished:
		return "DRAFT_ALREADY_PUBLISHED"
	case PriceCardApprovalDraftChanged:
		return "DRAFT_CHANGED"
	default:
		return ""
	}
}

// ApprovePriceCardDraftCommand 按（租户，方案版本）指名草稿；批准者是操作者主体（引用 + 授予集）。
type ApprovePriceCardDraftCommand struct {
	Tenant   domain.TenantID
	Plan     domain.VersionReference
	Approver domain.OperatorSubject
}

type ApprovePriceCardDraftResult struct {
	outcome  ApprovePriceCardDraftOutcome
	draft    domain.PriceCardDraft
	hasDraft bool
}

func (result ApprovePriceCardDraftResult) Outcome() ApprovePriceCardDraftOutcome {
	return result.outcome
}

// Draft 交回推进后的草稿；只在`已批准`时在场。
func (result ApprovePriceCardDraftResult) Draft() (domain.PriceCardDraft, bool) {
	return result.draft, result.hasDraft
}

type ApprovePriceCardDraftHandler struct {
	drafts ports.PriceCardDraftProgress
	rules  ports.PriceCardApprovalDutyRuleView
	clock  ports.Clock
}

func NewApprovePriceCardDraftHandler(
	drafts ports.PriceCardDraftProgress,
	rules ports.PriceCardApprovalDutyRuleView,
	clock ports.Clock,
) (*ApprovePriceCardDraftHandler, error) {
	if drafts == nil || rules == nil || clock == nil {
		return nil, errors.New("parcel pricing: price card draft approval needs a draft register, an approval duty rule view and a clock")
	}
	return &ApprovePriceCardDraftHandler{drafts: drafts, rules: rules, clock: clock}, nil
}

// Handle 读草稿、按它在哪一格先答，只有`已校验`才读审批职责规则、交领域批准门裁、写回。规则未登记即`未配置`、不放行
// （ADR-0101 决定六）——不以任何默认代替。
func (handler *ApprovePriceCardDraftHandler) Handle(ctx context.Context, command ApprovePriceCardDraftCommand) (ApprovePriceCardDraftResult, error) {
	draft, found, err := handler.drafts.LoadPriceCardDraft(ctx, command.Tenant, command.Plan)
	if err != nil {
		return ApprovePriceCardDraftResult{}, fmt.Errorf("approve price card draft: %w", err)
	}
	if !found {
		return ApprovePriceCardDraftResult{outcome: PriceCardApprovalDraftNotFound}, nil
	}
	switch draft.Status() {
	case domain.PriceCardDraftStatusDraft:
		return ApprovePriceCardDraftResult{outcome: PriceCardApprovalDraftNotValidated}, nil
	case domain.PriceCardDraftStatusApproved:
		return ApprovePriceCardDraftResult{outcome: PriceCardApprovalDraftAlreadyApproved}, nil
	case domain.PriceCardDraftStatusPublished:
		return ApprovePriceCardDraftResult{outcome: PriceCardApprovalDraftAlreadyPublished}, nil
	}

	rule, configured, err := handler.rules.LoadPriceCardApprovalDutyRule(ctx, command.Tenant)
	if err != nil {
		return ApprovePriceCardDraftResult{}, fmt.Errorf("approve price card draft: %w", err)
	}
	if !configured {
		return ApprovePriceCardDraftResult{outcome: PriceCardApprovalNotConfigured}, nil
	}

	approved, err := draft.Approve(command.Approver, rule, handler.clock.Now())
	switch {
	case errors.Is(err, domain.ErrDraftApproverIsSubmitter):
		return ApprovePriceCardDraftResult{outcome: PriceCardApprovalNeedsAnotherApprover}, nil
	case errors.Is(err, domain.ErrDraftApproverLacksRequiredGrant):
		return ApprovePriceCardDraftResult{outcome: PriceCardApprovalApproverNotQualified}, nil
	case err != nil:
		// 规则不是本租户的、批准者立不住、时钟早于录入：都是装配或调用方的错，不是业务答案。
		return ApprovePriceCardDraftResult{}, fmt.Errorf("approve price card draft: %w", err)
	}

	outcome, err := handler.drafts.AdvancePriceCardDraft(ctx, approved)
	if err != nil {
		return ApprovePriceCardDraftResult{}, fmt.Errorf("approve price card draft: %w", err)
	}
	switch outcome {
	case ports.PriceCardDraftAdvanced:
		return ApprovePriceCardDraftResult{outcome: PriceCardDraftApproved, draft: approved, hasDraft: true}, nil
	case ports.PriceCardDraftAdvanceNotFound:
		return ApprovePriceCardDraftResult{outcome: PriceCardApprovalDraftNotFound}, nil
	case ports.PriceCardDraftAdvanceSuperseded:
		return ApprovePriceCardDraftResult{outcome: PriceCardApprovalDraftChanged}, nil
	default:
		return ApprovePriceCardDraftResult{}, fmt.Errorf("approve price card draft: unexpected advance outcome %q", outcome)
	}
}

// PublishPriceCardDraftOutcome 是发布的答复。`已发布`说的是登记落定（入册或重放）且草稿已推进；`发布未落定`说的是登记
// 用例答了冲突、规范化不可比、未受理或未决之一——草稿留在`已批准`，登记的那一格随结果原样交回，续办照那一格办。
type PublishPriceCardDraftOutcome uint8

const (
	PublishPriceCardDraftOutcomeInvalid PublishPriceCardDraftOutcome = iota
	PriceCardDraftPublished
	PriceCardPublicationDraftNotFound
	PriceCardPublicationDraftNotApproved
	PriceCardPublicationDraftAlreadyPublished
	PriceCardPublicationNotLanded
)

func (outcome PublishPriceCardDraftOutcome) String() string {
	switch outcome {
	case PriceCardDraftPublished:
		return "DRAFT_PUBLISHED"
	case PriceCardPublicationDraftNotFound:
		return "DRAFT_NOT_FOUND"
	case PriceCardPublicationDraftNotApproved:
		return "DRAFT_NOT_APPROVED"
	case PriceCardPublicationDraftAlreadyPublished:
		return "DRAFT_ALREADY_PUBLISHED"
	case PriceCardPublicationNotLanded:
		return "PUBLICATION_NOT_LANDED"
	default:
		return ""
	}
}

// PublishPriceCardDraftCommand 按（租户，方案版本）指名草稿。没有身份格：批准者已记在草稿上，发布只是把批准过的东西交出去。
type PublishPriceCardDraftCommand struct {
	Tenant domain.TenantID
	Plan   domain.VersionReference
}

type PublishPriceCardDraftResult struct {
	outcome         PublishPriceCardDraftOutcome
	registration    RegisterPriceCardOutcome
	hasRegistration bool
	draft           domain.PriceCardDraft
	hasDraft        bool
}

func (result PublishPriceCardDraftResult) Outcome() PublishPriceCardDraftOutcome {
	return result.outcome
}

// Registration 交回登记用例的答复，只在真交给了它时在场（`已发布`与`发布未落定`两格）。
func (result PublishPriceCardDraftResult) Registration() (RegisterPriceCardOutcome, bool) {
	return result.registration, result.hasRegistration
}

// Draft 交回推进后的草稿；只在`已发布`时在场。
func (result PublishPriceCardDraftResult) Draft() (domain.PriceCardDraft, bool) {
	return result.draft, result.hasDraft
}

// PublishPriceCardDraftHandler 收的是具体的 *RegisterPriceCardHandler 而不是接口：发布 = 交既有登记用例是 ADR-0101
// 决定五的形，接口会让装配点把别的东西接进来而编译仍绿。
type PublishPriceCardDraftHandler struct {
	drafts    ports.PriceCardDraftProgress
	registrar *RegisterPriceCardHandler
	clock     ports.Clock
}

func NewPublishPriceCardDraftHandler(
	drafts ports.PriceCardDraftProgress,
	registrar *RegisterPriceCardHandler,
	clock ports.Clock,
) (*PublishPriceCardDraftHandler, error) {
	if drafts == nil || registrar == nil || clock == nil {
		return nil, errors.New("parcel pricing: price card draft publication needs a draft register, the price card registration and a clock")
	}
	return &PublishPriceCardDraftHandler{drafts: drafts, registrar: registrar, clock: clock}, nil
}

// Handle 只接`已批准`草稿：草稿交出的登记交给登记用例，落定（含重放）即推进草稿为`已发布`，没落定草稿原样留着。调用方
// 把本方法与登记用例的写入包在同一笔事务里——登记落定而草稿没跟上时这里上抛，整笔回滚。
func (handler *PublishPriceCardDraftHandler) Handle(ctx context.Context, command PublishPriceCardDraftCommand) (PublishPriceCardDraftResult, error) {
	draft, found, err := handler.drafts.LoadPriceCardDraft(ctx, command.Tenant, command.Plan)
	if err != nil {
		return PublishPriceCardDraftResult{}, fmt.Errorf("publish price card draft: %w", err)
	}
	if !found {
		return PublishPriceCardDraftResult{outcome: PriceCardPublicationDraftNotFound}, nil
	}
	switch draft.Status() {
	case domain.PriceCardDraftStatusDraft, domain.PriceCardDraftStatusValidated:
		return PublishPriceCardDraftResult{outcome: PriceCardPublicationDraftNotApproved}, nil
	case domain.PriceCardDraftStatusPublished:
		return PublishPriceCardDraftResult{outcome: PriceCardPublicationDraftAlreadyPublished}, nil
	}

	registration, err := draft.Registration()
	if err != nil {
		return PublishPriceCardDraftResult{}, fmt.Errorf("publish price card draft: %w", err)
	}
	registered, err := handler.registrar.Handle(ctx, RegisterPriceCardCommand{Registration: registration})
	result := PublishPriceCardDraftResult{registration: registered, hasRegistration: true}
	if err != nil {
		result.outcome = PriceCardPublicationNotLanded
		return result, fmt.Errorf("publish price card draft: %w", err)
	}
	switch registered {
	case PriceCardRecorded, PriceCardAlreadyOnRegister:
	default:
		result.outcome = PriceCardPublicationNotLanded
		return result, nil
	}

	published, err := draft.MarkPublished(handler.clock.Now())
	if err != nil {
		return PublishPriceCardDraftResult{}, fmt.Errorf("publish price card draft: %w", err)
	}
	outcome, err := handler.drafts.AdvancePriceCardDraft(ctx, published)
	if err != nil {
		return PublishPriceCardDraftResult{}, fmt.Errorf("publish price card draft: %w", err)
	}
	if outcome != ports.PriceCardDraftAdvanced {
		// 读时在、写时不在或已被别人动过：登记已写、草稿没跟上，上抛让整笔回滚、两边一起重来。
		return PublishPriceCardDraftResult{}, fmt.Errorf("publish price card draft: the draft could not follow the landed registration (%s)", outcome)
	}
	result.outcome, result.draft, result.hasDraft = PriceCardDraftPublished, published, true
	return result, nil
}
