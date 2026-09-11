package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 登记用例的用例。替身一律**同时实现写口与读口并共用一份存储**，因为本层唯一的职责
// 就是「写口说已在册之后，读回来比一比」——写读分家的替身会让那段比对无从触发，测了
// 也只是测了个 if。

var configBaseAt = time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)

var errRegistryDown = errors.New("登记册不可达")

type readinessStoreDouble struct {
	rows        map[string]domain.ReadinessJudgment
	registerErr error
	revokeErr   error
	loadErr     error
}

func newReadinessStore() *readinessStoreDouble {
	return &readinessStoreDouble{rows: map[string]domain.ReadinessJudgment{}}
}

func (double *readinessStoreDouble) RegisterReadiness(
	_ context.Context, tenant domain.TenantID, judgment domain.ReadinessJudgment,
) (ports.CaseConfigurationSaveOutcome, error) {
	if double.registerErr != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, double.registerErr
	}
	key := tenant.String() + "|" + judgment.Unit().String()
	if _, exists := double.rows[key]; exists {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	double.rows[key] = judgment
	return ports.CaseConfigurationRegistered, nil
}

func (double *readinessStoreDouble) RevokeReadiness(
	_ context.Context, tenant domain.TenantID, judgment domain.ReadinessJudgment,
) error {
	if double.revokeErr != nil {
		return double.revokeErr
	}
	double.rows[tenant.String()+"|"+judgment.Unit().String()] = judgment
	return nil
}

func (double *readinessStoreDouble) LoadReadiness(
	_ context.Context, tenant domain.TenantID, unit domain.DeclarationUnitID,
) (domain.ReadinessJudgment, bool, error) {
	if double.loadErr != nil {
		return domain.ReadinessJudgment{}, false, double.loadErr
	}
	judgment, found := double.rows[tenant.String()+"|"+unit.String()]
	return judgment, found, nil
}

func configValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

func readinessCommand(t *testing.T, basis string) application.RegisterReadinessCommand {
	t.Helper()
	return application.RegisterReadinessCommand{
		TenantID: configValue(t, domain.NewTenantID, "tenant-a"),
		Unit:     configValue(t, domain.NewDeclarationUnitID, "unit-1"),
		Basis:    configValue(t, domain.NewReadinessBasisReference, basis),
		JudgedAt: configBaseAt,
	}
}

func TestRegisteringReadinessTheFirstTimeIsRegistered(t *testing.T) {
	store := newReadinessStore()
	handler := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Readiness: store, ReadinessView: store,
	})

	outcome, err := handler.RegisterReadiness(t.Context(), readinessCommand(t, "DOSSIER/COMPLETE"))
	if err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("首次登记：err=%v outcome=%v", err, outcome)
	}
}

// 本层存在的全部理由，正反两格各一条。写口对这两次调用交回的是同一个`已登记`，
// 只有读回比对才把它们分开。
func TestReplayingTheSameReadinessIsExistingRatherThanConflict(t *testing.T) {
	store := newReadinessStore()
	handler := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Readiness: store, ReadinessView: store,
	})
	command := readinessCommand(t, "DOSSIER/COMPLETE")

	if _, err := handler.RegisterReadiness(t.Context(), command); err != nil {
		t.Fatalf("首次登记：%v", err)
	}
	outcome, err := handler.RegisterReadiness(t.Context(), command)
	if err != nil || outcome != application.ConfigurationExisting {
		t.Fatalf("重放同一份该是`已存在`：err=%v outcome=%v", err, outcome)
	}
}

func TestRegisteringADifferentBasisForTheSameUnitConflictsAndKeepsTheOriginal(t *testing.T) {
	store := newReadinessStore()
	handler := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Readiness: store, ReadinessView: store,
	})

	if _, err := handler.RegisterReadiness(t.Context(), readinessCommand(t, "DOSSIER/FIRST")); err != nil {
		t.Fatalf("首次登记：%v", err)
	}
	outcome, err := handler.RegisterReadiness(t.Context(), readinessCommand(t, "DOSSIER/SECOND"))
	if err != nil || outcome != application.ConfigurationContentConflict {
		t.Fatalf("换依据该是`内容冲突`：err=%v outcome=%v", err, outcome)
	}

	stored, _, err := store.LoadReadiness(t.Context(),
		configValue(t, domain.NewTenantID, "tenant-a"),
		configValue(t, domain.NewDeclarationUnitID, "unit-1"))
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	if stored.Basis().String() != "DOSSIER/FIRST" {
		t.Fatalf("冲突却把原依据顶掉了：%s", stored.Basis())
	}
}

