package main

import (
	"context"
	"sync"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	visibilityapp "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	visibilitydomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// Covers: 证据三口的第二参是真编排（UC-VE-007 步 2）——证据册、标识签发器与事务壳在真实 PostgreSQL 上装得起来。登记一份
// 材料：形成已收到的证据项，登记人落下；同一提供方再登同一份答已有、登记人不变。评价：采信，评价人与时刻落下；再评价
// 答已评价、在册的结论不变。准备披露版本：锚住原件指纹；脱敏指纹与原件相同不受理。另一个租户拿同一个证据标识评价，
// 答不在册。测试输入是隔离合成，只记 `S`。
func TestTheWiredEvidenceIsRegisteredAppraisedAndPreparedForDisclosureAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	faces, err := buildEvidenceFaces(db)
	if err != nil {
		t.Fatalf("装配证据编排：%v", err)
	}
	const tenant, alice, bob = "SYN-TENANT-1", "https://issuer#alice", "https://issuer#bob"
	tenantID := mustValue(t, visibilitydomain.NewTenantID, tenant)
	submit := visibilityapp.SubmitEvidenceCommand{
		TenantID: tenantID,
		Provider: mustValue(t, visibilitydomain.NewEvidenceProviderReference, "SYN-CUSTOMER-1"),
		Digest:   mustValue(t, visibilitydomain.NewEvidenceContentDigest, "sha256:syn-damage-photo"),
		Operator: alice,
	}

	submitted, err := faces.SubmitEvidence(t.Context(), submit)
	if err != nil || submitted.Outcome() != visibilityapp.EvidenceSubmitted {
		t.Fatalf("登记：outcome = %v err = %v", submitted.Outcome(), err)
	}
	item, _ := submitted.Evidence()
	submit.Operator = bob
	if again, err := faces.SubmitEvidence(t.Context(), submit); err != nil || again.Outcome() != visibilityapp.EvidenceExistingResult {
		t.Fatalf("再登：outcome = %v err = %v, want EVIDENCE_EXISTING_RESULT", again.Outcome(), err)
	}

	other := mustValue(t, visibilitydomain.NewTenantID, "SYN-TENANT-2")
	if foreign, err := faces.Appraise(t.Context(), visibilityapp.AppraiseEvidenceCommand{
		TenantID: other, Evidence: item.ID(), Appraisal: visibilitydomain.EvidenceDiscredited, Basis: "别家的依据", Operator: bob,
	}); err != nil || foreign.Outcome() != visibilityapp.EvidenceNotFound {
		t.Fatalf("另一个租户评价：outcome = %v err = %v, want EVIDENCE_NOT_FOUND", foreign.Outcome(), err)
	}

	appraised, err := faces.Appraise(t.Context(), visibilityapp.AppraiseEvidenceCommand{
		TenantID: tenantID, Evidence: item.ID(), Appraisal: visibilitydomain.EvidenceCredited, Basis: "照片元数据与收寄时间一致", Operator: bob,
	})
	if err != nil || appraised.Outcome() != visibilityapp.EvidenceAppraised {
		t.Fatalf("评价：outcome = %v err = %v", appraised.Outcome(), err)
	}
	if again, err := faces.Appraise(t.Context(), visibilityapp.AppraiseEvidenceCommand{
		TenantID: tenantID, Evidence: item.ID(), Appraisal: visibilitydomain.EvidenceDiscredited, Basis: "相反结论", Operator: alice,
	}); err != nil || again.Outcome() != visibilityapp.EvidenceAlreadyAppraisedResult {
		t.Fatalf("再评价：outcome = %v err = %v, want EVIDENCE_ALREADY_APPRAISED", again.Outcome(), err)
	}

	prepared, err := faces.PrepareDisclosure(t.Context(), visibilityapp.PrepareEvidenceDisclosureCommand{
		TenantID: tenantID, Evidence: item.ID(),
		Redacted: mustValue(t, visibilitydomain.NewEvidenceContentDigest, "sha256:syn-damage-photo-redacted"), Scope: "SYN-CARRIER-1",
	})
	if err != nil || prepared.Outcome() != visibilityapp.EvidenceDisclosurePrepared {
		t.Fatalf("披露版本：outcome = %v err = %v", prepared.Outcome(), err)
	}
	if version, _ := prepared.Disclosure(); version.Original().String() != "sha256:syn-damage-photo" {
		t.Fatalf("披露版本没有锚住原件指纹：%q", version.Original())
	}
	if leaked, err := faces.PrepareDisclosure(t.Context(), visibilityapp.PrepareEvidenceDisclosureCommand{
		TenantID: tenantID, Evidence: item.ID(), Redacted: submit.Digest, Scope: "SYN-CARRIER-1",
	}); err != nil || leaked.Outcome() != visibilityapp.ManageEvidenceNotAccepted {
		t.Fatalf("脱敏指纹与原件相同：outcome = %v err = %v, want NOT_ACCEPTED", leaked.Outcome(), err)
	}

	var registeredBy, appraisedBy, appraisal, basis string
	var appraisedAt *time.Time
	if err := pool.QueryRow(t.Context(),
		`SELECT registered_by, appraised_by, appraisal, appraisal_basis, appraised_at
		   FROM visibility_exception.evidence_item WHERE tenant_id = $1 AND evidence_id = $2`, tenant, item.ID().String(),
	).Scan(&registeredBy, &appraisedBy, &appraisal, &basis, &appraisedAt); err != nil {
		t.Fatalf("读证据项：%v", err)
	}
	if registeredBy != alice || appraisedBy != bob || appraisal != "CREDITED" || basis != "照片元数据与收寄时间一致" || appraisedAt == nil {
		t.Fatalf("登记人 %q 评价人 %q 评价 %q 依据 %q 时刻 %v", registeredBy, appraisedBy, appraisal, basis, appraisedAt)
	}
}

