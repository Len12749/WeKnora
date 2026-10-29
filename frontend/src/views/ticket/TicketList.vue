<template>
  <div class="ticket-page">
    <header class="ticket-header">
      <div>
        <h2 class="ticket-title">{{ t('ticket.title') }}</h2>
        <p class="ticket-desc">{{ t('ticket.description') }}</p>
      </div>
      <t-button variant="outline" :loading="loading" @click="reload">
        <template #icon><t-icon name="refresh" /></template>
        {{ t('ticket.refresh') }}
      </t-button>
    </header>

    <div class="ticket-toolbar">
      <t-input
        v-model="keyword"
        class="ticket-search"
        :placeholder="t('ticket.searchPlaceholder')"
        clearable
        @enter="applyFilters"
        @clear="applyFilters"
      >
        <template #prefix-icon><t-icon name="search" /></template>
      </t-input>

      <t-select
        v-model="statusFilter"
        class="ticket-status-filter"
        :options="filterStatusOptions"
        @change="applyFilters"
      />

      <t-button theme="primary" @click="applyFilters">{{ t('ticket.search') }}</t-button>

      <div class="ticket-toolbar-spacer" />

      <t-button
        v-if="canManage && selectedKeys.length"
        theme="danger"
        variant="outline"
        @click="confirmBatchDelete"
      >
        {{ t('ticket.batchDelete') }}({{ selectedKeys.length }})
      </t-button>
    </div>

    <t-table
      row-key="id"
      :data="rows"
      :columns="columns"
      :loading="loading"
      :selected-row-keys="selectedKeys"
      :empty="t('ticket.empty')"
      size="medium"
      hover
      @select-change="handleSelectChange"
    >
      <template #createdAt="{ row }">
        <span class="ticket-time">{{ formatTime(row.created_at) }}</span>
      </template>

      <template #submitter="{ row }">
        <span :title="row.submitter_id">{{ submitterLabel(row) }}</span>
      </template>

      <template #platform="{ row }">
        <t-tag size="small" variant="light-outline">{{ row.platform || '—' }}</t-tag>
      </template>

      <template #status="{ row }">
        <span class="ticket-badge" :class="`is-${row.status}`">{{ statusLabel(row.status) }}</span>
      </template>

      <template #statusUpdated="{ row }">
        <div class="ticket-meta">
          <span>{{ row.status_updated_at ? formatTime(row.status_updated_at) : '—' }}</span>
          <span v-if="statusActor(row)" class="ticket-meta-sub">{{ statusActor(row) }}</span>
        </div>
      </template>

      <template #actions="{ row }">
        <t-space size="small">
          <t-link theme="primary" underline @click="openDetail(row)">{{ t('ticket.view') }}</t-link>
          <t-link v-if="canManage" theme="danger" underline @click="confirmDelete(row)">
            {{ t('ticket.delete') }}
          </t-link>
        </t-space>
      </template>
    </t-table>

    <div class="ticket-pagination">
      <t-pagination
        :current="page"
        :page-size="pageSize"
        :total="total"
        :page-size-options="[10, 20, 50, 100]"
        show-jumper
        @current-change="handlePageChange"
        @page-size-change="handlePageSizeChange"
      />
    </div>

    <t-drawer
      v-model:visible="detailVisible"
      :header="t('ticket.detailTitle')"
      size="760px"
      :footer="false"
      destroy-on-close
    >
      <div v-if="detail" class="ticket-detail">
        <div class="ticket-detail-meta">
          <span class="ticket-badge" :class="`is-${detail.status}`">{{ statusLabel(detail.status) }}</span>
          <span>{{ t('ticket.submittedAt') }}：{{ formatTime(detail.created_at) }}</span>
          <span :title="detail.submitter_id">{{ t('ticket.submitter') }}：{{ submitterLabel(detail) }}</span>
          <span>{{ t('ticket.source') }}：{{ detail.platform || '—' }}</span>
        </div>

        <section class="ticket-detail-block">
          <div class="ticket-block-head">
            <h4>{{ t('ticket.question') }}</h4>
            <t-button size="extra-small" variant="text" @click="copyText(detail.question)">
              <template #icon><t-icon name="copy" /></template>
              {{ t('ticket.copy') }}
            </t-button>
          </div>
          <pre class="ticket-snapshot">{{ detail.question || '—' }}</pre>
        </section>

        <section class="ticket-detail-block">
          <div class="ticket-block-head">
            <h4>{{ t('ticket.answer') }}</h4>
            <t-button size="extra-small" variant="text" @click="copyText(detail.answer)">
              <template #icon><t-icon name="copy" /></template>
              {{ t('ticket.copy') }}
            </t-button>
          </div>
          <pre class="ticket-snapshot">{{ detail.answer || '—' }}</pre>
        </section>

        <section v-if="detail.submit_reason" class="ticket-detail-block">
          <div class="ticket-block-head">
            <h4>{{ t('ticket.submitReason') }}</h4>
            <t-button size="extra-small" variant="text" @click="copyText(detail.submit_reason)">
              <template #icon><t-icon name="copy" /></template>
              {{ t('ticket.copy') }}
            </t-button>
          </div>
          <pre class="ticket-snapshot">{{ detail.submit_reason }}</pre>
        </section>

        <section class="ticket-detail-block">
          <div class="ticket-block-head">
            <h4>{{ t('ticket.note') }}</h4>
            <t-button size="extra-small" variant="text" @click="copyText(canManage ? draftNote : detail.note)">
              <template #icon><t-icon name="copy" /></template>
              {{ t('ticket.copy') }}
            </t-button>
          </div>
          <t-textarea
            v-if="canManage"
            v-model="draftNote"
            class="ticket-note"
            :autosize="{ minRows: 2, maxRows: 6 }"
            :placeholder="t('ticket.notePlaceholder')"
          />
          <pre v-else class="ticket-snapshot">{{ detail.note || '—' }}</pre>
          <div v-if="canManage" class="ticket-detail-actions">
            <t-button size="small" :loading="savingNote" @click="saveNote">{{ t('ticket.saveNote') }}</t-button>
          </div>
        </section>

        <section v-if="canManage" class="ticket-detail-block">
          <h4>{{ t('ticket.changeStatus') }}</h4>
          <div class="ticket-status-switch" role="group" :aria-label="t('ticket.changeStatus')">
            <button
              v-for="opt in statusOptions"
              :key="opt.value"
              type="button"
              class="ticket-status-pill"
              :class="[`is-${opt.value}`, { 'is-active': draftStatus === opt.value }]"
              @click="saveStatus(opt.value)"
            >
              <span class="ticket-status-dot" />
              {{ opt.label }}
            </button>
          </div>
          <p v-if="detail.status_updated_at" class="ticket-hint">
            {{ t('ticket.lastUpdatedBy', {
              time: formatTime(detail.status_updated_at),
              user: statusActor(detail) || '—',
            }) }}
          </p>
        </section>
      </div>
      <t-loading v-else size="small" />
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { DialogPlugin, MessagePlugin } from 'tdesign-vue-next'
import type { PrimaryTableCol } from 'tdesign-vue-next'

