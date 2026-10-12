import { LogIn } from 'lucide-react';
import { beginLogin, currentLocation } from './oidc';

const sizeClass = {
  // 页级状态卡里那一颗，与错误态的「重试」同形。
  block:
    'inline-flex items-center gap-1.5 px-3 py-1.5 text-[12px] rounded border border-idpxyz-border text-idpxyz-text hover:bg-idpxyz-hover',
  // 接在一句说明末尾的那一颗。
  inline:
    'ml-2 inline-flex items-center gap-1 px-2 py-0.5 text-[11px] rounded border border-idpxyz-border text-idpxyz-text hover:bg-idpxyz-hover align-middle',
};

/**
 * 「需要登录」那一格唯一的出路：重走一遍登录，回来落在此刻这一页（currentLocation 带 hash）。
 *
 * 不在答 401 时自动跳走：页上可能有填了一半的表单，跳走就丢了；由人按。令牌只在内存，跳走之后旧的那枚随页面一起没了，
 * 不必先清。
 */
export function ReloginButton({ size = 'block' }: { size?: keyof typeof sizeClass }) {
  return (
    <button type="button" className={sizeClass[size]} onClick={() => void beginLogin(currentLocation())}>
      <LogIn className={size === 'block' ? 'h-3.5 w-3.5' : 'h-3 w-3'} aria-hidden />
      重新登录
    </button>
  );
}
