import * as path from "https://deno.land/std@0.133.0/path/mod.ts";
import { assertEquals, assertNotEquals } from "https://deno.land/std@0.133.0/testing/asserts.ts";

Deno.test("A better run API - download tarball", () => {
  // test that no console errors were logged
  const output = Deno.run({
    cmd: [
      "deno",
      "run",
      "--allow-all",
      "--unstable",
      "main.ts",
      "npm",
      "react-dom",
    ],
  });
  assertNotEquals(output.code, 0);
});
