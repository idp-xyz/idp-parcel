package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"
)

// 本文件是「关务适用性判断」的领域词形（票 routing-first-cut/12）：按路由候选逐条作
// 答、口岸与申报路径两级、答案闭合三格加理由（可用 / 不可用 / 状态未知）。它是即时作
// 答——不落判断史库，出处由判断标识（按 租户 + 时点 + 候选 + 目录版本 铸成、可重算）与
// 目录版本引用承载，由消费方随路由判断留痕（ADR-0148 决定一）。
//
// 判断方法只对词形作组合与校验，目录内容从快照输入来（FoldCustomsApplicability 的
// entries），本文件不读任何存储。区域维首版不建模、申报模式不在候选投影上，所以两岸
// 判断只核口岸在册-生效与路径的「口岸、方向」两维；模式维首版不评（CONTEXT「关务适
// 用性判断」规则照此写）。

// RouteCandidateReference 是被作答的路由候选。候选标识是 network-routing 的词汇——
// 本上下文只收引用不回读它，按引用作答、照引用交回，不 import 提供方的 domain。
type RouteCandidateReference struct{ requiredValue }

func NewRouteCandidateReference(value string) (RouteCandidateReference, error) {
	required, err := newRequiredValue("route candidate reference", value)
	return RouteCandidateReference{required}, err
}

// CustomsApplicabilityOutcome 是关务适用性作答的封闭三格。`状态未知`单独一格：它等
// 的是证据与目录，不是一条候选的定论。
type CustomsApplicabilityOutcome uint8

const (
	CustomsApplicabilityOutcomeInvalid CustomsApplicabilityOutcome = iota
	CustomsAvailable
	CustomsUnavailable
	CustomsStatusUnknown
)

func (outcome CustomsApplicabilityOutcome) valid() bool {
	return outcome >= CustomsAvailable && outcome <= CustomsStatusUnknown
}

func (outcome CustomsApplicabilityOutcome) String() string {
	switch outcome {
	case CustomsAvailable:
		return "AVAILABLE"
	case CustomsUnavailable:
		return "UNAVAILABLE"
	case CustomsStatusUnknown:
		return "STATUS_UNKNOWN"
	default:
		return ""
	}
}

// CustomsUnavailableReason 是「不可用」的封闭理由。口岸两格按**路径所经的那个口岸**
// 核：路径指名了口岸，口岸在册与否、在判断时点生效与否是路径三维的一部分——「任一在
// 册口岸」的弱读法会让一条经未登记口岸的路径靠别处一个在册口岸蒙混过关。
type CustomsUnavailableReason uint8

const (
	CustomsUnavailableReasonInvalid CustomsUnavailableReason = iota
	PortNotRegistered
	PortNotEffective
	PathDirectionNotCovered
)

func (reason CustomsUnavailableReason) valid() bool {
	return reason >= PortNotRegistered && reason <= PathDirectionNotCovered
}

func (reason CustomsUnavailableReason) String() string {
	switch reason {
	case PortNotRegistered:
		return "PORT_NOT_REGISTERED"
	case PortNotEffective:
		return "PORT_NOT_EFFECTIVE"
	case PathDirectionNotCovered:
		return "PATH_DIRECTION_NOT_COVERED"
	default:
		return ""
	}
}

// CustomsUnknownReason 是「状态未知」的封闭理由。三类恢复动作各不同：缺国家码向发起
// 方要，目录为空等租户登记，读不到靠运维救依赖——压成一格会让人催错对象。
type CustomsUnknownReason uint8

const (
	CustomsUnknownReasonInvalid CustomsUnknownReason = iota
	EndpointCountryMissing
	CatalogEmpty
	CatalogUnreadable
)

func (reason CustomsUnknownReason) valid() bool {
	return reason >= EndpointCountryMissing && reason <= CatalogUnreadable
}

func (reason CustomsUnknownReason) String() string {
	switch reason {
	case EndpointCountryMissing:
		return "ENDPOINT_COUNTRY_MISSING"
	case CatalogEmpty:
		return "CATALOG_EMPTY"
	case CatalogUnreadable:
		return "CATALOG_UNREADABLE"
	default:
		return ""
	}
}

// CustomsEndpointSide 指名缺国家码的是哪一端；两端都缺时是第三格，不是两格相加——
// 它是一份判断的一个缺口描述。
type CustomsEndpointSide uint8

