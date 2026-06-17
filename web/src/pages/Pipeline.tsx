import { useI18n } from '../i18n'
import PipelineEditor from '../components/PipelineEditor'

export default function Pipeline() {
  const { t } = useI18n()

  return (
    <div>
      <h1 className="text-2xl font-bold mb-4">Pipeline Editor</h1>
      <PipelineEditor />
    </div>
  )
}
