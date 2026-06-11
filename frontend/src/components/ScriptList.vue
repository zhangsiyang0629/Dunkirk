<script setup>
import { ref, computed, watch, onMounted } from 'vue'

const props = defineProps({ userId: String })
const emit = defineEmits(['close'])

const scripts = ref([])
const loading = ref(false)
const detail = ref(null)
const deleting = ref(null)
const offset = ref(0)
const limit = 20
const hasMore = ref(false)

onMounted(() => { if (props.userId) fetchScripts() })

watch(() => props.userId, () => {
  offset.value = 0
  scripts.value = []
  if (props.userId) fetchScripts()
})

async function fetchScripts() {
  if (!props.userId) return
  loading.value = true
  try {
    const url = `/api/v1/scripts?offset=${offset.value}&limit=${limit}`
    const res = await fetch(url, { headers: { 'X-User-ID': props.userId } })
    const data = await res.json()
    const items = data.scripts || []
    console.log('scripts data:', items)
    scripts.value = [...scripts.value, ...items]
    hasMore.value = items.length >= limit
  } catch (e) {
    console.error('fetch scripts error:', e)
  }
  loading.value = false
}

function loadMore() {
  offset.value += limit
  fetchScripts()
}

// 按 chapter_idx 分组
const grouped = computed(() => {
  const map = new Map()
  for (const s of scripts.value) {
    const key = `${s.book_ref || ''}-ch${s.chapter_idx}`
    if (!map.has(key)) {
      map.set(key, {
        bookRef: s.book_ref || '',
        chapterIdx: s.chapter_idx,
        topic: s.topic,
        segments: [],
      })
    }
    map.get(key).segments.push(s)
  }
  return Array.from(map.values()).sort((a, b) => {
    if (a.bookRef !== b.bookRef) return a.bookRef < b.bookRef ? -1 : 1
    return a.chapterIdx - b.chapterIdx
  })
})

async function viewScript(hash, bookRef) {
  try {
    const res = await fetch(`/api/v1/scripts/${hash}?book_ref=${encodeURIComponent(bookRef)}`, {
      headers: { 'X-User-ID': props.userId }
    })
    detail.value = await res.json()
  } catch (e) {
    console.error('fetch script error:', e)
  }
}

async function deleteScript(hash, bookRef) {
  if (!confirm('确定删除该脚本？')) return
  deleting.value = hash
  try {
    await fetch(`/api/v1/scripts/${hash}?book_ref=${encodeURIComponent(bookRef)}`, {
      method: 'DELETE',
      headers: { 'X-User-ID': props.userId }
    })
    scripts.value = scripts.value.filter(s => s.hash !== hash)
  } catch (e) {
    console.error('delete script error:', e)
  }
  deleting.value = null
}

function formatTime(t) {
  if (!t) return ''
  return t.slice(0, 19).replace('T', ' ')
}
</script>

<template>
  <div class="script-backdrop" @click.self="$emit('close')">
  <div class="script-panel">
    <div class="panel-header">
      <span>📝 脚本列表</span>
      <button class="close-btn" @click="$emit('close')">✕</button>
    </div>

    <div v-if="loading && scripts.length === 0" class="loading">加载中...</div>
    <div v-else-if="scripts.length === 0" class="empty">暂无脚本</div>

    <div v-else class="script-list">
      <div v-for="group in grouped" :key="group.bookRef + group.chapterIdx" class="chapter-group">
        <div class="chapter-title">{{ group.bookRef ? '📚 ' + group.bookRef : '' }} {{ group.topic }}</div>
        <div v-for="seg in group.segments" :key="seg.hash" class="seg-item">
          <div class="seg-label">第 {{ seg.segment_idx + 1 }} 集</div>
          <div class="seg-meta">
            <span class="time">{{ formatTime(seg.created_at) }}</span>
            <span class="preview">{{ seg.preview }}</span>
          </div>
          <div class="seg-actions">
            <button class="action-btn view" @click="viewScript(seg.hash, seg.book_ref)">查看</button>
            <button class="action-btn delete" :disabled="deleting === seg.hash" @click="deleteScript(seg.hash, seg.book_ref)">
              {{ deleting === seg.hash ? '删除中...' : '删除' }}
            </button>
          </div>
        </div>
      </div>

      <button v-if="hasMore && !loading" class="load-more" @click="loadMore">加载更多</button>
      <div v-if="loading && scripts.length > 0" class="loading">加载中...</div>
    </div>

    <!-- 详情弹窗 -->
    <div v-if="detail" class="modal-overlay" @click.self="detail = null">
      <div class="modal">
        <div class="modal-header">
          <span>📄 {{ detail.topic }} - 第 {{ (detail.segment_idx || 0) + 1 }} 集</span>
          <button class="close-btn" @click="detail = null">✕</button>
        </div>
        <div class="modal-body">
          <pre>{{ detail.content }}</pre>
        </div>
      </div>
    </div>
  </div>
  </div>
