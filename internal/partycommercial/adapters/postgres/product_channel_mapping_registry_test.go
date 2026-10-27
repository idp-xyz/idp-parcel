package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证产品—渠道映射登记册（0016）：往返重建（含显式“未
// 配置”绑定）、撞键重放/冲突、目录上列最新修订、跨租户读不到、无事务拒。

var mappingStartsAt = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

func newProductChannelMappings(t *testing.T) (*adapter.ProductChannelMappings, *adapter.OperationsCatalogue, bentoapp.Transactor) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	mappings, err := adapter.NewProductChannelMappings(db)
	if err != nil {
		t.Fatalf("构造映射登记册：%v", err)
	}
	catalogue, err := adapter.NewOperationsCatalogue(db)
	if err != nil {
		t.Fatalf("构造目录读口：%v", err)
	}
	return mappings, catalogue, db.Transactor()
}

func mappingRegistrationFixture(
	t *testing.T,
	tenant, id string,
	revision int,
	channels ...string,
) domain.ProductChannelMappingRegistration {
	t.Helper()
	binding := domain.UnconfiguredChannelBinding()
	if len(channels) > 0 {
		refs := make([]domain.ChannelProductReference, 0, len(channels))
		for _, channel := range channels {
			refs = append(refs, pcValue(t, domain.NewChannelProductReference, channel))
		}
		var err error
		binding, err = domain.NewConfiguredChannelBinding(refs)
		if err != nil {
			t.Fatalf("new configured binding: %v", err)
		}
	}
	interval, err := domain.NewEffectiveInterval(mappingStartsAt, time.Time{})
	if err != nil {
		t.Fatalf("new interval: %v", err)
	}
	registration, err := domain.NewProductChannelMappingRegistration(
		pcTenant(t, tenant),
		pcValue(t, domain.NewProductChannelMappingID, id),
		revision,
		domain.ProductChannelMappingSpec{
			Product:        pcValue(t, domain.NewCommercialObjectID, "product-1"),
			ProductVersion: pcValue(t, domain.NewCommercialVersionLabel, "v1"),
			Binding:        binding,
			Effective:      interval,
			Basis:          pcValue(t, domain.NewMappingBasisReference, "basis-"+id),
		},
	)
	if err != nil {
		t.Fatalf("new mapping registration: %v", err)
	}
	return registration
}

func mustSaveMapping(
	t *testing.T,
	transactor bentoapp.Transactor,
	mappings *adapter.ProductChannelMappings,
	registration domain.ProductChannelMappingRegistration,
) {
	t.Helper()
	mustWithinPublicationTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := mappings.SaveMapping(txCtx, registration)
		if err != nil {
			return err
		}
		if outcome != ports.MappingSaved {
			return fmt.Errorf("save outcome = %s", outcome)
		}
		return nil
	})
}

// mappingRegistrationWithEnd 把一笔登记换成带区间终点的同内容一笔：修订历史要证每一笔各读各的区间终点。
func mappingRegistrationWithEnd(
	t *testing.T,
	base domain.ProductChannelMappingRegistration,
	endsAt time.Time,
) domain.ProductChannelMappingRegistration {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(mappingStartsAt, endsAt)
	if err != nil {
		t.Fatalf("new interval: %v", err)
	}
	registration, err := domain.NewProductChannelMappingRegistration(
		base.Tenant(),
		base.ID(),
		base.Revision(),
		domain.ProductChannelMappingSpec{
			Product:        base.Product(),
			ProductVersion: base.ProductVersion(),
			Binding:        base.Binding(),
			Effective:      interval,
			Basis:          base.Basis(),
		},
	)
	if err != nil {
		t.Fatalf("new mapping registration: %v", err)
	}
	return registration
}

