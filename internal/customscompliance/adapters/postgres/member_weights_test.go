package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

func TestMemberWeightsRoundTripThroughBothReadsAndTheCatalogue(t *testing.T) {
	fixture := newDeclarationFixture(t)
	ctx := t.Context()
	record := weightedSubmission(t, "tenant-w", "unit-w", "IMPORT_STANDARD", "version-w")
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.submissions.Save(txCtx, record)
		return err
	})

	byKey, found, err := fixture.submissions.FindByKey(ctx, record.Key)
	if err != nil || !found {
		t.Fatalf("按键读回：%v found=%v", err, found)
	}
	byVersion, found, err := fixture.submissions.FindByVersion(ctx, record.Key.TenantID, record.Version.ID())
	if err != nil || !found {
		t.Fatalf("按版本读回：%v found=%v", err, found)
	}
	wantAt := declarationFixedAt.Add(time.Minute)
	for _, got := range [][]domain.MemberMeasuredWeight{byKey.Version.MemberWeights(), byVersion.Version.MemberWeights()} {
		if len(got) != 2 || got[0].Parcel().String() != "parcel-1" || got[0].GrossWeight() != "2.1" ||
			got[0].GrossWeightUnit() != "KG" || got[0].MeasurementSource() != "SYN-WEIGH-1" || !got[0].MeasuredAt().Equal(wantAt) ||
			got[1].Parcel().String() != "parcel-2" || got[1].GrossWeightUnit() != "G" || got[1].MeasurementSource() != "SYN-WEIGH-2" {
			t.Fatalf("读回 = %+v", got)
		}
	}

	db, err := bentopg.NewDB(fixture.pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	views, err := adapter.NewOperationsRecordViews(db)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := views.ListSubmissionVersions(ctx, record.Key.TenantID, 10, firstPage(t, ports.SubmissionVersionPages))
	if err != nil || len(listed.Rows) != 1 || len(listed.Rows[0].MemberWeights) != 2 ||
		listed.Rows[0].MemberWeights[0].MeasurementSource != "SYN-WEIGH-1" ||
		listed.Rows[0].MemberWeights[1].GrossWeightUnit != "G" {
		t.Fatalf("查阅 = %v %+v", err, listed.Rows)
	}
}

func TestARowFixedBeforeMemberWeightsReadsAsNone(t *testing.T) {
	fixture := newDeclarationFixture(t)
	ctx := t.Context()
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO customs_compliance.declaration_submission
			(tenant_id, unit_id, procedure_ref, version_id, content_digest, members, dossier_ref, roles_ref,
			 readiness_basis, authority_ref, fixed_at, recorded_at, is_current)
		 VALUES ('tenant-old', 'unit-old', 'IMPORT_STANDARD', 'version-old', 'digest-old', '["parcel-1"]',
		         'dossier-old', 'roles-old', 'basis-old', 'authority-old', $1, $1, true)`,
		declarationFixedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO customs_compliance.submission_attempt
			(tenant_id, version_id, sequence, target, result, sent_at)
		 VALUES ('tenant-old', 'version-old', 1, 'customs-gateway/prod', 'PENDING_CONFIRMATION', $1)`,
		declarationFixedAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	tenant := declarationValue(t, domain.NewTenantID, "tenant-old")
	version := declarationValue(t, domain.NewSubmissionVersionID, "version-old")
	found, exists, err := fixture.submissions.FindByVersion(ctx, tenant, version)
	if err != nil || !exists || len(found.Version.MemberWeights()) != 0 {
		t.Fatalf("迁移前的行 = %v exists=%v", err, exists)
	}
}

func TestMemberWeightsRejectANonArray(t *testing.T) {
	fixture := newDeclarationFixture(t)
	_, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO customs_compliance.declaration_submission
			(tenant_id, unit_id, procedure_ref, version_id, content_digest, members, member_weights, dossier_ref, roles_ref,
			 readiness_basis, authority_ref, fixed_at, recorded_at, is_current)
		 VALUES ('tenant-bad', 'unit-bad', 'IMPORT_STANDARD', 'version-bad', 'digest-bad', '["parcel-1"]', '{"parcel":"parcel-1"}',
		         'dossier-bad', 'roles-bad', 'basis-bad', 'authority-bad', $1, $1, true)`,
		declarationFixedAt)
	if err == nil || !strings.Contains(err.Error(), "declaration_submission_member_weights_array") {
		t.Fatalf("非数组 = %v", err)
	}
}

func weightedSubmission(t *testing.T, tenant, unit, procedure, version string) ports.DeclarationSubmissionRecord {
	t.Helper()
	record := submissionRecord(t, tenant, unit, procedure, version)
	snapshot := record.Version.Snapshot()
	formed, err := domain.FormDeclarationUnit(
		snapshot.Unit,
		declarationValue(t, domain.NewCustomsCaseID, "case-1"),
		record.Key.Procedure,
		snapshot.Members,
	)
	if err != nil {
		t.Fatal(err)
	}
	readiness, err := domain.JudgeReady(snapshot.Unit, snapshot.Basis, declarationFixedAt.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := domain.GrantSubmissionAuthority(snapshot.Unit, snapshot.Authority, declarationFixedAt.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	fixed, err := domain.FixSubmissionVersion(domain.CustomsSubmissionVersionSpec{
		ID: snapshot.ID, Unit: formed, Dossier: snapshot.Dossier, Roles: snapshot.Roles,
		Readiness: readiness, Authorization: authorization, FixedAt: snapshot.FixedAt,
		MemberWeights: []domain.MemberMeasuredWeight{
			memberWeight(t, "parcel-2", "4", "G", "SYN-WEIGH-2", declarationFixedAt),
			memberWeight(t, "parcel-1", "2.1", "KG", "SYN-WEIGH-1", declarationFixedAt.Add(time.Minute)),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := domain.InitialAttempt(fixed, "customs-gateway/prod", domain.AttemptPendingConfirmation, declarationFixedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	record.Version = fixed
	record.Attempt = attempt
	return record
}

func memberWeight(t *testing.T, parcel, gross, unit, source string, at time.Time) domain.MemberMeasuredWeight {
	t.Helper()
	weight, err := domain.NewMemberMeasuredWeight(declarationValue(t, domain.NewDeclaredParcelReference, parcel), gross, unit, source, at)
	if err != nil {
		t.Fatal(err)
	}
	return weight
}
