import api from './api';

export interface ConversationMessage {
  id: string;
  role: 'user' | 'assistant' | 'system';
  content: string;
  tokens?: number;
  created_at: string;
  attachments?: Array<{ id: string; name: string; url: string }>;
  metadata?: Record<string, unknown>;
}

export interface Conversation {
  id: string;
  agent_package_code: string;
  title: string;
  status: 'active' | 'archived';
  last_message_at?: string;
  created_at: string;
  updated_at: string;
  messages?: ConversationMessage[];
}

export interface ConversationListResponse {
  data: Conversation[];
  pagination?: { total: number; page: number; page_size: number };
}

export interface ClarificationSchema {
  type: 'object';
  properties: Record<string, {
    type: 'string' | 'number' | 'boolean';
    title?: string;
    description?: string;
    enum?: (string | number)[];
    default?: unknown;
    minimum?: number;
    maximum?: number;
    minLength?: number;
    maxLength?: number;
    format?: 'textarea' | 'date' | 'checkbox' | 'attachment';
  }>;
  required?: string[];
}

export interface ClarificationRequest {
  id: string;
  conversation_id: string;
  schema: ClarificationSchema;
  status: 'pending' | 'answered' | 'cancelled';
  created_at: string;
}

export interface ApprovalRequest {
  id: string;
  conversation_id: string;
  title: string;
  description?: string;
  tool_name: string;
  status: 'pending' | 'approved' | 'rejected';
  created_at: string;
}

export function createConversation(agent_package_code: string, title?: string) {
  return api.post('/conversations', { agent_package_code, title });
}

export function listConversations(group_by?: string) {
  return api.get('/conversations', { params: group_by ? { group_by } : {} });
}

export function getConversation(id: string) {
  return api.get(`/conversations/${encodeURIComponent(id)}`);
}

export function updateConversation(id: string, data: Partial<Pick<Conversation, 'title' | 'status'>>) {
  return api.patch(`/conversations/${encodeURIComponent(id)}`, data);
}

export function sendMessage(conversationId: string, content: string, attachments?: string[]) {
  return api.post(`/conversations/${encodeURIComponent(conversationId)}/messages`, {
    content,
    attachments: attachments || [],
  });
}

export function answerClarification(conversationId: string, interruptId: string, answer: Record<string, unknown>) {
  return api.post(`/conversations/${encodeURIComponent(conversationId)}/answers`, {
    interrupt_id: interruptId,
    answer,
  });
}

export function cancelRun(conversationId: string) {
  return api.post(`/conversations/${encodeURIComponent(conversationId)}/cancel`);
}