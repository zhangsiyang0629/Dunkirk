<script setup>
import { ref } from 'vue'
import { useChat } from './composables/useChat.js'
import FileUpload from './components/FileUpload.vue'
import ChatInput from './components/ChatInput.vue'
import ChatLog from './components/ChatLog.vue'

const { messages, isStreaming, pendingInterrupt, send, resume } = useChat()
const userId = ref('zsy')
const fileRefId = ref('')
const inputText = ref('')
const showUpload = ref(false)

function onFileUploaded(refId) {
  fileRefId.value = refId
  showUpload.value = false
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
</script>

<template>
  <div class="app">
    <header>
      <div class="header-left">
        <span class="logo">🎧</span>
        <span class="title">有声读物</span>
      </div>
      <div class="header-right">
        <button class="icon-btn" @click="showUpload = !showUpload" title="上传文件">📁</button>
        <button class="icon-btn" @click="clearChat" title="清空对话">🗑️</button>
        <span class="user-badge">{{ userId }}</span>
      </div>
    </header>

    <FileUpload
      v-if="showUpload"
      :userId="userId"
      @uploaded="onFileUploaded"
    />

    <div class="chat-area" ref="chatArea">
      <ChatLog :messages="messages" @interrupt-resume="onInterruptResume" />
    </div>

    <div class="bottom-bar">
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
  max-width: 860px; margin: 0 auto; height: 100vh;
  display: flex; flex-direction: column;
  background: #fff; box-shadow: 0 0 20px rgba(0,0,0,0.04);
}
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
.chat-area {
  flex: 1; overflow-y: auto; padding: 16px 20px;
  scroll-behavior: smooth;
}
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
