package postgres_test

import (
	"context"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 关务适用性判断口的真库端到端用例（票 routing-first-cut/12 完成判据二、三的落库那一半）：
// 合成口岸与申报路径目录上，候选得出可用、不可用（口岸未登记/未生效、申报路径方向缺）与
// 状态未知（目录为空、缺国家码）各一；状态未知不折成可用；同输入重复作答一致、答案带出处。
// 依赖读不到那格是端口层的行为，在应用层用替身钉（application 包测试），这里不造库故障。
//
// 各场景各用一个租户标识，同一次测试事务里互不串味；演示数据全为 SYN- 合成值（证据记 S）。

func judgeTenant(t *testing.T, fixture *viewFixture, tenant, candidate, origin, destination string, hasOrigin, hasDestination bool, asOf time.Time) domain.CustomsApplicabilityJudgment {
	t.Helper()
	snapshots, err := adapter.NewPortsPathsSnapshots(fixture.db)
	if err != nil {
		t.Fatalf("构造快照读口：%v", err)
	}
	handler := customsapp.NewCustomsApplicabilityHandler(customsapp.CustomsApplicabilityDeps{Catalog: snapshots})
	judgments, err := handler.Handle(t.Context(), ports.CustomsApplicabilityQuery{
		Tenant: viewValue(t, domain.NewTenantID, tenant),
		AsOf:   asOf,
		Candidates: []ports.CustomsApplicabilityCandidate{{
			Candidate: viewValue(t, domain.NewRouteCandidateReference, candidate),
			Origin:    origin, HasOrigin: hasOrigin,
			Destination: destination, HasDestination: hasDestination,
		}},
	})
	if err != nil {
		t.Fatalf("判断：%v", err)
	}
	if len(judgments) != 1 {
		t.Fatalf("交一条候选该回一条判断，实得 %d 条", len(judgments))
	}
	return judgments[0]
}

func registerSynPort(t *testing.T, fixture *viewFixture, registry *adapter.PortsPathsRegistrations, tenant, port string, appliesFrom time.Time) {
	t.Helper()
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterCandidatePort(ctx,
			viewValue(t, domain.NewTenantID, tenant),
			viewValue(t, domain.NewCustomsPortReference, port), appliesFrom)
	}); err != nil {
		t.Fatalf("登记口岸 %s：%v", port, err)
	}
}

func registerSynPath(t *testing.T, fixture *viewFixture, registry *adapter.PortsPathsRegistrations, tenant, path, port, mode string, direction domain.ManifestDirection, appliesFrom time.Time) {
	t.Helper()
	route, err := domain.NewDeclarationPathRoute(
		viewValue(t, domain.NewCustomsPortReference, port),
		direction,
		viewValue(t, domain.NewDeclarationModeReference, mode))
	if err != nil {
		t.Fatalf("构造路径三维：%v", err)
	}
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterDeclarationPath(ctx,
			viewValue(t, domain.NewTenantID, tenant),
			viewValue(t, domain.NewDeclarationPathReference, path), route, appliesFrom)
	}); err != nil {
		t.Fatalf("登记路径 %s：%v", path, err)
	}
}