import { useAuthStore } from '@/stores/auth'
import {
  batchDeleteTickets,
  deleteTicket,
  getTicket,
  listTickets,
  updateTicket,
  type Ticket,
  type TicketStatus,
} from '@/api/tickets'

const { t } = useI18n()
const authStore = useAuthStore()

// UI 权限：所有成员可读，管理员及以上才能改状态 / 写备注 / 删除。
// 后端同样会做一次校验，这里只决定按钮是否渲染。
const canManage = computed(() => authStore.hasRole('admin'))

const rows = ref<Ticket[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const keyword = ref('')
const statusFilter = ref<TicketStatus | ''>('')
const selectedKeys = ref<string[]>([])
const loading = ref(false)

const detailVisible = ref(false)
const detail = ref<Ticket | null>(null)
const draftStatus = ref<TicketStatus>('pending')
const draftNote = ref('')
const savingNote = ref(false)

const statusOptions: Array<{ label: string; value: TicketStatus }> = [
  { label: t('ticket.status.pending'), value: 'pending' },
  { label: t('ticket.status.in_progress'), value: 'in_progress' },
  { label: t('ticket.status.resolved'), value: 'resolved' },
]

const filterStatusOptions = computed(() => [{ label: t('ticket.status.all'), value: '' }, ...statusOptions])
const changeStatusOptions = computed(() => statusOptions)

const columns = computed<PrimaryTableCol[]>(() => [
  // 勾选列只给能批量删除的人，普通成员看不到无意义的选择框
  ...(canManage.value ? [{ colKey: 'row-select', type: 'multiple', width: 48 } as PrimaryTableCol] : []),
  { colKey: 'createdAt', title: t('ticket.columns.submittedAt'), width: 170 },
  { colKey: 'submitter', title: t('ticket.columns.submitter'), width: 140 },
  { colKey: 'platform', title: t('ticket.columns.source'), width: 110 },
  { colKey: 'status', title: t('ticket.columns.status'), width: 110 },
  { colKey: 'statusUpdated', title: t('ticket.columns.statusUpdated'), width: 200 },
  { colKey: 'actions', title: t('ticket.columns.actions'), width: 140, fixed: 'right' },
])

function statusLabel(status: TicketStatus) {
  return t(`ticket.status.${status}`)
}

// IM 平台标识 → 中文名，用于把 open_id 之类的一长串美化成「飞书用户·4cea」。
const PLATFORM_LABELS: Record<string, string> = {
  feishu: '飞书',
  lark: '飞书',
  dingtalk: '钉钉',
  wecom: '企业微信',
  slack: 'Slack',
  telegram: 'Telegram',
  mattermost: 'Mattermost',
  qqbot: 'QQ',
}

// 飞书统一显示成「飞书用户·xxxx」：飞书不提供昵称，且未开通通讯录权限，
// 不做"有名字就显示名字"的分支，避免同一个人一会儿有名字一会儿是短码。
function submitterLabel(ticket: Ticket) {
  const platform = (ticket.platform || '').toLowerCase()
  const raw = (ticket.submitter_id || '').trim()
  const isFeishu = platform === 'feishu' || platform === 'lark'
  if (!isFeishu && ticket.submitter_name) {
    return ticket.submitter_name
  }
  if (!raw) {
    return isFeishu ? '飞书用户' : '—'
  }
  const label = PLATFORM_LABELS[platform] || 'IM'
  const cleaned = raw
    .replace(/^ou_/, '')
    .replace(/^on_/, '')
    .replace(/^\$:LWCP_v1:\$/, '')
  const short = cleaned.slice(0, 4)
  return short ? `${label}用户·${short}` : `${label}用户`
}

function statusActor(ticket: Ticket) {
  return ticket.status_updated_by_name || ticket.status_updated_by || ''
}

function formatTime(raw: string | null | undefined) {
  if (!raw) return '—'
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return raw
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

async function load() {
  loading.value = true
  try {
    const res = await listTickets({
      page: page.value,
      page_size: pageSize.value,
      keyword: keyword.value.trim(),
      status: statusFilter.value,
    })
    rows.value = res.data?.items ?? []
    total.value = res.data?.total ?? 0
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('ticket.loadFailed'))
  } finally {
    loading.value = false
  }
}

function reload() {
  void load()
}

function applyFilters() {
  page.value = 1
  selectedKeys.value = []
  void load()
}

function handlePageChange(current: number) {
  page.value = current
  void load()
}

function handlePageSizeChange(size: number) {
  pageSize.value = size
  page.value = 1
  void load()
}

function handleSelectChange(keys: Array<string | number>) {
  selectedKeys.value = keys.map(String)
}

async function openDetail(row: Ticket) {
  detailVisible.value = true
  detail.value = null
  try {
    const res = await getTicket(row.id)
    detail.value = res.data ?? null
    if (detail.value) {
      draftStatus.value = detail.value.status
      draftNote.value = detail.value.note ?? ''
    }
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('ticket.loadFailed'))
  }
}

