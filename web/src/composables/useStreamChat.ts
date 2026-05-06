import { ref } from 'vue'
import type { ChatRequest, ChatResponse } from '@/api/ai'

export type StreamCallback = (content: string, done: boolean, toolCall?: ToolCallInfo) => void
export type StatusCallback = (status: string, phase: string) => void

export interface ToolCallInfo {
  id?: string
  name: string
  params: Record<string, unknown>
  result: string
  error?: string
}

export type LoadingPhase = 'analyzing' | 'querying' | 'generating' | 'done'

export function useStreamChat() {
  const isStreaming = ref(false)
  const abortController = ref<AbortController | null>(null)

  const streamChat = async (
    request: ChatRequest,
    onChunk: StreamCallback,
    onStatus?: StatusCallback,
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
          body: JSON.stringify(request),
          signal: abortController.value.signal
        })

        if (!response.ok) {
          throw new Error(`HTTP error! status: ${response.status}`)
        }

        const reader = response.body?.getReader()
        const decoder = new TextDecoder()
        let fullContent = ''
        let sessionId = ''
        let hasToolCall = false

        if (reader) {
          while (true) {
            const { done, value } = await reader.read()
            
            if (done) {
              onStatus?.('完成', 'done')
              onChunk(fullContent, true)
              break
            }

            const chunk = decoder.decode(value, { stream: true })
            const lines = chunk.split('\n')

            for (const line of lines) {
              if (line.startsWith('data: ')) {
                try {
                  const data = JSON.parse(line.slice(6))
                  
                  if (data.error) {
                    throw new Error(data.error)
                  }
                  
                  if (data.session_id) {
                    sessionId = data.session_id
                  }
                  
                  // 处理工具调用
                  if (data.tool_call) {
                    hasToolCall = true
                    onStatus?.('正在查询阻断日志数据...', 'querying')
                    onChunk(fullContent, false, data.tool_call)
                  }
                  
                  // 处理文本内容
                  if (data.content) {
                    if (hasToolCall && fullContent === '') {
                      onStatus?.('正在生成分析结果...', 'generating')
                    }
                    fullContent += data.content
                    onChunk(fullContent, false)
                  }
                  
                  // 处理完成信号
                  if (data.done) {
                    onStatus?.('完成', 'done')
                    onChunk(fullContent, true)
                  }
                } catch (e: any) {
                  if (e.message && !e.message.includes('Unexpected')) {
                    throw e
                  }
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
