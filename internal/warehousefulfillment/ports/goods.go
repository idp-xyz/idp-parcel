package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// GoodsSaveOutcome 是一笔商品修订在持久化面的落点。它比别的册的落点多两格：商品登记的判重空间是（租户，提交途径，来源，键）而不是修订号，
// 修订号错位答的是基准过期，识别码的唯一要在落册那一笔里判（UC-WF-002「一致性、幂等与并发」）。业务格不用 error 表达，事务因此保持可用。
type GoodsSaveOutcome uint8

const (
	GoodsSaveOutcomeInvalid GoodsSaveOutcome = iota
	// GoodsRevisionSaved 是新写入的一笔。
	GoodsRevisionSaved
	// GoodsRevisionAlreadyRegistered 是同一个键、判重要比的内容相同的重放。
	GoodsRevisionAlreadyRegistered
	// GoodsSourceConflict 是同一个键、内容不同。
	GoodsSourceConflict
	// GoodsBasisStale 是这一笔的修订号不是在册最新加一：别人已先落了同一修订号，或修订号根本接不上。
	GoodsBasisStale
	// GoodsIdentifierConflict 是这一笔占的某个码在同一货主内已被别的商品的当前修订占着，区间有交叠。
	GoodsIdentifierConflict
)

func (outcome GoodsSaveOutcome) String() string {
	switch outcome {
	case GoodsRevisionSaved:
		return "SAVED"
	case GoodsRevisionAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case GoodsSourceConflict:
		return "SOURCE_CONFLICT"
	case GoodsBasisStale:
		return "BASIS_STALE"
	case GoodsIdentifierConflict:
		return "IDENTIFIER_CONFLICT"
	default:
		return ""
	}
}

// GoodsSaveResult 是保存的结果：落点，识别码冲突时另带撞上的那个码。
type GoodsSaveResult struct {
	Outcome         GoodsSaveOutcome
	ConflictingCode domain.IdentifierCode
}

// GoodsRegistry 是商品登记册的写入面：键 = 租户 + 货主 + 商品编码 + 修订号，修订只追加、不覆盖；判重空间另是（租户，提交途径，来源，键）。
//
// 读不动走 error，不折成 found=false，理由同 WarehouseRegistry。登记用例在同一个环境事务里调用读方法与保存；保存在没有环境事务时拒绝。
//
// 保存要在同一个事务里把「判重、修订号接得上、识别码不撞」与落册串成一件不可分的事：识别码的唯一不能只靠应用层先查后写，两个商品并发抢同一个码时
// 至多一个落册，另一个答识别码冲突（UC-WF-002「一致性、幂等与并发」）。读方法给用例按判定次序提前作答，保存里的判定是它们在并发下的定论。
type GoodsRegistry interface {
	// LoadLatestGoods 取在册的最新修订，写入用例据它核基准、不可改的格与组件。
	LoadLatestGoods(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		code domain.GoodsCode,
	) (domain.GoodsRevision, bool, error)
	// LoadGoodsRevisionBySubmission 按判重位置（租户，提交途径，来源，键）取在册的那一笔，可能是任何一个商品的任何一笔：同一个键换了商品，
	// 也是同一个键、内容不同。依据引用不属于位置。
	LoadGoodsRevisionBySubmission(
		ctx context.Context,
		tenant domain.TenantID,
		submission domain.GoodsSubmission,
	) (domain.GoodsRevision, bool, error)
	// FindGoodsIdentifierConflict 在同一货主内比对别的商品的当前修订：这些占用里有没有哪一处与它们碰撞（同一个码、区间有交叠），
	// 有则交回撞上的那个码，按 claims 的次序取先撞上的。自己的商品编码不算「别的商品」；不成立的区间（止不晚于起）不占任何时刻，不参与比对。
	FindGoodsIdentifierConflict(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		code domain.GoodsCode,
		claims []domain.IdentifierClaim,
	) (domain.IdentifierCode, bool, error)
	SaveGoodsRevision(ctx context.Context, revision domain.GoodsRevision) (GoodsSaveResult, error)
}

// GoodsStockLookup 回答「这个商品有没有在库库存」：任何仓、任何一格上余额为正即为有（CONTEXT「主数据登记」：只在该商品没有在库库存时可改是否跟踪批号、效期）。
// 答案要撑到调用方这一笔修订落册，所以须在环境事务里调用：调用返回之后、事务结束之前，答案不变。
//
// 读不动走 error，不折成「没有」：折成没有会让一次该重试的故障放过本该拒的改动。
type GoodsStockLookup interface {
	HasOnHandStock(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		code domain.GoodsCode,
	) (bool, error)
}

