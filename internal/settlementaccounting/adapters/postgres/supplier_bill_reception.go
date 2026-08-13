package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// BillReceptions 实现 ports.BillReceptionStore。幂等三维（租户+主张身份+版本）就是
// 主键；`已有记录`由 ON CONFLICT DO NOTHING 加零行判定翻译（ADR-0031），撞键不把
// 事务打进中止态——Save 之后编排还要同事务读回赢家比对 content_digest 分重放/冲突。
type BillReceptions struct {
	db *bentopg.DB
}

func NewBillReceptions(db *bentopg.DB) (*BillReceptions, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &BillReceptions{db: db}, nil
}

// claimDocument 与 matchRow 是 jsonb 列的行模型。主张身份、供应商、法人、账期与币种
// 归平铺列独家拥有，不再进 json——存两份迟早各说各话；匹配的金额与币种同理从主张列
// 取（MatchBillLine 本就不允许旁路第二套数字）。
type claimDocument struct {
	Lines       []billLineRow `json:"lines"`
	Supplements string        `json:"supplements,omitempty"`
	ReceivedAt  time.Time     `json:"received_at"`
}

type billLineRow struct {
	Line         string `json:"line"`
	FeeItem      string `json:"fee_item"`
	ClaimedMinor int64  `json:"claimed_minor"`
}

type matchRow struct {
	Line           string    `json:"line"`
	Classification string    `json:"classification"`
	Expected       string    `json:"expected,omitempty"`
	ClaimedMinor   int64     `json:"claimed_minor"`
	ExpectedMinor  int64     `json:"expected_minor"`
	Basis          string    `json:"basis,omitempty"`
	MatchedAt      time.Time `json:"matched_at"`
}

