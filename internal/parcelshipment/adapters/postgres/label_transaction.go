package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// LabelTransactions 实现 ports.LabelTransactionRepository：以（租户 + 面单交易标识）为键的
// 全聚合快照持久化（ADR-0084 决定八）。
//
// 快照文档的形状照 RehydrateLabelTransactionSpec 设计，读回时逐字段过领域构造函数再进
// RehydrateLabelTransaction——库里一行坏数据在这两道门上暴露，不会变成一个看起来合法的聚合
// （ADR-0028）。重建门对状态集合里的每一格都开：面单交易没有「表达不出的状态」，未定案的行在渠道
// 墙未降前本来就是常态，把它挡掉登记册就只剩已定案的交易。
//
// 本适配器今天没有生产写入方。写入方是渠道适配器，而首发基线明写「独立面单渠道服务不进入
// 首发生产」——机制先立起来，墙降那天写编排对着的不是一张裸表。
type LabelTransactions struct {
	db *bentopg.DB
}

func NewLabelTransactions(db *bentopg.DB) (*LabelTransactions, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &LabelTransactions{db: db}, nil
}

var _ ports.LabelTransactionRepository = (*LabelTransactions)(nil)

// FindByID 按（租户 + 交易标识）取回聚合。否定结果只回 false，不区分「不存在」与「属于
// 另一个租户」——区分它们等于泄露其他租户下是否存在该交易号。
func (repository *LabelTransactions) FindByID(
	ctx context.Context,
	tenant domain.TenantID,
	transactionID domain.LabelTransactionID,
) (domain.LabelTransaction, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.LabelTransaction{}, false, fmt.Errorf("find label transaction: %w", err)
	}

	var revision int64
	var state uint8
	var raw []byte
	err = querier.QueryRow(ctx,
		`SELECT revision, state, snapshot
		   FROM parcel_shipment.label_transaction
		  WHERE tenant_id = $1
		    AND label_transaction_id = $2`,
		tenant.String(),
		transactionID.String(),
	).Scan(&revision, &state, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.LabelTransaction{}, false, nil
	}
	if err != nil {
		return domain.LabelTransaction{}, false, fmt.Errorf("find label transaction: %w", err)
	}

	var document labelTransactionDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return domain.LabelTransaction{}, false, fmt.Errorf("find label transaction: 快照不是本适配器写下的形状：%w", err)
	}
	spec, err := document.rehydrationSpec(revision, tenant, transactionID, domain.LabelTransactionState(state))
	if err != nil {
		return domain.LabelTransaction{}, false, fmt.Errorf("find label transaction: %w", err)
	}
	transaction, err := domain.RehydrateLabelTransaction(spec)
	if err != nil {
		return domain.LabelTransaction{}, false, fmt.Errorf("find label transaction: %w", err)
	}
	return transaction, true, nil
}

