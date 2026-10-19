package nodeopshttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 本文件是送站交接单的五口（UC-NO-006 应用流程步骤 3 至 5；ADR-0212 决定三）：开立、归入、关闭挂操作者渠道的「作业事实登记」能力面
// （ADR-0149 决定二），确认挂「运营决定」能力面的送站交接确认一种（ADR-0151 决定一），客户结论挂首方客户渠道族（ADR-0139）。三族的
// 身份格各只从自己的信封来，载荷封闭。

const codeDeliveryHandoverNoteNotVisible = "DELIVERY_HANDOVER_NOTE_NOT_VISIBLE"

// DeliveryHandoverFact 是作业事实登记面上交接单的三种事实，各在铸造侧判自己那一种的准入范围。字面量与接入身份的作业事实类型逐字
// 相同：认证适配器照它换准入要求。
type DeliveryHandoverFact string

const (
	DeliveryHandoverFactOpening    DeliveryHandoverFact = "DELIVERY_HANDOVER_NOTE_OPENING"
	DeliveryHandoverFactAssignment DeliveryHandoverFact = "DELIVERY_HANDOVER_NOTE_ASSIGNMENT"
	DeliveryHandoverFactClosure    DeliveryHandoverFact = "DELIVERY_HANDOVER_NOTE_CLOSURE"
)

// OperatorDecisionDeliveryHandoverConfirmation 是运营操作者依授权交付关系确认交接单对一个客户账户的绑定。
const OperatorDecisionDeliveryHandoverConfirmation OperatorDecision = "DELIVERY_HANDOVER_CONFIRMATION"

// HandoverNoteWorkFactAuthenticator 以作业事实登记能力面铸交接单这一种事实的信封，交回租户与节点。失败只交回本包的格，含设备不在用
// 与不在准入范围。
type HandoverNoteWorkFactAuthenticator interface {
	AuthenticateDeliveryHandoverFact(ctx context.Context, bearerToken, deviceID, scopeKind, scopeReference string, fact DeliveryHandoverFact) (domain.TenantID, domain.NodeReference, error)
}

// CustomerHandoverConclusionAuthenticator 认证客户结论口的首方出示，做法同 CustomerPreAdviceAuthenticator：认证与准入都过了才调读
// 载荷的口。
type CustomerHandoverConclusionAuthenticator interface {
	AuthenticateDeliveryHandoverConclusion(ctx context.Context, credential PresentedCustomerCredential, accessRequestID func() (string, error)) (CustomerIdentity, error)
}

// 五口各自把一次已认证的请求译成命令。
type (
	DeliveryHandoverNoteOpeningIntake interface {
		IntakeDeliveryHandoverNoteOpening(ctx context.Context, request *http.Request) (application.OpenDeliveryHandoverNoteCommand, error)
	}
	DeliveryHandoverAssignmentIntake interface {
		IntakeDeliveryHandoverAssignment(ctx context.Context, request *http.Request) (application.AssignToDeliveryHandoverNoteCommand, error)
	}
	DeliveryHandoverClosureIntake interface {
		IntakeDeliveryHandoverClosure(ctx context.Context, request *http.Request) (application.CloseDeliveryHandoverNoteCommand, error)
	}
	DeliveryHandoverConfirmationIntake interface {
		IntakeDeliveryHandoverConfirmation(ctx context.Context, request *http.Request) (application.ConfirmDeliveryHandoverCommand, error)
	}
	DeliveryHandoverConclusionIntake interface {
		IntakeDeliveryHandoverConclusion(ctx context.Context, request *http.Request) (application.ConcludeDeliveryHandoverCommand, error)
	}
)

// DeliveryHandoverNoteHandler 是五口转交的交接单编排，装配负责把每次调用包进一笔事务。
type DeliveryHandoverNoteHandler interface {
	Open(ctx context.Context, command application.OpenDeliveryHandoverNoteCommand) (application.DeliveryHandoverResult, error)
	Assign(ctx context.Context, command application.AssignToDeliveryHandoverNoteCommand) (application.DeliveryHandoverResult, error)
	Close(ctx context.Context, command application.CloseDeliveryHandoverNoteCommand) (application.DeliveryHandoverResult, error)
	ConfirmByOperationDecision(ctx context.Context, command application.ConfirmDeliveryHandoverCommand) (application.DeliveryHandoverResult, error)
	ConcludeByCustomer(ctx context.Context, command application.ConcludeDeliveryHandoverCommand) (application.DeliveryHandoverResult, error)
}

