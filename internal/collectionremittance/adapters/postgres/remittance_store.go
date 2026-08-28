package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
)

// RemittanceStore 实现 ports.RemittanceRegistry：回汇批次的形成与交出汇付主张。
//
// 全表唯一的 UPDATE 在 HandOverBatchForPayment，且带 `WHERE state = 'COLLECTED'`
// ——方向由 SQL 守着，没有任何语句能把批次退回已归集，也没有语句改得动键、币种、
// 归集截点与形成时刻。批次不可覆盖这条因此不靠调用方自觉（ADR-0082）。
type RemittanceStore struct {
	db *bentopg.DB
}

func NewRemittanceStore(db *bentopg.DB) (*RemittanceStore, error) {
	if db == nil {
		return nil, fmt.Errorf("collection remittance postgres: db is nil")
	}
	return &RemittanceStore{db: db}, nil
}

var _ ports.RemittanceRegistry = (*RemittanceStore)(nil)

// FormBatch 形成一个批次。状态不是入参——批次一律以`已归集`进册，落库词取领域常量
// 而不是字面量，免得这里与领域词表各持一份。
func (store *RemittanceStore) FormBatch(
	ctx context.Context,
	tenant domain.TenantID,
	batch domain.RemittanceBatch,
) (ports.SaveOutcome, error) {
	state := batch.State().String()
	if state == "" {
		return ports.SaveOutcomeInvalid,
			fmt.Errorf("form remittance batch: unknown batch state %d", batch.State())
	}
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return ports.SaveOutcomeInvalid, fmt.Errorf("form remittance batch: %w", err)
	}

	arguments := append(ledgerColumns(tenant, batch.Ledger()),
		batch.ID().String(), batch.CollectedThrough().UTC(), state, batch.FormedAt().UTC())
	tag, err := executor.Exec(ctx,
		`INSERT INTO collection_remittance.remittance_batch
			(tenant_id, customer_ref, legal_entity_ref, currency, channel_ref,
			 batch_ref, collected_through, state, formed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT DO NOTHING`,
		arguments...,
	)
	if err != nil {
		return ports.SaveOutcomeInvalid, fmt.Errorf("form remittance batch: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.SaveAlreadyRegistered, nil
	}
	return ports.SaveRegistered, nil
}

// HandOverBatchForPayment 把批次推进到`已交出汇付主张`。
//
// UPDATE 影响零行有两种成因，两者的处置相反，所以要分开答：批次根本不在册（改请求），
// 与批次已在已交出态（意图已达成）。分辨靠再读一次状态，不靠猜——「零行」这一个信号
// 在两种情况下长得一模一样。
func (store *RemittanceStore) HandOverBatchForPayment(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.RemittanceBatchID,
) (ports.HandOverOutcome, error) {
	if strings.TrimSpace(id.String()) == "" {
		return ports.HandOverOutcomeInvalid,
			fmt.Errorf("hand over remittance batch: the batch ID is blank")
	}
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return ports.HandOverOutcomeInvalid, fmt.Errorf("hand over remittance batch: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`UPDATE collection_remittance.remittance_batch
		    SET state = $3
		  WHERE tenant_id = $1 AND batch_ref = $2 AND state = $4`,
		tenant.String(), id.String(),
		domain.BatchHandedForPayment.String(), domain.BatchCollected.String(),
	)
	if err != nil {
		return ports.HandOverOutcomeInvalid, fmt.Errorf("hand over remittance batch: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return ports.HandedOver, nil
	}

	var stateRaw string
	err = executor.QueryRow(ctx,
		`SELECT state FROM collection_remittance.remittance_batch
		  WHERE tenant_id = $1 AND batch_ref = $2`,
		tenant.String(), id.String(),
	).Scan(&stateRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.HandOverTargetMissing, nil
	}
	if err != nil {
		return ports.HandOverOutcomeInvalid, fmt.Errorf("hand over remittance batch: %w", err)
	}
	if stateRaw == domain.BatchHandedForPayment.String() {
		return ports.AlreadyHandedOver, nil
	}
	return ports.HandOverOutcomeInvalid,
		fmt.Errorf("hand over remittance batch: batch %s sits in unknown state %q", id, stateRaw)
}

// RemittanceBatchView 实现 ports.RemittanceView：按批次标识读回批次，含状态。
type RemittanceBatchView struct {
	db *bentopg.DB
}

func NewRemittanceBatchView(db *bentopg.DB) (*RemittanceBatchView, error) {
	if db == nil {
		return nil, fmt.Errorf("collection remittance postgres: db is nil")
	}
	return &RemittanceBatchView{db: db}, nil
}

var _ ports.RemittanceView = (*RemittanceBatchView)(nil)

func (view *RemittanceBatchView) LoadBatch(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.RemittanceBatchID,
) (domain.RemittanceBatch, bool, error) {
	none := domain.RemittanceBatch{}
	if strings.TrimSpace(id.String()) == "" {
		return none, false, fmt.Errorf("load remittance batch: the batch ID is blank")
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load remittance batch: %w", err)
	}

	var (
		customerRaw, entityRaw, currencyRaw, channelRaw string
		stateRaw                                        string
		collectedThrough, formedAt                      time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT customer_ref, legal_entity_ref, currency, channel_ref,
		        collected_through, state, formed_at
		   FROM collection_remittance.remittance_batch
		  WHERE tenant_id = $1 AND batch_ref = $2`,
		tenant.String(), id.String(),
	).Scan(&customerRaw, &entityRaw, &currencyRaw, &channelRaw,
		&collectedThrough, &stateRaw, &formedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load remittance batch: %w", err)
	}

	key, err := rebuildLedgerKey(customerRaw, entityRaw, currencyRaw, channelRaw)
	if err != nil {
		return none, false, err
	}
	// 状态走 Rehydrate 而不是 Form：后者会把已交出的批次退回`已归集`，而汇付记账
	// 那道门恰恰就问这一格。
	state, known := domain.ParseRemittanceBatchState(stateRaw)
	if !known {
		return none, false, fmt.Errorf("rebuild remittance batch: unknown state %q", stateRaw)
	}
	batch, err := domain.RehydrateRemittanceBatch(domain.RemittanceBatchSpec{
		ID:               id,
		Ledger:           key,
		CollectedThrough: collectedThrough,
		FormedAt:         formedAt,
	}, state)
	if err != nil {
		return none, false, fmt.Errorf("rebuild remittance batch: %w", err)
	}
	return batch, true, nil
}
