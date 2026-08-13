import { createFileRoute } from "@tanstack/react-router";
import { MediaProvidersPage } from "../../pages/MediaProviders";

export const Route = createFileRoute("/_authed/media")({
  component: MediaProvidersPage,
});
