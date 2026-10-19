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
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 本文件是送站预报的提交口与撤回口（UC-NO-006 应用流程步骤 1、2；ADR-0212 决定二）。两口挂首方客户渠道族（ADR-0139），是节点作业在
// 客户面开的写口：租户与客户账户只取自信封，预报是生产写、准入范围在铸造侧判（ADR-0139 决定八）。

// 首方客户族在本包的两格答复（ADR-0139 决定四），与 ErrAccessChannelNotConfigured、ErrOutsideAdmissionScope、
// ErrIdentityDependencyUnavailable 并列，各对一种恢复动作（ADR-0029）。操作者族那两格不复用：持令牌的是货主客户的系统，错误码若
// 仍叫操作者，管授予的人会去操作者册里找一个不存在的人。
var (
	// ErrCustomerClientCredentialRejected：令牌缺失、过期、校验不过，或册上要求证书绑定而这次出示对不上（401）——持凭据的客户系统
	// 去换令牌。
	ErrCustomerClientCredentialRejected = errors.New("node operations http: customer client credential rejected")
	// ErrCustomerClientNotGranted：令牌有效，但客户端不在客户渠道册，或此刻没有这一口能力的生效授予（403）——租户运营去登记或
	// 恢复授予。几种情形同答这一格，不说哪一半不对。
	ErrCustomerClientNotGranted = errors.New("node operations http: customer client holds no effective grant for this capability")
)

const (
	codeCustomerClientCredentialRejected = "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"
	codeCustomerClientNotGranted         = "CUSTOMER_CLIENT_NOT_GRANTED"
	codeDeliveryPreAdviceNotVisible      = "DELIVERY_PRE_ADVICE_NOT_VISIBLE"
)

// PresentedCustomerCredential 是这一次 HTTP 出示：Bearer 令牌，与这次 TLS 连接上的客户端证书（DER；没有就为空）。证书只用来核对
// 证书绑定令牌（RFC 8705）。
type PresentedCustomerCredential struct {
	Token       string
	Certificate []byte
}

// CustomerIdentity 是认证结果：租户与客户账户取自客户渠道册上的绑定，Channel 是交来请求的客户端（来源格），RequestKey 是首方族
// 推导口从接入请求标识铸成的来源请求键。
type CustomerIdentity struct {
	Tenant          domain.TenantID
	CustomerAccount domain.CustomerAccountReference
	Channel         string
	RequestKey      string
}

// CustomerPreAdviceAuthenticator 认证预报两口的首方出示，按口取各自那一项能力。失败只交回本包的格；从共享接入身份能力译过来的
// 那一层在 adapters/accessidentity（跨上下文翻译的位置，见 internal/architecture 的边界门禁）。
//
// accessRequestID 在令牌、册、授予与准入都过了之后才被调用：认证没过的调用方看不到载荷校验的结果。它交回载荷里的接入请求标识；
// 它自己的错原样交回。
type CustomerPreAdviceAuthenticator interface {
	AuthenticateDeliveryPreAdviceSubmission(ctx context.Context, credential PresentedCustomerCredential, accessRequestID func() (string, error)) (CustomerIdentity, error)
	AuthenticateDeliveryPreAdviceWithdrawal(ctx context.Context, credential PresentedCustomerCredential, accessRequestID func() (string, error)) (CustomerIdentity, error)
}

// DeliveryPreAdviceSubmissionIntake 与 DeliveryPreAdviceWithdrawalIntake 把一次已认证的请求译成命令。
type DeliveryPreAdviceSubmissionIntake interface {
	IntakeDeliveryPreAdviceSubmission(ctx context.Context, request *http.Request) (application.SubmitDeliveryPreAdviceCommand, error)
}

type DeliveryPreAdviceWithdrawalIntake interface {
	IntakeDeliveryPreAdviceWithdrawal(ctx context.Context, request *http.Request) (application.WithdrawDeliveryPreAdviceCommand, error)
}

// DeliveryPreAdviceHandler 是两口转交的预报编排，装配负责把每次调用包进一笔事务。
type DeliveryPreAdviceHandler interface {
	Submit(ctx context.Context, command application.SubmitDeliveryPreAdviceCommand) (application.DeliveryPreAdviceSubmissionResult, error)
	Withdraw(ctx context.Context, command application.WithdrawDeliveryPreAdviceCommand) (application.DeliveryPreAdviceWithdrawalResult, error)
}

// CustomerPreAdviceIntake 是首方客户渠道在预报两口的 Intake。号码过形状门：界与 PS 入册门同一道（ports.PreAdvisedNumberGate 的注释）。
type CustomerPreAdviceIntake struct {
	authenticator CustomerPreAdviceAuthenticator
	numbers       ports.PreAdvisedNumberGate
}

