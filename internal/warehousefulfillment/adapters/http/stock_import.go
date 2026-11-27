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

// StockImportSubmissionAuthenticator 认证建账导入提交。先认证，后读请求体。
type StockImportSubmissionAuthenticator interface {
	AuthenticateStockImportSubmission(ctx context.Context, bearerToken string) (OperatorIdentity, error)
}

type StockImportSubmissionIntake interface {
	IntakeStockImport(ctx context.Context, request *http.Request) (application.SubmitStockImportCommand, error)
}

type OperatorStockImportIntake struct {
	authenticator StockImportSubmissionAuthenticator
}

func NewOperatorStockImportIntake(authenticator StockImportSubmissionAuthenticator) (*OperatorStockImportIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: stock import intake needs an authenticator")
	}
	return &OperatorStockImportIntake{authenticator: authenticator}, nil
}

func (intake *OperatorStockImportIntake) IntakeStockImport(ctx context.Context, request *http.Request) (application.SubmitStockImportCommand, error) {
	identity, err := intake.authenticator.AuthenticateStockImportSubmission(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.SubmitStockImportCommand{}, err
	}
	command, err := registrationjson.StockImportCommand(requestBody(request), identity.Tenant, identity.Registrant)
	if err != nil {
		return application.SubmitStockImportCommand{}, asMalformed(err)
	}
	return command, nil
}

// UnconfiguredStockImportIntake 在仓配没开时答接入渠道未配置，不读载荷。
type UnconfiguredStockImportIntake struct{}

func (UnconfiguredStockImportIntake) IntakeStockImport(context.Context, *http.Request) (application.SubmitStockImportCommand, error) {
	return application.SubmitStockImportCommand{}, ErrAccessChannelNotConfigured
}

type StockImportSubmissions interface {
	Submit(ctx context.Context, command application.SubmitStockImportCommand) (application.StockImportResult, error)
}

type StockImportDecisionIntake interface {
	IntakeApproveStockImport(ctx context.Context, request *http.Request) (application.PostStockImportCommand, error)
	IntakeRejectStockImport(ctx context.Context, request *http.Request) (application.RejectStockImportCommand, error)
}

type StockImportDecisions interface {
	Approve(ctx context.Context, command application.PostStockImportCommand) (application.StockImportResult, error)
	Reject(ctx context.Context, command application.RejectStockImportCommand) (application.StockImportResult, error)
}

type StockImportQueries interface {
	Load(ctx context.Context, tenant domain.TenantID, id domain.StockImportID) (domain.StockImport, bool, error)
	List(ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[domain.StockImport], error)
}

func NewSubmitStockImportEndpoint(intake StockImportSubmissionIntake, submissions StockImportSubmissions) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodPost) {
			return
		}
		command, err := intake.IntakeStockImport(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := submissions.Submit(request.Context(), command)
		if errors.Is(err, application.ErrOwnerAccountNotInTenant) {
			writeProblem(response, http.StatusNotFound, codeOwnerAccountNotVisible)
			return
		}
		if err != nil {
			slog.ErrorContext(request.Context(), "stock import submission did not form an answer",
				"importId", command.ID.String(), "warehouse", command.Warehouse.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeStockImportAnswer(response, result, true)
	})
}

func NewApproveStockImportEndpoint(intake StockImportDecisionIntake, decisions StockImportDecisions) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodPost) {
			return
		}
		command, err := intake.IntakeApproveStockImport(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := decisions.Approve(request.Context(), command)
		if err != nil {
			slog.ErrorContext(request.Context(), "stock import approval did not form an answer", "importId", command.ID.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeStockImportAnswer(response, result, true)
	})
}

func NewRejectStockImportEndpoint(intake StockImportDecisionIntake, decisions StockImportDecisions) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodPost) {
			return
		}
		command, err := intake.IntakeRejectStockImport(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := decisions.Reject(request.Context(), command)
		if err != nil {
			slog.ErrorContext(request.Context(), "stock import rejection did not form an answer", "importId", command.ID.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeStockImportAnswer(response, result, true)
	})
}

