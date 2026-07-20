import { definePipeline, parameter, shell, stage, trigger, watchService } from '@buildworld/pipeline'

export default definePipeline({
  name: 'web-release',
  environment: { NODE_ENV: 'production' },
  parameters: [parameter('target', 'choice', { choices: ['staging', 'production'], default: 'staging' })],
  on: [trigger('cron', { expression: '0 2 * * *' })],
  allowLongRunning: true,
  stages: [
    stage('Verify', shell('Unit tests', 'npm test')),
    stage('Release', shell('Deploy', './scripts/deploy.sh'), { dependsOn: ['Verify'] }),
    stage('Service log monitor', watchService('Follow server output', { targetDir: '/srv/web-release', pidFile: 'server.pid', logFile: 'server.log', port: 8700 })),
  ],
})
