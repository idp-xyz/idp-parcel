import { useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['claims-recovery'];

// 三个对象族（客户异常通知、客户索赔项、追偿事项）各有独立生命周期与栏目，
// 分页签呈现而不是折进一张表——CONTEXT 明文客户赔付不等待追偿完成、追偿
// 不等待客户索赔，三者没有共同主状态可以共列。
// 全页无任何金额列：客户赔付、供应商或保险追偿及其调整的最终金额由
// settlement-accounting 形成（UC-SA-007），真实到账由 UC-SA-005 采用。

/**
 * 客户异常通知决定列表行。字段取 visibility-exception CONTEXT.md「客户可见性
 * 与通知」原词。通知决定、消息渠道接受、实际送达和客户确认分别存在，
 * 故各占一列；哪个结果满足通知义务由客户合同和通知策略决定，本表不合并。
 */
export interface CustomerNoticeRow {
  /** 通知标识。 */
  id: string;
  /** 目标客户（货主客户账户；跨客户内部案件按客户分别形成披露内容）。 */
  customer: string;
  /** 通知对象范围。 */
  scope: string;
  /** 内容快照（版本化；更正以追加或明确替代表达，不覆盖原通知）。 */
  contentSnapshot: string;
  /** 披露依据。 */
  disclosureBasis: string;
  /** 适用渠道；门户展示与主动通知是不同结果。 */
  channel: string;
  /** 要求时限。 */
  deadline: string;
  /** 渠道接受。 */
  channelAccepted: string;
  /** 实际送达；失败按策略重试或升级，原尝试保留。 */
  delivered: string;
  /** 客户确认。 */
  customerConfirmed: string;
}

const noticeColumns: ListColumn<CustomerNoticeRow>[] = [
  { id: 'id', header: '通知标识', className: 'font-mono', render: (row) => row.id },
  { id: 'customer', header: '目标客户', render: (row) => row.customer },
  { id: 'scope', header: '对象范围', render: (row) => row.scope },
  { id: 'snapshot', header: '内容快照', render: (row) => row.contentSnapshot },
  { id: 'basis', header: '披露依据', render: (row) => row.disclosureBasis },
  { id: 'channel', header: '适用渠道', render: (row) => row.channel },
  { id: 'deadline', header: '要求时限', render: (row) => row.deadline },
  { id: 'accepted', header: '渠道接受', align: 'center', render: (row) => row.channelAccepted },
  { id: 'delivered', header: '送达', align: 'center', render: (row) => row.delivered },
  { id: 'confirmed', header: '客户确认', align: 'center', render: (row) => row.customerConfirmed },
];

/**
 * 客户索赔项列表行。字段取 visibility-exception CONTEXT.md「证据、客户索赔与
 * 追偿」原词。首次索赔期限、资料补充期限和结论复核期限是三个独立期限，
 * 不能互相代替或重置——三列不合并。本表无金额列是边界不是遗漏。
 */
export interface CustomerClaimRow {
  /** 索赔项标识：每个索赔项独立受理、审核和形成结论。 */
  id: string;
  /** 所属索赔提交批次；批次保留原始提交范围，允许部分成功。 */
  batch: string;
  /** 货主客户账户。 */
  account: string;
  /** 目标包裹或明确服务责任范围。 */
  target: string;
  /** 索赔类型。 */
  claimType: string;
  /** 资格审核结果：收到不表示受理，受理不表示责任成立，也不表示已同意赔付。 */
  eligibility: string;
  /** 责任结论（版本化）：全部成立、部分成立、不成立或当前无法认定；复核形成新版本不覆盖原结论。 */
  liabilityConclusion: string;
  /** 首次索赔期限。 */
  firstClaimDeadline: string;
  /** 资料补充期限；获批延期形成新期限版本，原期限保留。 */
  supplementDeadline: string;
  /** 结论复核期限；起算必须引用合同规定的通知、送达或可获取事实。 */
  reviewDeadline: string;
}

const claimColumns: ListColumn<CustomerClaimRow>[] = [
  { id: 'id', header: '索赔项标识', className: 'font-mono', render: (row) => row.id },
  { id: 'batch', header: '提交批次', className: 'font-mono', render: (row) => row.batch },
  { id: 'account', header: '货主客户账户', render: (row) => row.account },
  { id: 'target', header: '目标包裹 / 服务责任范围', render: (row) => row.target },
  { id: 'type', header: '索赔类型', render: (row) => row.claimType },
  { id: 'eligibility', header: '资格审核', render: (row) => row.eligibility },
  { id: 'liability', header: '责任结论', render: (row) => row.liabilityConclusion },
  { id: 'first-deadline', header: '首次索赔期限', render: (row) => row.firstClaimDeadline },
  { id: 'supplement-deadline', header: '资料补充期限', render: (row) => row.supplementDeadline },
  { id: 'review-deadline', header: '结论复核期限', render: (row) => row.reviewDeadline },
];

/**
 * 追偿事项列表行。字段取 visibility-exception CONTEXT.md「证据、客户索赔与追偿」
 * 原词。追偿在通知或主张条件成立时独立发起，不等待客户索赔或赔付；预先通知
 * 和正式主张是不同动作，不能合并为一个模糊的「已追偿」，故分列。
 */
export interface RecoveryMatterRow {
  /** 追偿事项标识。 */
  id: string;
  /** 责任相对方：供应商、实际承运商、保险或其他外部责任关系，按相对方分别建立。 */
  counterparty: string;
  /** 责任依据：协议或保险条款版本。 */
  basis: string;
  /** 关联异常案件。 */
  caseRef: string;
  /** 责任范围。 */
  scope: string;
  /** 预先通知动作：准备完成不等于已经对外提交。 */
  preNotice: string;
  /** 正式主张动作：提交、送达、确认分别记录，哪个满足期限义务来自适用协议或条款。 */
  formalClaim: string;
  /** 对方响应：已接收、要求补充、审核中、全部接受、部分接受、拒绝或无响应；无响应不自动等于拒绝。 */
  counterpartyResponse: string;
  /** 外部责任结论（版本化）：对方响应是形成或复核依据，不是最终追偿金额或真实到账。 */
  externalConclusion: string;
  /** 适用期限：届满按动作要求的提交、送达或确认结果作失权复核，不因系统时钟到期自动失权。 */
  deadline: string;
}

const recoveryColumns: ListColumn<RecoveryMatterRow>[] = [
  { id: 'id', header: '追偿事项标识', className: 'font-mono', render: (row) => row.id },
  { id: 'counterparty', header: '责任相对方', render: (row) => row.counterparty },
  { id: 'basis', header: '责任依据', render: (row) => row.basis },
  { id: 'case', header: '关联异常案件', className: 'font-mono', render: (row) => row.caseRef },
  { id: 'scope', header: '责任范围', render: (row) => row.scope },
  { id: 'pre-notice', header: '预先通知', align: 'center', render: (row) => row.preNotice },
  { id: 'formal-claim', header: '正式主张', align: 'center', render: (row) => row.formalClaim },
  { id: 'response', header: '对方响应', render: (row) => row.counterpartyResponse },
  { id: 'conclusion', header: '外部责任结论', render: (row) => row.externalConclusion },
  { id: 'deadline', header: '适用期限', render: (row) => row.deadline },
];

const unconfigured = {
  kind: 'unconfigured' as const,
  title: '追踪与异常模块尚未接线',
  description: '查阅读口尚未建立——索赔的受理与审核端点已接线（/claims），但那是提交面；本页是查阅面，其查询端点未建，不发请求、不含未确认参数的默认值。',
  facts: {
    owner: info.owner,
    source: info.source,
    unlock: '通知/索赔项/追偿事项的查询端点建成并经 ADR-0017 准入闸门放行后接线',
  },
};

function CustomerNoticesTable() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<CustomerNoticeRow>
      title="客户异常通知"
      description={`${info.owner}——通知决定、渠道接受、送达与客户确认分别记录；赔付与追偿金额归 settlement-accounting`}
      // 筛选维度（接线时实装进 filters 槽）：适用渠道、渠道接受/送达/客户确认三格
      // 各自的态（分别记录，不合并成一个「已通知」）、要求时限窗口。标识与客户经搜索。
      search={{ value: search, onChange: setSearch, placeholder: '搜索通知标识 / 目标客户' }}
      columns={noticeColumns}
      // 接线前无实例：行数据与总数届时由 visibility-exception 应用端口供给。
      rows={[]}
      rowKey={(row) => row.id}
      pagination={{ page, pageSize, total: 0, onPageChange: setPage, onPageSizeChange: setPageSize }}
      viewState={unconfigured}
    />
  );
}