// 同依据但形成时间不同也是两份判断：时间是判断内容的一部分，不是元数据。
func TestTheSameBasisJudgedAtADifferentTimeConflicts(t *testing.T) {
	store := newReadinessStore()
	handler := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Readiness: store, ReadinessView: store,
	})

	if _, err := handler.RegisterReadiness(t.Context(), readinessCommand(t, "DOSSIER/COMPLETE")); err != nil {
		t.Fatalf("首次登记：%v", err)
	}
	later := readinessCommand(t, "DOSSIER/COMPLETE")
	later.JudgedAt = configBaseAt.Add(time.Hour)

	outcome, err := handler.RegisterReadiness(t.Context(), later)
	if err != nil || outcome != application.ConfigurationContentConflict {
		t.Fatalf("换形成时间该是`内容冲突`：err=%v outcome=%v", err, outcome)
	}
}

// 基础设施故障交回`未决`而不是`未接受`：前者可重试，后者是请求本身不成立，两者的
// 续办动作相反。
func TestARegistryFailureIsUndecidedRatherThanRejected(t *testing.T) {
	store := newReadinessStore()
	store.registerErr = errRegistryDown
	handler := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Readiness: store, ReadinessView: store,
	})

	outcome, err := handler.RegisterReadiness(t.Context(), readinessCommand(t, "DOSSIER/COMPLETE"))
	if err != nil || outcome != application.ConfigurationUndecided {
		t.Fatalf("写口报错该是`未决`：err=%v outcome=%v", err, outcome)
	}
}

// 写口说已在册、读口却读不回来，是一个说不清的状态：不能当重放放过（那会让调用方
// 以为自己那份已生效），只能`未决`。
func TestAnUnreadableExistingRegistrationIsUndecided(t *testing.T) {
	store := newReadinessStore()
	handler := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Readiness: store, ReadinessView: store,
	})
	if _, err := handler.RegisterReadiness(t.Context(), readinessCommand(t, "DOSSIER/COMPLETE")); err != nil {
		t.Fatalf("首次登记：%v", err)
	}

	store.loadErr = errRegistryDown
	outcome, err := handler.RegisterReadiness(t.Context(), readinessCommand(t, "DOSSIER/COMPLETE"))
	if err != nil || outcome != application.ConfigurationUndecided {
		t.Fatalf("读不回该是`未决`：err=%v outcome=%v", err, outcome)
	}
}

func TestABlankTenantIsNotAccepted(t *testing.T) {
	store := newReadinessStore()
	handler := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Readiness: store, ReadinessView: store,
	})
	command := readinessCommand(t, "DOSSIER/COMPLETE")
	command.TenantID = domain.TenantID{}

	outcome, err := handler.RegisterReadiness(t.Context(), command)
	if err != nil || outcome != application.ConfigurationNotAccepted {
		t.Fatalf("空租户该是`未接受`：err=%v outcome=%v", err, outcome)
	}
}

func revokeReadinessCommand(t *testing.T, cause string, at time.Time) application.RevokeReadinessCommand {
	t.Helper()
	return application.RevokeReadinessCommand{
		TenantID: configValue(t, domain.NewTenantID, "tenant-a"),
		Unit:     configValue(t, domain.NewDeclarationUnitID, "unit-1"),
		Cause:    cause,
		At:       at,
	}
}

