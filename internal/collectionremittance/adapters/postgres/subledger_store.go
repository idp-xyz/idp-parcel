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

// SubledgerStore 实现 ports.SubledgerRegistry：分户账的开立与记账写口。
//
// **本类型没有 UPDATE，也没有 DELETE。** 记账只追加是分配守恒的结构前提：一旦某条
// 记账可被改写，余额就不再等于历史之和，而对得上与对不上在读的人眼里没有区别。写错
// 走反向记账冲正，原记账原样留在账上。
type SubledgerStore struct {
	db *bentopg.DB
}

func NewSubledgerStore(db *bentopg.DB) (*SubledgerStore, error) {
	if db == nil {
		return nil, fmt.Errorf("collection remittance postgres: db is nil")
	}
	return &SubledgerStore{db: db}, nil
}

var _ ports.SubledgerRegistry = (*SubledgerStore)(nil)

func (store *SubledgerStore) OpenSubledger(
	ctx context.Context,
	tenant domain.TenantID,
	ledger domain.Subledger,
) (ports.SaveOutcome, error) {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return ports.SaveOutcomeInvalid, fmt.Errorf("open subledger: %w", err)
	}

	arguments := append(ledgerColumns(tenant, ledger.Key()),
		ledger.CustodyBasis().String(), ledger.OpenedAt().UTC())
	tag, err := executor.Exec(ctx,
		`INSERT INTO collection_remittance.subledger
			(tenant_id, customer_ref, legal_entity_ref, currency, channel_ref,
			 custody_basis_ref, opened_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT DO NOTHING`,
		arguments...,
	)
	if err != nil {
		return ports.SaveOutcomeInvalid, fmt.Errorf("open subledger: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.SaveAlreadyRegistered, nil
	}
	return ports.SaveRegistered, nil
}

// AppendPosting 追加一笔记账。位置与依据种类的落库词由领域词表给出；空串是调用方
// 传了未知格——那是编程错误，让它撞库上的 CHECK 会把它折成「依赖故障」那一格。
//
// 余额非负不在这里判：它要按当刻账面判，而账面由 SubledgerBalanceView 读、由领域
// 的 Admit 裁（写口只认自己这一行的形状）。环境事务由进程级入口给出，读回账面与
// 落账因此看同一份快照。
func (store *SubledgerStore) AppendPosting(
	ctx context.Context,
	tenant domain.TenantID,
	posting domain.SubledgerPosting,
) (ports.SaveOutcome, error) {
	from := posting.From().String()
	to := posting.To().String()
	basisKind := posting.BasisKind().String()
	if from == "" || to == "" {
		return ports.SaveOutcomeInvalid,
			fmt.Errorf("append posting: unknown fund position (from %d, to %d)",
				posting.From(), posting.To())
	}
	if basisKind == "" {
		return ports.SaveOutcomeInvalid,
			fmt.Errorf("append posting: unknown basis kind %d", posting.BasisKind())
	}
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return ports.SaveOutcomeInvalid, fmt.Errorf("append posting: %w", err)
	}

	arguments := append(ledgerColumns(tenant, posting.Ledger()),
		posting.ID().String(), from, to, posting.Amount().AmountMinor(),
		basisKind, posting.Basis().String(), posting.PostedAt().UTC())
	tag, err := executor.Exec(ctx,
		`INSERT INTO collection_remittance.subledger_posting
			(tenant_id, customer_ref, legal_entity_ref, currency, channel_ref,
			 posting_ref, from_position, to_position, amount_minor,
			 basis_kind, basis_ref, posted_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT DO NOTHING`,
		arguments...,
	)
	if err != nil {
		return ports.SaveOutcomeInvalid, fmt.Errorf("append posting: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.SaveAlreadyRegistered, nil
	}
	return ports.SaveRegistered, nil
}

// SubledgerBalanceView 实现 ports.SubledgerView：读分户账开立面、派生余额与单笔记账。
type SubledgerBalanceView struct {
	db *bentopg.DB
}

func NewSubledgerBalanceView(db *bentopg.DB) (*SubledgerBalanceView, error) {
	if db == nil {
		return nil, fmt.Errorf("collection remittance postgres: db is nil")
	}
	return &SubledgerBalanceView{db: db}, nil
}

var _ ports.SubledgerView = (*SubledgerBalanceView)(nil)

