package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证并列的读法与裁决证据（ADR-0309 决定一、四、六；迁移 0030）：未形成登记按判断键、按包裹读回最近一次
// 停住的原因与并列的那几家；裁决证据与同键的判断库行、未形成登记行同在，无事务拒写；并列读面分待决定、已决定与另行形成三格。

func newTieStores(t *testing.T) (*adapter.InitialRoutes, *adapter.InitialRouteTieDecisions, *bentopg.DB) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	routes, err := adapter.NewInitialRoutes(db)
	if err != nil {
		t.Fatal(err)
	}
	ties, err := adapter.NewInitialRouteTieDecisions(db)
	if err != nil {
		t.Fatal(err)
	}
	return routes, ties, db
}

// Covers: AT-NR-100（ADR-0309 决定一）——登记行按判断键读回原因、交接关联、并列的那几家；按（租户，包裹）读回入账时刻最晚的那一行；
// 没有行答 false，他租户的行读不到；同一判断键再停在别的原因后，读回的就是别的原因（最近一次停住的原因）。
func TestTheNotFormedRegisterReadsBackTheLatestReasonAndTheTiedOnes(t *testing.T) {
	routes, _, db := newTieStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")

	if _, found, err := routes.FindNotFormed(ctx, key); err != nil || found {
		t.Fatalf("空册 found = %v err = %v", found, err)
	}
	within(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		return routes.RecordNotFormed(txCtx, tiedNotFormed(t, key))
	})
	read, found, err := routes.FindNotFormed(ctx, key)
	if err != nil || !found {
		t.Fatalf("found = %v err = %v", found, err)
	}
	if read.Key != key || read.Reason != ports.NotFormedCandidatesTied || read.Correlation.String() != "acceptance/decision-tie" ||
		len(read.Tied) != 2 || read.Tied[0].String() != "SYN-LINE-A@1" || read.Tied[1].String() != "SYN-LINE-B@1" ||
		!read.AcceptedAt.Equal(notFormedAcceptedAt) || !read.SettledAt.Equal(reassessedAtTS) {
		t.Fatalf("read = %+v", read)
	}
	byParcel, found, err := routes.FindLatestNotFormedOfParcel(ctx, key.TenantID, key.DeclaredParcelID)
	if err != nil || !found || byParcel.Key != key {
		t.Fatalf("by parcel = %+v found = %v err = %v", byParcel, found, err)
	}
	if _, found, _ := routes.FindLatestNotFormedOfParcel(ctx, scalar(t, domain.NewTenantID, "tenant-2"), key.DeclaredParcelID); found {
		t.Fatal("读到了他租户的登记行")
	}

	// 同一包裹换了一条基线重新受理：最近入账的那一行才是在等裁决的。
	rebased := key
	rebased.AcceptanceBaseline = scalar(t, domain.NewAcceptanceBaselineReference, "baseline-v2")
	later := notFormed(t, rebased, "acceptance/decision-2", ports.NotFormedCandidateCostsPending)
	later.SettledAt = reassessedAtTS.Add(time.Hour)
	within(t, db.Transactor(), ctx, func(txCtx context.Context) error { return routes.RecordNotFormed(txCtx, later) })
	latest, _, _ := routes.FindLatestNotFormedOfParcel(ctx, key.TenantID, key.DeclaredParcelID)
	if latest.Key != rebased || latest.Reason != ports.NotFormedCandidateCostsPending || len(latest.Tied) != 0 {
		t.Fatalf("latest = %+v, want 后入账的那条基线与它的原因", latest)
	}
}

