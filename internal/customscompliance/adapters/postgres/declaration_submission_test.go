package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证申报链库的行为：提交版本不可覆盖（同目标第二份答
// `已有记录`且尝试链不被搅动）、尝试不可能先于版本存在（外键——硬句 168 可钉的那半）、
// 重发形状入 CHECK（硬句 170）、租户隔离由 SQL 条件承担。断言一律在事务闭包外。

var declarationFixedAt = time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)

type declarationFixture struct {
	submissions *adapter.DeclarationSubmissions
	transactor  bentoapp.Transactor
	pool        *pgxpool.Pool
}

func newDeclarationFixture(t *testing.T) *declarationFixture {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	submissions, err := adapter.NewDeclarationSubmissions(db)
	if err != nil {
		t.Fatalf("构造申报链库：%v", err)
	}
	return &declarationFixture{submissions: submissions, transactor: db.Transactor(), pool: pool}
}

func (fixture *declarationFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func declarationValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

// submissionRecord 经领域真路径造一份提交申报：单元成形 → 就绪+授权双有效 → 成版 →
// 首次尝试。夹具不手搓版本——成版门（双有效、组成快照）就是被测对象的一半。
func submissionRecord(t *testing.T, tenant, unit, procedure, version string) ports.DeclarationSubmissionRecord {
	t.Helper()
	unitID := declarationValue(t, domain.NewDeclarationUnitID, unit)
	formed, err := domain.FormDeclarationUnit(
		unitID,
		declarationValue(t, domain.NewCustomsCaseID, "case-1"),
		declarationValue(t, domain.NewCustomsProcedureReference, procedure),
		[]domain.DeclaredParcelReference{
			declarationValue(t, domain.NewDeclaredParcelReference, "parcel-1"),
			declarationValue(t, domain.NewDeclaredParcelReference, "parcel-2"),
		},
	)
	if err != nil {
		t.Fatalf("构造申报单元：%v", err)
	}
	readiness, err := domain.JudgeReady(
		unitID,
		declarationValue(t, domain.NewReadinessBasisReference, "readiness/check-1"),
		declarationFixedAt.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("构造就绪判断：%v", err)
	}
	authorization, err := domain.GrantSubmissionAuthority(
		unitID,
		declarationValue(t, domain.NewSubmissionAuthorityReference, "authority/grant-1"),
		declarationFixedAt.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("构造提交授权：%v", err)
	}
	fixed, err := domain.FixSubmissionVersion(domain.CustomsSubmissionVersionSpec{
		ID:            declarationValue(t, domain.NewSubmissionVersionID, version),
		Unit:          formed,
		Dossier:       declarationValue(t, domain.NewDossierSnapshotReference, "dossier/snap-1"),
		Roles:         declarationValue(t, domain.NewRoleSnapshotReference, "roles/snap-1"),
		Readiness:     readiness,
		Authorization: authorization,
		FixedAt:       declarationFixedAt,
	})
	if err != nil {
		t.Fatalf("固定提交版本：%v", err)
	}
	attempt, err := domain.InitialAttempt(fixed, "customs-gateway/prod", domain.AttemptPendingConfirmation, declarationFixedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("构造首次尝试：%v", err)
	}
	return ports.DeclarationSubmissionRecord{
		Key: ports.DeclarationSubmissionKey{
			TenantID:  declarationValue(t, domain.NewTenantID, tenant),
			Unit:      unitID,
			Procedure: declarationValue(t, domain.NewCustomsProcedureReference, procedure),
		},
		ContentDigest: "digest-" + version,
		Version:       fixed,
		Attempt:       attempt,
		RecordedAt:    declarationFixedAt.Add(2 * time.Minute),
	}
}

func (fixture *declarationFixture) attemptCount(t *testing.T, ctx context.Context, tenant, version string) int {
	t.Helper()
	var count int
	if err := fixture.pool.QueryRow(ctx,
		`SELECT count(*) FROM customs_compliance.submission_attempt
		  WHERE tenant_id = $1 AND version_id = $2`,
		tenant, version,
	).Scan(&count); err != nil {
		t.Fatalf("数尝试行：%v", err)
	}
	return count
}

// TestSubmissionIsReadBackUnchanged 证往返：版本快照（组成、四件引用、固定时间）与
// 首次尝试原样读回，读回经领域重建口重验。
func TestSubmissionIsReadBackUnchanged(t *testing.T) {
	fixture := newDeclarationFixture(t)
	ctx := t.Context()

	record := submissionRecord(t, "tenant-a", "unit-1", "IMPORT_STANDARD", "version-1")
	var outcome ports.DeclarationSubmissionSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		saved, err := fixture.submissions.Save(txCtx, record)
		outcome = saved
		return err
	})
	if outcome != ports.DeclarationSubmissionSaved {
		t.Fatalf("首写 outcome = %d", outcome)
	}

	found, exists, err := fixture.submissions.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("读回提交申报：%v exists=%v", err, exists)
	}
	if found.Version.ID() != record.Version.ID() ||
		found.Version.Dossier() != record.Version.Dossier() ||
		found.Version.Authority() != record.Version.Authority() ||
		!found.Version.FixedAt().Equal(declarationFixedAt) {
		t.Fatalf("版本没原样读回：%+v", found.Version)
	}
	if len(found.Version.Members()) != 2 {
		t.Fatalf("组成快照没原样读回：%d 员", len(found.Version.Members()))
	}
	if found.ContentDigest != record.ContentDigest {
		t.Fatalf("内容指纹 = %q", found.ContentDigest)
	}
	if found.Attempt.Sequence() != 1 ||
		found.Attempt.Result() != domain.AttemptPendingConfirmation ||
		found.Attempt.Target() != "customs-gateway/prod" {
		t.Fatalf("首次尝试没原样读回：seq=%d result=%v", found.Attempt.Sequence(), found.Attempt.Result())
	}
	if _, resend := found.Attempt.SafeResend(); resend {
		t.Fatalf("首次尝试凭空长出安全重发判断")
	}
}

