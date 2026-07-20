import { message } from '../src/index.js'

if (message('matrix') !== 'hello, matrix') {
  throw new Error('compiled TypeScript package returned an unexpected value')
}

console.log('typescript smoke test passed')
