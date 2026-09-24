package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// ErrCatalogUnresolvable 说明目录登记了、判断时点也有适用的路由策略版本，而取数侧从它折不出这次判断的事实：
// 候选线路的连接或节点在判断时点没有适用版本、段链断开、作用在候选上的适用范围调整没有可读的范围内容、区域
// 覆盖读回不成形。它响亮上抛（ADR-0148 决定六；UC-NR-002 矩阵行 7「必需网络版本未发布、配置损坏 → 未形成
// 判断」）：退成`未配置`会让人去催租户登记一份已经登记过的东西；退成空证据或一条被淘汰的候选，会把「目录
// 坏了」讲成「网络里没有路」，而`不可达`要求必需权威证据完整。
var ErrCatalogUnresolvable = errors.New("network routing: the network catalog is registered but cannot be folded at the judgment time")

// ErrCustomsAnswerIncomplete 说明关务来源没有逐候选作答。漏答的候选在领域评估里没有硬约束事实就原样通过，
// 等于把没问到读成满足，所以它是端口坏了，响亮上抛。
var ErrCustomsAnswerIncomplete = errors.New("network routing: the customs applicability source left a candidate unanswered")

// CatalogNetworkEvidence 是可达性证据视图的目录折叠实现（ADR-0148 决定一、二、三、五、六）。折叠是产品策略
// （ADR-0146 决定七），不落持久化适配器：目录读口只按判断时点选版，候选怎样生成、事实怎样折出在这里定；
// 领域评估照旧在领域（ADR-0046），这里交的是事实不是结论。
//
// 三路来源各归其位：目录折出服务区域解析、候选与路径可执行性；地理解析投影随请求携带；关务适用性经端口取。
// 视图修订只标目录那一路——它就是目录修订锚，与事实同一条语句取回。
type CatalogNetworkEvidence struct {
	catalog ports.NetworkCatalogRead
	customs ports.CustomsApplicabilitySource
}

var _ ports.NetworkEvidenceView = (*CatalogNetworkEvidence)(nil)

func NewCatalogNetworkEvidence(
	catalog ports.NetworkCatalogRead,
	customs ports.CustomsApplicabilitySource,
) (*CatalogNetworkEvidence, error) {
	if catalog == nil {
		return nil, fmt.Errorf("network routing application: network catalog read is required")
	}
	if customs == nil {
		return nil, fmt.Errorf("network routing application: customs applicability source is required")
	}
	return &CatalogNetworkEvidence{catalog: catalog, customs: customs}, nil
}

// LoadNetworkEvidence 按判断键的 asOf 选版折出一次可达性判断的事实。三格见 ports.NetworkEvidenceView。
func (view *CatalogNetworkEvidence) LoadNetworkEvidence(
	ctx context.Context,
	key domain.ReachabilityJudgmentKey,
	carried ports.RequestCarriedContent,
) (ports.NetworkEvidence, bool, error) {
	none := ports.NetworkEvidence{}
	snapshot, configured, err := view.catalog.LoadDefinitionsAt(ctx, key.TenantID, key.AsOf.At())
	if err != nil {
		return none, false, fmt.Errorf("load network evidence: %w", err)
	}
	if !catalogConfiguredFor(snapshot, configured, key.ServicePurpose) {
		return none, false, nil
	}

	candidates, err := generateCatalogCandidates(snapshot, key.ServicePurpose)
	if err != nil {
		return none, false, err
	}
	areas, err := resolveCandidateServiceAreas(candidates, carried.Geo)
	if err != nil {
		return none, false, err
	}
	executability, err := foldPathExecutability(candidates, snapshot.Adjustments)
	if err != nil {
		return none, false, err
	}
	constraints, err := view.customsFindings(ctx, key, candidates)
	if err != nil {
		return none, false, err
	}
	return ports.NetworkEvidence{
		ServiceAreas:      areas,
		PathExecutability: executability,
		HardConstraints:   constraints,
		ViewRevision:      snapshot.Revision,
	}, true, nil
}

