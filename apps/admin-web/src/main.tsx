import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import './index.css';
import App from './App';
import { AuthGate } from './auth/AuthGate';
import { configureBearerSource } from './auth/bearer';
import { accessTokenForRequest } from './auth/oidc';
// 从各域的 api 模块取，不经 barrel：入口静态引 barrel 会把那个域的页面钉回入口块（懒加载的理由见 page-registry）。
import { configureShipmentRequestApi } from './pages/shipment-request/api';
import { configureVisibilityApi } from './pages/visibility/api';
import { configureMasterDataApi } from './pages/catalogue-api';
import { configureDisplayTimeZone, resolveDisplayTimeZone } from './pages/moment';
import { configureVocabularyWarnings } from './vocabulary/status';

// 前端一律带 /api 前缀发起，开发代理剥前缀转发（见 vite.config.ts 的裁决注释）。
configureShipmentRequestApi({ basePrefix: '/api' });
configureVisibilityApi({ basePrefix: '/api' });
configureMasterDataApi({ basePrefix: '/api' });
// 各出口的请求都带登录门那份会话的访问令牌（ADR-0100 决定二第一条）。
configureBearerSource(accessTokenForRequest);
// 时刻按 VITE_DISPLAY_TIME_ZONE（未配则操作者浏览器所在区）显示、带显式偏移；只在这里配一次
// （moment.ts 未配置即 UTC，Node 里的测试因此不随机器时区变）。
configureDisplayTimeZone(
  resolveDisplayTimeZone(import.meta.env.VITE_DISPLAY_TIME_ZONE, Intl.DateTimeFormat().resolvedOptions().timeZone),
);
// 用语词典查不到的码只在开发模式告警（每码一次），生产不注入——词典本身不读环境，理由见 vocabulary/status.ts 文件头。
if (import.meta.env.DEV) {
  configureVocabularyWarnings((code) => console.warn(`用语词典没有「${code}」的中文，界面上原样显示了这个码；请补进 src/vocabulary/status.ts`));
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <AuthGate>
      <App />
    </AuthGate>
  </StrictMode>,
);
