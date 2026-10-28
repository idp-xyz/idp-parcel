import { CircleDashed } from 'lucide-react';
import { moduleInfoById } from '../navigation';
import { TechnicalDetails } from '../components/TechnicalDetails';

// 未接线模块的统一占位页：先说这页用来做什么（summary），再说为何还没有数据。主责上下文与场景出处是给排查与对接的人看的，
// 收进「技术详情」折叠。不模拟列表或表单——那会把「看起来能用」误当成「已交付」。
export function UnwiredModule({ moduleId }: { moduleId: string }) {
  const info = moduleInfoById[moduleId];
  if (!info) return null;

  return (
    <div className="flex-1 flex items-center justify-center">
      <div className="max-w-[560px] px-6">
        <div className="flex items-center gap-2 mb-3">
          <CircleDashed className="h-5 w-5 text-idpxyz-textMuted" />
          <span className="text-[18px] text-idpxyz-textBright">{info.title}</span>
          <span className="text-[11px] px-2 py-0.5 rounded border border-idpxyz-border text-idpxyz-textMuted">
            尚未开放
          </span>
        </div>
        <p className="text-[13px] leading-6 text-idpxyz-text">{info.summary}</p>
        <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-3">
          这页的数据入口还没有开放：不发请求、不显示任何未经确认的默认值。开放后由对应模块供数。
        </p>
        <TechnicalDetails
          className="mt-4 text-idpxyz-textMuted"
          rows={[
            { label: '主责上下文', value: info.owner },
            { label: '场景出处', value: info.source },
          ]}
        />
      </div>
    </div>
  );
}
