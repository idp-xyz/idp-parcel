import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { RegistrationPanel } from '../../components/registration';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  customsRegistrationEndpoints,
  listAdmissionJudgements,
  listCaseRegisters,
  listCredentials,
  listCustomsCases,
  listExternalResults,
  listSubmissionVersions,
  registerCustomsConfiguration,
  registrationOutcomeLabels,
  type AdmissionJudgementListResponseBody,
  type ClosureObligationCatalogueRecord,
  type ClosureObligationListResponseBody,
  type CredentialListResponseBody,
  type ReadinessJudgmentRecord,
  type ReadinessJudgmentListResponseBody,
  type SubmissionAuthorityRecord,
  type SubmissionAuthorityListResponseBody,
} from './api';
import { problemNote, registrationSnapshotHints, registrationTitles } from './presentation';
import { credentialRows, type RegisterRow } from './register-rows';
import { caseRows, externalResultRows, judgementRows, snapshotRows, submissionRows, unitRows } from './record-rows';
import { shortenEmbeddedPrincipalRefs } from '../../components/principal-ref';
import { labelFor } from '../../vocabulary/status';
import { technicalRows } from '../settlement/ops-ui';

// 清关单页（主责上下文 customs-compliance：稳定关务案件、申报单元、正式申报资料快照与不可覆盖提交版本）。
// 界面词与领域词的对照：清关单 = 关务案件，报关批次 = 申报单元，申报资料 = 正式申报资料快照，申报记录 = 提交版本，
// 海关结果与放行 = 外部监管结果，放行记录 = 动作准入判断册，关闭核对 = 关闭义务目录，申报准备 = 就绪判断与提交授权。
const info = moduleInfoById['customs-cases'];

// 四个对象族各自成签、不折进一张表：案件是稳定业务容器，批次是申报对象集合，资料是版本化快照，申报记录是不可覆盖的对外
// 快照——各自的版本与替代关系独立，折进一张表就会让「案件状态」冒充其余三层的进度。海关结果（接收、受理、放行）另一口，
// 申报记录行把它按归属的版本挂回去，各层各占一列，缺层写「尚无结果」，不由别的层推出来。

/** 列说明：mono 等宽；multi 保留换行；pill 按 values[pillCode] 的码出状态胶囊；tech 给出技术详情的行。 */
interface ColumnSpec {
  id: string;
  header: string;
  mono?: boolean;
  multi?: boolean;
  pillCode?: string;
  nowrap?: boolean;
}

function recordColumns(spec: ColumnSpec[], tech?: [label: string, key: string][]): ListColumn<RegisterRow>[] {
  const columns: ListColumn<RegisterRow>[] = spec.map((column) => ({
    id: column.id,
    header: column.header,
    className:
      [column.mono ? 'font-mono text-xs' : '', column.multi ? 'whitespace-pre-line' : '', column.nowrap ? 'whitespace-nowrap' : '']
        .filter(Boolean)
        .join(' ') || undefined,
    render: (row) =>
      column.pillCode ? (
        row.values[column.pillCode] ? (
          <StatusPill code={row.values[column.pillCode]} label={column.pillCode === column.id ? undefined : row.values[column.id]} />
        ) : (
          (row.values[column.id] ?? '—')
        )
      ) : (
        (row.values[column.id] ?? '—')
      ),
  }));
  if (tech) {
    columns.push({
      id: 'technical',
      header: '',
      className: 'align-top',
      render: (row) => (
        <TechnicalDetails rows={technicalRows(tech.map(([label, key]) => ({ label, value: row.values[key] })))} className="max-w-md open:min-w-[18rem]" />
      ),
    });
  }
  return columns;
}

const caseColumns = recordColumns(
  [
    { id: 'parcels', header: '运单号', mono: true, multi: true },
    { id: 'jurisdiction', header: '辖区', mono: true, nowrap: true },
    { id: 'direction', header: '程序', nowrap: true },
    { id: 'progress', header: '最新进展', pillCode: 'progress' },
    { id: 'updatedAt', header: '最后更新', mono: true, nowrap: true },
  ],
  [
    ['清关单号', 'id'],
    ['客户账户', 'customers'],
    ['监管程序', 'procedure'],
    ['法定义务', 'obligation'],
    ['报关批次', 'units'],
    ['关案记录', 'closure'],
    ['建案时间', 'establishedAt'],
  ],
);

