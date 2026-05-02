import request from './request'

/** 聊天请求参数 */
export interface ChatRequest {
  /** 会话ID（可选，用于保持上下文） */
  session_id?: string
  /** 用户消息 */
  message: string
}

/** 工具调用结果 */
export interface ToolCallResult {
  /** 工具名称 */
  name: string
  /** 调用参数 */
  params: Record<string, unknown>
  /** 执行结果 */
  result: string
  /** 错误信息 */
  error?: string
  /** 是否成功 */
  success: boolean
  /** 执行时长（毫秒） */
  duration_ms: number
}

/** 聊天响应 */
export interface ChatResponse {
  /** 会话ID */
  session_id: string
  /** AI回复消息 */
  message: string
  /** 工具调用结果列表 */
  tool_calls?: ToolCallResult[]
  /** 时间戳 */
  timestamp: number
  /** 使用的token数量 */
  tokens_used: number
  /** 是否来自缓存 */
  cached: boolean
}

/** 工具信息 */
export interface ToolInfo {
  /** 工具名称 */
  name: string
  /** 工具描述 */
  description: string
  /** 参数定义（JSON Schema） */
  parameters: string
}

/** AI 相关 API */
export const aiApi = {
  /**
   * 发送聊天消息
   */
  chat(data: ChatRequest) {
    return request.post<ChatResponse>('/ai/chat', data)
  },

  /**
   * 获取可用工具列表
   */
  getTools() {
    return request.get<{ tools: ToolInfo[] }>('/ai/tools')
  },

  /**
   * 清除会话
   */
  clearSession(sessionId: string) {
    return request.delete(`/ai/sessions/${sessionId}`)
  },
}
