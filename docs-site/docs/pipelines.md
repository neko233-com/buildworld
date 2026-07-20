---
sidebar_position: 4
---

# Pipeline Guide

## Overview

buildworld uses TypeScript/JavaScript for pipeline definitions. Pipelines are written in `buildworld.config.ts` or `buildworld.config.js` files.

## Basic Pipeline

```typescript
// buildworld.config.ts
pipeline({
  name: "my-app-build",
  stages: [
    {
      name: "Checkout",
      steps: [
        git.clone("https://github.com/user/repo", { depth: 1 }),
      ],
    },
    {
      name: "Build",
      steps: [
        shell("npm ci"),
        shell("npm run build"),
      ],
    },
    {
      name: "Test",
      steps: [
        shell("npm test"),
      ],
    },
  ],
});
```

## Parameterized Builds

```typescript
pipeline({
  name: "deploy-app",
  parameters: [
    {
      name: "environment",
      type: "choice",
      description: "Target environment",
      choices: ["development", "staging", "production"],
      default: "staging",
      required: true,
    },
    {
      name: "version",
      type: "string",
      description: "Version to deploy",
      default: "latest",
    },
    {
      name: "api_key",
      type: "password",
      description: "API key for deployment",
      required: true,
      is_secret: true,
    },
  ],
  stages: [
    {
      name: "Deploy",
      steps: [
        shell("deploy.sh --env ${parameter.environment} --version ${parameter.version}"),
      ],
    },
  ],
});
```

## Environment Variables

```typescript
pipeline({
  name: "build-with-env",
  stages: [
    {
      name: "Build",
      steps: [
        shell("echo ${global.REGISTRY}"),
        shell("echo ${project.API_KEY}"),
      ],
    },
  ],
});
```

## Conditional Execution

```typescript
pipeline({
  name: "conditional-build",
  stages: [
    {
      name: "Deploy",
      steps: [
        shell("deploy.sh"),
      ],
      when: {
        branch: "main",
        condition: "success",
      },
    },
  ],
});
```

## Parallel Stages

```typescript
pipeline({
  name: "parallel-build",
  stages: [
    {
      name: "Test",
      steps: [
        shell("npm test"),
      ],
      parallel: true,
    },
    {
      name: "Lint",
      steps: [
        shell("npm run lint"),
      ],
      parallel: true,
    },
  ],
});
```

## Post Actions

```typescript
pipeline({
  name: "build-with-notifications",
  stages: [...],
  post: {
    always: [
      shell("echo 'Build finished'"),
    ],
    failure: [
      notify.slack("#builds", "Build failed!"),
    ],
    success: [
      notify.slack("#builds", "Build succeeded!"),
    ],
  },
});
```

## Templates

Use pre-built templates for common workflows:

```typescript
// Use a template
import { nodeTypescript } from "buildworld/templates";

pipeline(nodeTypescript({
  node_version: "20",
  build_command: "npm run build",
  test_command: "npm test",
}));
```

## Next Steps

- [Configuration](/configuration) - Configure variables and settings
- [Plugin Development](/plugins) - Write custom plugins
- [Agents](/agents) - Configure build agents
