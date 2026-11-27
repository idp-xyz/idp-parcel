package warehousehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// GoodsCatalogueReader 是一个货主名下各商品当前修订的列表读口。
type GoodsCatalogueReader interface {
	ListGoods(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.GoodsRow], error)
}

// GoodsCurrentReader 按（货主，商品编码）读当前修订。
type GoodsCurrentReader interface {
	CurrentGoods(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		code domain.GoodsCode,
	) (ports.GoodsRow, bool, error)
}

// GoodsRevisionsReader 读一个商品的全部修订。
type GoodsRevisionsReader interface {
	GoodsRevisions(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		code domain.GoodsCode,
	) ([]ports.GoodsRow, error)
}

// GoodsFinder 按（货主，一个码）找此刻指向的商品。
type GoodsFinder interface {
	FindGoods(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		code domain.IdentifierCode,
	) (ports.GoodsRow, bool, error)
}

var (
	_ GoodsCatalogueReader = (*application.QueryGoodsHandler)(nil)
	_ GoodsCurrentReader   = (*application.QueryGoodsHandler)(nil)
	_ GoodsRevisionsReader = (*application.QueryGoodsHandler)(nil)
	_ GoodsFinder          = (*application.QueryGoodsHandler)(nil)
)

const (
	outcomeGoodsListed          = "GOODS_LISTED"
	outcomeGoodsCurrent         = "GOODS_CURRENT"
	outcomeGoodsRevisionsListed = "GOODS_REVISIONS_LISTED"
	outcomeGoodsFound           = "GOODS_FOUND"
	ownerAccountQueryKey        = "ownerAccount"
	goodsCodeQueryKey           = "goodsCode"
	goodsIdentifierQueryKey     = "code"
)

type goodsListResponse struct {
	Outcome      string             `json:"outcome"`
	OwnerAccount string             `json:"ownerAccount"`
	Goods        []goodsBody        `json:"goods"`
	Page         cataloguepage.Page `json:"page"`
}

type goodsCurrentResponse struct {
	Outcome      string     `json:"outcome"`
	OwnerAccount string     `json:"ownerAccount"`
	GoodsCode    string     `json:"goodsCode"`
	Found        bool       `json:"found"`
	Goods        *goodsBody `json:"goods,omitempty"`
}

type goodsRevisionsResponse struct {
	Outcome      string      `json:"outcome"`
	OwnerAccount string      `json:"ownerAccount"`
	GoodsCode    string      `json:"goodsCode"`
	Revisions    []goodsBody `json:"revisions"`
}

type goodsFoundResponse struct {
	Outcome      string     `json:"outcome"`
	OwnerAccount string     `json:"ownerAccount"`
	Code         string     `json:"code"`
	Found        bool       `json:"found"`
	Goods        *goodsBody `json:"goods,omitempty"`
}

// NewQueryGoodsEndpoint 交回一个货主名下各商品当前修订的查阅入口（GET /warehouse-fulfillment-goods?ownerAccount=…）。
// 货主在查询串里，是业务内容；分页参数按 ports.GoodsPages 解，货主那一键先摘掉。查不到是空数组，不答 404。
func NewQueryGoodsEndpoint(intake CatalogueIntake, reader GoodsCatalogueReader) http.Handler {
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
		pageQuery, err := goodsPageQuery(request)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		if pageQuery.PageSize > 0 {
			query.Limit = pageQuery.PageSize
		}
		page, err := reader.ListGoods(request.Context(), query.Tenant, owner, query.Limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "goods list did not form an answer", "ownerAccount", owner.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeJSON(response, http.StatusOK, goodsListResponse{
			Outcome: outcomeGoodsListed, OwnerAccount: owner.String(), Goods: goodsBodiesOfRows(page.Rows), Page: page.Page(query.Limit),
		})
	})
}

// NewQueryCurrentGoodsEndpoint 交回一个商品当前修订的查阅入口（GET /warehouse-fulfillment-goods-current?ownerAccount=…&goodsCode=…）。
// 没登记与别的货主的同编码同答：found 为假，不答 404。
func NewQueryCurrentGoodsEndpoint(intake CatalogueIntake, reader GoodsCurrentReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		owner, code, reason := goodsIdentityOf(request)
		if reason != "" {
			writeMalformed(response, reason)
			return
		}
		row, found, err := reader.CurrentGoods(request.Context(), query.Tenant, owner, code)
		if err != nil {
			slog.ErrorContext(request.Context(), "current goods did not form an answer",
				"ownerAccount", owner.String(), "goodsCode", code.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		answer := goodsCurrentResponse{
			Outcome: outcomeGoodsCurrent, OwnerAccount: owner.String(), GoodsCode: code.String(), Found: found,
		}
		if found {
			body := goodsBodyOfRow(row)
			answer.Goods = &body
		}
		writeJSON(response, http.StatusOK, answer)
	})
}

