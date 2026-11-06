package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// LabelTransactionViews 实现 ports.LabelTransactionViews：面单交易查阅的只读面。
//
// 它与 LabelTransactions 分立，理由同委托那一对：仓储走聚合重建，读面读的是「登记过什么」。
// 本读面直读状态列与快照文档，不经 RehydrateLabelTransaction——一行因为某个字段坏了而重建
// 不出来时，查阅仍应如实交代这笔交易存在，而不是让整页消失。
//
// 过滤只在租户上（ADR-0084 决定七）：客户账户不是交易的维度，覆盖包裹可以跨委托，按客户
// 过滤会把一笔跨客户的交易归给其中一个客户。租户维照 ADR-0003 的隔离边界照常在。
type LabelTransactionViews struct {
	db *bentopg.DB
}

func NewLabelTransactionViews(db *bentopg.DB) (*LabelTransactionViews, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &LabelTransactionViews{db: db}, nil
}

var _ ports.LabelTransactionViews = (*LabelTransactionViews)(nil)

// labelTransactionPages 把 ports.LabelTransactionPages 落到列上。transactionResult 落在存代码的 state 列；q 在交易标识、
// 快照里的渠道账户与覆盖包裹（数组的文本形，包裹标识照原样出现在里面）上做字面包含。
var labelTransactionPages = ports.LabelTransactionPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"establishedAt": "established_at", "transactionId": "label_transaction_id"},
	Identity: []string{"label_transaction_id"},
	Filters:  map[string]string{"transactionResult": "state"},
	Codes:    map[string]func(string) (int64, error){"transactionResult": cataloguepage.Code[domain.LabelTransactionState]},
	Keyword:  []string{"label_transaction_id", "snapshot->>'channelAccount'", "(snapshot->'coveredParcels')::text"},
})

// scannedLabelTransaction 是取页读回的一行。establishedAt 取自列而不是快照：游标位置要与 ORDER BY 用的是同一个值，
// 快照里那份带纳秒、列只到微秒（cataloguepage.Finish 头注）。
type scannedLabelTransaction struct {
	id            string
	state         domain.LabelTransactionState
	establishedAt time.Time
	document      labelTransactionDocument
}

