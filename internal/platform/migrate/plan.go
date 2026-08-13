// Package migrate 拥有 Parcel 的迁移计划。它把框架的不可变迁移资产排成一份有序
// 历史并记录实际施加了什么；它从不编辑框架模板，也从不在应用启动时运行。
package migrate

import (
	"fmt"

	"go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/migrations"
)

// 迁移作业在施加计划前创建的 schema。框架技术表留在自己的 schema 里，业务迁移
// 因而不可能改动它们的形状。
const (
	// SchemaBento 归框架的技术表所有。
	SchemaBento = "bento"
	// SchemaParcelShipment 归 parcel-shipment 的业务表所有。
	SchemaParcelShipment = "parcel_shipment"
	// SchemaNetworkRouting 归 network-routing 的业务表所有。
	SchemaNetworkRouting = "network_routing"
	// SchemaNodeOperations 归 node-operations 的业务表所有。
	SchemaNodeOperations = "node_operations"
	// SchemaVisibilityException 归 visibility-exception 的业务表所有。
	SchemaVisibilityException = "visibility_exception"
	// SchemaSettlementAccounting 归 settlement-accounting 的业务表所有。
	SchemaSettlementAccounting = "settlement_accounting"
	// SchemaCustomsCompliance 归 customs-compliance 的业务表所有。
	SchemaCustomsCompliance = "customs_compliance"
	// SchemaTransportFulfillment 归 transport-fulfillment 的业务表所有。
	SchemaTransportFulfillment = "transport_fulfillment"
	// SchemaPilotGovernance 归 pilot-governance 的治理记录表所有。
	SchemaPilotGovernance = "pilot_governance"
	// SchemaPartyCommercial 归 party-commercial 的业务表所有。
	SchemaPartyCommercial = "party_commercial"
	// SchemaParcelPricing 归 parcel-pricing 的业务表所有。
	SchemaParcelPricing = "parcel_pricing"
	// SchemaHistory 归 Parcel 的迁移历史所有，既不是框架 schema 也不是业务 schema。
	SchemaHistory = "parcel_migration"
)

// FrameworkVersion 是本计划所施加迁移资产的精确候选版本。
//
// 它与 `tests/bentocontract` 的候选身份必须一致，由该包的测试比对——此处不 import
// 那个包：生产代码依赖合同测试包会把方向倒过来。
const FrameworkVersion = "v0.1.0-rc.2"

// Origin 区分迁移的所有者。框架迁移的模板不可编辑，Parcel 只能按 ID 渲染；
// 业务迁移由 Parcel 自己拥有并使用显式 SQL。
type Origin string

const (
	OriginFramework Origin = "FRAMEWORK"
	OriginParcel    Origin = "PARCEL"
)

// Step 是 Parcel 迁移历史中的一次有序迁移。
type Step struct {
	ID     string
	Origin Origin
	Schema string
	// Checksum 是规范模板的校验和。框架步骤取框架清单里的值，不自行计算——
	// 自行计算等于给同一件事立第二个口径，模板一旦被复制改写就看不出来了。
	Checksum string
	// FrameworkVersion 只在框架步骤上设置。
	FrameworkVersion string
	UpSQL            string
}