// customsFindings 向关务来源逐候选取适用性，并核它逐条答了。候选空间为空时不问——没有要作答的东西。
func (view *CatalogNetworkEvidence) customsFindings(
	ctx context.Context,
	key domain.ReachabilityJudgmentKey,
	candidates []catalogCandidate,
) ([]domain.HardConstraintFinding, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	query := ports.CustomsApplicabilityQuery{
		Tenant:     key.TenantID,
		AsOf:       key.AsOf.At(),
		Candidates: make([]ports.CustomsCandidate, 0, len(candidates)),
	}
	for _, candidate := range candidates {
		legs := make([]ports.CustomsCandidateLeg, 0, len(candidate.legs))
		for _, leg := range candidate.legs {
			legs = append(legs, ports.CustomsCandidateLeg{Connection: leg.Code, FromNode: leg.FromNode, ToNode: leg.ToNode})
		}
		query.Candidates = append(query.Candidates, ports.CustomsCandidate{Candidate: candidate.id, Legs: legs})
	}
	findings, err := view.customs.AssessCustomsApplicability(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("assess customs applicability: %w", err)
	}
	answered := make(map[domain.CandidateID]struct{}, len(findings))
	for _, finding := range findings {
		answered[finding.Candidate()] = struct{}{}
	}
	for _, candidate := range candidates {
		if _, ok := answered[candidate.id]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrCustomsAnswerIncomplete, candidate.id)
		}
	}
	return findings, nil
}

// catalogConfiguredFor 是`未配置`的唯一判法（ADR-0148 决定六），可达性与初始路由的证据视图共用：目录修订锚存在，且判断时点有
// 适用范围等于这次服务目的的路由策略版本。快照已按判断时点选过版，这里只比范围。
func catalogConfiguredFor(snapshot ports.NetworkCatalogSnapshot, configured bool, purpose domain.ServicePurpose) bool {
	if !configured {
		return false
	}
	for _, strategy := range snapshot.Strategies {
		if strategy.ApplicableScope == purpose.String() {
			return true
		}
	}
	return false
}

// catalogCandidate 是一条候选线路在判断时点的折叠结果：线路版本、按序解析出的连接，以及首节点所服务的始发
// 区域与末节点所服务的交付区域。
type catalogCandidate struct {
	id               domain.CandidateID
	line             ports.LineDefinitionVersion
	legs             []ports.ConnectionDefinitionVersion
	originAreas      []areaCoverage
	destinationAreas []areaCoverage
}

type areaCoverage struct {
	reference string
	coverage  domain.ServiceAreaCoverage
}

// generateCatalogCandidates 是首版候选生成（ADR-0148 决定五）：判断时点适用、适用范围是这次服务目的的每条线路，
// 首节点至少服务一个区域的始发角色、末节点至少服务一个区域的交付角色，即一个候选；不拼接、不截取中段。候选
// 空间与所携地址无关——哪条线路的区域不覆盖这个地址，要作为带依据的淘汰留在候选上，`不可达`才复算得出来。
//
// 线路的适用范围按路由策略版本同一解释（服务目的）：「新线路版本和路由策略版本必须具有明确生效时间和适用
// 范围」，另一个服务目的的线路不是这次判断的可能候选。
//
// 首段或末段连接在判断时点没有适用版本时，定不出线路两端，就判不了它是不是候选，候选空间闭合不了，交回
// ErrCatalogUnresolvable；判定为候选的线路再逐段核连接、段链与节点。
func generateCatalogCandidates(snapshot ports.NetworkCatalogSnapshot, purpose domain.ServicePurpose) ([]catalogCandidate, error) {
	connections := make(map[string]ports.ConnectionDefinitionVersion, len(snapshot.Connections))
	for _, connection := range snapshot.Connections {
		connections[connection.Code] = connection
	}
	nodes := make(map[string]struct{}, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		nodes[node.Code] = struct{}{}
	}
	originByNode, destinationByNode, err := areasByNodeRole(snapshot.ServiceAreas)
	if err != nil {
		return nil, err
	}

	candidates := make([]catalogCandidate, 0, len(snapshot.Lines))
	for _, line := range snapshot.Lines {
		if line.ApplicableScope != purpose.String() {
			continue
		}
		lineReference := versionReference(line.Code, line.Version)
		if len(line.Segments) == 0 {
			return nil, fmt.Errorf("%w: line %s has no segments", ErrCatalogUnresolvable, lineReference)
		}
		first, firstFound := connections[line.Segments[0]]
		last, lastFound := connections[line.Segments[len(line.Segments)-1]]
		if !firstFound || !lastFound {
			return nil, fmt.Errorf("%w: line %s: its first or last connection has no applicable version, so its ends are unknown",
				ErrCatalogUnresolvable, lineReference)
		}
		origin, destination := originByNode[first.FromNode], destinationByNode[last.ToNode]
		if len(origin) == 0 || len(destination) == 0 {
			continue
		}
		legs, err := resolveLineLegs(line, lineReference, connections, nodes)
		if err != nil {
			return nil, err
		}
		id, err := domain.NewCandidateID(lineReference)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, catalogCandidate{
			id: id, line: line, legs: legs, originAreas: origin, destinationAreas: destination,
		})
	}
	return candidates, nil
}