// ListLabelTransactions 按租户上列面单交易，按查询的筛选、q 与排序取游标之后的一页；缺省是建立时刻倒序、同刻按交易标识倒序。
// 继续尝试判断只对本页的交易现算。limit 非正是调用方编程错误，判据同 ListVisible。
func (views *LabelTransactionViews) ListLabelTransactions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.LabelTransactionRecord], error) {
	fail := func(err error) (cataloguepage.Result[ports.LabelTransactionRecord], error) {
		return cataloguepage.Result[ports.LabelTransactionRecord]{}, fmt.Errorf("list label transactions: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := labelTransactionPages.Plan(query, "tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := views.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	const source = "parcel_shipment.label_transaction"
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(source), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, plan.PageSQL("label_transaction_id, state, established_at, snapshot", source), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	var fetched []scannedLabelTransaction
	for rows.Next() {
		var row scannedLabelTransaction
		var state uint8
		var raw []byte
		if err := rows.Scan(&row.id, &state, &row.establishedAt, &raw); err != nil {
			return fail(err)
		}
		if err := json.Unmarshal(raw, &row.document); err != nil {
			return fail(fmt.Errorf("快照不是本适配器写下的形状：%w", err))
		}
		row.state = domain.LabelTransactionState(state)
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	rows.Close()

	page, err := cataloguepage.Finish(query, fetched, limit, total, labelTransactionPosition)
	if err != nil {
		return fail(err)
	}

	covered := make([]string, 0)
	for _, transaction := range page.Rows {
		covered = append(covered, transaction.document.CoveredParcels...)
	}
	judgments, err := views.continuedAttemptJudgments(ctx, tenant, covered)
	if err != nil {
		return fail(err)
	}

	records := make([]ports.LabelTransactionRecord, 0, len(page.Rows))
	for _, transaction := range page.Rows {
		record, err := transaction.document.viewRecord(transaction.id, transaction.state, judgments)
		if err != nil {
			return fail(err)
		}
		records = append(records, record)
	}
	return cataloguepage.Result[ports.LabelTransactionRecord]{Rows: records, Next: page.Next, Total: page.Total}, nil
}

func labelTransactionPosition(row scannedLabelTransaction, sortField string) cataloguepage.Position {
	value := row.id
	if sortField == "establishedAt" {
		value = cataloguepage.FormatInstant(row.establishedAt)
	}
	return cataloguepage.Position{Value: value, Identity: []string{row.id}}
}

// viewRecord 把快照摊成查阅记录，逐件覆盖包裹一行。
//
// 定案由 domain.LabelTransactionState.IsChannelResult 派生，不在这里重写一份「是不是那三格」
// ——那条规则的唯一来源是领域包（ADR-0084 决定四）。
func (document labelTransactionDocument) viewRecord(
	transactionID string,
	state domain.LabelTransactionState,
	judgments continuedAttemptJudgments,
) (ports.LabelTransactionRecord, error) {
	id, err := domain.NewLabelTransactionID(transactionID)
	if err != nil {
		return ports.LabelTransactionRecord{}, err
	}
	record := ports.LabelTransactionRecord{
		TransactionID:          id,
		State:                  state,
		Finalized:              state.IsChannelResult(),
		ChannelAccount:         document.ChannelAccount,
		AccountHolder:          document.AccountHolder,
		ServiceProvider:        document.ServiceProvider,
		SettlementCounterparty: document.SettlementCounterparty,
		Contract:               document.Contract,
		Rate:                   document.Rate,
		ResponsibilityBasis:    document.ResponsibilityBasis,
		EstablishedAt:          document.EstablishedAt,
		SubmittedAt:            document.SubmittedAt,
		ResultObservedAt:       document.ResultObservedAt,
	}
	if document.PriorLink != nil {
		record.PriorTransactionID = document.PriorLink.PriorTransactionID
		record.PriorLinkKind = domain.LabelTransactionLinkKind(document.PriorLink.Kind)
	}

	results := make(map[string]labelTransactionParcelResultDoc, len(document.ParcelResults))
	for _, result := range document.ParcelResults {
		results[result.Parcel] = result
	}
	for _, raw := range document.CoveredParcels {
		parcel, err := domain.NewDeclaredParcelID(raw)
		if err != nil {
			return ports.LabelTransactionRecord{}, err
		}
		judgment, err := judgments.judge(parcel)
		if err != nil {
			return ports.LabelTransactionRecord{}, err
		}
		row := ports.LabelTransactionParcelRow{
			Parcel:                  parcel,
			FollowUpKinds:           document.followUpKindsFor(raw),
			ContinuedAttemptOpen:    judgment.open,
			ContinuedAttemptDecided: judgment.decided,
		}
		if result, present := results[raw]; present {
			row.HasResult = true
			row.Accepted = result.Accepted
			row.Identifier = result.Identifier
			row.Reason = result.Reason
		}
		record.Parcels = append(record.Parcels, row)
	}

	for _, raw := range document.LabelDocuments {
		row := ports.LabelTransactionDocumentRow{
			Role:        raw.Role,
			Format:      raw.Format,
			Granularity: domain.LabelDocumentGranularity(raw.Granularity),
			Digest:      raw.Digest,
			// 本体在不在按定位符派生，与聚合上 BodyStored 同一条规则；读面不另存一格。
			BodyStored: raw.Locator != "",
			ObservedAt: raw.ObservedAt,
		}
		for _, rawParcel := range raw.Parcels {
			parcel, err := domain.NewDeclaredParcelID(rawParcel)
			if err != nil {
				return ports.LabelTransactionRecord{}, err
			}
			row.CoveredParcels = append(row.CoveredParcels, parcel)
		}
		record.Documents = append(record.Documents, row)
	}
	return record, nil
}

// followUpKindsFor 收集作用到某件包裹的后续动作种类，按追加顺序。整笔范围的动作作用于每一
// 件覆盖包裹，因此也计入——CONTEXT 说它们「针对明确交易范围或包裹范围形成」，整笔就是把
// 范围划到了全部覆盖包裹上。
func (document labelTransactionDocument) followUpKindsFor(parcel string) []domain.FollowUpActionKind {
	var kinds []domain.FollowUpActionKind
	for _, action := range document.FollowUpActions {
		if len(action.Parcels) == 0 || containsParcel(action.Parcels, parcel) {
			kinds = append(kinds, domain.FollowUpActionKind(action.Kind))
		}
	}
	return kinds
}

func containsParcel(parcels []string, parcel string) bool {
	for _, candidate := range parcels {
		if candidate == parcel {
			return true
		}
	}
	return false
}

// continuedAttemptJudgments 是一批包裹的继续尝试判断输入：各自的决定登记册（没开过册的按空册）
// 与「当前有效终局在不在」。判断本身不在这里算——它只能由聚合的 Judge 现算（CONTEXT「只由有效的
// 关闭、重开决定及当前有效终局结果派生」），读面绕过重建门自己看快照就是把那条规则抄第二份。
type continuedAttemptJudgments struct {
	tenant    domain.TenantID
	registers map[string]domain.ContinuedAttemptRegister
	finals    map[string]bool
}

type continuedAttemptJudgment struct {
	open    bool
	decided bool
}

// judge 对一件包裹现算。没开过册的包裹按一本空册判：空册与非空册同样合法（登记册没有生命周期
// 状态），空册上 Judge 的答案只取决于当前有效终局在不在——这正是登记册落地前那句「恒答开放」
// 的规则本体，现在它拿的是真输入。
//
// 开不出空册（租户或包裹不成立）时报错而不是答一个零值：零值读出来是「受控关闭、无人决定」，
// 一格看着合法的答案会把一行坏数据藏起来。
func (judgments continuedAttemptJudgments) judge(parcel domain.DeclaredParcelID) (continuedAttemptJudgment, error) {
	register, opened := judgments.registers[parcel.String()]
	if !opened {
		empty, err := domain.OpenContinuedAttemptRegister(judgments.tenant, parcel)
		if err != nil {
			return continuedAttemptJudgment{}, fmt.Errorf("continued attempt judgment for %s: %w", parcel, err)
		}
		register = empty
	}
	return continuedAttemptJudgment{
		open:    register.Judge(judgments.finals[parcel.String()]) == domain.ContinuedAttemptOpen,
		decided: register.HasAnyDecision(),
	}, nil
}

// continuedAttemptJudgments 一次取回一批包裹的登记册与当前有效终局在不在。两张表都按（租户 +
// 包裹）取，租户维照 ADR-0003 的隔离边界；终局只看 is_current 那一行——当前有效终局属本上下文的
// ParcelFinalOutcome，读面只问它在不在，不把它抄进登记册（票 label-channel/10 接手点那条告诫）。
func (views *LabelTransactionViews) continuedAttemptJudgments(
	ctx context.Context,
	tenant domain.TenantID,
	parcels []string,
) (continuedAttemptJudgments, error) {
	judgments := continuedAttemptJudgments{
		tenant:    tenant,
		registers: map[string]domain.ContinuedAttemptRegister{},
		finals:    map[string]bool{},
	}
	if len(parcels) == 0 {
		return judgments, nil
	}
	querier, err := views.db.ReadExecutor(ctx)
	if err != nil {
		return judgments, err
	}

	registerRows, err := querier.Query(ctx,
		`SELECT parcel_id, revision, snapshot
		   FROM parcel_shipment.continued_attempt_register
		  WHERE tenant_id = $1 AND parcel_id = ANY($2)`,
		tenant.String(), parcels)
	if err != nil {
		return judgments, fmt.Errorf("continued attempt registers: %w", err)
	}
	defer registerRows.Close()
	for registerRows.Next() {
		var parcel string
		var revision int64
		var raw []byte
		if err := registerRows.Scan(&parcel, &revision, &raw); err != nil {
			return judgments, fmt.Errorf("continued attempt registers: %w", err)
		}
		parcelID, err := domain.NewDeclaredParcelID(parcel)
		if err != nil {
			return judgments, fmt.Errorf("continued attempt registers: %w", err)
		}
		register, err := rehydrateContinuedAttemptRegister(revision, tenant, parcelID, raw)
		if err != nil {
			return judgments, fmt.Errorf("continued attempt registers: %w", err)
		}
		judgments.registers[parcel] = register
	}
	if err := registerRows.Err(); err != nil {
		return judgments, fmt.Errorf("continued attempt registers: %w", err)
	}

	finalRows, err := querier.Query(ctx,
		`SELECT parcel_id
		   FROM parcel_shipment.final_outcome
		  WHERE tenant_id = $1 AND parcel_id = ANY($2) AND is_current`,
		tenant.String(), parcels)
	if err != nil {
		return judgments, fmt.Errorf("current final outcomes: %w", err)
	}
	defer finalRows.Close()
	for finalRows.Next() {
		var parcel string
		if err := finalRows.Scan(&parcel); err != nil {
			return judgments, fmt.Errorf("current final outcomes: %w", err)
		}
		judgments.finals[parcel] = true
	}
	if err := finalRows.Err(); err != nil {
		return judgments, fmt.Errorf("current final outcomes: %w", err)
	}
	return judgments, nil
}
