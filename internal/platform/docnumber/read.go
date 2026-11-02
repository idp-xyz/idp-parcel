package docnumber

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.idp.xyz/idp-platform-go/numbering"
)

// previewScrambleSecret 是样例号与预览里加扰流水用的键。样例只示号的形状，不用租户的真密钥：用真密钥渲染出的
// 样例就是某个周期第一个真号的加扰值，读口与预览口没理由把它交给每个有查阅读授予的人。
const previewScrambleSecret = "document-numbering-preview"

// sampleSequence 是样例号用的流水：一个周期的第一个号。
const sampleSequence = 1

// ReaderDeps 是读口与预览口的依赖。
type ReaderDeps struct {
	Catalogue *Catalogue
	Book      RuleBook
	Secrets   ScrambleSecrets
	Clock     Clock
}

// Reader 答规则册的读与预览（ADR-0355 决定六）。两口都只读规则册、不读也不占序列。
type Reader struct {
	catalogue *Catalogue
	book      RuleBook
	secrets   ScrambleSecrets
	clock     Clock
}

func NewReader(deps ReaderDeps) (*Reader, error) {
	if deps.Catalogue == nil || deps.Book == nil || deps.Secrets == nil || deps.Clock == nil {
		return nil, errors.New("document numbering: reader needs a catalogue, a rule book, scramble secrets and a clock")
	}
	return &Reader{catalogue: deps.Catalogue, book: deps.Book, secrets: deps.Secrets, clock: deps.Clock}, nil
}

// RegisterView 是一个租户此刻的规则册。
type RegisterView struct {
	Tenant string
	At     time.Time
	// NumberingTimeZone 是此刻生效的单据编号时区；没有任何一版生效时为 nil，即「单据编号时区未配置」。
	NumberingTimeZone         *TimeZoneVersion
	NumberingTimeZoneVersions []TimeZoneVersion
	Types                     []TypeView
}

// TypeView 是一类单据此刻的规则。
type TypeView struct {
	Declaration TypeDeclaration
	Source      RuleSource
	// Template 是此刻生效的模板原文。
	Template string
	// EffectiveVersion 是此刻生效的租户版本；用产品默认时为 nil。
	EffectiveVersion *RuleVersion
	ResetPeriod      ResetPeriod
	// Sample 是按生效模板、单据编号时区与样例流水渲染的样例号；单据编号时区未配置时为空——没有时区就没有日期，
	// 拿 UTC 渲染一个样例等于在页面上替租户选了时区。开立方有业务时区的类型，样例按单据编号时区渲染，只示形状。
	Sample string
	// Versions 是该租户这一类的历次规则版本，按生效时刻排序。
	Versions []RuleVersion
}

// Read 答一个租户此刻的规则册：每类单据此刻生效的规则（租户版或产品默认）、日期所用时区与历次版本。
func (reader *Reader) Read(ctx context.Context, tenant string) (RegisterView, error) {
	if strings.TrimSpace(tenant) == "" {
		return RegisterView{}, errors.New("document numbering: read needs a tenant")
	}
	now := reader.clock.Now()
	view := RegisterView{Tenant: tenant, At: now}

	zones, err := reader.book.TimeZoneVersions(ctx, tenant)
	if err != nil {
		return RegisterView{}, fmt.Errorf("document numbering: read numbering time zones: %w", err)
	}
	view.NumberingTimeZoneVersions = zones
	zone, found, err := reader.book.EffectiveTimeZoneVersion(ctx, tenant, now)
	if err != nil {
		return RegisterView{}, fmt.Errorf("document numbering: read numbering time zone: %w", err)
	}
	var location *time.Location
	if found {
		view.NumberingTimeZone = &zone
		if location, err = loadTimeZone(zone.TimeZone); err != nil {
			return RegisterView{}, fmt.Errorf("document numbering: registered numbering time zone no longer loads: %w", err)
		}
	}

	versions, err := reader.book.RuleVersions(ctx, tenant)
	if err != nil {
		return RegisterView{}, fmt.Errorf("document numbering: read rule versions: %w", err)
	}
	byType := map[TypeCode][]RuleVersion{}
	for _, version := range versions {
		byType[version.Type] = append(byType[version.Type], version)
	}

	for _, declaration := range reader.catalogue.Declarations() {
		raw, source, err := effectiveTemplate(ctx, reader.catalogue, reader.book, tenant, declaration.Code, now)
		if err != nil {
			return RegisterView{}, err
		}
		typeView := TypeView{Declaration: declaration, Source: source, Template: raw, Versions: byType[declaration.Code]}
		if source == SourceTenantRule {
			effective, _ := latestEffective(typeView.Versions, now)
			typeView.EffectiveVersion = effective
		}
		checked, err := checkTemplate(declaration.Code, raw)
		if err != nil {
			return RegisterView{}, fmt.Errorf("document numbering: effective template of %s no longer passes the template check: %w", declaration.Code, err)
		}
		typeView.ResetPeriod = resetPeriodOf(checked.template)
		if location != nil {
			sample, err := render(checked.template, declaration.Code, tenant, previewScrambleSecret, sampleSequence, now.In(location), zeroDigits)
			if err != nil {
				return RegisterView{}, fmt.Errorf("document numbering: render sample of %s: %w", declaration.Code, err)
			}
			typeView.Sample = sample
		}
		view.Types = append(view.Types, typeView)
	}
	return view, nil
}

