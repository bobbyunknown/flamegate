import { createFileRoute } from "@tanstack/react-router";
import { ConsoleLogPage } from "../../pages/ConsoleLog";

export const Route = createFileRoute("/_authed/console")({
  component: ConsoleLogPage,
});
