<script setup>
import { ref } from 'vue'

const props = defineProps({ userId: String })
const emit = defineEmits(['uploaded'])
const file = ref(null)
const uploading = ref(false)
const progress = ref(0)
const fileName = ref('')

async function onUpload() {
  if (!file.value) return
  uploading.value = true
  progress.value = 0
  fileName.value = file.value.name
  const formData = new FormData()
  formData.append('file', file.value)
  const xhr = new XMLHttpRequest()
  xhr.upload.onprogress = (e) => {
    progress.value = Math.round((e.loaded / e.total) * 100)
  }
  xhr.onload = () => {
    uploading.value = false
    const data = JSON.parse(xhr.responseText)
    if (data.file_ref_id) {
      emit('uploaded', data.file_ref_id)
      progress.value = 100
    }
    if (data.cached) {
      progress.value = 100
    }
  }
  xhr.onerror = () => { uploading.value = false }
  xhr.open('POST', '/api/v1/upload')
  xhr.setRequestHeader('X-User-ID', props.userId)
  xhr.send(formData)
}
</script>

<template>
  <div class="file-upload">
    <div class="drop-zone" @click="$refs.input.click()">
      <input ref="input" type="file" accept=".pdf,.md,.docx" hidden
        @change="e => { file = e.target.files[0]; onUpload() }" />
      <div v-if="!file" class="drop-hint">
        <span class="icon">📄</span>
        <span>点击上传 PDF/MD/DOCX 文件</span>
      </div>
      <div v-else-if="uploading" class="uploading">
        <div class="progress-track">
          <div class="progress-fill" :style="{ width: progress + '%' }"></div>
        </div>
        <span class="progress-text">{{ fileName }} {{ progress }}%</span>
      </div>
      <div v-else class="uploaded">
        ✅ {{ fileName }} 上传完成
      </div>
    </div>
  </div>
</template>

<style scoped>
.file-upload { padding: 0 20px 12px; }
.drop-zone {
  border: 2px dashed #d0d5dd; border-radius: 12px;
  padding: 20px; text-align: center; cursor: pointer;
  transition: all 0.2s; background: #fafbfc;
}
.drop-zone:hover { border-color: #1976d2; background: #f0f7ff; }
.drop-hint { display: flex; flex-direction: column; align-items: center; gap: 6px; }
.drop-hint .icon { font-size: 28px; }
.drop-hint span { font-size: 13px; color: #666; }
.progress-track {
  height: 4px; background: #e0e0e0; border-radius: 2px;
  margin-bottom: 8px; overflow: hidden;
}
.progress-fill { height: 100%; background: #1976d2; transition: width 0.3s; border-radius: 2px; }
.progress-text { font-size: 13px; color: #666; }
.uploaded { color: #4caf50; font-size: 13px; font-weight: 500; }
</style>
