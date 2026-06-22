<script setup>
import { ref, computed, watch, nextTick, onMounted } from 'vue'
import { useChat } from './composables/useChat.js'
import FileUpload from './components/FileUpload.vue'
import ChatInput from './components/ChatInput.vue'
import ChatLog from './components/ChatLog.vue'
import ScriptList from './components/ScriptList.vue'

const {
  messages, isStreaming, pendingInterrupt,
  conversations, currentConversationId,
  send, resume,
  loadConversations, selectConversation, newConversation,
} = useChat()

const userId = ref('zsy')
const fileRefId = ref('')
const showFilePanel = ref(false)
const showScripts = ref(false)
const showConvSidebar = ref(true)
const chatArea = ref(null)

onMounted(async () => {
  await loadConversations(userId.value)
})

watch(messages, async () => {
  await nextTick()
  if (chatArea.value) {
    chatArea.value.scrollTop = chatArea.value.scrollHeight
  }
}, { deep: true })

function onFileUploaded(refId) {
  fileRefId.value = refId
  showFilePanel.value = false
}

function onSend(message) {
  send(userId.value, message, fileRefId.value)
}

function onInterruptResume(data) {
  resume(userId.value, data)
}

async function onSelectConv(convId) {
  await selectConversation(convId, userId.value)
  showConvSidebar.value = false
}

function onNewConv() {
  newConversation()
  showConvSidebar.value = false
}

function clearChat() {
  if (confirm('清空当前对话，不影响服务器记录？')) {
    messages.value = []
    fileRefId.value = ''
  }
}

const hasMessages = computed(() => messages.value.length > 0)

const downloadFiles = computed(() => {
  const files = []
  for (const msg of messages.value) {
    if (msg.content) {
      const matches = msg.content.matchAll(/([^\/]+\.(mp3|wav))/g)
      for (const m of matches) {
        if (!files.some(f => f.name === m[1])) files.push({ name: m[1] })
      }
    }
  }
  return files
})

function convTitle(conv) {
  return conv.title || `对话 ${conv.id}`
}

function convTime(conv) {
  const d = new Date(conv.updated_at || conv.created_at)
  const pad = n => String(n).padStart(2, '0')
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}
</script>

