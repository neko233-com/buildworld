import { useState } from 'react'
import { useI18n } from '../i18n'

export default function CreateProject() {
  const { t } = useI18n()
  const [step, setStep] = useState<'template' | 'config'>('template')
  const [selectedTemplate, setSelectedTemplate] = useState<string | null>(null)

  const templates = [
    { id: 'node-typescript', name: 'Node.js TypeScript', category: 'languages', difficulty: 'beginner', icon: '📦' },
    { id: 'go-cli', name: 'Go CLI', category: 'languages', difficulty: 'beginner', icon: '🔧' },
    { id: 'python-django', name: 'Python Django', category: 'languages', difficulty: 'intermediate', icon: '🐍' },
    { id: 'docker-build', name: 'Docker Build', category: 'platforms', difficulty: 'intermediate', icon: '🐳' },
    { id: 'k8s-deploy', name: 'Kubernetes Deploy', category: 'platforms', difficulty: 'advanced', icon: '☸️' },
    { id: 'unity-android', name: 'Unity Android', category: 'game-dev', difficulty: 'advanced', icon: '🎮' },
    { id: 'react-vercel', name: 'React (Vercel)', category: 'frontend', difficulty: 'beginner', icon: '⚛️' },
    { id: 'blank', name: 'Blank Project', category: 'custom', difficulty: 'beginner', icon: '📄' },
  ]

  const handleTemplateSelect = (templateId: string) => {
    setSelectedTemplate(templateId)
    setStep('config')
  }

  return (
    <div>
      <h1 className="text-2xl font-bold mb-4">{t('projects.newProject')}</h1>

      {step === 'template' && (
        <div>
          <p className="text-gray-600 mb-4">Select a template to get started:</p>
          
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            {templates.map((template) => (
              <div
                key={template.id}
                className={`bg-white border rounded-lg p-4 cursor-pointer hover:border-blue-500 transition ${
                  selectedTemplate === template.id ? 'border-blue-500 bg-blue-50' : 'border-gray-200'
                }`}
                onClick={() => handleTemplateSelect(template.id)}
              >
                <div className="text-3xl mb-2">{template.icon}</div>
                <h3 className="font-semibold">{template.name}</h3>
                <p className="text-sm text-gray-500">{template.category}</p>
                <span className={`inline-block mt-2 px-2 py-1 rounded text-xs ${
                  template.difficulty === 'beginner' ? 'bg-green-100 text-green-800' :
                  template.difficulty === 'intermediate' ? 'bg-yellow-100 text-yellow-800' :
                  'bg-red-100 text-red-800'
                }`}>
                  {template.difficulty}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}

      {step === 'config' && (
        <div className="bg-white shadow rounded-lg p-6">
          <div className="flex items-center gap-2 mb-4">
            <button 
              className="text-blue-500 hover:underline"
              onClick={() => setStep('template')}
            >
              ← Back to templates
            </button>
            <span className="text-gray-400">|</span>
            <span className="text-gray-600">
              Template: {templates.find(t => t.id === selectedTemplate)?.name}
            </span>
          </div>

          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">{t('projects.name')}</label>
              <input 
                type="text" 
                placeholder="my-project"
                className="w-full border rounded px-3 py-2"
              />
            </div>
            
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Description</label>
              <textarea 
                placeholder="A brief description of your project"
                className="w-full border rounded px-3 py-2"
                rows={3}
              />
            </div>
            
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Repository URL</label>
              <input 
                type="text" 
                placeholder="https://github.com/user/repo"
                className="w-full border rounded px-3 py-2"
              />
            </div>
            
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Default Branch</label>
              <input 
                type="text" 
                defaultValue="main"
                className="w-full border rounded px-3 py-2"
              />
            </div>

            <div className="flex gap-2 pt-4">
              <button className="bg-blue-500 text-white px-6 py-2 rounded">
                {t('common.save')}
              </button>
              <button 
                className="bg-gray-300 text-gray-700 px-6 py-2 rounded"
                onClick={() => setStep('template')}
              >
                {t('common.cancel')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
