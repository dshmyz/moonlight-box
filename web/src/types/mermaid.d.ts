declare module 'mermaid' {
  export interface MermaidConfig {
    startOnLoad?: boolean
    theme?: 'default' | 'dark' | 'forest' | 'neutral' | 'base'
    securityLevel?: 'strict' | 'loose' | 'sandbox'
    fontFamily?: string
    flowchart?: {
      useMaxWidth?: boolean
      htmlLabels?: boolean
      curve?: 'basis' | 'linear' | 'cardinal'
      padding?: number
    }
    themeVariables?: {
      primaryColor?: string
      primaryBorderColor?: string
      lineColor?: string
      textColor?: string
    }
    [key: string]: any
  }

  export interface RenderResult {
    svg: string
    bindFunctions?: (element: Element) => void
  }

  export interface Mermaid {
    initialize: (config: MermaidConfig) => void
    render: (id: string, text: string, element?: Element) => Promise<RenderResult>
    parse: (text: string) => Promise<boolean>
  }

  const mermaid: Mermaid
  export default mermaid
}
