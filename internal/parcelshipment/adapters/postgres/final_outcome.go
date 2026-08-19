package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// FinalOutcomes 实现 ports.FinalOutcomeStore。幂等三维加租户就是主键；「每包裹至多
// 一份当前终局」由部分唯一索引承担。重派生翻旧插新：新版本行以 prior_version 指回
// 前版并接过 is_current，原终局历史保留在原版本行上（AT-PS-063）——先翻旧（零行说明
// 锚已被并发赢家移走，答`已有记录`）再插新，两步在调用方事务里同生共死。
type FinalOutcomes struct {
	db *bentopg.DB
}

func NewFinalOutcomes(db *bentopg.DB) (*FinalOutcomes, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &FinalOutcomes{db: db}, nil
}

const findFinalSQL = `SELECT parcel_id, outcome_kind, outcome_version,
       content_digest, finalized,
       final_version, final_kind, rule_version,
       decision_ref, execution_ref, occurred_at,
       prior_version, rederivation_reason, refusal_basis, adopted_at
  FROM parcel_shipment.final_outcome`

// FindByKey 按幂等键取回已提交终局判断。否定结果只回 false，不区分「不存在」与
// 「属于另一个租户」。
func (repository *FinalOutcomes) FindByKey(
	ctx context.Context,
	key ports.FinalAdoptionKey,
) (ports.FinalOutcomeRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.FinalOutcomeRecord{}, false, fmt.Errorf("find final outcome: %w", err)
	}
	row := querier.QueryRow(ctx,
		findFinalSQL+`
		 WHERE tenant_id = $1
		   AND parcel_id = $2
		   AND outcome_kind = $3
		   AND outcome_version = $4`,
		key.TenantID.String(),
		key.Parcel.String(),
		key.Kind.String(),
		key.Version.String(),
	)
	return scanFinal(key.TenantID, row)
}

// FindCurrentFinal 按包裹找回当前有效终局——重派生的锚与委托完成派生的读口。部分
// 唯一索引保证至多一行。
func (repository *FinalOutcomes) FindCurrentFinal(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) (ports.FinalOutcomeRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.FinalOutcomeRecord{}, false, fmt.Errorf("find current final: %w", err)
	}
	row := querier.QueryRow(ctx,
		findFinalSQL+`
		 WHERE tenant_id = $1
		   AND parcel_id = $2
		   AND is_current`,
		tenant.String(),
		parcel.String(),
	)
	return scanFinal(tenant, row)
}

