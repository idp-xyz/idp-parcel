package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 价格政策口径册的持久化面（票 party-commercial-context-gaps/02、06，0022 迁移）。写口挂在
// CommercialPublications 上，与价格政策正文同笔登记；读口是独立的 PricePolicyCaliberContents
// ——口径不进整册装载（LoadForScope），理由见 ports.PricePolicyCaliberView。

// pgForeignKeyViolation 是 PostgreSQL 的 foreign_key_violation 代码。0022 的复合外键同时表达
// 两条领域规则（正文行先在、方向一致），撞上它时把两条都说出来，免得读到一串约束名去查表。
const pgForeignKeyViolation = "23503"

// SavePricePolicyCaliber 登记一份价格规则版本声明的计价口径。撞键不覆盖：同内容是重放，异内容
// （税务、体积、汇率任一格不同，含汇率格从缺席变在场）是需要商业责任方修正的冲突。
func (repository *CommercialPublications) SavePricePolicyCaliber(
	ctx context.Context,
	caliber domain.PricePolicyCaliber,
) (ports.PricePolicyCaliberSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PricePolicyCaliberSaveOutcomeInvalid, fmt.Errorf("save price policy caliber: %w", err)
	}

	columns, ok := pricePolicyCaliberColumns(caliber)
	if !ok {
		// 零值口径过不了两道构造门，走到这里只可能是绕开它们的零值。拦在 INSERT 前，否则
		// CHECK 会以一条技术错误报出一件领域上早该拒绝的事。
		return ports.PricePolicyCaliberSaveOutcomeInvalid,
			fmt.Errorf("save price policy caliber: %w", domain.ErrInvalidPricePolicyCaliber)
	}
	version := caliber.Version()

	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.price_policy_caliber
			(tenant_id, object_kind, object_id, version_label, direction,
			 tax_disposition, tax_classification_ref, volumetric_factor_ref,
			 fx_quote_type_ref, fx_as_of_semantics_ref, fx_as_of_policy_version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT DO NOTHING`,
		version.Tenant().String(),
		uint8(version.Kind()),
		version.ObjectID().String(),
		version.Version().String(),
		columns.direction,
		columns.taxDisposition,
		columns.taxClassification,
		columns.volumetricFactor,
		columns.fxQuoteType,
		columns.fxAsOfSemantics,
		columns.fxAsOfPolicyVersion,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation {
			return ports.PricePolicyCaliberSaveOutcomeInvalid, fmt.Errorf(
				"save price policy caliber: 找不到同方向（%s）的价格政策正文行——正文未登记，或口径的体积方向与政策方向不一致：%w",
				columns.direction, err)
		}
		return ports.PricePolicyCaliberSaveOutcomeInvalid, fmt.Errorf("save price policy caliber: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return ports.PricePolicyCaliberSaved, nil
	}

	var existing scannedPricePolicyCaliber
	err = executor.QueryRow(ctx,
		`SELECT direction, tax_disposition, tax_classification_ref, volumetric_factor_ref,
		        fx_quote_type_ref, fx_as_of_semantics_ref, fx_as_of_policy_version
		   FROM party_commercial.price_policy_caliber
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		version.Tenant().String(),
		uint8(version.Kind()),
		version.ObjectID().String(),
		version.Version().String(),
	).Scan(&existing.direction, &existing.taxDisposition, &existing.taxClassification, &existing.volumetricFactor,
		&existing.fxQuoteType, &existing.fxAsOfSemantics, &existing.fxAsOfPolicyVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.PricePolicyCaliberSaveOutcomeInvalid, fmt.Errorf("save price policy caliber: 撞键后读不回既有行")
	}
	if err != nil {
		return ports.PricePolicyCaliberSaveOutcomeInvalid, fmt.Errorf("save price policy caliber: %w", err)
	}
	if existing.sameAs(columns) {
		return ports.PricePolicyCaliberAlreadyRegistered, nil
	}
	return ports.PricePolicyCaliberContentConflict, nil
}

