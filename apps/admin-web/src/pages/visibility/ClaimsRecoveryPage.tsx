import { useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { labelFor } from '../../vocabulary/status';
import { objectNouns } from '../../vocabulary/objects';
import { formatInstant } from '../moment';
import { matchesSearch } from '../party/list-order';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { pastFirstPage } from '../shipment-request/cursor-pager';
import { TwoLine, technicalRows } from '../settlement/ops-ui';
import { namedRef, usePartyNames, type PartyNames } from '../settlement/party-names';
import {
  listClaimsRecoveryRecords,
  type ClaimItemRecord,
  type ClaimsRecoveryConditions,
  type CustomerNotificationRecord,
  type RecoveryActionRecord,
  type RecoveryMatterRecord,
} from './case-api';
import { notificationScope, recoveryMilestoneScope } from './case-presentation';
import { absenceText, caseNumberName, claimNumberName } from './document-number';
import { DocumentNumberCell } from './DocumentNumberCell';
import { RowStatusPill } from './ExceptionDetailPanel';
import { claimStatus, countByStatus, notificationStatus, type RowStatus } from './exception-list';
import { exceptionViewState } from './exception-view-state';
import { claimsRegistryConditions } from './list-conditions';
import { PageOnlySearch } from './page-only';

// 页面标题与主责出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['claims-recovery'];

// 索赔与追偿（visibility-exception），三签各接 GET /claims-recovery-records 的一本册。改版后列只放运营要看的：索赔号与案件号
// （单据号，ADR-0355 决定十一）、包裹号（索赔的目标包裹）、客户、类型、状态胶囊、时间与期限；客户自己的索赔项编号与批次号、各种内部标识、
// 合同、依据引用进每行末尾的「技术详情」。没有单据号的行照旧显标识，技术详情说为什么没有号。
//
// 三个对象各有独立的生命周期，分签而不折进一张表：客户赔付不等追偿完成，追偿也不等客户索赔。全页没有金额列——赔付与
// 追偿的金额在账务区形成，三本册的行上连金额都没有。首次索赔期限、对方响应与外部责任结论在行上没有登记格，不上列、不代填；
// 「对方没响应」不等于拒绝，这层真话靠不上列保住。
//
// 三本册已迁到 ADR-0144，各按游标翻页（模板的翻页条）。检索框下推成 `q`，读口只认本册的编号与编码列；往来单位名称在
// party-commercial、码的中文词在用语词典，都不在读口的列里——按它们找留一个只筛本页的小框，字面写明。三签今天没有筛选条，
// 读口另有的筛选维（索赔的 screen、conclusion）不为此新加控件。过滤条右端按状态的计数数的是这一页，字面写「本页」。

const tabContent = 'flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden';

/** 「本页 n 项 · 待审核 2 · 符合条件 1」：按状态词的计数接在本页条数后面；整册几条由翻页条说。 */
function countSummary<Row>(unit: string, rows: readonly Row[], statusOf?: (row: Row) => RowStatus): string {
  const parts = statusOf ? countByStatus(rows, statusOf).map((item) => `${item.label} ${item.count}`) : [];
  return [`本页 ${rows.length} ${unit}`, ...parts].join(' · ');
}

/** 只筛本页的小框把行筛空了，与读口答零行是两句话。 */
function emptyRowsNoteOf(subject: string, pageRows: number): string {
  return pageRows > 0 ? `这一页上没有匹配的${subject}；只筛本页的条件不看其余几页，翻页看看` : `没有匹配的${subject}`;
}

function customerCell(names: PartyNames, account: string) {
  const ref = namedRef(names.account(account), account);
  return <TwoLine primary={ref.primary} secondary={ref.secondary} />;
}

function technicalColumn<Row>(rowsOf: (row: Row) => { label: string; value: string | undefined }[]): ListColumn<Row> {
  return {
    id: 'technical',
    header: '',
    className: 'align-top',
    render: (row) => <TechnicalDetails rows={technicalRows(rowsOf(row))} className="max-w-md open:min-w-[18rem]" />,
  };
}

// —— 客户索赔签 ——

function claimDeadline(item: ClaimItemRecord): string {
  // 补资料期限获批延期会形成新版本、原期限保留；这里只显当前那一版，版本数进技术详情。
  if (item.supplementDeadline) return `补资料截止 ${formatInstant(item.supplementDeadline)}`;
  if (item.reviewBy) return `可申请复核至 ${formatInstant(item.reviewBy)}`;
  return '—';
}

function claimColumns(names: PartyNames): ListColumn<ClaimItemRecord>[] {
  return [
    {
      id: 'number',
      header: claimNumberName,
      // 没有索赔号的项照旧显客户自己的索赔项编号：索赔项没有另一个内部标识，（批次，项）就是它在册上的键。
      render: (row) => <DocumentNumberCell documentNumber={row.documentNumber} internal={row.itemId} absence={row.documentNumberAbsence} />,
    },
    {
      id: 'target',
      header: objectNouns.parcelNumber,
      render: (row) => <span className="font-mono type-body leading-4 text-idpxyz-textBright">{row.target}</span>,
    },
    { id: 'customer', header: objectNouns.customer, className: 'whitespace-nowrap', render: (row) => customerCell(names, row.customer) },
    { id: 'kind', header: '索赔类型', className: 'whitespace-nowrap', render: (row) => labelFor(row.kind, 'claim-kind') },
    { id: 'status', header: objectNouns.status, render: (row) => <RowStatusPill status={claimStatus(row)} /> },
    { id: 'submitted', header: '提交时间', className: 'whitespace-nowrap font-mono type-body leading-4', render: (row) => formatInstant(row.submittedAt) },
    { id: 'deadline', header: '期限', className: 'whitespace-nowrap type-body leading-4', render: claimDeadline },
    technicalColumn((row) => [
      { label: claimNumberName, value: row.documentNumber ?? absenceText(row.documentNumberAbsence) },
      { label: '客户的索赔项编号', value: row.itemId },
      { label: '客户的提交批次', value: row.batch },
      { label: '客户编码', value: row.customer },
      { label: '代提申请人', value: row.applicant },
      { label: '客户合同', value: row.contract },
      { label: '索赔类型码', value: row.kind },
      { label: '第几次更新', value: row.revision > 1 ? `第 ${row.revision} 次` : undefined },
      { label: '资格审核', value: row.screen ? `${labelFor(row.screen)}（${row.screen}）` : undefined },
      { label: '审核依据', value: row.screenBasis },
      { label: '缺少的材料', value: row.missingMaterials },
      { label: '补充范围', value: row.supplementScope },
      { label: '补资料通知', value: row.supplementNotice },
      { label: '期限版本数', value: row.deadlineVersions > 1 ? String(row.deadlineVersions) : undefined },
      { label: '责任结论', value: row.conclusion ? `${labelFor(row.conclusion)}（${row.conclusion}）` : undefined },
      { label: '结论时间', value: row.concludedAt ? formatInstant(row.concludedAt) : undefined },
      { label: '复核前结论', value: row.priorConclusion ? labelFor(row.priorConclusion) : undefined },
      { label: '撤回时间', value: row.withdrawnAt ? formatInstant(row.withdrawnAt) : undefined },
    ]),
  ];
}

const loadClaims = (conditions: ClaimsRecoveryConditions<'claim-item'>, after: string | null) =>
  listClaimsRecoveryRecords(conditions, after);

function CustomerClaimsTable({ names }: { names: PartyNames }) {
  const [search, setSearch] = useState('');
  const [refine, setRefine] = useState('');
  const conditions = claimsRegistryConditions('claim-item', useSettledKeyword(search));
  const { answer, pagination, retry } = useCatalogueCursor(conditions, loadClaims);
  const rows = answer?.kind === 'outcome' ? answer.body.items : [];
  const visible = rows.filter((row) =>
    matchesSearch(refine, [names.account(row.customer), labelFor(row.kind, 'claim-kind'), claimStatus(row).label]),
  );
  return (
    <ListPageTemplate<ClaimItemRecord>
      title="客户索赔"
      description="客户就丢失、破损等提出的索赔：资格审核、责任认定各走到哪一步。赔付金额在账务区。"
      search={{ value: search, onChange: setSearch, placeholder: '搜索赔号、包裹号、客户编码、批次…' }}
      filters={<PageOnlySearch value={refine} onChange={setRefine} subject="客户名称、索赔类型、状态" />}
      filterSummary={answer?.kind === 'outcome' ? countSummary('项', rows, claimStatus) : undefined}
      columns={claimColumns(names)}
      rows={visible}
      rowKey={(row) => `${row.batch}:${row.itemId}`}
      emptyRowsNote={emptyRowsNoteOf('索赔', rows.length)}
      pagination={pagination}
      viewState={exceptionViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /claims-recovery-records?registry=claim-item',
        subject: '客户索赔',
        emptyTitle: '还没有客户索赔',
        emptyDescription: '客户通过客户系统提交索赔后会出现在这里；哪些合同能索赔、谁能代客户提，在「索赔资格与授权」里设定。',
        narrowed: catalogueConditionsNarrow(conditions) || pastFirstPage(pagination),
      })}
    />
  );
}

