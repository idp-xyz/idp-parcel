import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  listClaimsRecoveryRecords,
  type ApiResult,
  type ClaimItemRecord,
  type CustomerNotificationRecord,
  type NotificationMilestoneRecord,
  type RecoveryActionRecord,
  type RecoveryMatterRecord,
} from './case-api';
import {
  caseLabelOf,
  claimConclusionLabels,
  claimScreenLabels,
  recoveryMilestoneLabels,
} from './case-presentation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['claims-recovery'];

// 三个对象族（客户异常通知、客户索赔项、追偿事项）各有独立生命周期与栏目，
// 分页签呈现而不是折进一张表——CONTEXT 明文客户赔付不等待追偿完成、追偿
// 不等待客户索赔，三者没有共同主状态可以共列。
// 全页无任何金额列：客户赔付、供应商或保险追偿及其调整的最终金额由
// settlement-accounting 形成（UC-SA-007），三本册子的行上连金额键都不存在。
// 三页签各接一册（GET /claims-recovery-records，票 admin-skeleton-closure-batch/06）。

interface ReviewRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

function col(
  id: string,
  header: string,
  options?: { mono?: boolean; align?: 'left' | 'center' | 'right'; className?: string },
): ListColumn<ReviewRow> {
  return {
    id,
    header,
    align: options?.align,
    className: options?.className,
    render: (row) =>
      options?.mono ? (
        <span className="font-mono text-[12px]">{row.values[id] ?? '—'}</span>
      ) : (
        (row.values[id] ?? '—')
      ),
  };
}

/** 通用的按册取数钩子：挂载时取一次，重试换 reloadKey。 */
function useRegistry<Body>(fetchRegistry: () => Promise<ApiResult<Body>>, reloadKey: number) {
  const [answer, setAnswer] = useState<ApiResult<Body> | null>(null);
  useEffect(() => {
    let cancelled = false;
    void fetchRegistry().then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
    // fetchRegistry 是模块级函数的绑定包装，身份稳定；依赖只随 reloadKey 变。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reloadKey]);
  return answer;
}

function filterRows(rows: ReviewRow[], search: string): ReviewRow[] {
  const needle = search.trim().toLowerCase();
  if (!needle) return rows;
  return rows.filter((row) =>
    Object.values(row.values).some((value) => value.toLowerCase().includes(needle)),
  );
}

// ---- 页签一：客户异常通知 ----

/**
 * 通知过程节点按种类取最新登记时刻：渠道接受、送达与客户确认分别记录，本表不把
 * 它们折成一个「已通知」——哪个结果满足通知义务由客户合同与通知策略判断。
 */
function milestoneCell(milestones: NotificationMilestoneRecord[], kind: string): string {
  const nodes = milestones.filter((node) => node.milestone === kind);
  if (nodes.length === 0) return '';
  return formatInstant(nodes[nodes.length - 1].recordedAt);
}

/** 送达格：无 DELIVERED 时如实示最近一次失败（原尝试保留，失败不是终局）。 */
function deliveredCell(milestones: NotificationMilestoneRecord[]): string {
  const delivered = milestoneCell(milestones, 'DELIVERED');
  if (delivered) return delivered;
  const failed = milestoneCell(milestones, 'FAILED');
  return failed ? `失败 · ${failed}` : '';
}

const noticeColumns: ListColumn<ReviewRow>[] = [
  col('notificationId', '通知标识', { mono: true }),
  col('customer', '目标客户', { mono: true }),
  col('episode', '对象范围（发作期）', { mono: true }),
  col('content', '内容快照（引用）', { mono: true }),
  col('policy', '披露依据', { mono: true }),
  col('channel', '适用渠道', { mono: true }),
  col('deadline', '要求时限', { mono: true }),
  col('channelAccepted', '渠道接受', { align: 'center', mono: true }),
  col('delivered', '送达', { align: 'center', mono: true }),
  col('customerConfirmed', '客户确认', { align: 'center', mono: true }),
];

function noticeRowsOf(records: CustomerNotificationRecord[]): ReviewRow[] {
  return records.map((record) => ({
    key: `notice:${record.notificationId}`,
    values: {
      notificationId: record.notificationId,
      customer: record.customer,
      episode: record.episode,
      content: record.content,
      policy: record.policy,
      channel: record.channel,
      deadline: formatInstant(record.deadline),
      channelAccepted: milestoneCell(record.milestones, 'CHANNEL_ACCEPTED'),
      delivered: deliveredCell(record.milestones),
      customerConfirmed: milestoneCell(record.milestones, 'CUSTOMER_CONFIRMED'),
    },
  }));
}

const fetchNotifications = () => listClaimsRecoveryRecords('customer-notification');

function CustomerNoticesTable() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const answer = useRegistry(fetchNotifications, reloadKey);
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const rows = body ? noticeRowsOf(body.notifications) : [];
  const visibleRows = filterRows(rows, search);

  return (
    <ListPageTemplate<ReviewRow>
      title="客户异常通知"
      description={`${info.owner}——通知决定、渠道接受、送达与客户确认分别记录；赔付与追偿金额归 settlement-accounting`}
      search={{ value: search, onChange: setSearch, placeholder: '搜索通知标识 / 目标客户' }}
      filterSummary={body ? `客户异常通知 ${visibleRows.length} 行` : undefined}
      columns={noticeColumns}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, () => setReloadKey((v) => v + 1), {
        module: info,
        endpoint: 'GET /claims-recovery-records?registry=customer-notification',
        emptyTitle: '当前租户尚无客户异常通知',
        emptyDescription:
          '读取入口已配置，登记册为空——通知义务由披露决定形成，事实在接入渠道墙后面，空册是预期不是缺陷。',
      })}
    />
  );
}

