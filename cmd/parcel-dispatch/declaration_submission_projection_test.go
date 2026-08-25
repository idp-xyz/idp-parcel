package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证 CONS-PROJ-DECL-B：CC 申报提交版本形成只投 VE 投影，不 FanOut 给 PS。
// 一封信带全体成员，消费侧按成员循环拆分（ADR-0066）；映射目录未配置必须未归类
// 入账，整封仍定稿。成员维进事实引用（引用跨版本稳定）、提交版本进版本维——这是
// 本消费面第一个真版本维。形成无语义分支，单一事实类型。不种 PAR-VIS-01，不登记
// tracking-projection.derived。

const (
	deriveDeclarationSubmissionConsumerName = "visibility-exception/derive-projection-from-declaration-submission"
	declarationSubmissionUnit               = "SYN-DECL-UNIT-01"
	declarationSubmissionProcedure          = "SYN-PROCEDURE-01"
	declarationSubmissionVersion            = "SYN-DECL-VER-01"
	declarationSubmissionMemberOne          = "SYN-PARCEL-01"
	declarationSubmissionMemberTwo          = "SYN-PARCEL-02"
)

// Covers: 生产 wireDispatcher 一拍——提交信封定稿 published==1，两个成员各得一条
// 未归类投影（成员维进引用、提交版本进版本维，ADR-0066）。不要学揽收 FanOut 去要
// published==0。
func TestAFormedDeclarationSubmissionDerivesAProjectionPerMemberAndPublishes(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	eventID := recordFormedDeclarationSubmission(t, fixture)

	published, err := fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("提交只投 VE 却没定稿：published = %d；失败码 = %q",
			published, recordedFailureCode(t, fixture.db, eventID))
	}

	assertUnclassifiedSubmissionProjection(t, fixture, declarationSubmissionMemberOne)
	assertUnclassifiedSubmissionProjection(t, fixture, declarationSubmissionMemberTwo)
	if n := fixture.countInbox(t, deriveDeclarationSubmissionConsumerName, eventID); n != 1 {
		t.Fatalf("VE inbox 行数 = %d, want 1", n)
	}
}

