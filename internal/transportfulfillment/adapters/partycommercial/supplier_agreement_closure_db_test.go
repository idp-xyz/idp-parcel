package partycommercial_test

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/partycommercial"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证代决复核 M16（ADR-0233 决定五）在核对缝上的那一格：PC 把旧版截止或退役之后，本包适配器
// 一行不改，经 PC 版本点读口核出来的就是收尾之后的口径——截止点之后答未生效，退役之后答已终止。

const (
	closureTenant   = "SYN-TENANT-01"
	closureProvider = "SYN-PARTY-CARRIER-TRUNK-01"
	closureObject   = "SYN-AGREEMENT-CLOSURE-01"
)

var (
	closureStart      = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	closureSuccessor  = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	closureRegistered = time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
)

type closureClock struct{}

func (closureClock) Now() time.Time { return closureRegistered }

// Covers: `AT-PC-058` 在运输履约核对上的那一半——旧版被截止在后继生效时点之后，按截止后的时点提交答
// `AGREEMENT_NOT_EFFECTIVE`、按截止前的时点照旧答在册；同起点重发的旧版退役之后答 `AGREEMENT_TERMINATED`，后继照旧在册。
func TestAClosedAgreementVersionIsCheckedAsClosed(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		t.Fatal(err)
	}
	v1 := saveClosureAgreement(t, db, publications, "v1", closureStart)
	saveClosureAgreement(t, db, publications, "v2", closureSuccessor)
	v3 := saveClosureAgreement(t, db, publications, "v3", closureSuccessor)

	contents, err := pcpostgres.NewSupplierAgreementContents(db)
	if err != nil {
		t.Fatal(err)
	}
	source, err := adapter.NewSupplierAgreementSource(contents)
	if err != nil {
		t.Fatal(err)
	}
	check := func(version string, at time.Time) tfports.SupplierAgreementStanding {
		t.Helper()
		agreement, err := tfdomain.NewSupplierAgreementVersion(closureObject, version)
		if err != nil {
			t.Fatal(err)
		}
		answer, err := source.CheckSupplierAgreement(t.Context(), mustBuild(t, tfdomain.NewTenantID, closureTenant), agreement,
			mustBuild(t, tfdomain.NewServiceProviderReference, closureProvider), at)
		if err != nil {
			t.Fatalf("%s 核对：%v", version, err)
		}
		return answer.Standing
	}
	afterCut, beforeCut := closureSuccessor.Add(48*time.Hour), closureSuccessor.Add(-48*time.Hour)
	if got := check("v1", afterCut); got != tfports.SupplierAgreementRegistered {
		t.Fatalf("截止前 v1 在后继生效之后答 %s, want AGREEMENT_REGISTERED（旧版未收尾时两版都能用）", got)
	}

	handler := pcapplication.NewCloseCommercialVersionHandler(publications, closureClock{})
	closeWith := func(close func(context.Context) (pcapplication.VersionClosureResult, error)) {
		t.Helper()
		var result pcapplication.VersionClosureResult
		if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			var closeErr error
			result, closeErr = close(txCtx)
			return closeErr
		}); err != nil {
			t.Fatal(err)
		}
		if result.Outcome() != pcapplication.VersionClosureRegistered {
			t.Fatalf("收尾 outcome = %s（cause %v）", result.Outcome(), result.Cause())
		}
	}
	closeWith(func(ctx context.Context) (pcapplication.VersionClosureResult, error) {
		return handler.Cut(ctx, pcapplication.CutCommercialVersionAtSuccessorCommand{
			VersionClosureTarget: closureTarget(t, v1, "v2"),
			Reference:            mustBuild(t, pcdomain.NewValidityCorrectionReference, "SYN-CUT-AGREEMENT-01"),
			CorrectedAt:          closureRegistered,
		})
	})
	if got := check("v1", afterCut); got != tfports.SupplierAgreementNotEffective || got.String() != "AGREEMENT_NOT_EFFECTIVE" {
		t.Fatalf("截止后 v1 在截止点之后答 %s, want AGREEMENT_NOT_EFFECTIVE", got)
	}
	if got := check("v1", beforeCut); got != tfports.SupplierAgreementRegistered {
		t.Fatalf("截止后 v1 在截止点之前答 %s, want AGREEMENT_REGISTERED", got)
	}

	closeWith(func(ctx context.Context) (pcapplication.VersionClosureResult, error) {
		return handler.RetireAtSameStart(ctx, pcapplication.RetireCommercialVersionAtSameStartCommand{
			VersionClosureTarget: closureTarget(t, v3, "v2"),
			Reference:            mustBuild(t, pcdomain.NewRetirementReference, "SYN-REISSUE-AGREEMENT-01"),
		})
	})
	if got := check("v3", afterCut); got != tfports.SupplierAgreementTerminated {
		t.Fatalf("退役后 v3 答 %s, want AGREEMENT_TERMINATED", got)
	}
	if got := check("v2", afterCut); got != tfports.SupplierAgreementRegistered {
		t.Fatalf("后继 v2 答 %s, want AGREEMENT_REGISTERED", got)
	}
}

func closureTarget(t *testing.T, version pcdomain.CommercialVersion, successor string) pcapplication.VersionClosureTarget {
	t.Helper()
	return pcapplication.VersionClosureTarget{
		Tenant: version.Tenant(), Scope: version.Scope(), Kind: version.Kind(), ObjectID: version.ObjectID(),
		Version: version.Version(), Successor: mustBuild(t, pcdomain.NewCommercialVersionLabel, successor),
	}
}

// saveClosureAgreement 经 PC 重建门造一版从 startsAt 起已生效的协议，连正文一起落进 PC 的册。
func saveClosureAgreement(
	t *testing.T,
	db *bentopg.DB,
	publications *pcpostgres.CommercialPublications,
	label string,
	startsAt time.Time,
) pcdomain.CommercialVersion {
	t.Helper()
	interval, err := pcdomain.NewEffectiveInterval(startsAt, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		mustBuild(t, pcdomain.NewApprovalReference, "SYN-APPROVAL-"+label),
		mustBuild(t, pcdomain.NewCommercialSourceReference, "SYN-SOURCE"),
		startsAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	version, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID:      mustBuild(t, pcdomain.NewTenantID, closureTenant),
		Kind:          pcdomain.SupplierAgreementObject,
		ObjectID:      mustBuild(t, pcdomain.NewCommercialObjectID, closureObject),
		Version:       mustBuild(t, pcdomain.NewCommercialVersionLabel, label),
		Scope:         mustBuild(t, pcdomain.NewCommercialScopeReference, "SYN-SCOPE"),
		ContentDigest: mustBuild(t, pcdomain.NewCommercialContentDigest, "sha256:closure-"+label),
		Effective:     interval,
		Status:        pcdomain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   startsAt,
		EffectiveAt:   startsAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	agreement, err := pcdomain.NewSupplierAgreement(version,
		mustBuild(t, pcdomain.NewPartyID, closureProvider),
		mustBuild(t, pcdomain.NewLegalEntityReference, "SYN-LE-01"),
		mustBuild(t, pcdomain.NewCommercialScopeReference, "SYN-SCOPE"),
		mustBuild(t, pcdomain.NewPricingPlanReference, "SYN-PLAN-BUY-01"),
		interval)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if _, err := publications.SaveVersion(txCtx, version); err != nil {
			return err
		}
		_, err := publications.SaveSupplierAgreement(txCtx, agreement)
		return err
	}); err != nil {
		t.Fatalf("落协议 %s：%v", label, err)
	}
	return version
}
