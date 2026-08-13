import { createFileRoute } from "@tanstack/react-router";
import { ProvidersPage } from "../../pages/Providers";

export const Route = createFileRoute("/_authed/providers/")({
  component: ProvidersPage,
});