// —— 客户通知签 ——

function notificationColumns(names: PartyNames): ListColumn<CustomerNotificationRecord>[] {
  return [
    { id: 'customer', header: objectNouns.customer, className: 'whitespace-nowrap', render: (row) => customerCell(names, row.customer) },
    { id: 'status', header: '通知进度', render: (row) => <RowStatusPill status={notificationStatus(row)} /> },
    { id: 'channel', header: '通知渠道', className: 'font-mono type-body leading-4', render: (row) => row.channel },
    { id: 'deadline', header: '要求在此前通知', className: 'whitespace-nowrap font-mono type-body leading-4', render: (row) => formatInstant(row.deadline) },
    { id: 'decided', header: '决定通知时间', className: 'whitespace-nowrap font-mono type-body leading-4', render: (row) => formatInstant(row.decidedAt) },
    technicalColumn((row) => [
      { label: '通知编号', value: row.notificationId },
      { label: '客户编码', value: row.customer },
      { label: '异常信号', value: row.episode },
      { label: '通知内容', value: row.content },
      { label: '披露策略', value: row.policy },
      { label: '通知义务', value: row.obligation },
      ...row.milestones.map((node) => ({
        label: labelFor(node.milestone, notificationScope),
        value: `${formatInstant(node.recordedAt)}（${node.milestone}）`,
      })),
    ]),
  ];
}

