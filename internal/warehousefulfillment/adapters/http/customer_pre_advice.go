package warehousehttp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 本文件是入库预报的货主口（UC-WF-003）。问题详情用首方族的 application/problem+json，不沿用本包运营口的 {"error":{"code"}}。
// 业务答复仍是 application/json，outcome 用预报核心已有的字面量。租户与货主只取自信封。

// ErrCustomerClientCredentialRejected：令牌缺失、过期、校验不过，或册上要求证书绑定而这次出示对不上（401）。
var ErrCustomerClientCredentialRejected = errors.New("warehouse fulfillment http: customer client credential rejected")

// ErrCustomerClientNotGranted：令牌有效，但客户端不在客户渠道册，或此刻没有这一口能力的生效授予（403）。
var ErrCustomerClientNotGranted = errors.New("warehouse fulfillment http: customer client holds no effective grant for this capability")

const (
	codeCustomerClientCredentialRejected = "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"
	codeCustomerClientNotGranted         = "CUSTOMER_CLIENT_NOT_GRANTED"
)

// PresentedCustomerCredential 是这一次 HTTP 出示：Bearer 令牌，与这次 TLS 连接上的客户端证书（DER；没有就为空）。
type PresentedCustomerCredential struct {
	Token       string
	Certificate []byte
}

// CustomerCommandIdentity 是三个写口认证之后的信封：租户与货主取自册上绑定，来源是客户渠道来源，键是这一口推导出的来源请求键。
// 登记责任方也取自信封上的同一来源，供受理用例记下；货主读口不把它带出去。
type CustomerCommandIdentity struct {
	Tenant     domain.TenantID
	Owner      domain.OwnerAccountID
	Source     domain.SubmissionSource
	RequestKey domain.RequestKey
	Registrant domain.Registrant
}

// CustomerReadIdentity 是查阅口的身份。查阅不形成来源请求，也没有登记责任方。
type CustomerReadIdentity struct {
	Tenant domain.TenantID
	Owner  domain.OwnerAccountID
}

// CustomerInboundPreAdviceAuthenticator 按口认证。写口的 accessRequestID 在令牌、册、授予与准入都过了之后才被调用。
type CustomerInboundPreAdviceAuthenticator interface {
	AuthenticateInboundPreAdviceSubmission(ctx context.Context, credential PresentedCustomerCredential, accessRequestID func() (string, error)) (CustomerCommandIdentity, error)
	AuthenticateInboundPreAdviceArrivalNotice(ctx context.Context, credential PresentedCustomerCredential, accessRequestID func() (string, error)) (CustomerCommandIdentity, error)
	AuthenticateInboundPreAdviceWithdrawal(ctx context.Context, credential PresentedCustomerCredential, accessRequestID func() (string, error)) (CustomerCommandIdentity, error)
	AuthenticateWarehouseRead(ctx context.Context, credential PresentedCustomerCredential) (CustomerReadIdentity, error)
}

// CustomerInboundPreAdvicePorts 是货主四类入口要的 Intake。未配置时四口都答接入渠道未配置，一个字节的业务内容都不读。
type CustomerInboundPreAdvicePorts interface {
	IntakeInboundPreAdviceSubmission(ctx context.Context, request *http.Request) (application.AcceptPreAdviceCommand, error)
	IntakeInboundPreAdviceArrivalNotice(ctx context.Context, request *http.Request) (application.UpdatePreAdviceNoticeCommand, error)
	IntakeInboundPreAdviceWithdrawal(ctx context.Context, request *http.Request) (application.WithdrawPreAdviceCommand, error)
	IntakeWarehouseRead(ctx context.Context, request *http.Request) (CustomerReadIdentity, error)
}

// CustomerInboundPreAdviceIntake 是首方客户渠道在入库预报货主口的 Intake。
type CustomerInboundPreAdviceIntake struct {
	authenticator CustomerInboundPreAdviceAuthenticator
}

func NewCustomerInboundPreAdviceIntake(authenticator CustomerInboundPreAdviceAuthenticator) (*CustomerInboundPreAdviceIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: inbound pre-advice customer intake needs an authenticator")
	}
	return &CustomerInboundPreAdviceIntake{authenticator: authenticator}, nil
}

