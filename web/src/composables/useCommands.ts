export interface Command {
  name: string
  description: string
  usage: string
  example: string
}

export interface ParsedCommand {
  isCommand: boolean
  command?: string
  args?: string[]
  raw: string
}

const COMMANDS: Record<string, Command> = {
  query: {
    name: 'query',
    description: '查询包信息',
    usage: '/query <包名>',
    example: '/query lodash'
  },
  search: {
    name: 'search',
    description: '搜索日志',
    usage: '/search <关键词>',
    example: '/search error'
  },
  analyze: {
    name: 'analyze',
    description: '分析安全问题',
    usage: '/analyze <包名>',
    example: '/analyze express'
  },
  generate: {
    name: 'generate',
    description: '生成代码示例',
    usage: '/generate <包名> [语言]',
    example: '/generate vue typescript'
  },
  help: {
    name: 'help',
    description: '显示帮助信息',
    usage: '/help',
    example: '/help'
  }
}

export function useCommands() {
  const parseCommand = (input: string): ParsedCommand => {
    const trimmed = input.trim()
    
    if (!trimmed.startsWith('/')) {
      return {
        isCommand: false,
        raw: input
      }
    }
    
    const parts = trimmed.slice(1).split(/\s+/)
    const command = parts[0].toLowerCase()
    const args = parts.slice(1)
    
    return {
      isCommand: true,
      command,
      args,
      raw: input
    }
  }

  const executeCommand = (parsed: ParsedCommand): string => {
    if (!parsed.isCommand || !parsed.command) {
      return parsed.raw
    }
    
    const cmd = COMMANDS[parsed.command]
    if (!cmd) {
      return `未知命令: ${parsed.command}。输入 /help 查看可用命令。`
    }
    
    switch (parsed.command) {
      case 'query':
        if (!parsed.args || parsed.args.length === 0) {
          return `用法: ${cmd.usage}\n示例: ${cmd.example}`
        }
        return `查询包 ${parsed.args[0]} 的信息`
        
      case 'search':
        if (!parsed.args || parsed.args.length === 0) {
          return `用法: ${cmd.usage}\n示例: ${cmd.example}`
        }
        return `搜索日志关键词: ${parsed.args.join(' ')}`
        
      case 'analyze':
        if (!parsed.args || parsed.args.length === 0) {
          return `用法: ${cmd.usage}\n示例: ${cmd.example}`
        }
        return `分析 ${parsed.args[0]} 的安全问题`
        
      case 'generate':
        if (!parsed.args || parsed.args.length === 0) {
          return `用法: ${cmd.usage}\n示例: ${cmd.example}`
        }
        const lang = parsed.args[1] || 'javascript'
        return `生成 ${parsed.args[0]} 的 ${lang} 示例代码`
        
      case 'help':
        const helpText = Object.values(COMMANDS)
          .map(c => `/${c.name} - ${c.description}\n  用法: ${c.usage}\n  示例: ${c.example}`)
          .join('\n\n')
        return `可用命令:\n\n${helpText}`
        
      default:
        return parsed.raw
    }
  }

  const getCommands = (): Command[] => {
    return Object.values(COMMANDS)
  }

  const getCommand = (name: string): Command | undefined => {
    return COMMANDS[name]
  }

  return {
    parseCommand,
    executeCommand,
    getCommands,
    getCommand
  }
}
