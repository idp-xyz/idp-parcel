// Package registrationjson 是网络目录七族登记行从 JSON 到登记命令的那一份译装（票 operator-channel/04 自
// cmd/parcel-network-register 下沉）。受控批量口与在线登记口两侧共用它：同一个登记口只有一套形状口径，分成两份就有
// 两套。
//
// 翻译严格且零默认：未知字段拒收（打错的键静默丢弃，会让登记方以为登进去的比实际多），可选终点用指针表达「不在场」。
// 两侧只差租户从哪来——受控批量口取批文里的 tenant_id（运维在库网内的治理动作），在线口取操作者信封给的租户、批文
// 带 tenant_id 即拒（采信自报租户会穿透 ADR-0003 的隔离边界）。
package registrationjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/referenceconfig"
)

// ErrSelfReportedTenant 表示在线口的批文里带了 tenant_id：租户只从认证结果来。
var ErrSelfReportedTenant = errors.New("network registration: online input must not carry tenant_id; the tenant comes from the operator envelope")

// ErrCitationOutsideAdoption 表示一行直接登记把参考配置的引用串写进了依据格。引用串只由采用路径写（ADR-0147
// 决定四）：直接行的内容是登记方自己给的，写上引用串就是在声称一次没发生过的采用。
var ErrCitationOutsideAdoption = errors.New("network registration: only adoption writes a reference configuration citation into the basis")

// basisFrom 译直接登记行上的可选依据格。缺席即没给依据；给了就过领域构造门，空白串是写坏了不是没给。
func basisFrom(raw *string) (domain.CatalogBasisReference, error) {
	if raw == nil {
		return domain.CatalogBasisReference{}, nil
	}
	if _, claimed, _ := referenceconfig.OpenCitation(*raw); claimed {
		return domain.CatalogBasisReference{}, fmt.Errorf("%w: %q", ErrCitationOutsideAdoption, *raw)
	}
	return domain.NewCatalogBasisReference(*raw)
}

// tenantOf 决定一份批文的租户取自哪（两侧的差别只在这一格，理由见包注）。
type tenantOf func(documentTenant string) (domain.TenantID, error)

func tenantFromDocument(documentTenant string) (domain.TenantID, error) {
	return domain.NewTenantID(documentTenant)
}

func injectedTenant(tenant domain.TenantID) tenantOf {
	return func(string) (domain.TenantID, error) { return tenant, nil }
}

// refuseSelfReportedTenant 键在场即拒、不看值：`"tenant_id": null` 也是自报。批文不是 JSON 对象时交给 decodeStrict 去答形状错。
func refuseSelfReportedTenant(raw []byte) error {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil
	}
	if _, present := top["tenant_id"]; present {
		return ErrSelfReportedTenant
	}
	return nil
}

// NodeVersionFromJSON 是受控批量口那一路：租户取批文里的 tenant_id。
func NodeVersionFromJSON(raw []byte) (application.RegisterNodeVersionCommand, error) {
	return nodeVersionFromJSON(raw, tenantFromDocument)
}

// NodeVersionFromJSONForTenant 是在线口那一路：租户取操作者信封给的，批文带 tenant_id 即拒。
func NodeVersionFromJSONForTenant(raw []byte, tenant domain.TenantID) (application.RegisterNodeVersionCommand, error) {
	if err := refuseSelfReportedTenant(raw); err != nil {
		return application.RegisterNodeVersionCommand{}, err
	}
	return nodeVersionFromJSON(raw, injectedTenant(tenant))
}

func nodeVersionFromJSON(raw []byte, tenantSource tenantOf) (application.RegisterNodeVersionCommand, error) {
	var payload nodePayload
	if err := decodeStrict(raw, &payload); err != nil {
		return application.RegisterNodeVersionCommand{}, err
	}
	tenant, err := tenantSource(payload.TenantID)
	if err != nil {
		return application.RegisterNodeVersionCommand{}, err
	}
	basis, err := basisFrom(payload.Basis)
	if err != nil {
		return application.RegisterNodeVersionCommand{}, err
	}
	return application.RegisterNodeVersionCommand{
		TenantID: tenant,
		Node: ports.NodeDefinitionVersion{
			Code:             payload.Code,
			Version:          payload.Version,
			BusinessTimezone: payload.BusinessTimezone,
			EffectiveFrom:    payload.EffectiveFrom,
			EffectiveTo:      timeOf(payload.EffectiveTo),
			HasEffectiveTo:   payload.EffectiveTo != nil,
			Basis:            basis,
		},
	}, nil
}