<template>
  <div class="app">
    <!-- 顶部导航 -->
    <header>
      <div class="header-left">
        <button class="icon-btn menu-btn" @click="showConvSidebar = !showConvSidebar" title="对话列表">☰</button>
        <span class="logo">🎧</span>
        <span class="title">有声读物</span>
      </div>
      <div class="header-right">
        <button class="icon-btn" @click="showScripts = !showScripts" title="查看脚本">📝</button>
        <button class="icon-btn" @click="showFilePanel = !showFilePanel" title="上传文件">📁</button>
        <button class="icon-btn" @click="clearChat" title="清空">🗑️</button>
        <span class="user-badge">{{ userId }}</span>
      </div>
    </header>

    <div class="main-wrapper">
      <!-- 对话侧边栏 -->
      <aside class="conv-sidebar" :class="{ open: showConvSidebar }">
        <div class="conv-header">
          <span class="conv-title">对话历史</span>
          <button class="new-conv-btn" @click="onNewConv">＋ 新建</button>
        </div>
        <div class="conv-list">
          <div
            v-for="conv in conversations"
            :key="conv.id"
            class="conv-item"
            :class="{ active: conv.id === currentConversationId }"
            @click="onSelectConv(conv.id)"
          >
            <div class="conv-item-title">{{ convTitle(conv) }}</div>
            <div class="conv-item-time">{{ convTime(conv) }}</div>
          </div>
          <div v-if="conversations.length === 0" class="conv-empty">
            暂无对话
          </div>
        </div>
      </aside>

      <!-- 遮罩层（移动端） -->
      <div v-if="showConvSidebar" class="sidebar-overlay" @click="showConvSidebar = false"></div>

      <!-- 主内容 -->
      <div class="content-area">
        <FileUpload
          v-if="showFilePanel"
          :userId="userId"
          @uploaded="onFileUploaded"
        />

        <!-- 空状态 -->
        <div v-if="!hasMessages" class="landing">
          <div class="hero">
            <div class="hero-icon">🎧</div>
            <h1>有声读物制作</h1>
            <p class="hero-desc">上传 PDF/MD/DOCX 文件，智能生成多集音频</p>
          </div>

          <div class="landing-input-area">
            <div v-if="fileRefId" class="file-tag">
              📎 文件已上传
              <button class="tag-close" @click="fileRefId = ''">✕</button>
            </div>
            <div class="landing-input-wrapper">
              <ChatInput :disabled="isStreaming" @send="onSend" />
            </div>
            <div class="landing-hints">
              <button class="hint-btn" @click="onSend('生成全本音频')">📖 生成全本音频</button>
              <button class="hint-btn" @click="onSend('生成第1到5回的音频，适合小朋友听')">📗 生成1-5回</button>
              <button class="hint-btn" @click="onSend('生成第10章的内容，时长5分钟')">⏱️ 指定时长</button>
            </div>
          </div>
        </div>

        <!-- 对话状态 -->
        <div v-else class="main-area">
          <div class="chat-area" ref="chatArea">
            <ChatLog :messages="messages" :userId="userId" @interrupt-resume="onInterruptResume" />
          </div>
          <aside v-if="downloadFiles.length" class="sidebar">
            <div class="sidebar-title">📥 下载列表</div>
            <div v-for="f in downloadFiles" :key="f.name" class="sidebar-item">
              <a :href="`/api/v1/audio/download/${userId}/${encodeURIComponent(f.name)}`"
                 target="_blank" class="sidebar-link" download>
                🔊 {{ f.name }}
              </a>
            </div>
            <div v-if="fileRefId" class="sidebar-file">📎 文件已上传</div>
          </aside>
        </div>

        <!-- 对话时的底部输入 -->
        <div v-if="hasMessages" class="bottom-bar">
          <div v-if="fileRefId" class="file-tag">
            📎 文件已上传
            <button class="tag-close" @click="fileRefId = ''">✕</button>
          </div>
          <ChatInput :disabled="isStreaming" @send="onSend" />
        </div>
      </div>
    </div>

    <!-- 脚本面板 -->
    <ScriptList
      v-if="showScripts"
      :userId="userId"
      :bookRef="fileRefId"
      @close="showScripts = false"
    />
  </div>
</template>

<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
  font-family: -apple-system, BlinkMacSystemFont, 'PingFang SC', 'Noto Sans SC', sans-serif;
  background: #f0f2f5;
  color: #1a1a2e;
}
.app {
  max-width: 1100px; margin: 0 auto; height: 100vh;
  display: flex; flex-direction: column;
  background: #fff; box-shadow: 0 0 20px rgba(0,0,0,0.04);
}

