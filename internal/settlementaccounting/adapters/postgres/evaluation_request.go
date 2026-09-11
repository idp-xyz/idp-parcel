package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrEvaluationRequestDigestDrift 说明库里一行的合格来源引用摘要与按行上成分重算出来的不一致。
var ErrEvaluationRequestDigestDrift = errors.New("settlement accounting postgres: evaluation request source digest drifted")

// EvaluationRequests 实现 ports.EvaluationRequestRegistry（登记面）与 ports.EvaluationRequestView（只读
// 半边），同一张表、同一份行模型——分开实现就是第二处定义。
//
// 幂等由库上的两条唯一约束守：主键（租户+铸造 ID）拦同一份重放，自然键（租户+主要范围+计算目的+引用
// 集合摘要）拦另一个 ID 带同一组成分再来（票 sa-cc/08 裁决 2）。两条撞上都答`已存在`、不覆盖先到者
// （写入代数同 ADR-0031）。
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
	_ ports.EvaluationRequestRegistry = (*EvaluationRequests)(nil)
	_ ports.EvaluationRequestView     = (*EvaluationRequests)(nil)
)

// Save 登记一份评价请求。撞主键或撞自然键唯一约束都交回`已存在`而不是错误：那是业务答案，编排据以按
// 自然键读回先到者、交回原 ID。
func (registry *EvaluationRequests) Save(
	ctx context.Context,
	record ports.EvaluationRequestRecord,
) (ports.EvaluationRequestSaveOutcome, error) {
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.EvaluationRequestSaveOutcomeInvalid, fmt.Errorf("save evaluation request: %w", err)
	}

	request := record.Request
	sources := request.Sources()
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.evaluation_request
			(tenant_id, request_id, scope_ref, purpose,
			 occurrence_id, occurrence_reason, occurrence_version, occurred_at,
			 fee_item, agreement_ref, source_digest,
			 requested_at, requested_by, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
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

// FindByNaturalKey 按（租户+自然键三成分）取回先到者。编排在 Save 答`已存在`时走这里读回赢家。
func (registry *EvaluationRequests) FindByNaturalKey(
	ctx context.Context,
	tenant domain.TenantID,
	key domain.EvaluationRequestNaturalKey,
) (ports.EvaluationRequestRecord, bool, error) {
	return registry.findOne(ctx, tenant,
		`WHERE tenant_id = $1 AND scope_ref = $2 AND purpose = $3 AND source_digest = $4`,
		tenant.String(), key.Scope.String(), key.Purpose.String(), key.SourceDigest)
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
	)
	err = querier.QueryRow(ctx,
		`SELECT request_id, scope_ref, purpose,
		        occurrence_id, occurrence_reason, occurrence_version, occurred_at,
		        fee_item, agreement_ref, source_digest,
		        requested_at, requested_by, recorded_at
		   FROM settlement_accounting.evaluation_request `+where,
		args...,
	).Scan(&requestID, &scope, &purpose,
		&occurrenceID, &occurrenceReason, &occurrenceVersion, &occurredAt,
		&feeItem, &agreement, &digest,
		&requestedAt, &requestedBy, &recordedAt)
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
	})
	if err != nil {
		return ports.EvaluationRequestRecord{}, false, fmt.Errorf("find evaluation request: %w", err)
	}
	return ports.EvaluationRequestRecord{
		Key:        ports.EvaluationRequestKey{TenantID: tenant, Request: request.ID()},
		Request:    request,
		RecordedAt: recordedAt.UTC(),
	}, true, nil
}

// evaluationRequestColumns 是一行评价请求的原样取值。列全部非空，没有指针列。
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
