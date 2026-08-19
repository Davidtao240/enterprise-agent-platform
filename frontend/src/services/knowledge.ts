import api from './api';

export interface KBCollection {
  id: string;
  tenant_id: string;
  name: string;
  description?: string;
  embedding_model: string;
  chunk_size: number;
  chunk_overlap: number;
  status: string;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface KBDocument {
  id: string;
  tenant_id: string;
  collection_id?: string;
  file_name: string;
  file_path: string;
  file_size: number;
  mime_type: string;
  status: string;
  chunk_count: number;
  processed_at?: string;
  error_message?: string;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface KBSearchResult {
  chunk_id: string;
  document_id: string;
  file_name: string;
  chunk_index: number;
  content: string;
  score: number;
  collection?: string;
}

export interface CreateCollectionRequest {
  name: string;
  description?: string;
}

export interface SearchRequest {
  query: string;
  top_k?: number;
  min_score?: number;
}

export const knowledgeService = {
  createCollection: async (data: CreateCollectionRequest): Promise<KBCollection> => {
    const res = await api.post('/knowledge/collections', data);
    return res.data?.data ?? res.data;
  },

  listCollections: async (): Promise<KBCollection[]> => {
    const res = await api.get('/knowledge/collections');
    return res.data?.data?.items ?? res.data?.items ?? [];
  },

  getCollection: async (id: string): Promise<KBCollection> => {
    const res = await api.get(`/knowledge/collections/${id}`);
    return res.data?.data ?? res.data;
  },

  deleteCollection: async (id: string): Promise<void> => {
    await api.delete(`/knowledge/collections/${id}`);
  },

  uploadDocument: async (collectionId: string, file: File): Promise<KBDocument> => {
    const formData = new FormData();
    formData.append('file', file);
    const res = await api.post(`/knowledge/collections/${collectionId}/documents`, formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    });
    return res.data?.data ?? res.data;
  },

  listDocuments: async (collectionId: string, status?: string): Promise<KBDocument[]> => {
    const params = status ? { status } : {};
    const res = await api.get(`/knowledge/collections/${collectionId}/documents`, { params });
    return res.data?.data?.items ?? res.data?.items ?? [];
  },

  deleteDocument: async (id: string): Promise<void> => {
    await api.delete(`/knowledge/documents/${id}`);
  },

  search: async (collectionId: string, data: SearchRequest): Promise<KBSearchResult[]> => {
    const res = await api.post(`/knowledge/collections/${collectionId}/search`, data);
    return res.data?.data?.items ?? res.data?.items ?? [];
  },
};