// OpenCustomerInboundPreAdviceIntake 交回端点用的 Intake。缺席时是字面量未配置，避免把带类型的空指针塞进接口。
func OpenCustomerInboundPreAdviceIntake(intake *CustomerInboundPreAdviceIntake) CustomerInboundPreAdvicePorts {
	if intake == nil {
		return unconfiguredCustomerInboundPreAdvice{}
	}
	return intake
}

type unconfiguredCustomerInboundPreAdvice struct{}

func (unconfiguredCustomerInboundPreAdvice) IntakeInboundPreAdviceSubmission(context.Context, *http.Request) (application.AcceptPreAdviceCommand, error) {
	return application.AcceptPreAdviceCommand{}, ErrAccessChannelNotConfigured
}

func (unconfiguredCustomerInboundPreAdvice) IntakeInboundPreAdviceArrivalNotice(context.Context, *http.Request) (application.UpdatePreAdviceNoticeCommand, error) {
	return application.UpdatePreAdviceNoticeCommand{}, ErrAccessChannelNotConfigured
}

func (unconfiguredCustomerInboundPreAdvice) IntakeInboundPreAdviceWithdrawal(context.Context, *http.Request) (application.WithdrawPreAdviceCommand, error) {
	return application.WithdrawPreAdviceCommand{}, ErrAccessChannelNotConfigured
}

func (unconfiguredCustomerInboundPreAdvice) IntakeWarehouseRead(context.Context, *http.Request) (CustomerReadIdentity, error) {
	return CustomerReadIdentity{}, ErrAccessChannelNotConfigured
}

func (intake *CustomerInboundPreAdviceIntake) IntakeInboundPreAdviceSubmission(
	ctx context.Context,
	request *http.Request,
) (application.AcceptPreAdviceCommand, error) {
	var none application.AcceptPreAdviceCommand
	var fields registrationjson.OwnerPreAdviceSubmissionFields
	identity, err := intake.authenticator.AuthenticateInboundPreAdviceSubmission(ctx, presentedCustomerCredential(request), func() (string, error) {
		read, err := registrationjson.ReadOwnerPreAdviceSubmission(bodyOrEmpty(request))
		if err != nil {
			return "", asMalformed(err)
		}
		fields = read
		return read.AccessRequestID, nil
	})
	if err != nil {
		return none, err
	}
	submission, err := domain.NewOwnerPreAdviceSubmission(identity.Source, identity.RequestKey)
	if err != nil {
		return none, err
	}
	return application.AcceptPreAdviceCommand{
		Tenant: identity.Tenant, Owner: identity.Owner, Warehouse: fields.Warehouse,
		Arrival: fields.Arrival, Inbound: fields.Inbound, GoodsLines: fields.GoodsLines, Cartons: fields.Cartons,
		ExpectedAt: fields.ExpectedAt, Clue: fields.Clue, Reference: fields.Reference,
		Submission: submission, Registrant: identity.Registrant,
	}, nil
}

func (intake *CustomerInboundPreAdviceIntake) IntakeInboundPreAdviceArrivalNotice(
	ctx context.Context,
	request *http.Request,
) (application.UpdatePreAdviceNoticeCommand, error) {
	var none application.UpdatePreAdviceNoticeCommand
	var fields registrationjson.OwnerPreAdviceNoticeFields
	identity, err := intake.authenticator.AuthenticateInboundPreAdviceArrivalNotice(ctx, presentedCustomerCredential(request), func() (string, error) {
		read, err := registrationjson.ReadOwnerPreAdviceNotice(bodyOrEmpty(request))
		if err != nil {
			return "", asMalformed(err)
		}
		fields = read
		return read.AccessRequestID, nil
	})
	if err != nil {
		return none, err
	}
	submission, err := domain.NewOwnerPreAdviceSubmission(identity.Source, identity.RequestKey)
	if err != nil {
		return none, err
	}
	return application.UpdatePreAdviceNoticeCommand{
		Tenant: identity.Tenant, Owner: identity.Owner, ID: fields.ID, ExpectedAt: fields.ExpectedAt,
		Clue: fields.Clue, Reference: fields.Reference, Submission: submission, Registrant: identity.Registrant,
	}, nil
}

