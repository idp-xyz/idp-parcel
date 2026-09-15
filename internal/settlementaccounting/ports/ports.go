// Package ports 声明 settlement-accounting 自有的语义边界。这些只是接口：它们的
// PostgreSQL 适配器仍阻断在 Bento 持久化闸门之后（ADR-0017），今天唯一的实现是测试替身。
package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// PreAcceptanceControlPolicyView 取商业侧对「这个范围要不要接受前财务控制」的回答。
//
// 本上下文只消费它，绝不自行推导：CONTEXT 把接受前财务控制策略的版本生命周期判给
// `party-commercial`，也明写不适用依据由那边提供。在这里判一次就成了第二处定义。
//
// 依赖调不通要作为错误返回。把它读成「不要求控制」正是 CONTEXT 禁止的「默认信用通过」，
// 一次商业侧故障会因此变成一个看起来通过了的接受前控制。
//
// 三格（ADR-0054）：
//
//   - found=true + policy：商业侧登记过本范围的控制策略，答案在 policy 里（要求带策略正文里
//     本费用范围的控制项、共同通过条件与采用的控制策略版本，外加要保存在结果上的结算方式与
//     结算政策引用——ADR-0122；不要求带商业不适用依据）。
//   - found=false：**未登记**。合同声明、解析键里的控制策略、策略正文、本范围下的控制项，
//     缺任何一样都落这一格。`PAR-COM-15` 是待提供的实例参数，首发没有租户时这是唯一走得到的
//     真实分支。消费方据以停在自己的未决格，不得当成`无控制`——后者是合同已经说过的终局
//     答案，据它可以放行接受判断，而没人说过话时放行就是默认信用通过。
//   - error：调不通，等重试。
//
// 第三格不是可有可无的形状之争：少了它，适配器交回零值加 nil 就会在编排里落成一个带空
// 依据的`无控制`，而两格代数里没有任何地方能把那一格拦下来。
//
// 键形（sa-preacceptance-policy-view/01 裁决）：除作用域外还收一格**商业解析回指**。
// 作用域是资金维（法人/账户/币种），而商业侧的控制声明按客户合同版本键入——两者不同维，
// 少了回指，实现者只能自己从作用域反查合同，那正是「本上下文只消费不自行推导」禁的事，
// 且反查目录会成为账户映射的第二处定义。回指由调用方从同一次解析回显带来（施加路径的
// 作用域本就派生自它），实现凭（租户+回指）取商业侧已固定的闭包再读声明。
//
// 回指是必备入参，不是可选：空回指下没有任何诚实答案——它既不是「未登记」（没人问过，
// 谈不上登记与否）也不是「调不通」。调用方给不出回指时，命令在编排的受理判断处就该停住。
type PreAcceptanceControlPolicyView interface {
	LoadControlPolicy(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
		resolution domain.CommercialResolutionReference,
	) (domain.PreAcceptanceControlPolicy, bool, error)
}

// OperationalBalanceView 取一个结算作用域当前的运营结算余额。按作用域取而不按账户取，
// 是因为责任法人、结算账户与币种三者共同决定一份余额的归属——少一维就可能拿另一个作用域
// 的钱来冻这一笔。
type OperationalBalanceView interface {
	LoadBalance(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
	) (domain.OperationalBalance, error)
}

// FreezeLedgerRepository 按结算作用域取回只增不删的冻结登记册。幂等与请求冲突都由登记册
// 自己判定，所以整册取回而不是按请求逐条查：判定「同一请求身份是否携带了不同内容」需要它
// 原先记下的摘要，逐条查回来的单条冻结带不出那个。
type FreezeLedgerRepository interface {
	LoadForScope(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
	) (*domain.FreezeLedger, error)
	Save(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
		ledger *domain.FreezeLedger,
	) error
}

// CreditStandingView 取一个账期作用域当前的信用状况（额度、已占用暴露、是否逾期）。
// 与运营余额分开取：SET-03 明写预付冻结不与同一客户账期范围共用余额、额度（ADR-0047）。
// 依赖调不通要作为错误返回，读成「有额度」同样是被禁止的默认信用通过。
type CreditStandingView interface {
	LoadCreditStanding(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
	) (domain.CreditStanding, error)
}

// CreditBasisView 取商业侧对「这个账期范围授权多少额度」的回答——闭包里已采用的信用政策版本
// 与它授权的额度（ADR-0127）。
//
// 本上下文只消费它，绝不自行推导：信用政策的版本生命周期与选择归 `party-commercial`，额度由那边
// 按（法人、等级、费用类型、时点）在闭包解析里选出。在这里按作用域猜一个额度、或拿登记状况里
// 的 limit 当政策额度，都是第二处定义。
//
// 三格照 PreAcceptanceControlPolicyView（ADR-0054）：
//
//   - found=true + basis：闭包采用了信用政策，额度与出处在 basis 里。
//   - found=false：**未配置**——闭包没采用信用政策：租户登记的解析键没要求这一项，或该范围没有
//     生效的信用政策正文。消费方停在自己的未决格，不得当成「无限信用」或「零额度」——两者都是
//     CONTEXT 禁止本上下文替商业侧说的话。
//   - error：坏回指 / 闭包读不回 / 闭包不是唯一已解析 / 闭包采用了信用政策却没带额度。都不是
//     「没人登记过」，答成 found=false 会把租户支去补一份其实已经存在的东西。
//
// 键形同 PreAcceptanceControlPolicyView：作用域是资金维，商业侧的额度按闭包键入，回指由调用方从
// 同一次解析回显带来；空回指下没有任何诚实答案，命令在编排的受理判断处就该停住。
type CreditBasisView interface {
	LoadCreditBasis(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
		resolution domain.CommercialResolutionReference,
	) (domain.CreditBasis, bool, error)
}

// CreditRatioBaseView 取比例额度声明的基数在本上下文账本里的当前取值（ADR-0129 决定三）：入账余额读
// 运营余额登记，上一结算周期已确认费用合计读最近一张已发布对账单的费用行——都是本上下文自己写下的事实，
// 不问任何人；基数**是什么**由商业侧的政策正文声明，这里只答**它现在是多少**。
//
// 三格照 PreAcceptanceControlPolicyView（ADR-0054）：
//
//   - found=true + 取值：基数在本作用域有事实。取值可为负（入账余额为负是账户欠款），折不折成额度、
//     折成多少由领域 CreditBasis.LimitOnBase 说，这里只如实交数。
//   - found=false：**尚无事实**——该作用域未登记运营余额，或本账户从未发布过对账单 / 最近一张已作废且
//     尚无替代。消费方停在自己的未决格，不得当成 0（那会让「没登记余额」与「余额为零」同形）也不得当成
//     无限；恢复动作是等事实出现（登记余额、发布首张对账单），不是重试，也不是补配置。
//   - error：读不回，等重试。
//
// 键取控制请求的结算作用域全部四维：作用域不是过滤器而是身份的一部分，少一维就可能拿另一个作用域的钱
// 当分母。基数取当前值、不按 asOf 回溯，与已占用暴露、逾期同一读法（ADR-0127 决定四）。
type CreditRatioBaseView interface {
	LoadCreditRatioBase(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
		base domain.CreditRatioBase,
	) (int64, bool, error)
}

