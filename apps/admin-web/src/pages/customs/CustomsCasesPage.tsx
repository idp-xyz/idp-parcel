import { useEffect, useState } from 'react';
import { useOwnAddressChange } from '../../templates/use-own-address-change';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn, type ListCursorPaginationProps } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { RegistrationPanel } from '../../components/registration';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import type { ApiResult } from '../catalogue-api';
import { catalogueConditionsNarrow, type CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { matchesSearch } from '../party/list-order';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { pastFirstPage } from '../shipment-request/cursor-pager';
import { pageEmptyNote } from '../settlement/list-conditions';
import { PageOnlySearch } from '../visibility/page-only';
import {
  customsRegistrationEndpoints,
  findCustomsCasesById,
  listAdmissionJudgements,
  listCaseRegisters,
  listCredentials,
  listCustomsCases,
  listExternalResults,
  listSubmissionVersions,
  listUnadmittedObjects,
  registerCustomsConfiguration,
  registrationOutcomeLabels,
  registryConditions,
  type ClosureObligationCatalogueRecord,
  type CustomsCaseBody,
  type ExternalResultBody,
  type ReadinessJudgmentRecord,
  type RegistryConditions,
  type SubmissionAuthorityRecord,
  type SubmissionVersionBody,
} from './api';
import { everyPage } from './every-page';
import { casesHash, casesTabOf, casesTabs, unadmittedOnlyOf, type CasesTab } from './cases-address';
import { problemNote, registrationSnapshotHints, registrationTitles } from './presentation';
import { credentialRows, type RegisterRow } from './register-rows';
import { placeLabel } from '../network/place-names';
import { usePlaceNames } from '../network/use-place-names';
import {
  caseNumberReader,
  caseRows,
  externalResultRows,
  judgementRows,
  snapshotRows,
  submissionRows,
  unitRows,
  type CaseNumberCell,
} from './record-rows';
import { documentTypeWords } from '../../vocabulary/document-numbering';
import { objectNouns } from '../../vocabulary/objects';
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
//
// 各签的读口已迁到 ADR-0144，按游标翻页（模板的翻页条），条数只数这一页。检索框下推成 `q`，各册覆盖哪几列在
// internal/customscompliance/adapters/postgres 的各册绑定里；码的中文词、页面派生的进展与发送情况、查来的网点名称读口检索不到，
// 按它们找留一个只筛本页的小框，字面写明。读口另有的筛选维（方向、现况、结果层、结论等），页面今天没有对应的筛选条，不为此新加
// 控件。被挂到行上的那几本册（申报记录、海关结果）没有能按这一页收窄的筛选维，读到末页再挂（every-page）；申报准备把就绪与授权
// 两册按批次并排，两册都读到末页、在并好的全部批次上检索，不翻页。

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
      [column.mono ? 'font-mono type-body leading-4' : '', column.multi ? 'whitespace-pre-line' : '', column.nowrap ? 'whitespace-nowrap' : '']
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

// 清关单的号名与单据名取单据编号词典那一处（ADR-0355 决定十一），各页同一个叫法。
const caseWords = documentTypeWords.CUSTOMS_CASE;
// 清关单、报关批次与放行检查的对象都是声明包裹，列上与检索里叫包裹号；「运单号」专指委托的单据号，不用在这里。
const parcelNumberName = objectNouns.parcelNumber;
const caseInternalIdLabel = `${caseWords.name}内部标识`;

// 清关单号一列是单据号；没有单据号的清关单照旧显案件标识，技术详情按它记下的情形说为什么没有号。
const caseColumns = recordColumns(
  [
    { id: 'number', header: caseWords.numberName, mono: true, nowrap: true },
    { id: 'parcels', header: parcelNumberName, mono: true, multi: true },
    { id: 'jurisdiction', header: '辖区', mono: true, nowrap: true },
    { id: 'direction', header: '程序', nowrap: true },
    { id: 'progress', header: '最新进展', pillCode: 'progress' },
    { id: 'updatedAt', header: '最后更新', mono: true, nowrap: true },
  ],
  [
    ['内部标识', 'id'],
    ['单据号', 'numberNote'],
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
    { id: 'members', header: parcelNumberName, mono: true, multi: true },
    { id: 'jurisdiction', header: '辖区', mono: true, nowrap: true },
    { id: 'direction', header: '程序', nowrap: true },
    { id: 'formedAt', header: '形成时间', mono: true, nowrap: true },
  ],
  [
    [caseWords.numberName, 'caseRef'],
    [caseInternalIdLabel, 'caseId'],
    ['单据号', 'caseNumberNote'],
    ['监管程序', 'procedure'],
    ['替代批次', 'replaces'],
  ],
);

const snapshotColumns = recordColumns(
  [
    { id: 'unit', header: '批次号', mono: true, nowrap: true },
    { id: 'members', header: parcelNumberName, mono: true, multi: true },
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
    { id: 'members', header: parcelNumberName, mono: true, multi: true },
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
    { id: 'object', header: parcelNumberName, mono: true, nowrap: true },
    { id: 'intent', header: '动作', nowrap: true },
    { id: 'route', header: '网点 → 去向', nowrap: true },
    { id: 'crossZone', header: '跨关区', nowrap: true },
    { id: 'answer', header: '结论', pillCode: 'answerCode' },
    { id: 'findings', header: '逐项检查', multi: true },
    { id: 'actionAt', header: '动作时点', mono: true, nowrap: true },
  ],
  [
    ['判断标识', 'id'],
    ['网点编码', 'routeCodes'],
    ['判断时间', 'judgedAt'],
  ],
);

/** 一本伴随册读到末页、取一次，交回答复与重取；被挂到行上的册与申报准备的两册用它。load 要是稳定引用（模块级函数）。 */
function useEveryPageAnswer<Row>(load: () => Promise<ApiResult<Row[]>>) {
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{ reloadKey: number; answer: ApiResult<Row[]> } | null>(null);
  useEffect(() => {
    let cancelled = false;
    void load().then((answer) => {
      if (!cancelled) setLoaded({ reloadKey, answer });
    });
    return () => {
      cancelled = true;
    };
  }, [load, reloadKey]);
  // 重取期间不留旧答复：按了「重试」，人要分得出是没按到还是还没回。
  return { answer: loaded?.reloadKey === reloadKey ? loaded.answer : null, retry: () => setReloadKey((value) => value + 1) };
}

const allSubmissions = () => everyPage((after) => listSubmissionVersions({}, after), (body) => body.submissions);
const allResults = () => everyPage((after) => listExternalResults({}, after), (body) => body.results);

/** 几问合成一个答案：任一问不成按那一问的状态呈现——只拿到半份数据就开画，会把另一半画成假话。 */
function combined(...answers: Array<ApiResult<unknown> | null>): ApiResult<unknown> | null {
  if (answers.some((answer) => answer === null)) return null;
  return answers.find((answer) => answer?.kind !== 'outcome') ?? answers[0];
}

function outcomeBody<Body>(answer: ApiResult<Body> | null): Body | null {
  return answer?.kind === 'outcome' ? answer.body : null;
}

/** 一签的检索词下推成 `q`，停稳之后才发；换词即换了条件签名，回第一页。 */
function useKeywordConditions(): { search: string; setSearch: (value: string) => void; conditions: CatalogueConditions } {
  const [search, setSearch] = useState('');
  return { search, setSearch, conditions: { q: useSettledKeyword(search) } };
}

/** 只有这一页、没有前后页：此时页上零行才说得上册里没有。 */
function onlyPage(pagination: ListCursorPaginationProps | undefined): boolean {
  return pagination === undefined || (pagination.page === 1 && pagination.next === null);
}

interface RecordTableProps {
  title: string;
  description: string;
  search: { value: string; onChange: (value: string) => void; placeholder: string };
  /** 只筛本页的小框筛什么：读口检索不覆盖、页上看得见的那几格。 */
  pageOnly: string;
  columns: ListColumn<RegisterRow>[];
  rows: RegisterRow[];
  /** 册里答回来几条：一条记录可展成几行（或零行）时与 rows 的条数不同，空态只看它。缺省即 rows 的条数。 */
  records?: number;
  answer: ApiResult<unknown> | null;
  retry: () => void;
  endpoint: string;
  emptyTitle: string;
  emptyDescription: string;
  filterSummary?: string;
  /** 下推的条件或页内补充检索之后没剩行时，那一句说的是什么。 */
  noun: string;
  emptyRowsNote?: string;
  pagination: ListCursorPaginationProps | undefined;
  narrowed: boolean;
}

function RecordTable(props: RecordTableProps) {
  const [refine, setRefine] = useState('');
  return (
    <ListPageTemplate<RegisterRow>
      title={props.title}
      description={props.description}
      search={props.search}
      filters={<PageOnlySearch value={refine} onChange={setRefine} subject={props.pageOnly} />}
      filterSummary={props.answer?.kind === 'outcome' ? props.filterSummary : undefined}
      columns={props.columns}
      rows={props.rows.filter((row) => matchesSearch(refine, Object.values(row.values)))}
      rowKey={(row) => row.key}
      emptyRowsNote={props.emptyRowsNote ?? pageEmptyNote(props.noun, props.rows.length)}
      pagination={props.pagination}
      viewState={catalogueViewState(props.answer, props.records ?? props.rows.length, props.retry, {
        module: info,
        endpoint: props.endpoint,
        emptyTitle: props.emptyTitle,
        emptyDescription: props.emptyDescription,
        narrowed: props.narrowed,
      })}
    />
  );
}

function CustomsCasesTable() {
  const { search, setSearch, conditions } = useKeywordConditions();
  const cases = useCatalogueCursor(conditions, listCustomsCases);
  const submissions = useEveryPageAnswer(allSubmissions);
  const results = useEveryPageAnswer(allResults);
  const answer = combined(cases.answer, submissions.answer, results.answer);
  const records = outcomeBody(cases.answer)?.cases ?? [];
  const rows =
    answer?.kind === 'outcome' ? caseRows(records, outcomeBody(submissions.answer) ?? [], outcomeBody(results.answer) ?? []) : [];
  const pending = rows.filter((row) => row.values.progress === 'CUSTOMS_PENDING_DECLARATION' || row.values.progress === 'DECLARATION_PENDING_SEND').length;
  return (
    <RecordTable
      title="清关单"
      description="每票出口或进口货物的清关单：走到哪一步、最近一次更新是什么时候"
      search={{ value: search, onChange: setSearch, placeholder: `搜${parcelNumberName}、辖区、${caseWords.numberName}、监管程序` }}
      pageOnly="程序、最新进展"
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
      filterSummary={`本页 ${rows.length} 张 · 待申报或待发送 ${pending} 张 · 已关案 ${records.filter((c) => c.closure).length} 张`}
      noun="清关单"
      pagination={cases.pagination}
      narrowed={catalogueConditionsNarrow(conditions) || pastFirstPage(cases.pagination)}
    />
  );
}

function DeclarationUnitsTable() {
  const { search, setSearch, conditions } = useKeywordConditions();
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listCustomsCases);
  const cases = outcomeBody(answer)?.cases ?? [];
  const rows = unitRows(cases);
  return (
    <RecordTable
      title="报关批次"
      description="一次向海关申报的一批运单；资料和申报记录都挂在批次上"
      // 读口翻的是清关单，批次嵌在清关单里：检索命中的是清关单，命中了就列出它的全部批次。
      search={{ value: search, onChange: setSearch, placeholder: `搜${parcelNumberName}、${caseWords.numberName}、辖区` }}
      pageOnly="批次号、程序"
      columns={unitColumns}
      rows={rows}
      // 一页清关单可能都还没组成批次、后面几页却有：页上零批次只在只有这一页时才配空态，不然翻页条跟着空态一起收掉了。
      records={onlyPage(pagination) ? rows.length : cases.length}
      answer={answer}
      retry={retry}
      endpoint="GET /customs-case-records"
      emptyTitle="还没有报关批次"
      emptyDescription="清关单下还没有组成报关批次；批次组成之后才能固定资料、向海关申报。"
      filterSummary={`本页 ${cases.length} 张清关单 · ${rows.length} 个批次`}
      noun="报关批次"
      emptyRowsNote={rows.length === 0 && cases.length > 0 ? '这一页的清关单还没有组成报关批次，翻页看看' : undefined}
      pagination={pagination}
      narrowed={catalogueConditionsNarrow(conditions) || pastFirstPage(pagination)}
    />
  );
}

