package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件对真实 PostgreSQL 16 证包裹→当前已接受委托反查（ADR-0060）：投影与快照同写、
// 只认已接受、零/一/多三格、历史成员与他租户不命中。

func TestInsertWritesCurrentParcelProjectionFromTheSnapshot(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	original := submittedShipmentRequest(t, "req-key-1", "request-1")
	mustInsert(t, transactor, t.Context(), repository, original)

	versionID, parcels := readProjection(t, pool, "req-key-1")
	if versionID != original.CurrentSubmissionVersion().VersionID().String() {
		t.Fatalf("current_submission_version_id = %q", versionID)
	}
	if strings.Join(parcels, ",") != "parcel-1,parcel-2" {
		t.Fatalf("declared_parcel_ids = %v", parcels)
	}

	// 回填表达式必须能从 snapshot 复原这两列——迁移 0006 的 UPDATE 用的就是这条。
	if _, err := pool.Exec(t.Context(),
		`UPDATE parcel_shipment.shipment_request
		    SET current_submission_version_id = 'stale',
		        declared_parcel_ids = ARRAY['stale']
		  WHERE source_request_key = 'req-key-1'`); err != nil {
		t.Fatalf("弄脏投影：%v", err)
	}
	if _, err := pool.Exec(t.Context(),
		`UPDATE parcel_shipment.shipment_request
		    SET current_submission_version_id = snapshot #>> '{currentVersion,versionId}',
		        declared_parcel_ids = ARRAY(
		            SELECT jsonb_array_elements_text(
		                COALESCE(snapshot #> '{currentVersion,declaredParcelIds}', '[]'::jsonb)
		            )
		        )
		  WHERE source_request_key = 'req-key-1'`); err != nil {
		t.Fatalf("按快照回填：%v", err)
	}
	restoredID, restoredParcels := readProjection(t, pool, "req-key-1")
	if restoredID != versionID || strings.Join(restoredParcels, ",") != strings.Join(parcels, ",") {
		t.Fatalf("回填后 %q %v，要 %q %v", restoredID, restoredParcels, versionID, parcels)
	}
}

func TestSaveAdvancesTheParcelProjectionWithTheSnapshot(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	ctx := t.Context()
	original := submittedShipmentRequest(t, "req-key-1", "request-1")
	mustInsert(t, transactor, ctx, repository, original)

	loaded, _, err := repository.FindBySourceIdentity(ctx, requestIdentity(t, "req-key-1"))
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	superseded, err := loaded.FormNewSubmissionVersion(domain.NewSubmissionVersionSpec{
		VersionID:        mustBuild(t, domain.NewSubmissionVersionID, "version-2"),
		TaskID:           mustBuild(t, domain.NewAcceptanceDecisionTaskID, "task-2"),
		SourceSubmission: requestFingerprint(t, "req-key-1-supplement", "digest-2"),
		DeclaredParcelIDs: []domain.DeclaredParcelID{
			mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"),
			mustBuild(t, domain.NewDeclaredParcelID, "parcel-2"),
		},
		EstablishedAt: submittedAtFixture.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("形成新提交版本：%v", err)
	}
	var saved ports.ShipmentRequestSaveOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var saveErr error
		saved, saveErr = repository.Save(txCtx, requestIdentity(t, "req-key-1"), superseded)
		return saveErr
	})
	if saved != ports.ShipmentRequestSaved {
		t.Fatalf("save outcome = %s", saved)
	}

	versionID, parcels := readProjection(t, pool, "req-key-1")
	if versionID != "version-2" {
		t.Fatalf("换代后投影仍是 %q", versionID)
	}
	if strings.Join(parcels, ",") != "parcel-1,parcel-2" {
		t.Fatalf("换代后成员投影 = %v", parcels)
	}
}

