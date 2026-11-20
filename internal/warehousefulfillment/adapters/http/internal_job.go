package warehousehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// InternalJobAuthenticator 铸库内作业事实的信封。事实种类由实现绑死，这里只铸造。
type InternalJobAuthenticator interface {
	AuthenticateInternalJob(ctx context.Context, bearerToken, deviceID, scopeKind, scopeReference string) (domain.TenantID, domain.ReceiptOperatorID, domain.ReceiptDeviceID, error)
}

// InternalJobWorkFactIntake 读作业事实请求头，先认证再读载荷。
type InternalJobWorkFactIntake struct {
	authenticator InternalJobAuthenticator
	kind          domain.InternalJobKind
}

func NewInternalJobWorkFactIntake(authenticator InternalJobAuthenticator, kind domain.InternalJobKind) (*InternalJobWorkFactIntake, error) {
	if authenticator == nil || kind.String() == "" {
		return nil, errors.New("warehouse fulfillment http: internal job intake needs an authenticator and a kind")
	}
	return &InternalJobWorkFactIntake{authenticator: authenticator, kind: kind}, nil
}

func (intake *InternalJobWorkFactIntake) Intake(ctx context.Context, request *http.Request) (application.RegisterInternalJobCommand, error) {
	none := application.RegisterInternalJobCommand{}
	device, scopeKind, reference, err := warehouseWorkScope(request)
	if err != nil {
		return none, err
	}
	tenant, operator, recordedDevice, err := intake.authenticator.AuthenticateInternalJob(ctx, httpapi.BearerToken(request), device, scopeKind, reference)
	if err != nil {
		return none, err
	}
	command, err := registrationjson.InternalJobCommand(requestBody(request), intake.kind)
	if err != nil {
		return none, asMalformed(err)
	}
	if reference != command.Warehouse.String() {
		return none, &malformedRequest{reason: httpapi.HeaderWorkScopeReference + " must equal warehouse"}
	}
	command.Tenant = tenant
	command.Operator = operator
	command.Device = recordedDevice
	return command, nil
}

// UnconfiguredInternalJobIntake 在作业事实渠道没配时答接入渠道未配置，不读载荷。
type UnconfiguredInternalJobIntake struct{}

func (UnconfiguredInternalJobIntake) Intake(context.Context, *http.Request) (application.RegisterInternalJobCommand, error) {
	return application.RegisterInternalJobCommand{}, ErrAccessChannelNotConfigured
}

// InternalJobRegistrar 是四个登记口转交的编排。
type InternalJobRegistrar interface {
	Register(ctx context.Context, command application.RegisterInternalJobCommand) (application.RegisterInternalJobResult, error)
}

// ConditionRestoreAuthenticator 认证次品转良品。
type ConditionRestoreAuthenticator interface {
	AuthenticateConditionRestore(ctx context.Context, bearerToken string) (OperatorIdentity, error)
}

// ConditionRestoreIntake 是次品转良品口的 Intake。先认证后读请求体。
type ConditionRestoreIntake interface {
	IntakeConditionRestore(ctx context.Context, request *http.Request) (application.RestoreConditionCommand, error)
}

// OperatorConditionRestoreIntake 用运营决定面认证次品转良品。
type OperatorConditionRestoreIntake struct {
	authenticator ConditionRestoreAuthenticator
}

func NewOperatorConditionRestoreIntake(authenticator ConditionRestoreAuthenticator) (*OperatorConditionRestoreIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: condition restore intake needs an authenticator")
	}
	return &OperatorConditionRestoreIntake{authenticator: authenticator}, nil
}

func (intake *OperatorConditionRestoreIntake) IntakeConditionRestore(ctx context.Context, request *http.Request) (application.RestoreConditionCommand, error) {
	identity, err := intake.authenticator.AuthenticateConditionRestore(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.RestoreConditionCommand{}, err
	}
	command, err := registrationjson.ConditionRestoreCommand(requestBody(request))
	if err != nil {
		return application.RestoreConditionCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	return command, nil
}

// UnconfiguredConditionRestoreIntake 在运营决定渠道没配时答接入渠道未配置。
type UnconfiguredConditionRestoreIntake struct{}

func (UnconfiguredConditionRestoreIntake) IntakeConditionRestore(context.Context, *http.Request) (application.RestoreConditionCommand, error) {
	return application.RestoreConditionCommand{}, ErrAccessChannelNotConfigured
}

// ConditionRestorer 是次品转良品转交的编排。
type ConditionRestorer interface {
	Restore(ctx context.Context, command application.RestoreConditionCommand) (application.RestoreConditionResult, error)
}

// InternalJobQueries 是运营查阅。
type InternalJobQueries interface {
	ListInternalJobs(ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[domain.RecordedInternalJob], error)
	GetInternalJob(ctx context.Context, tenant domain.TenantID, factID domain.InternalJobFactID) (domain.RecordedInternalJob, bool, error)
	GetConditionRestore(ctx context.Context, tenant domain.TenantID, id domain.ConditionRestoreID) (domain.ConditionRestore, bool, error)
}

func NewRegisterInternalJobEndpoint(intake interface {
	Intake(context.Context, *http.Request) (application.RegisterInternalJobCommand, error)
}, registrar InternalJobRegistrar) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.Intake(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := registrar.Register(request.Context(), command)
		if err != nil {
			slog.ErrorContext(request.Context(), "internal job did not form an answer", "factId", command.FactID.String())
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeInternalJobAnswer(response, request, result)
	})
}

