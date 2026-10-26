package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrEvaluationRequestDigestDrift 说明库里一行的合格来源引用摘要与按行上成分重算出来的不一致。
var ErrEvaluationRequestDigestDrift = errors.New("settlement accounting postgres: evaluation request source digest drifted")

// EvaluationRequests 实现 ports.EvaluationRequestRegistry（登记面）、ports.EvaluationRequestView（只读半边）与停因、
// 零额两只写口，同一张表、同一份行模型——分开实现就是第二处定义。
//
// 幂等由库上的唯一约束守：主键（租户+铸造 ID）拦同一份重放，（自然键、代次）拦另一个 ID 带同一组成分在同一代再来
// （票 sa-cc/08 裁决 2；自然键是租户+主要范围+计算目的+引用集合摘要），前一代上的唯一约束拦同一份请求的第二个
// 下一代（ADR-0229 决定一）。撞上哪一条都答`已存在`、不覆盖先到者（写入代数同 ADR-0031）。
type EvaluationRequests struct {
	db *bentopg.DB
}

func NewEvaluationRequests(db *bentopg.DB) (*EvaluationRequests, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &EvaluationRequests{db: db}, nil
}

var (
	_ ports.EvaluationRequestRegistry          = (*EvaluationRequests)(nil)
	_ ports.EvaluationRequestView              = (*EvaluationRequests)(nil)
	_ ports.EvaluationRequestStopRecorder      = (*EvaluationRequests)(nil)
	_ ports.EvaluationRequestZeroTotalRecorder = (*EvaluationRequests)(nil)
	_ ports.EvaluationRequestSuccessorView     = (*EvaluationRequests)(nil)
)

// Save 登记一份评价请求。撞主键或撞自然键唯一约束都交回`已存在`而不是错误：那是业务答案，编排据以按
// 自然键读回先到者、交回原 ID。
//
// 写之前先核键与对象说的是同一件事（照 CC SaveVerification 的形）：主键列来自 record.Key、内容列来自
// record.Request，两者之间库上没有约束能拦——键指 A、对象是 B 的一份会落成「按 A 查出来内容是 B」的行，
// 读口按内容列重建时交回的 Key 与调用方当初给的键就对不上。零值对象的 ID 与零值键相等，单比相等拦不住
// 它，所以键的两维先各核非空。
func (registry *EvaluationRequests) Save(
	ctx context.Context,
	record ports.EvaluationRequestRecord,
) (ports.EvaluationRequestSaveOutcome, error) {
	if strings.TrimSpace(record.Key.TenantID.String()) == "" || strings.TrimSpace(record.Key.Request.String()) == "" {
		return ports.EvaluationRequestSaveOutcomeInvalid,
			fmt.Errorf("save evaluation request: the tenant or the request ID in the key is blank")
	}
	if record.Key.Request != record.Request.ID() {
		return ports.EvaluationRequestSaveOutcomeInvalid,
			fmt.Errorf("save evaluation request: key %q disagrees with the request %q it claims to index",
				record.Key.Request.String(), record.Request.ID().String())
	}
	if record.StopCause != domain.EvaluationStopCauseInvalid || record.ZeroTotal {
		return ports.EvaluationRequestSaveOutcomeInvalid,
			fmt.Errorf("save evaluation request: a registration carries no stop cause and no zero total, got %q and zero total %v",
				record.StopCause.String(), record.ZeroTotal)
	}

	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.EvaluationRequestSaveOutcomeInvalid, fmt.Errorf("save evaluation request: %w", err)
	}

	request := record.Request
	sources := request.Sources()
	var previous, basis *string
	if id, has := request.Previous(); has {
		value := id.String()
		previous = &value
	}
	if resume := request.ResumeBasis(); resume.Present() {
		value := resume.String()
		basis = &value
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.evaluation_request
			(tenant_id, request_id, scope_ref, purpose,
			 occurrence_id, occurrence_reason, occurrence_version, occurred_at,
			 fee_item, agreement_ref, source_digest,
			 requested_at, requested_by, recorded_at,
			 generation, previous_request_id, resume_basis)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Request.String(),
		request.Scope().String(),
		request.Purpose().String(),
		sources.Occurrence.ID().String(),
		sources.Occurrence.Reason().String(),
		sources.Occurrence.Version().String(),
		sources.Occurrence.OccurredAt().UTC(),
		sources.FeeItem.String(),
		sources.Agreement.String(),
		request.NaturalKey().SourceDigest,
		request.RequestedAt().UTC(),
		request.RequestedBy().String(),
		record.RecordedAt.UTC(),
		request.Generation(),
		previous,
		basis,
	)
	if err != nil {
		return ports.EvaluationRequestSaveOutcomeInvalid, fmt.Errorf("save evaluation request: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.EvaluationRequestAlreadyRequested, nil
	}
	return ports.EvaluationRequestSaved, nil
}