const loadNotifications = (conditions: ClaimsRecoveryConditions<'customer-notification'>, after: string | null) =>
  listClaimsRecoveryRecords(conditions, after);

function CustomerNoticesTable({ names }: { names: PartyNames }) {
  const [search, setSearch] = useState('');
  const [refine, setRefine] = useState('');
  const conditions = claimsRegistryConditions('customer-notification', useSettledKeyword(search));
  const { answer, pagination, retry } = useCatalogueCursor(conditions, loadNotifications);
  const rows = answer?.kind === 'outcome' ? answer.body.notifications : [];
  const visible = rows.filter((row) => matchesSearch(refine, [names.account(row.customer), notificationStatus(row).label]));
  return (
    <ListPageTemplate<CustomerNotificationRecord>
      title="客户通知"
      // 进度只示最后一步走到哪；渠道接受、送达、客户确认在技术详情里逐条可查，哪一步算「已通知」由客户合同与通知策略定。
      description="出了异常需要告诉客户时的通知：发出、送达、客户确认各走到哪一步，要在什么时间前通知到。"
      search={{ value: search, onChange: setSearch, placeholder: '搜客户编码、通知渠道、通知编号…' }}
      filters={<PageOnlySearch value={refine} onChange={setRefine} subject="客户名称、通知进度" />}
      filterSummary={answer?.kind === 'outcome' ? countSummary('条', rows, notificationStatus) : undefined}
      columns={notificationColumns(names)}
      rows={visible}
      rowKey={(row) => row.notificationId}
      emptyRowsNote={emptyRowsNoteOf('通知', rows.length)}
      pagination={pagination}
      viewState={exceptionViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /claims-recovery-records?registry=customer-notification',
        subject: '客户通知',
        emptyTitle: '还没有客户通知',
        emptyDescription: '异常按「设置 · 通知与披露」的规则决定要告诉客户时，通知会出现在这里。',
        narrowed: catalogueConditionsNarrow(conditions) || pastFirstPage(pagination),
      })}
    />
  );
}

// —— 追偿签 ——

/** 一类追偿动作走到哪一步：节点胶囊，重试过的另注第几次；还没有动作如实说「还没有」。 */
function actionCell(action: RecoveryActionRecord | undefined) {
  if (!action) return <span className="type-body leading-4 text-idpxyz-breadcrumb">还没有</span>;
  return (
    <span className="inline-flex items-center gap-1.5 whitespace-nowrap">
      <StatusPill code={action.milestone} scope={recoveryMilestoneScope(action.milestone)} />
      <span className="font-mono type-caption text-idpxyz-textMuted">
        {formatInstant(action.occurredAt)}
        {action.attempt > 1 ? ` · 第 ${action.attempt} 次` : ''}
      </span>
    </span>
  );
}

