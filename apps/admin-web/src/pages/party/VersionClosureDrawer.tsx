import { useState } from 'react';
import { Button, ConfirmDialog, Drawer, DrawerBody, DrawerHeader, Input } from '@idpxyz/ui-primitives';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { primaryActionClass } from '../../components/primary-action';
import { RegistrationAnswerNote, type RegistrationPanelState } from '../../components/registration';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { currentDisplayTimeZone, formatInstant } from '../moment';
import {
  closeCommercialVersion,
  listCommercialPolicies,
  listCustomerContracts,
  listServiceProducts,
  listSupplierAgreements,
} from './api';
import { statusBadge } from './detail-primitives';
import {
  agreementKnownVersions,
  contractKnownVersions,
  policyKnownVersions,
  productKnownVersions,
  type KnownVersion,
} from './known-versions';
import { WallTimeField } from './party-registration-fields';
import { commercialStatusLabels, problemNote } from './presentation';
import { Field, ProblemText, selectClass, useLoaded } from './PublicationFormFields';
import {
  closureAnswerOf,
  closureConfirmLines,
  closureConfirmMessage,
  closureConfirmTitle,
  closureIncomplete,
  closureLocalProblems,
  closureModeOf,
  closureOldOf,
  closureOldProblem,
  closurePayloadOf,
  closureSuccessors,
  initialClosureDraft,
  versionClosureModeLabels,
  type ClosureDraft,
  type ClosureObjectKind,
  type ClosureTarget,
} from './version-closure';

const info = moduleInfoById['commercial-policies'];

function mapOutcome<From, To>(convert: (body: From) => To) {
  return (result: ApiResult<From>): ApiResult<To> =>
    result.kind === 'outcome' ? { ...result, body: convert(result.body) } : result;
}

/**
 * 抽屉打开时重读该册，不用页面上那份：收尾要对着此刻的册面判，行上的状态与区间可能已陈旧。函数在模块层各立一份，
 * useLoaded 按函数身份决定要不要重读，写在组件里每次渲染都是新函数会读个不停。
 */
const loadVersionsOf: Record<ClosureObjectKind, () => Promise<ApiResult<KnownVersion[]>>> = {
  SERVICE_PRODUCT: () => listServiceProducts().then(mapOutcome(productKnownVersions)),
  CUSTOMER_CONTRACT: () => listCustomerContracts().then(mapOutcome(contractKnownVersions)),
  SUPPLIER_AGREEMENT: () => listSupplierAgreements().then(mapOutcome(agreementKnownVersions)),
  AUTHORIZATION_RULE: () => listCommercialPolicies('AUTHORIZATION_RULE').then(mapOutcome(policyKnownVersions)),
  PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY: () =>
    listCommercialPolicies('PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY').then(mapOutcome(policyKnownVersions)),
  CUSTOMER_SERVICE_RULE: () => listCommercialPolicies('CUSTOMER_SERVICE_RULE').then(mapOutcome(policyKnownVersions)),
};

function describeRange(row: KnownVersion): string {
  return `${formatInstant(row.effectiveStartsAt)} → ${row.effectiveEndsAt ? formatInstant(row.effectiveEndsAt) : '持续有效'}`;
}

/**
 * 目录行上「收尾…」打开的抽屉（票 party-commercial-uiux/16）。同一对象换版后旧版不会自己结束，重叠的部分解析会答「适用冲突」；
 * 这里让登记方指名旧版让位给哪个后继：后继晚于旧版起点的截止在后继生效时点，同起点的退役。
 *
 * 次序：重读册面 → 认出旧版与可让位的后继 → 选后继、填依据（截止另有更正时刻）→ 确认框复述并说明不可撤回 → 提交。
 * 本组件不裁任何门：两版在不在册、都生效没有、后继接不接得住旧版此后那一段，由服务端按整册判，答复原样摆在按钮下方。
 * 纯逻辑在 version-closure.ts。
 */