const unitColumns = recordColumns(
  [
    { id: 'id', header: '批次号', mono: true, nowrap: true },
    { id: 'members', header: '运单号', mono: true, multi: true },
    { id: 'jurisdiction', header: '辖区', mono: true, nowrap: true },
    { id: 'direction', header: '程序', nowrap: true },
    { id: 'formedAt', header: '形成时间', mono: true, nowrap: true },
  ],
  [
    ['清关单号', 'caseRef'],
    ['监管程序', 'procedure'],
    ['替代批次', 'replaces'],
  ],
);

const snapshotColumns = recordColumns(
  [
    { id: 'unit', header: '批次号', mono: true, nowrap: true },
    { id: 'members', header: '运单号', mono: true, multi: true },
    { id: 'current', header: '现况', nowrap: true },
    { id: 'fixedAt', header: '固定时间', mono: true, nowrap: true },
  ],
  [
    ['申报版本', 'version'],
    ['监管程序', 'procedure'],
    ['资料引用', 'dossier'],
    ['角色快照', 'roles'],
    ['内容摘要', 'digest'],
  ],
);

const submissionColumns = recordColumns(
  [
    { id: 'unit', header: '批次号', mono: true, nowrap: true },
    { id: 'members', header: '运单号', mono: true, multi: true },
    { id: 'latestSend', header: '发送', nowrap: true },
    { id: 'received', header: '海关接收' },
    { id: 'accepted', header: '海关受理' },
    { id: 'current', header: '现况', nowrap: true },
    { id: 'fixedAt', header: '固定时间', mono: true, nowrap: true },
  ],
  [
    ['申报版本', 'version'],
    ['更正自', 'correctedFrom'],
    ['监管程序', 'procedure'],
    ['授权依据', 'authority'],
    ['就绪依据', 'readinessBasis'],
    ['全部尝试', 'attempts'],
    ['技术回执', 'receipts'],
    ['报文号', 'channelMessages'],
  ],
);

const resultColumns = recordColumns(
  [
    { id: 'scope', header: '批次号', mono: true, nowrap: true },
    { id: 'layer', header: '结果层', pillCode: 'layerCode', nowrap: true },
    { id: 'semantics', header: '海关怎么说' },
    { id: 'release', header: '放行' },
    { id: 'flags', header: '冲突与更正' },
    { id: 'occurredAt', header: '发生时间', mono: true, nowrap: true },
  ],
  [
    ['来源标识', 'source'],
    ['归属申报版本', 'version'],
    ['读法规则', 'rule'],
    ['接收时间', 'recordedAt'],
  ],
);

const judgementColumns = recordColumns(
  [
    { id: 'object', header: '运单号', mono: true, nowrap: true },
    { id: 'intent', header: '动作', nowrap: true },
    { id: 'route', header: '网点 → 去向', mono: true, nowrap: true },
    { id: 'crossZone', header: '跨关区', nowrap: true },
    { id: 'answer', header: '结论', pillCode: 'answerCode' },
    { id: 'findings', header: '逐项检查', multi: true },
    { id: 'actionAt', header: '动作时点', mono: true, nowrap: true },
  ],
  [
    ['判断标识', 'id'],
    ['判断时间', 'judgedAt'],
  ],
);

/** 取一次数据并交回行；各读签共用，页面只管列与文案。 */
function useRecordAnswer<Body>(load: () => Promise<ApiResult<Body>>) {
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<Body> | null>(null);
  useEffect(() => {
    let cancelled = false;
    void load().then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reloadKey]);
  return { answer, retry: () => setReloadKey((value) => value + 1) };
}

/** 几问合成一个答案：任一问不成按那一问的状态呈现——只拿到半份数据就开画，会把另一半画成假话。 */
function combined(...answers: Array<ApiResult<unknown> | null>): ApiResult<unknown> | null {
  if (answers.some((answer) => answer === null)) return null;
  return answers.find((answer) => answer?.kind !== 'outcome') ?? answers[0];
}

function filterRows<Row extends RegisterRow>(rows: Row[], search: string): Row[] {
  const needle = search.trim().toLowerCase();
  return needle ? rows.filter((row) => Object.values(row.values).some((value) => value.toLowerCase().includes(needle))) : rows;
}

