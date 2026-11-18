package migrate_test

import (
	"slices"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// Schemas() 是迁移作业在施加计划前创建的 schema 清单，生产账号不持有创建它们的权限。计划里每一步落进的 schema 要是
// 不在清单里，干净库上施加到那一步才报「schema 不存在」，而不是在装配时就暴露。
func TestEverySchemaAStepAppliesInIsCreatedBeforehand(t *testing.T) {
	plan, err := migrate.Plan()
	if err != nil {
		t.Fatalf("取迁移计划：%v", err)
	}
	created := migrate.Schemas()
	seen := map[string]bool{}
	for _, schema := range created {
		if seen[schema] {
			t.Errorf("Schemas() 里 %q 出现了两次", schema)
		}
		seen[schema] = true
	}
	for _, step := range plan {
		if !seen[step.Schema] {
			t.Errorf("%s 落进 schema %q，它不在 Schemas() 里", step.ID, step.Schema)
		}
	}
}

// Covers: 票面「迁移」——仓配履约的 schema 登进同一份迁移计划，次序在单据编号这一最后登记的业务 schema 之后，它的第一号迁移建仓修订表。
// 开关关着的部署也施加它，所以它不能靠开关从计划里摘掉。
func TestWarehouseFulfillmentSchemaIsPlannedAfterTheOtherBusinessSchemas(t *testing.T) {
	schemas := migrate.Schemas()
	numbering := slices.Index(schemas, migrate.SchemaDocumentNumbering)
	warehouse := slices.Index(schemas, migrate.SchemaWarehouseFulfillment)
	if migrate.SchemaWarehouseFulfillment != "warehouse_fulfillment" || warehouse < 0 || numbering < 0 || warehouse <= numbering {
		t.Fatalf("Schemas() 里 warehouse_fulfillment 位于 %d、document_numbering 位于 %d，want 前者在后者之后", warehouse, numbering)
	}

	plan, err := migrate.Plan()
	if err != nil {
		t.Fatalf("取迁移计划：%v", err)
	}
	lastNumbering, firstWarehouse := -1, -1
	for index, step := range plan {
		switch step.Schema {
		case migrate.SchemaDocumentNumbering:
			lastNumbering = index
		case migrate.SchemaWarehouseFulfillment:
			if firstWarehouse < 0 {
				firstWarehouse = index
			}
			if step.Origin != migrate.OriginParcel || !strings.HasPrefix(step.ID, "warehouse_fulfillment/") {
				t.Errorf("仓配履约的迁移步骤 %+v 的来源或标识不对", step)
			}
		}
	}
	if firstWarehouse < 0 || lastNumbering < 0 || firstWarehouse <= lastNumbering {
		t.Fatalf("计划里仓配履约的第一步在 %d、单据编号的最后一步在 %d，want 前者在后者之后", firstWarehouse, lastNumbering)
	}
	if plan[firstWarehouse].ID != "warehouse_fulfillment/0001_warehouse_revision" {
		t.Errorf("仓配履约的第一号迁移 = %q，want warehouse_fulfillment/0001_warehouse_revision", plan[firstWarehouse].ID)
	}
}
