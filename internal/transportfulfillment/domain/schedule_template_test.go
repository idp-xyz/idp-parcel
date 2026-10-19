package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 夹具全为合成值（S 级）：连接 CONN 自 A 至 B，周期安排 TPL 在 Asia/Shanghai 当地 22:00 出发、历时 600 分钟。

func directionFor(t *testing.T) domain.ScheduleDirection {
	t.Helper()
	direction, err := domain.NewScheduleDirection("CONN", "A", "B")
	if err != nil {
		t.Fatalf("方向：%v", err)
	}
	return direction
}

func mustRunningDate(t *testing.T, value string) domain.RunningDate {
	t.Helper()
	date, err := domain.ParseRunningDate(value)
	if err != nil {
		t.Fatalf("运行日期 %q：%v", value, err)
	}
	return date
}

func templateContentFor(t *testing.T, weekdays []time.Weekday, effectiveUntil string) domain.ScheduleTemplateContent {
	t.Helper()
	content := domain.ScheduleTemplateContent{
		Direction:            directionFor(t),
		Weekdays:             weekdays,
		DepartureLocalMinute: 22 * 60,
		Duration:             600 * time.Minute,
		TimeZone:             "Asia/Shanghai",
		ServiceScope:         mustValue(t, domain.NewScheduleServiceScopeReference, "SCOPE"),
		EffectiveFrom:        mustRunningDate(t, "2026-01-01"),
	}
	if effectiveUntil != "" {
		content.EffectiveUntil = mustRunningDate(t, effectiveUntil)
	}
	return content
}

func everyDay() []time.Weekday {
	return []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday}
}

