package postgres_test

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 票 label-channel/18 完成判据的最后一格：票 16 收编执行器用例矩阵里「凭证不认识→留痕」要能在**真实现**
// 上复现——不是替身答`未知`，而是真登记册按登记内容答出来。同一次装配再证反面：登记过并指向载运对象
// 的凭证让同一条素材被认领。执行器、事实登记册、留痕册、身份签发、凭证解析口全接真库；只有有效时间规则
// 与交接意图用替身——前者是票 19 的实例半边，后者在待判断版本上本就没有意图可交。

type absentRules struct{}

func (absentRules) JudgeEffectiveTime(context.Context, ports.EffectiveTimeRuleInput) (ports.EffectiveTimeRuling, error) {
	return ports.EffectiveTimeRuling{Outcome: ports.EffectiveTimeRuleAbsent}, nil
}

type noHandoff struct{}

func (noHandoff) HandOffExternalTrackingFact(context.Context, ports.ExternalTrackingFactHandoffIntent) error {
	return nil
}

func TestTheAdoptionExecutorLeavesMaterialUnadoptedWhenTheRealRegistryDoesNotKnowTheCredential(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	facts, err := adapter.NewExternalTrackingFacts(db)
	if err != nil {
		t.Fatalf("事实登记册：%v", err)
	}
	identities, err := adapter.NewResultVersions(db)
	if err != nil {
		t.Fatalf("身份签发：%v", err)
	}
	credentials, err := adapter.NewExternalCarrierCredentials(db, credentialClock{at: credentialNow})
	if err != nil {
		t.Fatalf("凭证登记册：%v", err)
	}
	executor := application.NewAdoptTrackingMaterialHandler(application.AdoptTrackingMaterialDeps{
		Facts:       facts,
		Identities:  identities,
		Credentials: credentials,
		Rules:       absentRules{},
		Ledger:      facts,
		Downstream:  noHandoff{},
		Clock:       credentialClock{at: credentialNow},
	})
	transactor := db.Transactor()
	ctx := t.Context()
	tenant := segmentRef(t, domain.NewTenantID, "tenant-1")

	material := ports.TrackingMaterial{
		Source:          "aggregator-a",
		Subject:         ports.TrackingSubject{Tenant: tenant, CredentialReference: "carrier-x/1Z-ADOPT-1"},
		SourceEventID:   "evt-adopt-1",
		OccurredAt:      ports.SourceTime{Given: true, At: credentialNow.Add(-time.Hour)},
		ReceivedAt:      credentialNow.Add(-30 * time.Minute),
		StatusReference: "IN_TRANSIT",
		PayloadDigest:   "sha256:adopt-1",
	}

	adopt := func(t *testing.T) application.AdoptTrackingMaterialResult {
		t.Helper()
		var result application.AdoptTrackingMaterialResult
		mustWithinDispatchTaskTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			var adoptErr error
			result, adoptErr = executor.Adopt(txCtx, material)
			return adoptErr
		})
		return result
	}

	unknown := adopt(t)
	if unknown.Outcome() != application.TrackingMaterialLeftUnadopted || unknown.UnadoptedReason() != ports.UnadoptedCredentialUnknown {
		t.Fatalf("凭证从未登记应留痕 CREDENTIAL_UNKNOWN：%s / %s", unknown.Outcome(), unknown.UnadoptedReason())
	}
	var ledgerRows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM transport_fulfillment.unadopted_tracking_material WHERE credential_ref = $1 AND reason = $2`,
		"carrier-x/1Z-ADOPT-1", ports.UnadoptedCredentialUnknown.String()).Scan(&ledgerRows); err != nil {
		t.Fatalf("数留痕：%v", err)
	}
	if ledgerRows != 1 {
		t.Fatalf("留痕册应有一条 CREDENTIAL_UNKNOWN：%d", ledgerRows)
	}

	// 登记这份凭证指向一个载运对象，同一条素材再来就被认领，对象取自登记册而不是凭证字符串。
	mustSaveCredential(t, transactor, ctx, credentials, credentialRecord(t, credentialFixtureOptions{
		credential: "carrier-x/1Z-ADOPT-1", version: "ECV-1", reference: "PCL-ADOPT-1",
	}))
	adopted := adopt(t)
	if adopted.Outcome() != application.TrackingFactAdoptedPendingJudgment {
		t.Fatalf("凭证登记后应认领（无规则即待判断）：%s reason=%s", adopted.Outcome(), adopted.UndecidedReason())
	}
	record, has := adopted.Record()
	if !has || record.Fact.Object().String() != "PCL-ADOPT-1" || record.Fact.Credential().String() != "carrier-x/1Z-ADOPT-1" {
		t.Fatalf("事实应挂在登记册解析出的载运对象上：%+v has=%v", record.Key, has)
	}
	if _, judged := record.Fact.EffectiveAt(); judged {
		t.Fatal("没有有效时间规则却判出了有效时间")
	}
}