// Covers: AT-NR-100、AT-NR-103（ADR-0309 决定四、六）——裁决证据只能落在已有判断库行与未形成登记行的判断键上（外键），无事务拒写；
// 并列读面按状态分三格：判断库没有这件包裹是 PENDING，有计划又有裁决证据是 DECIDED（带决定），有计划而没有裁决证据是
// FORMED_OTHERWISE；只列停在并列的行，他租户读不到。
func TestTieDecisionEvidenceAndTheTieReadFaceStates(t *testing.T) {
	routes, ties, db := newTieStores(t)
	ctx := t.Context()
	decided := routeKey(t, "tenant-1", "parcel-decided")
	pending := routeKey(t, "tenant-1", "parcel-pending")
	otherwise := routeKey(t, "tenant-1", "parcel-otherwise")
	elsewhere := routeKey(t, "tenant-1", "parcel-elsewhere")
	for _, key := range []domain.InitialRouteJudgmentKey{decided, pending, otherwise} {
		within(t, db.Transactor(), ctx, func(txCtx context.Context) error { return routes.RecordNotFormed(txCtx, tiedNotFormed(t, key)) })
	}
	within(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		return routes.RecordNotFormed(txCtx, notFormed(t, elsewhere, "acceptance/x", ports.NotFormedCandidateCostsPending))
	})
	record := func(key domain.InitialRouteJudgmentKey) ports.InitialRouteTieDecision {
		return ports.InitialRouteTieDecision{
			Key:         key,
			Candidate:   scalar(t, domain.NewCandidateID, "SYN-LINE-B@1"),
			PlanVersion: scalar(t, domain.NewRoutePlanVersionID, "SYN-RPV-97"),
			Basis:       scalar(t, domain.NewRerouteDecisionBasisReference, "SYN-APPROVAL-97"),
			DecidedBy:   scalar(t, domain.NewAuthorizedRoleReference, "OPERATOR/https://issuer.example#ops-7"),
			DecidedAt:   reassessedAtTS.Add(2 * time.Hour),
		}
	}

	// 先有计划后有证据：判断库没有这一行时外键拒。
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		return ties.Record(txCtx, record(decided))
	}); err == nil {
		t.Fatal("没有计划的裁决证据落了库")
	}
	if err := ties.Record(ctx, record(decided)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务记裁决证据应返回 ErrTransactionRequired，实得：%v", err)
	}
	within(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		for _, key := range []domain.InitialRouteJudgmentKey{decided, otherwise} {
			outcome, err := routes.Save(txCtx, ports.InitialRouteRecord{Key: key, Plan: formedPlan(t, key, "SYN-RPV-97"), HasPlan: true})
			if err != nil || outcome != ports.InitialRouteSaved {
				return errors.Join(err, errors.New("save plan"))
			}
		}
		return ties.Record(txCtx, record(decided))
	})
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		return ties.Record(txCtx, record(decided))
	}); err == nil {
		t.Fatal("同一判断键第二次记裁决证据没被拒")
	}

	rows, err := ties.ListInitialRouteTies(ctx, decided.TenantID, 10)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]ports.InitialRouteTieCatalogueRow{}
	for _, row := range rows {
		states[row.DeclaredParcelID] = row
	}
	if len(rows) != 3 || states["parcel-decided"].State != ports.InitialRouteTieDecided ||
		states["parcel-pending"].State != ports.InitialRouteTiePending ||
		states["parcel-otherwise"].State != ports.InitialRouteTieFormedOtherwise {
		t.Fatalf("rows = %+v, want 三条并列各一格、停在别处的那件不列", rows)
	}
	row := states["parcel-decided"]
	if !row.HasDecision || row.Decision.Candidate != "SYN-LINE-B@1" || row.Decision.PlanVersion != "SYN-RPV-97" ||
		row.Decision.DecidedBy != "OPERATOR/https://issuer.example#ops-7" || row.Decision.Basis != "SYN-APPROVAL-97" ||
		len(row.Candidates) != 2 || len(row.Tied) != 2 || row.Tied[0].AmountMinor != 4200 || row.Tied[0].Currency != "CNY" {
		t.Fatalf("decided row = %+v", row)
	}
	if states["parcel-pending"].HasDecision {
		t.Fatal("待决定的带了决定")
	}
	if other, err := ties.ListInitialRouteTies(ctx, scalar(t, domain.NewTenantID, "tenant-2"), 10); err != nil || len(other) != 0 {
		t.Fatalf("他租户 rows = %v err = %v", other, err)
	}
	if _, err := ties.ListInitialRouteTies(ctx, decided.TenantID, 0); err == nil {
		t.Fatal("limit 0 没被拒")
	}
}