// Covers: 两个人同时评价同一项证据，评价只落一次——真库上反复跑：每一轮恰有一笔形成评价，另一笔答已评价并交回在册的
// 那一份；册上的结论与评价人是同一笔的。
func TestConcurrentAppraisalsOfOneEvidenceItemSettleOnExactlyOne(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	faces, err := buildEvidenceFaces(db)
	if err != nil {
		t.Fatalf("装配证据编排：%v", err)
	}
	const tenant = "SYN-TENANT-1"
	tenantID := mustValue(t, visibilitydomain.NewTenantID, tenant)
	for round := 0; round < 20; round++ {
		submitted, err := faces.SubmitEvidence(t.Context(), visibilityapp.SubmitEvidenceCommand{
			TenantID: tenantID,
			Provider: mustValue(t, visibilitydomain.NewEvidenceProviderReference, "SYN-CUSTOMER-1"),
			Digest:   mustValue(t, visibilitydomain.NewEvidenceContentDigest, "sha256:syn-round-"+string(rune('a'+round))),
			Operator: "https://issuer#alice",
		})
		if err != nil || submitted.Outcome() != visibilityapp.EvidenceSubmitted {
			t.Fatalf("第 %d 轮登记：outcome = %v err = %v", round, submitted.Outcome(), err)
		}
		item, _ := submitted.Evidence()
		commands := []visibilityapp.AppraiseEvidenceCommand{
			{TenantID: tenantID, Evidence: item.ID(), Appraisal: visibilitydomain.EvidenceCredited, Basis: "采信的依据", Operator: "https://issuer#carol"},
			{TenantID: tenantID, Evidence: item.ID(), Appraisal: visibilitydomain.EvidenceDiscredited, Basis: "不采信的依据", Operator: "https://issuer#dave"},
		}
		outcomes := make([]visibilityapp.ManageEvidenceOutcome, len(commands))
		errs := make([]error, len(commands))
		var ready, done sync.WaitGroup
		start := make(chan struct{})
		for index, command := range commands {
			ready.Add(1)
			done.Add(1)
			go func() {
				defer done.Done()
				ready.Done()
				<-start
				result, err := faces.Appraise(context.Background(), command)
				outcomes[index], errs[index] = result.Outcome(), err
			}()
		}
		ready.Wait()
		close(start)
		done.Wait()

		winner := -1
		for index := range outcomes {
			if errs[index] != nil {
				t.Fatalf("第 %d 轮：评价报错：%v", round, errs[index])
			}
			switch outcomes[index] {
			case visibilityapp.EvidenceAppraised:
				if winner >= 0 {
					t.Fatalf("第 %d 轮：两笔评价都答形成了", round)
				}
				winner = index
			case visibilityapp.EvidenceAlreadyAppraisedResult:
			default:
				t.Fatalf("第 %d 轮：outcome = %v", round, outcomes[index])
			}
		}
		if winner < 0 {
			t.Fatalf("第 %d 轮：没有一笔评价形成", round)
		}
		var appraisal, basis, appraisedBy string
		if err := pool.QueryRow(t.Context(),
			`SELECT appraisal, appraisal_basis, appraised_by FROM visibility_exception.evidence_item WHERE tenant_id = $1 AND evidence_id = $2`,
			tenant, item.ID().String(),
		).Scan(&appraisal, &basis, &appraisedBy); err != nil {
			t.Fatalf("读证据项：%v", err)
		}
		want := commands[winner]
		if appraisal != want.Appraisal.String() || basis != want.Basis || appraisedBy != want.Operator {
			t.Fatalf("第 %d 轮：册上 %q/%q/%q，与答形成的那一笔（%q/%q/%q）不是同一笔", round, appraisal, basis, appraisedBy, want.Appraisal, want.Basis, want.Operator)
		}
	}
}
