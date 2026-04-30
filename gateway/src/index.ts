import { stdoutLogger } from "./access-log.ts";
import { createApp } from "./app.ts";
import { loadConfig } from "./config.ts";

const config = loadConfig();
const app = createApp(config, undefined, stdoutLogger).listen(config.port);

console.log(`api-gateway listening on ${app.server?.url}`);
