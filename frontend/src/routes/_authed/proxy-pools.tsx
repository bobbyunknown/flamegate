import { createFileRoute } from "@tanstack/react-router";
import { ProxyPoolsPage } from "../../pages/ProxyPools";

export const Route = createFileRoute("/_authed/proxy-pools")({
  component: ProxyPoolsPage,
});