</template>

<style scoped>
.script-panel {
  position: fixed;
  top: 0; right: 0;
  height: 100vh; width: 340px;
  padding: 16px;
  overflow-y: auto;
  background: #fafbfc;
  z-index: 50;
  box-shadow: -2px 0 8px rgba(0,0,0,0.1);
}
.script-backdrop {
  position: fixed;
  inset: 0;
  z-index: 49;
}
.panel-header {
  display: flex; justify-content: space-between; align-items: center;
  font-weight: 600; font-size: 14px; margin-bottom: 16px;
}
.close-btn {
  border: none; background: none; cursor: pointer; font-size: 16px; color: #999;
}
.close-btn:hover { color: #333; }
.loading, .empty { text-align: center; color: #999; font-size: 13px; padding: 32px 0; }
.script-list { display: flex; flex-direction: column; gap: 12px; }
.chapter-group { display: flex; flex-direction: column; gap: 6px; }
.chapter-title {
  font-weight: 700; font-size: 14px; color: #1a1a2e;
  padding: 4px 0; border-bottom: 1px solid #e0e0e0;
}
.seg-item {
  background: #fff; border: 1px solid #eee; border-radius: 8px;
  padding: 8px 12px; font-size: 13px; margin-left: 8px;
}
.seg-label { font-weight: 600; color: #1976d2; margin-bottom: 2px; font-size: 12px; }
.seg-meta { color: #888; font-size: 12px; line-height: 1.4; }
.time { display: block; margin-bottom: 2px; }
.preview { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.seg-actions { display: flex; gap: 6px; margin-top: 6px; }
.action-btn {
  padding: 3px 10px; border: 1px solid #ccc; border-radius: 4px;
  background: #fff; cursor: pointer; font-size: 12px;
}
.action-btn.view { color: #1976d2; border-color: #1976d2; }
.action-btn.delete { color: #d32f2f; border-color: #d32f2f; }
.action-btn:disabled { opacity: 0.5; cursor: not-allowed; }
.load-more {
  display: block; width: 100%; padding: 8px;
  border: 1px solid #1976d2; border-radius: 6px;
  background: #fff; color: #1976d2; cursor: pointer; font-size: 13px;
}
.load-more:hover { background: #f0f7ff; }

/* 弹窗 */
.modal-overlay {
  position: fixed; inset: 0; background: rgba(0,0,0,0.4);
  display: flex; align-items: center; justify-content: center; z-index: 100;
}
.modal {
  background: #fff; border-radius: 12px; width: 80%; max-width: 700px;
  max-height: 80vh; display: flex; flex-direction: column; box-shadow: 0 4px 20px rgba(0,0,0,0.15);
}
.modal-header {
  display: flex; justify-content: space-between; align-items: center;
  padding: 14px 20px; border-bottom: 1px solid #eee; font-weight: 600;
}
.modal-body { flex: 1; overflow-y: auto; padding: 16px 20px; }
.modal-body pre {
  white-space: pre-wrap; word-break: break-word;
  font-family: inherit; font-size: 14px; line-height: 1.6; margin: 0;
}
</style>
