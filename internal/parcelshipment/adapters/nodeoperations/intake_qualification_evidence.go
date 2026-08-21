package nodeoperations

import (
	"context"
	"fmt"
	"strings"
	"time"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ExecutionFactLookup 是本适配器向 node-operations 取执行事实的窄口。真实装配交给
// nodeoperations/adapters/postgres.ExecutionFacts。只取找回一件：理由同本包
// NetworkIntakeCommandHandler——测试替身不必背上写入半边，提供方仓储也不因多一个消费
// 方而扩宽。
type ExecutionFactLookup interface {
	FindByKey(
		ctx context.Context,
		key noports.ExecutionFactKey,
	) (noports.ExecutionFactRecord, bool, error)
}

// QualifyingNodeExecution 说一条已声明的硬资格由哪一件节点执行事实来证：协作事项 + 动作。
//
// 能登进来的只有 CollaborationActionKind 那五个动作（开封、隔离、呈验、清点、观察）——
// 节点说得了「做了什么」，说不了「监管认定了什么」，查验结论与放行仍归 customs-compliance
// （ADR-0063 第五条）。类型上没有第二个位置可以塞进一个结论。
type QualifyingNodeExecution struct {
	Item   nodomain.CollaborationItemReference
	Action nodomain.CollaborationActionKind
}

// NodeExecutionQualificationEvidence 用节点已登记的执行事实回答收寄硬资格，即 ADR-0063
// 第二条留的那句「权威方实现落在日后的消费侧适配器」。
//
// 本口只为节点自己拥有正文的那一段引用作证：authorityPrefix 是它认领的权威段，段外引用
// 一律答未证明。这是本设计最容易破的一处——把「呈验完成」译成「customs-precheck 已证明」
// 就是 ADR-0063 Consequences 点名禁的那件事：为了让判断变绿而拆出一个假关务身份。节点说
// 得了做过什么，说不了监管认定了什么，所以跨段的转写在本类型里没有出口。
//
// 认领哪一段、哪条引用由哪件执行事实证，都是实例半边：前缀与登记表都在装配期给，没有
// 租户就没有资格声明，因此生产上为空。空表答未证明——不默认已证明，也不折成资格目录未
// 配置（那是 PAR-COM-16 声明缺席那一格，恢复动作不同）。
type NodeExecutionQualificationEvidence struct {
	facts           ExecutionFactLookup
	authorityPrefix string
	qualifying      map[string]QualifyingNodeExecution
}

// NewNodeExecutionQualificationEvidence 在装配期就拒掉立不起来的登记项。留到判断时再发现
// 只会答出一个看不出原因的未证明——而未证明与配置写错是两回事。
//
// authorityPrefix 与登记表都由装配点交进来，这里一个字面量都不写死：认领哪个前缀、前缀下
// 挂哪些资格项属实例半边，登记动作留给接线那一票。
func NewNodeExecutionQualificationEvidence(
	facts ExecutionFactLookup,
	authorityPrefix string,
	qualifying map[string]QualifyingNodeExecution,
) (NodeExecutionQualificationEvidence, error) {
	if facts == nil {
		return NodeExecutionQualificationEvidence{},
			fmt.Errorf("parcel shipment nodeoperations adapter: execution fact lookup is nil")
	}
	// 带 `/` 的前缀永远匹配不上 QualificationRulePrefix 取出的那一段，装出来就是个恒答
	// 未证明的死口；空前缀则等于没认领任何权威段。
	if authorityPrefix == "" || strings.ContainsRune(authorityPrefix, '/') {
		return NodeExecutionQualificationEvidence{},
			fmt.Errorf("parcel shipment nodeoperations adapter: authority prefix %q must be a non-empty reference prefix without a slash",
				authorityPrefix)
	}
	registered := make(map[string]QualifyingNodeExecution, len(qualifying))
	for reference, execution := range qualifying {
		// 按消费方自己的构造器校验并归一，登记键与判断时的 rule.String() 才对得上。
		rule, err := psdomain.NewQualificationRuleReference(reference)
		if err != nil {
			return NodeExecutionQualificationEvidence{},
				fmt.Errorf("parcel shipment nodeoperations adapter: qualification reference: %w", err)
		}
		// 段外引用连登记都不许。留到判断时才拦，等于把「节点为别的权威段作证」这条配置
		// 一直留在表里，哪天路由变了它就变成一个假的已证明。
		if psdomain.QualificationRulePrefix(rule) != authorityPrefix {
			return NodeExecutionQualificationEvidence{},
				fmt.Errorf("parcel shipment nodeoperations adapter: qualification reference %q is outside the claimed authority prefix %q",
					rule.String(), authorityPrefix)
		}
		if execution.Item.String() == "" || execution.Action.String() == "" {
			return NodeExecutionQualificationEvidence{},
				fmt.Errorf("parcel shipment nodeoperations adapter: qualifying execution for %q needs both an item and a valid action",
					rule.String())
		}
		registered[rule.String()] = execution
	}
	return NodeExecutionQualificationEvidence{
		facts:           facts,
		authorityPrefix: authorityPrefix,
		qualifying:      registered,
	}, nil
}

// AuthorityPrefix 交回本口认领的权威段，供装配点按它登记进 KnownPrefix 组合口。两处各写
// 一份前缀就会有写岔的那一天，而写岔只表现为恒答未证明——那一格看不出是配置错还是证据
// 没到。
func (view NodeExecutionQualificationEvidence) AuthorityPrefix() string {
	return view.authorityPrefix
}

var _ psports.IntakeQualificationEvidenceView = NodeExecutionQualificationEvidence{}

// ProveIntakeQualification 按（身份 + 来源 + 引用 + 收寄业务时点）取证。三种未证明各有其
// 因：引用未登记、事实未到、事实晚于收寄时点；读不回则上抛（ADR-0029 三格恢复动作不同）。
func (view NodeExecutionQualificationEvidence) ProveIntakeQualification(
	ctx context.Context,
	identity psdomain.SourceIdentity,
	source psdomain.IntakeSource,
	rule psdomain.QualificationRuleReference,
	asOf time.Time,
) (psports.IntakeQualificationProof, error) {
	// 段外引用被路由进来只可能是装配写错。这里仍答未证明而不是已证明：对前缀这一维，
	// ADR-0063 只留了这一个安全答案，装配错不该由它兑出一个跨权威段的证明。
	if psdomain.QualificationRulePrefix(rule) != view.authorityPrefix {
		return psports.IntakeQualificationUnproven, nil
	}
	execution, registered := view.qualifying[rule.String()]
	if !registered {
		return psports.IntakeQualificationUnproven, nil
	}
	// 只有节点收寄这一路的来源对象才是节点的作业实物；场外揽收的对象来自
	// transport-fulfillment，两侧编号偶然同名就会误证成已证明。
	if source.Kind() != psdomain.NodeIntakeSource {
		return psports.IntakeQualificationUnproven, nil
	}
	key, err := view.factKeyFor(identity, source, execution)
	if err != nil {
		return psports.IntakeQualificationProofInvalid, err
	}
	record, found, err := view.facts.FindByKey(ctx, key)
	if err != nil {
		return psports.IntakeQualificationProofInvalid, fmt.Errorf("prove intake qualification: %w", err)
	}
	if !found {
		return psports.IntakeQualificationUnproven, nil
	}
	// 有效性按收寄业务时点判：收寄之后才做的执行证不了收寄当时已满足。asOf 是被采用来源
	// 的实际发生时间，原样用——换成处理时间或时钟，同一份证据重算一遍就会换个答案
	// （ADR-0063 第二条）。
	if record.Fact.PerformedAt().After(asOf) {
		return psports.IntakeQualificationUnproven, nil
	}
	return psports.IntakeQualificationProven, nil
}

// factKeyFor 把消费方话语译成提供方的执行事实幂等键。译不过去不折成未证明：那是编程错误
// 不是业务答案，折进去会让一处配置或身份写错长期伪装成「证据还没到」。
func (view NodeExecutionQualificationEvidence) factKeyFor(
	identity psdomain.SourceIdentity,
	source psdomain.IntakeSource,
	execution QualifyingNodeExecution,
) (noports.ExecutionFactKey, error) {
	tenant, err := nodomain.NewTenantID(identity.TenantID().String())
	if err != nil {
		return noports.ExecutionFactKey{}, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	unit, err := nodomain.NewHandlingUnitID(source.Object().String())
	if err != nil {
		return noports.ExecutionFactKey{}, fmt.Errorf("%w: handling unit: %v", ErrUntranslatableAnswer, err)
	}
	return noports.ExecutionFactKey{
		TenantID: tenant,
		Item:     execution.Item,
		Unit:     unit,
		Action:   execution.Action,
	}, nil
}