// scannedPricePolicyCaliber 是口径行的列形。可缺的几列用指针：分类只在含税/未税时在场，系数只在
// 销售方向在场，汇率三列全有或全无——三处缺席都是口径说出的真话，与库上 CHECK 同形。
type scannedPricePolicyCaliber struct {
	direction           string
	taxDisposition      string
	taxClassification   *string
	volumetricFactor    *string
	fxQuoteType         *string
	fxAsOfSemantics     *string
	fxAsOfPolicyVersion *string
}

func (row scannedPricePolicyCaliber) sameAs(other scannedPricePolicyCaliber) bool {
	return row.direction == other.direction &&
		row.taxDisposition == other.taxDisposition &&
		sameOptionalString(row.taxClassification, other.taxClassification) &&
		sameOptionalString(row.volumetricFactor, other.volumetricFactor) &&
		sameOptionalString(row.fxQuoteType, other.fxQuoteType) &&
		sameOptionalString(row.fxAsOfSemantics, other.fxAsOfSemantics) &&
		sameOptionalString(row.fxAsOfPolicyVersion, other.fxAsOfPolicyVersion)
}

// pricePolicyCaliberColumns 把三口径摊成列。第二个返回值为假表示这是一份绕开构造门的零值口径
// ——税务三值或方向落在集合外，任何一列都写不出真话。
func pricePolicyCaliberColumns(caliber domain.PricePolicyCaliber) (scannedPricePolicyCaliber, bool) {
	columns := scannedPricePolicyCaliber{
		direction:      caliber.Volumetric().Direction().String(),
		taxDisposition: caliber.Tax().Disposition().String(),
	}
	if columns.direction == "" || columns.taxDisposition == "" {
		return scannedPricePolicyCaliber{}, false
	}
	if classification, ok := caliber.Tax().Classification(); ok {
		value := classification.String()
		columns.taxClassification = &value
	}
	if factor, ok := caliber.Volumetric().Factor(); ok {
		value := factor.String()
		columns.volumetricFactor = &value
	}
	if fx, declared := caliber.Fx(); declared {
		quoteType, semantics, policyVersion := fx.QuoteType().String(), fx.AsOfSemantics().String(), fx.AsOfPolicyVersion().String()
		columns.fxQuoteType, columns.fxAsOfSemantics, columns.fxAsOfPolicyVersion = &quoteType, &semantics, &policyVersion
	}
	return columns, true
}

