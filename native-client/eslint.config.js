const { defineConfig, globalIgnores } = require('eslint/config');
const expo = require('eslint-config-expo/flat');

module.exports = defineConfig([
  expo,
  globalIgnores(['dist/*', '.expo/*', 'src-tauri/target/*', 'coverage/*']),
  {
    // These files use patterns the React Compiler rules of eslint-plugin-react-hooks
    // flag, on purpose and without the compiler: a ref holding the latest callback, a
    // ref read to build a pager once, state reset when a prop changes. The rules stay on
    // everywhere else, so a new use of those patterns has to be listed here.
    files: [
      'src/lib/records/hooks.ts',
      'src/components/ActivityTimeline.tsx',
      'src/components/RecordPicker.tsx',
      'src/components/DealForm.tsx',
      'src/app/(app)/(tabs)/pipeline.tsx',
      'src/app/(app)/contacts/new.tsx',
      'src/app/(app)/deals/new.tsx',
      'src/app/(app)/deals/[[]id]/index.tsx',
      'src/app/(app)/companies/[[]id]/index.tsx'
    ],
    rules: {
      'react-hooks/refs': 'off',
      'react-hooks/set-state-in-effect': 'off',
      'react-hooks/immutability': 'off',
      'react-hooks/use-memo': 'off'
    }
  },
  {
    // jest.mock calls sit before the imports they affect.
    files: ['__tests__/hooks.test.tsx', '__tests__/dealForm.test.tsx'],
    rules: { 'import/first': 'off' }
  }
]);
