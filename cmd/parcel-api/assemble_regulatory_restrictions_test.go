package main

import (
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	customsdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: 监管限制两口的第二参是真编排——限制册、外部结果册（核依据）、Outbox 意图交付与事务壳在真实 PostgreSQL 上装得
// 起来。两条监管决定先经外部结果口的真编排入册；依据第一条建立限制，范围与生效时刻取自它；引一条不在册的结果、
// 或一条已被来源更正掉的决定解除，都答依据不成立；依据第二条解除，解除依据与时刻落进同一行，建立与解除各入队一份意图；
// 再解除一次答已解除。测试输入是隔离合成，只记 `S`。
func TestTheWiredRegulatoryRestrictionsAreEstablishedAndReleasedOnRecordedResults(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	results, err := buildExternalResultsOrchestration(db)
	if err != nil {
		t.Fatalf("装配外部结果编排：%v", err)
	}
	faces, err := buildRegulatoryRestrictionFaces(db)
	if err != nil {
		t.Fatalf("装配监管限制编排：%v", err)
	}
	const tenant = "SYN-TENANT-1"
	seedExternalResultChain(t, pool, tenant, "SYN-CC-UNIT-1", "SYN-CC-CASE-1", "SYN-CC-VERSION-1")
	seedInterpretationRule(t, pool, tenant, customsdomain.ProcessDecisionLayer, "SYN-JURISDICTION-1", externalResultOccurredAt.Add(-48*time.Hour))
	hold := externalResultCommand(t, "SYN-HOLD-1", "SYN-CC-VERSION-1")
	lift := externalResultCommand(t, "SYN-LIFT-1", "SYN-CC-VERSION-1")
	lift.OccurredAt = hold.OccurredAt.Add(30 * time.Minute)
	// 一条被来源更正掉的决定：先交 SYN-LIFT-WITHDRAWN，来源再交一份更正指向它。
	withdrawn := externalResultCommand(t, "SYN-LIFT-WITHDRAWN", "SYN-CC-VERSION-1")
	withdrawn.OccurredAt = hold.OccurredAt.Add(10 * time.Minute)
	correction := externalResultCommand(t, "SYN-LIFT-CORRECTION", "SYN-CC-VERSION-1")
	correction.OccurredAt = hold.OccurredAt.Add(20 * time.Minute)
	correction.Corrects = "SYN-LIFT-WITHDRAWN"
	for _, command := range []customsapp.ReceiveExternalResultCommand{withdrawn, correction} {
		if recorded, err := results.Handle(t.Context(), command); err != nil || recorded.Outcome() != customsapp.ResultRecorded {
			t.Fatalf("%s 没入册：outcome = %v err = %v", command.SourceID, recorded.Outcome(), err)
		}
	}
	for name, command := range map[string]customsapp.ReceiveExternalResultCommand{"扣留决定": hold, "解除决定": lift} {
		recorded, err := results.Handle(t.Context(), command)
		if err != nil || recorded.Outcome() != customsapp.ResultRecorded {
			t.Fatalf("%s没入册：outcome = %v err = %v", name, recorded.Outcome(), err)
		}
	}
	tenantID := mustValue(t, customsdomain.NewTenantID, tenant)
	id := mustValue(t, customsdomain.NewRestrictionID, "SYN-RR-1")

	established, err := faces.EstablishOnResult(t.Context(), customsapp.EstablishRestrictionOnResultCommand{
		TenantID: tenantID, ID: id, Decision: "SYN-HOLD-1",
		Constrains: []customsdomain.GuardedAction{customsdomain.OutboundRelease},
	})
	if err != nil || established.Outcome() != customsapp.RestrictionEstablished {
		t.Fatalf("建立：outcome = %v err = %v", established.Outcome(), err)
	}
	restriction, _ := established.Restriction()
	if restriction.Scope().String() != "SYN-CC-UNIT-1" || !restriction.EffectiveAt().Equal(hold.OccurredAt) {
		t.Fatalf("限制没有取所引决定的范围与发生时间：%+v", restriction)
	}

	refused, err := faces.ReleaseOnResult(t.Context(), customsapp.ReleaseRestrictionOnResultCommand{TenantID: tenantID, ID: id, Result: "SYN-NO-SUCH-RESULT"})
	if err != nil || refused.Outcome() != customsapp.RestrictionBasisNotAccepted {
		t.Fatalf("引不在册的结果解除：outcome = %v err = %v, want BASIS_NOT_ACCEPTED", refused.Outcome(), err)
	}

	superseded, err := faces.ReleaseOnResult(t.Context(), customsapp.ReleaseRestrictionOnResultCommand{TenantID: tenantID, ID: id, Result: "SYN-LIFT-WITHDRAWN"})
	if err != nil || superseded.Outcome() != customsapp.RestrictionBasisNotAccepted {
		t.Fatalf("引一条已被来源更正掉的决定解除：outcome = %v err = %v, want BASIS_NOT_ACCEPTED", superseded.Outcome(), err)
	}
	establishedIntent := string(outboxintent.FingerprintEventID("restriction", tenant, "SYN-RR-1", "established"))
	releasedIntent := string(outboxintent.FingerprintEventID("restriction", tenant, "SYN-RR-1", "released"))
	if got, none := countOutboxRows(t, pool, establishedIntent), countOutboxRows(t, pool, releasedIntent); got != 1 || none != 0 {
		t.Fatalf("解除之前：建立意图 %d 行、解除意图 %d 行，want 1 与 0", got, none)
	}

	released, err := faces.ReleaseOnResult(t.Context(), customsapp.ReleaseRestrictionOnResultCommand{TenantID: tenantID, ID: id, Result: "SYN-LIFT-1"})
	if err != nil || released.Outcome() != customsapp.RestrictionReleased {
		t.Fatalf("解除：outcome = %v err = %v", released.Outcome(), err)
	}
	var releasedBy string
	var releasedAt time.Time
	if err := pool.QueryRow(t.Context(),
		`SELECT released_by, released_at FROM customs_compliance.regulatory_restriction WHERE tenant_id = $1 AND restriction_id = $2`,
		tenant, "SYN-RR-1",
	).Scan(&releasedBy, &releasedAt); err != nil {
		t.Fatalf("读在册限制：%v", err)
	}
	if releasedBy != "SYN-LIFT-1" || !releasedAt.Equal(lift.OccurredAt) {
		t.Fatalf("在册解除依据 %q 时刻 %v，want SYN-LIFT-1 / %v", releasedBy, releasedAt, lift.OccurredAt)
	}

	if got := countOutboxRows(t, pool, releasedIntent); got != 1 {
		t.Fatalf("解除意图 outbox 行数 = %d，want 1——解除要有自己的一份意图，不被建立那一份吞掉", got)
	}

	again, err := faces.ReleaseOnResult(t.Context(), customsapp.ReleaseRestrictionOnResultCommand{TenantID: tenantID, ID: id, Result: "SYN-LIFT-1"})
	if err != nil || again.Outcome() != customsapp.RestrictionAlreadyReleased {
		t.Fatalf("再解除：outcome = %v err = %v, want ALREADY_RELEASED", again.Outcome(), err)
	}
}