const (
	CustomsEndpointSideInvalid CustomsEndpointSide = iota
	EndpointSideOrigin
	EndpointSideDestination
	EndpointSideBoth
)

func (side CustomsEndpointSide) valid() bool {
	return side >= EndpointSideOrigin && side <= EndpointSideBoth
}

func (side CustomsEndpointSide) String() string {
	switch side {
	case EndpointSideOrigin:
		return "ORIGIN"
	case EndpointSideDestination:
		return "DESTINATION"
	case EndpointSideBoth:
		return "BOTH"
	default:
		return ""
	}
}

// CustomsPortRecord 是口岸目录里的一行：口岸标识与生效区间。AppliesUntil 零值即开放
// 版（后继版本登记时落定终点），与 CandidatePortEntry 同一约定。
type CustomsPortRecord struct {
	Port         CustomsPortReference
	AppliesFrom  time.Time
	AppliesUntil time.Time
}

// CustomsPathRecord 是申报路径目录里的一行：路径标识、三维路径事实与生效区间。
type CustomsPathRecord struct {
	Path         DeclarationPathReference
	Route        DeclarationPathRoute
	AppliesFrom  time.Time
	AppliesUntil time.Time
}

// CustomsApplicabilityEntries 是一次判断所依据的两本目录快照。快照带全量在册行而不
// 只带生效行：`口岸未登记`（任何时点都没有这一行）与`口岸未生效`（有行而判断时点不
// 覆盖）在答案上分得开，目录为空（两本皆零行）与它们也分得开——三种答案续办动作不同，
// 读口只交全量才折得出来。
type CustomsApplicabilityEntries struct {
	Ports []CustomsPortRecord
	Paths []CustomsPathRecord
}

// CustomsApplicabilityJudgment 是一份逐候选的作答：候选、三格结论、指名的那一格理由，
// 以及出处。出处两件——判断标识与目录版本引用——是答案的一部分：没有出处的一份「可
// 用」无从复核当时凭的是什么目录（ADR-0148 决定一）。
type CustomsApplicabilityJudgment struct {
	candidate  RouteCandidateReference
	outcome    CustomsApplicabilityOutcome
	reason     CustomsUnavailableReason
	port       CustomsPortReference
	direction  ManifestDirection
	unknown    CustomsUnknownReason
	side       CustomsEndpointSide
	judgmentID string
	versions   []string
}

// newCustomsApplicabilityJudgment 按结果分片校验后装配：可用不带任何理由字段；不可用
// 必须带成立的理由——口岸两格指名口岸，方向格指名方向；状态未知必须带未知理由——缺国
// 家码再指名哪一端。两头全带或全空的答案复核对不上任何一格，不让它成形。
func newCustomsApplicabilityJudgment(spec customsApplicabilityJudgmentSpec) (CustomsApplicabilityJudgment, error) {
	if !spec.Candidate.valid() || !spec.Outcome.valid() || spec.JudgmentID == "" {
		return CustomsApplicabilityJudgment{}, ErrInvalidCustomsApplicabilityJudgment
	}
	versions := append([]string(nil), spec.Versions...)
	for _, version := range versions {
		if strings.TrimSpace(version) == "" {
			return CustomsApplicabilityJudgment{}, ErrInvalidCustomsApplicabilityJudgment
		}
	}
	switch spec.Outcome {
	case CustomsAvailable:
		if spec.Reason.valid() || spec.Unknown.valid() {
			return CustomsApplicabilityJudgment{}, ErrInvalidCustomsApplicabilityJudgment
		}
	case CustomsUnavailable:
		if !spec.Reason.valid() || spec.Unknown.valid() {
			return CustomsApplicabilityJudgment{}, ErrInvalidCustomsApplicabilityJudgment
		}
		switch spec.Reason {
		case PortNotRegistered, PortNotEffective:
			if !spec.Port.valid() || spec.Direction.valid() {
				return CustomsApplicabilityJudgment{}, ErrInvalidCustomsApplicabilityJudgment
			}
		case PathDirectionNotCovered:
			if spec.Port.valid() || !spec.Direction.valid() {
				return CustomsApplicabilityJudgment{}, ErrInvalidCustomsApplicabilityJudgment
			}
		}
	case CustomsStatusUnknown:
		if spec.Reason.valid() || !spec.Unknown.valid() {
			return CustomsApplicabilityJudgment{}, ErrInvalidCustomsApplicabilityJudgment
		}
		if spec.Unknown == EndpointCountryMissing {
			if !spec.Side.valid() {
				return CustomsApplicabilityJudgment{}, ErrInvalidCustomsApplicabilityJudgment
			}
		} else if spec.Side.valid() {
			return CustomsApplicabilityJudgment{}, ErrInvalidCustomsApplicabilityJudgment
		}
	}
	sort.Strings(versions)
	return CustomsApplicabilityJudgment{
		candidate:  spec.Candidate,
		outcome:    spec.Outcome,
		reason:     spec.Reason,
		port:       spec.Port,
		direction:  spec.Direction,
		unknown:    spec.Unknown,
		side:       spec.Side,
		judgmentID: spec.JudgmentID,
		versions:   versions,
	}, nil
}