// WorkFactHandoverNoteIntake 是作业事实登记面在交接单三口的 Intake。设备与作业范围从三个头来，作业范围种类只认 NODE：交接单在节点
// 当场开立。
type WorkFactHandoverNoteIntake struct {
	authenticator HandoverNoteWorkFactAuthenticator
}

var (
	_ DeliveryHandoverNoteOpeningIntake = (*WorkFactHandoverNoteIntake)(nil)
	_ DeliveryHandoverAssignmentIntake  = (*WorkFactHandoverNoteIntake)(nil)
	_ DeliveryHandoverClosureIntake     = (*WorkFactHandoverNoteIntake)(nil)
)

func NewWorkFactHandoverNoteIntake(authenticator HandoverNoteWorkFactAuthenticator) (*WorkFactHandoverNoteIntake, error) {
	if authenticator == nil {
		return nil, errors.New("node operations http: work fact handover note intake needs an authenticator")
	}
	return &WorkFactHandoverNoteIntake{authenticator: authenticator}, nil
}

// authenticate 先认证、后解载荷：授予不过的调用方看不到载荷校验的结果。
func (intake *WorkFactHandoverNoteIntake) authenticate(ctx context.Context, request *http.Request, fact DeliveryHandoverFact) (domain.TenantID, domain.NodeReference, error) {
	device := strings.TrimSpace(request.Header.Get(headerWorkDevice))
	kind := strings.TrimSpace(request.Header.Get(headerWorkScopeKind))
	reference := strings.TrimSpace(request.Header.Get(headerWorkScopeRef))
	if device == "" || kind == "" || reference == "" {
		return domain.TenantID{}, domain.NodeReference{}, fmt.Errorf("%w: %s, %s and %s are required", ErrMalformedRequest, headerWorkDevice, headerWorkScopeKind, headerWorkScopeRef)
	}
	if kind != "NODE" {
		return domain.TenantID{}, domain.NodeReference{}, fmt.Errorf("%w: a delivery handover note lives at a node; Parcel-Work-Scope-Kind must be NODE", ErrMalformedRequest)
	}
	return intake.authenticator.AuthenticateDeliveryHandoverFact(ctx, httpapi.BearerToken(request), device, kind, reference, fact)
}

// handoverNoteOpeningPayload 是开立的线格式（UC-NO-006 输入语义契约「交接单开立」）：设备签发的交接单标识与开立时刻、交付方线索、
// 声称的客户账户。没有租户与节点——那两格取自信封。
type handoverNoteOpeningPayload struct {
	NoteID          string     `json:"noteId"`
	DeliveringParty string     `json:"deliveringParty"`
	ClaimedAccounts []string   `json:"claimedAccounts"`
	OpenedAt        *time.Time `json:"openedAt"`
}

// IntakeDeliveryHandoverNoteOpening 译开立（`/node-operations/delivery-handover-notes`）。声称为空或重复答坏载荷；客户账户标识逐字交下。
func (intake *WorkFactHandoverNoteIntake) IntakeDeliveryHandoverNoteOpening(ctx context.Context, request *http.Request) (application.OpenDeliveryHandoverNoteCommand, error) {
	none := application.OpenDeliveryHandoverNoteCommand{}
	tenant, node, err := intake.authenticate(ctx, request, DeliveryHandoverFactOpening)
	if err != nil {
		return none, err
	}
	var payload handoverNoteOpeningPayload
	if err := decodeClosedDocument(request, &payload); err != nil {
		return none, err
	}
	command := application.OpenDeliveryHandoverNoteCommand{TenantID: tenant, Node: node}
	if command.Note, err = handoverNoteIDField(payload.NoteID); err != nil {
		return none, err
	}
	if err := requiredField("deliveringParty", payload.DeliveringParty); err != nil {
		return none, err
	}
	if command.DeliveringParty, err = domain.NewDeliveringPartyReference(payload.DeliveringParty); err != nil {
		return none, malformedField("deliveringParty", err)
	}
	if len(payload.ClaimedAccounts) == 0 {
		return none, fmt.Errorf("%w: claimedAccounts needs at least one customer account", ErrMalformedRequest)
	}
	seen := make(map[string]bool, len(payload.ClaimedAccounts))
	for index, value := range payload.ClaimedAccounts {
		name := fmt.Sprintf("claimedAccounts[%d]", index)
		if err := requiredField(name, value); err != nil {
			return none, err
		}
		if seen[value] {
			return none, fmt.Errorf("%w: %s repeats a customer account", ErrMalformedRequest, name)
		}
		seen[value] = true
		account, err := domain.NewCustomerAccountReference(value)
		if err != nil {
			return none, malformedField(name, err)
		}
		command.ClaimedAccounts = append(command.ClaimedAccounts, account)
	}
	if command.OpenedAt, err = instantField("openedAt", payload.OpenedAt); err != nil {
		return none, err
	}
	return command, nil
}

