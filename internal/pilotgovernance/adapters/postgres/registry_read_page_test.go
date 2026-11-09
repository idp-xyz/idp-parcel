package postgres_test

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件对真实 PostgreSQL 证治理登记册四册按 ADR-0144 取页（票 catalogue-read-pagination-rollout/12）。四册同形，共用
// 一组表驱动用例、每册各过一遍。权威区间的登记时间是库给的 now()，这里直写 SQL 才排得出时刻；其余各册的时刻是领域
// 输入，经写侧仓储登记。夹具全是 SYN- 合成值。

// registerUnderTest 是一册在表驱动用例里的样子：seed 按序号与排序时刻登一行，序号进行标识（pageKey），list 取一页并把
// 行写成同一个键，用例按键比次序。
type registerUnderTest struct {
	catalogue *cataloguepage.Catalogue
	sortField string
	prefix    string
	seed      func(t *testing.T, fixture *registryReadFixture, index int, at time.Time)
	list      func(ctx context.Context, fixture *registryReadFixture, limit int, query cataloguepage.Query) (keyedPage, error)
}

type keyedPage struct {
	keys  []string
	next  string
	total int64
}

func keysOf[Row any](page cataloguepage.Result[Row], err error, key func(Row) string) (keyedPage, error) {
	if err != nil {
		return keyedPage{}, err
	}
	keys := make([]string, 0, len(page.Rows))
	for _, row := range page.Rows {
		keys = append(keys, key(row))
	}
	return keyedPage{keys: keys, next: page.Next, total: page.Total}, nil
}

func pageKey(prefix string, index int) string {
	return fmt.Sprintf("%sP%02d", prefix, index)
}

func pageKeys(prefix string, indexes ...int) []string {
	keys := make([]string, 0, len(indexes))
	for _, index := range indexes {
		keys = append(keys, pageKey(prefix, index))
	}
	return keys
}

func pageInventory(t *testing.T, takenAt time.Time) domain.InTransitInventory {
	t.Helper()
	inventory, err := domain.TakeInventory([]domain.InventoryEntry{{
		ObjectIdentity:   "SYN-OBJ-PAGE",
		CurrentFacts:     "SYN-FACTS/page",
		CurrentAuthority: "SYN-AUTH/page",
		ResponsibleParty: "SYN-DUTY/page",
		NextAction:       "SYN-NEXT/page",
		ReviewBy:         takenAt.Add(24 * time.Hour),
	}}, takenAt)
	if err != nil {
		t.Fatalf("盘点：%v", err)
	}
	return inventory
}

// pageTakeover 是一条接管登记：对象范围由调用方给，区间起点固定，终点为零即开放区间。
func pageTakeover(t *testing.T, scope string, to, effectiveAt time.Time) domain.TakeoverRecord {
	t.Helper()
	record, err := domain.RecordTakeover(domain.TakeoverRecordSpec{
		StopEvidence: "SYN-EVIDENCE/writer-stopped",
		Interval: domain.AuthorityInterval{
			ObjectScope: scope,
			Capability:  "SYN-CAP/shipment-intake",
			FactKind:    "SYN-FACT/acceptance-decision",
			Authority:   "SYN-AUTH/successor",
			From:        registryReadAt,
			To:          to,
		},
		AcceptedFacts:    "SYN-FACTS/cutover",
		PendingExternals: "SYN-EXT/none",
		ActualControl:    "SYN-CTRL/ops-desk",
		Responsibilities: "SYN-DUTY/ops-owner",
		NextAction:       "SYN-NEXT/hold",
		Inventory:        pageInventory(t, effectiveAt.Add(-time.Hour)),
		EffectiveAt:      effectiveAt,
	})
	if err != nil {
		t.Fatalf("形成接管：%v", err)
	}
	return record
}