async function saveStatus(status: TicketStatus) {
  if (!detail.value) return
  try {
    const res = await updateTicket(detail.value.id, { status })
    if (res.data) {
      detail.value = res.data
      draftStatus.value = res.data.status
    }
    MessagePlugin.success(t('ticket.statusSaved'))
    void load()
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('ticket.saveFailed'))
    draftStatus.value = detail.value.status
  }
}

async function saveNote() {
  if (!detail.value) return
  savingNote.value = true
  try {
    const res = await updateTicket(detail.value.id, { note: draftNote.value })
    if (res.data) detail.value = res.data
    MessagePlugin.success(t('ticket.noteSaved'))
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('ticket.saveFailed'))
  } finally {
    savingNote.value = false
  }
}

// 复制某一段内容：问题 / 回答 / 提交理由 / 备注都用同一套。
async function copyText(text: string | null | undefined) {
  const value = (text ?? '').trim()
  if (!value) {
    MessagePlugin.warning(t('ticket.copyEmpty'))
    return
  }
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value)
    } else {
      const helper = document.createElement('textarea')
      helper.value = value
      helper.style.position = 'fixed'
      helper.style.opacity = '0'
      document.body.appendChild(helper)
      helper.select()
      document.execCommand('copy')
      document.body.removeChild(helper)
    }
    MessagePlugin.success(t('ticket.copied'))
  } catch {
    MessagePlugin.error(t('ticket.copyFailed'))
  }
}