// ConnectionVersionFromJSON 是受控批量口那一路。
func ConnectionVersionFromJSON(raw []byte) (application.RegisterConnectionVersionCommand, error) {
	return connectionVersionFromJSON(raw, tenantFromDocument)
}

// ConnectionVersionFromJSONForTenant 是在线口那一路。
func ConnectionVersionFromJSONForTenant(raw []byte, tenant domain.TenantID) (application.RegisterConnectionVersionCommand, error) {
	if err := refuseSelfReportedTenant(raw); err != nil {
		return application.RegisterConnectionVersionCommand{}, err
	}
	return connectionVersionFromJSON(raw, injectedTenant(tenant))
}

func connectionVersionFromJSON(raw []byte, tenantSource tenantOf) (application.RegisterConnectionVersionCommand, error) {
	var payload connectionPayload
	if err := decodeStrict(raw, &payload); err != nil {
		return application.RegisterConnectionVersionCommand{}, err
	}
	tenant, err := tenantSource(payload.TenantID)
	if err != nil {
		return application.RegisterConnectionVersionCommand{}, err
	}
	basis, err := basisFrom(payload.Basis)
	if err != nil {
		return application.RegisterConnectionVersionCommand{}, err
	}
	return application.RegisterConnectionVersionCommand{
		TenantID: tenant,
		Connection: ports.ConnectionDefinitionVersion{
			Code:             payload.Code,
			Version:          payload.Version,
			FromNode:         payload.FromNode,
			ToNode:           payload.ToNode,
			BusinessTimezone: payload.BusinessTimezone,
			EffectiveFrom:    payload.EffectiveFrom,
			EffectiveTo:      timeOf(payload.EffectiveTo),
			HasEffectiveTo:   payload.EffectiveTo != nil,
			Basis:            basis,
		},
	}, nil
}

// LineVersionFromJSON 是受控批量口那一路。
func LineVersionFromJSON(raw []byte) (application.RegisterLineVersionCommand, error) {
	return lineVersionFromJSON(raw, tenantFromDocument)
}

// LineVersionFromJSONForTenant 是在线口那一路。
func LineVersionFromJSONForTenant(raw []byte, tenant domain.TenantID) (application.RegisterLineVersionCommand, error) {
	if err := refuseSelfReportedTenant(raw); err != nil {
		return application.RegisterLineVersionCommand{}, err
	}
	return lineVersionFromJSON(raw, injectedTenant(tenant))
}

func lineVersionFromJSON(raw []byte, tenantSource tenantOf) (application.RegisterLineVersionCommand, error) {
	var payload linePayload
	if err := decodeStrict(raw, &payload); err != nil {
		return application.RegisterLineVersionCommand{}, err
	}
	tenant, err := tenantSource(payload.TenantID)
	if err != nil {
		return application.RegisterLineVersionCommand{}, err
	}
	basis, err := basisFrom(payload.Basis)
	if err != nil {
		return application.RegisterLineVersionCommand{}, err
	}
	costBases, err := costBasesFrom(payload.CostBases)
	if err != nil {
		return application.RegisterLineVersionCommand{}, err
	}
	return application.RegisterLineVersionCommand{
		TenantID: tenant,
		Line: ports.LineDefinitionVersion{
			Code:             payload.Code,
			Version:          payload.Version,
			Segments:         payload.Segments,
			BusinessTimezone: payload.BusinessTimezone,
			ApplicableScope:  payload.ApplicableScope,
			EffectiveFrom:    payload.EffectiveFrom,
			EffectiveTo:      timeOf(payload.EffectiveTo),
			HasEffectiveTo:   payload.EffectiveTo != nil,
			Basis:            basis,
		},
		CostBases: costBases,
	}, nil
}