// Save 写下一次终局判断。三条写入路各有各的撞法，全部译写入代数不译错误：
//
//   - 不采用记录：普通插入，撞主键即`已有记录`；
//   - 首派生：插入即当前，撞主键或撞当前唯一索引（并发首派生）都答`已有记录`；
//   - 重派生（Final 带前版）：先把前版行翻下当前位——零行说明锚已被并发赢家移走，
//     答`已有记录`由重试方从新锚重走；翻成后插新行接过当前位。翻成却插不进说明
//     两个写入方拿着同一把键各走到一半，返回错误让事务整体回滚，不留半截翻转。
func (repository *FinalOutcomes) Save(
	ctx context.Context,
	record ports.FinalOutcomeRecord,
) (ports.FinalOutcomeSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.FinalOutcomeSaveOutcomeInvalid, fmt.Errorf("save final outcome: %w", err)
	}

	columns, err := finalColumns(record)
	if err != nil {
		return ports.FinalOutcomeSaveOutcomeInvalid, fmt.Errorf("save final outcome: %w", err)
	}

	key := record.Key
	flipped := false
	if columns.priorVersion != nil {
		tag, err := executor.Exec(ctx,
			`UPDATE parcel_shipment.final_outcome
			    SET is_current = false
			  WHERE tenant_id = $1
			    AND parcel_id = $2
			    AND final_version = $3
			    AND is_current`,
			key.TenantID.String(),
			key.Parcel.String(),
			*columns.priorVersion,
		)
		if err != nil {
			return ports.FinalOutcomeSaveOutcomeInvalid, fmt.Errorf("save final outcome: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ports.FinalOutcomeAlreadyRecorded, nil
		}
		flipped = true
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_shipment.final_outcome
			(tenant_id, parcel_id, outcome_kind, outcome_version,
			 content_digest, finalized, is_current,
			 final_version, final_kind, rule_version,
			 decision_ref, execution_ref, occurred_at,
			 prior_version, rederivation_reason, refusal_basis, adopted_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		 ON CONFLICT DO NOTHING`,
		key.TenantID.String(),
		key.Parcel.String(),
		key.Kind.String(),
		key.Version.String(),
		record.ContentDigest,
		record.Finalized,
		record.Finalized,
		columns.finalVersion,
		columns.finalKind,
		columns.ruleVersion,
		columns.decision,
		columns.execution,
		columns.occurredAt,
		columns.priorVersion,
		columns.reason,
		columns.refusalBasis,
		record.AdoptedAt.UTC(),
	)
	if err != nil {
		return ports.FinalOutcomeSaveOutcomeInvalid, fmt.Errorf("save final outcome: %w", err)
	}
	if tag.RowsAffected() == 0 {
		if flipped {
			// 前版翻下来了、新行却插不进：同键的另一半已经在场。半截翻转不能留，
			// 报错让调用方事务整体回滚。
			return ports.FinalOutcomeSaveOutcomeInvalid, errors.New(
				"save final outcome: the final chain moved during save")
		}
		return ports.FinalOutcomeAlreadyRecorded, nil
	}
	return ports.FinalOutcomeSaved, nil
}

type finalRowColumns struct {
	finalVersion *string
	finalKind    *string
	ruleVersion  *string
	decision     *string
	execution    *string
	occurredAt   *time.Time
	priorVersion *string
	reason       *string
	refusalBasis *string
}

// finalColumns 把记录折成行：终局面与不采用面互斥，终局的来源必须与幂等键同源
// ——键说的与本体说的不一致就是拼坏的聚合。
func finalColumns(record ports.FinalOutcomeRecord) (finalRowColumns, error) {
	if !record.Finalized {
		if record.RefusalBasis.String() == "" {
			return finalRowColumns{}, errors.New("a refusal carries its basis")
		}
		basis := record.RefusalBasis.String()
		return finalRowColumns{refusalBasis: &basis}, nil
	}

	final := record.Final
	source := final.Source()
	if final.Parcel() != record.Key.Parcel ||
		source.Kind() != record.Key.Kind ||
		source.Version() != record.Key.Version {
		return finalRowColumns{}, errors.New("record key disagrees with the final's source")
	}

	version := final.Version().String()
	kind := final.Kind().String()
	rule := final.RuleVersion().String()
	decision := source.Decision().String()
	execution := source.Execution().String()
	occurredAt := source.OccurredAt()
	columns := finalRowColumns{
		finalVersion: &version,
		finalKind:    &kind,
		ruleVersion:  &rule,
		decision:     &decision,
		execution:    &execution,
		occurredAt:   &occurredAt,
	}
	if prior, rederived := final.PriorVersion(); rederived {
		priorRaw := prior.String()
		columns.priorVersion = &priorRaw
		reason, _ := final.RederivationBasis()
		reasonRaw := reason.String()
		columns.reason = &reasonRaw
	}
	return columns, nil
}

// scanFinal 读一行并经领域重建门复验（NewResponsibilityOutcome +
// RehydrateParcelFinalOutcome）：决定与执行双引用、重派生痕迹互证都在读回处重走。
func scanFinal(
	tenant domain.TenantID,
	row pgx.Row,
) (ports.FinalOutcomeRecord, bool, error) {
	var (
		parcelRaw, kindRaw, versionRaw       string
		digest                               string
		finalized                            bool
		finalVersion, finalKind, ruleVersion *string
		decision, execution                  *string
		occurredAt                           *time.Time
		priorVersion, reason, refusalBasis   *string
		adoptedAt                            time.Time
	)
	err := row.Scan(&parcelRaw, &kindRaw, &versionRaw,
		&digest, &finalized,
		&finalVersion, &finalKind, &ruleVersion,
		&decision, &execution, &occurredAt,
		&priorVersion, &reason, &refusalBasis, &adoptedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.FinalOutcomeRecord{}, false, nil
	}
	if err != nil {
		return ports.FinalOutcomeRecord{}, false, fmt.Errorf("read final outcome: %w", err)
	}

	record := ports.FinalOutcomeRecord{
		ContentDigest: digest,
		Finalized:     finalized,
		AdoptedAt:     adoptedAt.UTC(),
	}
	record.Key.TenantID = tenant
	if record.Key.Parcel, err = domain.NewDeclaredParcelID(parcelRaw); err != nil {
		return ports.FinalOutcomeRecord{}, false, fmt.Errorf("read final outcome: %w", err)
	}
	if record.Key.Kind, err = responsibilityKindFrom(kindRaw); err != nil {
		return ports.FinalOutcomeRecord{}, false, fmt.Errorf("read final outcome: %w", err)
	}
	if record.Key.Version, err = domain.NewResponsibilityOutcomeVersion(versionRaw); err != nil {
		return ports.FinalOutcomeRecord{}, false, fmt.Errorf("read final outcome: %w", err)
	}

	if !finalized {
		if refusalBasis == nil {
			return ports.FinalOutcomeRecord{}, false, errors.New(
				"read final outcome: a refusal row arrived without its basis")
		}
		if record.RefusalBasis, err = domain.NewCheckReason(*refusalBasis); err != nil {
			return ports.FinalOutcomeRecord{}, false, fmt.Errorf("read final outcome: %w", err)
		}
		return record, true, nil
	}

	if finalVersion == nil || finalKind == nil || ruleVersion == nil ||
		decision == nil || execution == nil || occurredAt == nil {
		return ports.FinalOutcomeRecord{}, false, errors.New(
			"read final outcome: a finalized row arrived with missing final columns")
	}
	record.Final, err = rebuildFinal(record.Key, finalizedColumns{
		finalVersion: *finalVersion,
		finalKind:    *finalKind,
		ruleVersion:  *ruleVersion,
		decision:     *decision,
		execution:    *execution,
		occurredAt:   *occurredAt,
		priorVersion: priorVersion,
		reason:       reason,
	})
	if err != nil {
		return ports.FinalOutcomeRecord{}, false, fmt.Errorf("read final outcome: %w", err)
	}
	return record, true, nil
}

type finalizedColumns struct {
	finalVersion string
	finalKind    string
	ruleVersion  string
	decision     string
	execution    string
	occurredAt   time.Time
	priorVersion *string
	reason       *string
}

func rebuildFinal(
	key ports.FinalAdoptionKey,
	columns finalizedColumns,
) (domain.ParcelFinalOutcome, error) {
	decision, err := domain.NewResponsibilityDecisionReference(columns.decision)
	if err != nil {
		return domain.ParcelFinalOutcome{}, err
	}
	execution, err := domain.NewExecutionEvidenceReference(columns.execution)
	if err != nil {
		return domain.ParcelFinalOutcome{}, err
	}
	source, err := domain.NewResponsibilityOutcome(domain.ResponsibilityOutcomeSpec{
		Kind:       key.Kind,
		Parcel:     key.Parcel,
		Decision:   decision,
		Execution:  execution,
		Version:    key.Version,
		OccurredAt: columns.occurredAt,
	})
	if err != nil {
		return domain.ParcelFinalOutcome{}, err
	}
	spec := domain.RehydrateParcelFinalOutcomeSpec{
		Parcel: key.Parcel,
		Source: source,
	}
	if spec.Version, err = domain.NewFinalOutcomeVersionID(columns.finalVersion); err != nil {
		return domain.ParcelFinalOutcome{}, err
	}
	if spec.Kind, err = domain.NewFinalKindReference(columns.finalKind); err != nil {
		return domain.ParcelFinalOutcome{}, err
	}
	if spec.RuleVersion, err = domain.NewFinalRuleVersionReference(columns.ruleVersion); err != nil {
		return domain.ParcelFinalOutcome{}, err
	}
	if columns.priorVersion != nil {
		if spec.PriorVersion, err = domain.NewFinalOutcomeVersionID(*columns.priorVersion); err != nil {
			return domain.ParcelFinalOutcome{}, err
		}
	}
	if columns.reason != nil {
		if spec.Reason, err = domain.NewRederivationReason(*columns.reason); err != nil {
			return domain.ParcelFinalOutcome{}, err
		}
	}
	return domain.RehydrateParcelFinalOutcome(spec)
}

func responsibilityKindFrom(raw string) (domain.ResponsibilityOutcomeKind, error) {
	return domain.NewResponsibilityOutcomeKind(raw)
}