// recordFormedDeclarationSubmission 站在 CC 侧经领域真路径固定一份双成员提交版本并把
// 意图入队——单元成形 → 就绪+授权双有效 → 成版 → 首次尝试，夹具不手搓版本。信封只带
// 幂等键三维加版本维，成员快照由消费方按键重读本体取回。
func recordFormedDeclarationSubmission(t *testing.T, fixture *synVerticalFixture) string {
	t.Helper()

	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	submissions, err := ccpostgres.NewDeclarationSubmissions(fixture.db)
	if err != nil {
		t.Fatalf("构造申报链库：%v", err)
	}
	handoff, err := ccpostgres.NewOutboxDeclarationSubmissionHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造提交交接：%v", err)
	}

	tenant := mustCC(t, ccdomain.NewTenantID, fixture.identity.TenantID().String())
	procedure := mustCC(t, ccdomain.NewCustomsProcedureReference, declarationSubmissionProcedure)
	unitID := mustCC(t, ccdomain.NewDeclarationUnitID, declarationSubmissionUnit)
	fixedAt := time.Now().UTC().Add(-2 * time.Minute)

	customsCase := mustCC(t, ccdomain.NewCustomsCaseID, "SYN-CASE-01")
	formed, err := ccdomain.FormDeclarationUnit(
		unitID,
		customsCase,
		procedure,
		[]ccdomain.DeclaredParcelReference{
			mustCC(t, ccdomain.NewDeclaredParcelReference, declarationSubmissionMemberOne),
			mustCC(t, ccdomain.NewDeclaredParcelReference, declarationSubmissionMemberTwo),
		},
	)
	if err != nil {
		t.Fatalf("构造申报单元：%v", err)
	}
	readiness, err := ccdomain.JudgeReady(
		unitID,
		mustCC(t, ccdomain.NewReadinessBasisReference, "SYN-READINESS-01"),
		fixedAt.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("构造就绪判断：%v", err)
	}
	authorization, err := ccdomain.GrantSubmissionAuthority(
		unitID,
		mustCC(t, ccdomain.NewSubmissionAuthorityReference, "SYN-AUTHORITY-01"),
		fixedAt.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("构造提交授权：%v", err)
	}
	version, err := ccdomain.FixSubmissionVersion(ccdomain.CustomsSubmissionVersionSpec{
		ID:            mustCC(t, ccdomain.NewSubmissionVersionID, declarationSubmissionVersion),
		Unit:          formed,
		Dossier:       mustCC(t, ccdomain.NewDossierSnapshotReference, "SYN-DOSSIER-01"),
		Roles:         mustCC(t, ccdomain.NewRoleSnapshotReference, "SYN-ROLES-01"),
		Readiness:     readiness,
		Authorization: authorization,
		FixedAt:       fixedAt,
	})
	if err != nil {
		t.Fatalf("固定提交版本：%v", err)
	}
	attempt, err := ccdomain.InitialAttempt(
		version, "SYN-GATEWAY-01", ccdomain.AttemptPendingConfirmation, fixedAt.Add(30*time.Second))
	if err != nil {
		t.Fatalf("构造首次尝试：%v", err)
	}

	record := ccports.DeclarationSubmissionRecord{
		Key: ccports.DeclarationSubmissionKey{
			TenantID:  tenant,
			Unit:      unitID,
			Procedure: procedure,
		},
		ContentDigest: "SYN-DECL-DIGEST-01",
		Version:       version,
		Attempt:       attempt,
		RecordedAt:    fixedAt.Add(time.Minute),
	}
	var outcome ccports.DeclarationSubmissionSaveOutcome
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var saveErr error
		if outcome, saveErr = submissions.Save(txCtx, record); saveErr != nil {
			return saveErr
		}
		return handoff.HandOffDeclarationSubmission(txCtx,
			ccports.DeclarationSubmissionHandoffIntent{Record: record, Case: customsCase})
	})
	if outcome != ccports.DeclarationSubmissionSaved {
		t.Fatalf("save outcome = %v, want 已写入", outcome)
	}
	// 信封 ID 取提交幂等键三维加版本维（原案内更正在同一目标下换版出第二封，ID 按
	// 版本认领才不被 EnqueueOnce 吞掉），与 OutboxDeclarationSubmissionHandoff 的
	// ADR-0043 认领一致。
	return tenant.String() + "/" + declarationSubmissionUnit + "/" +
		declarationSubmissionProcedure + "/" + declarationSubmissionVersion
}

func assertUnclassifiedSubmissionProjection(t *testing.T, fixture *synVerticalFixture, member string) {
	t.Helper()

	tenant := mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String())
	parcel := mustVE(t, vedomain.NewTrackedParcelReference, member)

	projections, err := vepostgres.NewProjections(fixture.db)
	if err != nil {
		t.Fatalf("构造投影读口：%v", err)
	}
	projection, found, err := projections.FindCurrent(t.Context(), tenant, parcel)
	if err != nil {
		t.Fatalf("读当前投影：%v", err)
	}
	if !found {
		t.Fatalf("成员 %s 没有当前投影——逐成员拆分没走到它", member)
	}
	if len(projection.Entries()) != 1 {
		t.Fatalf("成员 %s entries = %d, want 1", member, len(projection.Entries()))
	}
	entry := projection.Entries()[0]
	if _, classified := entry.Milestone(); classified {
		t.Fatal("映射未配置却归了类——禁止发明里程碑")
	}
	if entry.MappingVersion().String() != "MAPPING_NOT_CONFIGURED" {
		t.Fatalf("mapping = %q, want MAPPING_NOT_CONFIGURED", entry.MappingVersion())
	}
	if entry.Fact().Kind().String() != "declaration-submission-formed" {
		t.Fatalf("kind = %q, want declaration-submission-formed（形成无语义分支，单一类型）", entry.Fact().Kind())
	}
	wantFact := "declaration-submission/" + member + "/" +
		declarationSubmissionUnit + "/" + declarationSubmissionProcedure
	if entry.Fact().Fact().String() != wantFact {
		t.Fatalf("fact = %q, want %s（成员维进引用，版本不进引用）", entry.Fact().Fact(), wantFact)
	}
	if entry.Fact().Version().String() != declarationSubmissionVersion {
		t.Fatalf("version = %q, want %s（提交版本走真版本维）", entry.Fact().Version(), declarationSubmissionVersion)
	}
}
