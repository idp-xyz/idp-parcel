package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 服务产品形态册的持久化面（ADR-0050）。装载不在这里而在 CommercialPublications.LoadForScope
// ——形态随整册一次取回，理由见 ports.CommercialPublicationView 的注释。这里只有写侧与把库里
// 的形态字符串折回领域封闭集那一步。

// SaveServiceProduct 登记一份服务产品版本的服务形态。撞键不覆盖：同形态是重放，异形态是
// 需要商业责任方修正的冲突——一次发布固定下来的形态改不了，要改只能另发一个版本。
func (repository *CommercialPublications) SaveServiceProduct(
	ctx context.Context,
	product domain.ServiceProduct,
) (ports.ServiceProductSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ServiceProductSaveOutcomeInvalid, fmt.Errorf("save service product form: %w", err)
	}

	version := product.Version()
	form := product.Form().String()
	if form == "" {
		// 零值形态过不了 NewServiceProduct，走到这里说明调用方绕过了构造函数递来一个零值
		// 产品。拦在写入之前，否则库里会多一行形态为空串的记录，而它读回来什么也不是。
		return ports.ServiceProductSaveOutcomeInvalid,
			fmt.Errorf("save service product form: 服务形态缺失，未经 NewServiceProduct 构造的产品不入册")
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.service_product_form
			(tenant_id, object_kind, object_id, version_label, form)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT DO NOTHING`,
		version.Tenant().String(),
		uint8(version.Kind()),
		version.ObjectID().String(),
		version.Version().String(),
		form,
	)
	if err != nil {
		return ports.ServiceProductSaveOutcomeInvalid, fmt.Errorf("save service product form: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return ports.ServiceProductSaved, nil
	}

	// 撞键：读回既有行判重放还是冲突。本表的正文只有形态一列，因此判据就是它。
	var existingForm string
	err = executor.QueryRow(ctx,
		`SELECT form
		   FROM party_commercial.service_product_form
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		version.Tenant().String(),
		uint8(version.Kind()),
		version.ObjectID().String(),
		version.Version().String(),
	).Scan(&existingForm)
	if errors.Is(err, pgx.ErrNoRows) {
		// 撞键后行又不见了：并发删除在本表不存在，这是库或适配器的 bug。
		return ports.ServiceProductSaveOutcomeInvalid, fmt.Errorf("save service product form: 撞键后读不回既有行")
	}
	if err != nil {
		return ports.ServiceProductSaveOutcomeInvalid, fmt.Errorf("save service product form: %w", err)
	}
	if existingForm == form {
		return ports.ServiceProductAlreadyRegistered, nil
	}
	return ports.ServiceProductContentConflict, nil
}

// registerServiceProduct 把一行形态挂回它的版本，供整册装载调用。
//
// 版本非`已生效`时不进登记册：NewServiceProduct 要求已生效，而解析选候选本来就跳过非生效
// 版本，装进来也永远选不中。这不会让失效检测漏掉任何东西——版本状态本身参与 ViewRevision
// 派生，生效与收尾这两次转变照样推动修订。
//
// 形态取值不认识则上抛而不是跳过。跳过会让登记册少一份形态，解析因此答出一个看起来完全
// 正常的`唯一已解析`、只是形态不可观察——而那与「这个产品根本没登记形态」在调用方那里
// 分不开，一次坏数据会伪装成一格合法的缺席。
func registerServiceProduct(
	registry *domain.CommercialRegistry,
	version domain.CommercialVersion,
	rawForm string,
) error {
	form, err := serviceProductFormFrom(rawForm)
	if err != nil {
		return err
	}
	if version.Status() != domain.CommercialVersionEffective {
		return nil
	}
	product, err := domain.NewServiceProduct(version, form)
	if err != nil {
		return err
	}
	registry.RegisterServiceProduct(product)
	return nil
}

// serviceProductFormFrom 把库里的形态字符串折回领域封闭集。集外取值上抛，不折成某个默认
// 形态：库上的 CHECK 与领域封闭集是同步扩展的一对，读到集外的值说明这一对已经脱节，那要
// 人去看，不该被静默吞成一种形态。
func serviceProductFormFrom(raw string) (domain.ServiceProductForm, error) {
	switch raw {
	case domain.NetworkServiceForm.String():
		return domain.NetworkServiceForm, nil
	case domain.LabelChannelServiceForm.String():
		return domain.LabelChannelServiceForm, nil
	default:
		return domain.ServiceProductFormInvalid, fmt.Errorf("unknown service product form %q", raw)
	}
}
