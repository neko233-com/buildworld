package engine

import (
	"fmt"
	"strconv"
)

const testEmptyPipelineSource = `import { definePipeline } from "@buildworld/pipeline"
export default definePipeline({ stages: [] })`

func testShellPipelineSource(stageName, stepName, command string) string {
	return fmt.Sprintf(`import { definePipeline, shell, stage } from "@buildworld/pipeline"
export default definePipeline({ stages: [stage(%s, shell(%s, %s))] })`,
		strconv.Quote(stageName), strconv.Quote(stepName), strconv.Quote(command))
}
