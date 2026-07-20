pipeline {
    agent {
        label 'linux-dotnet'
    }
    environment {
        DOTNET_CLI_TELEMETRY_OPTOUT = "1"
        CONFIGURATION = "Release"
    }
    stages {
        stage('Restore') {
            steps {
                sh 'dotnet restore BuildWorld.sln'
            }
        }
        stage('Test') {
            steps {
                sh """
                    dotnet test BuildWorld.sln --configuration ${CONFIGURATION} --no-restore
                """
            }
        }
        stage('Publish') {
            steps {
                sh """
                    dotnet publish src/BuildWorld/BuildWorld.csproj --configuration ${CONFIGURATION} --output artifacts/app
                """
            }
        }
    }
}
