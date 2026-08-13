import { createFileRoute } from "@tanstack/react-router";
import { MediaProviderDetailPage } from "../../pages/MediaProviderDetail";

export const Route = createFileRoute("/_authed/media/$kind/$id")({
  component: MediaProviderDetailPage,
});
