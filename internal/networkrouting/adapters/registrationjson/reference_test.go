package registrationjson

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/referenceconfig"
)

// 本文件是网络目录参考配置的发布门（ADR-0147 决定五）：每份已发布的版本逐行经采用路径的同一段翻译、再过登记用例的
// 受理门；网络自己前后对得上，采用之后折得出候选与时间投影，不是一堆各自合格、拼不成路的行。

// recordingRegistry 是写入口的替身，只记到达的行；受理门在它前面，过不了门的行到不了这里。
type recordingRegistry struct {
	rows int
}

func (registry *recordingRegistry) RegisterNodeVersion(context.Context, domain.TenantID, ports.NodeDefinitionVersion) error {
	registry.rows++
	return nil
}

func (registry *recordingRegistry) RegisterConnectionVersion(context.Context, domain.TenantID, ports.ConnectionDefinitionVersion) error {
	registry.rows++
	return nil
}

func (registry *recordingRegistry) RegisterLineVersion(context.Context, domain.TenantID, ports.LineDefinitionVersion) error {
	registry.rows++
	return nil
}

func (registry *recordingRegistry) RegisterLineCostBases(context.Context, domain.TenantID, string, int32, []ports.LineSegmentCostBasis) error {
	return nil
}

func (registry *recordingRegistry) RegisterServiceAreaVersion(context.Context, domain.TenantID, ports.ServiceAreaDefinitionVersion) error {
	registry.rows++
	return nil
}

func (registry *recordingRegistry) RegisterServiceCalendarVersion(context.Context, domain.TenantID, ports.ServiceCalendarDefinitionVersion) error {
	registry.rows++
	return nil
}

func (registry *recordingRegistry) RegisterRouteStrategyVersion(context.Context, domain.TenantID, ports.RouteStrategyDefinitionVersion) error {
	registry.rows++
	return nil
}

func (registry *recordingRegistry) RegisterAvailabilityAdjustment(context.Context, domain.TenantID, ports.AvailabilityAdjustmentStatement) error {
	registry.rows++
	return nil
}

func releasedNetworkCatalogs(t *testing.T) []referenceconfig.Reference {
	t.Helper()
	var found []referenceconfig.Reference
	for _, reference := range referenceconfig.Released() {
		if strings.HasPrefix(reference.Identifier(), networkCatalogReferenceDirectory) {
			found = append(found, reference)
		}
	}
	if len(found) == 0 {
		t.Fatal("没有任何已发布的网络目录参考配置")
	}
	return found
}

// adoptRow 是演示租户采用某一行时会写的那一行：只有身份、修订号与生效时点。
func adoptRow(reference referenceconfig.Reference, identity string) []byte {
	return []byte(fmt.Sprintf(`{"tenant_id": "SYN-TENANT-01", %s, "version": 1, "effective_from": "2026-01-01T00:00:00Z", "adopt": %q}`,
		identity, reference.String()))
}

func codeIdentity(code string) string { return fmt.Sprintf(`"code": %q`, code) }

