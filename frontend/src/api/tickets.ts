import { get, post, patch, del } from '@/utils/request'

// 工单接口，形状对应 internal/types/ticket.go。

export type TicketStatus = 'pending' | 'in_progress' | 'resolved'

export interface Ticket {
  id: string
  tenant_id?: number
  channel_id?: string
  platform?: string
  chat_id?: string
  session_id?: string
  /** 提交时刻的问题快照（原文 Markdown） */
  question: string
  /** 提交时刻的回答快照（原文 Markdown） */
  answer: string
  /** IM 平台侧用户标识，不是 WeKnora 账号 id */
  submitter_id: string
  submitter_name: string
  /** /sub 后面跟的提交理由，由提交人写 */
  submit_reason: string
  status: TicketStatus
  status_updated_at: string | null
  status_updated_by: string
  status_updated_by_name: string
  /** 管理员备注，只在详情里展示 */
  note: string
  created_at: string
  updated_at: string
}

export interface TicketListQuery {
  page?: number
  page_size?: number
  keyword?: string
  /** 空字符串表示不限状态 */
  status?: TicketStatus | ''
}

export interface TicketListData {
  items: Ticket[]
  total: number
  page: number
  page_size: number
}

export function listTickets(params: TicketListQuery = {}) {
  return get<TicketListData>('/api/v1/tickets', { params })
}

export function getTicket(id: string) {
  return get<Ticket>(`/api/v1/tickets/${id}`)
}

export function updateTicket(id: string, payload: { status?: TicketStatus; note?: string }) {
  return patch<Ticket>(`/api/v1/tickets/${id}`, payload)
}

export function deleteTicket(id: string) {
  return del<{ deleted: number }>(`/api/v1/tickets/${id}`)
}

export function batchDeleteTickets(ids: string[]) {
  return post<{ deleted: number }>('/api/v1/tickets/batch-delete', { ids })
}
