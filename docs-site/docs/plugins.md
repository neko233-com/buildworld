---
sidebar_position: 5
---

# Plugin Development

## Overview

Plugins extend buildworld233 functionality. Plugins are written in TypeScript/JavaScript and run in the goja runtime.

## Plugin Structure

```
plugins/
├── my-plugin/
│   ├── plugin.json      # Plugin metadata
│   ├── index.js         # Plugin entry point
│   └── package.json     # Dependencies (optional)
```

## Plugin Metadata

```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "A custom plugin"
}
```

## Plugin Entry Point

```javascript
// index.js
function onLoad() {
  console.log("Plugin loaded!");
  
  // Register custom steps
  return {
    steps: {
      "my-step": function(config) {
        // Custom step implementation
        console.log("Executing my-step with config:", config);
      }
    },
    triggers: {
      "my-trigger": function(event) {
        // Custom trigger implementation
        console.log("Triggered:", event);
      }
    }
  };
}

function onUnload() {
  console.log("Plugin unloaded!");
}

exports.onLoad = onLoad;
exports.onUnload = onUnload;
```

## Registering Custom Steps

```javascript
// index.js
function onLoad() {
  return {
    steps: {
      "notify-slack": function(config) {
        const { channel, message } = config;
        // Send Slack notification
        console.log(`Sending to ${channel}: ${message}`);
      },
      
      "deploy-s3": function(config) {
        const { bucket, source } = config;
        // Deploy to S3
        console.log(`Deploying ${source} to s3://${bucket}`);
      }
    }
  };
}
```

## Using Custom Steps in Pipeline

```typescript
// buildworld.config.ts
pipeline({
  name: "my-build",
  stages: [
    {
      name: "Deploy",
      steps: [
        {
          name: "notify-slack",
          type: "my-step",
          config: {
            channel: "#builds",
            message: "Build completed!"
          }
        }
      ]
    }
  ]
});
```

## Hot Reload

Plugins are automatically reloaded when files change:

1. Plugin files are watched by fsnotify
2. On change: unload old plugin → reload new plugin
3. Zero downtime — running builds continue with old version

## Plugin API

### Available APIs

```javascript
function onLoad() {
  return {
    // Pipeline API
    pipeline: {
      // Access pipeline configuration
    },
    
    // Build API
    build: {
      // Access build information
    },
    
    // Notification API
    notify: {
      slack: function(channel, message) { /* ... */ },
      email: function(to, subject, body) { /* ... */ },
      webhook: function(url, data) { /* ... */ }
    },
    
    // Environment API
    env: {
      get: function(name) { /* ... */ },
      set: function(name, value) { /* ... */ }
    }
  };
}
```

## Best Practices

1. **Keep plugins focused** - One plugin, one responsibility
2. **Handle errors gracefully** - Catch and log errors
3. **Use meaningful names** - Clear step and trigger names
4. **Document your plugin** - Add description and usage examples
5. **Test your plugin** - Verify it works in different scenarios

## Next Steps

- [Pipeline Guide](/pipelines) - Use plugins in pipelines
- [Templates](/templates) - Create plugin-based templates
