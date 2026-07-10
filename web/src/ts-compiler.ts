import { transform, type Transform } from 'sucrase'

export type ScriptLang = 'js' | 'ts' | 'tsx'

export interface CompileResult {
  code: string
  error?: string
}

export function compileToJS(code: string, lang: ScriptLang): CompileResult {
  if (!code.trim()) {
    return { code: '' }
  }
  if (lang === 'js') {
    return { code }
  }
  try {
    const transforms: Transform[] = []
    if (lang === 'ts') {
      transforms.push('typescript')
    } else if (lang === 'tsx') {
      transforms.push('typescript', 'jsx')
    }
    const result = transform(code, {
      transforms,
      production: true,
      jsxRuntime: 'classic',
      jsxPragma: 'React',
    })
    return { code: result.code }
  } catch (e: any) {
    return { code: '', error: e.message || String(e) }
  }
}

export const TS_TEMPLATE = `// plugin template
registerStep("my-step", function(ctx: any) {
    const name: string = ctx.config.name || "world"
    ctx.log("Hello, " + name)
    var result = exec("echo hello")
    if (result.error) {
        ctx.fail(result.error)
        return
    }
    ctx.output("greeting", "Hello " + name)
})
`

export const TSX_TEMPLATE = `// ui plugin template
__BW_PLUGINS__.register({
    point: 'global_menu',
    name: 'demo:menu',
    label: 'Demo',
    icon: '🔌',
    component: function(props: any) {
        return React.createElement('a', {
            href: '#',
            className: 'flex items-center px-3 py-2 text-gray-600 rounded'
        }, '🔌 Demo')
    }
})
`
