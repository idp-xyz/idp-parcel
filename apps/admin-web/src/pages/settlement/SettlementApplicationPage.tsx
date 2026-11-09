import { useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { MultiRegistrationPanel, type RegistrationTarget } from '../../components/registration';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { labelFor } from '../../vocabulary/status';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { matchesSearch } from '../party/list-order';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { pastFirstPage } from '../shipment-request/cursor-pager';
import { PageOnlySearch } from '../visibility/page-only';
import {
  fundsRegistrationEndpoints,
  fundsRegistrationOutcomeLabels,
  listSettlementFundsApplications,
  registerExternalFundsFact,
  type ExternalFundsFactRecord,
  type FundsMappingRecord,
  type FundsRegistrationKind,
} from './api';
import { pageEmptyNote } from './list-conditions';
import { fundsRegistrationSnapshotHints, fundsRegistrationTitles, problemNote, scopePostingText } from './presentation';
import { formatMoney, technicalRows } from './ops-ui';

// 收付款核销页（主责上下文 settlement-accounting，UC-SA-005 映射外部收付款并形成运营核销）。接 GET /settlement-funds-applications，
// 资金事实册只有一本、不设 registry 分派。
//
// 资金事实册已迁到 ADR-0144，按游标翻页（模板的翻页条）。检索框下推成 `q`，读口覆盖流水号与来源系统；币种与核销目标不在
// 读口检索的列里，按它们找留一个只筛本页的小框，字面写明。读口另有的 kind 一维与发生时刻排序，页面今天没有对应的筛选条与
// 表头排序，不为此新加控件。条数只数这一页，字面写「本页」。
const info = moduleInfoById['settlement-application'];

// 列只留运营对账要看的：流水号、来自哪个系统、种类、金额、核了多少、核到哪里、发生时间。版本、更正回指、登记人、入账余额
// 的记法进技术详情。三个金额各算各的和而不是一个净额（「从未核销过」与「核销过又撤销了」在净额上长着同一张脸），列上
// 只给已核与未核两个，已撤销金额进技术详情。
//
// 没有「人工分配」按钮：核销的在线口今天没有，摆一个永远灰的按钮是假动作。登记收付款另有一签。

/** 一笔收付款核到了哪里：映射目标种类译成中文、目标编号跟在后面；没有映射就说还没核到任何地方。 */
function mappedTo(mappings: FundsMappingRecord[]): string {
  if (mappings.length === 0) return '尚未核到任何地方';
  return mappings.map((mapping) => `${labelFor(mapping.targetKind, 'funds-target')} ${mapping.target}`).join('；');
}

function factTechnicalRows(row: ExternalFundsFactRecord) {
  return technicalRows([
    { label: '资金事实', value: row.fact },
    { label: '来源系统', value: row.source },
    { label: '版本', value: row.version },
    { label: '更正自', value: row.corrects },
    { label: '更正时间', value: row.correctedAt ? formatInstant(row.correctedAt) : undefined },
    // 缺席是那一路不记提交方（接口或受控 CLI 送入），照实说出缘由，不留白也不写「无」。
    { label: '登记人', value: row.operationEvidence ?? '未记（接口送入）' },
    { label: '已撤销金额', value: formatMoney(row.reversedAmount, row.currency) },
    { label: '核销笔数', value: String(row.applicationCount) },
    { label: '入账余额', value: scopePostingText(row) },
    {
      label: '映射依据',
      value: row.mappings.map((mapping) => `${mapping.mapping}：${mapping.basis}（${formatInstant(mapping.mappedAt)}）`).join('；') || undefined,
    },
  ]);
}

const columns: ListColumn<ExternalFundsFactRecord>[] = [
  { id: 'fact', header: '流水号', className: 'whitespace-nowrap', render: (row) => <span className="font-mono type-body leading-4 text-idpxyz-textBright">{row.fact}</span> },
  { id: 'source', header: '来源系统', className: 'whitespace-nowrap font-mono type-body leading-4', render: (row) => row.source },
  {
    id: 'kind',
    header: '种类',
    align: 'center',
    // 封闭三格照原样呈现：只有一格是「收到了钱」，折成收／付两向会把「付款失败」与「资金退回」压进同一格。
    render: (row) => <StatusPill code={row.kind} />,
  },
  {
    id: 'amount',
    header: '金额',
    align: 'right',
    className: 'font-mono type-body leading-4 tabular-nums',
    render: (row) => formatMoney(row.amount, row.currency),
  },
  {
    id: 'applied',
    header: '已核销 / 未核销',
    align: 'right',
    className: 'whitespace-nowrap font-mono type-body leading-4 tabular-nums',
    // 未核销是第一类结果不是尾差，不得被默认费用吞并；未核销不为零时亮出来。
    render: (row) => (
      <span>
        {formatMoney(row.appliedAmount, '')} /{' '}
        <span className={row.unappliedAmount !== '0' ? 'text-idpxyz-textBright' : 'text-idpxyz-textMuted'}>
          {formatMoney(row.unappliedAmount, '')}
        </span>
      </span>
    ),
  },
  { id: 'mapped', header: '核到哪里', className: 'type-body leading-4', render: (row) => mappedTo(row.mappings) },
  {
    id: 'occurred-at',
    header: '发生时间',
    className: 'whitespace-nowrap font-mono type-body leading-4',
    // 外部事实的业务发生时刻，不是系统接收或写入时间。
    render: (row) => formatInstant(row.occurredAt),
  },
  {
    id: 'technical',
    header: '',
    className: 'align-top',
    render: (row) => <TechnicalDetails rows={factTechnicalRows(row)} className="max-w-md open:min-w-[18rem]" />,
  },
];

/**
 * 收付款核销。独立成页而不是对账页内区块：核销的行对象是已采用的外部资金事实及其分配关系，未结项跨客户运营应收、
 * 供应商审核应付、赔付／退款义务、供应商费用贷项、追偿应收与代垫回收，不从属于对账单（CONTEXT「结算账户、对账与争议」）。
 *
 * 真实收付映射与核销分配片段、核销撤销届时按 DetailPageTemplate 另立详情——映射不是核销，事实接收、金额责任确认、
 * 真实到账与核销是不同结果。登记签登的是外部资金事实的采用与更正（ADR-0137 决定四：资金事实进产品只经 settlement-accounting
 * 采用；手工口 ADR-0236），映射与核销不在本签。
 */
export function SettlementApplicationPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="facts" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="facts">收付款</TabsTrigger>
          <TabsTrigger value="register">登记收付款</TabsTrigger>
        </TabsList>
        <TabsContent value="facts" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ExternalFundsFactsTable />
        </TabsContent>
        <TabsContent value="register" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          {/* 两口两张表单经选册切换而不是一张带模式的表单：登记与更正的输入形状互不相容（更正只带回指与金额），
              换册即换草稿那条纪律在 MultiRegistrationPanel 只有一处。 */}
          <MultiRegistrationPanel
            moduleId="settlement-application"
            targets={fundsRegistrationTargets}
            problemNote={problemNote}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}

