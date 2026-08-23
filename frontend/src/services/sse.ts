export type SSEEventHandler = (event: SSEEvent) => void;

export interface SSEEvent {
  type: string;
  data: Record<string, unknown>;
  eventId?: string;
}

export interface SSEConnection {
  close: () => void;
  on: (handler: SSEEventHandler) => () => void;
}

const RECONNECT_DELAY_MS = 1000;
const MAX_RECONNECT_DELAY_MS = 30000;
const HEARTBEAT_INTERVAL_MS = 30000;

function getBaseURL(): string {
  const base = import.meta.env.VITE_API_BASE_URL as string | undefined;
  if (!base) return '/api/v1';
  const trimmed = base.replace(/\/+$/, '');
  // 生产配置可能已包含 /api/v1 后缀，避免重复拼接
  return trimmed.endsWith('/api/v1') ? trimmed : `${trimmed}/api/v1`;
}

function getToken(): string | null {
  return localStorage.getItem('token');
}

export function createSSEConnection(conversationId: string, lastEventId?: string): SSEConnection {
  const url = `${getBaseURL()}/conversations/${encodeURIComponent(conversationId)}/stream`;

  const handlers = new Set<SSEEventHandler>();
  let reconnectDelay = RECONNECT_DELAY_MS;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  let closed = false;
  let currentLastEventId = lastEventId;
  let abortController: AbortController | null = null;

  const fireEvent = (event: SSEEvent) => {
    if (event.eventId) {
      currentLastEventId = event.eventId;
    }
    handlers.forEach((h) => {
      try { h(event); } catch { /* ignore handler errors */ }
    });
  };

  const startHeartbeat = () => {
    stopHeartbeat();
    heartbeatTimer = setInterval(() => {
      fireEvent({ type: '__heartbeat', data: {} });
    }, HEARTBEAT_INTERVAL_MS);
  };

  const stopHeartbeat = () => {
    if (heartbeatTimer) {
      clearInterval(heartbeatTimer);
      heartbeatTimer = null;
    }
  };

  const connect = async () => {
    if (closed) return;

    abortController = new AbortController();

    const headers: Record<string, string> = {
      'Accept': 'text/event-stream',
    };
    const token = getToken();
    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }
    if (currentLastEventId) {
      headers['Last-Event-Id'] = currentLastEventId;
    }

    try {
      const response = await fetch(url, {
        method: 'GET',
        headers,
        signal: abortController.signal,
      });

      if (!response.ok || !response.body) {
        fireEvent({ type: 'error', data: { message: `SSE connection failed: ${response.status}` } });
        scheduleReconnect();
        return;
      }

      reconnectDelay = RECONNECT_DELAY_MS;
      startHeartbeat();

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      while (!closed) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const events = buffer.split('\n\n');
        buffer = events.pop() ?? '';

        for (const rawEvent of events) {
          const lines = rawEvent.split('\n');
          const eventIdLine = lines.find((l) => l.startsWith('id:'));
          const eventLine = lines.find((l) => l.startsWith('event:'));
          const dataLine = lines.find((l) => l.startsWith('data:'));

          const eventId = eventIdLine ? eventIdLine.slice(3).trim() : undefined;
          const eventType = eventLine ? eventLine.slice(6).trim() : 'message';

          if (eventId) currentLastEventId = eventId;

          if (dataLine) {
            const dataStr = dataLine.slice(5).trim();
            if (eventType === 'ping') continue;
            try {
              const data = JSON.parse(dataStr);
              fireEvent({ type: eventType, data: data.data ?? data, eventId });
            } catch {
              fireEvent({ type: eventType, data: { raw: dataStr }, eventId });
            }
          }
        }
      }

      if (!closed) {
        scheduleReconnect();
      }
    } catch (err: unknown) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        return;
      }
      if (!closed) {
        fireEvent({ type: 'error', data: { message: 'SSE connection error' } });
        scheduleReconnect();
      }
    }
  };

  const scheduleReconnect = () => {
    if (closed) return;
    if (reconnectDelay >= MAX_RECONNECT_DELAY_MS) {
      fireEvent({ type: 'error', data: { message: 'SSE 连接已断开，请刷新页面重试' } });
      return;
    }
    reconnectTimer = setTimeout(() => {
      if (closed) return;
      connect();
    }, reconnectDelay);
    reconnectDelay = Math.min(reconnectDelay * 2, MAX_RECONNECT_DELAY_MS);
  };

  connect();

  return {
    close() {
      closed = true;
      if (reconnectTimer) {
        clearTimeout(reconnectTimer);
        reconnectTimer = null;
      }
      stopHeartbeat();
      if (abortController) {
        abortController.abort();
        abortController = null;
      }
      handlers.clear();
    },
    on(handler: SSEEventHandler) {
      handlers.add(handler);
      return () => handlers.delete(handler);
    },
  };
}
