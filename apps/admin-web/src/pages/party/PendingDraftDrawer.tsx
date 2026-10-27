import { useEffect, useRef, useState } from 'react';
import { Button, ConfirmDialog, Drawer, DrawerBody, DrawerHeader } from '@idpxyz/ui-primitives';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { primaryActionClass } from '../../components/primary-action';
import type { ApiResult } from '../catalogue-api';
import { currentDisplayTimeZone } from '../moment';
import { statusWordBadge } from './detail-primitives';
import { DraftNote, FactList, FlowAnswerNote, PublicationNote } from './PublicationAnswerNotes';
import { labelOf } from './presentation';
import { draftReferenceOf } from './publication-draft-flow';
import {
  approveOutcomeLabels,
  approvePublicationDraft,
  draftStatusLabels,
  publishPublicationDraft,
  readPublicationDraft,
  type CommercialObjectKindName,
  type PublicationDraftPublicationResponseBody,
  type PublicationDraftReadbackResponseBody,
  type PublicationDraftRecord,
  type PublicationDraftResponseBody,
} from './publication-draft-api';
import { canonicalDocumentText, draftDecisions, draftFacts, unreadableRecovery } from './pending-publications';
import { publicationKindLabels } from './publication-summary';
import { useMayWriteRegistry } from '../use-operator-grants';

export interface PendingDraftIdentity {
  kind: CommercialObjectKindName;
  objectId: string;
  version: string;
}

type Busy = 'approve' | 'publish' | null;

/**
 * 本构建读不回这份载体的内容：上面的事实取自库上的列，规范化文档不给——折不回的文档不能冒充摘要盖住的那一份。
 * 说清为什么、该怎么办（随状态），成因原话跟在后面给要查的人。
 */
function UnreadableNote({ draft }: { draft: PublicationDraftRecord }) {
  return (
    <section className="flex flex-col gap-1 rounded border border-idpxyz-border bg-idpxyz-inputBg p-3 text-xs text-idpxyz-textMuted">
      <p className="font-medium text-idpxyz-text">本构建读不回这份发布稿的内容</p>
      <p>
        它的规范化版本本构建不认得，或含本构建不收的内容，所以看不到文档，也不能批准或发布；重试不会改变结果。
        {unreadableRecovery(draft.status)}
      </p>
      {draft.unreadableCause ? <p className="font-mono break-all">原因：{draft.unreadableCause}</p> : null}
    </section>
  );
}

/**
 * 待批准载体的详情抽屉（ADR-0314 决定三）：打开时按身份读回这份载体——版本壳、录入与批准的痕迹、规范化文档——批准与发布的
 * 按钮就在这里，带的摘要是**抽屉里此刻看到的这份**读回的摘要，不是列表行上的、更不是前端算的。别的操作者在这之间改过内容，
 * 服务端答`已变更`，抽屉随即重读，批准者看到的就是现在的内容，再决定批不批。
 *
 * 批准者能不能批由服务端的审批职责规则判；这里只按状态词摆按钮，按下去的结果原样示出。
 */
