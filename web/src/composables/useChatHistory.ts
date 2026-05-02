import type { ChatResponse } from '@/api/ai'

const STORAGE_KEY = 'ai_chat_history'
const MAX_HISTORY_SIZE = 100

export interface Message extends ChatResponse {
  role: 'user' | 'assistant'
  content: string
  timestamp?: number
}

export interface ChatHistory {
  sessionId: string
  messages: Message[]
  lastUpdated: number
}

export function useChatHistory() {
  const saveHistory = (sessionId: string, messages: Message[]) => {
    try {
      const history: ChatHistory = {
        sessionId,
        messages,
        lastUpdated: Date.now()
      }
      
      const allHistories = getAllHistories()
      
      const existingIndex = allHistories.findIndex(h => h.sessionId === sessionId)
      if (existingIndex >= 0) {
        allHistories[existingIndex] = history
      } else {
        allHistories.unshift(history)
      }
      
      if (allHistories.length > MAX_HISTORY_SIZE) {
        allHistories.splice(MAX_HISTORY_SIZE)
      }
      
      localStorage.setItem(STORAGE_KEY, JSON.stringify(allHistories))
    } catch (error) {
      console.error('Failed to save chat history:', error)
    }
  }

  const loadHistory = (sessionId: string): Message[] => {
    try {
      const allHistories = getAllHistories()
      const history = allHistories.find(h => h.sessionId === sessionId)
      return history?.messages || []
    } catch (error) {
      console.error('Failed to load chat history:', error)
      return []
    }
  }

  const getAllHistories = (): ChatHistory[] => {
    try {
      const data = localStorage.getItem(STORAGE_KEY)
      return data ? JSON.parse(data) : []
    } catch (error) {
      console.error('Failed to get all histories:', error)
      return []
    }
  }

  const clearHistory = (sessionId?: string) => {
    try {
      if (sessionId) {
        const allHistories = getAllHistories()
        const filtered = allHistories.filter(h => h.sessionId !== sessionId)
        localStorage.setItem(STORAGE_KEY, JSON.stringify(filtered))
      } else {
        localStorage.removeItem(STORAGE_KEY)
      }
    } catch (error) {
      console.error('Failed to clear chat history:', error)
    }
  }

  const getRecentSessions = (limit: number = 10): ChatHistory[] => {
    const allHistories = getAllHistories()
    return allHistories.slice(0, limit)
  }

  return {
    saveHistory,
    loadHistory,
    clearHistory,
    getAllHistories,
    getRecentSessions
  }
}
