import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { ConfigProvider, App as AntdApp } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import App from './App';
import MessageBridge from './components/MessageBridge';
import './styles/global.css';
import { themeConfig } from './styles/theme';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ConfigProvider
      locale={zhCN}
      theme={themeConfig}
    >
      <AntdApp>
        <MessageBridge>
          <BrowserRouter>
            <App />
          </BrowserRouter>
        </MessageBridge>
      </AntdApp>
    </ConfigProvider>
  </React.StrictMode>,
);
