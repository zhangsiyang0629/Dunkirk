<script setup>
import { ref } from 'vue'
const props = defineProps({ interrupt: Object })
const emit = defineEmits(['resume', 'close'])
const selected = ref('')
function confirm() {
  if (!selected.value) return
  emit('resume', {
    checkpoint_id: props.interrupt.checkpoint_id,
    interrupt_id: props.interrupt.interrupt_id,
    choice: selected.value,
  })
}
</script>
<template>
  <div class="overlay">
    <div class="dialog">
      <h3>❓ {{ interrupt.question }}</h3>
      <div class="options">
        <button
          v-for="opt in interrupt.options"
          :key="opt"
          :class="{ active: selected === opt }"
          @click="selected = opt"
        >
          {{ opt }}
        </button>
      </div>
      <div class="actions">
        <button class="cancel" @click="$emit('close')">取消</button>
        <button class="confirm" :disabled="!selected" @click="confirm">确认</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.overlay { position: fixed; inset: 0; background: rgba(0,0,0,0.4); display: flex; align-items: center; justify-content: center; z-index: 100; }
.dialog { background: #fff; border-radius: 12px; padding: 24px; min-width: 360px; box-shadow: 0 4px 20px rgba(0,0,0,0.15); }
.options { display: flex; flex-direction: column; gap: 8px; margin: 16px 0; }
.options button { padding: 10px; border: 1px solid #ccc; border-radius: 6px; background: #fff; cursor: pointer; text-align: left; }
.options button.active { border-color: #1976d2; background: #e3f2fd; }
.actions { display: flex; gap: 8px; justify-content: flex-end; }
.cancel { padding: 6px 16px; border: 1px solid #ccc; border-radius: 6px; background: #fff; }
.confirm { padding: 6px 16px; border: none; border-radius: 6px; background: #1976d2; color: #fff; }
.confirm:disabled { background: #ccc; }
</style>