export function VersionClosureDrawer({
  target,
  onClose,
  onClosed,
}: {
  target: ClosureTarget | null;
  onClose: () => void;
  /** 答复说收尾已落（或认出已落过）时调用，页面借它重取目录。 */
  onClosed: () => void;
}) {
  return (
    <Drawer open={target !== null} onOpenChange={(open) => (open ? undefined : onClose())} aria-label="收尾旧版">
      {target ? (
        <ClosureBody
          key={`${target.kind}|${target.scope}|${target.objectId}|${target.version}`}
          target={target}
          onClose={onClose}
          onClosed={onClosed}
        />
      ) : null}
    </Drawer>
  );
}

function ClosureBody({
  target,
  onClose,
  onClosed,
}: {
  target: ClosureTarget;
  onClose: () => void;
  onClosed: () => void;
}) {
  const answer = useLoaded(loadVersionsOf[target.kind]);
  const all = answer?.kind === 'outcome' ? answer.body : null;
  const old = all === null ? null : closureOldOf(all, target);
  const successors = all !== null && old !== null ? closureSuccessors(old, all) : [];
  const oldProblem = all === null ? null : closureOldProblem(old);

  return (
    <>
      <DrawerHeader>
        <div className="flex items-center gap-2">
          <span className="text-[13px] text-idpxyz-textMuted">收尾旧版</span>
          <span className="font-mono font-medium text-idpxyz-text">{target.objectId}</span>
          <span className="font-mono text-xs text-idpxyz-textMuted">{target.version}</span>
          {old?.status ? statusBadge(commercialStatusLabels, old.status) : null}
        </div>
        <p className="mt-0.5 text-[12px] text-idpxyz-textMuted">
          {target.kindLabel}
          {old ? ` · 生效区间 ${describeRange(old)}` : ''}
        </p>
      </DrawerHeader>
      <DrawerBody>
        <div className="flex flex-col gap-3">
          <p className="text-[12px] text-idpxyz-textMuted">
            {'同一对象发了新版本后，旧版不会自己结束：两版区间重叠的部分，解析会答「适用冲突」，直到旧版被收尾。' +
              '收尾让旧版把位置让给指名的后继，分两种：后继晚于旧版起点的，旧版截止在后继生效的那一刻；' +
              '两版同一起点的，旧版整版退役。'}
          </p>
          {answer === null ? (
            <p className="text-[12px] text-idpxyz-textMuted">正在读取这个对象的各个版本…</p>
          ) : all === null ? (
            <p className="text-[12px] text-idpxyz-danger">
              没能读到册面，无法核对旧版与后继，请稍后重试。这不是收尾被拒绝，什么都没有写。
            </p>
          ) : oldProblem !== null ? (
            <p className="text-[12px] text-idpxyz-danger">{oldProblem}</p>
          ) : successors.length === 0 ? (
            <p className="text-[12px] text-idpxyz-textMuted">
              {'现在没有可以让这一版让位的后继。后继必须是同一对象、同一范围里「在用」、生效起点不早于这一版的另一个版本；' +
                '需要的话先到发布页签发布新版本。'}
            </p>
          ) : (
            <ClosureForm target={target} old={old!} successors={successors} onClose={onClose} onClosed={onClosed} />
          )}
        </div>
      </DrawerBody>
    </>
  );
}

