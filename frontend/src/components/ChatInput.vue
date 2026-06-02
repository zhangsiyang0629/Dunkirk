<script setup>
import { ref } from 'vue'

const props = defineProps({ disabled: Boolean })
const emit = defineEmits(['send'])
const text = ref('')
const textarea = ref(null)

function send() {
  if (!text.value.trim() || props.disabled) return
  emit('send', text.value)
  text.value = ''
  if (textarea.value) {
    textarea.value.style.height = 'auto'
  }
}

function autoResize() {
  const el = textarea.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = el.scrollHeight + 'px'
}
</script>

<template>
  <div class="chat-input">
    <textarea
      ref="textarea"
      v-model="text"
      :disabled="disabled"
      placeholder="输入需求，如「生成第1到5回的音频」"
      @keydown.enter.exact="send"
      @keydown.shift.enter="(e) => { /* allow default newline */ }"
      @input="autoResize"
      rows="1"
    ></textarea>
    <button @click="send" :disabled="disabled || !text.trim()" class="send-btn">
      <span v-if="!disabled">➤</span>
      <span v-else class="spinner">⏳</span>
    </button>
  </div>
</template>

<style scoped>
.chat-input { display: flex; align-items: flex-end; gap: 8px; }
textarea {
  flex: 1; min-height: 44px; max-height: 120px;
  padding: 10px 14px; border: 1px solid #ddd;
  border-radius: 22px; resize: none; font-size: 14px;
  font-family: inherit; line-height: 1.5;
  outline: none; transition: border-color 0.2s;
}
textarea:focus { border-color: #1976d2; }
.send-btn {
  width: 44px; height: 44px; border: none; border-radius: 50%;
  background: #1976d2; color: #fff; font-size: 18px;
  cursor: pointer; display: flex; align-items: center; justify-content: center;
  transition: background 0.2s; flex-shrink: 0;
}
.send-btn:hover:not(:disabled) { background: #1565c0; }
.send-btn:disabled { background: #ccc; cursor: not-allowed; }
.spinner { font-size: 16px; }
</style>
