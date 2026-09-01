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

	var records []ports.LabelTransactionRecord
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
		record, err := document.viewRecord(transactionID, domain.LabelTransactionState(state))
		if err != nil {
			return nil, fmt.Errorf("list label transactions: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list label transactions: %w", err)
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
		row := ports.LabelTransactionParcelRow{
			Parcel:               parcel,
			FollowUpKinds:        document.followUpKindsFor(raw),
			ContinuedAttemptOpen: deriveContinuedAttemptOpen(),
		}
		if result, present := results[raw]; present {
			row.HasResult = true
			row.Accepted = result.Accepted
			row.Identifier = result.Identifier
			row.Reason = result.Reason
		}
		record.Parcels = append(record.Parcels, row)
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

// deriveContinuedAttemptOpen 把 CONTEXT 的包裹级继续尝试规则——「只由有效的关闭、重开决定
// 及当前有效终局结果派生」——应用在**当前真实的决定历史**上。
//
// 那段历史此刻是空的：继续尝试决定登记册尚未落地（ADR-0084 决定六判为另票，重启条件是写面
// 裁决或渠道墙任一先到），全仓没有任何地方能形成一条关闭或重开决定。无生效关闭且无有效终局
// 即开放，于是本函数恒答开放。
//
// **它不是默认值，也不该被读成「已核对过关闭册」。** 两者的区别在读面上要看得见：页头必须
// 注明这条派生依据。登记册落地后本函数改为联查那册与当前终局，规则一字不变——正因为规则写
// 在这里而不是写成一个常量 true，那一天要改的只有输入。
func deriveContinuedAttemptOpen() bool {
	const effectiveClosurePresent = false
	const currentFinalOutcomePresent = false
	return !effectiveClosurePresent && !currentFinalOutcomePresent
}