// handoverAssignmentPayload 是归入的线格式：设备签发的归入事实身份与业务时间、交接单、作业实物。
type handoverAssignmentPayload struct {
	FactID     string     `json:"factId"`
	NoteID     string     `json:"noteId"`
	Unit       string     `json:"unit"`
	AssignedAt *time.Time `json:"assignedAt"`
}

// IntakeDeliveryHandoverAssignment 译归入（`/node-operations/delivery-handover-assignments`）。
func (intake *WorkFactHandoverNoteIntake) IntakeDeliveryHandoverAssignment(ctx context.Context, request *http.Request) (application.AssignToDeliveryHandoverNoteCommand, error) {
	none := application.AssignToDeliveryHandoverNoteCommand{}
	tenant, node, err := intake.authenticate(ctx, request, DeliveryHandoverFactAssignment)
	if err != nil {
		return none, err
	}
	var payload handoverAssignmentPayload
	if err := decodeClosedDocument(request, &payload); err != nil {
		return none, err
	}
	command := application.AssignToDeliveryHandoverNoteCommand{TenantID: tenant, Node: node, FactID: payload.FactID}
	if err := requiredField("factId", payload.FactID); err != nil {
		return none, err
	}
	if command.Note, err = handoverNoteIDField(payload.NoteID); err != nil {
		return none, err
	}
	if err := requiredField("unit", payload.Unit); err != nil {
		return none, err
	}
	if command.Unit, err = domain.NewHandlingUnitID(payload.Unit); err != nil {
		return none, malformedField("unit", err)
	}
	if command.AssignedAt, err = instantField("assignedAt", payload.AssignedAt); err != nil {
		return none, err
	}
	return command, nil
}

// handoverClosurePayload 是关闭的线格式：设备签发的关闭事实身份与关闭时刻、交接单。
type handoverClosurePayload struct {
	FactID   string     `json:"factId"`
	NoteID   string     `json:"noteId"`
	ClosedAt *time.Time `json:"closedAt"`
}

// IntakeDeliveryHandoverClosure 译关闭（`/node-operations/delivery-handover-closures`）。
func (intake *WorkFactHandoverNoteIntake) IntakeDeliveryHandoverClosure(ctx context.Context, request *http.Request) (application.CloseDeliveryHandoverNoteCommand, error) {
	none := application.CloseDeliveryHandoverNoteCommand{}
	tenant, node, err := intake.authenticate(ctx, request, DeliveryHandoverFactClosure)
	if err != nil {
		return none, err
	}
	var payload handoverClosurePayload
	if err := decodeClosedDocument(request, &payload); err != nil {
		return none, err
	}
	command := application.CloseDeliveryHandoverNoteCommand{TenantID: tenant, Node: node, FactID: payload.FactID}
	if err := requiredField("factId", payload.FactID); err != nil {
		return none, err
	}
	if command.Note, err = handoverNoteIDField(payload.NoteID); err != nil {
		return none, err
	}
	if command.ClosedAt, err = instantField("closedAt", payload.ClosedAt); err != nil {
		return none, err
	}
	return command, nil
}

