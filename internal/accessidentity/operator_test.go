package accessidentity

import (
	"errors"
	"testing"
	"time"
)

const (
	testIssuer = "https://syn-issuer-01.example.invalid"
	testSub    = "SYN-OPERATOR-01"
)

func mustSubject(t *testing.T) OperatorSubject {
	t.Helper()
	subject, err := NewOperatorSubject(testIssuer, testSub)
	if err != nil {
		t.Fatalf("合格的发行方与 sub 应建得出主体：%v", err)
	}
	return subject
}

// Covers: ADR-0100 决定二第三条「操作者主体（发行方 + sub）绑定唯一租户」——主体两件缺一不成立，
// 绑定缺租户或登记依据不成立；首尾空白不进册，免得同一主体因抄写多一个空格成了两行。
func TestOperatorSubjectAndBindingRefuseMissingParts(t *testing.T) {
	for name, parts := range map[string][2]string{
		"缺发行方":  {"  ", testSub},
		"缺 sub": {testIssuer, ""},
	} {
		if _, err := NewOperatorSubject(parts[0], parts[1]); !errors.Is(err, ErrInvalidOperatorSubject) {
			t.Fatalf("%s：err = %v, want ErrInvalidOperatorSubject", name, err)
		}
	}

	padded, err := NewOperatorSubject(" "+testIssuer+" ", " "+testSub)
	if err != nil {
		t.Fatalf("带首尾空白的主体：%v", err)
	}
	if padded != mustSubject(t) {
		t.Fatalf("主体 = %+v, want 去掉首尾空白后与 %q / %q 相同", padded, testIssuer, testSub)
	}

	subject := mustSubject(t)
	for name, build := range map[string]func() error{
		"零值主体": func() error {
			_, err := NewOperatorBinding(OperatorSubject{}, "SYN-TENANT-01", "SYN-BASIS-01")
			return err
		},
		"缺租户":   func() error { _, err := NewOperatorBinding(subject, " ", "SYN-BASIS-01"); return err },
		"缺登记依据": func() error { _, err := NewOperatorBinding(subject, "SYN-TENANT-01", ""); return err },
	} {
		if err := build(); !errors.Is(err, ErrIncompleteOperatorRegistration) {
			t.Fatalf("%s：err = %v, want ErrIncompleteOperatorRegistration", name, err)
		}
	}

	binding, err := NewOperatorBinding(subject, "SYN-TENANT-01", "SYN-BASIS-01")
	if err != nil {
		t.Fatalf("完整绑定：%v", err)
	}
	if binding.Subject() != subject || binding.TenantID() != "SYN-TENANT-01" || binding.Basis() != "SYN-BASIS-01" {
		t.Fatalf("绑定 = %+v", binding)
	}
}

var grantStartsAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// Covers: 授予「带生效区间」——含起点、不含终点，终点缺席即不设终点；起点不代填，终点不晚于
// 起点的区间建不成（空区间与倒置区间读起来都像一笔授予，却在任何时点都不生效）。
func TestEffectiveIntervalIsHalfOpenAndRefusesIncoherentBounds(t *testing.T) {
	endsAt := grantStartsAt.Add(30 * 24 * time.Hour)
	bounded := mustInterval(t, grantStartsAt, endsAt)
	for at, want := range map[time.Time]bool{
		grantStartsAt.Add(-time.Nanosecond): false,
		grantStartsAt:                       true,
		endsAt.Add(-time.Nanosecond):        true,
		endsAt:                              false,
	} {
		if got := bounded.Contains(at); got != want {
			t.Fatalf("有界区间在 %s：Contains = %v, want %v", at, got, want)
		}
	}
	if gotEnd, bounded := bounded.EndsAt(); !bounded || !gotEnd.Equal(endsAt) {
		t.Fatalf("EndsAt = %s, %v; want %s, true", gotEnd, bounded, endsAt)
	}

	open := mustInterval(t, grantStartsAt, time.Time{})
	if !open.Contains(grantStartsAt.Add(100 * 365 * 24 * time.Hour)) {
		t.Fatal("不设终点的区间应对起点之后任意时点生效")
	}
	if _, bounded := open.EndsAt(); bounded {
		t.Fatal("不设终点的区间 EndsAt 应答无终点")
	}

	for name, bounds := range map[string][2]time.Time{
		"缺起点":    {{}, endsAt},
		"终点等于起点": {grantStartsAt, grantStartsAt},
		"终点早于起点": {grantStartsAt, grantStartsAt.Add(-time.Hour)},
	} {
		if _, err := NewEffectiveInterval(bounds[0], bounds[1]); !errors.Is(err, ErrInvalidEffectiveInterval) {
			t.Fatalf("%s：err = %v, want ErrInvalidEffectiveInterval", name, err)
		}
	}
	if _, err := NewOperatorGrant("SYN-TENANT-01", "SYN-GRANT-01", mustSubject(t),
		CapabilityRegistryConfigurationWrite, EffectiveInterval{}, "SYN-GRANT-BASIS-01"); !errors.Is(err, ErrIncompleteOperatorRegistration) {
		t.Fatalf("零值区间的授予：err = %v, want ErrIncompleteOperatorRegistration", err)
	}
}

