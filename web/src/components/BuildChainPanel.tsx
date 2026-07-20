import { ArrowRight, GitBranch, GitFork, Pin, TriangleAlert } from 'lucide-react'
import { Link } from 'react-router-dom'
import { useI18n } from '../i18n'
import { buildStatusLabel, buildStatusTone } from '../lib/buildPresentation'
import { buildChainRelationKey, type BuildChain, type BuildChainNode } from '../lib/buildChain'

function ChainNode({ node }: { node: BuildChainNode }) {
  const { t } = useI18n()
  const label = `${node.project_name} ${t('builds.build')} #${node.number} ${buildStatusLabel(t, node.status)}${node.focus ? ` ${t('builds.currentBuild')}` : ''}`
  return <Link className={`build-chain-node ${node.focus ? 'focus' : ''}`} to={`/builds/${node.id}`} aria-label={label}>
    <span className={`chain-node-status ${buildStatusTone(node.status)}`} aria-hidden="true" />
    <div>
      <small>{node.project_name || `#${node.project_id}`}</small>
      <strong>{node.pinned && <Pin size={11} />}#{node.number}</strong>
      <em><GitBranch size={11} />{node.branch || '-'}</em>
    </div>
    <span className={`build-status ${buildStatusTone(node.status)}`}>{buildStatusLabel(t, node.status)}</span>
    {node.focus && <b>{t('builds.currentBuild')}</b>}
  </Link>
}

export default function BuildChainPanel({ chain }: { chain: BuildChain }) {
  const { t } = useI18n()
  if (chain.nodes.length < 2 || chain.edges.length === 0) return null
  const nodes = new Map(chain.nodes.map(node => [node.id, node]))

  return <section id="build-chain" className="build-chain-panel" aria-label={t('builds.buildChain')}>
    <header>
      <div><GitFork size={17} /><div><h2>{t('builds.buildChain')}</h2><p>{t('builds.buildChainHelp')}</p></div></div>
      <strong>{t('builds.chainSummary').replace('{builds}', String(chain.nodes.length)).replace('{relations}', String(chain.edges.length))}</strong>
    </header>
    <div className="build-chain-relations">
      {chain.edges.map(edge => {
        const from = nodes.get(edge.from_build_id)
        const to = nodes.get(edge.to_build_id)
        if (!from || !to) return null
        return <article key={`${edge.from_build_id}-${edge.to_build_id}-${edge.type}`}>
          <ChainNode node={from} />
          <div className={`chain-connector ${edge.type}`}>
            <span>{t(buildChainRelationKey(edge.type))}</span>
            <i aria-hidden="true" /><ArrowRight size={15} />
          </div>
          <ChainNode node={to} />
        </article>
      })}
    </div>
    {chain.truncated && <footer><TriangleAlert size={14} />{t('builds.chainTruncated')}</footer>}
  </section>
}
