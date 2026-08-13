import { createFileRoute } from "@tanstack/react-router";
import { SystemPage } from "../../pages/System";

export const Route = createFileRoute("/_authed/system")({
  component: SystemPage,
});
