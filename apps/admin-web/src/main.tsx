import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import './index.css';
import App from './App';
import { configureShipmentRequestApi } from './pages/shipment-request';

// 前端一律带 /api 前缀发起，开发代理剥前缀转发（见 vite.config.ts 的裁决注释）。
configureShipmentRequestApi({ basePrefix: '/api' });

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
