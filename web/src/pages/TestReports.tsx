import { useRef, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useI18n } from '../i18n'
import { api } from '../api'
import { useApi } from '../hooks'

function fmtDuration(ms?: number): string {
  if (!ms) return '-'
  return `${(ms / 1000).toFixed(2)}s`
}

function Stat({ label, value, color }: { label: string; value: number; color: string }) {
  return (
    <div className="bg-white p-4 rounded-lg shadow border">
      <p className="text-sm text-gray-500">{label}</p>
      <p className={`text-2xl font-bold ${color}`}>{value}</p>
    </div>
  )
}

export default function TestReports() {
  const { t } = useI18n()
  const { id } = useParams<{ id: string }>()
  const buildId = Number(id)
  const { data, loading, error, reload } = useApi(() => api.getBuildTestResults(buildId), [buildId])
  const [uploading, setUploading] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    setUploading(true)
    try {
      const text = await file.text()
      await api.uploadTestResults(buildId, text)
      reload()
    } catch (err: any) {
      alert(err.message)
    } finally {
      setUploading(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  if (loading) return <div className="text-gray-500">{t('common.loading')}</div>
  if (error) return <div className="text-red-500">{t('common.error')}: {error}</div>

  const summary = data?.summary || data
  const passed = summary?.passed ?? summary?.tests ?? 0
  const failed = summary?.failed ?? summary?.failures ?? 0
  const skipped = summary?.skipped ?? summary?.skips ?? 0
  const total = summary?.total ?? (passed + failed + skipped)
  const cases: any[] = data?.cases || data?.test_cases || data?.testsuites || []

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h1 className="text-2xl font-bold">{t('testReports.title')}</h1>
        <div className="flex items-center gap-3">
          <input ref={fileRef} type="file" accept=".xml,text/xml" onChange={handleUpload} className="hidden" id="test-upload" />
          <label
            htmlFor="test-upload"
            className={`inline-block cursor-pointer bg-blue-500 text-white px-4 py-2 rounded ${uploading ? 'opacity-50 pointer-events-none' : ''}`}
          >
            {uploading ? t('common.loading') : t('testReports.upload')}
          </label>
          <Link to={`/builds/${buildId}`} className="text-blue-600 hover:underline text-sm">← #{buildId}</Link>
        </div>
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
        <Stat label={t('testReports.total')} value={total} color="text-gray-900" />
        <Stat label={t('testReports.passed')} value={passed} color="text-green-600" />
        <Stat label={t('testReports.failed')} value={failed} color="text-red-600" />
        <Stat label={t('testReports.skipped')} value={skipped} color="text-yellow-600" />
      </div>

      <div className="bg-white shadow rounded-lg">
        <h2 className="text-lg font-semibold p-4 border-b">{t('testReports.testCases')}</h2>
        {cases.length === 0 ? (
          <p className="text-gray-500 text-center py-6">{t('common.noData')}</p>
        ) : (
          <table className="min-w-full">
            <thead>
              <tr className="border-b">
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">{t('testReports.name')}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">{t('builds.status')}</th>
                <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">{t('testReports.duration')}</th>
              </tr>
            </thead>
            <tbody>
              {cases.map((c: any, i: number) => {
                const status = c.status || (c.failure ? 'failed' : 'passed')
                return (
                  <tr key={i} className="border-b">
                    <td className="px-4 py-3 text-sm font-medium">{c.name || c.testname || c.classname || '-'}</td>
                    <td className="px-4 py-3 text-sm">
                      <span className={`px-2 py-0.5 rounded ${status === 'passed' ? 'bg-green-100 text-green-800' : status === 'failed' ? 'bg-red-100 text-red-800' : 'bg-yellow-100 text-yellow-800'}`}>
                        {status}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-500">{fmtDuration(c.time ? c.time * 1000 : c.duration_ms)}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