func TestCustomsApplicabilityJudgesFromRealCatalog(t *testing.T) {
	fixture := newViewFixture(t)
	registry, err := adapter.NewPortsPathsRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造口岸路径写口：%v", err)
	}
	asOf := registryBaseAt.Add(time.Hour)

	// 完整目录：CN 出口与 SG 进口各一条过硬路径。
	registerSynPort(t, fixture, registry, "tenant-c1", "SYN-PORT-SZX-01", registryBaseAt)
	registerSynPort(t, fixture, registry, "tenant-c1", "SYN-PORT-SIN-01", registryBaseAt)
	registerSynPath(t, fixture, registry, "tenant-c1", "SYN-PATH-CN-EXPORT-01", "SYN-PORT-SZX-01", "SYN-MODE-MANIFEST-01", domain.ExportManifest, registryBaseAt)
	registerSynPath(t, fixture, registry, "tenant-c1", "SYN-PATH-SG-IMPORT-01", "SYN-PORT-SIN-01", "SYN-MODE-FORMAL-01", domain.ImportManifest, registryBaseAt)

	first := judgeTenant(t, fixture, "tenant-c1", "cand-cn-sg", "CN", "SG", true, true, asOf)
	if first.Outcome() != domain.CustomsAvailable {
		t.Fatalf("outcome = %q，两岸过硬目录上的 CN→SG 候选该可用", first.Outcome().String())
	}
	if first.JudgmentID() == "" || len(first.Versions()) != 4 {
		t.Fatalf("出处 = %q/%d 个版本引用，该带判断标识与两口岸两路径的版本", first.JudgmentID(), len(first.Versions()))
	}

	// 同输入重复作答（判据三）：穿同一组装路径再判一次，逐字节一致。
	again := judgeTenant(t, fixture, "tenant-c1", "cand-cn-sg", "CN", "SG", true, true, asOf)
	if again.JudgmentID() != first.JudgmentID() || again.Outcome() != first.Outcome() {
		t.Fatal("同租户、同时点、同候选、同目录，重复作答必须一致")
	}

	// 口岸未登记：出口路径经一个任何时点都不在册的口岸。
	registerSynPort(t, fixture, registry, "tenant-c2", "SYN-PORT-SIN-01", registryBaseAt)
	registerSynPath(t, fixture, registry, "tenant-c2", "SYN-PATH-CN-EXPORT-GHOST", "SYN-PORT-GHOST-01", "SYN-MODE-MANIFEST-01", domain.ExportManifest, registryBaseAt)
	registerSynPath(t, fixture, registry, "tenant-c2", "SYN-PATH-SG-IMPORT-01", "SYN-PORT-SIN-01", "SYN-MODE-FORMAL-01", domain.ImportManifest, registryBaseAt)

	unregistered := judgeTenant(t, fixture, "tenant-c2", "cand-cn-sg", "CN", "SG", true, true, asOf)
	if unregistered.Outcome() != domain.CustomsUnavailable ||
		unregistered.UnavailableReason() != domain.PortNotRegistered {
		t.Fatalf("outcome/reason = %q/%q，经未登记口岸的路径该答不可用·口岸未登记",
			unregistered.Outcome().String(), unregistered.UnavailableReason().String())
	}
	if port, ok := unregistered.UnavailablePort(); !ok || port.String() != "SYN-PORT-GHOST-01" {
		t.Fatalf("port = %q/%v，该指名未登记的那个口岸", port.String(), ok)
	}

	// 口岸未生效：口岸在册但判断时点不在区间内（未来起点的版本）。
	registerSynPort(t, fixture, registry, "tenant-c3", "SYN-PORT-FUTURE-01", registryBaseAt.AddDate(0, 3, 0))
	registerSynPort(t, fixture, registry, "tenant-c3", "SYN-PORT-SIN-01", registryBaseAt)
	registerSynPath(t, fixture, registry, "tenant-c3", "SYN-PATH-CN-EXPORT-FUTURE", "SYN-PORT-FUTURE-01", "SYN-MODE-MANIFEST-01", domain.ExportManifest, registryBaseAt)
	registerSynPath(t, fixture, registry, "tenant-c3", "SYN-PATH-SG-IMPORT-01", "SYN-PORT-SIN-01", "SYN-MODE-FORMAL-01", domain.ImportManifest, registryBaseAt)

	ineffective := judgeTenant(t, fixture, "tenant-c3", "cand-cn-sg", "CN", "SG", true, true, asOf)
	if ineffective.Outcome() != domain.CustomsUnavailable ||
		ineffective.UnavailableReason() != domain.PortNotEffective {
		t.Fatalf("outcome/reason = %q/%q，未来才生效的口岸该答不可用·口岸未生效",
			ineffective.Outcome().String(), ineffective.UnavailableReason().String())
	}
	if port, ok := ineffective.UnavailablePort(); !ok || port.String() != "SYN-PORT-FUTURE-01" {
		t.Fatalf("port = %q/%v，该指名未生效的那个口岸", port.String(), ok)
	}

	// 申报路径方向缺：只有出口路径，收件国一侧没有进口路径。
	registerSynPort(t, fixture, registry, "tenant-c4", "SYN-PORT-SZX-01", registryBaseAt)
	registerSynPath(t, fixture, registry, "tenant-c4", "SYN-PATH-CN-EXPORT-01", "SYN-PORT-SZX-01", "SYN-MODE-MANIFEST-01", domain.ExportManifest, registryBaseAt)

	noImport := judgeTenant(t, fixture, "tenant-c4", "cand-cn-sg", "CN", "SG", true, true, asOf)
	if noImport.Outcome() != domain.CustomsUnavailable ||
		noImport.UnavailableReason() != domain.PathDirectionNotCovered {
		t.Fatalf("outcome/reason = %q/%q，缺进口路径该答不可用·方向缺",
			noImport.Outcome().String(), noImport.UnavailableReason().String())
	}
	if direction, ok := noImport.UnavailableDirection(); !ok || direction != domain.ImportManifest {
		t.Fatalf("direction = %q/%v，该指名缺进口方向", direction.String(), ok)
	}

	// 状态未知·目录为空：什么都没登记的租户，跨境候选既不折可用也不折不可用。
	empty := judgeTenant(t, fixture, "tenant-c5", "cand-cn-sg", "CN", "SG", true, true, asOf)
	if empty.Outcome() != domain.CustomsStatusUnknown || empty.UnknownReason() != domain.CatalogEmpty {
		t.Fatalf("outcome/unknown = %q/%q，目录为空该答状态未知", empty.Outcome().String(), empty.UnknownReason().String())
	}
	// 同一租户上的同国候选不受目录空影响——不含关务段即无阻。
	domestic := judgeTenant(t, fixture, "tenant-c5", "cand-cn-cn", "CN", "CN", true, true, asOf)
	if domestic.Outcome() != domain.CustomsAvailable {
		t.Fatalf("outcome = %q，两端同国候选在空目录上也该可用", domestic.Outcome().String())
	}

	// 状态未知·缺国家码：寄件国缺席按证据不足作答，指名哪一端。
	missing := judgeTenant(t, fixture, "tenant-c1", "cand-no-origin", "", "SG", false, true, asOf)
	if missing.Outcome() != domain.CustomsStatusUnknown ||
		missing.UnknownReason() != domain.EndpointCountryMissing {
		t.Fatalf("outcome/unknown = %q/%q，缺寄件国该答状态未知", missing.Outcome().String(), missing.UnknownReason().String())
	}
	if side, ok := missing.EndpointSide(); !ok || side != domain.EndpointSideOrigin {
		t.Fatalf("side = %q/%v，该指名缺寄件国", side.String(), ok)
	}
}