function ClosureForm({
  target,
  old,
  successors,
  onClose,
  onClosed,
}: {
  target: ClosureTarget;
  old: KnownVersion;
  successors: KnownVersion[];
  onClose: () => void;
  onClosed: () => void;
}) {
  const timeZone = currentDisplayTimeZone();
  const [draft, setDraft] = useState<ClosureDraft>(() => initialClosureDraft(successors, new Date(), timeZone));
  const [confirming, setConfirming] = useState(false);
  const [state, setState] = useState<RegistrationPanelState>({ kind: 'idle' });
  const [finished, setFinished] = useState(false);

  const successor = successors.find((row) => row.version === draft.successor) ?? null;
  const mode = successor === null ? null : closureModeOf(old, successor);
  const problems = closureLocalProblems(draft, mode, timeZone);
  const locked = state.kind === 'submitting' || finished;

  const submit = async () => {
    if (mode === null) return;
    setState({ kind: 'submitting' });
    const result = await closeCommercialVersion(mode, closurePayloadOf(target, mode, draft, timeZone));
    setConfirming(false);
    setState({ kind: 'answered', answer: result });
    if (result.kind === 'outcome' && closureAnswerOf(mode, target, result.body).done) {
      setFinished(true);
      onClosed();
    }
  };

  const answered = state.kind === 'answered' && state.answer.kind === 'outcome' && mode !== null ? state.answer : null;
  const outcome = answered !== null && mode !== null ? closureAnswerOf(mode, target, answered.body) : null;

  return (
    <>
      <Field label="让位给哪一版 *" path="successor" problems={problems}>
        <select
          className={selectClass}
          value={draft.successor}
          disabled={locked}
          onChange={(event) => setDraft((current) => ({ ...current, successor: event.target.value }))}
        >
          <option value="">请选择</option>
          {successors.map((row) => (
            <option key={row.version} value={row.version}>
              {row.version}（{formatInstant(row.effectiveStartsAt)} 起）
            </option>
          ))}
        </select>
      </Field>
      {successor !== null && mode !== null ? (
        <p className="text-[12px] text-idpxyz-text">
          {mode === 'cut'
            ? `${successor.version} 晚于 ${old.version} 起点：${old.version} 将截止在 ${formatInstant(successor.effectiveStartsAt)}，此前照旧可选，此后不再参选。`
            : `${successor.version} 与 ${old.version} 同一起点：${old.version} 将整版退役，此后不再参与新的选择。`}
        </p>
      ) : null}
      <Field label="收尾依据 *" path="reference" problems={problems}>
        <Input
          value={draft.reference}
          readOnly={locked}
          className="font-mono text-[13px]"
          placeholder="做出这个决定的依据，例如决议或工单编号"
          onChange={(event) => setDraft((current) => ({ ...current, reference: event.target.value }))}
        />
      </Field>
      {mode === 'cut' ? (
        <WallTimeField
          label="更正时刻 *"
          path="correctedAt"
          problems={problems}
          value={draft.correctedAt}
          locked={locked}
          timeZone={timeZone}
          onChange={(correctedAt) => setDraft((current) => ({ ...current, correctedAt }))}
          note="默认是打开这个窗口的时刻。请求超时要重发时保持它不变，服务端才认得出是同一次截止。"
        />
      ) : null}
      <div className="flex items-center gap-2">
        {finished ? (
          <Button variant="default" size="sm" className={`${primaryActionClass} text-[12px]`} onClick={onClose}>
            完成
          </Button>
        ) : (
          <>
            <Button
              variant="default"
              size="sm"
              className={`${primaryActionClass} text-[12px]`}
              disabled={locked || mode === null || closureIncomplete(draft, mode) || Object.keys(problems).length > 0}
              onClick={() => setConfirming(true)}
            >
              核对并{mode === null ? '收尾' : versionClosureModeLabels[mode]}
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="text-[12px] text-idpxyz-breadcrumb"
              disabled={locked}
              onClick={onClose}
            >
              不收尾了
            </Button>
          </>
        )}
      </div>

      {outcome !== null ? (
        <div className={`text-[12px] ${outcome.done ? 'text-idpxyz-text' : 'text-idpxyz-danger'}`} role="status">
          <p>{outcome.text}</p>
          {outcome.cause ? (
            <p className="mt-1">
              原因：<ProblemText problem={outcome.cause} />
            </p>
          ) : null}
        </div>
      ) : (
        <RegistrationAnswerNote state={state} owner={info.owner} outcomeLabels={{}} problemNote={problemNote} />
      )}

      {successor !== null && mode !== null ? (
        <ConfirmDialog
          renderFooter={confirmDialogFooter}
          open={confirming}
          tone={mode === 'retirement' ? 'danger' : 'warning'}
          title={closureConfirmTitle(mode, target)}
          message={closureConfirmMessage(mode)}
          extraContent={
            <ul className="text-[12px] text-idpxyz-text flex flex-col gap-1">
              {closureConfirmLines(target, old, successor, mode, draft, timeZone).map((line) => (
                <li key={line}>{line}</li>
              ))}
            </ul>
          }
          confirmLabel={`确认${versionClosureModeLabels[mode]}`}
          cancelLabel="再想想"
          onConfirm={submit}
          onCancel={() => setConfirming(false)}
        />
      ) : null}
    </>
  );
}
