pipeline {
  agent any
  options { disableConcurrentBuilds() }
  parameters {
    string(name: 'RELEASE_VERSION', defaultValue: '', description: 'Release version with a matching changelog entry')
    string(name: 'SOURCE_COMMIT', defaultValue: '', description: 'Exact reviewed Git commit to build and deploy')
  }
  stages {
    stage('Build and deploy BuildWorld') {
      steps {
        sh '''
set -eu
if [ "$(uname -s):$(uname -m)" = 'Darwin:arm64' ]; then
  tools="${BUILDWORLD_RELEASE_TOOLS:-$HOME/.local/share/buildworld-release/toolchains/pwsh-7.6.9-node-24.22.2-gh-2.103.3-gtar-1.35}"
  export PATH="$tools/pwsh:$tools/node/bin:$tools/gnu/bin:$tools/gh/gh_2.103.3_macOS_arm64/bin:$PATH"
fi
exec go run ./cmd/selfrelease
'''
      }
    }
  }
}