// 撤销那一族的三格分开：没登记过、撤销成功、已撤销。把前两格合并会让「撤销一个不
// 存在的判断」看起来成功。
func TestRevokingReadinessDistinguishesNotRegisteredFromAlreadyRevoked(t *testing.T) {
	store := newReadinessStore()
	handler := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Readiness: store, ReadinessView: store,
	})

	outcome, err := handler.RevokeReadiness(t.Context(), revokeReadinessCommand(t, "RULE/CHANGED", configBaseAt.Add(time.Hour)))
	if err != nil || outcome != application.ConfigurationNotRegistered {
		t.Fatalf("撤销未登记的判断该是`未登记`：err=%v outcome=%v", err, outcome)
	}

	if _, err := handler.RegisterReadiness(t.Context(), readinessCommand(t, "DOSSIER/COMPLETE")); err != nil {
		t.Fatalf("登记：%v", err)
	}
	outcome, err = handler.RevokeReadiness(t.Context(), revokeReadinessCommand(t, "RULE/CHANGED", configBaseAt.Add(time.Hour)))
	if err != nil || outcome != application.ConfigurationRevoked {
		t.Fatalf("撤销：err=%v outcome=%v", err, outcome)
	}

	outcome, err = handler.RevokeReadiness(t.Context(), revokeReadinessCommand(t, "CREDENTIAL/EXPIRED", configBaseAt.Add(5*time.Hour)))
	if err != nil || outcome != application.ConfigurationAlreadyRevoked {
		t.Fatalf("重复撤销该是`已撤销`：err=%v outcome=%v", err, outcome)
	}
}

// 撤销的合法性由领域判，本层不自己放行：早于形成时间的撤销时间不成立。
func TestARevocationBeforeTheJudgmentIsNotAccepted(t *testing.T) {
	store := newReadinessStore()
	handler := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Readiness: store, ReadinessView: store,
	})
	if _, err := handler.RegisterReadiness(t.Context(), readinessCommand(t, "DOSSIER/COMPLETE")); err != nil {
		t.Fatalf("登记：%v", err)
	}

	outcome, err := handler.RevokeReadiness(t.Context(),
		revokeReadinessCommand(t, "RULE/CHANGED", configBaseAt.Add(-time.Hour)))
	if err != nil || outcome != application.ConfigurationNotAccepted {
		t.Fatalf("撤销早于形成时间该是`未接受`：err=%v outcome=%v", err, outcome)
	}
}

// ruleVersionRow 是替身里的一版解释规则：终点零值即「尚无终点」。
type ruleVersionRow struct {
	rule  domain.InterpretationRuleReference
	from  time.Time
	until time.Time
}

// ruleStoreDouble 按真写口的版本化语义行事：同支同起点撞键与撞重叠都折成`已登记`；
// 后继登记给开放前版落终点（换版）。读口按半开区间解析。
type ruleStoreDouble struct {
	rows map[string][]ruleVersionRow
}

func newRuleStore() *ruleStoreDouble {
	return &ruleStoreDouble{rows: map[string][]ruleVersionRow{}}
}

func ruleLineage(tenant domain.TenantID, layer domain.ResultLayer, jurisdiction domain.RegulatoryJurisdictionReference) string {
	return tenant.String() + "|" + layer.String() + "|" + jurisdiction.String()
}

func (double *ruleStoreDouble) RegisterInterpretationRule(
	_ context.Context,
	tenant domain.TenantID,
	layer domain.ResultLayer,
	jurisdiction domain.RegulatoryJurisdictionReference,
	rule domain.InterpretationRuleReference,
	appliesFrom time.Time,
) (ports.CaseConfigurationSaveOutcome, error) {
	lineage := ruleLineage(tenant, layer, jurisdiction)
	rows := double.rows[lineage]
	predecessor := -1
	for index, row := range rows {
		if row.from.Equal(appliesFrom) {
			return ports.CaseConfigurationAlreadyRegistered, nil
		}
		if row.until.IsZero() && row.from.Before(appliesFrom) {
			predecessor = index
			continue
		}
		// 候选以开放区间进册：与任何终点晚于其起点的既有行重叠。
		if row.until.IsZero() || row.until.After(appliesFrom) {
			return ports.CaseConfigurationAlreadyRegistered, nil
		}
	}
	if predecessor >= 0 {
		rows[predecessor].until = appliesFrom
	}
	double.rows[lineage] = append(rows, ruleVersionRow{rule: rule, from: appliesFrom})
	return ports.CaseConfigurationRegistered, nil
}

