import { ref } from 'vue'
import type { ChatRequest, ChatResponse } from '@/api/ai'

export type StreamCallback = (chunk: string, done: boolean) => void

export function useStreamChat() {
  const isStreaming = ref(false)
  const abortController = ref<AbortController | null>(null)

  const streamChat = async (
    request: ChatRequest,
    onChunk: StreamCallback,
    useStream: boolean = true
  ): Promise<ChatResponse | null> => {
    if (isStreaming.value) {
      console.warn('Already streaming')
      return null
    }

    isStreaming.value = true
    abortController.value = new AbortController()

    try {
      const token = localStorage.getItem('token')
      const baseURL = '/api/v1'
      
      if (useStream) {
        const response = await fetch(`${baseURL}/ai/chat/stream`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            ...(token ? { Authorization: `Bearer ${token}` } : {})
          },
          body: JSON.stringify({ ...request, stream: true }),
          signal: abortController.value.signal
        })

        if (!response.ok) {
          throw new Error(`HTTP error! status: ${response.status}`)
        }

        const reader = response.body?.getReader()
        const decoder = new TextDecoder()
        let fullContent = ''
        let sessionId = ''

        if (reader) {
          while (true) {
            const { done, value } = await reader.read()
            
            if (done) {
              onChunk(fullContent, true)
              break
            }

            const chunk = decoder.decode(value, { stream: true })
            const lines = chunk.split('\n')

            for (const line of lines) {
              if (line.startsWith('data: ')) {
                try {
                  const data = JSON.parse(line.slice(6))
                  
                  if (data.session_id) {
                    sessionId = data.session_id
                  }
                  
                  if (data.content) {
                    fullContent += data.content
                    onChunk(fullContent, false)
                  }
                } catch (e) {
                  // Ignore parse errors
                }
              }
            }
          }
        }

        return {
          session_id: sessionId,
          message: fullContent,
          timestamp: Date.now() / 1000,
          tokens_used: 0,
          cached: false
        }
      } else {
        const response = await fetch(`${baseURL}/ai/chat`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            ...(token ? { Authorization: `Bearer ${token}` } : {})
          },
          body: JSON.stringify(request),
          signal: abortController.value.signal
        })

        if (!response.ok) {
          throw new Error(`HTTP error! status: ${response.status}`)
        }

        const data = await response.json()
        onChunk(data.message, true)
        
        return data as ChatResponse
      }
    } catch (error: any) {
      if (error.name === 'AbortError') {
        console.log('Stream aborted')
        return null
      }
      throw error
    } finally {
      isStreaming.value = false
      abortController.value = null
    }
  }

  const abort = () => {
    if (abortController.value) {
      abortController.value.abort()
    }
  }

  return {
    isStreaming,
    streamChat,
    abort
  }
}
