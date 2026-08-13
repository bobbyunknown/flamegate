import { createFileRoute } from "@tanstack/react-router";
import { OverviewPage } from "../../pages/Overview";

export const Route = createFileRoute("/_authed/")({
  component: OverviewPage,
});