// costBasesFrom 逐行译线路的成本依据：种类按封闭两类译，集外的词在入库前拒；段号与引用的形状由登记用例的受理门核。
func costBasesFrom(payloads []costBasisPayload) ([]ports.LineSegmentCostBasis, error) {
	if len(payloads) == 0 {
		return nil, nil
	}
	bases := make([]ports.LineSegmentCostBasis, 0, len(payloads))
	for _, payload := range payloads {
		kind, err := ports.LineCostBasisKindFrom(payload.Kind)
		if err != nil {
			return nil, err
		}
		bases = append(bases, ports.LineSegmentCostBasis{
			SegmentIndex: payload.SegmentIndex, Kind: kind, Reference: payload.Reference,
		})
	}
	return bases, nil
}

// ServiceAreaVersionFromJSON 是受控批量口那一路。
func ServiceAreaVersionFromJSON(raw []byte) (application.RegisterServiceAreaVersionCommand, error) {
	return serviceAreaVersionFromJSON(raw, tenantFromDocument)
}

// ServiceAreaVersionFromJSONForTenant 是在线口那一路。
func ServiceAreaVersionFromJSONForTenant(raw []byte, tenant domain.TenantID) (application.RegisterServiceAreaVersionCommand, error) {
	if err := refuseSelfReportedTenant(raw); err != nil {
		return application.RegisterServiceAreaVersionCommand{}, err
	}
	return serviceAreaVersionFromJSON(raw, injectedTenant(tenant))
}

func serviceAreaVersionFromJSON(raw []byte, tenantSource tenantOf) (application.RegisterServiceAreaVersionCommand, error) {
	var payload areaPayload
	if err := decodeStrict(raw, &payload); err != nil {
		return application.RegisterServiceAreaVersionCommand{}, err
	}
	tenant, err := tenantSource(payload.TenantID)
	if err != nil {
		return application.RegisterServiceAreaVersionCommand{}, err
	}
	basis, err := basisFrom(payload.Basis)
	if err != nil {
		return application.RegisterServiceAreaVersionCommand{}, err
	}
	area := ports.ServiceAreaDefinitionVersion{
		Code:           payload.Code,
		Version:        payload.Version,
		EffectiveFrom:  payload.EffectiveFrom,
		EffectiveTo:    timeOf(payload.EffectiveTo),
		HasEffectiveTo: payload.EffectiveTo != nil,
		Basis:          basis,
	}
	if coverage := payload.Coverage; coverage != nil {
		area.HasCoverage = true
		area.CoverageCountry = coverage.Country
		area.PostalPrefixes = coverage.PostalPrefixes
		area.OriginNodes = coverage.OriginNodes
		area.DestinationNodes = coverage.DestinationNodes
	}
	return application.RegisterServiceAreaVersionCommand{TenantID: tenant, Area: area}, nil
}

// ServiceCalendarVersionFromJSON 是受控批量口那一路。
func ServiceCalendarVersionFromJSON(raw []byte) (application.RegisterServiceCalendarVersionCommand, error) {
	return serviceCalendarVersionFromJSON(raw, tenantFromDocument)
}

// ServiceCalendarVersionFromJSONForTenant 是在线口那一路。
func ServiceCalendarVersionFromJSONForTenant(raw []byte, tenant domain.TenantID) (application.RegisterServiceCalendarVersionCommand, error) {
	if err := refuseSelfReportedTenant(raw); err != nil {
		return application.RegisterServiceCalendarVersionCommand{}, err
	}
	return serviceCalendarVersionFromJSON(raw, injectedTenant(tenant))
}

