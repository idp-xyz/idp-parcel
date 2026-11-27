package application

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// CumulativeReceived 是累计实收的一行。商品入库按商品折成基本单位；整箱入库按箱数，一箱记 1，不把箱单折成件。
// 没收过是 0，这个 0 是把已入账的行加出来的。
type CumulativeReceived struct {
	Goods    domain.GoodsCode
	Carton   domain.CartonNumber
	Quantity int64
}

// PreAdviceReading 是一份预报加上逐次收货、累计实收、完结与少收，以及各份差异的结论。没有完结时 Closure 为空。
// 少收不是某一次收货事实的差异，不在逐次收货里再编一行。收货形成的差异与完结形成的少收各有自己的标识，结论按标识挂。
// UnclaimedArrival 为真表示有待认领到货指着这份预报。为假时读口不出现这一格，也不写 false。
type PreAdviceReading struct {
	Advice           domain.PreAdvice
	Receipts         []domain.RecordedReceipt
	Cumulative       []CumulativeReceived
	Closure          *domain.PreAdviceClosure
	Conclusions      []DiscrepancyConclusionView
	UnclaimedArrival bool
}

// QueryPreAdviceHandler 是入库预报的查阅用例：分页列表、按（货主，预报标识）读一份、按单据号在本租户逐字找。
// 读口带上已落册的收货与完结：逐次收货、累计实收、完结与少收，以及每份差异的现行结论。
// 有待认领到货指着这份预报时只多一格有无，不带那件到货的内容、标识与数量。
// 转成待认领的那条收货不放进逐次收货：放进去就是把到货内容带出来。
// 商品入库的累计实收按商品折成基本单位；整箱入库按箱数计。别的货主的预报与没登记的预报仍答「没有」。
//
// 读不动原样上抛，不折成「没有」，也不把读失败答成累计 0。
type QueryPreAdviceHandler struct {
	preAdvices  ports.PreAdviceRead
	receipts    ports.ReceiptRead
	goods       ports.StockGoodsLookup
	closures    ports.PreAdviceClosureRead
	conclusions ports.DiscrepancyConclusionRead
	arrivals    ports.UnclaimedArrivalRead
}

func NewQueryPreAdviceHandler(
	preAdvices ports.PreAdviceRead,
	receipts ports.ReceiptRead,
	goods ports.StockGoodsLookup,
	closures ports.PreAdviceClosureRead,
	conclusions ports.DiscrepancyConclusionRead,
	arrivals ports.UnclaimedArrivalRead,
) *QueryPreAdviceHandler {
	return &QueryPreAdviceHandler{
		preAdvices: preAdvices, receipts: receipts, goods: goods, closures: closures, conclusions: conclusions,
		arrivals: arrivals,
	}
}

// ListPreAdvices 答一页一个货主名下的预报。每一行带自己的逐次收货和累计实收。
func (handler *QueryPreAdviceHandler) ListPreAdvices(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[PreAdviceReading], error) {
	page, err := handler.preAdvices.ListPreAdvices(ctx, tenant, owner, limit, query)
	if err != nil {
		return cataloguepage.Result[PreAdviceReading]{}, fmt.Errorf("list inbound pre-advices: %w", err)
	}
	rows, err := handler.decorate(ctx, tenant, page.Rows)
	if err != nil {
		return cataloguepage.Result[PreAdviceReading]{}, fmt.Errorf("list inbound pre-advices: %w", err)
	}
	return cataloguepage.Result[PreAdviceReading]{Rows: rows, Next: page.Next, Total: page.Total, Facets: page.Facets}, nil
}

// PreAdviceForOwner 按（货主，预报标识）读一份。别的货主的同标识与没登记同形，第二个返回值为假。
func (handler *QueryPreAdviceHandler) PreAdviceForOwner(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	id domain.PreAdviceID,
) (PreAdviceReading, bool, error) {
	advice, found, err := handler.preAdvices.PreAdviceForOwner(ctx, tenant, owner, id)
	if err != nil {
		return PreAdviceReading{}, false, fmt.Errorf("get inbound pre-advice: %w", err)
	}
	if !found {
		return PreAdviceReading{}, false, nil
	}
	reading, err := handler.one(ctx, tenant, advice)
	if err != nil {
		return PreAdviceReading{}, false, fmt.Errorf("get inbound pre-advice: %w", err)
	}
	return reading, true, nil
}

// PreAdviceByDocumentNumber 按单据号在本租户逐字找。没有单据号的预报不出现在这一口。找不到时第二个返回值为假。
func (handler *QueryPreAdviceHandler) PreAdviceByDocumentNumber(
	ctx context.Context,
	tenant domain.TenantID,
	number domain.DocumentNumber,
) (PreAdviceReading, bool, error) {
	advice, found, err := handler.preAdvices.PreAdviceByDocumentNumber(ctx, tenant, number)
	if err != nil {
		return PreAdviceReading{}, false, fmt.Errorf("find inbound pre-advice by document number: %w", err)
	}
	if !found {
		return PreAdviceReading{}, false, nil
	}
	reading, err := handler.one(ctx, tenant, advice)
	if err != nil {
		return PreAdviceReading{}, false, fmt.Errorf("find inbound pre-advice by document number: %w", err)
	}
	return reading, true, nil
}

