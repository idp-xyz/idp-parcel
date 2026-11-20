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

// PreAdviceQueries 是入库预报的三个运营查阅口。
type PreAdviceQueries interface {
	ListPreAdvices(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[application.PreAdviceReading], error)
	PreAdviceForOwner(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		id domain.PreAdviceID,
	) (application.PreAdviceReading, bool, error)
	PreAdviceByDocumentNumber(
		ctx context.Context,
		tenant domain.TenantID,
		number domain.DocumentNumber,
	) (application.PreAdviceReading, bool, error)
}

var _ PreAdviceQueries = (*application.QueryPreAdviceHandler)(nil)

const (
	outcomePreAdvicesListed  = "PRE_ADVICES_LISTED"
	outcomePreAdvice         = "PRE_ADVICE"
	outcomePreAdviceByNumber = "PRE_ADVICE_BY_NUMBER"
	preAdviceIDQueryKey      = "preAdviceId"
	documentNumberQueryKey   = "documentNumber"
)

type preAdviceListResponse struct {
	Outcome      string             `json:"outcome"`
	OwnerAccount string             `json:"ownerAccount"`
	PreAdvices   []preAdviceBody    `json:"preAdvices"`
	Page         cataloguepage.Page `json:"page"`
}

type preAdviceOneResponse struct {
	Outcome      string         `json:"outcome"`
	OwnerAccount string         `json:"ownerAccount"`
	PreAdviceID  string         `json:"preAdviceId"`
	Found        bool           `json:"found"`
	PreAdvice    *preAdviceBody `json:"preAdvice,omitempty"`
}

type preAdviceByNumberResponse struct {
	Outcome        string         `json:"outcome"`
	DocumentNumber string         `json:"documentNumber"`
	Found          bool           `json:"found"`
	PreAdvice      *preAdviceBody `json:"preAdvice,omitempty"`
}

// NewQueryPreAdvicesEndpoint 交回一个货主名下预报的查阅入口（GET /warehouse-fulfillment-inbound-pre-advices?ownerAccount=…）。
// 货主在查询串里，是业务内容；分页参数按 ports.PreAdvicePages 解，货主那一键先摘掉。列表带现行告知，不带历史版。
func NewQueryPreAdvicesEndpoint(intake CatalogueIntake, queries PreAdviceQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		owner, reason := ownerAccountOf(request)
		if reason != "" {
			writeMalformed(response, reason)
			return
		}
		pageQuery, err := preAdvicePageQuery(request)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		page, err := queries.ListPreAdvices(request.Context(), query.Tenant, owner, query.Limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "inbound pre-advice list did not form an answer", "ownerAccount", owner.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeJSON(response, http.StatusOK, preAdviceListResponse{
			Outcome: outcomePreAdvicesListed, OwnerAccount: owner.String(),
			PreAdvices: operatorPreAdviceBodies(page.Rows), Page: page.Page(query.Limit),
		})
	})
}

// NewQueryPreAdviceEndpoint 交回一份预报的查阅入口（GET /warehouse-fulfillment-inbound-pre-advice?ownerAccount=…&preAdviceId=…）。
// 找不到答 found 为假，不答 404。告知全部版本按册上的顺序带出，那一序是版本号升序。
func NewQueryPreAdviceEndpoint(intake CatalogueIntake, queries PreAdviceQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		owner, id, reason := preAdviceIdentityOf(request)
		if reason != "" {
			writeMalformed(response, reason)
			return
		}
		reading, found, err := queries.PreAdviceForOwner(request.Context(), query.Tenant, owner, id)
		if err != nil {
			slog.ErrorContext(request.Context(), "inbound pre-advice did not form an answer",
				"ownerAccount", owner.String(), "preAdviceId", id.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		answer := preAdviceOneResponse{
			Outcome: outcomePreAdvice, OwnerAccount: owner.String(), PreAdviceID: id.String(), Found: found,
		}
		if found {
			body := operatorPreAdviceBody(reading, true)
			answer.PreAdvice = &body
		}
		writeJSON(response, http.StatusOK, answer)
	})
}

// NewQueryPreAdviceByNumberEndpoint 交回按单据号查阅的入口（GET /warehouse-fulfillment-inbound-pre-advice-by-number?documentNumber=…）。
// 逐字找，只在本租户。找不到答 found 为假，不答 404。
func NewQueryPreAdviceByNumberEndpoint(intake CatalogueIntake, queries PreAdviceQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		number, reason := documentNumberOf(request)
		if reason != "" {
			writeMalformed(response, reason)
			return
		}
		reading, found, err := queries.PreAdviceByDocumentNumber(request.Context(), query.Tenant, number)
		if err != nil {
			slog.ErrorContext(request.Context(), "inbound pre-advice by document number did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		answer := preAdviceByNumberResponse{Outcome: outcomePreAdviceByNumber, DocumentNumber: number.String(), Found: found}
		if found {
			body := operatorPreAdviceBody(reading, true)
			answer.PreAdvice = &body
		}
		writeJSON(response, http.StatusOK, answer)
	})
}

// preAdvicePageQuery 先摘掉 ownerAccount 再 Decode：那一键是业务内容，留在查询串里会被当成未知键。
func preAdvicePageQuery(request *http.Request) (cataloguepage.Query, error) {
	values, err := cataloguepage.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return cataloguepage.Query{}, err
	}
	delete(values, ownerAccountQueryKey)
	return ports.PreAdvicePages.Decode(values)
}

func preAdviceIdentityOf(request *http.Request) (domain.OwnerAccountID, domain.PreAdviceID, string) {
	cells, reason := queryCells(request, ownerAccountQueryKey, preAdviceIDQueryKey)
	if reason != "" {
		return domain.OwnerAccountID{}, domain.PreAdviceID{}, reason
	}
	owner, err := domain.NewOwnerAccountID(cells[0])
	if err != nil {
		return domain.OwnerAccountID{}, domain.PreAdviceID{}, blankCellReason(ownerAccountQueryKey)
	}
	id, err := domain.NewPreAdviceID(cells[1])
	if err != nil {
		return domain.OwnerAccountID{}, domain.PreAdviceID{}, blankCellReason(preAdviceIDQueryKey)
	}
	return owner, id, ""
}

func documentNumberOf(request *http.Request) (domain.DocumentNumber, string) {
	cells, reason := queryCells(request, documentNumberQueryKey)
	if reason != "" {
		return domain.DocumentNumber{}, reason
	}
	number, err := domain.NewDocumentNumber(cells[0])
	if err != nil {
		return domain.DocumentNumber{}, blankCellReason(documentNumberQueryKey)
	}
	return number, ""
}
