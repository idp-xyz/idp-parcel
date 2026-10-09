package accessidentity_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

const (
	clientIssuer = "https://auth.syn.example/clients"
	clientTenant = "SYN-TENANT-01"
	clientSource = "SYN-SOURCE/bank-01"
)

var clientGrantStartsAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func clientSubjectOf(t *testing.T, sub string) accessidentity.IntegrationClientSubject {
	t.Helper()
	subject, err := accessidentity.NewIntegrationClientSubject(clientIssuer, sub)
	if err != nil {
		t.Fatalf("合格的发行方与 sub 应建得出客户端主体：%v", err)
	}
	return subject
}

func credentialReferenceOf(t *testing.T, value string) accessidentity.CredentialReference {
	t.Helper()
	reference, err := accessidentity.NewCredentialReference(value)
	if err != nil {
		t.Fatalf("凭据引用：%v", err)
	}
	return reference
}

func clientBindingOf(t *testing.T, subject accessidentity.IntegrationClientSubject, tenant string, certificateBound bool) accessidentity.IntegrationClientBinding {
	t.Helper()
	binding, err := accessidentity.NewIntegrationClientBinding(subject, tenant, clientSource,
		credentialReferenceOf(t, "SYN-CREDENTIAL-REF/bank-01"), certificateBound, "SYN-CLIENT-BASIS-01")
	if err != nil {
		t.Fatalf("完整绑定：%v", err)
	}
	return binding
}

func clientIntervalOf(t *testing.T, startsAt, endsAt time.Time) accessidentity.EffectiveInterval {
	t.Helper()
	interval, err := accessidentity.NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		t.Fatalf("区间：%v", err)
	}
	return interval
}

func clientGrantOf(
	t *testing.T,
	id string,
	subject accessidentity.IntegrationClientSubject,
	fact accessidentity.ExternalFactType,
	interval accessidentity.EffectiveInterval,
) accessidentity.IntegrationClientGrant {
	t.Helper()
	grant, err := accessidentity.NewIntegrationClientGrant(clientTenant, id, subject, fact, interval, "SYN-CLIENT-GRANT-BASIS-"+id)
	if err != nil {
		t.Fatalf("授予：%v", err)
	}
	return grant
}

func recordedClientGrantOf(t *testing.T, grant accessidentity.IntegrationClientGrant, revocation *accessidentity.GrantRevocation) accessidentity.RecordedClientGrant {
	t.Helper()
	recorded, err := accessidentity.NewRecordedClientGrant(grant, revocation)
	if err != nil {
		t.Fatalf("在册授予：%v", err)
	}
	return recorded
}

// Covers: ADR-0149 决定三「客户端标识绑定唯一租户与一个来源身份」与票面「凭据本体不入库，只存凭据引用」——主体两件
// 缺一不成立；绑定缺租户、来源身份、凭据引用或登记依据都不成立；首尾空白不进册。
func TestIntegrationClientRegistrationRefusesMissingParts(t *testing.T) {
	for name, parts := range map[string][2]string{
		"缺发行方":  {" ", "SYN-CLIENT-01"},
		"缺 sub": {clientIssuer, ""},
	} {
		if _, err := accessidentity.NewIntegrationClientSubject(parts[0], parts[1]); !errors.Is(err, accessidentity.ErrInvalidIntegrationClientSubject) {
			t.Fatalf("%s：err = %v, want ErrInvalidIntegrationClientSubject", name, err)
		}
	}
	padded, err := accessidentity.NewIntegrationClientSubject(" "+clientIssuer, "SYN-CLIENT-01 ")
	if err != nil || padded != clientSubjectOf(t, "SYN-CLIENT-01") {
		t.Fatalf("带首尾空白的主体 = %+v, %v; want 去掉空白后相同", padded, err)
	}

	subject := clientSubjectOf(t, "SYN-CLIENT-01")
	reference := credentialReferenceOf(t, "SYN-CREDENTIAL-REF/bank-01")
	for name, build := range map[string]func() error{
		"零值主体": func() error {
			_, err := accessidentity.NewIntegrationClientBinding(accessidentity.IntegrationClientSubject{}, clientTenant, clientSource, reference, false, "SYN-BASIS")
			return err
		},
		"缺租户": func() error {
			_, err := accessidentity.NewIntegrationClientBinding(subject, " ", clientSource, reference, false, "SYN-BASIS")
			return err
		},
		"缺来源身份": func() error {
			_, err := accessidentity.NewIntegrationClientBinding(subject, clientTenant, "", reference, false, "SYN-BASIS")
			return err
		},
		"缺凭据引用": func() error {
			_, err := accessidentity.NewIntegrationClientBinding(subject, clientTenant, clientSource, accessidentity.CredentialReference{}, false, "SYN-BASIS")
			return err
		},
		"缺登记依据": func() error {
			_, err := accessidentity.NewIntegrationClientBinding(subject, clientTenant, clientSource, reference, false, " ")
			return err
		},
	} {
		if err := build(); !errors.Is(err, accessidentity.ErrIncompleteIntegrationClientRegistration) {
			t.Fatalf("%s：err = %v, want ErrIncompleteIntegrationClientRegistration", name, err)
		}
	}

	binding, err := accessidentity.NewIntegrationClientBinding(subject, " "+clientTenant, clientSource+" ", reference, true, "SYN-BASIS")
	if err != nil {
		t.Fatalf("完整绑定：%v", err)
	}
	if binding.Subject() != subject || binding.TenantID() != clientTenant || binding.SourceIdentity() != clientSource ||
		binding.CredentialReference() != reference || !binding.CertificateBoundTokenRequired() || binding.Basis() != "SYN-BASIS" {
		t.Fatalf("绑定 = %+v", binding)
	}
}

