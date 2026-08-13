import { createFileRoute } from "@tanstack/react-router";
import { CLIToolDetailPage } from "../../pages/CLIToolDetail";

export const Route = createFileRoute("/_authed/cli-tools/$toolId")({
  component: CLIToolDetailPage,
});