func serviceCalendarVersionFromJSON(raw []byte, tenantSource tenantOf) (application.RegisterServiceCalendarVersionCommand, error) {
	var payload calendarPayload
	if err := decodeStrict(raw, &payload); err != nil {
		return application.RegisterServiceCalendarVersionCommand{}, err
	}
	tenant, err := tenantSource(payload.TenantID)
	if err != nil {
		return application.RegisterServiceCalendarVersionCommand{}, err
	}
	targetKind, err := ports.CatalogTargetKindFrom(payload.TargetKind)
	if err != nil {
		return application.RegisterServiceCalendarVersionCommand{}, err
	}
	basis, err := basisFrom(payload.Basis)
	if err != nil {
		return application.RegisterServiceCalendarVersionCommand{}, err
	}
	return application.RegisterServiceCalendarVersionCommand{
		TenantID: tenant,
		Calendar: ports.ServiceCalendarDefinitionVersion{
			TargetKind:        targetKind,
			TargetCode:        payload.TargetCode,
			Version:           payload.Version,
			EffectiveFrom:     payload.EffectiveFrom,
			EffectiveTo:       timeOf(payload.EffectiveTo),
			HasEffectiveTo:    payload.EffectiveTo != nil,
			CutoffLocalMinute: payload.CutoffLocalMinute,
			ProcessingMinutes: payload.ProcessingMinutes,
			BufferMinutes:     payload.BufferMinutes,
			Basis:             basis,
		},
	}, nil
}

// AvailabilityAdjustmentFromJSON 是受控批量口那一路。
func AvailabilityAdjustmentFromJSON(raw []byte) (application.RegisterAvailabilityAdjustmentCommand, error) {
	return availabilityAdjustmentFromJSON(raw, tenantFromDocument)
}

// AvailabilityAdjustmentFromJSONForTenant 是在线口那一路。
func AvailabilityAdjustmentFromJSONForTenant(raw []byte, tenant domain.TenantID) (application.RegisterAvailabilityAdjustmentCommand, error) {
	if err := refuseSelfReportedTenant(raw); err != nil {
		return application.RegisterAvailabilityAdjustmentCommand{}, err
	}
	return availabilityAdjustmentFromJSON(raw, injectedTenant(tenant))
}

func availabilityAdjustmentFromJSON(raw []byte, tenantSource tenantOf) (application.RegisterAvailabilityAdjustmentCommand, error) {
	var payload adjustmentPayload
	if err := decodeStrict(raw, &payload); err != nil {
		return application.RegisterAvailabilityAdjustmentCommand{}, err
	}
	tenant, err := tenantSource(payload.TenantID)
	if err != nil {
		return application.RegisterAvailabilityAdjustmentCommand{}, err
	}
	targetKind, err := ports.CatalogTargetKindFrom(payload.TargetKind)
	if err != nil {
		return application.RegisterAvailabilityAdjustmentCommand{}, err
	}
	adjustmentKind, err := ports.AvailabilityAdjustmentKindFrom(payload.Kind)
	if err != nil {
		return application.RegisterAvailabilityAdjustmentCommand{}, err
	}
	return application.RegisterAvailabilityAdjustmentCommand{
		TenantID: tenant,
		Adjustment: ports.AvailabilityAdjustmentStatement{
			Code:        payload.Code,
			Version:     payload.Version,
			TargetKind:  targetKind,
			TargetCode:  payload.TargetCode,
			Kind:        adjustmentKind,
			Source:      payload.Source,
			EffectiveAt: payload.EffectiveAt,
			LiftedAt:    timeOf(payload.LiftedAt),
			HasLiftedAt: payload.LiftedAt != nil,
		},
	}, nil
}

// RouteStrategyVersionFromJSON 是受控批量口那一路。
func RouteStrategyVersionFromJSON(raw []byte) (application.RegisterRouteStrategyVersionCommand, error) {
	return routeStrategyVersionFromJSON(raw, tenantFromDocument)
}

// RouteStrategyVersionFromJSONForTenant 是在线口那一路。
func RouteStrategyVersionFromJSONForTenant(raw []byte, tenant domain.TenantID) (application.RegisterRouteStrategyVersionCommand, error) {
	if err := refuseSelfReportedTenant(raw); err != nil {
		return application.RegisterRouteStrategyVersionCommand{}, err
	}
	return routeStrategyVersionFromJSON(raw, injectedTenant(tenant))
}