// FindByID 按（租户+铸造 ID）取回。否定结果不区分「不存在」与「属于另一个租户」：租户条件写在 SQL 里，
// 越权探到的与真不存在长得一样（ADR-0029 同款）。
func (registry *EvaluationRequests) FindByID(
	ctx context.Context,
	key ports.EvaluationRequestKey,
) (ports.EvaluationRequestRecord, bool, error) {
	return registry.findOne(ctx, key.TenantID,
		`WHERE tenant_id = $1 AND request_id = $2`,
		key.TenantID.String(), key.Request.String())
}

// FindByNaturalKey 按（租户+自然键三成分）取回这条代次链上最新的那一代。编排在 Save 答`已存在`时走这里读回赢家。
func (registry *EvaluationRequests) FindByNaturalKey(
	ctx context.Context,
	tenant domain.TenantID,
	key domain.EvaluationRequestNaturalKey,
) (ports.EvaluationRequestRecord, bool, error) {
	return registry.findOne(ctx, tenant,
		`WHERE tenant_id = $1 AND scope_ref = $2 AND purpose = $3 AND source_digest = $4
		 ORDER BY generation DESC LIMIT 1`,
		tenant.String(), key.Scope.String(), key.Purpose.String(), key.SourceDigest)
}

// FindSuccessor 按前一代取回下一代。前一代上的唯一约束保证至多一份。
func (registry *EvaluationRequests) FindSuccessor(
	ctx context.Context,
	key ports.EvaluationRequestKey,
) (ports.EvaluationRequestRecord, bool, error) {
	return registry.findOne(ctx, key.TenantID,
		`WHERE tenant_id = $1 AND previous_request_id = $2`,
		key.TenantID.String(), key.Request.String())
}

