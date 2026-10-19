package postgres_test

import (
	"context"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// 票 tf-face-algebra/11 的真库回归：凭证登记编排接真登记册（timestamptz 只存到微秒）。按纳秒判时，微秒以下有非零位的
// 有效期原样重放被判成另一张凭证；零时刻之后不足一微秒的起点入册后读回成零时刻，整租户的凭证列面重建不出来。

type credentialChannel struct {
	fixture   *viewFixture
	handler   *customsapp.RegisterCredentialHandler
	catalogue *adapter.CredentialCatalogue
	tenant    domain.TenantID
}

func newCredentialChannel(t *testing.T) credentialChannel {
	t.Helper()
	registry, view, fixture := newCredentialRegistry(t)
	catalogue, err := adapter.NewCredentialCatalogue(fixture.db)
	if err != nil {
		t.Fatalf("构造凭证列面：%v", err)
	}
	return credentialChannel{
		fixture:   fixture,
		handler:   customsapp.NewRegisterCredentialHandler(customsapp.RegisterCredentialDeps{Registry: registry, View: view}),
		catalogue: catalogue,
		tenant:    viewValue(t, domain.NewTenantID, "SYN-TENANT-TFA11"),
	}
}

// register 在环境事务里跑一次编排，与生产进程级入口同形：编排答了什么，事务就按什么提交。
func (channel credentialChannel) register(t *testing.T, id string, from, to time.Time) customsapp.CaseConfigurationOutcome {
	t.Helper()
	var outcome customsapp.CaseConfigurationOutcome
	err := channel.fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var handleErr error
		outcome, handleErr = channel.handler.Handle(txCtx, customsapp.RegisterCredentialCommand{
			TenantID:  channel.tenant,
			ID:        viewValue(t, domain.NewCredentialID, id),
			Issuer:    viewValue(t, domain.NewRegulatoryAuthorityReference, "SYN-AUTHORITY-TFA11"),
			Holder:    viewValue(t, domain.NewCredentialHolderReference, "SYN-HOLDER-TFA11"),
			Procedure: viewValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-TFA11"),
			ValidFrom: from,
			ValidTo:   to,
			Uses:      3,
		})
		return handleErr
	})
	if err != nil {
		t.Fatalf("登记 %s 没形成答案：%v", id, err)
	}
	return outcome
}

func TestCredentialReplayWithSubMicrosecondValidityIsExistingOnTheRealRegistry(t *testing.T) {
	channel := newCredentialChannel(t)
	validTo := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, raw := range []string{"2026-01-01T00:00:00.1234567Z", "2026-01-01T00:00:00.123456789Z", "2026-01-01T00:00:00.000000001Z"} {
		t.Run(raw, func(t *testing.T) {
			from, err := time.Parse(time.RFC3339Nano, raw)
			if err != nil {
				t.Fatal(err)
			}
			id := "SYN-CRED-TFA11-" + raw
			if outcome := channel.register(t, id, from, validTo); outcome != customsapp.ConfigurationRegistered {
				t.Fatalf("首登 = %s，want REGISTERED", outcome)
			}
			if outcome := channel.register(t, id, from, validTo); outcome != customsapp.ConfigurationExisting {
				t.Fatalf("原样重放 = %s，want EXISTING——写得进的就读得回", outcome)
			}
			if outcome := channel.register(t, id, from, validTo.Add(time.Nanosecond)); outcome != customsapp.ConfigurationExisting {
				t.Fatalf("终点只差微秒以下的重放 = %s，want EXISTING", outcome)
			}
			if outcome := channel.register(t, id, from.Add(time.Microsecond), validTo); outcome != customsapp.ConfigurationContentConflict {
				t.Fatalf("起点晚一微秒 = %s，want CONTENT_CONFLICT——登记册上那是另一份内容", outcome)
			}
		})
	}
}

func TestACredentialStartingWithinAMicrosecondOfTheZeroInstantIsNotAcceptedAndTheCatalogueStaysReadable(t *testing.T) {
	channel := newCredentialChannel(t)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if outcome := channel.register(t, "SYN-CRED-TFA11-NORMAL", from, from.AddDate(1, 0, 0)); outcome != customsapp.ConfigurationRegistered {
		t.Fatalf("正常凭证首登 = %s，want REGISTERED", outcome)
	}
	poisoned := time.Time{}.Add(100 * time.Nanosecond)
	if outcome := channel.register(t, "SYN-CRED-TFA11-ZERO", poisoned, from); outcome != customsapp.ConfigurationNotAccepted {
		t.Errorf("零时刻后一百纳秒的起点 = %s，want NOT_ACCEPTED——入册后读回成零时刻，重建不出来", outcome)
	}
	entries, err := channel.catalogue.ListCredentials(t.Context(), channel.tenant, 50)
	if err != nil {
		t.Fatalf("租户凭证列面：%v", err)
	}
	if len(entries) != 1 || entries[0].Credential.ID().String() != "SYN-CRED-TFA11-NORMAL" {
		t.Fatalf("列面 %d 行，want 只有正常那一张", len(entries))
	}
}

func TestCredentialValuesTheRegistryCannotStoreAreNotAccepted(t *testing.T) {
	channel := newCredentialChannel(t)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if outcome := channel.register(t, "SYN-CRED-TFA11-SAME-MICRO", from.Add(100*time.Nanosecond), from.Add(900*time.Nanosecond)); outcome != customsapp.ConfigurationNotAccepted {
		t.Fatalf("有效期两端落在同一微秒 = %s，want NOT_ACCEPTED", outcome)
	}
	var rows int
	if err := channel.fixture.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM customs_compliance.regulatory_credential WHERE tenant_id = $1`, channel.tenant.String()).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("册上 %d 行，want 零", rows)
	}
}