func routeStrategyVersionFromJSON(raw []byte, tenantSource tenantOf) (application.RegisterRouteStrategyVersionCommand, error) {
	var payload strategyPayload
	if err := decodeStrict(raw, &payload); err != nil {
		return application.RegisterRouteStrategyVersionCommand{}, err
	}
	tenant, err := tenantSource(payload.TenantID)
	if err != nil {
		return application.RegisterRouteStrategyVersionCommand{}, err
	}
	form := domain.RankingFormUndeclared
	if payload.RankingForm != nil {
		if form, err = domain.RankingFormFrom(*payload.RankingForm); err != nil {
			return application.RegisterRouteStrategyVersionCommand{}, err
		}
	}
	freeze := domain.FreezeFormUndeclared
	var freezeLimit *int
	if payload.FreezeForm != nil || payload.FreezeRemainingSegments != nil {
		if payload.FreezeForm == nil || payload.FreezeRemainingSegments == nil {
			return application.RegisterRouteStrategyVersionCommand{}, fmt.Errorf("freeze form and remaining segments must be declared together")
		}
		if freeze, err = domain.FreezeFormFrom(*payload.FreezeForm); err != nil {
			return application.RegisterRouteStrategyVersionCommand{}, err
		}
		if *payload.FreezeRemainingSegments < 0 {
			return application.RegisterRouteStrategyVersionCommand{}, fmt.Errorf("freeze remaining segments must be >= 0")
		}
		freezeLimit = payload.FreezeRemainingSegments
	}
	autoForm := domain.AutoRerouteFormUndeclared
	var autoThreshold *int
	if payload.AutoRerouteForm != nil || payload.AutoRerouteImprovementThresholdMinor != nil {
		if payload.AutoRerouteForm == nil || payload.AutoRerouteImprovementThresholdMinor == nil {
			return application.RegisterRouteStrategyVersionCommand{}, fmt.Errorf("auto reroute form and improvement threshold must be declared together")
		}
		if autoForm, err = domain.AutoRerouteFormFrom(*payload.AutoRerouteForm); err != nil {
			return application.RegisterRouteStrategyVersionCommand{}, err
		}
		if *payload.AutoRerouteImprovementThresholdMinor < 0 {
			return application.RegisterRouteStrategyVersionCommand{}, fmt.Errorf("auto reroute improvement threshold must be >= 0")
		}
		autoThreshold = payload.AutoRerouteImprovementThresholdMinor
	}
	basis, err := basisFrom(payload.Basis)
	if err != nil {
		return application.RegisterRouteStrategyVersionCommand{}, err
	}
	return application.RegisterRouteStrategyVersionCommand{
		TenantID: tenant,
		Strategy: ports.RouteStrategyDefinitionVersion{
			Code:                                 payload.Code,
			Version:                              payload.Version,
			ApplicableScope:                      payload.ApplicableScope,
			RankingForm:                          form,
			FreezeForm:                           freeze,
			FreezeRemainingSegmentLimit:          freezeLimit,
			AutoRerouteForm:                      autoForm,
			AutoRerouteImprovementThresholdMinor: autoThreshold,
			EffectiveFrom:                        payload.EffectiveFrom,
			EffectiveTo:                          timeOf(payload.EffectiveTo),
			HasEffectiveTo:                       payload.EffectiveTo != nil,
			Basis:                                basis,
		},
	}, nil
}

