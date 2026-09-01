import { copyFile, mkdir } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptDirectory = dirname(fileURLToPath(import.meta.url));
const source = resolve(scriptDirectory, "../dist/index.html");
const target = resolve(scriptDirectory, "../../web/dist/index.html");

await mkdir(dirname(target), { recursive: true });
await copyFile(source, target);