function DeclarationSnapshotsTable() {
  const { search, setSearch, conditions } = useKeywordConditions();
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listSubmissionVersions);
  const submissions = outcomeBody(answer)?.submissions ?? [];
  return (
    <RecordTable
      title="申报资料"
      description="每次申报发出前固定下来的那份资料；发出后不再改，改了就是下一次申报"
      search={{ value: search, onChange: setSearch, placeholder: `搜批次号、${parcelNumberName}、申报版本` }}
      pageOnly="现况"
      columns={snapshotColumns}
      rows={snapshotRows(submissions)}
      answer={answer}
      retry={retry}
      endpoint="GET /customs-submission-records"
      emptyTitle="还没有固定过申报资料"
      emptyDescription="还没有发起过申报，所以没有固定的资料；页面不会预置。"
      filterSummary={`本页 ${submissions.length} 份`}
      noun="申报资料"
      pagination={pagination}
      narrowed={catalogueConditionsNarrow(conditions) || pastFirstPage(pagination)}
    />
  );
}

function SubmissionVersionsTable() {
  const { search, setSearch, conditions } = useKeywordConditions();
  const submissionsCursor = useCatalogueCursor(conditions, listSubmissionVersions);
  const resultsAnswer = useEveryPageAnswer(allResults);
  const submissions: SubmissionVersionBody[] = outcomeBody(submissionsCursor.answer)?.submissions ?? [];
  const results: ExternalResultBody[] = outcomeBody(resultsAnswer.answer) ?? [];
  const answer = combined(submissionsCursor.answer, resultsAnswer.answer);
  return (
    <RecordTable
      title="申报记录"
      description="每次向海关申报的发送情况与海关的接收、受理结果；各层分开记，发出去不等于海关收到"
      search={{ value: search, onChange: setSearch, placeholder: `搜批次号、${parcelNumberName}、申报版本` }}
      pageOnly="发送情况、海关接收与受理、现况"
      columns={submissionColumns}
      rows={answer?.kind === 'outcome' ? submissionRows(submissions, results) : []}
      answer={answer}
      retry={() => {
        submissionsCursor.retry();
        resultsAnswer.retry();
      }}
      endpoint="GET /customs-submission-records 与 /customs-external-result-records"
      emptyTitle="还没有申报记录"
      emptyDescription="还没有向海关申报过；申报发出之后才有发送情况、技术回执与海关结果。"
      filterSummary={`本页 ${submissions.length} 次申报`}
      noun="申报记录"
      pagination={submissionsCursor.pagination}
      narrowed={catalogueConditionsNarrow(conditions) || pastFirstPage(submissionsCursor.pagination)}
    />
  );
}

