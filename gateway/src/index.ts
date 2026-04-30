import { createApp } from "./app.ts";
import { loadConfig } from "./config.ts";

const config = loadConfig();
const app = createApp(config).listen(config.port);

console.log(`api-gateway listening on ${app.server?.url}`);
