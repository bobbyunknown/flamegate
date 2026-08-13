import { createFileRoute } from "@tanstack/react-router";
import { KeyDetailPage } from "../../pages/KeyDetail";

export const Route = createFileRoute("/_authed/keys/$id")({
  component: KeyDetailPage,
});