// RecordStop 给一份请求记停因（ADR-0229 决定三）。UPDATE 只认既没有停因、也没记零额的那一行：改得到就是写下了；改不到
// 就读回比，同一个停因答重放，另一个停因或零额答另有停因，都不覆盖。并发的两拍里后到者在行锁上等先到者提交，再按提交
// 后的行判。
func (registry *EvaluationRequests) RecordStop(
	ctx context.Context,
	key ports.EvaluationRequestKey,
	cause domain.EvaluationStopCause,
	stoppedAt time.Time,
) (ports.EvaluationRequestStopEffect, error) {
	none := ports.EvaluationRequestStopEffectInvalid
	if strings.TrimSpace(key.TenantID.String()) == "" || strings.TrimSpace(key.Request.String()) == "" {
		return none, fmt.Errorf("record evaluation request stop: the tenant or the request ID in the key is blank")
	}
	if cause.Stage() == domain.StopBeforeRequest || !cause.OccursIn(domain.BuyEvaluationDirection) || stoppedAt.IsZero() {
		return none, fmt.Errorf("record evaluation request stop: %q at %s is not a stop of a buy request", cause.String(), stoppedAt)
	}
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return none, fmt.Errorf("record evaluation request stop: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE settlement_accounting.evaluation_request
		    SET stop_cause = $3, stopped_at = $4
		  WHERE tenant_id = $1 AND request_id = $2 AND stop_cause IS NULL AND negative_end IS NULL`,
		key.TenantID.String(), key.Request.String(), cause.String(), stoppedAt.UTC())
	if err != nil {
		return none, fmt.Errorf("record evaluation request stop: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.EvaluationRequestStopped, nil
	}
	stored, found, err := registry.FindByID(ctx, key)
	if err != nil {
		return none, fmt.Errorf("record evaluation request stop: %w", err)
	}
	if !found {
		return none, fmt.Errorf("record evaluation request stop: request %s is not in the register", key.Request)
	}
	if !stored.ZeroTotal && stored.StopCause == cause {
		return ports.EvaluationRequestStopReplayed, nil
	}
	return ports.EvaluationRequestStoppedOtherwise, nil
}

// zeroTotalEnd 是`负向结束（零额）`在 negative_end 一列上的词，库上的 CHECK 只收它。
const zeroTotalEnd = "ZERO_TOTAL"

// RecordZeroTotal 给一份请求记`负向结束（零额）`（ADR-0306 决定三）。UPDATE 只认既没有停因、也没记零额的那一行：改得到
// 就是写下了；改不到就读回比，已是零额答重放，已有停因答已停下，都不覆盖。并发与 RecordStop 同一把行锁。
func (registry *EvaluationRequests) RecordZeroTotal(
	ctx context.Context,
	key ports.EvaluationRequestKey,
) (ports.EvaluationRequestZeroTotalEffect, error) {
	none := ports.EvaluationRequestZeroTotalEffectInvalid
	if strings.TrimSpace(key.TenantID.String()) == "" || strings.TrimSpace(key.Request.String()) == "" {
		return none, fmt.Errorf("record evaluation request zero total: the tenant or the request ID in the key is blank")
	}
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return none, fmt.Errorf("record evaluation request zero total: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE settlement_accounting.evaluation_request
		    SET negative_end = $3
		  WHERE tenant_id = $1 AND request_id = $2 AND stop_cause IS NULL AND negative_end IS NULL`,
		key.TenantID.String(), key.Request.String(), zeroTotalEnd)
	if err != nil {
		return none, fmt.Errorf("record evaluation request zero total: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.EvaluationRequestZeroTotalRecorded, nil
	}
	stored, found, err := registry.FindByID(ctx, key)
	if err != nil {
		return none, fmt.Errorf("record evaluation request zero total: %w", err)
	}
	if !found {
		return none, fmt.Errorf("record evaluation request zero total: request %s is not in the register", key.Request)
	}
	if stored.ZeroTotal {
		return ports.EvaluationRequestZeroTotalReplayed, nil
	}
	return ports.EvaluationRequestAlreadyStopped, nil
}

func (registry *EvaluationRequests) findOne(
	ctx context.Context,
	tenant domain.TenantID,
	where string,
	args ...any,
) (ports.EvaluationRequestRecord, bool, error) {
	querier, err := registry.db.ReadExecutor(ctx)
	if err != nil {
		return ports.EvaluationRequestRecord{}, false, fmt.Errorf("find evaluation request: %w", err)
	}

	var (
		requestID, scope, purpose                         string
		occurrenceID, occurrenceReason, occurrenceVersion string
		occurredAt, requestedAt, recordedAt               time.Time
		feeItem, agreement, digest, requestedBy           string
		generation                                        int
		previous, basis, cause, negativeEnd               *string
	)
	err = querier.QueryRow(ctx,
		`SELECT request_id, scope_ref, purpose,
		        occurrence_id, occurrence_reason, occurrence_version, occurred_at,
		        fee_item, agreement_ref, source_digest,
		        requested_at, requested_by, recorded_at,
		        generation, previous_request_id, resume_basis, stop_cause, negative_end
		   FROM settlement_accounting.evaluation_request `+where,
		args...,
	).Scan(&requestID, &scope, &purpose,
		&occurrenceID, &occurrenceReason, &occurrenceVersion, &occurredAt,
		&feeItem, &agreement, &digest,
		&requestedAt, &requestedBy, &recordedAt,
		&generation, &previous, &basis, &cause, &negativeEnd)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.EvaluationRequestRecord{}, false, nil
	}
	if err != nil {
		return ports.EvaluationRequestRecord{}, false, fmt.Errorf("find evaluation request: %w", err)
	}

	request, err := rebuildEvaluationRequest(evaluationRequestColumns{
		requestID:         requestID,
		scope:             scope,
		purpose:           purpose,
		occurrenceID:      occurrenceID,
		occurrenceReason:  occurrenceReason,
		occurrenceVersion: occurrenceVersion,
		occurredAt:        occurredAt,
		feeItem:           feeItem,
		agreement:         agreement,
		digest:            digest,
		requestedAt:       requestedAt,
		requestedBy:       requestedBy,
		generation:        generation,
		previous:          derefText(previous),
		basis:             derefText(basis),
	})
	if err != nil {
		return ports.EvaluationRequestRecord{}, false, fmt.Errorf("find evaluation request: %w", err)
	}
	var stopCause domain.EvaluationStopCause
	if cause != nil {
		if stopCause, err = domain.EvaluationStopCauseFromName(*cause); err != nil {
			return ports.EvaluationRequestRecord{}, false, fmt.Errorf("find evaluation request: %w", err)
		}
	}
	// 库上的 CHECK 只收零额一个词、且与停因互斥；这里再核一次，只为迁移改坏约束时不静默。
	zeroTotal := negativeEnd != nil
	if zeroTotal && (*negativeEnd != zeroTotalEnd || cause != nil) {
		return ports.EvaluationRequestRecord{}, false, fmt.Errorf("find evaluation request: %s carries negative end %q with stop cause %v",
			requestID, *negativeEnd, cause != nil)
	}
	return ports.EvaluationRequestRecord{
		Key:        ports.EvaluationRequestKey{TenantID: tenant, Request: request.ID()},
		Request:    request,
		RecordedAt: recordedAt.UTC(),
		StopCause:  stopCause,
		ZeroTotal:  zeroTotal,
	}, true, nil
}

