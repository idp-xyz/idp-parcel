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

// DutyPaymentVerificationAdoptions 实现 ports.DutyPaymentVerificationAdoptionStore（写入代数同 ADR-0031）：
// 同一版核对只采用一次，键是提供方核对版本的完整引用。表上只有引用与采用时刻（迁移 0020 自注）。
type DutyPaymentVerificationAdoptions struct {
	db *bentopg.DB
}

func NewDutyPaymentVerificationAdoptions(db *bentopg.DB) (*DutyPaymentVerificationAdoptions, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &DutyPaymentVerificationAdoptions{db: db}, nil
}

var _ ports.DutyPaymentVerificationAdoptionStore = (*DutyPaymentVerificationAdoptions)(nil)

// FindByKey 按（租户 + 引用四维）取回。否定结果只回 false。读回经公开构造门复验引用与时刻的形状。
func (repository *DutyPaymentVerificationAdoptions) FindByKey(
	ctx context.Context,
	key ports.DutyPaymentVerificationAdoptionKey,
) (ports.DutyPaymentVerificationAdoptionRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.DutyPaymentVerificationAdoptionRecord{}, false, fmt.Errorf("find duty payment verification adoption: %w", err)
	}

	verification := key.Verification
	var adoptedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT adopted_at
		   FROM settlement_accounting.duty_payment_verification_adoption
		  WHERE tenant_id = $1
		    AND scope_ref = $2
		    AND duty_ref = $3
		    AND funds_ref = $4
		    AND version_digest = $5`,
		key.TenantID.String(),
		verification.Scope().String(),
		verification.Duty().String(),
		verification.Funds().String(),
		verification.Version().String(),
	).Scan(&adoptedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.DutyPaymentVerificationAdoptionRecord{}, false, nil
	}
	if err != nil {
		return ports.DutyPaymentVerificationAdoptionRecord{}, false, fmt.Errorf("find duty payment verification adoption: %w", err)
	}

	adoption, err := domain.AdoptDutyPaymentVerification(verification, adoptedAt)
	if err != nil {
		return ports.DutyPaymentVerificationAdoptionRecord{}, false, fmt.Errorf("find duty payment verification adoption: %w", err)
	}
	return ports.DutyPaymentVerificationAdoptionRecord{Key: key, Adoption: adoption}, true, nil
}

// Save 写下一次采用。同引用已有记录时答`已采用`，不覆盖先到者——也没有可覆盖的内容，键就是全部。
func (repository *DutyPaymentVerificationAdoptions) Save(
	ctx context.Context,
	record ports.DutyPaymentVerificationAdoptionRecord,
) (ports.DutyPaymentVerificationAdoptionSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DutyPaymentVerificationAdoptionSaveOutcomeInvalid, fmt.Errorf("save duty payment verification adoption: %w", err)
	}

	verification := record.Adoption.Verification()
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.duty_payment_verification_adoption
			(tenant_id, scope_ref, duty_ref, funds_ref, version_digest, adopted_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		verification.Scope().String(),
		verification.Duty().String(),
		verification.Funds().String(),
		verification.Version().String(),
		record.Adoption.AdoptedAt().UTC(),
	)
	if err != nil {
		return ports.DutyPaymentVerificationAdoptionSaveOutcomeInvalid, fmt.Errorf("save duty payment verification adoption: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.DutyPaymentVerificationAlreadyAdopted, nil
	}
	return ports.DutyPaymentVerificationAdoptionSaved, nil
}