func mustInterval(t *testing.T, startsAt, endsAt time.Time) EffectiveInterval {
	t.Helper()
	interval, err := NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		t.Fatalf("合格区间应建得出：%v", err)
	}
	return interval
}

func mustGrant(t *testing.T, tenant, id string, face CapabilityFace, interval EffectiveInterval) OperatorGrant {
	t.Helper()
	grant, err := NewOperatorGrant(tenant, id, mustSubject(t), face, interval, "SYN-GRANT-BASIS-"+id)
	if err != nil {
		t.Fatalf("合格授予 %s：%v", id, err)
	}
	return grant
}

// Covers: 授予「可撤销」——撤销自撤销时刻起生效（含该时刻），之前照常；撤销早于起点的授予从未
// 生效；撤销缺时刻或依据建不成（只记时刻答不出凭什么撤，只记依据答不出从哪一刻起不能用）；
// 一笔撤销只能挂在它所撤的那一笔授予上。
func TestRevocationEndsAGrantFromTheRevocationInstant(t *testing.T) {
	grant := mustGrant(t, "SYN-TENANT-01", "SYN-GRANT-01", CapabilityRegistryConfigurationWrite,
		mustInterval(t, grantStartsAt, time.Time{}))
	revokedAt := grantStartsAt.Add(10 * 24 * time.Hour)
	revocation, err := NewGrantRevocation("SYN-TENANT-01", "SYN-GRANT-01", revokedAt, "SYN-REVOKE-BASIS-01")
	if err != nil {
		t.Fatalf("合格撤销：%v", err)
	}
	recorded, err := NewRecordedGrant(grant, &revocation)
	if err != nil {
		t.Fatalf("挂撤销：%v", err)
	}
	for at, want := range map[time.Time]bool{
		grantStartsAt:                   true,
		revokedAt.Add(-time.Nanosecond): true,
		revokedAt:                       false,
		revokedAt.Add(time.Hour):        false,
	} {
		if got := recorded.EffectiveAt(at); got != want {
			t.Fatalf("在 %s：EffectiveAt = %v, want %v", at, got, want)
		}
	}
	if got, revoked := recorded.Revocation(); !revoked || got != revocation {
		t.Fatalf("Revocation = %+v, %v", got, revoked)
	}

	unrevoked, err := NewRecordedGrant(grant, nil)
	if err != nil {
		t.Fatalf("未撤销的授予：%v", err)
	}
	if !unrevoked.EffectiveAt(revokedAt.Add(time.Hour)) {
		t.Fatal("未撤销的授予在区间内应生效")
	}

	early, err := NewGrantRevocation("SYN-TENANT-01", "SYN-GRANT-01", grantStartsAt.Add(-time.Hour), "SYN-REVOKE-BASIS-02")
	if err != nil {
		t.Fatalf("早于起点的撤销：%v", err)
	}
	cancelled, err := NewRecordedGrant(grant, &early)
	if err != nil {
		t.Fatalf("挂早于起点的撤销：%v", err)
	}
	if cancelled.EffectiveAt(grantStartsAt) || cancelled.EffectiveAt(grantStartsAt.Add(24*time.Hour)) {
		t.Fatal("起点前就撤掉的授予不应在任何时点生效")
	}

	for name, build := range map[string]func() error{
		"缺时刻": func() error {
			_, err := NewGrantRevocation("SYN-TENANT-01", "SYN-GRANT-01", time.Time{}, "SYN-REVOKE-BASIS-01")
			return err
		},
		"缺依据": func() error {
			_, err := NewGrantRevocation("SYN-TENANT-01", "SYN-GRANT-01", revokedAt, " ")
			return err
		},
		"缺租户": func() error {
			_, err := NewGrantRevocation("", "SYN-GRANT-01", revokedAt, "SYN-REVOKE-BASIS-01")
			return err
		},
		"缺授予": func() error {
			_, err := NewGrantRevocation("SYN-TENANT-01", "", revokedAt, "SYN-REVOKE-BASIS-01")
			return err
		},
	} {
		if err := build(); !errors.Is(err, ErrIncompleteOperatorRegistration) {
			t.Fatalf("%s：err = %v, want ErrIncompleteOperatorRegistration", name, err)
		}
	}

	for name, other := range map[string][2]string{
		"别的授予": {"SYN-TENANT-01", "SYN-GRANT-02"},
		"别的租户": {"SYN-TENANT-02", "SYN-GRANT-01"},
	} {
		foreign, err := NewGrantRevocation(other[0], other[1], revokedAt, "SYN-REVOKE-BASIS-01")
		if err != nil {
			t.Fatalf("%s 的撤销：%v", name, err)
		}
		if _, err := NewRecordedGrant(grant, &foreign); !errors.Is(err, ErrRevocationDoesNotMatchGrant) {
			t.Fatalf("挂%s的撤销：err = %v, want ErrRevocationDoesNotMatchGrant", name, err)
		}
	}
}