func (intake *CustomerInboundPreAdviceIntake) IntakeInboundPreAdviceWithdrawal(
	ctx context.Context,
	request *http.Request,
) (application.WithdrawPreAdviceCommand, error) {
	var none application.WithdrawPreAdviceCommand
	var fields registrationjson.OwnerPreAdviceWithdrawalFields
	identity, err := intake.authenticator.AuthenticateInboundPreAdviceWithdrawal(ctx, presentedCustomerCredential(request), func() (string, error) {
		read, err := registrationjson.ReadOwnerPreAdviceWithdrawal(bodyOrEmpty(request))
		if err != nil {
			return "", asMalformed(err)
		}
		fields = read
		return read.AccessRequestID, nil
	})
	if err != nil {
		return none, err
	}
	submission, err := domain.NewOwnerPreAdviceSubmission(identity.Source, identity.RequestKey)
	if err != nil {
		return none, err
	}
	return application.WithdrawPreAdviceCommand{
		Tenant: identity.Tenant, Owner: identity.Owner, ID: fields.ID,
		Submission: submission, Registrant: identity.Registrant,
	}, nil
}

func (intake *CustomerInboundPreAdviceIntake) IntakeWarehouseRead(ctx context.Context, request *http.Request) (CustomerReadIdentity, error) {
	identity, err := intake.authenticator.AuthenticateWarehouseRead(ctx, presentedCustomerCredential(request))
	if err != nil {
		return CustomerReadIdentity{}, err
	}
	// 首方读口在认证之后才判请求目标：令牌不过的调用方看不到查询串校验的结果。判据与装配处挂在别的族上的那一道相同。
	if !httpapi.StorableRequestTarget(request) {
		return CustomerReadIdentity{}, &malformedRequest{reason: "the request target is not storable text"}
	}
	return identity, nil
}

func bodyOrEmpty(request *http.Request) io.Reader {
	if request.Body == nil {
		return http.NoBody
	}
	return request.Body
}

func presentedCustomerCredential(request *http.Request) PresentedCustomerCredential {
	var certificate []byte
	if request.TLS != nil && len(request.TLS.PeerCertificates) > 0 {
		certificate = request.TLS.PeerCertificates[0].Raw
	}
	return PresentedCustomerCredential{Token: httpapi.BearerToken(request), Certificate: certificate}
}

// NewSubmitInboundPreAdviceEndpoint 交回货主提交口（POST /v1/warehouse/inbound-pre-advices）。新建答 201，其余业务答复答 200。
func NewSubmitInboundPreAdviceEndpoint(intake CustomerInboundPreAdvicePorts, preAdvices PreAdviceOnBehalf) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseCustomerMethod(response, request, http.MethodPost) {
			return
		}
		command, err := intake.IntakeInboundPreAdviceSubmission(request.Context(), request)
		if err != nil {
			answerCustomerIntake(response, request, err)
			return
		}
		result, err := preAdvices.Accept(request.Context(), command)
		if err != nil {
			slog.ErrorContext(request.Context(), "inbound pre-advice submission did not form an answer",
				"ownerAccount", command.Owner.String(), "warehouseCode", command.Warehouse.String())
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		writeCustomerPreAdviceAnswer(response, request, result)
	})
}

// NewUpdateInboundPreAdviceNoticeEndpoint 交回货主告知更新口。别的货主的预报与查无此预报同答 PRE_ADVICE_NOT_FOUND。
func NewUpdateInboundPreAdviceNoticeEndpoint(intake CustomerInboundPreAdvicePorts, preAdvices PreAdviceOnBehalf) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseCustomerMethod(response, request, http.MethodPost) {
			return
		}
		command, err := intake.IntakeInboundPreAdviceArrivalNotice(request.Context(), request)
		if err != nil {
			answerCustomerIntake(response, request, err)
			return
		}
		result, err := preAdvices.UpdateNotice(request.Context(), command)
		if err != nil {
			slog.ErrorContext(request.Context(), "inbound pre-advice arrival notice did not form an answer",
				"preAdviceId", command.ID.String())
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		writeCustomerPreAdviceAnswer(response, request, result)
	})
}