// Covers: 票 02 完成判据「渠道产品目录页非空册、渠道绑定格如实显示未配置」的库上
// 半边——已配置与显式未配置两种绑定落册往返、目录上列最新修订、空数组原样转写。
func TestProductChannelMappingRoundTripAndCatalogue(t *testing.T) {
	mappings, catalogue, transactor := newProductChannelMappings(t)
	ctx := t.Context()

	configured := mappingRegistrationFixture(t, "tenant-1", "map-configured", 1, "CH-SG-POST", "CH-AGG")
	mustSaveMapping(t, transactor, mappings, configured)
	unconfigured := mappingRegistrationFixture(t, "tenant-1", "map-open", 1)
	mustSaveMapping(t, transactor, mappings, unconfigured)

	// 往返重建：显式“未配置”照未配置回来，已配置的引用原样、顺序不乱。
	loaded, found, err := mappings.LoadLatestMapping(
		ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewProductChannelMappingID, "map-open"))
	if err != nil || !found {
		t.Fatalf("load unconfigured = (%v, %v)", found, err)
	}
	if loaded.Binding().Configured() || len(loaded.Binding().Channels()) != 0 {
		t.Fatalf("未配置绑定读回来 = %+v", loaded.Binding())
	}
	loaded, found, err = mappings.LoadLatestMapping(
		ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewProductChannelMappingID, "map-configured"))
	if err != nil || !found {
		t.Fatalf("load configured = (%v, %v)", found, err)
	}
	channels := loaded.Binding().Channels()
	if len(channels) != 2 || channels[0].String() != "CH-SG-POST" || channels[1].String() != "CH-AGG" {
		t.Fatalf("已配置绑定读回来 = %+v", channels)
	}

	// 修订推进：rev2 调整绑定，LoadLatest 与目录都只见最新修订。
	revised := mappingRegistrationFixture(t, "tenant-1", "map-open", 2, "CH-NEW")
	mustSaveMapping(t, transactor, mappings, revised)

	rows, err := catalogue.ListProductChannelMappings(ctx, pcTenant(t, "tenant-1"), 10)
	if err != nil {
		t.Fatalf("list mappings: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	byID := make(map[string]ports.ProductChannelMappingRow, len(rows))
	for _, row := range rows {
		byID[row.MappingID] = row
	}
	configuredRow := byID["map-configured"]
	if configuredRow.Revision != 1 || len(configuredRow.Channels) != 2 ||
		configuredRow.ProductObjectID != "product-1" || configuredRow.ProductVersionLabel != "v1" ||
		configuredRow.HasEffectiveEnd {
		t.Fatalf("configured row = %+v", configuredRow)
	}
	openRow := byID["map-open"]
	if openRow.Revision != 2 || len(openRow.Channels) != 1 || openRow.Channels[0] != "CH-NEW" {
		t.Fatalf("open row（应是 rev2）= %+v", openRow)
	}

	// 跨租户读不到：目录与点读都与从未登记同形。
	foreign, err := catalogue.ListProductChannelMappings(ctx, pcTenant(t, "tenant-b"), 10)
	if err != nil || len(foreign) != 0 {
		t.Fatalf("foreign rows = (%d, %v)", len(foreign), err)
	}
	_, found, err = mappings.LoadLatestMapping(
		ctx, pcTenant(t, "tenant-b"), pcValue(t, domain.NewProductChannelMappingID, "map-open"))
	if err != nil || found {
		t.Fatalf("foreign load = (%v, %v)", found, err)
	}
}

// Covers: AT-PC-067——一个映射的整条修订链按修订号升序交出（旧笔不被顶掉），别的映射、别的租户读不到；不在册答空切片；
// 显式“未配置”的那一笔仍是空数组。
func TestProductChannelMappingRevisionsExpandOneMappingAscending(t *testing.T) {
	mappings, catalogue, transactor := newProductChannelMappings(t)
	ctx := t.Context()

	// 落库次序故意是 2、1、3：序按修订号而不按 recorded_at，到达次序不是修订号的次序（登记不要求连续到达）。
	mustSaveMapping(t, transactor, mappings, mappingRegistrationFixture(t, "tenant-1", "map-1", 2))
	mustSaveMapping(t, transactor, mappings, mappingRegistrationFixture(t, "tenant-1", "map-1", 1, "CH-A"))
	closing := mappingRegistrationFixture(t, "tenant-1", "map-1", 3, "CH-B", "CH-C")
	closing = mappingRegistrationWithEnd(t, closing, mappingStartsAt.Add(30*24*time.Hour))
	mustSaveMapping(t, transactor, mappings, closing)
	mustSaveMapping(t, transactor, mappings, mappingRegistrationFixture(t, "tenant-1", "map-2", 1, "CH-Z"))
	mustSaveMapping(t, transactor, mappings, mappingRegistrationFixture(t, "tenant-b", "map-1", 1, "CH-FOREIGN"))

	chain, err := catalogue.ListProductChannelMappingRevisions(
		ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewProductChannelMappingID, "map-1"))
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(chain) != 3 || chain[0].Revision != 1 || chain[1].Revision != 2 || chain[2].Revision != 3 {
		t.Fatalf("修订链次序 = %+v", chain)
	}
	if len(chain[0].Channels) != 1 || chain[0].Channels[0] != "CH-A" {
		t.Fatalf("第 1 笔绑定 = %+v", chain[0].Channels)
	}
	if chain[1].Channels == nil || len(chain[1].Channels) != 0 {
		t.Fatalf("第 2 笔是显式未配置，绑定应是空切片：%+v", chain[1].Channels)
	}
	if len(chain[2].Channels) != 2 || chain[2].Channels[1] != "CH-C" {
		t.Fatalf("第 3 笔绑定 = %+v", chain[2].Channels)
	}
	if chain[0].HasEffectiveEnd || !chain[2].HasEffectiveEnd || !chain[2].EffectiveEndsAt.Equal(mappingStartsAt.Add(30*24*time.Hour)) {
		t.Fatalf("区间终点没有逐笔读回：第 1 笔 %+v，第 3 笔 %+v", chain[0].HasEffectiveEnd, chain[2])
	}
	for _, row := range chain {
		if row.MappingID != "map-1" || row.TenantID != "tenant-1" {
			t.Fatalf("链里混进了别的映射或租户：%+v", row)
		}
	}

	unknown, err := catalogue.ListProductChannelMappingRevisions(
		ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewProductChannelMappingID, "map-nobody"))
	if err != nil || unknown == nil || len(unknown) != 0 {
		t.Fatalf("不在册 = (%v, %v)，应是空切片而不是 nil", unknown, err)
	}
	foreign, err := catalogue.ListProductChannelMappingRevisions(
		ctx, pcTenant(t, "tenant-b"), pcValue(t, domain.NewProductChannelMappingID, "map-2"))
	if err != nil || len(foreign) != 0 {
		t.Fatalf("跨租户读到了别家的映射：(%d, %v)", len(foreign), err)
	}
}