function CustomerClaimsTable() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<CustomerClaimRow>
      title="客户索赔项"
      description={`${info.owner}——最终赔付金额由 settlement-accounting 管理，本表无金额列是边界不是遗漏`}
      // 筛选维度（接线时实装进 filters 槽）：索赔类型、资格审核结果、责任结论
      //（全部成立/部分成立/不成立/当前无法认定，封闭四格；复核出新版本不覆盖）。
      // 索赔项标识与货主客户账户经搜索。
      search={{ value: search, onChange: setSearch, placeholder: '搜索索赔项标识 / 货主客户账户' }}
      columns={claimColumns}
      // 接线前无实例：行数据与总数届时由 visibility-exception 应用端口供给。
      rows={[]}
      rowKey={(row) => row.id}
      pagination={{ page, pageSize, total: 0, onPageChange: setPage, onPageSizeChange: setPageSize }}
      viewState={unconfigured}
    />
  );
}

function RecoveryMattersTable() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<RecoveryMatterRow>
      title="追偿事项"
      description={`${info.owner}——追偿金额由 settlement-accounting 形成；预先通知与正式主张不合并为「已追偿」`}
      // 筛选维度（接线时实装进 filters 槽）：责任相对方、对方响应（已接收/要求补充/
      // 审核中/全部接受/部分接受/拒绝/无响应，封闭七格；无响应不自动等于拒绝）、
      // 适用期限窗口。追偿事项标识经搜索。
      search={{ value: search, onChange: setSearch, placeholder: '搜索追偿事项标识 / 责任相对方' }}
      columns={recoveryColumns}
      // 接线前无实例：行数据与总数届时由 visibility-exception 应用端口供给。
      rows={[]}
      rowKey={(row) => row.id}
      pagination={{ page, pageSize, total: 0, onPageChange: setPage, onPageSizeChange: setPageSize }}
      viewState={unconfigured}
    />
  );
}

/**
 * 索赔与追偿（visibility-exception）。页签顺序按对象族在客户方向的发生序：
 * 先有披露（客户异常通知），客户才可能提出索赔项；追偿独立于两者发起。
 */
export function ClaimsRecoveryPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="notices" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="notices">客户异常通知</TabsTrigger>
          <TabsTrigger value="claims">客户索赔项</TabsTrigger>
          <TabsTrigger value="recoveries">追偿事项</TabsTrigger>
        </TabsList>
        <TabsContent value="notices" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <CustomerNoticesTable />
        </TabsContent>
        <TabsContent value="claims" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <CustomerClaimsTable />
        </TabsContent>
        <TabsContent value="recoveries" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <RecoveryMattersTable />
        </TabsContent>
      </Tabs>
    </div>
  );
}