// Plan 返回 Parcel 施加的有序迁移历史：先框架技术表，后业务表。
//
// 框架在前不是习惯问题：业务表可以引用框架已建立的东西，反过来不成立——框架的
// 迁移模板不知道任何业务上下文的存在。
//
// **只接线 SQL 已随提交落库的模块**：新模块的目录、嵌入行、模块函数与本处接线必须
// 同一笔提交一起落——半截接线会让干净检出编译不过或 Plan 读不到嵌入目录（本文件
// 已多次因跨会话卷带断过远端构建）。
func Plan() ([]Step, error) {
	steps, err := frameworkSteps()
	if err != nil {
		return nil, err
	}
	shipment, err := businessSteps(migrations.ParcelShipment, SchemaParcelShipment)
	if err != nil {
		return nil, err
	}
	routing, err := businessSteps(migrations.NetworkRouting, SchemaNetworkRouting)
	if err != nil {
		return nil, err
	}
	nodes, err := businessSteps(migrations.NodeOperations, SchemaNodeOperations)
	if err != nil {
		return nil, err
	}
	visibility, err := businessSteps(migrations.VisibilityException, SchemaVisibilityException)
	if err != nil {
		return nil, err
	}
	settlement, err := businessSteps(migrations.SettlementAccounting, SchemaSettlementAccounting)
	if err != nil {
		return nil, err
	}
	customs, err := businessSteps(migrations.CustomsCompliance, SchemaCustomsCompliance)
	if err != nil {
		return nil, err
	}
	transport, err := businessSteps(migrations.TransportFulfillment, SchemaTransportFulfillment)
	if err != nil {
		return nil, err
	}
	governance, err := businessSteps(migrations.PilotGovernance, SchemaPilotGovernance)
	if err != nil {
		return nil, err
	}
	commercial, err := businessSteps(migrations.PartyCommercial, SchemaPartyCommercial)
	if err != nil {
		return nil, err
	}
	pricing, err := businessSteps(migrations.ParcelPricing, SchemaParcelPricing)
	if err != nil {
		return nil, err
	}
	steps = append(steps, shipment...)
	steps = append(steps, routing...)
	steps = append(steps, nodes...)
	steps = append(steps, visibility...)
	steps = append(steps, settlement...)
	steps = append(steps, customs...)
	steps = append(steps, transport...)
	steps = append(steps, governance...)
	steps = append(steps, commercial...)
	steps = append(steps, pricing...)
	return steps, nil
}

// Schemas 返回迁移作业在施加计划前创建的 schema。生产 API 与 Outbox 账号不持有
// 创建它们的权限。
func Schemas() []string {
	return []string{
		SchemaHistory, SchemaBento,
		SchemaParcelShipment, SchemaNetworkRouting, SchemaNodeOperations, SchemaVisibilityException,
		SchemaSettlementAccounting, SchemaCustomsCompliance, SchemaTransportFulfillment,
		SchemaPilotGovernance, SchemaPartyCommercial, SchemaParcelPricing,
	}
}

func businessSteps(load func() ([]migrations.Asset, error), schema string) ([]Step, error) {
	assets, err := load()
	if err != nil {
		return nil, err
	}

	steps := make([]Step, 0, len(assets))
	for _, asset := range assets {
		steps = append(steps, Step{
			ID:     asset.Module + "/" + asset.Name,
			Origin: OriginParcel,
			Schema: schema,
			// 业务迁移的校验和由 Parcel 自己对文件内容计算；框架那半取框架清单里
			// 的值。两者不混：模板归框架认定，自有 SQL 归自己认定。
			Checksum: asset.Checksum,
			UpSQL:    asset.SQL,
		})
	}
	return steps, nil
}

func frameworkSteps() ([]Step, error) {
	assets := postgres.Migrations()
	if len(assets) == 0 {
		return nil, fmt.Errorf("migrate: 框架未提供任何迁移资产；计划为空会让 schema 检查空过")
	}

	steps := make([]Step, 0, len(assets))
	for _, asset := range assets {
		rendered, err := postgres.RenderMigration(asset.ID, SchemaBento)
		if err != nil {
			return nil, fmt.Errorf("migrate: 渲染框架迁移 %s：%w", asset.ID, err)
		}
		steps = append(steps, Step{
			// 前缀标出所有者，使历史表里框架步骤与将来的业务步骤不可能重名。
			ID:               "framework/" + asset.ID,
			Origin:           OriginFramework,
			Schema:           SchemaBento,
			Checksum:         asset.Checksum,
			FrameworkVersion: FrameworkVersion,
			UpSQL:            rendered,
		})
	}
	return steps, nil
}
