package warehousehttp

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// StockReader 是库存读口转交的编排。
type StockReader interface {
	Query(ctx context.Context, query application.StockQuery) (application.StockSnapshot, error)
}

func NewQueryStockEndpoint(intake CatalogueIntake, reader StockReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		identity, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		query, reason := operatorStockQuery(request, identity.Tenant, identity.Limit)
		if reason != "" {
			writeMalformed(response, reason)
			return
		}
		snapshot, err := reader.Query(request.Context(), query)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeJSON(response, http.StatusOK, stockSnapshotBody(snapshot))
	})
}

func NewQueryCustomerStockEndpoint(intake CustomerInboundPreAdvicePorts, reader StockReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseCustomerMethod(response, request, http.MethodGet) {
			return
		}
		identity, err := intake.IntakeWarehouseRead(request.Context(), request)
		if err != nil {
			answerCustomerIntake(response, request, err)
			return
		}
		query, reason := customerStockQuery(request, identity.Tenant, identity.Owner)
		if reason != "" {
			writeCustomerProblem(response, request, http.StatusBadRequest, codeMalformedRequest, reason)
			return
		}
		snapshot, err := reader.Query(request.Context(), query)
		if err != nil {
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		body := stockSnapshotBody(snapshot)
		// 货主面不翻页（见 customerStockQuery），page 里的游标不交出去。
		body.Page = nil
		writeJSON(response, http.StatusOK, body)
	})
}

func operatorStockQuery(request *http.Request, tenant domain.TenantID, limit int) (application.StockQuery, string) {
	values, reason := stockQueryValues(request, "ownerAccount", "warehouseCode", "view", stockAfterQueryKey)
	if reason != "" {
		return application.StockQuery{}, reason
	}
	owner, err := domain.NewOwnerAccountID(values["ownerAccount"])
	if err != nil {
		return application.StockQuery{}, "ownerAccount must not be blank"
	}
	warehouse, reason := optionalWarehouse(values["warehouseCode"])
	if reason != "" {
		return application.StockQuery{}, reason
	}
	view, reason := stockView(values["view"])
	if reason != "" {
		return application.StockQuery{}, reason
	}
	query := application.StockQuery{Tenant: tenant, Owner: owner, Warehouse: warehouse, View: view, Now: time.Now()}
	return stockPaging(query, values, limit)
}

func customerStockQuery(request *http.Request, tenant domain.TenantID, owner domain.OwnerAccountID) (application.StockQuery, string) {
	values, reason := stockQueryValues(request, "warehouseCode", "view")
	if reason != "" {
		return application.StockQuery{}, reason
	}
	warehouse, reason := optionalWarehouse(values["warehouseCode"])
	if reason != "" {
		return application.StockQuery{}, reason
	}
	view, reason := stockView(values["view"])
	if reason != "" {
		return application.StockQuery{}, reason
	}
	query := application.StockQuery{Tenant: tenant, Owner: owner, Warehouse: warehouse, View: view, Customer: true, Now: time.Now()}
	if view != application.StockViewMovements {
		return query, ""
	}
	// 货主面的流水不翻页，照旧只答最早入账的一页：游标里带着末行的流水标识与入账时刻，这两格货主面都不给，
	// 游标又不防解读。要让货主面翻页，先得定这两格能不能随游标交出去。
	conditions := stockMovementConditions(query)
	conditions.Set("sort", "postedAt")
	return stockMovementPage(query, conditions, operatorCatalogueReadLimit)
}

const stockAfterQueryKey = "after"

// stockPaging 给运营面流水那一种看法解出翻页。其余看法答整份、不翻页，带了 after 即拒：收下不用，调用方会以为自己翻到了下一页。
func stockPaging(query application.StockQuery, values map[string]string, limit int) (application.StockQuery, string) {
	after, carried := values[stockAfterQueryKey]
	if query.View != application.StockViewMovements {
		if carried {
			return application.StockQuery{}, "after is only taken by the movements view"
		}
		return query, ""
	}
	conditions := stockMovementConditions(query)
	if carried {
		conditions.Set(stockAfterQueryKey, after)
	}
	return stockMovementPage(query, conditions, limit)
}

