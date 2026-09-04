package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 监管凭证登记面的行为面（票 mechanism-executor-triage/07 CC-a）：受理门逐格拒、重放/
// 冲突两格分得开、额度「未提供」如实落册不补齐、依赖故障折未决。替身照真库代数——同键
// 只答`已登记`绝不顶替（credential_registry.go 同一段规则的内存版）。

var credentialValidFrom = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

type credentialRow struct {
	tenant     string
	credential domain.RegulatoryCredential
}

type credentialStoreDouble struct {
	rows        []credentialRow
	registerErr error
	loadErr     error
}

func (double *credentialStoreDouble) RegisterCredential(
	_ context.Context,
	tenant domain.TenantID,
	credential domain.RegulatoryCredential,
) (ports.CaseConfigurationSaveOutcome, error) {
	if double.registerErr != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, double.registerErr
	}
	for _, row := range double.rows {
		if row.tenant == tenant.String() && row.credential.ID() == credential.ID() {
			return ports.CaseConfigurationAlreadyRegistered, nil
		}
	}
	double.rows = append(double.rows, credentialRow{tenant: tenant.String(), credential: credential})
	return ports.CaseConfigurationRegistered, nil
}

func (double *credentialStoreDouble) LoadCredential(
	_ context.Context,
	tenant domain.TenantID,
	id domain.CredentialID,
) (domain.RegulatoryCredential, bool, error) {
	if double.loadErr != nil {
		return domain.RegulatoryCredential{}, false, double.loadErr
	}
	for _, row := range double.rows {
		if row.tenant == tenant.String() && row.credential.ID() == id {
			return row.credential, true, nil
		}
	}
	return domain.RegulatoryCredential{}, false, nil
}

func newCredentialHandler(store *credentialStoreDouble) *application.RegisterCredentialHandler {
	return application.NewRegisterCredentialHandler(application.RegisterCredentialDeps{
		Registry: store,
		View:     store,
	})
}

func credentialCommand(t *testing.T) application.RegisterCredentialCommand {
	t.Helper()
	return application.RegisterCredentialCommand{
		TenantID:  configValue(t, domain.NewTenantID, "tenant-a"),
		ID:        configValue(t, domain.NewCredentialID, "SYN-CRED-01"),
		Issuer:    configValue(t, domain.NewRegulatoryAuthorityReference, "SYN-AUTHORITY-01"),
		Holder:    configValue(t, domain.NewCredentialHolderReference, "SYN-HOLDER-01"),
		Procedure: configValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT"),
		ValidFrom: credentialValidFrom,
		ValidTo:   credentialValidFrom.Add(365 * 24 * time.Hour),
		Uses:      12,
	}
}

func TestRegisteringACredentialLands(t *testing.T) {
	store := &credentialStoreDouble{}

	outcome, err := newCredentialHandler(store).Handle(t.Context(), credentialCommand(t))
	if err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("凭证登记：err=%v outcome=%v", err, outcome)
	}
	if len(store.rows) != 1 {
		t.Fatalf("册上行数走样：%d", len(store.rows))
	}
	uses, provided := store.rows[0].credential.Uses()
	if uses != 12 || !provided {
		t.Fatalf("次数额度没如实落册：uses=%d provided=%v", uses, provided)
	}
}

// 额度未提供必须明确记录为未提供，不猜测补齐——零进去、读口答「未提供」，不是「零次」。
func TestRegisteringACredentialWithoutAUsesQuotaRecordsItAsNotProvided(t *testing.T) {
	store := &credentialStoreDouble{}
	command := credentialCommand(t)
	command.Uses = 0

	outcome, err := newCredentialHandler(store).Handle(t.Context(), command)
	if err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("无额度登记：err=%v outcome=%v", err, outcome)
	}
	if _, provided := store.rows[0].credential.Uses(); provided {
		t.Fatal("未提供的额度被当成了已提供")
	}
}

