import { useEffect, useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
import { OperatorRefusalNote } from '../../components/states';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { technicalRows } from '../settlement/ops-ui';
import { useMayMakeDecision } from '../use-operator-grants';
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

// 列只放判断时要看的：哪件货、承运商说了什么（源的原始状态词原样示出不译，ADR-0102 决定五）、它说的发生时间、我们收到的
// 时间、怎么确认的、从何时起算。轨迹编号、记录版本、源事件、凭据、规则版本与接续关系进每行末尾的「技术详情」。
const columns: ListColumn<FactRow>[] = [
  col('object', '货物', { mono: true }),
  col('status', '承运商的说法（原词）', { mono: true }),
  col('occurredAt', '承运商说的发生时间', { mono: true }),
  col('receivedAt', '我们收到的时间', { mono: true }),
  {
    id: 'effectiveBasis',
    header: '确认方式',
    align: 'center',
    className: 'w-[132px]',
    render: (row) => <StatusPill label={row.values.effectiveBasis} tone={row.values.effectiveBasis === '待确认' ? 'warn' : 'good'} />,
  },
  col('effectiveAt', '从何时起算', { mono: true }),
  {
    id: 'technical',
    header: '',
    className: 'align-top',
    render: (row) => (
      <TechnicalDetails
        className="max-w-md open:min-w-[18rem]"
        rows={technicalRows([
          { label: '轨迹编号', value: row.values.fact },
          { label: '记录版本', value: row.values.version },
          { label: '来源事件', value: row.values.sourceEvent === '—' ? '' : row.values.sourceEvent },
          { label: '凭据', value: row.values.credential },
          { label: '规则版本', value: row.values.basisDetail },
          { label: '接续', value: row.values.supersedes },
          { label: '记录来源', value: row.values.origin },
        ])}
      />
    ),
  },
];

// 行动作列单独拼装：它要 setState，而上面那张表是模块级常量。动作只有「判断」——判断是回指前版的
// 新版本，原版本与原判断一字不动；页面上因此没有「编辑」或「改时间」。
function columnsWithActions(onJudge: (row: FactRow) => void, mayJudge: boolean): ListColumn<FactRow>[] {
  // 写入口按授予收起（ADR-0315）：明确没有外部轨迹时间确认授予时整列不摆。
  if (!mayJudge) return columns;
  return [
    ...columns,
    {
      id: 'actions',
      header: '动作',
      align: 'center',
      className: 'w-[96px]',
      render: (row) => (
        <Button variant="outline" onClick={() => onJudge(row)}>
          确认时间
        </Button>
      ),
    },
  ];
}

const viewWords: Record<ExternalTrackingFactView, string> = {
  pending: '待确认',
  current: '全部轨迹',
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
  const mayJudge = useMayMakeDecision('EFFECTIVE_TIME_JUDGMENT');

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
        description="承运商回传的轨迹从什么时候起算，由我们确认后才进物流动态。输入轨迹来源查看待确认的轨迹，逐条填写起算时间；系统不替你推测时间。"
        headerActions={
          mayJudge ? (
            <Button variant="outline" onClick={() => setJudging({ fact: '' })}>
              按轨迹编号确认
            </Button>
          ) : null
        }
        search={{
          value: sourceDraft,
          onChange: setSourceDraft,
          placeholder: '输入轨迹来源（如承运商的接口编码），再点「查询」',
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
        filterSummary={body ? `${viewWords[view]} ${rows.length} 条` : undefined}
        columns={columnsWithActions(openJudgment, mayJudge)}
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
    <Card className="m-4">
      <CardHeader>
        <CardTitle>
          确认起算时间
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <p className="text-xs text-idpxyz-textMuted">
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
            <span className="text-xs text-idpxyz-textMuted">轨迹编号 *</span>
            <Input
              value={fact}
              className="font-mono text-[13px]"
              placeholder="从列表那一行的「技术详情」里复制"
              onChange={(event) => setFact(event.target.value)}
            />
          </label>
          <label className="block">
            <span className="text-xs text-idpxyz-textMuted">从何时起算 *</span>
            <Input
              value={effectiveAt}
              className="font-mono text-[13px]"
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
    return <p className="text-xs text-idpxyz-danger">没有提交：{state.message}</p>;
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
          操作入口还没有配置好，这次什么都没有保存；重试也不会改变结果，请联系系统管理员完成配置。在那之前这条轨迹仍是待确认，
          不会进物流动态。
        </p>
      );
    case 'loginRequired':
    case 'notGranted':
      return <OperatorRefusalNote kind={answer.kind} what="确认起算时间" />;
    case 'callerProblem':
      return (
        <p className="text-xs text-idpxyz-danger">
          没有确认：{judgmentProblemNote(answer.code)}
        </p>
      );
    case 'noAnswer':
      return (
        <p className="text-xs text-idpxyz-danger">
          结果还不确定：{judgmentProblemNote(answer.code)}；稍后刷新看这一条确认上没有，重新提交同一时间不会重复记录。
        </p>
      );
    case 'transport':
      return <p className="text-xs text-idpxyz-danger">连不上服务，这次没有提交；请检查网络后重试（{answer.message}）</p>;
  }
}