interface RecordTableProps {
  title: string;
  description: string;
  searchPlaceholder: string;
  columns: ListColumn<RegisterRow>[];
  rows: RegisterRow[];
  answer: ApiResult<unknown> | null;
  retry: () => void;
  endpoint: string;
  emptyTitle: string;
  emptyDescription: string;
  filterSummary?: string;
}

function RecordTable(props: RecordTableProps) {
  const [search, setSearch] = useState('');
  return (
    <ListPageTemplate<RegisterRow>
      title={props.title}
      description={props.description}
      search={{ value: search, onChange: setSearch, placeholder: props.searchPlaceholder }}
      filterSummary={props.answer?.kind === 'outcome' ? props.filterSummary : undefined}
      columns={props.columns}
      rows={filterRows(props.rows, search)}
      rowKey={(row) => row.key}
      emptyRowsNote="没有匹配的记录"
      viewState={catalogueViewState(props.answer, props.rows.length, props.retry, {
        module: info,
        endpoint: props.endpoint,
        emptyTitle: props.emptyTitle,
        emptyDescription: props.emptyDescription,
      })}
    />
  );
}

function CustomsCasesTable() {
  const cases = useRecordAnswer(listCustomsCases);
  const submissions = useRecordAnswer(listSubmissionVersions);
  const results = useRecordAnswer(listExternalResults);
  const answer = combined(cases.answer, submissions.answer, results.answer);
  const records = cases.answer?.kind === 'outcome' ? cases.answer.body.cases : [];
  const rows =
    answer?.kind === 'outcome'
      ? caseRows(
          records,
          submissions.answer?.kind === 'outcome' ? submissions.answer.body.submissions : [],
          results.answer?.kind === 'outcome' ? results.answer.body.results : [],
        )
      : [];
  const pending = rows.filter((row) => row.values.progress === 'CUSTOMS_PENDING_DECLARATION' || row.values.progress === 'DECLARATION_PENDING_SEND').length;
  return (
    <RecordTable
      title="清关单"
      description="每票出口或进口货物的清关单：走到哪一步、最近一次更新是什么时候"
      searchPlaceholder="搜运单号、辖区、清关单号"
      columns={caseColumns}
      rows={rows}
      answer={answer}
      retry={() => {
        cases.retry();
        submissions.retry();
        results.retry();
      }}
      endpoint="GET /customs-case-records 与 /customs-submission-records、/customs-external-result-records"
      emptyTitle="还没有清关单"
      emptyDescription="运单收寄后按规则自动立清关单，这里会随之出现；页面不会预置。"
      filterSummary={`${rows.length} 张 · 待申报或待发送 ${pending} 张 · 已关案 ${records.filter((c) => c.closure).length} 张`}
    />
  );
}

function DeclarationUnitsTable() {
  const { answer, retry } = useRecordAnswer(listCustomsCases);
  const cases = answer?.kind === 'outcome' ? answer.body.cases : [];
  const rows = unitRows(cases);
  return (
    <RecordTable
      title="报关批次"
      description="一次向海关申报的一批运单；资料和申报记录都挂在批次上"
      searchPlaceholder="搜批次号、运单号、清关单号"
      columns={unitColumns}
      rows={rows}
      answer={answer}
      retry={retry}
      endpoint="GET /customs-case-records"
      emptyTitle="还没有报关批次"
      emptyDescription="清关单下还没有组成报关批次；批次组成之后才能固定资料、向海关申报。"
      filterSummary={`${rows.length} 个批次`}
    />
  );
}

function DeclarationSnapshotsTable() {
  const { answer, retry } = useRecordAnswer(listSubmissionVersions);
  const submissions = answer?.kind === 'outcome' ? answer.body.submissions : [];
  return (
    <RecordTable
      title="申报资料"
      description="每次申报发出前固定下来的那份资料；发出后不再改，改了就是下一次申报"
      searchPlaceholder="搜批次号、运单号"
      columns={snapshotColumns}
      rows={snapshotRows(submissions)}
      answer={answer}
      retry={retry}
      endpoint="GET /customs-submission-records"
      emptyTitle="还没有固定过申报资料"
      emptyDescription="还没有发起过申报，所以没有固定的资料；页面不会预置。"
      filterSummary={`${submissions.length} 份`}
    />
  );
}

