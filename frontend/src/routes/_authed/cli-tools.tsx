import { createFileRoute } from "@tanstack/react-router";
import { CLIToolsPage } from "../../pages/CLITools";

export const Route = createFileRoute("/_authed/cli-tools")({
  component: CLIToolsPage,
});
