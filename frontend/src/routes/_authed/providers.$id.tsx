import { createFileRoute } from "@tanstack/react-router";
import { ProviderDetailPage } from "../../pages/ProviderDetail";

export const Route = createFileRoute("/_authed/providers/$id")({
  component: ProviderDetailPage,
});