function confirmDelete(row: Ticket) {
  const dialog = DialogPlugin.confirm({
    header: t('ticket.deleteTitle'),
    body: t('ticket.deleteConfirm'),
    theme: 'danger',
    onConfirm: async () => {
      try {
        await deleteTicket(row.id)
        MessagePlugin.success(t('ticket.deleted'))
        selectedKeys.value = selectedKeys.value.filter((id) => id !== row.id)
        void load()
      } catch (error: any) {
        MessagePlugin.error(error?.message || t('ticket.deleteFailed'))
      }
      dialog.destroy()
    },
  })
}

function confirmBatchDelete() {
  if (!selectedKeys.value.length) return
  const dialog = DialogPlugin.confirm({
    header: t('ticket.deleteTitle'),
    body: t('ticket.batchDeleteConfirm', { count: selectedKeys.value.length }),
    theme: 'danger',
    onConfirm: async () => {
      try {
        await batchDeleteTickets(selectedKeys.value)
        MessagePlugin.success(t('ticket.deleted'))
        selectedKeys.value = []
        void load()
      } catch (error: any) {
        MessagePlugin.error(error?.message || t('ticket.deleteFailed'))
      }
      dialog.destroy()
    },
  })
}

onMounted(load)
</script>

<style scoped>
.ticket-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 24px;
  height: 100%;
  overflow: auto;
}

.ticket-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.ticket-title {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
}

.ticket-desc {
  margin: 6px 0 0;
  color: var(--td-text-color-secondary);
  font-size: 13px;
}

.ticket-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
}

.ticket-toolbar-spacer {
  flex: 1;
}

.ticket-search {
  width: 320px;
}

.ticket-status-filter {
  width: 160px;
}

.ticket-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 10px;
  border-radius: 999px;
  font-size: 12px;
  line-height: 20px;
  white-space: nowrap;
}

.ticket-badge::before {
  content: '';
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}

.ticket-badge.is-pending {
  color: #ef9d1f;
  background: rgba(239, 157, 31, 0.12);
}

.ticket-badge.is-in_progress {
  color: #0052d9;
  background: rgba(0, 82, 217, 0.1);
}

.ticket-badge.is-resolved {
  color: #2ba471;
  background: rgba(43, 164, 113, 0.12);
}

.ticket-time,
.ticket-meta {
  color: var(--td-text-color-secondary);
  font-size: 13px;
}

.ticket-meta {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.ticket-meta-sub {
  color: var(--td-text-color-placeholder);
  font-size: 12px;
}

.ticket-pagination {
  display: flex;
  justify-content: flex-end;
}

.ticket-detail {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.ticket-detail-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  color: var(--td-text-color-secondary);
  font-size: 13px;
}

.ticket-detail-block h4 {
  margin: 0 0 8px;
  font-size: 14px;
  font-weight: 600;
}

.ticket-snapshot {
  margin: 0;
  padding: 12px;
  border-radius: 8px;
  background: var(--td-bg-color-page);
  font-family: inherit;
  font-size: 13px;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
}

/* 备注框：与只读态的快照块保持同样内边距，未 hover 时也看得见边框 */
.ticket-note :deep(.t-textarea__inner) {
  box-sizing: border-box;
  width: 100%;
  padding: 12px;
  border-color: var(--td-gray-color-4);
  overflow-wrap: anywhere;
}

.ticket-note :deep(.t-textarea__inner:hover) {
  border-color: var(--td-brand-color);
}

.ticket-detail-actions {
  margin-top: 8px;
  display: flex;
  justify-content: flex-end;
}

.ticket-block-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.ticket-block-head h4 {
  margin: 0;
}

.ticket-status-switch {
  display: inline-flex;
  gap: 4px;
  padding: 4px;
  border-radius: 999px;
  background: var(--td-bg-color-secondarycontainer);
}

.ticket-status-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 16px;
  border: none;
  border-radius: 999px;
  background: transparent;
  color: var(--td-text-color-secondary);
  font-size: 13px;
  line-height: 20px;
  cursor: pointer;
  transition: background-color 0.18s ease, color 0.18s ease;
}

.ticket-status-pill:hover {
  background: var(--td-bg-color-container-hover);
}

.ticket-status-pill.is-active {
  color: #fff;
  font-weight: 500;
}

.ticket-status-pill.is-active.is-pending {
  background: #ef9d1f;
}

.ticket-status-pill.is-active.is-in_progress {
  background: #0052d9;
}

.ticket-status-pill.is-active.is-resolved {
  background: #2ba471;
}

.ticket-status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
  opacity: 0.55;
}

.ticket-status-pill.is-active .ticket-status-dot {
  background: #fff;
  opacity: 0.95;
}

.ticket-hint {
  margin: 8px 0 0;
  color: var(--td-text-color-placeholder);
  font-size: 12px;
}
</style>
