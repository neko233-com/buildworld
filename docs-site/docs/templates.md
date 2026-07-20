---
sidebar_position: 7
---

# Templates

## Overview

Templates provide pre-built pipeline configurations for common workflows. Use templates to quickly set up builds for popular frameworks.

## Built-in Templates

### Languages

| Template | Description | Difficulty |
|----------|-------------|------------|
| Node.js TypeScript | Build, test, deploy Node.js TS projects | Beginner |
| Go CLI | Build Go CLI applications | Beginner |
| Python Django | Build Python Django apps | Intermediate |

### Platforms

| Template | Description | Difficulty |
|----------|-------------|------------|
| Docker Build | Build and push Docker images | Intermediate |
| Kubernetes Deploy | Deploy to K8s cluster | Advanced |

### Game Dev

| Template | Description | Difficulty |
|----------|-------------|------------|
| Unity Android | Build Unity games for Android | Advanced |

### Frontend

| Template | Description | Difficulty |
|----------|-------------|------------|
| React (Vercel) | Deploy React apps to Vercel | Beginner |

## Using Templates

### From UI

1. Go to Projects → New Project
2. Select "From Template"
3. Choose a template
4. Customize configuration
5. Create project

### From Code

```typescript
// buildworld.config.ts
import { nodeTypescript } from "buildworld/templates";

pipeline(nodeTypescript({
  node_version: "20",
  build_command: "npm run build",
  test_command: "npm test",
}));
```

## Template Configuration

Each template accepts parameters:

```typescript
// Node.js TypeScript template parameters
{
  node_version: "20" | "22" | "24",
  build_command: "npm run build",
  test_command: "npm test",
  lint_command: "npm run lint"
}
```

## Custom Templates

### Create Template

```typescript
// buildworld.config.ts
const myTemplate = {
  id: "my-template",
  name: "My Custom Template",
  description: "A custom build template",
  category: "custom",
  difficulty: "beginner",
  tags: ["custom", "my-app"],
  config: {
    name: "my-build",
    stages: [
      {
        name: "Build",
        steps: [
          { name: "build", type: "shell", command: "make build" }
        ]
      }
    ]
  }
};

pipeline(myTemplate);
```

### Export/Import

```bash
# Export template
buildworld template export my-template

# Import template
buildworld template import template.json
```

## Template Search

```bash
# Search templates
buildworld template search "docker"

# List all templates
buildworld template list
```

## Next Steps

- [Pipeline Guide](/pipelines) - Use templates in pipelines
- [Plugin Development](/plugins) - Create plugin-based templates
