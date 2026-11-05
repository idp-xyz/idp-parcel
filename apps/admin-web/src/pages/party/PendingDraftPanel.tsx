import { useState } from 'react';
import { X } from 'lucide-react';
import { partyAnswersDomain } from './party-writes';
import { useAnswer, useRetryWhenChanged } from '../../query/answers';
import { Button, ConfirmDialog } from '@idpxyz/ui-primitives';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { primaryActionClass } from '../../components/primary-action';
import type { ApiResult } from '../catalogue-api';
import { currentDisplayTimeZone } from '../moment';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { DraftNote, FactList, FlowAnswerNote, PublicationNote } from './PublicationAnswerNotes';
import { draftReferenceOf } from './publication-draft-flow';
import {
  approveOutcomeLabels,
  approvePublicationDraft,
  publishPublicationDraft,
  readPublicationDraft,
  type CommercialObjectKindName,
  type PublicationDraftPublicationResponseBody,
  type PublicationDraftRecord,
  type PublicationDraftResponseBody,
} from './publication-draft-api';
import {
  canonicalDocumentText,
  draftDecisions,
  draftFacts,
  draftTechnicalRows,
  unreadableRecovery,
  unreadableWord,
} from './pending-publications';
import { publicationKindLabels } from './publication-summary';
import { useMayWriteRegistry } from '../use-operator-grants';

export interface PendingDraftIdentity {
  kind: CommercialObjectKindName;
  objectId: string;
  version: string;
}

type Busy = 'approve' | 'publish' | null;

/**
 * 本版系统读不出这份改动的内容：复述的事实取自库上的列，内容不给——折不回的内容不能冒充摘要盖住的那一份。
 * 说清为什么、该怎么办（随状态）；成因原话进技术详情给要查的人。
 */
function UnreadableNote({ draft }: { draft: PublicationDraftRecord }) {
  return (
    <section className="flex flex-col gap-1 rounded border border-idpxyz-border bg-idpxyz-inputBg p-3 type-body leading-4 text-idpxyz-textMuted">
      <p className="font-medium text-idpxyz-text">{unreadableWord}，不能批准或发布</p>
      <p>
        这份改动的格式是当前版本的系统不认得的，或含系统已不再接受的内容，所以看不到内容；刷新或重试不会改变结果。
        {unreadableRecovery(draft.status)}
      </p>
    </section>
  );
}

/**
 * 审批与发布的右栏详情（ADR-0314 决定三；容器照票 commercial-registers-edit-containers/06，从 PendingDraftDrawer 换来）：打开时按身份
 * 读回这份载体——版本壳、录入与批准的痕迹、规范化文档——批准与发布在栏底的动作行做，带的摘要是**栏里此刻看到的这份**读回的
 * 摘要，不是列表行上的、更不是前端算的。别的操作者在这之间改过内容，服务端答`已变更`，栏随即重读，批准者看到的就是现在的内容，
 * 再决定批不批。
 *
 * 批准者能不能批由服务端的审批职责规则判；这里只按状态词摆按钮，按下去的结果原样示出。
 * 调用方按载体身份给 key：换一份载体整个重挂，上一份的批准与发布答复说的是别的东西，不留。
 */
