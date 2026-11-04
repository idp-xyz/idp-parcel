package postgres_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// 本文件对真实 PostgreSQL 证操作者册的按租户查阅口与按授予标识的装载口（ADR-0363 决定二、三）：只列本租户、主体与授予各按登记
// 时刻排、撤销随授予一起回、空租户答空列表、别的租户拿同一个授予标识读不到。

// Covers: ADR-0363 决定二——ListOperators 只列本租户，主体按登记先后、授予按登记先后，决定种类、作业范围、终点与撤销各随行回来；
// 没授予的主体也在列上；空租户答空列表而不是 error；空白租户拒。
func TestListOperatorsListsTheTenantInRecordedOrder(t *testing.T) {
	register := newOperatorRegister(t)
	first := subjectOf(t, syntheticIssuer, "SYN-OPERATOR-CAT-01")
	second := subjectOf(t, syntheticIssuer, "SYN-OPERATOR-CAT-00")
	foreign := subjectOf(t, syntheticIssuer, "SYN-OPERATOR-CAT-02")
	expectOutcome(t, "bind first", register.bind(t, bindingOf(t, first, tenantOne, "SYN-OPERATOR-BASIS-01")), accessidentity.OperatorRegistrationRecorded)
	expectOutcome(t, "bind second", register.bind(t, bindingOf(t, second, tenantOne, "SYN-OPERATOR-BASIS-00")), accessidentity.OperatorRegistrationRecorded)
	expectOutcome(t, "bind foreign", register.bind(t, bindingOf(t, foreign, tenantTwo, "SYN-OPERATOR-BASIS-02")), accessidentity.OperatorRegistrationRecorded)

	endsAt := grantStartsAt.Add(30 * 24 * time.Hour)
	expectOutcome(t, "grant write", register.grant(t, grantOf(t, tenantOne, "SYN-CAT-GRANT-B", first, accessidentity.CapabilityRegistryConfigurationWrite, grantStartsAt, endsAt)), accessidentity.OperatorRegistrationRecorded)
	expectOutcome(t, "grant admin", register.grant(t, grantOf(t, tenantOne, "SYN-CAT-GRANT-A", first, accessidentity.CapabilityOperatorAccessAdministration, grantStartsAt, time.Time{})), accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(grantStartsAt, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := accessidentity.NewOperationDecisionGrant(tenantOne, "SYN-CAT-GRANT-D", first, accessidentity.DecisionSegmentClosure, interval, "SYN-GRANT-BASIS-D")
	if err != nil {
		t.Fatal(err)
	}
	expectOutcome(t, "grant decision", register.grant(t, decision), accessidentity.OperatorRegistrationRecorded)
	scope, err := accessidentity.NewWorkScope(accessidentity.WorkScopeOffsite, "SYN-OFFSITE-CN-EAST")
	if err != nil {
		t.Fatal(err)
	}
	work, err := accessidentity.NewWorkFactRegistrationGrant(tenantOne, "SYN-CAT-GRANT-W", first, scope, interval, "SYN-GRANT-BASIS-W")
	if err != nil {
		t.Fatal(err)
	}
	expectOutcome(t, "grant work fact", register.grant(t, work), accessidentity.OperatorRegistrationRecorded)
	expectOutcome(t, "grant foreign", register.grant(t, grantOf(t, tenantTwo, "SYN-CAT-GRANT-F", foreign, accessidentity.CapabilityMasterDataAndOperationsRead, grantStartsAt, time.Time{})), accessidentity.OperatorRegistrationRecorded)
	revokedAt := grantStartsAt.Add(24 * time.Hour)
	expectOutcome(t, "revoke write", register.revoke(t, revocationOf(t, tenantOne, "SYN-CAT-GRANT-B", revokedAt, "SYN-REVOKE-BASIS-B")), accessidentity.OperatorRegistrationRecorded)

	rows, err := register.registry.ListOperators(t.Context(), tenantOne)
	if err != nil {
		t.Fatalf("ListOperators：%v", err)
	}
	if len(rows) != 2 || rows[0].Binding().Subject() != first || rows[1].Binding().Subject() != second {
		t.Fatalf("主体顺序 = %+v, want 先登的在前", rows)
	}
	if rows[0].RecordedAt().IsZero() || !rows[0].RecordedAt().Before(rows[1].RecordedAt()) {
		t.Fatalf("登记时刻 = %v / %v", rows[0].RecordedAt(), rows[1].RecordedAt())
	}
	if rows[0].Binding().Basis() != "SYN-OPERATOR-BASIS-01" || len(rows[1].Grants()) != 0 {
		t.Fatalf("绑定 = %+v，第二个主体的授予 = %d", rows[0].Binding(), len(rows[1].Grants()))
	}
	grants := rows[0].Grants()
	if len(grants) != 4 {
		t.Fatalf("授予 = %d, want 4", len(grants))
	}
	wantOrder := []string{"SYN-CAT-GRANT-B", "SYN-CAT-GRANT-A", "SYN-CAT-GRANT-D", "SYN-CAT-GRANT-W"}
	for index, row := range grants {
		if row.Recorded().Grant().GrantID() != wantOrder[index] || row.RecordedAt().IsZero() {
			t.Fatalf("第 %d 笔 = %s（登记于 %v），want %s", index, row.Recorded().Grant().GrantID(), row.RecordedAt(), wantOrder[index])
		}
		if index > 0 && row.RecordedAt().Before(grants[index-1].RecordedAt()) {
			t.Fatalf("授予没按登记先后排：%v 在 %v 之后", row.RecordedAt(), grants[index-1].RecordedAt())
		}
	}
	write := grants[0].Recorded()
	revocation, revoked := write.Revocation()
	if end, bounded := write.Grant().Interval().EndsAt(); !bounded || !end.Equal(endsAt.Truncate(time.Microsecond)) || !revoked ||
		!revocation.RevokedAt().Equal(revokedAt.Truncate(time.Microsecond)) || revocation.Basis() != "SYN-REVOKE-BASIS-B" {
		t.Fatalf("登记册配置写那笔 = %+v / 撤销 %+v", write.Grant(), revocation)
	}
	if _, revoked := grants[1].Recorded().Revocation(); revoked || grants[1].Recorded().Grant().Face() != accessidentity.CapabilityOperatorAccessAdministration {
		t.Fatalf("操作者接入管理那笔 = %+v", grants[1].Recorded())
	}
	if grants[2].Recorded().Grant().DecisionKind() != accessidentity.DecisionSegmentClosure {
		t.Fatalf("运营决定那笔的种类 = %s", grants[2].Recorded().Grant().DecisionKind())
	}
	if grants[3].Recorded().Grant().WorkScope() != scope {
		t.Fatalf("作业事实登记那笔的范围 = %+v", grants[3].Recorded().Grant().WorkScope())
	}

	none, err := register.registry.ListOperators(t.Context(), "SYN-TENANT-EMPTY")
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("空租户 = %v, %v; want 空列表", none, err)
	}
	if _, err := register.registry.ListOperators(t.Context(), " "); !errors.Is(err, accessidentity.ErrOperatorCatalogueTenantRequired) {
		t.Fatalf("空白租户 err = %v", err)
	}
}

// Covers: ADR-0363 决定三——FindGrant 按（租户、授予标识）取一笔连同撤销；不在册答 found=false 不报错；别的租户拿同一个标识读不到。
func TestFindGrantReadsOneGrantWithinTheTenant(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, syntheticIssuer, "SYN-OPERATOR-FIND-01")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, tenantOne, "SYN-OPERATOR-BASIS-01")), accessidentity.OperatorRegistrationRecorded)
	expectOutcome(t, "grant", register.grant(t, grantOf(t, tenantOne, "SYN-FIND-GRANT", subject, accessidentity.CapabilityOperatorAccessAdministration, grantStartsAt, time.Time{})), accessidentity.OperatorRegistrationRecorded)

	recorded, found, err := register.registry.FindGrant(t.Context(), tenantOne, "SYN-FIND-GRANT")
	if err != nil || !found || recorded.Grant().Subject() != subject || recorded.Grant().Face() != accessidentity.CapabilityOperatorAccessAdministration {
		t.Fatalf("FindGrant = %+v, %v, %v", recorded, found, err)
	}
	if _, revoked := recorded.Revocation(); revoked || !recorded.LocksOutCallerIfRevoked(subject, grantStartsAt.Add(time.Hour)) {
		t.Fatal("未撤销、生效中的管理授予应当锁得住持有者自己")
	}

	revokedAt := grantStartsAt.Add(time.Hour)
	expectOutcome(t, "revoke", register.revoke(t, revocationOf(t, tenantOne, "SYN-FIND-GRANT", revokedAt, "SYN-REVOKE-BASIS")), accessidentity.OperatorRegistrationRecorded)
	recorded, found, err = register.registry.FindGrant(t.Context(), tenantOne, "SYN-FIND-GRANT")
	revocation, revoked := recorded.Revocation()
	if err != nil || !found || !revoked || !revocation.RevokedAt().Equal(revokedAt.Truncate(time.Microsecond)) || recorded.LocksOutCallerIfRevoked(subject, revokedAt.Add(time.Hour)) {
		t.Fatalf("撤销后 FindGrant = %+v, %v, %v", recorded, found, err)
	}

	if _, found, err := register.registry.FindGrant(t.Context(), tenantTwo, "SYN-FIND-GRANT"); found || err != nil {
		t.Fatalf("别的租户：found %v, err %v", found, err)
	}
	if _, found, err := register.registry.FindGrant(t.Context(), tenantOne, "SYN-FIND-GHOST"); found || err != nil {
		t.Fatalf("不在册：found %v, err %v", found, err)
	}
}