// NewWithdrawInboundPreAdviceEndpoint 交回货主撤回口。
func NewWithdrawInboundPreAdviceEndpoint(intake CustomerInboundPreAdvicePorts, preAdvices PreAdviceOnBehalf) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseCustomerMethod(response, request, http.MethodPost) {
			return
		}
		command, err := intake.IntakeInboundPreAdviceWithdrawal(request.Context(), request)
		if err != nil {
			answerCustomerIntake(response, request, err)
			return
		}
		result, err := preAdvices.Withdraw(request.Context(), command)
		if err != nil {
			slog.ErrorContext(request.Context(), "inbound pre-advice withdrawal did not form an answer",
				"preAdviceId", command.ID.String())
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		writeCustomerPreAdviceAnswer(response, request, result)
	})
}

// NewListCustomerInboundPreAdvicesEndpoint 交回货主名下预报的列表。货主取自信封，查询串里再带租户或货主即坏载荷。
func NewListCustomerInboundPreAdvicesEndpoint(intake CustomerInboundPreAdvicePorts, queries PreAdviceQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseCustomerMethod(response, request, http.MethodGet) {
			return
		}
		identity, err := intake.IntakeWarehouseRead(request.Context(), request)
		if err != nil {
			answerCustomerIntake(response, request, err)
			return
		}
		pageQuery, err := customerPreAdviceListQuery(request)
		if err != nil {
			writeCustomerQueryProblem(response, request, err)
			return
		}
		if err := arrivalWindowOf(pageQuery); err != nil {
			writeCustomerQueryProblem(response, request, err)
			return
		}
		limit := operatorCatalogueReadLimit
		if pageQuery.PageSize > 0 {
			limit = pageQuery.PageSize
		}
		page, err := queries.ListPreAdvices(request.Context(), identity.Tenant, identity.Owner, limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "customer inbound pre-advice list did not form an answer",
				"ownerAccount", identity.Owner.String())
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		writeJSON(response, http.StatusOK, customerPreAdviceListResponse{
			Outcome: outcomePreAdvicesListed, OwnerAccount: identity.Owner.String(),
			PreAdvices: customerPreAdviceBodiesOf(page.Rows), Page: page.Page(limit),
		})
	})
}

// NewGetCustomerInboundPreAdviceEndpoint 按预报标识读一份。别的货主的与查无此预报都是 found 为假，不答 404。
func NewGetCustomerInboundPreAdviceEndpoint(intake CustomerInboundPreAdvicePorts, queries PreAdviceQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseCustomerMethod(response, request, http.MethodGet) {
			return
		}
		identity, err := intake.IntakeWarehouseRead(request.Context(), request)
		if err != nil {
			answerCustomerIntake(response, request, err)
			return
		}
		id, err := customerPreAdviceIDQuery(request)
		if err != nil {
			writeCustomerQueryProblem(response, request, err)
			return
		}
		reading, found, err := queries.PreAdviceForOwner(request.Context(), identity.Tenant, identity.Owner, id)
		if err != nil {
			slog.ErrorContext(request.Context(), "customer inbound pre-advice did not form an answer",
				"preAdviceId", id.String())
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		answer := customerPreAdviceOneResponse{
			Outcome: outcomePreAdvice, OwnerAccount: identity.Owner.String(), PreAdviceID: id.String(), Found: found,
		}
		if found {
			body := customerPreAdviceReading(reading, true)
			answer.PreAdvice = &body
		}
		writeJSON(response, http.StatusOK, answer)
	})
}