// Insert 建立只发生一次：revision 从 1 起写入，主键冲突译成`已存在`——那是业务答案（另一方
// 先建了这笔交易），不是技术故障（ADR-0031）。用 ON CONFLICT DO NOTHING 而不是捕 23505：
// 撞键的 INSERT 会把整个事务打进中止态，而拿到`已存在`的编排还要在同一事务里继续读原交易。
func (repository *LabelTransactions) Insert(
	ctx context.Context,
	transaction domain.LabelTransaction,
) (ports.LabelTransactionInsertOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.LabelTransactionInsertOutcomeInvalid, fmt.Errorf("insert label transaction: %w", err)
	}

	raw, err := json.Marshal(labelTransactionDocumentOf(transaction))
	if err != nil {
		return ports.LabelTransactionInsertOutcomeInvalid, fmt.Errorf("insert label transaction: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_shipment.label_transaction
			(tenant_id, label_transaction_id, revision, state, snapshot, established_at)
		 VALUES ($1, $2, 1, $3, $4, $5)
		 ON CONFLICT DO NOTHING`,
		transaction.Tenant().String(),
		transaction.ID().String(),
		uint8(transaction.State()),
		raw,
		transaction.EstablishedAt().UTC(),
	)
	if err != nil {
		return ports.LabelTransactionInsertOutcomeInvalid, fmt.Errorf("insert label transaction: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.LabelTransactionAlreadyExists, nil
	}
	return ports.LabelTransactionInserted, nil
}

// Save 在既有交易上推进。预期版本由聚合自己携带（转移一律不动它），UPDATE 的 WHERE 带上它
// 并加一：零行命中即`版本冲突`——抢先那一方已经落库，本方要重读再重放。
//
// established_at 不在 SET 里：它是出生属性，聚合上没有任何改写它的路径，写进 UPDATE 只会给
// 「改一改出生时间」留一道适配器侧的门。
func (repository *LabelTransactions) Save(
	ctx context.Context,
	transaction domain.LabelTransaction,
) (ports.LabelTransactionSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.LabelTransactionSaveOutcomeInvalid, fmt.Errorf("save label transaction: %w", err)
	}

	raw, err := json.Marshal(labelTransactionDocumentOf(transaction))
	if err != nil {
		return ports.LabelTransactionSaveOutcomeInvalid, fmt.Errorf("save label transaction: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE parcel_shipment.label_transaction
		    SET revision = $3 + 1,
		        state = $4,
		        snapshot = $5,
		        saved_at = now()
		  WHERE tenant_id = $1
		    AND label_transaction_id = $2
		    AND revision = $3`,
		transaction.Tenant().String(),
		transaction.ID().String(),
		transaction.Revision(),
		uint8(transaction.State()),
		raw,
	)
	if err != nil {
		return ports.LabelTransactionSaveOutcomeInvalid, fmt.Errorf("save label transaction: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.LabelTransactionRevisionConflict, nil
	}
	return ports.LabelTransactionSaved, nil
}

// labelTransactionDocument 是快照列里的文档形状——RehydrateLabelTransactionSpec 的 JSON
// 表达。租户、交易标识、revision 与 state 刻意不进文档：四者都是列（键定位、版本挡并发、
// 状态供巡检与过滤），文档里再存一份就是第二个来源，改列不改文档的一次写入会让两处各说
// 各话。established_at 两处都有是另一回事：列上那份是排序投影，与文档同一条 SQL 写下。
type labelTransactionDocument struct {
	CoveredParcels         []string                            `json:"coveredParcels"`
	ChannelAccount         string                              `json:"channelAccount"`
	AccountHolder          string                              `json:"accountHolder"`
	ServiceProvider        string                              `json:"serviceProvider"`
	SettlementCounterparty string                              `json:"settlementCounterparty"`
	Contract               string                              `json:"contract"`
	Rate                   string                              `json:"rate"`
	ResponsibilityBasis    string                              `json:"responsibilityBasis"`
	EstablishedAt          time.Time                           `json:"establishedAt"`
	PriorLink              *labelTransactionLinkDocument       `json:"priorLink,omitempty"`
	SubmittedAt            time.Time                           `json:"submittedAt"`
	ParcelResults          []labelTransactionParcelResultDoc   `json:"parcelResults,omitempty"`
	ResultObservedAt       time.Time                           `json:"resultObservedAt"`
	FollowUpActions        []labelTransactionFollowUpActionDoc `json:"followUpActions,omitempty"`
}

type labelTransactionLinkDocument struct {
	PriorTransactionID string `json:"priorTransactionId"`
	Kind               uint8  `json:"kind"`
}

type labelTransactionParcelResultDoc struct {
	Parcel     string `json:"parcel"`
	Accepted   bool   `json:"accepted"`
	Identifier string `json:"identifier,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type labelTransactionFollowUpActionDoc struct {
	Kind       uint8     `json:"kind"`
	Parcels    []string  `json:"parcels,omitempty"`
	Reason     string    `json:"reason"`
	OccurredAt time.Time `json:"occurredAt"`
}

// labelTransactionDocumentOf 从聚合的公开访问器摊出文档。只读不判断：校验在读回那一侧的
// RehydrateLabelTransaction 上。
func labelTransactionDocumentOf(transaction domain.LabelTransaction) labelTransactionDocument {
	document := labelTransactionDocument{
		ChannelAccount:         transaction.ChannelAccount().String(),
		AccountHolder:          transaction.AccountHolder().String(),
		ServiceProvider:        transaction.ServiceProvider().String(),
		SettlementCounterparty: transaction.SettlementCounterparty().String(),
		Contract:               transaction.Contract().String(),
		Rate:                   transaction.Rate().String(),
		ResponsibilityBasis:    transaction.ResponsibilityBasis().String(),
		EstablishedAt:          transaction.EstablishedAt().UTC(),
		SubmittedAt:            transaction.SubmittedAt().UTC(),
		ResultObservedAt:       transaction.ResultObservedAt().UTC(),
	}
	for _, parcel := range transaction.CoveredParcels() {
		document.CoveredParcels = append(document.CoveredParcels, parcel.String())
	}
	if link, established := transaction.PriorLink(); established {
		document.PriorLink = &labelTransactionLinkDocument{
			PriorTransactionID: link.PriorTransactionID().String(),
			Kind:               uint8(link.Kind()),
		}
	}
	for _, result := range transaction.ParcelResults() {
		document.ParcelResults = append(document.ParcelResults, labelTransactionParcelResultDoc{
			Parcel:     result.Parcel().String(),
			Accepted:   result.Accepted(),
			Identifier: result.Identifier().String(),
			Reason:     result.Reason().String(),
		})
	}
	for _, action := range transaction.FollowUpActions() {
		actionDoc := labelTransactionFollowUpActionDoc{
			Kind:       uint8(action.Kind()),
			Reason:     action.Reason().String(),
			OccurredAt: action.OccurredAt().UTC(),
		}
		for _, parcel := range action.Parcels() {
			actionDoc.Parcels = append(actionDoc.Parcels, parcel.String())
		}
		document.FollowUpActions = append(document.FollowUpActions, actionDoc)
	}
	return document
}

// rehydrationSpec 把文档逐字段过领域构造函数拼回重建规格。任何一个构造函数拒绝都说明这一行
// 不是本适配器写下的形状（或写它的版本有 bug），错误如实上抛。
func (document labelTransactionDocument) rehydrationSpec(
	revision int64,
	tenant domain.TenantID,
	transactionID domain.LabelTransactionID,
	state domain.LabelTransactionState,
) (domain.RehydrateLabelTransactionSpec, error) {
	channelAccount, err := domain.NewChannelAccountReference(document.ChannelAccount)
	if err != nil {
		return domain.RehydrateLabelTransactionSpec{}, err
	}
	accountHolder, err := domain.NewChannelAccountHolderReference(document.AccountHolder)
	if err != nil {
		return domain.RehydrateLabelTransactionSpec{}, err
	}
	serviceProvider, err := domain.NewChannelServiceProviderReference(document.ServiceProvider)
	if err != nil {
		return domain.RehydrateLabelTransactionSpec{}, err
	}
	settlementCounterparty, err := domain.NewSettlementCounterpartyReference(document.SettlementCounterparty)
	if err != nil {
		return domain.RehydrateLabelTransactionSpec{}, err
	}
	contract, err := domain.NewChannelContractReference(document.Contract)
	if err != nil {
		return domain.RehydrateLabelTransactionSpec{}, err
	}
	rate, err := domain.NewChannelRateReference(document.Rate)
	if err != nil {
		return domain.RehydrateLabelTransactionSpec{}, err
	}
	responsibilityBasis, err := domain.NewResponsibilityBasisSnapshotReference(document.ResponsibilityBasis)
	if err != nil {
		return domain.RehydrateLabelTransactionSpec{}, err
	}

	spec := domain.RehydrateLabelTransactionSpec{
		Revision:               revision,
		Tenant:                 tenant,
		ID:                     transactionID,
		ChannelAccount:         channelAccount,
		AccountHolder:          accountHolder,
		ServiceProvider:        serviceProvider,
		SettlementCounterparty: settlementCounterparty,
		Contract:               contract,
		Rate:                   rate,
		ResponsibilityBasis:    responsibilityBasis,
		EstablishedAt:          document.EstablishedAt,
		SubmittedAt:            document.SubmittedAt,
		State:                  state,
		ResultObservedAt:       document.ResultObservedAt,
	}
	for _, raw := range document.CoveredParcels {
		parcel, err := domain.NewDeclaredParcelID(raw)
		if err != nil {
			return domain.RehydrateLabelTransactionSpec{}, err
		}
		spec.CoveredParcels = append(spec.CoveredParcels, parcel)
	}
	if document.PriorLink != nil {
		priorID, err := domain.NewLabelTransactionID(document.PriorLink.PriorTransactionID)
		if err != nil {
			return domain.RehydrateLabelTransactionSpec{}, err
		}
		spec.PriorTransactionID = priorID
		spec.PriorLinkKind = domain.LabelTransactionLinkKind(document.PriorLink.Kind)
	}
	for _, raw := range document.ParcelResults {
		result, err := raw.spec()
		if err != nil {
			return domain.RehydrateLabelTransactionSpec{}, err
		}
		spec.ParcelResults = append(spec.ParcelResults, result)
	}
	for _, raw := range document.FollowUpActions {
		action, err := raw.spec()
		if err != nil {
			return domain.RehydrateLabelTransactionSpec{}, err
		}
		spec.FollowUpActions = append(spec.FollowUpActions, action)
	}
	return spec, nil
}

func (document labelTransactionParcelResultDoc) spec() (domain.LabelTransactionParcelResultSpec, error) {
	parcel, err := domain.NewDeclaredParcelID(document.Parcel)
	if err != nil {
		return domain.LabelTransactionParcelResultSpec{}, err
	}
	spec := domain.LabelTransactionParcelResultSpec{Parcel: parcel, Accepted: document.Accepted}
	// 标识与原因各自可缺：受理带标识、未受理带原因，哪一半在场由领域那道结构校验裁决，
	// 适配器只如实带回——在这里补一个空值判断，等于把同一条规则抄成第二份。
	if document.Identifier != "" {
		identifier, err := domain.NewChannelParcelIdentifier(document.Identifier)
		if err != nil {
			return domain.LabelTransactionParcelResultSpec{}, err
		}
		spec.Identifier = identifier
	}
	if document.Reason != "" {
		reason, err := domain.NewChannelResultReasonReference(document.Reason)
		if err != nil {
			return domain.LabelTransactionParcelResultSpec{}, err
		}
		spec.Reason = reason
	}
	return spec, nil
}

func (document labelTransactionFollowUpActionDoc) spec() (domain.FollowUpActionSpec, error) {
	reason, err := domain.NewChannelResultReasonReference(document.Reason)
	if err != nil {
		return domain.FollowUpActionSpec{}, err
	}
	spec := domain.FollowUpActionSpec{
		Kind:       domain.FollowUpActionKind(document.Kind),
		Reason:     reason,
		OccurredAt: document.OccurredAt,
	}
	for _, raw := range document.Parcels {
		parcel, err := domain.NewDeclaredParcelID(raw)
		if err != nil {
			return domain.FollowUpActionSpec{}, err
		}
		spec.Parcels = append(spec.Parcels, parcel)
	}
	return spec, nil
}