// evaluationRequestColumns 是一行评价请求的原样取值。列全部非空，没有指针列。
//
// 表上的 inserted_at 不在这里，也不在 Save 的列清单里：它是库侧审计列（这一行何时物理落库，DEFAULT now()
// 由库填），是本仓迁移的通形、一律不进领域。它与 recorded_at 不同义——recorded_at 是编排时钟给的登记
// 时刻、随记录往返、进 ports.EvaluationRequestRecord；两者相等是巧合不是规则。留着它不是为日后读回，
// 是让这张表与其余各表在库侧对得上号（票 sa-cc/16 做法 3 取 (b)）。
type evaluationRequestColumns struct {
	requestID         string
	scope             string
	purpose           string
	occurrenceID      string
	occurrenceReason  string
	occurrenceVersion string
	occurredAt        time.Time
	feeItem           string
	agreement         string
	digest            string
	requestedAt       time.Time
	requestedBy       string
	generation        int
	previous          string
	basis             string
}

// rebuildEvaluationRequest 把一行折回领域对象。请求没有独立的重建门：SubmitEvaluationRequest 从成分当场
// 算自然键、不相信任何输入，走它就是复验。读回后再把库里那份摘要与重算的比一次——自然键唯一约束在库上
// 比的是库里那一列的字节，若规范化换了号（ESRC-1 → 下一版）而旧行未迁，重算出的自然键与库上守着的
// 就不再是同一个键，编排按新键找不到先到者、Save 却撞旧行答`已存在`，两口自相矛盾。这里响亮报错，
// 不让它静默。
func rebuildEvaluationRequest(columns evaluationRequestColumns) (domain.EvaluationRequest, error) {
	id, err := domain.NewEvaluationRequestID(columns.requestID)
	if err != nil {
		return domain.EvaluationRequest{}, err
	}
	scope, err := domain.NewPrimaryScopeReference(columns.scope)
	if err != nil {
		return domain.EvaluationRequest{}, err
	}
	purpose, err := domain.ParseCalculationPurpose(columns.purpose)
	if err != nil {
		return domain.EvaluationRequest{}, err
	}
	occurrence, err := rebuildChargeOccurrence(
		columns.occurrenceID, columns.occurrenceReason, columns.occurrenceVersion, columns.occurredAt)
	if err != nil {
		return domain.EvaluationRequest{}, err
	}
	feeItem, err := domain.NewFeeItemReference(columns.feeItem)
	if err != nil {
		return domain.EvaluationRequest{}, err
	}
	agreement, err := domain.NewSupplierAgreementReference(columns.agreement)
	if err != nil {
		return domain.EvaluationRequest{}, err
	}
	requestedBy, err := domain.NewRequesterReference(columns.requestedBy)
	if err != nil {
		return domain.EvaluationRequest{}, err
	}
	var previous domain.EvaluationRequestID
	if columns.previous != "" {
		if previous, err = domain.NewEvaluationRequestID(columns.previous); err != nil {
			return domain.EvaluationRequest{}, err
		}
	}
	basis, err := rebuildResumeBasis(columns.basis)
	if err != nil {
		return domain.EvaluationRequest{}, err
	}
	request, err := domain.SubmitEvaluationRequest(domain.EvaluationRequestSpec{
		ID:      id,
		Scope:   scope,
		Purpose: purpose,
		Sources: domain.EligibleSourceReferences{
			Occurrence: occurrence,
			FeeItem:    feeItem,
			Agreement:  agreement,
		},
		RequestedAt: columns.requestedAt,
		RequestedBy: requestedBy,
		Generation:  columns.generation,
		Previous:    previous,
		ResumeBasis: basis,
	})
	if err != nil {
		return domain.EvaluationRequest{}, err
	}
	if request.NaturalKey().SourceDigest != columns.digest {
		return domain.EvaluationRequest{}, fmt.Errorf("%w: %s stored %q, recomputed %q",
			ErrEvaluationRequestDigestDrift, columns.requestID, columns.digest, request.NaturalKey().SourceDigest)
	}
	return request, nil
}