// Covers: ADR-0149 决定三「授予按事实类型登记」——事实类型是封闭集、字面取值不折叠大小写；外部资金事实的更正口
// 不另立一格，随原事实（ADR-0151 决定四），所以集合里没有「更正」。
func TestExternalFactTypesAreAClosedSet(t *testing.T) {
	for _, literal := range []string{
		"CUSTOMS_EXTERNAL_RESULT", "REGULATORY_CREDENTIAL", "CARRIER_TRACKING",
		"CARRIER_FIRST_EFFECTIVE_PICKUP_EVIDENCE", "EXTERNAL_FUNDS_FACT",
	} {
		fact, err := accessidentity.ParseExternalFactType(literal)
		if err != nil || fact.String() != literal {
			t.Fatalf("ParseExternalFactType(%q) = %q, %v", literal, fact, err)
		}
	}
	for _, literal := range []string{"", "external_funds_fact", "EXTERNAL_FUNDS_FACT_CORRECTION", "ANYTHING"} {
		if _, err := accessidentity.ParseExternalFactType(literal); !errors.Is(err, accessidentity.ErrExternalFactTypeUnknown) {
			t.Fatalf("ParseExternalFactType(%q)：err = %v, want ErrExternalFactTypeUnknown", literal, err)
		}
	}

	subject := clientSubjectOf(t, "SYN-CLIENT-01")
	interval := clientIntervalOf(t, clientGrantStartsAt, time.Time{})
	if _, err := accessidentity.NewIntegrationClientGrant(clientTenant, "SYN-CLIENT-GRANT-01", subject,
		accessidentity.ExternalFactType("ANYTHING"), interval, "SYN-BASIS"); !errors.Is(err, accessidentity.ErrExternalFactTypeUnknown) {
		t.Fatalf("包外转型递进来的未知事实类型：err = %v, want ErrExternalFactTypeUnknown", err)
	}
	for name, build := range map[string]func() error{
		"缺租户": func() error {
			_, err := accessidentity.NewIntegrationClientGrant("", "SYN-CLIENT-GRANT-01", subject, accessidentity.FactExternalFunds, interval, "SYN-BASIS")
			return err
		},
		"缺授予标识": func() error {
			_, err := accessidentity.NewIntegrationClientGrant(clientTenant, " ", subject, accessidentity.FactExternalFunds, interval, "SYN-BASIS")
			return err
		},
		"零值主体": func() error {
			_, err := accessidentity.NewIntegrationClientGrant(clientTenant, "SYN-CLIENT-GRANT-01", accessidentity.IntegrationClientSubject{}, accessidentity.FactExternalFunds, interval, "SYN-BASIS")
			return err
		},
		"零值区间": func() error {
			_, err := accessidentity.NewIntegrationClientGrant(clientTenant, "SYN-CLIENT-GRANT-01", subject, accessidentity.FactExternalFunds, accessidentity.EffectiveInterval{}, "SYN-BASIS")
			return err
		},
		"缺依据": func() error {
			_, err := accessidentity.NewIntegrationClientGrant(clientTenant, "SYN-CLIENT-GRANT-01", subject, accessidentity.FactExternalFunds, interval, "")
			return err
		},
	} {
		if err := build(); !errors.Is(err, accessidentity.ErrIncompleteIntegrationClientRegistration) {
			t.Fatalf("授予%s：err = %v, want ErrIncompleteIntegrationClientRegistration", name, err)
		}
	}
}