// 两本册的登记面配置，逐项取 api / presentation 里的同一份词，不在此处另造。
const fundsRegistrationTargets: RegistrationTarget[] = (
  ['external-funds-fact', 'external-funds-fact-correction'] as FundsRegistrationKind[]
).map((kind) => ({
  id: kind,
  label: fundsRegistrationTitles[kind],
  title: fundsRegistrationTitles[kind],
  endpoint: `POST ${fundsRegistrationEndpoints[kind]}`,
  snapshotHint: fundsRegistrationSnapshotHints[kind],
  submit: (snapshot: unknown) => registerExternalFundsFact(kind, snapshot),
  outcomeLabels: fundsRegistrationOutcomeLabels,
}));

function ExternalFundsFactsTable() {
  const [search, setSearch] = useState('');
  const [refine, setRefine] = useState('');
  const conditions = { q: useSettledKeyword(search) };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listSettlementFundsApplications);
  const facts = answer?.kind === 'outcome' ? answer.body.facts : [];
  const visibleFacts = facts.filter((row) => matchesSearch(refine, [row.currency, mappedTo(row.mappings)]));
  const unapplied = facts.filter((row) => row.unappliedAmount !== '0').length;

  return (
    <ListPageTemplate<ExternalFundsFactRecord>
      title="收付款"
      description="银行和支付系统传来的每一笔收付款，以及它核到了哪张对账单或账户上"
      search={{ value: search, onChange: setSearch, placeholder: '搜流水号、来源系统' }}
      filters={<PageOnlySearch value={refine} onChange={setRefine} subject="币种、核销目标" />}
      filterSummary={answer?.kind === 'outcome' ? `本页 ${facts.length} 笔 · 有未核销金额 ${unapplied} 笔` : undefined}
      columns={columns}
      rows={visibleFacts}
      // 行键循身份加版本：相同身份与版本只采用一次，更正形成新有效版本。
      rowKey={(row) => `${row.fact}:${row.version}`}
      emptyRowsNote={pageEmptyNote('收付款', facts.length)}
      pagination={pagination}
      viewState={catalogueViewState(answer, facts.length, retry, {
        module: info,
        endpoint: 'GET /settlement-funds-applications',
        emptyTitle: '还没有收付款记录',
        emptyDescription: '收付款由银行或支付系统传入，或在「登记收付款」里手工登记后出现在这里；页面不会预置任何收付款。',
        narrowed: catalogueConditionsNarrow(conditions) || pastFirstPage(pagination),
      })}
    />
  );
}
