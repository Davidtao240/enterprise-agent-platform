import api from './api';

export interface UsageDay {
  date: string;
  conversation_count: number;
  message_count: number;
}

export interface AgentPackageListItem {
  id: string;
  package_code: string;
  name: string;
  description: string;
  category: string;
  business_app_code: string;
  icon?: string;
  status: string;
  usage_count: number;
  last_used_at?: string;
}

export interface AgentPackageDetail extends AgentPackageListItem {
  capabilities?: Record<string, unknown>;
  sample_prompts?: string[];
  total_conversations: number;
  recent_7_days: UsageDay[];
}

export interface GalleryResponse {
  packages: AgentPackageListItem[];
  total: number;
}

export interface GalleryQueryParams {
  category?: string;
  business_app?: string;
  q?: string;
}

export function getAgentGallery(params: GalleryQueryParams = {}) {
  return api.get('/agent-gallery', { params });
}

export function getAgentPackage(code: string) {
  return api.get(`/agent-gallery/${encodeURIComponent(code)}`);
}

export function createAgentPackage(data: Record<string, unknown>) {
  return api.post('/agent-packages', data);
}

export function updateAgentPackage(code: string, data: Record<string, unknown>) {
  return api.patch(`/agent-packages/${encodeURIComponent(code)}`, data);
}