function ExternalResultsTable() {
  const { search, setSearch, conditions } = useKeywordConditions();
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listExternalResults);
  const results = outcomeBody(answer)?.results ?? [];
  return (
    <RecordTable
      title="海关结果与放行"
      description="海关和报关行传回来的每一条结果：接收、受理、核定税费、放行；归属不上的只留原文，不猜"
      search={{ value: search, onChange: setSearch, placeholder: '搜来源标识、申报版本' }}
      pageOnly="批次号、结果层、海关怎么说"
      columns={resultColumns}
      rows={externalResultRows(results)}
      answer={answer}
      retry={retry}
      endpoint="GET /customs-external-result-records"
      emptyTitle="还没有收到海关结果"
      emptyDescription="接收、受理、放行等结果由报关行或海关接口送入；还没有收到不等于被拒绝。"
      filterSummary={`本页 ${results.length} 条 · 其中放行 ${results.filter((r) => r.release).length} 条`}
      noun="海关结果"
      pagination={pagination}
      narrowed={catalogueConditionsNarrow(conditions) || pastFirstPage(pagination)}
    />
  );
}

// 放行记录按包裹号查（判断的对象是声明包裹，筛选维 object，逐字比）：不填则列最近的记录。记录是判断当时的结论，换了规则之后同一请求的
// 新判断是另一行，旧行不改。
// 「只看卡关」换一口读：每件运单只看它最近那一刻的放行检查，有一份没过门的（阻断、未决、未配置）才列，件数是读口答的整个队列的
// 件数，不随翻页与检索变——在判断册这一页里数「已拦截」只数得到这一页，也会把后来已放行的运单算进去。首页「今日待办」的卡关格点进来
// 就落在这里。两口各一本游标：切换即换了读口，各自从第一页起。
function AdmissionJudgementsTable({
  unadmittedOnly,
  onUnadmittedOnlyChange,
}: {
  unadmittedOnly: boolean;
  onUnadmittedOnlyChange: (next: boolean) => void;
}) {
  const [draft, setDraft] = useState('');
  const [object, setObject] = useState('');
  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-2 shrink-0">
        <label className="flex items-center gap-1.5 type-body leading-5">
          <input type="checkbox" checked={unadmittedOnly} onChange={(event) => onUnadmittedOnlyChange(event.target.checked)} />
          只看卡关
        </label>
        {unadmittedOnly ? (
          <span className="type-body leading-4 text-idpxyz-textMuted">每件运单只看最近那一刻的放行检查，有一项没过的（被拦、待定、缺配置）就列，卡得最久的在前</span>
        ) : (
          <form
            className="flex items-center gap-2"
            onSubmit={(event) => {
              event.preventDefault();
              setObject(draft.trim());
            }}
          >
            <label className="type-body leading-5" htmlFor="admission-object">
              按{parcelNumberName}查
            </label>
            <input
              id="admission-object"
              className="border rounded px-2 py-1 type-body leading-5 font-mono"
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              placeholder={`${parcelNumberName}；留空列出最近的记录`}
            />
            <button type="submit" className="border rounded px-3 py-1 type-body leading-5">
              查询
            </button>
          </form>
        )}
      </div>
      {unadmittedOnly ? <UnadmittedObjectsList /> : <JudgementsList object={object} />}
    </div>
  );
}