// CreditExposureLedgerRepository 按结算作用域取回只增不删的信用暴露登记册。整册取回的
// 理由与冻结登记册一字不差；它是另一本账，与冻结账本互不借用。
type CreditExposureLedgerRepository interface {
	LoadForScope(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
	) (*domain.CreditExposureLedger, error)
	Save(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
		ledger *domain.CreditExposureLedger,
	) error
}

type Clock interface {
	Now() time.Time
}

// BillReceptionKey 是供应商账单接收的幂等键：相同供应商、账期、主张身份和版本重复
// 到达返回原结果；同一身份内容变化形成版本冲突（UC-SA-004 一致性）。
type BillReceptionKey struct {
	TenantID domain.TenantID
	Claim    domain.BillClaimID
	Version  domain.BillClaimVersion
}

// BillReceptionRecord 是一次账单接收越过提交边界留下的东西：主张本体与逐行匹配。
// 这里刻意没有任何审核应付字段——匹配完成不是应付，审核通过才是（分步），应付由
// 审核步骤依据本记录另行形成。
type BillReceptionRecord struct {
	Key                      BillReceptionKey
	ContentDigest            string
	Claim                    domain.SupplierBillClaim
	Matches                  []domain.BillLineMatch
	AuditAuthorityConfigured bool
	RecordedAt               time.Time
}

type BillSaveOutcome uint8

const (
	BillSaveOutcomeInvalid BillSaveOutcome = iota
	BillSaved
	BillAlreadyRecorded
)

// BillReceptionStore 按幂等键找回并保存账单接收（写入代数同 ADR-0031：并发落败读回
// 赢家，不覆盖）。
type BillReceptionStore interface {
	FindByKey(ctx context.Context, key BillReceptionKey) (BillReceptionRecord, bool, error)
	Save(ctx context.Context, record BillReceptionRecord) (BillSaveOutcome, error)
}

// ExpectedCostView 按版本取回供应商预期成本供逐行匹配引用。found=false 表示该版本
// 不存在——指错版本是提交矛盾，不是等谁。
type ExpectedCostView interface {
	LoadExpectedCost(
		ctx context.Context,
		tenant domain.TenantID,
		version domain.SupplierCostVersionID,
	) (domain.SupplierExpectedCost, bool, error)
}

// BuyEvaluationOutcome 是一份 BUY `PricingEvaluation` 在 SA 眼里的封闭五格：已完成，与提供方
// 的四种非完成结果——待判断（缺事实/缺汇率序列）、不可计价（价卡明确排除）、冲突、未形成
// （技术）。四格逐一保留不合并：它们在 UC-SA-002 里各有自己的结束格（待判断 / 计价明确排除
// 已接收 / 冲突 / 未形成），续办完全不同（补事实 / 不重试 / 等裁决 / 重试），折成一格就答不出
// 该等谁；折成零金额更是把「算不出」写成「不要钱」（AT-SA-175）。
type BuyEvaluationOutcome uint8

const (
	BuyEvaluationOutcomeInvalid BuyEvaluationOutcome = iota
	BuyEvaluationCompleted
	BuyEvaluationPending
	BuyEvaluationUnratable
	BuyEvaluationConflict
	BuyEvaluationNotFormed
)

func (outcome BuyEvaluationOutcome) String() string {
	switch outcome {
	case BuyEvaluationCompleted:
		return "COMPLETED"
	case BuyEvaluationPending:
		return "PENDING"
	case BuyEvaluationUnratable:
		return "UNRATABLE"
	case BuyEvaluationConflict:
		return "CONFLICT"
	case BuyEvaluationNotFormed:
		return "NOT_FORMED"
	default:
		return ""
	}
}

// BuyEvaluationAdoption 是 SA 从一份 BUY `PricingEvaluation` 采用的那几件（UC-SA-002 步 5
// 「保存评价引用、结算采用快照……原币与合同结算币不同时，直接采用评价内的原币金额、汇率
// 序列版本和换算步骤作为换算依据」）。金额已是本上下文的最小币单位；换算步骤只在跨币种
// 且评价内已换算时在场——评价没算过的数这里不补（AT-SA-177 由领域形成门拒）。
//
// 计价基准时点（asOf）不在这里：它由评价固定在评价内，SA 以 Evaluation 引用回指，不复制
// 第二份；发生项、费用项目与供应商协议也不在这里——它们归 TF/PC，随命令进入，与评价一起
// 形成预期成本。RuleVersion 是评价命中的可执行价卡版本（GLOSSARY「价格规则版本」的 PP 半边）。
type BuyEvaluationAdoption struct {
	Evaluation         domain.BuyEvaluationReference
	Outcome            BuyEvaluationOutcome
	RuleVersion        domain.PurchaseRuleVersionReference
	OriginalCurrency   domain.CurrencyCode
	OriginalMinor      int64
	SettlementCurrency domain.CurrencyCode
	SettlementMinor    int64
	Conversion         domain.ConversionStepReference
	// SubjectKind 与 MemberPackages 是票级 / 主单级评价（ADR-0111 Decision 三）交过来的那两件：金额向包裹的归因
	// 归本上下文既有的「成本分摊结果 / 未分摊余额」机制、按版本化分摊规则做，分摊规则是实例参数——这里只把
	// 评价主体的种类与成员清单如实带过来，不摊。逐包裹评价的 MemberPackages 为空。
	SubjectKind    string
	MemberPackages []string
}

// BuyEvaluationView 是 BUY `PricingEvaluation` → SA 的入向缝在本上下文这一侧的形状：按评价
// 引用查一份采用快照。found=false 表示该评价不存在——指错评价是提交矛盾，不是等谁。
//
// 缝的形状（mechanism-executor-triage/06 SA-c 裁定）：触发用提供方已发的
// `parcel-pricing.evaluation.recorded`（指针载荷），内容按引用查回；适配器落在消费侧
// internal/settlementaccounting/adapters/parcelpricing/（ADR-0025），读提供方的评价库翻译——
// 方向/目的不是 BUY·SUPPLIER_COST 拒；四种非完成结果逐格译。金额的精度依据是提供方的取整留痕
// （ADR-0107）：声明了策略的卡，合计已按卡上进位单位取整，进位单位的 scale 就是最小币单位的依据；
// 没声明的卡评价带 AMOUNT_PRECISION_UNDECLARED，适配器拒——**不得自己补一次取整**（label-channel/13
// 已裁没有币种小数位表）。
type BuyEvaluationView interface {
	LoadBuyEvaluation(
		ctx context.Context,
		tenant domain.TenantID,
		evaluation domain.BuyEvaluationReference,
	) (BuyEvaluationAdoption, bool, error)
}