var (
	_ DeliveryPreAdviceSubmissionIntake = (*CustomerPreAdviceIntake)(nil)
	_ DeliveryPreAdviceWithdrawalIntake = (*CustomerPreAdviceIntake)(nil)
)

// NewCustomerPreAdviceIntake 由装配点构造。立不起来的依赖在这里拒：装配错误要在启动时暴露，不该等到第一个请求。
func NewCustomerPreAdviceIntake(authenticator CustomerPreAdviceAuthenticator, numbers ports.PreAdvisedNumberGate) (*CustomerPreAdviceIntake, error) {
	if authenticator == nil || numbers == nil {
		return nil, errors.New("node operations http: customer pre-advice intake needs an authenticator and a number gate")
	}
	return &CustomerPreAdviceIntake{authenticator: authenticator, numbers: numbers}, nil
}

// deliveryPreAdviceSubmissionPayload 是预报提交口的线格式：接入请求标识、节点、预计送达时刻、客户包裹件号集合、可选的交付方线索
// （UC-NO-006 输入语义契约「送站预报」）。没有租户、客户账户与来源——封闭解码下自报这几格都是未知字段。指针区分「没给」与「给了
// 空」：交付方线索给了就不许空白，缺席与在场是两份内容。
type deliveryPreAdviceSubmissionPayload struct {
	AccessRequestID       string     `json:"accessRequestId"`
	Node                  string     `json:"node"`
	ExpectedArrivalAt     *time.Time `json:"expectedArrivalAt"`
	CustomerParcelNumbers []string   `json:"customerParcelNumbers"`
	DeliveringParty       *string    `json:"deliveringParty"`
}

// deliveryPreAdviceWithdrawalPayload 是预报撤回口的线格式：接入请求标识与预报标识（UC-NO-006 输入语义契约「预报撤回」）。
type deliveryPreAdviceWithdrawalPayload struct {
	AccessRequestID string `json:"accessRequestId"`
	PreAdviceID     string `json:"preAdviceId"`
}

// IntakeDeliveryPreAdviceSubmission 译预报提交（`/node-operations/delivery-pre-advices`）。先认证、后读载荷。坏载荷的各种都答坏报文：
// 缺格、空白、带 NUL、时刻解不出、号码为空、重复或进不了形状门。号码与节点逐字交下去——替客户改一个字符就是另一个号、另一个节点。
func (intake *CustomerPreAdviceIntake) IntakeDeliveryPreAdviceSubmission(ctx context.Context, request *http.Request) (application.SubmitDeliveryPreAdviceCommand, error) {
	none := application.SubmitDeliveryPreAdviceCommand{}
	var payload deliveryPreAdviceSubmissionPayload
	customer, err := intake.authenticator.AuthenticateDeliveryPreAdviceSubmission(ctx, presentedCustomerCredential(request), func() (string, error) {
		if err := decodeClosedDocument(request, &payload); err != nil {
			return "", err
		}
		return payload.AccessRequestID, nil
	})
	if err != nil {
		return none, err
	}
	command := application.SubmitDeliveryPreAdviceCommand{
		TenantID:        customer.Tenant,
		CustomerAccount: customer.CustomerAccount,
		Channel:         customer.Channel,
		RequestKey:      customer.RequestKey,
	}
	if err := requiredField("node", payload.Node); err != nil {
		return none, err
	}
	if command.Node, err = domain.NewNodeReference(payload.Node); err != nil {
		return none, malformedField("node", err)
	}
	if payload.ExpectedArrivalAt == nil || payload.ExpectedArrivalAt.IsZero() {
		return none, fmt.Errorf("%w: expectedArrivalAt is required", ErrMalformedRequest)
	}
	command.ExpectedArrivalAt = *payload.ExpectedArrivalAt
	if len(payload.CustomerParcelNumbers) == 0 {
		return none, fmt.Errorf("%w: customerParcelNumbers needs at least one number", ErrMalformedRequest)
	}
	seen := make(map[string]bool, len(payload.CustomerParcelNumbers))
	for index, value := range payload.CustomerParcelNumbers {
		name := fmt.Sprintf("customerParcelNumbers[%d]", index)
		if seen[value] {
			return none, fmt.Errorf("%w: %s repeats a number", ErrMalformedRequest, name)
		}
		seen[value] = true
		number, err := intake.numbers.AdmitPreAdvisedNumber(value)
		if err != nil {
			return none, malformedField(name, err)
		}
		command.Numbers = append(command.Numbers, number)
	}
	if payload.DeliveringParty != nil {
		if err := requiredField("deliveringParty", *payload.DeliveringParty); err != nil {
			return none, err
		}
		if command.DeliveringParty, err = domain.NewDeliveringPartyReference(*payload.DeliveringParty); err != nil {
			return none, malformedField("deliveringParty", err)
		}
	}
	return command, nil
}

