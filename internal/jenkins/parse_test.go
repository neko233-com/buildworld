package jenkins

import "testing"

func TestParseConfigNormalizesJenkinsBranchSpec(t *testing.T) {
	definition, err := ParseConfig([]byte(`
<flow-definition>
  <definition class="org.jenkinsci.plugins.workflow.cps.CpsScmFlowDefinition">
    <scm class="hudson.plugins.git.GitSCM">
      <userRemoteConfigs><hudson.plugins.git.UserRemoteConfig><url>https://example.invalid/game.git</url></hudson.plugins.git.UserRemoteConfig></userRemoteConfigs>
      <branches><hudson.plugins.git.BranchSpec><name>*/main</name></hudson.plugins.git.BranchSpec></branches>
    </scm>
    <scriptPath>server_game/.jenkins/Jenkinsfile_GameServer</scriptPath>
  </definition>
</flow-definition>`), "server-game-go")
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if definition.SCMBranch != "main" || definition.DefaultBranch != "main" {
		t.Fatalf("branches = (%q, %q), want main", definition.SCMBranch, definition.DefaultBranch)
	}
}
