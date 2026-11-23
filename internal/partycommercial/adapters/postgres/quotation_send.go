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

// QuotationSends 是报价发出记录册。只插入。同一标识再交来相同事实是重放，不同事实是冲突，都不改已有行。
type QuotationSends struct {
	db *bentopg.DB
}

func NewQuotationSends(db *bentopg.DB) (*QuotationSends, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &QuotationSends{db: db}, nil
}

var _ ports.QuotationSendStore = (*QuotationSends)(nil)

const quotationSendColumns = `quotation_id, price_lane, plan_id, plan_version, canonicalization_version, content_digest,
	counterparty_party_id, counterparty_name, legal_entity_id, legal_entity_name,
	valid_from, valid_until, terms, sent_by, sent_at, method`

func (repository *QuotationSends) AppendQuotationSend(ctx context.Context, send domain.QuotationSend) (ports.QuotationAppendOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.QuotationAppendOutcomeInvalid, fmt.Errorf("append quotation send: %w", err)
	}
	_, err = executor.Exec(ctx,
		`INSERT INTO party_commercial.quotation_send (
		    tenant_id, quotation_id, price_lane, plan_id, plan_version, canonicalization_version, content_digest,
		    counterparty_party_id, counterparty_name, legal_entity_id, legal_entity_name,
		    valid_from, valid_until, terms, sent_by, sent_at, method)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		send.Tenant().String(), send.ID().String(), send.Lane().String(), send.PlanID(), send.PlanVersion(),
		send.Canonicalization(), send.ContentDigest(), send.Counterparty().String(), send.CounterpartyName().String(),
		send.LegalEntity().String(), send.LegalEntityName().String(), send.Validity().StartsAt(), send.Validity().EndsAt(),
		send.Terms().Text(), send.Sender().String(), send.SentAt(), send.Method().String(),
	)
	if err == nil {
		return ports.QuotationAppended, nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != pgUniqueViolation {
		return ports.QuotationAppendOutcomeInvalid, fmt.Errorf("append quotation send: %w", err)
	}
	existing, found, loadErr := repository.load(ctx, executor, send.Tenant(), send.ID())
	if loadErr != nil {
		return ports.QuotationAppendOutcomeInvalid, loadErr
	}
	if !found {
		return ports.QuotationAppendOutcomeInvalid, fmt.Errorf("append quotation send: 撞键后读不回既有行")
	}
	if existing.SameFact(send) {
		return ports.QuotationAppendReplay, nil
	}
	return ports.QuotationAppendConflict, nil
}

func (repository *QuotationSends) GetQuotationSend(ctx context.Context, tenant domain.TenantID, id domain.QuotationID) (domain.QuotationSend, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.QuotationSend{}, false, fmt.Errorf("get quotation send: %w", err)
	}
	return repository.load(ctx, querier, tenant, id)
}

func (repository *QuotationSends) ListQuotationSends(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	cursor ports.QuotationSendCursor,
) (ports.QuotationSendPage, error) {
	var none ports.QuotationSendPage
	if tenant.String() == "" || limit <= 0 {
		return none, fmt.Errorf("list quotation sends: tenant and a positive limit are required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, fmt.Errorf("list quotation sends: %w", err)
	}
	var rows pgx.Rows
	if cursor.IsZero() {
		rows, err = querier.Query(ctx,
			`SELECT `+quotationSendColumns+`
			   FROM party_commercial.quotation_send
			  WHERE tenant_id = $1
			  ORDER BY sent_at DESC, quotation_id DESC
			  LIMIT $2`,
			tenant.String(), limit+1)
	} else {
		rows, err = querier.Query(ctx,
			`SELECT `+quotationSendColumns+`
			   FROM party_commercial.quotation_send
			  WHERE tenant_id = $1
			    AND (sent_at, quotation_id) < ($2, $3)
			  ORDER BY sent_at DESC, quotation_id DESC
			  LIMIT $4`,
			tenant.String(), cursor.SentAt, cursor.ID.String(), limit+1)
	}
	if err != nil {
		return none, fmt.Errorf("list quotation sends: %w", err)
	}
	defer rows.Close()
	var sends []domain.QuotationSend
	for rows.Next() {
		send, scanErr := scanQuotationSend(tenant, rows)
		if scanErr != nil {
			return none, scanErr
		}
		sends = append(sends, send)
	}
	if err := rows.Err(); err != nil {
		return none, fmt.Errorf("list quotation sends: %w", err)
	}
	page := ports.QuotationSendPage{Sends: sends}
	if len(sends) > limit {
		page.HasNext = true
		page.Sends = sends[:limit]
		last := page.Sends[limit-1]
		page.Next = ports.QuotationSendCursor{SentAt: last.SentAt(), ID: last.ID()}
	}
	if page.Sends == nil {
		page.Sends = []domain.QuotationSend{}
	}
	return page, nil
}

type quotationQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (repository *QuotationSends) load(ctx context.Context, querier quotationQuerier, tenant domain.TenantID, id domain.QuotationID) (domain.QuotationSend, bool, error) {
	row := querier.QueryRow(ctx,
		`SELECT `+quotationSendColumns+`
		   FROM party_commercial.quotation_send
		  WHERE tenant_id = $1 AND quotation_id = $2`,
		tenant.String(), id.String())
	send, err := scanQuotationSend(tenant, row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.QuotationSend{}, false, nil
	}
	if err != nil {
		return domain.QuotationSend{}, false, err
	}
	return send, true, nil
}

type quotationScanner interface {
	Scan(dest ...any) error
}

func scanQuotationSend(tenant domain.TenantID, scanner quotationScanner) (domain.QuotationSend, error) {
	var (
		id, lane, planID, planVersion, canonicalization, digest          string
		counterpartyID, counterpartyName, legalEntityID, legalEntityName string
		validFrom, validUntil, sentAt                                    time.Time
		terms, sentBy, method                                            string
	)
	if err := scanner.Scan(
		&id, &lane, &planID, &planVersion, &canonicalization, &digest,
		&counterpartyID, &counterpartyName, &legalEntityID, &legalEntityName,
		&validFrom, &validUntil, &terms, &sentBy, &sentAt, &method,
	); err != nil {
		return domain.QuotationSend{}, err
	}
	priceLane, known := domain.QuotationPriceLaneNamed(lane)
	if !known || method != domain.QuotationSendByWorkbookExport.String() {
		return domain.QuotationSend{}, fmt.Errorf("load quotation send: lane %s method %s", lane, method)
	}
	quotationID, err := domain.NewQuotationID(id)
	if err != nil {
		return domain.QuotationSend{}, err
	}
	counterparty, err := domain.NewPartyID(counterpartyID)
	if err != nil {
		return domain.QuotationSend{}, err
	}
	counterpartyLabel, err := domain.NewPartyName(counterpartyName)
	if err != nil {
		return domain.QuotationSend{}, err
	}
	legalEntity, err := domain.NewLegalEntityReference(legalEntityID)
	if err != nil {
		return domain.QuotationSend{}, err
	}
	legalEntityLabel, err := domain.NewPartyName(legalEntityName)
	if err != nil {
		return domain.QuotationSend{}, err
	}
	sender, err := domain.NewOperatorSubjectReference(sentBy)
	if err != nil {
		return domain.QuotationSend{}, err
	}
	sendMethod := domain.QuotationSendByWorkbookExport
	return domain.NewQuotationSend(domain.QuotationSendSpec{
		ID: quotationID, Tenant: tenant, Lane: priceLane,
		PlanID: planID, PlanVersion: planVersion,
		Canonicalization: canonicalization, ContentDigest: digest,
		Counterparty: counterparty, CounterpartyName: counterpartyLabel,
		LegalEntity: legalEntity, LegalEntityName: legalEntityLabel,
		StartsAt: validFrom, EndsAt: validUntil, Terms: terms,
		Sender: sender, SentAt: sentAt, Method: sendMethod,
	})
}