// NewGetCustomerInboundPreAdviceByNumberEndpoint 按单据号读一份。别的货主的与查无此预报都是 found 为假。
func NewGetCustomerInboundPreAdviceByNumberEndpoint(intake CustomerInboundPreAdvicePorts, queries PreAdviceQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseCustomerMethod(response, request, http.MethodGet) {
			return
		}
		identity, err := intake.IntakeWarehouseRead(request.Context(), request)
		if err != nil {
			answerCustomerIntake(response, request, err)
			return
		}
		number, err := customerDocumentNumberQuery(request)
		if err != nil {
			writeCustomerQueryProblem(response, request, err)
			return
		}
		reading, found, err := queries.PreAdviceByDocumentNumber(request.Context(), identity.Tenant, number)
		if err != nil {
			slog.ErrorContext(request.Context(), "customer inbound pre-advice by document number did not form an answer")
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		if found && reading.Advice.Owner() != identity.Owner {
			found = false
		}
		answer := customerPreAdviceByNumberResponse{
			Outcome: outcomePreAdviceByNumber, DocumentNumber: number.String(), Found: found,
		}
		if found {
			body := customerPreAdviceReading(reading, true)
			answer.PreAdvice = &body
		}
		writeJSON(response, http.StatusOK, answer)
	})
}

