package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
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

// ListLabelTransactions 按租户取回最近建立的面单交易，新的在前；同刻并列时按交易标识倒序，
// 保证分页可重复。limit 非正是调用方编程错误，判据同 ListVisible。
func (views *LabelTransactionViews) ListLabelTransactions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.LabelTransactionRecord, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list label transactions: limit must be positive, got %d", limit)
	}
	querier, err := views.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list label transactions: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT label_transaction_id, state, snapshot
		   FROM parcel_shipment.label_transaction
		  WHERE tenant_id = $1
		  ORDER BY established_at DESC, label_transaction_id DESC
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list label transactions: %w", err)
	}
	defer rows.Close()

	type scannedTransaction struct {
		id       string
		state    domain.LabelTransactionState
		document labelTransactionDocument
	}
	var scanned []scannedTransaction
	for rows.Next() {
		var transactionID string
		var state uint8
		var raw []byte
		if err := rows.Scan(&transactionID, &state, &raw); err != nil {
			return nil, fmt.Errorf("list label transactions: %w", err)
		}
		var document labelTransactionDocument
		if err := json.Unmarshal(raw, &document); err != nil {
			return nil, fmt.Errorf("list label transactions: 快照不是本适配器写下的形状：%w", err)
		}
		scanned = append(scanned, scannedTransaction{id: transactionID, state: domain.LabelTransactionState(state), document: document})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list label transactions: %w", err)
	}
	rows.Close()

	covered := make([]string, 0)
	for _, transaction := range scanned {
		covered = append(covered, transaction.document.CoveredParcels...)
	}
	judgments, err := views.continuedAttemptJudgments(ctx, tenant, covered)
	if err != nil {
		return nil, fmt.Errorf("list label transactions: %w", err)
	}

	records := make([]ports.LabelTransactionRecord, 0, len(scanned))
	for _, transaction := range scanned {
		record, err := transaction.document.viewRecord(transaction.id, transaction.state, judgments)
		if err != nil {
			return nil, fmt.Errorf("list label transactions: %w", err)
		}
		records = append(records, record)
	}
	return records, nil
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