function SubmissionVersionsTable() {
  const submissionsAnswer = useRecordAnswer(listSubmissionVersions);
  const resultsAnswer = useRecordAnswer(listExternalResults);
  const submissions = submissionsAnswer.answer?.kind === 'outcome' ? submissionsAnswer.answer.body.submissions : [];
  const results = resultsAnswer.answer?.kind === 'outcome' ? resultsAnswer.answer.body.results : [];
  const answer = combined(submissionsAnswer.answer, resultsAnswer.answer);
  return (
    <RecordTable
      title="申报记录"
      description="每次向海关申报的发送情况与海关的接收、受理结果；各层分开记，发出去不等于海关收到"
      searchPlaceholder="搜批次号、运单号"
      columns={submissionColumns}
      rows={answer?.kind === 'outcome' ? submissionRows(submissions, results) : []}
      answer={answer}
      retry={() => {
        submissionsAnswer.retry();
        resultsAnswer.retry();
      }}
      endpoint="GET /customs-submission-records 与 /customs-external-result-records"
      emptyTitle="还没有申报记录"
      emptyDescription="还没有向海关申报过；申报发出之后才有发送情况、技术回执与海关结果。"
      filterSummary={`${submissions.length} 次申报 · 海关结果 ${results.length} 份`}
    />
  );
}

function ExternalResultsTable() {
  const { answer, retry } = useRecordAnswer(listExternalResults);
  const results = answer?.kind === 'outcome' ? answer.body.results : [];
  return (
    <RecordTable
      title="海关结果与放行"
      description="海关和报关行传回来的每一条结果：接收、受理、核定税费、放行；归属不上的只留原文，不猜"
      searchPlaceholder="搜批次号、来源、结果层"
      columns={resultColumns}
      rows={externalResultRows(results)}
      answer={answer}
      retry={retry}
      endpoint="GET /customs-external-result-records"
      emptyTitle="还没有收到海关结果"
      emptyDescription="接收、受理、放行等结果由报关行或海关接口送入；还没有收到不等于被拒绝。"
      filterSummary={`${results.length} 条 · 其中放行 ${results.filter((r) => r.release).length} 条`}
    />
  );
}

// 放行记录按运单号查：不填则列最近的记录。记录是判断当时的结论，换了规则之后同一请求的新判断是另一行，旧行不改。
function AdmissionJudgementsTable() {
  const [draft, setDraft] = useState('');
  const [object, setObject] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<AdmissionJudgementListResponseBody> | null>(null);
  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listAdmissionJudgements(object).then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [object, reloadKey]);
  const judgements = answer?.kind === 'outcome' ? answer.body.judgements : [];
  const blocked = judgements.filter((judgement) => judgement.answer === 'BLOCKED').length;
  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <form
        className="flex items-center gap-2 px-4 py-2 shrink-0"
        onSubmit={(event) => {
          event.preventDefault();
          setObject(draft.trim());
        }}
      >
        <label className="text-sm" htmlFor="admission-object">
          按运单号查
        </label>
        <input
          id="admission-object"
          className="border rounded px-2 py-1 text-sm font-mono"
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          placeholder="运单号；留空列出最近的记录"
        />
        <button type="submit" className="border rounded px-3 py-1 text-sm">
          查询
        </button>
      </form>
      <RecordTable
        title="放行记录"
        description="货物出库、装车、跨关区、交付前每一次放行检查的结论；按当时的规则判，不回头改"
        searchPlaceholder="在结果内搜运单号、网点、结论"
        columns={judgementColumns}
        rows={judgementRows(judgements)}
        answer={answer}
        retry={() => setReloadKey((value) => value + 1)}
        endpoint={`GET /customs-action-admission-judgements${object ? `?object=${object}` : ''}`}
        emptyTitle={object ? `没有运单「${object}」的放行记录` : '还没有放行记录'}
        emptyDescription="只有出库、装车、交付时问过放行检查才有记录；没有记录不等于被拦，也不等于可放行。"
        filterSummary={`${judgements.length} 条 · 已拦截 ${blocked} 条`}
      />
    </div>
  );
}

// —— 申报准备（就绪判断与提交授权）——

interface CatalogueRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

function col(id: string, header: string, mono = false, nowrap = false): ListColumn<CatalogueRow> {
  return {
    id,
    header,
    className: [mono ? 'font-mono text-xs' : '', nowrap ? 'whitespace-nowrap' : ''].filter(Boolean).join(' ') || undefined,
    render: (row) => row.values[id] ?? '—',
  };
}