// Covers: 册上现状按能力面对时点判——同一格两笔授予之间的空档不生效，另一格的授予不顶替；
// 现状只收绑定那个主体、那个租户名下的授予，混进别人的授予建不成。
func TestOperatorStandingHoldsAFaceOnlyWhileSomeGrantOfThatFaceIsEffective(t *testing.T) {
	binding, err := NewOperatorBinding(mustSubject(t), "SYN-TENANT-01", "SYN-BASIS-01")
	if err != nil {
		t.Fatalf("绑定：%v", err)
	}
	firstEnds := grantStartsAt.Add(10 * 24 * time.Hour)
	secondStarts := grantStartsAt.Add(20 * 24 * time.Hour)
	first, _ := NewRecordedGrant(mustGrant(t, "SYN-TENANT-01", "SYN-GRANT-01",
		CapabilityRegistryConfigurationWrite, mustInterval(t, grantStartsAt, firstEnds)), nil)
	second, _ := NewRecordedGrant(mustGrant(t, "SYN-TENANT-01", "SYN-GRANT-02",
		CapabilityRegistryConfigurationWrite, mustInterval(t, secondStarts, time.Time{})), nil)

	standing, err := NewOperatorStanding(binding, []RecordedGrant{first, second})
	if err != nil {
		t.Fatalf("现状：%v", err)
	}
	for name, probe := range map[string]struct {
		face CapabilityFace
		at   time.Time
		want bool
	}{
		"第一笔区间内":  {CapabilityRegistryConfigurationWrite, grantStartsAt, true},
		"两笔之间的空档": {CapabilityRegistryConfigurationWrite, firstEnds, false},
		"第二笔区间内":  {CapabilityRegistryConfigurationWrite, secondStarts, true},
		"起点之前":    {CapabilityRegistryConfigurationWrite, grantStartsAt.Add(-time.Hour), false},
		"未授予的那一格": {CapabilityMasterDataAndOperationsRead, grantStartsAt, false},
	} {
		if got := standing.HoldsAt(probe.face, probe.at); got != probe.want {
			t.Fatalf("%s：HoldsAt = %v, want %v", name, got, probe.want)
		}
	}
	if standing.Binding() != binding || len(standing.Grants()) != 2 {
		t.Fatalf("现状 = %+v", standing)
	}

	otherSubject, err := NewOperatorSubject(testIssuer, "SYN-OPERATOR-02")
	if err != nil {
		t.Fatalf("另一主体：%v", err)
	}
	strangersGrant, err := NewOperatorGrant("SYN-TENANT-01", "SYN-GRANT-09", otherSubject,
		CapabilityRegistryConfigurationWrite, mustInterval(t, grantStartsAt, time.Time{}), "SYN-GRANT-BASIS-09")
	if err != nil {
		t.Fatalf("另一主体的授予：%v", err)
	}
	strangers, _ := NewRecordedGrant(strangersGrant, nil)
	otherTenants, _ := NewRecordedGrant(mustGrant(t, "SYN-TENANT-02", "SYN-GRANT-01",
		CapabilityRegistryConfigurationWrite, mustInterval(t, grantStartsAt, time.Time{})), nil)
	for name, foreign := range map[string]RecordedGrant{"别的主体": strangers, "别的租户": otherTenants} {
		if _, err := NewOperatorStanding(binding, []RecordedGrant{first, foreign}); !errors.Is(err, ErrGrantOutsideBinding) {
			t.Fatalf("混进%s的授予：err = %v, want ErrGrantOutsideBinding", name, err)
		}
	}
}

