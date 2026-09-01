package accessidentity

// SourceEnvelope 是铸造出来的来源信封四要素。
//
// 字段不导出、包外无构造函数：**拿到一个 SourceEnvelope 就等于它经过了铸造**，没有第二
// 条产生途径。这是把「四要素无一取自请求体或自报头部」做进结构，而不是留给每个 Intake
// 实现自己守——两种实现在调用点长得一模一样，靠读代码分不出来。
//
// 四要素是四个字符串，不是各上下文的领域类型：本包是技术能力，不拥有业务领域语言
// （ADR-0072 一）。消费方拿去构造自己的 SourceIdentity，校验按各自领域的不变式来。
type SourceEnvelope struct {
	tenantID          string
	customerAccountID string
	source            string
	requestKey        string
}

func (envelope SourceEnvelope) TenantID() string          { return envelope.tenantID }
func (envelope SourceEnvelope) CustomerAccountID() string { return envelope.customerAccountID }
func (envelope SourceEnvelope) Source() string            { return envelope.source }
func (envelope SourceEnvelope) RequestKey() string        { return envelope.requestKey }

// SubmissionEnvelope 与 WithdrawalEnvelope 是两个类型而不是同一个类型的两个值。
//
// 票 01 的 S2 验收点要求「提交与撤回各自铸信封，不合用」。做成两个类型之后，把撤回的
// 信封递给收提交的那个口**编译不过**；做成同一个类型加一个动作字段，则要靠调用方每次
// 都传对，而传错与传对在调用点长得一样。这两条哪条守得住不看写法看漏的时候倒向哪边。
type SubmissionEnvelope struct{ envelope SourceEnvelope }

func (submission SubmissionEnvelope) Envelope() SourceEnvelope { return submission.envelope }

type WithdrawalEnvelope struct{ envelope SourceEnvelope }

func (withdrawal WithdrawalEnvelope) Envelope() SourceEnvelope { return withdrawal.envelope }
