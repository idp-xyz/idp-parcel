package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var disclosureBaseAt = time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

type disclosureFixture struct {
	pool *pgxpool.Pool
	db   *bentopg.DB
}

func newDisclosureFixture(t *testing.T) *disclosureFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return &disclosureFixture{pool: pool, db: db}
}

func (fixture *disclosureFixture) viewFor(t *testing.T, tenant string) *adapter.DisclosurePolicies {
	t.Helper()
	var tenantID domain.TenantID
	if tenant != "" {
		tenantID = projectionValue(t, domain.NewTenantID, tenant)
	}
	view, err := adapter.NewDisclosurePolicies(fixture.db, tenantID)
	if err != nil {
		t.Fatalf("构造披露策略视图：%v", err)
	}
	return view
}

// 目录内容属实例半边，尚无登记入口，因此用例直接写行——证的是视图读得对。
func (fixture *disclosureFixture) publish(t *testing.T, tenant, version string, from time.Time, to *time.Time) {
	t.Helper()
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.disclosure_policy_version
			(tenant_id, policy_version, effective_from, effective_to, approved_by)
		 VALUES ($1, $2, $3, $4, 'customer-service')`,
		tenant, version, from, to); err != nil {
		t.Fatalf("发布披露策略版本 %s：%v", version, err)
	}
}

func (fixture *disclosureFixture) addEntry(
	t *testing.T,
	tenant, version, customer string,
	milestones, eta, final, note ports.DimensionDisclosure,
) {
	t.Helper()
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.disclosure_policy_entry
			(tenant_id, policy_version, customer_account_ref,
			 milestones_state, eta_state, final_state, note_state,
			 milestones_content, eta_content, final_content, note_content)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		tenant, version, customer,
		milestones.State.String(), eta.State.String(), final.State.String(), note.State.String(),
		nullableContent(milestones), nullableContent(eta), nullableContent(final), nullableContent(note),
	); err != nil {
		t.Fatalf("写入披露条目 %s：%v", customer, err)
	}
}

func nullableContent(disclosure ports.DimensionDisclosure) *string {
	if disclosure.State != domain.DimensionShown {
		return nil
	}
	value := disclosure.Content.String()
	return &value
}

func shownDimension(t *testing.T, raw string) ports.DimensionDisclosure {
	t.Helper()
	return ports.DimensionDisclosure{
		State:   domain.DimensionShown,
		Content: projectionValue(t, domain.NewViewContentReference, raw),
	}
}

func pendingDimension() ports.DimensionDisclosure {
	return ports.DimensionDisclosure{State: domain.DimensionPendingConfirmation}
}

func withheldDimension() ports.DimensionDisclosure {
	return ports.DimensionDisclosure{State: domain.DimensionNotDisclosed}
}

func disclosureCustomer(t *testing.T, raw string) domain.CustomerAccountReference {
	t.Helper()
	return projectionValue(t, domain.NewCustomerAccountReference, raw)
}

func disclosureProjection(t *testing.T, derivedAt time.Time) domain.TrackingProjection {
	t.Helper()
	projection, err := domain.DeriveTrackingProjection(
		projectionValue(t, domain.NewProjectionVersionID, "projection-1"),
		projectionValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		[]domain.MilestoneClassification{classifiedEntry(t, "fact-1", "PICKED_UP")},
		derivedAt,
	)
	if err != nil {
		t.Fatalf("构造投影：%v", err)
	}
	return projection
}

func assertDimension(t *testing.T, name string, got, want ports.DimensionDisclosure) {
	t.Helper()
	if got.State != want.State {
		t.Fatalf("%s 状态 = %s，要 %s", name, got.State, want.State)
	}
	gotContent, gotShown := got.Content.String(), got.State == domain.DimensionShown
	wantContent, wantShown := want.Content.String(), want.State == domain.DimensionShown
	if gotShown != wantShown || gotContent != wantContent {
		t.Fatalf("%s 内容 = %q shown=%v，要 %q shown=%v", name, gotContent, gotShown, wantContent, wantShown)
	}
}

// 空目录是首发唯一走得到的真实分支：没有租户就没有已发布规则。它必须与「不展示」
// 分得开——后者是一次已经作出的披露决定，前者是还没人作过。
func TestMissingDisclosureCatalogIsNotConfiguredRatherThanWithheld(t *testing.T) {
	fixture := newDisclosureFixture(t)
	customer := disclosureCustomer(t, "customer-1")
	projection := disclosureProjection(t, disclosureBaseAt)

	for _, tenant := range []string{"tenant-a", ""} {
		answer, configured, err := fixture.viewFor(t, tenant).
			AssessDisclosure(t.Context(), customer, projection)
		if err != nil || configured {
			t.Fatalf("租户 %q：err=%v configured=%v", tenant, err, configured)
		}
		if answer.Milestones.State == domain.DimensionNotDisclosed ||
			answer.ETA.State == domain.DimensionNotDisclosed ||
			answer.Final.State == domain.DimensionNotDisclosed ||
			answer.Note.State == domain.DimensionNotDisclosed {
			t.Fatalf("租户 %q：未配置被翻成了不展示", tenant)
		}
	}
}

func TestDisclosureCatalogAnswersFourDimensionsIndependently(t *testing.T) {
	fixture := newDisclosureFixture(t)
	fixture.publish(t, "tenant-a", "disclose/v1", disclosureBaseAt.Add(-time.Hour), nil)
	wantMilestones := shownDimension(t, "milestones/v1")
	wantETA := pendingDimension()
	wantFinal := withheldDimension()
	wantNote := shownDimension(t, "note/v1")
	fixture.addEntry(t, "tenant-a", "disclose/v1", "customer-1",
		wantMilestones, wantETA, wantFinal, wantNote)

	answer, configured, err := fixture.viewFor(t, "tenant-a").
		AssessDisclosure(t.Context(), disclosureCustomer(t, "customer-1"), disclosureProjection(t, disclosureBaseAt))
	if err != nil || !configured {
		t.Fatalf("命中：err=%v configured=%v", err, configured)
	}
	assertDimension(t, "milestones", answer.Milestones, wantMilestones)
	assertDimension(t, "eta", answer.ETA, wantETA)
	assertDimension(t, "final", answer.Final, wantFinal)
	assertDimension(t, "note", answer.Note, wantNote)
}

// 目录已发布但这个客户没有条目：没有默认披露值可发明，不能读成「什么都不给看」。
func TestPublishedCatalogWithoutThisCustomerIsNotConfigured(t *testing.T) {
	fixture := newDisclosureFixture(t)
	fixture.publish(t, "tenant-a", "disclose/v1", disclosureBaseAt.Add(-time.Hour), nil)
	fixture.addEntry(t, "tenant-a", "disclose/v1", "customer-1",
		shownDimension(t, "milestones/v1"), pendingDimension(), withheldDimension(), pendingDimension())

	_, configured, err := fixture.viewFor(t, "tenant-a").
		AssessDisclosure(t.Context(), disclosureCustomer(t, "customer-2"), disclosureProjection(t, disclosureBaseAt))
	if err != nil || configured {
		t.Fatalf("他户：err=%v configured=%v", err, configured)
	}
}

func TestDisclosurePoliciesOfAnotherTenantAreInvisible(t *testing.T) {
	fixture := newDisclosureFixture(t)
	fixture.publish(t, "tenant-a", "disclose/v1", disclosureBaseAt.Add(-time.Hour), nil)
	fixture.addEntry(t, "tenant-a", "disclose/v1", "customer-1",
		shownDimension(t, "milestones/v1"), pendingDimension(), pendingDimension(), pendingDimension())

	_, configured, err := fixture.viewFor(t, "tenant-b").
		AssessDisclosure(t.Context(), disclosureCustomer(t, "customer-1"), disclosureProjection(t, disclosureBaseAt))
	if err != nil || configured {
		t.Fatalf("跨租户目录可见：err=%v configured=%v", err, configured)
	}
}

func TestIncompleteDisclosureQueryIsNotAnswered(t *testing.T) {
	fixture := newDisclosureFixture(t)
	fixture.publish(t, "tenant-a", "disclose/v1", disclosureBaseAt.Add(-time.Hour), nil)
	fixture.addEntry(t, "tenant-a", "disclose/v1", "customer-1",
		shownDimension(t, "milestones/v1"), pendingDimension(), pendingDimension(), pendingDimension())
	view := fixture.viewFor(t, "tenant-a")
	projection := disclosureProjection(t, disclosureBaseAt)

	if _, configured, err := view.AssessDisclosure(t.Context(), domain.CustomerAccountReference{}, projection); err != nil || configured {
		t.Fatalf("缺账户拿到了答复：err=%v configured=%v", err, configured)
	}
	if _, configured, err := view.AssessDisclosure(t.Context(), disclosureCustomer(t, "customer-1"), domain.TrackingProjection{}); err != nil || configured {
		t.Fatalf("零值投影拿到了答复：err=%v configured=%v", err, configured)
	}
}

// 适用版本按投影派生时点选：一份在换版前派生的投影仍按旧规则判。
func TestDisclosureVersionIsChosenByProjectionDerivedAt(t *testing.T) {
	fixture := newDisclosureFixture(t)
	switchover := disclosureBaseAt
	fixture.publish(t, "tenant-a", "disclose/v1", switchover.Add(-30*24*time.Hour), &switchover)
	fixture.publish(t, "tenant-a", "disclose/v2", switchover, nil)
	fixture.addEntry(t, "tenant-a", "disclose/v1", "customer-1",
		shownDimension(t, "milestones/old"), pendingDimension(), pendingDimension(), pendingDimension())
	fixture.addEntry(t, "tenant-a", "disclose/v2", "customer-1",
		shownDimension(t, "milestones/new"), pendingDimension(), pendingDimension(), pendingDimension())
	view := fixture.viewFor(t, "tenant-a")

	old, _, err := view.AssessDisclosure(t.Context(),
		disclosureCustomer(t, "customer-1"), disclosureProjection(t, switchover.Add(-time.Hour)))
	if err != nil || old.Milestones.Content.String() != "milestones/old" {
		t.Fatalf("旧投影按 %s 判（err=%v）", old.Milestones.Content, err)
	}
	fresh, _, err := view.AssessDisclosure(t.Context(),
		disclosureCustomer(t, "customer-1"), disclosureProjection(t, switchover.Add(time.Hour)))
	if err != nil || fresh.Milestones.Content.String() != "milestones/new" {
		t.Fatalf("新投影按 %s 判（err=%v）", fresh.Milestones.Content, err)
	}
}

func TestOverlappingDisclosureVersionsAreRefusedNotRanked(t *testing.T) {
	fixture := newDisclosureFixture(t)
	closed := disclosureBaseAt.Add(60 * 24 * time.Hour)
	fixture.publish(t, "tenant-a", "disclose/v1", disclosureBaseAt.Add(-30*24*time.Hour), &closed)
	fixture.publish(t, "tenant-a", "disclose/v2", disclosureBaseAt.Add(-10*24*time.Hour), &closed)

	_, configured, err := fixture.viewFor(t, "tenant-a").
		AssessDisclosure(t.Context(), disclosureCustomer(t, "customer-1"), disclosureProjection(t, disclosureBaseAt))
	if !errors.Is(err, adapter.ErrAmbiguousCatalog) {
		t.Fatalf("重叠版本没有报冲突：err=%v configured=%v", err, configured)
	}
}

func TestSecondOpenDisclosureVersionIsRejectedByTheIndex(t *testing.T) {
	fixture := newDisclosureFixture(t)
	fixture.publish(t, "tenant-a", "disclose/v1", disclosureBaseAt, nil)
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.disclosure_policy_version
			(tenant_id, policy_version, effective_from, effective_to, approved_by)
		 VALUES ('tenant-a', 'disclose/v2', $1, NULL, 'customer-service')`, disclosureBaseAt); err == nil {
		t.Fatal("库接受了同租户第二份未闭区间的披露策略版本")
	}
	fixture.publish(t, "tenant-b", "disclose/v1", disclosureBaseAt, nil)
}