func TestCurrentAcceptedParcelIsFoundByTenantAndParcel(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	mustInsert(t, transactor, t.Context(), repository, submittedShipmentRequest(t, "req-key-1", "request-1"))
	markRequestState(t, pool, "req-key-1", domain.ShipmentRequestAccepted)

	target, found, err := repository.FindCurrentAcceptedByParcel(
		t.Context(), mustBuild(t, domain.NewTenantID, "tenant-1"),
		mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"),
	)
	if err != nil || !found {
		t.Fatalf("found = %v err = %v", found, err)
	}
	if target.ShipmentRequestID().String() != "request-1" ||
		target.SubmissionVersion().String() != "version-1" ||
		target.Identity() != requestIdentity(t, "req-key-1") {
		t.Fatalf("目标 = %+v", target)
	}
}

func TestOnlyCurrentAcceptedRequestsAreParcelTargets(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	cases := []struct {
		key   string
		id    string
		state domain.ShipmentRequestState
	}{
		{"submitted", "request-s", domain.ShipmentRequestSubmitted},
		{"rejected", "request-r", domain.ShipmentRequestRejected},
		{"withdrawn", "request-w", domain.ShipmentRequestWithdrawn},
	}
	for _, item := range cases {
		mustInsert(t, transactor, t.Context(), repository, submittedShipmentRequest(t, item.key, item.id))
		if item.state != domain.ShipmentRequestSubmitted {
			markRequestState(t, pool, item.key, item.state)
		}
		_, found, err := repository.FindCurrentAcceptedByParcel(
			t.Context(), mustBuild(t, domain.NewTenantID, "tenant-1"),
			mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"),
		)
		if err != nil || found {
			t.Fatalf("%s 被当成当前已接受目标：found = %v err = %v", item.state, found, err)
		}
	}
}

func TestLookupUsesCurrentProjectionNotSnapshotMembers(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	mustInsert(t, transactor, t.Context(), repository, submittedShipmentRequest(t, "req-key-1", "request-1"))
	markRequestState(t, pool, "req-key-1", domain.ShipmentRequestAccepted)
	if _, err := pool.Exec(t.Context(),
		`UPDATE parcel_shipment.shipment_request
		    SET declared_parcel_ids = ARRAY['parcel-1']
		  WHERE source_request_key = 'req-key-1'`); err != nil {
		t.Fatalf("收窄投影：%v", err)
	}
	var snapshotMembers []string
	if err := pool.QueryRow(t.Context(),
		`SELECT ARRAY(SELECT jsonb_array_elements_text(snapshot #> '{currentVersion,declaredParcelIds}'))
		   FROM parcel_shipment.shipment_request
		  WHERE source_request_key = 'req-key-1'`,
	).Scan(&snapshotMembers); err != nil {
		t.Fatalf("读快照成员：%v", err)
	}
	if strings.Join(snapshotMembers, ",") != "parcel-1,parcel-2" {
		t.Fatalf("夹具被破坏：快照成员 = %v", snapshotMembers)
	}

	if _, found, err := repository.FindCurrentAcceptedByParcel(
		t.Context(), mustBuild(t, domain.NewTenantID, "tenant-1"),
		mustBuild(t, domain.NewDeclaredParcelID, "parcel-2"),
	); err != nil || found {
		t.Fatalf("快照里仍有的旧成员命中了投影：found = %v err = %v", found, err)
	}
	if _, found, err := repository.FindCurrentAcceptedByParcel(
		t.Context(), mustBuild(t, domain.NewTenantID, "tenant-1"),
		mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"),
	); err != nil || !found {
		t.Fatalf("当前投影成员没命中：found = %v err = %v", found, err)
	}
}

func TestAcceptedParcelTargetsAreIsolatedByTenant(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	mustInsert(t, transactor, t.Context(), repository, submittedShipmentRequest(t, "req-key-1", "request-1"))
	markRequestState(t, pool, "req-key-1", domain.ShipmentRequestAccepted)

	_, found, err := repository.FindCurrentAcceptedByParcel(
		t.Context(), mustBuild(t, domain.NewTenantID, "tenant-b"),
		mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"),
	)
	if err != nil || found {
		t.Fatalf("他租户读到了本租户的已接受目标：found = %v err = %v", found, err)
	}
}

