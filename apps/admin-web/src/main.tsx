import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import './index.css';
import App from './App';
import { AuthGate } from './auth/AuthGate';
import { configureShipmentRequestApi } from './pages/shipment-request';
import { configureVisibilityApi } from './pages/visibility';
import { configureMasterDataApi } from './pages/catalogue-api';
import { configureDisplayTimeZone } from './pages/moment';

// 前端一律带 /api 前缀发起，开发代理剥前缀转发（见 vite.config.ts 的裁决注释）。
configureShipmentRequestApi({ basePrefix: '/api' });
configureVisibilityApi({ basePrefix: '/api' });
configureMasterDataApi({ basePrefix: '/api' });
// 时刻按操作者所在时区显示、带显式偏移；只有浏览器知道这一维，所以只在这里配一次
// （moment.ts 未配置即 UTC，Node 里的测试因此不随机器时区变）。
configureDisplayTimeZone(Intl.DateTimeFormat().resolvedOptions().timeZone);

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <AuthGate>
      <App />
    </AuthGate>
  </StrictMode>,
);
