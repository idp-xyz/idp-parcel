package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// SubmissionSends 是申报通道、发送事实与技术回执三本册的仓储（迁移 0030，ADR-0342）。三本写口同一条代数：插不进就按键读回、
// 逐格比，同内容答重放，异内容答冲突、不覆盖。
type SubmissionSends struct {
	db *bentopg.DB
}

var (
	_ ports.DeclarationChannelRegistry = (*SubmissionSends)(nil)
	_ ports.SubmissionSendFactStore    = (*SubmissionSends)(nil)
	_ ports.TechnicalReceiptStore      = (*SubmissionSends)(nil)
)

func NewSubmissionSends(db *bentopg.DB) (*SubmissionSends, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &SubmissionSends{db: db}, nil
}

func optionalSendText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func sendText(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (repository *SubmissionSends) SaveDeclarationChannel(
	ctx context.Context,
	tenant domain.TenantID,
	channel domain.DeclarationChannel,
	at time.Time,
) (ports.SubmissionSendEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save declaration channel: %w", err)
	}
	tag, err := executor.Exec(ctx, `
		INSERT INTO customs_compliance.declaration_channel (tenant_id, target, endpoint, credential_ref, registered_at)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`,
		tenant.String(), channel.Target(), channel.Endpoint(), optionalSendText(channel.Credential()), at.UTC())
	if err != nil {
		return 0, fmt.Errorf("save declaration channel: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.SubmissionSendRegistered, nil
	}
	stored, found, err := repository.loadChannel(ctx, executor, tenant, channel.Target())
	if err != nil {
		return 0, fmt.Errorf("save declaration channel: %w", err)
	}
	if found && stored.SameChannel(channel) {
		return ports.SubmissionSendReplayed, nil
	}
	return ports.SubmissionSendConflict, nil
}

type sendQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (repository *SubmissionSends) LoadDeclarationChannel(
	ctx context.Context,
	tenant domain.TenantID,
	target string,
) (domain.DeclarationChannel, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.DeclarationChannel{}, false, fmt.Errorf("load declaration channel: %w", err)
	}
	return repository.loadChannel(ctx, querier, tenant, target)
}

func (repository *SubmissionSends) loadChannel(
	ctx context.Context,
	querier sendQuerier,
	tenant domain.TenantID,
	target string,
) (domain.DeclarationChannel, bool, error) {
	var endpoint string
	var credential *string
	err := querier.QueryRow(ctx, `
		SELECT endpoint, credential_ref FROM customs_compliance.declaration_channel
		 WHERE tenant_id = $1 AND target = $2`, tenant.String(), target).Scan(&endpoint, &credential)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DeclarationChannel{}, false, nil
	}
	if err != nil {
		return domain.DeclarationChannel{}, false, fmt.Errorf("load declaration channel: %w", err)
	}
	channel, err := domain.NewDeclarationChannel(target, endpoint, sendText(credential))
	if err != nil {
		return domain.DeclarationChannel{}, false, fmt.Errorf("load declaration channel: %w", err)
	}
	return channel, true, nil
}

func (repository *SubmissionSends) SaveSendFact(
	ctx context.Context,
	tenant domain.TenantID,
	fact domain.SubmissionSendFact,
	at time.Time,
) (ports.SubmissionSendEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save submission send fact: %w", err)
	}
	// 不带冲突目标：撞主键（同一份尝试已有发送事实）与撞消息号（通道把同一个消息号给了另一份尝试）都落到下面读回比对。
	tag, err := executor.Exec(ctx, `
		INSERT INTO customs_compliance.submission_send_fact
			(tenant_id, version_id, sequence, sent_at, outcome, channel_message, response_ref, recorded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT DO NOTHING`,
		tenant.String(), fact.Version().String(), fact.Sequence(), fact.SentAt(), fact.Outcome().String(),
		optionalSendText(fact.ChannelMessage()), optionalSendText(fact.Response()), at.UTC())
	if err != nil {
		return 0, fmt.Errorf("save submission send fact: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.SubmissionSendRegistered, nil
	}
	stored, found, err := repository.findSendFact(ctx, executor, tenant, fact.Version(), fact.Sequence())
	if err != nil {
		return 0, fmt.Errorf("save submission send fact: %w", err)
	}
	if found && stored.SentAt().Equal(fact.SentAt()) && stored.Outcome() == fact.Outcome() &&
		stored.ChannelMessage() == fact.ChannelMessage() {
		return ports.SubmissionSendReplayed, nil
	}
	return ports.SubmissionSendConflict, nil
}

const sendFactSelect = `
	SELECT version_id, sequence, sent_at, outcome, channel_message, response_ref
	  FROM customs_compliance.submission_send_fact`