// IntakeDeliveryPreAdviceWithdrawal 译预报撤回（`/node-operations/delivery-pre-advice-withdrawals`）。载荷只指名预报；撤的是哪个客户
// 账户的，只看信封。
func (intake *CustomerPreAdviceIntake) IntakeDeliveryPreAdviceWithdrawal(ctx context.Context, request *http.Request) (application.WithdrawDeliveryPreAdviceCommand, error) {
	none := application.WithdrawDeliveryPreAdviceCommand{}
	var payload deliveryPreAdviceWithdrawalPayload
	customer, err := intake.authenticator.AuthenticateDeliveryPreAdviceWithdrawal(ctx, presentedCustomerCredential(request), func() (string, error) {
		if err := decodeClosedDocument(request, &payload); err != nil {
			return "", err
		}
		return payload.AccessRequestID, nil
	})
	if err != nil {
		return none, err
	}
	if err := requiredField("preAdviceId", payload.PreAdviceID); err != nil {
		return none, err
	}
	preAdvice, err := domain.NewDeliveryPreAdviceID(payload.PreAdviceID)
	if err != nil {
		return none, malformedField("preAdviceId", err)
	}
	return application.WithdrawDeliveryPreAdviceCommand{
		TenantID:        customer.Tenant,
		CustomerAccount: customer.CustomerAccount,
		Channel:         customer.Channel,
		RequestKey:      customer.RequestKey,
		PreAdvice:       preAdvice,
	}, nil
}

func requiredField(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: %s is required", ErrMalformedRequest, name)
	}
	if strings.ContainsRune(value, 0) {
		return fmt.Errorf("%w: %s carries NUL", ErrMalformedRequest, name)
	}
	return nil
}

func malformedField(name string, err error) error {
	return fmt.Errorf("%w: %s: %v", ErrMalformedRequest, name, err)
}

func presentedCustomerCredential(request *http.Request) PresentedCustomerCredential {
	var certificate []byte
	if request.TLS != nil && len(request.TLS.PeerCertificates) > 0 {
		certificate = request.TLS.PeerCertificates[0].Raw
	}
	return PresentedCustomerCredential{Token: httpapi.BearerToken(request), Certificate: certificate}
}

var (
	_ DeliveryPreAdviceSubmissionIntake = UnconfiguredIntake{}
	_ DeliveryPreAdviceWithdrawalIntake = UnconfiguredIntake{}
)

// IntakeDeliveryPreAdviceSubmission 同 IntakeReception：不读请求，只答未配置。
func (UnconfiguredIntake) IntakeDeliveryPreAdviceSubmission(context.Context, *http.Request) (application.SubmitDeliveryPreAdviceCommand, error) {
	return application.SubmitDeliveryPreAdviceCommand{}, ErrAccessChannelNotConfigured
}

// IntakeDeliveryPreAdviceWithdrawal 同 IntakeReception：不读请求，只答未配置。
func (UnconfiguredIntake) IntakeDeliveryPreAdviceWithdrawal(context.Context, *http.Request) (application.WithdrawDeliveryPreAdviceCommand, error) {
	return application.WithdrawDeliveryPreAdviceCommand{}, ErrAccessChannelNotConfigured
}

// NewSubmitDeliveryPreAdviceEndpoint 交回预报提交的 HTTP 入口。按 ADR-0022 映射：首次形成答 201；已有结果与来源冲突都是形成了的
// 答案，答 200 带 outcome 并回显在册那一份。编排拒形答坏载荷；编排读写不动答没形成答案，册上什么也没落。
func NewSubmitDeliveryPreAdviceEndpoint(intake DeliveryPreAdviceSubmissionIntake, handler DeliveryPreAdviceHandler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeDeliveryPreAdviceSubmission(request.Context(), request)
		if err != nil {
			writeCustomerIntakeProblem(response, err)
			return
		}
		result, err := handler.Submit(request.Context(), command)
		if err != nil {
			writePreAdviceProblem(response, err)
			return
		}
		writePreAdviceOutcome(response, result.Outcome(), deliveryPreAdviceResponse{PreAdvice: preAdviceBodyOf(result.PreAdvice())},
			application.DeliveryPreAdviceRegistered)
	})
}

