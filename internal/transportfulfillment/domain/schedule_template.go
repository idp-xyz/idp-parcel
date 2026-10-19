package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

var (
	ErrInvalidScheduleTemplate = errors.New("transport fulfillment: invalid schedule template")
	// ErrScheduleTemplateNotRunning：周期安排在这一运行日期不开——星期不在它的集合里，或日期落在生效区间外。
	// 与 ErrInvalidScheduleTemplate 分开，是因为续办不同：前者换一个日期，后者改登记内容。
	ErrScheduleTemplateNotRunning = errors.New("transport fulfillment: the schedule template does not run on that date")
)

// ScheduleTemplateCode 指名一份周期安排。代码在租户内稳定，版本另记：具体班次的身份由（代码、运行日期）派生，
// 代码一旦随版本变，同一班就会有两个身份。
type ScheduleTemplateCode struct{ requiredValue }

func NewScheduleTemplateCode(value string) (ScheduleTemplateCode, error) {
	required, err := newRequiredValue("schedule template code", value)
	return ScheduleTemplateCode{required}, err
}

// ScheduleTemplateVersion 是周期安排在登记册上的版本序号，从 1 起；理由同 ScheduleVersion。
type ScheduleTemplateVersion int

func (version ScheduleTemplateVersion) valid() bool {
	return version >= 1
}

// RunningDate 是班次的当地运行日期：周期安排按它物化，具体班次的身份由（周期安排代码、运行日期）派生。它是日历
// 日期不是时刻——同一日期在不同时区是不同的出发时刻，时区归周期安排。
type RunningDate struct {
	midnight time.Time
}

const runningDateLayout = "2006-01-02"

// ParseRunningDate 只认 YYYY-MM-DD。宽松认法（带时刻、带时区）会让同一个运行日期有两种写法，派生出两个班次身份。
func ParseRunningDate(value string) (RunningDate, error) {
	parsed, err := time.Parse(runningDateLayout, value)
	if err != nil || parsed.Format(runningDateLayout) != value {
		return RunningDate{}, fmt.Errorf("%w: running date %q is not YYYY-MM-DD", ErrInvalidScheduleTemplate, value)
	}
	return RunningDate{midnight: parsed}, nil
}

func runningDateIn(at time.Time, location *time.Location) RunningDate {
	local := at.In(location)
	return RunningDate{midnight: time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)}
}

func (date RunningDate) valid() bool {
	return !date.midnight.IsZero()
}

func (date RunningDate) String() string {
	if !date.valid() {
		return ""
	}
	return date.midnight.Format(runningDateLayout)
}

func (date RunningDate) addDays(days int) RunningDate {
	return RunningDate{midnight: date.midnight.AddDate(0, 0, days)}
}

func (date RunningDate) before(other RunningDate) bool {
	return date.midnight.Before(other.midnight)
}

func (date RunningDate) weekday() time.Weekday {
	return date.midnight.Weekday()
}

// at 把运行日期上的当地分钟数折成绝对时刻。夏令时跳过或重复的那一刻按 time.Date 的规范化落到其中一侧：同一
// （日期、分钟、时区）总落在同一处，物化因此仍是确定的。
func (date RunningDate) at(minuteOfDay int, location *time.Location) time.Time {
	return time.Date(date.midnight.Year(), date.midnight.Month(), date.midnight.Day(),
		minuteOfDay/60, minuteOfDay%60, 0, 0, location).UTC()
}

var scheduleWeekdayWords = map[time.Weekday]string{
	time.Monday:    "MON",
	time.Tuesday:   "TUE",
	time.Wednesday: "WED",
	time.Thursday:  "THU",
	time.Friday:    "FRI",
	time.Saturday:  "SAT",
	time.Sunday:    "SUN",
}

