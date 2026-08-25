package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// DeclarationSubmissions 实现 ports.DeclarationSubmissionStore（写入代数同 ADR-0031）。
// 版本表与尝试表分表同库、Save/SaveCorrection 同一事务两表写入（照 VE 发作期/结论的
// 同笔纪律）：版本是首次实际发送前固定的不可覆盖快照（CONTEXT 硬句 168），**内容列**
// 没有 UPDATE 语句；SaveCorrection 唯一翻动的是前版的 is_current 当前指针（迁移 0012，
// 先例 TF 交付登记翻旧插新），不属版本内容。尝试不可能先于版本存在由外键承担。
type DeclarationSubmissions struct {
	db *bentopg.DB
}

func NewDeclarationSubmissions(db *bentopg.DB) (*DeclarationSubmissions, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &DeclarationSubmissions{db: db}, nil
}

// FindByKey 按逻辑申报目标（租户+单元+程序）取回**当前版**与其最近一次发送尝试。
// 读回经领域重建口重验：版本的组成快照与尝试的重发形状都在那里把门。
func (repository *DeclarationSubmissions) FindByKey(
	ctx context.Context,
	key ports.DeclarationSubmissionKey,
) (ports.DeclarationSubmissionRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.DeclarationSubmissionRecord{}, false, fmt.Errorf("find declaration submission: %w", err)
	}

	var (
		versionID, digest, dossier, roles, basis, authority string
		correctedFrom                                       *string
		membersRaw                                          []byte
		fixedAt, recordedAt                                 time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT version_id, content_digest, members, dossier_ref, roles_ref,
		        readiness_basis, authority_ref, corrected_from, fixed_at, recorded_at
		   FROM customs_compliance.declaration_submission
		  WHERE tenant_id = $1 AND unit_id = $2 AND procedure_ref = $3 AND is_current`,
		key.TenantID.String(), key.Unit.String(), key.Procedure.String(),
	).Scan(&versionID, &digest, &membersRaw, &dossier, &roles, &basis, &authority,
		&correctedFrom, &fixedAt, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.DeclarationSubmissionRecord{}, false, nil
	}
	if err != nil {
		return ports.DeclarationSubmissionRecord{}, false, fmt.Errorf("find declaration submission: %w", err)
	}

	return repository.rebuildRecord(ctx, key, versionID, digest, membersRaw,
		dossier, roles, basis, authority, correctedFrom, fixedAt, recordedAt)
}

// FindByVersion 按版本标识读回留存版本——当前版或已被更正的历史版皆可（CONTEXT
// 硬句 169：原提交及其结果永久保留）。下游按信封宣告的版本取数走这里，当前版推进
// 不改变已发出信封的所指。
func (repository *DeclarationSubmissions) FindByVersion(
	ctx context.Context,
	tenant domain.TenantID,
	version domain.SubmissionVersionID,
) (ports.DeclarationSubmissionRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.DeclarationSubmissionRecord{}, false, fmt.Errorf("find declaration submission version: %w", err)
	}

	var (
		unitID, procedure, digest, dossier, roles, basis, authority string
		correctedFrom                                               *string
		membersRaw                                                  []byte
		fixedAt, recordedAt                                         time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT unit_id, procedure_ref, content_digest, members, dossier_ref, roles_ref,
		        readiness_basis, authority_ref, corrected_from, fixed_at, recorded_at
		   FROM customs_compliance.declaration_submission
		  WHERE tenant_id = $1 AND version_id = $2`,
		tenant.String(), version.String(),
	).Scan(&unitID, &procedure, &digest, &membersRaw, &dossier, &roles, &basis, &authority,
		&correctedFrom, &fixedAt, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.DeclarationSubmissionRecord{}, false, nil
	}
	if err != nil {
		return ports.DeclarationSubmissionRecord{}, false, fmt.Errorf("find declaration submission version: %w", err)
	}

	key := ports.DeclarationSubmissionKey{TenantID: tenant}
	if key.Unit, err = domain.NewDeclarationUnitID(unitID); err != nil {
		return ports.DeclarationSubmissionRecord{}, false, fmt.Errorf("rebuild submission key: %w", err)
	}
	if key.Procedure, err = domain.NewCustomsProcedureReference(procedure); err != nil {
		return ports.DeclarationSubmissionRecord{}, false, fmt.Errorf("rebuild submission key: %w", err)
	}
	return repository.rebuildRecord(ctx, key, version.String(), digest, membersRaw,
		dossier, roles, basis, authority, correctedFrom, fixedAt, recordedAt)
}

