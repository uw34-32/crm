const { defineConfig, globalIgnores } = require('eslint/config');
const expo = require('eslint-config-expo/flat');

module.exports = defineConfig([
  expo,
  globalIgnores(['dist/*', '.expo/*', 'src-tauri/target/*', 'coverage/*']),
  {
    // The React Compiler rules flag patterns this code uses on purpose (a "latest value"
    // ref, resetting state when a prop changes) and the app does not use the compiler.
    // The rules of hooks and exhaustive-deps stay on.
    rules: {
      'react-hooks/refs': 'off',
      'react-hooks/set-state-in-effect': 'off',
      'react-hooks/immutability': 'off',
      'react-hooks/use-memo': 'off'
    }
  },
  {
    // jest.mock calls sit before the imports they affect.
    files: ['__tests__/**'],
    rules: { 'import/first': 'off' }
  }
]);