// Covers: 票面「授予按事实类型登记、可撤销、带生效区间」——授予只在自己的区间内生效（含起点、不含终点），自撤销时刻起
// （含该时刻）不再生效；另一类事实不顶替。
func TestIntegrationClientHoldsAFactTypeOnlyInsideItsGrantAndUntilRevoked(t *testing.T) {
	subject := clientSubjectOf(t, "SYN-CLIENT-01")
	endsAt := clientGrantStartsAt.Add(90 * 24 * time.Hour)
	revokedAt := clientGrantStartsAt.Add(30 * 24 * time.Hour)
	funds := clientGrantOf(t, "SYN-CLIENT-GRANT-01", subject, accessidentity.FactExternalFunds, clientIntervalOf(t, clientGrantStartsAt, endsAt))
	results := clientGrantOf(t, "SYN-CLIENT-GRANT-02", subject, accessidentity.FactCustomsExternalResult, clientIntervalOf(t, clientGrantStartsAt, endsAt))
	revocation, err := accessidentity.NewGrantRevocation(clientTenant, "SYN-CLIENT-GRANT-02", revokedAt, "SYN-REVOKE-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	standing, err := accessidentity.NewIntegrationClientStanding(clientBindingOf(t, subject, clientTenant, false), []accessidentity.RecordedClientGrant{
		recordedClientGrantOf(t, funds, nil),
		recordedClientGrantOf(t, results, &revocation),
	})
	if err != nil {
		t.Fatalf("现状：%v", err)
	}

	for name, probe := range map[string]struct {
		fact accessidentity.ExternalFactType
		at   time.Time
		want bool
	}{
		"起点前一刻":     {accessidentity.FactExternalFunds, clientGrantStartsAt.Add(-time.Nanosecond), false},
		"起点":        {accessidentity.FactExternalFunds, clientGrantStartsAt, true},
		"终点前一刻":     {accessidentity.FactExternalFunds, endsAt.Add(-time.Nanosecond), true},
		"终点":        {accessidentity.FactExternalFunds, endsAt, false},
		"撤销时刻前一刻":   {accessidentity.FactCustomsExternalResult, revokedAt.Add(-time.Nanosecond), true},
		"撤销时刻":      {accessidentity.FactCustomsExternalResult, revokedAt, false},
		"未授予的那一类事实": {accessidentity.FactRegulatoryCredential, clientGrantStartsAt, false},
	} {
		if got := standing.HoldsAt(probe.fact, probe.at); got != probe.want {
			t.Fatalf("%s：HoldsAt = %v, want %v", name, got, probe.want)
		}
	}
}

// Covers: 现状只装这个客户端主体、这个租户名下的授予；撤销只挂得到它撤的那一笔上。
func TestIntegrationClientStandingRefusesForeignGrantsAndMismatchedRevocations(t *testing.T) {
	subject := clientSubjectOf(t, "SYN-CLIENT-01")
	interval := clientIntervalOf(t, clientGrantStartsAt, time.Time{})
	binding := clientBindingOf(t, subject, clientTenant, false)

	foreign := clientGrantOf(t, "SYN-CLIENT-GRANT-09", clientSubjectOf(t, "SYN-CLIENT-02"), accessidentity.FactExternalFunds, interval)
	if _, err := accessidentity.NewIntegrationClientStanding(binding, []accessidentity.RecordedClientGrant{recordedClientGrantOf(t, foreign, nil)}); !errors.Is(err, accessidentity.ErrIntegrationClientGrantOutsideBinding) {
		t.Fatalf("别的客户端的授予：err = %v, want ErrIntegrationClientGrantOutsideBinding", err)
	}
	otherTenant, err := accessidentity.NewIntegrationClientGrant("SYN-TENANT-02", "SYN-CLIENT-GRANT-09", subject, accessidentity.FactExternalFunds, interval, "SYN-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := accessidentity.NewIntegrationClientStanding(binding, []accessidentity.RecordedClientGrant{recordedClientGrantOf(t, otherTenant, nil)}); !errors.Is(err, accessidentity.ErrIntegrationClientGrantOutsideBinding) {
		t.Fatalf("别的租户名下的授予：err = %v, want ErrIntegrationClientGrantOutsideBinding", err)
	}

	grant := clientGrantOf(t, "SYN-CLIENT-GRANT-01", subject, accessidentity.FactExternalFunds, interval)
	misplaced, err := accessidentity.NewGrantRevocation(clientTenant, "SYN-CLIENT-GRANT-02", clientGrantStartsAt.Add(time.Hour), "SYN-REVOKE-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := accessidentity.NewRecordedClientGrant(grant, &misplaced); !errors.Is(err, accessidentity.ErrRevocationDoesNotMatchGrant) {
		t.Fatalf("挂错的撤销：err = %v, want ErrRevocationDoesNotMatchGrant", err)
	}
}