// rebuildRecord 把一行版本连同其最近尝试重建成记录，两条读法共用。
func (repository *DeclarationSubmissions) rebuildRecord(
	ctx context.Context,
	key ports.DeclarationSubmissionKey,
	versionID, digest string,
	membersRaw []byte,
	dossier, roles, basis, authority string,
	correctedFrom *string,
	fixedAt, recordedAt time.Time,
) (ports.DeclarationSubmissionRecord, bool, error) {
	version, err := rebuildSubmissionVersion(key.Unit, versionID, membersRaw, dossier, roles, basis, authority, fixedAt)
	if err != nil {
		return ports.DeclarationSubmissionRecord{}, false, err
	}

	attempt, err := repository.latestAttempt(ctx, key.TenantID, version.ID())
	if err != nil {
		return ports.DeclarationSubmissionRecord{}, false, err
	}

	record := ports.DeclarationSubmissionRecord{
		Key:           key,
		ContentDigest: digest,
		Version:       version,
		Attempt:       attempt,
		RecordedAt:    recordedAt,
	}
	if correctedFrom != nil {
		if record.CorrectedFrom, err = domain.NewSubmissionVersionID(*correctedFrom); err != nil {
			return ports.DeclarationSubmissionRecord{}, false, fmt.Errorf("rebuild submission version: corrected from: %w", err)
		}
	}
	return record, true, nil
}

