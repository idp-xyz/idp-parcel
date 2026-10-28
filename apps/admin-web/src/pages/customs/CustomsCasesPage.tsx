import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { RegistrationPanel } from '../../components/registration';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant, formatRange } from '../catalogue-view';
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
import {
  caseRegisterLabels,
  labelOf,
  obligationStateLabels,
  problemNote,
  registrationSnapshotHints,
  registrationTitles,
} from './presentation';
import { credentialRows, type RegisterRow } from './register-rows';
import {
  caseRows,
  externalResultRows,
  judgementRows,
  snapshotRows,
  submissionRows,
  unitRows,
} from './record-rows';
import { shortenEmbeddedPrincipalRefs } from '../../components/principal-ref';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['customs-cases'];

// 本页两类页签（票 admin-web-page-wiring-frontier/05）：
//
// 已接线的是案件配置册查阅两签——「就绪与授权」（单元维，两栏各带撤销态）与
// 「关闭义务」（案件维，目录连义务项），都接 GET /customs-case-registers 按
// registry 分派；以及监管凭证册一签（票 sa-cc/10），接 GET /customs-credentials
// ——凭证是 UC-CC-003 就绪门禁第 4 道的依据，所以挂在就绪与授权签旁；凭证的登记签
// （票 sa-cc/07 步二，接手工口 POST /customs-regulatory-credential-manual-registrations，ADR-0236）跟着读签
// 落在本页——写签跟着读签走。本页只有凭证一本册有在线登记口：就绪、授权与关闭义务
// 改的是案上此刻的事实，按票 admin-write-faces/02 的范围裁定不进写面。
//
// 四个对象族（关务案件、申报单元、正式申报资料快照、提交版本）分页签呈现：
// 案件是稳定业务容器，单元是申报对象集合，快照是版本化资料，提交版本是
// 不可覆盖的对外快照——各自的版本与替代关系独立，折进一张表就会让
// 「案件状态」冒充其余三层的进度。四族与外部结果、动作准入判断册现已接线（票 demo-2026-10-30/01）。

// —— 对象族四签与判断册签（接真面，票 demo-2026-10-30/01）——
//
// 四口各立入口、不并成一个状态：案件与申报单元同源（案件读回带单元），资料快照与提交版本同源
// （提交版本读回带资料引用与尝试），外部结果（监管接收、业务受理、放行）另一口——提交版本行把它按
// 归属的版本挂回去，各层各占一列，缺层写「尚无结果」，不由别的层推出来。

type RecordRowShape = { key: string; values: Readonly<Record<string, string>> };

function recordColumns(spec: [id: string, header: string, mono?: boolean, multiline?: boolean][]): ListColumn<RecordRowShape>[] {
  return spec.map(([id, header, mono, multiline]) => ({
    id,
    header,
    className: [mono ? 'font-mono text-xs' : '', multiline ? 'whitespace-pre-line' : ''].filter(Boolean).join(' ') || undefined,
    render: (row) => row.values[id] ?? '—',
  }));
}

const caseColumns = recordColumns([
  ['id', '案件标识', true],
  ['jurisdiction', '监管辖区'],
  ['direction', '进出口方向'],
  ['procedure', '监管程序', true],
  ['obligation', '法定义务范围'],
  ['parcels', '关联包裹', true],
  ['units', '申报单元', true],
  ['closure', '关案记录'],
  ['establishedAt', '建立时间', true],
]);

const unitColumns = recordColumns([
  ['id', '申报单元标识', true],
  ['caseRef', '所属关务案件', true],
  ['procedure', '监管程序', true],
  ['members', '对象范围（包裹）', true],
  ['formedAt', '形成时间', true],
  ['replaces', '替代单元', true],
]);

const snapshotColumns = recordColumns([
  ['version', '固定于提交版本', true],
  ['unit', '所属申报单元', true],
  ['dossier', '资料引用', true],
  ['roles', '角色快照引用', true],
  ['digest', '内容摘要', true],
  ['members', '对象范围（包裹）', true],
  ['current', '版本现况'],
  ['fixedAt', '固定时间', true],
]);