// areasByNodeRole 按节点索引区域的始发与交付角色。没登覆盖的区域不解析任何地址，也就不进索引；覆盖读回不成形
// 是目录坏了，不是这个区域不覆盖。
func areasByNodeRole(areas []ports.ServiceAreaDefinitionVersion) (map[string][]areaCoverage, map[string][]areaCoverage, error) {
	origin := map[string][]areaCoverage{}
	destination := map[string][]areaCoverage{}
	for _, area := range areas {
		if !area.HasCoverage {
			continue
		}
		reference := versionReference(area.Code, area.Version)
		coverage, err := domain.NewServiceAreaCoverage(area.CoverageCountry, area.PostalPrefixes)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: service area %s: %v", ErrCatalogUnresolvable, reference, err)
		}
		entry := areaCoverage{reference: reference, coverage: coverage}
		for _, node := range area.OriginNodes {
			origin[node] = append(origin[node], entry)
		}
		for _, node := range area.DestinationNodes {
			destination[node] = append(destination[node], entry)
		}
	}
	return origin, destination, nil
}

// resolveLineLegs 按序解析一条候选线路的连接：每段在判断时点都要有适用版本、相邻两段首尾相接、途经节点都要
// 有适用版本。缺一格就是必需网络版本未发布或目录配置损坏，交回 ErrCatalogUnresolvable。
func resolveLineLegs(
	line ports.LineDefinitionVersion,
	lineReference string,
	connections map[string]ports.ConnectionDefinitionVersion,
	nodes map[string]struct{},
) ([]ports.ConnectionDefinitionVersion, error) {
	legs := make([]ports.ConnectionDefinitionVersion, 0, len(line.Segments))
	for index, segment := range line.Segments {
		connection, found := connections[segment]
		if !found {
			return nil, fmt.Errorf("%w: line %s: connection %s has no applicable version",
				ErrCatalogUnresolvable, lineReference, segment)
		}
		if index > 0 && legs[index-1].ToNode != connection.FromNode {
			return nil, fmt.Errorf("%w: line %s: connections %s and %s do not meet",
				ErrCatalogUnresolvable, lineReference, legs[index-1].Code, connection.Code)
		}
		for _, node := range []string{connection.FromNode, connection.ToNode} {
			if _, found := nodes[node]; !found {
				return nil, fmt.Errorf("%w: line %s: node %s has no applicable version",
					ErrCatalogUnresolvable, lineReference, node)
			}
		}
		legs = append(legs, connection)
	}
	return legs, nil
}

// resolveCandidateServiceAreas 逐候选折出服务区域解析事实（UC-NR-002 层次 3：起止服务区域）。
func resolveCandidateServiceAreas(
	candidates []catalogCandidate,
	projection domain.GeoResolutionProjection,
) ([]domain.ServiceAreaResolution, error) {
	resolutions := make([]domain.ServiceAreaResolution, 0, len(candidates))
	for _, candidate := range candidates {
		spec, err := serviceAreaSpecFor(candidate, projection)
		if err != nil {
			return nil, err
		}
		resolution, err := domain.NewServiceAreaResolution(spec)
		if err != nil {
			return nil, err
		}
		resolutions = append(resolutions, resolution)
	}
	return resolutions, nil
}

