import { useEffect, useState } from 'react';
import {
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Input,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
import { formatInstant } from '../catalogue-view';
import {
  findChannelSelectionDecision,
  listTiedChannelSelectionDecisions,
  type ApiResult,
  type ChannelSelectionDecisionResponseBody,
  type ChannelSelectionSubjectFilter,
  type TiedChannelSelectionDecisionsResponseBody,
} from './api';
import {
  candidateRowsOf,
  conclusionLabels,
  decisionDetailState,
  decisionRowsOf,
  ruleLabels,
  tiedListViewState,
  wordOf,
  type DecisionDetailState,
  type DecisionRow,
} from './channel-selection-decisions';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['channel-selection-decisions'];

/**
 * 渠道择优决定的运营查阅面（票 label-channel/23；票 14 裁决「谁读它：运营查阅面」）。
 *
 * 列表列的是停在**并列冲突**的择优——`PAR-NET-16`「并列且无法选出唯一一条时为冲突，交人工裁决」那一格的
 * 待办面：哪一票、在哪个商业范围下按哪笔映射、何时、哪几家并列。点一行展开逐候选结果：选中 / 落选 / 出局
 * （带因由）/ 并列，与所用评价的引用。按标识也能查一条不在列表里的决定（选出唯一者的、无人参选的）。
 *
 * 本页**只列不裁**：人工裁决的形状（裁决人、裁决规则属实例半边；机制侧怎么落要先过 /domain-modeling）不在
 * 本票，页面上没有任何「裁决」动作。也**没有金额**：金额留在 parcel-pricing 的评价上，这里只透评价引用；
 * 不按时间隐式截断，窄口只有对象一维，由输入框显式给。
 */
function col(id: string, header: string, options?: { mono?: boolean; className?: string }): ListColumn<DecisionRow> {
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

const columns: ListColumn<DecisionRow>[] = [
  col('decisionId', '决定', { mono: true }),
  col('scope', '商业范围引用', { mono: true }),
  col('mapping', '产品—渠道映射引用', { mono: true }),
  col('decidedAt', '决定时刻', { mono: true }),
  col('assembledAsOf', '候选装配时点', { mono: true }),
  col('rule', '择优规则', { className: 'w-[96px]' }),
  col('tiedCandidates', '并列的候选', { mono: true }),
  col('candidateCount', '候选数', { className: 'w-[72px]' }),
];

export function ChannelSelectionDecisionsPage() {
  // 收窄入参分两格：输入框里的草稿与已提交的那一个。合成一格就会逐键触发查询，而每问一次都是一次真请求。
  const [scopeDraft, setScopeDraft] = useState('');
  const [mappingDraft, setMappingDraft] = useState('');
  const [subject, setSubject] = useState<ChannelSelectionSubjectFilter | undefined>(undefined);
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{
    subject: ChannelSelectionSubjectFilter | undefined;
    answer: ApiResult<TiedChannelSelectionDecisionsResponseBody>;
  } | null>(null);
  const [inspecting, setInspecting] = useState<string | null>(null);
  const [inspectDraft, setInspectDraft] = useState('');

  useEffect(() => {
    let cancelled = false;
    void listTiedChannelSelectionDecisions(subject).then((answer) => {
      if (!cancelled) setLoaded({ subject, answer });
    });
    return () => {
      cancelled = true;
    };
  }, [subject, reloadKey]);

  const answer = loaded && loaded.subject === subject ? loaded.answer : null;
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const rows = body ? decisionRowsOf(body) : [];
  const retry = () => setReloadKey((value) => value + 1);

  // 两个都填才收窄：服务端只给一半答 400，那条 400 说的是「你问错了」，页面不替它兜成「没收窄」。
  const draftComplete = scopeDraft.trim() !== '' && mappingDraft.trim() !== '';
  const applySubject = () => {
    if (!draftComplete) return;
    setSubject({ scope: scopeDraft.trim(), mapping: mappingDraft.trim() });
  };
  const clearSubject = () => {
    setScopeDraft('');
    setMappingDraft('');
    setSubject(undefined);
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <ListPageTemplate<DecisionRow>
        title={info.title}
        description={`${info.owner} · 列出停在「并列冲突」等人工裁决的择优决定（PAR-NET-16）；页面只列不裁、只透评价引用不透金额。`}
        headerActions={
          <div className="flex items-center gap-2">
            <Input
              value={inspectDraft}
              className="font-mono text-[12px] w-[220px]"
              placeholder="按决定标识查看（如 CSDN-…）"
              onChange={(event) => setInspectDraft(event.target.value)}
            />
            <Button
              variant="outline"
              disabled={inspectDraft.trim() === ''}
              onClick={() => setInspecting(inspectDraft.trim())}
            >
              查看
            </Button>
          </div>
        }
        filters={
          <>
            <Input
              value={scopeDraft}
              className="font-mono text-[12px] w-[200px]"
              placeholder="商业范围引用"
              onChange={(event) => setScopeDraft(event.target.value)}
            />
            <Input
              value={mappingDraft}
              className="font-mono text-[12px] w-[200px]"
              placeholder="产品—渠道映射引用"
              onChange={(event) => setMappingDraft(event.target.value)}
            />
            <button type="button" className={chipClass(draftComplete)} onClick={applySubject} disabled={!draftComplete}>
              按对象收窄
            </button>
            {subject ? (
              <button type="button" className={chipClass(false)} onClick={clearSubject}>
                清除收窄
              </button>
            ) : null}
          </>
        }
        // 计数只在拿到业务答案后显示，未配置态不报「0 行」。
        filterSummary={body ? `并列冲突 ${rows.length} 条${subject ? '（已按对象收窄）' : ''}` : undefined}
        columns={columns}
        rows={rows}
        rowKey={(row) => row.key}
        onRowClick={(row) => setInspecting(row.decisionId)}
        viewState={tiedListViewState(answer, subject, rows.length, retry, {
          module: info,
          endpoint: 'GET /channel-selection-decisions?view=tied',
        })}
      />
      {inspecting ? (
        <DecisionPanel
          // 换一条决定时重建面板：上一条的候选留着会让人把 A 的出局因由读成 B 的。
          key={inspecting}
          decisionId={inspecting}
          onClose={() => setInspecting(null)}
        />
      ) : null}
    </div>
  );
}

/**
 * 一条决定的逐候选面板。走单份分支（`?decisionId=`）而不是复用列表行上的候选：列表只列并列冲突，按标识
 * 查看要能拿到不在列表里的决定（选出唯一者的、无人参选的），两条分支各自是读面的一半。
 */
function DecisionPanel({ decisionId, onClose }: { decisionId: string; onClose: () => void }) {
  const [answer, setAnswer] = useState<ApiResult<ChannelSelectionDecisionResponseBody> | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void findChannelSelectionDecision(decisionId).then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [decisionId, reloadKey]);

  const state = decisionDetailState(answer, decisionId);

  return (
    <Card className="m-4">
      <CardHeader>
        <CardTitle>
          渠道择优决定 <span className="font-mono text-[13px]">{decisionId}</span>
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <DecisionPanelBody state={state} retry={() => setReloadKey((value) => value + 1)} />
        <div>
          <Button variant="outline" onClick={onClose}>
            收起
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function DecisionPanelBody({ state, retry }: { state: DecisionDetailState; retry: () => void }) {
  switch (state.kind) {
    case 'loading':
      return <p className="text-xs text-idpxyz-textMuted">读取中…</p>;
    case 'notVisible':
      return (
        <p className="text-xs text-idpxyz-textMuted">
          当前租户下没有决定 <span className="font-mono">{state.decisionId}</span>（404
          CHANNEL_SELECTION_DECISION_NOT_VISIBLE）。这是终局答案：不存在与属别的租户同答，页面不区分、也不重试。
        </p>
      );
    case 'unconfigured':
      return (
        <p className="text-xs text-idpxyz-textMuted">
          接入渠道未配置（403 ACCESS_CHANNEL_NOT_CONFIGURED）。查阅端点已建立并装配，但运营查阅的接入渠道尚未
          登记，服务端按 ADR-0055 如实拒绝——<strong>这是诚实答案不是尚未实现</strong>。
        </p>
      );
    case 'error':
      return (
        <div className="flex items-center gap-3 text-xs text-idpxyz-danger">
          <span>{state.message}</span>
          {state.canRetry ? (
            <Button variant="outline" onClick={retry}>
              重试
            </Button>
          ) : null}
        </div>
      );
    case 'decision': {
      const decision = state.decision;
      const candidates = candidateRowsOf(decision);
      return (
        <>
          <p className="text-xs text-idpxyz-textMuted">
            结论：<strong>{wordOf(conclusionLabels, decision.conclusion)}</strong>
            {decision.selectedCandidate ? (
              <>
                ，选中 <span className="font-mono">{decision.selectedCandidate}</span>
              </>
            ) : null}
            ；规则 {wordOf(ruleLabels, decision.rule)}；决定时刻 {formatInstant(decision.decidedAt)}；候选装配时点{' '}
            {formatInstant(decision.assembledAsOf)}；对象 <span className="font-mono">{decision.scope}</span> /{' '}
            <span className="font-mono">{decision.mapping}</span>。金额不在本页：要看去价格评价页按评价引用查。
          </p>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>候选</TableHead>
                <TableHead className="w-[88px]">结果</TableHead>
                <TableHead>所用评价引用</TableHead>
                <TableHead>出局因由</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {candidates.map((row) => (
                <TableRow key={row.key}>
                  <TableCell className="font-mono text-[12px]">{row.values.candidate}</TableCell>
                  <TableCell>{row.values.outcome}</TableCell>
                  <TableCell className="font-mono text-[12px]">{row.values.evaluation}</TableCell>
                  <TableCell>{row.values.exclusion || '—'}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </>
      );
    }
  }
}
