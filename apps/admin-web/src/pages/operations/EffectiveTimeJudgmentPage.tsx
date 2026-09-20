import { useEffect, useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
import {
  judgeEffectiveTime,
  listExternalTrackingFacts,
  type ApiResult,
  type EffectiveTimeJudgmentResponseBody,
  type ExternalTrackingFactListResponseBody,
  type ExternalTrackingFactView,
} from './effective-time-judgment-api';
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
 */
function col(
  id: string,
  header: string,
  options?: { mono?: boolean; className?: string },
): ListColumn<FactRow> {
  return {
    id,
    header,
    className: options?.className,
    render: (row) => {
      const value = row.values[id] ?? '—';
      return options?.mono ? <span className="font-mono text-[12px]">{value}</span> : value;
    },
  };
}

const columns: ListColumn<FactRow>[] = [
  col('fact', '事实', { mono: true }),
  col('version', '当前版', { mono: true }),
  col('object', '载运对象', { mono: true }),
  col('sourceEvent', '源事件', { mono: true }),
  // 源的原始状态词，原样示出不译（ADR-0102 决定五）；「它对本仓意味着什么」是另一份登记的事。
  col('status', '状态词（源原词）', { mono: true }),
  col('occurredAt', '发生时间（源给）', { mono: true }),
  col('receivedAt', '接收时间', { mono: true }),
  col('effectiveBasis', '有效时间依据', { className: 'w-[140px]' }),
  col('effectiveAt', '有效时间', { mono: true }),
  col('basisDetail', '规则版本', { mono: true }),
  col('supersedes', '回指前版', { mono: true }),
  col('origin', '版本来源', { className: 'w-[88px]' }),
];

// 行动作列单独拼装：它要 setState，而上面那张表是模块级常量。动作只有「判断」——判断是回指前版的
// 新版本，原版本与原判断一字不动；页面上因此没有「编辑」或「改时间」。
function columnsWithActions(onJudge: (row: FactRow) => void): ListColumn<FactRow>[] {
  return [
    ...columns,
    {
      id: 'actions',
      header: '动作',
      align: 'center',
      className: 'w-[96px]',
      render: (row) => (
        <Button variant="outline" onClick={() => onJudge(row)}>
          判断
        </Button>
      ),
    },
  ];
}

const viewWords: Record<ExternalTrackingFactView, string> = {
  pending: '待判断',
  current: '全部当前版',
};

/** 判断面板的目标：从行上带来的当前版摘要，让面板在提交前就摆出「这一条判过没有、按哪个依据」。 */
interface JudgmentTarget {
  fact: string;
  version?: string;
  effectiveBasis?: string;
  effectiveAt?: string;
  basisDetail?: string;
}

export function EffectiveTimeJudgmentPage() {
  // 源入参分两格：输入框里的草稿与已提交的那一个。合成一格就会逐键触发查询，而每问一次都是一次真请求。
  const [sourceDraft, setSourceDraft] = useState('');
  const [source, setSource] = useState('');
  const [view, setView] = useState<ExternalTrackingFactView>('pending');
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{
    source: string;
    view: ExternalTrackingFactView;
    answer: ApiResult<ExternalTrackingFactListResponseBody>;
  } | null>(null);
  const [judging, setJudging] = useState<JudgmentTarget | null>(null);

  useEffect(() => {
    // 源空着不发请求：源是这个读面的必备维，缺席在服务端是 400，而那条 400 说的是「你问错了」，
    // 与「还没问」不是一件事。
    if (source.trim() === '') return undefined;
    let cancelled = false;
    void listExternalTrackingFacts(source, view).then((answer) => {
      if (!cancelled) setLoaded({ source, view, answer });
    });
    return () => {
      cancelled = true;
    };
  }, [source, view, reloadKey]);

  const answer = loaded?.source === source && loaded.view === view ? loaded.answer : null;
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const rows = body ? factRowsOf(body) : [];
  const retry = () => setReloadKey((value) => value + 1);

  const openJudgment = (row: FactRow) =>
    setJudging({
      fact: row.fact,
      version: row.version,
      effectiveBasis: row.values.effectiveBasis,
      effectiveAt: row.values.effectiveAt,
      basisDetail: row.values.basisDetail,
    });

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <ListPageTemplate<FactRow>
        title={info.title}
        description={`${info.owner} · 所有者就一条外部承运轨迹事实显式给出「从何时起对本仓有效」（ADR-0102 决定三）；页面不解释状态词、不建议时间。`}
        headerActions={
          <Button variant="outline" onClick={() => setJudging({ fact: '' })}>
            判断指名事实
          </Button>
        }
        search={{
          value: sourceDraft,
          onChange: setSourceDraft,
          placeholder: '输入轨迹源引用，再点「查询」',
        }}
        filters={
          <>
            {(Object.keys(viewWords) as ExternalTrackingFactView[]).map((candidate) => (
              <button
                key={candidate}
                type="button"
                className={chipClass(candidate === view)}
                onClick={() => setView(candidate)}
              >
                {viewWords[candidate]}
              </button>
            ))}
            <button
              type="button"
              className={chipClass(sourceDraft.trim() !== '')}
              onClick={() => setSource(sourceDraft.trim())}
            >
              查询
            </button>
          </>
        }
        // 计数只在拿到业务答案后显示，未配置/未指定源不报「0 行」。
        filterSummary={body ? `${viewWords[view]} ${rows.length} 行` : undefined}
        columns={columnsWithActions(openJudgment)}
        rows={rows}
        rowKey={(row) => row.key}
        viewState={factListViewState(answer, source, view, rows.length, retry, {
          module: info,
          endpoint: `GET /transport-fulfillment-external-tracking-facts?source=…&view=${view}`,
        })}
      />
      {judging ? (
        <JudgmentPanel
          // 换一条事实时重建面板：时刻与答复是上一条的，留着会让人把 A 的时间给 B。
          key={`${judging.fact}@${judging.version ?? ''}`}
          target={judging}
          onClose={() => setJudging(null)}
          onJudged={retry}
        />
      ) : null}
    </div>
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
      setState({ kind: 'malformed', message: `「${instant}」不是可解析的 RFC 3339 时刻` });
      return;
    }
    setState({ kind: 'submitting' });
    void judgeEffectiveTime({ fact: fact.trim(), effectiveAt: instant }).then((answer) => {
      setState({ kind: 'answered', answer });
      if (answer.kind === 'outcome' && answer.body.outcome === 'EFFECTIVE_TIME_JUDGED') onJudged();
    });
  };

  return (
    <Card className="m-4">
      <CardHeader>
        <CardTitle>
          判断有效时间
          {target.version ? (
            <span className="font-mono text-[13px] text-idpxyz-textMuted"> · 当前版 {target.version}</span>
          ) : null}
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <p className="text-xs text-idpxyz-textMuted">
          判断形成回指当前版的新版本，原版本与原判断保留；判断过的版本交 visibility-exception 进客户可见面。
          判断人身份由接入渠道给，本表单不收也不送。
          {target.version ? (
            // 再判之前先把「这一条判过没有、按哪个依据」摆出来（票 21 红线）；待判断行如实说待判断。
            <>
              <br />
              该当前版：{target.effectiveBasis}
              {target.effectiveAt && target.effectiveAt !== '—' ? `，有效时间 ${target.effectiveAt}` : ''}
              {target.basisDetail ? `，规则 ${target.basisDetail}` : ''}。
            </>
          ) : null}
        </p>
        <div className="grid grid-cols-2 gap-3">
          <label className="block">
            <span className="text-xs text-idpxyz-textMuted">事实引用 *</span>
            <Input
              value={fact}
              className="font-mono text-[13px]"
              placeholder="本上下文铸的事实身份，如列表「事实」一列"
              onChange={(event) => setFact(event.target.value)}
            />
          </label>
          <label className="block">
            <span className="text-xs text-idpxyz-textMuted">从何时起有效（RFC 3339）*</span>
            <Input
              value={effectiveAt}
              className="font-mono text-[13px]"
              placeholder="例：2026-09-04T06:05:00Z（不预填，页面不建议时间）"
              onChange={(event) => setEffectiveAt(event.target.value)}
            />
          </label>
        </div>
        <div className="flex items-center gap-3">
          <Button onClick={send} disabled={!ready || state.kind === 'submitting'}>
            {state.kind === 'submitting' ? '提交中…' : '提交判断'}
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
    return <p className="text-xs text-idpxyz-danger">未提交：{state.message}</p>;
  }

  const answer = state.answer;
  switch (answer.kind) {
    case 'outcome': {
      const note = describeJudgmentAnswer(answer.body);
      const tone = note.tone === 'answered' ? 'text-idpxyz-textMuted' : 'text-idpxyz-danger';
      return (
        <div className={`text-xs ${tone}`}>
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
        <p className="text-xs text-idpxyz-textMuted">
          接入渠道未配置（403 ACCESS_CHANNEL_NOT_CONFIGURED）。判断端点已建立并装配，但操作者身份的
          接入渠道尚未登记，服务端按 ADR-0055 如实拒绝——<strong>这是诚实答案不是尚未实现</strong>，
          改请求或重试都不会改变结果。登记接入渠道认证参数（PAR-INT-01，实例半边）后由装配侧换上真
          Intake 即放行；此前该事实继续留为待判断、不进客户可见面（ADR-0102 Consequences）。
        </p>
      );
    case 'callerProblem':
      return (
        <p className="text-xs text-idpxyz-danger">
          调用方式问题（HTTP {answer.status}）：{judgmentProblemNote(answer.code)}
        </p>
      );
    case 'noAnswer':
      return (
        <p className="text-xs text-idpxyz-danger">
          服务端未形成答案（HTTP {answer.status}）：{judgmentProblemNote(answer.code)}；判断落没落上
          未知，可稍后重试——重放同一判断无害。
        </p>
      );
    case 'transport':
      return <p className="text-xs text-idpxyz-danger">请求未到达 parcel-api：{answer.message}</p>;
  }
}
