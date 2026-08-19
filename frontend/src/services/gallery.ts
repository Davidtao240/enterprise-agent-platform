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

export async function getAgentGallery(params: GalleryQueryParams = {}) {
  const res = await api.get('/agent-gallery', { params });
  return res.data?.data ?? res.data;
}

export async function getAgentPackage(code: string) {
  const res = await api.get(`/agent-gallery/${encodeURIComponent(code)}`);
  return res.data?.data ?? res.data;
}

export async function createAgentPackage(data: Record<string, unknown>) {
  const res = await api.post('/agent-packages', data);
  return res.data?.data ?? res.data;
}

export async function updateAgentPackage(code: string, data: Record<string, unknown>) {
  const res = await api.patch(`/agent-packages/${encodeURIComponent(code)}`, data);
  return res.data?.data ?? res.data;
}

// ── M8-A: Agent Package Version Management ──

export interface PackageVersion {
  id: string;
  tenant_id: string;
  package_code: string;
  version: string;
  manifest?: Record<string, unknown>;
  graph_key: string;
  graph_version: string;
  entry_type: string;
  status: string;
  is_current: boolean;
  created_by?: string;
  created_at: string;
  updated_at: string;
}

export interface VersionListResponse {
  items: PackageVersion[];
  total: number;
}

export interface CreateVersionRequest {
  version: string;
  graph_key: string;
  graph_version?: string;
  entry_type?: string;
  manifest?: Record<string, unknown>;
}

export async function createPackageVersion(code: string, data: CreateVersionRequest) {
  const res = await api.post(`/agent-packages/${encodeURIComponent(code)}/versions`, data);
  return res.data?.data ?? res.data;
}

export async function listPackageVersions(code: string) {
  const res = await api.get(`/agent-packages/${encodeURIComponent(code)}/versions`);
  return res.data?.data ?? res.data;
}

export async function publishPackageVersion(code: string, version: string) {
  const res = await api.post(`/agent-packages/${encodeURIComponent(code)}/versions/${encodeURIComponent(version)}/publish`);
  return res.data?.data ?? res.data;
}

// ── M8-A: Agent Package Installation Management ──

export interface PackageInstallation {
  id: string;
  tenant_id: string;
  package_code: string;
  installed_version: string;
  installed_at: string;
  installed_by?: string;
  status: string;
  updated_at: string;
}

export interface InstallationResponse {
  installation: PackageInstallation;
  package?: AgentPackageListItem;
  version?: PackageVersion;
}

export interface InstalledListResponse {
  items: InstallationResponse[];
  total: number;
}

export interface ListInstalledParams {
  status?: string;
  category?: string;
  q?: string;
}

export interface InstallPackageRequest {
  package_code: string;
  version?: string;
  graph_key?: string;
  graph_version?: string;
  entry_type?: string;
}

export async function listInstalledPackages(params: ListInstalledParams = {}) {
  const res = await api.get('/agent-package-installations', { params });
  return res.data?.data ?? res.data;
}

export async function installPackage(data: InstallPackageRequest) {
  const res = await api.post('/agent-package-installations', data);
  return res.data?.data ?? res.data;
}

export async function uninstallPackage(code: string) {
  const res = await api.post(`/agent-package-installations/${encodeURIComponent(code)}/uninstall`);
  return res.data?.data ?? res.data;
}

export async function updateInstallationStatus(code: string, status: string) {
  const res = await api.patch(`/agent-package-installations/${encodeURIComponent(code)}`, { status });
  return res.data?.data ?? res.data;
}

// ── M8-A: Agent Package Registration Protocol ──

export interface PackageManifest {
  package_code: string;
  name: string;
  version: string;
  description: string;
  category: string;
  business_app_code: string;
  graph_key: string;
  graph_version: string;
  entry_type: string;
  icon?: string;
  capabilities: Record<string, unknown>;
  sample_prompts: string[];
  author: string;
  license: string;
  min_platform_version: string;
}

export interface PackageRegistration {
  id: string;
  tenant_id: string;
  package_code: string;
  source_type: string;
  source_url?: string;
  manifest?: Record<string, unknown>;
  signature?: string;
  verified: boolean;
  status: string;
  registered_by?: string;
  created_at: string;
  updated_at: string;
}

export interface RegistrationListResponse {
  items: PackageRegistration[];
  total: number;
}

export interface RegisterPackageRequest {
  package_code: string;
  source_type: 'official' | 'third_party' | 'marketplace';
  source_url?: string;
  manifest: PackageManifest;
  signature?: string;
  verify_only?: boolean;
}

export interface ListRegistrationsParams {
  status?: string;
  source_type?: string;
}

export async function registerPackage(data: RegisterPackageRequest) {
  const res = await api.post('/agent-package-registrations', data);
  return res.data?.data ?? res.data;
}

export async function listPackageRegistrations(params: ListRegistrationsParams = {}) {
  const res = await api.get('/agent-package-registrations', { params });
  return res.data?.data ?? res.data;
}

export async function getPackageRegistration(code: string) {
  const res = await api.get(`/agent-package-registrations/${encodeURIComponent(code)}`);
  return res.data?.data ?? res.data;
}

export async function verifyRegistration(code: string) {
  const res = await api.post(`/agent-package-registrations/${encodeURIComponent(code)}/verify`);
  return res.data?.data ?? res.data;
}

export async function rejectRegistration(code: string) {
  const res = await api.post(`/agent-package-registrations/${encodeURIComponent(code)}/reject`);
  return res.data?.data ?? res.data;
}