func writeCustomerPreAdviceAnswer(response http.ResponseWriter, request *http.Request, result application.AcceptPreAdviceResult) {
	name := result.Outcome().String()
	if name == "" {
		writeCustomerProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome, "")
		return
	}
	answer := customerPreAdviceAnswer{Outcome: name}
	status := http.StatusOK
	switch result.Outcome() {
	case application.PreAdviceOutcomeAdvised, application.PreAdviceOutcomeNoticeUpdated,
		application.PreAdviceOutcomeCancelled, application.PreAdviceOutcomeExistingResult:
		advice, present := result.PreAdvice()
		if !present {
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		body := customerPreAdviceBodyOf(advice, true)
		answer.PreAdvice = &body
		if result.Outcome() == application.PreAdviceOutcomeAdvised {
			status = http.StatusCreated
		}
	case application.PreAdviceOutcomeWarehouseNotAccepting:
		reason, present := result.NotAcceptingReason()
		if !present || reason.String() == "" {
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		answer.Reason = reason.String()
	case application.PreAdviceOutcomeContentInvalid:
		issues := result.Issues()
		answer.Issues = make([]preAdviceIssueBody, 0, len(issues))
		for _, issue := range issues {
			answer.Issues = append(answer.Issues, preAdviceIssueBody{
				Kind: issue.Kind().String(), Line: issue.Line(), Carton: issue.Carton(),
			})
		}
	case application.PreAdviceOutcomeStatusMismatch, application.PreAdviceOutcomeReceiptStarted:
		echoed, present := result.Status()
		if !present || echoed.String() == "" {
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		answer.Status = echoed.String()
	}
	writeJSON(response, status, answer)
}

type customerPreAdviceAnswer struct {
	Outcome   string                 `json:"outcome"`
	Reason    string                 `json:"reason,omitempty"`
	Status    string                 `json:"status,omitempty"`
	Issues    []preAdviceIssueBody   `json:"issues,omitempty"`
	PreAdvice *customerPreAdviceBody `json:"preAdvice,omitempty"`
}

type customerPreAdviceListResponse struct {
	Outcome      string                  `json:"outcome"`
	OwnerAccount string                  `json:"ownerAccount"`
	PreAdvices   []customerPreAdviceBody `json:"preAdvices"`
	Page         cataloguepage.Page      `json:"page"`
}

type customerPreAdviceOneResponse struct {
	Outcome      string                 `json:"outcome"`
	OwnerAccount string                 `json:"ownerAccount"`
	PreAdviceID  string                 `json:"preAdviceId"`
	Found        bool                   `json:"found"`
	PreAdvice    *customerPreAdviceBody `json:"preAdvice,omitempty"`
}

type customerPreAdviceByNumberResponse struct {
	Outcome        string                 `json:"outcome"`
	DocumentNumber string                 `json:"documentNumber"`
	Found          bool                   `json:"found"`
	PreAdvice      *customerPreAdviceBody `json:"preAdvice,omitempty"`
}

// customerPreAdviceBody 是货主读到的一份预报。不带登记责任方，也不带收货事实的设备与操作者，也不带完结操作者。
// 没有单据号就不出现 documentNumber，改带 documentNumberAbsence。没有少收就不出现少收行。
// 有待认领到货指着这份预报时带 unclaimedArrival，不带那件到货的内容、标识与数量；没有就不出现这一格。
type customerPreAdviceBody struct {
	PreAdviceID           string                        `json:"preAdviceId"`
	OwnerAccount          string                        `json:"ownerAccount"`
	WarehouseCode         string                        `json:"warehouseCode"`
	ArrivalMode           string                        `json:"arrivalMode"`
	InboundMode           string                        `json:"inboundMode"`
	Status                string                        `json:"status"`
	DocumentNumber        *string                       `json:"documentNumber,omitempty"`
	DocumentNumberAbsence string                        `json:"documentNumberAbsence,omitempty"`
	SubmissionPath        string                        `json:"submissionPath"`
	EvidenceReference     string                        `json:"evidenceReference,omitempty"`
	AcceptedAt            string                        `json:"acceptedAt"`
	GoodsLines            []preAdviceGoodsLineBody      `json:"goodsLines,omitempty"`
	Cartons               []preAdviceCartonBody         `json:"cartons,omitempty"`
	CurrentNotice         *customerPreAdviceNoticeBody  `json:"currentNotice,omitempty"`
	Notices               []customerPreAdviceNoticeBody `json:"notices,omitempty"`
	CumulativeReceived    *[]cumulativeReceivedBody     `json:"cumulativeReceived,omitempty"`
	Receipts              *[]customerReceiptBody        `json:"receipts,omitempty"`
	Closure               *preAdviceClosureBody         `json:"closure,omitempty"`
	UnclaimedArrival      bool                          `json:"unclaimedArrival,omitempty"`
}

// customerReceiptBody 是货主看得到的一次收货。没有设备，也没有操作者。
type customerReceiptBody struct {
	FactID                    string                      `json:"factId"`
	OccurredAt                string                      `json:"occurredAt"`
	Lines                     []receiptLineBody           `json:"lines"`
	DiscrepancyID             string                      `json:"discrepancyId,omitempty"`
	Discrepancies             []receiptDiscrepancyBody    `json:"discrepancies,omitempty"`
	AwaitingOwnerConfirmation bool                        `json:"awaitingOwnerConfirmation,omitempty"`
	Conclusion                *discrepancyConclusionBody  `json:"conclusion,omitempty"`
	SupersededConclusions     []discrepancyConclusionBody `json:"supersededConclusions,omitempty"`
}

type customerPreAdviceNoticeBody struct {
	Version           int    `json:"version"`
	ExpectedAt        string `json:"expectedAt"`
	ArrivalClue       string `json:"arrivalClue,omitempty"`
	OwnerReference    string `json:"ownerReference,omitempty"`
	SubmissionPath    string `json:"submissionPath"`
	EvidenceReference string `json:"evidenceReference,omitempty"`
	RecordedAt        string `json:"recordedAt"`
}

func customerPreAdviceBodyOf(advice domain.PreAdvice, withNotices bool) customerPreAdviceBody {
	body := customerPreAdviceBody{
		PreAdviceID:    advice.ID().String(),
		OwnerAccount:   advice.Owner().String(),
		WarehouseCode:  advice.Warehouse().String(),
		ArrivalMode:    advice.Arrival().String(),
		InboundMode:    advice.Inbound().String(),
		Status:         advice.Status().String(),
		SubmissionPath: advice.Submission().Path().String(),
		AcceptedAt:     rfc3339(advice.AcceptedAt()),
	}
	if number, assigned := advice.DocumentNumber(); assigned {
		text := number.String()
		body.DocumentNumber = &text
	} else if reason := advice.NumberingAbsence().String(); reason != "" {
		body.DocumentNumberAbsence = reason
	}
	if evidence, given := advice.Submission().EvidenceReference(); given {
		body.EvidenceReference = evidence.String()
	}
	if advice.Inbound() == domain.InboundByCarton {
		body.Cartons = preAdviceCartonBodies(advice.Cartons())
	} else {
		body.GoodsLines = preAdviceGoodsLineBodies(advice.GoodsLines())
	}
	notices := advice.Notices()
	if len(notices) > 0 {
		current := customerPreAdviceNoticeBodyOf(notices[len(notices)-1])
		body.CurrentNotice = &current
	}
	if withNotices {
		body.Notices = make([]customerPreAdviceNoticeBody, 0, len(notices))
		for _, notice := range notices {
			body.Notices = append(body.Notices, customerPreAdviceNoticeBodyOf(notice))
		}
	}
	return body
}

func customerPreAdviceReading(reading application.PreAdviceReading, withNotices bool) customerPreAdviceBody {
	body := customerPreAdviceBodyOf(reading.Advice, withNotices)
	cumulative := cumulativeBodies(reading.Cumulative)
	receipts := customerReceiptBodies(reading.Receipts, reading.Conclusions)
	body.CumulativeReceived = &cumulative
	body.Receipts = &receipts
	body.Closure = closureBodyOf(reading.Closure, reading.Conclusions, true, false)
	body.UnclaimedArrival = reading.UnclaimedArrival
	return body
}

func customerPreAdviceBodiesOf(readings []application.PreAdviceReading) []customerPreAdviceBody {
	bodies := make([]customerPreAdviceBody, 0, len(readings))
	for _, reading := range readings {
		bodies = append(bodies, customerPreAdviceReading(reading, false))
	}
	return bodies
}

func customerReceiptBodies(
	receipts []domain.RecordedReceipt,
	conclusions []application.DiscrepancyConclusionView,
) []customerReceiptBody {
	bodies := make([]customerReceiptBody, 0, len(receipts))
	for _, receipt := range receiptBodies(receipts, conclusions, true, false) {
		bodies = append(bodies, customerReceiptBody{
			FactID: receipt.FactID, OccurredAt: receipt.OccurredAt, Lines: receipt.Lines,
			DiscrepancyID: receipt.DiscrepancyID, Discrepancies: receipt.Discrepancies,
			AwaitingOwnerConfirmation: receipt.AwaitingOwnerConfirmation, Conclusion: receipt.Conclusion,
			SupersededConclusions: receipt.SupersededConclusions,
		})
	}
	return bodies
}

func customerPreAdviceNoticeBodyOf(notice domain.PreAdviceNotice) customerPreAdviceNoticeBody {
	body := customerPreAdviceNoticeBody{
		Version:        notice.Version(),
		ExpectedAt:     rfc3339(notice.ExpectedAt()),
		SubmissionPath: notice.Submission().Path().String(),
		RecordedAt:     rfc3339(notice.RecordedAt()),
	}
	if notice.Clue().Given() {
		body.ArrivalClue = notice.Clue().String()
	}
	if notice.Reference().Given() {
		body.OwnerReference = notice.Reference().String()
	}
	if evidence, given := notice.Submission().EvidenceReference(); given {
		body.EvidenceReference = evidence.String()
	}
	return body
}

func customerPreAdviceListQuery(request *http.Request) (cataloguepage.Query, error) {
	if reason := refuseCarriedQueryIdentity(request); reason != "" {
		return cataloguepage.Query{}, &malformedRequest{reason: reason}
	}
	values, err := cataloguepage.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return cataloguepage.Query{}, err
	}
	return ports.PreAdvicePages.Decode(values)
}

func customerPreAdviceIDQuery(request *http.Request) (domain.PreAdviceID, error) {
	if reason := refuseCarriedQueryIdentity(request); reason != "" {
		return domain.PreAdviceID{}, &malformedRequest{reason: reason}
	}
	cells, reason := queryCells(request, preAdviceIDQueryKey)
	if reason != "" {
		return domain.PreAdviceID{}, &malformedRequest{reason: reason}
	}
	id, err := domain.NewPreAdviceID(cells[0])
	if err != nil {
		return domain.PreAdviceID{}, &malformedRequest{reason: blankCellReason(preAdviceIDQueryKey)}
	}
	return id, nil
}

func customerDocumentNumberQuery(request *http.Request) (domain.DocumentNumber, error) {
	if reason := refuseCarriedQueryIdentity(request); reason != "" {
		return domain.DocumentNumber{}, &malformedRequest{reason: reason}
	}
	cells, reason := queryCells(request, documentNumberQueryKey)
	if reason != "" {
		return domain.DocumentNumber{}, &malformedRequest{reason: reason}
	}
	number, err := domain.NewDocumentNumber(cells[0])
	if err != nil {
		return domain.DocumentNumber{}, &malformedRequest{reason: blankCellReason(documentNumberQueryKey)}
	}
	return number, nil
}

func refuseCarriedQueryIdentity(request *http.Request) string {
	values, err := cataloguepage.ParseQuery(request.URL.RawQuery)
	if err != nil {
		var malformed *cataloguepage.MalformedQuery
		if errors.As(err, &malformed) {
			return malformed.Reason
		}
		return "the query string could not be read"
	}
	for _, key := range []string{"tenantId", ownerAccountQueryKey, "registrant"} {
		if _, present := values[key]; present {
			return key + " must not be carried in the query; the tenant and the owner come from the access channel"
		}
	}
	return ""
}

// NewCustomerNotEnabledEndpoint 是开关关着时货主口的固定处理器。403 与本包其余仓配口相同，是
// WAREHOUSE_FULFILLMENT_NOT_ENABLED；405 用首方问题详情，与开关开着时这一口的方法门同一份。
func NewCustomerNotEnabledEndpoint(allowedMethod string) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseCustomerMethod(response, request, allowedMethod) {
			return
		}
		writeCustomerProblem(response, request, http.StatusForbidden, CodeWarehouseFulfillmentNotEnabled, "")
	})
}

