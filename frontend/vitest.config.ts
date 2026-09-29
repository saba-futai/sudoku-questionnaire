import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    include: ['src/**/*.spec.ts'],
    // The codec tests read spec/vectors from the repository root and spawn the
    // Go conformance binary; both are outside the Vite root.
    server: { deps: { inline: [] } },
    testTimeout: 120_000,
  },
});
