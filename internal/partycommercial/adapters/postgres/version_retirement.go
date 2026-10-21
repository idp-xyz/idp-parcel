package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 同起点退役册的持久化面（ADR-0233，迁移 0040），以及按版本点读、装载授权授予与合同委派（ADR-0261）时把版本层的收尾
// （最新一条区间更正、退役）折进来的那一段。整册装载自己聚全部更正（LoadForScope），其余只要最新的一条：选用区间取
// 登记顺序上的最后一条（ADR-0056）。

// SaveVersionRetirement 登记一次同起点退役。一版只退一次：主键撞上即读回比对，同一个决定（同一后继、同一决定引用）答
// `已登记`，异内容答`内容冲突`，原行不动。退役时刻不进比对，理由同 domain.sameRetirementDecision。
func (repository *CommercialPublications) SaveVersionRetirement(
	ctx context.Context,
	retirement domain.VersionRetirement,
) (ports.VersionRetirementSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.VersionRetirementSaveOutcomeInvalid, fmt.Errorf("save version retirement: %w", err)
	}
	if retirement.Reference().String() == "" || retirement.Successor().String() == "" || retirement.RetiredAt().IsZero() {
		return ports.VersionRetirementSaveOutcomeInvalid,
			fmt.Errorf("save version retirement: 决定引用、后继或退役时刻缺失，未经 RetireInFavourOf 构造的退役不入册")
	}

	var inserted bool
	err = executor.QueryRow(ctx,
		`WITH inserted AS (
			INSERT INTO party_commercial.commercial_version_retirement
				(tenant_id, object_kind, object_id, version_label, successor_label, retirement_ref, retired_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT ON CONSTRAINT commercial_version_retirement_pkey DO NOTHING
			RETURNING 1
		)
		SELECT EXISTS (SELECT 1 FROM inserted)`,
		retirement.Tenant().String(),
		uint8(retirement.Kind()),
		retirement.ObjectID().String(),
		retirement.Version().String(),
		retirement.Successor().String(),
		retirement.Reference().String(),
		retirement.RetiredAt().UTC(),
	).Scan(&inserted)
	if err != nil {
		return ports.VersionRetirementSaveOutcomeInvalid, fmt.Errorf("save version retirement: %w", err)
	}
	if inserted {
		return ports.VersionRetirementSaved, nil
	}

	var successor, reference string
	err = executor.QueryRow(ctx,
		`SELECT successor_label, retirement_ref
		   FROM party_commercial.commercial_version_retirement
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		retirement.Tenant().String(),
		uint8(retirement.Kind()),
		retirement.ObjectID().String(),
		retirement.Version().String(),
	).Scan(&successor, &reference)
	if err != nil {
		return ports.VersionRetirementSaveOutcomeInvalid, fmt.Errorf("save version retirement: read back: %w", err)
	}
	if successor == retirement.Successor().String() && reference == retirement.Reference().String() {
		return ports.VersionRetirementAlreadyRegistered, nil
	}
	return ports.VersionRetirementContentConflict, nil
}

// versionClosureJoins 是把收尾折进来的两段连接：最新一条区间更正与退役。alias 是外层查询里带版本四元组那一行的别名；
// 与所折的行同一条语句取回，理由同 LoadSupplierAgreementVersion 头注。
func versionClosureJoins(alias string) string {
	return `LEFT JOIN LATERAL (
		        SELECT c.corrected_starts_at, c.corrected_ends_at, c.correction_ref, c.corrected_at
		          FROM party_commercial.commercial_validity_correction AS c
		         WHERE c.tenant_id     = ` + alias + `.tenant_id
		           AND c.object_kind   = ` + alias + `.object_kind
		           AND c.object_id     = ` + alias + `.object_id
		           AND c.version_label = ` + alias + `.version_label
		         ORDER BY c.registration_id DESC
		         LIMIT 1
		   ) AS latest_correction ON TRUE
		   LEFT JOIN party_commercial.commercial_version_retirement AS retirement
		          ON retirement.tenant_id     = ` + alias + `.tenant_id
		         AND retirement.object_kind   = ` + alias + `.object_kind
		         AND retirement.object_id     = ` + alias + `.object_id
		         AND retirement.version_label = ` + alias + `.version_label`
}

// versionClosureColumns 是与 versionClosureJoins 配对的投影，顺序即 scannedVersionClosure.targets 的顺序。
const versionClosureColumns = `latest_correction.corrected_starts_at, latest_correction.corrected_ends_at,
		        latest_correction.correction_ref, latest_correction.corrected_at,
		        retirement.successor_label, retirement.retirement_ref, retirement.retired_at`

// retirementColumns 只取退役那三列，给自己另聚更正的整册装载用。
const retirementColumns = `retirement.successor_label, retirement.retirement_ref, retirement.retired_at`

// retirementJoin 只连退役，给整册装载用。
func retirementJoin(alias string) string {
	return `LEFT JOIN party_commercial.commercial_version_retirement AS retirement
		          ON retirement.tenant_id     = ` + alias + `.tenant_id
		         AND retirement.object_kind   = ` + alias + `.object_kind
		         AND retirement.object_id     = ` + alias + `.object_id
		         AND retirement.version_label = ` + alias + `.version_label`
}

type scannedVersionClosure struct {
	correctedStartsAt *time.Time
	correctedEndsAt   *time.Time
	correctionRef     *string
	correctedAt       *time.Time
	scannedRetirement
}

type scannedRetirement struct {
	successor     *string
	retirementRef *string
	retiredAt     *time.Time
}

func (closure *scannedVersionClosure) targets() []any {
	return append([]any{&closure.correctedStartsAt, &closure.correctedEndsAt, &closure.correctionRef, &closure.correctedAt},
		closure.scannedRetirement.targets()...)
}

func (retirement *scannedRetirement) targets() []any {
	return []any{&retirement.successor, &retirement.retirementRef, &retirement.retiredAt}
}

// correctionFor 把最新一条更正折回领域对象；没有更正答 false。有行却立不起来是坏数据，整次上抛——跳过它会让点读按
// 原区间放行一版已被截止的版本。
func (closure scannedVersionClosure) correctionFor(version domain.CommercialVersion) (domain.ValidityCorrection, bool, error) {
	if closure.correctionRef == nil {
		return domain.ValidityCorrection{}, false, nil
	}
	if closure.correctedStartsAt == nil || closure.correctedAt == nil {
		return domain.ValidityCorrection{}, false, errors.New("区间更正行不完整")
	}
	reference, err := domain.NewValidityCorrectionReference(*closure.correctionRef)
	if err != nil {
		return domain.ValidityCorrection{}, false, err
	}
	interval, err := intervalFrom(*closure.correctedStartsAt, closure.correctedEndsAt)
	if err != nil {
		return domain.ValidityCorrection{}, false, err
	}
	correction, err := version.CorrectEffectiveInterval(interval, reference, *closure.correctedAt)
	if err != nil {
		return domain.ValidityCorrection{}, false, err
	}
	return correction, true, nil
}

// retirementFor 把退役行折回领域对象；没有退役答 false。version 必须是快照重建出来、尚未施加退役的那一版（已生效）。
func (retirement scannedRetirement) retirementFor(version domain.CommercialVersion) (domain.VersionRetirement, bool, error) {
	if retirement.retirementRef == nil {
		return domain.VersionRetirement{}, false, nil
	}
	if retirement.successor == nil || retirement.retiredAt == nil {
		return domain.VersionRetirement{}, false, errors.New("退役行不完整")
	}
	successor, err := domain.NewCommercialVersionLabel(*retirement.successor)
	if err != nil {
		return domain.VersionRetirement{}, false, err
	}
	reference, err := domain.NewRetirementReference(*retirement.retirementRef)
	if err != nil {
		return domain.VersionRetirement{}, false, err
	}
	built, err := version.RetireInFavourOf(successor, reference, *retirement.retiredAt)
	if err != nil {
		return domain.VersionRetirement{}, false, err
	}
	return built, true, nil
}

// retiredShell 把退役施加到一版重建出来的壳上，交回`已退役`的那一版；没有退役原样交回。
func (retirement scannedRetirement) retiredShell(version domain.CommercialVersion) (domain.CommercialVersion, error) {
	built, retired, err := retirement.retirementFor(version)
	if err != nil || !retired {
		return version, err
	}
	return version.Retire(built.Reference(), built.RetiredAt())
}
