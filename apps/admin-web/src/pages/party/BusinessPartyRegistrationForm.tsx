import { useEffect, useState } from 'react';
import { Building2, Receipt } from 'lucide-react';
import { Button, Input } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { RegistrationAnswerNote } from '../../components/registration';
import { partyIdentityOutcomeLabels, type BusinessPartyRecord } from './api';
import { problemNote } from './presentation';
import { Field } from './PublicationFormFields';
import { AdvancedGroup, FormGroup, RevisionField, SnapshotJsonDetails, WallTimeField, useRegistrationForm } from './party-registration-fields';
import { advancedGroupOpen } from './registration-form';
import {
  businessPartyLocalProblems,
  businessPartyPayloadOf,
  emptyBusinessPartyDraft,
  identificationChanged,
  registeredNumbersChanged,
  suggestedBusinessPartyRevision,
  type BusinessPartyDraft,
} from './business-party-form';
import { EditSectionCard } from './BusinessPartyEditShell';
import { BusinessPartyIdentificationFields } from './BusinessPartyIdentificationFields';
import { BusinessPartySimilarParties } from './BusinessPartySimilarParties';
import './business-party-create.css';

/**
 * 「新增往来单位」表单（票 admin-web-group-legal-entities/10 第 1 条；ADR-0101 决定八自裁：格少、低频、无矩阵，直接逐字段表单）。
 * 壳在 useRegistrationForm，这里只摆本册的格。
 *
 * 表单只问业务要填的：名称、编码、来源单据、生效时间。修订号由表单按已取回的册算好、收在「高级」里（有问题时自动展开），
 * 不再当成一个要人填的必填项——运营不该自己数「第几次更新」。
 *
 * **本组件不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）：编码与名称是否为空、修订号连不连续，
 * 一律原样送上去让服务端逐格答，答回来的问题码经 field-problem-text.ts 译成「哪里错、怎么改」；本地只拦编码层（修订号编
 * 不进正整数、生效时刻换不成 RFC 3339），纯函数在 business-party-form.ts。**不带租户格**、**不裁首尾空白**，理由在那个文件头上。
 *
 * 版式（票 business-party-redesign/11）：各组是一张张带标题的卡，与编辑态的卡同一种长相（BusinessPartyEditShell.tsx 的
 * EditSectionCard）；旁边一栏随名称与编码的输入列出名录里相近的单位（BusinessPartySimilarParties.tsx），只提醒、不拦保存；
 * 「取消」「保存」在底部一条固定的动作条里，表单滚到哪儿都够得着。排法在 business-party-create.css。
 */
export interface BusinessPartyRegistrationFormProps {
  /** 页面已取回的参与方列表（给修订号建议用）；列表没取到传 null，建议一律为 1。 */
  knownParties: readonly BusinessPartyRecord[] | null;
  /**
   * 登记册答 `REGISTERED` 时回调，页面借它重取列表让新行在「往来单位」册立刻可见。逐字段那条路把送出的草稿交回，
   * 页面据它选中刚登的那一行（票 business-party-ux/06）；JSON 镜像那条路交 null。
   */
  onRegistered: (sent: BusinessPartyDraft | null) => void;
  /** 草稿有没有没提交的输入；页面据此决定离开登记视图前要不要先确认。卸载时报 false。 */
  onUnsavedChange?: (unsaved: boolean) => void;
  /** 动作条里的「取消」：离开登记视图。有没提交的输入时要不要先问由页面管，这里只报「人点了取消」。 */
  onCancel: () => void;
}

const info = moduleInfoById['business-parties'];
const kind = 'business-party';
const advancedPaths = ['businessParties[0].revision'] as const;

