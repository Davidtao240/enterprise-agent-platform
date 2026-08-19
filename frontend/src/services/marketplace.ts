import api from './api';

export interface MarketItem {
  id: string;
  code: string;
  name: string;
  description: string;
  category: string;
  type: 'agent' | 'connector' | 'skill';
  author?: string;
  version: string;
  rating: number;
  usage_count: number;
  installed: boolean;
  status: 'available' | 'installed' | 'disabled';
  install_count: number;
  review_count: number;
  icon?: string;
  tags: string[];
  created_at?: string;
  updated_at?: string;
}

export interface MarketReview {
  id: string;
  item_code: string;
  user_id: string;
  rating: number;
  comment: string;
  created_at: string;
  user_name: string;
}

export interface MarketListItem {
  packages: MarketItem[];
  total: number;
  page: number;
  page_size: number;
}

export interface MarketQueryParams {
  category?: string;
  type?: string;
  q?: string;
  page?: number;
  page_size?: number;
  sort?: 'rating' | 'usage' | 'newest';
}

export async function getMarketItems(params: MarketQueryParams = {}) {
  const res = await api.get('/marketplace', { params });
  return res.data?.data ?? res.data;
}

export async function getMarketItem(code: string) {
  const res = await api.get(`/marketplace/${encodeURIComponent(code)}`);
  return res.data?.data ?? res.data;
}

export async function installMarketItem(code: string, data?: { business_app_code?: string }) {
  const res = await api.post(`/marketplace/${encodeURIComponent(code)}/install`, data || {});
  return res.data?.data ?? res.data;
}

export async function uninstallMarketItem(code: string) {
  const res = await api.post(`/marketplace/${encodeURIComponent(code)}/uninstall`);
  return res.data?.data ?? res.data;
}

export async function rateMarketItem(code: string, data: { rating: number; comment?: string }) {
  const res = await api.post(`/marketplace/${encodeURIComponent(code)}/rate`, data);
  return res.data?.data ?? res.data;
}

export async function getMarketReviews(code: string) {
  const res = await api.get(`/marketplace/${encodeURIComponent(code)}/reviews`);
  return res.data?.data ?? res.data;
}

export async function getConnectorMarket(params: MarketQueryParams = {}) {
  const res = await api.get('/connector-market', { params });
  return res.data?.data ?? res.data;
}

export async function installConnector(code: string) {
  const res = await api.post(`/connector-market/${encodeURIComponent(code)}/install`);
  return res.data?.data ?? res.data;
}

export async function uninstallConnector(code: string) {
  const res = await api.post(`/connector-market/${encodeURIComponent(code)}/uninstall`);
  return res.data?.data ?? res.data;
}