func (double *ruleStoreDouble) LoadInterpretationRule(
	_ context.Context,
	tenant domain.TenantID,
	layer domain.ResultLayer,
	jurisdiction domain.RegulatoryJurisdictionReference,
	evaluatedAt time.Time,
) (domain.InterpretationRuleReference, bool, error) {
	for _, row := range double.rows[ruleLineage(tenant, layer, jurisdiction)] {
		if !row.from.After(evaluatedAt) && (row.until.IsZero() || row.until.After(evaluatedAt)) {
			return row.rule, true, nil
		}
	}
	return domain.InterpretationRuleReference{}, false, nil
}

func ruleCommand(t *testing.T, layer domain.ResultLayer, rule string) application.RegisterInterpretationRuleCommand {
	t.Helper()
	return application.RegisterInterpretationRuleCommand{
		TenantID:     configValue(t, domain.NewTenantID, "tenant-a"),
		Layer:        layer,
		Jurisdiction: configValue(t, domain.NewRegulatoryJurisdictionReference, "jurisdiction-1"),
		Rule:         configValue(t, domain.NewInterpretationRuleReference, rule),
		AppliesFrom:  configBaseAt,
	}
}

// 同键（同支同起点）换规则是冲突，不是覆盖：既有 ExternalResult 上「实际采用的规则」
// 不接受被顶替。换版走登记更晚起点的新版本，见下一个用例。
func TestRegisteringAnotherRuleAtTheSameStartIsAConflictNotAnOverwrite(t *testing.T) {
	store := newRuleStore()
	handler := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Rules: store, RuleView: store,
	})

	if _, err := handler.RegisterInterpretationRule(t.Context(),
		ruleCommand(t, domain.ReleaseResultLayer, "interpret/release/v1")); err != nil {
		t.Fatalf("首次登记：%v", err)
	}
	outcome, err := handler.RegisterInterpretationRule(t.Context(),
		ruleCommand(t, domain.ReleaseResultLayer, "interpret/release/v2"))
	if err != nil || outcome != application.ConfigurationContentConflict {
		t.Fatalf("同键换规则该是`内容冲突`：err=%v outcome=%v", err, outcome)
	}

	stored, _, err := store.LoadInterpretationRule(t.Context(),
		configValue(t, domain.NewTenantID, "tenant-a"), domain.ReleaseResultLayer,
		configValue(t, domain.NewRegulatoryJurisdictionReference, "jurisdiction-1"), configBaseAt)
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	if stored.String() != "interpret/release/v1" {
		t.Fatalf("原规则被顶替成 %s", stored)
	}
}

func TestRegisteringTheSameRuleAgainIsExisting(t *testing.T) {
	store := newRuleStore()
	handler := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Rules: store, RuleView: store,
	})
	command := ruleCommand(t, domain.ReleaseResultLayer, "interpret/release/v1")

	if _, err := handler.RegisterInterpretationRule(t.Context(), command); err != nil {
		t.Fatalf("首次登记：%v", err)
	}
	outcome, err := handler.RegisterInterpretationRule(t.Context(), command)
	if err != nil || outcome != application.ConfigurationExisting {
		t.Fatalf("重放该是`已存在`：err=%v outcome=%v", err, outcome)
	}
}