func TestDisclosureEntryShapeIsPinnedByChecks(t *testing.T) {
	fixture := newDisclosureFixture(t)
	fixture.publish(t, "tenant-a", "disclose/v1", disclosureBaseAt, nil)

	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.disclosure_policy_entry
			(tenant_id, policy_version, customer_account_ref,
			 milestones_state, eta_state, final_state, note_state,
			 milestones_content, eta_content, final_content, note_content)
		 VALUES ('tenant-a', 'disclose/v1', 'customer-1',
		         'SHOWN', 'PENDING_CONFIRMATION', 'NOT_DISCLOSED', 'PENDING_CONFIRMATION',
		         NULL, NULL, NULL, NULL)`); err == nil {
		t.Fatal("库接受了展示维缺少内容来处的条目")
	}
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.disclosure_policy_entry
			(tenant_id, policy_version, customer_account_ref,
			 milestones_state, eta_state, final_state, note_state,
			 milestones_content, eta_content, final_content, note_content)
		 VALUES ('tenant-a', 'disclose/v1', 'customer-1',
		         'PENDING_CONFIRMATION', 'PENDING_CONFIRMATION', 'NOT_DISCLOSED', 'PENDING_CONFIRMATION',
		         'milestones/v1', NULL, NULL, NULL)`); err == nil {
		t.Fatal("库接受了待确认维携带内容来处的条目")
	}
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.disclosure_policy_entry
			(tenant_id, policy_version, customer_account_ref,
			 milestones_state, eta_state, final_state, note_state,
			 milestones_content, eta_content, final_content, note_content)
		 VALUES ('tenant-a', 'disclose/v1', 'customer-1',
		         'VISIBLE', 'PENDING_CONFIRMATION', 'NOT_DISCLOSED', 'PENDING_CONFIRMATION',
		         'milestones/v1', NULL, NULL, NULL)`); err == nil {
		t.Fatal("库接受了三态之外的展示维状态")
	}
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.disclosure_policy_entry
			(tenant_id, policy_version, customer_account_ref,
			 milestones_state, eta_state, final_state, note_state,
			 milestones_content, eta_content, final_content, note_content)
		 VALUES ('tenant-a', 'disclose/v9', 'customer-1',
		         'PENDING_CONFIRMATION', 'PENDING_CONFIRMATION', 'PENDING_CONFIRMATION', 'PENDING_CONFIRMATION',
		         NULL, NULL, NULL, NULL)`); err == nil {
		t.Fatal("库接受了挂在未发布版本下的披露条目")
	}
}