// ExpectedCostSaveOutcome 是一份预期成本版本在持久化面的落点封闭代数（ADR-0031）：同
// （租户+版本）已有行、或同（发生项+费用项目+规则版本）已有首版，都是`已登记`——绝不覆盖，
// 纠错是新版本不是改写。
type ExpectedCostSaveOutcome uint8

const (
	ExpectedCostSaveOutcomeInvalid ExpectedCostSaveOutcome = iota
	ExpectedCostSaved
	ExpectedCostAlreadyRecorded
)

func (outcome ExpectedCostSaveOutcome) String() string {
	switch outcome {
	case ExpectedCostSaved:
		return "SAVED"
	case ExpectedCostAlreadyRecorded:
		return "ALREADY_RECORDED"
	default:
		return ""
	}
}

// ExpectedCostRegistry 是供应商预期成本的登记面（UC-SA-002 步 5 BUY 侧形成、步 8 纠错追加）。
// 与读取面 ExpectedCostView 分两个接口、同一个适配器实现：读方（UC-SA-004 逐行匹配）不该拿到
// 写口。LoadFirstVersion 按幂等三维取首版——「同一发生项、费用项目和规则版本不得重复形成预期
// 成本」撞上时，编排要读回先到的首版作答，而它的版本身份调用方不知道。
type ExpectedCostRegistry interface {
	Save(
		ctx context.Context,
		tenant domain.TenantID,
		cost domain.SupplierExpectedCost,
		recordedAt time.Time,
	) (ExpectedCostSaveOutcome, error)
	LoadFirstVersion(
		ctx context.Context,
		tenant domain.TenantID,
		occurrence domain.ChargeOccurrenceID,
		feeItem domain.FeeItemReference,
		ruleVersion domain.PurchaseRuleVersionReference,
	) (domain.SupplierExpectedCost, bool, error)
}

// SupplierAuditAuthorityView 取该供应商/责任法人范围的审核授权配置。found=false 表示
// 授权未配置——实例半边未提供时审核停在未决，不默认放行也不虚构授权人（UC-SA-004
// 「无授权不得人工接受或拒绝」）。
type SupplierAuditAuthorityView interface {
	LoadSupplierAuditAuthority(
		ctx context.Context,
		tenant domain.TenantID,
		supplier domain.SupplierPartyReference,
		legalEntity domain.LegalEntityReference,
	) (domain.AuditorReference, bool, error)
}

// SupplierPayableAccountView 取该供应商/责任法人/币种的供应商审核应付所归集的结算账户
// （`PAR-SET-01`「首发主伙伴供应商审核应付/费用贷项的结算账户、方向、币种和归集范围」）。
// found=false 表示账户未登记——实例半边未提供时审核停在未决，不默认账户、不从供应商身份
// 推导：CONTEXT「结算账户……不能由客户账户或当前组织临时推导」对供应商侧同样成立。
//
// 与 SupplierAuditAuthorityView 分两个读口：授权答「谁能审」，账户答「审过的应付归哪本账」，
// 两者的未配置态等的东西不同，并成一格之后未决理由就说不出等的是哪一半。
type SupplierPayableAccountView interface {
	LoadSupplierPayableAccount(
		ctx context.Context,
		tenant domain.TenantID,
		supplier domain.SupplierPartyReference,
		legalEntity domain.LegalEntityReference,
		currency domain.CurrencyCode,
	) (domain.SettlementAccountID, bool, error)
}

// AuditedPayableKey 是审核应付的幂等键：同一应付身份只形成一次。
type AuditedPayableKey struct {
	TenantID domain.TenantID
	Payable  domain.PayableID
}

// AuditedPayableRecord 是一次审核越过提交边界留下的东西。这里刻意没有付款、收款或净额
// 字段——审核应付不表示供应商已付款（UC-SA-004 结果契约），贷项也不净入它。
type AuditedPayableRecord struct {
	Key           AuditedPayableKey
	ContentDigest string
	Payable       domain.AuditedPayable
	RecordedAt    time.Time
}

type AuditedPayableSaveOutcome uint8

const (
	AuditedPayableSaveOutcomeInvalid AuditedPayableSaveOutcome = iota
	AuditedPayableSaved
	AuditedPayableAlreadyRecorded
)

// AuditedPayableStore 按幂等键找回并保存审核应付（写入代数同 ADR-0031）。FindByLine 是第二
// 个读法：一行主张只成立一份应付（AT-SA-089 的「其他范围保持原状态」反过来说就是通过的那一
// 格不再第二次通过），撞上同一行的第二份应付时编排要读回先到的那份作答。
type AuditedPayableStore interface {
	FindByKey(ctx context.Context, key AuditedPayableKey) (AuditedPayableRecord, bool, error)
	FindByLine(
		ctx context.Context,
		tenant domain.TenantID,
		claim domain.BillClaimID,
		line domain.BillLineReference,
	) (AuditedPayableRecord, bool, error)
	Save(ctx context.Context, record AuditedPayableRecord) (AuditedPayableSaveOutcome, error)
}

// SupplierCreditNoteKey 是供应商费用贷项的幂等键：同一贷项身份与版本只形成一次。
type SupplierCreditNoteKey struct {
	TenantID domain.TenantID
	Note     domain.CreditNoteID
	Version  domain.CreditNoteVersion
}

// SupplierCreditNoteRecord 是一次贷项形成越过提交边界留下的东西。
type SupplierCreditNoteRecord struct {
	Key           SupplierCreditNoteKey
	ContentDigest string
	Note          domain.SupplierCreditNote
	RecordedAt    time.Time
}

type SupplierCreditNoteSaveOutcome uint8

const (
	SupplierCreditNoteSaveOutcomeInvalid SupplierCreditNoteSaveOutcome = iota
	SupplierCreditNoteSaved
	SupplierCreditNoteAlreadyRecorded
)

// SupplierCreditNoteStore 按幂等键找回并保存供应商费用贷项（写入代数同 ADR-0031）。只追加、
// 无改写口：贷项回指原应付而不改它，贷项自己的更正是新版本。
type SupplierCreditNoteStore interface {
	FindByKey(ctx context.Context, key SupplierCreditNoteKey) (SupplierCreditNoteRecord, bool, error)
	Save(ctx context.Context, record SupplierCreditNoteRecord) (SupplierCreditNoteSaveOutcome, error)
}

