package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

var declaredAt = time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)

func declarationUnit(t *testing.T, members ...string) domain.DeclarationUnit {
	t.Helper()
	references := make([]domain.DeclaredParcelReference, 0, len(members))
	for _, member := range members {
		references = append(references, mustValue(t, domain.NewDeclaredParcelReference, member))
	}
	unit, err := domain.FormDeclarationUnit(
		mustValue(t, domain.NewDeclarationUnitID, "declaration-unit-1"),
		mustValue(t, domain.NewCustomsCaseID, "case-1"),
		mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		references,
	)
	if err != nil {
		t.Fatalf("form declaration unit: %v", err)
	}
	return unit
}

func readiness(t *testing.T) domain.ReadinessJudgment {
	t.Helper()
	judgment, err := domain.JudgeReady(
		mustValue(t, domain.NewDeclarationUnitID, "declaration-unit-1"),
		mustValue(t, domain.NewReadinessBasisReference, "readiness-check/v1"),
		declaredAt,
	)
	if err != nil {
		t.Fatalf("judge ready: %v", err)
	}
	return judgment
}

func authorization(t *testing.T) domain.SubmissionAuthorization {
	t.Helper()
	granted, err := domain.GrantSubmissionAuthority(
		mustValue(t, domain.NewDeclarationUnitID, "declaration-unit-1"),
		mustValue(t, domain.NewSubmissionAuthorityReference, "submit-authority/v1"),
		declaredAt,
	)
	if err != nil {
		t.Fatalf("grant authority: %v", err)
	}
	return granted
}

func versionSpec(t *testing.T) domain.CustomsSubmissionVersionSpec {
	t.Helper()
	return domain.CustomsSubmissionVersionSpec{
		ID:            mustValue(t, domain.NewSubmissionVersionID, "submission-1/v1"),
		Unit:          declarationUnit(t, "parcel-1", "parcel-2"),
		Dossier:       mustValue(t, domain.NewDossierSnapshotReference, "dossier/v1"),
		Roles:         mustValue(t, domain.NewRoleSnapshotReference, "roles/v1"),
		Readiness:     readiness(t),
		Authorization: authorization(t),
		FixedAt:       declaredAt.Add(time.Hour),
	}
}

// Covers: CC CONTEXT「申报单元必须具有独立身份和可追溯组成」与「申报单元的组成在
// 逻辑提交版本形成时固定……已提交版本保持不变」——版本固定时快照组成，值类型无回写
// 入口；重复成员拒；版本上没有传输/受理/放行字段（版本存在不证明那些，结构性）。
func TestAVersionFreezesTheUnitCompositionAtFixTime(t *testing.T) {
	version, err := domain.FixSubmissionVersion(versionSpec(t))
	if err != nil {
		t.Fatalf("fix submission version: %v", err)
	}
	if len(version.Members()) != 2 {
		t.Fatalf("members = %d", len(version.Members()))
	}
	if version.Dossier().String() != "dossier/v1" || version.ReadinessBasis().String() != "readiness-check/v1" {
		t.Fatalf("version = %#v; 资料与就绪依据必须随版本固定", version)
	}

	if _, err := domain.FormDeclarationUnit(
		mustValue(t, domain.NewDeclarationUnitID, "declaration-unit-2"),
		mustValue(t, domain.NewCustomsCaseID, "case-1"),
		mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		[]domain.DeclaredParcelReference{
			mustValue(t, domain.NewDeclaredParcelReference, "parcel-1"),
			mustValue(t, domain.NewDeclaredParcelReference, "parcel-1"),
		},
	); !errors.Is(err, domain.ErrInvalidDeclarationUnit) {
		t.Fatalf("err = %v; 重复成员被收下了", err)
	}

	// 案件维随形成即定（ADR-0073 决定二）：缺案件的单元成不了形。
	if _, err := domain.FormDeclarationUnit(
		mustValue(t, domain.NewDeclarationUnitID, "declaration-unit-3"),
		domain.CustomsCaseID{},
		mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		[]domain.DeclaredParcelReference{
			mustValue(t, domain.NewDeclaredParcelReference, "parcel-1"),
		},
	); !errors.Is(err, domain.ErrInvalidDeclarationUnit) {
		t.Fatalf("err = %v; 缺案件维的单元被收下了", err)
	}
}