// ADR-0070 支点场景的登记半边：换版是登记一个更晚起点的新版本，前版终点随之落定。
// 之后按业务发生时间解析，落在旧区间的迟到响应取回旧版——不是到达时刻的当前指针。
func TestSupersedingRegistersANewVersionAndOldInstantsStillResolveTheOldRule(t *testing.T) {
	store := newRuleStore()
	handler := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Rules: store, RuleView: store,
	})
	tenant := configValue(t, domain.NewTenantID, "tenant-a")
	jurisdiction := configValue(t, domain.NewRegulatoryJurisdictionReference, "jurisdiction-1")

	if _, err := handler.RegisterInterpretationRule(t.Context(),
		ruleCommand(t, domain.ReleaseResultLayer, "interpret/release/v1")); err != nil {
		t.Fatalf("登记 v1：%v", err)
	}
	succession := ruleCommand(t, domain.ReleaseResultLayer, "interpret/release/v2")
	succession.AppliesFrom = configBaseAt.Add(48 * time.Hour)
	outcome, err := handler.RegisterInterpretationRule(t.Context(), succession)
	if err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("换版该是新登记：err=%v outcome=%v", err, outcome)
	}

	early, foundEarly, err := store.LoadInterpretationRule(t.Context(),
		tenant, domain.ReleaseResultLayer, jurisdiction, configBaseAt.Add(time.Hour))
	if err != nil || !foundEarly || early.String() != "interpret/release/v1" {
		t.Fatalf("旧区间的时点没解析回 v1：err=%v found=%v rule=%s", err, foundEarly, early)
	}
	late, foundLate, err := store.LoadInterpretationRule(t.Context(),
		tenant, domain.ReleaseResultLayer, jurisdiction, configBaseAt.Add(72*time.Hour))
	if err != nil || !foundLate || late.String() != "interpret/release/v2" {
		t.Fatalf("新区间的时点没解析到 v2：err=%v found=%v rule=%s", err, foundLate, late)
	}
}

// 起点早于既有开放版的登记是对历史区间的追改：写口折成`已登记`、按请求起点读不回
// 版本，编排判冲突——登记按生效起点升序进行，错序不静默落成任何一版。
func TestABackdatedOpenRegistrationIsAConflictNotAQuietBackfill(t *testing.T) {
	store := newRuleStore()
	handler := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Rules: store, RuleView: store,
	})

	if _, err := handler.RegisterInterpretationRule(t.Context(),
		ruleCommand(t, domain.ReleaseResultLayer, "interpret/release/v2")); err != nil {
		t.Fatalf("登记 v2：%v", err)
	}
	backdated := ruleCommand(t, domain.ReleaseResultLayer, "interpret/release/v1")
	backdated.AppliesFrom = configBaseAt.Add(-48 * time.Hour)
	outcome, err := handler.RegisterInterpretationRule(t.Context(), backdated)
	if err != nil || outcome != application.ConfigurationContentConflict {
		t.Fatalf("错序登记该是`内容冲突`：err=%v outcome=%v", err, outcome)
	}
}

// 封闭六层之外的层在本层就被挡下，不下沉到写口。
func TestAnUnknownResultLayerIsNotAccepted(t *testing.T) {
	store := newRuleStore()
	handler := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Rules: store, RuleView: store,
	})
	outcome, err := handler.RegisterInterpretationRule(t.Context(),
		ruleCommand(t, domain.ResultLayerInvalid, "interpret/x"))
	if err != nil || outcome != application.ConfigurationNotAccepted {
		t.Fatalf("非法结果层该是`未接受`：err=%v outcome=%v", err, outcome)
	}
}

type obligationStoreDouble struct {
	catalogs map[string]bool
	items    map[string][]ports.ObligationRegistration
}

func newObligationStore() *obligationStoreDouble {
	return &obligationStoreDouble{
		catalogs: map[string]bool{},
		items:    map[string][]ports.ObligationRegistration{},
	}
}

