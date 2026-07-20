pipeline {
    agent {
        label 'mac-unity'
    }
    environment {
        UNITY_PATH = "/Applications/Unity/Hub/Editor/2022.3.45f1/Unity.app/Contents/MacOS/Unity"
        PROJECT_PATH = "${env.WORKSPACE}/game-client"
    }
    stages {
        stage('Validate project') {
            steps {
                sh """
                    "${UNITY_PATH}" -batchmode -nographics -quit -projectPath "${PROJECT_PATH}" -executeMethod CI.Validate
                """
            }
        }
        stage('Run EditMode tests') {
            steps {
                sh """
                    "${UNITY_PATH}" -batchmode -nographics -quit -projectPath "${PROJECT_PATH}" -runTests -testPlatform EditMode -testResults artifacts/editmode.xml
                """
            }
        }
        stage('Build client') {
            steps {
                sh """
                    "${UNITY_PATH}" -batchmode -nographics -quit -projectPath "${PROJECT_PATH}" -executeMethod CI.BuildLinux64
                """
            }
        }
    }
    post {
        cleanup {
            sh 'rm -rf "${PROJECT_PATH}/Library/Bee"'
        }
    }
}