const judgementKeywordHint = `搜${parcelNumberName}、网点编码、判断标识`;
const judgementPageOnly = '网点名称、动作、结论';

function UnadmittedObjectsList() {
  const { search, setSearch, conditions } = useKeywordConditions();
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listUnadmittedObjects);
  const body = outcomeBody(answer);
  const judgements = body?.judgements ?? [];
  // 网点一格按此刻生效那一版显名称，原编码进技术详情（record-rows 的 judgementRows 收查名）。
  const places = usePlaceNames();
  return (
    <RecordTable
      title="卡关的运单"
      description="最近一次放行检查没过、还卡在关务上的运单：被拦的等限制解除或放行，待定的等补齐，缺配置的等配好；放行之后要再过一次检查才会离开这张表"
      search={{ value: search, onChange: setSearch, placeholder: judgementKeywordHint }}
      pageOnly={judgementPageOnly}
      columns={judgementColumns}
      rows={judgementRows(judgements, (node) => placeLabel(places, node))}
      answer={answer}
      retry={retry}
      endpoint="GET /customs-unadmitted-objects"
      emptyTitle="没有卡关的运单"
      emptyDescription="每件问过放行检查的运单，最近一次都过了或本就不受关务门禁。"
      filterSummary={body ? `共 ${body.unadmittedCount} 件卡关 · 本页 ${judgements.length} 件` : undefined}
      noun="卡关的运单"
      pagination={pagination}
      narrowed={catalogueConditionsNarrow(conditions) || pastFirstPage(pagination)}
    />
  );
}