func (double *obligationStoreDouble) RegisterObligationCatalog(
	_ context.Context, tenant domain.TenantID, caseRef domain.CustomsCaseID, _ time.Time,
) (ports.CaseConfigurationSaveOutcome, error) {
	key := tenant.String() + "|" + caseRef.String()
	if double.catalogs[key] {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	double.catalogs[key] = true
	return ports.CaseConfigurationRegistered, nil
}

func (double *obligationStoreDouble) RegisterObligationItem(
	_ context.Context, tenant domain.TenantID, caseRef domain.CustomsCaseID, registration ports.ObligationRegistration,
) (ports.CaseConfigurationSaveOutcome, error) {
	key := tenant.String() + "|" + caseRef.String()
	for _, existing := range double.items[key] {
		if existing.Item.Obligation == registration.Item.Obligation {
			return ports.CaseConfigurationAlreadyRegistered, nil
		}
	}
	double.items[key] = append(double.items[key], registration)
	return ports.CaseConfigurationRegistered, nil
}

// LoadObligationItems 照真库读口的半开区间过滤，否则「换了区间」那一格测不出来。
func (double *obligationStoreDouble) LoadObligationItems(
	_ context.Context, tenant domain.TenantID, caseRef domain.CustomsCaseID, cutoffAt time.Time,
) ([]domain.ClosureObligationItem, bool, error) {
	key := tenant.String() + "|" + caseRef.String()
	if !double.catalogs[key] {
		return nil, false, nil
	}
	items := make([]domain.ClosureObligationItem, 0)
	for _, registration := range double.items[key] {
		if registration.AppliesFrom.After(cutoffAt) {
			continue
		}
		if !registration.AppliesUntil.IsZero() && !registration.AppliesUntil.After(cutoffAt) {
			continue
		}
		items = append(items, registration.Item)
	}
	return items, true, nil
}

func obligationItemCommand(t *testing.T, state domain.ObligationItemState, from time.Time) application.RegisterObligationItemCommand {
	t.Helper()
	return application.RegisterObligationItemCommand{
		TenantID: configValue(t, domain.NewTenantID, "tenant-a"),
		CaseRef:  configValue(t, domain.NewCustomsCaseID, "case-1"),
		Registration: ports.ObligationRegistration{
			Item: domain.ClosureObligationItem{
				Obligation: "DUTY/PAYMENT",
				Scope:      "case-1",
				State:      state,
				Basis:      "PROGRAM/DDP",
			},
			AppliesFrom: from,
		},
	}
}

// 目录行只表达在场，没有可比内容——重复登记因此永远是`已存在`，不可能冲突。
func TestReRegisteringAnObligationCatalogIsAlwaysExisting(t *testing.T) {
	store := newObligationStore()
	handler := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Obligations: store, ObligationView: store,
	})
	command := application.RegisterObligationCatalogCommand{
		TenantID: configValue(t, domain.NewTenantID, "tenant-a"),
		CaseRef:  configValue(t, domain.NewCustomsCaseID, "case-1"), RegisteredAt: configBaseAt,
	}

	if _, err := handler.RegisterObligationCatalog(t.Context(), command); err != nil {
		t.Fatalf("首次登记目录：%v", err)
	}
	// 换个登记时间再登一次：目录仍只是「在场」，不该判成冲突。
	command.RegisteredAt = configBaseAt.Add(time.Hour)
	outcome, err := handler.RegisterObligationCatalog(t.Context(), command)
	if err != nil || outcome != application.ConfigurationExisting {
		t.Fatalf("重复登记目录该是`已存在`：err=%v outcome=%v", err, outcome)
	}
}

func TestReRegisteringAnObligationItemWithADifferentStateConflicts(t *testing.T) {
	store := newObligationStore()
	handler := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Obligations: store, ObligationView: store,
	})
	if _, err := handler.RegisterObligationCatalog(t.Context(), application.RegisterObligationCatalogCommand{
		TenantID: configValue(t, domain.NewTenantID, "tenant-a"),
		CaseRef:  configValue(t, domain.NewCustomsCaseID, "case-1"), RegisteredAt: configBaseAt,
	}); err != nil {
		t.Fatalf("登记目录：%v", err)
	}

	if _, err := handler.RegisterObligationItem(t.Context(),
		obligationItemCommand(t, domain.ObligationUnresolved, configBaseAt)); err != nil {
		t.Fatalf("首次登记义务：%v", err)
	}
	outcome, err := handler.RegisterObligationItem(t.Context(),
		obligationItemCommand(t, domain.ObligationConcluded, configBaseAt))
	if err != nil || outcome != application.ConfigurationContentConflict {
		t.Fatalf("换状态该是`内容冲突`：err=%v outcome=%v", err, outcome)
	}

	same, err := handler.RegisterObligationItem(t.Context(),
		obligationItemCommand(t, domain.ObligationUnresolved, configBaseAt))
	if err != nil || same != application.ConfigurationExisting {
		t.Fatalf("重放同一项该是`已存在`：err=%v outcome=%v", err, same)
	}
}

