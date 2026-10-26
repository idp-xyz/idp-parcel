import { useState } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import { currentDisplayTimeZone } from '../moment';
import type { NewVersionStart } from './new-version';
import { PublicationDraftFlow, type PublicationFormContext } from './PublicationDraftFlow';
import { DeliveryConditionFields } from './DeliveryConditionFields';
import { Field, Problems, fieldLabel } from './PublicationFormFields';
import type { DeliveryConditionDraft } from './delivery-condition-section';
import { WallTimeField } from './party-registration-fields';
import {
  emptyReferenceRow,
  emptyServiceProductDraft,
  referencePath,
  serviceProductDeliveryConditionsPath,
  serviceProductFieldPaths,
  serviceProductLocalProblems,
  serviceProductPayloadOf,
  type ReferenceRowDraft,
  type ServiceProductDraft,
} from './service-product-form';

/**
 * 服务产品版本的逐字段表单（票 admin-write-faces/09；ADR-0101 决定八）。五步「表单 → 预览摘要 → 存为待批准 →
 * 批准 → 发布」由 PublicationDraftFlow 走，本组件只摆本册的几格并把草稿组成载荷。
 *
 * **本册的表单是壳加一节可缺的正文。** 对象标识、版本号、范围引用、有效起止，加一张引用表——发布的是给合同、接单
 * 规则包、价格政策引用的**版本身份**；产品属性与渠道映射走「登记服务形态」签与渠道产品目录页那条登记路，
 * 不在这里。壳之外唯一的正文是产品层交付条件一节（票 admin-write-faces/25）：一格都不填即这一版没有交付条件。
 * 摘要与批准照 08 的机制来，表单只呈现服务端答的摘要（伞票 07 硬句）。
 *
 * **引用表是「加一行」不是「从这几个里挑」**（票 09 硬句）：`references` 是开放词汇，键名从哪来、指向什么由发布
 * 用例与领域答；今天服务端没有词表读口，表单不代填键名、不内置词表，集合外的键由预览在对应行上逐格点名。
 *
 * 时刻格按操作者显示时区的本地墙钟填、组载荷时换成 UTC：同叫「生效起点」的格在各张发布表单与登记表单上落在
 * 同一个时刻（票 party-commercial-uiux/10）。
 */
export interface ServiceProductPublicationFormProps {
  /** 载体到达「发布」那一步时回调，页面借它刷同页的目录读面。 */
  onPublished?: () => void;
  /** 基于目录里某一版预填（票 party-commercial-uiux/14）；换一次预填由页面换 key 重建本表单。 */
  start?: NewVersionStart<ServiceProductDraft>;
}

export function ServiceProductPublicationForm({ onPublished, start }: ServiceProductPublicationFormProps) {
  const [draft, setDraft] = useState<ServiceProductDraft>(start?.draft ?? emptyServiceProductDraft());
  const patch = (change: Partial<ServiceProductDraft>) => setDraft((current) => ({ ...current, ...change }));
  const patchRow = (index: number, change: Partial<ReferenceRowDraft>) =>
    patch({ references: draft.references.map((row, at) => (at === index ? { ...row, ...change } : row)) });
  const patchDeliveryConditions = (change: Partial<DeliveryConditionDraft>) =>
    setDraft((current) => ({ ...current, deliveryConditions: { ...current.deliveryConditions, ...change } }));
  const timeZone = currentDisplayTimeZone();

  return (
    <PublicationDraftFlow
      kind="SERVICE_PRODUCT"
      title="发布服务产品版本"
      assemblePayload={() => serviceProductPayloadOf(draft, timeZone)}
      fieldPaths={serviceProductFieldPaths(draft)}
      localProblems={serviceProductLocalProblems(draft, timeZone)}
      onPublished={onPublished}
      prefilledFrom={start?.from}
      prefilledCaution={start?.caution}
    >
      {(form) => (
        <div className="flex flex-col gap-5">
          <ServiceProductFields
            draft={draft}
            form={form}
            timeZone={timeZone}
            onPatch={patch}
            onPatchRow={patchRow}
            onAddRow={() => patch({ references: [...draft.references, emptyReferenceRow()] })}
            onRemoveRow={(index) => patch({ references: draft.references.filter((_, at) => at !== index) })}
          />
          <DeliveryConditionFields
            root={serviceProductDeliveryConditionsPath}
            layer="product"
            draft={draft.deliveryConditions}
            form={form}
            onPatch={patchDeliveryConditions}
          />
        </div>
      )}
    </PublicationDraftFlow>
  );
}