// 就绪与授权两栏并排、各带自己的现况（CONTEXT「申报就绪判断与提交授权必须独立存在」）。不设「可提交」合成列：「就绪还在、
// 授权已撤销」是必须表达得出的一格，合成布尔正好把它抹掉。
const preconditionColumns: ListColumn<CatalogueRow>[] = [
  col('unit', '批次号', true, true),
  col('readinessState', '资料就绪', false, true),
  col('readinessAt', '就绪时间', true, true),
  col('authorityState', '提交授权', false, true),
  col('authorityAt', '授权时间', true, true),
  {
    id: 'technical',
    header: '',
    className: 'align-top',
    render: (row) => (
      <TechnicalDetails
        rows={technicalRows([
          { label: '就绪依据', value: row.values.readinessBasis },
          { label: '授权依据', value: row.values.authorityRef },
        ])}
        className="max-w-md open:min-w-[18rem]"
      />
    ),
  },
];

// 现况一栏封闭三态：「未登记」＝这本册子没有该批次的行；「仍有效」＝撤销两列缺席；「已撤销」＝失效在场且原依据原样留在
// 技术详情——失效不是删除，也不得谎报成未配置。
function trackState(record?: { revokedBy?: string; revokedAt?: string }): string {
  if (!record) return '未登记';
  if (record.revokedBy && record.revokedAt) {
    return `已撤销：${shortenEmbeddedPrincipalRefs(record.revokedBy)}（${formatInstant(record.revokedAt)}）`;
  }
  return '仍有效';
}

function preconditionRows(judgments: ReadinessJudgmentRecord[], authorities: SubmissionAuthorityRecord[]): CatalogueRow[] {
  const byUnit = new Map<string, { judgment?: ReadinessJudgmentRecord; authority?: SubmissionAuthorityRecord }>();
  for (const judgment of judgments) byUnit.set(judgment.unit, { ...byUnit.get(judgment.unit), judgment });
  for (const authority of authorities) byUnit.set(authority.unit, { ...byUnit.get(authority.unit), authority });
  return [...byUnit.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([unit, pair]) => ({
      key: `unit:${unit}`,
      values: {
        unit,
        readinessBasis: shortenEmbeddedPrincipalRefs(pair.judgment?.basis ?? ''),
        readinessAt: pair.judgment ? formatInstant(pair.judgment.judgedAt) : '—',
        readinessState: trackState(pair.judgment),
        authorityRef: shortenEmbeddedPrincipalRefs(pair.authority?.authority ?? ''),
        authorityAt: pair.authority ? formatInstant(pair.authority.grantedAt) : '—',
        authorityState: trackState(pair.authority),
      },
    }));
}

function SubmissionPreconditionsTable() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{
    readiness: ApiResult<ReadinessJudgmentListResponseBody>;
    authority: ApiResult<SubmissionAuthorityListResponseBody>;
  } | null>(null);

  useEffect(() => {
    let cancelled = false;
    void Promise.all([listCaseRegisters('readiness'), listCaseRegisters('submission-authority')]).then(([readiness, authority]) => {
      if (!cancelled) setLoaded({ readiness, authority });
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const answer = combined(loaded?.readiness ?? null, loaded?.authority ?? null);
  const listed =
    loaded?.readiness.kind === 'outcome' && loaded.authority.kind === 'outcome'
      ? { judgments: loaded.readiness.body.judgments, authorities: loaded.authority.body.authorities }
      : null;
  const rows = listed ? preconditionRows(listed.judgments, listed.authorities) : [];
  const needle = search.trim().toLowerCase();
  const visibleRows = needle ? rows.filter((row) => Object.values(row.values).some((value) => value.toLowerCase().includes(needle))) : rows;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<CatalogueRow>
      title="申报准备"
      description="每个报关批次申报前的两道准备：资料是否就绪、是否授权提交；两道各自成立、各自撤销"
      search={{ value: search, onChange: setSearch, placeholder: '搜批次号' }}
      filterSummary={listed ? `${rows.length} 个批次 · 就绪 ${listed.judgments.length} · 授权 ${listed.authorities.length}` : undefined}
      columns={preconditionColumns}
      rows={visibleRows}
      rowKey={(row) => row.key}
      emptyRowsNote="没有匹配的批次"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /customs-case-registers?registry=readiness 与 ?registry=submission-authority',
        emptyTitle: '还没有批次做过申报准备',
        emptyDescription: '报关批次的资料就绪与提交授权登记之后在这里出现；页面不会预置。',
      })}
    />
  );
}

