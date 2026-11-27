package warehousehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 本文件是待认领到货的作业事实登记口、运营决定三口（认领、撤销、处置）与运营查阅两口（UC-WF-005）。
// 待认领到货不上客户面（BD-WF-012）。日志绝不记录观察正文、撤销原因或处置说明。

// UnclaimedArrivalWorkFactAuthenticator 铸待认领到货登记的作业事实信封（能力词 WORK_FACT_REGISTRATION，事实类型 UNCLAIMED_ARRIVAL）。
type UnclaimedArrivalWorkFactAuthenticator interface {
	AuthenticateUnclaimedArrival(
		ctx context.Context,
		bearerToken, deviceID, scopeKind, scopeReference string,
	) (domain.TenantID, domain.ReceiptOperatorID, domain.ReceiptDeviceID, error)
}

// UnclaimedArrivalWorkFactIntakeFace 是待认领到货登记口的 Intake 接口。
type UnclaimedArrivalWorkFactIntakeFace interface {
	IntakeUnclaimedArrival(ctx context.Context, request *http.Request) (application.RegisterUnclaimedArrivalCommand, error)
}

// UnclaimedArrivalRegistrar 是直接登记待认领到货的编排。
type UnclaimedArrivalRegistrar interface {
	Register(ctx context.Context, command application.RegisterUnclaimedArrivalCommand) (application.RegisterUnclaimedArrivalResult, error)
}

// UnclaimedArrivalClaimAuthenticator 认证待认领到货认领与撤销（决定种类 UNCLAIMED_ARRIVAL_CLAIM）。
type UnclaimedArrivalClaimAuthenticator interface {
	AuthenticateUnclaimedArrivalClaim(ctx context.Context, bearerToken string) (OperatorIdentity, error)
}

// UnclaimedArrivalDispositionAuthenticator 认证待认领到货处置（决定种类 UNCLAIMED_ARRIVAL_DISPOSITION）。
type UnclaimedArrivalDispositionAuthenticator interface {
	AuthenticateUnclaimedArrivalDisposition(ctx context.Context, bearerToken string) (OperatorIdentity, error)
}

// UnclaimedArrivalClaimIntake 是认领与撤销两口共用的运营决定 Intake。
type UnclaimedArrivalClaimIntake interface {
	IntakeClaimUnclaimedArrival(ctx context.Context, request *http.Request) (application.ClaimUnclaimedArrivalCommand, error)
	IntakeRevokeUnclaimedClaim(ctx context.Context, request *http.Request) (application.RevokeUnclaimedClaimCommand, error)
}

// UnclaimedArrivalDispositionIntake 是处置口的运营决定 Intake。
type UnclaimedArrivalDispositionIntake interface {
	IntakeDisposeUnclaimedArrival(ctx context.Context, request *http.Request) (application.DisposeUnclaimedArrivalCommand, error)
}

// UnclaimedArrivalManager 是认领、撤销与处置三口共用的编排。
type UnclaimedArrivalManager interface {
	Claim(ctx context.Context, command application.ClaimUnclaimedArrivalCommand) (application.ClaimUnclaimedArrivalResult, error)
	RevokeClaim(ctx context.Context, command application.RevokeUnclaimedClaimCommand) (application.RevokeUnclaimedClaimResult, error)
	Dispose(ctx context.Context, command application.DisposeUnclaimedArrivalCommand) (application.DisposeUnclaimedArrivalResult, error)
}

// UnclaimedArrivalQueries 是待认领到货运营查阅的编排。
type UnclaimedArrivalQueries interface {
	ListUnclaimedArrivals(ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query, receivedFrom, receivedTo time.Time) (cataloguepage.Result[domain.UnclaimedArrivalDetail], error)
	GetUnclaimedArrival(ctx context.Context, tenant domain.TenantID, id domain.UnclaimedArrivalID) (domain.UnclaimedArrivalDetail, bool, error)
}

var (
	_ UnclaimedArrivalRegistrar = (*application.RegisterUnclaimedArrivalHandler)(nil)
	_ UnclaimedArrivalManager   = (*application.ManageUnclaimedArrivalHandler)(nil)
	_ UnclaimedArrivalQueries   = (*application.QueryUnclaimedArrivalsHandler)(nil)
)