// decodeStrict 拒未知字段：打错的键静默丢弃，会让登记方以为登进去的比实际多。
func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// timeOf 把可选时刻译回值语义；在不在场由调用处的 Has 位携带。
func timeOf(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

// 七族登记行的 JSON 形状。可选终点用指针表达「不在场」——零时刻是合法的绝对时刻，不能兼作「没有终点」。
// 稳定定义各族另有一格可选的 basis：这一版的登记依据，缺席即没给（译法见 basisFrom）。

type nodePayload struct {
	TenantID         string     `json:"tenant_id"`
	Code             string     `json:"code"`
	Version          int32      `json:"version"`
	BusinessTimezone string     `json:"business_timezone"`
	EffectiveFrom    time.Time  `json:"effective_from"`
	EffectiveTo      *time.Time `json:"effective_to"`
	Basis            *string    `json:"basis"`
}

type connectionPayload struct {
	TenantID         string     `json:"tenant_id"`
	Code             string     `json:"code"`
	Version          int32      `json:"version"`
	FromNode         string     `json:"from_node"`
	ToNode           string     `json:"to_node"`
	BusinessTimezone string     `json:"business_timezone"`
	EffectiveFrom    time.Time  `json:"effective_from"`
	EffectiveTo      *time.Time `json:"effective_to"`
	Basis            *string    `json:"basis"`
}

type linePayload struct {
	TenantID         string     `json:"tenant_id"`
	Code             string     `json:"code"`
	Version          int32      `json:"version"`
	Segments         []string   `json:"segments"`
	BusinessTimezone string     `json:"business_timezone"`
	ApplicableScope  string     `json:"applicable_scope"`
	EffectiveFrom    time.Time  `json:"effective_from"`
	EffectiveTo      *time.Time `json:"effective_to"`
	Basis            *string    `json:"basis"`
	// CostBases 缺席即这一版不挂成本依据；段号对 segments 数组下标。
	CostBases []costBasisPayload `json:"cost_bases"`
}

type costBasisPayload struct {
	SegmentIndex int    `json:"segment_index"`
	Kind         string `json:"kind"`
	Reference    string `json:"reference"`
}

type areaPayload struct {
	TenantID      string     `json:"tenant_id"`
	Code          string     `json:"code"`
	Version       int32      `json:"version"`
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to"`
	Basis         *string    `json:"basis"`
	// Coverage 缺席即这版没登覆盖；给了就由受理门按覆盖文法与节点角色逐格核。
	Coverage *areaCoveragePayload `json:"coverage"`
}

type areaCoveragePayload struct {
	Country          string   `json:"country"`
	PostalPrefixes   []string `json:"postal_prefixes"`
	OriginNodes      []string `json:"origin_nodes"`
	DestinationNodes []string `json:"destination_nodes"`
}

type calendarPayload struct {
	TenantID      string     `json:"tenant_id"`
	TargetKind    string     `json:"target_kind"`
	TargetCode    string     `json:"target_code"`
	Version       int32      `json:"version"`
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to"`
	Basis         *string    `json:"basis"`
	// 三格内容（ADR-0175 决定一）各自可缺：缺席是没登这一格，写 0 才是零分钟。
	CutoffLocalMinute *int `json:"cutoff_local_minute"`
	ProcessingMinutes *int `json:"processing_minutes"`
	BufferMinutes     *int `json:"buffer_minutes"`
}

type adjustmentPayload struct {
	TenantID    string     `json:"tenant_id"`
	Code        string     `json:"code"`
	Version     int32      `json:"version"`
	TargetKind  string     `json:"target_kind"`
	TargetCode  string     `json:"target_code"`
	Kind        string     `json:"kind"`
	Source      string     `json:"source"`
	EffectiveAt time.Time  `json:"effective_at"`
	LiftedAt    *time.Time `json:"lifted_at"`
}

type strategyPayload struct {
	TenantID        string     `json:"tenant_id"`
	Code            string     `json:"code"`
	Version         int32      `json:"version"`
	ApplicableScope string     `json:"applicable_scope"`
	EffectiveFrom   time.Time  `json:"effective_from"`
	EffectiveTo     *time.Time `json:"effective_to"`
	Basis           *string    `json:"basis"`
	// RankingForm 缺席即这一版没有声明排序形态；给了就必须是族内的词，空词同样拒。
	RankingForm *string `json:"ranking_form"`
	// FreezeForm 与 FreezeRemainingSegments 同缺即未声明；只给一半拒。
	FreezeForm              *string `json:"freeze_form"`
	FreezeRemainingSegments *int    `json:"freeze_remaining_segments"`
	// AutoRerouteForm 与阈值同缺即未声明；只给一半拒。阈值是租户取值。
	AutoRerouteForm                      *string `json:"auto_reroute_form"`
	AutoRerouteImprovementThresholdMinor *int    `json:"auto_reroute_improvement_threshold_minor"`
}