// ---- 页签二：客户索赔项 ----

/**
 * 索赔项列面：三判分步（收到、通过资格审核、确认赔偿责任是不同判断），各判的格
 * 分别转写不压并。骨架期的「首次索赔期限」列不在本表：项行上没有该登记格（合同侧
 * 口径，票 06 Comments），列出来只能代填。本表无金额列是边界不是遗漏。
 */
const claimColumns: ListColumn<ReviewRow>[] = [
  col('itemId', '索赔项标识', { mono: true }),
  col('batch', '提交批次', { mono: true }),
  col('customer', '货主客户账户', { mono: true }),
  col('applicant', '代提申请人', { mono: true }),
  col('target', '目标包裹 / 服务责任范围', { mono: true }),
  col('kind', '索赔类型', { mono: true }),
  col('submittedAt', '提交时刻', { mono: true }),
  col('screen', '资格审核'),
  col('conclusion', '责任结论'),
  col('supplementDeadline', '资料补充期限', { mono: true }),
  col('reviewBy', '结论复核期限', { mono: true }),
  col('withdrawn', '客户撤回', { align: 'center' }),
];

function claimRowsOf(records: ClaimItemRecord[]): ReviewRow[] {
  return records.map((record) => ({
    key: `claim:${record.batch}:${record.itemId}`,
    values: {
      itemId: record.itemId,
      batch: record.batch,
      customer: record.customer,
      applicant: record.applicant ?? '',
      target: record.target,
      kind: record.kind,
      submittedAt: formatInstant(record.submittedAt),
      screen: record.screen ? caseLabelOf(claimScreenLabels, record.screen) : '',
      // 复核换版时前版结论照示（原结论保留在前版列，不翻旧插新）。
      conclusion: record.conclusion
        ? `${caseLabelOf(claimConclusionLabels, record.conclusion)}${
            record.priorConclusion
              ? `（复核自 ${caseLabelOf(claimConclusionLabels, record.priorConclusion)}）`
              : ''
          }`
        : '',
      // 获批延期形成新期限版本，原期限保留——版本数把「从未设过」与「延过几次」分开。
      supplementDeadline: record.supplementDeadline
        ? `${formatInstant(record.supplementDeadline)}${
            record.deadlineVersions > 1 ? `（第 ${record.deadlineVersions} 版）` : ''
          }`
        : '',
      reviewBy: record.reviewBy ? formatInstant(record.reviewBy) : '',
      withdrawn: record.withdrawn
        ? `已撤回${record.withdrawnAt ? ` · ${formatInstant(record.withdrawnAt)}` : ''}`
        : '',
    },
  }));
}

const fetchClaims = () => listClaimsRecoveryRecords('claim-item');

