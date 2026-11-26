package engine

import "regexp"

// ExecutionGroupVariable applies only to variables defined on a project.
// Build parameters, global variables, and pipeline code cannot override queue admission.
const ExecutionGroupVariable = "BUILDWORLD_EXECUTION_GROUP"

var executionGroupPattern = regexp.MustCompile("^[a-z0-9][a-z0-9._-]{0,63}$")

func (r *BuildRunner) resolveExecutionGroup(projectID int64) (string, bool, error) {
	variables, err := r.store.ListEnvVars("project", &projectID)
	if err != nil {
		return "", false, err
	}
	for _, variable := range variables {
		if variable.Name != ExecutionGroupVariable || variable.ProjectID == nil || *variable.ProjectID != projectID {
			continue
		}
		if variable.IsSecret || !executionGroupPattern.MatchString(variable.Value) {
			return "", false, nil
		}
		return variable.Value, true, nil
	}
	return "", true, nil
}
