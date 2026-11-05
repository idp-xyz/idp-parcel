import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import type { ModuleInfo } from '../../navigation';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorContent } from '../../templates/inspector';
import { MultiRegistrationPanel, chipClass, type RegistrationTarget } from '../../components/registration';
import { StatusPill } from '../../components/StatusPill';
import { shortPrincipalRef } from '../../components/principal-ref';
import { labelFor } from '../../vocabulary/status';
import { catalogueViewState } from '../catalogue-view';
import { entryHint } from '../operator-grants';
import { useMayWriteRegistry } from '../use-operator-grants';
import { useCustomerNames } from '../shipment-request/use-customer-names';
import {
  listVisibilityCatalogues,
  registerVisibilityCatalogue,
  visibilityRegistrationEndpoints,
  type ApiResult,
  type VisibilityCatalogueListResponseBody,
} from './catalogue-api';
import {
  registrationOutcomeLabels,
  registrationProblemNote,
  registrationRefusalReasonLabels,
  registrationSnapshotHints,
  registrationTitles,
  visibilityCatalogueKindLabels,
} from './presentation';
import {
  ruleBookNotes,
  ruleCountSummary,
  ruleRowMatches,
  ruleRowsOf,
  type RuleBookKind,
  type RuleRow,
} from './rule-books';

// 「追踪规则」「通知与披露」两页共用的规则册页：上签查阅、下签登记，各册一套列。两页只差收哪几本册与页头一句话，
// 结构写一份——两页各写一份会在改列、改空态时分叉。
//
// 列只放运营要看的：管的事（动态 → 状态、信号 → 处理方式、客户 → 能看到什么）、是否生效、生效期；版本号、批准人全串、
// 原码进右栏「审计」。登记签不是逐字段表单：登记快照的形状与受控登记命令行吃的同一份，渠道原始载荷怎么译成快照属渠道
// 接入契约（PAR-INT-01），本页只给填写提示。各册都没有覆盖或删除动作，换规则就是登一版新的。

/** 一列：表头、格里的字（右栏概要也用它），可选把格渲染成状态胶囊（格里存的是码）。 */
interface RuleColumn {
  id: string;
  header: string;
  pill?: boolean;
  mono?: boolean;
}

const lifecycleColumn: RuleColumn = { id: 'lifecycle', header: '是否生效', pill: true };

const columnsByKind: Record<RuleBookKind, RuleColumn[]> = {
  MILESTONE_MAPPING: [
    { id: 'source', header: '环节' },
    { id: 'fact', header: '报上来的动态' },
    { id: 'milestone', header: '归成状态', pill: true },
    lifecycleColumn,
    { id: 'effective', header: '生效期' },
  ],
  TRIAGE_RULE: [
    { id: 'signal', header: '异常信号' },
    { id: 'confidence', header: '可信度' },
    { id: 'outcome', header: '怎么处理', pill: true },
    lifecycleColumn,
    { id: 'effective', header: '生效期' },
  ],
  CONFLICT_SIGNAL_RULE: [
    { id: 'signal', header: '记成的信号' },
    { id: 'confidence', header: '可信度依据' },
    { id: 'approvedBy', header: '批准人', mono: true },
    { id: 'registeredAt', header: '设定时间', mono: true },
  ],
  NOTIFICATION_POLICY: [
    { id: 'channel', header: '通知渠道' },
    { id: 'deadline', header: '时限' },
    { id: 'obligation', header: '怎样算通知到' },
    { id: 'approvedBy', header: '批准人', mono: true },
  ],
  DISCLOSURE_POLICY: [
    { id: 'customer', header: '客户' },
    { id: 'milestones', header: '物流状态' },
    { id: 'eta', header: '预计送达' },
    { id: 'final', header: '最终结果' },
    { id: 'note', header: '备注' },
    lifecycleColumn,
  ],
  EXCEPTION_DISCLOSURE_RULE: [
    { id: 'customer', header: '客户' },
    { id: 'signal', header: '异常类型' },
    { id: 'confidence', header: '可信度' },
    { id: 'disclosable', header: '告诉客户' },
    { id: 'autoRelease', header: '发出方式' },
    lifecycleColumn,
  ],
};

function cellCode(row: RuleRow, id: string): string | undefined {
  return id === 'lifecycle' ? row.lifecycle : row.cells[id];
}

/** 格里的字：胶囊格存的是码，经词典译；批准人显短名。右栏概要与表格同一份。 */
function cellText(row: RuleRow, column: RuleColumn): string {
  const value = cellCode(row, column.id);
  if (value === undefined || value === '') return '—';
  if (column.pill) return labelFor(value);
  if (column.id === 'approvedBy') return shortPrincipalRef(value);
  return value;
}

function listColumnsOf(kind: RuleBookKind): ListColumn<RuleRow>[] {
  return columnsByKind[kind].map((column) => ({
    id: column.id,
    header: column.header,
    className: column.mono ? 'font-mono type-body leading-4' : undefined,
    render: (row: RuleRow) => {
      const code = cellCode(row, column.id);
      if (column.pill && code) return <StatusPill code={code} />;
      const text = cellText(row, column);
      return column.id === 'approvedBy' ? <span title={row.cells.approvedBy}>{text}</span> : text;
    },
  }));
}