export function PendingDraftPanel({
  target,
  onClose,
  onChanged,
}: {
  target: PendingDraftIdentity;
  onClose: () => void;
  /** 批准或发布有结果之后叫一声，让页面重取列表。 */
  onChanged: () => void;
}) {
  const [reloadKey, setReloadKey] = useState(0);
  const [approval, setApproval] = useState<ApiResult<PublicationDraftResponseBody> | null>(null);
  const [publication, setPublication] = useState<ApiResult<PublicationDraftPublicationResponseBody> | null>(null);
  const [busy, setBusy] = useState<Busy>(null);
  const [confirmingPublish, setConfirmingPublish] = useState(false);

  // 键带载体的三格，换一份载体就是另一份；批准、发布答到后写口整域作废（party-writes.ts），读回随之重问。
  const readbackQuery = useAnswer(
    [partyAnswersDomain, 'publication-draft', target.kind, target.objectId, target.version],
    () => readPublicationDraft(target.kind, target.objectId, target.version),
  );
  const readback = readbackQuery.answer;
  useRetryWhenChanged(reloadKey, readbackQuery.retry);

  const draft = readback?.kind === 'outcome' ? readback.body.draft : undefined;
  // 读不回的载体没有可批准或发布的内容，动作按钮不摆；按下去服务端本也只会答 DRAFT_NOT_READABLE。
  const decisions = draft ? draftDecisions(draft) : { approve: false, publish: false };
  // 没有登记册配置写授予的操作者不摆批准与发布（ADR-0315）；摆不摆只是呈现，两个写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  const timeZone = currentDisplayTimeZone();

  const decide = (which: 'approve' | 'publish') => {
    if (draft === undefined) return;
    setBusy(which);
    // 带的是栏里读回的那一对摘要：内容摘要盖正文，壳摘要盖适用范围、有效区间与指名引用。
    const reference = draftReferenceOf(draft, { content: draft.contentDigest, shell: draft.shellDigest });
    const finish = () => {
      setBusy(null);
      // 结果出来之后重读：状态变了、或内容被别人改过（已变更），栏都要显示现在的样子。
      setReloadKey((value) => value + 1);
      onChanged();
    };
    if (which === 'approve') {
      void approvePublicationDraft(reference).then((answer) => {
        setApproval(answer);
        setPublication(null);
        finish();
      });
    } else {
      void publishPublicationDraft(reference).then((answer) => {
        setPublication(answer);
        finish();
      });
    }
  };

  const actionable = draft !== undefined && (decisions.approve || decisions.publish);

  return (
    <div className="flex h-full w-full flex-col bg-idpxyz-bg" aria-label={`改动 ${target.objectId} ${target.version}`}>
      <div className="space-y-2 border-b border-idpxyz-border px-5 pb-3 pt-4">
        <div className="flex items-start justify-between gap-3">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <h2 className="truncate font-mono type-title font-semibold text-idpxyz-textBright">{target.objectId}</h2>
            <span className="font-mono type-body leading-4 text-idpxyz-textMuted">{target.version}</span>
            {draft ? <StatusPill code={draft.status} /> : null}
          </div>
          <Button className="shrink-0 text-idpxyz-breadcrumb" variant="ghost" size="icon" onClick={onClose} aria-label="关闭详情" title="关闭（Esc）">
            <X className="h-4 w-4" aria-hidden="true" />
          </Button>
        </div>
        <p className="type-body text-idpxyz-textMuted">{publicationKindLabels[target.kind] ?? target.kind}</p>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto px-5 py-3">
        {readback === null ? (
          <p className="py-3 type-body text-idpxyz-textMuted">正在读取这份改动…</p>
        ) : (
          <FlowAnswerNote answer={readback} what="读取这份改动">
            {(body) =>
              body.draft === undefined ? (
                <p className="py-3 type-body text-idpxyz-textMuted">
                  找不到这份改动：它可能已被替换或从未存过。请回到列表刷新后再看。
                </p>
              ) : (
                <div className="flex flex-col gap-4 py-2">
                  <FactList facts={draftFacts(body.draft, timeZone)} />
                  {body.draft.readable ? null : <UnreadableNote draft={body.draft} />}
                  {approval ? <DraftNote answer={approval} what="批准" labels={approveOutcomeLabels} /> : null}
                  {publication ? <PublicationNote answer={publication} /> : null}
                  {body.draft.readable && body.draft.status === 'PENDING_APPROVAL' ? (
                    <p className="type-body leading-4 text-idpxyz-textMuted">
                      批准的是下面这份内容。如果在你批准之前有人改过它，这次不会批准，页面会换成改过的内容让你重新核对。
                    </p>
                  ) : null}
                  {body.draft.readable ? (
                    <section className="flex flex-col gap-1">
                      <h3 className="type-body font-medium text-idpxyz-text">提交的内容</h3>
                      <pre className="max-h-[50vh] overflow-auto rounded border border-idpxyz-border bg-idpxyz-inputBg p-3 font-mono type-body text-idpxyz-text whitespace-pre-wrap break-all">
                        {canonicalDocumentText(body.draft.content)}
                      </pre>
                    </section>
                  ) : null}
                  <TechnicalDetails rows={draftTechnicalRows(body.draft)} />
                </div>
              )
            }
          </FlowAnswerNote>
        )}
      </div>
      {/* 栏底动作行：有可做的决定才摆；没有写授予的人看到的是一句说明，不是一排灰按钮。 */}
      {actionable ? (
        <div className="flex shrink-0 flex-wrap items-center justify-end gap-2 border-t border-idpxyz-border px-5 py-3">
          {mayWrite ? (
            <>
              {decisions.approve ? (
                <Button variant="default" size="sm" className={primaryActionClass} disabled={busy !== null} onClick={() => decide('approve')}>
                  {busy === 'approve' ? '批准中…' : '批准'}
                </Button>
              ) : null}
              {decisions.publish ? (
                <Button
                  variant="default"
                  size="sm"
                  className={primaryActionClass}
                  disabled={busy !== null}
                  // 发布不能撤回、改内容要换版本号：按下之前再拦一道，复述要发的是哪一版。
                  onClick={() => setConfirmingPublish(true)}
                >
                  {busy === 'publish' ? '发布中…' : '发布'}
                </Button>
              ) : null}
            </>
          ) : (
            <p className="type-body leading-4 text-idpxyz-textMuted">你的账号没有批准与发布的权限；需要时请联系管理员开通。</p>
          )}
        </div>
      ) : null}
      <ConfirmDialog
        renderFooter={confirmDialogFooter}
        open={confirmingPublish}
        tone="warning"
        title="发布这一版？"
        message="发布后这一版不能修改；要改内容，须用新的版本号再发布。"
        extraContent={draft ? <FactList facts={draftFacts(draft, timeZone)} /> : undefined}
        confirmLabel="发布"
        cancelLabel="先不发布"
        onConfirm={() => {
          setConfirmingPublish(false);
          decide('publish');
        }}
        onCancel={() => setConfirmingPublish(false)}
      />
    </div>
  );
}
