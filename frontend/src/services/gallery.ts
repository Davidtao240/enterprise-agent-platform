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

export function createPackageVersion(code: string, data: CreateVersionRequest) {
  return api.post(`/agent-packages/${encodeURIComponent(code)}/versions`, data);
}

export function listPackageVersions(code: string) {
  return api.get(`/agent-packages/${encodeURIComponent(code)}/versions`);
}

export function publishPackageVersion(code: string, version: string) {
  return api.post(`/agent-packages/${encodeURIComponent(code)}/versions/${encodeURIComponent(version)}/publish`);
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

export function listInstalledPackages(params: ListInstalledParams = {}) {
  return api.get('/agent-package-installations', { params });
}

export function installPackage(data: InstallPackageRequest) {
  return api.post('/agent-package-installations', data);
}

export function uninstallPackage(code: string) {
  return api.post(`/agent-package-installations/${encodeURIComponent(code)}/uninstall`);
}

export function updateInstallationStatus(code: string, status: string) {
  return api.patch(`/agent-package-installations/${encodeURIComponent(code)}`, { status });
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

export function registerPackage(data: RegisterPackageRequest) {
  return api.post('/agent-package-registrations', data);
}

export function listPackageRegistrations(params: ListRegistrationsParams = {}) {
  return api.get('/agent-package-registrations', { params });
}

export function getPackageRegistration(code: string) {
  return api.get(`/agent-package-registrations/${encodeURIComponent(code)}`);
}

export function verifyRegistration(code: string) {
  return api.post(`/agent-package-registrations/${encodeURIComponent(code)}/verify`);
}

export function rejectRegistration(code: string) {
  return api.post(`/agent-package-registrations/${encodeURIComponent(code)}/reject`);
}