// UnclaimedArrivalWorkFactIntake 读作业事实请求头、先认证/核授予/查设备/判准入，再解码载荷并核范围引用等于仓。
type UnclaimedArrivalWorkFactIntake struct {
	authenticator UnclaimedArrivalWorkFactAuthenticator
}

func NewUnclaimedArrivalWorkFactIntake(authenticator UnclaimedArrivalWorkFactAuthenticator) (*UnclaimedArrivalWorkFactIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: unclaimed arrival intake needs an authenticator")
	}
	return &UnclaimedArrivalWorkFactIntake{authenticator: authenticator}, nil
}

var _ UnclaimedArrivalWorkFactIntakeFace = (*UnclaimedArrivalWorkFactIntake)(nil)

func (intake *UnclaimedArrivalWorkFactIntake) IntakeUnclaimedArrival(
	ctx context.Context,
	request *http.Request,
) (application.RegisterUnclaimedArrivalCommand, error) {
	var none application.RegisterUnclaimedArrivalCommand
	device, kind, reference, err := warehouseWorkScope(request)
	if err != nil {
		return none, err
	}
	// 先认证、核授予、查设备与判准入，再读请求体：未配置渠道或未授权方在读载荷前即答其格。
	tenant, operator, recordedDevice, err := intake.authenticator.AuthenticateUnclaimedArrival(
		ctx, httpapi.BearerToken(request), device, kind, reference,
	)
	if err != nil {
		return none, err
	}
	body := request.Body
	if body == nil {
		body = http.NoBody
	}
	command, err := registrationjson.UnclaimedArrivalCommand(body)
	if err != nil {
		var malformed *registrationjson.MalformedDocumentError
		if errors.As(err, &malformed) {
			return none, &malformedRequest{reason: malformed.Reason}
		}
		return none, err
	}
	if reference != command.Warehouse.String() {
		return none, &malformedRequest{reason: httpapi.HeaderWorkScopeReference + " must equal warehouse"}
	}
	command.Tenant = tenant
	command.Operator = operator
	command.Device = recordedDevice
	return command, nil
}

// UnconfiguredUnclaimedArrivalWorkFactIntake 在作业事实渠道未配时答接入渠道未配置，不读载荷。
type UnconfiguredUnclaimedArrivalWorkFactIntake struct{}

func (UnconfiguredUnclaimedArrivalWorkFactIntake) IntakeUnclaimedArrival(context.Context, *http.Request) (application.RegisterUnclaimedArrivalCommand, error) {
	return application.RegisterUnclaimedArrivalCommand{}, ErrAccessChannelNotConfigured
}

var _ UnclaimedArrivalWorkFactIntakeFace = UnconfiguredUnclaimedArrivalWorkFactIntake{}

// OperatorUnclaimedClaimIntake 是认领与撤销两口的运营决定 Intake。先认证后读请求体。
type OperatorUnclaimedClaimIntake struct {
	authenticator UnclaimedArrivalClaimAuthenticator
}

func NewOperatorUnclaimedClaimIntake(authenticator UnclaimedArrivalClaimAuthenticator) (*OperatorUnclaimedClaimIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: unclaimed arrival claim intake needs an authenticator")
	}
	return &OperatorUnclaimedClaimIntake{authenticator: authenticator}, nil
}

var _ UnclaimedArrivalClaimIntake = (*OperatorUnclaimedClaimIntake)(nil)

func (intake *OperatorUnclaimedClaimIntake) IntakeClaimUnclaimedArrival(
	ctx context.Context,
	request *http.Request,
) (application.ClaimUnclaimedArrivalCommand, error) {
	identity, err := intake.authenticator.AuthenticateUnclaimedArrivalClaim(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.ClaimUnclaimedArrivalCommand{}, err
	}
	command, err := registrationjson.ClaimUnclaimedArrivalCommand(requestBody(request), identity.Tenant, identity.Registrant)
	if err != nil {
		return application.ClaimUnclaimedArrivalCommand{}, asMalformed(err)
	}
	return command, nil
}

