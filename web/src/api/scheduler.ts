import request from './request'

export interface TaskConfigField {
  key: string
  label: string
  kind: 'bool' | 'int' | 'string'
  value: boolean | number | string
}

export interface ScheduledTaskItem {
  name: string
  schedule: string
  custom: boolean
  config?: TaskConfigField[]
}

export interface SchedulerListResult {
  tasks: ScheduledTaskItem[]
  interval: string
}

export const schedulerApi = {
  list() {
    return request.get<SchedulerListResult>('/scheduler/tasks')
  },

  update(name: string, data: { schedule?: string; config?: Record<string, unknown> }) {
    return request.put<ScheduledTaskItem>(`/scheduler/tasks/${name}`, data)
  },

  run(name: string) {
    return request.post<{ name: string; processed: number }>(`/scheduler/tasks/${name}/run`)
  },

  updateInterval(interval: string) {
    return request.put<{ interval: string }>('/scheduler/interval', { interval })
  },
}