// SupplierBillHandoffIntent 把 UC-SA-004 越过提交边界的三种东西之一交给下游：接收记录
// （审核与对账消费）、审核应付引用、供应商费用贷项引用（UC-SA-005 核销与 UC-SA-006 经营口径
// 消费）。三者各携其记录，同一时刻只填一格，消费方自分；意图由各自幂等键认领，重放重发
// 同一份（ADR-0043 同款纪律）。应付与贷项**分别发布**（UC-SA-004 步 7）：一方投递成功不推定
// 另一方已发布（AT-SA-098）。
type SupplierBillHandoffIntent struct {
	Record     BillReceptionRecord
	Payable    AuditedPayableRecord
	CreditNote SupplierCreditNoteRecord
}

// SupplierBillHandoff 把接收记录、审核应付与贷项写入 Outbox（`OutboxSupplierBillHandoff`）。
type SupplierBillHandoff interface {
	HandOffSupplierBill(ctx context.Context, intent SupplierBillHandoffIntent) error
}

// ConfirmationCondition 是费用类型确认条件的核对应答：Met 时携带确认依据（交付确认、
// 里程碑达成……），未满足时携带缺口引用——缺口是续办入口，不是拒绝理由。
type ConfirmationCondition struct {
	Met   bool
	Basis domain.ConfirmationBasisReference
	Gap   string
}

// ConfirmationConditionView 取该费用类型的确认条件核对结果。found=false 表示确认
// 条件目录未配置——实例半边未提供时确认停在未决，不默认转正（UC-SA-002「费用已确认：
// 确认条件已满足」，条件本身是实例参数）。
type ConfirmationConditionView interface {
	LoadConfirmationCondition(
		ctx context.Context,
		tenant domain.TenantID,
		charge domain.CustomerChargeID,
		feeItem domain.FeeItemReference,
	) (ConfirmationCondition, bool, error)
}

// ConfirmedChargeFactsView 取这笔费用在确认时必须固定的七项事实（SA CONTEXT「费用形成
// 与证据」硬句，ADR-0087 决定一）。found=false 表示这些事实无处可取——实例半边未提供时
// 确认停在未决，不用空值凑格。
//
// 与 ConfirmationConditionView 分两个读口而不并成一个：条件核对答的是「这笔费用能不能
// 确认」，本读口答的是「确认下来钉哪些事实」，两者的未配置态等的东西不同（一个等确认
// 条件目录，一个等结算事实册），并成一格之后未决理由就说不出等的是哪一半。
type ConfirmedChargeFactsView interface {
	LoadConfirmedChargeFacts(
		ctx context.Context,
		tenant domain.TenantID,
		charge domain.CustomerChargeID,
	) (domain.ConfirmedChargeFacts, bool, error)
}

type ChargeSaveOutcome uint8

const (
	ChargeSaveOutcomeInvalid ChargeSaveOutcome = iota
	ChargeSaved
	ChargeAlreadyConfirmed
)

// CustomerChargeStore 按标识找回并保存客户费用（写入代数同 ADR-0031：并发二确落败
// 读回赢家，不覆盖）。
type CustomerChargeStore interface {
	FindByID(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.CustomerChargeID,
	) (domain.CustomerCharge, bool, error)
	SaveConfirmed(
		ctx context.Context,
		tenant domain.TenantID,
		charge domain.CustomerCharge,
	) (ChargeSaveOutcome, error)
}

// ChargeConfirmationHandoffIntent 把已确认的费用交给对账单纳入消费（UC-SA-003 只
// 纳已确认费用）。意图由费用标识认领，重放重发同一份（ADR-0043 同款纪律）。
type ChargeConfirmationHandoffIntent struct {
	TenantID domain.TenantID
	Charge   domain.CustomerCharge
}

// ChargeConfirmationHandoff 把已确认费用写入 Outbox（`OutboxChargeConfirmationHandoff`）。
type ChargeConfirmationHandoff interface {
	HandOffChargeConfirmation(ctx context.Context, intent ChargeConfirmationHandoffIntent) error
}

// AdvanceAssessmentKey 是实际代垫评估的幂等键：同一评估标识只登一次。
type AdvanceAssessmentKey struct {
	TenantID   domain.TenantID
	Assessment domain.AdvanceAssessmentID
}

// AdvanceAssessmentRecord 是一次代垫评估越过提交边界留下的东西。
type AdvanceAssessmentRecord struct {
	Key           AdvanceAssessmentKey
	ContentDigest string
	Assessment    domain.ActualAdvanceAssessment
	RecordedAt    time.Time
}

type AdvanceAssessmentSaveOutcome uint8

const (
	AdvanceAssessmentSaveOutcomeInvalid AdvanceAssessmentSaveOutcome = iota
	AdvanceAssessmentSaved
	AdvanceAssessmentAlreadyRecorded
)

// AdvanceAssessmentStore 按幂等键找回并保存代垫评估（写入代数同 ADR-0031）。
type AdvanceAssessmentStore interface {
	FindByKey(ctx context.Context, key AdvanceAssessmentKey) (AdvanceAssessmentRecord, bool, error)
	Save(ctx context.Context, record AdvanceAssessmentRecord) (AdvanceAssessmentSaveOutcome, error)
}

// AdvanceRecoveryKey 是客户代垫回收的幂等键。
type AdvanceRecoveryKey struct {
	TenantID domain.TenantID
	Recovery domain.AdvanceRecoveryID
}

// AdvanceRecoveryRecord 是一次回收形成越过提交边界留下的东西。
type AdvanceRecoveryRecord struct {
	Key           AdvanceRecoveryKey
	ContentDigest string
	Recovery      domain.CustomerAdvanceRecovery
	RecordedAt    time.Time
}

type AdvanceRecoverySaveOutcome uint8

const (
	AdvanceRecoverySaveOutcomeInvalid AdvanceRecoverySaveOutcome = iota
	AdvanceRecoverySaved
	AdvanceRecoveryAlreadyFormed
)

// AdvanceRecoveryStore 按幂等键找回并保存客户代垫回收（写入代数同 ADR-0031）。
type AdvanceRecoveryStore interface {
	FindByKey(ctx context.Context, key AdvanceRecoveryKey) (AdvanceRecoveryRecord, bool, error)
	Save(ctx context.Context, record AdvanceRecoveryRecord) (AdvanceRecoverySaveOutcome, error)
}

// RecoveryAdjustmentKey 是回收调整的幂等键。
type RecoveryAdjustmentKey struct {
	TenantID   domain.TenantID
	Adjustment domain.RecoveryAdjustmentID
}

// RecoveryAdjustmentRecord 是一次回收调整越过提交边界留下的东西。
type RecoveryAdjustmentRecord struct {
	Key           RecoveryAdjustmentKey
	ContentDigest string
	Adjustment    domain.RecoveryAdjustment
	RecordedAt    time.Time
}