func (intake *OperatorUnclaimedClaimIntake) IntakeRevokeUnclaimedClaim(
	ctx context.Context,
	request *http.Request,
) (application.RevokeUnclaimedClaimCommand, error) {
	identity, err := intake.authenticator.AuthenticateUnclaimedArrivalClaim(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.RevokeUnclaimedClaimCommand{}, err
	}
	command, err := registrationjson.RevokeUnclaimedClaimCommand(requestBody(request), identity.Tenant, identity.Registrant)
	if err != nil {
		return application.RevokeUnclaimedClaimCommand{}, asMalformed(err)
	}
	return command, nil
}

// OperatorUnclaimedDispositionIntake 是处置口的运营决定 Intake。先认证后读请求体。
type OperatorUnclaimedDispositionIntake struct {
	authenticator UnclaimedArrivalDispositionAuthenticator
}

func NewOperatorUnclaimedDispositionIntake(authenticator UnclaimedArrivalDispositionAuthenticator) (*OperatorUnclaimedDispositionIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: unclaimed arrival disposition intake needs an authenticator")
	}
	return &OperatorUnclaimedDispositionIntake{authenticator: authenticator}, nil
}

var _ UnclaimedArrivalDispositionIntake = (*OperatorUnclaimedDispositionIntake)(nil)

func (intake *OperatorUnclaimedDispositionIntake) IntakeDisposeUnclaimedArrival(
	ctx context.Context,
	request *http.Request,
) (application.DisposeUnclaimedArrivalCommand, error) {
	identity, err := intake.authenticator.AuthenticateUnclaimedArrivalDisposition(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.DisposeUnclaimedArrivalCommand{}, err
	}
	command, err := registrationjson.DisposeUnclaimedArrivalCommand(requestBody(request), identity.Tenant, identity.Registrant)
	if err != nil {
		return application.DisposeUnclaimedArrivalCommand{}, asMalformed(err)
	}
	return command, nil
}

// --- 响应体 ---

type unclaimedArrivalBody struct {
	ArrivalID         string                    `json:"arrivalId"`
	FactID            string                    `json:"factId"`
	Origin            string                    `json:"origin"`
	Kind              string                    `json:"kind"`
	Status            string                    `json:"status"`
	WarehouseCode     string                    `json:"warehouseCode"`
	Device            string                    `json:"device"`
	Operator          string                    `json:"operator"`
	OccurredAt        string                    `json:"occurredAt"`
	DeviceSequence    int64                     `json:"deviceSequence,omitempty"`
	ReceivedAt        string                    `json:"receivedAt"`
	Observation       string                    `json:"observation,omitempty"`
	OwnerClue         string                    `json:"ownerClue,omitempty"`
	PreAdviceClue     string                    `json:"preAdviceClue,omitempty"`
	StorageLocation   string                    `json:"storageLocation,omitempty"`
	DivertedReceipt   *receiptBody              `json:"divertedReceipt,omitempty"`
	Claims            []unclaimedClaimEntryBody `json:"claims,omitempty"`
	PostingReferences []string                  `json:"postingReferences,omitempty"`
	Disposition       *unclaimedDispositionBody `json:"disposition,omitempty"`
}

type unclaimedClaimBody struct {
	DecisionID   string `json:"decisionId"`
	ArrivalID    string `json:"arrivalId"`
	OwnerAccount string `json:"ownerAccount"`
	PreAdviceID  string `json:"preAdviceId,omitempty"`
	Operator     string `json:"operator"`
	ClaimedAt    string `json:"claimedAt"`
}

type unclaimedClaimRevocationBody struct {
	DecisionID      string `json:"decisionId"`
	ArrivalID       string `json:"arrivalId"`
	ClaimDecisionID string `json:"claimDecisionId"`
	Reason          string `json:"reason,omitempty"`
	Operator        string `json:"operator"`
	RevokedAt       string `json:"revokedAt"`
}

type unclaimedClaimEntryBody struct {
	Claim      unclaimedClaimBody            `json:"claim"`
	Revocation *unclaimedClaimRevocationBody `json:"revocation,omitempty"`
}

type unclaimedDispositionBody struct {
	ArrivalID         string `json:"arrivalId"`
	Method            string `json:"method"`
	RuleWarehouseCode string `json:"ruleWarehouseCode"`
	RuleRevision      int    `json:"ruleRevision"`
	Note              string `json:"note,omitempty"`
	Operator          string `json:"operator"`
	DisposedAt        string `json:"disposedAt"`
}

