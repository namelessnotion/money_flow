// src/main.ts
import { DefaultApolloClient } from "@vue/apollo-composable";
import { createApp } from "vue";
import { apolloClient } from "./apollo";
import "./style.css";
import App from "./App.vue";

const app = createApp(App);
app.provide(DefaultApolloClient, apolloClient);
app.mount("#app");
