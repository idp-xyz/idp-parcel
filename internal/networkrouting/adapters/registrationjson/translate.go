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
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// ErrSelfReportedTenant 表示在线口的批文里带了 tenant_id：租户只从认证结果来。
var ErrSelfReportedTenant = errors.New("network registration: online input must not carry tenant_id; the tenant comes from the operator envelope")

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
	return application.RegisterNodeVersionCommand{
		TenantID: tenant,
		Node: ports.NodeDefinitionVersion{
			Code:             payload.Code,
			Version:          payload.Version,
			BusinessTimezone: payload.BusinessTimezone,
			EffectiveFrom:    payload.EffectiveFrom,
			EffectiveTo:      timeOf(payload.EffectiveTo),
			HasEffectiveTo:   payload.EffectiveTo != nil,
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
		},
	}, nil
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
	area := ports.ServiceAreaDefinitionVersion{
		Code:           payload.Code,
		Version:        payload.Version,
		EffectiveFrom:  payload.EffectiveFrom,
		EffectiveTo:    timeOf(payload.EffectiveTo),
		HasEffectiveTo: payload.EffectiveTo != nil,
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
	return application.RegisterServiceCalendarVersionCommand{
		TenantID: tenant,
		Calendar: ports.ServiceCalendarDefinitionVersion{
			TargetKind:     targetKind,
			TargetCode:     payload.TargetCode,
			Version:        payload.Version,
			EffectiveFrom:  payload.EffectiveFrom,
			EffectiveTo:    timeOf(payload.EffectiveTo),
			HasEffectiveTo: payload.EffectiveTo != nil,
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
	return application.RegisterRouteStrategyVersionCommand{
		TenantID: tenant,
		Strategy: ports.RouteStrategyDefinitionVersion{
			Code:            payload.Code,
			Version:         payload.Version,
			ApplicableScope: payload.ApplicableScope,
			RankingForm:     form,
			EffectiveFrom:   payload.EffectiveFrom,
			EffectiveTo:     timeOf(payload.EffectiveTo),
			HasEffectiveTo:  payload.EffectiveTo != nil,
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

type nodePayload struct {
	TenantID         string     `json:"tenant_id"`
	Code             string     `json:"code"`
	Version          int32      `json:"version"`
	BusinessTimezone string     `json:"business_timezone"`
	EffectiveFrom    time.Time  `json:"effective_from"`
	EffectiveTo      *time.Time `json:"effective_to"`
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
}

type areaPayload struct {
	TenantID      string     `json:"tenant_id"`
	Code          string     `json:"code"`
	Version       int32      `json:"version"`
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to"`
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
	// RankingForm 缺席即这一版没有声明排序形态；给了就必须是族内的词，空词同样拒。
	RankingForm *string `json:"ranking_form"`
}
