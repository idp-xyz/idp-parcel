package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

const preAdviceHeaderColumns = `tenant_id, pre_advice_id, owner_account_id, warehouse_code, arrival_mode, inbound_mode, status,
	document_number, numbering_absence_reason,
	submission_path, submission_source, request_key, evidence_reference,
	registrant, accepted_at`

type storedPreAdviceHeader struct {
	tenant     string
	id         string
	owner      string
	warehouse  string
	arrival    string
	inbound    string
	status     string
	number     *string
	absence    *string
	path       string
	source     string
	key        string
	evidence   *string
	registrant string
	acceptedAt time.Time
}

func loadPreAdviceWhere(ctx context.Context, querier bentopg.Querier, where string, args ...any) (domain.PreAdvice, bool, error) {
	header, found, err := scanPreAdviceHeader(querier.QueryRow(ctx,
		`SELECT `+preAdviceHeaderColumns+` FROM warehouse_fulfillment.inbound_pre_advice WHERE `+where, args...))
	if err != nil || !found {
		return domain.PreAdvice{}, found, err
	}
	goodsLines, err := loadGoodsLines(ctx, querier, header.tenant, header.id)
	if err != nil {
		return domain.PreAdvice{}, false, err
	}
	cartons, err := loadCartons(ctx, querier, header.tenant, header.id)
	if err != nil {
		return domain.PreAdvice{}, false, err
	}
	notices, err := loadNotices(ctx, querier, header.tenant, header.id)
	if err != nil {
		return domain.PreAdvice{}, false, err
	}
	withdrawal, err := loadWithdrawal(ctx, querier, header.tenant, header.id)
	if err != nil {
		return domain.PreAdvice{}, false, err
	}
	tenant, err := domain.NewTenantID(header.tenant)
	if err != nil {
		return domain.PreAdvice{}, false, err
	}
	id, err := domain.NewPreAdviceID(header.id)
	if err != nil {
		return domain.PreAdvice{}, false, err
	}
	owner, err := domain.NewOwnerAccountID(header.owner)
	if err != nil {
		return domain.PreAdvice{}, false, err
	}
	warehouse, err := domain.NewWarehouseCode(header.warehouse)
	if err != nil {
		return domain.PreAdvice{}, false, err
	}
	arrival, err := domain.NewArrivalMode(header.arrival)
	if err != nil {
		return domain.PreAdvice{}, false, err
	}
	inbound, err := domain.NewInboundMode(header.inbound)
	if err != nil {
		return domain.PreAdvice{}, false, err
	}
	status, err := domain.NewPreAdviceStatus(header.status)
	if err != nil {
		return domain.PreAdvice{}, false, err
	}
	number, absence, err := numberAndAbsence(header.number, header.absence)
	if err != nil {
		return domain.PreAdvice{}, false, err
	}
	submission, err := preAdviceSubmission(header.path, header.source, header.key, header.evidence)
	if err != nil {
		return domain.PreAdvice{}, false, err
	}
	registrant, err := domain.NewRegistrant(header.registrant)
	if err != nil {
		return domain.PreAdvice{}, false, err
	}
	advice, err := domain.RehydratePreAdvice(
		tenant, id, owner, warehouse, arrival, inbound, goodsLines, cartons, status,
		number, absence, submission, registrant, header.acceptedAt, notices, withdrawal,
	)
	if err != nil {
		return domain.PreAdvice{}, false, err
	}
	return advice, true, nil
}

func scanPreAdviceHeader(row rowScanner) (storedPreAdviceHeader, bool, error) {
	var header storedPreAdviceHeader
	err := row.Scan(
		&header.tenant, &header.id, &header.owner, &header.warehouse, &header.arrival, &header.inbound, &header.status,
		&header.number, &header.absence, &header.path, &header.source, &header.key, &header.evidence,
		&header.registrant, &header.acceptedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return storedPreAdviceHeader{}, false, nil
	}
	if err != nil {
		return storedPreAdviceHeader{}, false, err
	}
	return header, true, nil
}