const submissionColumns = recordColumns([
  ['version', '提交版本', true],
  ['unit', '申报单元', true],
  ['authority', '授权依据', true],
  ['current', '版本现况'],
  ['attempts', '提交尝试与发送事实', false, true],
  ['receipts', '技术回执', false, true],
  ['received', '监管接收'],
  ['accepted', '业务受理'],
  ['fixedAt', '固定时间', true],
]);

const resultColumns = recordColumns([
  ['source', '来源标识', true],
  ['layer', '结果层'],
  ['version', '归属提交版本', true],
  ['semantics', '来源原始语义'],
  ['release', '放行事实'],
  ['flags', '冲突与更正'],
  ['occurredAt', '业务发生时间', true],
  ['recordedAt', '接收时间', true],
]);

const judgementColumns = recordColumns([
  ['object', '对象', true],
  ['intent', '动作意图'],
  ['route', '节点 → 去向', true],
  ['crossZone', '跨区域'],
  ['answer', '整次答复'],
  ['findings', '逐动作答复与依据', false, true],
  ['actionAt', '动作时点', true],
  ['judgedAt', '判断时间', true],
]);

/** 取一次数据并交回行；四个读签共用，页面只管列与文案。 */
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

function filterRows<Row extends RecordRowShape>(rows: Row[], search: string): Row[] {
  const needle = search.trim().toLowerCase();
  return needle
    ? rows.filter((row) => Object.values(row.values).some((value) => value.toLowerCase().includes(needle)))
    : rows;
}

interface RecordTableProps {
  title: string;
  description: string;
  searchPlaceholder: string;
  columns: ListColumn<RecordRowShape>[];
  rows: RecordRowShape[];
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
    <ListPageTemplate<RecordRowShape>
      title={props.title}
      description={props.description}
      search={{ value: search, onChange: setSearch, placeholder: props.searchPlaceholder }}
      filterSummary={props.answer?.kind === 'outcome' ? props.filterSummary : undefined}
      columns={props.columns}
      rows={filterRows(props.rows, search)}
      rowKey={(row) => row.key}
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
  const { answer, retry } = useRecordAnswer(listCustomsCases);
  const cases = answer?.kind === 'outcome' ? answer.body.cases : [];
  return (
    <RecordTable
      title="关务案件"
      description={`${info.owner}——出口、进口及其他独立监管程序分别建立案件；关案记录缺席即未关闭`}
      searchPlaceholder="搜索案件标识 / 监管辖区 / 包裹"
      columns={caseColumns}
      rows={caseRows(cases)}
      answer={answer}
      retry={retry}
      endpoint="GET /customs-case-records"
      emptyTitle="当前租户尚无关务案件"
      emptyDescription="读取入口已配置，但还没有立过案；案件由包裹建立或经建案口建立，页面不会预置。"
      filterSummary={`案件 ${cases.length} 个 · 已关闭 ${cases.filter((c) => c.closure).length} 个`}
    />
  );
}

function DeclarationUnitsTable() {
  const { answer, retry } = useRecordAnswer(listCustomsCases);
  const cases = answer?.kind === 'outcome' ? answer.body.cases : [];
  const rows = unitRows(cases);
  return (
    <RecordTable
      title="申报单元"
      description={`${info.owner}——申报单元不是客户委托、集运单元、总单、舱单或运输班次；随案件读回`}
      searchPlaceholder="搜索申报单元 / 所属案件 / 包裹"
      columns={unitColumns}
      rows={rows}
      answer={answer}
      retry={retry}
      endpoint="GET /customs-case-records"
      emptyTitle="当前租户尚无申报单元"
      emptyDescription="案件下还没有形成申报单元；单元形成之后才能固定资料、提交。"
      filterSummary={`申报单元 ${rows.length} 个`}
    />
  );
}

function DeclarationSnapshotsTable() {
  const { answer, retry } = useRecordAnswer(listSubmissionVersions);
  const submissions = answer?.kind === 'outcome' ? answer.body.submissions : [];
  return (
    <RecordTable
      title="正式申报资料快照"
      description={`${info.owner}——每个提交版本在对外发送前固定的那份资料；字段级来源溯源不在本读面，需另查`}
      searchPlaceholder="搜索提交版本 / 申报单元 / 资料引用"
      columns={snapshotColumns}
      rows={snapshotRows(submissions)}
      answer={answer}
      retry={retry}
      endpoint="GET /customs-submission-records"
      emptyTitle="当前租户尚未固定过申报资料"
      emptyDescription="还没有提交版本，所以没有固定的资料快照；页面不会预置。"
      filterSummary={`资料快照 ${submissions.length} 份`}
    />
  );
}

