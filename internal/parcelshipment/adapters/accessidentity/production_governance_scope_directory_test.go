package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	psaccess "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/accessidentity"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

type mappingReaderDouble struct {
	rows []identity.RecordedAdmissionScopeMapping
	err  error
	// asked 记下收到的键：目录必须按委托提交那一对词问，问错一格查到的就是别的行。
	asked  identity.AdmissionRequirement
	tenant string
}

func (reader *mappingReaderDouble) FindAdmissionScopeMappings(
	_ context.Context, tenantID string, requirement identity.AdmissionRequirement,
) ([]identity.RecordedAdmissionScopeMapping, error) {
	reader.tenant, reader.asked = tenantID, requirement
	return reader.rows, reader.err
}

type clockDouble struct{ at time.Time }

func (clock clockDouble) Now() time.Time { return clock.at }

var directoryNow = time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)

func recordedMapping(t *testing.T, id, objectScope string, from, to time.Time) identity.RecordedAdmissionScopeMapping {
	t.Helper()
	interval, err := identity.NewEffectiveInterval(from, to)
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := identity.NewAdmissionScopeMapping("SYN-TENANT-01", id, identity.ShipmentRequestSubmissionAdmission(), objectScope, interval, "SYN-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := identity.NewRecordedAdmissionScopeMapping(mapping, nil)
	if err != nil {
		t.Fatal(err)
	}
	return recorded
}

func accountScope(t *testing.T) psdomain.AdmissionScope {
	t.Helper()
	tenant, err := psdomain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	account, err := psdomain.NewCustomerAccountID("SYN-ACCOUNT-01")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := psdomain.NewCustomerAccountAdmissionScope(tenant, account)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func directoryOver(t *testing.T, reader *mappingReaderDouble) *psaccess.ProductionGovernanceScopeDirectory {
	t.Helper()
	directory, err := psaccess.NewProductionGovernanceScopeDirectory(reader, clockDouble{at: directoryNow})
	if err != nil {
		t.Fatal(err)
	}
	return directory
}

// Covers: AT-PS-117（ADR-0308 决定二、三）——生产目录读准入范围对照册：查到交回对象范围、能力与事实类型（委托提交那一对词），
// 试点范围版本取对象范围本身；按拟受理范围认回的租户问，不另解析引用串。
func TestTheProductionDirectoryResolvesTheMappedObjectScope(t *testing.T) {
	reader := &mappingReaderDouble{rows: []identity.RecordedAdmissionScopeMapping{
		recordedMapping(t, "SYN-MAP-1", "SYN-PILOT-SCOPE/shipment-intake@v1", directoryNow.Add(-time.Hour), time.Time{}),
	}}
	governance, found, err := directoryOver(t, reader).FindGovernanceScope(t.Context(), accountScope(t))
	if err != nil || !found {
		t.Fatalf("found = %v, err = %v, want 查到", found, err)
	}
	if governance.ObjectScope != "SYN-PILOT-SCOPE/shipment-intake@v1" ||
		governance.Capability != "CUSTOMER_COMMAND" || governance.FactKind != "SHIPMENT_REQUEST_SUBMISSION" ||
		governance.PilotScope.String() != "SYN-PILOT-SCOPE/shipment-intake@v1" {
		t.Fatalf("坐标 = %+v", governance)
	}
	if reader.tenant != "SYN-TENANT-01" || reader.asked != identity.ShipmentRequestSubmissionAdmission() {
		t.Fatalf("问的键 = %q %+v", reader.tenant, reader.asked)
	}
}

// Covers: AT-PS-117——查不到如实答缺席、不代拟：册上没有行、行还没到生效时刻、行已经结束、拟受理范围认不出租户，都是
// （false, nil），编排据此照旧答`权威未确定`。
func TestTheProductionDirectoryAnswersAbsenceWithoutInventingACoordinate(t *testing.T) {
	ended := func() identity.RecordedAdmissionScopeMapping {
		mapping := recordedMapping(t, "SYN-MAP-2", "SYN-PILOT-SCOPE/shipment-intake@v1", directoryNow.Add(-2*time.Hour), time.Time{})
		end, err := identity.NewAdmissionScopeMappingEnd("SYN-TENANT-01", "SYN-MAP-2", directoryNow.Add(-time.Hour), "SYN-END-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		recorded, err := identity.NewRecordedAdmissionScopeMapping(mapping.Mapping(), &end)
		if err != nil {
			t.Fatal(err)
		}
		return recorded
	}
	for name, testCase := range map[string]struct {
		rows  []identity.RecordedAdmissionScopeMapping
		scope psdomain.AdmissionScope
	}{
		"no row":             {nil, accountScope(t)},
		"not yet effective":  {[]identity.RecordedAdmissionScopeMapping{recordedMapping(t, "SYN-MAP-3", "SYN-PILOT-SCOPE/shipment-intake@v1", directoryNow.Add(time.Hour), time.Time{})}, accountScope(t)},
		"ended":              {[]identity.RecordedAdmissionScopeMapping{ended()}, accountScope(t)},
		"unrecognised scope": {[]identity.RecordedAdmissionScopeMapping{recordedMapping(t, "SYN-MAP-4", "SYN-PILOT-SCOPE/shipment-intake@v1", directoryNow.Add(-time.Hour), time.Time{})}, psdomain.AdmissionScope{}},
	} {
		t.Run(name, func(t *testing.T) {
			_, found, err := directoryOver(t, &mappingReaderDouble{rows: testCase.rows}).FindGovernanceScope(t.Context(), testCase.scope)
			if err != nil || found {
				t.Fatalf("found = %v, err = %v, want 缺席且无错误", found, err)
			}
		})
	}
}

// Covers: AT-PS-117——读不动答错误，不折成缺席；同键两行同时生效（绕过登记口的直插）也答错误，不任选一行。
func TestTheProductionDirectoryDoesNotTurnAFailureIntoAbsence(t *testing.T) {
	readFailure := errors.New("mapping register unreachable")
	_, found, err := directoryOver(t, &mappingReaderDouble{err: readFailure}).FindGovernanceScope(t.Context(), accountScope(t))
	if !errors.Is(err, readFailure) || found {
		t.Fatalf("found = %v, err = %v, want 读不动的错误", found, err)
	}

	_, found, err = directoryOver(t, &mappingReaderDouble{rows: []identity.RecordedAdmissionScopeMapping{
		recordedMapping(t, "SYN-MAP-5", "SYN-PILOT-SCOPE/a@v1", directoryNow.Add(-time.Hour), time.Time{}),
		recordedMapping(t, "SYN-MAP-6", "SYN-PILOT-SCOPE/b@v1", directoryNow.Add(-time.Hour), time.Time{}),
	}}).FindGovernanceScope(t.Context(), accountScope(t))
	if !errors.Is(err, identity.ErrConflictingAdmissionScopeMappings) || found {
		t.Fatalf("found = %v, err = %v, want 重叠对照的错误", found, err)
	}
}

func TestTheProductionDirectoryNeedsItsInputs(t *testing.T) {
	if _, err := psaccess.NewProductionGovernanceScopeDirectory(nil, clockDouble{}); err == nil {
		t.Fatal("缺对照读口应拒")
	}
	if _, err := psaccess.NewProductionGovernanceScopeDirectory(&mappingReaderDouble{}, nil); err == nil {
		t.Fatal("缺时钟应拒")
	}
}