// serviceAreaSpecFor 按起止两侧的覆盖匹配定一条候选的解析事实。确定性排除先于资料不足：一侧已被已发布区域明确
// 排除，另一侧缺什么也救不回这条候选；两侧都没被排除而有一侧答不出，才是资料不足，缺口点名缺的是哪一侧哪一格。
// 覆盖时引所用的收件侧区域版本。
func serviceAreaSpecFor(candidate catalogCandidate, projection domain.GeoResolutionProjection) (domain.ServiceAreaResolutionSpec, error) {
	spec := domain.ServiceAreaResolutionSpec{Candidate: candidate.id}
	if !projection.Carried() {
		return insufficientAreaSpec(spec, "GEO_PROJECTION_NOT_CARRIED", "GEO_PROJECTION_CARRIED")
	}
	origin, _ := matchSide(projection.Sender(), candidate.originAreas)
	destination, covering := matchSide(projection.Delivery(), candidate.destinationAreas)

	var err error
	switch {
	case origin == domain.CoverageNotCovered:
		spec.Outcome = domain.AreaExcludesOrigin
		spec.AreaVersion, err = areaVersionReference(candidate.originAreas)
	case destination == domain.CoverageNotCovered:
		spec.Outcome = domain.AreaExcludesDestination
		spec.AreaVersion, err = areaVersionReference(candidate.destinationAreas)
	case origin == domain.CoverageInsufficient || destination == domain.CoverageInsufficient:
		var missing []string
		if origin == domain.CoverageInsufficient {
			missing = append(missing, missingGeoElement("SENDER", projection.Sender()))
		}
		if destination == domain.CoverageInsufficient {
			missing = append(missing, missingGeoElement("DELIVERY", projection.Delivery()))
		}
		elements := strings.Join(missing, "+")
		return insufficientAreaSpec(spec, "GEO_ELEMENTS_MISSING/"+elements, "GEO_ELEMENTS_SUPPLEMENTED/"+elements)
	default:
		spec.Outcome = domain.AreaCoversDestination
		spec.AreaVersion, err = areaVersionReference(covering)
	}
	return spec, err
}

func insufficientAreaSpec(spec domain.ServiceAreaResolutionSpec, missing, reassess string) (domain.ServiceAreaResolutionSpec, error) {
	gap, err := domain.NewEvidenceGapReference(missing)
	if err != nil {
		return spec, err
	}
	condition, err := domain.NewReassessmentCondition(reassess)
	if err != nil {
		return spec, err
	}
	spec.Outcome, spec.Missing, spec.Reassess = domain.AddressInformationInsufficient, gap, condition
	return spec, nil
}

// matchSide 把一侧地址对这一侧各区域的匹配合成一格：任一覆盖即覆盖（交回覆盖它的那些区域）；无一覆盖而有答不出
// 的即资料不足；全部明确不覆盖才是不覆盖。
func matchSide(side domain.GeoResolutionSide, areas []areaCoverage) (domain.CoverageMatch, []areaCoverage) {
	var covering []areaCoverage
	insufficient := false
	for _, area := range areas {
		switch area.coverage.Match(side) {
		case domain.CoverageCovered:
			covering = append(covering, area)
		case domain.CoverageInsufficient:
			insufficient = true
		}
	}
	switch {
	case len(covering) > 0:
		return domain.CoverageCovered, covering
	case insufficient:
		return domain.CoverageInsufficient, nil
	default:
		return domain.CoverageNotCovered, nil
	}
}

func missingGeoElement(side string, geo domain.GeoResolutionSide) string {
	if !geo.HasUsableCountry() {
		return side + "_COUNTRY"
	}
	return side + "_POSTAL_CODE"
}

// areaVersionReference 把一组区域版本写成一个依据引用：多版并列时按字典序以「+」连，同一组区域只有一种写法。
func areaVersionReference(areas []areaCoverage) (domain.ServiceAreaVersionReference, error) {
	references := make([]string, 0, len(areas))
	for _, area := range areas {
		references = append(references, area.reference)
	}
	return domain.NewServiceAreaVersionReference(joinSortedUnique(references))
}

