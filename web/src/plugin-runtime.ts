import React from 'react'
import { api } from './api'

declare global {
  interface Window {
    __BW_PLUGINS__: {
      register: (ext: {
        point: string
        name: string
        label: string
        component: React.ComponentType<any>
        icon?: string
      }) => void
      _components: Record<string, React.ComponentType<any>>
      _extensions: Array<{
        plugin: string
        point: string
        name: string
        label: string
        component: string
        icon?: string
      }>
    }
  }
}

if (typeof window !== 'undefined') {
  if (!window.__BW_PLUGINS__) {
    window.__BW_PLUGINS__ = {
      register(ext) {
        const key = `${ext.point}:${ext.name}`
        this._components[key] = ext.component
        this._extensions.push({
          plugin: ext.name.split(':')[0] || 'unknown',
          point: ext.point,
          name: ext.name,
          label: ext.label,
          component: key,
          icon: ext.icon,
        })
      },
      _components: {},
      _extensions: [],
    }
  }
}

const loadedUIScripts = new Set<string>()

export async function loadPluginUI(pluginName: string): Promise<void> {
  if (loadedUIScripts.has(pluginName)) return
  try {
    const code = await api.getPluginUI(pluginName)
    if (!code) return
    const fn = new Function('React', '__BW_PLUGINS__', code)
    fn(React, window.__BW_PLUGINS__)
    loadedUIScripts.add(pluginName)
  } catch (e) {
    console.error(`Failed to load plugin UI for ${pluginName}:`, e)
  }
}

export async function loadAllPluginUI(): Promise<void> {
  try {
    const plugins = await api.listPlugins()
    for (const p of plugins) {
      if (p.enabled && p.source !== 'builtin') {
        await loadPluginUI(p.name)
      }
    }
  } catch (e) {
    console.error('Failed to load plugin UIs:', e)
  }
}

export function getExtensions(point: string): Array<{
  key: string
  label: string
  icon?: string
  Component: React.ComponentType<any>
}> {
  if (!window.__BW_PLUGINS__) return []
  return window.__BW_PLUGINS__._extensions
    .filter(e => e.point === point)
    .map(e => ({
      key: e.component,
      label: e.label,
      icon: e.icon,
      Component: window.__BW_PLUGINS__._components[e.component],
    }))
    .filter(e => e.Component)
}