// NewWithdrawDeliveryPreAdviceEndpoint 交回预报撤回的 HTTP 入口。映射同提交口；指名的预报看不见（不存在，或不属信封里那个客户账户）
// 答 404，两种同答，与越权探针同答。
func NewWithdrawDeliveryPreAdviceEndpoint(intake DeliveryPreAdviceWithdrawalIntake, handler DeliveryPreAdviceHandler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeDeliveryPreAdviceWithdrawal(request.Context(), request)
		if err != nil {
			writeCustomerIntakeProblem(response, err)
			return
		}
		result, err := handler.Withdraw(request.Context(), command)
		if err != nil {
			writePreAdviceProblem(response, err)
			return
		}
		withdrawal := result.Withdrawal()
		writePreAdviceOutcome(response, result.Outcome(), deliveryPreAdviceResponse{Withdrawal: &deliveryPreAdviceWithdrawalBody{
			PreAdviceID: withdrawal.PreAdvice().String(),
			WithdrawnAt: rfc3339(withdrawal.WithdrawnAt()),
		}}, application.DeliveryPreAdviceWithdrawn)
	})
}

// writeCustomerIntakeProblem 先映射首方客户族自己的两格，其余各格与操作者族各口同码，交给 writeIntakeProblem。
func writeCustomerIntakeProblem(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrCustomerClientCredentialRejected):
		writeProblem(response, http.StatusUnauthorized, codeCustomerClientCredentialRejected)
	case errors.Is(err, ErrCustomerClientNotGranted):
		writeProblem(response, http.StatusForbidden, codeCustomerClientNotGranted)
	default:
		writeIntakeProblem(response, err)
	}
}

func writePreAdviceProblem(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, application.ErrDeliveryPreAdviceNotFound):
		writeProblem(response, http.StatusNotFound, codeDeliveryPreAdviceNotVisible)
	case errors.Is(err, application.ErrDeliveryPreAdviceRejected):
		writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
	default:
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
	}
}

// deliveryPreAdviceResponse 是两口的封闭响应形状。outcome 取编排结果的原名；preAdvice 随提交口出现，是答案所指的那一份预报（新落的
// 或在册的）；withdrawal 随撤回口出现，是答案所指的那一份撤回。
type deliveryPreAdviceResponse struct {
	Outcome    string                           `json:"outcome"`
	PreAdvice  *deliveryPreAdviceBody           `json:"preAdvice,omitempty"`
	Withdrawal *deliveryPreAdviceWithdrawalBody `json:"withdrawal,omitempty"`
}

// deliveryPreAdviceBody 回显一份预报：签发的标识（送站时随交付出示）与客户交来的内容。来源请求键与内容摘要不回显：客户手里本来
// 就有接入请求标识，摘要是本上下文判重用的。
type deliveryPreAdviceBody struct {
	PreAdviceID           string   `json:"preAdviceId"`
	CustomerAccount       string   `json:"customerAccount"`
	Node                  string   `json:"node"`
	ExpectedArrivalAt     string   `json:"expectedArrivalAt"`
	CustomerParcelNumbers []string `json:"customerParcelNumbers"`
	DeliveringParty       string   `json:"deliveringParty,omitempty"`
	FormedAt              string   `json:"formedAt"`
}

type deliveryPreAdviceWithdrawalBody struct {
	PreAdviceID string `json:"preAdviceId"`
	WithdrawnAt string `json:"withdrawnAt"`
}

func preAdviceBodyOf(preAdvice domain.DeliveryPreAdvice) *deliveryPreAdviceBody {
	body := &deliveryPreAdviceBody{
		PreAdviceID:           preAdvice.ID().String(),
		CustomerAccount:       preAdvice.CustomerAccount().String(),
		Node:                  preAdvice.Node().String(),
		ExpectedArrivalAt:     rfc3339(preAdvice.ExpectedArrivalAt()),
		CustomerParcelNumbers: []string{},
		FormedAt:              rfc3339(preAdvice.FormedAt()),
	}
	for _, number := range preAdvice.Numbers() {
		body.CustomerParcelNumbers = append(body.CustomerParcelNumbers, number.String())
	}
	if clue, present := preAdvice.DeliveringParty(); present {
		body.DeliveringParty = clue.String()
	}
	return body
}

// writePreAdviceOutcome 写一个形成了的答案：首次形成（created 那一格）答 201，其余各格答 200；答不出名字的 outcome 是实现坏了，答
// 500、不带 outcome。
func writePreAdviceOutcome(
	response http.ResponseWriter,
	outcome application.DeliveryPreAdviceOutcome,
	body deliveryPreAdviceResponse,
	created application.DeliveryPreAdviceOutcome,
) {
	body.Outcome = outcome.String()
	if body.Outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	status := http.StatusOK
	if outcome == created {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