func sameOptionalString(left, right *string) bool {
	return (left == nil && right == nil) ||
		(left != nil && right != nil && *left == *right)
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// PricePolicyCaliberContents 实现 ports.PricePolicyCaliberView：按已唯一选出的价格规则版本取回它
// 声明的计价口径。
//
// 只读。口径属实例半边，本适配器不提供写口（写口在 CommercialPublications 上随发布同笔），也不在
// 读不到时代拟任何口径——缺口径不是「不含税」也不是「零加点」，该是哪一种只有拥有商业依据的一方
// 能说。
type PricePolicyCaliberContents struct {
	db *bentopg.DB
}

func NewPricePolicyCaliberContents(db *bentopg.DB) (*PricePolicyCaliberContents, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &PricePolicyCaliberContents{db: db}, nil
}

var _ ports.PricePolicyCaliberView = (*PricePolicyCaliberContents)(nil)

// LoadPricePolicyCaliber 取回口径正文。
//
// found=false = 口径未登记（无行）。显式租户与版本必须同一身份，否则 error 且不交内容
// （ADR-0003/0040，租户是身份不是过滤器）。读回的每一行都过三道构造门重建，不按列直接拼结构体
// ——构造门是「什么算合法」的单一权威，绕过它库里一行坏数据就会变成一份合法口径。
func (repository *PricePolicyCaliberContents) LoadPricePolicyCaliber(
	ctx context.Context,
	tenant domain.TenantID,
	policy domain.CommercialVersion,
) (domain.PricePolicyCaliber, bool, error) {
	none := domain.PricePolicyCaliber{}
	if tenant.String() == "" ||
		policy.ObjectID().String() == "" || policy.Version().String() == "" {
		return none, false, fmt.Errorf("load price policy caliber: tenant and policy identity are required")
	}
	if tenant != policy.Tenant() {
		return none, false, fmt.Errorf("load price policy caliber: tenant does not own this policy")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load price policy caliber: %w", err)
	}

	var row scannedPricePolicyCaliber
	err = querier.QueryRow(ctx,
		`SELECT direction, tax_disposition, tax_classification_ref, volumetric_factor_ref,
		        fx_quote_type_ref, fx_as_of_semantics_ref, fx_as_of_policy_version
		   FROM party_commercial.price_policy_caliber
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant.String(),
		uint8(domain.PriceRuleObject),
		policy.ObjectID().String(),
		policy.Version().String(),
	).Scan(&row.direction, &row.taxDisposition, &row.taxClassification, &row.volumetricFactor,
		&row.fxQuoteType, &row.fxAsOfSemantics, &row.fxAsOfPolicyVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load price policy caliber: %w", err)
	}

	caliber, err := pricePolicyCaliberFrom(policy, row)
	if err != nil {
		return none, false, fmt.Errorf("load price policy caliber: %w", err)
	}
	return caliber, true, nil
}

func pricePolicyCaliberFrom(version domain.CommercialVersion, row scannedPricePolicyCaliber) (domain.PricePolicyCaliber, error) {
	disposition, err := taxDispositionFrom(row.taxDisposition)
	if err != nil {
		return domain.PricePolicyCaliber{}, err
	}
	classification := domain.TaxClassificationReference{}
	if row.taxClassification != nil {
		if classification, err = domain.NewTaxClassificationReference(*row.taxClassification); err != nil {
			return domain.PricePolicyCaliber{}, err
		}
	}
	tax, err := domain.NewTaxCaliber(disposition, classification)
	if err != nil {
		return domain.PricePolicyCaliber{}, err
	}

	direction, err := priceDirectionFrom(row.direction)
	if err != nil {
		return domain.PricePolicyCaliber{}, err
	}
	factor := domain.VolumetricFactorReference{}
	if row.volumetricFactor != nil {
		if factor, err = domain.NewVolumetricFactorReference(*row.volumetricFactor); err != nil {
			return domain.PricePolicyCaliber{}, err
		}
	}
	volumetric, err := domain.NewVolumetricCaliber(direction, factor)
	if err != nil {
		return domain.PricePolicyCaliber{}, err
	}

	switch {
	case row.fxQuoteType == nil && row.fxAsOfSemantics == nil && row.fxAsOfPolicyVersion == nil:
		return domain.NewPricePolicyCaliber(version, tax, volumetric)
	case row.fxQuoteType != nil && row.fxAsOfSemantics != nil && row.fxAsOfPolicyVersion != nil:
		quoteType, err := domain.NewFxQuoteTypeReference(*row.fxQuoteType)
		if err != nil {
			return domain.PricePolicyCaliber{}, err
		}
		semantics, err := domain.NewAsOfSemanticsReference(*row.fxAsOfSemantics)
		if err != nil {
			return domain.PricePolicyCaliber{}, err
		}
		policyVersion, err := domain.NewAsOfPolicyVersion(*row.fxAsOfPolicyVersion)
		if err != nil {
			return domain.PricePolicyCaliber{}, err
		}
		fx, err := domain.NewFxCaliber(quoteType, semantics, policyVersion)
		if err != nil {
			return domain.PricePolicyCaliber{}, err
		}
		return domain.NewPricePolicyCaliberWithFx(version, tax, volumetric, fx)
	default:
		// CHECK 保证三列全有或全无，走到这里说明库与领域已经分叉，报错不吸收。
		return domain.PricePolicyCaliber{}, fmt.Errorf("fx caliber row is half declared")
	}
}

func taxDispositionFrom(raw string) (domain.TaxDisposition, error) {
	switch raw {
	case domain.TaxInclusive.String():
		return domain.TaxInclusive, nil
	case domain.TaxExclusive.String():
		return domain.TaxExclusive, nil
	case domain.TaxNotApplicable.String():
		return domain.TaxNotApplicable, nil
	default:
		return domain.TaxDispositionInvalid, fmt.Errorf("unknown tax disposition %q", raw)
	}
}