// Covers: ADR-0031 登记面代数在映射册上的镜像——同键同内容重放，同键异内容冲突。
func TestProductChannelMappingSavesReplayAndConflict(t *testing.T) {
	mappings, _, transactor := newProductChannelMappings(t)

	registration := mappingRegistrationFixture(t, "tenant-1", "map-1", 1, "CH-01")
	mustSaveMapping(t, transactor, mappings, registration)

	var outcome ports.MappingSaveOutcome
	mustWithinPublicationTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = mappings.SaveMapping(txCtx, registration)
		return err
	})
	if outcome != ports.MappingAlreadyRegistered {
		t.Fatalf("replay outcome = %s, want ALREADY_REGISTERED", outcome)
	}

	conflicting := mappingRegistrationFixture(t, "tenant-1", "map-1", 1, "CH-02")
	mustWithinPublicationTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = mappings.SaveMapping(txCtx, conflicting)
		return err
	})
	if outcome != ports.MappingContentConflict {
		t.Fatalf("conflict outcome = %s, want CONTENT_CONFLICT", outcome)
	}
}

func TestProductChannelMappingWritesRefuseToRunOutsideATransaction(t *testing.T) {
	mappings, _, _ := newProductChannelMappings(t)
	if _, err := mappings.SaveMapping(t.Context(),
		mappingRegistrationFixture(t, "tenant-1", "map-1", 1, "CH-01"),
	); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 SaveMapping 应返回 ErrTransactionRequired，实得：%v", err)
	}
}