// customsApplicabilityJudgmentSpec 是构造一份作答所需的全部输入。哪个理由格带哪件负载
// 见构造时的分片校验，这里不重复列。
type customsApplicabilityJudgmentSpec struct {
	Candidate  RouteCandidateReference
	Outcome    CustomsApplicabilityOutcome
	Reason     CustomsUnavailableReason
	Port       CustomsPortReference
	Direction  ManifestDirection
	Unknown    CustomsUnknownReason
	Side       CustomsEndpointSide
	JudgmentID string
	Versions   []string
}

// ErrInvalidCustomsApplicabilityJudgment 说一份作答的形状拼不拢（三格与负载对不上）。
var ErrInvalidCustomsApplicabilityJudgment = errors.New(
	"customs compliance: invalid customs applicability judgment")

// FoldCustomsApplicability 按两本目录快照逐候选折出作答（CONTEXT「关务适用性判断」规则）：
//
//   - 任一端缺国家码 → 状态未知（证据不足，指名哪一端）；
//   - 两端同国 → 可用（不含关务段，无需口岸与申报路径在场）；
//   - 两端异国（含关务段）→ 两本目录皆空 → 状态未知（目录为空）；否则逐侧核：寄件国
//     一侧要生效的出口路径、收件国一侧要生效的进口路径，路径所经口岸须在册且判断时点
//     生效。任一侧对不上即不可用并指名理由——口岸未登记、口岸未生效，或该侧无生效且
//     方向对得上的路径。
//
// 出处随每一份作答带出：目录版本引用取快照全量行，判断标识按 租户 + 时点 + 候选 + 目录
// 版本 铸成。快照不变则两次折叠逐字节相同（判据三由目录可重放性保证）。
func FoldCustomsApplicability(
	tenant TenantID,
	candidate RouteCandidateReference,
	originCountry string,
	hasOriginCountry bool,
	destinationCountry string,
	hasDestinationCountry bool,
	asOf time.Time,
	entries CustomsApplicabilityEntries,
) (CustomsApplicabilityJudgment, error) {
	if !tenant.valid() || !candidate.valid() || asOf.IsZero() {
		return CustomsApplicabilityJudgment{}, ErrInvalidCustomsApplicabilityJudgment
	}
	versions := catalogVersionReferences(entries)
	judgmentID := customsApplicabilityJudgmentID(tenant, asOf, candidate, versions)

	if !hasOriginCountry || !hasDestinationCountry {
		return unknownJudgment(candidate, EndpointCountryMissing, endpointSide(hasOriginCountry, hasDestinationCountry), judgmentID, versions)
	}
	if originCountry == destinationCountry {
		return newCustomsApplicabilityJudgment(customsApplicabilityJudgmentSpec{
			Candidate: candidate, Outcome: CustomsAvailable, JudgmentID: judgmentID, Versions: versions,
		})
	}
	if len(entries.Ports) == 0 && len(entries.Paths) == 0 {
		return unknownJudgment(candidate, CatalogEmpty, CustomsEndpointSideInvalid, judgmentID, versions)
	}

	for _, side := range customsCrossingSides() {
		covered, reason, port, direction := sideCovered(asOf, side, entries)
		if covered {
			continue
		}
		return newCustomsApplicabilityJudgment(customsApplicabilityJudgmentSpec{
			Candidate: candidate, Outcome: CustomsUnavailable, Reason: reason,
			Port: port, Direction: direction, JudgmentID: judgmentID, Versions: versions,
		})
	}
	return newCustomsApplicabilityJudgment(customsApplicabilityJudgmentSpec{
		Candidate: candidate, Outcome: CustomsAvailable, JudgmentID: judgmentID, Versions: versions,
	})
}