// PreAdviceByCode 给作业台认单：单据号优先，对不上再按预报标识。找不到时第二个返回值为假。
func (handler *QueryPreAdviceHandler) PreAdviceByCode(
	ctx context.Context,
	tenant domain.TenantID,
	code string,
) (PreAdviceReading, bool, error) {
	advice, found, err := handler.preAdvices.PreAdviceByCode(ctx, tenant, code)
	if err != nil {
		return PreAdviceReading{}, false, fmt.Errorf("find inbound pre-advice by scan code: %w", err)
	}
	if !found {
		return PreAdviceReading{}, false, nil
	}
	reading, err := handler.one(ctx, tenant, advice)
	if err != nil {
		return PreAdviceReading{}, false, fmt.Errorf("find inbound pre-advice by scan code: %w", err)
	}
	return reading, true, nil
}

func (handler *QueryPreAdviceHandler) one(ctx context.Context, tenant domain.TenantID, advice domain.PreAdvice) (PreAdviceReading, error) {
	rows, err := handler.decorate(ctx, tenant, []domain.PreAdvice{advice})
	if err != nil {
		return PreAdviceReading{}, err
	}
	return rows[0], nil
}

func (handler *QueryPreAdviceHandler) decorate(
	ctx context.Context,
	tenant domain.TenantID,
	advices []domain.PreAdvice,
) ([]PreAdviceReading, error) {
	readings := make([]PreAdviceReading, len(advices))
	if len(advices) == 0 {
		return readings, nil
	}
	ids := make([]domain.PreAdviceID, len(advices))
	for index, advice := range advices {
		ids[index] = advice.ID()
	}
	receipts, err := handler.receipts.ListByPreAdvices(ctx, tenant, ids)
	if err != nil {
		return nil, err
	}
	pointed, err := handler.preAdvicesWithUnclaimedArrival(ctx, tenant, ids)
	if err != nil {
		return nil, err
	}
	closures, err := handler.closures.ListByPreAdvices(ctx, tenant, ids)
	if err != nil {
		return nil, err
	}
	grouped := map[string][]domain.RecordedReceipt{}
	for _, receipt := range receipts {
		grouped[receipt.PreAdvice().String()] = append(grouped[receipt.PreAdvice().String()], receipt)
	}
	closed := map[string]domain.PreAdviceClosure{}
	for _, closure := range closures {
		closed[closure.PreAdvice().String()] = closure
	}
	conclusionIDs := discrepancyIDsOf(receipts, closures)
	conclusionRows, err := handler.conclusions.ListByDiscrepancies(ctx, tenant, conclusionIDs)
	if err != nil {
		return nil, err
	}
	conclusionViews, err := GroupDiscrepancyConclusions(conclusionIDs, conclusionRows)
	if err != nil {
		return nil, err
	}
	byDiscrepancy := map[string]DiscrepancyConclusionView{}
	for _, view := range conclusionViews {
		byDiscrepancy[view.ID().String()] = view
	}
	cache := map[string]domain.GoodsRevision{}
	for index, advice := range advices {
		list := grouped[advice.ID().String()]
		if list == nil {
			list = []domain.RecordedReceipt{}
		}
		var cumulative []CumulativeReceived
		if advice.Inbound() == domain.InboundByCarton {
			cumulative = cartonCumulative(advice, list)
		} else {
			cumulative, err = handler.cumulative(ctx, advice, list, cache)
		}
		if err != nil {
			return nil, err
		}
		shown := withoutDivertedReceipts(list)
		readings[index] = PreAdviceReading{
			Advice: advice, Receipts: shown, Cumulative: cumulative,
			UnclaimedArrival: pointed[advice.ID().String()],
		}
		var own []DiscrepancyConclusionView
		for _, receipt := range shown {
			if id, has := receipt.Discrepancy(); has {
				if view, ok := byDiscrepancy[id.String()]; ok {
					own = append(own, view)
				}
			}
		}
		if recorded, ok := closed[advice.ID().String()]; ok {
			copy := recorded
			readings[index].Closure = &copy
			if id, has := recorded.Discrepancy(); has {
				if view, known := byDiscrepancy[id.String()]; known {
					own = append(own, view)
				}
			}
		}
		readings[index].Conclusions = own
	}
	return readings, nil
}

func (handler *QueryPreAdviceHandler) preAdvicesWithUnclaimedArrival(
	ctx context.Context,
	tenant domain.TenantID,
	ids []domain.PreAdviceID,
) (map[string]bool, error) {
	if handler.arrivals == nil {
		return nil, errors.New("待认领到货没有接上")
	}
	found, err := handler.arrivals.PreAdviceIDsWithUnclaimedArrival(ctx, tenant, ids)
	if err != nil {
		return nil, err
	}
	pointed := map[string]bool{}
	for _, id := range found {
		pointed[id.String()] = true
	}
	return pointed, nil
}

