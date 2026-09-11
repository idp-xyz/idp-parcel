package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 凭证门禁判断登记册写口与读口的往返用例（0018 建表，票 sa-cc/04）。做法承 credential_registry_test.go
// 头注：断言穿读口取回、写入一律进环境事务。写入代数与其余登记册同款——不可覆盖、同键只答`已登记`
// ——但按本册的键（三维 + 指纹）与各列逐格钉一遍：这张表的约束与 SQL 是新写的，先例的绿证不了它们。

var (
	gateAsOf     = time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC)
	gateJudgedAt = time.Date(2026, 9, 11, 10, 30, 0, 0, time.UTC)
)

func newCredentialGateRegistry(t *testing.T) (*adapter.CredentialGateRegistrations, *adapter.CredentialGateView, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	registry, err := adapter.NewCredentialGateRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造凭证门禁写口：%v", err)
	}
	view, err := adapter.NewCredentialGateView(fixture.db)
	if err != nil {
		t.Fatalf("构造凭证门禁读口：%v", err)
	}
	return registry, view, fixture
}

// synGateRecord 构造一条各维齐全的合成判断连同它的幂等键；结论与依据由用例给，因为四格与换版各要单独钉。
func synGateRecord(t *testing.T, conclusion domain.CredentialGateConclusion, basis string) ports.CredentialGateRecord {
	t.Helper()
	judgment, err := domain.RecordCredentialGate(domain.CredentialGateSpec{
		Unit:       viewValue(t, domain.NewDeclarationUnitID, "SYN-UNIT-01"),
		Credential: viewValue(t, domain.NewCredentialID, "SYN-CRED-01"),
		Procedure:  viewValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT"),
		Holder:     viewValue(t, domain.NewCredentialHolderReference, "SYN-HOLDER-01"),
		AsOf:       gateAsOf,
		Conclusion: conclusion,
		Basis:      viewValue(t, domain.NewCredentialGateBasisReference, basis),
		Role:       viewValue(t, domain.NewResponsibleRoleReference, "SYN-ROLE-CUSTOMS-ASSESSOR"),
		JudgedAt:   gateJudgedAt,
	})
	if err != nil {
		t.Fatalf("构造合成判断：%v", err)
	}
	return ports.CredentialGateRecord{
		Key: ports.CredentialGateKey{
			TenantID:   viewValue(t, domain.NewTenantID, "tenant-a"),
			Unit:       judgment.Unit(),
			Credential: judgment.Credential(),
			Digest:     ports.CredentialGateDigest(judgment),
		},
		Judgment: judgment,
	}
}

func registerGate(
	t *testing.T,
	fixture *viewFixture,
	registry *adapter.CredentialGateRegistrations,
	record ports.CredentialGateRecord,
) (ports.CaseConfigurationSaveOutcome, error) {
	t.Helper()
	return register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterCredentialGate(ctx, record)
	})
}

// Covers: 登记往返——逐格如实读回（AT-CC-056：适用性与截至时点都在），两个时间各归各轴。
func TestACredentialGateJudgmentRoundTripsThroughTheView(t *testing.T) {
	registry, view, fixture := newCredentialGateRegistry(t)
	record := synGateRecord(t, domain.CredentialGateApplicable, "SYN-CRED-EVIDENCE/2026-09")

	outcome, err := registerGate(t, fixture, registry, record)
	if err != nil || outcome != ports.CaseConfigurationRegistered {
		t.Fatalf("首次登记：err=%v outcome=%v", err, outcome)
	}
	loaded, found, err := view.LoadCredentialGate(t.Context(), record.Key)
	if err != nil || !found {
		t.Fatalf("登记后读不回：err=%v found=%v", err, found)
	}
	want, got := record.Judgment, loaded.Judgment
	if loaded.Key != record.Key ||
		got.Unit() != want.Unit() || got.Credential() != want.Credential() ||
		got.Procedure() != want.Procedure() || got.Holder() != want.Holder() ||
		!got.AsOf().Equal(want.AsOf()) || got.Conclusion() != want.Conclusion() ||
		got.Basis() != want.Basis() || got.Role() != want.Role() || !got.JudgedAt().Equal(want.JudgedAt()) {
		t.Fatalf("判断行走样：%+v", loaded)
	}
}

// Covers: 四格各自落得进、读得回同一格——「凭证未登记」与「不适用」在库里是两个词，不压成一格。
func TestEveryCredentialGateConclusionRoundTripsAsItsOwnWord(t *testing.T) {
	registry, view, fixture := newCredentialGateRegistry(t)

	for _, conclusion := range []domain.CredentialGateConclusion{
		domain.CredentialGateApplicable,
		domain.CredentialGateNotApplicable,
		domain.CredentialGateCredentialNotRegistered,
		domain.CredentialGateUndecided,
	} {
		record := synGateRecord(t, conclusion, "SYN-CRED-EVIDENCE/2026-09")
		if outcome, err := registerGate(t, fixture, registry, record); err != nil || outcome != ports.CaseConfigurationRegistered {
			t.Fatalf("%s 登记：err=%v outcome=%v", conclusion, err, outcome)
		}
		loaded, found, err := view.LoadCredentialGate(t.Context(), record.Key)
		if err != nil || !found || loaded.Judgment.Conclusion() != conclusion {
			t.Fatalf("%s 读回走样：err=%v found=%v got=%v", conclusion, err, found, loaded.Judgment.Conclusion())
		}
	}
}