// FindByKey 按幂等键取回已提交接收。否定结果只回 false，不区分「不存在」与「属于
// 另一个租户」。主张经 ReceiveSupplierBillClaim 重建、逐行匹配经 RehydrateBillLineMatch
// 重走五值矩阵——一次坏写入在读回处暴露。
func (repository *BillReceptions) FindByKey(
	ctx context.Context,
	key ports.BillReceptionKey,
) (ports.BillReceptionRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.BillReceptionRecord{}, false, fmt.Errorf("find bill reception: %w", err)
	}

	var (
		supplier, legalEntity, period, currency string
		digest                                  string
		claimJSON, matchesJSON                  []byte
		auditConfigured                         bool
		recordedAt                              time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT supplier_ref, legal_entity, period_ref, currency,
		        content_digest, claim, matches, audit_authority_configured, recorded_at
		   FROM settlement_accounting.supplier_bill_reception
		  WHERE tenant_id = $1
		    AND claim_id = $2
		    AND claim_version = $3`,
		key.TenantID.String(),
		key.Claim.String(),
		key.Version.String(),
	).Scan(&supplier, &legalEntity, &period, &currency,
		&digest, &claimJSON, &matchesJSON, &auditConfigured, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.BillReceptionRecord{}, false, nil
	}
	if err != nil {
		return ports.BillReceptionRecord{}, false, fmt.Errorf("find bill reception: %w", err)
	}

	claim, err := rebuildClaim(key, billClaimColumns{
		supplier:    supplier,
		legalEntity: legalEntity,
		period:      period,
		currency:    currency,
	}, claimJSON)
	if err != nil {
		return ports.BillReceptionRecord{}, false, fmt.Errorf("find bill reception: %w", err)
	}
	matches, err := rebuildMatches(key.Claim, claim.Currency(), matchesJSON)
	if err != nil {
		return ports.BillReceptionRecord{}, false, fmt.Errorf("find bill reception: %w", err)
	}

	return ports.BillReceptionRecord{
		Key:                      key,
		ContentDigest:            digest,
		Claim:                    claim,
		Matches:                  matches,
		AuditAuthorityConfigured: auditConfigured,
		RecordedAt:               recordedAt.UTC(),
	}, true, nil
}

// Save 写下一次账单接收。同键已有记录时答`已有记录`——业务答案不是错误（ADR-0031），
// 编排据此读回赢家、按 content_digest 分重放与版本冲突（UC-SA-004 一致性）。
func (repository *BillReceptions) Save(
	ctx context.Context,
	record ports.BillReceptionRecord,
) (ports.BillSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.BillSaveOutcomeInvalid, fmt.Errorf("save bill reception: %w", err)
	}

	claim := record.Claim
	if claim.Claim() != record.Key.Claim || claim.Version() != record.Key.Version {
		return ports.BillSaveOutcomeInvalid, fmt.Errorf(
			"save bill reception: record key disagrees with the claim's identity")
	}
	document := claimDocument{ReceivedAt: claim.ReceivedAt()}
	for _, line := range claim.Lines() {
		document.Lines = append(document.Lines, billLineRow{
			Line:         line.Line.String(),
			FeeItem:      line.FeeItem.String(),
			ClaimedMinor: line.ClaimedMinor,
		})
	}
	if supplements, present := claim.Supplements(); present {
		document.Supplements = supplements.String()
	}
	claimJSON, err := json.Marshal(document)
	if err != nil {
		return ports.BillSaveOutcomeInvalid, fmt.Errorf("save bill reception: %w", err)
	}

	matchRows := make([]matchRow, 0, len(record.Matches))
	for _, match := range record.Matches {
		row := matchRow{
			Line:           match.Line().String(),
			Classification: match.Classification().String(),
			ClaimedMinor:   match.ClaimedMinor(),
			ExpectedMinor:  match.ExpectedMinor(),
			MatchedAt:      match.MatchedAt(),
		}
		if expected, present := match.Expected(); present {
			row.Expected = expected.String()
		}
		if basis, present := match.Basis(); present {
			row.Basis = basis.String()
		}
		matchRows = append(matchRows, row)
	}
	matchesJSON, err := json.Marshal(matchRows)
	if err != nil {
		return ports.BillSaveOutcomeInvalid, fmt.Errorf("save bill reception: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.supplier_bill_reception
			(tenant_id, claim_id, claim_version,
			 supplier_ref, legal_entity, period_ref, currency,
			 content_digest, claim, matches, audit_authority_configured, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Claim.String(),
		record.Key.Version.String(),
		claim.Supplier().String(),
		claim.LegalEntity().String(),
		claim.Period().String(),
		claim.Currency().String(),
		record.ContentDigest,
		claimJSON,
		matchesJSON,
		record.AuditAuthorityConfigured,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.BillSaveOutcomeInvalid, fmt.Errorf("save bill reception: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.BillAlreadyRecorded, nil
	}
	return ports.BillSaved, nil
}

type billClaimColumns struct {
	supplier    string
	legalEntity string
	period      string
	currency    string
}

func rebuildClaim(
	key ports.BillReceptionKey,
	columns billClaimColumns,
	data []byte,
) (domain.SupplierBillClaim, error) {
	var document claimDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return domain.SupplierBillClaim{}, err
	}
	supplier, err := domain.NewSupplierPartyReference(columns.supplier)
	if err != nil {
		return domain.SupplierBillClaim{}, err
	}
	legalEntity, err := domain.NewLegalEntityReference(columns.legalEntity)
	if err != nil {
		return domain.SupplierBillClaim{}, err
	}
	period, err := domain.NewBillingPeriodReference(columns.period)
	if err != nil {
		return domain.SupplierBillClaim{}, err
	}
	currency, err := domain.NewCurrencyCode(columns.currency)
	if err != nil {
		return domain.SupplierBillClaim{}, err
	}
	spec := domain.SupplierBillClaimSpec{
		Claim:       key.Claim,
		Version:     key.Version,
		Supplier:    supplier,
		LegalEntity: legalEntity,
		Period:      period,
		Currency:    currency,
		ReceivedAt:  document.ReceivedAt,
	}
	for _, row := range document.Lines {
		line, err := domain.NewBillLineReference(row.Line)
		if err != nil {
			return domain.SupplierBillClaim{}, err
		}
		feeItem, err := domain.NewFeeItemReference(row.FeeItem)
		if err != nil {
			return domain.SupplierBillClaim{}, err
		}
		spec.Lines = append(spec.Lines, domain.BillLine{
			Line: line, FeeItem: feeItem, ClaimedMinor: row.ClaimedMinor,
		})
	}
	if document.Supplements != "" {
		if spec.SupplementsClaim, err = domain.NewBillClaimID(document.Supplements); err != nil {
			return domain.SupplierBillClaim{}, err
		}
	}
	return domain.ReceiveSupplierBillClaim(spec)
}

func rebuildMatches(
	claim domain.BillClaimID,
	currency domain.CurrencyCode,
	data []byte,
) ([]domain.BillLineMatch, error) {
	var rows []matchRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	matches := make([]domain.BillLineMatch, 0, len(rows))
	for _, row := range rows {
		line, err := domain.NewBillLineReference(row.Line)
		if err != nil {
			return nil, err
		}
		classification, err := matchClassificationFrom(row.Classification)
		if err != nil {
			return nil, err
		}
		spec := domain.RehydrateBillLineMatchSpec{
			Claim:          claim,
			Line:           line,
			Classification: classification,
			ClaimedMinor:   row.ClaimedMinor,
			ExpectedMinor:  row.ExpectedMinor,
			Currency:       currency,
			MatchedAt:      row.MatchedAt,
		}
		if row.Expected != "" {
			if spec.Expected, err = domain.NewSupplierCostVersionID(row.Expected); err != nil {
				return nil, err
			}
		}
		if row.Basis != "" {
			if spec.Basis, err = domain.NewMatchBasisReference(row.Basis); err != nil {
				return nil, err
			}
		}
		match, err := domain.RehydrateBillLineMatch(spec)
		if err != nil {
			return nil, err
		}
		matches = append(matches, match)
	}
	return matches, nil
}

func matchClassificationFrom(raw string) (domain.MatchClassification, error) {
	switch raw {
	case domain.LineMatched.String():
		return domain.LineMatched, nil
	case domain.QuantityVariance.String():
		return domain.QuantityVariance, nil
	case domain.PriceVariance.String():
		return domain.PriceVariance, nil
	case domain.NoMatchingOccurrence.String():
		return domain.NoMatchingOccurrence, nil
	case domain.DuplicateBilling.String():
		return domain.DuplicateBilling, nil
	default:
		return domain.MatchClassificationInvalid, fmt.Errorf("unknown match classification %q", raw)
	}
}
