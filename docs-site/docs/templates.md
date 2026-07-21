---
sidebar_position: 7
---

# Build templates

Build templates are reusable pipeline sources stored by BuildWorld. A template
contains a name, description, and a validated TypeScript or GitHub Actions-style YAML
pipeline configuration.

Templates are instance data. The current release does not expose a
`buildworld/templates` TypeScript module, built-in framework catalog, or
`buildworld template …` CLI commands.

## Create a template

1. Open **Templates** in the BuildWorld navigation.
2. Select **New template**.
3. Enter a name and optional description.
4. Write a TypeScript or GitHub Actions-style YAML pipeline.
5. Validate and save it.

TypeScript templates use the same restricted, typed API as project pipelines:

```typescript
import { definePipeline, shell, stage } from '@buildworld/pipeline'

export default definePipeline({
  name: 'Go validation',
  agentRequirements: ['go'],
  stages: [
    stage('Verify', [
      shell('Format', 'gofmt -w .'),
      shell('Test', 'go test ./...'),
      shell('Build', 'go build ./...'),
    ]),
  ],
})
```

The Monaco editor provides comments, completion, syntax diagnostics, and live
server validation. BuildWorld parses this source as declarative data; it does
not execute it as JavaScript.

## Use a template

Select **Use** on the Templates page, or choose a template while creating a
project. The project keeps a template reference and can provide its own
pipeline values. At build time BuildWorld validates the template and project
configuration before merging them.

As a practical rule, put shared environment, parameters, triggers, policies,
and default stages in the template. Keep repository-specific commands and
deployment details in the project.

## Authenticated API

Template management is also available to administrators and developers:

```text
GET    /api/templates/
POST   /api/templates/
GET    /api/templates/{id}
PUT    /api/templates/{id}
DELETE /api/templates/{id}
```

Create and update requests use this shape:

```json
{
  "name": "Go validation",
  "description": "Shared Go checks",
  "config": "import { definePipeline } from '@buildworld/pipeline'\n\nexport default definePipeline({ stages: [] })\n"
}
```

Invalid pipeline source is rejected with HTTP `422`.

## Next steps

- [Pipeline Guide](./pipelines.md) - Author TypeScript and YAML pipelines
- [Go binary plugins](./plugins.md) - Add controlled pipeline step types