// Save 写下一份**首版**提交申报：版本行与首次尝试行同一事务落库。同一逻辑申报目标
// 已有当前版时答`已有记录`且不落尝试——原版本的尝试链不被第二次提交搅动（ADR-0031，
// ON CONFLICT DO NOTHING 保事务可用，编排拿到它还要同事务读回原版本作答）。带前身的
// 记录走 SaveCorrection，不走这里：首版声称有前身是编排缺陷，响亮拒绝。
func (repository *DeclarationSubmissions) Save(
	ctx context.Context,
	record ports.DeclarationSubmissionRecord,
) (ports.DeclarationSubmissionSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeclarationSubmissionSaveOutcomeInvalid, fmt.Errorf("save declaration submission: %w", err)
	}
	if record.CorrectedFrom.String() != "" {
		return ports.DeclarationSubmissionSaveOutcomeInvalid,
			fmt.Errorf("save declaration submission: a first version cannot claim a predecessor")
	}

	version := record.Version.Snapshot()
	attempt := record.Attempt.Snapshot()
	if record.Key.Unit != version.Unit {
		return ports.DeclarationSubmissionSaveOutcomeInvalid,
			fmt.Errorf("save declaration submission: key disagrees with the version it claims to index")
	}
	if attempt.Version != version.ID {
		return ports.DeclarationSubmissionSaveOutcomeInvalid,
			fmt.Errorf("save declaration submission: attempt belongs to another version")
	}

	membersRaw, err := submissionMembersJSON(version)
	if err != nil {
		return ports.DeclarationSubmissionSaveOutcomeInvalid, fmt.Errorf("save declaration submission: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.declaration_submission
			(tenant_id, unit_id, procedure_ref, version_id, content_digest, members,
			 dossier_ref, roles_ref, readiness_basis, authority_ref, corrected_from,
			 is_current, fixed_at, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULL, true, $11, $12)
		 ON CONFLICT (tenant_id, unit_id, procedure_ref) WHERE is_current DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Unit.String(),
		record.Key.Procedure.String(),
		version.ID.String(),
		record.ContentDigest,
		membersRaw,
		version.Dossier.String(),
		version.Roles.String(),
		version.Basis.String(),
		version.Authority.String(),
		version.FixedAt,
		record.RecordedAt,
	)
	if err != nil {
		return ports.DeclarationSubmissionSaveOutcomeInvalid, fmt.Errorf("save declaration submission: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.DeclarationSubmissionAlreadyRecorded, nil
	}

	if err := repository.insertAttempt(ctx, executor, record.Key.TenantID.String(), attempt); err != nil {
		return ports.DeclarationSubmissionSaveOutcomeInvalid, err
	}
	return ports.DeclarationSubmissionSaved, nil
}

// SaveCorrection 落一份原案内更正/补充版本：同一事务里把 CorrectedFrom 指名的当前版
// 转为非当前、插入新当前版行与其首次尝试行。前版内容一列不改（CONTEXT 硬句 169：原
// 提交及其结果永久保留）。翻转到零行即`当前版已被换`——并发更正先落或前身早已非当前，
// 由部分唯一索引与这条 WHERE 共同裁决，调用方读回当前版再作答，这里绝不顶替。
func (repository *DeclarationSubmissions) SaveCorrection(
	ctx context.Context,
	record ports.DeclarationSubmissionRecord,
) (ports.DeclarationCorrectionSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeclarationCorrectionSaveOutcomeInvalid, fmt.Errorf("save declaration correction: %w", err)
	}
	if record.CorrectedFrom.String() == "" {
		return ports.DeclarationCorrectionSaveOutcomeInvalid,
			fmt.Errorf("save declaration correction: the corrected version is required")
	}

	version := record.Version.Snapshot()
	attempt := record.Attempt.Snapshot()
	if record.CorrectedFrom == version.ID {
		// 指名自己为前身是覆盖不是更正（同 VE 已接受事实拒绝自替代的道理）。
		return ports.DeclarationCorrectionSaveOutcomeInvalid,
			fmt.Errorf("save declaration correction: a version cannot correct itself")
	}
	if record.Key.Unit != version.Unit {
		return ports.DeclarationCorrectionSaveOutcomeInvalid,
			fmt.Errorf("save declaration correction: key disagrees with the version it claims to index")
	}
	if attempt.Version != version.ID {
		return ports.DeclarationCorrectionSaveOutcomeInvalid,
			fmt.Errorf("save declaration correction: attempt belongs to another version")
	}

	membersRaw, err := submissionMembersJSON(version)
	if err != nil {
		return ports.DeclarationCorrectionSaveOutcomeInvalid, fmt.Errorf("save declaration correction: %w", err)
	}

	flipped, err := executor.Exec(ctx,
		`UPDATE customs_compliance.declaration_submission
		    SET is_current = false
		  WHERE tenant_id = $1 AND unit_id = $2 AND procedure_ref = $3
		    AND version_id = $4 AND is_current`,
		record.Key.TenantID.String(),
		record.Key.Unit.String(),
		record.Key.Procedure.String(),
		record.CorrectedFrom.String(),
	)
	if err != nil {
		return ports.DeclarationCorrectionSaveOutcomeInvalid, fmt.Errorf("save declaration correction: %w", err)
	}
	if flipped.RowsAffected() == 0 {
		return ports.DeclarationCorrectionCurrentMoved, nil
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.declaration_submission
			(tenant_id, unit_id, procedure_ref, version_id, content_digest, members,
			 dossier_ref, roles_ref, readiness_basis, authority_ref, corrected_from,
			 is_current, fixed_at, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, true, $12, $13)`,
		record.Key.TenantID.String(),
		record.Key.Unit.String(),
		record.Key.Procedure.String(),
		version.ID.String(),
		record.ContentDigest,
		membersRaw,
		version.Dossier.String(),
		version.Roles.String(),
		version.Basis.String(),
		version.Authority.String(),
		record.CorrectedFrom.String(),
		version.FixedAt,
		record.RecordedAt,
	); err != nil {
		// 版本标识撞唯一约束等一律如实报错回滚——前版翻转随事务一并退回。
		return ports.DeclarationCorrectionSaveOutcomeInvalid, fmt.Errorf("save declaration correction: %w", err)
	}

	if err := repository.insertAttempt(ctx, executor, record.Key.TenantID.String(), attempt); err != nil {
		return ports.DeclarationCorrectionSaveOutcomeInvalid, err
	}
	return ports.DeclarationCorrectionSaved, nil
}

func submissionMembersJSON(version domain.CustomsSubmissionVersionSnapshot) ([]byte, error) {
	members := make([]string, 0, len(version.Members))
	for _, member := range version.Members {
		members = append(members, member.String())
	}
	return json.Marshal(members)
}

// insertAttempt 落首次尝试行。版本是新行，首次尝试不可能已存在——这里不译
// ON CONFLICT，撞键是身份或序号纪律失守，如实报错让事务整体回退。
func (repository *DeclarationSubmissions) insertAttempt(
	ctx context.Context,
	executor bentopg.Executor,
	tenant string,
	attempt domain.SubmissionAttemptSnapshot,
) error {
	if _, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.submission_attempt
			(tenant_id, version_id, sequence, target, result, safe_resend_ref, sent_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		tenant,
		attempt.Version.String(),
		attempt.Sequence,
		attempt.Target,
		attempt.Result.String(),
		nullIfBlankRef(attempt.SafeResend.String()),
		attempt.SentAt,
	); err != nil {
		return fmt.Errorf("save declaration submission: attempt: %w", err)
	}
	return nil
}

// latestAttempt 取该版本序号最大的一次尝试。序号递增由主键承担，最近一次即当前
// 发送状态的如实答案。
func (repository *DeclarationSubmissions) latestAttempt(
	ctx context.Context,
	tenant domain.TenantID,
	version domain.SubmissionVersionID,
) (domain.SubmissionAttempt, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.SubmissionAttempt{}, fmt.Errorf("find submission attempt: %w", err)
	}

	var (
		sequence       int
		target, result string
		safeResend     *string
		sentAt         time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT sequence, target, result, safe_resend_ref, sent_at
		   FROM customs_compliance.submission_attempt
		  WHERE tenant_id = $1 AND version_id = $2
		  ORDER BY sequence DESC
		  LIMIT 1`,
		tenant.String(), version.String(),
	).Scan(&sequence, &target, &result, &safeResend, &sentAt)
	if err != nil {
		// 版本在而尝试不在是坏写入——Save 把两行钉在同一事务里，缺一行不该可见。
		return domain.SubmissionAttempt{}, fmt.Errorf("find submission attempt: %w", err)
	}

	snapshot := domain.SubmissionAttemptSnapshot{
		Version:  version,
		Sequence: sequence,
		Target:   target,
		SentAt:   sentAt,
	}
	if snapshot.Result, err = attemptResultFrom(result); err != nil {
		return domain.SubmissionAttempt{}, err
	}
	if safeResend != nil {
		if snapshot.SafeResend, err = domain.NewSafeResendReference(*safeResend); err != nil {
			return domain.SubmissionAttempt{}, fmt.Errorf("rebuild submission attempt: %w", err)
		}
	}
	attempt, err := domain.RehydrateSubmissionAttempt(snapshot)
	if err != nil {
		return domain.SubmissionAttempt{}, fmt.Errorf("rebuild submission attempt: %w", err)
	}
	return attempt, nil
}

