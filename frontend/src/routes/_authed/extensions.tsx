import { createFileRoute } from "@tanstack/react-router";
import { ExtensionsPage } from "../../pages/Extensions";

export const Route = createFileRoute("/_authed/extensions")({
  component: ExtensionsPage,
});
