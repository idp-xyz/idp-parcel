import { useState, type ReactNode } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import type { WorkbenchColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { OperatorRefusalNote } from '../../components/states';
import { RowStatusPill } from '../visibility/ExceptionDetailPanel';
import { RecordDetailPanel, RouteWorkbench, SectionTabs, techRows, type RowStatus } from '../network/route-workbench';
import { CursorPager, pastFirstPage } from '../shipment-request/cursor-pager';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { useCatalogueCursor, useSettledKeyword, type CatalogueCursor } from '../use-catalogue-cursor';
import { useMayMakeDecision } from '../use-operator-grants';
import { effectiveTimeJudgmentDecision, hiddenDecisionHint } from '../operator-grants';
import { labelFor } from '../../vocabulary/status';
import {
  externalTrackingFactConditions,
  judgeEffectiveTime,
  listExternalTrackingFacts,
  type ApiResult,
  type EffectiveTimeJudgmentResponseBody,
  type ExternalTrackingFactConditions,
  type ExternalTrackingFactListResponseBody,
  type ExternalTrackingFactView,
} from './effective-time-judgment-api';
import { allStatuses, effectiveBasisStatus, pushedStatusChips, reviewListConditions } from './list-conditions';
import {
  describeJudgmentAnswer,
  factListViewState,
  factRowsOf,
  judgmentProblemNote,
  type FactRow,
} from './effective-time-judgment';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['effective-time-judgment'];

/**
 * 外部承运轨迹事实的有效时间判断面（票 label-channel/21；ADR-0102 决定三第一种来源）。
 *
 * 读半边按（租户，轨迹源）上列当前版：「待判断」是判断人的「该判哪几条」，「全部当前版」让再判之前
 * 看得见「这一条已经判过、按哪个依据判的」（票 21 红线）。写半边一条事实一次判断，答复四格各有落点。
 *
 * 本页**不解释状态词、不建议一个「推荐的有效时间」**——状态词原词直示，有效时间由判断人自己填；
 * 页面不算不裁，只呈现后端答复。写口今天挂的是字面量 UnconfiguredIntake{}，提交必然答 403
 * 「接入渠道未配置」——机制在，墙也在，页面要说清这不是「尚未实现」。
 *
 * 列表接读口分页（ADR-0144，票 catalogue-read-pagination-rollout/14）：来源与视图是选择器，检索框下推成 `q`（货物、承运商的
 * 说法、轨迹编号、来源事件四列，与原先在结果里搜的那几格相同），确认方式胶囊下推成 effectiveBasis；顺序是读口默认的发生时间。
 */
// 工作台形态。行首是货物，确认方式一列用状态胶囊（待确认琥珀，人工或按规则确认绿）；承运商的说法是源的原始状态
// 词，原样示出不译（ADR-0102 决定五）。轨迹编号、记录版本、源事件、凭据、规则版本与接续关系进右栏「技术详情」。
function factStatus(row: FactRow): RowStatus {
  return { label: row.values.effectiveBasis, tone: row.values.effectiveBasis === '待确认' ? 'warn' : 'good' };
}

const mono = (value: string | undefined) => <span className="font-mono type-body">{value ?? '—'}</span>;

const columns: WorkbenchColumn<FactRow>[] = [
  {
    key: 'object',
    header: '货物',
    render: (row) => <span className="truncate font-mono font-medium text-[color:var(--ds-text-link-hover)]">{row.values.object}</span>,
  },
  { key: 'basis', header: '确认方式', render: (row) => <RowStatusPill status={factStatus(row)} /> },
  { key: 'status', header: '承运商的说法（原词）', render: (row) => mono(row.values.status) },
  { key: 'occurred', header: '承运商说的发生时间', hideWhenDetailOpen: true, render: (row) => mono(row.values.occurredAt) },
  { key: 'effective', header: '从何时起算', hideWhenDetailOpen: true, render: (row) => mono(row.values.effectiveAt) },
];

const viewWords: { id: ExternalTrackingFactView; label: string }[] = [
  { id: 'pending', label: '待确认' },
  { id: 'current', label: '全部轨迹' },
];

/** 判断面板的目标：从行上带来的当前版摘要，让面板在提交前就摆出「这一条判过没有、按哪个依据」。 */
interface JudgmentTarget {
  fact: string;
  version?: string;
  effectiveBasis?: string;
  effectiveAt?: string;
  basisDetail?: string;
}

// useCatalogueCursor 只在填了来源时挂上（读口要求 source，空着发出去必然 400）：取数的这一件不渲染别的，交回游标给页面摆。
function FactCursor({
  conditions,
  children,
}: {
  conditions: ExternalTrackingFactConditions;
  children: (cursor: CatalogueCursor<ExternalTrackingFactListResponseBody>) => ReactNode;
}) {
  return <>{children(useCatalogueCursor(conditions, listExternalTrackingFacts))}</>;
}

export function EffectiveTimeJudgmentPage() {
  const [sourceDraft, setSourceDraft] = useState('');
  const [source, setSource] = useState('');
  const [view, setView] = useState<ExternalTrackingFactView>('pending');
  const [keyword, setKeyword] = useState('');
  // 「待确认」视图里每一条都是待确认，按确认方式筛没有意义：胶囊只在「全部轨迹」视图里摆，切视图即清。
  const [status, setStatus] = useState(allStatuses);
  // 页头「按轨迹编号确认」开的面板：指名一条不在当前列表里的轨迹（换了来源、或列表还没刷到）。行上的确认在右栏里。
  const [judgingByRef, setJudgingByRef] = useState(false);
  const mayJudge = useMayMakeDecision(effectiveTimeJudgmentDecision);
  const pushed = view === 'current' ? effectiveBasisStatus : undefined;
  const settled = useSettledKeyword(keyword);
  const conditions =
    source === '' ? null : externalTrackingFactConditions(source, view, reviewListConditions(settled, pushed, status));

  const render = (cursor: CatalogueCursor<ExternalTrackingFactListResponseBody> | null) => {
    const answer = cursor?.answer ?? null;
    const body = answer?.kind === 'outcome' ? answer.body : null;
    const rows = body ? factRowsOf(body) : [];
    const retry = () => cursor?.retry();
    return (
      <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
        <RouteWorkbench<FactRow>
          key={`${source}#${view}`}
          title={info.title}
          subtitle="承运商回传的轨迹从什么时候起算，由我们确认后才进物流动态。先输入轨迹来源，再逐条确认；系统不替你推测时间。"
          primaryAction={
            mayJudge ? (
              <Button variant="outline" size="sm" onClick={() => setJudgingByRef(true)}>
                按轨迹编号确认
              </Button>
            ) : undefined
          }
          sections={
            <SectionTabs
              sections={viewWords}
              active={view}
              onChange={(next) => {
                setView(next);
                setStatus(allStatuses);
              }}
            />
          }
          extraFilters={
            <span className="inline-flex items-center gap-2">
              <Input
                value={sourceDraft}
                className="h-8 w-[260px] font-mono type-body"
                placeholder="轨迹来源（如承运商的接口编码）"
                aria-label="轨迹来源"
                onChange={(event) => setSourceDraft(event.target.value)}
                onKeyDown={(event) => event.key === 'Enter' && setSource(sourceDraft.trim())}
              />
              <Button variant="outline" size="sm" disabled={sourceDraft.trim() === ''} onClick={() => setSource(sourceDraft.trim())}>
                查询
              </Button>
            </span>
          }
          answered={body !== null}
          rows={rows}
          rowKey={(row) => row.key}
          statusChips={pushed ? pushedStatusChips(pushed, status, setStatus) : undefined}
          onClearFilters={() => setStatus(allStatuses)}
          rowAttention={(row) => row.values.effectiveBasis === '待确认'}
          searchCells={() => []}
          search={{ value: keyword, onChange: setKeyword, pushedDown: true }}
          searchPlaceholder="搜货物、承运商的说法、轨迹编号、来源事件"
          columns={columns}
          renderDetail={(row, close) => <FactDetail key={row.key} row={row} mayJudge={mayJudge} onClose={close} onJudged={retry} />}
          onRefresh={cursor ? cursor.retry : undefined}
          toolbarRight={cursor?.pagination ? <CursorPager pagination={cursor.pagination} /> : undefined}
          viewState={factListViewState(answer, source, view, rows.length, retry, {
            module: info,
            endpoint: `GET /transport-fulfillment-external-tracking-facts?source=…&view=${view}`,
            narrowed:
              conditions !== null && (catalogueConditionsNarrow(conditions) || (cursor !== null && pastFirstPage(cursor.pagination))),
          })}
        />
        {judgingByRef ? (
          <div className="max-h-[50%] shrink-0 overflow-y-auto border-t border-idpxyz-border p-4">
            <JudgmentPanel target={{ fact: '' }} onClose={() => setJudgingByRef(false)} onJudged={retry} />
          </div>
        ) : null}
      </div>
    );
  };

  return conditions === null ? render(null) : <FactCursor conditions={conditions}>{render}</FactCursor>;
}

/** 一条轨迹的右栏：两个时间、确认方式与起算时间；能确认时摆「确认时间」，点开在栏内填。 */
function FactDetail({ row, mayJudge, onClose, onJudged }: { row: FactRow; mayJudge: boolean; onClose: () => void; onJudged: () => void }) {
  const [judging, setJudging] = useState(false);
  return (
    <RecordDetailPanel
      title={row.values.object}
      status={factStatus(row)}
      subtitle={<>承运商说：<span className="font-mono">{row.values.status}</span></>}
      facts={[
        { label: '承运商说的发生时间', value: row.values.occurredAt },
        { label: '我们收到的时间', value: row.values.receivedAt },
        { label: '确认方式', value: row.values.effectiveBasis },
        { label: '从何时起算', value: row.values.effectiveAt },
      ]}
      technical={techRows([
        { label: '轨迹编号', value: row.fact },
        { label: '记录版本', value: row.version },
        { label: '来源事件', value: row.values.sourceEvent === '—' ? '' : row.values.sourceEvent },
        { label: '凭据', value: row.values.credential },
        { label: '规则版本', value: row.values.basisDetail },
        { label: '接续', value: row.values.supersedes },
        { label: '记录来源', value: row.values.origin },
      ])}
      actions={
        mayJudge ? (
          judging ? (
            <JudgmentPanel
              target={{
                fact: row.fact,
                version: row.version,
                effectiveBasis: row.values.effectiveBasis,
                effectiveAt: row.values.effectiveAt,
                basisDetail: row.values.basisDetail,
              }}
              onClose={() => setJudging(false)}
              onJudged={onJudged}
            />
          ) : (
            <Button variant="outline" onClick={() => setJudging(true)}>
              确认时间
            </Button>
          )
        ) : (
          <p className="type-body text-idpxyz-breadcrumb">{hiddenDecisionHint('确认时间', labelFor(effectiveTimeJudgmentDecision))}</p>
        )
      }
      onClose={onClose}
    >
      <p>发生时间是承运商报的，收到时间是我们收到回传的时刻；起算时间由人确认或按规则确认，系统不会默认取其中任何一个。</p>
    </RecordDetailPanel>
  );
}

type PanelState =
  | { kind: 'idle' }
  | { kind: 'submitting' }
  | { kind: 'malformed'; message: string }
  | { kind: 'answered'; answer: ApiResult<EffectiveTimeJudgmentResponseBody> };

/**
 * 一条事实一次判断的表单。两格：事实引用（从行上带来，也可手填——再判一条不在列表里的事实要能指名它）
 * 与有效时间（RFC 3339）。表单上没有「判断人」——那是 ADR-0100 操作者信封的事，从浏览器收一个上去就是
 * 自报身份；也没有任何预填的时间——预填就是「推荐的有效时间」，票 21 红线禁的正是它。
 */
function JudgmentPanel({
  target,
  onClose,
  onJudged,
}: {
  target: JudgmentTarget;
  onClose: () => void;
  onJudged: () => void;
}) {
  const [fact, setFact] = useState(target.fact);
  const [effectiveAt, setEffectiveAt] = useState('');
  const [state, setState] = useState<PanelState>({ kind: 'idle' });

  const ready = fact.trim() !== '' && effectiveAt.trim() !== '';

  const send = () => {
    const instant = effectiveAt.trim();
    // 本地就能判的畸形不送上去：解析不了的时刻送上去回来的 400 会与服务端的业务答案挤在同一格里，
    // 而两者的续办动作不同。这里只判「是不是一个时刻」，不替判断人改它。
    if (Number.isNaN(Date.parse(instant))) {
      setState({ kind: 'malformed', message: `「${instant}」不是能识别的时间，请写成 2026-09-04T06:05:00Z 这样的格式` });
      return;
    }
    setState({ kind: 'submitting' });
    void judgeEffectiveTime({ fact: fact.trim(), effectiveAt: instant }).then((answer) => {
      setState({ kind: 'answered', answer });
      if (answer.kind === 'outcome' && answer.body.outcome === 'EFFECTIVE_TIME_JUDGED') onJudged();
    });
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>
          确认起算时间
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <p className="type-body leading-4 text-idpxyz-textMuted">
          确认后会新记一条，原来的记录照留；确认过的时间会同步到物流动态，客户看到的轨迹随之更新。确认人记为当前登录的你。
          {target.version ? (
            // 再判之前先把「这一条判过没有、按哪个依据」摆出来（票 21 红线）；待确认行如实说待确认。
            <>
              <br />
              这一条现在：{target.effectiveBasis}
              {target.effectiveAt && target.effectiveAt !== '—' ? `，起算时间 ${target.effectiveAt}` : ''}
              {target.basisDetail ? `，规则 ${target.basisDetail}` : ''}。
            </>
          ) : null}
        </p>
        <div className="grid grid-cols-2 gap-3">
          <label className="block">
            <span className="type-body leading-4 text-idpxyz-textMuted">轨迹编号 *</span>
            <Input
              value={fact}
              className="font-mono type-value"
              placeholder="从列表那一行的「技术详情」里复制"
              onChange={(event) => setFact(event.target.value)}
            />
          </label>
          <label className="block">
            <span className="type-body leading-4 text-idpxyz-textMuted">从何时起算 *</span>
            <Input
              value={effectiveAt}
              className="font-mono type-value"
              placeholder="如 2026-09-04T06:05:00Z 或 2026-09-04T14:05:00+08:00"
              onChange={(event) => setEffectiveAt(event.target.value)}
            />
          </label>
        </div>
        <div className="flex items-center gap-3">
          <Button
            variant="default"
            className={primaryActionClass}
            onClick={send}
            disabled={!ready || state.kind === 'submitting'}
          >
            {state.kind === 'submitting' ? '提交中…' : '确认'}
          </Button>
          <Button variant="outline" onClick={onClose}>
            收起
          </Button>
        </div>
        <JudgmentAnswerNote state={state} />
      </CardContent>
    </Card>
  );
}

function JudgmentAnswerNote({ state }: { state: PanelState }) {
  if (state.kind === 'idle' || state.kind === 'submitting') return null;
  if (state.kind === 'malformed') {
    return <p className="type-body leading-4 text-idpxyz-danger">没有提交：{state.message}</p>;
  }

  const answer = state.answer;
  switch (answer.kind) {
    case 'outcome': {
      const note = describeJudgmentAnswer(answer.body);
      const tone = note.tone === 'answered' ? 'text-idpxyz-textMuted' : 'text-idpxyz-danger';
      return (
        <div className={`type-body leading-4 ${tone}`}>
          <p>{note.headline}</p>
          {note.details.length > 0 ? (
            <ul className="mt-0.5 ml-4 list-disc">
              {note.details.map((line) => (
                <li key={line}>{line}</li>
              ))}
            </ul>
          ) : null}
        </div>
      );
    }
    case 'unconfigured':
      return (
        <p className="type-body leading-4 text-idpxyz-textMuted">
          操作入口还没有配置好，这次什么都没有保存；重试也不会改变结果，请联系系统管理员完成配置。在那之前这条轨迹仍是待确认，
          不会进物流动态。
        </p>
      );
    case 'loginRequired':
    case 'notGranted':
      return <OperatorRefusalNote kind={answer.kind} what="确认起算时间" />;
    case 'callerProblem':
      return (
        <p className="type-body leading-4 text-idpxyz-danger">
          没有确认：{judgmentProblemNote(answer.code)}
        </p>
      );
    case 'noAnswer':
      return (
        <p className="type-body leading-4 text-idpxyz-danger">
          结果还不确定：{judgmentProblemNote(answer.code)}；稍后刷新看这一条确认上没有，重新提交同一时间不会重复记录。
        </p>
      );
    case 'transport':
      return <p className="type-body leading-4 text-idpxyz-danger">连不上服务，这次没有提交；请检查网络后重试（{answer.message}）</p>;
  }
}