// TestSecondSubmissionGetsAlreadyRecorded 证不可覆盖：同一逻辑申报目标第二份答
// `已有记录`，原版本与尝试链不被搅动，事务保持可用（同事务读回原版本作答）。
func TestSecondSubmissionGetsAlreadyRecorded(t *testing.T) {
	fixture := newDeclarationFixture(t)
	ctx := t.Context()

	original := submissionRecord(t, "tenant-a", "unit-1", "IMPORT_STANDARD", "version-1")
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.submissions.Save(txCtx, original)
		return err
	})

	impostor := submissionRecord(t, "tenant-a", "unit-1", "IMPORT_STANDARD", "version-2")
	var outcome ports.DeclarationSubmissionSaveOutcome
	var foundInTx ports.DeclarationSubmissionRecord
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		saved, err := fixture.submissions.Save(txCtx, impostor)
		if err != nil {
			return err
		}
		outcome = saved
		found, _, err := fixture.submissions.FindByKey(txCtx, original.Key)
		foundInTx = found
		return err
	})
	if outcome != ports.DeclarationSubmissionAlreadyRecorded {
		t.Fatalf("重写 outcome = %d, 想要 AlreadyRecorded", outcome)
	}
	if foundInTx.Version.ID() != original.Version.ID() {
		t.Fatalf("原版本被顶替成 %s", foundInTx.Version.ID())
	}
	if fixture.attemptCount(t, ctx, "tenant-a", "version-2") != 0 {
		t.Fatalf("被拒的第二份仍然落了尝试行")
	}
	if fixture.attemptCount(t, ctx, "tenant-a", "version-1") != 1 {
		t.Fatalf("原版本的尝试链被搅动")
	}
}