function SubmissionVersionsTable() {
  const submissionsAnswer = useRecordAnswer(listSubmissionVersions);
  const resultsAnswer = useRecordAnswer(listExternalResults);
  const submissions = submissionsAnswer.answer?.kind === 'outcome' ? submissionsAnswer.answer.body.submissions : [];
  const results = resultsAnswer.answer?.kind === 'outcome' ? resultsAnswer.answer.body.results : [];
  // 版本一问是主问；结果一问未成时仍列版本，层列写「尚无结果」会说谎，所以结果不成就按那一问的状态呈现。
  const answer: ApiResult<unknown> | null =
    submissionsAnswer.answer === null || resultsAnswer.answer === null
      ? null
      : submissionsAnswer.answer.kind !== 'outcome'
        ? submissionsAnswer.answer
        : resultsAnswer.answer;
  return (
    <RecordTable
      title="提交版本"
      description={`${info.owner}——提交版本是不可覆盖快照，其存在不证明技术传输、监管接收、业务受理或放行成功；各层各占一列`}
      searchPlaceholder="搜索提交版本 / 申报单元 / 授权依据"
      columns={submissionColumns}
      rows={answer?.kind === 'outcome' ? submissionRows(submissions, results) : []}
      answer={answer}
      retry={() => {
        submissionsAnswer.retry();
        resultsAnswer.retry();
      }}
      endpoint="GET /customs-submission-records 与 /customs-external-result-records"
      emptyTitle="当前租户尚无提交版本"
      emptyDescription="还没有固定过提交版本；申报提交之后才有提交尝试、技术回执与监管结果。"
      filterSummary={`提交版本 ${submissions.length} 个 · 外部监管结果 ${results.length} 份`}
    />
  );
}

function ExternalResultsTable() {
  const { answer, retry } = useRecordAnswer(listExternalResults);
  const results = answer?.kind === 'outcome' ? answer.body.results : [];
  return (
    <RecordTable
      title="外部监管结果与放行"
      description={`${info.owner}——六层结果分层保存，放行层另带范围与条件明确的放行事实；归属不上的响应只留原始语义，不猜版本`}
      searchPlaceholder="搜索来源标识 / 结果层 / 提交版本"
      columns={resultColumns}
      rows={externalResultRows(results)}
      answer={answer}
      retry={retry}
      endpoint="GET /customs-external-result-records"
      emptyTitle="当前租户尚未收到外部监管结果"
      emptyDescription="监管接收、业务受理、放行等结果由报关服务商经集成客户端送入；还没有收到不等于被拒绝。"
      filterSummary={`外部结果 ${results.length} 份 · 其中放行事实 ${results.filter((r) => r.release).length} 份`}
    />
  );
}

// 判断册按对象查：包裹号或申报单元号都可。不填则列最近的判断。判断册是判断当时的记录，
// 换了登记册之后同一请求的新判断是另一行，旧行不改。
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
  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <form
        className="flex items-center gap-2 px-4 py-2 shrink-0"
        onSubmit={(event) => {
          event.preventDefault();
          setObject(draft.trim());
        }}
      >
        <label className="text-sm" htmlFor="admission-object">按包裹号查</label>
        <input
          id="admission-object"
          className="border rounded px-2 py-1 text-sm font-mono"
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          placeholder="例如包裹号 / 申报单元号；留空列出最近判断"
        />
        <button type="submit" className="border rounded px-3 py-1 text-sm">查询</button>
      </form>
      <RecordTable
        title="动作准入判断册"
        description={`${info.owner}——每行是判断当时的整次答复与逐动作答复及依据；查阅面不重判，也不用此刻的登记册覆盖当时的答案`}
        searchPlaceholder="在结果内搜索对象 / 节点 / 答复"
        columns={judgementColumns}
        rows={judgementRows(judgements)}
        answer={answer}
        retry={() => setReloadKey((value) => value + 1)}
        endpoint={`GET /customs-action-admission-judgements${object ? `?object=${object}` : ''}`}
        emptyTitle={object ? `没有关于「${object}」的准入判断` : '当前租户尚无动作准入判断'}
        emptyDescription="判断册只在有人问过（运输履约登记出发、交出、交付时同步询问）才有行；没有行不等于被拦截，也不等于准入。"
        filterSummary={`判断 ${judgements.length} 条`}
      />
    </div>
  );
}