function CustomerClaimsTable() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const answer = useRegistry(fetchClaims, reloadKey);
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const rows = body ? claimRowsOf(body.items) : [];
  const visibleRows = filterRows(rows, search);

  return (
    <ListPageTemplate<ReviewRow>
      title="客户索赔项"
      description={`${info.owner}——最终赔付金额由 settlement-accounting 管理，本表无金额列是边界不是遗漏；首次索赔期限在项行上无登记格，不代填`}
      search={{ value: search, onChange: setSearch, placeholder: '搜索索赔项标识 / 货主客户账户' }}
      filterSummary={body ? `客户索赔项 ${visibleRows.length} 行` : undefined}
      columns={claimColumns}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, () => setReloadKey((v) => v + 1), {
        module: info,
        endpoint: 'GET /claims-recovery-records?registry=claim-item',
        emptyTitle: '当前租户尚无客户索赔项',
        emptyDescription:
          '读取入口已配置，登记册为空——索赔项由客户经受理面提交（/claims），事实在接入渠道墙后面，空册是预期不是缺陷。',
      })}
    />
  );
}

// ---- 页签三：追偿事项 ----

/** 动作格：该种类最近一个过程节点（词 · 第 n 次 · 时刻）；缺席即该种类尚无动作。 */
function actionCell(action?: RecoveryActionRecord): string {
  if (!action) return '';
  return `${caseLabelOf(recoveryMilestoneLabels, action.milestone)} · 第 ${action.attempt} 次 · ${formatInstant(action.occurredAt)}`;
}

/**
 * 追偿事项列面：事项要件与两类最新动作节点照行转写。骨架期的「对方响应」与
 * 「外部责任结论」列不在本表：两表在存储上都没有响应/结论登记格（票 06 Comments），
 * 列出来只能代填——「无响应」不自动等于拒绝，这层真话靠缺席保住。
 */
const recoveryColumns: ListColumn<ReviewRow>[] = [
  col('matterId', '追偿事项标识', { mono: true }),
  col('counterparty', '责任相对方', { mono: true }),
  col('basis', '责任依据', { mono: true }),
  col('caseId', '关联异常案件', { mono: true }),
  col('scope', '责任范围', { mono: true }),
  col('legalEntity', '责任法人', { mono: true }),
  col('preliminaryNotice', '预先通知', { mono: true }),
  col('formalAssertion', '正式主张', { mono: true }),
  col('deadline', '适用期限', { mono: true }),
  col('openedAt', '发起时刻', { mono: true }),
];

function recoveryRowsOf(records: RecoveryMatterRecord[]): ReviewRow[] {
  return records.map((record) => ({
    key: `recovery:${record.matterId}`,
    values: {
      matterId: record.matterId,
      counterparty: record.counterparty,
      basis: record.basis,
      caseId: record.caseId,
      scope: record.scope,
      legalEntity: record.legalEntity,
      // 预先通知与正式主张是不同动作，不能合并为一个模糊的「已追偿」——两格各示
      // 该种类最近节点，准备完成不等于已经对外提交（节点词如实转写）。
      preliminaryNotice: actionCell(record.preliminaryNotice),
      formalAssertion: actionCell(record.formalAssertion),
      deadline: formatInstant(record.deadline),
      openedAt: formatInstant(record.openedAt),
    },
  }));
}

const fetchRecoveries = () => listClaimsRecoveryRecords('recovery-matter');

function RecoveryMattersTable() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const answer = useRegistry(fetchRecoveries, reloadKey);
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const rows = body ? recoveryRowsOf(body.matters) : [];
  const visibleRows = filterRows(rows, search);

  return (
    <ListPageTemplate<ReviewRow>
      title="追偿事项"
      description={`${info.owner}——追偿金额由 settlement-accounting 形成；预先通知与正式主张不合并为「已追偿」；对方响应与外部结论在存储上无登记格，不代填`}
      search={{ value: search, onChange: setSearch, placeholder: '搜索追偿事项标识 / 责任相对方' }}
      filterSummary={body ? `追偿事项 ${visibleRows.length} 行` : undefined}
      columns={recoveryColumns}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, () => setReloadKey((v) => v + 1), {
        module: info,
        endpoint: 'GET /claims-recovery-records?registry=recovery-matter',
        emptyTitle: '当前租户尚无追偿事项',
        emptyDescription:
          '读取入口已配置，登记册为空——追偿在通知或主张条件成立时由内部编排发起，事实在接入渠道墙后面，空册是预期不是缺陷。',
      })}
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
