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

// 监管凭证登记册写口与读口的往返用例（0014 建表，票 mechanism-executor-triage/07 CC-a）。
// 做法承 case_config_registry_test.go 头注那两条：断言穿读口取回、写入一律进环境事务。
// 覆盖的写入代数与其余登记册同款——不可覆盖、同键只答`已登记`——但按本册的键与七件
// 逐格钉一遍：这张表的约束与 SQL 是新写的，先例的绿证不了它们。

var credentialBaseAt = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

func newCredentialRegistry(t *testing.T) (*adapter.CredentialRegistrations, *adapter.CredentialView, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	registry, err := adapter.NewCredentialRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造凭证写口：%v", err)
	}
	view, err := adapter.NewCredentialView(fixture.db)
	if err != nil {
		t.Fatalf("构造凭证读口：%v", err)
	}
	return registry, view, fixture
}

// synCredential 构造一张七件齐全的合成凭证；uses 由用例给，因为「未提供」那一格要单独钉。
func synCredential(t *testing.T, holder string, uses int) domain.RegulatoryCredential {
	t.Helper()
	credential, err := domain.RegisterCredential(
		viewValue(t, domain.NewCredentialID, "SYN-CRED-01"),
		viewValue(t, domain.NewRegulatoryAuthorityReference, "SYN-AUTHORITY-01"),
		viewValue(t, domain.NewCredentialHolderReference, holder),
		viewValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT"),
		credentialBaseAt, credentialBaseAt.Add(365*24*time.Hour), uses)
	if err != nil {
		t.Fatalf("构造合成凭证：%v", err)
	}
	return credential
}

func registerCredential(
	t *testing.T,
	fixture *viewFixture,
	registry *adapter.CredentialRegistrations,
	credential domain.RegulatoryCredential,
) (ports.CaseConfigurationSaveOutcome, error) {
	t.Helper()
	return register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterCredential(ctx, viewValue(t, domain.NewTenantID, "tenant-a"), credential)
	})
}

func loadCredential(t *testing.T, view *adapter.CredentialView) (domain.RegulatoryCredential, bool, error) {
	t.Helper()
	return view.LoadCredential(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"),
		viewValue(t, domain.NewCredentialID, "SYN-CRED-01"))
}

// Covers: 登记往返——七件逐格如实读回，额度已提供那一格连同「已提供」一起回来。
func TestACredentialRoundTripsThroughTheView(t *testing.T) {
	registry, view, fixture := newCredentialRegistry(t)
	credential := synCredential(t, "SYN-HOLDER-01", 12)

	outcome, err := registerCredential(t, fixture, registry, credential)
	if err != nil || outcome != ports.CaseConfigurationRegistered {
		t.Fatalf("首次登记：err=%v outcome=%v", err, outcome)
	}
	loaded, found, err := loadCredential(t, view)
	if err != nil || !found {
		t.Fatalf("登记后读不回：err=%v found=%v", err, found)
	}
	uses, provided := loaded.Uses()
	if loaded.ID() != credential.ID() || loaded.Issuer() != credential.Issuer() ||
		loaded.Holder() != credential.Holder() || loaded.Procedure() != credential.Procedure() ||
		!loaded.ValidFrom().Equal(credential.ValidFrom()) || !loaded.ValidTo().Equal(credential.ValidTo()) ||
		uses != 12 || !provided {
		t.Fatalf("凭证行走样：%+v", loaded)
	}
}

// Covers: 额度未提供（零）原样落册、原样读回为「未提供」——不猜测补齐，也不被读成零次。
func TestAnAbsentUsesQuotaRoundTripsAsNotProvided(t *testing.T) {
	registry, view, fixture := newCredentialRegistry(t)

	if _, err := registerCredential(t, fixture, registry, synCredential(t, "SYN-HOLDER-01", 0)); err != nil {
		t.Fatalf("登记：%v", err)
	}
	loaded, found, err := loadCredential(t, view)
	if err != nil || !found {
		t.Fatalf("读不回：err=%v found=%v", err, found)
	}
	if _, provided := loaded.Uses(); provided {
		t.Fatal("未提供的额度被读成了已提供")
	}
}

// Covers: 同身份重登交回`已登记`且不顶替——换持有人也一样，库里仍是首版；内容是否同一份
// 由编排读回自己比（写口不判）。
func TestSameCredentialIdentityNeverReplacesTheFirstVersion(t *testing.T) {
	registry, view, fixture := newCredentialRegistry(t)

	if _, err := registerCredential(t, fixture, registry, synCredential(t, "SYN-HOLDER-01", 12)); err != nil {
		t.Fatalf("首次登记：%v", err)
	}
	outcome, err := registerCredential(t, fixture, registry, synCredential(t, "SYN-HOLDER-02", 3))
	if err != nil || outcome != ports.CaseConfigurationAlreadyRegistered {
		t.Fatalf("同身份重登该交回`已登记`：err=%v outcome=%v", err, outcome)
	}
	loaded, found, err := loadCredential(t, view)
	if err != nil || !found || loaded.Holder().String() != "SYN-HOLDER-01" {
		t.Fatalf("首版被顶替：err=%v found=%v holder=%v", err, found, loaded.Holder())
	}
}

// Covers: 未登记的凭证读口答 found=false 而不是错误——实例半边没到是如实答案，不是故障。
func TestAnUnregisteredCredentialIsAbsentNotBroken(t *testing.T) {
	_, view, _ := newCredentialRegistry(t)

	if _, found, err := loadCredential(t, view); err != nil || found {
		t.Fatalf("未登记该 found=false：err=%v found=%v", err, found)
	}
}

// Covers: 库内再守一遍领域不变量——期限倒置与负额度的旁路写入被 CHECK 挡在门外。
func TestTheCredentialTableRejectsWhatTheDomainRejects(t *testing.T) {
	_, _, fixture := newCredentialRegistry(t)
	insert := `INSERT INTO customs_compliance.regulatory_credential
		(tenant_id, credential_id, issuer_ref, holder_ref, procedure_ref, valid_from, valid_to, uses, registered_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	fixture.rejects(t, "期限倒置", insert,
		"tenant-a", "SYN-CRED-X", "SYN-AUTHORITY-01", "SYN-HOLDER-01", "SYN-PROC-IMPORT",
		credentialBaseAt, credentialBaseAt.Add(-time.Hour), 1, credentialBaseAt)
	fixture.rejects(t, "负额度", insert,
		"tenant-a", "SYN-CRED-X", "SYN-AUTHORITY-01", "SYN-HOLDER-01", "SYN-PROC-IMPORT",
		credentialBaseAt, credentialBaseAt.Add(time.Hour), -1, credentialBaseAt)
	fixture.rejects(t, "持有人空白", insert,
		"tenant-a", "SYN-CRED-X", "SYN-AUTHORITY-01", "   ", "SYN-PROC-IMPORT",
		credentialBaseAt, credentialBaseAt.Add(time.Hour), 1, credentialBaseAt)
}

// Covers: 写口在无事务上下文一律被 RequireExecutor 拒绝（ErrTransactionRequired），不退回
// 连接池旁路写入——判据同包 transaction_guard_test.go 那族，随适配器逐个成立。
func TestCredentialWritesRefuseToRunOutsideATransaction(t *testing.T) {
	registry, _, _ := newCredentialRegistry(t)

	if _, err := registry.RegisterCredential(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"),
		synCredential(t, "SYN-HOLDER-01", 1)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记凭证应返回 ErrTransactionRequired，实得：%v", err)
	}
}