// FoldCustomsApplicabilityUnreadable 折「依赖读不到」那格：读口没有读数可引用，出处只
// 剩 租户 + 时点 + 候选 铸成的判断标识。把依赖故障折成错误上抛，会把它从答案代数里赶
// 出去——而状态未知那格本为它立（CONTEXT「关务适用性判断」规则）。
func FoldCustomsApplicabilityUnreadable(
	tenant TenantID,
	candidate RouteCandidateReference,
	asOf time.Time,
) (CustomsApplicabilityJudgment, error) {
	if !tenant.valid() || !candidate.valid() || asOf.IsZero() {
		return CustomsApplicabilityJudgment{}, ErrInvalidCustomsApplicabilityJudgment
	}
	judgmentID := customsApplicabilityJudgmentID(tenant, asOf, candidate, nil)
	return unknownJudgment(candidate, CatalogUnreadable, CustomsEndpointSideInvalid, judgmentID, nil)
}

// customsCrossingSides 是含关务段的候选要核的两侧：寄件国一侧出口、收件国一侧进口。
func customsCrossingSides() []ManifestDirection {
	return []ManifestDirection{ExportManifest, ImportManifest}
}

// endpointSide 把「哪端缺码」折成一格：只有一端缺指名那一端，两端都缺是 Both。
func endpointSide(hasOrigin, hasDestination bool) CustomsEndpointSide {
	switch {
	case !hasOrigin && !hasDestination:
		return EndpointSideBoth
	case !hasOrigin:
		return EndpointSideOrigin
	default:
		return EndpointSideDestination
	}
}

// sideCovered 核一侧是否过得去：生效中且方向对得上的路径里，只要有一条所经口岸在册
// 且判断时点生效，这一侧就过得去；全都过不去时按路径引用排序取第一条失败路径的理
// 由——口岸未登记、口岸未生效；一条生效路径都没有才是方向没被覆盖。排序取第一条是
// 为让同一份快照上的失败理由只可能有一个答案。
func sideCovered(
	asOf time.Time,
	direction ManifestDirection,
	entries CustomsApplicabilityEntries,
) (bool, CustomsUnavailableReason, CustomsPortReference, ManifestDirection) {
	var effective []CustomsPathRecord
	for _, path := range entries.Paths {
		if path.Route.Direction() != direction || !effectiveAt(asOf, path.AppliesFrom, path.AppliesUntil) {
			continue
		}
		effective = append(effective, path)
	}
	if len(effective) == 0 {
		return false, PathDirectionNotCovered, CustomsPortReference{}, direction
	}
	sort.Slice(effective, func(i, j int) bool { return effective[i].Path.String() < effective[j].Path.String() })

	var firstFailure customsSideFailure
	for _, path := range effective {
		if failure, failed := pathFailure(asOf, path, entries); failed {
			if !firstFailure.set {
				firstFailure = failure
			}
			continue
		}
		return true, CustomsUnavailableReasonInvalid, CustomsPortReference{}, ManifestDirectionInvalid
	}
	return false, firstFailure.reason, firstFailure.port, ManifestDirectionInvalid
}

// customsSideFailure 是第一条失败路径留下的理由：口岸未登记或口岸未生效。
type customsSideFailure struct {
	set    bool
	reason CustomsUnavailableReason
	port   CustomsPortReference
}

// pathFailure 核一条路径所经口岸：任何时点都没有这一行即未登记；有行而判断时点没有
// 生效版即未生效。
func pathFailure(asOf time.Time, path CustomsPathRecord, entries CustomsApplicabilityEntries) (customsSideFailure, bool) {
	port := path.Route.Port()
	found := false
	for _, row := range entries.Ports {
		if row.Port != port {
			continue
		}
		found = true
		if effectiveAt(asOf, row.AppliesFrom, row.AppliesUntil) {
			return customsSideFailure{}, false
		}
	}
	if !found {
		return customsSideFailure{set: true, reason: PortNotRegistered, port: port}, true
	}
	return customsSideFailure{set: true, reason: PortNotEffective, port: port}, true
}