export function PendingDraftDrawer({
  target,
  onClose,
  onChanged,
}: {
  target: PendingDraftIdentity | null;
  onClose: () => void;
  /** 批准或发布有结果之后叫一声，让页面重取列表。 */
  onChanged: () => void;
}) {
  const identity = target === null ? '' : `${target.kind}|${target.objectId}|${target.version}`;
  const [reloadKey, setReloadKey] = useState(0);
  const [readback, setReadback] = useState<ApiResult<PublicationDraftReadbackResponseBody> | null>(null);
  const [approval, setApproval] = useState<ApiResult<PublicationDraftResponseBody> | null>(null);
  const [publication, setPublication] = useState<ApiResult<PublicationDraftPublicationResponseBody> | null>(null);
  const [busy, setBusy] = useState<Busy>(null);
  const [confirmingPublish, setConfirmingPublish] = useState(false);

  // 换一份载体：上一份的批准与发布答复说的是别的东西，不留。
  useEffect(() => {
    setApproval(null);
    setPublication(null);
    setBusy(null);
    setConfirmingPublish(false);
  }, [identity]);

  useEffect(() => {
    if (target === null) return undefined;
    let cancelled = false;
    setReadback(null);
    void readPublicationDraft(target.kind, target.objectId, target.version).then((next) => {
      if (!cancelled) setReadback(next);
    });
    return () => {
      cancelled = true;
    };
    // target 的三格由 identity 代表；对象引用每次渲染都新，不放进依赖。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [identity, reloadKey]);

  const draft = readback?.kind === 'outcome' ? readback.body.draft : undefined;
  // 读不回的载体没有可批准或发布的内容，动作按钮不摆；按下去服务端本也只会答 DRAFT_NOT_READABLE。
  const decisions = draft ? draftDecisions(draft) : { approve: false, publish: false };
  // 没有登记册配置写授予的操作者不摆批准与发布（ADR-0315）；摆不摆只是呈现，两个写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  const timeZone = currentDisplayTimeZone();

  // 答复回来时抽屉可能已经换了一份载体：旧答复说的是别的东西，不写进新的抽屉，只让列表与读回照常重取。
  const currentIdentity = useRef(identity);
  currentIdentity.current = identity;

  const decide = (which: 'approve' | 'publish') => {
    if (draft === undefined) return;
    setBusy(which);
    const startedFor = identity;
    // 带的是抽屉里读回的那一对摘要：内容摘要盖正文，壳摘要盖适用范围、有效区间与指名引用。
    const reference = draftReferenceOf(draft, { content: draft.contentDigest, shell: draft.shellDigest });
    const finish = () => {
      setBusy(null);
      // 结果出来之后重读：状态变了、或内容被别人改过（已变更），抽屉都要显示现在的样子。
      setReloadKey((value) => value + 1);
      onChanged();
    };
    if (which === 'approve') {
      void approvePublicationDraft(reference).then((answer) => {
        if (currentIdentity.current === startedFor) {
          setApproval(answer);
          setPublication(null);
          finish();
        } else onChanged();
      });
    } else {
      void publishPublicationDraft(reference).then((answer) => {
        if (currentIdentity.current === startedFor) {
          setPublication(answer);
          finish();
        } else onChanged();
      });
    }
  };

  return (
    <Drawer open={target !== null} onOpenChange={(open) => (open ? undefined : onClose())} aria-label="待批准发布详情">
      {target !== null ? (
        <>
          <DrawerHeader>
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-mono font-medium text-idpxyz-text">{target.objectId}</span>
              <span className="font-mono text-xs text-idpxyz-textMuted">{target.version}</span>
              {draft ? statusWordBadge(labelOf(draftStatusLabels, draft.status)) : null}
            </div>
            <p className="mt-0.5 text-[12px] text-idpxyz-textMuted">
              {publicationKindLabels[target.kind] ?? target.kind}
              {draft ? ` · 适用范围 ${draft.scope}` : ''}
            </p>
            {draft && !mayWrite && (decisions.approve || decisions.publish) ? (
              <p className="mt-2 text-xs text-idpxyz-textMuted">你没有登记册配置写授予，不能批准或发布这份发布稿。</p>
            ) : null}
            {draft && mayWrite ? (
              <div className="mt-2 flex flex-wrap items-center gap-2">
                {decisions.approve ? (
                  <Button
                    variant="default"
                    size="sm"
                    className={primaryActionClass}
                    disabled={busy !== null}
                    onClick={() => decide('approve')}
                  >
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
              </div>
            ) : null}
          </DrawerHeader>
          <DrawerBody>
            {readback === null ? (
              <p className="py-3 text-[12px] text-idpxyz-textMuted">正在读取这份发布稿…</p>
            ) : (
              <FlowAnswerNote answer={readback} what="读回发布稿">
                {(body) =>
                  body.draft === undefined ? (
                    <p className="py-3 text-[12px] text-idpxyz-textMuted">
                      读不到这份发布稿：这一版没有存过，或不在本租户。请回到列表核对。
                    </p>
                  ) : (
                    <div className="flex flex-col gap-4 py-2">
                      <FactList facts={draftFacts(body.draft, timeZone)} />
                      {body.draft.readable ? null : <UnreadableNote draft={body.draft} />}
                      {approval ? <DraftNote answer={approval} what="批准" labels={approveOutcomeLabels} /> : null}
                      {publication ? <PublicationNote answer={publication} /> : null}
                      {body.draft.readable && body.draft.status === 'PENDING_APPROVAL' ? (
                        <p className="text-xs text-idpxyz-textMuted">
                          批准的是下面这份内容：按下批准时带着上面的内容摘要，若这之间有人改过内容，服务端不批，并显示现在的内容。
                        </p>
                      ) : null}
                      {body.draft.readable ? (
                        <section className="flex flex-col gap-1">
                          <h3 className="text-[12px] font-medium text-idpxyz-text">规范化文档（内容摘要盖住的那一份）</h3>
                          <pre className="max-h-[50vh] overflow-auto rounded border border-idpxyz-border bg-idpxyz-inputBg p-3 font-mono text-[12px] text-idpxyz-text whitespace-pre-wrap break-all">
                            {canonicalDocumentText(body.draft.content)}
                          </pre>
                        </section>
                      ) : null}
                    </div>
                  )
                }
              </FlowAnswerNote>
            )}
          </DrawerBody>
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
        </>
      ) : null}
    </Drawer>
  );
}