func refuseCustomerMethod(response http.ResponseWriter, request *http.Request, allowed string) bool {
	if request.Method == allowed {
		return false
	}
	response.Header().Set("Allow", allowed)
	writeCustomerProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed, "")
	return true
}

func writeCustomerProblem(response http.ResponseWriter, request *http.Request, status int, code, detail string) {
	httpapi.WriteProblemWithDetail(response, request, status, code, detail)
}

func answerCustomerIntake(response http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, ErrAccessChannelNotConfigured):
		writeCustomerProblem(response, request, http.StatusForbidden, codeAccessChannelNotConfigured, "")
	case errors.Is(err, ErrCustomerClientCredentialRejected):
		writeCustomerProblem(response, request, http.StatusUnauthorized, codeCustomerClientCredentialRejected, "")
	case errors.Is(err, ErrCustomerClientNotGranted):
		writeCustomerProblem(response, request, http.StatusForbidden, codeCustomerClientNotGranted, "")
	case errors.Is(err, ErrOutsideAdmissionScope):
		writeCustomerProblem(response, request, http.StatusForbidden, codeOutsideAdmissionScope, "")
	case errors.Is(err, ErrIdentityDependencyUnavailable):
		writeCustomerProblem(response, request, http.StatusServiceUnavailable, codeIdentityDependencyUnavailable, "")
	case errors.Is(err, ErrMalformedRequest):
		var malformed *malformedRequest
		detail := ""
		if errors.As(err, &malformed) {
			detail = malformed.reason
		}
		writeCustomerProblem(response, request, http.StatusBadRequest, codeMalformedRequest, detail)
	default:
		writeCustomerProblem(response, request, http.StatusInternalServerError, codeIntakeFailed, "")
	}
}

func writeCustomerQueryProblem(response http.ResponseWriter, request *http.Request, err error) {
	var page *cataloguepage.MalformedQuery
	if errors.As(err, &page) {
		writeCustomerProblem(response, request, http.StatusBadRequest, codeMalformedRequest, page.Reason)
		return
	}
	answerCustomerIntake(response, request, err)
}

var _ CustomerInboundPreAdvicePorts = (*CustomerInboundPreAdviceIntake)(nil)
var _ CustomerInboundPreAdvicePorts = unconfiguredCustomerInboundPreAdvice{}