// —— 关闭核对清单 ——

// 目录行与义务项是两条独立信息：有义务项的目录逐项成行；目录已登记而清单为空占一行写明——它与「目录未登记 → 未决」
// 含义相反，后者根本不在本列（空册文案另说）。
const closureColumns: ListColumn<CatalogueRow>[] = [
  col('caseRef', '清关单号', true, true),
  col('obligation', '核对事项'),
  col('state', '处理情况', false, true),
  col('handedTo', '承接方', true, true),
  col('appliesFrom', '生效自', true, true),
  {
    id: 'technical',
    header: '',
    className: 'align-top',
    render: (row) => (
      <TechnicalDetails
        rows={technicalRows([
          { label: '适用范围', value: row.values.scope },
          { label: '依据', value: row.values.basis },
          { label: '截止', value: row.values.appliesUntil },
          { label: '清单登记时间', value: row.values.registeredAt },
        ])}
        className="max-w-md open:min-w-[18rem]"
      />
    ),
  },
];

function closureRows(catalogues: ClosureObligationCatalogueRecord[]): CatalogueRow[] {
  return catalogues.flatMap((catalogue) => {
    const base = { caseRef: catalogue.case, registeredAt: formatInstant(catalogue.registeredAt) };
    if (catalogue.items.length === 0) {
      return [{ key: `catalogue:${catalogue.case}`, values: { ...base, obligation: '清单已登记，当前没有要核对的事项' } }];
    }
    return catalogue.items.map((item) => ({
      // 行键循库主键 (tenant, case_ref, obligation)：同目录内义务名唯一。
      key: `item:${catalogue.case}:${item.obligation}`,
      values: {
        ...base,
        obligation: item.obligation,
        scope: item.scope,
        state: labelFor(item.state, 'customs-obligation'),
        basis: item.basis,
        ...(item.handedTo ? { handedTo: item.handedTo } : {}),
        appliesFrom: formatInstant(item.appliesFrom),
        appliesUntil: item.appliesUntil ? formatInstant(item.appliesUntil) : '',
      },
    }));
  });
}

function ClosureObligationsTable() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<ClosureObligationListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listCaseRegisters('closure-obligation').then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const catalogues = answer?.kind === 'outcome' ? answer.body.catalogues : [];
  const rows = closureRows(catalogues);
  const needle = search.trim().toLowerCase();
  const visibleRows = needle ? rows.filter((row) => Object.values(row.values).some((value) => value.toLowerCase().includes(needle))) : rows;
  const retry = () => setReloadKey((value) => value + 1);
  const unresolved = catalogues.reduce((sum, catalogue) => sum + catalogue.items.filter((item) => item.state === 'UNRESOLVED').length, 0);

  return (
    <ListPageTemplate<CatalogueRow>
      title="关闭核对"
      description="清关单关案前要逐项核对的事项：哪些已了结、哪些交给了别人、哪些还没解决"
      search={{ value: search, onChange: setSearch, placeholder: '搜清关单号、事项' }}
      filterSummary={answer?.kind === 'outcome' ? `${catalogues.length} 张清关单 · 未解决 ${unresolved} 项` : undefined}
      columns={closureColumns}
      rows={visibleRows}
      rowKey={(row) => row.key}
      emptyRowsNote="没有匹配的事项"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /customs-case-registers?registry=closure-obligation',
        emptyTitle: '还没有关闭核对清单',
        emptyDescription: '清关单登记了关闭核对清单后在这里出现；没登记清单的清关单关不了案。',
      })}
    />
  );
}

// —— 监管凭证 ——

// 一行即一版不可变凭证（一身份一版，换期限或额度是另一张凭证）。刻意不设「适用性」列：凭证对某程序、某持有人、某时点适用
// 不适用是判断链的产物，查阅面转述登记册本身。
const credentialColumns: ListColumn<RegisterRow>[] = [
  col('credential', '凭证号', true, true) as ListColumn<RegisterRow>,
  col('holder', '持有人', true, true) as ListColumn<RegisterRow>,
  col('procedure', '适用程序', true, true) as ListColumn<RegisterRow>,
  col('validity', '有效期', true, true) as ListColumn<RegisterRow>,
  col('uses', '次数额度', false, true) as ListColumn<RegisterRow>,
  {
    id: 'technical',
    header: '',
    className: 'align-top',
    render: (row) => (
      <TechnicalDetails
        rows={technicalRows([
          { label: '签发机构', value: row.values.issuer },
          { label: '登记时间', value: row.values.registeredAt },
          { label: '登记人', value: row.values.registeredBy },
        ])}
        className="max-w-md open:min-w-[18rem]"
      />
    ),
  },
];