type RecoveryAdjustmentSaveOutcome uint8

const (
	RecoveryAdjustmentSaveOutcomeInvalid RecoveryAdjustmentSaveOutcome = iota
	RecoveryAdjustmentSaved
	RecoveryAdjustmentAlreadyFormed
)

// RecoveryAdjustmentStore 按幂等键找回并保存回收调整（写入代数同 ADR-0031）。
type RecoveryAdjustmentStore interface {
	FindByKey(ctx context.Context, key RecoveryAdjustmentKey) (RecoveryAdjustmentRecord, bool, error)
	Save(ctx context.Context, record RecoveryAdjustmentRecord) (RecoveryAdjustmentSaveOutcome, error)
}

// ContractResponsibilityView 取客户对该税费义务的合同责任依据。found=false 表示合同
// 责任目录未配置——回收需要合同依据（实例半边），未配置停在未决，不默认可回收。
type ContractResponsibilityView interface {
	LoadContractResponsibility(
		ctx context.Context,
		tenant domain.TenantID,
		customer domain.RecoveryCustomerReference,
		obligation domain.TaxObligationReference,
	) (domain.ContractResponsibilityReference, bool, error)
}

// AdvanceRecoveryIntent 把已形成的回收/调整交给对账单纳入消费（UC-SA-003 上游）。
// 意图由幂等键认领，重放重发同一份（ADR-0043）。
type AdvanceRecoveryIntent struct {
	Recovery   AdvanceRecoveryRecord
	Adjustment RecoveryAdjustmentRecord
}

// AdvanceRecoveryHandoff 把回收与调整写入 Outbox（`OutboxAdvanceRecoveryHandoff`）。
type AdvanceRecoveryHandoff interface {
	HandOffAdvanceRecovery(ctx context.Context, intent AdvanceRecoveryIntent) error
}

// StatementKey 是已发布对账单的幂等键：同一单号只发布一次；作废以 Replace 换值，
// 替代单用新单号。
type StatementKey struct {
	TenantID domain.TenantID
	Number   domain.StatementNumber
}

// StatementRecord 是一次对账单发布越过提交边界留下的东西。
type StatementRecord struct {
	Key           StatementKey
	ContentDigest string
	Statement     domain.PublishedStatement
	RecordedAt    time.Time
}

type StatementSaveOutcome uint8

const (
	StatementSaveOutcomeInvalid StatementSaveOutcome = iota
	StatementSaved
	StatementAlreadyPublished
)

// PublishedStatementStore 按幂等键找回并保存已发布对账单（写入代数同 ADR-0031）。
type PublishedStatementStore interface {
	FindByKey(ctx context.Context, key StatementKey) (StatementRecord, bool, error)
	Save(ctx context.Context, record StatementRecord) (StatementSaveOutcome, error)
	Replace(ctx context.Context, record StatementRecord) (bool, error)
}

// InclusionKey 是后续账期纳入的幂等键。
type InclusionKey struct {
	TenantID  domain.TenantID
	Inclusion domain.InclusionReference
}

// InclusionRecord 是一次后续账期纳入越过提交边界留下的东西。
type InclusionRecord struct {
	Key           InclusionKey
	ContentDigest string
	Inclusion     domain.SubsequentInclusion
	RecordedAt    time.Time
}

type InclusionSaveOutcome uint8

const (
	InclusionSaveOutcomeInvalid InclusionSaveOutcome = iota
	InclusionSaved
	InclusionAlreadyRecorded
)

// SubsequentInclusionStore 按幂等键找回并保存后续账期纳入（写入代数同 ADR-0031）。
type SubsequentInclusionStore interface {
	FindByKey(ctx context.Context, key InclusionKey) (InclusionRecord, bool, error)
	Save(ctx context.Context, record InclusionRecord) (InclusionSaveOutcome, error)
}

// ChargeAdjustmentKey 是一笔客户费用调整的幂等键。
type ChargeAdjustmentKey struct {
	TenantID   domain.TenantID
	Adjustment domain.ChargeAdjustmentID
}

// ChargeAdjustmentRecord 是一笔调整越过提交边界留下的东西。
type ChargeAdjustmentRecord struct {
	Key        ChargeAdjustmentKey
	Adjustment domain.ChargeAdjustment
	RecordedAt time.Time
}

type ChargeAdjustmentSaveOutcome uint8

const (
	ChargeAdjustmentSaveOutcomeInvalid ChargeAdjustmentSaveOutcome = iota
	ChargeAdjustmentSaved
	ChargeAdjustmentAlreadyRecorded
)

// ChargeAdjustmentStore 是普通客户费用调整的追加式登记册（ADR-0087 决定二）。写入代数
// 同 ADR-0031：同标识重放答`已登记`，不覆盖先到者。
//
// 只追加、无改写口：CONTEXT「重复触发返回原结果，语义或范围不同则形成独立关联调整」
// ——语义不同的那一笔本就该另起一个调整标识，因此这里不需要内容冲突格。
//
// 本册只收 domain.ChargeAdjustment，而它的种类封闭在 UC-SA-002 拥有的计价纠错与商业让利
// 两格；赔付、索赔退款、追偿与供应商贷项各归其唯一创建用例与各自的册，在这里连可表达的
// 取值都没有（CONTEXT「调整类型与唯一所有权」表）。
type ChargeAdjustmentStore interface {
	FindByKey(ctx context.Context, key ChargeAdjustmentKey) (ChargeAdjustmentRecord, bool, error)
	ListByCharge(
		ctx context.Context,
		tenant domain.TenantID,
		charge domain.CustomerChargeID,
	) ([]ChargeAdjustmentRecord, error)
	Save(ctx context.Context, record ChargeAdjustmentRecord) (ChargeAdjustmentSaveOutcome, error)
}

// ChargeAdjustmentView 是调整册的只读半边：按标识取回一笔既有调整，供 UC-SA-003 把它纳入
// 后续账期（步 7「接收发布后的……既有调整，校验其唯一创建用例和有效性，关联原账单并归入
// 后续周期」）。
//
// 它与 ChargeAdjustmentStore 分成两个名字，理由在 CONTEXT「调整类型与唯一所有权」表：
// 后续对账纳入「只拥有纳入关系，不创建任何……借项或贷项调整」，而 internal/architecture 的
// TestOnlyTheOwningUseCaseReachesTheChargeAdjustmentRegister 按名字守着「编排包内只有唯一
// 创建用例够得着 ChargeAdjustmentStore」。截单编排要读调整却不许拿到写口，只能给它一个
// **没有 Save 的类型**——这不是绕那道门，是那道门要的形状：读与写在类型上分开，读方连
// 能写的方法都拿不到。适配器同一个类型两个接口都实现，行模型只有一份。
type ChargeAdjustmentView interface {
	FindByKey(ctx context.Context, key ChargeAdjustmentKey) (ChargeAdjustmentRecord, bool, error)
}