type registerUnclaimedArrivalAnswer struct {
	Outcome string                `json:"outcome"`
	Reason  string                `json:"reason,omitempty"`
	Arrival *unclaimedArrivalBody `json:"arrival,omitempty"`
}

type claimUnclaimedArrivalAnswer struct {
	Outcome string              `json:"outcome"`
	Field   string              `json:"field,omitempty"`
	Status  string              `json:"status,omitempty"`
	Missing string              `json:"missing,omitempty"`
	Claim   *unclaimedClaimBody `json:"claim,omitempty"`
}

type revokeUnclaimedClaimAnswer struct {
	Outcome    string                        `json:"outcome"`
	Status     string                        `json:"status,omitempty"`
	Missing    string                        `json:"missing,omitempty"`
	Revocation *unclaimedClaimRevocationBody `json:"revocation,omitempty"`
}

type disposeUnclaimedArrivalAnswer struct {
	Outcome     string                    `json:"outcome"`
	Reason      string                    `json:"reason,omitempty"`
	Status      string                    `json:"status,omitempty"`
	Missing     string                    `json:"missing,omitempty"`
	Disposition *unclaimedDispositionBody `json:"disposition,omitempty"`
}

const (
	outcomeUnclaimedArrivalsListed = "UNCLAIMED_ARRIVALS_LISTED"
	outcomeUnclaimedArrivalRead    = "UNCLAIMED_ARRIVAL_READ"
	arrivalIDQueryKey              = "arrivalId"
	receivedFromQueryKey           = "receivedFrom"
	receivedToQueryKey             = "receivedTo"
)

type unclaimedArrivalListResponse struct {
	Outcome  string                 `json:"outcome"`
	Arrivals []unclaimedArrivalBody `json:"arrivals"`
	Page     cataloguepage.Page     `json:"page"`
	Facets   cataloguepage.Facets   `json:"facets,omitempty"`
}

type unclaimedArrivalOneResponse struct {
	Outcome   string                `json:"outcome"`
	ArrivalID string                `json:"arrivalId"`
	Found     bool                  `json:"found"`
	Arrival   *unclaimedArrivalBody `json:"arrival,omitempty"`
}

// --- 端点 ---

// NewRegisterUnclaimedArrivalEndpoint 交回仓内作业登记待认领到货的入口（POST /warehouse-fulfillment-unclaimed-arrival-registrations）。
// 新建答 201，其余业务答复答 200；问题体用作业事实族问题详情。日志不带观察正文。
func NewRegisterUnclaimedArrivalEndpoint(intake UnclaimedArrivalWorkFactIntakeFace, registrar UnclaimedArrivalRegistrar) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeUnclaimedArrival(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := registrar.Register(request.Context(), command)
		if err != nil {
			slog.ErrorContext(request.Context(), "unclaimed arrival registration did not form an answer",
				"warehouse", command.Warehouse.String(), "factId", command.FactID.String())
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		name := result.Outcome().String()
		if name == "" {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		answer := registerUnclaimedArrivalAnswer{Outcome: name}
		status := http.StatusOK
		switch result.Outcome() {
		case application.RegisterUnclaimedRegistered, application.RegisterUnclaimedExistingResult:
			arrival, has := result.Arrival()
			if !has {
				httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
				return
			}
			body := unclaimedArrivalBodyOf(arrival)
			answer.Arrival = &body
			if result.Outcome() == application.RegisterUnclaimedRegistered {
				status = http.StatusCreated
			}
		case application.RegisterUnclaimedWarehouseNotAccepting:
			reason, has := result.NotAcceptingReason()
			if !has || reason.String() == "" {
				httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
				return
			}
			answer.Reason = reason.String()
		}
		writeJSON(response, status, answer)
	})
}