// Covers: 同键（三维 + 指纹）重登交回`已登记`且不顶替；换内容（这里换依据引用）换指纹追加新版，
// 首版原样在册——判断是不可覆盖的版本。
func TestSameCredentialGateKeyNeverReplacesAndChangedContentAppendsAVersion(t *testing.T) {
	registry, view, fixture := newCredentialGateRegistry(t)
	first := synGateRecord(t, domain.CredentialGateNotApplicable, "SYN-CRED-EVIDENCE/2026-09")

	if _, err := registerGate(t, fixture, registry, first); err != nil {
		t.Fatalf("首次登记：%v", err)
	}
	outcome, err := registerGate(t, fixture, registry, first)
	if err != nil || outcome != ports.CaseConfigurationAlreadyRegistered {
		t.Fatalf("同键重登该交回`已登记`：err=%v outcome=%v", err, outcome)
	}

	second := synGateRecord(t, domain.CredentialGateApplicable, "SYN-CRED-EVIDENCE/2026-10")
	if second.Key == first.Key {
		t.Fatal("换内容没有换指纹")
	}
	outcome, err = registerGate(t, fixture, registry, second)
	if err != nil || outcome != ports.CaseConfigurationRegistered {
		t.Fatalf("新版该落成：err=%v outcome=%v", err, outcome)
	}
	loaded, found, err := view.LoadCredentialGate(t.Context(), first.Key)
	if err != nil || !found || loaded.Judgment.Conclusion() != domain.CredentialGateNotApplicable {
		t.Fatalf("首版被顶替：err=%v found=%v conclusion=%v", err, found, loaded.Judgment.Conclusion())
	}
}

// Covers: 键与判断对象说的不是同一件事（指纹对不上、三维对不上）——写口响亮拒，不让库里出现
// 按 A 查、内容是 B 的行。
func TestCredentialGateWritesRefuseAKeyThatDisagreesWithTheJudgment(t *testing.T) {
	registry, _, fixture := newCredentialGateRegistry(t)

	wrongDigest := synGateRecord(t, domain.CredentialGateApplicable, "SYN-CRED-EVIDENCE/2026-09")
	wrongDigest.Key.Digest = "not-the-digest-of-this-judgment"
	if _, err := registerGate(t, fixture, registry, wrongDigest); err == nil {
		t.Fatal("指纹对不上的键被接受")
	}

	wrongUnit := synGateRecord(t, domain.CredentialGateApplicable, "SYN-CRED-EVIDENCE/2026-09")
	wrongUnit.Key.Unit = viewValue(t, domain.NewDeclarationUnitID, "SYN-UNIT-02")
	if _, err := registerGate(t, fixture, registry, wrongUnit); err == nil {
		t.Fatal("单元对不上的键被接受")
	}
}

// Covers: 未登记的那一版读口答 found=false 而不是错误；空指纹是调用方编程错误，作错误抛出。
func TestAnAbsentCredentialGateVersionIsAbsentNotBroken(t *testing.T) {
	_, view, _ := newCredentialGateRegistry(t)
	record := synGateRecord(t, domain.CredentialGateApplicable, "SYN-CRED-EVIDENCE/2026-09")

	if _, found, err := view.LoadCredentialGate(t.Context(), record.Key); err != nil || found {
		t.Fatalf("未登记该 found=false：err=%v found=%v", err, found)
	}
	blank := record.Key
	blank.Digest = ""
	if _, _, err := view.LoadCredentialGate(t.Context(), blank); err == nil {
		t.Fatal("空指纹被当成正常查询")
	}
}

// Covers: 库内再守一遍领域不变量——结论集外、依据空白、角色空白的旁路写入被 CHECK 挡在门外。
func TestTheCredentialGateTableRejectsWhatTheDomainRejects(t *testing.T) {
	_, _, fixture := newCredentialGateRegistry(t)
	insert := `INSERT INTO customs_compliance.credential_gate_judgment
		(tenant_id, unit_id, credential_id, version_digest, procedure_ref, holder_ref, as_of, conclusion, basis_ref, role_ref, judged_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	fixture.rejects(t, "结论集外", insert,
		"tenant-a", "SYN-UNIT-X", "SYN-CRED-X", "digest-1", "SYN-PROC-IMPORT", "SYN-HOLDER-01",
		gateAsOf, "MAYBE", "SYN-CRED-EVIDENCE/2026-09", "SYN-ROLE-CUSTOMS-ASSESSOR", gateJudgedAt)
	fixture.rejects(t, "依据空白", insert,
		"tenant-a", "SYN-UNIT-X", "SYN-CRED-X", "digest-1", "SYN-PROC-IMPORT", "SYN-HOLDER-01",
		gateAsOf, "APPLICABLE", "   ", "SYN-ROLE-CUSTOMS-ASSESSOR", gateJudgedAt)
	fixture.rejects(t, "角色空白", insert,
		"tenant-a", "SYN-UNIT-X", "SYN-CRED-X", "digest-1", "SYN-PROC-IMPORT", "SYN-HOLDER-01",
		gateAsOf, "APPLICABLE", "SYN-CRED-EVIDENCE/2026-09", "", gateJudgedAt)
}

// Covers: 写口在无事务上下文一律被 RequireExecutor 拒绝（ErrTransactionRequired），不退回连接池
// 旁路写入——判据同包 transaction_guard_test.go 那族，随适配器逐个成立。
func TestCredentialGateWritesRefuseToRunOutsideATransaction(t *testing.T) {
	registry, _, _ := newCredentialGateRegistry(t)

	if _, err := registry.RegisterCredentialGate(t.Context(),
		synGateRecord(t, domain.CredentialGateApplicable, "SYN-CRED-EVIDENCE/2026-09")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记凭证门禁判断应返回 ErrTransactionRequired，实得：%v", err)
	}
}