// withoutDivertedReceipts 把已转待认领的收货从逐次收货里拿掉。
// 那些行是到货的内容；预报读口只标有无，不把内容、标识和数量带出来。
func withoutDivertedReceipts(receipts []domain.RecordedReceipt) []domain.RecordedReceipt {
	shown := make([]domain.RecordedReceipt, 0, len(receipts))
	for _, receipt := range receipts {
		if receipt.Outcome() == domain.RecordedReceiptDivertedToUnclaimed {
			continue
		}
		shown = append(shown, receipt)
	}
	return shown
}

func discrepancyIDsOf(receipts []domain.RecordedReceipt, closures []domain.PreAdviceClosure) []domain.ReceiptDiscrepancyID {
	var ids []domain.ReceiptDiscrepancyID
	seen := map[string]bool{}
	add := func(id domain.ReceiptDiscrepancyID, has bool) {
		if !has || seen[id.String()] {
			return
		}
		seen[id.String()] = true
		ids = append(ids, id)
	}
	for _, receipt := range receipts {
		id, has := receipt.Discrepancy()
		add(id, has)
	}
	for _, closure := range closures {
		id, has := closure.Discrepancy()
		add(id, has)
	}
	return ids
}

func (handler *QueryPreAdviceHandler) cumulative(
	ctx context.Context,
	advice domain.PreAdvice,
	receipts []domain.RecordedReceipt,
	cache map[string]domain.GoodsRevision,
) ([]CumulativeReceived, error) {
	posted, err := postedBase(receipts, func(goods domain.GoodsCode, unit domain.GoodsUnitName) (int64, error) {
		return handler.factor(ctx, advice, goods, unit, cache)
	})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	rows := make([]CumulativeReceived, 0)
	for _, line := range advice.GoodsLines() {
		code := line.Goods().String()
		if seen[code] {
			continue
		}
		seen[code] = true
		rows = append(rows, CumulativeReceived{Goods: line.Goods(), Quantity: posted[code]})
	}
	extra := make([]string, 0)
	for code, quantity := range posted {
		if !seen[code] && quantity != 0 {
			extra = append(extra, code)
		}
	}
	slices.Sort(extra)
	for _, code := range extra {
		goods, err := domain.NewGoodsCode(code)
		if err != nil {
			return nil, err
		}
		rows = append(rows, CumulativeReceived{Goods: goods, Quantity: posted[code]})
	}
	return rows, nil
}

// cartonCumulative 按货主箱号计已入账的箱数。箱单里的件数不进来。预报列了还没入账的箱记 0。
func cartonCumulative(advice domain.PreAdvice, receipts []domain.RecordedReceipt) []CumulativeReceived {
	posted := map[string]int64{}
	for _, receipt := range receipts {
		if receipt.Outcome() != domain.RecordedReceiptConfirmed {
			continue
		}
		for _, line := range receipt.CartonLines() {
			if line.Result() != domain.ReceiptLinePosted {
				continue
			}
			posted[line.Number().String()]++
		}
	}
	seen := map[string]bool{}
	rows := make([]CumulativeReceived, 0)
	for _, carton := range advice.Cartons() {
		number := carton.Number().String()
		if seen[number] {
			continue
		}
		seen[number] = true
		rows = append(rows, CumulativeReceived{Carton: carton.Number(), Quantity: posted[number]})
	}
	extra := make([]string, 0)
	for number, quantity := range posted {
		if !seen[number] && quantity != 0 {
			extra = append(extra, number)
		}
	}
	slices.Sort(extra)
	for _, number := range extra {
		carton, err := domain.NewCartonNumber(number)
		if err != nil {
			continue
		}
		rows = append(rows, CumulativeReceived{Carton: carton, Quantity: posted[number]})
	}
	return rows
}

func (handler *QueryPreAdviceHandler) factor(
	ctx context.Context,
	advice domain.PreAdvice,
	goods domain.GoodsCode,
	unit domain.GoodsUnitName,
	cache map[string]domain.GoodsRevision,
) (int64, error) {
	key := advice.Owner().String() + "\x1f" + goods.String()
	revision, ok := cache[key]
	if !ok {
		loaded, found, err := handler.goods.LoadLatestGoods(ctx, advice.Tenant(), advice.Owner(), goods)
		if err != nil {
			return 0, err
		}
		if !found {
			return 0, fmt.Errorf("商品 %s 没有当前修订", goods)
		}
		revision = loaded
		cache[key] = revision
	}
	value, ok := unitFactor(revision, unit)
	if !ok {
		return 0, fmt.Errorf("商品 %s 的计量单位 %s 没有折合数", goods, unit)
	}
	return value, nil
}