function JudgementsList({ object }: { object: string }) {
  const [search, setSearch] = useState('');
  const conditions: CatalogueConditions = { q: useSettledKeyword(search), filters: { object: object === '' ? [] : [object] } };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listAdmissionJudgements);
  const judgements = outcomeBody(answer)?.judgements ?? [];
  const places = usePlaceNames();
  const blocked = judgements.filter((judgement) => judgement.answer === 'BLOCKED').length;
  return (
    <RecordTable
      title="放行记录"
      description="货物出库、装车、跨关区、交付前每一次放行检查的结论；按当时的规则判，不回头改"
      search={{ value: search, onChange: setSearch, placeholder: judgementKeywordHint }}
      pageOnly={judgementPageOnly}
      columns={judgementColumns}
      rows={judgementRows(judgements, (node) => placeLabel(places, node))}
      answer={answer}
      retry={retry}
      endpoint={`GET /customs-action-admission-judgements${object ? `?object=${object}` : ''}`}
      emptyTitle="还没有放行记录"
      emptyDescription="只有出库、装车、交付时问过放行检查才有记录；没有记录不等于被拦，也不等于可放行。"
      filterSummary={`本页 ${judgements.length} 条 · 已拦截 ${blocked} 条`}
      noun="放行记录"
      emptyRowsNote={
        judgements.length === 0 && object !== ''
          ? `没有${parcelNumberName}「${object}」的放行记录；没有记录不等于被拦，也不等于可放行`
          : undefined
      }
      pagination={pagination}
      narrowed={catalogueConditionsNarrow(conditions) || pastFirstPage(pagination)}
    />
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
    className: [mono ? 'font-mono type-body leading-4' : '', nowrap ? 'whitespace-nowrap' : ''].filter(Boolean).join(' ') || undefined,
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

const allReadiness = () =>
  everyPage((after) => listCaseRegisters(registryConditions('readiness'), after), (body) => body.judgments);
const allAuthorities = () =>
  everyPage((after) => listCaseRegisters(registryConditions('submission-authority'), after), (body) => body.authorities);

// 两册按批次并排，各读到末页再并：授权册没有筛选维，按就绪册的一页去挂授权，页外批次的授权会被报成「未登记」；两册各翻各的页又
// 对不上同一批批次。检索因此在并好的全部批次上做，不下推——下推到两册各筛一遍，一个批次只在一册里命中时另一栏同样会被报成「未登记」。
function SubmissionPreconditionsTable() {
  const [search, setSearch] = useState('');
  const readiness = useEveryPageAnswer(allReadiness);
  const authority = useEveryPageAnswer(allAuthorities);
  const answer = combined(readiness.answer, authority.answer);
  const judgments = outcomeBody(readiness.answer);
  const authorities = outcomeBody(authority.answer);
  const listed = judgments !== null && authorities !== null ? { judgments, authorities } : null;
  const rows = listed ? preconditionRows(listed.judgments, listed.authorities) : [];
  const visibleRows = rows.filter((row) => matchesSearch(search, Object.values(row.values)));
  const retry = () => {
    readiness.retry();
    authority.retry();
  };

  return (
    <ListPageTemplate<CatalogueRow>
      title="申报准备"
      description="每个报关批次申报前的两道准备：资料是否就绪、是否授权提交；两道各自成立、各自撤销"
      search={{ value: search, onChange: setSearch, placeholder: '搜批次号、依据' }}
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
  col('caseRef', caseWords.numberName, true, true),
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
          { label: caseInternalIdLabel, value: row.values.caseId },
          { label: '单据号', value: row.values.caseNumberNote },
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

/** numberOf 把关闭核对引到的案件标识换成清关单号（见 caseNumberReader）。 */
function closureRows(catalogues: ClosureObligationCatalogueRecord[], numberOf: (caseRef: string) => CaseNumberCell): CatalogueRow[] {
  return catalogues.flatMap((catalogue) => {
    const cell = numberOf(catalogue.case);
    const base = {
      caseRef: cell.number,
      caseId: catalogue.case,
      caseNumberNote: cell.note,
      registeredAt: formatInstant(catalogue.registeredAt),
    };
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

/**
 * 这一页关闭核对引到的清关单，按案件标识读回来（findCustomsCasesById）：还在途是 undefined，读不到是 null，空页不发请求。
 * reloadKey 随这一签的「重试」变，读不到的那一次也跟着重问。
 */
function useCasesOnPage(caseIds: readonly string[], reloadKey: number): CustomsCaseBody[] | null | undefined {
  const key = JSON.stringify([caseIds, reloadKey]);
  const [loaded, setLoaded] = useState<{ key: string; cases: CustomsCaseBody[] | null } | null>(null);
  useEffect(() => {
    const [ids] = JSON.parse(key) as [string[], number];
    if (ids.length === 0) return undefined;
    let cancelled = false;
    void findCustomsCasesById(ids).then((answer) => {
      if (!cancelled) setLoaded({ key, cases: answer.kind === 'outcome' ? answer.body : null });
    });
    return () => {
      cancelled = true;
    };
  }, [key]);
  if (caseIds.length === 0) return [];
  return loaded?.key === key ? loaded.cases : undefined;
}

const loadClosureObligations = (conditions: RegistryConditions<'closure-obligation'>, after: string | null) =>
  listCaseRegisters(conditions, after);

// 关闭核对引到的清关单按清关单号显（ADR-0355 决定十一），号按这一页引到的案件标识去清关单读口读回来。视图状态只由关闭目录读口定：
// 清关单读口没答只让号那一格降级（显案件标识并注明读不到），不拖垮这一签——核对事项本身照样读得到。检索下推成 `q`，读口只覆盖
// 案件标识；清关单号是另一本册上的格，与事项、处理情况一起留给只筛本页的小框。
function ClosureObligationsTable() {
  const [search, setSearch] = useState('');
  const [refine, setRefine] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const conditions = registryConditions('closure-obligation', { q: useSettledKeyword(search) });
  const cursor = useCatalogueCursor(conditions, loadClosureObligations);
  const catalogues = outcomeBody(cursor.answer)?.catalogues ?? [];
  const cases = useCasesOnPage([...new Set(catalogues.map((catalogue) => catalogue.case))].sort(), reloadKey);
  // 号还在途时整签照加载中：先画出来再换号，号那一格会从内部标识跳成单据号。
  const answer = cursor.answer?.kind === 'outcome' && cases === undefined ? null : cursor.answer;
  const rows = closureRows(catalogues, caseNumberReader(cases ?? null));
  const unresolved = catalogues.reduce((sum, catalogue) => sum + catalogue.items.filter((item) => item.state === 'UNRESOLVED').length, 0);
  const retry = () => {
    cursor.retry();
    setReloadKey((value) => value + 1);
  };

  return (
    <ListPageTemplate<CatalogueRow>
      title="关闭核对"
      description="清关单关案前要逐项核对的事项：哪些已了结、哪些交给了别人、哪些还没解决"
      search={{ value: search, onChange: setSearch, placeholder: `搜${caseInternalIdLabel}` }}
      filters={<PageOnlySearch value={refine} onChange={setRefine} subject={`${caseWords.numberName}、事项、处理情况`} />}
      filterSummary={answer?.kind === 'outcome' ? `本页 ${catalogues.length} 张清关单 · 未解决 ${unresolved} 项` : undefined}
      columns={closureColumns}
      rows={rows.filter((row) => matchesSearch(refine, Object.values(row.values)))}
      rowKey={(row) => row.key}
      emptyRowsNote={pageEmptyNote('事项', rows.length)}
      pagination={cursor.pagination}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /customs-case-registers?registry=closure-obligation 与 /customs-case-records',
        emptyTitle: '还没有关闭核对清单',
        emptyDescription: '清关单登记了关闭核对清单后在这里出现；没登记清单的清关单关不了案。',
        narrowed: catalogueConditionsNarrow(conditions) || pastFirstPage(cursor.pagination),
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
  const conditions = { q: useSettledKeyword(search) };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listCredentials);
  const credentials = outcomeBody(answer)?.credentials ?? [];
  const rows = credentialRows(credentials);

  return (
    <ListPageTemplate<RegisterRow>
      title="监管凭证"
      description="报关要用到的许可、备案等凭证；一张凭证一条，到期或换额度另登一张"
      // 读口检索覆盖凭证号、签发机构、持有人与程序，页上要找的都在里面，不另摆只筛本页的框。
      search={{ value: search, onChange: setSearch, placeholder: '搜凭证号、持有人、签发机构、程序' }}
      filterSummary={answer?.kind === 'outcome' ? `本页 ${credentials.length} 张` : undefined}
      columns={credentialColumns}
      rows={rows}
      rowKey={(row) => row.key}
      emptyRowsNote="没有匹配的凭证"
      pagination={pagination}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /customs-credentials',
        emptyTitle: '还没有登记监管凭证',
        emptyDescription: '凭证没登记时，申报准备会停在「凭证未登记」，不会当作不需要。请在「登记监管凭证」里照原件登记。',
        narrowed: catalogueConditionsNarrow(conditions) || pastFirstPage(pagination),
      })}
    />
  );
}

/**
 * 清关单页。签按一票货清关的顺序排：清关单 → 报关批次 → 申报资料 → 申报记录 → 海关结果与放行 → 放行记录 → 关闭核对，
 * 之后是申报前的两道准备（申报准备、监管凭证）与凭证的登记签。本页只有凭证一本册有在线登记口：就绪、授权与关闭核对改的
 * 是案上此刻的事实，不进写面。
 */
/** 签与「只看卡关」的地址同步：读自 hash、写回 hash（replaceState，不压后退记录、不触发壳层的 hashchange），做法同 useAddressKeyword。 */
function useCasesPlace(): [CasesTab, boolean, (tab: CasesTab, unadmittedOnly: boolean) => void] {
  const [tab, setTab] = useState<CasesTab>(() => casesTabOf(window.location.hash));
  const [unadmittedOnly, setUnadmittedOnly] = useState(() => unadmittedOnlyOf(window.location.hash));

  // 本签已开着时从首页点卡关格进来，地址变了而页不重挂载，要跟着地址走。
  useOwnAddressChange(() => {
    setTab(casesTabOf(window.location.hash));
    setUnadmittedOnly(unadmittedOnlyOf(window.location.hash));
  });

  const place = (nextTab: CasesTab, nextUnadmittedOnly: boolean) => {
    setTab(nextTab);
    setUnadmittedOnly(nextUnadmittedOnly);
    const hash = window.location.hash;
    if (!hash.startsWith('#/')) return;
    window.history.replaceState(window.history.state, '', casesHash(hash, { tab: nextTab, unadmittedOnly: nextUnadmittedOnly }));
  };
  return [tab, unadmittedOnly, place];
}

export function CustomsCasesPage() {
  const [tab, unadmittedOnly, place] = useCasesPlace();
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs
        value={tab}
        onValueChange={(value) => {
          if ((casesTabs as readonly string[]).includes(value)) place(value as CasesTab, unadmittedOnly);
        }}
        className="flex-1 flex flex-col overflow-hidden gap-0"
      >
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
          <AdmissionJudgementsTable unadmittedOnly={unadmittedOnly} onUnadmittedOnlyChange={(next) => place('admission', next)} />
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