// DisputeKey 是客户异议的幂等键。
type DisputeKey struct {
	TenantID domain.TenantID
	Dispute  domain.DisputeID
}

// DisputeRecord 是一次异议越过提交边界留下的东西。裁定以 Replace 换值——异议是独立
// 对象，不改写对账单。
type DisputeRecord struct {
	Key           DisputeKey
	ContentDigest string
	Dispute       domain.StatementDispute
	RecordedAt    time.Time
}

type DisputeSaveOutcome uint8

const (
	DisputeSaveOutcomeInvalid DisputeSaveOutcome = iota
	DisputeSaved
	DisputeAlreadyOpened
)

// StatementDisputeStore 按幂等键找回并保存异议（写入代数同 ADR-0031）。
type StatementDisputeStore interface {
	FindByKey(ctx context.Context, key DisputeKey) (DisputeRecord, bool, error)
	Save(ctx context.Context, record DisputeRecord) (DisputeSaveOutcome, error)
	Replace(ctx context.Context, record DisputeRecord) (bool, error)
}

// StatementIntent 把对账单事件交给下游（客户通知、外部资金核销的目标索引）。发布、
// 作废与后续纳入各携其记录，消费方自分；重放重发同一份（ADR-0043）。
type StatementIntent struct {
	Statement StatementRecord
	Inclusion InclusionRecord
}

// StatementHandoff 把对账单事件写入 Outbox（`OutboxStatementHandoff`）。
type StatementHandoff interface {
	HandOffStatement(ctx context.Context, intent StatementIntent) error
}

// FundsFactKey 是外部资金事实的**身份**键：事实归银行/支付系统拥有，同一事实引用只有一条身份、
// 首版只采用一次；更正是同一身份下回指前版的新版本（UC-SA-001「更正必须形成新来源版本」），
// 版本在记录的 Fact 上、不在键上——映射与核销引用的也是身份，不是某一版（票 sa-cc/20 裁决 1）。
type FundsFactKey struct {
	TenantID domain.TenantID
	Fact     domain.FundsFactReference
}

// FundsFactRecord 是一次资金事实采用越过提交边界留下的东西：一条记录就是一个版本。
type FundsFactRecord struct {
	Key           FundsFactKey
	ContentDigest string
	Fact          domain.ExternalFundsFact
	RecordedAt    time.Time
}

type FundsFactSaveOutcome uint8

const (
	FundsFactSaveOutcomeInvalid FundsFactSaveOutcome = iota
	FundsFactSaved
	// FundsFactAlreadyAdopted 落在（身份、版本）已在——身份已在而版本是新的，正是更正版本该走进去的口。
	// 库上守链形的唯一约束（一条事实一个首版、一个前版只被更正一次）撞上时也答它：这一版没有落，
	// 占着那个位置的是先到的那一版，编排读回链头作答。同一个「回指的不是当前链头」，顺序到达在 CorrectFact
	// 里答`未受理`，并发到达撞约束才答它、交回赢家那一版——两答不同是有意的：前者是调用方编程错误，后者是
	// 谁先落谁是当前；调用方从交回记录的版本字面 ≠ 命令版本分得出是输掉竞态而非重放。
	FundsFactAlreadyAdopted
)

// ExternalFundsFactStore 按身份键找回并保存资金事实引用（写入代数同 ADR-0031）。
//
// FindByKey 交回**链头**——无后继的那一版，也就是最近采用的那一版：本上下文是铸造方，更正必须回指当前
// 链头（票 sa-cc/20 裁决 2），链因此线性、链头唯一，不需要「谁是当前」的标记。Map / Apply 按它读当前
// 有效版本；FindVersion 按版本字面读某一版，给「同版本重放 / 冲突」的判定与回指核对用。
type ExternalFundsFactStore interface {
	FindByKey(ctx context.Context, key FundsFactKey) (FundsFactRecord, bool, error)
	FindVersion(ctx context.Context, key FundsFactKey, version domain.FundsFactVersion) (FundsFactRecord, bool, error)
	Save(ctx context.Context, record FundsFactRecord) (FundsFactSaveOutcome, error)
}

// AdoptedFundsFactView 按（租户、资金事实引用、采用版本）取回一条已采用事实的只读本体——
// 给下游按 `settlement-accounting.external-funds-fact.adopted` 信封所带的引用回查内容用
// （今天是 `customs-compliance` 的税费付款核对，票 sa-cc/03）。
//
// 键带版本、取信封所指的那一版而不取当前版：信封先后与版本先后不同源，按 latest 读会把后到的
// 更正当成原事实（票 lc/24 的教训）。库里还没有那一版（可见性滞后，或存的是另一版）就诚实答
// found=false，由消费方按自己的续办纪律处置。
//
// 与 ExternalFundsFactStore 分名（形照 parcel-shipment 的 LabelTransactionsByParcelView）：读方拿到
// 事实本体，但不该拿到 Save；写侧登记面的键也不带版本。
type AdoptedFundsFactView interface {
	LoadAdoptedFundsFact(
		ctx context.Context,
		tenant domain.TenantID,
		fact domain.FundsFactReference,
		version domain.FundsFactVersion,
	) (domain.ExternalFundsFact, bool, error)
}

// FundsMappingKey 是资金映射的幂等键。
type FundsMappingKey struct {
	TenantID domain.TenantID
	Mapping  domain.MappingReference
}

// FundsMappingRecord 是一次映射越过提交边界留下的东西。
type FundsMappingRecord struct {
	Key           FundsMappingKey
	ContentDigest string
	Mapping       domain.FundsMapping
	RecordedAt    time.Time
}

type FundsMappingSaveOutcome uint8

const (
	FundsMappingSaveOutcomeInvalid FundsMappingSaveOutcome = iota
	FundsMappingSaved
	FundsMappingAlreadyRecorded
)

// FundsMappingStore 按幂等键找回并保存资金映射（写入代数同 ADR-0031）。
type FundsMappingStore interface {
	FindByKey(ctx context.Context, key FundsMappingKey) (FundsMappingRecord, bool, error)
	Save(ctx context.Context, record FundsMappingRecord) (FundsMappingSaveOutcome, error)
}

// SettlementApplicationKey 是核销的幂等键。撤销以 Replace 换值，撤销关系在本体上。
type SettlementApplicationKey struct {
	TenantID    domain.TenantID
	Application domain.ApplicationReference
}

// SettlementApplicationRecord 是一次核销越过提交边界留下的东西。
type SettlementApplicationRecord struct {
	Key           SettlementApplicationKey
	ContentDigest string
	Application   domain.SettlementApplication
	RecordedAt    time.Time
}