// 适用区间也是登记内容：同一项义务换了起点，按新起点盘点时在册那份的身影对不上，
// 仍是冲突而不是重放。这一格若判成重放，改区间就会被静默吞掉。
func TestReRegisteringAnObligationItemWithADifferentIntervalConflicts(t *testing.T) {
	store := newObligationStore()
	handler := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Obligations: store, ObligationView: store,
	})
	if _, err := handler.RegisterObligationCatalog(t.Context(), application.RegisterObligationCatalogCommand{
		TenantID: configValue(t, domain.NewTenantID, "tenant-a"),
		CaseRef:  configValue(t, domain.NewCustomsCaseID, "case-1"), RegisteredAt: configBaseAt,
	}); err != nil {
		t.Fatalf("登记目录：%v", err)
	}

	if _, err := handler.RegisterObligationItem(t.Context(),
		obligationItemCommand(t, domain.ObligationUnresolved, configBaseAt.Add(10*time.Hour))); err != nil {
		t.Fatalf("首次登记义务：%v", err)
	}
	// 新起点早于在册那份的起点：按新起点盘点，在册那项还没生效，盘不出来。
	outcome, err := handler.RegisterObligationItem(t.Context(),
		obligationItemCommand(t, domain.ObligationUnresolved, configBaseAt))
	if err != nil || outcome != application.ConfigurationContentConflict {
		t.Fatalf("换区间起点该是`内容冲突`：err=%v outcome=%v", err, outcome)
	}
}

// gateConfigStoreDouble 与 verify_release_gate_test.go 里的 gateStoreDouble 不是一
// 回事：那个装的是门禁核对结果，这个装的是前置条件目录与逐项判断的登记内容。
type gateConfigStoreDouble struct {
	catalogs map[string]bool
	findings map[string][]domain.PreconditionFinding
}

func newGateConfigStore() *gateConfigStoreDouble {
	return &gateConfigStoreDouble{
		catalogs: map[string]bool{},
		findings: map[string][]domain.PreconditionFinding{},
	}
}

func gateConfigKey(tenant domain.TenantID, scope domain.DecisionScopeReference,
	action domain.GuardedAction, boundary domain.CustomsProcedureReference) string {
	return tenant.String() + "|" + scope.String() + "|" + action.String() + "|" + boundary.String()
}

