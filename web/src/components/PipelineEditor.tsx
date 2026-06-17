import { useState } from 'react'
import { useI18n } from '../i18n'

interface PipelineStage {
  id: string
  name: string
  steps: PipelineStep[]
}

interface PipelineStep {
  id: string
  name: string
  type: 'shell' | 'git' | 'notify'
  command?: string
  config?: Record<string, string>
}

export default function PipelineEditor() {
  const { t } = useI18n()
  const [stages, setStages] = useState<PipelineStage[]>([
    {
      id: '1',
      name: 'Checkout',
      steps: [
        { id: '1-1', name: 'git clone', type: 'git', config: { action: 'clone' } }
      ]
    },
    {
      id: '2',
      name: 'Build',
      steps: [
        { id: '2-1', name: 'npm ci', type: 'shell', command: 'npm ci' },
        { id: '2-2', name: 'npm build', type: 'shell', command: 'npm run build' }
      ]
    },
    {
      id: '3',
      name: 'Test',
      steps: [
        { id: '3-1', name: 'npm test', type: 'shell', command: 'npm test' }
      ]
    }
  ])
  
  const [selectedStage, setSelectedStage] = useState<string | null>(null)
  const [selectedStep, setSelectedStep] = useState<string | null>(null)

  const addStage = () => {
    const newStage: PipelineStage = {
      id: String(stages.length + 1),
      name: `Stage ${stages.length + 1}`,
      steps: []
    }
    setStages([...stages, newStage])
  }

  const addStep = (stageId: string) => {
    setStages(stages.map(stage => {
      if (stage.id === stageId) {
        return {
          ...stage,
          steps: [...stage.steps, {
            id: `${stage.id}-${stage.steps.length + 1}`,
            name: 'New Step',
            type: 'shell',
            command: ''
          }]
        }
      }
      return stage
    }))
  }

  const deleteStage = (stageId: string) => {
    setStages(stages.filter(stage => stage.id !== stageId))
    if (selectedStage === stageId) {
      setSelectedStage(null)
    }
  }

  const deleteStep = (stageId: string, stepId: string) => {
    setStages(stages.map(stage => {
      if (stage.id === stageId) {
        return {
          ...stage,
          steps: stage.steps.filter(step => step.id !== stepId)
        }
      }
      return stage
    }))
    if (selectedStep === stepId) {
      setSelectedStep(null)
    }
  }

  const updateStageName = (stageId: string, name: string) => {
    setStages(stages.map(stage => {
      if (stage.id === stageId) {
        return { ...stage, name }
      }
      return stage
    }))
  }

  const updateStep = (stageId: string, stepId: string, updates: Partial<PipelineStep>) => {
    setStages(stages.map(stage => {
      if (stage.id === stageId) {
        return {
          ...stage,
          steps: stage.steps.map(step => {
            if (step.id === stepId) {
              return { ...step, ...updates }
            }
            return step
          })
        }
      }
      return stage
    }))
  }

  const moveStage = (stageId: string, direction: 'up' | 'down') => {
    const index = stages.findIndex(s => s.id === stageId)
    if (index === -1) return
    
    const newIndex = direction === 'up' ? index - 1 : index + 1
    if (newIndex < 0 || newIndex >= stages.length) return
    
    const newStages = [...stages]
    ;[newStages[index], newStages[newIndex]] = [newStages[newIndex], newStages[index]]
    setStages(newStages)
  }

  const exportPipeline = () => {
    const config = {
      name: 'my-pipeline',
      stages: stages.map(stage => ({
        name: stage.name,
        steps: stage.steps.map(step => ({
          name: step.name,
          type: step.type,
          command: step.command,
          config: step.config
        }))
      }))
    }
    
    const json = JSON.stringify(config, null, 2)
    const blob = new Blob([json], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'pipeline.json'
    a.click()
  }

  return (
    <div className="bg-white shadow rounded-lg p-6">
      <div className="flex justify-between items-center mb-4">
        <h2 className="text-lg font-semibold">Pipeline Editor</h2>
        <div className="space-x-2">
          <button
            onClick={addStage}
            className="bg-blue-500 text-white px-4 py-2 rounded text-sm"
          >
            + Add Stage
          </button>
          <button
            onClick={exportPipeline}
            className="bg-green-500 text-white px-4 py-2 rounded text-sm"
          >
            Export JSON
          </button>
        </div>
      </div>
      
      <div className="flex gap-4">
        {/* Pipeline Visualization */}
        <div className="flex-1">
          <div className="flex items-center gap-2 overflow-x-auto pb-4">
            {stages.map((stage, index) => (
              <div key={stage.id} className="flex items-center">
                <div
                  className={`border rounded-lg p-4 min-w-[200px] cursor-pointer ${
                    selectedStage === stage.id
                      ? 'border-blue-500 bg-blue-50'
                      : 'border-gray-300 hover:border-gray-400'
                  }`}
                  onClick={() => setSelectedStage(stage.id)}
                >
                  <div className="flex justify-between items-center mb-2">
                    <input
                      type="text"
                      value={stage.name}
                      onChange={(e) => updateStageName(stage.id, e.target.value)}
                      className="font-semibold bg-transparent border-none p-0 focus:outline-none"
                      onClick={(e) => e.stopPropagation()}
                    />
                    <div className="flex gap-1">
                      <button
                        onClick={(e) => { e.stopPropagation(); moveStage(stage.id, 'up') }}
                        className="text-gray-400 hover:text-gray-600 text-xs"
                      >
                        ▲
                      </button>
                      <button
                        onClick={(e) => { e.stopPropagation(); moveStage(stage.id, 'down') }}
                        className="text-gray-400 hover:text-gray-600 text-xs"
                      >
                        ▼
                      </button>
                      <button
                        onClick={(e) => { e.stopPropagation(); deleteStage(stage.id) }}
                        className="text-red-400 hover:text-red-600 text-xs"
                      >
                        ×
                      </button>
                    </div>
                  </div>
                  
                  <div className="space-y-1">
                    {stage.steps.map(step => (
                      <div
                        key={step.id}
                        className={`text-sm p-2 rounded cursor-pointer ${
                          selectedStep === step.id
                            ? 'bg-blue-100 text-blue-800'
                            : 'bg-gray-100 text-gray-700'
                        }`}
                        onClick={(e) => { e.stopPropagation(); setSelectedStep(step.id) }}
                      >
                        {step.name}
                      </div>
                    ))}
                  </div>
                  
                  <button
                    onClick={(e) => { e.stopPropagation(); addStep(stage.id) }}
                    className="mt-2 text-sm text-blue-500 hover:text-blue-700"
                  >
                    + Add Step
                  </button>
                </div>
                
                {index < stages.length - 1 && (
                  <div className="text-gray-400 mx-2">→</div>
                )}
              </div>
            ))}
          </div>
        </div>
        
        {/* Step Details */}
        {selectedStep && (
          <div className="w-80 border-l pl-4">
            <h3 className="font-semibold mb-2">Step Details</h3>
            {stages.map(stage => {
              const step = stage.steps.find(s => s.id === selectedStep)
              if (!step) return null
              
              return (
                <div key={stage.id} className="space-y-3">
                  <div>
                    <label className="block text-sm text-gray-600 mb-1">Name</label>
                    <input
                      type="text"
                      value={step.name}
                      onChange={(e) => updateStep(stage.id, step.id, { name: e.target.value })}
                      className="w-full border rounded px-2 py-1 text-sm"
                    />
                  </div>
                  
                  <div>
                    <label className="block text-sm text-gray-600 mb-1">Type</label>
                    <select
                      value={step.type}
                      onChange={(e) => updateStep(stage.id, step.id, { type: e.target.value as any })}
                      className="w-full border rounded px-2 py-1 text-sm"
                    >
                      <option value="shell">Shell</option>
                      <option value="git">Git</option>
                      <option value="notify">Notify</option>
                    </select>
                  </div>
                  
                  {step.type === 'shell' && (
                    <div>
                      <label className="block text-sm text-gray-600 mb-1">Command</label>
                      <input
                        type="text"
                        value={step.command || ''}
                        onChange={(e) => updateStep(stage.id, step.id, { command: e.target.value })}
                        className="w-full border rounded px-2 py-1 text-sm font-mono"
                        placeholder="npm run build"
                      />
                    </div>
                  )}
                  
                  <button
                    onClick={() => deleteStep(stage.id, step.id)}
                    className="text-red-500 hover:text-red-700 text-sm"
                  >
                    Delete Step
                  </button>
                </div>
              )
            })}
          </div>
        )}
      </div>
      
      {/* Generated Code Preview */}
      <div className="mt-6 border-t pt-4">
        <h3 className="font-semibold mb-2">Generated Pipeline Code</h3>
        <pre className="bg-gray-900 text-green-400 p-4 rounded text-sm overflow-x-auto">
{`pipeline({
  name: "my-pipeline",
  stages: [
${stages.map(stage => `    {
      name: "${stage.name}",
      steps: [
${stage.steps.map(step => `        { name: "${step.name}", type: "${step.type}"${step.command ? `, command: "${step.command}"` : ''} }`).join(',\n')}
      ]
    }`).join(',\n')}
  ]
});`}
        </pre>
      </div>
    </div>
  )
}