// foldPathExecutability 逐候选折出路径可执行性（UC-NR-002 层次 4 的可用性半边；日历与截单随 routing-first-cut/09）。
// 生效中的停运或关闭作用于线路、其连接或途经节点即不可执行，引那几条调整；恢复陈述不挡路；适用范围调整的范围
// 内容目录里没有列，作用在候选上就折不出它的效果，交回 ErrCatalogUnresolvable。可执行时引线路版本。
func foldPathExecutability(
	candidates []catalogCandidate,
	adjustments []ports.AvailabilityAdjustmentStatement,
) ([]domain.PathExecutability, error) {
	findings := make([]domain.PathExecutability, 0, len(candidates))
	for _, candidate := range candidates {
		var blocking []string
		for _, adjustment := range adjustments {
			if !candidate.touches(adjustment.TargetKind, adjustment.TargetCode) {
				continue
			}
			reference := versionReference(adjustment.Code, adjustment.Version)
			switch adjustment.Kind {
			case ports.AdjustmentSuspension, ports.AdjustmentClosure:
				blocking = append(blocking, reference)
			case ports.AdjustmentResumption:
			default:
				return nil, fmt.Errorf("%w: adjustment %s (%s) acts on candidate %s and its effect cannot be read from the catalog",
					ErrCatalogUnresolvable, reference, adjustment.Kind, candidate.id)
			}
		}
		spec := domain.PathExecutabilitySpec{Candidate: candidate.id, Outcome: domain.PathExecutable}
		schedule := "LINE/" + candidate.id.String()
		if len(blocking) > 0 {
			spec.Outcome = domain.PathNotExecutable
			schedule = "ADJUSTMENT/" + joinSortedUnique(blocking)
		}
		var err error
		if spec.Schedule, err = domain.NewScheduleVersionReference(schedule); err != nil {
			return nil, err
		}
		finding, err := domain.NewPathExecutability(spec)
		if err != nil {
			return nil, err
		}
		findings = append(findings, finding)
	}
	return findings, nil
}

// touches 答一条调整的适用对象在不在这条候选上：线路本身、它的连接或途经节点。
func (candidate catalogCandidate) touches(kind ports.CatalogTargetKind, code string) bool {
	switch kind {
	case ports.TargetLine:
		return candidate.line.Code == code
	case ports.TargetConnection:
		for _, leg := range candidate.legs {
			if leg.Code == code {
				return true
			}
		}
	case ports.TargetNode:
		for _, leg := range candidate.legs {
			if leg.FromNode == code || leg.ToNode == code {
				return true
			}
		}
	}
	return false
}

func versionReference(code string, version int32) string {
	return code + "@" + strconv.FormatInt(int64(version), 10)
}

func joinSortedUnique(values []string) string {
	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	unique := sorted[:0]
	for index, value := range sorted {
		if index == 0 || value != sorted[index-1] {
			unique = append(unique, value)
		}
	}
	return strings.Join(unique, "+")
}

// CustomsApplicabilityNotConnected 是关务来源未接时的如实作答（ADR-0148 决定三）：逐候选答状态未知，领域照既有
// 规则得出`资料不足`（可达性）或未决（初始路由）；不答满足，也不自行推断候选是否跨关务区域——那是
// customs-compliance 的判断。CC 侧判断口落地后由消费方适配器取代它（routing-first-cut/12）。
type CustomsApplicabilityNotConnected struct{}

var _ ports.CustomsApplicabilitySource = CustomsApplicabilityNotConnected{}

func (CustomsApplicabilityNotConnected) AssessCustomsApplicability(
	_ context.Context,
	query ports.CustomsApplicabilityQuery,
) ([]domain.HardConstraintFinding, error) {
	missing, err := domain.NewEvidenceGapReference("CUSTOMS_APPLICABILITY_NOT_CONNECTED")
	if err != nil {
		return nil, err
	}
	reassess, err := domain.NewReassessmentCondition("CUSTOMS_APPLICABILITY_SOURCE_CONNECTED")
	if err != nil {
		return nil, err
	}
	findings := make([]domain.HardConstraintFinding, 0, len(query.Candidates))
	for _, candidate := range query.Candidates {
		finding, err := domain.NewHardConstraintFinding(domain.HardConstraintFindingSpec{
			Candidate: candidate.Candidate,
			Outcome:   domain.ConstraintStatusUnknown,
			Missing:   missing,
			Reassess:  reassess,
		})
		if err != nil {
			return nil, err
		}
		findings = append(findings, finding)
	}
	return findings, nil
}
