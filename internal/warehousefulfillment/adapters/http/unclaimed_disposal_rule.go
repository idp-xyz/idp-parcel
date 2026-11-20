package warehousehttp

import (
	"context"
	"log/slog"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// DisposalRuleRegistrationIntake 是待认领处置规则登记口的 Intake（登记册配置写面，不判准入范围）。
type DisposalRuleRegistrationIntake interface {
	IntakeDisposalRuleRegistration(ctx context.Context, request *http.Request) (application.RegisterDisposalRuleCommand, error)
}

// DisposalRuleRegistrar 是处置规则登记口转交的编排。
type DisposalRuleRegistrar interface {
	Register(ctx context.Context, command application.RegisterDisposalRuleCommand) (application.RegisterDisposalRuleResult, error)
}

// DisposalRuleCatalogueReader 分页读各仓当前待认领处置规则。
type DisposalRuleCatalogueReader interface {
	ListDisposalRules(ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.DisposalRuleRow], error)
}

// DisposalRuleRevisionsReader 按修订号升序读一个仓的全部待认领处置规则修订。
type DisposalRuleRevisionsReader interface {
	DisposalRuleRevisions(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode) ([]ports.DisposalRuleRow, error)
}

var (
	_ DisposalRuleRegistrar       = (*application.RegisterDisposalRuleHandler)(nil)
	_ DisposalRuleCatalogueReader = (*application.QueryDisposalRulesHandler)(nil)
	_ DisposalRuleRevisionsReader = (*application.QueryDisposalRulesHandler)(nil)
)

const (
	outcomeDisposalRulesListed         = "DISPOSAL_RULES_LISTED"
	outcomeDisposalRuleRevisionsListed = "DISPOSAL_RULE_REVISIONS_LISTED"
)

type disposalRuleBody struct {
	WarehouseCode      string   `json:"warehouseCode"`
	Revision           int      `json:"revision"`
	ClaimWindowSeconds int64    `json:"claimWindowSeconds"`
	DisposalMethods    []string `json:"disposalMethods"`
	Registrant         string   `json:"registrant"`
	RegisteredAt       string   `json:"registeredAt"`
}

type registerDisposalRuleAnswer struct {
	registrationAnswerHead
	DisposalRule *disposalRuleBody `json:"disposalRule,omitempty"`
}

type disposalRuleListResponse struct {
	Outcome       string             `json:"outcome"`
	DisposalRules []disposalRuleBody `json:"disposalRules"`
	Page          cataloguepage.Page `json:"page"`
}

type disposalRuleRevisionsResponse struct {
	Outcome       string             `json:"outcome"`
	WarehouseCode string             `json:"warehouseCode"`
	Revisions     []disposalRuleBody `json:"revisions"`
}

// NewRegisterDisposalRuleEndpoint 交回待认领处置规则登记入口（POST /warehouse-fulfillment-unclaimed-disposal-rule-registrations）。
// 挂登记册配置写面，不判准入范围（ADR-0384 决定五、AT-WF-062）。新建答 201，其余业务答复答 200。
func NewRegisterDisposalRuleEndpoint(intake DisposalRuleRegistrationIntake, registrar DisposalRuleRegistrar) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodPost) {
			return
		}
		command, err := intake.IntakeDisposalRuleRegistration(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := registrar.Register(request.Context(), command)
		if err != nil {
			slog.ErrorContext(request.Context(), "unclaimed disposal rule registration did not form an answer",
				"warehouseCode", command.Warehouse.String(), "revision", command.Revision)
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeRegistrationAnswer(response, result, func(head registrationAnswerHead, echoed *domain.DisposalRuleRevision) any {
			answer := registerDisposalRuleAnswer{registrationAnswerHead: head}
			if echoed != nil {
				body := disposalRuleBodyOfRevision(*echoed)
				answer.DisposalRule = &body
			}
			return answer
		})
	})
}

// NewQueryDisposalRulesEndpoint 交回各仓当前处置规则的列表查阅入口（GET /warehouse-fulfillment-unclaimed-disposal-rules）。
func NewQueryDisposalRulesEndpoint(intake CatalogueIntake, reader DisposalRuleCatalogueReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		pageQuery, err := ports.UnclaimedDisposalRulePages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		page, err := reader.ListDisposalRules(request.Context(), query.Tenant, query.Limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "unclaimed disposal rules list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeJSON(response, http.StatusOK, disposalRuleListResponse{
			Outcome:       outcomeDisposalRulesListed,
			DisposalRules: disposalRuleBodiesOfRows(page.Rows),
			Page:          page.Page(query.Limit),
		})
	})
}

// NewQueryDisposalRuleRevisionsEndpoint 交回一个仓全部处置规则修订的查阅入口
// （GET /warehouse-fulfillment-unclaimed-disposal-rule-revisions?warehouseCode=…）。没登记答空数组，不答 404（AT-WF-064）。
func NewQueryDisposalRuleRevisionsEndpoint(intake CatalogueIntake, reader DisposalRuleRevisionsReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		cells, reason := queryCells(request, warehouseCodeQueryKey)
		if reason != "" {
			writeMalformed(response, reason)
			return
		}
		warehouse, err := domain.NewWarehouseCode(cells[0])
		if err != nil {
			writeMalformed(response, blankCellReason(warehouseCodeQueryKey))
			return
		}
		rows, err := reader.DisposalRuleRevisions(request.Context(), query.Tenant, warehouse)
		if err != nil {
			slog.ErrorContext(request.Context(), "unclaimed disposal rule revisions did not form an answer",
				"warehouseCode", warehouse.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeJSON(response, http.StatusOK, disposalRuleRevisionsResponse{
			Outcome:       outcomeDisposalRuleRevisionsListed,
			WarehouseCode: warehouse.String(),
			Revisions:     disposalRuleBodiesOfRows(rows),
		})
	})
}

func disposalRuleBodyOfRevision(revision domain.DisposalRuleRevision) disposalRuleBody {
	methods := make([]string, 0, len(revision.Content().Methods()))
	for _, method := range revision.Content().Methods() {
		methods = append(methods, method.String())
	}
	return disposalRuleBody{
		WarehouseCode:      revision.Warehouse().String(),
		Revision:           revision.Revision(),
		ClaimWindowSeconds: revision.Content().ClaimWindowSeconds(),
		DisposalMethods:    methods,
		Registrant:         revision.Registrant().String(),
		RegisteredAt:       rfc3339(revision.RegisteredAt()),
	}
}

func disposalRuleBodiesOfRows(rows []ports.DisposalRuleRow) []disposalRuleBody {
	bodies := make([]disposalRuleBody, 0, len(rows))
	for _, row := range rows {
		methods := make([]string, len(row.DisposalMethods))
		copy(methods, row.DisposalMethods)
		bodies = append(bodies, disposalRuleBody{
			WarehouseCode:      row.WarehouseCode,
			Revision:           row.Revision,
			ClaimWindowSeconds: row.ClaimWindowSeconds,
			DisposalMethods:    methods,
			Registrant:         row.Registrant,
			RegisteredAt:       rfc3339(row.RegisteredAt),
		})
	}
	return bodies
}