func registeredTemplate(t *testing.T, content domain.ScheduleTemplateContent) domain.ScheduleTemplate {
	t.Helper()
	template, err := domain.RegisterScheduleTemplate(domain.ScheduleTemplateSpec{
		TenantID: mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		Code:     mustValue(t, domain.NewScheduleTemplateCode, "TPL"),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("登周期安排：%v", err)
	}
	return template
}

func materializedFor(t *testing.T, date string) domain.TransportSchedule {
	t.Helper()
	schedule, err := domain.MaterializeSchedule(registeredTemplate(t, templateContentFor(t, everyDay(), "")), mustRunningDate(t, date))
	if err != nil {
		t.Fatalf("物化 %s：%v", date, err)
	}
	return schedule
}

func adHocScheduleFor(t *testing.T, identity string, departsAt, arrivesAt time.Time) domain.TransportSchedule {
	t.Helper()
	schedule, err := domain.FormTransportSchedule(domain.TransportScheduleSpec{
		TenantID:     mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		Schedule:     mustValue(t, domain.NewScheduleReference, identity),
		Direction:    directionFor(t),
		DepartsAt:    departsAt,
		ArrivesAt:    arrivesAt,
		ServiceScope: mustValue(t, domain.NewScheduleServiceScopeReference, "SCOPE"),
	})
	if err != nil {
		t.Fatalf("临时班次：%v", err)
	}
	return schedule
}

// Covers: `AT-TF-027`「周期模板在具体日期生成一次运输机会 → 形成独立班次身份，模板仍独立存在」与 ADR-0201 决定一：
// 同一（周期安排、运行日期）物化两次得同一班——身份由（代码、日期）派生，出发按当地时刻与时区折成绝对时刻，到达
// 是出发加历时；物化不改周期安排本体。
func TestMaterializingATemplateOnADateIsDeterministic(t *testing.T) {
	template := registeredTemplate(t, templateContentFor(t, everyDay(), ""))
	date := mustRunningDate(t, "2026-03-04")

	first, err := domain.MaterializeSchedule(template, date)
	if err != nil {
		t.Fatalf("物化：%v", err)
	}
	second, err := domain.MaterializeSchedule(template, date)
	if err != nil {
		t.Fatalf("再物化：%v", err)
	}
	if first != second {
		t.Fatalf("同一（周期安排、日期）物化出两班：%+v / %+v", first, second)
	}
	if first.Schedule().String() != "TPL/2026-03-04" || first.Version() != 1 {
		t.Fatalf("身份 = %s v%d，要 TPL/2026-03-04 v1", first.Schedule(), first.Version())
	}
	wantDeparts := time.Date(2026, 3, 4, 14, 0, 0, 0, time.UTC)
	if !first.DepartsAt().Equal(wantDeparts) || !first.ArrivesAt().Equal(wantDeparts.Add(10*time.Hour)) {
		t.Fatalf("出发 %s 到达 %s，要当地 22:00 起、历时十小时", first.DepartsAt(), first.ArrivesAt())
	}
	code, version, runningDate, materialized := first.Origin().Template()
	if !materialized || code.String() != "TPL" || version != 1 || runningDate != date {
		t.Fatalf("来历 = %v %s v%d %s", materialized, code, version, runningDate)
	}
	if template.Version() != 1 || len(template.Content().Weekdays) != 7 {
		t.Fatal("物化改动了周期安排本体")
	}
}

// Covers: ADR-0201 决定三——星期不在集合、日期在生效区间外（终点不含）都不开，答 ErrScheduleTemplateNotRunning，
// 不物化一个不存在的班次。
func TestATemplateRunsOnlyOnItsWeekdaysWithinItsInterval(t *testing.T) {
	template := registeredTemplate(t, templateContentFor(t, []time.Weekday{time.Wednesday}, "2026-03-11"))
	for name, date := range map[string]string{
		"周四":       "2026-03-05",
		"生效起点之前":   "2025-12-31",
		"生效终点当天不含": "2026-03-11",
	} {
		if _, err := domain.MaterializeSchedule(template, mustRunningDate(t, date)); !errors.Is(err, domain.ErrScheduleTemplateNotRunning) {
			t.Fatalf("%s（%s）：err = %v，要 ErrScheduleTemplateNotRunning", name, date, err)
		}
	}
	if _, err := domain.MaterializeSchedule(template, mustRunningDate(t, "2026-03-04")); err != nil {
		t.Fatalf("区间内的周三应当开：%v", err)
	}
}

// Covers: ADR-0201 决定三的文法边界——每一格坏法各被构造门拒；星期集合去重并按周一到周日排。
func TestTemplateContentIsGated(t *testing.T) {
	broken := map[string]func(*domain.ScheduleTemplateContent){
		"空星期":        func(content *domain.ScheduleTemplateContent) { content.Weekdays = nil },
		"出发分钟越界":     func(content *domain.ScheduleTemplateContent) { content.DepartureLocalMinute = 24 * 60 },
		"历时为零":       func(content *domain.ScheduleTemplateContent) { content.Duration = 0 },
		"历时不足整分":     func(content *domain.ScheduleTemplateContent) { content.Duration = 90 * time.Second },
		"空时区":        func(content *domain.ScheduleTemplateContent) { content.TimeZone = "" },
		"Local 不是时区": func(content *domain.ScheduleTemplateContent) { content.TimeZone = "Local" },
		"认不出的时区":     func(content *domain.ScheduleTemplateContent) { content.TimeZone = "SYN/Nowhere" },
		"终点不晚于起点": func(content *domain.ScheduleTemplateContent) {
			content.EffectiveUntil = content.EffectiveFrom
		},
		"缺方向": func(content *domain.ScheduleTemplateContent) { content.Direction = domain.ScheduleDirection{} },
	}
	for name, breakContent := range broken {
		t.Run(name, func(t *testing.T) {
			content := templateContentFor(t, everyDay(), "")
			breakContent(&content)
			_, err := domain.RegisterScheduleTemplate(domain.ScheduleTemplateSpec{
				TenantID: mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
				Code:     mustValue(t, domain.NewScheduleTemplateCode, "TPL"),
				Content:  content,
			})
			if !errors.Is(err, domain.ErrInvalidScheduleTemplate) {
				t.Fatalf("err = %v，要 ErrInvalidScheduleTemplate", err)
			}
		})
	}
	if _, err := domain.NewScheduleDirection("CONN", "A", "A"); !errors.Is(err, domain.ErrInvalidScheduleDirection) {
		t.Fatalf("起讫相同：err = %v", err)
	}

	template := registeredTemplate(t, templateContentFor(t, []time.Weekday{time.Sunday, time.Monday, time.Sunday}, ""))
	if got := template.Content().Weekdays; len(got) != 2 || got[0] != time.Monday || got[1] != time.Sunday {
		t.Fatalf("星期 = %v，要 [周一 周日]", got)
	}
	for _, value := range []string{"2026-3-4", "2026-03-04T00:00:00Z", " 2026-03-04"} {
		if _, err := domain.ParseRunningDate(value); err == nil {
			t.Fatalf("运行日期 %q 应拒——同一日期只许一种写法", value)
		}
	}
	for _, value := range []string{"7:00", "24:00", "07:00:00"} {
		if _, err := domain.ParseLocalTimeOfDay(value); err == nil {
			t.Fatalf("当地时刻 %q 应拒", value)
		}
	}
}

// Covers: ADR-0201 决定四——周期安排换版代码不变、版本加一，已物化的班次不随之改写；按新的一版再物化同一日期，
// 身份不变、正文随新版。
func TestTemplateRevisionKeepsTheCodeAndRematerializesTheSameIdentity(t *testing.T) {
	template := registeredTemplate(t, templateContentFor(t, everyDay(), ""))
	date := mustRunningDate(t, "2026-03-04")
	before, err := domain.MaterializeSchedule(template, date)
	if err != nil {
		t.Fatalf("物化：%v", err)
	}

	content := template.Content()
	content.DepartureLocalMinute = 23 * 60
	revised, err := template.Revise(content)
	if err != nil {
		t.Fatalf("换版：%v", err)
	}
	if revised.Code() != template.Code() || revised.Version() != 2 {
		t.Fatalf("换版后 %s v%d", revised.Code(), revised.Version())
	}
	after, err := domain.MaterializeSchedule(revised, date)
	if err != nil {
		t.Fatalf("按新版物化：%v", err)
	}
	if after.Schedule() != before.Schedule() || !after.DepartsAt().Equal(before.DepartsAt().Add(time.Hour)) {
		t.Fatalf("同一日期按新版物化：%s %s，要同一身份晚一小时", after.Schedule(), after.DepartsAt())
	}
	if _, version, _, _ := after.Origin().Template(); version != 2 {
		t.Fatalf("出处版本 = %d，要 2", version)
	}
}

// Covers: ADR-0201 决定五——临时班次不得取「代码/日期」形的身份；班次下一版身份与来历种类不变、版本加一。
func TestAdHocSchedulesStayOutOfTheMaterializedIdentityShape(t *testing.T) {
	departs := time.Date(2026, 3, 5, 2, 0, 0, 0, time.UTC)
	_, err := domain.FormTransportSchedule(domain.TransportScheduleSpec{
		TenantID:     mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		Schedule:     mustValue(t, domain.NewScheduleReference, "TPL/2026-03-04"),
		Direction:    directionFor(t),
		DepartsAt:    departs,
		ArrivesAt:    departs.Add(time.Hour),
		ServiceScope: mustValue(t, domain.NewScheduleServiceScopeReference, "SCOPE"),
	})
	if !errors.Is(err, domain.ErrInvalidTransportSchedule) {
		t.Fatalf("临时班次冒用物化身份：err = %v", err)
	}

	adHoc := adHocScheduleFor(t, "SYN-ADHOC-1", departs, departs.Add(time.Hour))
	if adHoc.Origin().Kind() != domain.ScheduleAdHoc || adHoc.Version() != 1 {
		t.Fatalf("临时班次 = %s v%d", adHoc.Origin().Kind(), adHoc.Version())
	}
	delayed := adHocScheduleFor(t, "SYN-ADHOC-1", departs.Add(time.Hour), departs.Add(2*time.Hour))
	revised, err := adHoc.Revise(delayed)
	if err != nil || revised.Version() != 2 || !revised.DepartsAt().Equal(departs.Add(time.Hour)) {
		t.Fatalf("下一版 = v%d %s，%v", revised.Version(), revised.DepartsAt(), err)
	}
	if _, err := adHoc.Revise(adHocScheduleFor(t, "SYN-ADHOC-2", departs, departs.Add(time.Hour))); !errors.Is(err, domain.ErrInvalidTransportSchedule) {
		t.Fatalf("换身份不是下一版：err = %v", err)
	}
	if _, err := adHoc.Revise(materializedFor(t, "2026-03-04")); !errors.Is(err, domain.ErrInvalidTransportSchedule) {
		t.Fatalf("换来历不是下一版：err = %v", err)
	}
	if !adHocScheduleFor(t, "SYN-ADHOC-3", departs, departs.Add(time.Minute)).ArrivesAt().After(departs) {
		t.Fatal("到达应晚于出发")
	}
	_, err = domain.FormTransportSchedule(domain.TransportScheduleSpec{
		TenantID:     mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		Schedule:     mustValue(t, domain.NewScheduleReference, "SYN-ADHOC-4"),
		Direction:    directionFor(t),
		DepartsAt:    departs,
		ArrivesAt:    departs,
		ServiceScope: mustValue(t, domain.NewScheduleServiceScopeReference, "SCOPE"),
	})
	if !errors.Is(err, domain.ErrInvalidTransportSchedule) {
		t.Fatalf("到达不晚于出发：err = %v", err)
	}
}

// Covers: ADR-0028 与 ADR-0201——读回一版班次时身份与来历必须对得上：物化来的身份必须正是（代码、日期）派生的那
// 一个，临时班次身份不得是那一形。
func TestRehydrationChecksTheIdentityAgainstTheOrigin(t *testing.T) {
	materialized := materializedFor(t, "2026-03-04")
	spec := domain.RehydrateTransportScheduleSpec{
		Schedule: domain.TransportScheduleSpec{
			TenantID:     materialized.TenantID(),
			Schedule:     materialized.Schedule(),
			Direction:    materialized.Direction(),
			DepartsAt:    materialized.DepartsAt(),
			ArrivesAt:    materialized.ArrivesAt(),
			ServiceScope: materialized.ServiceScope(),
		},
		Version:         3,
		OriginKind:      domain.ScheduleMaterializedFromTemplate,
		Template:        mustValue(t, domain.NewScheduleTemplateCode, "TPL"),
		TemplateVersion: 2,
		RunningDate:     mustRunningDate(t, "2026-03-04"),
	}
	if rehydrated, err := domain.RehydrateTransportSchedule(spec); err != nil || rehydrated.Version() != 3 {
		t.Fatalf("读回 = v%d，%v", rehydrated.Version(), err)
	}
	wrongDate := spec
	wrongDate.RunningDate = mustRunningDate(t, "2026-03-05")
	if _, err := domain.RehydrateTransportSchedule(wrongDate); !errors.Is(err, domain.ErrInvalidTransportSchedule) {
		t.Fatalf("身份与运行日期对不上：err = %v", err)
	}
	adHocShaped := spec
	adHocShaped.OriginKind = domain.ScheduleAdHoc
	adHocShaped.Template = domain.ScheduleTemplateCode{}
	adHocShaped.TemplateVersion = 0
	adHocShaped.RunningDate = domain.RunningDate{}
	if _, err := domain.RehydrateTransportSchedule(adHocShaped); !errors.Is(err, domain.ErrInvalidTransportSchedule) {
		t.Fatalf("临时班次顶着物化身份：err = %v", err)
	}
}

// Covers: ADR-0201 决定五——准备评估候选那一处物化只交出发时刻落在区间内的班次，按出发先后；生效区间之外的日期不出。
func TestMaterializingWithinAWindowYieldsOnlyDeparturesInside(t *testing.T) {
	template := registeredTemplate(t, templateContentFor(t, everyDay(), "2026-03-06"))
	from := time.Date(2026, 3, 3, 15, 0, 0, 0, time.UTC) // 当地 3 月 3 日 23:00，已过当日 22:00
	to := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	got := template.MaterializeDepartingWithin(from, to)
	var identities []string
	for _, schedule := range got {
		identities = append(identities, schedule.Schedule().String())
	}
	want := []string{"TPL/2026-03-04", "TPL/2026-03-05"}
	if len(identities) != len(want) || identities[0] != want[0] || identities[1] != want[1] {
		t.Fatalf("区间内的班次 = %v，要 %v", identities, want)
	}
	if template.MaterializeDepartingWithin(to, from) != nil {
		t.Fatal("倒置区间应当一班都没有")
	}
}
