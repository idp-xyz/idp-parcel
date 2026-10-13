package pilotgovernance_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipilot "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/pilotgovernance"
	pgapplication "go.idp.xyz/idp-parcel/internal/pilotgovernance/application"
	pgdomain "go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
)

// 本文件证准入范围桥（票 operator-channel/19 完成判据第一条）：对照已登且区间覆盖才放行；对照未登、或登了而区间不覆盖
// （别的权威、已关、未始、别的对象范围）答不在准入范围；对照读不动、区间读不动答依赖故障格，经铸造器出来不被「不在
// 准入范围」认出。区间那一半用真的 AuthorityCoverage，只把登记册换成内存替身。

const (
	tenant        = "SYN-TENANT-01"
	selfAuthority = "SYN-AUTH/idp-parcel-pilot"
	decisionScope = "SYN-PILOT-SCOPE/operation-decisions@v1"
)

var (
	day       = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now       = day.Add(10 * 24 * time.Hour)
	loadScope = accessidentity.AdmissionRequirement{
		Capability: accessidentity.CapabilityOperationDecision.String(),
		FactKind:   accessidentity.DecisionLoadAssignment.String(),
	}
)

// mappingBook 是对照读口的替身：按键交回登过的行，err 非空时演读不动。
type mappingBook struct {
	rows []accessidentity.RecordedAdmissionScopeMapping
	err  error
}

func (book mappingBook) FindAdmissionScopeMappings(_ context.Context, tenantID string, requirement accessidentity.AdmissionRequirement) ([]accessidentity.RecordedAdmissionScopeMapping, error) {
	if book.err != nil {
		return nil, book.err
	}
	var found []accessidentity.RecordedAdmissionScopeMapping
	for _, row := range book.rows {
		if row.Mapping().TenantID() == tenantID && row.Mapping().Requirement() == requirement {
			found = append(found, row)
		}
	}
	return found, nil
}

// intervalBook 是治理生产权威区间的替身：交回登过的区间，err 非空时演读不动。
type intervalBook struct {
	intervals []pgdomain.AuthorityInterval
	err       error
	reads     int
}

func (book *intervalBook) ListCurrent(context.Context) ([]pgdomain.AuthorityInterval, error) {
	book.reads++
	return book.intervals, book.err
}

func (book *intervalBook) Append(context.Context, pgdomain.AuthorityInterval) error {
	return errors.New("read-only interval book")
}

