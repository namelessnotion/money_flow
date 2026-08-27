import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";

// Set when running behind the docker/proxy nginx container, which serves the
// dev server under a different hostname (and, over HTTPS on 443, a different
// port) than the one Vite listens on internally — both the host-check
// allowlist and the HMR websocket need to be told about that externally
// visible host. Plain HTTP always redirects to HTTPS in nginx.conf, so 443
// is the only external port there is to report back.
const devHost = process.env.VITE_DEV_HOST;

// https://vite.dev/config/
export default defineConfig({
  plugins: [tailwindcss(), vue()],
  server: {
    host: true,
    allowedHosts: devHost ? [devHost] : undefined,
    proxy: {
      "/graphql": {
        target: process.env.VITE_RUBY_SERVER_URL ?? "http://localhost:9292",
        changeOrigin: true,
      },
    },
    hmr: devHost ? { host: devHost, protocol: "wss", clientPort: 443 } : undefined,
  },
});