func (double *gateConfigStoreDouble) RegisterGateCatalog(
	_ context.Context, tenant domain.TenantID, scope domain.DecisionScopeReference,
	action domain.GuardedAction, boundary domain.CustomsProcedureReference, _ time.Time,
) (ports.CaseConfigurationSaveOutcome, error) {
	key := gateConfigKey(tenant, scope, action, boundary)
	if double.catalogs[key] {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	double.catalogs[key] = true
	return ports.CaseConfigurationRegistered, nil
}

func (double *gateConfigStoreDouble) RegisterGateFinding(
	_ context.Context, tenant domain.TenantID, scope domain.DecisionScopeReference,
	action domain.GuardedAction, boundary domain.CustomsProcedureReference,
	finding domain.PreconditionFinding,
) (ports.CaseConfigurationSaveOutcome, error) {
	key := gateConfigKey(tenant, scope, action, boundary)
	for _, existing := range double.findings[key] {
		if existing.Precondition == finding.Precondition {
			return ports.CaseConfigurationAlreadyRegistered, nil
		}
	}
	double.findings[key] = append(double.findings[key], finding)
	return ports.CaseConfigurationRegistered, nil
}

func (double *gateConfigStoreDouble) LoadPreconditionFindings(
	_ context.Context, tenant domain.TenantID, scope domain.DecisionScopeReference,
	action domain.GuardedAction, boundary domain.CustomsProcedureReference,
) ([]domain.PreconditionFinding, bool, error) {
	key := gateConfigKey(tenant, scope, action, boundary)
	if !double.catalogs[key] {
		return nil, false, nil
	}
	return double.findings[key], true, nil
}

func gateCatalogCommand(t *testing.T) application.RegisterGateCatalogCommand {
	t.Helper()
	return application.RegisterGateCatalogCommand{
		TenantID:     configValue(t, domain.NewTenantID, "tenant-a"),
		Scope:        configValue(t, domain.NewDecisionScopeReference, "case-1/unit-1"),
		Action:       domain.OutboundRelease,
		Boundary:     configValue(t, domain.NewCustomsProcedureReference, "EXPORT/GENERAL"),
		RegisteredAt: configBaseAt,
	}
}

func gateFindingCommand(t *testing.T, state domain.PreconditionState) application.RegisterGateFindingCommand {
	t.Helper()
	return application.RegisterGateFindingCommand{
		TenantID: configValue(t, domain.NewTenantID, "tenant-a"),
		Scope:    configValue(t, domain.NewDecisionScopeReference, "case-1/unit-1"),
		Action:   domain.OutboundRelease,
		Boundary: configValue(t, domain.NewCustomsProcedureReference, "EXPORT/GENERAL"),
		Finding: domain.PreconditionFinding{
			Precondition: configValue(t, domain.NewPreconditionReference, "DUTY/SETTLED"),
			State:        state,
		},
	}
}

// 同一前置条件换判断是冲突：门禁判断绑定动作与边界（CONTEXT「不能复用于其他动作或监管边界」），改判断要走复核而不
// 是把原判断顶掉。
func TestReRegisteringAGateFindingWithADifferentStateConflicts(t *testing.T) {
	store := newGateConfigStore()
	handler := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Gates: store, GateView: store,
	})
	if _, err := handler.RegisterGateCatalog(t.Context(), gateCatalogCommand(t)); err != nil {
		t.Fatalf("登记门禁目录：%v", err)
	}

	if _, err := handler.RegisterGateFinding(t.Context(), gateFindingCommand(t, domain.PreconditionMet)); err != nil {
		t.Fatalf("首次登记判断：%v", err)
	}
	outcome, err := handler.RegisterGateFinding(t.Context(), gateFindingCommand(t, domain.PreconditionUnmet))
	if err != nil || outcome != application.ConfigurationContentConflict {
		t.Fatalf("换判断该是`内容冲突`：err=%v outcome=%v", err, outcome)
	}

	same, err := handler.RegisterGateFinding(t.Context(), gateFindingCommand(t, domain.PreconditionMet))
	if err != nil || same != application.ConfigurationExisting {
		t.Fatalf("重放同一判断该是`已存在`：err=%v outcome=%v", err, same)
	}
}

// 只登目录不登任何前置条件，是「此动作在此边界本就不受门禁」的如实登记，必须登得出来。
func TestAGateCatalogAloneIsRegisteredAndFoldsToNotApplicable(t *testing.T) {
	store := newGateConfigStore()
	handler := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Gates: store, GateView: store,
	})

	outcome, err := handler.RegisterGateCatalog(t.Context(), gateCatalogCommand(t))
	if err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("登记门禁目录：err=%v outcome=%v", err, outcome)
	}

	command := gateCatalogCommand(t)
	findings, configured, err := store.LoadPreconditionFindings(t.Context(),
		command.TenantID, command.Scope, command.Action, command.Boundary)
	if err != nil || !configured {
		t.Fatalf("登了目录却答未配置：err=%v configured=%v", err, configured)
	}
	conclusion, err := domain.FoldGateConclusion(findings)
	if err != nil || conclusion != domain.GateNotApplicable {
		t.Fatalf("空清单没折成`不适用`：err=%v conclusion=%v", err, conclusion)
	}
}

// 非法受管动作在本层挡下，不下沉到写口。
func TestAnUnknownGuardedActionIsNotAccepted(t *testing.T) {
	store := newGateConfigStore()
	handler := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Gates: store, GateView: store,
	})
	command := gateCatalogCommand(t)
	command.Action = domain.GuardedActionInvalid

	outcome, err := handler.RegisterGateCatalog(t.Context(), command)
	if err != nil || outcome != application.ConfigurationNotAccepted {
		t.Fatalf("非法动作该是`未接受`：err=%v outcome=%v", err, outcome)
	}
}
