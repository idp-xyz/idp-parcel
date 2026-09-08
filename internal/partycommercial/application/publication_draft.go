package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件是运营操作者面发布路径的四个用例（ADR-0126 Decision 三、四）：预览、录入、批准、发布。它们不新造
// 发布语义——发布那一步把已批准的载体交给既有的 PublishCommercialAuthorityHandler，答案代数一格不改；前三个
// 用例只经营载体。身份（录入者、批准者）由 Intake 从操作者信封交进命令，用例不从任何载荷里读身份。

// PreviewCommercialPublicationOutcome 是预览的结果代数：算出了摘要，或这份输入立不住。
type PreviewCommercialPublicationOutcome uint8

const (
	PreviewCommercialPublicationOutcomeInvalid PreviewCommercialPublicationOutcome = iota
	CommercialPublicationPreviewed
	CommercialPublicationPreviewNotAccepted
)

func (outcome PreviewCommercialPublicationOutcome) String() string {
	switch outcome {
	case CommercialPublicationPreviewed:
		return "PREVIEWED"
	case CommercialPublicationPreviewNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// PreviewCommercialPublicationCommand 携带一份拟录入的壳与正文。没有身份格：预览不落库、不记谁预览的；租户在壳上，
// 由 Intake 从信封填。
type PreviewCommercialPublicationCommand struct {
	Shell   domain.PublicationDraftShell
	Content domain.PublicationContent
}

// CommercialPublicationPreview 是预览的答案：规范化版本与摘要，或`未受理`的成因。
type CommercialPublicationPreview struct {
	outcome      PreviewCommercialPublicationOutcome
	canonical    domain.CanonicalPublicationContent
	hasCanonical bool
	refusalCause error
}

func (preview CommercialPublicationPreview) Outcome() PreviewCommercialPublicationOutcome {
	return preview.outcome
}

// Canonical 交回算出的规范化结果；`未受理`时第二个返回值为假——不交一个空摘要让人误以为算出来了。
func (preview CommercialPublicationPreview) Canonical() (domain.CanonicalPublicationContent, bool) {
	return preview.canonical, preview.hasCanonical
}

// RefusalCause 只在`未受理`时非空：没接的册 / 正文缺席 / 壳立不住，恢复动作各不相同，随结果交回。
func (preview CommercialPublicationPreview) RefusalCause() error { return preview.refusalCause }

// PreviewCommercialPublicationHandler 没有依赖：预览只过构造门，不读也不写任何登记册（ADR-0126 Decision 四）。
type PreviewCommercialPublicationHandler struct{}

func NewPreviewCommercialPublicationHandler() *PreviewCommercialPublicationHandler {
	return &PreviewCommercialPublicationHandler{}
}

// Handle 把壳与正文过一遍与录入相同的门。构造门拒是`未受理`不是 error：这一格是答案（哪里不对），改载荷才会好。
func (handler *PreviewCommercialPublicationHandler) Handle(
	_ context.Context,
	command PreviewCommercialPublicationCommand,
) (CommercialPublicationPreview, error) {
	canonical, err := domain.PreviewPublication(command.Shell, command.Content)
	if err != nil {
		return CommercialPublicationPreview{outcome: CommercialPublicationPreviewNotAccepted, refusalCause: err}, nil
	}
	return CommercialPublicationPreview{outcome: CommercialPublicationPreviewed, canonical: canonical, hasCanonical: true}, nil
}

// SubmitPublicationDraftOutcome 是录入的结果代数：登记册三格落点逐名翻译，外加输入自己立不住的`未受理`。
type SubmitPublicationDraftOutcome uint8

const (
	SubmitPublicationDraftOutcomeInvalid SubmitPublicationDraftOutcome = iota
	PublicationDraftSubmitted
	PublicationDraftReplayed
	PublicationDraftRevised
	PublicationDraftContentFixed
	PublicationDraftNotAccepted
)

func (outcome SubmitPublicationDraftOutcome) String() string {
	switch outcome {
	case PublicationDraftSubmitted:
		return "DRAFT_SUBMITTED"
	case PublicationDraftReplayed:
		return "DRAFT_REPLAYED"
	case PublicationDraftRevised:
		return "DRAFT_REVISED"
	case PublicationDraftContentFixed:
		return "CONTENT_FIXED"
	case PublicationDraftNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// SubmitPublicationDraftCommand 携带一次录入：壳、正文与录入者。录入者由 Intake 从操作者信封交进来。
type SubmitPublicationDraftCommand struct {
	Shell     domain.PublicationDraftShell
	Content   domain.PublicationContent
	Submitter domain.OperatorSubjectReference
}

type SubmitPublicationDraftResult struct {
	outcome      SubmitPublicationDraftOutcome
	draft        domain.PublicationDraft
	hasDraft     bool
	refusalCause error
}

func (result SubmitPublicationDraftResult) Outcome() SubmitPublicationDraftOutcome {
	return result.outcome
}

// Draft 交回本次形成的载体（含算出的规范化版本与摘要）；`未受理`时没有载体可交。`内容已固定`时交回的是本次
// 拟录的那份而不是库上的——调用方要的是「我交的这份算出什么」，库上那份由读面另答。
func (result SubmitPublicationDraftResult) Draft() (domain.PublicationDraft, bool) {
	return result.draft, result.hasDraft
}

func (result SubmitPublicationDraftResult) RefusalCause() error { return result.refusalCause }

type SubmitPublicationDraftHandler struct {
	drafts ports.PublicationDraftRegistry
	clock  ports.Clock
}

func NewSubmitPublicationDraftHandler(drafts ports.PublicationDraftRegistry, clock ports.Clock) *SubmitPublicationDraftHandler {
	return &SubmitPublicationDraftHandler{drafts: drafts, clock: clock}
}

// Handle 形成`待批准`载体并落册。构造门拒是`未受理`、不碰登记册；落点由登记册答（同内容重放 / 待批准期间修订 /
// 已批准后内容固定），这里逐名翻译。
func (handler *SubmitPublicationDraftHandler) Handle(
	ctx context.Context,
	command SubmitPublicationDraftCommand,
) (SubmitPublicationDraftResult, error) {
	draft, err := domain.SubmitPublicationDraft(command.Shell, command.Content, command.Submitter, handler.clock.Now())
	if err != nil {
		return SubmitPublicationDraftResult{outcome: PublicationDraftNotAccepted, refusalCause: err}, nil
	}
	outcome, err := handler.drafts.SubmitDraft(ctx, draft)
	if err != nil {
		return SubmitPublicationDraftResult{}, fmt.Errorf("submit publication draft: %w", err)
	}
	result := SubmitPublicationDraftResult{draft: draft, hasDraft: true}
	switch outcome {
	case ports.PublicationDraftSaved:
		result.outcome = PublicationDraftSubmitted
	case ports.PublicationDraftReplayed:
		result.outcome = PublicationDraftReplayed
	case ports.PublicationDraftRevised:
		result.outcome = PublicationDraftRevised
	case ports.PublicationDraftContentFixed:
		result.outcome = PublicationDraftContentFixed
	default:
		return SubmitPublicationDraftResult{}, fmt.Errorf("submit publication draft: unexpected register outcome %q", outcome)
	}
	return result, nil
}

// ApprovePublicationDraftOutcome 是批准的结果代数。拒绝按恢复动作分格（ADR-0029）：`未配置`要租户登规则；
// `需换人批准`与`批准者不合格`要换人；`已批准`/`已发布`是载体已过了这一格；`已被替换`是两个操作者先后动手，
// 后到的重读再来。
type ApprovePublicationDraftOutcome uint8

const (
	ApprovePublicationDraftOutcomeInvalid ApprovePublicationDraftOutcome = iota
	PublicationDraftApproved
	ApprovalDraftNotFound
	ApprovalDutyRuleNotConfigured
	PublicationDraftNeedsAnotherApprover
	PublicationDraftApproverNotQualified
	PublicationDraftAlreadyApproved
	ApprovalDraftAlreadyPublished
	ApprovalDraftChanged
)

func (outcome ApprovePublicationDraftOutcome) String() string {
	switch outcome {
	case PublicationDraftApproved:
		return "DRAFT_APPROVED"
	case ApprovalDraftNotFound:
		return "DRAFT_NOT_FOUND"
	case ApprovalDutyRuleNotConfigured:
		return "NOT_CONFIGURED"
	case PublicationDraftNeedsAnotherApprover:
		return "NEEDS_ANOTHER_APPROVER"
	case PublicationDraftApproverNotQualified:
		return "APPROVER_NOT_QUALIFIED"
	case PublicationDraftAlreadyApproved:
		return "DRAFT_ALREADY_APPROVED"
	case ApprovalDraftAlreadyPublished:
		return "DRAFT_ALREADY_PUBLISHED"
	case ApprovalDraftChanged:
		return "DRAFT_CHANGED"
	default:
		return ""
	}
}

// ApprovePublicationDraftCommand 按版本身份四元指名载体，批准者是操作者主体（引用 + 授予集），由 Intake 从信封交进来。
type ApprovePublicationDraftCommand struct {
	Tenant   domain.TenantID
	Kind     domain.CommercialObjectKind
	ObjectID domain.CommercialObjectID
	Version  domain.CommercialVersionLabel
	Approver domain.OperatorSubject
}

type ApprovePublicationDraftResult struct {
	outcome  ApprovePublicationDraftOutcome
	draft    domain.PublicationDraft
	hasDraft bool
}

func (result ApprovePublicationDraftResult) Outcome() ApprovePublicationDraftOutcome {
	return result.outcome
}

// Draft 交回推进后的载体；只在`已批准`时在场。
func (result ApprovePublicationDraftResult) Draft() (domain.PublicationDraft, bool) {
	return result.draft, result.hasDraft
}

type ApprovePublicationDraftHandler struct {
	drafts ports.PublicationDraftRegistry
	rules  ports.ApprovalDutyRuleView
	clock  ports.Clock
}

func NewApprovePublicationDraftHandler(
	drafts ports.PublicationDraftRegistry,
	rules ports.ApprovalDutyRuleView,
	clock ports.Clock,
) *ApprovePublicationDraftHandler {
	return &ApprovePublicationDraftHandler{drafts: drafts, rules: rules, clock: clock}
}

// Handle 读载体、读审批职责规则、由领域批准门裁、写回。规则未登记即`未配置`不放行（PAR-COM-18）——不以任何默认
// 代替；载体已不在`待批准`时按它此刻在哪一格答，不重复批。
func (handler *ApprovePublicationDraftHandler) Handle(
	ctx context.Context,
	command ApprovePublicationDraftCommand,
) (ApprovePublicationDraftResult, error) {
	draft, found, err := handler.drafts.LoadDraft(ctx, command.Tenant, command.Kind, command.ObjectID, command.Version)
	if err != nil {
		return ApprovePublicationDraftResult{}, fmt.Errorf("approve publication draft: %w", err)
	}
	if !found {
		return ApprovePublicationDraftResult{outcome: ApprovalDraftNotFound}, nil
	}
	switch draft.Status() {
	case domain.PublicationDraftApproved:
		return ApprovePublicationDraftResult{outcome: PublicationDraftAlreadyApproved}, nil
	case domain.PublicationDraftPublished:
		return ApprovePublicationDraftResult{outcome: ApprovalDraftAlreadyPublished}, nil
	}

	rule, configured, err := handler.rules.LoadApprovalDutyRule(ctx, command.Tenant)
	if err != nil {
		return ApprovePublicationDraftResult{}, fmt.Errorf("approve publication draft: %w", err)
	}
	if !configured {
		return ApprovePublicationDraftResult{outcome: ApprovalDutyRuleNotConfigured}, nil
	}

	approved, err := draft.Approve(command.Approver, rule, handler.clock.Now())
	switch {
	case errors.Is(err, domain.ErrApproverIsSubmitter):
		return ApprovePublicationDraftResult{outcome: PublicationDraftNeedsAnotherApprover}, nil
	case errors.Is(err, domain.ErrApproverLacksRequiredLevel):
		return ApprovePublicationDraftResult{outcome: PublicationDraftApproverNotQualified}, nil
	case err != nil:
		// 规则不是本租户的、批准者立不住、时刻早于录入：都是装配或调用方的错，不是业务答案。
		return ApprovePublicationDraftResult{}, fmt.Errorf("approve publication draft: %w", err)
	}

	outcome, err := handler.drafts.AdvanceDraft(ctx, approved)
	if err != nil {
		return ApprovePublicationDraftResult{}, fmt.Errorf("approve publication draft: %w", err)
	}
	switch outcome {
	case ports.PublicationDraftAdvanced:
		return ApprovePublicationDraftResult{outcome: PublicationDraftApproved, draft: approved, hasDraft: true}, nil
	case ports.PublicationDraftAdvanceNotFound:
		return ApprovePublicationDraftResult{outcome: ApprovalDraftNotFound}, nil
	case ports.PublicationDraftAdvanceSuperseded:
		return ApprovePublicationDraftResult{outcome: ApprovalDraftChanged}, nil
	default:
		return ApprovePublicationDraftResult{}, fmt.Errorf("approve publication draft: unexpected register outcome %q", outcome)
	}
}

// PublishPublicationDraftOutcome 是发布载体的结果代数。`已发布`说的是受控发布落定且载体推进；`发布未落定`说的是
// 发布用例答了`未决`/`冲突`/`未受理`之一——载体留在`已批准`，发布用例的答案随结果交回，续办照那一格办；
// `等待生效边界`是载体区间未开：发布用例对挂在`已计划生效`版本上的声明整项拒绝、只增仓储没有日后补声明的口
// （PublishCommercialAuthorityHandler 的既有语义），而载体永远带正文，所以到界前不交给它，届期再发布。
type PublishPublicationDraftOutcome uint8

const (
	PublishPublicationDraftOutcomeInvalid PublishPublicationDraftOutcome = iota
	PublicationDraftPublished
	PublicationDraftNotFound
	PublicationDraftNotApproved
	PublicationDraftAlreadyPublished
	PublicationDraftAwaitsEffectiveStart
	PublicationDraftPublicationNotLanded
)

func (outcome PublishPublicationDraftOutcome) String() string {
	switch outcome {
	case PublicationDraftPublished:
		return "DRAFT_PUBLISHED"
	case PublicationDraftNotFound:
		return "DRAFT_NOT_FOUND"
	case PublicationDraftNotApproved:
		return "DRAFT_NOT_APPROVED"
	case PublicationDraftAlreadyPublished:
		return "DRAFT_ALREADY_PUBLISHED"
	case PublicationDraftAwaitsEffectiveStart:
		return "DRAFT_AWAITS_EFFECTIVE_START"
	case PublicationDraftPublicationNotLanded:
		return "PUBLICATION_NOT_LANDED"
	default:
		return ""
	}
}

// PublishPublicationDraftCommand 按版本身份四元指名载体。没有身份格：发布不再记谁按的键——批准者已在载体上，
// 发布只是把批准过的东西交出去（ADR-0126 Decision 三）。
type PublishPublicationDraftCommand struct {
	Tenant   domain.TenantID
	Kind     domain.CommercialObjectKind
	ObjectID domain.CommercialObjectID
	Version  domain.CommercialVersionLabel
}

type PublishPublicationDraftResult struct {
	outcome        PublishPublicationDraftOutcome
	publication    PublishCommercialAuthorityResult
	hasPublication bool
}

func (result PublishPublicationDraftResult) Outcome() PublishPublicationDraftOutcome {
	return result.outcome
}

// Publication 交回受控发布用例的答案，只在真交给了它时在场（`已发布`与`发布未落定`两格）。
func (result PublishPublicationDraftResult) Publication() (PublishCommercialAuthorityResult, bool) {
	return result.publication, result.hasPublication
}

// PublishPublicationDraftHandler 收的是具体的 *PublishCommercialAuthorityHandler 而不是接口：发布 = 交既有用例
// 是 ADR-0126 Decision 三的形，接口会让装配点把别的东西接进来而编译仍绿。
type PublishPublicationDraftHandler struct {
	drafts    ports.PublicationDraftRegistry
	publisher *PublishCommercialAuthorityHandler
	clock     ports.Clock
}

func NewPublishPublicationDraftHandler(
	drafts ports.PublicationDraftRegistry,
	publisher *PublishCommercialAuthorityHandler,
	clock ports.Clock,
) *PublishPublicationDraftHandler {
	return &PublishPublicationDraftHandler{drafts: drafts, publisher: publisher, clock: clock}
}

// Handle 只接`已批准`载体：壳 + 算出的摘要作规格、载体交出的批准依据作批准、角色确认由批准门代答、正文折成
// 声明，交给受控发布；落定（含重放）即推进载体为`已发布`，没落定载体原样留着。调用方把本方法与发布用例的
// 写入包在同一笔事务里。
func (handler *PublishPublicationDraftHandler) Handle(
	ctx context.Context,
	command PublishPublicationDraftCommand,
) (PublishPublicationDraftResult, error) {
	draft, found, err := handler.drafts.LoadDraft(ctx, command.Tenant, command.Kind, command.ObjectID, command.Version)
	if err != nil {
		return PublishPublicationDraftResult{}, fmt.Errorf("publish publication draft: %w", err)
	}
	if !found {
		return PublishPublicationDraftResult{outcome: PublicationDraftNotFound}, nil
	}
	switch draft.Status() {
	case domain.PublicationDraftPendingApproval:
		return PublishPublicationDraftResult{outcome: PublicationDraftNotApproved}, nil
	case domain.PublicationDraftPublished:
		return PublishPublicationDraftResult{outcome: PublicationDraftAlreadyPublished}, nil
	}
	now := handler.clock.Now()
	if now.Before(draft.Effective().StartsAt()) {
		// 到界前发布会把正文挂在`已计划生效`的版本上，发布用例整项拒——那是它的既有语义（声明只能随已生效
		// 版本登记），这里把它翻成一格答案而不是让调用方撞一个 error。
		return PublishPublicationDraftResult{outcome: PublicationDraftAwaitsEffectiveStart}, nil
	}

	approval, err := draft.PublicationApproval()
	if err != nil {
		return PublishPublicationDraftResult{}, fmt.Errorf("publish publication draft: %w", err)
	}
	publication, err := handler.publisher.Handle(ctx, PublishCommercialAuthorityCommand{
		Spec:     draft.PublicationSpec(),
		Approval: approval,
		// 角色是否够由批准门按审批职责规则判过了（ADR-0126 Decision 三），这一格不再是载荷里一句自报。
		RoleStanding: domain.ApprovalRoleConfirmed,
		Declarations: declarationsOfContent(draft.Content()),
	})
	if err != nil {
		return PublishPublicationDraftResult{}, fmt.Errorf("publish publication draft: %w", err)
	}
	result := PublishPublicationDraftResult{publication: publication, hasPublication: true}
	switch publication.Outcome() {
	case CommercialVersionPublishedEffective, CommercialVersionPlannedEffective, CommercialPublicationReplayed:
	default:
		result.outcome = PublicationDraftPublicationNotLanded
		return result, nil
	}

	published, err := draft.MarkPublished(now)
	if err != nil {
		return PublishPublicationDraftResult{}, fmt.Errorf("publish publication draft: %w", err)
	}
	outcome, err := handler.drafts.AdvanceDraft(ctx, published)
	if err != nil {
		return PublishPublicationDraftResult{}, fmt.Errorf("publish publication draft: %w", err)
	}
	switch outcome {
	case ports.PublicationDraftAdvanced:
		result.outcome = PublicationDraftPublished
	case ports.PublicationDraftAdvanceNotFound:
		// 读时在、写时不在：载体行不删，这一格正常运行里到不了；照实翻译，让事务回滚由调用方按 error 处置。
		return PublishPublicationDraftResult{}, fmt.Errorf("publish publication draft: 载体在发布落定后读不回")
	case ports.PublicationDraftAdvanceSuperseded:
		// 载体在读与写之间被别的操作者动过：发布已落定，但载体状态没跟上——上抛让整笔事务回滚，两边一起重来。
		return PublishPublicationDraftResult{}, fmt.Errorf("publish publication draft: 载体在发布过程中被替换，整笔回滚")
	default:
		return PublishPublicationDraftResult{}, fmt.Errorf("publish publication draft: unexpected register outcome %q", outcome)
	}
	return result, nil
}

// declarationsOfContent 把载体上的正文折成发布用例的声明输入面——publicationContentOf 的反向。今天只有信用
// 政策一格；各册子票在此加一分支时，两个方向要同笔加。
func declarationsOfContent(content domain.PublicationContent) CommercialDeclarations {
	var declarations CommercialDeclarations
	if content.CreditPolicy != nil {
		body := content.CreditPolicy
		declarations.CreditPolicyBody = &CreditPolicyBodyDeclaration{
			LegalEntity: body.LegalEntity,
			Level:       body.Level,
			ChargeType:  body.ChargeType,
			Limit:       body.Limit,
			Effective:   body.Effective,
		}
	}
	if content.SupplierAgreement != nil {
		body := content.SupplierAgreement
		declarations.SupplierAgreementBody = &SupplierAgreementBodyDeclaration{
			Supplier:     body.Supplier,
			LegalEntity:  body.LegalEntity,
			Scope:        body.Scope,
			PurchasePlan: body.PurchasePlan,
			Effective:    body.Effective,
		}
	}
	if content.CustomerContract != nil {
		body := content.CustomerContract
		declarations.ContractContent = &ContractContentDeclaration{RulePackage: body.RulePackage, Bindings: body.Bindings}
		// 合同级声明缺席就不交这一通道：载体上没说「要不要」，发布用例也不替它说。
		if body.Control != nil {
			declarations.PreAcceptanceControl = &PreAcceptanceControlInstruction{
				Requirement: body.Control.Requirement,
				Basis:       body.Control.Basis,
			}
		}
	}
	if content.AuthorizationRule != nil {
		// 授权规则的正文就是取消授权目录一通道（票 admin-write-faces/17）；行照载体上的原样交给发布用例，目录门在那里再过一遍。
		declarations.CancellationAuthority = append([]domain.CancellationAuthorityDeclaration(nil), content.AuthorizationRule.CancellationAuthority...)
	}
	return declarations
}