type SettlementApplicationSaveOutcome uint8

const (
	SettlementApplicationSaveOutcomeInvalid SettlementApplicationSaveOutcome = iota
	SettlementApplicationSaved
	SettlementApplicationAlreadyApplied
)

// SettlementApplicationStore 按幂等键找回并保存核销（写入代数同 ADR-0031）。
type SettlementApplicationStore interface {
	FindByKey(ctx context.Context, key SettlementApplicationKey) (SettlementApplicationRecord, bool, error)
	Save(ctx context.Context, record SettlementApplicationRecord) (SettlementApplicationSaveOutcome, error)
	Replace(ctx context.Context, record SettlementApplicationRecord) (bool, error)
}

// SettlementApplicationIntent 把核销/撤销交给下游（对账单与应付的已结视图）。重放
// 重发同一份（ADR-0043）。
type SettlementApplicationIntent struct {
	Record SettlementApplicationRecord
}

// SettlementApplicationHandoff 把核销与撤销写入 Outbox（`OutboxSettlementApplicationHandoff`）。
type SettlementApplicationHandoff interface {
	HandOffSettlementApplication(ctx context.Context, intent SettlementApplicationIntent) error
}

// ExternalFundsFactIntent 把一条已采用的外部资金事实交给下游——今天是 `customs-compliance`
// 的税费付款核对（票 sa-cc/02）。载荷只带引用（租户、事实引用、采用版本），金额与币种由
// 消费方按引用读 SA 读口：事实归银行、支付或财务系统拥有，SA 只采用一次、只发引用，不把
// 金额复制成第二处权威（ADR-0137 决定四）。更正是回指原事实的新版本，同一事件类型再发一
// 封（票 02 裁决 2）。重放重发同一份（ADR-0043）。
type ExternalFundsFactIntent struct {
	Record FundsFactRecord
}

// ExternalFundsFactHandoff 把资金事实采用写入 Outbox（`OutboxExternalFundsFactHandoff`）。
type ExternalFundsFactHandoff interface {
	HandOffExternalFundsFact(ctx context.Context, intent ExternalFundsFactIntent) error
}

// DutyPaymentVerificationAdoptionKey 是结算输入版本里「付款核对」一格的幂等键：租户加提供方核对版本的
// 完整引用（申报范围、税费义务、资金事实、版本指纹）。同一版重放答`已采用`；没有「同键异内容」这一格
// ——核对版本在 customs-compliance 那头不可变，内容变了是新指纹、新版本、新信封，到这里就是另一个键。
type DutyPaymentVerificationAdoptionKey struct {
	TenantID     domain.TenantID
	Verification domain.DutyPaymentVerificationReference
}

// DutyPaymentVerificationAdoptionRecord 是一次采用越过提交边界留下的东西。没有 ContentDigest：键就是
// 全部内容，采用时刻不参与重放比对——同一版第二次到达是重放，不是另一次采用。
type DutyPaymentVerificationAdoptionRecord struct {
	Key      DutyPaymentVerificationAdoptionKey
	Adoption domain.DutyPaymentVerificationAdoption
}

type DutyPaymentVerificationAdoptionSaveOutcome uint8

const (
	DutyPaymentVerificationAdoptionSaveOutcomeInvalid DutyPaymentVerificationAdoptionSaveOutcome = iota
	DutyPaymentVerificationAdoptionSaved
	DutyPaymentVerificationAlreadyAdopted
)

// DutyPaymentVerificationAdoptionStore 按幂等键找回并保存付款核对的采用（写入代数同 ADR-0031）。它是
// 结算输入版本今天唯一落成的一格（票 sa-cc/09）：税费版本、资金事实、付款方、合同责任几格的采用口另有
// 各自的票，不预开。
type DutyPaymentVerificationAdoptionStore interface {
	FindByKey(ctx context.Context, key DutyPaymentVerificationAdoptionKey) (DutyPaymentVerificationAdoptionRecord, bool, error)
	Save(ctx context.Context, record DutyPaymentVerificationAdoptionRecord) (DutyPaymentVerificationAdoptionSaveOutcome, error)
}

// DutyPaymentVerificationView 按引用向 `customs-compliance` 问：信封所指的那一版税费付款核对在不在册
// （票 sa-cc/09 做法 2，口径同票 03 裁决：按键取、走提供方只读口、取信封所指版本不取 latest）。
// 消费侧适配器 `adapters/customscompliance` 实现它；`application` 不 import 提供方。
//
// 只答在不在，不带三态：采用一格只需要确认引用指得到一版真实存在的核对；覆盖 / 差额 / 有效性由
// customs-compliance 持有，实际代垫判断形成时再按同一引用回读，不在采用时转述——转述一次就多一处口径。
// found=false 即那一版在提供方还看不见：可见性滞后是续办不是毒丸，重投会改变结果。
type DutyPaymentVerificationView interface {
	DutyPaymentVerificationExists(
		ctx context.Context,
		tenant domain.TenantID,
		verification domain.DutyPaymentVerificationReference,
	) (bool, error)
}

// AllocationRuleView 取来源费用适用的分摊规则版本。found=false 表示分摊规则目录未
// 配置——无规则不分摊、不默认均摊（实例半边，AT-SA-123）。
type AllocationRuleView interface {
	LoadAllocationRule(
		ctx context.Context,
		tenant domain.TenantID,
		source domain.AllocationSourceReference,
	) (domain.AllocationRuleVersionReference, bool, error)
}

// AllocationKey 是成本分摊的幂等键。重分摊以 Replace 换值，版本链在本体上回指。
type AllocationKey struct {
	TenantID   domain.TenantID
	Allocation domain.AllocationID
}

// AllocationRecord 是一次分摊越过提交边界留下的东西。
type AllocationRecord struct {
	Key           AllocationKey
	ContentDigest string
	Allocation    domain.CostAllocation
	RecordedAt    time.Time
}

type AllocationSaveOutcome uint8

const (
	AllocationSaveOutcomeInvalid AllocationSaveOutcome = iota
	AllocationSaved
	AllocationAlreadyFormed
)

// CostAllocationStore 按幂等键找回并保存成本分摊（写入代数同 ADR-0031）。
type CostAllocationStore interface {
	FindByKey(ctx context.Context, key AllocationKey) (AllocationRecord, bool, error)
	Save(ctx context.Context, record AllocationRecord) (AllocationSaveOutcome, error)
	Replace(ctx context.Context, record AllocationRecord) (bool, error)
}

// OperatingResultKey 是经营结果快照的幂等键：同一（口径+账期+基准）一版一登，重派生
// 以 Replace 换值、新版本关联原截点（AT-SA-137）。
type OperatingResultKey struct {
	TenantID domain.TenantID
	Scope    domain.OperatingScopeReference
	Period   domain.BillingPeriodReference
	Basis    domain.OperatingBasis
}