func governanceRegistersUnderTest() map[string]registerUnderTest {
	return map[string]registerUnderTest{
		"authority-interval": {
			catalogue: ports.AuthorityIntervalRegistryPages,
			sortField: "insertedAt",
			prefix:    "SYN-SCOPE/",
			seed: func(t *testing.T, fixture *registryReadFixture, index int, at time.Time) {
				t.Helper()
				if _, err := fixture.pool.Exec(t.Context(),
					`INSERT INTO pilot_governance.authority_interval
						(object_scope, capability, fact_kind, authority, from_at, inserted_at)
					 VALUES ($1, 'SYN-CAP/route-planning', 'SYN-FACT/route-plan', 'SYN-AUTH/pilot-engine', $2, $3)`,
					pageKey("SYN-SCOPE/", index), registryReadAt, at); err != nil {
					t.Fatalf("登区间 %d：%v", index, err)
				}
			},
			list: func(ctx context.Context, fixture *registryReadFixture, limit int, query cataloguepage.Query) (keyedPage, error) {
				page, err := fixture.registers.ListAuthorityIntervals(ctx, limit, query)
				return keysOf(page, err, func(row ports.AuthorityIntervalRegistryRow) string { return row.ObjectScope })
			},
		},
		"suspension": {
			catalogue: ports.SuspensionRegistryPages,
			sortField: "occurredAt",
			prefix:    "SYN-GOV-SUS-",
			seed: func(t *testing.T, fixture *registryReadFixture, index int, at time.Time) {
				t.Helper()
				decision := registrySuspension(t, pageKey("SYN-GOV-SUS-", index), "SYN-PILOT-SCOPE@v3", at)
				fixture.write(t, t.Context(), func(txCtx context.Context) error {
					_, err := fixture.suspensions.Save(txCtx, decision)
					return err
				})
			},
			list: func(ctx context.Context, fixture *registryReadFixture, limit int, query cataloguepage.Query) (keyedPage, error) {
				page, err := fixture.registers.ListSuspensions(ctx, limit, query)
				return keysOf(page, err, func(row ports.SuspensionRegistryRow) string { return row.SuspensionID })
			},
		},
		"resumption": {
			catalogue: ports.ResumptionRegistryPages,
			sortField: "decidedAt",
			prefix:    "SYN-GOV-SUS-",
			seed: func(t *testing.T, fixture *registryReadFixture, index int, at time.Time) {
				t.Helper()
				id := pageKey("SYN-GOV-SUS-", index)
				suspension := registrySuspension(t, id, "SYN-PILOT-SCOPE@v3", at.Add(-24*time.Hour))
				resumption, err := domain.RecordResumption(domain.ResumptionDecisionSpec{
					Suspension:       govRef(t, domain.NewSuspensionID, id),
					ReleaseEvidence:  "SYN-EVIDENCE/release",
					ConsistencyCheck: "SYN-CHECK/ledger-consistent",
					Inventory:        pageInventory(t, at.Add(-time.Hour)),
					DecidedBy:        "SYN-GOV-OPERATOR",
					DecidedAt:        at,
					EffectiveAt:      at.Add(time.Hour),
				})
				if err != nil {
					t.Fatalf("形成恢复：%v", err)
				}
				fixture.write(t, t.Context(), func(txCtx context.Context) error {
					if _, err := fixture.suspensions.Save(txCtx, suspension); err != nil {
						return err
					}
					_, err := fixture.resumptions.Save(txCtx, resumption)
					return err
				})
			},
			list: func(ctx context.Context, fixture *registryReadFixture, limit int, query cataloguepage.Query) (keyedPage, error) {
				page, err := fixture.registers.ListResumptions(ctx, limit, query)
				return keysOf(page, err, func(row ports.ResumptionRegistryRow) string { return row.SuspensionID })
			},
		},
		"takeover": {
			catalogue: ports.TakeoverRegistryPages,
			sortField: "effectiveAt",
			prefix:    "SYN-SCOPE/",
			seed: func(t *testing.T, fixture *registryReadFixture, index int, at time.Time) {
				t.Helper()
				record := pageTakeover(t, pageKey("SYN-SCOPE/", index), time.Time{}, at)
				fixture.write(t, t.Context(), func(txCtx context.Context) error {
					_, err := fixture.takeovers.Save(txCtx, record)
					return err
				})
			},
			list: func(ctx context.Context, fixture *registryReadFixture, limit int, query cataloguepage.Query) (keyedPage, error) {
				page, err := fixture.registers.ListTakeovers(ctx, limit, query)
				return keysOf(page, err, func(row ports.TakeoverRegistryRow) string { return row.ObjectScope })
			},
		},
	}
}