func insertPreAdvice(ctx context.Context, executor bentopg.Executor, advice domain.PreAdvice) (bool, error) {
	number, absence := storedNumber(advice)
	path, source, key, evidence := submissionArgs(advice.Submission())
	var id string
	err := executor.QueryRow(ctx,
		`INSERT INTO warehouse_fulfillment.inbound_pre_advice (
		    tenant_id, pre_advice_id, owner_account_id, warehouse_code, arrival_mode, inbound_mode, status,
		    document_number, numbering_absence_reason,
		    submission_path, submission_source, request_key, evidence_reference,
		    registrant, accepted_at
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		 ON CONFLICT ON CONSTRAINT inbound_pre_advice_submission_key DO NOTHING
		 RETURNING pre_advice_id`,
		advice.Tenant().String(), advice.ID().String(), advice.Owner().String(), advice.Warehouse().String(),
		advice.Arrival().String(), advice.Inbound().String(), advice.Status().String(),
		number, absence, path, source, key, evidence, advice.Registrant().String(), advice.AcceptedAt(),
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	switch advice.Inbound() {
	case domain.InboundByGoods:
		for index, line := range advice.GoodsLines() {
			if err := insertGoodsLine(ctx, executor, advice, index, line); err != nil {
				return false, err
			}
		}
	case domain.InboundByCarton:
		for index, carton := range advice.Cartons() {
			if err := insertCarton(ctx, executor, advice, index, carton); err != nil {
				return false, err
			}
		}
	default:
		return false, fmt.Errorf("insert pre-advice: inbound mode %s", advice.Inbound())
	}
	notices := advice.Notices()
	if len(notices) != 1 {
		return false, errors.New("insert pre-advice: a new pre-advice carries its first notice only")
	}
	inserted, err := insertNotice(ctx, executor, advice.Tenant(), advice.ID(), notices[0])
	if err != nil {
		return false, err
	}
	if !inserted {
		return false, errors.New("insert pre-advice: the first notice key is already used")
	}
	return true, nil
}

func insertGoodsLine(ctx context.Context, executor bentopg.Executor, advice domain.PreAdvice, index int, line domain.GoodsInboundLine) error {
	_, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.inbound_pre_advice_goods_line (
		    tenant_id, pre_advice_id, line_index, goods_code, unit_name, quantity, batch_number, expiry_date, carton_count
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::date,$9)`,
		advice.Tenant().String(), advice.ID().String(), index, line.Goods().String(), line.Unit().String(), line.Quantity(),
		optionalText(line.Batch().String(), line.Batch().Given()),
		optionalText(line.Expiry().String(), line.Expiry().Given()),
		optionalCartonCount(line.CartonCount()),
	)
	return err
}

func insertCarton(ctx context.Context, executor bentopg.Executor, advice domain.PreAdvice, index int, carton domain.Carton) error {
	_, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.inbound_pre_advice_carton (
		    tenant_id, pre_advice_id, carton_index, carton_number
		 ) VALUES ($1,$2,$3,$4)`,
		advice.Tenant().String(), advice.ID().String(), index, carton.Number().String())
	if err != nil {
		return err
	}
	for lineIndex, line := range carton.Lines() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.inbound_pre_advice_carton_line (
			    tenant_id, pre_advice_id, carton_index, line_index, goods_code, quantity
			 ) VALUES ($1,$2,$3,$4,$5,$6)`,
			advice.Tenant().String(), advice.ID().String(), index, lineIndex, line.Goods().String(), line.Quantity(),
		); err != nil {
			return err
		}
	}
	return nil
}

func insertNotice(
	ctx context.Context,
	executor bentopg.Executor,
	tenant domain.TenantID,
	id domain.PreAdviceID,
	notice domain.PreAdviceNotice,
) (bool, error) {
	path, source, key, evidence := submissionArgs(notice.Submission())
	var version int
	err := executor.QueryRow(ctx,
		`INSERT INTO warehouse_fulfillment.inbound_pre_advice_notice (
		    tenant_id, pre_advice_id, version, expected_at, arrival_clue, owner_reference,
		    submission_path, submission_source, request_key, evidence_reference, registrant, recorded_at
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		 ON CONFLICT ON CONSTRAINT inbound_pre_advice_notice_submission_key DO NOTHING
		 RETURNING version`,
		tenant.String(), id.String(), notice.Version(), notice.ExpectedAt(),
		optionalText(notice.Clue().String(), notice.Clue().Given()),
		optionalText(notice.Reference().String(), notice.Reference().Given()),
		path, source, key, evidence, notice.Registrant().String(), notice.RecordedAt(),
	).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func insertWithdrawal(
	ctx context.Context,
	executor bentopg.Executor,
	tenant domain.TenantID,
	id domain.PreAdviceID,
	withdrawal domain.PreAdviceWithdrawal,
) (bool, error) {
	path, source, key, evidence := submissionArgs(withdrawal.Submission())
	var stored string
	err := executor.QueryRow(ctx,
		`INSERT INTO warehouse_fulfillment.inbound_pre_advice_withdrawal (
		    tenant_id, pre_advice_id, submission_path, submission_source, request_key, evidence_reference,
		    registrant, withdrawn_at
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 ON CONFLICT ON CONSTRAINT inbound_pre_advice_withdrawal_submission_key DO NOTHING
		 RETURNING pre_advice_id`,
		tenant.String(), id.String(), path, source, key, evidence, withdrawal.Registrant().String(), withdrawal.At(),
	).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func loadGoodsLines(ctx context.Context, querier bentopg.Querier, tenant, id string) ([]domain.GoodsInboundLine, error) {
	rows, err := querier.Query(ctx,
		`SELECT goods_code, unit_name, quantity, batch_number, expiry_date::text, carton_count
		   FROM warehouse_fulfillment.inbound_pre_advice_goods_line
		  WHERE tenant_id = $1 AND pre_advice_id = $2
		  ORDER BY line_index`,
		tenant, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lines []domain.GoodsInboundLine
	for rows.Next() {
		var goodsCode, unit string
		var quantity int64
		var batch, expiry *string
		var cartonCount *int64
		if err := rows.Scan(&goodsCode, &unit, &quantity, &batch, &expiry, &cartonCount); err != nil {
			return nil, err
		}
		line, err := goodsLineFrom(goodsCode, unit, quantity, batch, expiry, cartonCount)
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func loadCartons(ctx context.Context, querier bentopg.Querier, tenant, id string) ([]domain.Carton, error) {
	rows, err := querier.Query(ctx,
		`SELECT carton_index, carton_number
		   FROM warehouse_fulfillment.inbound_pre_advice_carton
		  WHERE tenant_id = $1 AND pre_advice_id = $2
		  ORDER BY carton_index`,
		tenant, id)
	if err != nil {
		return nil, err
	}
	type storedCarton struct {
		index  int
		number string
	}
	var stored []storedCarton
	for rows.Next() {
		var carton storedCarton
		if err := rows.Scan(&carton.index, &carton.number); err != nil {
			rows.Close()
			return nil, err
		}
		stored = append(stored, carton)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	lines, err := loadCartonLines(ctx, querier, tenant, id)
	if err != nil {
		return nil, err
	}
	cartons := make([]domain.Carton, 0, len(stored))
	for _, carton := range stored {
		number, err := domain.NewCartonNumber(carton.number)
		if err != nil {
			return nil, err
		}
		built, err := domain.NewCarton(number, lines[carton.index])
		if err != nil {
			return nil, err
		}
		cartons = append(cartons, built)
	}
	return cartons, nil
}

func loadCartonLines(ctx context.Context, querier bentopg.Querier, tenant, id string) (map[int][]domain.CartonContentLine, error) {
	rows, err := querier.Query(ctx,
		`SELECT carton_index, goods_code, quantity
		   FROM warehouse_fulfillment.inbound_pre_advice_carton_line
		  WHERE tenant_id = $1 AND pre_advice_id = $2
		  ORDER BY carton_index, line_index`,
		tenant, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lines := map[int][]domain.CartonContentLine{}
	for rows.Next() {
		var index int
		var goodsCode string
		var quantity int64
		if err := rows.Scan(&index, &goodsCode, &quantity); err != nil {
			return nil, err
		}
		goods, err := domain.NewGoodsCode(goodsCode)
		if err != nil {
			return nil, err
		}
		line, err := domain.NewCartonContentLine(goods, quantity)
		if err != nil {
			return nil, err
		}
		lines[index] = append(lines[index], line)
	}
	return lines, rows.Err()
}

func loadNotices(ctx context.Context, querier bentopg.Querier, tenant, id string) ([]domain.PreAdviceNotice, error) {
	rows, err := querier.Query(ctx,
		`SELECT version, expected_at, arrival_clue, owner_reference,
		        submission_path, submission_source, request_key, evidence_reference, registrant, recorded_at
		   FROM warehouse_fulfillment.inbound_pre_advice_notice
		  WHERE tenant_id = $1 AND pre_advice_id = $2
		  ORDER BY version`,
		tenant, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var notices []domain.PreAdviceNotice
	for rows.Next() {
		notice, err := scanNotice(rows)
		if err != nil {
			return nil, err
		}
		notices = append(notices, notice)
	}
	return notices, rows.Err()
}

func loadNoticeBySubmission(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	submission domain.PreAdviceSubmission,
) (domain.PreAdviceID, domain.PreAdviceNotice, bool, error) {
	var preAdviceID string
	var version int
	var expectedAt, recordedAt time.Time
	var clue, reference, evidence *string
	var path, source, key, registrant string
	err := querier.QueryRow(ctx,
		`SELECT pre_advice_id, version, expected_at, arrival_clue, owner_reference,
		        submission_path, submission_source, request_key, evidence_reference, registrant, recorded_at
		   FROM warehouse_fulfillment.inbound_pre_advice_notice
		  WHERE tenant_id = $1 AND submission_path = $2 AND submission_source = $3 AND request_key = $4`,
		tenant.String(), submission.Path().String(), submissionSource(submission), submission.Key().String(),
	).Scan(&preAdviceID, &version, &expectedAt, &clue, &reference, &path, &source, &key, &evidence, &registrant, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PreAdviceID{}, domain.PreAdviceNotice{}, false, nil
	}
	if err != nil {
		return domain.PreAdviceID{}, domain.PreAdviceNotice{}, false, err
	}
	id, err := domain.NewPreAdviceID(preAdviceID)
	if err != nil {
		return domain.PreAdviceID{}, domain.PreAdviceNotice{}, false, err
	}
	notice, err := noticeFrom(version, expectedAt, clue, reference, path, source, key, evidence, registrant, recordedAt)
	if err != nil {
		return domain.PreAdviceID{}, domain.PreAdviceNotice{}, false, err
	}
	return id, notice, true, nil
}

func loadWithdrawal(ctx context.Context, querier bentopg.Querier, tenant, id string) (domain.PreAdviceWithdrawal, error) {
	var path, source, key, registrant string
	var evidence *string
	var at time.Time
	err := querier.QueryRow(ctx,
		`SELECT submission_path, submission_source, request_key, evidence_reference, registrant, withdrawn_at
		   FROM warehouse_fulfillment.inbound_pre_advice_withdrawal
		  WHERE tenant_id = $1 AND pre_advice_id = $2`,
		tenant, id).Scan(&path, &source, &key, &evidence, &registrant, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PreAdviceWithdrawal{}, nil
	}
	if err != nil {
		return domain.PreAdviceWithdrawal{}, err
	}
	submission, err := preAdviceSubmission(path, source, key, evidence)
	if err != nil {
		return domain.PreAdviceWithdrawal{}, err
	}
	who, err := domain.NewRegistrant(registrant)
	if err != nil {
		return domain.PreAdviceWithdrawal{}, err
	}
	return domain.NewPreAdviceWithdrawal(submission, who, at)
}

func scanNotice(row rowScanner) (domain.PreAdviceNotice, error) {
	var version int
	var expectedAt, recordedAt time.Time
	var clue, reference, evidence *string
	var path, source, key, registrant string
	if err := row.Scan(&version, &expectedAt, &clue, &reference, &path, &source, &key, &evidence, &registrant, &recordedAt); err != nil {
		return domain.PreAdviceNotice{}, err
	}
	return noticeFrom(version, expectedAt, clue, reference, path, source, key, evidence, registrant, recordedAt)
}

func noticeFrom(
	version int,
	expectedAt time.Time,
	clue, reference *string,
	path, source, key string,
	evidence *string,
	registrant string,
	recordedAt time.Time,
) (domain.PreAdviceNotice, error) {
	arrivalClue, err := optionalClue(clue)
	if err != nil {
		return domain.PreAdviceNotice{}, err
	}
	ownerReference, err := optionalReference(reference)
	if err != nil {
		return domain.PreAdviceNotice{}, err
	}
	submission, err := preAdviceSubmission(path, source, key, evidence)
	if err != nil {
		return domain.PreAdviceNotice{}, err
	}
	who, err := domain.NewRegistrant(registrant)
	if err != nil {
		return domain.PreAdviceNotice{}, err
	}
	return domain.NewPreAdviceNotice(version, expectedAt, arrivalClue, ownerReference, submission, who, recordedAt)
}

func goodsLineFrom(goodsCode, unit string, quantity int64, batch, expiry *string, cartonCount *int64) (domain.GoodsInboundLine, error) {
	goods, err := domain.NewGoodsCode(goodsCode)
	if err != nil {
		return domain.GoodsInboundLine{}, err
	}
	unitName, err := domain.NewGoodsUnitName(unit)
	if err != nil {
		return domain.GoodsInboundLine{}, err
	}
	var batchNumber domain.BatchNumber
	if batch != nil {
		batchNumber, err = domain.NewBatchNumber(*batch)
		if err != nil {
			return domain.GoodsInboundLine{}, err
		}
	}
	var expiryDate domain.ExpiryDate
	if expiry != nil {
		expiryDate, err = domain.NewExpiryDate(*expiry)
		if err != nil {
			return domain.GoodsInboundLine{}, err
		}
	}
	var count domain.CartonCount
	if cartonCount != nil {
		count = domain.GivenCartonCount(*cartonCount)
	}
	return domain.NewGoodsInboundLine(goods, unitName, quantity, batchNumber, expiryDate, count)
}

func preAdviceSubmission(path, source, key string, evidence *string) (domain.PreAdviceSubmission, error) {
	requestKey, err := domain.NewRequestKey(key)
	if err != nil {
		return domain.PreAdviceSubmission{}, err
	}
	switch path {
	case domain.SubmissionByOwner.String():
		named, err := domain.NewSubmissionSource(source)
		if err != nil {
			return domain.PreAdviceSubmission{}, err
		}
		return domain.NewOwnerPreAdviceSubmission(named, requestKey)
	case domain.SubmissionOnBehalf.String():
		if evidence == nil {
			return domain.PreAdviceSubmission{}, errors.New("on-behalf submission has no evidence reference")
		}
		reference, err := domain.NewEvidenceReference(*evidence)
		if err != nil {
			return domain.PreAdviceSubmission{}, err
		}
		return domain.NewOnBehalfPreAdviceSubmission(requestKey, reference)
	default:
		return domain.PreAdviceSubmission{}, fmt.Errorf("submission path %q", path)
	}
}

func submissionArgs(submission domain.PreAdviceSubmission) (path, source, key string, evidence any) {
	path = submission.Path().String()
	source = submissionSource(submission)
	key = submission.Key().String()
	if reference, ok := submission.EvidenceReference(); ok {
		evidence = reference.String()
	}
	return path, source, key, evidence
}

func storedNumber(advice domain.PreAdvice) (number, absence any) {
	if assigned, ok := advice.DocumentNumber(); ok {
		return assigned.String(), nil
	}
	return nil, advice.NumberingAbsence().String()
}

func numberAndAbsence(number, absence *string) (domain.DocumentNumber, domain.NumberingAbsence, error) {
	if number != nil {
		parsed, err := domain.NewDocumentNumber(*number)
		return parsed, domain.NumberingAbsenceNone, err
	}
	if absence != nil && *absence == domain.NumberingTimeZoneNotConfigured.String() {
		return domain.DocumentNumber{}, domain.NumberingTimeZoneNotConfigured, nil
	}
	return domain.DocumentNumber{}, 0, errors.New("pre-advice number shape")
}

func optionalClue(value *string) (domain.ArrivalClue, error) {
	if value == nil {
		return domain.ArrivalClue{}, nil
	}
	return domain.NewArrivalClue(*value)
}

func optionalReference(value *string) (domain.OwnerReference, error) {
	if value == nil {
		return domain.OwnerReference{}, nil
	}
	return domain.NewOwnerReference(*value)
}

func optionalCartonCount(count domain.CartonCount) any {
	if !count.Given() {
		return nil
	}
	return count.Value()
}