// OperatingResultRecord 是一次指标派生越过提交边界留下的东西。
type OperatingResultRecord struct {
	Key           OperatingResultKey
	ContentDigest string
	Result        domain.OperatingResult
	RecordedAt    time.Time
}

type OperatingResultSaveOutcome uint8

const (
	OperatingResultSaveOutcomeInvalid OperatingResultSaveOutcome = iota
	OperatingResultSaved
	OperatingResultAlreadyDerived
)

// OperatingResultStore 按幂等键找回并保存经营结果（写入代数同 ADR-0031）。
type OperatingResultStore interface {
	FindByKey(ctx context.Context, key OperatingResultKey) (OperatingResultRecord, bool, error)
	Save(ctx context.Context, record OperatingResultRecord) (OperatingResultSaveOutcome, error)
	Replace(ctx context.Context, record OperatingResultRecord) (bool, error)
}

// OperatingIntent 把分摊与指标交给分析消费。重放重发同一份（ADR-0043）。
type OperatingIntent struct {
	Allocation AllocationRecord
	Result     OperatingResultRecord
}

// OperatingHandoff 把分摊与指标写入 Outbox（`OutboxOperatingHandoff`）。
type OperatingHandoff interface {
	HandOffOperating(ctx context.Context, intent OperatingIntent) error
}

// ClaimAmountRuleView 取责任结论适用的限额/比例/免赔金额规则版本。found=false 表示
// 金额规则目录未配置——没有规则版本不形成金额（实例半边，AT-SA-147）。
type ClaimAmountRuleView interface {
	LoadClaimAmountRule(
		ctx context.Context,
		tenant domain.TenantID,
		responsibility domain.ResponsibilityConclusionReference,
	) (domain.AmountRuleVersionReference, bool, error)
}

// ClaimAmountKey 是客户方向索赔金额的幂等键。
type ClaimAmountKey struct {
	TenantID domain.TenantID
	Amount   domain.CustomerClaimAmountID
}

// ClaimAmountRecord 是一笔赔付/退款金额越过提交边界留下的东西。
type ClaimAmountRecord struct {
	Key           ClaimAmountKey
	ContentDigest string
	Amount        domain.CustomerClaimAmount
	RecordedAt    time.Time
}

type ClaimAmountSaveOutcome uint8

const (
	ClaimAmountSaveOutcomeInvalid ClaimAmountSaveOutcome = iota
	ClaimAmountSaved
	ClaimAmountAlreadyFormed
)

// CustomerClaimAmountStore 按幂等键找回并保存索赔金额（写入代数同 ADR-0031）。
type CustomerClaimAmountStore interface {
	FindByKey(ctx context.Context, key ClaimAmountKey) (ClaimAmountRecord, bool, error)
	Save(ctx context.Context, record ClaimAmountRecord) (ClaimAmountSaveOutcome, error)
}

// ReceivableKey 是应追偿金额的幂等键。
type ReceivableKey struct {
	TenantID   domain.TenantID
	Receivable domain.RecoveryReceivableID
}

// ReceivableRecord 是一笔应追偿越过提交边界留下的东西。
type ReceivableRecord struct {
	Key           ReceivableKey
	ContentDigest string
	Receivable    domain.RecoveryReceivable
	RecordedAt    time.Time
}

type ReceivableSaveOutcome uint8

const (
	ReceivableSaveOutcomeInvalid ReceivableSaveOutcome = iota
	ReceivableSaved
	ReceivableAlreadyFormed
)

// RecoveryReceivableStore 按幂等键找回并保存应追偿（写入代数同 ADR-0031）。
type RecoveryReceivableStore interface {
	FindByKey(ctx context.Context, key ReceivableKey) (ReceivableRecord, bool, error)
	Save(ctx context.Context, record ReceivableRecord) (ReceivableSaveOutcome, error)
}

// AcknowledgementKey 是追偿认可的幂等键。
type AcknowledgementKey struct {
	TenantID        domain.TenantID
	Acknowledgement domain.AcknowledgementID
}

// AcknowledgementRecord 是一次认可越过提交边界留下的东西。
type AcknowledgementRecord struct {
	Key             AcknowledgementKey
	ContentDigest   string
	Acknowledgement domain.RecoveryAcknowledgement
	RecordedAt      time.Time
}

type AcknowledgementSaveOutcome uint8

const (
	AcknowledgementSaveOutcomeInvalid AcknowledgementSaveOutcome = iota
	AcknowledgementSaved
	AcknowledgementAlreadyRecorded
)

// RecoveryAcknowledgementStore 按幂等键找回并保存认可（写入代数同 ADR-0031）。
type RecoveryAcknowledgementStore interface {
	FindByKey(ctx context.Context, key AcknowledgementKey) (AcknowledgementRecord, bool, error)
	Save(ctx context.Context, record AcknowledgementRecord) (AcknowledgementSaveOutcome, error)
}

// ClaimAdjustmentKey 是索赔金额调整的幂等键。
type ClaimAdjustmentKey struct {
	TenantID   domain.TenantID
	Adjustment domain.ClaimAmountAdjustmentID
}

// ClaimAdjustmentRecord 是一次调整越过提交边界留下的东西。
type ClaimAdjustmentRecord struct {
	Key           ClaimAdjustmentKey
	ContentDigest string
	Adjustment    domain.ClaimAmountAdjustment
	RecordedAt    time.Time
}

type ClaimAdjustmentSaveOutcome uint8

const (
	ClaimAdjustmentSaveOutcomeInvalid ClaimAdjustmentSaveOutcome = iota
	ClaimAdjustmentSaved
	ClaimAdjustmentAlreadyFormed
)

// ClaimAmountAdjustmentStore 按幂等键找回并保存调整（写入代数同 ADR-0031）。
type ClaimAmountAdjustmentStore interface {
	FindByKey(ctx context.Context, key ClaimAdjustmentKey) (ClaimAdjustmentRecord, bool, error)
	Save(ctx context.Context, record ClaimAdjustmentRecord) (ClaimAdjustmentSaveOutcome, error)
}

// ClaimSettlementIntent 把赔付金额与调整交给对账单纳入、把应追偿与认可交给追偿链。
// 各携其记录，消费方自分；重放重发同一份（ADR-0043）。
type ClaimSettlementIntent struct {
	ClaimAmount     ClaimAmountRecord
	Receivable      ReceivableRecord
	Acknowledgement AcknowledgementRecord
	Adjustment      ClaimAdjustmentRecord
}

// ClaimSettlementHandoff 把索赔结算各记录写入 Outbox（`OutboxClaimSettlementHandoff`）。
type ClaimSettlementHandoff interface {
	HandOffClaimSettlement(ctx context.Context, intent ClaimSettlementIntent) error
}