// Covers: ADR-0144 决定一、五——四册各按「排序时刻 + 行标识」的游标翻页：空册答空页、total 为 0；翻完第一页后在已翻过那头
// 与还没翻到那头各登一行，第二页不重出第一页的行（偏移分页会在第二页开头重出 P04），新登在后头的那一行也不漏；total 随之
// 5 → 7，末页 next 为空。
func TestGovernanceRegistersPageByCursorWithoutRepeatsOrGapsWhileRowsAreRegistered(t *testing.T) {
	for name, register := range governanceRegistersUnderTest() {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			fixture := newRegistryReadFixture(t)
			list := func(query cataloguepage.Query) keyedPage {
				t.Helper()
				page, err := register.list(ctx, fixture, 2, query)
				if err != nil {
					t.Fatal(err)
				}
				return page
			}

			if empty := list(firstPage(t, register.catalogue)); len(empty.keys) != 0 || empty.total != 0 || empty.next != "" {
				t.Fatalf("空册：%+v", empty)
			}
			for index := 1; index <= 5; index++ {
				register.seed(t, fixture, index, registryReadAt.Add(time.Duration(index)*time.Hour))
			}

			first := list(firstPage(t, register.catalogue))
			if !reflect.DeepEqual(first.keys, pageKeys(register.prefix, 5, 4)) || first.total != 5 {
				t.Fatalf("第一页：%+v", first)
			}
			register.seed(t, fixture, 6, registryReadAt.Add(6*time.Hour))
			register.seed(t, fixture, 0, registryReadAt)

			second := list(nextPage(t, register.catalogue, url.Values{}, first.next))
			if !reflect.DeepEqual(second.keys, pageKeys(register.prefix, 3, 2)) || second.total != 7 {
				t.Fatalf("第二页：%+v", second)
			}
			last := list(nextPage(t, register.catalogue, url.Values{}, second.next))
			if !reflect.DeepEqual(last.keys, pageKeys(register.prefix, 1, 0)) || last.total != 7 || last.next != "" {
				t.Fatalf("末页：%+v", last)
			}
		})
	}
}

// Covers: ADR-0144 决定三——同一时刻的几行按行标识与排序同向收尾，跨页续得上：倒序（缺省）时同刻的按标识倒序，正序时按
// 标识正序。
func TestGovernanceRegistersBreakTiesInTheSortDirection(t *testing.T) {
	for name, register := range governanceRegistersUnderTest() {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			fixture := newRegistryReadFixture(t)
			for index := 1; index <= 4; index++ {
				register.seed(t, fixture, index, registryReadAt)
			}
			register.seed(t, fixture, 5, registryReadAt.Add(time.Hour+500*time.Microsecond))

			for values, want := range map[string][]string{
				"":                       pageKeys(register.prefix, 5, 4, 3, 2, 1),
				"-" + register.sortField: pageKeys(register.prefix, 5, 4, 3, 2, 1),
				register.sortField:       pageKeys(register.prefix, 1, 2, 3, 4, 5),
			} {
				query := url.Values{}
				if values != "" {
					query.Set("sort", values)
				}
				var walked []string
				page, err := register.list(ctx, fixture, 2, pageQuery(t, register.catalogue, query))
				for turned := 0; ; turned++ {
					if err != nil {
						t.Fatal(err)
					}
					if page.total != 5 || turned > 5 {
						t.Fatalf("sort=%q 第 %d 页：%+v", values, turned+1, page)
					}
					walked = append(walked, page.keys...)
					if page.next == "" {
						break
					}
					page, err = register.list(ctx, fixture, 2, nextPage(t, register.catalogue, query, page.next))
				}
				if !reflect.DeepEqual(walked, want) {
					t.Fatalf("sort=%q 走完全册：得 %v\nwant %v", values, walked, want)
				}
			}
		})
	}
}