function inspectorOf(kind: RuleBookKind, row: RuleRow): InspectorContent {
  // 概要放业务格（含生效期与批准人，表格里有的列被收起时这里仍看得到），审计放册名、版本号与原码。
  const summaryColumns: RuleColumn[] = [
    ...columnsByKind[kind],
    ...(row.cells.effective && !columnsByKind[kind].some((column) => column.id === 'effective')
      ? [{ id: 'effective', header: '生效期' }]
      : []),
    ...(row.cells.approvedBy && !columnsByKind[kind].some((column) => column.id === 'approvedBy')
      ? [{ id: 'approvedBy', header: '批准人' }]
      : []),
  ];
  return {
    title: visibilityCatalogueKindLabels[kind],
    sections: [
      {
        kind: 'summary',
        fields: summaryColumns.slice(0, 8).map((column) => ({ label: column.header, value: cellText(row, column) })),
      },
      { kind: 'audit', fields: row.audit.filter((entry) => entry.value !== '').map((entry) => ({ ...entry, mono: true })) },
    ],
  };
}

/** 登记签按授予收起时，空态里原本指向它的那半句换成这句：说谁能新增，不让人去找看不见的签。 */
const hiddenRuleEntryHint = '新增规则需要配置权限，需要时请联系管理员；';

type ListBody = Extract<VisibilityCatalogueListResponseBody, { kind: RuleBookKind }>;

function RuleBookTable({
  info,
  kinds,
  description,
  mayWrite,
}: {
  info: ModuleInfo;
  kinds: readonly RuleBookKind[];
  description: string;
  mayWrite: boolean;
}) {
  const [kind, setKind] = useState<RuleBookKind>(kinds[0]);
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{ kind: RuleBookKind; answer: ApiResult<ListBody> } | null>(null);
  const names = useCustomerNames();

  useEffect(() => {
    let cancelled = false;
    void listVisibilityCatalogues(kind).then((answer) => {
      if (!cancelled) setLoaded({ kind, answer: answer as ApiResult<ListBody> });
    });
    return () => {
      cancelled = true;
    };
  }, [kind, reloadKey]);

  const answer = loaded?.kind === kind ? loaded.answer : null;
  const rows = answer?.kind === 'outcome' ? ruleRowsOf(answer.body, names, new Date()) : [];
  const visibleRows = rows.filter((row) => ruleRowMatches(row, search));
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<RuleRow>
      title={info.title}
      description={`${description}当前：${ruleBookNotes[kind]}`}
      search={{ value: search, onChange: setSearch, placeholder: '搜状态、信号、客户或版本号' }}
      filters={
        <>
          {kinds.map((candidate) => (
            <button
              key={candidate}
              type="button"
              className={chipClass(candidate === kind)}
              aria-pressed={candidate === kind}
              onClick={() => setKind(candidate)}
            >
              {visibilityCatalogueKindLabels[candidate]}
            </button>
          ))}
        </>
      }
      // 计数只在拿到业务答案后显示，未配置态不报「0 条」。
      filterSummary={answer?.kind === 'outcome' ? ruleCountSummary(kind, rows) : undefined}
      emptyRowsNote="当前搜索条件下没有匹配的规则"
      columns={listColumnsOf(kind)}
      rows={visibleRows}
      rowKey={(row) => row.key}
      inspector={(row) => inspectorOf(kind, row)}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: `GET /visibility-catalogues?kind=${kind}`,
        emptyTitle: `还没有「${visibilityCatalogueKindLabels[kind]}」规则`,
        // 空册就是没登记：系统不会替公司生成默认规则（租户取值留给租户）。
        emptyDescription: `${ruleBookNotes[kind]}${entryHint(mayWrite, '在旁边的登记签里新增后会出现在这里；', hiddenRuleEntryHint)}系统不会自带默认规则。`,
      })}
    />
  );
}

/**
 * 规则册页：查阅一签、登记一签（ADR-0085）。登记签铺本页查阅签里的每一册，不多铺别页的册——多铺一册会让同一本册在两处
 * 都能登，而其中一处的页面上看不到登进去的结果。
 */
export function RuleBookPage({
  moduleId,
  info,
  kinds,
  description,
  registerLabel,
}: {
  moduleId: string;
  info: ModuleInfo;
  kinds: readonly RuleBookKind[];
  /** 页头一句话：这页是干什么的、什么时候来。 */
  description: string;
  /** 登记签的签名。 */
  registerLabel: string;
}) {
  // 没有登记册配置写授予的操作者不摆登记签（ADR-0315）；摆不摆只是呈现，登记口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  const targets: RegistrationTarget[] = kinds.map((candidate) => ({
    id: candidate,
    label: visibilityCatalogueKindLabels[candidate],
    title: registrationTitles[candidate],
    endpoint: `POST ${visibilityRegistrationEndpoints[candidate]}`,
    snapshotHint: registrationSnapshotHints[candidate],
    submit: (snapshot) => registerVisibilityCatalogue(candidate, snapshot),
    outcomeLabels: registrationOutcomeLabels,
    refusalReasonLabels: registrationRefusalReasonLabels,
  }));

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="catalogue" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="catalogue">{info.title}</TabsTrigger>
          {mayWrite ? <TabsTrigger value="register">{registerLabel}</TabsTrigger> : null}
        </TabsList>
        <TabsContent value="catalogue" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <RuleBookTable info={info} kinds={kinds} description={description} mayWrite={mayWrite} />
        </TabsContent>
        {mayWrite ? (
          <TabsContent value="register" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
            <MultiRegistrationPanel moduleId={moduleId} targets={targets} problemNote={registrationProblemNote} />
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