// ParseScheduleWeekday 认星期的封闭词（MON…SUN）。词取 ISO 8601 的英文缩写，不认数字：周日是 0 还是 7 各家
// 写法不一，词没有这个歧义。
func ParseScheduleWeekday(word string) (time.Weekday, error) {
	for weekday, known := range scheduleWeekdayWords {
		if known == word {
			return weekday, nil
		}
	}
	return time.Sunday, fmt.Errorf("%w: weekday %q", ErrInvalidScheduleTemplate, word)
}

// ScheduleWeekdayWord 是 ParseScheduleWeekday 的反方向。
func ScheduleWeekdayWord(weekday time.Weekday) string {
	return scheduleWeekdayWords[weekday]
}

// isoWeekdayOrder 让星期集合按周一到周日排：存与比都按同一顺序，同一集合只有一种写法。
func isoWeekdayOrder(weekday time.Weekday) int {
	if weekday == time.Sunday {
		return 7
	}
	return int(weekday)
}

// ParseLocalTimeOfDay 认 HH:MM（00:00–23:59），交回当日分钟数。
func ParseLocalTimeOfDay(value string) (int, error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil || parsed.Format("15:04") != value {
		return 0, fmt.Errorf("%w: local time of day %q is not HH:MM", ErrInvalidScheduleTemplate, value)
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

// LocalTimeOfDay 是 ParseLocalTimeOfDay 的反方向。
func LocalTimeOfDay(minuteOfDay int) string {
	return fmt.Sprintf("%02d:%02d", minuteOfDay/60, minuteOfDay%60)
}

// ScheduleTemplateContent 是周期安排一版的全部正文，即 ADR-0201 决定三内置的那一种文法：按周循环（星期集合）、
// 当地出发时刻、历时、IANA 时区、方向、适用服务范围与运行日期的生效区间。生效区间终点不含，缺席即不设终点。
//
// 到达用历时表达，不收到站当地时刻：起讫可能跨时区，按历时算出的到达时刻只有一种读法。
type ScheduleTemplateContent struct {
	Direction            ScheduleDirection
	Weekdays             []time.Weekday
	DepartureLocalMinute int
	Duration             time.Duration
	TimeZone             string
	ServiceScope         ScheduleServiceScopeReference
	EffectiveFrom        RunningDate
	EffectiveUntil       RunningDate
}

// ScheduleTemplate 是 TF 自有的周期安排：运行规律，不是班次（CONTEXT「周期运行安排或车次模板不是具体班次」）。
// 它不出现在任何订舱、分配或移动里——那些只引用由它按日期物化出来的具体班次。
type ScheduleTemplate struct {
	tenantID TenantID
	code     ScheduleTemplateCode
	version  ScheduleTemplateVersion
	content  ScheduleTemplateContent
	location *time.Location
}

type ScheduleTemplateSpec struct {
	TenantID TenantID
	Code     ScheduleTemplateCode
	Content  ScheduleTemplateContent
}

// RegisterScheduleTemplate 立一份周期安排的首版。
func RegisterScheduleTemplate(spec ScheduleTemplateSpec) (ScheduleTemplate, error) {
	return formScheduleTemplate(spec, 1)
}

// RehydrateScheduleTemplate 是持久化适配器读回某一版的门：与首版同过一遍正文校验，只是版本取读回的那一个。
func RehydrateScheduleTemplate(spec ScheduleTemplateSpec, version ScheduleTemplateVersion) (ScheduleTemplate, error) {
	return formScheduleTemplate(spec, version)
}

// Revise 以新正文形成下一版。代码不变（理由见 ScheduleTemplateCode）；已物化、已登记的具体班次不随之改写——它们
// 是登记册上自己的版本，要跟上新的一版就按同一（代码、日期）再物化一次（ADR-0201 决定四）。
func (template ScheduleTemplate) Revise(content ScheduleTemplateContent) (ScheduleTemplate, error) {
	return formScheduleTemplate(ScheduleTemplateSpec{
		TenantID: template.tenantID,
		Code:     template.code,
		Content:  content,
	}, template.version+1)
}

func formScheduleTemplate(spec ScheduleTemplateSpec, version ScheduleTemplateVersion) (ScheduleTemplate, error) {
	content := spec.Content
	if !spec.TenantID.valid() || !spec.Code.valid() || !version.valid() ||
		!content.Direction.valid() || !content.ServiceScope.valid() {
		return ScheduleTemplate{}, ErrInvalidScheduleTemplate
	}
	if content.DepartureLocalMinute < 0 || content.DepartureLocalMinute >= 24*60 {
		return ScheduleTemplate{}, ErrInvalidScheduleTemplate
	}
	if content.Duration <= 0 || content.Duration%time.Minute != 0 {
		return ScheduleTemplate{}, ErrInvalidScheduleTemplate
	}
	if !content.EffectiveFrom.valid() ||
		(content.EffectiveUntil.valid() && !content.EffectiveFrom.before(content.EffectiveUntil)) {
		return ScheduleTemplate{}, ErrInvalidScheduleTemplate
	}
	weekdays, err := normalizedWeekdays(content.Weekdays)
	if err != nil {
		return ScheduleTemplate{}, err
	}
	location, err := scheduleLocation(content.TimeZone)
	if err != nil {
		return ScheduleTemplate{}, err
	}
	content.Weekdays = weekdays
	return ScheduleTemplate{
		tenantID: spec.TenantID,
		code:     spec.Code,
		version:  version,
		content:  content,
		location: location,
	}, nil
}

// normalizedWeekdays 去重并按周一到周日排；空集合拒——一天都不开的周期安排物化不出任何班次，登它只会让该连接
// 看上去「已配置」。
func normalizedWeekdays(weekdays []time.Weekday) ([]time.Weekday, error) {
	seen := map[time.Weekday]bool{}
	normalized := make([]time.Weekday, 0, len(weekdays))
	for _, weekday := range weekdays {
		if _, known := scheduleWeekdayWords[weekday]; !known {
			return nil, ErrInvalidScheduleTemplate
		}
		if seen[weekday] {
			continue
		}
		seen[weekday] = true
		normalized = append(normalized, weekday)
	}
	if len(normalized) == 0 {
		return nil, ErrInvalidScheduleTemplate
	}
	slices.SortFunc(normalized, func(left, right time.Weekday) int {
		return isoWeekdayOrder(left) - isoWeekdayOrder(right)
	})
	return normalized, nil
}

// scheduleLocation 只收 IANA 时区名。空串与 Local 都被 time.LoadLocation 接受，却分别读成 UTC 与进程所在机器
// 的时区——同一份周期安排在两台机器上物化出两个时刻，所以拒。
func scheduleLocation(name string) (*time.Location, error) {
	if strings.TrimSpace(name) == "" || name == "Local" {
		return nil, ErrInvalidScheduleTemplate
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("%w: time zone %q: %v", ErrInvalidScheduleTemplate, name, err)
	}
	return location, nil
}

func (template ScheduleTemplate) TenantID() TenantID {
	return template.tenantID
}

func (template ScheduleTemplate) Code() ScheduleTemplateCode {
	return template.code
}

func (template ScheduleTemplate) Version() ScheduleTemplateVersion {
	return template.version
}

// Content 交回正文的一份拷贝：星期集合是切片，交出去的不能回头改到周期安排本体。
func (template ScheduleTemplate) Content() ScheduleTemplateContent {
	content := template.content
	content.Weekdays = append([]time.Weekday(nil), template.content.Weekdays...)
	return content
}

// RunsOn 判这一运行日期开不开：星期在集合里，且日期落在生效区间内。
func (template ScheduleTemplate) RunsOn(date RunningDate) bool {
	if !date.valid() || date.before(template.content.EffectiveFrom) {
		return false
	}
	if template.content.EffectiveUntil.valid() && !date.before(template.content.EffectiveUntil) {
		return false
	}
	return slices.Contains(template.content.Weekdays, date.weekday())
}

// MaterializeSchedule 把周期安排在一个运行日期上物化成具体班次（AT-TF-027）。同一（周期安排、日期）总得出同一个
// 班次：身份由（代码、日期）派生，正文由这一版正文与日期算出，不读时钟、不读登记册——物化发生在准备评估候选与
// 显式登记两处（ADR-0201 决定五），两处得出的必须是同一班。
func MaterializeSchedule(template ScheduleTemplate, date RunningDate) (TransportSchedule, error) {
	if !template.RunsOn(date) {
		return TransportSchedule{}, ErrScheduleTemplateNotRunning
	}
	departsAt := date.at(template.content.DepartureLocalMinute, template.location)
	return formSchedule(TransportScheduleSpec{
		TenantID:     template.tenantID,
		Schedule:     materializedScheduleReference(template.code, date),
		Direction:    template.content.Direction,
		DepartsAt:    departsAt,
		ArrivesAt:    departsAt.Add(template.content.Duration),
		ServiceScope: template.content.ServiceScope,
	}, 1, ScheduleOrigin{
		kind:            ScheduleMaterializedFromTemplate,
		template:        template.code,
		templateVersion: template.version,
		runningDate:     date,
	})
}

// MaterializeDepartingWithin 物化出发时刻落在 [from, to] 内的全部班次，按出发先后排。它是准备评估候选那一处物化
// （ADR-0201 决定五），只算不存：没被选中的候选不进登记册。
//
// 枚举的日期比区间两端在当地各宽一天，再按出发时刻筛：夏令时的规范化可能把某个日期的出发推过当地午夜，只按
// 区间两端的当地日期枚举会漏掉它。
func (template ScheduleTemplate) MaterializeDepartingWithin(from, to time.Time) []TransportSchedule {
	if from.IsZero() || to.IsZero() || to.Before(from) {
		return nil
	}
	first := runningDateIn(from, template.location).addDays(-1)
	last := runningDateIn(to, template.location).addDays(1)
	if first.before(template.content.EffectiveFrom) {
		first = template.content.EffectiveFrom
	}
	if until := template.content.EffectiveUntil; until.valid() && !last.before(until) {
		last = until.addDays(-1)
	}
	var schedules []TransportSchedule
	for date := first; !last.before(date); date = date.addDays(1) {
		schedule, err := MaterializeSchedule(template, date)
		if err != nil {
			continue
		}
		if schedule.departsAt.Before(from) || schedule.departsAt.After(to) {
			continue
		}
		schedules = append(schedules, schedule)
	}
	return schedules
}

// materializedScheduleSeparator 隔开周期安排代码与运行日期。取 `/` 而不取 `@`：NR 的版本引用写作 `代码@版本`，
// 班次标识若也用 `@`，读的人会把运行日期读成版本。
const materializedScheduleSeparator = "/"

// materializedScheduleReference 由（代码、运行日期）派生班次身份。两次物化同一（代码、日期）得同一身份，是登记
// 幂等与候选去重的前提。
func materializedScheduleReference(code ScheduleTemplateCode, date RunningDate) ScheduleReference {
	return ScheduleReference{requiredValue{value: code.String() + materializedScheduleSeparator + date.String()}}
}

// parseMaterializedScheduleReference 认出「代码/YYYY-MM-DD」形的身份。临时班次据此被挡在这一形之外。
func parseMaterializedScheduleReference(value string) (ScheduleTemplateCode, RunningDate, bool) {
	index := strings.LastIndex(value, materializedScheduleSeparator)
	if index <= 0 {
		return ScheduleTemplateCode{}, RunningDate{}, false
	}
	code, err := NewScheduleTemplateCode(value[:index])
	if err != nil {
		return ScheduleTemplateCode{}, RunningDate{}, false
	}
	date, err := ParseRunningDate(value[index+len(materializedScheduleSeparator):])
	if err != nil {
		return ScheduleTemplateCode{}, RunningDate{}, false
	}
	return code, date, true
}