// Covers: ADR-0144 决定四、五——q 在各册声明的列上做不分大小写的字面包含（% 与 _ 按字面、零命中），total 与本页出自同一组
// 条件。
func TestGovernanceRegistersTotalFollowsTheKeyword(t *testing.T) {
	for name, register := range governanceRegistersUnderTest() {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			fixture := newRegistryReadFixture(t)
			for index := 1; index <= 3; index++ {
				register.seed(t, fixture, index, registryReadAt.Add(time.Duration(index)*time.Hour))
			}
			for keyword, want := range map[string][]string{
				"":    pageKeys(register.prefix, 3, 2, 1),
				"p02": pageKeys(register.prefix, 2),
				"%":   {},
				"P_2": {},
			} {
				query := url.Values{}
				if keyword != "" {
					query.Set("q", keyword)
				}
				page, err := register.list(ctx, fixture, 10, pageQuery(t, register.catalogue, query))
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(page.keys, want) || page.total != int64(len(want)) || page.next != "" {
					t.Fatalf("q=%q：%+v，应为 %v", keyword, page, want)
				}
			}
		})
	}
}

// Covers: ports.TakeoverRegistryPages 头注——接管册行标识里「区间终点，开放区间取起点」那一段把一开一闭的两行分开：区间
// 四维与起点都相同的这样两行都在册，按一行一页翻，倒序时闭区间在前、正序时开放区间在前，各只出现一次。
func TestTakeoverRegistryKeepsOpenAndClosedTwinsApart(t *testing.T) {
	ctx := t.Context()
	fixture := newRegistryReadFixture(t)
	effectiveAt := registryReadAt.Add(2 * time.Hour)
	for _, record := range []domain.TakeoverRecord{
		pageTakeover(t, "SYN-SCOPE/twin", time.Time{}, effectiveAt),
		pageTakeover(t, "SYN-SCOPE/twin", registryReadAt.Add(48*time.Hour), effectiveAt),
	} {
		fixture.write(t, ctx, func(txCtx context.Context) error {
			_, err := fixture.takeovers.Save(txCtx, record)
			return err
		})
	}

	for sort, wantOpen := range map[string][]bool{"-effectiveAt": {false, true}, "effectiveAt": {true, false}} {
		values := url.Values{"sort": {sort}}
		var open []bool
		page, err := fixture.registers.ListTakeovers(ctx, 1, pageQuery(t, ports.TakeoverRegistryPages, values))
		for turned := 0; ; turned++ {
			if err != nil {
				t.Fatal(err)
			}
			if page.Total != 2 || turned > 2 {
				t.Fatalf("sort=%s 第 %d 页：%+v", sort, turned+1, page)
			}
			for _, row := range page.Rows {
				open = append(open, !row.HasToAt)
			}
			if page.Next == "" {
				break
			}
			page, err = fixture.registers.ListTakeovers(ctx, 1, nextPage(t, ports.TakeoverRegistryPages, values, page.Next))
		}
		if !reflect.DeepEqual(open, wantOpen) {
			t.Fatalf("sort=%s 逐页的「开放」：得 %v，want %v", sort, open, wantOpen)
		}
	}
}