function CredentialsTable() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<CredentialListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listCredentials().then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const credentials = answer?.kind === 'outcome' ? answer.body.credentials : [];
  const rows = credentialRows(credentials);
  const needle = search.trim().toLowerCase();
  const visibleRows = needle ? rows.filter((row) => Object.values(row.values).some((value) => value.toLowerCase().includes(needle))) : rows;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<RegisterRow>
      title="监管凭证"
      description="报关要用到的许可、备案等凭证；一张凭证一条，到期或换额度另登一张"
      search={{ value: search, onChange: setSearch, placeholder: '搜凭证号、持有人、程序' }}
      filterSummary={answer?.kind === 'outcome' ? `${credentials.length} 张` : undefined}
      columns={credentialColumns}
      rows={visibleRows}
      rowKey={(row) => row.key}
      emptyRowsNote="没有匹配的凭证"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /customs-credentials',
        emptyTitle: '还没有登记监管凭证',
        emptyDescription: '凭证没登记时，申报准备会停在「凭证未登记」，不会当作不需要。请在「登记监管凭证」里照原件登记。',
      })}
    />
  );
}

/**
 * 清关单页。签按一票货清关的顺序排：清关单 → 报关批次 → 申报资料 → 申报记录 → 海关结果与放行 → 放行记录 → 关闭核对，
 * 之后是申报前的两道准备（申报准备、监管凭证）与凭证的登记签。本页只有凭证一本册有在线登记口：就绪、授权与关闭核对改的
 * 是案上此刻的事实，不进写面。
 */
export function CustomsCasesPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="cases" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="cases">清关单</TabsTrigger>
          <TabsTrigger value="units">报关批次</TabsTrigger>
          <TabsTrigger value="snapshots">申报资料</TabsTrigger>
          <TabsTrigger value="submissions">申报记录</TabsTrigger>
          <TabsTrigger value="results">海关结果与放行</TabsTrigger>
          <TabsTrigger value="admission">放行记录</TabsTrigger>
          <TabsTrigger value="closure-obligations">关闭核对</TabsTrigger>
          <TabsTrigger value="preconditions">申报准备</TabsTrigger>
          <TabsTrigger value="credentials">监管凭证</TabsTrigger>
          <TabsTrigger value="register">登记监管凭证</TabsTrigger>
        </TabsList>
        <TabsContent value="cases" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <CustomsCasesTable />
        </TabsContent>
        <TabsContent value="units" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <DeclarationUnitsTable />
        </TabsContent>
        <TabsContent value="snapshots" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <DeclarationSnapshotsTable />
        </TabsContent>
        <TabsContent value="submissions" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <SubmissionVersionsTable />
        </TabsContent>
        <TabsContent value="results" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ExternalResultsTable />
        </TabsContent>
        <TabsContent value="admission" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <AdmissionJudgementsTable />
        </TabsContent>
        <TabsContent value="closure-obligations" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ClosureObligationsTable />
        </TabsContent>
        <TabsContent value="preconditions" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <SubmissionPreconditionsTable />
        </TabsContent>
        <TabsContent value="credentials" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <CredentialsTable />
        </TabsContent>
        <TabsContent value="register" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          {/* 本页只有一本册可在线登记，直接摆 RegistrationPanel，不套选册。凭证交回配置族那套答案，词表复用 registrationOutcomeLabels。 */}
          <RegistrationPanel
            moduleId="customs-cases"
            title={registrationTitles['regulatory-credential']}
            endpoint={`POST ${customsRegistrationEndpoints['regulatory-credential']}`}
            snapshotHint={registrationSnapshotHints['regulatory-credential']}
            submit={(snapshot) => registerCustomsConfiguration('regulatory-credential', snapshot)}
            outcomeLabels={registrationOutcomeLabels}
            problemNote={problemNote}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
