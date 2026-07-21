import { definePipeline, parameter, shell, stage, trigger, watchService } from '@buildworld/pipeline'

export default definePipeline({
  name: 'web-release',
  environment: { NODE_ENV: 'production' },
  parameters: [parameter('target', 'choice', { choices: ['staging', 'production'], default: 'staging' })],
  on: [trigger('cron', { expression: '0 2 * * *' })],
  stages: [
    stage('Verify', shell('Unit tests', 'npm test')),
    stage('Release', shell('Deploy', './scripts/deploy.sh'), { dependsOn: ['Verify'] }),
    // watchService is automatically long-running. Explicit pipeline/stage
    // timeoutSec still limits it.
    stage('Service log monitor', watchService('Follow server output', { targetDir: '/srv/web-release', pidFile: 'server.pid', logFile: 'server.log', port: 8700, initialLines: 0 })),
  ],
})