// stockMovementConditions 按已经判过的货主、仓与看法重写一份交给共用件的条件，不拿原始查询串去解：仓没给与给了空串
// 要是同一份列表，调用方带来的 q 与 sort 读口不收，也不该有路进到解码里。
func stockMovementConditions(query application.StockQuery) url.Values {
	conditions := url.Values{"ownerAccount": {query.Owner.String()}, "view": {string(query.View)}}
	if warehouse := query.Warehouse.String(); warehouse != "" {
		conditions.Set("warehouseCode", warehouse)
	}
	return conditions
}

func stockMovementPage(query application.StockQuery, conditions url.Values, limit int) (application.StockQuery, string) {
	page, err := ports.StockMovementPages.Decode(conditions)
	if err != nil {
		var malformed *cataloguepage.MalformedQuery
		if errors.As(err, &malformed) {
			return application.StockQuery{}, malformed.Reason
		}
		return application.StockQuery{}, "the paging parameters could not be read"
	}
	query.Page, query.Limit = page, limit
	return query, ""
}

func stockQueryValues(request *http.Request, allowed ...string) (map[string]string, string) {
	parsed, err := cataloguepage.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return nil, "the query string could not be read"
	}
	allowedSet := map[string]bool{}
	for _, key := range allowed {
		allowedSet[key] = true
	}
	values := map[string]string{}
	for key, given := range parsed {
		if !allowedSet[key] {
			return nil, "the query has a key this read does not take"
		}
		if len(given) != 1 {
			return nil, key + " must be given exactly once"
		}
		values[key] = given[0]
	}
	return values, ""
}

func optionalWarehouse(raw string) (domain.WarehouseCode, string) {
	if raw == "" {
		return domain.WarehouseCode{}, ""
	}
	warehouse, err := domain.NewWarehouseCode(raw)
	if err != nil {
		return domain.WarehouseCode{}, "warehouseCode must not be blank"
	}
	return warehouse, ""
}

func stockView(raw string) (application.StockView, string) {
	if raw == "" {
		return application.StockViewCells, ""
	}
	view := application.StockView(raw)
	switch view {
	case application.StockViewCells, application.StockViewCartons, application.StockViewSummary, application.StockViewHolds, application.StockViewMovements:
		return view, ""
	default:
		return "", "view is not a stock read"
	}
}

func stockSnapshotBody(snapshot application.StockSnapshot) stockReadBody {
	body := stockReadBody{Outcome: "LISTED"}
	for _, cell := range snapshot.Cells {
		item := stockCellBody{
			Warehouse: cell.Warehouse, Location: cell.Location, Goods: cell.Goods, Unit: cell.Unit, State: cell.State,
			Alias: cell.Alias, Batch: cell.Batch, Expiry: cell.Expiry, InboundBatch: cell.InboundBatch,
			InboundAt: cell.InboundAt, Quantity: cell.Quantity, Allocatable: cell.Allocatable, Frozen: cell.Frozen, Available: cell.Available,
		}
		if cell.AgeConfigured {
			days := cell.AgeDays
			item.AgeDays = &days
		} else {
			item.Age = application.StockAgeNotConfigured
		}
		body.Cells = append(body.Cells, item)
	}
	for _, carton := range snapshot.Cartons {
		item := stockCartonBody{Warehouse: carton.Warehouse, Carton: carton.Carton, Location: carton.Location, InboundAt: carton.InboundAt, Held: carton.Held}
		if carton.AgeConfigured {
			days := carton.AgeDays
			item.AgeDays = &days
		} else {
			item.Age = application.StockAgeNotConfigured
		}
		for _, line := range carton.Bill {
			item.Bill = append(item.Bill, stockBillBody{Goods: line.Goods().String(), Quantity: line.Quantity()})
		}
		body.Cartons = append(body.Cartons, item)
	}
	for _, summary := range snapshot.Summary {
		body.Summary = append(body.Summary, stockSummaryBody{
			Warehouse: summary.Warehouse, Goods: summary.Goods, Unit: summary.Unit, State: summary.State,
			OnHand: summary.OnHand, Allocatable: summary.Allocatable, Frozen: summary.Frozen,
			Reserved: summary.Reserved, Locked: summary.Locked, Available: summary.Available,
		})
	}
	for _, hold := range snapshot.Holds {
		body.Holds = append(body.Holds, stockHoldBody{
			Kind: hold.Kind, Warehouse: hold.Warehouse, Goods: hold.Goods, Unit: hold.Unit, State: hold.State,
			Location: hold.Location, Carton: hold.Carton, Quantity: hold.Quantity, Reason: hold.Reason,
			Operator: hold.Operator, FrozenAt: hold.FrozenAt,
		})
	}
	for _, movement := range snapshot.Movements {
		item := stockMovementBody{Warehouse: movement.Warehouse, Reason: movement.Reason, Source: movement.Source, Occurred: movement.Occurred, Posted: movement.Posted}
		for _, line := range movement.Lines {
			item.Lines = append(item.Lines, stockMovementLineBody{Location: line.Location, Goods: line.Goods, Unit: line.Unit, State: line.State, Delta: line.Delta})
		}
		body.Movements = append(body.Movements, item)
	}
	body.Page = snapshot.MovementPage
	return body
}