// handoverConfirmationPayload 是运营决定确认的线格式（UC-NO-006 输入语义契约「运营决定确认」）：交接单、客户账户、交付方参与方
// （party-commercial 的参与方标识）与依据引用。没有租户、操作者与时间：前两格取自信封，形成时刻取服务端时刻。
type handoverConfirmationPayload struct {
	NoteID                string `json:"noteId"`
	CustomerAccount       string `json:"customerAccount"`
	DeliveringParticipant string `json:"deliveringParticipant"`
	Basis                 string `json:"basis"`
}

var _ DeliveryHandoverConfirmationIntake = (*OperatorDecisionIntake)(nil)

// IntakeDeliveryHandoverConfirmation 译运营决定确认（`/node-operations/delivery-handover-confirmations`），按送站交接确认这一种决定认证。
// 依据引用去两端空白后逐字交下，别的格原样。
func (intake *OperatorDecisionIntake) IntakeDeliveryHandoverConfirmation(ctx context.Context, request *http.Request) (application.ConfirmDeliveryHandoverCommand, error) {
	none := application.ConfirmDeliveryHandoverCommand{}
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionDeliveryHandoverConfirmation)
	if err != nil {
		return none, err
	}
	var payload handoverConfirmationPayload
	if err := decodeClosedDocument(request, &payload); err != nil {
		return none, err
	}
	command := application.ConfirmDeliveryHandoverCommand{TenantID: operator.Tenant, Operator: "OPERATOR/" + operator.Operator}
	if command.Note, err = handoverNoteIDField(payload.NoteID); err != nil {
		return none, err
	}
	for name, value := range map[string]string{
		"customerAccount": payload.CustomerAccount, "deliveringParticipant": payload.DeliveringParticipant, "basis": payload.Basis,
	} {
		if err := requiredField(name, value); err != nil {
			return none, err
		}
	}
	if command.Account, err = domain.NewCustomerAccountReference(payload.CustomerAccount); err != nil {
		return none, malformedField("customerAccount", err)
	}
	if command.DeliveringParticipant, err = domain.NewDeliveringPartyReference(payload.DeliveringParticipant); err != nil {
		return none, malformedField("deliveringParticipant", err)
	}
	command.Basis = strings.TrimSpace(payload.Basis)
	return command, nil
}

// CustomerHandoverConclusionIntake 是首方客户渠道在客户结论口的 Intake。
type CustomerHandoverConclusionIntake struct {
	authenticator CustomerHandoverConclusionAuthenticator
}

var _ DeliveryHandoverConclusionIntake = (*CustomerHandoverConclusionIntake)(nil)

func NewCustomerHandoverConclusionIntake(authenticator CustomerHandoverConclusionAuthenticator) (*CustomerHandoverConclusionIntake, error) {
	if authenticator == nil {
		return nil, errors.New("node operations http: customer handover conclusion intake needs an authenticator")
	}
	return &CustomerHandoverConclusionIntake{authenticator: authenticator}, nil
}

// handoverConclusionPayload 是客户结论的线格式（UC-NO-006 输入语义契约「客户渠道结论」）：接入请求标识、交接单、结论。没有租户与
// 客户账户——封闭解码下自报这两格都是未知字段。
type handoverConclusionPayload struct {
	AccessRequestID string `json:"accessRequestId"`
	NoteID          string `json:"noteId"`
	Conclusion      string `json:"conclusion"`
}

// IntakeDeliveryHandoverConclusion 译客户结论（`/node-operations/delivery-handover-conclusions`）。结论只认 CONFIRMED 与 DENIED 两个线上
// 名，大小写不折叠。
func (intake *CustomerHandoverConclusionIntake) IntakeDeliveryHandoverConclusion(ctx context.Context, request *http.Request) (application.ConcludeDeliveryHandoverCommand, error) {
	none := application.ConcludeDeliveryHandoverCommand{}
	var payload handoverConclusionPayload
	customer, err := intake.authenticator.AuthenticateDeliveryHandoverConclusion(ctx, presentedCustomerCredential(request), func() (string, error) {
		if err := decodeClosedDocument(request, &payload); err != nil {
			return "", err
		}
		return payload.AccessRequestID, nil
	})
	if err != nil {
		return none, err
	}
	command := application.ConcludeDeliveryHandoverCommand{
		TenantID:        customer.Tenant,
		CustomerAccount: customer.CustomerAccount,
		Channel:         customer.Channel,
		RequestKey:      customer.RequestKey,
	}
	if command.Note, err = handoverNoteIDField(payload.NoteID); err != nil {
		return none, err
	}
	if err := requiredField("conclusion", payload.Conclusion); err != nil {
		return none, err
	}
	if command.Conclusion, err = domain.ParseHandoverBindingConclusion(payload.Conclusion); err != nil {
		return none, malformedField("conclusion", err)
	}
	return command, nil
}

