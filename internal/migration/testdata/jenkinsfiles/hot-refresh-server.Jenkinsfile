pipeline {
    agent any

    environment {
        // 项目配置
        TARGET_DIR = "/Users/gamer/Desktop/Code/DevOps-Projects/jenkins-for-project-sf/server-project-sf-go"
        GIT_REPO_URL = "http://192.168.110.42:3000/poke-game/server-project-sf-go.git"

        // 服务器配置
        PORT = "10101"
        BINARY_NAME = "server-game-sf"
        LOG_FILE = "logs_game_server/server.log"

        // Go 环境变量
        PATH = "${env.PATH}:/usr/local/go/bin:/opt/homebrew/bin:/opt/homebrew/opt/go/bin"
    }

    stages {

        stage('飞书通知') {
            steps {
                script {
                    echo "================= 发送飞书通知 ================="
                    sh '''
                        cd /Users/gamer/Desktop/Code/DevOps-Projects/jenkins-for-project-sf/feishu-robot
                        python3 feishu-robot-game-server-config-refresh.py || echo "⚠️ 飞书通知发送失败（继续执行）"
                    '''
                }
            }
        }

        stage('更新 Team-Resources') {
            steps {
                script {
                    echo "================= 更新 Team-Resources ================="
                    sh """
                        cd ${TARGET_DIR}
                        chmod +x ./_scripts/deploy/update-team-resources.sh
                        ./_scripts/deploy/update-team-resources.sh
                    """
                }
            }
        }

        stage('校验 BusinessConfig') {
            steps {
                sh """
                    cd ${TARGET_DIR}
                    python3 .deploy/logic-server/validate-business-config.py --config-dir Team-Resources/BusinessConfig --registry server_game/auto_generate/business_config/config_registry.go
                """
            }
        }
    }

    post {
        always {
            script {
                echo "================= 构建流程结束 ================="
            }
        }
        success {
            echo "✅ 服务器启动成功"
        }
        failure {
            echo "❌ 构建失败，请检查日志"
        }
        cleanup {
            script {
                // 清理临时文件
                echo "清理临时文件..."
            }
        }
    }
}