// —— 案件配置册查阅两签（接真面）——

interface CatalogueRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

function col(id: string, header: string, mono = false): ListColumn<CatalogueRow> {
  return {
    id,
    header,
    className: mono ? 'font-mono text-xs' : undefined,
    render: (row) => row.values[id] ?? '—',
  };
}

// 就绪与授权两栏并排、各带自己的依据/时间/现况三列（CONTEXT「申报就绪判断与提交授权必须独立存在」；票 05 形状约束一）。并排只对齐单元行——不设「可提交」合成
// 列：0006 迁移自注点名「就绪还在、授权已撤销」是必须表达得出的一格，合成布尔正好把
// 它抹掉。
const preconditionColumns: ListColumn<CatalogueRow>[] = [
  col('unit', '申报单元', true),
  col('readinessBasis', '就绪判断依据', true),
  col('readinessAt', '就绪形成时间', true),
  col('readinessState', '就绪现况'),
  col('authorityRef', '提交授权依据', true),
  col('authorityAt', '授权形成时间', true),
  col('authorityState', '授权现况'),
];

// 现况一栏封闭三态，词各有所指：「未登记」＝这本册子没有该单元的行；「仍有效」＝
// 撤销两列缺席；「已撤销」＝失效在场且原依据原样留在左边两列——失效不是删除，也
// 不得谎报成未配置（0006 自注、票 05 形状约束二）。
function trackState(record?: { revokedBy?: string; revokedAt?: string }): string {
  if (!record) return '未登记';
  if (record.revokedBy && record.revokedAt) {
    return `已撤销：${shortenEmbeddedPrincipalRefs(record.revokedBy)}（${formatInstant(record.revokedAt)}）`;
  }
  return '仍有效';
}