func rebuildSubmissionVersion(
	unit domain.DeclarationUnitID,
	versionID string,
	membersRaw []byte,
	dossier, roles, basis, authority string,
	fixedAt time.Time,
) (domain.CustomsSubmissionVersion, error) {
	snapshot := domain.CustomsSubmissionVersionSnapshot{Unit: unit, FixedAt: fixedAt}
	var err error
	if snapshot.ID, err = domain.NewSubmissionVersionID(versionID); err != nil {
		return domain.CustomsSubmissionVersion{}, fmt.Errorf("rebuild submission version: %w", err)
	}
	if snapshot.Dossier, err = domain.NewDossierSnapshotReference(dossier); err != nil {
		return domain.CustomsSubmissionVersion{}, fmt.Errorf("rebuild submission version: %w", err)
	}
	if snapshot.Roles, err = domain.NewRoleSnapshotReference(roles); err != nil {
		return domain.CustomsSubmissionVersion{}, fmt.Errorf("rebuild submission version: %w", err)
	}
	if snapshot.Basis, err = domain.NewReadinessBasisReference(basis); err != nil {
		return domain.CustomsSubmissionVersion{}, fmt.Errorf("rebuild submission version: %w", err)
	}
	if snapshot.Authority, err = domain.NewSubmissionAuthorityReference(authority); err != nil {
		return domain.CustomsSubmissionVersion{}, fmt.Errorf("rebuild submission version: %w", err)
	}

	var members []string
	if err := json.Unmarshal(membersRaw, &members); err != nil {
		return domain.CustomsSubmissionVersion{}, fmt.Errorf("rebuild submission version: members: %w", err)
	}
	for _, member := range members {
		reference, err := domain.NewDeclaredParcelReference(member)
		if err != nil {
			return domain.CustomsSubmissionVersion{}, fmt.Errorf("rebuild submission version: %w", err)
		}
		snapshot.Members = append(snapshot.Members, reference)
	}

	version, err := domain.RehydrateSubmissionVersion(snapshot)
	if err != nil {
		return domain.CustomsSubmissionVersion{}, fmt.Errorf("rebuild submission version: %w", err)
	}
	return version, nil
}

// attemptResultFrom 把列值译回封闭三值。迁移 CHECK 已拦住集合外取值。
func attemptResultFrom(value string) (domain.AttemptResult, error) {
	switch value {
	case "ACKNOWLEDGED":
		return domain.AttemptAcknowledged, nil
	case "FAILED":
		return domain.AttemptFailed, nil
	case "PENDING_CONFIRMATION":
		return domain.AttemptPendingConfirmation, nil
	default:
		return domain.AttemptResultInvalid,
			fmt.Errorf("customs compliance postgres: unknown attempt result %q", value)
	}
}

func nullIfBlankRef(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
