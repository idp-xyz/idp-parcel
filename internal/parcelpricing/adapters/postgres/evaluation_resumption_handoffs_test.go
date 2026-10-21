package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证计价交给结算续办那一路的两只交接口（ADR-0229 决定三、五）：「评价未形成」一份请求至多
// 一封、载荷带方向停因与说明、按请求分区；「参数已登记」同一面重放发同一封、每一面都带计价范围与方向、分区装租户；
// 写口无事务拒（PBC-08）。

func newResumptionHandoffs(t *testing.T) (*adapter.OutboxEvaluationResumptionHandoffs, *bentopg.DB) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	handoffs, err := adapter.NewOutboxEvaluationResumptionHandoffs(db, store, evaluationHandoffClock{
		at: time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return handoffs, db
}

type outboxPayload struct {
	partition string
	payload   map[string]any
}

func outboxOfType(t *testing.T, db *bentopg.DB, eventType string) []outboxPayload {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := querier.Query(t.Context(),
		`SELECT partition_key, payload FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1 ORDER BY event_id`, eventType)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var found []outboxPayload
	for rows.Next() {
		var (
			partition string
			raw       []byte
		)
		if err := rows.Scan(&partition, &raw); err != nil {
			t.Fatal(err)
		}
		body := map[string]any{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		found = append(found, outboxPayload{partition: partition, payload: body})
	}
	return found
}

// Covers: AT-PP-002 的交接口半边——一份请求至多一封「评价未形成」（重投吞掉第二封），载荷带方向、停因与说明，按请求分区。
func TestAnUnformedRequestIsHandedOffOncePerRequest(t *testing.T) {
	handoffs, db := newResumptionHandoffs(t)
	tenant, err := domain.NewTenantID("tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	request, err := domain.NewEvaluationRequestReference("SELLREQ-SYN-1")
	if err != nil {
		t.Fatal(err)
	}
	intent := ports.EvaluationRequestUnformedIntent{
		Tenant: tenant, Request: request, Direction: domain.PricingDirectionSell,
		Cause: ports.UnformedPriceCardNotConfigured, Detail: []string{"no price card in force"},
	}
	if err := handoffs.HandOffEvaluationRequestUnformed(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外交信封应答 ErrTransactionRequired，实得：%v", err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		if err := handoffs.HandOffEvaluationRequestUnformed(ctx, intent); err != nil {
			return err
		}
		return handoffs.HandOffEvaluationRequestUnformed(ctx, intent)
	}); err != nil {
		t.Fatal(err)
	}
	found := outboxOfType(t, db, "parcel-pricing.evaluation-request.unformed")
	if len(found) != 1 || found[0].partition != "tenant-a/evaluation-request-unformed/SELL/SELLREQ-SYN-1" ||
		found[0].payload["cause"] != "PRICE_CARD_NOT_CONFIGURED" ||
		found[0].payload["direction"] != "SELL" || found[0].payload["evaluationRequestId"] != "SELLREQ-SYN-1" {
		t.Fatalf("评价未形成 = %+v", found)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		blank := intent
		blank.Cause = ports.EvaluationRequestUnformedCauseInvalid
		if err := handoffs.HandOffEvaluationRequestUnformed(ctx, blank); err == nil {
			return errors.New("没有停因的评价未形成交了出去")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Covers: AT-PP-003、AT-PP-004 的交接口半边——同一面重放一封、两版两封、一版序列绑在两个范围上两封；每一封都带计价范围
// 与方向；缺范围、方向不是 BUY 或 SELL 就拒。
func TestARegisteredPricingParameterIsHandedOffOncePerVersionAndScope(t *testing.T) {
	handoffs, db := newResumptionHandoffs(t)
	tenant, err := domain.NewTenantID("tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	scopeOf := func(value string) domain.PricingScopeID {
		scope, err := domain.NewPricingScopeID(value)
		if err != nil {
			t.Fatal(err)
		}
		return scope
	}
	version := func(artifact domain.ArtifactKind, id, v string) domain.VersionReference {
		reference, err := domain.NewVersionReference(artifact, id, v, "sha256:syn")
		if err != nil {
			t.Fatal(err)
		}
		return reference
	}
	card := ports.PricingParameterRegisteredIntent{
		Tenant: tenant, Kind: ports.PricingParameterPriceCard, Registration: version(domain.ArtifactPricingPlan, "SYN-CARD", "v1"),
		Scope: scopeOf("SYN-SCOPE-01"), Direction: domain.PricingDirectionBuy,
	}
	if err := handoffs.HandOffPricingParameterRegistered(t.Context(), card); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外交信封应答 ErrTransactionRequired，实得：%v", err)
	}
	cardV2 := card
	cardV2.Registration = version(domain.ArtifactPricingPlan, "SYN-CARD", "v2")
	series := ports.PricingParameterRegisteredIntent{
		Tenant: tenant, Kind: ports.PricingParameterReferenceSeries, Registration: version(domain.ArtifactReferenceSeries, "SYN-FX", "v1"),
		Scope: scopeOf("SYN-SCOPE-01"), Direction: domain.PricingDirectionBuy,
	}
	seriesOnSell := series
	seriesOnSell.Scope, seriesOnSell.Direction = scopeOf("SYN-SCOPE-02"), domain.PricingDirectionSell
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		for _, intent := range []ports.PricingParameterRegisteredIntent{card, card, cardV2, series, seriesOnSell, seriesOnSell} {
			if err := handoffs.HandOffPricingParameterRegistered(ctx, intent); err != nil {
				return err
			}
		}
		withoutScope := series
		withoutScope.Scope = domain.PricingScopeID{}
		if err := handoffs.HandOffPricingParameterRegistered(ctx, withoutScope); err == nil {
			return errors.New("不带计价范围的序列登记交了出去")
		}
		internal := card
		internal.Direction = domain.PricingDirectionInternal
		if err := handoffs.HandOffPricingParameterRegistered(ctx, internal); err == nil {
			return errors.New("内部方向的价卡登记交了出去")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	found := outboxOfType(t, db, "parcel-pricing.pricing-parameter.registered")
	faces := map[string]int{}
	for _, each := range found {
		if each.partition != "tenant-a" {
			t.Fatalf("分区 = %q", each.partition)
		}
		faces[fmt.Sprint(each.payload["registrationKind"], "/", each.payload["pricingScope"], "/", each.payload["direction"])]++
	}
	want := map[string]int{
		"PRICE_CARD/SYN-SCOPE-01/BUY":        2,
		"REFERENCE_SERIES/SYN-SCOPE-01/BUY":  1,
		"REFERENCE_SERIES/SYN-SCOPE-02/SELL": 1,
	}
	if len(faces) != len(want) {
		t.Fatalf("各面 = %+v, want %+v", faces, want)
	}
	for face, count := range want {
		if faces[face] != count {
			t.Fatalf("各面 = %+v, want %+v（同一面重放不翻倍）", faces, want)
		}
	}
}