func scanSendFact(row pgx.Row) (domain.SubmissionSendFact, bool, error) {
	var (
		versionID, outcome string
		sequence           int
		sentAt             time.Time
		message, response  *string
	)
	err := row.Scan(&versionID, &sequence, &sentAt, &outcome, &message, &response)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SubmissionSendFact{}, false, nil
	}
	if err != nil {
		return domain.SubmissionSendFact{}, false, err
	}
	version, err := domain.NewSubmissionVersionID(versionID)
	if err != nil {
		return domain.SubmissionSendFact{}, false, err
	}
	result, err := attemptResultFrom(outcome)
	if err != nil {
		return domain.SubmissionSendFact{}, false, err
	}
	fact, err := domain.RecordSubmissionSend(version, sequence, sentAt, result, sendText(message), sendText(response))
	if err != nil {
		return domain.SubmissionSendFact{}, false, err
	}
	return fact, true, nil
}

func (repository *SubmissionSends) findSendFact(
	ctx context.Context,
	querier sendQuerier,
	tenant domain.TenantID,
	version domain.SubmissionVersionID,
	sequence int,
) (domain.SubmissionSendFact, bool, error) {
	return scanSendFact(querier.QueryRow(ctx, sendFactSelect+`
		 WHERE tenant_id = $1 AND version_id = $2 AND sequence = $3`, tenant.String(), version.String(), sequence))
}

func (repository *SubmissionSends) FindSendFact(
	ctx context.Context,
	tenant domain.TenantID,
	version domain.SubmissionVersionID,
	sequence int,
) (domain.SubmissionSendFact, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.SubmissionSendFact{}, false, fmt.Errorf("find submission send fact: %w", err)
	}
	fact, found, err := repository.findSendFact(ctx, querier, tenant, version, sequence)
	if err != nil {
		return domain.SubmissionSendFact{}, false, fmt.Errorf("find submission send fact: %w", err)
	}
	return fact, found, nil
}

func (repository *SubmissionSends) FindSendFactByChannelMessage(
	ctx context.Context,
	tenant domain.TenantID,
	message string,
) (domain.SubmissionSendFact, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.SubmissionSendFact{}, false, fmt.Errorf("find submission send fact by message: %w", err)
	}
	fact, found, err := scanSendFact(querier.QueryRow(ctx, sendFactSelect+`
		 WHERE tenant_id = $1 AND channel_message = $2`, tenant.String(), message))
	if err != nil {
		return domain.SubmissionSendFact{}, false, fmt.Errorf("find submission send fact by message: %w", err)
	}
	return fact, found, nil
}

func (repository *SubmissionSends) SaveTechnicalReceipt(
	ctx context.Context,
	tenant domain.TenantID,
	receipt domain.TechnicalReceipt,
	at time.Time,
) (ports.SubmissionSendEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save technical receipt: %w", err)
	}
	tag, err := executor.Exec(ctx, `
		INSERT INTO customs_compliance.technical_receipt
			(tenant_id, source_id, version_id, sequence, outcome, received_at, content_digest, recorded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT DO NOTHING`,
		tenant.String(), receipt.Source(), receipt.Version().String(), receipt.Sequence(), receipt.Outcome().String(),
		receipt.ReceivedAt(), receipt.Digest(), at.UTC())
	if err != nil {
		return 0, fmt.Errorf("save technical receipt: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.SubmissionSendRegistered, nil
	}
	var digest string
	if err := executor.QueryRow(ctx, `
		SELECT content_digest FROM customs_compliance.technical_receipt WHERE tenant_id = $1 AND source_id = $2`,
		tenant.String(), receipt.Source()).Scan(&digest); err != nil {
		return 0, fmt.Errorf("save technical receipt: %w", err)
	}
	if digest == receipt.Digest() {
		return ports.SubmissionSendReplayed, nil
	}
	return ports.SubmissionSendConflict, nil
}

func (repository *SubmissionSends) ListTechnicalReceipts(
	ctx context.Context,
	tenant domain.TenantID,
	version domain.SubmissionVersionID,
	sequence int,
) ([]domain.TechnicalReceipt, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list technical receipts: %w", err)
	}
	rows, err := querier.Query(ctx, `
		SELECT source_id, outcome, received_at, content_digest FROM customs_compliance.technical_receipt
		 WHERE tenant_id = $1 AND version_id = $2 AND sequence = $3 ORDER BY received_at, source_id`,
		tenant.String(), version.String(), sequence)
	if err != nil {
		return nil, fmt.Errorf("list technical receipts: %w", err)
	}
	defer rows.Close()
	var receipts []domain.TechnicalReceipt
	for rows.Next() {
		var (
			source, outcome, digest string
			receivedAt              time.Time
		)
		if err := rows.Scan(&source, &outcome, &receivedAt, &digest); err != nil {
			return nil, fmt.Errorf("list technical receipts: %w", err)
		}
		result, err := attemptResultFrom(outcome)
		if err != nil {
			return nil, fmt.Errorf("list technical receipts: %w", err)
		}
		receipt, err := domain.ReceiveTechnicalReceipt(source, version, sequence, result, receivedAt, digest)
		if err != nil {
			return nil, fmt.Errorf("list technical receipts: %w", err)
		}
		receipts = append(receipts, receipt)
	}
	return receipts, rows.Err()
}
