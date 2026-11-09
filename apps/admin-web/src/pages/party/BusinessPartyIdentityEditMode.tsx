import { useEffect, useMemo, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { Button, Input } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { RegistrationAnswerNote } from '../../components/registration';
import { moduleInfoById } from '../../navigation';
import { currentDisplayTimeZone, wallTimeOf } from '../moment';
import { partyIdentityOutcomeLabels, type BusinessPartyRecord } from './api';
import {
  businessPartyLocalProblems,
  businessPartyPayloadOf,
  emptyBusinessPartyDraft,
  identificationChanged,
  identificationDraftOf,
  registeredNumbersChanged,
  suggestedBusinessPartyRevision,
  type BusinessPartyDraft,
} from './business-party-form';
import { BusinessPartyIdentificationFields } from './BusinessPartyIdentificationFields';
import { AdvancedGroup, FormGroup, RevisionField, WallTimeField, useRegistrationForm } from './party-registration-fields';
import { problemNote, updateOrdinalLabel } from './presentation';
import { Field } from './PublicationFormFields';
import { advancedGroupOpen } from './registration-form';

const info = moduleInfoById['business-parties'];
const kind = 'business-party';
const advancedPaths = ['businessParties[0].revision', 'businessParties[0].effectiveFrom'];

/**
 * 往来单位对象页的编辑态（票 business-party-edit-mode/02，照公司主体的 ProfileEditMode）：更正就是同一个编码再登记一笔
 * （CONTEXT「参与方身份」：内容更正形成新的登记修订，不原地改写），所以壳与载荷仍是「新增往来单位」那一套——useRegistrationForm
 * 与 business-party-form.ts——只是编码锁死、名称与识别信息预填当前值、生效时间预填打开时的此刻（收在高级组里可改）。识别信息
 * 必须预填：一笔修订是整份内容，只改名称时不带原有的号，服务端就当成去掉了它（ADR-0367 决定一）。
 *
 * 「保存 / 取消」与答复不在正文里：经 `footerSlot` 钉进对象页的页脚动作条，槽位还没挂上时暂摆在正文末尾。
 * 不算任何门：名称空不空、修订号连不连续都原样送上去让服务端逐格答（与新增表单同一条纪律）。
 */
export function BusinessPartyIdentityEditMode({
  row,
  ours,
  knownParties,
  footerSlot,
  onLanded,
  onUnsavedChange,
  onCancel,
}: {
  row: BusinessPartyRecord;
  /** 我方公司的往来单位不摆注册号：号只登在公司主体上（ADR-0367 决定一），摆出来也只会被服务端拒。 */
  ours: boolean;
  /** 页面持有的参与方册；没取到传 null，修订号建议按 1 算。 */
  knownParties: readonly BusinessPartyRecord[] | null;
  footerSlot: HTMLElement | null;
  /** 登记册答 `REGISTERED` 时调用，带送出的草稿（更新序号在里面）。 */
  onLanded: (sent: BusinessPartyDraft | null) => void;
  onUnsavedChange: (unsaved: boolean) => void;
  onCancel: () => void;
}) {
  const timeZone = currentDisplayTimeZone();
  // 基线就是预填进去的那份：只有预填、人什么都没动，不算没提交。
  // 依赖按内容取键而不是整行对象：列表重取会交回一个新对象，内容没变时基线不该跟着重算（生效时间会被刷成新的此刻）。
  const identificationKey = JSON.stringify(identificationDraftOf(row));
  const opening = useMemo<BusinessPartyDraft>(
    () => ({
      ...emptyBusinessPartyDraft(),
      ...(JSON.parse(identificationKey) as ReturnType<typeof identificationDraftOf>),
      partyId: row.partyId,
      name: row.partyName,
      effectiveFrom: wallTimeOf(new Date(), timeZone),
    }),
    [row.partyId, row.partyName, identificationKey, timeZone],
  );
  const form = useRegistrationForm<BusinessPartyDraft>({
    kind,
    empty: () => opening,
    suggestion: (draft) => suggestedBusinessPartyRevision(knownParties, draft.partyId),
    localProblems: businessPartyLocalProblems,
    payloadOf: businessPartyPayloadOf,
    landedOutcome: 'REGISTERED',
    onLanded,
  });
  const { draft, patch, problems, locked } = form;
  useEffect(() => {
    onUnsavedChange(form.unsaved);
  }, [onUnsavedChange, form.unsaved]);
  useEffect(() => () => onUnsavedChange(false), [onUnsavedChange]);
  const advancedOpen = advancedGroupOpen(null, problems, advancedPaths, false);
  const advancedSummary = `生效时间 ${draft.effectiveFrom.trim().replace('T', ' ') || '未填'} · 更新序号 ${
    form.revisionField.value.trim() || '未填'
  }${form.revisionField.edited ? '（手填）' : '（建议值）'}`;
  const renderFooter = (node: ReactNode) => (footerSlot ? createPortal(node, footerSlot) : node);
  // 名称与识别信息都没变就不登新版（票 company-subject-section-edit/06）。
  const changed = draft.name !== row.partyName || identificationChanged(row, draft);
  const correcting = registeredNumbersChanged(row, draft);

  return (
    <div className="flex flex-col gap-4">
      <p className="rounded-md border border-idpxyz-border bg-idpxyz-sidebar px-3 py-2 type-body leading-5 text-idpxyz-text">
        更正后登记为新的一版（当前 {updateOrdinalLabel(row.revision)}），历次记录都保留；编码不能改——编码变了就是另一家单位。
      </p>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="编码" path="businessParties[0].partyId" problems={problems}>
          <Input value={draft.partyId} readOnly className="font-mono type-value text-idpxyz-breadcrumb" aria-label="编码" />
        </Field>
        <Field label="名称 *" path="businessParties[0].name" problems={problems}>
          <Input
            value={draft.name}
            readOnly={locked}
            className="type-value"
            aria-label="名称"
            onChange={(event) => patch({ name: event.target.value })}
          />
        </Field>
        <Field label="来源单据 *" path="businessParties[0].basis" problems={problems}>
          <Input
            value={draft.basis}
            readOnly={locked}
            className="font-mono type-value"
            placeholder="合同编号、邮件或审批单号"
            aria-label="来源单据"
            onChange={(event) => patch({ basis: event.target.value })}
          />
          <span className="mt-1 block type-body text-idpxyz-breadcrumb">这次更正依据的是哪份单据，方便以后追溯。</span>
        </Field>
      </div>
      <FormGroup
        title="识别信息"
        description="注册号用来认出同一家公司有没有被登过两次；外部系统编码用来与财务等系统对上。都可以不填。"
      >
        <BusinessPartyIdentificationFields
          draft={draft}
          patch={patch}
          problems={problems}
          locked={locked}
          ourCompany={ours}
          correction={
            correcting
              ? { note: '已登的注册号改了或去掉了：号真的变了就是另一家单位，应停用本单位、另登新单位；是当初录错了，才在这里填更正依据。' }
              : undefined
          }
        />
      </FormGroup>
      <AdvancedGroup open={advancedOpen} onToggle={() => undefined} summary={advancedSummary}>
        <WallTimeField
          label="生效时间 *"
          path="businessParties[0].effectiveFrom"
          problems={problems}
          value={draft.effectiveFrom}
          locked={locked}
          timeZone={timeZone}
          note="默认是打开这张表单的时刻；可以改到未来。"
          onChange={(effectiveFrom) => patch({ effectiveFrom })}
        />
        <RevisionField
          label="更新序号"
          path="businessParties[0].revision"
          problems={problems}
          {...form.revisionField}
          note="系统按已登记的记录自动算好，一般不用改。保存时提示序号对不上，说明列表已过时，刷新后再试。"
        />
      </AdvancedGroup>
      {renderFooter(
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="min-w-0 flex-1 type-body">
            <RegistrationAnswerNote state={form.state} owner={info.owner} outcomeLabels={partyIdentityOutcomeLabels} problemNote={problemNote} />
            {!changed && form.state.kind === 'idle' ? <p className="text-idpxyz-breadcrumb">名称与识别信息都没有改动，不用保存。</p> : null}
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button variant="outline" disabled={locked} onClick={onCancel}>
              取消
            </Button>
            <Button variant="default" className={primaryActionClass} onClick={form.send} disabled={locked || !form.canSend || !changed}>
              {locked ? '保存中…' : '保存'}
            </Button>
          </div>
        </div>,
      )}
    </div>
  );
}
