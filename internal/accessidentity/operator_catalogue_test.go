package accessidentity

import (
	"errors"
	"slices"
	"testing"
	"time"
)

var catalogueAt = time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)

func catalogueSubject(t *testing.T, sub string) OperatorSubject {
	t.Helper()
	subject, err := NewOperatorSubject("https://id.syn.example/dex", sub)
	if err != nil {
		t.Fatal(err)
	}
	return subject
}

func catalogueRecorded(t *testing.T, subject OperatorSubject, face CapabilityFace, startsAt, endsAt time.Time, revokedAt *time.Time) RecordedGrant {
	t.Helper()
	interval, err := NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := NewOperatorGrant("SYN-TENANT-01", "SYN-GRANT-01", subject, face, interval, "SYN-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	var revocation *GrantRevocation
	if revokedAt != nil {
		built, err := NewGrantRevocation("SYN-TENANT-01", "SYN-GRANT-01", *revokedAt, "SYN-REVOKE")
		if err != nil {
			t.Fatal(err)
		}
		revocation = &built
	}
	recorded, err := NewRecordedGrant(grant, revocation)
	if err != nil {
		t.Fatal(err)
	}
	return recorded
}

// Covers: ADR-0363 决定一——操作者接入管理可授、HoldsAt 认它；可授能力面的词表不含预留的治理登记；决定种类与作业范围种类的词表与
// 解析门同一份。
func TestOperatorAccessAdministrationIsGrantableAndListed(t *testing.T) {
	if _, err := ParseCapabilityFace("OPERATOR_ACCESS_ADMINISTRATION"); err != nil {
		t.Fatalf("解析：%v", err)
	}
	subject := catalogueSubject(t, "SYN-OPERATOR-01")
	binding, err := NewOperatorBinding(subject, "SYN-TENANT-01", "SYN-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	standing, err := NewOperatorStanding(binding, []RecordedGrant{catalogueRecorded(t, subject, CapabilityOperatorAccessAdministration, catalogueAt.Add(-time.Hour), time.Time{}, nil)})
	if err != nil {
		t.Fatal(err)
	}
	if !standing.HoldsAt(CapabilityOperatorAccessAdministration, catalogueAt) || standing.HoldsAt(CapabilityRegistryConfigurationWrite, catalogueAt) {
		t.Fatal("HoldsAt 没按格认操作者接入管理")
	}
	faces := GrantableCapabilityFaces()
	if slices.Contains(faces, CapabilityFace(reservedGovernanceRegistration)) || !slices.Contains(faces, CapabilityOperatorAccessAdministration) {
		t.Fatalf("可授能力面 = %v", faces)
	}
	for _, face := range faces {
		if _, err := ParseCapabilityFace(face.String()); err != nil {
			t.Fatalf("%s 列为可授却解析不过：%v", face, err)
		}
	}
	kinds := DecisionKinds()
	if len(kinds) != len(decisionKinds) {
		t.Fatalf("决定种类 = %d, want %d", len(kinds), len(decisionKinds))
	}
	kinds[0] = "TAMPERED"
	if decisionKinds[0] == "TAMPERED" {
		t.Fatal("DecisionKinds 交出的不是拷贝")
	}
	for _, kind := range WorkScopeKinds() {
		if _, err := ParseWorkScopeKind(kind.String()); err != nil {
			t.Fatalf("%s：%v", kind, err)
		}
	}
	if OperatorRevocationWouldLockOutCaller.String() != "REVOCATION_WOULD_LOCK_OUT_CALLER" {
		t.Fatal(OperatorRevocationWouldLockOutCaller)
	}
}

// Covers: ADR-0363 决定三——只有调用者自己此刻生效的操作者接入管理授予才锁得住他：别人的、别的格的、未到的、已到期的、已撤销的都不。
func TestLocksOutCallerIfRevokedOnlyForTheCallersEffectiveAdministrationGrant(t *testing.T) {
	self, other := catalogueSubject(t, "SYN-OPERATOR-01"), catalogueSubject(t, "SYN-OPERATOR-02")
	past, future, earlier := catalogueAt.Add(-time.Hour), catalogueAt.Add(time.Hour), catalogueAt.Add(-time.Minute)
	cases := []struct {
		name  string
		grant RecordedGrant
		locks bool
	}{
		{"own effective", catalogueRecorded(t, self, CapabilityOperatorAccessAdministration, past, time.Time{}, nil), true},
		{"own bounded still effective", catalogueRecorded(t, self, CapabilityOperatorAccessAdministration, past, future, nil), true},
		{"someone else's", catalogueRecorded(t, other, CapabilityOperatorAccessAdministration, past, time.Time{}, nil), false},
		{"another face", catalogueRecorded(t, self, CapabilityRegistryConfigurationWrite, past, time.Time{}, nil), false},
		{"not yet effective", catalogueRecorded(t, self, CapabilityOperatorAccessAdministration, future, time.Time{}, nil), false},
		{"expired", catalogueRecorded(t, self, CapabilityOperatorAccessAdministration, past, earlier, nil), false},
		{"already revoked", catalogueRecorded(t, self, CapabilityOperatorAccessAdministration, past, time.Time{}, &earlier), false},
	}
	for _, testCase := range cases {
		if got := testCase.grant.LocksOutCallerIfRevoked(self, catalogueAt); got != testCase.locks {
			t.Fatalf("%s：%v, want %v", testCase.name, got, testCase.locks)
		}
	}
}

// Covers: ADR-0363 决定二——查阅行要登记时刻，混进别的主体或别的租户的授予即拒；交出的授予是拷贝。
func TestOperatorCatalogueRowGuardsItsShape(t *testing.T) {
	self, other := catalogueSubject(t, "SYN-OPERATOR-01"), catalogueSubject(t, "SYN-OPERATOR-02")
	binding, err := NewOperatorBinding(self, "SYN-TENANT-01", "SYN-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	own := catalogueRecorded(t, self, CapabilityOperatorAccessAdministration, catalogueAt.Add(-time.Hour), time.Time{}, nil)
	if _, err := NewRecordedGrantRow(own, time.Time{}); !errors.Is(err, ErrIncompleteOperatorCatalogueRow) {
		t.Fatalf("零值登记时刻 err = %v", err)
	}
	ownRow, err := NewRecordedGrantRow(own, catalogueAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewOperatorCatalogueRow(binding, time.Time{}, nil); !errors.Is(err, ErrIncompleteOperatorCatalogueRow) {
		t.Fatalf("零值登记时刻 err = %v", err)
	}
	foreignRow, err := NewRecordedGrantRow(catalogueRecorded(t, other, CapabilityRegistryConfigurationWrite, catalogueAt.Add(-time.Hour), time.Time{}, nil), catalogueAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewOperatorCatalogueRow(binding, catalogueAt, []RecordedGrantRow{foreignRow}); !errors.Is(err, ErrGrantOutsideBinding) {
		t.Fatalf("别的主体的授予 err = %v", err)
	}
	row, err := NewOperatorCatalogueRow(binding, catalogueAt, []RecordedGrantRow{ownRow})
	if err != nil {
		t.Fatal(err)
	}
	grants := row.Grants()
	grants[0] = RecordedGrantRow{}
	if row.Grants()[0].RecordedAt() != catalogueAt || row.RecordedAt() != catalogueAt || row.Binding() != binding {
		t.Fatal("Grants 交出的不是拷贝，或行上的格丢了")
	}
}

// Covers: GrantableCapabilityFaces 与 checkGrantable 是同一个集合——列表里的每一格都解析得过，解析得过的每一格都在列表里
// （含预留格不在、陌生码不在）。两处各写一份，少一格就在这里红。
func TestGrantableCapabilityFacesMatchTheGrantableGate(t *testing.T) {
	listed := map[CapabilityFace]bool{}
	for _, face := range GrantableCapabilityFaces() {
		if _, err := ParseCapabilityFace(face.String()); err != nil {
			t.Fatalf("列表里的 %s 解析不过：%v", face, err)
		}
		listed[face] = true
	}
	for _, candidate := range []string{"REGISTRY_CONFIGURATION_WRITE", "MASTER_DATA_AND_OPERATIONS_READ", "OPERATION_DECISION", "WORK_FACT_REGISTRATION", "OPERATOR_ACCESS_ADMINISTRATION", reservedGovernanceRegistration, "SOMETHING_ELSE"} {
		face, err := ParseCapabilityFace(candidate)
		if (err == nil) != listed[face] {
			t.Fatalf("%s：解析 err=%v，列表里=%v", candidate, err, listed[face])
		}
	}
}
