import { useEffect } from 'react';
import { App as AntdApp } from 'antd';
import { setMessageApi } from '../utils/messageApi';

export default function MessageBridge({ children }: { children: React.ReactNode }) {
  const { message } = AntdApp.useApp();

  useEffect(() => {
    setMessageApi(message);
  }, [message]);

  return <>{children}</>;
}
