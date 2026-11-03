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
		billNumber                              *string
		digest                                  string
		claimJSON, matchesJSON                  []byte
		auditConfigured                         bool
		recordedAt                              time.Time
	)
	// 账单号标识的是主张而不是某一版：取这份主张在账单号册上的号，不论哪一版带来（ADR-0357 决定一）。
	err = querier.QueryRow(ctx,
		`SELECT r.supplier_ref, r.legal_entity, r.period_ref, r.currency, n.bill_number,
		        r.content_digest, r.claim, r.matches, r.audit_authority_configured, r.recorded_at
		   FROM settlement_accounting.supplier_bill_reception AS r
		   LEFT JOIN settlement_accounting.supplier_bill_number AS n
		     ON n.tenant_id = r.tenant_id AND n.claim_id = r.claim_id
		  WHERE r.tenant_id = $1
		    AND r.claim_id = $2
		    AND r.claim_version = $3`,
		key.TenantID.String(),
		key.Claim.String(),
		key.Version.String(),
	).Scan(&supplier, &legalEntity, &period, &currency, &billNumber,
		&digest, &claimJSON, &matchesJSON, &auditConfigured, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.BillReceptionRecord{}, false, nil
	}
	if err != nil {
		return ports.BillReceptionRecord{}, false, fmt.Errorf("find bill reception: %w", err)
	}

	claim, err := rebuildClaim(key, billClaimColumns{
		supplier:    supplier,
		billNumber:  catalogueText(billNumber),
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
	if err := registerBillNumber(ctx, executor, record); err != nil {
		return ports.BillSaveOutcomeInvalid, fmt.Errorf("save bill reception: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.supplier_bill_reception
			(tenant_id, claim_id, claim_version,
			 supplier_ref, legal_entity, period_ref, currency,
			 content_digest, claim, matches, audit_authority_configured, recorded_at, operation_evidence)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
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
		optionalText(record.OperationEvidence),
	)
	if err != nil {
		return ports.BillSaveOutcomeInvalid, fmt.Errorf("save bill reception: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.BillAlreadyRecorded, nil
	}
	return ports.BillSaved, nil
}

// registerBillNumber 在接收行之前写下账单号这层关系：主张没带号就不写。同一主张同一号已登（新版本带同号）撞主键什么也不做；
// 撞上另一份主张或这份主张的另一个号是编排没排队就来写——查重归编排（它先按号与主张排队、再查），这里只核写下去的那一行确实是
// 这份主张的这个号，不是就交回错误：这时接收行还没写，回滚不留半份。
func registerBillNumber(ctx context.Context, executor bentopg.Executor, record ports.BillReceptionRecord) error {
	number, numbered := record.Claim.BillNumber()
	if !numbered {
		return nil
	}
	tenant, supplier, claim := record.Key.TenantID.String(), record.Claim.Supplier().String(), record.Claim.Claim().String()
	if _, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.supplier_bill_number
			(tenant_id, supplier_ref, bill_number, claim_id, claim_version, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT DO NOTHING`,
		tenant, supplier, number.String(), claim, record.Key.Version.String(), record.RecordedAt.UTC(),
	); err != nil {
		return fmt.Errorf("register supplier bill number: %w", err)
	}
	var holder string
	err := executor.QueryRow(ctx,
		`SELECT claim_id FROM settlement_accounting.supplier_bill_number
		  WHERE tenant_id = $1 AND supplier_ref = $2 AND bill_number = $3`,
		tenant, supplier, number.String(),
	).Scan(&holder)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("register supplier bill number: 主张 %s 已以另一家供应商的号登记", claim)
	}
	if err != nil {
		return fmt.Errorf("register supplier bill number: %w", err)
	}
	if holder != claim {
		return fmt.Errorf("register supplier bill number: %s 已标识主张 %s", number, holder)
	}
	assigner, registered, found, err := findClaimBillNumber(ctx, executor, record.Key.TenantID, record.Claim.Claim())
	if err != nil {
		return fmt.Errorf("register supplier bill number: %w", err)
	}
	if !found || registered != number || assigner != record.Claim.Supplier() {
		return fmt.Errorf("register supplier bill number: 主张 %s 已以另一个号登记", claim)
	}
	return nil
}

// LockBillNumber 以事务级咨询锁把同一主张、带号时还有同一供应商的同一账单号的接收排成一队（见 ports.BillReceptionStore）。先主张后
// 号，次序固定：每笔接收各只拿一个主张与至多一个号，按同一次序取就不会互等成环。
func (repository *BillReceptions) LockBillNumber(
	ctx context.Context,
	tenant domain.TenantID,
	supplier domain.SupplierPartyReference,
	number domain.SupplierBillNumber,
	claim domain.BillClaimID,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("lock supplier bill reception: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('settlement_accounting.supplier_bill_reception.claim'), hashtext($1 || chr(31) || $2))`,
		tenant.String(), claim.String()); err != nil {
		return fmt.Errorf("lock supplier bill reception: %w", err)
	}
	if number.String() == "" {
		return nil
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('settlement_accounting.supplier_bill_number'), hashtext($1 || chr(31) || $2 || chr(31) || $3))`,
		tenant.String(), supplier.String(), number.String()); err != nil {
		return fmt.Errorf("lock supplier bill reception: %w", err)
	}
	return nil
}

// FindBillNumberHolder 答（供应商，账单号）标识的是哪份主张。
func (repository *BillReceptions) FindBillNumberHolder(
	ctx context.Context,
	tenant domain.TenantID,
	supplier domain.SupplierPartyReference,
	number domain.SupplierBillNumber,
) (domain.BillClaimID, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.BillClaimID{}, false, fmt.Errorf("find supplier bill number holder: %w", err)
	}
	var holder string
	err = querier.QueryRow(ctx,
		`SELECT claim_id FROM settlement_accounting.supplier_bill_number
		  WHERE tenant_id = $1 AND supplier_ref = $2 AND bill_number = $3`,
		tenant.String(), supplier.String(), number.String(),
	).Scan(&holder)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.BillClaimID{}, false, nil
	}
	if err != nil {
		return domain.BillClaimID{}, false, fmt.Errorf("find supplier bill number holder: %w", err)
	}
	claim, err := domain.NewBillClaimID(holder)
	if err != nil {
		return domain.BillClaimID{}, false, fmt.Errorf("find supplier bill number holder: %w", err)
	}
	return claim, true, nil
}

// FindClaimSupplier 答一份主张最早落库的那一版是哪家供应商的。
func (repository *BillReceptions) FindClaimSupplier(
	ctx context.Context,
	tenant domain.TenantID,
	claim domain.BillClaimID,
) (domain.SupplierPartyReference, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.SupplierPartyReference{}, false, fmt.Errorf("find claim supplier: %w", err)
	}
	var raw string
	err = querier.QueryRow(ctx,
		`SELECT supplier_ref FROM settlement_accounting.supplier_bill_reception
		  WHERE tenant_id = $1 AND claim_id = $2
		  ORDER BY recorded_at, claim_version
		  LIMIT 1`,
		tenant.String(), claim.String(),
	).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SupplierPartyReference{}, false, nil
	}
	if err != nil {
		return domain.SupplierPartyReference{}, false, fmt.Errorf("find claim supplier: %w", err)
	}
	supplier, err := domain.NewSupplierPartyReference(raw)
	if err != nil {
		return domain.SupplierPartyReference{}, false, fmt.Errorf("find claim supplier: %w", err)
	}
	return supplier, true, nil
}

// FindClaimBillNumber 答一份主张已以哪个号登记、号的分配方是哪家供应商。
func (repository *BillReceptions) FindClaimBillNumber(
	ctx context.Context,
	tenant domain.TenantID,
	claim domain.BillClaimID,
) (domain.SupplierPartyReference, domain.SupplierBillNumber, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.SupplierPartyReference{}, domain.SupplierBillNumber{}, false, fmt.Errorf("find claim bill number: %w", err)
	}
	assigner, number, found, err := findClaimBillNumber(ctx, querier, tenant, claim)
	if err != nil {
		return domain.SupplierPartyReference{}, domain.SupplierBillNumber{}, false, fmt.Errorf("find claim bill number: %w", err)
	}
	return assigner, number, found, nil
}

func findClaimBillNumber(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	claim domain.BillClaimID,
) (domain.SupplierPartyReference, domain.SupplierBillNumber, bool, error) {
	var rawSupplier, rawNumber string
	err := querier.QueryRow(ctx,
		`SELECT supplier_ref, bill_number FROM settlement_accounting.supplier_bill_number WHERE tenant_id = $1 AND claim_id = $2`,
		tenant.String(), claim.String(),
	).Scan(&rawSupplier, &rawNumber)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SupplierPartyReference{}, domain.SupplierBillNumber{}, false, nil
	}
	if err != nil {
		return domain.SupplierPartyReference{}, domain.SupplierBillNumber{}, false, err
	}
	assigner, err := domain.NewSupplierPartyReference(rawSupplier)
	if err != nil {
		return domain.SupplierPartyReference{}, domain.SupplierBillNumber{}, false, err
	}
	number, err := domain.NewSupplierBillNumber(rawNumber)
	if err != nil {
		return domain.SupplierPartyReference{}, domain.SupplierBillNumber{}, false, err
	}
	return assigner, number, true, nil
}

type billClaimColumns struct {
	supplier    string
	billNumber  string
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
	if columns.billNumber != "" {
		if spec.BillNumber, err = domain.NewSupplierBillNumber(columns.billNumber); err != nil {
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