func NewRestoreConditionEndpoint(intake ConditionRestoreIntake, restorer ConditionRestorer) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeConditionRestore(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := restorer.Restore(request.Context(), command)
		if err != nil {
			slog.ErrorContext(request.Context(), "condition restore did not form an answer", "decisionId", command.ID.String())
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeRestoreAnswer(response, request, result)
	})
}

func NewQueryInternalJobsEndpoint(intake CatalogueIntake, queries InternalJobQueries) http.Handler {
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
		pageQuery, err := ports.InternalJobPages.Decode(values)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		page, err := queries.ListInternalJobs(request.Context(), query.Tenant, query.Limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "internal jobs list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		jobs := make([]internalJobBody, 0, len(page.Rows))
		for _, job := range page.Rows {
			jobs = append(jobs, internalJobBodyOf(job))
		}
		writeJSON(response, http.StatusOK, internalJobListResponse{Outcome: "LISTED", Jobs: jobs, Page: page.Page(query.Limit)})
	})
}

func NewQueryInternalJobEndpoint(intake CatalogueIntake, queries InternalJobQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		cells, reason := queryCells(request, "factId")
		if reason != "" {
			writeMalformed(response, reason)
			return
		}
		factID, err := domain.NewInternalJobFactID(cells[0])
		if err != nil {
			writeMalformed(response, blankCellReason("factId"))
			return
		}
		job, found, err := queries.GetInternalJob(request.Context(), query.Tenant, factID)
		if err != nil {
			slog.ErrorContext(request.Context(), "internal job read did not form an answer", "factId", factID.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		answer := internalJobOneResponse{Outcome: "READ", FactID: factID.String(), Found: found}
		if found {
			body := internalJobBodyOf(job)
			answer.Job = &body
		}
		writeJSON(response, http.StatusOK, answer)
	})
}

func NewQueryConditionRestoreEndpoint(intake CatalogueIntake, queries InternalJobQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		cells, reason := queryCells(request, "decisionId")
		if reason != "" {
			writeMalformed(response, reason)
			return
		}
		id, err := domain.NewConditionRestoreID(cells[0])
		if err != nil {
			writeMalformed(response, blankCellReason("decisionId"))
			return
		}
		restore, found, err := queries.GetConditionRestore(request.Context(), query.Tenant, id)
		if err != nil {
			slog.ErrorContext(request.Context(), "condition restore read did not form an answer", "decisionId", id.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		answer := conditionRestoreOneResponse{Outcome: "READ", DecisionID: id.String(), Found: found}
		if found {
			body := conditionRestoreBodyOf(restore)
			answer.Restore = &body
		}
		writeJSON(response, http.StatusOK, answer)
	})
}

type internalJobAnswer struct {
	Outcome string                 `json:"outcome"`
	Issues  []internalJobIssueBody `json:"issues,omitempty"`
	Job     *internalJobBody       `json:"job,omitempty"`
}

type internalJobIssueBody struct {
	Line int    `json:"line"`
	Kind string `json:"kind"`
}

type internalJobBody struct {
	FactID         string                `json:"factId"`
	Kind           string                `json:"kind"`
	Warehouse      string                `json:"warehouse"`
	OccurredAt     string                `json:"occurredAt"`
	DeviceSequence int64                 `json:"deviceSequence"`
	ReceivedAt     string                `json:"receivedAt"`
	Lines          []internalJobLineBody `json:"lines"`
}

type internalJobLineBody struct {
	Owner         string   `json:"ownerAccount"`
	Goods         string   `json:"goodsCode,omitempty"`
	Unit          string   `json:"unit,omitempty"`
	State         string   `json:"stockState,omitempty"`
	Quantity      int64    `json:"quantity,omitempty"`
	Source        string   `json:"sourceLocation,omitempty"`
	Target        string   `json:"targetLocation,omitempty"`
	Carton        string   `json:"cartonNumber,omitempty"`
	Posted        int64    `json:"posted"`
	PostedInbound int64    `json:"postedInbound"`
	ShortOnBook   int64    `json:"shortOnBook"`
	Held          int64    `json:"held"`
	CartonMissing bool     `json:"cartonMissing,omitempty"`
	MovementID    string   `json:"movementId,omitempty"`
	DiscrepancyID string   `json:"discrepancyId,omitempty"`
	Batches       []string `json:"inboundBatches,omitempty"`
}

