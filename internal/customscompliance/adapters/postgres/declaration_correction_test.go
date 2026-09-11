package postgres_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 本文件对真实 PostgreSQL 16 证原案内更正的存储行为（迁移 0012）：同一逻辑申报目标
// 容纳多版本、当前版恰一个、前版内容一列不改仍可按版本读回、并发换版由部分唯一索引
// 与翻转 WHERE 共同裁决。断言一律在事务闭包外。

// correctionRecord 经领域真路径造一份更正版记录：同一单元身份、新资料快照、携带前身。
func correctionRecord(
	t *testing.T,
	tenant, unit, procedure, version, correctedFrom, dossier string,
) ports.DeclarationSubmissionRecord {
	t.Helper()
	record := submissionRecordWithDossier(t, tenant, unit, procedure, version, dossier)
	record.CorrectedFrom = declarationValue(t, domain.NewSubmissionVersionID, correctedFrom)
	return record
}

// submissionRecordWithDossier 与 submissionRecord 同路径，只替换资料快照引用——更正
// 的差异恰在新资料，其余身份四件保持。
func submissionRecordWithDossier(
	t *testing.T,
	tenant, unit, procedure, version, dossier string,
) ports.DeclarationSubmissionRecord {
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
		declarationValue(t, domain.NewReadinessBasisReference, "readiness/check-2"),
		declarationFixedAt.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("构造就绪判断：%v", err)
	}
	authorization, err := domain.GrantSubmissionAuthority(
		unitID,
		declarationValue(t, domain.NewSubmissionAuthorityReference, "authority/grant-2"),
		declarationFixedAt.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("构造提交授权：%v", err)
	}
	fixed, err := domain.FixSubmissionVersion(domain.CustomsSubmissionVersionSpec{
		ID:            declarationValue(t, domain.NewSubmissionVersionID, version),
		Unit:          formed,
		Dossier:       declarationValue(t, domain.NewDossierSnapshotReference, dossier),
		Roles:         declarationValue(t, domain.NewRoleSnapshotReference, "roles/snap-1"),
		Readiness:     readiness,
		Authorization: authorization,
		FixedAt:       declarationFixedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("固定提交版本：%v", err)
	}
	attempt, err := domain.InitialAttempt(fixed, "customs-gateway/prod",
		domain.AttemptPendingConfirmation, declarationFixedAt.Add(time.Hour+time.Minute))
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
		RecordedAt:    declarationFixedAt.Add(time.Hour + 2*time.Minute),
	}
}

// Covers: CONTEXT「原提交及其结果永久保留」——更正翻旧插新后当前版是新版，原提交及其结果永久保留：
// 前身仍可按版本原样读回（内容一列不改），两版各有自己的尝试链，前身引用随新版携带。
func TestACorrectionSupersedesTheCurrentVersionAndKeepsThePrior(t *testing.T) {
	fixture := newDeclarationFixture(t)
	ctx := t.Context()

	first := submissionRecord(t, "tenant-a", "unit-1", "IMPORT_STANDARD", "version-1")
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.submissions.Save(txCtx, first)
		return err
	})

	correction := correctionRecord(t, "tenant-a", "unit-1", "IMPORT_STANDARD",
		"version-2", "version-1", "dossier/snap-2")
	var outcome ports.DeclarationCorrectionSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		saved, err := fixture.submissions.SaveCorrection(txCtx, correction)
		outcome = saved
		return err
	})
	if outcome != ports.DeclarationCorrectionSaved {
		t.Fatalf("更正写入 outcome = %d, want Saved", outcome)
	}

	current, exists, err := fixture.submissions.FindByKey(ctx, first.Key)
	if err != nil || !exists {
		t.Fatalf("读回当前版：%v exists=%v", err, exists)
	}
	if current.Version.ID().String() != "version-2" {
		t.Fatalf("当前版 = %q, want version-2", current.Version.ID())
	}
	if current.CorrectedFrom.String() != "version-1" {
		t.Fatalf("前身引用 = %q, want version-1（供下游登记替代关系）", current.CorrectedFrom)
	}
	if current.Version.Dossier().String() != "dossier/snap-2" {
		t.Fatalf("新资料快照 = %q", current.Version.Dossier())
	}

	prior, exists, err := fixture.submissions.FindByVersion(ctx,
		first.Key.TenantID, first.Version.ID())
	if err != nil || !exists {
		t.Fatalf("按版本读回前身：%v exists=%v", err, exists)
	}
	if prior.Version.Dossier().String() != "dossier/snap-1" ||
		prior.ContentDigest != "digest-version-1" ||
		prior.CorrectedFrom.String() != "" {
		t.Fatalf("前身没原样保留：dossier=%q digest=%q correctedFrom=%q",
			prior.Version.Dossier(), prior.ContentDigest, prior.CorrectedFrom)
	}
	if prior.Key != first.Key {
		t.Fatalf("前身键走样：%+v", prior.Key)
	}

	if got := fixture.attemptCount(t, ctx, "tenant-a", "version-1"); got != 1 {
		t.Fatalf("前身尝试链被搅动：%d 行", got)
	}
	if got := fixture.attemptCount(t, ctx, "tenant-a", "version-2"); got != 1 {
		t.Fatalf("新版尝试链 = %d 行, want 1", got)
	}
}

