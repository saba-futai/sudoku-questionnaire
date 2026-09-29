/**
 * Pinned codec digests, mirroring backend/internal/codec/spec_pins.go.
 *
 * The browser verifies the shipped validity bitmap against
 * PINNED_VALIDITY_SHA256 before it will derive a table from it: a swapped or
 * truncated artifact would otherwise produce a table that only decodes some
 * inputs, which is exactly the silent corruption the whole conformance effort
 * exists to prevent.
 *
 * A test asserts these equal the values in spec/vectors, so they cannot drift
 * from the Go side unnoticed.
 */

/** sha256 over the 288 grids in enumeration order. */
export const PINNED_GRIDS_SHA256 =
  '2865b433b40cedd051916982f14f4d4bb26680b65070b1d1354cc54e7f778b67';

/** sha256 over the 1820 position sets. */
export const PINNED_POSSETS_SHA256 =
  '32385ff1df7b77c831a6f5562e9fedd9a27c662a06e6ae6613571c431a1bb005';

/** sha256 over the validity bitmap artifact. */
export const PINNED_VALIDITY_SHA256 =
  '813d1d73a9bfbcbf947fc850d3d43daf1326432336599b0cf7d052b6dad567c1';

/** Where the bitmap is served from. */
export const VALIDITY_URL = '/codec/validity.bin';
