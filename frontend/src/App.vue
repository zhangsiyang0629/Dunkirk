<script setup>
import { ref, computed, watch, nextTick } from 'vue'
import { useChat } from './composables/useChat.js'
import FileUpload from './components/FileUpload.vue'
import ChatInput from './components/ChatInput.vue'
import ChatLog from './components/ChatLog.vue'

const { messages, isStreaming, pendingInterrupt, send, resume } = useChat()
const userId = ref('zsy')
const fileRefId = ref('')
const inputText = ref('')
const showFilePanel = ref(false)
const chatArea = ref(null)

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
function clearChat() {
  messages.value = []
  fileRefId.value = ''
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
</script>

<template>
  <div class="app">
    <!-- 顶部导航 -->
    <header v-if="hasMessages">
      <div class="header-left">
        <span class="logo">🎧</span>
        <span class="title">有声读物</span>
      </div>
      <div class="header-right">
        <button class="icon-btn" @click="showFilePanel = !showFilePanel" title="上传文件">📁</button>
        <button class="icon-btn" @click="clearChat" title="新建对话">✏️</button>
        <span class="user-badge">{{ userId }}</span>
      </div>
    </header>

    <FileUpload
      v-if="showFilePanel"
      :userId="userId"
      @uploaded="onFileUploaded"
    />

    <!-- 空状态（类似 deepseek 首页） -->
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
.user-badge {
  font-size: 12px; background: #e8f4fd; color: #1976d2;
  padding: 3px 10px; border-radius: 12px; font-weight: 500;
}

/* ===== 空状态首页（deepseek 风格） ===== */
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
.landing-input-wrapper {
  width: 100%;
}
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
</style>