// Covers: ADR-0144 决定四「有状态的册至少放出状态一维」——权威区间按 open、暂停按 resumed 筛出的行，读回的终点在场与否、
// Resumed 与所给的词一致，total 随这一维变、与 q 为与。恢复的生效时点落在将来也算有恢复决定：这一维说的是登记，不说此刻。
func TestGovernanceStatusDimensionsAgreeWithTheAnswerFields(t *testing.T) {
	ctx := t.Context()
	fixture := newRegistryReadFixture(t)
	for index, closed := range map[int]bool{1: false, 2: true, 3: false} {
		var to any
		if closed {
			to = registryReadAt.Add(24 * time.Hour)
		}
		if _, err := fixture.pool.Exec(ctx,
			`INSERT INTO pilot_governance.authority_interval
				(object_scope, capability, fact_kind, authority, from_at, to_at, inserted_at)
			 VALUES ($1, 'SYN-CAP/route-planning', 'SYN-FACT/route-plan', 'SYN-AUTH/pilot-engine', $2, $3, $4)`,
			pageKey("SYN-SCOPE/", index), registryReadAt, to, registryReadAt.Add(time.Duration(index)*time.Hour)); err != nil {
			t.Fatalf("登区间 %d：%v", index, err)
		}
	}
	for index := 1; index <= 3; index++ {
		id := pageKey("SYN-GOV-SUS-", index)
		at := registryReadAt.Add(time.Duration(index) * time.Hour)
		suspension := registrySuspension(t, id, "SYN-PILOT-SCOPE@v3", at)
		fixture.write(t, ctx, func(txCtx context.Context) error {
			_, err := fixture.suspensions.Save(txCtx, suspension)
			return err
		})
		if index != 2 {
			continue
		}
		resumption, err := domain.RecordResumption(domain.ResumptionDecisionSpec{
			Suspension:       govRef(t, domain.NewSuspensionID, id),
			ReleaseEvidence:  "SYN-EVIDENCE/release",
			ConsistencyCheck: "SYN-CHECK/ledger-consistent",
			Inventory:        pageInventory(t, at.Add(time.Hour)),
			DecidedBy:        "SYN-GOV-OPERATOR",
			DecidedAt:        at.Add(2 * time.Hour),
			EffectiveAt:      at.Add(365 * 24 * time.Hour),
		})
		if err != nil {
			t.Fatalf("形成恢复：%v", err)
		}
		fixture.write(t, ctx, func(txCtx context.Context) error {
			_, err := fixture.resumptions.Save(txCtx, resumption)
			return err
		})
	}

	type statusCase struct {
		values url.Values
		want   []string
	}
	for name, test := range map[string]statusCase{
		"不带条件":       {values: url.Values{}, want: pageKeys("SYN-SCOPE/", 3, 2, 1)},
		"开放":         {values: url.Values{"open": {"true"}}, want: pageKeys("SYN-SCOPE/", 3, 1)},
		"已有终点":       {values: url.Values{"open": {"false"}}, want: pageKeys("SYN-SCOPE/", 2)},
		"维内为或":       {values: url.Values{"open": {"true", "false"}}, want: pageKeys("SYN-SCOPE/", 3, 2, 1)},
		"开放且 q 命中":   {values: url.Values{"open": {"true"}, "q": {"p01"}}, want: pageKeys("SYN-SCOPE/", 1)},
		"已有终点且 q 不中": {values: url.Values{"open": {"false"}, "q": {"p01"}}, want: []string{}},
	} {
		t.Run("权威区间/"+name, func(t *testing.T) {
			page, err := fixture.registers.ListAuthorityIntervals(ctx, 10, pageQuery(t, ports.AuthorityIntervalRegistryPages, test.values))
			if err != nil {
				t.Fatal(err)
			}
			keys := make([]string, 0, len(page.Rows))
			for _, row := range page.Rows {
				keys = append(keys, row.ObjectScope)
				if words := test.values["open"]; len(words) == 1 && row.HasToAt == (words[0] == "true") {
					t.Fatalf("按 open=%s 筛出的 %s 读回终点在场 %v", words[0], row.ObjectScope, row.HasToAt)
				}
			}
			if !reflect.DeepEqual(keys, test.want) || page.Total != int64(len(test.want)) {
				t.Fatalf("得 %v / total %d，应为 %v / total %d", keys, page.Total, test.want, len(test.want))
			}
		})
	}
	for name, test := range map[string]statusCase{
		"不带条件":       {values: url.Values{}, want: pageKeys("SYN-GOV-SUS-", 3, 2, 1)},
		"有恢复决定":      {values: url.Values{"resumed": {"true"}}, want: pageKeys("SYN-GOV-SUS-", 2)},
		"没有恢复决定":     {values: url.Values{"resumed": {"false"}}, want: pageKeys("SYN-GOV-SUS-", 3, 1)},
		"没有恢复且 q 命中": {values: url.Values{"resumed": {"false"}, "q": {"p03"}}, want: pageKeys("SYN-GOV-SUS-", 3)},
	} {
		t.Run("暂停/"+name, func(t *testing.T) {
			page, err := fixture.registers.ListSuspensions(ctx, 10, pageQuery(t, ports.SuspensionRegistryPages, test.values))
			if err != nil {
				t.Fatal(err)
			}
			keys := make([]string, 0, len(page.Rows))
			for _, row := range page.Rows {
				keys = append(keys, row.SuspensionID)
				if words := test.values["resumed"]; len(words) == 1 && row.Resumed != (words[0] == "true") {
					t.Fatalf("按 resumed=%s 筛出的 %s 读回 Resumed=%v", words[0], row.SuspensionID, row.Resumed)
				}
			}
			if !reflect.DeepEqual(keys, test.want) || page.Total != int64(len(test.want)) {
				t.Fatalf("得 %v / total %d，应为 %v / total %d", keys, page.Total, test.want, len(test.want))
			}
		})
	}
}
