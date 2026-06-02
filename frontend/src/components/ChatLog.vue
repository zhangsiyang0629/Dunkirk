<script setup>
const props = defineProps({ messages: Array, userId: String })
const emit = defineEmits(['interrupt-resume'])

function onInterruptClick(msg, opt) {
  emit('interrupt-resume', {
    checkpoint_id: msg.checkpoint_id,
    interrupt_id: msg.interrupt_id,
    choice: opt,
  })
}

function extractAudio(text) {
  if (!text) return []
  const matches = [...text.matchAll(/([^\/]+\.mp3)/g)]
  return matches.map(m => m[1]).filter((v, i, a) => a.indexOf(v) === i)
}
</script>
<template>
  <div class="chat-log">
    <div v-for="msg in messages" :key="msg.id" class="msg-row" :class="msg.type">

      <!-- 用户消息 - 右侧气泡 -->
      <div v-if="msg.type === 'user_msg'" class="bubble user-bubble">
        {{ msg.content }}
      </div>

      <!-- 进度消息 - 左侧 -->
      <div v-else-if="msg.type === 'agent_msg'" class="agent-row">
        <div class="agent-label">{{ msg.agent }}</div>
        <div class="bubble agent-bubble">
          <div class="msg-content">{{ msg.content }}</div>
          <div v-if="extractAudio(msg.content).length" class="inline-audio">
            <div v-for="af in extractAudio(msg.content)" :key="af" class="inline-audio-item">
              <span>🔊 {{ af }}</span>
              <a :href="`/api/v1/audio/download/${userId}/${encodeURIComponent(af)}`" target="_blank" class="dl-btn" download>下载</a>
            </div>
          </div>
        </div>
      </div>

      <!-- Token 流 -->
      <div v-else-if="msg.type === 'token'" class="agent-row">
        <div class="bubble agent-bubble token-bubble">{{ msg.content }}</div>
      </div>

      <!-- 中断 -->
      <div v-else-if="msg.type === 'interrupt'" class="interrupt-card">
        <div class="interrupt-question">{{ msg.question }}</div>
        <div class="interrupt-options">
          <button v-for="opt in msg.options" :key="opt" @click="onInterruptClick(msg, opt)">
            {{ opt }}
          </button>
        </div>
      </div>

      <!-- 完成 -->
      <div v-else-if="msg.type === 'done'" class="done-row">✅ 全部完成</div>

      <!-- 错误 -->
      <div v-else-if="msg.type === 'error'" class="error-row">❌ {{ msg.content }}</div>

    </div>
  </div>
</template>

<style scoped>
.chat-log { display: flex; flex-direction: column; gap: 6px; min-height: 100%; }
.msg-row { display: flex; flex-direction: column; }

/* 用户气泡 */
.user-bubble {
  align-self: flex-end;
  background: #1976d2; color: #fff;
  border-radius: 18px 18px 4px 18px;
  padding: 10px 16px; max-width: 75%;
  font-size: 14px; line-height: 1.5; white-space: pre-wrap;
  margin: 4px 0;
}

/* Agent 消息 */
.agent-row { align-items: flex-start; margin: 2px 0; }
.agent-label {
  font-size: 11px; color: #999; margin-bottom: 2px; margin-left: 4px;
}
.agent-bubble {
  background: #f0f2f5; color: #1a1a2e;
  border-radius: 18px 18px 18px 4px;
  padding: 10px 16px; max-width: 80%;
  font-size: 14px; line-height: 1.6; white-space: pre-wrap;
}
.token-bubble {
  background: #f5f7fa; border: 1px solid #e8ecf1;
  font-size: 14px; line-height: 1.6;
}

/* 中断卡片 */
.interrupt-card {
  background: #fff8e1; border: 1px solid #ffecb3;
  border-radius: 12px; padding: 14px 16px; margin: 8px 0;
}
.interrupt-question { font-size: 14px; color: #795548; margin-bottom: 10px; }
.interrupt-options { display: flex; flex-wrap: wrap; gap: 8px; }
.interrupt-options button {
  padding: 6px 16px; border: 1px solid #ffcc02; background: #fff;
  border-radius: 20px; cursor: pointer; font-size: 13px; color: #333;
  transition: all 0.2s;
}
.interrupt-options button:hover { background: #fff3c4; border-color: #ffa000; }

/* 完成 & 错误 */
.done-row { text-align: center; color: #4caf50; font-weight: 500; font-size: 14px; padding: 12px; }
.dl-btn {
  padding: 4px 12px; background: #22c55e; color: #fff;
  border-radius: 6px; text-decoration: none; font-size: 12px; flex-shrink: 0;
  transition: background 0.2s;
}
.dl-btn:hover { background: #16a34a; }
.error-row { text-align: center; color: #d32f2f; font-size: 14px; padding: 12px; }
.inline-audio {
  margin-top: 6px; padding-top: 6px; border-top: 1px solid #e0e0e0;
  font-size: 13px;
}
.inline-audio-item {
  display: flex; align-items: center; justify-content: space-between; gap: 8px;
  padding: 2px 0;
}
.msg-content { white-space: pre-wrap; }
</style>
