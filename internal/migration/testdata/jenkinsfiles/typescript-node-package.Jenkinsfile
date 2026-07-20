pipeline {
    agent {
        label 'linux-node'
    }
    environment {
        NODE_ENV = "production"
        NPM_CONFIG_CACHE = "${env.WORKSPACE}/.npm-cache"
    }
    stages {
        stage('Install') {
            steps {
                dir('web') {
                    sh 'npm ci'
                }
            }
        }
        stage('Test') {
            steps {
                dir('web') {
                    sh(script: '''npm run lint
npm test -- --run
''')
                }
            }
        }
        stage('Package') {
            steps {
                dir('web') {
                    sh 'npm run build && tar -czf web-dist.tgz dist'
                }
            }
        }
    }
    post {
        always {
            echo 'Node package pipeline finished'
        }
    }
}