function ServiceProductFields({
  draft,
  form,
  timeZone,
  onPatch,
  onPatchRow,
  onAddRow,
  onRemoveRow,
}: {
  draft: ServiceProductDraft;
  form: PublicationFormContext;
  timeZone: string;
  onPatch: (change: Partial<ServiceProductDraft>) => void;
  onPatchRow: (index: number, change: Partial<ReferenceRowDraft>) => void;
  onAddRow: () => void;
  onRemoveRow: (index: number) => void;
}) {
  const field = (
    path: keyof Omit<ServiceProductDraft, 'references' | 'deliveryConditions'>,
    label: string,
    placeholder: string,
  ) => (
    <Field label={label} path={path} problems={form.problems}>
      <Input
        value={draft[path]}
        readOnly={form.locked}
        className="font-mono text-[13px]"
        placeholder={placeholder}
        onChange={(event) => onPatch({ [path]: event.target.value })}
      />
    </Field>
  );
  const moment = (path: 'effectiveStartsAt' | 'effectiveEndsAt', label: string, note?: string) => (
    <WallTimeField
      label={label}
      path={path}
      problems={form.problems}
      value={draft[path]}
      locked={form.locked}
      timeZone={timeZone}
      note={note}
      onChange={(value) => onPatch({ [path]: value })}
    />
  );

  return (
    <div className="flex flex-col gap-4">
      {/* 本册载荷是版本壳加一节可缺的正文；哪几格立不住由预览逐格答，表单不自判。 */}
      <p className="text-xs text-idpxyz-textMuted">
        这里发布服务产品的<strong>版本信息</strong>
        ，供合同、接单规则包、价格政策引用。产品属性在「登记服务形态」签登记，渠道映射在渠道产品目录页登记，都不在这里。除版本信息外，只能另带下面一节产品层交付条件，可不填。
      </p>

      <div className="grid grid-cols-2 gap-3">
        {field('objectId', '对象标识 *', '服务产品的对象标识')}
        {field('version', '版本号 *', '同一产品下不重复；改内容要用新的版本号')}
        {field('scope', '范围引用 *', '商业范围引用')}
        {moment('effectiveStartsAt', '生效起点 *')}
        {moment('effectiveEndsAt', '生效止点', '留空即持续有效。')}
      </div>

      <ReferencesEditor
        rows={draft.references}
        problems={form.problems}
        locked={form.locked}
        onChange={onPatchRow}
        onAdd={onAddRow}
        onRemove={onRemoveRow}
      />
    </div>
  );
}

/**
 * 指名引用表：每行一个「被引对象类别 → 对象标识」。键由操作者自填，表单不给候选——今天没有词表读口，给一份
 * 就是替租户拟词汇（票 09 硬句）；集合外的键预览会在这一行上点名。同一键两行由本地报出，因为载荷里放不下。
 */
function ReferencesEditor({
  rows,
  problems,
  locked,
  onChange,
  onAdd,
  onRemove,
}: {
  rows: ReferenceRowDraft[];
  problems: Record<string, string[]>;
  locked: boolean;
  onChange: (index: number, change: Partial<ReferenceRowDraft>) => void;
  onAdd: () => void;
  onRemove: (index: number) => void;
}) {
  return (
    <div>
      <div className="flex items-center justify-between mb-1">
        <span className={fieldLabel}>指名引用（被引对象类别 → 对象标识；可不填）</span>
        <Button variant="outline" disabled={locked} onClick={onAdd}>
          加一行
        </Button>
      </div>
      {/* 类别是不是集合内的词由预览逐格点名；被引对象发布了没有由发布那一步答（发布未决）。 */}
      <p className="text-[11px] text-idpxyz-textMuted mb-2">
        左格填被引对象的类别（原词），右格填该对象的标识；两格都空的行不会提交。类别填错时预览会指出；被引对象还没发布时，这一版暂时发布不了，先发布被引对象再来。
      </p>
      {rows.length === 0 ? (
        <p className="text-[11px] text-idpxyz-textMuted">暂无引用。服务产品版本可以不指名任何对象。</p>
      ) : (
        <div className="flex flex-col gap-2">
          {rows.map((row, index) => (
            <div key={index} className="flex flex-col gap-1">
              <div className="grid grid-cols-[1fr_1fr_auto] gap-2 items-center">
                <Input
                  value={row.kind}
                  readOnly={locked}
                  className="font-mono text-[12px]"
                  placeholder="被引对象类别（原词）"
                  onChange={(event) => onChange(index, { kind: event.target.value })}
                />
                <Input
                  value={row.objectId}
                  readOnly={locked}
                  className="font-mono text-[12px]"
                  placeholder="对象标识"
                  onChange={(event) => onChange(index, { objectId: event.target.value })}
                />
                <Button variant="outline" disabled={locked} onClick={() => onRemove(index)}>
                  删
                </Button>
              </div>
              <Problems lines={problems[referencePath(row)]} />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