// latestEffective 在已排序的版本里找 at 时刻生效的那一版，与 RuleBook.EffectiveRuleVersion 同一判法。
func latestEffective(versions []RuleVersion, at time.Time) (*RuleVersion, bool) {
	var found *RuleVersion
	for index := range versions {
		if !versions[index].EffectiveFrom.After(at) {
			found = &versions[index]
		}
	}
	return found, found != nil
}

// PreviewRequest 是一次预览：给定类型与模板，按样例流水渲染一个号。
type PreviewRequest struct {
	Tenant   string
	Type     TypeCode
	Template string
	// TimeZone 是预览用的 IANA 时区；空则取租户此刻的单据编号时区。
	TimeZone string
	// At 是样例时刻；零值取此刻。登记新规则带生效时刻时传它，样例就是生效那一刻的号。
	At time.Time
}

// PreviewOutcome 是预览的答案。
type PreviewOutcome string

const (
	// PreviewRendered：渲染出了样例。
	PreviewRendered PreviewOutcome = "PREVIEW_RENDERED"
	// PreviewRejected：这份模板（或给的时区）登记会被拒，拒因在 Preview.Rejection。
	PreviewRejected PreviewOutcome = "PREVIEW_REJECTED"
	// PreviewTimeZoneNotConfigured：模板可登，但没给时区、租户也没有单据编号时区，渲染不出带日期的样例。
	PreviewTimeZoneNotConfigured PreviewOutcome = "NUMBERING_TIME_ZONE_NOT_CONFIGURED"
)

// Preview 是预览的结果。
type Preview struct {
	Outcome PreviewOutcome
	// Rejection 是拒因哨兵（模板检查各格、ErrTypeNotDeclared、ErrScrambleSecretNotConfigured、ErrTimeZoneNotRecognized）。
	Rejection   error
	Sample      string
	TimeZone    string
	At          time.Time
	ResetPeriod ResetPeriod
	// Overlaps 是这一类单据此前用过的模板里，与新模板可能渲染出同一个号的那几份（ADR-0355 决定八：登记新版时
	// 预览提示可能与已用号段重叠）。
	Overlaps []Overlap
}

// Overlap 指一份可能重叠的早先模板。EffectiveFrom 在产品默认上为零值。
type Overlap struct {
	Source        RuleSource
	Template      string
	EffectiveFrom time.Time
}

