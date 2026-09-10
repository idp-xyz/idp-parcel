import { useState } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import { PublicationDraftFlow, type PublicationFormContext } from './PublicationDraftFlow';
import { DeliveryConditionFields } from './DeliveryConditionFields';
import { Field, Problems, fieldLabel } from './PublicationFormFields';
import type { DeliveryConditionDraft } from './delivery-condition-section';
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
 * 时刻按 RFC 3339 原样送、不在本地补零点也不换时区：补哪个时区的零点是一条规则，表单不替操作者定；格式
 * 立不立得住由服务端在那一格上答。
 */
export interface ServiceProductPublicationFormProps {
  /** 载体到达「发布」那一步时回调，页面借它刷同页的目录读面。 */
  onPublished?: () => void;
}

export function ServiceProductPublicationForm({ onPublished }: ServiceProductPublicationFormProps) {
  const [draft, setDraft] = useState<ServiceProductDraft>(emptyServiceProductDraft());
  const patch = (change: Partial<ServiceProductDraft>) => setDraft((current) => ({ ...current, ...change }));
  const patchRow = (index: number, change: Partial<ReferenceRowDraft>) =>
    patch({ references: draft.references.map((row, at) => (at === index ? { ...row, ...change } : row)) });
  const patchDeliveryConditions = (change: Partial<DeliveryConditionDraft>) =>
    setDraft((current) => ({ ...current, deliveryConditions: { ...current.deliveryConditions, ...change } }));

  return (
    <PublicationDraftFlow
      kind="SERVICE_PRODUCT"
      title="发布服务产品版本"
      assemblePayload={() => serviceProductPayloadOf(draft)}
      fieldPaths={serviceProductFieldPaths(draft)}
      localProblems={serviceProductLocalProblems(draft)}
      onPublished={onPublished}
    >
      {(form) => (
        <div className="flex flex-col gap-5">
          <ServiceProductFields
            draft={draft}
            form={form}
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
  onPatch,
  onPatchRow,
  onAddRow,
  onRemoveRow,
}: {
  draft: ServiceProductDraft;
  form: PublicationFormContext;
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

  return (
    <div className="flex flex-col gap-4">
      <p className="text-xs text-idpxyz-textMuted">
        本册发布的是<strong>商业版本壳</strong>：给合同、接单规则包、价格政策一个可引用的版本身份。产品属性与渠道
        映射走「登记服务形态」签与渠道产品目录页，不在这里；壳之外唯一可带的正文是下面那一节产品层交付条件，可缺。
        哪几格立不住由预览答，答什么显什么。
      </p>

      <div className="grid grid-cols-2 gap-3">
        {field('objectId', '对象标识 *', '服务产品的对象标识')}
        {field('version', '版本号 *', '同一对象下唯一；同版本号换内容是冲突，不是覆盖')}
        {field('scope', '范围引用 *', '商业范围引用')}
        {field('effectiveStartsAt', '生效起点 *', 'RFC 3339，如 2026-08-01T00:00:00Z')}
        {field('effectiveEndsAt', '生效止点', '留空即无上界')}
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
        <span className={fieldLabel}>指名引用（被引对象类别 → 对象标识；可空）</span>
        <Button variant="outline" disabled={locked} onClick={onAdd}>
          加一行
        </Button>
      </div>
      <p className="text-[11px] text-idpxyz-textMuted mb-2">
        键填被引对象类别的原词、值填该对象的标识；两格全空的行不进载荷。键是不是集合内的词、被引对象发布了没有，
        都由服务端答——前者预览时逐格点名，后者发布时答「发布未决」。
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