// NewClaimUnclaimedArrivalEndpoint 交回运营认领入口（POST /warehouse-fulfillment-unclaimed-arrival-claims）。
// 货主客户账户在本租户查不到答 404 OWNER_ACCOUNT_NOT_VISIBLE，不带 outcome。
func NewClaimUnclaimedArrivalEndpoint(intake UnclaimedArrivalClaimIntake, manager UnclaimedArrivalManager) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodPost) {
			return
		}
		command, err := intake.IntakeClaimUnclaimedArrival(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := manager.Claim(request.Context(), command)
		if errors.Is(err, application.ErrOwnerAccountNotInTenant) {
			writeProblem(response, http.StatusNotFound, codeOwnerAccountNotVisible)
			return
		}
		if err != nil {
			slog.ErrorContext(request.Context(), "unclaimed arrival claim did not form an answer",
				"arrivalId", command.ArrivalID.String(), "ownerAccount", command.Owner.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		name := result.Outcome().String()
		if name == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		answer := claimUnclaimedArrivalAnswer{Outcome: name}
		status := http.StatusOK
		switch result.Outcome() {
		case application.ClaimUnclaimedClaimed, application.ClaimUnclaimedExistingResult, application.ClaimUnclaimedAlreadyClaimed:
			claim, has := result.Claim()
			if !has {
				writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
				return
			}
			body := unclaimedClaimBodyOf(claim)
			answer.Claim = &body
			if result.Outcome() == application.ClaimUnclaimedClaimed {
				status = http.StatusCreated
			}
		case application.ClaimUnclaimedClaimInvalid:
			answer.Field = result.Field()
		case application.ClaimUnclaimedStatusMismatch:
			echoed, has := result.Status()
			if !has {
				writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
				return
			}
			answer.Status = echoed.String()
		case application.ClaimUnclaimedNotConfigured:
			answer.Missing = result.Missing()
		}
		writeJSON(response, status, answer)
	})
}

// NewRevokeUnclaimedClaimEndpoint 交回运营撤销认领入口（POST /warehouse-fulfillment-unclaimed-arrival-claim-revocations）。
func NewRevokeUnclaimedClaimEndpoint(intake UnclaimedArrivalClaimIntake, manager UnclaimedArrivalManager) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodPost) {
			return
		}
		command, err := intake.IntakeRevokeUnclaimedClaim(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := manager.RevokeClaim(request.Context(), command)
		if err != nil {
			slog.ErrorContext(request.Context(), "unclaimed arrival claim revocation did not form an answer",
				"arrivalId", command.ArrivalID.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		name := result.Outcome().String()
		if name == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		answer := revokeUnclaimedClaimAnswer{Outcome: name}
		status := http.StatusOK
		switch result.Outcome() {
		case application.RevokeClaimRevoked, application.RevokeClaimExistingResult:
			rev, has := result.Revocation()
			if !has {
				writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
				return
			}
			body := unclaimedClaimRevocationBodyOf(rev)
			answer.Revocation = &body
			if result.Outcome() == application.RevokeClaimRevoked {
				status = http.StatusCreated
			}
		case application.RevokeClaimStatusMismatch:
			echoed, has := result.Status()
			if !has {
				writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
				return
			}
			answer.Status = echoed.String()
		case application.RevokeClaimNotConfigured:
			answer.Missing = result.Missing()
		}
		writeJSON(response, status, answer)
	})
}

// NewDisposeUnclaimedArrivalEndpoint 交回运营处置入口（POST /warehouse-fulfillment-unclaimed-arrival-dispositions）。
func NewDisposeUnclaimedArrivalEndpoint(intake UnclaimedArrivalDispositionIntake, manager UnclaimedArrivalManager) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodPost) {
			return
		}
		command, err := intake.IntakeDisposeUnclaimedArrival(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := manager.Dispose(request.Context(), command)
		if err != nil {
			slog.ErrorContext(request.Context(), "unclaimed arrival disposition did not form an answer",
				"arrivalId", command.ArrivalID.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		name := result.Outcome().String()
		if name == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		answer := disposeUnclaimedArrivalAnswer{Outcome: name}
		status := http.StatusOK
		switch result.Outcome() {
		case application.DisposeUnclaimedDisposed, application.DisposeUnclaimedExistingResult:
			disp, has := result.Disposition()
			if !has {
				writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
				return
			}
			body := unclaimedDispositionBodyOf(disp)
			answer.Disposition = &body
			if result.Outcome() == application.DisposeUnclaimedDisposed {
				status = http.StatusCreated
			}
		case application.DisposeUnclaimedNotAllowed:
			reason, has := result.Reason()
			if !has || reason.String() == "" {
				writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
				return
			}
			answer.Reason = reason.String()
		case application.DisposeUnclaimedStatusMismatch:
			echoed, has := result.Status()
			if !has {
				writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
				return
			}
			answer.Status = echoed.String()
			if disp, hasDisp := result.Disposition(); hasDisp {
				body := unclaimedDispositionBodyOf(disp)
				answer.Disposition = &body
			}
		case application.DisposeUnclaimedNotConfigured:
			answer.Missing = result.Missing()
		}
		writeJSON(response, status, answer)
	})
}