func handoverNoteIDField(value string) (domain.DeliveryHandoverNoteID, error) {
	if err := requiredField("noteId", value); err != nil {
		return domain.DeliveryHandoverNoteID{}, err
	}
	id, err := domain.NewDeliveryHandoverNoteID(value)
	if err != nil {
		return domain.DeliveryHandoverNoteID{}, malformedField("noteId", err)
	}
	return id, nil
}

func instantField(name string, value *time.Time) (time.Time, error) {
	if value == nil || value.IsZero() {
		return time.Time{}, fmt.Errorf("%w: %s is required", ErrMalformedRequest, name)
	}
	return *value, nil
}

var (
	_ DeliveryHandoverNoteOpeningIntake  = UnconfiguredIntake{}
	_ DeliveryHandoverAssignmentIntake   = UnconfiguredIntake{}
	_ DeliveryHandoverClosureIntake      = UnconfiguredIntake{}
	_ DeliveryHandoverConfirmationIntake = UnconfiguredIntake{}
	_ DeliveryHandoverConclusionIntake   = UnconfiguredIntake{}
)

// 以下五个同 IntakeReception：不读请求，只答未配置。
func (UnconfiguredIntake) IntakeDeliveryHandoverNoteOpening(context.Context, *http.Request) (application.OpenDeliveryHandoverNoteCommand, error) {
	return application.OpenDeliveryHandoverNoteCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeDeliveryHandoverAssignment(context.Context, *http.Request) (application.AssignToDeliveryHandoverNoteCommand, error) {
	return application.AssignToDeliveryHandoverNoteCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeDeliveryHandoverClosure(context.Context, *http.Request) (application.CloseDeliveryHandoverNoteCommand, error) {
	return application.CloseDeliveryHandoverNoteCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeDeliveryHandoverConfirmation(context.Context, *http.Request) (application.ConfirmDeliveryHandoverCommand, error) {
	return application.ConfirmDeliveryHandoverCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeDeliveryHandoverConclusion(context.Context, *http.Request) (application.ConcludeDeliveryHandoverCommand, error) {
	return application.ConcludeDeliveryHandoverCommand{}, ErrAccessChannelNotConfigured
}

// handoverEndpoint 是五口共用的 HTTP 外壳：方法门 → Intake（各格照信封族映射）→ 编排 → 按 ADR-0022 映射答复。
func handoverEndpoint[Command any](
	intake func(context.Context, *http.Request) (Command, error),
	handle func(context.Context, Command) (application.DeliveryHandoverResult, error),
	respond func(http.ResponseWriter, Command, application.DeliveryHandoverResult),
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake(request.Context(), request)
		if err != nil {
			writeHandoverIntakeProblem(response, err)
			return
		}
		result, err := handle(request.Context(), command)
		switch {
		case errors.Is(err, application.ErrDeliveryHandoverNoteNotFound):
			writeProblem(response, http.StatusNotFound, codeDeliveryHandoverNoteNotVisible)
			return
		case errors.Is(err, application.ErrDeliveryHandoverCommandRejected):
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		case err != nil:
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if result.Outcome().String() == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		respond(response, command, result)
	})
}

// NewOpenDeliveryHandoverNoteEndpoint 交回开立口。首次开立答 201；已有结果与来源冲突答 200 带 outcome，回显在册那一张。
func NewOpenDeliveryHandoverNoteEndpoint(intake DeliveryHandoverNoteOpeningIntake, handler DeliveryHandoverNoteHandler) http.Handler {
	return handoverEndpoint(intake.IntakeDeliveryHandoverNoteOpening, handler.Open, writeOperatorHandoverAnswer[application.OpenDeliveryHandoverNoteCommand])
}

// NewAssignToDeliveryHandoverNoteEndpoint 交回归入口。首次归入答 201；已有结果、来源冲突与交接单状态不符答 200 带 outcome。
func NewAssignToDeliveryHandoverNoteEndpoint(intake DeliveryHandoverAssignmentIntake, handler DeliveryHandoverNoteHandler) http.Handler {
	return handoverEndpoint(intake.IntakeDeliveryHandoverAssignment, handler.Assign, writeOperatorHandoverAnswer[application.AssignToDeliveryHandoverNoteCommand])
}

// NewCloseDeliveryHandoverNoteEndpoint 交回关闭口。首次关闭答 201；已有结果与来源冲突答 200 带 outcome。
func NewCloseDeliveryHandoverNoteEndpoint(intake DeliveryHandoverClosureIntake, handler DeliveryHandoverNoteHandler) http.Handler {
	return handoverEndpoint(intake.IntakeDeliveryHandoverClosure, handler.Close, writeOperatorHandoverAnswer[application.CloseDeliveryHandoverNoteCommand])
}

// NewConfirmDeliveryHandoverEndpoint 交回运营决定确认口。首次确认答 201；已有结果、已有结论、不在授权交付关系内与未配置答 200 带
// outcome——服务端知道为什么没落结论，这不是调用方的错，也不是没形成答案。授权交付判断读不动答 500：没形成答案。
func NewConfirmDeliveryHandoverEndpoint(intake DeliveryHandoverConfirmationIntake, handler DeliveryHandoverNoteHandler) http.Handler {
	return handoverEndpoint(intake.IntakeDeliveryHandoverConfirmation, handler.ConfirmByOperationDecision, writeOperatorHandoverAnswer[application.ConfirmDeliveryHandoverCommand])
}

// NewConcludeDeliveryHandoverEndpoint 交回客户结论口。首次确认或否认答 201；已有结果、已有结论与交接单状态不符答 200 带 outcome；
// 交接单不存在或没声称信封里的客户账户答 404，两种同答。
func NewConcludeDeliveryHandoverEndpoint(intake DeliveryHandoverConclusionIntake, handler DeliveryHandoverNoteHandler) http.Handler {
	return handoverEndpoint(intake.IntakeDeliveryHandoverConclusion, handler.ConcludeByCustomer, writeCustomerHandoverAnswer)
}

// writeHandoverIntakeProblem 先映射作业事实一族的设备一格与首方客户族自己的两格，其余各格与操作者族各口同码。
func writeHandoverIntakeProblem(response http.ResponseWriter, err error) {
	if errors.Is(err, ErrDeviceNotInUse) {
		writeProblem(response, http.StatusForbidden, codeDeviceNotInUse)
		return
	}
	writeCustomerIntakeProblem(response, err)
}

func handoverStatus(outcome application.DeliveryHandoverOutcome) int {
	switch outcome {
	case application.DeliveryHandoverNoteOpened, application.DeliveryHandoverUnitAssigned, application.DeliveryHandoverNoteClosed,
		application.DeliveryHandoverBindingConfirmed, application.DeliveryHandoverBindingDenied:
		return http.StatusCreated
	default:
		return http.StatusOK
	}
}

// deliveryHandoverResponse 是操作者面四口的封闭响应形状。outcome 取编排结果的原名；stateMismatch 只随交接单状态不符出现；
// handoverNote 是交接单此刻的全貌（含关闭）；member 随归入、binding 随结论出现，是答案所指的那一份（新落的或在册的）；
// scopeEstablishedAt 只在交接单此刻已成立范围时出现。
type deliveryHandoverResponse struct {
	Outcome            string                       `json:"outcome"`
	StateMismatch      string                       `json:"stateMismatch,omitempty"`
	HandoverNote       *deliveryHandoverNoteBody    `json:"handoverNote,omitempty"`
	Member             *deliveryHandoverMemberBody  `json:"member,omitempty"`
	Binding            *deliveryHandoverBindingBody `json:"binding,omitempty"`
	ScopeEstablishedAt string                       `json:"scopeEstablishedAt,omitempty"`
}

type deliveryHandoverNoteBody struct {
	NoteID          string   `json:"noteId"`
	Node            string   `json:"node"`
	DeliveringParty string   `json:"deliveringParty"`
	ClaimedAccounts []string `json:"claimedAccounts"`
	OpenedAt        string   `json:"openedAt"`
	ClosureFactID   string   `json:"closureFactId,omitempty"`
	ClosedAt        string   `json:"closedAt,omitempty"`
}

type deliveryHandoverMemberBody struct {
	FactID     string `json:"factId"`
	Unit       string `json:"unit"`
	Membership string `json:"membership"`
	FormedAt   string `json:"formedAt"`
}

// deliveryHandoverBindingBody 回显一份结论。deliveringParticipant 与 basis 只随运营决定的确认出现；客户面不回显这两格。
type deliveryHandoverBindingBody struct {
	CustomerAccount       string `json:"customerAccount"`
	Conclusion            string `json:"conclusion"`
	Route                 string `json:"route"`
	ConcludedAt           string `json:"concludedAt"`
	DeliveringParticipant string `json:"deliveringParticipant,omitempty"`
	Basis                 string `json:"basis,omitempty"`
}

func writeOperatorHandoverAnswer[Command any](response http.ResponseWriter, _ Command, result application.DeliveryHandoverResult) {
	body := deliveryHandoverResponse{Outcome: result.Outcome().String(), StateMismatch: result.StateMismatch().String()}
	if note := result.Note(); note.ID().String() != "" {
		body.HandoverNote = &deliveryHandoverNoteBody{
			NoteID:          note.ID().String(),
			Node:            note.Node().String(),
			DeliveringParty: note.DeliveringParty().String(),
			ClaimedAccounts: []string{},
			OpenedAt:        rfc3339(note.OpenedAt()),
		}
		for _, account := range note.ClaimedAccounts() {
			body.HandoverNote.ClaimedAccounts = append(body.HandoverNote.ClaimedAccounts, account.String())
		}
		if closure, closed := result.Closure(); closed {
			body.HandoverNote.ClosureFactID, body.HandoverNote.ClosedAt = closure.FactID(), rfc3339(closure.ClosedAt())
		}
	}
	if member, present := result.Member(); present {
		body.Member = &deliveryHandoverMemberBody{
			FactID: member.FactID(), Unit: member.Unit().String(), Membership: member.Kind().String(), FormedAt: rfc3339(member.FormedAt()),
		}
	}
	if binding, present := result.Binding(); present {
		body.Binding = bindingBodyOf(binding)
		body.Binding.DeliveringParticipant, body.Binding.Basis = binding.OperationDecisionEvidence()
	}
	if at, established := result.ScopeEstablishedAt(); established {
		body.ScopeEstablishedAt = rfc3339(at)
	}
	writeJSON(response, handoverStatus(result.Outcome()), body)
}

// deliveryHandoverConclusionResponse 是客户面那一口的封闭响应形状：只回显交接单标识与信封里那个客户账户的结论。不回显声称了谁、
// 成员与范围成立没有——那几格会让一个客户账户看到别的客户账户在不在声称之列、确认没有（UC-NO-006「权限、隔离与审计」）。
type deliveryHandoverConclusionResponse struct {
	Outcome       string                       `json:"outcome"`
	StateMismatch string                       `json:"stateMismatch,omitempty"`
	NoteID        string                       `json:"noteId"`
	Binding       *deliveryHandoverBindingBody `json:"binding,omitempty"`
}

func writeCustomerHandoverAnswer(response http.ResponseWriter, command application.ConcludeDeliveryHandoverCommand, result application.DeliveryHandoverResult) {
	body := deliveryHandoverConclusionResponse{
		Outcome:       result.Outcome().String(),
		StateMismatch: result.StateMismatch().String(),
		NoteID:        command.Note.String(),
	}
	if binding, present := result.Binding(); present && binding.Account() == command.CustomerAccount {
		body.Binding = bindingBodyOf(binding)
	}
	writeJSON(response, handoverStatus(result.Outcome()), body)
}

func bindingBodyOf(binding domain.DeliveryHandoverBinding) *deliveryHandoverBindingBody {
	return &deliveryHandoverBindingBody{
		CustomerAccount: binding.Account().String(),
		Conclusion:      binding.Conclusion().String(),
		Route:           binding.Route().String(),
		ConcludedAt:     rfc3339(binding.ConcludedAt()),
	}
}