// GoodsLevelRow 是读面上的一个包装层级。
type GoodsLevelRow struct {
	Name   string
	Factor int64
}

// GoodsAliasRow 是读面上的一个商品别名；平台或渠道名没带是空串（带了的取值都不是空白），有效区间没给的那一端是 nil。
type GoodsAliasRow struct {
	Code      string
	Type      string
	Platform  string
	ValidFrom *time.Time
	ValidTo   *time.Time
}

type GoodsComponentRow struct {
	Code     string
	Quantity int64
}

// GoodsDeclarationRow 是读面上的商品申报资料；没给的格是空串（给了的取值都不是空白）。数值是去掉多余前导零与尾零的写法。
type GoodsDeclarationRow struct {
	Name          string
	HSCode        string
	OriginCountry string
	UnitPrice     string
	Currency      string
}

// GoodsDimensionsRow 与 GoodsWeightRow 是货主声明的尺寸与重量，数值是去掉多余前导零与尾零的写法，单位是登记时的码、原样。
type GoodsDimensionsRow struct {
	Length string
	Width  string
	Height string
	Unit   string
}

type GoodsWeightRow struct {
	Value string
	Unit  string
}

// GoodsMeasurementRow 是读面上一条按包装层级声明的尺寸重量；没声明的那一样是 nil。
type GoodsMeasurementRow struct {
	Level      string
	Dimensions *GoodsDimensionsRow
	Weight     *GoodsWeightRow
}

// GoodsRow 是商品册读面上的一笔修订，供当前修订、列表、修订史与按码找商品共用。各列表保持登记时交来的先后。
//
// 每笔都带提交途径；代录的另带依据引用（货主口的依据引用是空串）。登记责任方也在这一行上，是否对货主出示由接入侧定：货主的读口不带出那一格
// （UC-WF-002「权限、隔离与审计」）。
type GoodsRow struct {
	OwnerAccount         string
	GoodsCode            string
	Revision             int
	Name                 string
	BaseUnit             string
	PackagingLevels      []GoodsLevelRow
	TrackBatch           bool
	TrackExpiry          bool
	Aliases              []GoodsAliasRow
	Composite            bool
	OutboundMode         string
	Components           []GoodsComponentRow
	Declaration          GoodsDeclarationRow
	DeclaredMeasurements []GoodsMeasurementRow
	SubmissionPath       string
	EvidenceReference    string
	Registrant           string
	RegisteredAt         time.Time
}

// GoodsCatalogueRead 是商品册的列表读口（ADR-0077）：一个货主名下各商品的当前修订，按 GoodsPages 分页（ADR-0144）。租户与货主在签名上、不取自查询；
// limit 非正拒；空册答空页，不答未配置。
type GoodsCatalogueRead interface {
	ListGoods(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[GoodsRow], error)
}

// GoodsCurrentRead 按（货主，商品编码）读一个商品的当前修订，或按（货主，一个码）找商品。别的货主的商品读不到，与没登记同形：找不到、无错。
type GoodsCurrentRead interface {
	GetCurrentGoods(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		code domain.GoodsCode,
	) (GoodsRow, bool, error)
	// FindGoodsByCode 按各商品的当前修订找 code 在 at 这一刻指向的商品：码是它的商品编码，或是它的某个别名且 at 落在别名的有效区间里
	// （起含、止不含，没给的那一端无界）。同一货主内一个码在同一时刻至多指向一个商品，所以至多一行。
	FindGoodsByCode(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		code domain.IdentifierCode,
		at time.Time,
	) (GoodsRow, bool, error)
}

// OwnerAccountLookup 问一个货主客户账户在本租户有没有登记。只有代录问这一口：客户面的货主取自信封，不问。
// 停用了的账户仍然算登记过——查不到指的是本租户没有这个账户。读不动交 error，调用方不得把它答成查不到。
type OwnerAccountLookup interface {
	OwnerAccountRegistered(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID) (bool, error)
}

// GoodsHistoryRead 按一个商品交回册上的全部修订，按修订号升序。不在册、别的货主的同编码商品与别的租户的同形：零行、无错——读口不泄露别处有没有这个商品。
// 一条修订链就是要全部交出，不分页。
type GoodsHistoryRead interface {
	ListGoodsRevisions(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		code domain.GoodsCode,
	) ([]GoodsRow, error)
}