// Covers: ADR-0147 决定五——每份已发布的网络目录参考配置，每一行经采用路径的同一段翻译，过登记用例的受理门答
// `已登记`，依据格是这一版的引用串。构造门或受理门不过的文件不能发布。
func TestEveryReleasedNetworkCatalogReferencePassesTheRegistrationGates(t *testing.T) {
	for _, reference := range releasedNetworkCatalogs(t) {
		document, basis, err := adoptedReference(reference.String())
		if err != nil {
			t.Fatalf("%s 打不开：%v", reference, err)
		}
		if basis.String() != reference.Citation() {
			t.Fatalf("%s 的依据 = %q，想要 %q", reference, basis, reference.Citation())
		}
		registry := &recordingRegistry{}
		registration, err := application.NewNetworkCatalogRegistration(registry)
		if err != nil {
			t.Fatalf("构造登记用例：%v", err)
		}
		ctx := t.Context()
		expect := func(what string, result application.RegisterCatalogResult, err error) {
			t.Helper()
			if err != nil || result.Outcome() != application.CatalogRegistered {
				t.Fatalf("%s %s：outcome=%s refusal=%s err=%v", reference, what, result.Outcome(), result.RefusalReason(), err)
			}
		}
		for _, node := range document.Nodes {
			command, err := NodeVersionFromJSON(adoptRow(reference, codeIdentity(node.Code)))
			if err != nil || command.Node.Basis != basis {
				t.Fatalf("%s 节点 %s 译装：%+v err=%v", reference, node.Code, command, err)
			}
			result, err := registration.RegisterNodeVersion(ctx, command)
			expect("节点 "+node.Code, result, err)
		}
		for _, connection := range document.Connections {
			command, err := ConnectionVersionFromJSON(adoptRow(reference, codeIdentity(connection.Code)))
			if err != nil || command.Connection.Basis != basis {
				t.Fatalf("%s 连接 %s 译装：%+v err=%v", reference, connection.Code, command, err)
			}
			result, err := registration.RegisterConnectionVersion(ctx, command)
			expect("连接 "+connection.Code, result, err)
		}
		for _, line := range document.Lines {
			command, err := LineVersionFromJSON(adoptRow(reference, codeIdentity(line.Code)))
			if err != nil || command.Line.Basis != basis {
				t.Fatalf("%s 线路 %s 译装：%+v err=%v", reference, line.Code, command, err)
			}
			result, err := registration.RegisterLineVersion(ctx, command)
			expect("线路 "+line.Code, result, err)
		}
		for _, area := range document.ServiceAreas {
			command, err := ServiceAreaVersionFromJSON(adoptRow(reference, codeIdentity(area.Code)))
			if err != nil || command.Area.Basis != basis {
				t.Fatalf("%s 服务区域 %s 译装：%+v err=%v", reference, area.Code, command, err)
			}
			result, err := registration.RegisterServiceAreaVersion(ctx, command)
			expect("服务区域 "+area.Code, result, err)
		}
		for _, calendar := range document.ServiceCalendars {
			identity := fmt.Sprintf(`"target_kind": %q, "target_code": %q`, calendar.TargetKind, calendar.TargetCode)
			command, err := ServiceCalendarVersionFromJSON(adoptRow(reference, identity))
			if err != nil || command.Calendar.Basis != basis {
				t.Fatalf("%s 日历 %s/%s 译装：%+v err=%v", reference, calendar.TargetKind, calendar.TargetCode, command, err)
			}
			result, err := registration.RegisterServiceCalendarVersion(ctx, command)
			expect("日历 "+calendar.TargetKind+"/"+calendar.TargetCode, result, err)
		}
		for _, strategy := range document.RouteStrategies {
			command, err := RouteStrategyVersionFromJSON(adoptRow(reference, codeIdentity(strategy.Code)))
			if err != nil || command.Strategy.Basis != basis {
				t.Fatalf("%s 路由策略 %s 译装：%+v err=%v", reference, strategy.Code, command, err)
			}
			result, err := registration.RegisterRouteStrategyVersion(ctx, command)
			expect("路由策略 "+strategy.Code, result, err)
		}
		if registry.rows == 0 {
			t.Fatalf("%s 一行都没有", reference)
		}
	}
}