// Preview 按样例流水渲染一份模板，不读也不占序列，并提示与此前用过的模板可能重叠。
//
// 重叠是提示，不是判决：它报的是「在样例时刻，这两份模板按形状能出同一个号」（见 overlaps），不知道那个号是不是
// 真被用过（预览不读单据）。真撞上时由取号编排跳过（决定八），这里只让登记的人在换格式之前看见。
func (reader *Reader) Preview(ctx context.Context, request PreviewRequest) (Preview, error) {
	if strings.TrimSpace(request.Tenant) == "" {
		return Preview{}, errors.New("document numbering: preview needs a tenant")
	}
	if _, declared := reader.catalogue.Declaration(request.Type); !declared {
		return Preview{Outcome: PreviewRejected, Rejection: ErrTypeNotDeclared}, nil
	}
	checked, err := checkTemplate(request.Type, request.Template)
	if err != nil {
		return Preview{Outcome: PreviewRejected, Rejection: RejectionReason(err)}, nil
	}
	if checked.scrambled {
		if _, configured := reader.secrets.ScrambleSecret(request.Tenant); !configured {
			return Preview{Outcome: PreviewRejected, Rejection: ErrScrambleSecretNotConfigured}, nil
		}
	}
	preview := Preview{ResetPeriod: resetPeriodOf(checked.template)}

	now := reader.clock.Now()
	location, zone, err := reader.previewTimeZone(ctx, request, now)
	switch {
	case errors.Is(err, ErrTimeZoneNotRecognized):
		return Preview{Outcome: PreviewRejected, Rejection: ErrTimeZoneNotRecognized}, nil
	case err != nil:
		return Preview{}, err
	case location == nil:
		preview.Outcome = PreviewTimeZoneNotConfigured
		return preview, nil
	}
	at := request.At
	if at.IsZero() {
		at = now
	}
	at = at.In(location)
	preview.TimeZone, preview.At = zone, at

	sample, err := render(checked.template, request.Type, request.Tenant, previewScrambleSecret, sampleSequence, at, zeroDigits)
	if err != nil {
		return Preview{}, fmt.Errorf("document numbering: render preview: %w", err)
	}
	preview.Outcome, preview.Sample = PreviewRendered, sample

	overlaps, err := reader.overlaps(ctx, request, checked.template, at)
	if err != nil {
		return Preview{}, err
	}
	preview.Overlaps = overlaps
	return preview, nil
}

func (reader *Reader) previewTimeZone(ctx context.Context, request PreviewRequest, now time.Time) (*time.Location, string, error) {
	if request.TimeZone != "" {
		location, err := loadTimeZone(request.TimeZone)
		if err != nil {
			return nil, "", err
		}
		return location, request.TimeZone, nil
	}
	version, found, err := reader.book.EffectiveTimeZoneVersion(ctx, request.Tenant, now)
	if err != nil {
		return nil, "", fmt.Errorf("document numbering: read numbering time zone: %w", err)
	}
	if !found {
		return nil, "", nil
	}
	location, err := loadTimeZone(version.TimeZone)
	if err != nil {
		return nil, "", fmt.Errorf("document numbering: registered numbering time zone no longer loads: %w", err)
	}
	return location, version.TimeZone, nil
}

// overlaps 拿新模板与这一类此前用过的每一份模板（产品默认与该租户的历次版本）比。与新模板逐字相同的那份不报：
// 同一份模板共用同一条序列（序列键不含模板），流水只往前走，出不了重号。
//
// 不逐个流水渲染去碰：撞号常在十万量级的流水上才出现（按日改按月、日期段与流水段错位），逐个渲染到那里一次预览要
// 几十兆内存。改为按位比形状：每份模板按流水位数分段，每段渲染几个探针号，逐位取字符的上下界；两份模板有某两段
// 等长且每一位的区间都有交集，就可能出同一个号。按日与按月这种不同重置周期的模板，各自的流水从 1 数起，任意两段
// 都要比；重置周期相同的两份模板共用一条序列，同一位数段若逐个探针都渲染得一样，那一段两边是同一个函数，同号必
// 同流水，不会撞，跳过。加扰流水与随机段只有探针那几个取值，区间可能偏窄，宁可少报——撞上了取号编排照样跳过。
func (reader *Reader) overlaps(ctx context.Context, request PreviewRequest, candidate *numbering.Template, at time.Time) ([]Overlap, error) {
	declaration, _ := reader.catalogue.Declaration(request.Type)
	earlier := []Overlap{{Source: SourceProductDefault, Template: declaration.DefaultTemplate}}
	versions, err := reader.book.RuleVersions(ctx, request.Tenant)
	if err != nil {
		return nil, fmt.Errorf("document numbering: read rule versions: %w", err)
	}
	for _, version := range versions {
		if version.Type == request.Type {
			earlier = append(earlier, Overlap{Source: SourceTenantRule, Template: version.Template, EffectiveFrom: version.EffectiveFrom})
		}
	}

	candidateShapes, err := shapesOf(candidate, request.Type, request.Tenant, at)
	if err != nil {
		return nil, fmt.Errorf("document numbering: render preview: %w", err)
	}
	var found []Overlap
	seen := map[string]bool{request.Template: true}
	for _, previous := range earlier {
		if seen[previous.Template] {
			continue
		}
		seen[previous.Template] = true
		template, err := numbering.Parse(previous.Template)
		if err != nil {
			// 早先的一版今天解析不了（平台库升级收紧了文法）：它也就不会再出号，谈不上与新号重叠。
			continue
		}
		previousShapes, err := shapesOf(template, request.Type, request.Tenant, at)
		if err != nil {
			continue
		}
		sharedSequence := template.Granularity() == candidate.Granularity()
		if shapesMeet(candidateShapes, previousShapes, sharedSequence) {
			found = append(found, previous)
		}
	}
	return found, nil
}

