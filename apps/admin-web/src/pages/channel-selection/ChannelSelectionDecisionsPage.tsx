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
import { OperatorRefusalNote } from '../../components/states';
import { StatusPill } from '../../components/StatusPill';
import { CodeRef } from '../../components/CodeRef';
import { TechnicalDetails } from '../../components/TechnicalDetails';
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
  decisionTechnicalRows,
  ruleLabels,
  tiedListViewState,
  wordOf,
  type DecisionDetailState,
  type DecisionRow,
} from './channel-selection-decisions';

// 页面标题取导航登记的那一份;主责上下文(parcel-shipment)只在代码里出现,屏上不说系统的话。
const info = moduleInfoById['channel-selection-decisions'];

/**
 * 渠道择优决定的运营查阅面(票 label-channel/23;票 14 裁决「谁读它:运营查阅面」)。
 *
 * 列表列的是停在**并列冲突**的择优——`PAR-NET-16`「并列且无法选出唯一一条时为冲突,交人工裁决」那一格的
 * 待办面:哪一票、何时、哪几家并列。点一行展开逐候选结果:选中 / 落选 / 出局(带因由)/ 并列。按决定号也能查一条
 * 不在列表里的决定(选出唯一者的、无人参选的)。商业范围与产品—渠道映射引用是收窄条件与技术详情,不上列。
 *
 * 本页**只列不裁**:人工裁决的形状(裁决人、裁决规则属实例半边;机制侧怎么落要先过 /domain-modeling)不在
 * 本票,页面上没有任何「裁决」动作。也**没有金额**:金额留在 parcel-pricing 的评价上,这里只透评价引用;
 * 不按时间隐式截断,窄口只有对象一维,由输入框显式给。
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
  col('decidedAt', '比较时间', { className: 'w-[190px]' }),
  col('tiedCandidates', '并列的渠道', { mono: true }),
  col('candidateCount', '参与比较', { className: 'w-[90px]' }),
  col('rule', '比法', { className: 'w-[96px]' }),
];

export function ChannelSelectionDecisionsPage() {
  // 收窄入参分两格:输入框里的草稿与已提交的那一个。合成一格就会逐键触发查询,而每问一次都是一次真请求。
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

  // 两个都填才收窄:服务端只给一半答 400,那条 400 说的是「你问错了」,页面不替它兜成「没收窄」。
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
        description="系统给一票货比渠道时,几家并列、选不出唯一一家的停在这里等人裁;点一行看各家的比较结果。"
        headerActions={
          <div className="flex items-center gap-2">
            <Input
              value={inspectDraft}
              className="font-mono text-[12px] w-[220px]"
              placeholder="按决定号查看"
              onChange={(event) => setInspectDraft(event.target.value)}
            />
            <Button variant="outline" disabled={inspectDraft.trim() === ''} onClick={() => setInspecting(inspectDraft.trim())}>
              查看
            </Button>
          </div>
        }
        filters={
          <>
            <Input
              value={scopeDraft}
              className="font-mono text-[12px] w-[200px]"
              placeholder="商业范围"
              onChange={(event) => setScopeDraft(event.target.value)}
            />
            <Input
              value={mappingDraft}
              className="font-mono text-[12px] w-[200px]"
              placeholder="产品—渠道映射"
              onChange={(event) => setMappingDraft(event.target.value)}
            />
            <button type="button" className={chipClass(draftComplete)} onClick={applySubject} disabled={!draftComplete}>
              只看这个范围
            </button>
            {subject ? (
              <button type="button" className={chipClass(false)} onClick={clearSubject}>
                看全部
              </button>
            ) : null}
          </>
        }
        // 计数只在拿到业务答案后显示,未配置态不报「0 行」。
        filterSummary={body ? `并列 ${rows.length} 条${subject ? '(已收窄)' : ''}` : undefined}
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
          // 换一条决定时重建面板:上一条的候选留着会让人把 A 的出局因由读成 B 的。
          key={inspecting}
          decisionId={inspecting}
          onClose={() => setInspecting(null)}
        />
      ) : null}
    </div>
  );
}

/**
 * 一条决定的逐候选面板。走单份分支(`?decisionId=`)而不是复用列表行上的候选:列表只列并列冲突,按标识
 * 查看要能拿到不在列表里的决定(选出唯一者的、无人参选的),两条分支各自是读面的一半。
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
        <CardTitle className="flex items-center gap-2">
          这次择优的结果 <CodeRef value={decisionId} title="决定号" />
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
        <div className="text-xs text-idpxyz-textMuted">
          没有这条决定,可能是号填错了;请核对后再查。
          <TechnicalDetails
            rows={[
              { label: '决定标识', value: state.decisionId },
              { label: 'HTTP 状态', value: '404' },
              { label: '问题码', value: 'CHANNEL_SELECTION_DECISION_NOT_VISIBLE' },
            ]}
          />
        </div>
      );
    case 'unconfigured':
      return (
        <div className="text-xs text-idpxyz-textMuted">
          管理台还没接上操作者登录渠道,读不到这条决定;请部署的人配置后再试。
          <TechnicalDetails
            rows={[
              { label: 'HTTP 状态', value: '403' },
              { label: '问题码', value: 'ACCESS_CHANNEL_NOT_CONFIGURED' },
            ]}
          />
        </div>
      );
    case 'loginRequired':
    case 'notGranted':
      return <OperatorRefusalNote kind={state.kind} />;
    case 'error':
      return (
        <div className="flex flex-col gap-1 text-xs text-idpxyz-textMuted">
          <div className="flex items-center gap-3">
            <span>{state.message}</span>
            {state.canRetry ? (
              <Button variant="outline" onClick={retry}>
                重试
              </Button>
            ) : null}
          </div>
          <TechnicalDetails rows={state.technical} />
        </div>
      );
    case 'decision': {
      const decision = state.decision;
      const candidates = candidateRowsOf(decision);
      return (
        <>
          <p className="text-xs text-idpxyz-text">
            结论:<strong>{wordOf(conclusionLabels, decision.conclusion)}</strong>
            {decision.selectedCandidate ? (
              <>
                ,选中 <span className="font-mono">{decision.selectedCandidate}</span>
              </>
            ) : null}
            ;比法:{wordOf(ruleLabels, decision.rule)};比较时间 {formatInstant(decision.decidedAt)}。
          </p>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>候选渠道</TableHead>
                <TableHead className="w-[88px]">结果</TableHead>
                <TableHead>出局原因</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {candidates.map((row, index) => (
                <TableRow key={row.key}>
                  <TableCell className="font-mono text-[12px]">{row.values.candidate}</TableCell>
                  <TableCell>
                    <StatusPill code={decision.candidates[index]?.outcome ?? ''} label={row.values.outcome} />
                  </TableCell>
                  <TableCell>{row.values.exclusion || '—'}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          <p className="text-[11px] text-idpxyz-textMuted">报价金额不在本页:到「价格评价」页按技术详情里的评价引用查。</p>
          <TechnicalDetails rows={decisionTechnicalRows(decision)} />
        </>
      );
    }
  }
}