// Covers: 每份网络目录参考配置自己拼得成路（ADR-0148 决定五、ADR-0175 决定三）：连接两端是本份的节点，线路段链是本份
// 首尾相接的连接，首节点是某个服务区域的收寄节点、末节点是某个服务区域的交付节点，段链上每个节点都有带处理时长的
// 节点日历，日历指的对象在本份里，每版策略的适用范围都有线路对得上。键以 SYN- 起头的是演示网络，里面的身份全是
// SYN- 合成值。
func TestEveryReleasedNetworkCatalogReferenceHangsTogether(t *testing.T) {
	for _, reference := range releasedNetworkCatalogs(t) {
		document, _, err := adoptedReference(reference.String())
		if err != nil {
			t.Fatalf("%s 打不开：%v", reference, err)
		}
		synthetic := strings.HasPrefix(strings.TrimPrefix(reference.Identifier(), networkCatalogReferenceDirectory), "SYN-")
		identities := []string{}
		nodes := map[string]bool{}
		for _, node := range document.Nodes {
			nodes[node.Code] = true
			identities = append(identities, node.Code)
		}
		connections := map[string]referenceConnection{}
		for _, connection := range document.Connections {
			if !nodes[connection.FromNode] || !nodes[connection.ToNode] {
				t.Fatalf("%s 连接 %s 的端点不在本份节点里", reference, connection.Code)
			}
			connections[connection.Code] = connection
			identities = append(identities, connection.Code)
		}
		origins, destinations := map[string]bool{}, map[string]bool{}
		for _, area := range document.ServiceAreas {
			identities = append(identities, area.Code)
			if area.Coverage == nil {
				t.Fatalf("%s 服务区域 %s 没登覆盖：它解析不了任何地址", reference, area.Code)
			}
			for _, node := range append(append([]string{}, area.Coverage.OriginNodes...), area.Coverage.DestinationNodes...) {
				if !nodes[node] {
					t.Fatalf("%s 服务区域 %s 的节点 %s 不在本份节点里", reference, area.Code, node)
				}
			}
			for _, node := range area.Coverage.OriginNodes {
				origins[node] = true
			}
			for _, node := range area.Coverage.DestinationNodes {
				destinations[node] = true
			}
		}
		processing := map[string]bool{}
		lines := map[string]bool{}
		for _, line := range document.Lines {
			lines[line.Code] = true
		}
		for _, calendar := range document.ServiceCalendars {
			identities = append(identities, calendar.TargetCode)
			switch calendar.TargetKind {
			case "NODE":
				if !nodes[calendar.TargetCode] {
					t.Fatalf("%s 日历指的节点 %s 不在本份里", reference, calendar.TargetCode)
				}
				if calendar.ProcessingMinutes != nil {
					processing[calendar.TargetCode] = true
				}
			case "CONNECTION":
				if _, found := connections[calendar.TargetCode]; !found {
					t.Fatalf("%s 日历指的连接 %s 不在本份里", reference, calendar.TargetCode)
				}
			case "LINE":
				if !lines[calendar.TargetCode] {
					t.Fatalf("%s 日历指的线路 %s 不在本份里", reference, calendar.TargetCode)
				}
			}
		}
		scopes := map[string]bool{}
		for _, line := range document.Lines {
			identities = append(identities, line.Code)
			scopes[line.ApplicableScope] = true
			var path []string
			for index, segment := range line.Segments {
				connection, found := connections[segment]
				if !found {
					t.Fatalf("%s 线路 %s 的段 %s 不在本份连接里", reference, line.Code, segment)
				}
				if index == 0 {
					path = append(path, connection.FromNode)
				} else if path[len(path)-1] != connection.FromNode {
					t.Fatalf("%s 线路 %s 的段 %s 没接上前一段", reference, line.Code, segment)
				}
				path = append(path, connection.ToNode)
			}
			if !origins[path[0]] || !destinations[path[len(path)-1]] {
				t.Fatalf("%s 线路 %s 首节点不收寄或末节点不交付：%v", reference, line.Code, path)
			}
			for _, node := range path {
				if !processing[node] {
					t.Fatalf("%s 线路 %s 经过的节点 %s 没有带处理时长的日历：候选得不到时间投影", reference, line.Code, node)
				}
			}
		}
		for _, strategy := range document.RouteStrategies {
			identities = append(identities, strategy.Code)
			if !scopes[strategy.ApplicableScope] {
				t.Fatalf("%s 路由策略 %s 的适用范围 %s 没有线路对得上", reference, strategy.Code, strategy.ApplicableScope)
			}
		}
		if synthetic {
			for _, identity := range identities {
				if !strings.HasPrefix(identity, "SYN-") {
					t.Fatalf("%s 是演示网络，身份 %s 不是 SYN- 合成值", reference, identity)
				}
			}
		}
	}
}