// NewQueryUnclaimedArrivalsEndpoint 交回运营分页查阅待认领到货的入口（GET /warehouse-fulfillment-unclaimed-arrivals）。
func NewQueryUnclaimedArrivalsEndpoint(intake CatalogueIntake, queries UnclaimedArrivalQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		values, err := cataloguepage.ParseQuery(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		receivedFrom, receivedTo, reason := parseReceivedRange(values)
		if reason != "" {
			writeMalformed(response, reason)
			return
		}
		pageQuery, err := ports.UnclaimedArrivalPages.Decode(values)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		if pageQuery.PageSize > 0 {
			query.Limit = pageQuery.PageSize
		}
		page, err := queries.ListUnclaimedArrivals(request.Context(), query.Tenant, query.Limit, pageQuery, receivedFrom, receivedTo)
		if err != nil {
			slog.ErrorContext(request.Context(), "unclaimed arrivals list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		arrivals := make([]unclaimedArrivalBody, 0, len(page.Rows))
		for _, detail := range page.Rows {
			arrivals = append(arrivals, unclaimedArrivalDetailBodyOf(detail))
		}
		writeJSON(response, http.StatusOK, unclaimedArrivalListResponse{
			Outcome:  outcomeUnclaimedArrivalsListed,
			Arrivals: arrivals,
			Page:     page.Page(query.Limit),
			Facets:   page.Facets,
		})
	})
}

// NewQueryUnclaimedArrivalEndpoint 交回运营查阅单件待认领到货详情的入口（GET /warehouse-fulfillment-unclaimed-arrival?arrivalId=…）。
func NewQueryUnclaimedArrivalEndpoint(intake CatalogueIntake, queries UnclaimedArrivalQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		cells, reason := queryCells(request, arrivalIDQueryKey)
		if reason != "" {
			writeMalformed(response, reason)
			return
		}
		arrivalID, err := domain.NewUnclaimedArrivalID(cells[0])
		if err != nil {
			writeMalformed(response, blankCellReason(arrivalIDQueryKey))
			return
		}
		detail, found, err := queries.GetUnclaimedArrival(request.Context(), query.Tenant, arrivalID)
		if err != nil {
			slog.ErrorContext(request.Context(), "unclaimed arrival read did not form an answer",
				"arrivalId", arrivalID.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		answer := unclaimedArrivalOneResponse{
			Outcome: outcomeUnclaimedArrivalRead, ArrivalID: arrivalID.String(), Found: found,
		}
		if found {
			body := unclaimedArrivalDetailBodyOf(detail)
			answer.Arrival = &body
		}
		writeJSON(response, http.StatusOK, answer)
	})
}

func parseReceivedRange(values map[string][]string) (time.Time, time.Time, string) {
	var from, to time.Time
	if raw := values[receivedFromQueryKey]; len(raw) > 0 {
		if len(raw) != 1 {
			return time.Time{}, time.Time{}, receivedFromQueryKey + " must be given at most once"
		}
		parsed, err := time.Parse(time.RFC3339Nano, raw[0])
		if err != nil {
			return time.Time{}, time.Time{}, receivedFromQueryKey + " must be an RFC3339 timestamp"
		}
		from = parsed
	}
	if raw := values[receivedToQueryKey]; len(raw) > 0 {
		if len(raw) != 1 {
			return time.Time{}, time.Time{}, receivedToQueryKey + " must be given at most once"
		}
		parsed, err := time.Parse(time.RFC3339Nano, raw[0])
		if err != nil {
			return time.Time{}, time.Time{}, receivedToQueryKey + " must be an RFC3339 timestamp"
		}
		to = parsed
	}
	if !from.IsZero() && !to.IsZero() && to.Before(from) {
		return time.Time{}, time.Time{}, receivedToQueryKey + " must not be earlier than " + receivedFromQueryKey
	}
	return from, to, ""
}

func unclaimedArrivalBodyOf(arrival domain.RecordedUnclaimedArrival) unclaimedArrivalBody {
	body := unclaimedArrivalBody{
		ArrivalID:      arrival.ID().String(),
		FactID:         arrival.FactID().String(),
		Origin:         arrival.Origin().String(),
		Kind:           arrival.Kind().String(),
		Status:         arrival.Status().String(),
		WarehouseCode:  arrival.Warehouse().String(),
		Device:         arrival.Device().String(),
		Operator:       arrival.Operator().String(),
		OccurredAt:     rfc3339(arrival.OccurredAt()),
		DeviceSequence: arrival.DeviceSequence(),
		ReceivedAt:     rfc3339(arrival.ReceivedAt()),
	}
	if obs, has := arrival.Observation(); has {
		body.Observation = obs.String()
	}
	if owner, has := arrival.OwnerClue(); has {
		body.OwnerClue = owner.String()
	}
	if preAdvice, has := arrival.PreAdviceClue(); has {
		body.PreAdviceClue = preAdvice.String()
	}
	if storage, has := arrival.StorageLocation(); has {
		body.StorageLocation = storage.String()
	}
	if diverted, has := arrival.DivertedReceipt(); has {
		rb := receiptBodies([]domain.RecordedReceipt{diverted}, nil, false, true)[0]
		body.DivertedReceipt = &rb
	}
	return body
}

func unclaimedArrivalDetailBodyOf(detail domain.UnclaimedArrivalDetail) unclaimedArrivalBody {
	body := unclaimedArrivalBodyOf(detail.Arrival)
	if len(detail.Claims) > 0 {
		body.Claims = make([]unclaimedClaimEntryBody, 0, len(detail.Claims))
		for _, entry := range detail.Claims {
			item := unclaimedClaimEntryBody{Claim: unclaimedClaimBodyOf(entry.Claim)}
			if entry.Revocation != nil {
				rev := unclaimedClaimRevocationBodyOf(*entry.Revocation)
				item.Revocation = &rev
			}
			body.Claims = append(body.Claims, item)
		}
	}
	if len(detail.PostingReferences) > 0 {
		body.PostingReferences = make([]string, 0, len(detail.PostingReferences))
		for _, ref := range detail.PostingReferences {
			body.PostingReferences = append(body.PostingReferences, ref.String())
		}
	}
	if detail.Disposition != nil {
		disp := unclaimedDispositionBodyOf(*detail.Disposition)
		body.Disposition = &disp
	}
	return body
}

func unclaimedClaimBodyOf(claim domain.UnclaimedArrivalClaim) unclaimedClaimBody {
	body := unclaimedClaimBody{
		DecisionID:   claim.DecisionID().String(),
		ArrivalID:    claim.ArrivalID().String(),
		OwnerAccount: claim.Owner().String(),
		Operator:     claim.Operator().String(),
		ClaimedAt:    rfc3339(claim.ClaimedAt()),
	}
	if advice, has := claim.PreAdvice(); has {
		body.PreAdviceID = advice.String()
	}
	return body
}

func unclaimedClaimRevocationBodyOf(rev domain.UnclaimedArrivalClaimRevocation) unclaimedClaimRevocationBody {
	body := unclaimedClaimRevocationBody{
		DecisionID:      rev.DecisionID().String(),
		ArrivalID:       rev.ArrivalID().String(),
		ClaimDecisionID: rev.ClaimDecisionID().String(),
		Operator:        rev.Operator().String(),
		RevokedAt:       rfc3339(rev.RevokedAt()),
	}
	if reason, has := rev.Reason(); has {
		body.Reason = reason.String()
	}
	return body
}

func unclaimedDispositionBodyOf(disp domain.UnclaimedArrivalDisposition) unclaimedDispositionBody {
	body := unclaimedDispositionBody{
		ArrivalID:         disp.ArrivalID().String(),
		Method:            disp.Method().String(),
		RuleWarehouseCode: disp.RuleWarehouse().String(),
		RuleRevision:      disp.RuleRevision(),
		Operator:          disp.Operator().String(),
		DisposedAt:        rfc3339(disp.DisposedAt()),
	}
	if note, has := disp.Note(); has {
		body.Note = note.String()
	}
	return body
}