// 受理门逐格拒：租户与领域构造拒的每一格（身份/机构/持有人/程序缺席、期限倒置、额度为负）
// 都不受理，被拒的登记不落册。领域那几格由 RegisterCredential 自己判，这里只钉「拒了且没落」。
func TestCredentialRegistrationRefusesWhatTheDomainRefuses(t *testing.T) {
	store := &credentialStoreDouble{}
	handler := newCredentialHandler(store)

	blankTenant := credentialCommand(t)
	blankTenant.TenantID = domain.TenantID{}
	blankHolder := credentialCommand(t)
	blankHolder.Holder = domain.CredentialHolderReference{}
	inverted := credentialCommand(t)
	inverted.ValidTo = inverted.ValidFrom.Add(-time.Hour)
	negativeUses := credentialCommand(t)
	negativeUses.Uses = -1

	for name, command := range map[string]application.RegisterCredentialCommand{
		"租户":   blankTenant,
		"持有人":  blankHolder,
		"期限倒置": inverted,
		"额度为负": negativeUses,
	} {
		outcome, err := handler.Handle(t.Context(), command)
		if err != nil || outcome != application.ConfigurationNotAccepted {
			t.Fatalf("凭证登记缺/错%s该拒：err=%v outcome=%v", name, err, outcome)
		}
	}
	if len(store.rows) != 0 {
		t.Fatal("被拒的登记落了册")
	}
}

// 同身份重放同内容是`已存在`；同身份换任何一件（这里换持有人与额度）是`内容冲突`，册面
// 纹丝不动——凭证是不可变版本，换内容是另一张凭证，不是覆盖。
func TestReRegisteringACredentialSplitsReplayFromConflict(t *testing.T) {
	store := &credentialStoreDouble{}
	handler := newCredentialHandler(store)

	if outcome, err := handler.Handle(t.Context(), credentialCommand(t)); err != nil ||
		outcome != application.ConfigurationRegistered {
		t.Fatalf("首登：err=%v outcome=%v", err, outcome)
	}
	if outcome, err := handler.Handle(t.Context(), credentialCommand(t)); err != nil ||
		outcome != application.ConfigurationExisting {
		t.Fatalf("重放该是`已存在`：err=%v outcome=%v", err, outcome)
	}

	changedHolder := credentialCommand(t)
	changedHolder.Holder = configValue(t, domain.NewCredentialHolderReference, "SYN-HOLDER-02")
	if outcome, err := handler.Handle(t.Context(), changedHolder); err != nil ||
		outcome != application.ConfigurationContentConflict {
		t.Fatalf("换持有人该是`内容冲突`：err=%v outcome=%v", err, outcome)
	}
	changedUses := credentialCommand(t)
	changedUses.Uses = 0
	if outcome, err := handler.Handle(t.Context(), changedUses); err != nil ||
		outcome != application.ConfigurationContentConflict {
		t.Fatalf("额度从已提供变未提供该是`内容冲突`：err=%v outcome=%v", err, outcome)
	}
	if len(store.rows) != 1 || store.rows[0].credential.Holder().String() != "SYN-HOLDER-01" {
		t.Fatalf("冲突顶掉了在册凭证：%+v", store.rows)
	}
}

func TestCredentialRegistrationDependencyFailuresAreUndecided(t *testing.T) {
	writerDown := &credentialStoreDouble{registerErr: errors.New("writer unavailable")}
	outcome, err := newCredentialHandler(writerDown).Handle(t.Context(), credentialCommand(t))
	if err != nil || outcome != application.ConfigurationUndecided {
		t.Fatalf("写口故障该未决：err=%v outcome=%v", err, outcome)
	}

	viewDown := &credentialStoreDouble{}
	handler := newCredentialHandler(viewDown)
	if outcome, err := handler.Handle(t.Context(), credentialCommand(t)); err != nil ||
		outcome != application.ConfigurationRegistered {
		t.Fatalf("首登：err=%v outcome=%v", err, outcome)
	}
	viewDown.loadErr = errors.New("view unavailable")
	outcome, err = handler.Handle(t.Context(), credentialCommand(t))
	if err != nil || outcome != application.ConfigurationUndecided {
		t.Fatalf("已在册但读不回该未决：err=%v outcome=%v", err, outcome)
	}
}
