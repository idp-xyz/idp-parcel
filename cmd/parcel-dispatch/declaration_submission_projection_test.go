package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证 CONS-PROJ-DECL-B：CC 申报提交版本形成只投 VE 投影，不 FanOut 给 PS。
// 一封信带全体成员，消费侧按成员循环拆分（ADR-0066）；映射目录未配置必须未归类
// 入账，整封仍定稿。成员维进事实引用（引用跨版本稳定）、提交版本进版本维——这是
// 本消费面第一个真版本维。形成无语义分支，单一事实类型。不种 PAR-VIS-01；派生信封
// tracking-projection.derived 由生产 wireDispatcher 登记进客户视图链（WIRE-CUSTOMER-VIEW），
// 本文件不为测试另装订阅者——它们在下一拍定稿，数拍内 published 时要把它们算进去。

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

// Covers: 票 sa-cc/34 裁决 5 / 判据 (4)——同一提交版本事实以两个不同的信封 ID 各投一封（指纹形与 `760332c7` 上的
// 四维串接形），Inbox 门按 ID 放行第二封，VE 幂等靠 ports.FactKey + 内容摘要：两封各自 PUBLISHED、两行 inbox、成员
// 投影仍各一条、派生信封仍只有首封那两封。「两封都定稿、派生信封不增」即第二封在 DeriveProjectionHandler 里答了
// FactExistingResult（理由与两拍各定稿什么见案件那格）。
func TestTheSameDeclarationSubmissionUnderTwoEnvelopeIDsDerivesOnce(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	fingerprintID := recordFormedDeclarationSubmission(t, fixture)
	concatenatedID := fixture.identity.TenantID().String() + "/" + declarationSubmissionUnit + "/" +
		declarationSubmissionProcedure + "/" + declarationSubmissionVersion
	reissueUnderAnotherEnvelopeID(t, fixture, fingerprintID, concatenatedID)

	// 两封同分区、分区一次只放一个头，所以要两拍；第 2 拍定稿的是重发那封加第 1 拍入队的两封派生信封，拍内 published
	// 因此只弱断，定稿与否按两封各自的 status 断。
	for beat := 1; beat <= 2; beat++ {
		published, err := fixture.beat.DispatchOnce(ctx)
		if err != nil {
			t.Fatalf("第 %d 拍：%v", beat, err)
		}
		if published < 1 {
			t.Fatalf("第 %d 拍一封都没定稿；失败码 = %q / %q", beat,
				recordedFailureCode(t, fixture.db, fingerprintID), recordedFailureCode(t, fixture.db, concatenatedID))
		}
	}
	for _, id := range []string{fingerprintID, concatenatedID} {
		if status := outboxStatus(t, fixture.db, id); status != "PUBLISHED" {
			t.Fatalf("两封都该定稿：%s 的 status = %q, want PUBLISHED；失败码 = %q",
				id, status, recordedFailureCode(t, fixture.db, id))
		}
		if n := fixture.countInbox(t, deriveDeclarationSubmissionConsumerName, id); n != 1 {
			t.Fatalf("Inbox 门该按 ID 放行每一封：%s 的 inbox 行数 = %d, want 1", id, n)
		}
	}
	if n := fixture.countOutboxOfType(t, trackingProjectionDerivedType); n != 2 {
		t.Fatalf("派生信封 = %d, want 2（首封每成员一封；第二封同键同摘要不得再派生）", n)
	}
	assertUnclassifiedSubmissionProjection(t, fixture, declarationSubmissionMemberOne)
	assertUnclassifiedSubmissionProjection(t, fixture, declarationSubmissionMemberTwo)
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
	// 信封 ID 由提交幂等键三维加版本维按生产同一公式重算（票 sa-cc/34 裁决 3：口名 + 四维全进哈希；原案内更正
	// 在同一目标下换版出第二封，版本维进 ID 才不被 EnqueueOnce 吞掉），与 OutboxDeclarationSubmissionHandoff 的
	// ADR-0043 认领一致；这里不再写字面串接。
	return string(outboxintent.FingerprintEventID("declaration-submission",
		tenant.String(), declarationSubmissionUnit, declarationSubmissionProcedure, declarationSubmissionVersion))
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
