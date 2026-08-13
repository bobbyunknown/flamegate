import { createFileRoute } from "@tanstack/react-router";
import { SettingsPage } from "../../pages/Settings";

export const Route = createFileRoute("/_authed/settings")({
  component: SettingsPage,
});