// Covers: ADR-0100 决定二第三条的能力面三格——登记册配置写与主数据与运营查阅读可授予；治理登记
// 那一格只预留（授予模型按 ADR-0085 决定四另裁），答「预留」而不是「不认识」，且绕过解析、
// 直接转型递进来也授不出去。
func TestOnlyTheTwoDecidedCapabilityFacesAreGrantable(t *testing.T) {
	for raw, want := range map[string]CapabilityFace{
		"REGISTRY_CONFIGURATION_WRITE":    CapabilityRegistryConfigurationWrite,
		"MASTER_DATA_AND_OPERATIONS_READ": CapabilityMasterDataAndOperationsRead,
	} {
		face, err := ParseCapabilityFace(raw)
		if err != nil || face != want {
			t.Fatalf("解析 %q = %q, %v; want %q", raw, face, err, want)
		}
	}
	if _, err := ParseCapabilityFace("GOVERNANCE_REGISTRATION"); !errors.Is(err, ErrCapabilityFaceReserved) {
		t.Fatalf("治理登记：err = %v, want ErrCapabilityFaceReserved", err)
	}
	for _, raw := range []string{"", "registry_configuration_write", "OPERATION_FACT_REGISTRATION"} {
		if _, err := ParseCapabilityFace(raw); !errors.Is(err, ErrCapabilityFaceUnknown) {
			t.Fatalf("%q：err = %v, want ErrCapabilityFaceUnknown", raw, err)
		}
	}

	interval := mustInterval(t, grantStartsAt, time.Time{})
	for name, face := range map[string]CapabilityFace{
		"强转的治理登记": CapabilityFace("GOVERNANCE_REGISTRATION"),
		"强转的未知面":  CapabilityFace("ANYTHING"),
	} {
		if _, err := NewOperatorGrant("SYN-TENANT-01", "SYN-GRANT-01", mustSubject(t), face, interval, "SYN-GRANT-BASIS-01"); err == nil {
			t.Fatalf("%s：授予应建不成", name)
		}
	}
	grant, err := NewOperatorGrant("SYN-TENANT-01", "SYN-GRANT-01", mustSubject(t),
		CapabilityRegistryConfigurationWrite, interval, "SYN-GRANT-BASIS-01")
	if err != nil {
		t.Fatalf("合格授予：%v", err)
	}
	if grant.Face() != CapabilityRegistryConfigurationWrite || grant.GrantID() != "SYN-GRANT-01" ||
		grant.TenantID() != "SYN-TENANT-01" || grant.Subject() != mustSubject(t) || grant.Basis() != "SYN-GRANT-BASIS-01" {
		t.Fatalf("授予 = %+v", grant)
	}
}
