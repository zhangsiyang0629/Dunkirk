import { ref } from 'vue'

export function useChat() {
  const messages = ref([])
  const isStreaming = ref(false)
  const pendingInterrupt = ref(null)
  const conversations = ref([])
  const currentConversationId = ref(crypto.randomUUID().slice(0, 8))

  async function loadConversations(userId) {
    const res = await fetch('/api/v1/conversations', {
      headers: { 'X-User-ID': userId },
    })
    if (res.ok) {
      const data = await res.json()
      conversations.value = data.conversations || []
    }
  }

  async function selectConversation(convId, userId) {
    currentConversationId.value = convId
    messages.value = []

    const res = await fetch(`/api/v1/conversations/${convId}/messages`, {
      headers: { 'X-User-ID': userId },
    })
    if (res.ok) {
      const data = await res.json()
      messages.value = (data.messages || []).map(m => ({
        id: Date.now() + Math.random(),
        type: m.role === 'user' ? 'user_msg' : 'agent_msg',
        content: m.content,
        agent: m.role === 'assistant' ? 'memory' : undefined,
      }))
    }
  }

  function newConversation() {
    currentConversationId.value = crypto.randomUUID().slice(0, 8)
    messages.value = []
  }

  function processSSE(event, data) {
    switch (event) {
      case 'conv_ready':
        if (typeof data === 'string' && data) {
          currentConversationId.value = data
        }
        break
      case 'intent':
        messages.value.push({ id: Date.now(), type: 'intent', content: data.reasoning || '' })
        break
      case 'token':
        const last = messages.value[messages.value.length - 1]
        if (last && last.type === 'token') {
          last.content += data.content
        } else {
          messages.value.push({ id: Date.now(), type: 'token', content: data.content })
        }
        break
      case 'progress':
        const lastProgress = messages.value[messages.value.length - 1]
        if (data.agent === 'tts' && lastProgress && lastProgress.type === 'agent_msg' && lastProgress.agent === 'tts') {
          lastProgress.content = data.content
        } else if (lastProgress && lastProgress.type === 'agent_msg' && lastProgress.agent === data.agent) {
          lastProgress.content += data.content
        } else {
          messages.value.push({ id: Date.now(), type: 'agent_msg', agent: data.agent, content: data.content })
        }
        break
      case 'interrupt':
        pendingInterrupt.value = data
        messages.value.push({ id: Date.now(), type: 'interrupt', ...data })
        break
      case 'task_created':
        break
      case 'done':
        isStreaming.value = false
        messages.value.push({ id: Date.now(), type: 'done' })
        break
      case 'error':
        isStreaming.value = false
        messages.value.push({ id: Date.now(), type: 'error', content: data.message })
        break
    }
  }

  function ensureDone() {
    const hasDone = messages.value.some(m => m.type === 'done' || m.type === 'error')
    if (!hasDone) {
      messages.value.push({ id: Date.now(), type: 'done' })
    }
  }

  async function send(userId, message, fileRefId) {
    isStreaming.value = true
    if (message) {
      messages.value.push({ id: Date.now(), type: 'user_msg', content: message })
    }
    const formData = new FormData()
    if (message) formData.append('message', message)
    if (fileRefId) formData.append('file_ref_id', fileRefId)
    formData.append('conversation_id', currentConversationId.value)

    const res = await fetch('/api/v1/chat', {
      method: 'POST',
      headers: { 'X-User-ID': userId },
      body: formData,
    })
    const reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    let currentEvent = ''
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''
      for (const line of lines) {
        const t = line.trim()
        if (t.startsWith('event: ')) {
          currentEvent = t.slice(7)
        } else if (t.startsWith('data: ')) {
          try {
            const data = JSON.parse(t.slice(6))
            processSSE(currentEvent, data)
          } catch (e) {
            console.warn('SSE parse error:', e)
          }
        }
      }
    }
    ensureDone()
    isStreaming.value = false
    await loadConversations(userId)
  }

  async function resume(userId, interrupt) {
    isStreaming.value = true
    pendingInterrupt.value = null
    const idx = messages.value.findIndex(m => m.type === 'interrupt')
    if (idx >= 0) messages.value.splice(idx, 1)
    messages.value.push({ id: Date.now(), type: 'user_msg', content: '选择: ' + interrupt.choice })

    const res = await fetch('/api/v1/resume', {
      method: 'POST',
      headers: { 'X-User-ID': userId, 'Content-Type': 'application/json' },
      body: JSON.stringify({
        checkpoint_id: interrupt.checkpoint_id,
        interrupt_id: interrupt.interrupt_id,
        choice: interrupt.choice,
      }),
    })
    const ct = res.headers.get('content-type') || ''
    if (ct.includes('application/json')) {
      const json = await res.json()
      isStreaming.value = false
      if (json.reply) {
        messages.value.push({ id: Date.now(), type: 'agent_msg', agent: 'system', content: json.reply })
      }
      return
    }
    const reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    let currentEvent = ''
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''
      for (const line of lines) {
        const t = line.trim()
        if (t.startsWith('event: ')) currentEvent = t.slice(7)
        else if (t.startsWith('data: ')) {
          try { processSSE(currentEvent, JSON.parse(t.slice(6))) }
          catch (e) { console.warn('SSE parse error:', e) }
        }
      }
    }
    ensureDone()
    isStreaming.value = false
    await loadConversations(userId)
  }

  return {
    messages, isStreaming, pendingInterrupt,
    conversations, currentConversationId,
    send, resume,
    loadConversations, selectConversation, newConversation,
  }
}
