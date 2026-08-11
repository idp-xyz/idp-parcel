// Package migrate 拥有 Parcel 的迁移计划。它把框架的不可变迁移资产排成一份有序
// 历史并记录实际施加了什么；它从不编辑框架模板，也从不在应用启动时运行。
package migrate

import (
	"fmt"

	"go.idp.xyz/idp-bento-go/postgres"
)

// 迁移作业在施加计划前创建的 schema。框架技术表留在自己的 schema 里，业务迁移
// 因而不可能改动它们的形状。
const (
	// SchemaBento 归框架的技术表所有。
	SchemaBento = "bento"
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

// Plan 返回 Parcel 施加的有序迁移历史。
//
// 当前只含框架那一半。业务迁移要按 `parcel_shipment` 的表形状写，而那取决于尚在
// 成形的领域模型；先摆一个空的业务目录不会让任何东西更早可用，只会多一处将来要
// 改的空壳。`PBC-06` 要证的恰好也只是框架迁移这一半。
func Plan() ([]Step, error) {
	return frameworkSteps()
}

// Schemas 返回迁移作业在施加计划前创建的 schema。生产 API 与 Outbox 账号不持有
// 创建它们的权限。
func Schemas() []string {
	return []string{SchemaHistory, SchemaBento}
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