// overlapMaxDigits 是比形状时看到的最长流水位数：十二位已远超任何一类单据一个周期的量。
const overlapMaxDigits = 12

// overlapProbes 是每个位数段的探针个数：段首、段尾与中间三处。
const overlapProbes = 5

// segmentShape 是一份模板在一个流水位数段里渲染出的号：探针原样与逐位的字符上下界。探针渲染得不等长（加扰或三十六
// 进制流水在段内变宽）时，各探针各成一个只含自己的形状。
type segmentShape struct {
	digits int
	probes []string
	low    []rune
	high   []rune
}

func shapesOf(template *numbering.Template, code TypeCode, tenant string, at time.Time) ([]segmentShape, error) {
	var shapes []segmentShape
	for digits := 1; digits <= overlapMaxDigits; digits++ {
		low := int64(1)
		for range digits - 1 {
			low *= 10
		}
		high := low*10 - 1
		probes := make([]string, 0, overlapProbes)
		for index := range overlapProbes {
			seq := low + (high-low)*int64(index)/(overlapProbes-1)
			// 随机段在各探针上取不同的全同数字（0、2、4、6、9），逐位的上下界才张得开。
			digit := strconv.Itoa(index * 9 / (overlapProbes - 1))
			number, err := render(template, code, tenant, previewScrambleSecret, seq, at, func(width int) (string, error) {
				return strings.Repeat(digit, width), nil
			})
			if err != nil {
				return nil, err
			}
			probes = append(probes, number)
		}
		shapes = append(shapes, shapeOf(digits, probes)...)
	}
	return shapes, nil
}

func shapeOf(digits int, probes []string) []segmentShape {
	first := []rune(probes[0])
	low, high := append([]rune(nil), first...), append([]rune(nil), first...)
	for _, probe := range probes[1:] {
		runes := []rune(probe)
		if len(runes) != len(first) {
			shapes := make([]segmentShape, 0, len(probes))
			for _, single := range probes {
				fixed := []rune(single)
				shapes = append(shapes, segmentShape{digits: digits, probes: []string{single}, low: fixed, high: fixed})
			}
			return shapes
		}
		for index, r := range runes {
			low[index], high[index] = min(low[index], r), max(high[index], r)
		}
	}
	return []segmentShape{{digits: digits, probes: probes, low: low, high: high}}
}

func shapesMeet(candidate, previous []segmentShape, sharedSequence bool) bool {
	for _, left := range candidate {
		for _, right := range previous {
			if len(left.low) != len(right.low) {
				continue
			}
			if sharedSequence && left.digits == right.digits && slices.Equal(left.probes, right.probes) {
				continue
			}
			meet := true
			for index := range left.low {
				if max(left.low[index], right.low[index]) > min(left.high[index], right.high[index]) {
					meet = false
					break
				}
			}
			if meet {
				return true
			}
		}
	}
	return false
}

// RejectionReason 从登记或模板检查的错误里取出拒因哨兵，预览答复与登记答复用同一组码；认不出时原样交回。
func RejectionReason(err error) error {
	for _, reason := range []error{
		ErrTemplateUnparseable, ErrTemplateWithoutSequence, ErrTemplateNeedsUnsuppliedValue, ErrTemplateUnrenderable,
		ErrTypeNotDeclared, ErrScrambleSecretNotConfigured, ErrTimeZoneNotRecognized, ErrRegistrationIncomplete,
	} {
		if errors.Is(err, reason) {
			return reason
		}
	}
	return err
}