function recoveryColumns(names: PartyNames): ListColumn<RecoveryMatterRecord>[] {
  return [
    {
      id: 'case',
      header: caseNumberName,
      // 追偿册只带关联案件的号、不带案件的无号原因，没号时只说没有号，原因到异常案件页看。
      render: (row) => <DocumentNumberCell documentNumber={row.caseDocumentNumber} internal={row.caseId} />,
    },
    {
      id: 'counterparty',
      header: '向谁追偿',
      className: 'whitespace-nowrap',
      render: (row) => {
        const ref = namedRef(names.party(row.counterparty), row.counterparty);
        return <TwoLine primary={ref.primary} secondary={ref.secondary} />;
      },
    },
    // 预先通知与正式主张是两类动作，各示各的最近一步，不合并成一个「已追偿」。
    { id: 'notice', header: '预先通知', render: (row) => actionCell(row.preliminaryNotice) },
    { id: 'assertion', header: '正式主张', render: (row) => actionCell(row.formalAssertion) },
    { id: 'deadline', header: '追偿期限', className: 'whitespace-nowrap font-mono type-body leading-4', render: (row) => formatInstant(row.deadline) },
    { id: 'opened', header: '发起时间', className: 'whitespace-nowrap font-mono type-body leading-4', render: (row) => formatInstant(row.openedAt) },
    technicalColumn((row) => [
      { label: '追偿编号', value: row.matterId },
      { label: '案件内部标识', value: row.caseId },
      { label: '责任方编码', value: row.counterparty },
      { label: '责任范围', value: row.scope },
      { label: '责任依据', value: row.basis },
      { label: objectNouns.companyEntity, value: row.legalEntity },
      { label: '证据', value: row.evidence },
    ]),
  ];
}

const loadRecoveries = (conditions: ClaimsRecoveryConditions<'recovery-matter'>, after: string | null) =>
  listClaimsRecoveryRecords(conditions, after);

function RecoveryMattersTable({ names }: { names: PartyNames }) {
  const [search, setSearch] = useState('');
  const [refine, setRefine] = useState('');
  const conditions = claimsRegistryConditions('recovery-matter', useSettledKeyword(search));
  const { answer, pagination, retry } = useCatalogueCursor(conditions, loadRecoveries);
  const rows = answer?.kind === 'outcome' ? answer.body.matters : [];
  const visible = rows.filter((row) => matchesSearch(refine, [names.party(row.counterparty)]));
  return (
    <ListPageTemplate<RecoveryMatterRecord>
      title="追偿"
      description="异常的责任在承运商、代理或保险时，向他们追回损失：预先通知与正式主张各走到哪一步。追偿金额在账务区。"
      search={{ value: search, onChange: setSearch, placeholder: '搜案件号、责任方编码、追偿编号…' }}
      filters={<PageOnlySearch value={refine} onChange={setRefine} subject="责任方名称" />}
      filterSummary={answer?.kind === 'outcome' ? countSummary('项', rows) : undefined}
      columns={recoveryColumns(names)}
      rows={visible}
      rowKey={(row) => row.matterId}
      emptyRowsNote={emptyRowsNoteOf('追偿', rows.length)}
      pagination={pagination}
      viewState={exceptionViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /claims-recovery-records?registry=recovery-matter',
        subject: '追偿',
        emptyTitle: '还没有追偿',
        emptyDescription: '异常案件查明责任在合作伙伴一方、需要向对方追回时，追偿会出现在这里。',
        narrowed: catalogueConditionsNarrow(conditions) || pastFirstPage(pagination),
      })}
    />
  );
}

/**
 * 索赔与追偿。签的顺序按客户方向的发生先后：先有客户索赔这件运营最常处理的事，再是异常通知，追偿独立发起排最后。
 * 客户名与责任方名从往来单位册转写，读不到时显编码。
 */
export function ClaimsRecoveryPage() {
  const names = usePartyNames();
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="claims" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="claims">客户索赔</TabsTrigger>
          <TabsTrigger value="notices">客户通知</TabsTrigger>
          <TabsTrigger value="recoveries">追偿</TabsTrigger>
        </TabsList>
        <TabsContent value="claims" className={tabContent}>
          <CustomerClaimsTable names={names} />
        </TabsContent>
        <TabsContent value="notices" className={tabContent}>
          <CustomerNoticesTable names={names} />
        </TabsContent>
        <TabsContent value="recoveries" className={tabContent}>
          <RecoveryMattersTable names={names} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