func NewQueryStockImportsEndpoint(intake CatalogueIntake, queries StockImportQueries) http.Handler {
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
		pageQuery, err := ports.StockImportPages.Decode(values)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		if pageQuery.PageSize > 0 {
			query.Limit = pageQuery.PageSize
		}
		page, err := queries.List(request.Context(), query.Tenant, query.Limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "stock import list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		imports := make([]stockImportBody, 0, len(page.Rows))
		for _, item := range page.Rows {
			imports = append(imports, stockImportBodyOf(item))
		}
		writeJSON(response, http.StatusOK, stockImportListResponse{
			Outcome: "LISTED",
			Imports: imports,
			Page:    page.Page(query.Limit),
			Facets:  page.Facets,
		})
	})
}

func NewQueryStockImportEndpoint(intake CatalogueIntake, queries StockImportQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		cells, reason := queryCells(request, "importId")
		if reason != "" {
			writeMalformed(response, reason)
			return
		}
		id, err := domain.NewStockImportID(cells[0])
		if err != nil {
			writeMalformed(response, blankCellReason("importId"))
			return
		}
		item, found, err := queries.Load(request.Context(), query.Tenant, id)
		if err != nil {
			slog.ErrorContext(request.Context(), "stock import read did not form an answer", "importId", id.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if !found {
			writeProblem(response, http.StatusNotFound, "STOCK_IMPORT_NOT_FOUND")
			return
		}
		writeJSON(response, http.StatusOK, stockImportBodyOf(item))
	})
}

type stockImportAnswer struct {
	Outcome string             `json:"outcome"`
	Reason  string             `json:"reason,omitempty"`
	Issues  []stockImportIssue `json:"issues,omitempty"`
	Import  *stockImportBody   `json:"import,omitempty"`
}

type stockImportIssue struct {
	Side string `json:"side"`
	Line int    `json:"line"`
	Kind string `json:"kind"`
}

type stockImportListResponse struct {
	Outcome string               `json:"outcome"`
	Imports []stockImportBody    `json:"imports"`
	Page    cataloguepage.Page   `json:"page"`
	Facets  cataloguepage.Facets `json:"facets,omitempty"`
}

type stockImportBody struct {
	ImportID         string                  `json:"importId"`
	OwnerAccount     string                  `json:"ownerAccount"`
	WarehouseCode    string                  `json:"warehouseCode"`
	Reason           string                  `json:"reason"`
	Status           string                  `json:"status"`
	Submitter        string                  `json:"submitter"`
	SubmittedAt      string                  `json:"submittedAt"`
	GoodsLines       []stockImportGoodsBody  `json:"goodsLines"`
	CartonLines      []stockImportCartonBody `json:"cartonLines"`
	Poster           string                  `json:"poster,omitempty"`
	DutyRevision     int                     `json:"dutyRevision,omitempty"`
	PostedAt         string                  `json:"postedAt,omitempty"`
	GoodsMovementID  string                  `json:"goodsMovementId,omitempty"`
	CartonMovementID string                  `json:"cartonMovementId,omitempty"`
	Rejector         string                  `json:"rejector,omitempty"`
	RejectedAt       string                  `json:"rejectedAt,omitempty"`
	RejectionNote    string                  `json:"rejectionNote,omitempty"`
}

type stockImportGoodsBody struct {
	LocationCode      string `json:"locationCode"`
	GoodsCode         string `json:"goodsCode"`
	Unit              string `json:"unit"`
	StockState        string `json:"stockState"`
	Quantity          int64  `json:"quantity"`
	BatchNumber       string `json:"batchNumber,omitempty"`
	ExpiryDate        string `json:"expiryDate,omitempty"`
	LabelAlias        string `json:"labelAlias,omitempty"`
	DeclaredInboundAt string `json:"declaredInboundAt,omitempty"`
}