/* ===== 顶部导航 ===== */
header {
  padding: 14px 20px; border-bottom: 1px solid #eee;
  display: flex; justify-content: space-between; align-items: center; flex-shrink: 0;
  z-index: 10; background: #fff;
}
.header-left { display: flex; align-items: center; gap: 8px; }
.logo { font-size: 22px; }
.title { font-size: 17px; font-weight: 600; color: #1a1a2e; }
.header-right { display: flex; align-items: center; gap: 10px; }
.icon-btn {
  background: none; border: none; font-size: 18px; cursor: pointer;
  padding: 4px 6px; border-radius: 6px; transition: background 0.2s;
}
.icon-btn:hover { background: #f0f2f5; }
.menu-btn { font-size: 20px; }
.user-badge {
  font-size: 12px; background: #e8f4fd; color: #1976d2;
  padding: 3px 10px; border-radius: 12px; font-weight: 500;
}

/* ===== 主布局 ===== */
.main-wrapper {
  flex: 1; display: flex; overflow: hidden;
  position: relative;
}

/* ===== 对话侧边栏 ===== */
.conv-sidebar {
  width: 260px; border-right: 1px solid #eee;
  display: flex; flex-direction: column;
  background: #fafbfc; flex-shrink: 0;
  overflow: hidden;
}
.conv-header {
  padding: 14px 16px; border-bottom: 1px solid #eee;
  display: flex; justify-content: space-between; align-items: center;
}
.conv-title { font-size: 14px; font-weight: 600; color: #333; }
.new-conv-btn {
  padding: 4px 12px; border: 1px solid #1976d2; border-radius: 14px;
  background: #fff; color: #1976d2; font-size: 12px; cursor: pointer;
  transition: all 0.2s;
}
.new-conv-btn:hover { background: #1976d2; color: #fff; }
.conv-list { flex: 1; overflow-y: auto; padding: 6px 0; }
.conv-item {
  padding: 12px 16px; cursor: pointer; border-bottom: 1px solid #f0f0f0;
  transition: background 0.15s;
}
.conv-item:hover { background: #f0f2f5; }
.conv-item.active { background: #e8f4fd; border-left: 3px solid #1976d2; }
.conv-item-title {
  font-size: 13px; color: #333; font-weight: 500;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.conv-item-time { font-size: 11px; color: #999; margin-top: 3px; }
.conv-empty {
  padding: 30px; text-align: center; color: #bbb; font-size: 13px;
}
.sidebar-overlay { display: none; }

/* ===== 内容区 ===== */
.content-area {
  flex: 1; display: flex; flex-direction: column; overflow: hidden;
}

/* ===== 空状态首页 ===== */
.landing {
  flex: 1; display: flex; flex-direction: column;
  align-items: center; justify-content: center;
  padding: 40px 20px;
}
.hero { text-align: center; margin-bottom: 40px; }
.hero-icon { font-size: 56px; margin-bottom: 16px; }
h1 { font-size: 28px; font-weight: 700; color: #1a1a2e; margin-bottom: 8px; }
.hero-desc { font-size: 15px; color: #888; }
.landing-input-area {
  width: 100%; max-width: 640px;
  display: flex; flex-direction: column; align-items: center;
}
.landing-input-wrapper { width: 100%; }
.landing-hints {
  display: flex; flex-wrap: wrap; gap: 8px; margin-top: 16px;
  justify-content: center;
}
.hint-btn {
  padding: 6px 14px; border: 1px solid #e0e0e0; border-radius: 20px;
  background: #fff; color: #555; font-size: 13px; cursor: pointer;
  transition: all 0.2s;
}
.hint-btn:hover { border-color: #1976d2; color: #1976d2; background: #f5f9ff; }

/* ===== 对话布局 ===== */
.main-area {
  flex: 1; display: flex; overflow: hidden;
}
.chat-area {
  flex: 1; overflow-y: auto; padding: 16px 20px;
  scroll-behavior: smooth;
}
.sidebar {
  width: 240px; border-left: 1px solid #eee; padding: 16px;
  overflow-y: auto; background: #fafbfc; flex-shrink: 0;
}
.sidebar-title { font-size: 14px; font-weight: 600; color: #333; margin-bottom: 12px; }
.sidebar-item {
  padding: 6px 0; border-bottom: 1px solid #eee; font-size: 13px;
}
.sidebar-link {
  color: #1976d2; text-decoration: none; word-break: break-all;
  display: flex; align-items: center; gap: 4px;
}
.sidebar-link:hover { text-decoration: underline; }
.sidebar-file { font-size: 13px; color: #888; margin-top: 12px; }

/* ===== 底部 ===== */
.bottom-bar {
  border-top: 1px solid #eee; padding: 12px 20px 16px;
  flex-shrink: 0;
}
.file-tag {
  display: inline-flex; align-items: center; gap: 6px;
  font-size: 13px; color: #1976d2; background: #e8f4fd;
  padding: 4px 12px; border-radius: 16px; margin-bottom: 8px;
}
.tag-close { border: none; background: none; cursor: pointer; color: #999; font-size: 14px; }
.tag-close:hover { color: #333; }

/* ===== 响应式 ===== */
@media (max-width: 768px) {
  .conv-sidebar {
    position: fixed; left: -280px; top: 0; bottom: 0; width: 280px;
    z-index: 100; transition: left 0.25s; box-shadow: 2px 0 12px rgba(0,0,0,0.1);
  }
  .conv-sidebar.open { left: 0; }
  .sidebar-overlay {
    display: block; position: fixed; inset: 0; z-index: 99;
    background: rgba(0,0,0,0.3);
  }
}
</style>
