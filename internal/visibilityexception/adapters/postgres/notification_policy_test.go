package postgres_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

var policyBaseAt = time.Date(2026, 6, 5, 11, 0, 0, 0, time.UTC)

type policyFixture struct {
	pool *pgxpool.Pool
	db   *bentopg.DB
}

func newPolicyFixture(t *testing.T) *policyFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return &policyFixture{pool: pool, db: db}
}

func (fixture *policyFixture) viewFor(t *testing.T, tenant string) *adapter.NotificationPolicies {
	t.Helper()
	var tenantID domain.TenantID
	if tenant != "" {
		tenantID = projectionValue(t, domain.NewTenantID, tenant)
	}
	view, err := adapter.NewNotificationPolicies(fixture.db, tenantID)
	if err != nil {
		t.Fatalf("构造通知策略视图：%v", err)
	}
	return view
}

// 目录内容属实例半边，尚无登记入口，用例直接写行。
func (fixture *policyFixture) register(t *testing.T, tenant, policyRef, channel, after, obligation string) {
	t.Helper()
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.notification_policy
			(tenant_id, disclosure_policy_ref, channel_ref, deadline_after, obligation_ref)
		 VALUES ($1, $2, $3, $4::interval, $5)`,
		tenant, policyRef, channel, after, obligation); err != nil {
		t.Fatalf("登记通知策略 %s：%v", policyRef, err)
	}
}

func disclosedDecision(t *testing.T, policyRef string, decidedAt time.Time) domain.DisclosureDecision {
	t.Helper()
	decision, err := domain.DecideDisclosure(
		projectionValue(t, domain.NewEpisodeID, "episode-1"),
		projectionValue(t, domain.NewCustomerAccountReference, "customer-1"),
		projectionValue(t, domain.NewDisclosurePolicyReference, policyRef),
		domain.DiscloseToCustomer,
		projectionValue(t, domain.NewDisclosureContentReference, "content/v1"),
		decidedAt,
	)
	if err != nil {
		t.Fatalf("构造披露决定：%v", err)
	}
	return decision
}

// 没有渠道的通知不存在一个如实的空白格：查不到就是未配置，编排停在未决等登记，
// 绝不造一个占位渠道。
func TestMissingNotificationPolicyIsNotConfigured(t *testing.T) {
	fixture := newPolicyFixture(t)
	fixture.register(t, "tenant-a", "disclosure/v1", "SMS", "4 hours", "DELIVERED")
	decision := disclosedDecision(t, "disclosure/v9", policyBaseAt)

	// 租户已登记但这条策略没有行。
	if _, configured, err := fixture.viewFor(t, "tenant-a").
		DirectNotification(t.Context(), decision); err != nil || configured {
		t.Fatalf("未登记策略拿到了指令：err=%v configured=%v", err, configured)
	}
	// 租户未登记：今天没有租户，这是首发唯一走得到的真实分支。
	if _, configured, err := fixture.viewFor(t, "").
		DirectNotification(t.Context(), disclosedDecision(t, "disclosure/v1", policyBaseAt)); err != nil || configured {
		t.Fatalf("未登记租户拿到了指令：err=%v configured=%v", err, configured)
	}
	// 跨租户不可见（ADR-0003）。
	if _, configured, err := fixture.viewFor(t, "tenant-b").
		DirectNotification(t.Context(), disclosedDecision(t, "disclosure/v1", policyBaseAt)); err != nil || configured {
		t.Fatalf("跨租户策略可见：err=%v configured=%v", err, configured)
	}
}

func TestNotificationDirectiveCarriesChannelDeadlineAndObligation(t *testing.T) {
	fixture := newPolicyFixture(t)
	fixture.register(t, "tenant-a", "disclosure/v1", "SMS", "4 hours", "DELIVERED")

	directive, configured, err := fixture.viewFor(t, "tenant-a").
		DirectNotification(t.Context(), disclosedDecision(t, "disclosure/v1", policyBaseAt))
	if err != nil || !configured {
		t.Fatalf("命中：err=%v configured=%v", err, configured)
	}
	if directive.Channel.String() != "SMS" {
		t.Fatalf("渠道 = %s", directive.Channel)
	}
	if directive.Obligation.String() != "DELIVERED" {
		t.Fatalf("义务判据 = %s", directive.Obligation)
	}
	if !directive.Deadline.Equal(policyBaseAt.Add(4 * time.Hour)) {
		t.Fatalf("截止时间 = %s，应为披露决定时间加 4 小时", directive.Deadline)
	}
}

// 时限是相对量：两份披露决定时间不同，截止时间各自算，不共用一个钉死的截止点。
func TestDeadlineIsRelativeToEachDisclosureDecision(t *testing.T) {
	fixture := newPolicyFixture(t)
	fixture.register(t, "tenant-a", "disclosure/v1", "EMAIL", "2 hours", "CHANNEL_ACCEPTED")
	view := fixture.viewFor(t, "tenant-a")

	early, _, err := view.DirectNotification(t.Context(), disclosedDecision(t, "disclosure/v1", policyBaseAt))
	if err != nil {
		t.Fatalf("首份：%v", err)
	}
	late, _, err := view.DirectNotification(t.Context(),
		disclosedDecision(t, "disclosure/v1", policyBaseAt.Add(72*time.Hour)))
	if err != nil {
		t.Fatalf("次份：%v", err)
	}
	if late.Deadline.Sub(early.Deadline) != 72*time.Hour {
		t.Fatalf("两份截止时间相差 %s，应与两份披露决定时间相差一致", late.Deadline.Sub(early.Deadline))
	}
}

// 同一策略换版是换引用值：新旧两行并存，各自被自己那批披露决定命中，历史通知
// 不因换版被改判。
func TestPolicyVersionsCoexistAndDoNotOverwriteEachOther(t *testing.T) {
	fixture := newPolicyFixture(t)
	fixture.register(t, "tenant-a", "disclosure/v1", "SMS", "4 hours", "DELIVERED")
	fixture.register(t, "tenant-a", "disclosure/v2", "EMAIL", "1 hour", "CUSTOMER_CONFIRMED")
	view := fixture.viewFor(t, "tenant-a")

	old, _, err := view.DirectNotification(t.Context(), disclosedDecision(t, "disclosure/v1", policyBaseAt))
	if err != nil || old.Channel.String() != "SMS" || !old.Deadline.Equal(policyBaseAt.Add(4*time.Hour)) {
		t.Fatalf("旧版 = %s / %s（err=%v）", old.Channel, old.Deadline, err)
	}
	fresh, _, err := view.DirectNotification(t.Context(), disclosedDecision(t, "disclosure/v2", policyBaseAt))
	if err != nil || fresh.Channel.String() != "EMAIL" || !fresh.Deadline.Equal(policyBaseAt.Add(time.Hour)) {
		t.Fatalf("新版 = %s / %s（err=%v）", fresh.Channel, fresh.Deadline, err)
	}
}

func TestNotificationPolicyChecksRejectUnusableRows(t *testing.T) {
	fixture := newPolicyFixture(t)

	for _, testCase := range []struct {
		name   string
		values string
	}{
		{"零时限（通知一生成就已逾期）", `'t', 'd1', 'SMS', interval '0', 'DELIVERED'`},
		{"负时限（截止时间早于披露决定）", `'t', 'd2', 'SMS', interval '-1 hour', 'DELIVERED'`},
		{"空渠道", `'t', 'd3', '  ', interval '1 hour', 'DELIVERED'`},
		{"空义务判据", `'t', 'd4', 'SMS', interval '1 hour', ''`},
	} {
		if _, err := fixture.pool.Exec(t.Context(),
			`INSERT INTO visibility_exception.notification_policy
				(tenant_id, disclosure_policy_ref, channel_ref, deadline_after, obligation_ref)
			 VALUES (`+testCase.values+`)`); err == nil {
			t.Fatalf("库接受了「%s」的策略行", testCase.name)
		}
	}
}