func (view *SubledgerBalanceView) LoadSubledger(
	ctx context.Context,
	tenant domain.TenantID,
	key domain.SubledgerKey,
) (domain.Subledger, bool, error) {
	none := domain.Subledger{}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load subledger: %w", err)
	}

	var (
		custodyRaw string
		openedAt   time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT custody_basis_ref, opened_at
		   FROM collection_remittance.subledger
		  WHERE tenant_id = $1 AND customer_ref = $2 AND legal_entity_ref = $3
		    AND currency = $4 AND channel_ref = $5`,
		ledgerColumns(tenant, key)...,
	).Scan(&custodyRaw, &openedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load subledger: %w", err)
	}

	custody, err := domain.NewCustodyBasisReference(custodyRaw)
	if err != nil {
		return none, false, fmt.Errorf("rebuild subledger: %w", err)
	}
	ledger, err := domain.OpenSubledger(key, custody, openedAt)
	if err != nil {
		return none, false, fmt.Errorf("rebuild subledger: %w", err)
	}
	return ledger, true, nil
}

// LoadBalance 读回一本账的全部记账再交给领域派生余额。
//
// 不在 SQL 里 SUM：守恒与非负那两道核在 DeriveSubledgerBalance 上，用 SQL 聚合等于
// 给同一条不变式立第二个口径，而两个口径迟早会在某一格上不一致——不一致的那天，两边
// 各自都说自己对。代价是按键取全量记账，这本账的记账量由代收笔数决定，索引
// subledger_posting_by_subledger 正是为它建的。
//
// found=false 只表示这本账未开立。已开立而零记账交回一个各位置皆零的余额——那是
// 「已开立且当期无本金」的如实答案，与未开立含义相反。
func (view *SubledgerBalanceView) LoadBalance(
	ctx context.Context,
	tenant domain.TenantID,
	key domain.SubledgerKey,
) (domain.SubledgerBalance, bool, error) {
	none := domain.SubledgerBalance{}
	if _, opened, err := view.LoadSubledger(ctx, tenant, key); err != nil || !opened {
		return none, false, err
	}

	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load subledger balance: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT posting_ref, from_position, to_position, amount_minor,
		        basis_kind, basis_ref, posted_at
		   FROM collection_remittance.subledger_posting
		  WHERE tenant_id = $1 AND customer_ref = $2 AND legal_entity_ref = $3
		    AND currency = $4 AND channel_ref = $5
		  ORDER BY posted_at, posting_ref`,
		ledgerColumns(tenant, key)...,
	)
	if err != nil {
		return none, false, fmt.Errorf("load subledger balance: %w", err)
	}
	defer rows.Close()

	postings := make([]domain.SubledgerPosting, 0)
	for rows.Next() {
		var (
			idRaw, fromRaw, toRaw, basisKindRaw, basisRaw string
			amountMinor                                   int64
			postedAt                                      time.Time
		)
		if err := rows.Scan(&idRaw, &fromRaw, &toRaw, &amountMinor,
			&basisKindRaw, &basisRaw, &postedAt); err != nil {
			return none, false, fmt.Errorf("load subledger balance: %w", err)
		}
		posting, err := rebuildPosting(key, idRaw, fromRaw, toRaw, amountMinor,
			basisKindRaw, basisRaw, postedAt)
		if err != nil {
			return none, false, err
		}
		postings = append(postings, posting)
	}
	if err := rows.Err(); err != nil {
		return none, false, fmt.Errorf("load subledger balance: %w", err)
	}

	balance, err := domain.DeriveSubledgerBalance(key, postings)
	if err != nil {
		return none, false, fmt.Errorf("load subledger balance: %w", err)
	}
	return balance, true, nil
}

func (view *SubledgerBalanceView) LoadPosting(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.PostingID,
) (domain.SubledgerPosting, bool, error) {
	none := domain.SubledgerPosting{}
	if strings.TrimSpace(id.String()) == "" {
		return none, false, fmt.Errorf("load posting: the posting ID is blank")
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load posting: %w", err)
	}

	var (
		customerRaw, entityRaw, currencyRaw, channelRaw string
		fromRaw, toRaw, basisKindRaw, basisRaw          string
		amountMinor                                     int64
		postedAt                                        time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT customer_ref, legal_entity_ref, currency, channel_ref,
		        from_position, to_position, amount_minor, basis_kind, basis_ref, posted_at
		   FROM collection_remittance.subledger_posting
		  WHERE tenant_id = $1 AND posting_ref = $2`,
		tenant.String(), id.String(),
	).Scan(&customerRaw, &entityRaw, &currencyRaw, &channelRaw,
		&fromRaw, &toRaw, &amountMinor, &basisKindRaw, &basisRaw, &postedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load posting: %w", err)
	}

	key, err := rebuildLedgerKey(customerRaw, entityRaw, currencyRaw, channelRaw)
	if err != nil {
		return none, false, err
	}
	posting, err := rebuildPosting(key, id.String(), fromRaw, toRaw, amountMinor,
		basisKindRaw, basisRaw, postedAt)
	if err != nil {
		return none, false, err
	}
	return posting, true, nil
}

// rebuildPosting 把一行记账还原成领域记账，逐格走同一道构造门——包括三条依据门。
// 坏行在读口拦下上抛：一条绕过写口进来的记账，正是账面唯一可能不守恒的来源。
func rebuildPosting(
	key domain.SubledgerKey,
	idRaw, fromRaw, toRaw string,
	amountMinor int64,
	basisKindRaw, basisRaw string,
	postedAt time.Time,
) (domain.SubledgerPosting, error) {
	none := domain.SubledgerPosting{}
	id, err := domain.NewPostingID(idRaw)
	if err != nil {
		return none, fmt.Errorf("rebuild posting: %w", err)
	}
	from, known := domain.ParseFundPosition(fromRaw)
	if !known {
		return none, fmt.Errorf("rebuild posting: unknown fund position %q", fromRaw)
	}
	to, known := domain.ParseFundPosition(toRaw)
	if !known {
		return none, fmt.Errorf("rebuild posting: unknown fund position %q", toRaw)
	}
	basisKind, known := domain.ParsePostingBasisKind(basisKindRaw)
	if !known {
		return none, fmt.Errorf("rebuild posting: unknown basis kind %q", basisKindRaw)
	}
	basis, err := domain.NewBasisReference(basisRaw)
	if err != nil {
		return none, fmt.Errorf("rebuild posting: %w", err)
	}
	amount, err := rebuildMoney(key.Currency().String(), amountMinor)
	if err != nil {
		return none, err
	}
	posting, err := domain.RecordSubledgerPosting(domain.SubledgerPostingSpec{
		ID:        id,
		Ledger:    key,
		From:      from,
		To:        to,
		Amount:    amount,
		BasisKind: basisKind,
		Basis:     basis,
		PostedAt:  postedAt,
	})
	if err != nil {
		return none, fmt.Errorf("rebuild posting: %w", err)
	}
	return posting, nil
}