function preconditionRows(
  judgments: ReadinessJudgmentRecord[],
  authorities: SubmissionAuthorityRecord[],
): CatalogueRow[] {
  const byUnit = new Map<
    string,
    { judgment?: ReadinessJudgmentRecord; authority?: SubmissionAuthorityRecord }
  >();
  for (const judgment of judgments) {
    byUnit.set(judgment.unit, { ...byUnit.get(judgment.unit), judgment });
  }
  for (const authority of authorities) {
    byUnit.set(authority.unit, { ...byUnit.get(authority.unit), authority });
  }
  return [...byUnit.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([unit, pair]) => ({
      key: `unit:${unit}`,
      values: {
        unit,
        readinessBasis: shortenEmbeddedPrincipalRefs(pair.judgment?.basis ?? '—'),
        readinessAt: pair.judgment ? formatInstant(pair.judgment.judgedAt) : '—',
        readinessState: trackState(pair.judgment),
        authorityRef: shortenEmbeddedPrincipalRefs(pair.authority?.authority ?? '—'),
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
    void Promise.all([
      listCaseRegisters('readiness'),
      listCaseRegisters('submission-authority'),
    ]).then(([readiness, authority]) => {
      if (!cancelled) setLoaded({ readiness, authority });
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  // 两册并排要两问都成业务答案才可判读：任一问不成，按那一问的状态呈现——只拿到
  // 半份数据就开画，会把另一栏整列画成「未登记」的假话。
  const readiness = loaded?.readiness ?? null;
  const authority = loaded?.authority ?? null;
  const answer: ApiResult<unknown> | null =
    readiness === null || authority === null
      ? null
      : readiness.kind !== 'outcome'
        ? readiness
        : authority;
  const listed =
    readiness?.kind === 'outcome' && authority?.kind === 'outcome'
      ? { judgments: readiness.body.judgments, authorities: authority.body.authorities }
      : null;
  const rows = listed ? preconditionRows(listed.judgments, listed.authorities) : [];
  const needle = search.trim().toLowerCase();
  const visibleRows = needle
    ? rows.filter((row) =>
        Object.values(row.values).some((value) => value.toLowerCase().includes(needle)),
      )
    : rows;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<CatalogueRow>
      title="申报就绪判断与提交授权"
      description={`${info.owner}——就绪与授权分别形成、分别失效，两栏各带自己的撤销态；页面不合成「可提交」标记`}
      search={{ value: search, onChange: setSearch, placeholder: '搜索申报单元或依据引用' }}
      filterSummary={
        listed
          ? `${caseRegisterLabels.readiness} ${listed.judgments.length} 条 · ${caseRegisterLabels['submission-authority']} ${listed.authorities.length} 条 · 覆盖申报单元 ${rows.length} 个`
          : undefined
      }
      columns={preconditionColumns}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /customs-case-registers?registry=readiness 与 ?registry=submission-authority',
        emptyTitle: '当前租户尚无就绪判断或提交授权登记',
        emptyDescription: '读取入口已配置,但两本登记册都为空;页面不会预置判断或授权。',
      })}
    />
  );
}

// 目录行与义务项是两条独立信息（0008 自注、票 05 形状约束三）：有义务项的目录逐项
// 成行；目录已登记而清单为空占一行写明——它与「目录未登记 → 未决」含义相反，后者
// 根本不在本列（空册文案另说）。适用区间原样上列不设截点参数，判读归读者（裁决在
// query_case_registers.go 文件头）。
const closureColumns: ListColumn<CatalogueRow>[] = [
  col('caseRef', '关务案件', true),
  col('registeredAt', '目录登记时间', true),
  col('obligation', '义务事项'),
  col('scope', '适用范围'),
  col('state', '处理状态'),
  col('basis', '依据引用', true),
  col('handedTo', '承接方'),
  col('applies', '适用区间', true),
];

function closureRows(catalogues: ClosureObligationCatalogueRecord[]): CatalogueRow[] {
  return catalogues.flatMap((catalogue) => {
    const base = {
      caseRef: catalogue.case,
      registeredAt: formatInstant(catalogue.registeredAt),
    };
    if (catalogue.items.length === 0) {
      return [
        {
          key: `catalogue:${catalogue.case}`,
          values: { ...base, obligation: '目录已登记，当前无义务项' },
        },
      ];
    }
    return catalogue.items.map((item) => ({
      // 行键循库主键 (tenant, case_ref, obligation)：同目录内义务名唯一。
      key: `item:${catalogue.case}:${item.obligation}`,
      values: {
        ...base,
        obligation: item.obligation,
        scope: item.scope,
        state: labelOf(obligationStateLabels, item.state),
        basis: item.basis,
        // 承接方只在承接项在场（CONTEXT「来源责任方、接收责任方、接受决定及权限」）；非承接项由 col 的 '—' 兜底。
        ...(item.handedTo ? { handedTo: item.handedTo } : {}),
        applies: formatRange(item.appliesFrom, item.appliesUntil),
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
  const visibleRows = needle
    ? rows.filter((row) =>
        Object.values(row.values).some((value) => value.toLowerCase().includes(needle)),
      )
    : rows;
  const retry = () => setReloadKey((value) => value + 1);
  const itemCount = catalogues.reduce((sum, catalogue) => sum + catalogue.items.length, 0);

  return (
    <ListPageTemplate<CatalogueRow>
      title="关闭义务目录"
      description={`${info.owner}——目录未登记的案件按未决处理，不在本列；目录已登记而清单为空才是如实的「无义务项」`}
      search={{ value: search, onChange: setSearch, placeholder: '搜索关务案件、义务事项或依据' }}
      filterSummary={
        answer?.kind === 'outcome'
          ? `${caseRegisterLabels['closure-obligation']} ${catalogues.length} 份 · 义务项 ${itemCount} 项`
          : undefined
      }
      columns={closureColumns}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /customs-case-registers?registry=closure-obligation',
        emptyTitle: '当前租户尚无已登记的关闭义务目录',
        emptyDescription:
          '上列只含已登记目录:目录未登记 ≠ 无义务,未登记案件的关闭核对读作未决。页面不会预置目录。',
      })}
    />
  );
}

// —— 监管凭证册（接真面，票 sa-cc/10）——
//
// 一行即一版不可变凭证（0014 自注：一身份一版，换期限或额度是另一张凭证），所以没有
// 「当前版 / 历史版」可折。刻意不设「适用性」列：凭证对某程序、某持有人、某时点适用不适用
// 是判断链（JudgeApplicability）的产物，查阅面转述登记册本身，不在这里替就绪门禁下判。
// 次数额度那格的「来源未提供」与数字分得开（判读在 register-rows.ts），不显成 0。
const credentialColumns: ListColumn<RegisterRow>[] = [
  col('credential', '凭证身份', true),
  col('issuer', '签发机构', true),
  col('holder', '持有人', true),
  col('procedure', '适用监管程序', true),
  col('validity', '有效期', true),
  col('uses', '次数额度'),
  col('registeredAt', '登记时间', true),
  col('registeredBy', '登记人', true),
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
  const visibleRows = needle
    ? rows.filter((row) =>
        Object.values(row.values).some((value) => value.toLowerCase().includes(needle)),
      )
    : rows;
  const retry = () => setReloadKey((value) => value + 1);
  const unprovided = credentials.filter((credential) => credential.uses === undefined).length;

  return (
    <ListPageTemplate<RegisterRow>
      title="监管凭证"
      description={`${info.owner}——一身份一版、不可变；附件文件只是证据，不能代替凭证身份和适用性判断。本签只列在册凭证，不判适用性，也不登余额`}
      search={{ value: search, onChange: setSearch, placeholder: '搜索凭证身份 / 签发机构 / 持有人 / 监管程序' }}
      filterSummary={
        answer?.kind === 'outcome'
          ? `凭证 ${credentials.length} 版 · 其中来源未提供次数额度 ${unprovided} 版`
          : undefined
      }
      columns={credentialColumns}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /customs-credentials',
        emptyTitle: '当前租户尚无已登记的监管凭证',
        emptyDescription:
          '上列只含已登记凭证：凭证未登记时就绪门禁第 4 道停在「凭证未登记」，不读作「不适用」。真实凭证属实例半边（PAR-CUS-04 待提供），页面不会预置任何凭证。',
      })}
    />
  );
}

/**
 * 关务案件与申报（customs-compliance）。已接线的三签在前（就绪与授权、监管凭证、
 * 关闭义务——页面当前能如实作答的查阅面；凭证紧挨就绪与授权，因为它是就绪门禁第 4 道
 * 的依据）；对象族四签（案件容器 → 申报单元 → 资料快照 → 提交版本，逐层向外，越靠后
 * 越接近对外发送）与监管结果、门禁判断册两签排在后。
 * 凭证的登记签排在所有读签之后，读写各占各的签（与 customs-restrictions 页同一处置）。
 *
 * 登记签登的是一版不可变凭证本身，不判它对哪个程序、哪个持有人、哪个时点适用——适用性
 * 是就绪门禁判断链的产物，读签不替它下判，写签同样不替它下判。
 */
export function CustomsCasesPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="preconditions" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="preconditions">就绪与授权</TabsTrigger>
          <TabsTrigger value="credentials">监管凭证</TabsTrigger>
          <TabsTrigger value="closure-obligations">关闭义务</TabsTrigger>
          <TabsTrigger value="cases">关务案件</TabsTrigger>
          <TabsTrigger value="units">申报单元</TabsTrigger>
          <TabsTrigger value="snapshots">资料快照</TabsTrigger>
          <TabsTrigger value="submissions">提交版本</TabsTrigger>
          <TabsTrigger value="results">监管结果与放行</TabsTrigger>
          <TabsTrigger value="admission">门禁判断册</TabsTrigger>
          <TabsTrigger value="register">登记监管凭证</TabsTrigger>
        </TabsList>
        <TabsContent value="preconditions" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <SubmissionPreconditionsTable />
        </TabsContent>
        <TabsContent value="credentials" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <CredentialsTable />
        </TabsContent>
        <TabsContent value="closure-obligations" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ClosureObligationsTable />
        </TabsContent>
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
        <TabsContent value="register" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          {/* 本页只有一本册可在线登记，直接摆 RegistrationPanel，不套选册（判据在 MultiRegistrationPanel
              文件头）。凭证交回配置族那套答案，词表复用 registrationOutcomeLabels。 */}
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