// TestAttemptCannotExistWithoutItsVersion 证硬句 168 可钉的那半在库内：尝试行指不到
// 版本行就进不来（外键），受控重发缺安全判断、首发带安全判断同样被 CHECK 拦住
// （硬句 170）。
func TestAttemptCannotExistWithoutItsVersion(t *testing.T) {
	fixture := newDeclarationFixture(t)
	ctx := t.Context()

	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO customs_compliance.submission_attempt
			(tenant_id, version_id, sequence, target, result, safe_resend_ref, sent_at)
		 VALUES ('tenant-a', 'version-ghost', 1, 'gateway', 'FAILED', NULL, now())`); err == nil {
		t.Fatalf("没有版本的尝试被库接受了")
	}

	record := submissionRecord(t, "tenant-a", "unit-1", "IMPORT_STANDARD", "version-1")
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.submissions.Save(txCtx, record)
		return err
	})

	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO customs_compliance.submission_attempt
			(tenant_id, version_id, sequence, target, result, safe_resend_ref, sent_at)
		 VALUES ('tenant-a', 'version-1', 2, 'gateway', 'FAILED', NULL, now())`); err == nil {
		t.Fatalf("缺安全重发判断的再次尝试被库接受了")
	}
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO customs_compliance.submission_attempt
			(tenant_id, version_id, sequence, target, result, safe_resend_ref, sent_at)
		 VALUES ('tenant-a', 'version-1', 1, 'gateway', 'FAILED', 'resend/safe-1', now())`); err == nil {
		t.Fatalf("带安全重发判断的首次尝试被库接受了")
	}
}

// TestLatestAttemptWinsTheReadFace 证读面交回序号最大的尝试：受控重发落库后
// FindByKey 的尝试半边推进到新序号并携安全判断。
func TestLatestAttemptWinsTheReadFace(t *testing.T) {
	fixture := newDeclarationFixture(t)
	ctx := t.Context()

	record := submissionRecord(t, "tenant-a", "unit-1", "IMPORT_STANDARD", "version-1")
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.submissions.Save(txCtx, record)
		return err
	})

	resend, err := record.Attempt.ControlledResend(
		declarationValue(t, domain.NewSafeResendReference, "resend/safe-1"),
		domain.AttemptAcknowledged,
		declarationFixedAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("构造受控重发：%v", err)
	}
	snapshot := resend.Snapshot()
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO customs_compliance.submission_attempt
			(tenant_id, version_id, sequence, target, result, safe_resend_ref, sent_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		"tenant-a", snapshot.Version.String(), snapshot.Sequence, snapshot.Target,
		snapshot.Result.String(), snapshot.SafeResend.String(), snapshot.SentAt); err != nil {
		t.Fatalf("落受控重发行：%v", err)
	}

	found, _, err := fixture.submissions.FindByKey(ctx, record.Key)
	if err != nil {
		t.Fatalf("读回提交申报：%v", err)
	}
	if found.Attempt.Sequence() != 2 || found.Attempt.Result() != domain.AttemptAcknowledged {
		t.Fatalf("读面没交回最近尝试：seq=%d", found.Attempt.Sequence())
	}
	if _, resendPresent := found.Attempt.SafeResend(); !resendPresent {
		t.Fatalf("受控重发读回丢了安全判断")
	}
}

// TestSubmissionsOfAnotherTenantAreInvisible 证租户隔离：同名键在另一租户不可见，
// 另一租户写同名键是新行不是重放。
func TestSubmissionsOfAnotherTenantAreInvisible(t *testing.T) {
	fixture := newDeclarationFixture(t)
	ctx := t.Context()

	record := submissionRecord(t, "tenant-a", "unit-1", "IMPORT_STANDARD", "version-1")
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.submissions.Save(txCtx, record)
		return err
	})

	probe := record.Key
	probe.TenantID = declarationValue(t, domain.NewTenantID, "tenant-b")
	if _, exists, err := fixture.submissions.FindByKey(ctx, probe); err != nil || exists {
		t.Fatalf("跨租户可见：err=%v exists=%v", err, exists)
	}

	var outcome ports.DeclarationSubmissionSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		saved, err := fixture.submissions.Save(txCtx,
			submissionRecord(t, "tenant-b", "unit-1", "IMPORT_STANDARD", "version-b1"))
		outcome = saved
		return err
	})
	if outcome != ports.DeclarationSubmissionSaved {
		t.Fatalf("另一租户同名键 outcome = %d, 想要 Saved", outcome)
	}
}

// TestSubmissionWritesRequireTransactionAndRollBack 证事务纪律：无事务写一律拒；
// 事务失败后版本与尝试都不存在——两行同生共死。
func TestSubmissionWritesRequireTransactionAndRollBack(t *testing.T) {
	fixture := newDeclarationFixture(t)
	ctx := t.Context()

	record := submissionRecord(t, "tenant-a", "unit-1", "IMPORT_STANDARD", "version-1")
	if _, err := fixture.submissions.Save(ctx, record); err == nil {
		t.Fatalf("无事务写入被接受了")
	}

	rollback := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.submissions.Save(txCtx, record); err != nil {
			return err
		}
		return context.Canceled
	})
	if rollback == nil {
		t.Fatalf("事务该失败没失败")
	}
	if _, exists, err := fixture.submissions.FindByKey(ctx, record.Key); err != nil || exists {
		t.Fatalf("回滚后版本仍在：err=%v exists=%v", err, exists)
	}
	if fixture.attemptCount(t, ctx, "tenant-a", "version-1") != 0 {
		t.Fatalf("回滚后尝试仍在")
	}
}
