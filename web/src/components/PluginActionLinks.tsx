import { IoExtensionPuzzleOutline } from 'react-icons/io5'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { useApi } from '../hooks'

type PluginActionLocation = 'project.action' | 'build.action'

type PluginActionLinksProps = {
  location: PluginActionLocation
  projectId: number
  buildId?: number
  buildNumber?: number
}

type UIExtension = {
  location: PluginActionLocation
  label: string
  url: string
  open_in_new_tab?: boolean
}

function extensionURL(extension: UIExtension, values: PluginActionLinksProps): string {
  return extension.url
    .replaceAll('{projectId}', encodeURIComponent(String(values.projectId)))
    .replaceAll('{buildId}', encodeURIComponent(String(values.buildId ?? '')))
    .replaceAll('{buildNumber}', encodeURIComponent(String(values.buildNumber ?? '')))
}

export default function PluginActionLinks(props: PluginActionLinksProps) {
  const { data: plugins } = useApi(
    () => (typeof api.listPlugins === 'function' ? api.listPlugins() : Promise.resolve([])),
  )
  const actions = (plugins || []).flatMap(plugin => {
    if (!plugin.enabled || !plugin.loaded || !Array.isArray(plugin.ui_extensions)) return []
    return plugin.ui_extensions
      .filter((extension: UIExtension) => extension.location === props.location)
      .map((extension: UIExtension) => ({ ...extension, plugin: plugin.name, href: extensionURL(extension, props) }))
  })

  return <>{actions.map(action => {
    const content = <><IoExtensionPuzzleOutline aria-hidden="true" />{action.label}</>
    const key = `${action.plugin}:${action.location}:${action.label}:${action.href}`
    if (action.href.startsWith('/')) {
      return <Link key={key} to={action.href}>{content}</Link>
    }
    return <a key={key} href={action.href} target={action.open_in_new_tab ? '_blank' : undefined} rel={action.open_in_new_tab ? 'noopener noreferrer' : undefined}>{content}</a>
  })}</>
}