type stockImportCartonBody struct {
	LocationCode      string                         `json:"locationCode"`
	CartonNumber      string                         `json:"cartonNumber"`
	Contents          []stockImportCartonContentBody `json:"contents"`
	DeclaredInboundAt string                         `json:"declaredInboundAt,omitempty"`
}

type stockImportCartonContentBody struct {
	GoodsCode string `json:"goodsCode"`
	Quantity  int64  `json:"quantity"`
}

func writeStockImportAnswer(response http.ResponseWriter, result application.StockImportResult, created bool) {
	name := result.Outcome().String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := stockImportAnswer{Outcome: name}
	status := http.StatusOK
	switch result.Outcome() {
	case application.StockImportSubmitted, application.StockImportExistingResult, application.StockImportPosted, application.StockImportRejected:
		item, present := result.Import()
		if !present {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		body := stockImportBodyOf(item)
		answer.Import = &body
		if created && (result.Outcome() == application.StockImportSubmitted || result.Outcome() == application.StockImportPosted || result.Outcome() == application.StockImportRejected) {
			status = http.StatusCreated
		}
	case application.StockImportWarehouseNotAccepting:
		reason, present := result.NotAcceptingReason()
		if !present || reason.String() == "" {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		answer.Reason = reason.String()
	case application.StockImportContentInvalid, application.StockImportNoLongerValid:
		for _, issue := range result.Issues() {
			answer.Issues = append(answer.Issues, stockImportIssue{Side: issue.Side.String(), Line: issue.Line, Kind: issue.Kind.String()})
		}
	}
	writeJSON(response, status, answer)
}

func stockImportBodyOf(item domain.StockImport) stockImportBody {
	body := stockImportBody{
		ImportID: item.ID().String(), OwnerAccount: item.Owner().String(), WarehouseCode: item.Warehouse().String(),
		Reason: item.Reason().String(), Status: item.Status().String(), Submitter: item.Submitter().String(),
		SubmittedAt: cataloguepage.FormatInstant(item.SubmittedAt()),
		GoodsLines:  make([]stockImportGoodsBody, 0, len(item.GoodsLines())),
		CartonLines: make([]stockImportCartonBody, 0, len(item.CartonLines())),
	}
	for _, line := range item.GoodsLines() {
		goods := stockImportGoodsBody{
			LocationCode: line.Location().String(), GoodsCode: line.Goods().String(), Unit: line.Unit().String(),
			StockState: line.State().String(), Quantity: line.Quantity(),
			BatchNumber: line.BatchNumber().String(), ExpiryDate: line.Expiry().String(), LabelAlias: line.Alias().String(),
		}
		if !line.DeclaredInboundAt().IsZero() {
			goods.DeclaredInboundAt = cataloguepage.FormatInstant(line.DeclaredInboundAt())
		}
		body.GoodsLines = append(body.GoodsLines, goods)
	}
	for _, line := range item.CartonLines() {
		carton := stockImportCartonBody{
			LocationCode: line.Location().String(), CartonNumber: line.Number().String(),
			Contents: make([]stockImportCartonContentBody, 0, len(line.Contents())),
		}
		if !line.DeclaredInboundAt().IsZero() {
			carton.DeclaredInboundAt = cataloguepage.FormatInstant(line.DeclaredInboundAt())
		}
		for _, content := range line.Contents() {
			carton.Contents = append(carton.Contents, stockImportCartonContentBody{GoodsCode: content.Goods().String(), Quantity: content.Quantity()})
		}
		body.CartonLines = append(body.CartonLines, carton)
	}
	if item.Poster().String() != "" {
		body.Poster = item.Poster().String()
		body.PostedAt = cataloguepage.FormatInstant(item.PostedAt())
		body.DutyRevision = item.DutyRevision()
	}
	body.GoodsMovementID = item.GoodsMovementID().String()
	body.CartonMovementID = item.CartonMovementID().String()
	if item.Rejector().String() != "" {
		body.Rejector = item.Rejector().String()
		body.RejectedAt = cataloguepage.FormatInstant(item.RejectedAt())
		body.RejectionNote = item.RejectionNote()
	}
	return body
}