export function BusinessPartyRegistrationForm({
  knownParties,
  onRegistered,
  onUnsavedChange,
  onCancel,
}: BusinessPartyRegistrationFormProps) {
  const form = useRegistrationForm<BusinessPartyDraft>({
    kind,
    empty: emptyBusinessPartyDraft,
    suggestion: (draft) => suggestedBusinessPartyRevision(knownParties, draft.partyId),
    localProblems: businessPartyLocalProblems,
    payloadOf: businessPartyPayloadOf,
    // 答 `REGISTERED`（PartyRegistryOutcome 里 PartyIdentityRegistered 的线上名）才重取列表。
    landedOutcome: 'REGISTERED',
    onLanded: onRegistered,
  });
  const { draft, patch, problems, locked, timeZone, unsaved } = form;
  useEffect(() => {
    onUnsavedChange?.(unsaved);
  }, [onUnsavedChange, unsaved]);
  useEffect(() => () => onUnsavedChange?.(false), [onUnsavedChange]);

  // 按原串找而不裁空白：载荷也不裁，" X" 送上去就是另一个身份，这里若裁了就会把它说成「已在册」。
  const known = knownParties?.find((row) => row.partyId === draft.partyId);
  // 高级组：null 即操作者还没点过，按 advancedGroupOpen 的缺省走（编码已在册时展开——这一笔是更新，让人看见序号）。
  const [advancedChoice, setAdvancedChoice] = useState<boolean | null>(null);
  const advancedOpen = advancedGroupOpen(advancedChoice, problems, advancedPaths, known !== undefined);
  const advancedSummary = form.revisionField.edited
    ? `更新序号改为 ${form.revisionField.value}`
    : known
      ? `这是「${known.partyName}」的第 ${form.revisionField.suggestion} 次更新，序号已算好`
      : '首次登记，序号由系统填';

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="business-party-create-body min-h-0 flex-1 overflow-y-auto">
        <div className="business-party-create-wrap">
          {/* 字段的来历都说在这一段里，格下只留会改变操作的那一句（票 business-party-ux/06）——每格下面都挂一两句解释，整张表读起来像文档。 */}
          <p className="rounded-md border border-idpxyz-border bg-idpxyz-sidebar px-3 py-2 type-body leading-5 text-idpxyz-text">
            登记一家新的往来单位：名称、内部编码（建议用单位简称）、这次登记依据的单据，以及从何时起可以与它建立合作关系。要更正已登记单位的名称或
            识别信息，到它的对象页用「更正登记信息」，那里会带上它已登的内容。
          </p>

          <EditSectionCard id="new-identity" icon={Building2} title="单位身份" note="编码登记后不能改">
            <FormGroup title="单位身份" titleHidden>
              <Field label="名称 *" path="businessParties[0].name" problems={problems}>
                <Input
                  value={draft.name}
                  readOnly={locked}
                  className="type-value"
                  placeholder="如 深圳某某跨境电商有限公司"
                  onChange={(event) => patch({ name: event.target.value })}
                />
              </Field>

              <Field label="编码 *" path="businessParties[0].partyId" problems={problems}>
                <Input
                  value={draft.partyId}
                  readOnly={locked}
                  className="font-mono type-value"
                  placeholder="如 SHIPPER-01"
                  onChange={(event) => patch({ partyId: event.target.value })}
                />
                <span className="block type-body text-idpxyz-breadcrumb mt-1">
                  {known ? `这个编码已登记为「${known.partyName}」；这次保存会更新它的名称或来源单据。` : '登记后不能改；前后不要带空格。'}
                </span>
              </Field>

              <Field label="来源单据 *" path="businessParties[0].basis" problems={problems}>
                <Input
                  value={draft.basis}
                  readOnly={locked}
                  className="font-mono type-value"
                  placeholder="合同编号、邮件或审批单号"
                  onChange={(event) => patch({ basis: event.target.value })}
                />
              </Field>

              <WallTimeField
                label="生效时间 *"
                path="businessParties[0].effectiveFrom"
                problems={problems}
                value={draft.effectiveFrom}
                locked={locked}
                timeZone={timeZone}
                onChange={(effectiveFrom) => patch({ effectiveFrom })}
                note="填未来的时间就先显示为待生效。"
              />
            </FormGroup>

            <AdvancedGroup open={advancedOpen} onToggle={() => setAdvancedChoice(!advancedOpen)} summary={advancedSummary}>
              <RevisionField
                label="更新序号"
                path="businessParties[0].revision"
                problems={problems}
                {...form.revisionField}
                note="系统按已登记的记录自动算好，一般不用改。保存时提示序号对不上，说明列表已过时，刷新后再试。"
              />
            </AdvancedGroup>
          </EditSectionCard>

          {/* 在文档里紧跟「单位身份」：只有一列时它就在人正在填的名称与编码下面；宽处由样式表挪到右边一栏。 */}
          <BusinessPartySimilarParties
            name={draft.name}
            partyId={draft.partyId}
            className="business-party-create-similar"
          />

          <EditSectionCard id="new-identification" icon={Receipt} title="识别信息" note="选填，以后可以在「更正登记信息」里补">
            <FormGroup
              title="识别信息（选填）"
              titleHidden
              description="注册号用来认出同一家公司有没有被登过两次；外部系统编码用来与财务等系统对上。"
            >
              <BusinessPartyIdentificationFields
                draft={draft}
                patch={patch}
                problems={problems}
                locked={locked}
                notice={
                  known && identificationChanged(known, draft)
                    ? '这个编码已登记过，它已登的注册号或外部编码与本表单不同：保存后以本表单为准，没填的就去掉了。'
                    : undefined
                }
                correction={
                  known && registeredNumbersChanged(known, draft)
                    ? { note: '这个编码已登过注册号，这次没有原样带上：要保留就在对象页用「更正登记信息」；是当初录错了，才在这里填更正依据。' }
                    : undefined
                }
              />
            </FormGroup>
          </EditSectionCard>

          <SnapshotJsonDetails kind={kind} submit={form.submit} />
        </div>
      </div>

      <footer className="business-party-create-footer shrink-0 border-t border-idpxyz-border bg-idpxyz-bg">
        <div className="business-party-create-footer-inner flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
          <div className="min-w-0 flex-1 type-body">
            <RegistrationAnswerNote
              state={form.state}
              owner={info.owner}
              outcomeLabels={partyIdentityOutcomeLabels}
              problemNote={problemNote}
            />
            {form.state.kind === 'idle' ? <p className="text-idpxyz-breadcrumb">保存后回到名录，并打开这家单位。</p> : null}
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button variant="outline" disabled={locked} onClick={onCancel}>
              取消
            </Button>
            <Button variant="default" className={primaryActionClass} onClick={form.send} disabled={locked || !form.canSend}>
              {locked ? '保存中…' : '保存'}
            </Button>
          </div>
        </div>
      </footer>
    </div>
  );
}