// Covers: 并发换版裁决——前身已非当前版时答`当前版已被换`且一行不落（版本行、尝试行
// 都没有），绝不顶替；覆盖格刻意不存在。
func TestACorrectionOnAMovedCurrentAnswersCurrentMoved(t *testing.T) {
	fixture := newDeclarationFixture(t)
	ctx := t.Context()

	first := submissionRecord(t, "tenant-a", "unit-1", "IMPORT_STANDARD", "version-1")
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.submissions.Save(txCtx, first)
		return err
	})
	winner := correctionRecord(t, "tenant-a", "unit-1", "IMPORT_STANDARD",
		"version-2", "version-1", "dossier/snap-2")
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.submissions.SaveCorrection(txCtx, winner)
		return err
	})

	// 输家仍指着 version-1 当前身——当前版已是 version-2。
	loser := correctionRecord(t, "tenant-a", "unit-1", "IMPORT_STANDARD",
		"version-3", "version-1", "dossier/snap-3")
	var outcome ports.DeclarationCorrectionSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		saved, err := fixture.submissions.SaveCorrection(txCtx, loser)
		outcome = saved
		return err
	})
	if outcome != ports.DeclarationCorrectionCurrentMoved {
		t.Fatalf("outcome = %d, want CurrentMoved", outcome)
	}
	if _, exists, err := fixture.submissions.FindByVersion(ctx,
		first.Key.TenantID, loser.Version.ID()); err != nil || exists {
		t.Fatalf("输家的版本行不该落库：err=%v exists=%v", err, exists)
	}
	if got := fixture.attemptCount(t, ctx, "tenant-a", "version-3"); got != 0 {
		t.Fatalf("输家的尝试行不该落库：%d 行", got)
	}
	current, _, err := fixture.submissions.FindByKey(ctx, first.Key)
	if err != nil || current.Version.ID().String() != "version-2" {
		t.Fatalf("当前版被搅动：%v %v", err, current.Version.ID())
	}
}

// Covers: 写入代数的入口纪律——首版声称有前身、更正缺前身、自指前身都是编排缺陷，
// 响亮拒绝不落库。
func TestCorrectionEntryGuardsAreLoud(t *testing.T) {
	fixture := newDeclarationFixture(t)
	ctx := t.Context()

	withPredecessor := correctionRecord(t, "tenant-a", "unit-1", "IMPORT_STANDARD",
		"version-1", "version-0", "dossier/snap-1")
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := fixture.submissions.Save(txCtx, withPredecessor)
		return err
	}); err == nil {
		t.Fatal("首版声称有前身必须响亮报错")
	}

	missingPredecessor := submissionRecordWithDossier(t, "tenant-a", "unit-1", "IMPORT_STANDARD",
		"version-2", "dossier/snap-2")
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := fixture.submissions.SaveCorrection(txCtx, missingPredecessor)
		return err
	}); err == nil {
		t.Fatal("更正缺前身必须响亮报错")
	}

	selfCorrecting := correctionRecord(t, "tenant-a", "unit-1", "IMPORT_STANDARD",
		"version-2", "version-2", "dossier/snap-2")
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := fixture.submissions.SaveCorrection(txCtx, selfCorrecting)
		return err
	}); err == nil {
		t.Fatal("自指前身是覆盖不是更正，必须响亮报错")
	}
}
