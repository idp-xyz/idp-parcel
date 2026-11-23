package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// QuotationReplies 是报价答复册。一条发出至多一条，只插入。
// 同一条再交来、决定与客户时刻与证据与原因都相同，是重放；有任何不同是冲突。都不改已有行。
type QuotationReplies struct {
	db *bentopg.DB
}

func NewQuotationReplies(db *bentopg.DB) (*QuotationReplies, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &QuotationReplies{db: db}, nil
}

var _ ports.QuotationReplyStore = (*QuotationReplies)(nil)

const quotationReplyColumns = `quotation_id, decision, replied_at, evidence_index, rejection_reason, recorded_by, recorded_at`

func (repository *QuotationReplies) AppendQuotationReply(ctx context.Context, reply domain.QuotationReply) (ports.QuotationReplyAppendOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.QuotationReplyAppendOutcomeInvalid, fmt.Errorf("append quotation reply: %w", err)
	}
	_, err = executor.Exec(ctx,
		`INSERT INTO party_commercial.quotation_reply (
		    tenant_id, quotation_id, decision, replied_at, evidence_index, rejection_reason, recorded_by, recorded_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		reply.Tenant().String(), reply.QuotationID().String(), reply.Decision().String(), reply.RepliedAt(),
		reply.EvidenceIndex().Text(), reply.RejectionReason().Text(), reply.RecordedBy().String(), reply.RecordedAt(),
	)
	if err == nil {
		return ports.QuotationReplyAppended, nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != pgUniqueViolation {
		return ports.QuotationReplyAppendOutcomeInvalid, fmt.Errorf("append quotation reply: %w", err)
	}
	existing, found, loadErr := repository.load(ctx, executor, reply.Tenant(), reply.QuotationID())
	if loadErr != nil {
		return ports.QuotationReplyAppendOutcomeInvalid, loadErr
	}
	if !found {
		return ports.QuotationReplyAppendOutcomeInvalid, fmt.Errorf("append quotation reply: 撞键后读不回既有行")
	}
	if existing.SameFact(reply) {
		return ports.QuotationReplyAppendReplay, nil
	}
	return ports.QuotationReplyAppendConflict, nil
}

func (repository *QuotationReplies) GetQuotationReply(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.QuotationID,
) (domain.QuotationReply, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.QuotationReply{}, false, fmt.Errorf("get quotation reply: %w", err)
	}
	return repository.load(ctx, querier, tenant, id)
}

func (repository *QuotationReplies) load(
	ctx context.Context,
	querier quotationQuerier,
	tenant domain.TenantID,
	id domain.QuotationID,
) (domain.QuotationReply, bool, error) {
	row := querier.QueryRow(ctx,
		`SELECT `+quotationReplyColumns+`
		   FROM party_commercial.quotation_reply
		  WHERE tenant_id = $1 AND quotation_id = $2`,
		tenant.String(), id.String())
	reply, err := scanQuotationReply(tenant, row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.QuotationReply{}, false, nil
	}
	if err != nil {
		return domain.QuotationReply{}, false, err
	}
	return reply, true, nil
}

func scanQuotationReply(tenant domain.TenantID, scanner quotationScanner) (domain.QuotationReply, error) {
	var (
		id, decision, evidence, reason, recordedBy string
		repliedAt, recordedAt                      time.Time
	)
	if err := scanner.Scan(&id, &decision, &repliedAt, &evidence, &reason, &recordedBy, &recordedAt); err != nil {
		return domain.QuotationReply{}, err
	}
	quotationID, err := domain.NewQuotationID(id)
	if err != nil {
		return domain.QuotationReply{}, err
	}
	choice, known := domain.QuotationReplyDecisionNamed(decision)
	if !known {
		return domain.QuotationReply{}, fmt.Errorf("load quotation reply: decision %s", decision)
	}
	recorder, err := domain.NewOperatorSubjectReference(recordedBy)
	if err != nil {
		return domain.QuotationReply{}, err
	}
	return domain.NewQuotationReply(domain.QuotationReplySpec{
		QuotationID: quotationID, Tenant: tenant, Decision: choice,
		RepliedAt: repliedAt, EvidenceIndex: evidence, RejectionReason: reason,
		RecordedBy: recorder, RecordedAt: recordedAt,
	})
}