func row(t *testing.T, id, scope string, startsAt, endedAt time.Time) accessidentity.RecordedAdmissionScopeMapping {
	t.Helper()
	interval, err := accessidentity.NewEffectiveInterval(startsAt, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := accessidentity.NewAdmissionScopeMapping(tenant, id, loadScope, scope, interval, "SYN-BASIS-MAPPING")
	if err != nil {
		t.Fatal(err)
	}
	var end *accessidentity.AdmissionScopeMappingEnd
	if !endedAt.IsZero() {
		ending, err := accessidentity.NewAdmissionScopeMappingEnd(tenant, id, endedAt, "SYN-BASIS-END")
		if err != nil {
			t.Fatal(err)
		}
		end = &ending
	}
	recorded, err := accessidentity.NewRecordedAdmissionScopeMapping(mapping, end)
	if err != nil {
		t.Fatal(err)
	}
	return recorded
}

func authorityInterval(scope, authority string, from, to time.Time) pgdomain.AuthorityInterval {
	return pgdomain.AuthorityInterval{
		ObjectScope: scope,
		Capability:  loadScope.Capability,
		FactKind:    loadScope.FactKind,
		Authority:   authority,
		From:        from,
		To:          to,
	}
}

func bridge(t *testing.T, mappings mappingBook, intervals *intervalBook) *aipilot.AdmissionScope {
	t.Helper()
	coverage, err := pgapplication.NewAuthorityCoverage(intervals, selfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := aipilot.NewAdmissionScope(mappings, coverage)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

// Covers: 对照已登且区间覆盖 → 放行；对照未登、对照已结束、或对照指向的对象范围上区间不覆盖（别的权威、已关、未始、
// 别的对象范围）→ 不在准入范围。对照没有生效的那一行时，区间一行都不读。
func TestTheBridgeAdmitsOnlyWhereTheMappingAndTheIntervalBothHold(t *testing.T) {
	registered := mappingBook{rows: []accessidentity.RecordedAdmissionScopeMapping{row(t, "SYN-MAP-1", decisionScope, day, time.Time{})}}
	covering := authorityInterval(decisionScope, selfAuthority, day, time.Time{})
	cases := map[string]struct {
		mappings  mappingBook
		intervals []pgdomain.AuthorityInterval
		admitted  bool
		reads     int
	}{
		"mapping and interval":     {registered, []pgdomain.AuthorityInterval{covering}, true, 1},
		"no mapping":               {mappingBook{}, []pgdomain.AuthorityInterval{covering}, false, 0},
		"mapping ended":            {mappingBook{rows: []accessidentity.RecordedAdmissionScopeMapping{row(t, "SYN-MAP-1", decisionScope, day, now)}}, []pgdomain.AuthorityInterval{covering}, false, 0},
		"another authority":        {registered, []pgdomain.AuthorityInterval{authorityInterval(decisionScope, "SYN-AUTH/legacy-engine", day, time.Time{})}, false, 1},
		"interval closed":          {registered, []pgdomain.AuthorityInterval{authorityInterval(decisionScope, selfAuthority, day, now)}, false, 1},
		"interval not yet open":    {registered, []pgdomain.AuthorityInterval{authorityInterval(decisionScope, selfAuthority, now.Add(time.Hour), time.Time{})}, false, 1},
		"another object scope":     {registered, []pgdomain.AuthorityInterval{authorityInterval("SYN-PILOT-SCOPE/routing@v3", selfAuthority, day, time.Time{})}, false, 1},
		"mapping to another scope": {mappingBook{rows: []accessidentity.RecordedAdmissionScopeMapping{row(t, "SYN-MAP-1", "SYN-PILOT-SCOPE/routing@v3", day, time.Time{})}}, []pgdomain.AuthorityInterval{covering}, false, 1},
	}
	for name, testCase := range cases {
		intervals := &intervalBook{intervals: testCase.intervals}
		admitted, err := bridge(t, testCase.mappings, intervals).Admits(context.Background(), tenant, loadScope, now)
		if err != nil || admitted != testCase.admitted {
			t.Fatalf("%s: admitted = %v, err = %v; want %v", name, admitted, err, testCase.admitted)
		}
		if intervals.reads != testCase.reads {
			t.Fatalf("%s: 区间读了 %d 次，want %d", name, intervals.reads, testCase.reads)
		}
	}

	otherTenant, err := bridge(t, registered, &intervalBook{intervals: []pgdomain.AuthorityInterval{covering}}).Admits(context.Background(), "SYN-TENANT-02", loadScope, now)
	if err != nil || otherTenant {
		t.Fatalf("别的租户借这一行对照：admitted = %v, err = %v", otherTenant, err)
	}
}

// operatorBook 与 tokenVerifier 是铸造器的替身：一个持装载分配授予的合成操作者。
type operatorBook struct{}

func (operatorBook) FindOperator(_ context.Context, subject accessidentity.OperatorSubject) (accessidentity.OperatorStanding, bool, error) {
	binding, err := accessidentity.NewOperatorBinding(subject, tenant, "SYN-BASIS-BINDING")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	validity, err := accessidentity.NewEffectiveInterval(day, time.Time{})
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	grant, err := accessidentity.NewOperationDecisionGrant(tenant, "SYN-GRANT-LOAD", subject, accessidentity.DecisionLoadAssignment, validity, "SYN-BASIS-GRANT")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	recorded, err := accessidentity.NewRecordedGrant(grant, nil)
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	standing, err := accessidentity.NewOperatorStanding(binding, []accessidentity.RecordedGrant{recorded})
	return standing, err == nil, err
}

type tokenVerifier struct{}

func (tokenVerifier) VerifyOperatorCredential(context.Context, accessidentity.OperatorCredential) (accessidentity.OperatorSubject, error) {
	return accessidentity.NewOperatorSubject("https://id.syn.example/dex", "SYN-OPERATOR-01")
}

// Covers: 对照读不动、同键两行同时生效、区间读不动——经铸造器出来都是准入范围读不动，且不被「不在准入范围」认出；
// 齐备时铸得出信封。
func TestTheBridgeFailuresReachTheMinterAsDependencyFaults(t *testing.T) {
	registered := mappingBook{rows: []accessidentity.RecordedAdmissionScopeMapping{row(t, "SYN-MAP-1", decisionScope, day, time.Time{})}}
	covering := []pgdomain.AuthorityInterval{authorityInterval(decisionScope, selfAuthority, day, time.Time{})}
	request := accessidentity.OperatorRequest{
		Face:         accessidentity.CapabilityOperationDecision,
		DecisionKind: accessidentity.DecisionLoadAssignment,
		Admission:    &loadScope,
	}
	mint := func(mappings mappingBook, intervals *intervalBook) error {
		minter, err := accessidentity.NewOperatorMinter(tokenVerifier{}, operatorBook{}, bridge(t, mappings, intervals), func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		_, err = minter.MintOperator(context.Background(), accessidentity.NewOperatorCredential("presented.operator.token"), request)
		return err
	}

	if err := mint(registered, &intervalBook{intervals: covering}); err != nil {
		t.Fatalf("对照与区间齐备：%v", err)
	}
	if err := mint(mappingBook{}, &intervalBook{intervals: covering}); !errors.Is(err, accessidentity.ErrOutsideAdmissionScope) {
		t.Fatalf("对照未登：err = %v, want ErrOutsideAdmissionScope", err)
	}
	clashing := mappingBook{rows: []accessidentity.RecordedAdmissionScopeMapping{
		row(t, "SYN-MAP-1", decisionScope, day, time.Time{}),
		row(t, "SYN-MAP-2", "SYN-PILOT-SCOPE/operation-decisions@v2", day, time.Time{}),
	}}
	for name, failure := range map[string]struct {
		mappings  mappingBook
		intervals *intervalBook
	}{
		"mapping register down":  {mappingBook{err: errors.New("mapping register unreachable")}, &intervalBook{intervals: covering}},
		"two mappings effective": {clashing, &intervalBook{intervals: covering}},
		"interval register down": {registered, &intervalBook{err: errors.New("interval register unreachable")}},
	} {
		err := mint(failure.mappings, failure.intervals)
		if !errors.Is(err, accessidentity.ErrAdmissionScopeUnavailable) || errors.Is(err, accessidentity.ErrOutsideAdmissionScope) {
			t.Fatalf("%s: err = %v, want ErrAdmissionScopeUnavailable 且不是 ErrOutsideAdmissionScope", name, err)
		}
	}
}