type internalJobListResponse struct {
	Outcome string             `json:"outcome"`
	Jobs    []internalJobBody  `json:"jobs"`
	Page    cataloguepage.Page `json:"page"`
}

type internalJobOneResponse struct {
	Outcome string           `json:"outcome"`
	FactID  string           `json:"factId"`
	Found   bool             `json:"found"`
	Job     *internalJobBody `json:"job,omitempty"`
}

type restoreAnswer struct {
	Outcome string                 `json:"outcome"`
	Issues  []internalJobIssueBody `json:"issues,omitempty"`
	Restore *conditionRestoreBody  `json:"restore,omitempty"`
}

type conditionRestoreBody struct {
	DecisionID string `json:"decisionId"`
	Warehouse  string `json:"warehouse"`
	Owner      string `json:"ownerAccount"`
	Location   string `json:"locationCode"`
	Goods      string `json:"goodsCode"`
	Unit       string `json:"unit"`
	Quantity   int64  `json:"quantity"`
	Reason     string `json:"reason"`
	MovementID string `json:"movementId"`
	OccurredAt string `json:"occurredAt"`
}

type conditionRestoreOneResponse struct {
	Outcome    string                `json:"outcome"`
	DecisionID string                `json:"decisionId"`
	Found      bool                  `json:"found"`
	Restore    *conditionRestoreBody `json:"restore,omitempty"`
}

func writeInternalJobAnswer(response http.ResponseWriter, request *http.Request, result application.RegisterInternalJobResult) {
	name := result.Outcome().String()
	if name == "" {
		httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := internalJobAnswer{Outcome: name}
	status := http.StatusOK
	switch result.Outcome() {
	case application.RegisterInternalJobRegistered, application.RegisterInternalJobShortageRegistered:
		status = http.StatusCreated
		fallthrough
	case application.RegisterInternalJobExisting:
		job, has := result.Job()
		if !has {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		body := internalJobBodyOf(job)
		answer.Job = &body
	case application.RegisterInternalJobContentInvalid:
		for _, issue := range result.Issues() {
			answer.Issues = append(answer.Issues, internalJobIssueBody{Line: issue.Line(), Kind: issue.Kind().String()})
		}
	}
	writeJSON(response, status, answer)
}

func writeRestoreAnswer(response http.ResponseWriter, request *http.Request, result application.RestoreConditionResult) {
	name := result.Outcome().String()
	if name == "" {
		httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := restoreAnswer{Outcome: name}
	status := http.StatusOK
	switch result.Outcome() {
	case application.RestoreConditionRestored:
		status = http.StatusCreated
		fallthrough
	case application.RestoreConditionExisting:
		restore, has := result.Restore()
		if !has {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		body := conditionRestoreBodyOf(restore)
		answer.Restore = &body
	case application.RestoreConditionNotValid:
		for _, issue := range result.Issues() {
			answer.Issues = append(answer.Issues, internalJobIssueBody{Line: issue.Line(), Kind: issue.Kind().String()})
		}
	}
	writeJSON(response, status, answer)
}

func internalJobBodyOf(job domain.RecordedInternalJob) internalJobBody {
	body := internalJobBody{
		FactID: job.FactID().String(), Kind: job.Kind().String(), Warehouse: job.Warehouse().String(),
		OccurredAt: job.OccurredAt().UTC().Format("2006-01-02T15:04:05.000000Z"), DeviceSequence: job.DeviceSequence(),
		ReceivedAt: job.ReceivedAt().UTC().Format("2006-01-02T15:04:05.000000Z"),
	}
	for _, line := range job.Lines() {
		input := line.Input()
		result := line.Result()
		item := internalJobLineBody{
			Owner: input.Owner().String(), Goods: input.Goods().String(), Unit: input.Unit().String(), State: input.State().String(),
			Quantity: input.Quantity(), Source: input.Source().String(), Target: input.Target().String(), Carton: input.Carton().String(),
			Posted: result.Posted, PostedInbound: result.PostedInbound, ShortOnBook: result.ShortOnBook, Held: result.Held,
			CartonMissing: result.CartonMissing, MovementID: result.Movement, DiscrepancyID: result.Discrepancy.String(),
		}
		for _, batch := range result.Batches {
			item.Batches = append(item.Batches, batch.String())
		}
		body.Lines = append(body.Lines, item)
	}
	return body
}

func conditionRestoreBodyOf(restore domain.ConditionRestore) conditionRestoreBody {
	return conditionRestoreBody{
		DecisionID: restore.ID().String(), Warehouse: restore.Warehouse().String(), Owner: restore.Owner().String(),
		Location: restore.Location().String(), Goods: restore.Goods().String(), Unit: restore.Unit().String(),
		Quantity: restore.Quantity(), Reason: restore.Reason(), MovementID: restore.Movement().String(),
		OccurredAt: restore.OccurredAt().UTC().Format("2006-01-02T15:04:05.000000Z"),
	}
}