// NewQueryGoodsRevisionsEndpoint 交回一个商品全部修订的查阅入口（GET /warehouse-fulfillment-goods-revisions?ownerAccount=…&goodsCode=…），
// 按修订号升序。没登记过答空数组，不答 404。
func NewQueryGoodsRevisionsEndpoint(intake CatalogueIntake, reader GoodsRevisionsReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		owner, code, reason := goodsIdentityOf(request)
		if reason != "" {
			writeMalformed(response, reason)
			return
		}
		rows, err := reader.GoodsRevisions(request.Context(), query.Tenant, owner, code)
		if err != nil {
			slog.ErrorContext(request.Context(), "goods revisions did not form an answer",
				"ownerAccount", owner.String(), "goodsCode", code.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeJSON(response, http.StatusOK, goodsRevisionsResponse{
			Outcome: outcomeGoodsRevisionsListed, OwnerAccount: owner.String(), GoodsCode: code.String(), Revisions: goodsBodiesOfRows(rows),
		})
	})
}

// NewFindGoodsEndpoint 交回按一个码找商品的查阅入口（GET /warehouse-fulfillment-goods-by-code?ownerAccount=…&code=…）。
// 码是商品编码，或是此刻有效的别名。找不到答 found 为假，不答 404。
func NewFindGoodsEndpoint(intake CatalogueIntake, finder GoodsFinder) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		owner, code, reason := goodsLookupOf(request)
		if reason != "" {
			writeMalformed(response, reason)
			return
		}
		row, found, err := finder.FindGoods(request.Context(), query.Tenant, owner, code)
		if err != nil {
			slog.ErrorContext(request.Context(), "find goods did not form an answer", "ownerAccount", owner.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		answer := goodsFoundResponse{Outcome: outcomeGoodsFound, OwnerAccount: owner.String(), Code: code.String(), Found: found}
		if found {
			body := goodsBodyOfRow(row)
			answer.Goods = &body
		}
		writeJSON(response, http.StatusOK, answer)
	})
}

func ownerAccountOf(request *http.Request) (domain.OwnerAccountID, string) {
	values, err := cataloguepage.ParseQuery(request.URL.RawQuery)
	if err != nil {
		var malformed *cataloguepage.MalformedQuery
		if errors.As(err, &malformed) {
			return domain.OwnerAccountID{}, malformed.Reason
		}
		return domain.OwnerAccountID{}, "the query string could not be read"
	}
	given := values[ownerAccountQueryKey]
	if len(given) != 1 {
		return domain.OwnerAccountID{}, ownerAccountQueryKey + " must be given exactly once"
	}
	owner, err := domain.NewOwnerAccountID(given[0])
	if err != nil {
		return domain.OwnerAccountID{}, blankCellReason(ownerAccountQueryKey)
	}
	return owner, ""
}

func goodsPageQuery(request *http.Request) (cataloguepage.Query, error) {
	values, err := cataloguepage.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return cataloguepage.Query{}, err
	}
	delete(values, ownerAccountQueryKey)
	return ports.GoodsPages.Decode(values)
}

func goodsIdentityOf(request *http.Request) (domain.OwnerAccountID, domain.GoodsCode, string) {
	cells, reason := queryCells(request, ownerAccountQueryKey, goodsCodeQueryKey)
	if reason != "" {
		return domain.OwnerAccountID{}, domain.GoodsCode{}, reason
	}
	owner, err := domain.NewOwnerAccountID(cells[0])
	if err != nil {
		return domain.OwnerAccountID{}, domain.GoodsCode{}, blankCellReason(ownerAccountQueryKey)
	}
	code, err := domain.NewGoodsCode(cells[1])
	if err != nil {
		return domain.OwnerAccountID{}, domain.GoodsCode{}, blankCellReason(goodsCodeQueryKey)
	}
	return owner, code, ""
}

func goodsLookupOf(request *http.Request) (domain.OwnerAccountID, domain.IdentifierCode, string) {
	cells, reason := queryCells(request, ownerAccountQueryKey, goodsIdentifierQueryKey)
	if reason != "" {
		return domain.OwnerAccountID{}, domain.IdentifierCode{}, reason
	}
	owner, err := domain.NewOwnerAccountID(cells[0])
	if err != nil {
		return domain.OwnerAccountID{}, domain.IdentifierCode{}, blankCellReason(ownerAccountQueryKey)
	}
	code, err := domain.NewIdentifierCode(cells[1])
	if err != nil {
		return domain.OwnerAccountID{}, domain.IdentifierCode{}, blankCellReason(goodsIdentifierQueryKey)
	}
	return owner, code, ""
}