func TestTwoAcceptedRequestsForTheSameParcelAreAmbiguous(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	mustInsert(t, transactor, t.Context(), repository, submittedShipmentRequest(t, "req-key-1", "request-1"))
	mustInsert(t, transactor, t.Context(), repository, submittedShipmentRequest(t, "req-key-2", "request-2"))
	markRequestState(t, pool, "req-key-1", domain.ShipmentRequestAccepted)
	markRequestState(t, pool, "req-key-2", domain.ShipmentRequestAccepted)

	got, found, err := repository.FindCurrentAcceptedByParcel(
		t.Context(), mustBuild(t, domain.NewTenantID, "tenant-1"),
		mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"),
	)
	if !errors.Is(err, domain.ErrAmbiguousParcelTarget) || found {
		t.Fatalf("found = %v err = %v；两份已接受应收歧义，不得任选", found, err)
	}
	if got.ShipmentRequestID().String() != "" {
		t.Fatalf("歧义时交回了委托 %q", got.ShipmentRequestID())
	}
}

func TestParcelProjectionCheckRejectsEmptyOrBlankMembers(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	mustInsert(t, transactor, t.Context(), repository, submittedShipmentRequest(t, "req-key-1", "request-1"))

	if _, err := pool.Exec(t.Context(),
		`UPDATE parcel_shipment.shipment_request
		    SET declared_parcel_ids = ARRAY[]::text[]
		  WHERE source_request_key = 'req-key-1'`); err == nil {
		t.Fatal("空成员数组进了投影列")
	}
	if _, err := pool.Exec(t.Context(),
		`UPDATE parcel_shipment.shipment_request
		    SET declared_parcel_ids = ARRAY['parcel-1', '  ']
		  WHERE source_request_key = 'req-key-1'`); err == nil {
		t.Fatal("空白成员进了投影列")
	}
	if _, err := pool.Exec(t.Context(),
		`UPDATE parcel_shipment.shipment_request
		    SET current_submission_version_id = '  '
		  WHERE source_request_key = 'req-key-1'`); err == nil {
		t.Fatal("空白当前版本号进了投影列")
	}
}

func TestAcceptedParcelIndexIsAPartialGinOnDeclaredParcels(t *testing.T) {
	_, _, pool := newShipmentRequests(t)
	var definition string
	if err := pool.QueryRow(t.Context(),
		`SELECT indexdef FROM pg_indexes WHERE indexname = 'shipment_request_accepted_parcels_gin'`,
	).Scan(&definition); err != nil {
		t.Fatalf("读索引定义：%v", err)
	}
	if !strings.Contains(strings.ToUpper(definition), "GIN") ||
		!strings.Contains(definition, "declared_parcel_ids") ||
		!strings.Contains(definition, "state") {
		t.Fatalf("索引不是按已接受成员建的部分 GIN：%s", definition)
	}
}

func markRequestState(t *testing.T, pool *pgxpool.Pool, key string, state domain.ShipmentRequestState) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`UPDATE parcel_shipment.shipment_request SET state = $1 WHERE source_request_key = $2`,
		uint8(state), key); err != nil {
		t.Fatalf("推进状态列：%v", err)
	}
}

func readProjection(t *testing.T, pool *pgxpool.Pool, key string) (string, []string) {
	t.Helper()
	var versionID string
	var parcels []string
	if err := pool.QueryRow(t.Context(),
		`SELECT current_submission_version_id, declared_parcel_ids
		   FROM parcel_shipment.shipment_request
		  WHERE source_request_key = $1`, key,
	).Scan(&versionID, &parcels); err != nil {
		t.Fatalf("读投影列：%v", err)
	}
	return versionID, parcels
}