type stockReadBody struct {
	Outcome   string              `json:"outcome"`
	Cells     []stockCellBody     `json:"cells,omitempty"`
	Cartons   []stockCartonBody   `json:"cartons,omitempty"`
	Summary   []stockSummaryBody  `json:"summary,omitempty"`
	Holds     []stockHoldBody     `json:"holds,omitempty"`
	Movements []stockMovementBody `json:"movements,omitempty"`
	// Page 只在看流水时有：只有那一种看法翻页。
	Page *cataloguepage.Page `json:"page,omitempty"`
}

type stockCellBody struct {
	Warehouse    string    `json:"warehouse"`
	Location     string    `json:"location,omitempty"`
	Goods        string    `json:"goodsCode"`
	Unit         string    `json:"unit"`
	State        string    `json:"stockState"`
	Alias        string    `json:"labelAlias,omitempty"`
	Batch        string    `json:"batchNumber,omitempty"`
	Expiry       string    `json:"expiryDate,omitempty"`
	InboundBatch string    `json:"inboundBatchId"`
	InboundAt    time.Time `json:"inboundAt"`
	Quantity     int64     `json:"quantity"`
	Allocatable  bool      `json:"allocatable"`
	Frozen       int64     `json:"frozen"`
	Available    int64     `json:"available"`
	AgeDays      *int      `json:"ageDays,omitempty"`
	Age          string    `json:"age,omitempty"`
}

type stockCartonBody struct {
	Warehouse string          `json:"warehouse"`
	Carton    string          `json:"cartonNumber"`
	Location  string          `json:"location,omitempty"`
	InboundAt time.Time       `json:"inboundAt"`
	Held      bool            `json:"held"`
	Bill      []stockBillBody `json:"bill,omitempty"`
	AgeDays   *int            `json:"ageDays,omitempty"`
	Age       string          `json:"age,omitempty"`
}

type stockBillBody struct {
	Goods    string `json:"goodsCode"`
	Quantity int64  `json:"quantity"`
}

type stockSummaryBody struct {
	Warehouse   string `json:"warehouse"`
	Goods       string `json:"goodsCode"`
	Unit        string `json:"unit"`
	State       string `json:"stockState"`
	OnHand      int64  `json:"onHand"`
	Allocatable int64  `json:"allocatable"`
	Frozen      int64  `json:"frozen"`
	Reserved    int64  `json:"reserved"`
	Locked      int64  `json:"locked"`
	Available   int64  `json:"available"`
}

type stockHoldBody struct {
	Kind      string    `json:"kind"`
	Warehouse string    `json:"warehouse"`
	Goods     string    `json:"goodsCode,omitempty"`
	Unit      string    `json:"unit,omitempty"`
	State     string    `json:"stockState,omitempty"`
	Location  string    `json:"location,omitempty"`
	Carton    string    `json:"cartonNumber,omitempty"`
	Quantity  int64     `json:"quantity"`
	Reason    string    `json:"reason,omitempty"`
	Operator  string    `json:"operator,omitempty"`
	FrozenAt  time.Time `json:"frozenAt,omitempty"`
}

type stockMovementBody struct {
	Warehouse string                  `json:"warehouse"`
	Reason    string                  `json:"reason"`
	Source    string                  `json:"source,omitempty"`
	Occurred  time.Time               `json:"occurredAt"`
	Posted    time.Time               `json:"postedAt,omitempty"`
	Lines     []stockMovementLineBody `json:"lines,omitempty"`
}

type stockMovementLineBody struct {
	Location string `json:"location,omitempty"`
	Goods    string `json:"goodsCode"`
	Unit     string `json:"unit"`
	State    string `json:"stockState"`
	Delta    int64  `json:"delta"`
}