func effectiveAt(asOf, appliesFrom, appliesUntil time.Time) bool {
	return !appliesFrom.After(asOf) && (appliesUntil.IsZero() || appliesUntil.After(asOf))
}

// catalogVersionReferences 把快照全量行写成目录版本引用（排序、去重）。引用形如
// `PORT:<口岸>@<起点>` 与 `PATH:<路径>@<起点>`——起点就是册子里的版本身份（换版以
// 后继起点落前版终点），全量行都进出处：未生效的行也参与判断（未生效正是它们答的）。
func catalogVersionReferences(entries CustomsApplicabilityEntries) []string {
	refs := make([]string, 0, len(entries.Ports)+len(entries.Paths))
	for _, row := range entries.Ports {
		refs = append(refs, "PORT:"+row.Port.String()+"@"+row.AppliesFrom.UTC().Format(time.RFC3339Nano))
	}
	for _, row := range entries.Paths {
		refs = append(refs, "PATH:"+row.Path.String()+"@"+row.AppliesFrom.UTC().Format(time.RFC3339Nano))
	}
	sort.Strings(refs)
	unique := refs[:0]
	for index, ref := range refs {
		if index == 0 || ref != refs[index-1] {
			unique = append(unique, ref)
		}
	}
	return unique
}

// customsApplicabilityJudgmentID 按 租户 + 时点 + 候选 + 目录版本引用 铸成可重算的判断
// 标识。目录版本引用进标识而答案不进：同一份快照折出的答案确定不变，标识不必替答案
// 背书；快照变了标识跟着变，出处自然指到新依据上。
func customsApplicabilityJudgmentID(
	tenant TenantID,
	asOf time.Time,
	candidate RouteCandidateReference,
	versions []string,
) string {
	lines := make([]string, 0, len(versions)+3)
	lines = append(lines, tenant.String(), asOf.UTC().Format(time.RFC3339Nano), candidate.String())
	lines = append(lines, versions...)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\x00")))
	return hex.EncodeToString(sum[:])
}

func unknownJudgment(
	candidate RouteCandidateReference,
	reason CustomsUnknownReason,
	side CustomsEndpointSide,
	judgmentID string,
	versions []string,
) (CustomsApplicabilityJudgment, error) {
	return newCustomsApplicabilityJudgment(customsApplicabilityJudgmentSpec{
		Candidate: candidate, Outcome: CustomsStatusUnknown, Unknown: reason,
		Side: side, JudgmentID: judgmentID, Versions: versions,
	})
}

func (judgment CustomsApplicabilityJudgment) Candidate() RouteCandidateReference {
	return judgment.candidate
}

func (judgment CustomsApplicabilityJudgment) Outcome() CustomsApplicabilityOutcome {
	return judgment.outcome
}

// UnavailableReason 只在不可用时不缺席；其余两格交回零值——调用的前提是读过 Outcome。
func (judgment CustomsApplicabilityJudgment) UnavailableReason() CustomsUnavailableReason {
	return judgment.reason
}

// UnavailablePort 只在口岸两格理由时不缺席，交出所指名的口岸。
func (judgment CustomsApplicabilityJudgment) UnavailablePort() (CustomsPortReference, bool) {
	return judgment.port, judgment.port.valid()
}

// UnavailableDirection 只在方向格理由时不缺席，交出缺覆盖的方向。
func (judgment CustomsApplicabilityJudgment) UnavailableDirection() (ManifestDirection, bool) {
	return judgment.direction, judgment.direction.valid()
}

func (judgment CustomsApplicabilityJudgment) UnknownReason() CustomsUnknownReason {
	return judgment.unknown
}

// EndpointSide 只在缺国家码时不缺席，交出错在哪一端。
func (judgment CustomsApplicabilityJudgment) EndpointSide() (CustomsEndpointSide, bool) {
	return judgment.side, judgment.side.valid()
}

// JudgmentID 交回可重算的判断标识。
func (judgment CustomsApplicabilityJudgment) JudgmentID() string {
	return judgment.judgmentID
}

// Versions 交回出处里的目录版本引用拷贝（排序、无空白、无重复）。
func (judgment CustomsApplicabilityJudgment) Versions() []string {
	return append([]string(nil), judgment.versions...)
}