// Covers: CC CONTEXT 生命周期「已就绪 → 不再就绪……原判断保留，但不得继续支持实际提交」与「提交授权与就绪判断分别形成和失效。只有两者在逻辑提交首次实际发送时均有效，才形成新的提交版本」
// ——撤销后的就绪固定不出版本；撤销保留原依据与时间；重复撤销拒；别的单元的就绪
// 支持不了这个单元。
func TestRevokedReadinessCannotSupportASubmission(t *testing.T) {
	ready := readiness(t)
	revoked, err := ready.Revoke("CREDENTIAL_EXPIRED", declaredAt.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if revoked.Effective() {
		t.Fatal("撤销后的就绪还有效")
	}
	if revoked.Basis().String() != "readiness-check/v1" || !revoked.JudgedAt().Equal(declaredAt) {
		t.Fatal("撤销改写了原判断——失效不是删除")
	}
	if _, err := revoked.Revoke("AGAIN", declaredAt.Add(time.Hour)); !errors.Is(err, domain.ErrReadinessAlreadyRevoked) {
		t.Fatalf("err = %v; 撤销撤了两次", err)
	}

	spec := versionSpec(t)
	spec.Readiness = revoked
	if _, err := domain.FixSubmissionVersion(spec); !errors.Is(err, domain.ErrInvalidSubmissionVersion) {
		t.Fatalf("err = %v; 不再就绪的判断支持了实际提交", err)
	}

	foreign, err := domain.JudgeReady(
		mustValue(t, domain.NewDeclarationUnitID, "declaration-unit-9"),
		mustValue(t, domain.NewReadinessBasisReference, "readiness-check/v9"),
		declaredAt,
	)
	if err != nil {
		t.Fatalf("judge foreign ready: %v", err)
	}
	spec = versionSpec(t)
	spec.Readiness = foreign
	if _, err := domain.FixSubmissionVersion(spec); !errors.Is(err, domain.ErrInvalidSubmissionVersion) {
		t.Fatalf("err = %v; 别的单元的就绪支持了这个单元", err)
	}
}

// Covers: CC CONTEXT「提交授权与就绪判断分别形成和失效。只有两者在逻辑提交首次实际发送时均有效，才形成新的提交版本」的授权半边——失效授权固定不出版本（就绪在场也不行）；失效保留原授予；
// 重复失效拒；别的单元的授权支持不了这个单元。
func TestRevokedAuthorizationCannotSupportASubmission(t *testing.T) {
	granted := authorization(t)
	revoked, err := granted.Revoke("MANDATE_WITHDRAWN", declaredAt.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("revoke authorization: %v", err)
	}
	if revoked.Effective() {
		t.Fatal("失效后的授权还有效")
	}
	if revoked.Authority().String() != "submit-authority/v1" || !revoked.GrantedAt().Equal(declaredAt) {
		t.Fatal("失效改写了原授予——失效不是删除")
	}
	if _, err := revoked.Revoke("AGAIN", declaredAt.Add(time.Hour)); !errors.Is(err, domain.ErrAuthorizationAlreadyRevoked) {
		t.Fatalf("err = %v; 失效失了两次", err)
	}

	spec := versionSpec(t)
	spec.Authorization = revoked
	if _, err := domain.FixSubmissionVersion(spec); !errors.Is(err, domain.ErrInvalidSubmissionVersion) {
		t.Fatalf("err = %v; 失效授权支持了实际提交——就绪单轨顶替了双轨", err)
	}

	foreign, err := domain.GrantSubmissionAuthority(
		mustValue(t, domain.NewDeclarationUnitID, "declaration-unit-9"),
		mustValue(t, domain.NewSubmissionAuthorityReference, "submit-authority/v9"),
		declaredAt,
	)
	if err != nil {
		t.Fatalf("grant foreign authority: %v", err)
	}
	spec = versionSpec(t)
	spec.Authorization = foreign
	if _, err := domain.FixSubmissionVersion(spec); !errors.Is(err, domain.ErrInvalidSubmissionVersion) {
		t.Fatalf("err = %v; 别的单元的授权支持了这个单元", err)
	}
}

// Covers: CC CONTEXT「同一次提交的通信超时或结果缺失必须保持待确认……才可以形成安全再次发送判断；在此之前不得盲目重发」——首次尝试随版本形成，
// 结果三值含待确认（超时不解释为失败）；无安全判断重发独立哨兵拒；受控重发换序号带
// 安全判断引用、前次尝试不动。
func TestResendIsControlledByTheSafeResendJudgment(t *testing.T) {
	version, err := domain.FixSubmissionVersion(versionSpec(t))
	if err != nil {
		t.Fatalf("fix submission version: %v", err)
	}

	first, err := domain.InitialAttempt(version, "channel-A", domain.AttemptPendingConfirmation, declaredAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("initial attempt: %v", err)
	}
	if first.Sequence() != 1 || first.Result() != domain.AttemptPendingConfirmation {
		t.Fatalf("first = %#v", first)
	}
	if _, has := first.SafeResend(); has {
		t.Fatal("首次尝试凭空带了安全重发判断")
	}

	if _, err := first.ControlledResend(
		domain.SafeResendReference{},
		domain.AttemptPendingConfirmation,
		declaredAt.Add(3*time.Hour),
	); !errors.Is(err, domain.ErrUnsafeResend) {
		t.Fatalf("err = %v; 盲目重发被收下了", err)
	}

	resend, err := first.ControlledResend(
		mustValue(t, domain.NewSafeResendReference, "safe-resend/attempt-1"),
		domain.AttemptAcknowledged,
		declaredAt.Add(4*time.Hour),
	)
	if err != nil {
		t.Fatalf("controlled resend: %v", err)
	}
	if resend.Sequence() != 2 || resend.Version() != first.Version() {
		t.Fatalf("resend = %#v; 受控重发复用同一版本换序号", resend)
	}
	safe, has := resend.SafeResend()
	if !has || safe.String() != "safe-resend/attempt-1" {
		t.Fatal("受控重发没带安全判断引用")
	}
	if first.Result() != domain.AttemptPendingConfirmation {
		t.Fatal("前次尝试被改写了")
	}
}
