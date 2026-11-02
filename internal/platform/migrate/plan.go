// Package migrate 拥有 Parcel 的迁移计划。它把框架的不可变迁移资产排成一份有序
// 历史并记录实际施加了什么；它从不编辑框架模板，也从不在应用启动时运行。
package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"go.idp.xyz/idp-bento-go/postgres"
	numberingpg "go.idp.xyz/idp-platform-go/numbering/postgres"

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
	// SchemaCollectionRemittance 归 collection-remittance 的业务表所有。代收本金与
	// 运营结算是两个口径，两边不共享表，因此也不合用 settlement_accounting 那个 schema。
	SchemaCollectionRemittance = "collection_remittance"
	// SchemaAccessIdentity 归共享接入身份能力的登记册所有（ADR-0072 一、ADR-0100）。它不是限界
	// 上下文，但册的数据归它，不借任何业务上下文的 schema。
	SchemaAccessIdentity = "access_identity"
	// SchemaPlatform 归公司平台库的技术表所有（ADR-0355 决定四：编号序列表）。资产归平台库、执行与权限归本仓，
	// 与 SchemaBento 同一种关系；不并入任何业务 schema，业务迁移因而改不到它的形状。
	SchemaPlatform = "platform"
	// SchemaDocumentNumbering 归单据编号规则册所有（ADR-0355 决定六）。它是编号技术组件的配置，不是限界上下文，
	// 但册的数据归本仓，不放进平台库的 schema——那里的资产只能由平台库发版改。
	SchemaDocumentNumbering = "document_numbering"
	// SchemaHistory 归 Parcel 的迁移历史所有，既不是框架 schema 也不是业务 schema。
	SchemaHistory = "parcel_migration"
)

// FrameworkVersion 是本计划所施加迁移资产的精确候选版本。
//
// 它与 `tests/bentocontract` 的候选身份必须一致，由该包的测试比对——此处不 import
// 那个包：生产代码依赖合同测试包会把方向倒过来。
const FrameworkVersion = "v0.1.0-rc.2"

// PlatformVersion 是本计划所施加平台库迁移资产的精确版本，与 go.mod 锁定的 `go.idp.xyz/idp-platform-go`
// 必须一致，由本包测试比对 go.mod 与 go.sum。迁移历史把它写进已部署的库，两边不一致库就在声称一个它没用过的工件。
const PlatformVersion = "v0.1.0-rc.1"

// Origin 区分迁移的所有者。框架迁移的模板不可编辑，Parcel 只能按 ID 渲染；
// 业务迁移由 Parcel 自己拥有并使用显式 SQL。
type Origin string

const (
	OriginFramework Origin = "FRAMEWORK"
	OriginParcel    Origin = "PARCEL"
	// OriginPlatform 是公司平台库的迁移：与框架一样只能按 ID 渲染、不可编辑，但版本是平台库自己的。
	OriginPlatform Origin = "PLATFORM"
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
	// PlatformVersion 只在平台步骤上设置。
	PlatformVersion string
	UpSQL           string
}

// Plan 返回 Parcel 施加的有序迁移历史：先框架技术表，再平台技术表，后业务表。
//
// 框架在前不是习惯问题：业务表可以引用框架已建立的东西，反过来不成立——框架的
// 迁移模板不知道任何业务上下文的存在。平台技术表排在框架之后、业务之前，理由相同：
// 它们不知道任何业务上下文，业务表却可能用到它们。
//
// **只接线 SQL 已随提交落库的模块**：新模块的目录、嵌入行、模块函数与本处接线必须
// 同一笔提交一起落——半截接线会让干净检出编译不过或 Plan 读不到嵌入目录（本文件
// 已多次因跨会话卷带断过远端构建）。
func Plan() ([]Step, error) {
	steps, err := frameworkSteps()
	if err != nil {
		return nil, err
	}
	platform, err := platformSteps()
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
	collection, err := businessSteps(migrations.CollectionRemittance, SchemaCollectionRemittance)
	if err != nil {
		return nil, err
	}
	access, err := businessSteps(migrations.AccessIdentity, SchemaAccessIdentity)
	if err != nil {
		return nil, err
	}
	numbering, err := businessSteps(migrations.DocumentNumbering, SchemaDocumentNumbering)
	if err != nil {
		return nil, err
	}
	steps = append(steps, platform...)
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
	steps = append(steps, collection...)
	steps = append(steps, access...)
	steps = append(steps, numbering...)
	return steps, nil
}

// Schemas 返回迁移作业在施加计划前创建的 schema。生产 API 与 Outbox 账号不持有
// 创建它们的权限。
func Schemas() []string {
	return []string{
		SchemaHistory, SchemaBento, SchemaPlatform,
		SchemaParcelShipment, SchemaNetworkRouting, SchemaNodeOperations, SchemaVisibilityException,
		SchemaSettlementAccounting, SchemaCustomsCompliance, SchemaTransportFulfillment,
		SchemaPilotGovernance, SchemaPartyCommercial, SchemaParcelPricing,
		SchemaCollectionRemittance, SchemaAccessIdentity, SchemaDocumentNumbering,
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

// platformSteps 渲染公司平台库的迁移资产（ADR-0355 决定四：编号序列表）。
//
// 校验和由本仓对渲染结果算，与框架那半不同：平台库只交 ID 与渲染后的 SQL，没有校验和清单。算在渲染结果上
// 仍守得住漂移——schema 是本包常量，同一个平台库版本渲染出的 SQL 逐字不变；平台库若在新版本里改写了已发布
// 的资产（它承诺只增不改），已部署的库会在下一次运行时以 ErrChecksumDrift 拦下，而不是悄悄施加。
func platformSteps() ([]Step, error) {
	assets, err := numberingpg.Migrations(SchemaPlatform)
	if err != nil {
		return nil, fmt.Errorf("migrate: 渲染平台编号迁移：%w", err)
	}
	if len(assets) == 0 {
		return nil, fmt.Errorf("migrate: 平台库未提供任何编号迁移资产；序列表缺席会让取号在运行时才失败")
	}

	steps := make([]Step, 0, len(assets))
	for _, asset := range assets {
		sum := sha256.Sum256([]byte(asset.SQL))
		steps = append(steps, Step{
			// 前缀标出所有者，与框架步骤、业务步骤不可能重名。
			ID:              "platform/" + asset.ID,
			Origin:          OriginPlatform,
			Schema:          SchemaPlatform,
			Checksum:        "sha256:" + hex.EncodeToString(sum[:]),
			PlatformVersion: PlatformVersion,
			UpSQL:           asset.SQL,
		})
	}
	return steps, nil
}
