pipeline {
    agent any
    options {
        timeout(time: 20, unit: 'MINUTES')
        disableConcurrentBuilds()
    }
    environment {
        MAVEN_OPTS = "-Dmaven.repo.local=${env.WORKSPACE}/.m2"
        ARTIFACT_DIR = "target"
    }
    stages {
        stage('Compile') {
            steps {
                sh(script: 'mvn -B -DskipTests compile')
            }
        }
        stage('Unit tests') {
            steps {
                sh '''
                    mvn -B test
                '''
            }
        }
        stage('Package JAR') {
            steps {
                sh """
                    mvn -B -DskipTests package
                    test -f ${ARTIFACT_DIR}/*.jar
                """
            }
        }
    }
}
