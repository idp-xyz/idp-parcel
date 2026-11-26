package warehousehttp

import (
	"context"
	"log/slog"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// ToteIntake 是周转箱登记与停用的 Intake。周转箱是主数据，挂主数据登记，不挂运营决定。
type ToteIntake interface {
	IntakeToteRegistration(ctx context.Context, request *http.Request) (application.RegisterTotesCommand, error)
	IntakeToteRetirement(ctx context.Context, request *http.Request) (application.RetireToteCommand, error)
}

func (intake *OperatorRegistryIntake) IntakeToteRegistration(ctx context.Context, request *http.Request) (application.RegisterTotesCommand, error) {
	return intakeRegistration(ctx, request, intake.authenticator, registrationjson.RegisterTotesCommand)
}

func (intake *OperatorRegistryIntake) IntakeToteRetirement(ctx context.Context, request *http.Request) (application.RetireToteCommand, error) {
	return intakeRegistration(ctx, request, intake.authenticator, registrationjson.RetireToteCommand)
}

func (UnconfiguredIntake) IntakeToteRegistration(context.Context, *http.Request) (application.RegisterTotesCommand, error) {
	return application.RegisterTotesCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeToteRetirement(context.Context, *http.Request) (application.RetireToteCommand, error) {
	return application.RetireToteCommand{}, ErrAccessChannelNotConfigured
}

// NewRegisterTotesEndpoint 交回周转箱登记的入口（POST /warehouse-fulfillment-tote-registrations）。
func NewRegisterTotesEndpoint(intake ToteIntake, waves WaveService) http.Handler {
	return toteEndpoint(func(request *http.Request) (application.WaveResult, error) {
		command, err := intake.IntakeToteRegistration(request.Context(), request)
		if err != nil {
			return application.WaveResult{}, &toteIntakeError{err}
		}
		return waves.RegisterTotes(request.Context(), command)
	})
}

// NewRetireToteEndpoint 交回周转箱停用的入口（POST /warehouse-fulfillment-tote-retirements）。
func NewRetireToteEndpoint(intake ToteIntake, waves WaveService) http.Handler {
	return toteEndpoint(func(request *http.Request) (application.WaveResult, error) {
		command, err := intake.IntakeToteRetirement(request.Context(), request)
		if err != nil {
			return application.WaveResult{}, &toteIntakeError{err}
		}
		return waves.RetireTote(request.Context(), command)
	})
}

// toteIntakeError 把认证与载荷这一段的错与编排的错分开：前者照 Intake 的口径答，后者是没形成答案。
type toteIntakeError struct{ err error }

func (wrapped *toteIntakeError) Error() string { return wrapped.err.Error() }
func (wrapped *toteIntakeError) Unwrap() error { return wrapped.err }

func toteEndpoint(run func(*http.Request) (application.WaveResult, error)) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		result, err := run(request)
		if intake, ok := err.(*toteIntakeError); ok {
			writeReceiptProblem(response, request, intake.err)
			return
		}
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeWaveAnswer(response, request, result)
	})
}

// ToteQueries 是周转箱名册的读。
type ToteQueries interface {
	ListTotes(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.ToteRow], error)
}

// NewQueryTotesEndpoint 分页列出周转箱（GET /warehouse-fulfillment-totes）。
func NewQueryTotesEndpoint(intake CatalogueIntake, queries ToteQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		pageQuery, err := ports.TotePages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		page, err := queries.ListTotes(request.Context(), query.Tenant, query.Limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "tote list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		items := make([]toteBody, 0, len(page.Rows))
		for _, row := range page.Rows {
			items = append(items, toteBody{
				WarehouseCode: row.WarehouseCode, ToteCode: row.ToteCode, State: row.State, RequestID: row.RequestID,
				BoundAt: optRFC3339(row.BoundAt), RegisteredBy: row.RegisteredBy, RegisteredAt: rfc3339(row.RegisteredAt),
				RetiredBy: row.RetiredBy, RetiredAt: optRFC3339(row.RetiredAt),
			})
		}
		writeJSON(response, http.StatusOK, toteListResponse{
			Outcome: "TOTES_LISTED",
			Totes:   items,
			Page:    page.Page(query.Limit),
		})
	})
}

type toteListResponse struct {
	Outcome string             `json:"outcome"`
	Totes   []toteBody         `json:"totes"`
	Page    cataloguepage.Page `json:"page"`
}

type toteBody struct {
	WarehouseCode string `json:"warehouseCode"`
	ToteCode      string `json:"toteCode"`
	// State 是 FREE、IN_USE 或 RETIRED；IN_USE 时带着绑着的出库请求与绑上的时刻。
	State        string `json:"state"`
	RequestID    string `json:"requestId,omitempty"`
	BoundAt      string `json:"boundAt,omitempty"`
	RegisteredBy string `json:"registeredBy"`
	RegisteredAt string `json:"registeredAt"`
	RetiredBy    string `json:"retiredBy,omitempty"`
	RetiredAt    string `json:"retiredAt,omitempty"`
}

// NewReleaseToteEndpoint 交回由人放开周转箱的入口（POST /warehouse-fulfillment-tote-releases）。
// 它动的是一张单在拣货里的归属，与放波次、撤单挂同一种运营决定，不挂主数据登记。
func NewReleaseToteEndpoint(intake WaveReleaseIntake, waves WaveService) http.Handler {
	return toteEndpoint(func(request *http.Request) (application.WaveResult, error) {
		command, err := intake.IntakeToteRelease(request.Context(), request)
		if err != nil {
			return application.WaveResult{}, &toteIntakeError{err}
		}
		return waves.ReleaseTote(request.Context(), command)
	})
}
